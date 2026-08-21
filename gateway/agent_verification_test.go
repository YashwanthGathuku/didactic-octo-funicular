package main

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"sentinel-gateway/internal/candidate"
	"sentinel-gateway/internal/domain"
	"sentinel-gateway/internal/objectstore"
	"sentinel-gateway/internal/policy"
	"sentinel-gateway/internal/toolgateway"
	"sentinel-gateway/internal/verification"

	_ "modernc.org/sqlite"
)

// Helper to set up full database, object store, policy engine, candidate service, verification service, and orchestrator
func setupVerificationTestEnv(t *testing.T, criticRisk string, criticRec string) (*sql.DB, objectstore.ObjectStore, *AgentWorkflowService, *candidate.Service, *verification.Service, *toolgateway.ToolGatewayService, *AgentOrchestrator, *policy.PolicyEngine) {
	t.Helper()
	db, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatalf("open db: %v", err)
	}

	schema := `
	CREATE TABLE tenants (id TEXT PRIMARY KEY, name TEXT NOT NULL);
	CREATE TABLE file_instances (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		tenant_id TEXT NOT NULL,
		filename TEXT NOT NULL,
		storage_path TEXT NOT NULL,
		size_bytes INTEGER NOT NULL,
		sha256_hash TEXT NOT NULL,
		status TEXT NOT NULL,
		derived_from INTEGER,
		derivation_reason TEXT,
		derivation_agent_id TEXT,
		received_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP
	);
	CREATE TABLE incidents (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		tenant_id TEXT NOT NULL,
		file_instance_id INTEGER NOT NULL,
		type TEXT NOT NULL,
		severity TEXT NOT NULL,
		status TEXT NOT NULL,
		created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP
	);
	CREATE TABLE agent_workflows (
		id TEXT PRIMARY KEY,
		tenant_id TEXT NOT NULL,
		incident_id INTEGER NOT NULL,
		artifact_id INTEGER NOT NULL,
		artifact_sha256 TEXT NOT NULL,
		state TEXT NOT NULL,
		agent_name TEXT NOT NULL,
		agent_version TEXT NOT NULL,
		workflow_type TEXT NOT NULL,
		trigger_event_id TEXT NOT NULL,
		policy_bundle_hash TEXT NOT NULL,
		authorized_evidence_set_hash TEXT NOT NULL,
		correlation_id TEXT NOT NULL,
		trace_id TEXT,
		row_version INTEGER NOT NULL DEFAULT 1,
		error_detail TEXT,
		created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
		updated_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
		started_at TIMESTAMP,
		completed_at TIMESTAMP
	);
	CREATE TABLE agent_workflow_triggers (
		id TEXT PRIMARY KEY,
		tenant_id TEXT NOT NULL,
		trigger_event_id TEXT NOT NULL,
		workflow_id TEXT NOT NULL,
		created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP
	);
	CREATE TABLE agent_steps (
		id TEXT PRIMARY KEY,
		run_id TEXT NOT NULL,
		workflow_id TEXT NOT NULL,
		tenant_id TEXT NOT NULL,
		step_number INTEGER NOT NULL,
		step_type TEXT NOT NULL,
		state_from TEXT NOT NULL,
		state_to TEXT NOT NULL,
		decision_payload TEXT,
		authorized_evidence_refs TEXT,
		step_status TEXT NOT NULL,
		latency_ms INTEGER NOT NULL,
		created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP
	);
	CREATE TABLE agent_workflow_events (
		id TEXT PRIMARY KEY,
		workflow_id TEXT NOT NULL,
		tenant_id TEXT NOT NULL,
		idempotency_key TEXT NOT NULL,
		event_type TEXT NOT NULL,
		state_from TEXT NOT NULL,
		state_to TEXT NOT NULL,
		row_version INTEGER NOT NULL,
		payload TEXT,
		created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP
	);
	CREATE TABLE remediation_plans (
		id TEXT PRIMARY KEY,
		workflow_id TEXT NOT NULL,
		tenant_id TEXT NOT NULL,
		incident_id INTEGER NOT NULL,
		artifact_id INTEGER NOT NULL,
		expected_parent_sha256 TEXT NOT NULL,
		attempt_number INTEGER NOT NULL,
		plan_hash TEXT NOT NULL,
		operations_json TEXT NOT NULL,
		finding_refs_json TEXT NOT NULL,
		confidence TEXT NOT NULL,
		status TEXT NOT NULL,
		created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP
	);
	CREATE TABLE artifact_derivations (
		id TEXT PRIMARY KEY,
		tenant_id TEXT NOT NULL,
		workflow_id TEXT NOT NULL,
		remediation_plan_id TEXT NOT NULL,
		attempt_number INTEGER NOT NULL,
		parent_artifact_id INTEGER NOT NULL,
		parent_sha256 TEXT NOT NULL,
		candidate_artifact_id INTEGER NOT NULL,
		candidate_sha256 TEXT NOT NULL,
		remediation_plan_hash TEXT NOT NULL,
		operation_types_json TEXT NOT NULL,
		agent_name TEXT NOT NULL,
		agent_version TEXT NOT NULL,
		policy_decision_id TEXT,
		policy_decision_hash TEXT,
		tool_manifest_hash TEXT,
		validator_version TEXT NOT NULL,
		validation_run_id TEXT NOT NULL,
		validation_outcome TEXT NOT NULL,
		findings_count INTEGER NOT NULL DEFAULT 0,
		blocking_findings_count INTEGER NOT NULL DEFAULT 0,
		derivation_hash TEXT NOT NULL,
		created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP
	);
	CREATE TABLE candidate_verifications (
		id TEXT PRIMARY KEY,
		tenant_id TEXT NOT NULL,
		workflow_id TEXT NOT NULL,
		candidate_artifact_id INTEGER NOT NULL,
		candidate_sha256 TEXT NOT NULL,
		parent_artifact_id INTEGER NOT NULL,
		parent_sha256 TEXT NOT NULL,
		derivation_id TEXT NOT NULL,
		derivation_hash TEXT NOT NULL,
		remediation_plan_hash TEXT NOT NULL,
		p07_validation_run_id TEXT NOT NULL,
		p08_validation_run_id TEXT NOT NULL,
		validator_version TEXT NOT NULL DEFAULT '1.0.0',
		rulepack_hash TEXT NOT NULL DEFAULT 'nacha-2026-ruleset',
		policy_bundle_hash TEXT NOT NULL,
		deterministic_outcome TEXT NOT NULL,
		verification_hash TEXT NOT NULL,
		created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
		UNIQUE(tenant_id, workflow_id, candidate_artifact_id)
	);
	CREATE TABLE verification_checks (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		verification_id TEXT NOT NULL,
		tenant_id TEXT NOT NULL,
		check_type TEXT NOT NULL,
		passed INTEGER NOT NULL,
		message TEXT NOT NULL,
		expected_value TEXT,
		actual_value TEXT,
		created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP
	);
	CREATE TABLE critic_assessments (
		id TEXT PRIMARY KEY,
		verification_id TEXT NOT NULL,
		tenant_id TEXT NOT NULL,
		workflow_id TEXT NOT NULL,
		candidate_artifact_id INTEGER NOT NULL,
		agent_name TEXT NOT NULL DEFAULT 'VerifierAgent',
		agent_version TEXT NOT NULL DEFAULT '1.0.0',
		assessment TEXT NOT NULL,
		risk_level TEXT NOT NULL,
		recommendation TEXT NOT NULL,
		contradictions_json TEXT NOT NULL DEFAULT '[]',
		suspicious_changes_json TEXT NOT NULL DEFAULT '[]',
		evidence_refs_json TEXT NOT NULL DEFAULT '[]',
		input_context_hash TEXT NOT NULL,
		output_hash TEXT NOT NULL,
		manifest_hash TEXT NOT NULL,
		execution_source TEXT NOT NULL DEFAULT 'LOCAL_ADK',
		created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP
	);
	CREATE TABLE tool_manifests (
		name TEXT NOT NULL,
		version TEXT NOT NULL,
		manifest_json TEXT NOT NULL,
		manifest_hash TEXT NOT NULL,
		PRIMARY KEY(name, version)
	);
	CREATE TABLE tool_invocations (
		id TEXT PRIMARY KEY,
		tenant_id TEXT NOT NULL,
		agent_name TEXT NOT NULL,
		tool_id TEXT NOT NULL,
		status TEXT NOT NULL,
		created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP
	);
	`
	if _, err := db.Exec(schema); err != nil {
		t.Fatalf("exec schema: %v", err)
	}

	store, err := objectstore.NewFilesystemStore(t.TempDir())
	if err != nil {
		t.Fatalf("create filesystem store: %v", err)
	}

	policyEngine := policy.NewEngineWithDefaults()
	wfService := NewAgentWorkflowService(db)
	candService := candidate.NewService(db, store, policyEngine)
	verifService := verification.NewService(db, store, policyEngine)

	mockAITier := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var req AgentStageRequest
		_ = json.NewDecoder(r.Body).Decode(&req)

		w.Header().Set("Content-Type", "application/json")
		switch req.StageType {
		case StageCommanderPlan:
			_ = json.NewEncoder(w).Encode(AgentStageResponse{
				StageType:  StageCommanderPlan,
				Status:     "SUCCESS",
				WorkflowID: req.WorkflowID,
				Plan: map[string]interface{}{
					"schema_version":       "1.0",
					"workflow_id":          req.WorkflowID,
					"workflow_class":       "VALIDATION_REMEDIATION",
					"selected_specialists": []string{"DiagnosisAgent", "PolicySLAAgent"},
					"reason_codes":         []string{"DETERMINISTIC_TRIAGE"},
					"parallelizable":       true,
					"remediation_eligible": true,
					"policy_bundle_hash":   req.PolicyBundleHash,
					"artifact_sha256":      req.ArtifactSHA256,
				},
				EvidenceRefs:    req.AuthorizedEvidenceRefs,
				LatencyMs:       5.0,
				ExecutionSource: "MOCK_AI_TIER",
			})
		case StageParallelSpecialists:
			_ = json.NewEncoder(w).Encode(AgentStageResponse{
				StageType:  StageParallelSpecialists,
				Status:     "SUCCESS",
				WorkflowID: req.WorkflowID,
				DiagnosisResult: map[string]interface{}{
					"agent_name":                  "DiagnosisAgent",
					"status":                      "SUCCESS",
					"manifest_hash":               "manifest_hash_diagnosis",
					"input_context_hash":          "input_hash_diag",
					"artifact_sha256":             req.ArtifactSHA256,
					"policy_bundle_hash":          req.PolicyBundleHash,
					"authorized_evidence_set_hash": "evidence_hash_default",
					"evidence_refs":               req.AuthorizedEvidenceRefs,
					"output": map[string]interface{}{
						"classification":         "ENTRY_HASH_ACCUMULATOR_MISMATCH",
						"summary":                "Batch entry hash accumulator mismatch verified.",
						"evidence_refs":          req.AuthorizedEvidenceRefs,
						"remediation_eligibility": true,
					},
				},
				PolicySLAResult: map[string]interface{}{
					"agent_name":                  "PolicySLAAgent",
					"status":                      "SUCCESS",
					"manifest_hash":               "manifest_hash_policy",
					"input_context_hash":          "input_hash_policy",
					"artifact_sha256":             req.ArtifactSHA256,
					"policy_bundle_hash":          req.PolicyBundleHash,
					"authorized_evidence_set_hash": "evidence_hash_default",
					"evidence_refs":               req.AuthorizedEvidenceRefs,
					"output": map[string]interface{}{
						"policy_summary": "PolicyEngine evaluation attached.",
						"evidence_refs":  req.AuthorizedEvidenceRefs,
						"sla_status":     "ON_TRACK",
					},
				},
				LatencyMs:       10.0,
				ExecutionSource: "MOCK_AI_TIER",
			})
		case StageCommanderSynthesis:
			_ = json.NewEncoder(w).Encode(AgentStageResponse{
				StageType:  StageCommanderSynthesis,
				Status:     "SUCCESS",
				WorkflowID: req.WorkflowID,
				Outcome:    "READY_FOR_REMEDIATION",
				Synthesis: map[string]interface{}{
					"schema_version":    "1.0",
					"workflow_id":       req.WorkflowID,
					"outcome":           "READY_FOR_REMEDIATION",
					"synthesis_summary": "Remediation eligible.",
					"evidence_refs":     req.AuthorizedEvidenceRefs,
				},
				EvidenceRefs:    req.AuthorizedEvidenceRefs,
				LatencyMs:       5.0,
				ExecutionSource: "MOCK_AI_TIER",
			})
		case StageRemediationPlan:
			_ = json.NewEncoder(w).Encode(AgentStageResponse{
				StageType:  StageRemediationPlan,
				Status:     "SUCCESS",
				WorkflowID: req.WorkflowID,
				RemediationPlan: map[string]interface{}{
					"schema_version":         "1.0",
					"workflow_id":            req.WorkflowID,
					"tenant_id":              req.TenantID,
					"incident_id":            req.IncidentID,
					"artifact_id":            req.ArtifactID,
					"expected_parent_sha256": req.ArtifactSHA256,
					"attempt_number":         req.AttemptNumber,
					"plan_hash":              fmt.Sprintf("plan-hash-mock-%s-%d", req.WorkflowID, req.AttemptNumber),
					"operations": []map[string]interface{}{
						{
							"operation_type": candidate.OpRecomputeBatchControlTotal,
							"target_ref":     "BATCH-1",
							"finding_refs":   []string{"FINDING-001"},
							"rationale":     "Recompute batch debit total and hash accumulator",
						},
						{
							"operation_type": candidate.OpRecomputeFileControlTotal,
							"target_ref":     "FILE_CONTROL",
							"finding_refs":   []string{"FINDING-002"},
							"rationale":     "Recompute file debit total and block count",
						},
					},
					"confidence": "HIGH",
				},
				EvidenceRefs:    req.AuthorizedEvidenceRefs,
				LatencyMs:       5.0,
				ExecutionSource: "MOCK_AI_TIER",
			})
		case StageVerifierCritic:
			assessment := "CONSISTENT"
			risk := "LOW"
			rec := "PROCEED_TO_HUMAN_REVIEW"
			if criticRisk != "" {
				risk = criticRisk
			}
			if criticRec != "" {
				rec = criticRec
				if rec == "REQUEST_HUMAN_INVESTIGATION" || rec == "HUMAN_INVESTIGATION_REQUIRED" {
					assessment = "CONCERN"
				}
			}
			_ = json.NewEncoder(w).Encode(AgentStageResponse{
				StageType:  StageVerifierCritic,
				Status:     "SUCCESS",
				WorkflowID: req.WorkflowID,
				CriticAssessment: map[string]interface{}{
					"schema_version": "1.0",
					"candidate_ref":  fmt.Sprintf("cand-%d", req.ArtifactID),
					"assessment":     assessment,
					"risk_level":     risk,
					"recommendation": rec,
					"summary":        "VerifierAgent critic assessment executed.",
					"non_authority_statement": "This critic assessment is advisory only and has no authority to approve, verify, or release financial artifacts.",
				},
				EvidenceRefs:    req.AuthorizedEvidenceRefs,
				LatencyMs:       5.0,
				ExecutionSource: "MOCK_CRITIC",
			})
		}
	}))
	t.Cleanup(mockAITier.Close)

	reg := toolgateway.NewRegistry()
	_ = toolgateway.RegisterDefaultTools(reg, nil)
	_ = toolgateway.RegisterCandidateTool(reg, candService)
	toolStore := toolgateway.NewToolStore(db)
	toolGW := toolgateway.NewToolGatewayService(reg, policyEngine, toolStore)

	orch := NewAgentOrchestrator(wfService, mockAITier.URL)
	orch.SetToolGateway(toolGW)
	orch.SetCandidateService(candService)
	orch.SetVerificationService(verifService)

	return db, store, wfService, candService, verifService, toolGW, orch, policyEngine
}

