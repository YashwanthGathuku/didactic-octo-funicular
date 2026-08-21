package main

import (
	"context"
	"database/sql"
	"testing"

	"sentinel-gateway/internal/domain"

	_ "modernc.org/sqlite"
)

func setupTestDB(t *testing.T) *sql.DB {
	db, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatalf("open db: %v", err)
	}

	if _, err := Migrate(db); err != nil {
		t.Fatalf("migrate: %v", err)
	}

	// Seed tenant, artifact, and incident
	_, err = db.Exec(`
		INSERT OR IGNORE INTO tenants (id, name) VALUES ('TENANT-GO-01', 'Tenant Go Authority');
		INSERT INTO file_instances (id, tenant_id, filename, storage_path, size_bytes, sha256_hash, status, received_at)
		VALUES (101, 'TENANT-GO-01', 'nacha_batch_01.ach', 's3://bucket/nacha_01.ach', 2048, 'sha256-original-aaa', 'QUARANTINED', CURRENT_TIMESTAMP);
		INSERT INTO incidents (id, tenant_id, file_instance_id, type, severity, status)
		VALUES (201, 'TENANT-GO-01', 101, 'VALIDATION_FAILED', 'HIGH', 'OPEN');
	`)
	if err != nil {
		t.Fatalf("seed test data: %v", err)
	}

	return db
}

func TestGoControlPlane_TriggerIdempotencyAndRestart(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()
	ctx := context.Background()

	// 1. Process 1: Create workflow W1 for Trigger E1
	svc1 := NewAgentWorkflowService(db)
	orch1 := NewAgentOrchestrator(svc1, "")

	wf1, resp1, err := orch1.RunWorkflow(
		ctx,
		"TENANT-GO-01",
		"EVT-TRIG-GO-001",
		"ARTIFACT_QUARANTINED",
		201,
		101,
		"sha256-original-aaa",
		"bundle-policy-v1",
		"ev-hash-set-01",
		[]string{"FINDING-001", "RUNBOOK-RB-05"},
		[]interface{}{"Batch entry hash mismatch in batch 001"},
		[]string{"RB-01", "RB-05"},
		map[string]interface{}{"decision": "ALLOW", "decision_id": "POL-DEC-001"},
		map[string]interface{}{"sla_status": "ON_TRACK"},
		"corr-go-001",
		"trace-go-001",
	)
	if err != nil {
		t.Fatalf("run workflow process 1: %v", err)
	}
	if wf1.ID == "" {
		t.Fatalf("expected non-empty workflow ID")
	}
	if resp1.Outcome != "READY_FOR_REMEDIATION" {
		t.Fatalf("expected outcome READY_FOR_REMEDIATION, got %s", resp1.Outcome)
	}
	if wf1.State != domain.WorkflowCompleted {
		t.Fatalf("expected completed state, got %s", wf1.State)
	}

	// 2. Destroy Process 1 objects (simulate crash)
	wf1ID := wf1.ID
	orch1 = nil
	svc1 = nil

	// 3. Process 2: Re-create Go service and orchestrator objects from database
	svc2 := NewAgentWorkflowService(db)
	orch2 := NewAgentOrchestrator(svc2, "")

	// 4. Resubmit identical trigger event E1
	wf2, resp2, err := orch2.RunWorkflow(
		ctx,
		"TENANT-GO-01",
		"EVT-TRIG-GO-001",
		"ARTIFACT_QUARANTINED",
		201,
		101,
		"sha256-original-aaa",
		"bundle-policy-v1",
		"ev-hash-set-01",
		[]string{"FINDING-001", "RUNBOOK-RB-05"},
		[]interface{}{"Batch entry hash mismatch in batch 001"},
		[]string{"RB-01", "RB-05"},
		map[string]interface{}{"decision": "ALLOW", "decision_id": "POL-DEC-001"},
		map[string]interface{}{"sla_status": "ON_TRACK"},
		"corr-go-001",
		"trace-go-001",
	)
	if err != nil {
		t.Fatalf("run workflow process 2: %v", err)
	}

	// 5. Invariants: Same workflow ID, no duplicate workflow record
	if wf2.ID != wf1ID {
		t.Errorf("expected same workflow ID %s on restart, got %s", wf1ID, wf2.ID)
	}
	if resp2.Outcome != resp1.Outcome {
		t.Errorf("expected same outcome %s on restart, got %s", resp1.Outcome, resp2.Outcome)
	}
}

