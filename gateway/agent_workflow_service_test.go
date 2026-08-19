package main

import (
	"context"
	"database/sql"
	"errors"
	"testing"

	"sentinel-gateway/internal/domain"
	"sentinel-gateway/internal/repository"

	_ "modernc.org/sqlite"
)

func TestAgentWorkflowService_FullLifecycleAndLedger(t *testing.T) {
	db, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	defer db.Close()

	if _, err := Migrate(db); err != nil {
		t.Fatalf("migrate: %v", err)
	}

	// Seed tenant and artifact
	_, err = db.Exec(`
		INSERT OR IGNORE INTO tenants (id, name) VALUES ('TENANT-1', 'Tenant One'), ('TENANT-2', 'Tenant Two');
		INSERT INTO file_instances (id, tenant_id, filename, storage_path, size_bytes, sha256_hash, status, received_at)
		VALUES (501, 'TENANT-1', 'file_1.ach', 's3://bucket/file_1.ach', 1024, 'sha-501', 'QUARANTINED', CURRENT_TIMESTAMP),
		       (502, 'TENANT-2', 'file_2.ach', 's3://bucket/file_2.ach', 2048, 'sha-502', 'QUARANTINED', CURRENT_TIMESTAMP);
		INSERT INTO incidents (id, tenant_id, file_instance_id, type, severity, status)
		VALUES (601, 'TENANT-1', 501, 'VALIDATION_FAILED', 'CRITICAL', 'OPEN'),
		       (602, 'TENANT-2', 502, 'VALIDATION_FAILED', 'CRITICAL', 'OPEN');
	`)
	if err != nil {
		t.Fatalf("seed test data: %v", err)
	}

	svc := NewAgentWorkflowService(db)
	ctx := context.Background()

	// 1. Create Workflow
	wf, err := svc.CreateWorkflow(
		ctx,
		"TENANT-1",
		601,
		501,
		"sha-501",
		"SentinelCoordinator",
		"1.0.0",
		"corr-wf-1",
		"trace-wf-1",
	)
	if err != nil {
		t.Fatalf("create workflow: %v", err)
	}

	if wf.State != domain.WorkflowPending {
		t.Errorf("expected initial state PENDING, got %s", wf.State)
	}
	if wf.RowVersion != 1 {
		t.Errorf("expected row_version 1, got %d", wf.RowVersion)
	}

	// Verify creation event in ledger
	ledger1, err := GetLedger(db, "TENANT-1")
	if err != nil {
		t.Fatalf("get ledger: %v", err)
	}
	if len(ledger1.Events) == 0 || ledger1.Events[len(ledger1.Events)-1].EventType != "AGENT_WORKFLOW_CREATED" {
		t.Fatalf("expected AGENT_WORKFLOW_CREATED event in ledger, got %+v", ledger1.Events)
	}

	// 2. Transition Workflow: PENDING -> CONTEXT_BUILDING
	t1, err := svc.TransitionWorkflow(
		ctx,
		"TENANT-1",
		wf.ID,
		1,
		domain.WorkflowContextBuilding,
		"ik-step-1",
		"",
		"Starting context assembly and Model Armor screening.",
	)
	if err != nil {
		t.Fatalf("transition to CONTEXT_BUILDING: %v", err)
	}
	if t1.State != domain.WorkflowContextBuilding {
		t.Errorf("expected state CONTEXT_BUILDING, got %s", t1.State)
	}
	if t1.RowVersion != 2 {
		t.Errorf("expected row_version 2, got %d", t1.RowVersion)
	}

	// 3. Transition: CONTEXT_BUILDING -> INVESTIGATING
	t2, err := svc.TransitionWorkflow(
		ctx,
		"TENANT-1",
		wf.ID,
		2,
		domain.WorkflowInvestigating,
		"ik-step-2",
		"",
		"Triage and compliance investigation underway.",
	)
	if err != nil {
		t.Fatalf("transition to INVESTIGATING: %v", err)
	}
	if t2.State != domain.WorkflowInvestigating {
		t.Errorf("expected state INVESTIGATING, got %s", t2.State)
	}
	if t2.RowVersion != 3 {
		t.Errorf("expected row_version 3, got %d", t2.RowVersion)
	}

	// 4. Concurrency Conflict: attempt transition with old version (version 2 instead of 3)
	_, err = svc.TransitionWorkflow(
		ctx,
		"TENANT-1",
		wf.ID,
		2,
		domain.WorkflowPlanning,
		"ik-step-conflict",
		"",
		"Should fail due to concurrency conflict.",
	)
	if !errors.Is(err, repository.ErrWorkflowConflict) {
		t.Errorf("expected ErrWorkflowConflict on stale version, got %v", err)
	}

	// 5. Invalid Transition: attempt illegal jump (INVESTIGATING -> COMPLETED)
	_, err = svc.TransitionWorkflow(
		ctx,
		"TENANT-1",
		wf.ID,
		3,
		domain.WorkflowCompleted,
		"ik-step-illegal",
		"",
		"Illegal direct transition.",
	)
	if err == nil {
		t.Errorf("expected error on illegal transition INVESTIGATING -> COMPLETED, got nil")
	}

	// 6. Idempotent Transition: re-send same idempotency key "ik-step-2" with version 3
	idempotent, err := svc.TransitionWorkflow(
		ctx,
		"TENANT-1",
		wf.ID,
		3,
		domain.WorkflowInvestigating,
		"ik-step-2",
		"",
		"Duplicate event.",
	)
	if err != nil {
		t.Fatalf("idempotent transition failed: %v", err)
	}
	if idempotent.State != domain.WorkflowInvestigating {
		t.Errorf("expected state INVESTIGATING, got %s", idempotent.State)
	}
	if idempotent.RowVersion != 3 {
		t.Errorf("expected row_version 3 unchanged, got %d", idempotent.RowVersion)
	}

	// 7. Tenant Isolation: Tenant 2 cannot access or transition Tenant 1's workflow
	_, err = svc.GetWorkflow(ctx, "TENANT-2", wf.ID)
	if !errors.Is(err, repository.ErrNotFound) {
		t.Errorf("expected ErrNotFound for cross-tenant GetWorkflow, got %v", err)
	}

	_, err = svc.TransitionWorkflow(
		ctx,
		"TENANT-2",
		wf.ID,
		3,
		domain.WorkflowPlanning,
		"ik-cross-tenant",
		"",
		"Cross-tenant transition attempt.",
	)
	if !errors.Is(err, repository.ErrNotFound) {
		t.Errorf("expected ErrNotFound for cross-tenant TransitionWorkflow, got %v", err)
	}

	// 8. Progress to terminal state: INVESTIGATING -> PLANNING -> REMEDIATING -> VALIDATING_CANDIDATE -> VERIFIED -> HUMAN_REVIEW -> COMPLETED
	steps := []struct {
		expectedVer int
		nextState   domain.AgentWorkflowState
		ik          string
	}{
		{3, domain.WorkflowPlanning, "ik-p1"},
		{4, domain.WorkflowRemediating, "ik-p2"},
		{5, domain.WorkflowValidatingCandidate, "ik-p3"},
		{6, domain.WorkflowVerified, "ik-p4"},
		{7, domain.WorkflowHumanReview, "ik-p5"},
		{8, domain.WorkflowCompleted, "ik-p6"},
	}

	for _, step := range steps {
		res, err := svc.TransitionWorkflow(ctx, "TENANT-1", wf.ID, step.expectedVer, step.nextState, step.ik, "", "Progression step")
		if err != nil {
			t.Fatalf("failed transition to %s (ver %d): %v", step.nextState, step.expectedVer, err)
		}
		if res.State != step.nextState {
			t.Fatalf("expected state %s, got %s", step.nextState, res.State)
		}
	}

	// Verify terminal workflow state
	finalWF, err := svc.GetWorkflow(ctx, "TENANT-1", wf.ID)
	if err != nil {
		t.Fatalf("get final workflow: %v", err)
	}
	if finalWF.State != domain.WorkflowCompleted {
		t.Errorf("expected final state COMPLETED, got %s", finalWF.State)
	}
	if finalWF.CompletedAt == nil {
		t.Errorf("expected completed_at timestamp to be set on terminal state")
	}

	// Verify ledger integrity
	finalLedger, err := GetLedger(db, "TENANT-1")
	if err != nil {
		t.Fatalf("final ledger get: %v", err)
	}
	if !finalLedger.IsChainValid {
		t.Errorf("expected valid ledger hash chain after workflow transitions")
	}
}