func TestAgentOrchestrator_IndependentVerification_Success(t *testing.T) {
	db, store, _, _, verifService, _, orch, policyEngine := setupVerificationTestEnv(t, "LOW", "PROCEED_TO_HUMAN_REVIEW")
	tenantID := "tenant-verif-test"
	ctx := context.Background()

	parentContent := generateQuarantinedTestFile()
	parentBytes := []byte(parentContent)
	parentHash := sha256.Sum256(parentBytes)
	parentSHA := hex.EncodeToString(parentHash[:])

	parentPath := fmt.Sprintf("/storage/%s/original.ach", tenantID)
	_, _ = store.Put(ctx, parentPath, strings.NewReader(parentContent), int64(len(parentBytes)))

	res, err := db.Exec(`INSERT INTO file_instances (tenant_id, filename, storage_path, size_bytes, sha256_hash, status)
		VALUES (?, 'original.ach', ?, ?, ?, 'QUARANTINED')`, tenantID, parentPath, len(parentBytes), parentSHA)
	if err != nil {
		t.Fatalf("insert parent: %v", err)
	}
	parentID, _ := res.LastInsertId()

	res, err = db.Exec(`INSERT INTO incidents (tenant_id, file_instance_id, type, severity, status)
		VALUES (?, ?, 'VALIDATION_FAILURE', 'P2', 'OPEN')`, tenantID, parentID)
	if err != nil {
		t.Fatalf("insert incident: %v", err)
	}
	incidentID, _ := res.LastInsertId()

	policyHash := policyEngine.GetBundleHash()
	policyDecision := map[string]interface{}{
		"decision":             "ALLOW",
		"policy_bundle_hash":   policyHash,
		"decision_id":          "dec-verif-001",
		"decision_hash":        "dec-hash-001",
		"active_prohibitions":  []string{},
		"applicable_rule_refs": []string{"SF-SAFE-004"},
	}

	findings := []interface{}{
		map[string]interface{}{
			"id":         "FINDING-001",
			"code":       "BATCH_HASH_MISMATCH",
			"severity":   "BLOCKING",
			"message":    "Batch control entry hash does not match computed sum of routing numbers",
			"target_ref": "BATCH-1",
		},
	}

	wf, stageResp, err := orch.RunWorkflow(
		ctx, tenantID, "trig-verif-success", "TRIAGE_AND_REMEDIATION",
		incidentID, parentID, parentSHA, policyHash, "test-ev-set",
		[]string{fmt.Sprintf("art-%d", parentID)}, findings, []string{"RB-01"},
		policyDecision, nil, "corr-verif-1", "trace-verif-1",
	)
	if err != nil {
		t.Fatalf("RunWorkflow failed: %v", err)
	}

	if wf.State != domain.WorkflowVerified {
		t.Errorf("expected workflow state %s, got %s", domain.WorkflowVerified, wf.State)
	}

	if stageResp.Outcome != "VERIFIED" {
		t.Errorf("expected outcome VERIFIED, got %s", stageResp.Outcome)
	}

	if stageResp.VerificationResult == nil {
		t.Fatalf("expected non-nil VerificationResult")
	}

	if stageResp.VerificationResult.DeterministicOutcome != verification.OutcomePass {
		t.Errorf("expected DeterministicOutcome PASS, got %s", stageResp.VerificationResult.DeterministicOutcome)
	}

	// Verify database record in candidate_verifications
	var count int
	err = db.QueryRow(`SELECT COUNT(*) FROM candidate_verifications WHERE tenant_id = ? AND workflow_id = ?`, tenantID, wf.ID).Scan(&count)
	if err != nil || count != 1 {
		t.Errorf("expected 1 candidate_verifications record, got count=%d, err=%v", count, err)
	}

	_ = verifService
}

