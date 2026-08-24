package candidate

import (
	"bytes"
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math"
	"strconv"
	"strings"
	"time"

	"sentinel-gateway/internal/nacha"
	"sentinel-gateway/internal/objectstore"
	"sentinel-gateway/internal/policy"
	"sentinel-gateway/internal/repository"
)

var (
	ErrParentSHA256Mismatch   = errors.New("parent artifact SHA-256 mismatch (resource context stale)")
	ErrMaxAttemptsExceeded    = errors.New("maximum candidate remediation attempts (3) exceeded")
	ErrInvalidOperation       = errors.New("unsupported or disallowed remediation operation")
	ErrAmbiguousTarget        = errors.New("ambiguous or nonexistent semantic target reference")
	ErrCandidateExists        = errors.New("candidate attempt already exists for workflow and attempt number")
	ErrPolicyObligationFailed = errors.New("remediation policy obligation verification failed")
	ErrOriginalMutated        = errors.New("CRITICAL SAFETY VIOLATION: original artifact bytes modified during candidate generation")
	ErrIdempotencyConflict    = errors.New("idempotency conflict: attempt already executed with different parameters")
)

// Allowed Remediation Operation Types for P07
const (
	OpRecomputeBatchControlTotal = "RECOMPUTE_BATCH_CONTROL_TOTAL"
	OpRecomputeFileControlTotal  = "RECOMPUTE_FILE_CONTROL_TOTAL"
)

// RemediationOperation defines a typed repair intent proposed by RemediationAgent.
type RemediationOperation struct {
	OperationType string   `json:"operation_type"`
	TargetRef     string   `json:"target_ref"`
	FindingRefs   []string `json:"finding_refs"`
	Rationale     string   `json:"rationale"`
	EvidenceRefs  []string `json:"evidence_refs"`
}

// CandidateCreationRequest specifies the parameters for generating an immutable candidate.
type CandidateCreationRequest struct {
	TenantID             string                 `json:"tenant_id"`
	WorkflowID           string                 `json:"workflow_id"`
	IncidentID           int64                  `json:"incident_id"`
	ParentArtifactID     int64                  `json:"parent_artifact_id"`
	ExpectedParentSHA256 string                 `json:"expected_parent_sha256"`
	AttemptNumber        int                    `json:"attempt_number"`
	PlanHash             string                 `json:"plan_hash"`
	Operations           []RemediationOperation `json:"operations"`
	FindingRefs          []string               `json:"finding_refs"`
	Confidence           string                 `json:"confidence"`
	AgentName            string                 `json:"agent_name"`
	AgentVersion         string                 `json:"agent_version"`
	PolicyDecisionID     string                 `json:"policy_decision_id"`
	PolicyDecisionHash   string                 `json:"policy_decision_hash"`
	ToolManifestHash     string                 `json:"tool_manifest_hash"`
}

// CandidateResult represents the authoritative result of candidate generation and revalidation.
type CandidateResult struct {
	DerivationID          string          `json:"derivation_id"`
	WorkflowID            string          `json:"workflow_id"`
	AttemptNumber         int             `json:"attempt_number"`
	ParentArtifactID      int64           `json:"parent_artifact_id"`
	ParentSHA256          string          `json:"parent_sha256"`
	CandidateArtifactID   int64           `json:"candidate_artifact_id"`
	CandidateSHA256       string          `json:"candidate_sha256"`
	RemediationPlanHash   string          `json:"remediation_plan_hash"`
	ValidationOutcome     string          `json:"validation_outcome"` // "VALIDATION_PASSED", "VALIDATION_FAILED"
	FindingsCount         int             `json:"findings_count"`
	BlockingFindingsCount int             `json:"blocking_findings_count"`
	Findings              []nacha.Finding `json:"findings"`
	DerivationHash        string          `json:"derivation_hash"`
	DiffSummary           string          `json:"diff_summary"`
	CreatedAt             time.Time       `json:"created_at"`
}

// Service manages the authoritative creation, derivation tracking, and deterministic validation of candidates.
type Service struct {
	db     *sql.DB
	repo   *repository.Repository
	store  objectstore.ObjectStore
	engine *policy.PolicyEngine
}

// NewService creates a new Go Candidate Service.
func NewService(db *sql.DB, store objectstore.ObjectStore, engine *policy.PolicyEngine) *Service {
	return &Service{
		db:     db,
		repo:   repository.New(db),
		store:  store,
		engine: engine,
	}
}

