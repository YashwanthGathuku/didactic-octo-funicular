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
	"time"

	"sentinel-gateway/internal/candidate"
	"sentinel-gateway/internal/domain"
	"sentinel-gateway/internal/objectstore"
	"sentinel-gateway/internal/policy"
	"sentinel-gateway/internal/toolgateway"

	_ "modernc.org/sqlite"
)

func setupGatewayTestEnv(t *testing.T) (*sql.DB, objectstore.ObjectStore, *AgentWorkflowService, *candidate.Service, *toolgateway.ToolGatewayService, *toolgateway.Registry, *policy.PolicyEngine, string) {
	t.Helper()
	db, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatalf("open db: %v", err)
	}

	schema := `
	CREATE TABLE tenants (id TEXT PRIMARY KEY, name TEXT NOT NULL);
	CREATE TABLE file_instances (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		tenant_id TEXT NOT NULL REFERENCES tenants(id),
		filename TEXT NOT NULL, storage_path TEXT NOT NULL DEFAULT '/p',
		size_bytes INTEGER NOT NULL, sha256_hash TEXT NOT NULL,
		status TEXT NOT NULL,
		derived_from TEXT,
		derivation_reason TEXT,
		derivation_agent_id TEXT,
		received_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP
	);
	CREATE TABLE incidents (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		tenant_id TEXT NOT NULL REFERENCES tenants(id),
		file_instance_id INTEGER REFERENCES file_instances(id),
		type TEXT NOT NULL, severity TEXT NOT NULL,
		status TEXT NOT NULL
	);
	CREATE TABLE agent_workflows (
		id TEXT PRIMARY KEY,
		tenant_id TEXT NOT NULL REFERENCES tenants(id),
		incident_id INTEGER NOT NULL REFERENCES incidents(id),
		artifact_id INTEGER NOT NULL REFERENCES file_instances(id),
		artifact_sha256 TEXT NOT NULL,
		state TEXT NOT NULL,
		agent_name TEXT NOT NULL,
		agent_version TEXT NOT NULL,
		workflow_type TEXT NOT NULL DEFAULT 'TRIAGE_AND_REMEDIATION',
		trigger_event_id TEXT,
		policy_bundle_hash TEXT,
		authorized_evidence_set_hash TEXT,
		correlation_id TEXT NOT NULL,
		trace_id TEXT,
		row_version INTEGER NOT NULL DEFAULT 0,
		error_detail TEXT,
		created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
		updated_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
		started_at TIMESTAMP,
		completed_at TIMESTAMP
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
		created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
		UNIQUE(tenant_id, workflow_id, attempt_number)
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
		created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
		UNIQUE(tenant_id, workflow_id, attempt_number)
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

	reg := toolgateway.NewRegistry()
	_ = toolgateway.RegisterDefaultTools(reg, nil)
	_ = toolgateway.RegisterCandidateTool(reg, candService)

	toolStore := toolgateway.NewToolStore(db)
	toolGW := toolgateway.NewToolGatewayService(reg, policyEngine, toolStore)

	// Mock AI Tier Server
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
					"agent_name":                   "DiagnosisAgent",
					"status":                       "SUCCESS",
					"manifest_hash":                "manifest_hash_diagnosis",
					"input_context_hash":           "input_hash_diag",
					"artifact_sha256":              req.ArtifactSHA256,
					"policy_bundle_hash":           req.PolicyBundleHash,
					"authorized_evidence_set_hash": "evidence_hash_default",
					"evidence_refs":                req.AuthorizedEvidenceRefs,
					"output": map[string]interface{}{
						"classification":          "ENTRY_HASH_ACCUMULATOR_MISMATCH",
						"summary":                 "Batch entry hash accumulator mismatch verified.",
						"evidence_refs":           req.AuthorizedEvidenceRefs,
						"remediation_eligibility": true,
					},
				},
				PolicySLAResult: map[string]interface{}{
					"agent_name":                   "PolicySLAAgent",
					"status":                       "SUCCESS",
					"manifest_hash":                "manifest_hash_policy",
					"input_context_hash":           "input_hash_policy",
					"artifact_sha256":              req.ArtifactSHA256,
					"policy_bundle_hash":           req.PolicyBundleHash,
					"authorized_evidence_set_hash": "evidence_hash_default",
					"evidence_refs":                req.AuthorizedEvidenceRefs,
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
							"rationale":      "Recompute batch debit total and hash accumulator",
						},
						{
							"operation_type": candidate.OpRecomputeFileControlTotal,
							"target_ref":     "FILE_CONTROL",
							"finding_refs":   []string{"FINDING-002"},
							"rationale":      "Recompute file debit total and block count",
						},
					},
					"confidence": "HIGH",
				},
				EvidenceRefs:    req.AuthorizedEvidenceRefs,
				LatencyMs:       5.0,
				ExecutionSource: "MOCK_AI_TIER",
			})
		}
	}))
	t.Cleanup(mockAITier.Close)

	return db, store, wfService, candService, toolGW, reg, policyEngine, mockAITier.URL
}

func seedQuarantinedFileAndIncident(t *testing.T, db *sql.DB, store objectstore.ObjectStore, tenantID string) (int64, int64, string) {
	t.Helper()
	_, _ = db.Exec(`INSERT INTO tenants (id, name) VALUES (?, ?)`, tenantID, "Tenant "+tenantID)

	content := generateQuarantinedTestFile()
	h := sha256.Sum256([]byte(content))
	parentSHAHex := hex.EncodeToString(h[:])

	objKey, err := objectstore.NewKey(tenantID, time.Now().UTC())
	if err != nil {
		t.Fatalf("new key: %v", err)
	}

	_, err = store.Put(context.Background(), objKey, strings.NewReader(content), int64(len(content)))
	if err != nil {
		t.Fatalf("put object: %v", err)
	}

	res, err := db.Exec(`INSERT INTO file_instances (tenant_id, filename, storage_path, size_bytes, sha256_hash, status) VALUES (?, ?, ?, ?, ?, 'QUARANTINED')`,
		tenantID, "test_ach_file.ach", objKey, len(content), parentSHAHex)
	if err != nil {
		t.Fatalf("insert file: %v", err)
	}
	artID, _ := res.LastInsertId()

	res, err = db.Exec(`INSERT INTO incidents (tenant_id, file_instance_id, type, severity, status) VALUES (?, ?, 'NACHA_VALIDATION_FAILURE', 'CRITICAL', 'OPEN')`,
		tenantID, artID)
	if err != nil {
		t.Fatalf("insert incident: %v", err)
	}
	incID, _ := res.LastInsertId()

	return artID, incID, parentSHAHex
}

// Test 1: Exclusive Tool Gateway Path Success
func TestAgentOrchestrator_ExclusiveToolGateway_Success(t *testing.T) {
	db, store, wfService, candService, toolGW, _, _, aiTierURL := setupGatewayTestEnv(t)
	tenantID := "TENANT-GW-SUCCESS"
	artID, incID, parentSHAHex := seedQuarantinedFileAndIncident(t, db, store, tenantID)

	orch := NewAgentOrchestrator(wfService, aiTierURL)
	orch.SetToolGateway(toolGW)
	orch.SetCandidateService(candService)

	ctx := context.Background()
	finalWF, resp, err := orch.RunWorkflow(
		ctx, tenantID, "evt-gw-001", "ARTIFACT_QUARANTINED",
		incID, artID, parentSHAHex,
		"pol-bundle-hash-1", "evidence-set-hash-1",
		[]string{"FINDING-001", "FINDING-002"},
		[]interface{}{"Batch debit mismatch"},
		[]string{"RB-01"},
		map[string]interface{}{"decision": "ALLOW_WITH_OBLIGATIONS"},
		map[string]interface{}{"cutoff": "17:00"},
		"corr-gw-1", "trace-gw-1",
	)
	if err != nil {
		t.Fatalf("run workflow: %v", err)
	}

	if finalWF.State != domain.WorkflowAwaitingVerification {
		t.Fatalf("expected state %s, got %s", domain.WorkflowAwaitingVerification, finalWF.State)
	}
	if resp.Outcome != "CANDIDATE_REVALIDATION_PASSED" {
		t.Fatalf("expected outcome CANDIDATE_REVALIDATION_PASSED, got %s", resp.Outcome)
	}

	// Verify candidate record created in database
	var candCount int
	_ = db.QueryRow(`SELECT count(*) FROM artifact_derivations WHERE tenant_id = ? AND workflow_id = ?`, tenantID, finalWF.ID).Scan(&candCount)
	if candCount != 1 {
		t.Fatalf("expected 1 derivation record, got %d", candCount)
	}
}

// Test 2: Missing Manifest in Tool Gateway fails closed
func TestAgentOrchestrator_ToolGateway_MissingManifest_FailsClosed(t *testing.T) {
	db, store, wfService, candService, _, _, policyEngine, aiTierURL := setupGatewayTestEnv(t)
	tenantID := "TENANT-GW-NOMANIFEST"
	artID, incID, parentSHAHex := seedQuarantinedFileAndIncident(t, db, store, tenantID)

	// Empty registry with NO tools registered
	emptyReg := toolgateway.NewRegistry()
	toolStore := toolgateway.NewToolStore(db)
	emptyGW := toolgateway.NewToolGatewayService(emptyReg, policyEngine, toolStore)

	orch := NewAgentOrchestrator(wfService, aiTierURL)
	orch.SetToolGateway(emptyGW)
	orch.SetCandidateService(candService)

	ctx := context.Background()
	finalWF, resp, err := orch.RunWorkflow(
		ctx, tenantID, "evt-gw-002", "ARTIFACT_QUARANTINED",
		incID, artID, parentSHAHex,
		"pol-bundle-hash-1", "evidence-set-hash-1",
		[]string{"FINDING-001", "FINDING-002"},
		[]interface{}{"Batch debit mismatch"},
		[]string{"RB-01"},
		map[string]interface{}{"decision": "ALLOW_WITH_OBLIGATIONS"},
		map[string]interface{}{"cutoff": "17:00"},
		"corr-gw-2", "trace-gw-2",
	)
	if err != nil {
		t.Fatalf("run workflow: %v", err)
	}

	if finalWF.State != domain.WorkflowUnresolved {
		t.Fatalf("expected state %s on missing manifest, got %s", domain.WorkflowUnresolved, finalWF.State)
	}
	if resp.Outcome != "UNRESOLVED" {
		t.Fatalf("expected outcome UNRESOLVED, got %s", resp.Outcome)
	}

	// Verify ZERO candidates created
	var candCount int
	_ = db.QueryRow(`SELECT count(*) FROM artifact_derivations WHERE tenant_id = ? AND workflow_id = ?`, tenantID, finalWF.ID).Scan(&candCount)
	if candCount != 0 {
		t.Fatalf("expected 0 candidates on missing manifest, got %d", candCount)
	}
}

// Test 3: SHADOW mode blocks candidate creation via Tool Gateway
func TestAgentOrchestrator_ToolGateway_ShadowMode_BlocksCandidateWrite(t *testing.T) {
	db, store, wfService, candService, toolGW, _, _, aiTierURL := setupGatewayTestEnv(t)
	tenantID := "TENANT-GW-SHADOW"
	artID, incID, parentSHAHex := seedQuarantinedFileAndIncident(t, db, store, tenantID)

	orch := NewAgentOrchestrator(wfService, aiTierURL)
	orch.SetToolGateway(toolGW)
	orch.SetCandidateService(candService)
	orch.SetExecutionMode("SHADOW") // Set execution mode to SHADOW

	ctx := context.Background()
	finalWF, resp, err := orch.RunWorkflow(
		ctx, tenantID, "evt-gw-003", "ARTIFACT_QUARANTINED",
		incID, artID, parentSHAHex,
		"pol-bundle-hash-1", "evidence-set-hash-1",
		[]string{"FINDING-001", "FINDING-002"},
		[]interface{}{"Batch debit mismatch"},
		[]string{"RB-01"},
		map[string]interface{}{"decision": "ALLOW_WITH_OBLIGATIONS"},
		map[string]interface{}{"cutoff": "17:00"},
		"corr-gw-3", "trace-gw-3",
	)
	if err != nil {
		t.Fatalf("run workflow: %v", err)
	}

	// In SHADOW mode, candidate writing is blocked with ErrShadowModeProhibited
	if finalWF.State != domain.WorkflowUnresolved {
		t.Fatalf("expected state %s on SHADOW mode blocking, got %s", domain.WorkflowUnresolved, finalWF.State)
	}
	if resp.Outcome != "UNRESOLVED" {
		t.Fatalf("expected outcome UNRESOLVED, got %s", resp.Outcome)
	}

	// Verify ZERO candidate records created in database
	var candCount int
	_ = db.QueryRow(`SELECT count(*) FROM artifact_derivations WHERE tenant_id = ? AND workflow_id = ?`, tenantID, finalWF.ID).Scan(&candCount)
	if candCount != 0 {
		t.Fatalf("expected 0 candidates in SHADOW mode, got %d", candCount)
	}

	var candFiles int
	_ = db.QueryRow(`SELECT count(*) FROM file_instances WHERE tenant_id = ? AND status = 'CANDIDATE'`, tenantID).Scan(&candFiles)
	if candFiles != 0 {
		t.Fatalf("expected 0 candidate files in SHADOW mode, got %d", candFiles)
	}
}

// Test 4: Policy Engine DENY blocks candidate creation via Tool Gateway
func TestAgentOrchestrator_ToolGateway_PolicyDeny_FailsClosed(t *testing.T) {
	db, store, wfService, candService, _, _, _, aiTierURL := setupGatewayTestEnv(t)
	tenantID := "TENANT-GW-DENY"
	artID, incID, parentSHAHex := seedQuarantinedFileAndIncident(t, db, store, tenantID)

	// Create custom policy engine with explicit DENY rule on remediation.candidate.create
	denyRule := &policy.PolicyDefinition{
		PolicyID:      "TEST-DENY-REMEDIATION",
		Version:       1,
		Domain:        policy.DomainRemediation,
		Layer:         policy.LayerSentinelSafety,
		Priority:      1000,
		Status:        policy.StatusActive,
		EffectiveFrom: time.Now().Add(-1 * time.Hour),
		Action:        policy.ActionCreateCandidate,
		Effect:        policy.DecisionDeny,
		ReasonCode:    "TEST_EXPLICIT_DENIAL",
	}

	policies := append(policy.SeedSafetyPolicies(), denyRule)
	engine, err := policy.NewEngine(policies)
	if err != nil {
		t.Fatalf("create policy engine: %v", err)
	}
	reg := toolgateway.NewRegistry()
	_ = toolgateway.RegisterCandidateTool(reg, candService)
	toolStore := toolgateway.NewToolStore(db)
	toolGW := toolgateway.NewToolGatewayService(reg, engine, toolStore)

	orch := NewAgentOrchestrator(wfService, aiTierURL)
	orch.SetToolGateway(toolGW)
	orch.SetCandidateService(candService)

	ctx := context.Background()
	finalWF, resp, err := orch.RunWorkflow(
		ctx, tenantID, "evt-gw-004", "ARTIFACT_QUARANTINED",
		incID, artID, parentSHAHex,
		"pol-bundle-hash-1", "evidence-set-hash-1",
		[]string{"FINDING-001", "FINDING-002"},
		[]interface{}{"Batch debit mismatch"},
		[]string{"RB-01"},
		map[string]interface{}{"decision": "ALLOW_WITH_OBLIGATIONS"},
		map[string]interface{}{"cutoff": "17:00"},
		"corr-gw-4", "trace-gw-4",
	)
	if err != nil {
		t.Fatalf("run workflow: %v", err)
	}

	if finalWF.State != domain.WorkflowUnresolved {
		t.Fatalf("expected state %s on policy DENY, got %s", domain.WorkflowUnresolved, finalWF.State)
	}
	if resp.Outcome != "UNRESOLVED" {
		t.Fatalf("expected outcome UNRESOLVED, got %s", resp.Outcome)
	}

	// Verify ZERO candidates created
	var candCount int
	_ = db.QueryRow(`SELECT count(*) FROM artifact_derivations WHERE tenant_id = ? AND workflow_id = ?`, tenantID, finalWF.ID).Scan(&candCount)
	if candCount != 0 {
		t.Fatalf("expected 0 candidates on policy DENY, got %d", candCount)
	}
}