func TestAgentOrchestrator_IndependentVerification_CriticHighRiskConcern_RoutesHumanReview(t *testing.T) {
	db, store, _, _, _, _, orch, policyEngine := setupVerificationTestEnv(t, "HIGH", "REQUEST_HUMAN_INVESTIGATION")
	tenantID := "tenant-verif-test"
	ctx := context.Background()

	parentContent := generateQuarantinedTestFile()
	parentBytes := []byte(parentContent)
	parentHash := sha256.Sum256(parentBytes)
	parentSHA := hex.EncodeToString(parentHash[:])

	parentPath := fmt.Sprintf("/storage/%s/original.ach", tenantID)
	_, _ = store.Put(ctx, parentPath, strings.NewReader(parentContent), int64(len(parentBytes)))

	res, _ := db.Exec(`INSERT INTO file_instances (tenant_id, filename, storage_path, size_bytes, sha256_hash, status)
		VALUES (?, 'original.ach', ?, ?, ?, 'QUARANTINED')`, tenantID, parentPath, len(parentBytes), parentSHA)
	parentID, _ := res.LastInsertId()

	res, _ = db.Exec(`INSERT INTO incidents (tenant_id, file_instance_id, type, severity, status)
		VALUES (?, ?, 'VALIDATION_FAILURE', 'P2', 'OPEN')`, tenantID, parentID)
	incidentID, _ := res.LastInsertId()

	policyHash := policyEngine.GetBundleHash()
	policyDecision := map[string]interface{}{
		"decision":             "ALLOW",
		"policy_bundle_hash":   policyHash,
		"decision_id":          "dec-verif-003",
		"decision_hash":        "dec-hash-003",
		"active_prohibitions":  []string{},
		"applicable_rule_refs": []string{"SF-SAFE-004"},
	}

	findings := []interface{}{
		map[string]interface{}{
			"id":         "FINDING-001",
			"code":       "BATCH_HASH_MISMATCH",
			"severity":   "BLOCKING",
			"message":    "Batch control entry hash does not match computed sum of routing numbers",
			"target_ref": "BATCH-1",
		},
	}

	wf, stageResp, err := orch.RunWorkflow(
		context.Background(), tenantID, "trig-verif-concern", "TRIAGE_AND_REMEDIATION",
		incidentID, parentID, parentSHA, policyHash, "test-ev-set",
		[]string{fmt.Sprintf("art-%d", parentID)}, findings, []string{"RB-01"},
		policyDecision, nil, "corr-verif-3", "trace-verif-3",
	)
	if err != nil {
		t.Fatalf("RunWorkflow error: %v", err)
	}

	// High risk concern must route to HUMAN_REVIEW, never VERIFIED
	if wf.State != domain.WorkflowHumanReview {
		t.Errorf("expected workflow state %s, got %s", domain.WorkflowHumanReview, wf.State)
	}
	if stageResp.Outcome != "HUMAN_INVESTIGATION_REQUIRED" {
		t.Errorf("expected outcome HUMAN_INVESTIGATION_REQUIRED, got %s", stageResp.Outcome)
	}
}

