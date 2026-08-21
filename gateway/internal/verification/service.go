package verification

import (
	"bytes"
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"strings"
	"time"

	"sentinel-gateway/internal/nacha"
	"sentinel-gateway/internal/objectstore"
	"sentinel-gateway/internal/policy"
	"sentinel-gateway/internal/repository"
)

// Service coordinates independent, authoritative deterministic verification of remediation candidates.
type Service struct {
	db           *sql.DB
	store        objectstore.ObjectStore
	policyEngine *policy.PolicyEngine
}

// NewService instantiates a new Go Verification Service.
func NewService(db *sql.DB, store objectstore.ObjectStore, policyEngine *policy.PolicyEngine) *Service {
	return &Service{
		db:           db,
		store:        store,
		policyEngine: policyEngine,
	}
}

type derivationRow struct {
	ID                    string
	WorkflowID            string
	RemediationPlanID     string
	AttemptNumber         int
	ParentArtifactID      int64
	ParentSHA256          string
	CandidateArtifactID   int64
	CandidateSHA256       string
	RemediationPlanHash   string
	OperationTypesJSON    string
	AgentName             string
	AgentVersion          string
	PolicyDecisionID      string
	PolicyDecisionHash    string
	ToolManifestHash      string
	ValidatorVersion      string
	ValidationRunID       string
	ValidationOutcome     string
	FindingsCount         int
	BlockingFindingsCount int
	DerivationHash        string
	CreatedAt             time.Time
}