func TestGoControlPlane_PolicyBundleTOCTOU_FailsClosed(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()
	ctx := context.Background()

	svc := NewAgentWorkflowService(db)

	// 1. Direct unit verification of CheckTOCTOU
	valid, violation := svc.CheckTOCTOU("bundle-policy-v1", "bundle-policy-v2", "sha256-aaa", "sha256-aaa")
	if valid || violation != "POLICY_CONTEXT_STALE" {
		t.Errorf("expected POLICY_CONTEXT_STALE, got valid=%v, violation=%s", valid, violation)
	}

	validArt, violationArt := svc.CheckTOCTOU("bundle-policy-v1", "bundle-policy-v1", "sha256-original", "sha256-mutated")
	if validArt || violationArt != "RESOURCE_CONTEXT_STALE" {
		t.Errorf("expected RESOURCE_CONTEXT_STALE, got valid=%v, violation=%s", validArt, violationArt)
	}

	// 2. Integration verification with workflow state transition
	wf, _, err := svc.GetOrCreateWorkflowByTrigger(
		ctx, "TENANT-GO-01", "EVT-TRIG-TOCTOU-01", "ARTIFACT_QUARANTINED",
		201, 101, "sha256-original-aaa", "bundle-policy-v2", "ev-hash-set-01", "corr-toctou-01", "trace-toctou-01",
	)
	if err != nil {
		t.Fatalf("create workflow: %v", err)
	}

	// Transition to PLANNING
	wf, _ = svc.TransitionWorkflow(ctx, "TENANT-GO-01", wf.ID, wf.RowVersion, domain.WorkflowContextBuilding, "ik-1", "", "")
	wf, _ = svc.TransitionWorkflow(ctx, "TENANT-GO-01", wf.ID, wf.RowVersion, domain.WorkflowInvestigating, "ik-2", "", "")
	wf, _ = svc.TransitionWorkflow(ctx, "TENANT-GO-01", wf.ID, wf.RowVersion, domain.WorkflowPlanning, "ik-3", "", "")

	// Check TOCTOU: plan was v1, current is v2 -> record violation and fail closed to UNRESOLVED
	_ = svc.RecordWorkflowEvent(ctx, "TENANT-GO-01", wf.ID, "ik-toctou-ev", violation, domain.WorkflowPlanning, domain.WorkflowUnresolved, wf.RowVersion, map[string]interface{}{"violation": violation})
	wf, err = svc.TransitionWorkflow(ctx, "TENANT-GO-01", wf.ID, wf.RowVersion, domain.WorkflowUnresolved, "ik-toctou-tx", violation, "TOCTOU check failed")
	if err != nil {
		t.Fatalf("transition unresolved: %v", err)
	}

	if wf.State != domain.WorkflowUnresolved {
		t.Errorf("expected state UNRESOLVED, got %s", wf.State)
	}

	// Verify events in Go event journal
	events, err := svc.GetEvents(ctx, "TENANT-GO-01", wf.ID)
	if err != nil {
		t.Fatalf("get events: %v", err)
	}
	foundViolation := false
	for _, ev := range events {
		if ev.EventType == "POLICY_CONTEXT_STALE" {
			foundViolation = true
			break
		}
	}
	if !foundViolation {
		t.Errorf("expected POLICY_CONTEXT_STALE in event journal")
	}
}