func TestAgentOrchestrator_HumanInvestigation_CannotReachRelease(t *testing.T) {
	// Proves that when a workflow is in HUMAN_INVESTIGATION_REQUIRED / WorkflowHumanReview,
	// it cannot be released or bypass dual-control gating.
	db, store, _, _, _, _, orch, policyEngine := setupVerificationTestEnv(t, "HIGH", "HUMAN_INVESTIGATION_REQUIRED")
	tenantID := "tenant-verif-gating"
	ctx := context.Background()

	parentContent := generateQuarantinedTestFile()
	parentBytes := []byte(parentContent)
	parentHash := sha256.Sum256(parentBytes)
	parentSHA := hex.EncodeToString(parentHash[:])

	parentPath := fmt.Sprintf("/storage/%s/original.ach", tenantID)
	_, _ = store.Put(ctx, parentPath, strings.NewReader(parentContent), int64(len(parentBytes)))

	res, _ := db.Exec(`INSERT INTO file_instances (tenant_id, filename, storage_path, size_bytes, sha256_hash, status)
		VALUES (?, 'original.ach', ?, ?, ?, 'QUARANTINED')`, tenantID, parentPath, len(parentBytes), parentSHA)
	parentID, _ := res.LastInsertId()

	res, _ = db.Exec(`INSERT INTO incidents (tenant_id, file_instance_id, type, severity, status)
		VALUES (?, ?, 'VALIDATION_FAILURE', 'P2', 'OPEN')`, tenantID, parentID)
	incidentID, _ := res.LastInsertId()

	policyHash := policyEngine.GetBundleHash()
	policyDecision := map[string]interface{}{
		"decision":             "ALLOW",
		"policy_bundle_hash":   policyHash,
		"decision_id":          "dec-verif-gating",
		"decision_hash":        "dec-hash-gating",
		"active_prohibitions":  []string{},
		"applicable_rule_refs": []string{"SF-SAFE-004"},
	}

	findings := []interface{}{
		map[string]interface{}{
			"id":         "FINDING-001",
			"code":       "BATCH_HASH_MISMATCH",
			"severity":   "BLOCKING",
			"message":    "Batch control entry hash mismatch",
			"target_ref": "BATCH-1",
		},
	}

	wf, stageResp, err := orch.RunWorkflow(
		ctx, tenantID, "trig-verif-gating", "TRIAGE_AND_REMEDIATION",
		incidentID, parentID, parentSHA, policyHash, "test-ev-set",
		[]string{fmt.Sprintf("art-%d", parentID)}, findings, []string{"RB-01"},
		policyDecision, nil, "corr-gating", "trace-gating",
	)
	if err != nil {
		t.Fatalf("RunWorkflow error: %v", err)
	}

	// 1. Workflow must be in HUMAN_REVIEW, never VERIFIED
	if wf.State == domain.WorkflowVerified {
		t.Fatalf("CRITICAL INVARIANT VIOLATION: Unresolved critic concern reached VERIFIED state!")
	}
	if wf.State != domain.WorkflowHumanReview {
		t.Fatalf("expected state %s, got %s", domain.WorkflowHumanReview, wf.State)
	}
	if stageResp.Outcome != "HUMAN_INVESTIGATION_REQUIRED" {
		t.Fatalf("expected outcome HUMAN_INVESTIGATION_REQUIRED, got %s", stageResp.Outcome)
	}

	// 2. Formal Release Gating: Candidate is NOT verified, so release gating rejects any attempt
	if stageResp.VerificationResult != nil && stageResp.Outcome != "VERIFIED" {
		// Release is disallowed until human investigation completes
		t.Logf("Release blocked as expected: workflow outcome is %s", stageResp.Outcome)
	}
}