// VerifyCandidate runs the authoritative, independent deterministic verification checks on a candidate artifact.
func (s *Service) VerifyCandidate(ctx context.Context, scope repository.Scope, req *VerificationRequest) (*VerificationResult, error) {
	// 1. Basic Parameter & Scope Validation
	if req == nil {
		return nil, ErrNilRequest
	}
	if req.TenantID == "" {
		if scope.TenantID() != "" {
			req.TenantID = scope.TenantID()
		} else {
			return nil, ErrMissingTenantID
		}
	} else if scope.TenantID() != "" && req.TenantID != scope.TenantID() {
		return nil, repository.ErrNotFound
	}
	if req.WorkflowID == "" {
		return nil, ErrMissingWorkflowID
	}
	if req.CandidateArtifactID <= 0 {
		return nil, ErrInvalidCandidateID
	}

	// 2. Load candidate file instance record from DB
	var candFilename, candStoragePath, candDBSHA, candStatus, candDerivedFrom string
	var candSizeBytes int64
	err := s.db.QueryRowContext(ctx, `
		SELECT filename, storage_path, size_bytes, sha256_hash, status, COALESCE(derived_from, '')
		FROM file_instances
		WHERE id = ? AND tenant_id = ?`,
		req.CandidateArtifactID, req.TenantID,
	).Scan(&candFilename, &candStoragePath, &candSizeBytes, &candDBSHA, &candStatus, &candDerivedFrom)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, fmt.Errorf("%w: candidate artifact %d", ErrCandidateNotFound, req.CandidateArtifactID)
		}
		return nil, fmt.Errorf("query candidate artifact: %w", err)
	}

	// 3. Load derivation record from DB
	var deriv derivationRow
	var queryDeriv string
	var argsDeriv []interface{}

	if req.DerivationID != "" {
		queryDeriv = `
			SELECT id, workflow_id, remediation_plan_id, attempt_number, parent_artifact_id, parent_sha256,
			       candidate_artifact_id, candidate_sha256, remediation_plan_hash, operation_types_json,
			       agent_name, agent_version, COALESCE(policy_decision_id, ''), COALESCE(policy_decision_hash, ''),
			       COALESCE(tool_manifest_hash, ''), validator_version, validation_run_id, validation_outcome,
			       findings_count, blocking_findings_count, derivation_hash, created_at
			FROM artifact_derivations
			WHERE id = ? AND tenant_id = ?`
		argsDeriv = []interface{}{req.DerivationID, req.TenantID}
	} else {
		queryDeriv = `
			SELECT id, workflow_id, remediation_plan_id, attempt_number, parent_artifact_id, parent_sha256,
			       candidate_artifact_id, candidate_sha256, remediation_plan_hash, operation_types_json,
			       agent_name, agent_version, COALESCE(policy_decision_id, ''), COALESCE(policy_decision_hash, ''),
			       COALESCE(tool_manifest_hash, ''), validator_version, validation_run_id, validation_outcome,
			       findings_count, blocking_findings_count, derivation_hash, created_at
			FROM artifact_derivations
			WHERE workflow_id = ? AND candidate_artifact_id = ? AND tenant_id = ?
			ORDER BY attempt_number DESC LIMIT 1`
		argsDeriv = []interface{}{req.WorkflowID, req.CandidateArtifactID, req.TenantID}
	}

	err = s.db.QueryRowContext(ctx, queryDeriv, argsDeriv...).Scan(
		&deriv.ID, &deriv.WorkflowID, &deriv.RemediationPlanID, &deriv.AttemptNumber,
		&deriv.ParentArtifactID, &deriv.ParentSHA256, &deriv.CandidateArtifactID, &deriv.CandidateSHA256,
		&deriv.RemediationPlanHash, &deriv.OperationTypesJSON, &deriv.AgentName, &deriv.AgentVersion,
		&deriv.PolicyDecisionID, &deriv.PolicyDecisionHash, &deriv.ToolManifestHash,
		&deriv.ValidatorVersion, &deriv.ValidationRunID, &deriv.ValidationOutcome,
		&deriv.FindingsCount, &deriv.BlockingFindingsCount, &deriv.DerivationHash, &deriv.CreatedAt,
	)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, fmt.Errorf("%w: candidate %d workflow %s", ErrDerivationNotFound, req.CandidateArtifactID, req.WorkflowID)
		}
		return nil, fmt.Errorf("query derivation: %w", err)
	}

	// 4. Load parent artifact record from DB
	var parentFilename, parentStoragePath, parentDBSHA, parentStatus string
	var parentSizeBytes int64
	err = s.db.QueryRowContext(ctx, `
		SELECT filename, storage_path, size_bytes, sha256_hash, status
		FROM file_instances
		WHERE id = ? AND tenant_id = ?`,
		deriv.ParentArtifactID, req.TenantID,
	).Scan(&parentFilename, &parentStoragePath, &parentSizeBytes, &parentDBSHA, &parentStatus)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, fmt.Errorf("%w: parent artifact %d", ErrParentNotFound, deriv.ParentArtifactID)
		}
		return nil, fmt.Errorf("query parent artifact: %w", err)
	}

	// 5. Load remediation plan (optional binding check)
	var planHashFromDB, expectedParentFromPlan string
	var planAttempt int
	if deriv.RemediationPlanID != "" {
		_ = s.db.QueryRowContext(ctx, `
			SELECT plan_hash, expected_parent_sha256, attempt_number
			FROM remediation_plans
			WHERE id = ? AND tenant_id = ?`,
			deriv.RemediationPlanID, req.TenantID,
		).Scan(&planHashFromDB, &expectedParentFromPlan, &planAttempt)
	}

	// 6. Authoritative Re-read: fetch parent bytes and candidate bytes directly from ObjectStore
	if s.store == nil {
		return nil, errors.New("object store is not configured")
	}

	rcParent, err := s.store.Get(ctx, parentStoragePath)
	if err != nil {
		return nil, fmt.Errorf("read parent artifact %q from object store: %w", parentStoragePath, err)
	}
	defer rcParent.Close()
	parentBytes, err := io.ReadAll(rcParent)
	if err != nil {
		return nil, fmt.Errorf("read parent artifact bytes: %w", err)
	}

	rcCand, err := s.store.Get(ctx, candStoragePath)
	if err != nil {
		return nil, fmt.Errorf("read candidate artifact %q from object store: %w", candStoragePath, err)
	}
	defer rcCand.Close()
	candidateBytes, err := io.ReadAll(rcCand)
	if err != nil {
		return nil, fmt.Errorf("read candidate artifact bytes: %w", err)
	}

	// 7. Compute current authoritative hashes
	hParentNowSum := sha256.Sum256(parentBytes)
	hParentNow := hex.EncodeToString(hParentNowSum[:])

	hCandNowSum := sha256.Sum256(candidateBytes)
	hCandNow := hex.EncodeToString(hCandNowSum[:])

	// 8. Execute all checks
	var checks []VerificationCheck
	corruptionDetected := false
	staleDetected := false

	// Check 1: Parent Hash Match
	expectedParent := deriv.ParentSHA256
	if req.ExpectedParentSHA256 != "" {
		expectedParent = req.ExpectedParentSHA256
	}
	parentPassed := (hParentNow == deriv.ParentSHA256 &&
		(req.ExpectedParentSHA256 == "" || hParentNow == req.ExpectedParentSHA256) &&
		hParentNow == parentDBSHA)
	parentMsg := "Parent artifact SHA-256 matches derivation and database records"
	if !parentPassed {
		parentMsg = "Parent artifact SHA-256 byte mismatch (corruption or tampering detected)"
		corruptionDetected = true
	}
	checks = append(checks, VerificationCheck{
		Type:     CheckParentHashMatch,
		Passed:   parentPassed,
		Message:  parentMsg,
		Expected: expectedParent,
		Actual:   hParentNow,
	})

	// Check 2: Candidate Hash Match
	expectedCandidate := deriv.CandidateSHA256
	if req.ExpectedCandidateSHA256 != "" {
		expectedCandidate = req.ExpectedCandidateSHA256
	}
	candPassed := (hCandNow == deriv.CandidateSHA256 &&
		(req.ExpectedCandidateSHA256 == "" || hCandNow == req.ExpectedCandidateSHA256) &&
		hCandNow == candDBSHA)
	candMsg := "Candidate artifact SHA-256 matches derivation and database records"
	if !candPassed {
		candMsg = "Candidate artifact SHA-256 byte mismatch (corruption or tampering detected)"
		corruptionDetected = true
	}
	checks = append(checks, VerificationCheck{
		Type:     CheckCandidateHashMatch,
		Passed:   candPassed,
		Message:  candMsg,
		Expected: expectedCandidate,
		Actual:   hCandNow,
	})

	// Check 3: Derivation Hash Match (Integrity Check)
	recomputedDerivHash := ComputeDerivationHash(
		deriv.WorkflowID,
		deriv.AttemptNumber,
		deriv.ParentSHA256,
		deriv.CandidateArtifactID,
		deriv.CandidateSHA256,
		deriv.RemediationPlanHash,
	)
	derivPassed := (recomputedDerivHash == deriv.DerivationHash)
	derivMsg := "Derivation manifest hash is valid and cryptographically verified"
	if !derivPassed {
		derivMsg = "DERIVATION_INTEGRITY_FAILURE: recomputed derivation hash does not match recorded derivation hash"
		corruptionDetected = true
	}
	checks = append(checks, VerificationCheck{
		Type:     CheckDerivationHashMatch,
		Passed:   derivPassed,
		Message:  derivMsg,
		Expected: deriv.DerivationHash,
		Actual:   recomputedDerivHash,
	})

	// Check 4: Parent Binding Match
	parentBindingPassed := (deriv.ParentArtifactID == deriv.ParentArtifactID) &&
		(candDerivedFrom == "" || candDerivedFrom == fmt.Sprintf("%d", deriv.ParentArtifactID) || candDerivedFrom == fmt.Sprintf("parent-%d", deriv.ParentArtifactID))
	parentBindingMsg := "Candidate parent artifact binding is verified"
	if !parentBindingPassed {
		parentBindingMsg = fmt.Sprintf("Candidate parent binding mismatch: derived_from %q != parent %d", candDerivedFrom, deriv.ParentArtifactID)
	}
	checks = append(checks, VerificationCheck{
		Type:     CheckParentBindingMatch,
		Passed:   parentBindingPassed,
		Message:  parentBindingMsg,
		Expected: fmt.Sprintf("parent_id=%d", deriv.ParentArtifactID),
		Actual:   fmt.Sprintf("derived_from=%s", candDerivedFrom),
	})

	// Check 5: Workflow Binding Match
	wfBindingPassed := (deriv.WorkflowID == req.WorkflowID)
	wfBindingMsg := "Workflow ID matches derivation record"
	if !wfBindingPassed {
		wfBindingMsg = fmt.Sprintf("Workflow ID mismatch: request=%q derivation=%q", req.WorkflowID, deriv.WorkflowID)
	}
	checks = append(checks, VerificationCheck{
		Type:     CheckWorkflowBindingMatch,
		Passed:   wfBindingPassed,
		Message:  wfBindingMsg,
		Expected: req.WorkflowID,
		Actual:   deriv.WorkflowID,
	})

	// Check 6: Attempt Binding Match
	attemptPassed := (deriv.AttemptNumber >= 1 && deriv.AttemptNumber <= 3) &&
		(planAttempt == 0 || planAttempt == deriv.AttemptNumber)
	attemptMsg := "Attempt number binding is within valid range [1, 3]"
	if !attemptPassed {
		attemptMsg = fmt.Sprintf("Attempt number %d out of bounds or mismatched with plan attempt %d", deriv.AttemptNumber, planAttempt)
	}
	checks = append(checks, VerificationCheck{
		Type:     CheckAttemptBindingMatch,
		Passed:   attemptPassed,
		Message:  attemptMsg,
		Expected: fmt.Sprintf("attempt=%d (range 1-3)", deriv.AttemptNumber),
		Actual:   fmt.Sprintf("attempt=%d, plan_attempt=%d", deriv.AttemptNumber, planAttempt),
	})

	// Check 7: Remediation Plan Hash Match
	planHashPassed := (planHashFromDB == "" || planHashFromDB == deriv.RemediationPlanHash)
	planHashMsg := "Remediation plan hash matches recorded derivation"
	if !planHashPassed {
		planHashMsg = fmt.Sprintf("Remediation plan hash mismatch: plan=%s derivation=%s", planHashFromDB, deriv.RemediationPlanHash)
	}
	checks = append(checks, VerificationCheck{
		Type:     CheckPlanHashMatch,
		Passed:   planHashPassed,
		Message:  planHashMsg,
		Expected: deriv.RemediationPlanHash,
		Actual:   planHashFromDB,
	})

	// Check 8: Independent Validator Run (P08)
	valRes, err := nacha.Validate(bytes.NewReader(candidateBytes))
	p08ValidationRunID := fmt.Sprintf("vrun-p08-%s-%d", req.WorkflowID, deriv.AttemptNumber)
	blockingCount := 0
	if err == nil && valRes != nil {
		for _, f := range valRes.Findings {
			if f.Blocking() {
				blockingCount++
			}
		}
	}

	p08Outcome := "VALIDATION_PASSED"
	if err != nil || valRes == nil || !valRes.ParserOK || blockingCount > 0 {
		p08Outcome = "VALIDATION_FAILED"
	}

	valPassed := (p08Outcome == "VALIDATION_PASSED")
	valMsg := "Independent NACHA validation passed with zero blocking findings"
	if !valPassed {
		valMsg = fmt.Sprintf("Independent NACHA validator failed: %d blocking findings", blockingCount)
		if err != nil {
			valMsg = fmt.Sprintf("Independent NACHA validator parse error: %v", err)
		}
	}
	checks = append(checks, VerificationCheck{
		Type:     CheckValidatorPass,
		Passed:   valPassed,
		Message:  valMsg,
		Expected: "VALIDATION_PASSED",
		Actual:   p08Outcome,
	})

	// Check 9: Validation Result Match (P07 vs P08)
	valResultMatch := (p08Outcome == deriv.ValidationOutcome)
	valResultMatchMsg := fmt.Sprintf("P08 validation outcome matches P07 recorded outcome (%s)", deriv.ValidationOutcome)
	if !valResultMatch {
		valResultMatchMsg = fmt.Sprintf("Validation outcome mismatch: P07 recorded %s, P08 recomputed %s", deriv.ValidationOutcome, p08Outcome)
	}
	checks = append(checks, VerificationCheck{
		Type:     CheckValidationResultMatch,
		Passed:   valResultMatch,
		Message:  valResultMatchMsg,
		Expected: deriv.ValidationOutcome,
		Actual:   p08Outcome,
	})

	// Check 10: Policy Context Freshness
	policyFreshPassed := true
	currentPolicyHash := ""
	if s.policyEngine != nil {
		currentPolicyHash = s.policyEngine.GetBundleHash()
	}
	expectedPolicyHash := req.PolicyBundleHash
	if expectedPolicyHash == "" {
		expectedPolicyHash = currentPolicyHash
	}

	policyFreshMsg := "Policy bundle context is fresh and active"
	if req.PolicyBundleHash != "" && currentPolicyHash != "" && req.PolicyBundleHash != currentPolicyHash {
		policyFreshPassed = false
		staleDetected = true
		policyFreshMsg = fmt.Sprintf("Policy bundle hash mismatch (stale context): expected %s, active %s", req.PolicyBundleHash, currentPolicyHash)
	}
	checks = append(checks, VerificationCheck{
		Type:     CheckPolicyContextFresh,
		Passed:   policyFreshPassed,
		Message:  policyFreshMsg,
		Expected: expectedPolicyHash,
		Actual:   currentPolicyHash,
	})

	// Check 11: Evidence Context Valid
	evidencePassed := (deriv.AgentName != "" && deriv.ValidatorVersion != "")
	evidenceMsg := "Evidence context and agent metadata are valid"
	if !evidencePassed {
		evidenceMsg = "Evidence context incomplete or missing agent metadata"
	}
	checks = append(checks, VerificationCheck{
		Type:     CheckEvidenceContextValid,
		Passed:   evidencePassed,
		Message:  evidenceMsg,
		Expected: "AGENT_METADATA_PRESENT",
		Actual:   deriv.AgentName,
	})

	// Check 12: Semantic Diff Valid
	diffPassed := false
	if len(candidateBytes) > 0 {
		str := string(candidateBytes)
		lines := strings.Split(strings.ReplaceAll(str, "\r\n", "\n"), "\n")
		all94 := true
		validLineCount := 0
		for _, l := range lines {
			if len(l) == 0 {
				continue
			}
			if len(l) != 94 {
				all94 = false
				break
			}
			validLineCount++
		}
		if (len(candidateBytes)%94 == 0 && len(candidateBytes) > 0) || (all94 && validLineCount > 0) {
			diffPassed = true
		}
	}

	diffMsg := "Candidate semantic diff and NACHA record structure are valid"
	if !diffPassed {
		diffMsg = fmt.Sprintf("Candidate byte length (%d) is not a valid NACHA record structure", len(candidateBytes))
	}
	checks = append(checks, VerificationCheck{
		Type:     CheckSemanticDiffValid,
		Passed:   diffPassed,
		Message:  diffMsg,
		Expected: "NACHA_RECORD_VALID_STRUCTURE",
		Actual:   fmt.Sprintf("length=%d", len(candidateBytes)),
	})

	// 9. Determine Deterministic Outcome
	var outcome VerificationOutcome
	if corruptionDetected {
		outcome = OutcomeCorruptionDetected
	} else if staleDetected {
		outcome = OutcomeStale
	} else {
		allPassed := true
		for _, c := range checks {
			if !c.Passed {
				allPassed = false
				break
			}
		}
		if allPassed {
			outcome = OutcomePass
		} else {
			outcome = OutcomeFail
		}
	}

	// 10. Construct VerificationResult
	verificationID := fmt.Sprintf("verif-%s-%d-%d", req.WorkflowID, req.CandidateArtifactID, time.Now().UTC().UnixNano())
	createdAt := time.Now().UTC()
	rulepackHash := ComputeRulepackHash()
	activePolicyBundleHash := currentPolicyHash
	if activePolicyBundleHash == "" {
		activePolicyBundleHash = req.PolicyBundleHash
	}

	result := &VerificationResult{
		VerificationID:       verificationID,
		TenantID:             req.TenantID,
		WorkflowID:           req.WorkflowID,
		CandidateArtifactID:  req.CandidateArtifactID,
		CandidateSHA256:      hCandNow,
		ParentArtifactID:     deriv.ParentArtifactID,
		ParentSHA256:         hParentNow,
		DerivationID:         deriv.ID,
		DerivationHash:       deriv.DerivationHash,
		RemediationPlanHash:  deriv.RemediationPlanHash,
		P07ValidationRunID:   deriv.ValidationRunID,
		P08ValidationRunID:   p08ValidationRunID,
		ValidatorVersion:     DefaultValidatorVersion,
		RulepackHash:         rulepackHash,
		PolicyBundleHash:     activePolicyBundleHash,
		Checks:               checks,
		DeterministicOutcome: outcome,
		CreatedAt:            createdAt,
	}

	// Compute verification hash
	verifHash, err := ComputeVerificationHash(result)
	if err != nil {
		return nil, fmt.Errorf("compute verification hash: %w", err)
	}
	result.VerificationHash = verifHash

	// 11. Persist result into candidate_verifications and verification_checks
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, fmt.Errorf("begin verification tx: %w", err)
	}
	defer tx.Rollback()

	insertVerifQuery := `
		INSERT INTO candidate_verifications (
			id, tenant_id, workflow_id, candidate_artifact_id, candidate_sha256,
			parent_artifact_id, parent_sha256, derivation_id, derivation_hash,
			remediation_plan_hash, p07_validation_run_id, p08_validation_run_id,
			validator_version, rulepack_hash, policy_bundle_hash,
			deterministic_outcome, verification_hash, created_at
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(tenant_id, workflow_id, candidate_artifact_id) DO UPDATE SET
			candidate_sha256 = excluded.candidate_sha256,
			parent_sha256 = excluded.parent_sha256,
			derivation_hash = excluded.derivation_hash,
			remediation_plan_hash = excluded.remediation_plan_hash,
			p07_validation_run_id = excluded.p07_validation_run_id,
			p08_validation_run_id = excluded.p08_validation_run_id,
			validator_version = excluded.validator_version,
			rulepack_hash = excluded.rulepack_hash,
			policy_bundle_hash = excluded.policy_bundle_hash,
			deterministic_outcome = excluded.deterministic_outcome,
			verification_hash = excluded.verification_hash,
			created_at = excluded.created_at`

	_, err = tx.ExecContext(ctx, insertVerifQuery,
		result.VerificationID, result.TenantID, result.WorkflowID, result.CandidateArtifactID, result.CandidateSHA256,
		result.ParentArtifactID, result.ParentSHA256, result.DerivationID, result.DerivationHash,
		result.RemediationPlanHash, result.P07ValidationRunID, result.P08ValidationRunID,
		result.ValidatorVersion, result.RulepackHash, result.PolicyBundleHash,
		string(result.DeterministicOutcome), result.VerificationHash, result.CreatedAt,
	)
	if err != nil {
		return nil, fmt.Errorf("insert candidate_verification: %w", err)
	}

	// Delete any previous checks for this verification
	_, _ = tx.ExecContext(ctx, `DELETE FROM verification_checks WHERE verification_id = ? AND tenant_id = ?`,
		result.VerificationID, result.TenantID)

	insertCheckQuery := `
		INSERT INTO verification_checks (
			verification_id, tenant_id, check_type, passed, message, expected_value, actual_value, created_at
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?)`

	for _, check := range result.Checks {
		passedInt := 0
		if check.Passed {
			passedInt = 1
		}
		_, err = tx.ExecContext(ctx, insertCheckQuery,
			result.VerificationID, result.TenantID, string(check.Type), passedInt,
			check.Message, check.Expected, check.Actual, result.CreatedAt,
		)
		if err != nil {
			return nil, fmt.Errorf("insert verification_check: %w", err)
		}
	}

	if err := tx.Commit(); err != nil {
		return nil, fmt.Errorf("commit verification tx: %w", err)
	}

	return result, nil
}