// GenerateCandidate executes governed candidate creation from an immutable original artifact.
func (s *Service) GenerateCandidate(ctx context.Context, scope repository.Scope, req *CandidateCreationRequest) (*CandidateResult, error) {
	// 1. Validate inputs and attempt bounds (Max 3 attempts)
	if req.AttemptNumber < 1 || req.AttemptNumber > 3 {
		return nil, ErrMaxAttemptsExceeded
	}
	if len(req.Operations) == 0 {
		return nil, errors.New("at least one remediation operation is required")
	}

	// 1a. Idempotency & Conflict Check
	var existingParentSHA, existingPlanHash, existingCandSHA, existingValidation, existingDerivationHash, existingDerivID string
	var existingCandID int64
	var existingFindingsCount, existingBlockingCount int
	errCheck := s.db.QueryRowContext(ctx, `
		SELECT id, parent_sha256, remediation_plan_hash, candidate_artifact_id, candidate_sha256, validation_outcome, findings_count, blocking_findings_count, derivation_hash
		FROM artifact_derivations 
		WHERE tenant_id = ? AND workflow_id = ? AND attempt_number = ?`,
		req.TenantID, req.WorkflowID, req.AttemptNumber,
	).Scan(
		&existingDerivID, &existingParentSHA, &existingPlanHash, &existingCandID, &existingCandSHA, &existingValidation, &existingFindingsCount, &existingBlockingCount, &existingDerivationHash,
	)
	if errCheck == nil {
		if existingParentSHA != req.ExpectedParentSHA256 {
			return nil, fmt.Errorf("%w: expected %s, existing is %s", ErrParentSHA256Mismatch, req.ExpectedParentSHA256, existingParentSHA)
		}
		if existingPlanHash != req.PlanHash {
			return nil, fmt.Errorf("%w: attempt %d already executed with plan hash %s", ErrIdempotencyConflict, req.AttemptNumber, existingPlanHash)
		}

		return &CandidateResult{
			DerivationID:          existingDerivID,
			WorkflowID:            req.WorkflowID,
			AttemptNumber:         req.AttemptNumber,
			ParentArtifactID:      req.ParentArtifactID,
			ParentSHA256:          existingParentSHA,
			CandidateArtifactID:   existingCandID,
			CandidateSHA256:       existingCandSHA,
			RemediationPlanHash:   existingPlanHash,
			ValidationOutcome:     existingValidation,
			FindingsCount:         existingFindingsCount,
			BlockingFindingsCount: existingBlockingCount,
			Findings:              nil,
			DerivationHash:        existingDerivationHash,
			DiffSummary:           "Replayed from idempotency check",
			CreatedAt:             time.Now().UTC(),
		}, nil
	} else if !errors.Is(errCheck, sql.ErrNoRows) {
		return nil, fmt.Errorf("query artifact_derivations for idempotency check: %w", errCheck)
	}

	// 2. Fetch original parent artifact record from database
	var parentStoragePath, parentFilename, currentParentSHA string
	var parentSize int64
	query := `SELECT filename, storage_path, size_bytes, sha256_hash FROM file_instances WHERE id = ? AND tenant_id = ?`
	err := s.db.QueryRowContext(ctx, query, req.ParentArtifactID, req.TenantID).Scan(
		&parentFilename, &parentStoragePath, &parentSize, &currentParentSHA,
	)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, fmt.Errorf("parent artifact %d not found: %w", req.ParentArtifactID, repository.ErrNotFound)
		}
		return nil, fmt.Errorf("query parent artifact: %w", err)
	}

	// 3. Strict Precondition Check: Expected parent SHA-256 == Current parent SHA-256
	if currentParentSHA != req.ExpectedParentSHA256 {
		return nil, fmt.Errorf("%w: expected %s, current is %s", ErrParentSHA256Mismatch, req.ExpectedParentSHA256, currentParentSHA)
	}

	// 4. Policy Engine Obligation Enforcement (SF-SAFE-004)
	if s.engine != nil {
		evalReq := &policy.PolicyEvaluationRequest{
			RequestID: fmt.Sprintf("req-cand-%s-%d", req.WorkflowID, req.AttemptNumber),
			TenantID:  req.TenantID,
			Action:    policy.ActionCreateCandidate,
			Subject: policy.PolicySubject{
				ID:            req.AgentName,
				Type:          "AGENT",
				Roles:         []string{"REMEDIATION_AGENT"},
				TenantID:      req.TenantID,
				AutonomyLevel: 2, // A2 (Sandbox Candidate Creation)
			},
			Resource: policy.PolicyResource{
				ID:       fmt.Sprintf("artifact-%d", req.ParentArtifactID),
				Type:     "ARTIFACT",
				SHA256:   req.ExpectedParentSHA256,
				State:    "QUARANTINED",
				TenantID: req.TenantID,
			},
			Workflow: policy.PolicyWorkflowContext{
				WorkflowID: req.WorkflowID,
				State:      "REMEDIATING",
				Attempt:    req.AttemptNumber,
			},
			Environment: policy.PolicyEnvironment{
				EvaluationTime: time.Now().UTC(),
				FleetMode:      "ACTIVE",
			},
		}
		decision, err := s.engine.Evaluate(evalReq)
		if err != nil || decision.Decision == policy.DecisionDeny {
			return nil, fmt.Errorf("%w: policy evaluation denied candidate creation: %v", ErrPolicyObligationFailed, err)
		}

		// Enforce MaxAttempts obligation parameter
		for _, obl := range decision.Obligations {
			if obl.Type == policy.ObligationMaxAttempts {
				if maxCount, ok := obl.Parameters["count"].(int); ok && req.AttemptNumber > maxCount {
					return nil, ErrMaxAttemptsExceeded
				}
				if maxCountF, ok := obl.Parameters["count"].(float64); ok && req.AttemptNumber > int(maxCountF) {
					return nil, ErrMaxAttemptsExceeded
				}
			}
		}
	}

	// 5. Load original artifact bytes from ObjectStore
	rc, err := s.store.Get(ctx, parentStoragePath)
	if err != nil {
		return nil, fmt.Errorf("read parent artifact from object store: %w", err)
	}
	defer rc.Close()

	originalBytes, err := io.ReadAll(rc)
	if err != nil {
		return nil, fmt.Errorf("read original bytes: %w", err)
	}

	// 6. Compute H_before = SHA256(original)
	hBefore := sha256.Sum256(originalBytes)
	hBeforeHex := hex.EncodeToString(hBefore[:])
	if hBeforeHex != currentParentSHA {
		return nil, fmt.Errorf("%w: object store hash %s != db hash %s", ErrParentSHA256Mismatch, hBeforeHex, currentParentSHA)
	}

	// 7. Apply deterministic remediation operations to produce Candidate_i
	candidateBytes, diffSummary, opTypes, err := s.applyDeterministicOperations(originalBytes, req.Operations)
	if err != nil {
		return nil, fmt.Errorf("apply operations: %w", err)
	}

	// 8. Assert Original Immutability Law: H_after(original) == H_before(original)
	rcAfter, errAfter := s.store.Get(ctx, parentStoragePath)
	if errAfter != nil {
		return nil, fmt.Errorf("read parent artifact after generation: %w", errAfter)
	}
	defer rcAfter.Close()
	originalBytesAfter, errAfterRead := io.ReadAll(rcAfter)
	if errAfterRead != nil {
		return nil, fmt.Errorf("read original bytes after: %w", errAfterRead)
	}
	hAfter := sha256.Sum256(originalBytesAfter)
	hAfterHex := hex.EncodeToString(hAfter[:])
	if hBeforeHex != hAfterHex {
		return nil, ErrOriginalMutated
	}

	// 9. Compute Candidate SHA-256
	hCand := sha256.Sum256(candidateBytes)
	candidateSHAHex := hex.EncodeToString(hCand[:])

	// 10. Store candidate bytes in ObjectStore under new immutable key
	logicalPayload := fmt.Sprintf("%s:%s:%d:%s:%s", req.TenantID, req.WorkflowID, req.AttemptNumber, req.ExpectedParentSHA256, req.PlanHash)
	hLogical := sha256.Sum256([]byte(logicalPayload))
	candidateLogicalID := hex.EncodeToString(hLogical[:])
	candidateKey, err := objectstore.DeterministicKey(req.TenantID, candidateLogicalID)
	if err != nil {
		return nil, fmt.Errorf("generate deterministic candidate key: %w", err)
	}

	putRes, err := s.store.Put(ctx, candidateKey, bytes.NewReader(candidateBytes), int64(len(candidateBytes)))
	if err != nil {
		if errors.Is(err, objectstore.ErrObjectExists) {
			existingRC, errGet := s.store.Get(ctx, candidateKey)
			if errGet != nil {
				return nil, fmt.Errorf("read existing candidate: %w", errGet)
			}
			defer existingRC.Close()
			existingBytes, errRead := io.ReadAll(existingRC)
			if errRead != nil {
				return nil, fmt.Errorf("read existing candidate bytes: %w", errRead)
			}
			existingHash := sha256.Sum256(existingBytes)
			if hex.EncodeToString(existingHash[:]) != candidateSHAHex {
				return nil, fmt.Errorf("existing candidate hash mismatch")
			}
		} else {
			return nil, fmt.Errorf("put candidate to object store: %w", err)
		}
	} else if putRes.SHA256 != candidateSHAHex {
		candidateSHAHex = putRes.SHA256
	}

	// Re-read written candidate and verify SHA-256 matches candidateSHAHex
	verifyRC, errVerify := s.store.Get(ctx, candidateKey)
	if errVerify != nil {
		return nil, fmt.Errorf("verify candidate read: %w", errVerify)
	}
	defer verifyRC.Close()
	verifyBytes, errVerifyRead := io.ReadAll(verifyRC)
	if errVerifyRead != nil {
		return nil, fmt.Errorf("verify candidate read bytes: %w", errVerifyRead)
	}
	verifyHash := sha256.Sum256(verifyBytes)
	if hex.EncodeToString(verifyHash[:]) != candidateSHAHex {
		return nil, fmt.Errorf("verify candidate hash mismatch: got %s, want %s", hex.EncodeToString(verifyHash[:]), candidateSHAHex)
	}

	// 11. Run authoritative deterministic NACHA Validator on candidate bytes
	valRes, err := nacha.Validate(bytes.NewReader(candidateBytes))
	if err != nil {
		return nil, fmt.Errorf("validate candidate: %w", err)
	}

	validationOutcome := "VALIDATION_PASSED"
	blockingCount := 0
	for _, f := range valRes.Findings {
		if f.Blocking() {
			blockingCount++
			validationOutcome = "VALIDATION_FAILED"
		}
	}

	// 12 & 13. DB Persistence within transaction
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, fmt.Errorf("begin transaction: %w", err)
	}
	defer tx.Rollback()

	candidateFilename := fmt.Sprintf("candidate_att%d_%s", req.AttemptNumber, parentFilename)
	insertFileQuery := `
		INSERT INTO file_instances (
			tenant_id, filename, storage_path, size_bytes, sha256_hash, status,
			derived_from, derivation_reason, derivation_agent_id, received_at
		) VALUES (?, ?, ?, ?, ?, 'CANDIDATE', ?, ?, ?, CURRENT_TIMESTAMP)`

	res, err := tx.ExecContext(ctx, insertFileQuery,
		req.TenantID, candidateFilename, candidateKey, len(candidateBytes), candidateSHAHex,
		fmt.Sprintf("%d", req.ParentArtifactID), req.PlanHash, req.AgentName,
	)
	if err != nil {
		return nil, fmt.Errorf("insert candidate file_instance: %w", err)
	}
	candArtifactID, _ := res.LastInsertId()

	planID := fmt.Sprintf("plan-%s-%d", req.WorkflowID, req.AttemptNumber)
	opsJSON, _ := json.Marshal(req.Operations)
	findingRefsJSON, _ := json.Marshal(req.FindingRefs)
	opTypesJSON, _ := json.Marshal(opTypes)

	planQuery := `
		INSERT INTO remediation_plans (
			id, workflow_id, tenant_id, incident_id, artifact_id,
			expected_parent_sha256, attempt_number, plan_hash,
			operations_json, finding_refs_json, confidence, status, created_at
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`
	_, err = tx.ExecContext(ctx, planQuery,
		planID, req.WorkflowID, req.TenantID, req.IncidentID, req.ParentArtifactID,
		req.ExpectedParentSHA256, req.AttemptNumber, req.PlanHash,
		string(opsJSON), string(findingRefsJSON), req.Confidence, "APPLIED", time.Now().UTC(),
	)
	if err != nil {
		return nil, fmt.Errorf("insert remediation plan: %w", err)
	}

	derivationID := fmt.Sprintf("deriv-%s-%d", req.WorkflowID, req.AttemptNumber)
	derivationPayload := fmt.Sprintf("%s:%d:%s:%d:%s:%s", req.WorkflowID, req.AttemptNumber, req.ExpectedParentSHA256, candArtifactID, candidateSHAHex, req.PlanHash)
	hDeriv := sha256.Sum256([]byte(derivationPayload))
	derivationHash := hex.EncodeToString(hDeriv[:])

	derivQuery := `
		INSERT INTO artifact_derivations (
			id, tenant_id, workflow_id, remediation_plan_id, attempt_number,
			parent_artifact_id, parent_sha256, candidate_artifact_id, candidate_sha256,
			remediation_plan_hash, operation_types_json, agent_name, agent_version,
			policy_decision_id, policy_decision_hash, tool_manifest_hash,
			validator_version, validation_run_id, validation_outcome,
			findings_count, blocking_findings_count, derivation_hash, created_at
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`
	_, err = tx.ExecContext(ctx, derivQuery,
		derivationID, req.TenantID, req.WorkflowID, planID, req.AttemptNumber,
		req.ParentArtifactID, req.ExpectedParentSHA256, candArtifactID, candidateSHAHex,
		req.PlanHash, string(opTypesJSON), req.AgentName, req.AgentVersion,
		req.PolicyDecisionID, req.PolicyDecisionHash, req.ToolManifestHash,
		"1.0.0", fmt.Sprintf("vrun-%s-%d", req.WorkflowID, req.AttemptNumber), validationOutcome,
		len(valRes.Findings), blockingCount, derivationHash, time.Now().UTC(),
	)
	if err != nil {
		return nil, fmt.Errorf("insert artifact derivation: %w", err)
	}

	if err := tx.Commit(); err != nil {
		return nil, fmt.Errorf("commit transaction: %w", err)
	}

	return &CandidateResult{
		DerivationID:          derivationID,
		WorkflowID:            req.WorkflowID,
		AttemptNumber:         req.AttemptNumber,
		ParentArtifactID:      req.ParentArtifactID,
		ParentSHA256:          req.ExpectedParentSHA256,
		CandidateArtifactID:   candArtifactID,
		CandidateSHA256:       candidateSHAHex,
		RemediationPlanHash:   req.PlanHash,
		ValidationOutcome:     validationOutcome,
		FindingsCount:         len(valRes.Findings),
		BlockingFindingsCount: blockingCount,
		Findings:              valRes.Findings,
		DerivationHash:        derivationHash,
		DiffSummary:           diffSummary,
		CreatedAt:             time.Now().UTC(),
	}, nil
}