func TestGoControlPlane_DENY_vs_REQUIRE_HUMAN_Authority(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()
	ctx := context.Background()

	svc := NewAgentWorkflowService(db)
	orch := NewAgentOrchestrator(svc, "")

	// Case A: Policy Decision is DENY
	wfDeny, respDeny, err := orch.RunWorkflow(
		ctx,
		"TENANT-GO-01",
		"EVT-TRIG-DENY-01",
		"ARTIFACT_QUARANTINED",
		201,
		101,
		"sha256-original-aaa",
		"bundle-policy-v1",
		"ev-hash-set-01",
		[]string{"FINDING-001"},
		[]interface{}{"Batch entry hash mismatch"},
		[]string{"RB-01"},
		map[string]interface{}{"decision": "DENY", "decision_id": "POL-DEC-DENY-01"},
		map[string]interface{}{},
		"corr-deny-01",
		"trace-deny-01",
	)
	if err != nil {
		t.Fatalf("run deny workflow: %v", err)
	}
	if respDeny.Outcome != "POLICY_BLOCKED" {
		t.Errorf("expected outcome POLICY_BLOCKED on DENY, got %s", respDeny.Outcome)
	}
	if wfDeny.State != domain.WorkflowPolicyDenied {
		t.Errorf("expected state POLICY_DENIED on DENY, got %s", wfDeny.State)
	}

	// Case B: Policy Decision is REQUIRE_HUMAN
	wfHuman, respHuman, err := orch.RunWorkflow(
		ctx,
		"TENANT-GO-01",
		"EVT-TRIG-HUMAN-01",
		"ARTIFACT_QUARANTINED",
		201,
		101,
		"sha256-original-aaa",
		"bundle-policy-v1",
		"ev-hash-set-01",
		[]string{"FINDING-001"},
		[]interface{}{"Batch entry hash mismatch"},
		[]string{"RB-01"},
		map[string]interface{}{"decision": "REQUIRE_HUMAN", "decision_id": "POL-DEC-HUMAN-01"},
		map[string]interface{}{},
		"corr-human-01",
		"trace-human-01",
	)
	if err != nil {
		t.Fatalf("run human workflow: %v", err)
	}
	if respHuman.Outcome != "HUMAN_AUTHORIZATION_REQUIRED" {
		t.Errorf("expected outcome HUMAN_AUTHORIZATION_REQUIRED on REQUIRE_HUMAN, got %s", respHuman.Outcome)
	}
	if wfHuman.State != domain.WorkflowHumanReview {
		t.Errorf("expected state HUMAN_REVIEW on REQUIRE_HUMAN, got %s", wfHuman.State)
	}
}

func TestGoControlPlane_ResultFreshness7ProtectedBindings(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()
	ctx := context.Background()

	svc := NewAgentWorkflowService(db)

	wf, _, err := svc.GetOrCreateWorkflowByTrigger(
		ctx, "TENANT-GO-01", "EVT-TRIG-BIND-01", "ARTIFACT_QUARANTINED",
		201, 101, "sha256-aaa", "bundle-v1", "ev-hash-01", "corr-bind-01", "trace-bind-01",
	)
	if err != nil {
		t.Fatalf("get or create workflow: %v", err)
	}

	// Record a specialist step with 7 protected binding hashes
	stepPayload := `{"agent_name":"DiagnosisAgent","manifest_hash":"man-01","input_context_hash":"in-01","artifact_sha256":"sha256-aaa","policy_bundle_hash":"bundle-v1","authorized_evidence_set_hash":"ev-hash-01"}`
	err = svc.RecordStep(ctx, "TENANT-GO-01", &domain.AgentStep{
		ID:              "step-bind-01",
		RunID:           "run-bind-01",
		WorkflowID:      wf.ID,
		TenantID:        "TENANT-GO-01",
		StepNumber:      1,
		StepType:        domain.StepDecision,
		StateFrom:       domain.WorkflowPlanning,
		StateTo:         domain.WorkflowPlanning,
		DecisionPayload: stepPayload,
		StepStatus:      "COMPLETED",
	})
	if err != nil {
		t.Fatalf("record step: %v", err)
	}

	// Check with matching 7 bindings -> Fresh (true)
	_, fresh, err := svc.EvaluateResultFreshness(
		ctx, "TENANT-GO-01", wf.ID, "DiagnosisAgent",
		"man-01", "in-01", "sha256-aaa", "bundle-v1", "ev-hash-01",
	)
	if err != nil || !fresh {
		t.Errorf("expected result to be fresh with matching bindings, got fresh=%v, err=%v", fresh, err)
	}

	// Check with mismatched manifest_hash -> Stale (false)
	_, freshMan, _ := svc.EvaluateResultFreshness(
		ctx, "TENANT-GO-01", wf.ID, "DiagnosisAgent",
		"man-MODIFIED", "in-01", "sha256-aaa", "bundle-v1", "ev-hash-01",
	)
	if freshMan {
		t.Errorf("expected result to be stale with mismatched manifest_hash")
	}

	// Check with mismatched artifact_sha -> Stale (false)
	_, freshArt, _ := svc.EvaluateResultFreshness(
		ctx, "TENANT-GO-01", wf.ID, "DiagnosisAgent",
		"man-01", "in-01", "sha256-MUTATED", "bundle-v1", "ev-hash-01",
	)
	if freshArt {
		t.Errorf("expected result to be stale with mismatched artifact_sha256")
	}
}