// applyDeterministicOperations applies allowlisted typed remediation operations to NACHA records.
func (s *Service) applyDeterministicOperations(
	originalBytes []byte,
	ops []RemediationOperation,
) ([]byte, string, []string, error) {
	// Parse into 94-byte fixed-width lines
	content := string(originalBytes)
	var rawLines []string
	if strings.Contains(content, "\r\n") {
		rawLines = strings.Split(content, "\r\n")
	} else if strings.Contains(content, "\n") {
		rawLines = strings.Split(content, "\n")
	} else {
		// Single contiguous block of 94-char records
		for i := 0; i+94 <= len(content); i += 94 {
			rawLines = append(rawLines, content[i:i+94])
		}
	}

	lines := make([]string, 0, len(rawLines))
	for _, l := range rawLines {
		if len(l) == 94 {
			lines = append(lines, l)
		}
	}

	if len(lines) == 0 {
		return nil, "", nil, errors.New("empty or unparseable NACHA artifact")
	}

	var opTypes []string
	var diffs []string

	// Track batches and records
	type batchData struct {
		batchNumStr    string
		batchNumber    int
		headerIndex    int
		controlIndex   int
		serviceClass   string
		companyID      string
		originatingDFI string
		entryCount     int
		addendaCount   int
		totalDebits    int64
		totalCredits   int64
		routingHashSum int64
	}

	var batches []*batchData
	var currentBatch *batchData

	for idx, line := range lines {
		if len(line) < 94 {
			continue
		}
		recType := line[0]

		switch recType {
		case '5': // Batch Header
			bNumStr := strings.TrimSpace(line[87:94])
			bNum, _ := strconv.Atoi(bNumStr)
			currentBatch = &batchData{
				batchNumStr:    line[87:94],
				batchNumber:    bNum,
				headerIndex:    idx,
				controlIndex:   -1,
				serviceClass:   line[1:4],
				companyID:      line[40:50],
				originatingDFI: line[79:87],
			}
			batches = append(batches, currentBatch)

		case '6': // Entry Detail
			if currentBatch != nil {
				currentBatch.entryCount++
				amt, _ := strconv.ParseInt(line[29:39], 10, 64)
				txCode := line[1:3]
				isDebit := nacha.IsDebitTransaction(txCode)
				if isDebit {
					currentBatch.totalDebits += amt
				} else {
					currentBatch.totalCredits += amt
				}
				routing8, _ := strconv.ParseInt(line[3:11], 10, 64)
				currentBatch.routingHashSum += routing8
			}

		case '7': // Addenda
			if currentBatch != nil {
				currentBatch.addendaCount++
			}

		case '8': // Batch Control
			if currentBatch != nil {
				currentBatch.controlIndex = idx
			}
		}
	}

	// Apply each allowlisted operation
	for _, op := range ops {
		opTypes = append(opTypes, op.OperationType)

		switch op.OperationType {
		case OpRecomputeBatchControlTotal:
			targetBatchIdx := 0 // default first batch
			if strings.HasPrefix(strings.ToUpper(op.TargetRef), "BATCH-") {
				if num, err := strconv.Atoi(op.TargetRef[6:]); err == nil && num > 0 && num <= len(batches) {
					targetBatchIdx = num - 1
				}
			}

			if targetBatchIdx >= len(batches) || batches[targetBatchIdx].controlIndex < 0 {
				return nil, "", nil, fmt.Errorf("%w: %s", ErrAmbiguousTarget, op.TargetRef)
			}

			b := batches[targetBatchIdx]

			// Authoritative arithmetic computed in Go
			totalEntryAddenda := b.entryCount + b.addendaCount
			entryHashModulo := b.routingHashSum % 10000000000

			// Format exact 94-char fixed-width Batch Control record (Type '8')
			newControl := fmt.Sprintf(
				"8%3s%06d%010d%012d%012d%10s%19s%6s%8s%7s",
				b.serviceClass,
				totalEntryAddenda,
				entryHashModulo,
				b.totalDebits,
				b.totalCredits,
				b.companyID,
				strings.Repeat(" ", 19),
				strings.Repeat(" ", 6),
				b.originatingDFI,
				b.batchNumStr,
			)

			if len(newControl) != 94 {
				return nil, "", nil, fmt.Errorf("computed batch control record length is %d, expected 94", len(newControl))
			}

			lines[b.controlIndex] = newControl
			diffs = append(diffs, fmt.Sprintf("Batch %d Control Record (line %d): Recomputed Debits=%d, Credits=%d, EntryHash=%d", b.batchNumber, b.controlIndex+1, b.totalDebits, b.totalCredits, entryHashModulo))

		case OpRecomputeFileControlTotal:
			fileControlIdx := -1
			for idx, l := range lines {
				if len(l) > 0 && l[0] == '9' {
					fileControlIdx = idx
					break
				}
			}
			if fileControlIdx < 0 {
				return nil, "", nil, fmt.Errorf("%w: File Control Record (Type 9) missing", ErrAmbiguousTarget)
			}

			var totalFileDebits, totalFileCredits, totalFileHashSum int64
			var totalFileEntries int
			for _, b := range batches {
				totalFileDebits += b.totalDebits
				totalFileCredits += b.totalCredits
				totalFileHashSum += b.routingHashSum
				totalFileEntries += (b.entryCount + b.addendaCount)
			}

			batchCount := len(batches)
			blockCount := int(math.Ceil(float64(len(lines)) / 10.0))
			fileHashModulo := totalFileHashSum % 10000000000

			newFileControl := fmt.Sprintf(
				"9%06d%06d%08d%010d%012d%012d%39s",
				batchCount,
				blockCount,
				totalFileEntries,
				fileHashModulo,
				totalFileDebits,
				totalFileCredits,
				strings.Repeat(" ", 39),
			)

			if len(newFileControl) != 94 {
				return nil, "", nil, fmt.Errorf("computed file control record length is %d, expected 94", len(newFileControl))
			}

			lines[fileControlIdx] = newFileControl
			diffs = append(diffs, fmt.Sprintf("File Control Record (line %d): Recomputed Batches=%d, Debits=%d, Credits=%d, Hash=%d", fileControlIdx+1, batchCount, totalFileDebits, totalFileCredits, fileHashModulo))

		default:
			return nil, "", nil, fmt.Errorf("%w: %s", ErrInvalidOperation, op.OperationType)
		}
	}

	// Reconstruct candidate byte stream using newline delimiter
	candidateText := strings.Join(lines, "\n") + "\n"
	return []byte(candidateText), strings.Join(diffs, "; "), opTypes, nil
}
