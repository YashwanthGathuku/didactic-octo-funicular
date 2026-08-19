package repository

import (
	"context"
	"database/sql"
	"errors"
	"sync"
	"testing"
	"time"

	"sentinel-gateway/internal/auth"
	"sentinel-gateway/internal/domain"

	_ "modernc.org/sqlite"
)

func newWorkflowDB(t *testing.T) *sql.DB {
	t.Helper()
	db, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatal(err)
	}
	db.SetMaxOpenConns(1)
	t.Cleanup(func() { db.Close() })

	schema := `
	CREATE TABLE tenants (id TEXT PRIMARY KEY, name TEXT NOT NULL);
	CREATE TABLE file_instances (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		tenant_id TEXT NOT NULL REFERENCES tenants(id),
		filename TEXT NOT NULL, storage_path TEXT NOT NULL DEFAULT '/p',
		size_bytes INTEGER NOT NULL, sha256_hash TEXT NOT NULL,
		status TEXT NOT NULL,
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
		correlation_id TEXT NOT NULL,
		trace_id TEXT,
		row_version INTEGER NOT NULL DEFAULT 0,
		error_detail TEXT,
		created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
		updated_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
		started_at TIMESTAMP,
		completed_at TIMESTAMP,
		UNIQUE(tenant_id, id)
	);
	CREATE TABLE agent_workflow_events (
		id TEXT PRIMARY KEY,
		workflow_id TEXT NOT NULL REFERENCES agent_workflows(id),
		tenant_id TEXT NOT NULL REFERENCES tenants(id),
		idempotency_key TEXT NOT NULL,
		event_type TEXT NOT NULL,
		state_from TEXT NOT NULL,
		state_to TEXT NOT NULL,
		row_version INTEGER NOT NULL,
		payload TEXT NOT NULL,
		created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
		UNIQUE(tenant_id, workflow_id, idempotency_key)
	);
	CREATE TABLE agent_runs (
		id TEXT PRIMARY KEY,
		workflow_id TEXT NOT NULL REFERENCES agent_workflows(id),
		tenant_id TEXT NOT NULL REFERENCES tenants(id),
		agent_name TEXT NOT NULL,
		agent_version TEXT NOT NULL,
		provider TEXT,
		model_name TEXT,
		model_version TEXT,
		status TEXT NOT NULL,
		input_tokens INTEGER NOT NULL DEFAULT 0,
		output_tokens INTEGER NOT NULL DEFAULT 0,
		latency_ms INTEGER NOT NULL DEFAULT 0,
		estimated_cost_microusd BIGINT NOT NULL DEFAULT 0,
		pricing_version TEXT,
		error_message TEXT,
		started_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
		completed_at TIMESTAMP
	);
	CREATE TABLE agent_steps (
		id TEXT PRIMARY KEY,
		run_id TEXT NOT NULL REFERENCES agent_runs(id),
		workflow_id TEXT NOT NULL REFERENCES agent_workflows(id),
		tenant_id TEXT NOT NULL REFERENCES tenants(id),
		step_number INTEGER NOT NULL,
		step_type TEXT NOT NULL,
		state_from TEXT NOT NULL,
		state_to TEXT NOT NULL,
		decision_payload TEXT,
		authorized_evidence_refs TEXT,
		step_status TEXT NOT NULL DEFAULT 'COMPLETED',
		step_hash TEXT,
		latency_ms INTEGER NOT NULL DEFAULT 0,
		created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP
	);
	CREATE TABLE agent_tool_calls (
		id TEXT PRIMARY KEY,
		step_id TEXT NOT NULL REFERENCES agent_steps(id),
		workflow_id TEXT NOT NULL REFERENCES agent_workflows(id),
		tenant_id TEXT NOT NULL REFERENCES tenants(id),
		tool_name TEXT NOT NULL,
		tool_scope TEXT NOT NULL,
		input_redacted TEXT NOT NULL,
		output_redacted TEXT NOT NULL,
		is_error INTEGER NOT NULL DEFAULT 0,
		latency_ms INTEGER NOT NULL DEFAULT 0,
		executed_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP
	);
	CREATE TABLE verification_attestations (
		id TEXT PRIMARY KEY,
		workflow_id TEXT NOT NULL REFERENCES agent_workflows(id),
		tenant_id TEXT NOT NULL REFERENCES tenants(id),
		verifier_agent TEXT NOT NULL,
		candidate_artifact_id INTEGER REFERENCES file_instances(id),
		candidate_sha256 TEXT NOT NULL,
		findings_count INTEGER NOT NULL DEFAULT 0,
		blocking_findings_count INTEGER NOT NULL DEFAULT 0,
		status TEXT NOT NULL,
		attestation_digest TEXT NOT NULL,
		created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP
	);

	INSERT INTO tenants (id, name) VALUES ('TENANT-A', 'A'), ('TENANT-B', 'B');
	INSERT INTO file_instances (id, tenant_id, filename, size_bytes, sha256_hash, status)
		VALUES (101, 'TENANT-A', 'a1.ach', 100, 'hash-a1', 'QUARANTINED'),
		       (202, 'TENANT-B', 'b1.ach', 200, 'hash-b1', 'QUARANTINED');
	INSERT INTO incidents (id, tenant_id, file_instance_id, type, severity, status)
		VALUES (1001, 'TENANT-A', 101, 'VALIDATION_FAILED', 'CRITICAL', 'OPEN'),
		       (2002, 'TENANT-B', 202, 'VALIDATION_FAILED', 'CRITICAL', 'OPEN');
	`

	if _, err := db.Exec(schema); err != nil {
		t.Fatal(err)
	}
	return db
}

func TestAgentWorkflowRepository_TenantIsolationAndLifecycle(t *testing.T) {
	db := newWorkflowDB(t)
	repo := New(db)
	ctx := context.Background()

	scopeA := scopeFor(t, "TENANT-A", auth.RoleOperator, auth.PermReadTenant)
	scopeB := scopeFor(t, "TENANT-B", auth.RoleOperator, auth.PermReadTenant)

	wfA := &domain.AgentWorkflow{
		ID:             "wf-1001",
		IncidentID:     1001,
		ArtifactID:     101,
		ArtifactSHA256: "hash-a1",
		State:          domain.WorkflowPending,
		AgentName:      "SentinelCoordinator",
		AgentVersion:   "1.0.0",
		CorrelationID:  "corr-1001",
		TraceID:        "trace-1001",
	}

	if err := repo.CreateWorkflow(ctx, scopeA, wfA); err != nil {
		t.Fatalf("create workflow: %v", err)
	}

	// 1. Same-tenant read succeeds
	loadedA, err := repo.GetWorkflow(ctx, scopeA, "wf-1001")
	if err != nil {
		t.Fatalf("get workflow same tenant: %v", err)
	}
	if loadedA.State != domain.WorkflowPending {
		t.Errorf("expected state PENDING, got %s", loadedA.State)
	}
	if loadedA.RowVersion != 1 {
		t.Errorf("expected initial row_version 1, got %d", loadedA.RowVersion)
	}

	// 2. Cross-tenant read returns ErrNotFound (404)
	_, err = repo.GetWorkflow(ctx, scopeB, "wf-1001")
	if !errors.Is(err, ErrNotFound) {
		t.Errorf("expected ErrNotFound on cross-tenant read, got %v", err)
	}

	// 3. Valid transition with optimistic concurrency & idempotency key
	transitioned, err := repo.TransitionWorkflowTx(ctx, scopeA, "wf-1001", 1, domain.WorkflowContextBuilding, "ik-step-1", "", map[string]interface{}{"step": 1})
	if err != nil {
		t.Fatalf("transition workflow: %v", err)
	}
	if transitioned.State != domain.WorkflowContextBuilding {
		t.Errorf("expected state CONTEXT_BUILDING, got %s", transitioned.State)
	}
	if transitioned.RowVersion != 2 {
		t.Errorf("expected row_version 2, got %d", transitioned.RowVersion)
	}

	// 4. Stale version concurrency conflict
	_, err = repo.TransitionWorkflowTx(ctx, scopeA, "wf-1001", 1, domain.WorkflowInvestigating, "ik-step-2", "", nil)
	if !errors.Is(err, ErrWorkflowConflict) {
		t.Errorf("expected ErrWorkflowConflict on stale version, got %v", err)
	}

	// 5. Idempotent replay with same idempotency key does not error or double-bump version
	replayed, err := repo.TransitionWorkflowTx(ctx, scopeA, "wf-1001", 2, domain.WorkflowContextBuilding, "ik-step-1", "", nil)
	if err != nil {
		t.Fatalf("idempotent transition: %v", err)
	}
	if replayed.State != domain.WorkflowContextBuilding {
		t.Errorf("expected state CONTEXT_BUILDING, got %s", replayed.State)
	}
	if replayed.RowVersion != 2 {
		t.Errorf("expected row_version 2, got %d", replayed.RowVersion)
	}
}

func TestAgentWorkflowRepository_ConcurrentIdempotencyReplay(t *testing.T) {
	db := newWorkflowDB(t)
	repo := New(db)
	ctx := context.Background()

	scopeA := scopeFor(t, "TENANT-A", auth.RoleOperator, auth.PermReadTenant)

	wf := &domain.AgentWorkflow{
		ID:             "wf-conc-1",
		IncidentID:     1001,
		ArtifactID:     101,
		ArtifactSHA256: "hash-a1",
		State:          domain.WorkflowPending,
		AgentName:      "SentinelCoordinator",
		AgentVersion:   "1.0.0",
		CorrelationID:  "corr-conc-1",
	}
	if err := repo.CreateWorkflow(ctx, scopeA, wf); err != nil {
		t.Fatal(err)
	}

	// Replay the exact same transition concurrently from 10 goroutines
	var wg sync.WaitGroup
	idempotencyKey := "ik-concurrent-test"
	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, _ = repo.TransitionWorkflowTx(ctx, scopeA, "wf-conc-1", 1, domain.WorkflowContextBuilding, idempotencyKey, "", nil)
		}()
	}
	wg.Wait()

	finalWF, err := repo.GetWorkflow(ctx, scopeA, "wf-conc-1")
	if err != nil {
		t.Fatal(err)
	}
	if finalWF.State != domain.WorkflowContextBuilding {
		t.Errorf("expected state CONTEXT_BUILDING, got %s", finalWF.State)
	}
	if finalWF.RowVersion != 2 {
		t.Errorf("expected row_version exactly 2 after concurrent replays, got %d", finalWF.RowVersion)
	}

	// Verify exactly 1 event recorded in agent_workflow_events
	var eventCount int
	if err := db.QueryRow("SELECT COUNT(*) FROM agent_workflow_events WHERE workflow_id = 'wf-conc-1'").Scan(&eventCount); err != nil {
		t.Fatal(err)
	}
	if eventCount != 1 {
		t.Errorf("expected exactly 1 event in agent_workflow_events, got %d", eventCount)
	}
}

func TestAgentWorkflowRepository_CrashConsistencyAndRollback(t *testing.T) {
	db := newWorkflowDB(t)
	repo := New(db)

	scopeA := scopeFor(t, "TENANT-A", auth.RoleOperator, auth.PermReadTenant)

	wf := &domain.AgentWorkflow{
		ID:             "wf-crash-1",
		IncidentID:     1001,
		ArtifactID:     101,
		ArtifactSHA256: "hash-a1",
		State:          domain.WorkflowPending,
		AgentName:      "SentinelCoordinator",
		AgentVersion:   "1.0.0",
		CorrelationID:  "corr-crash-1",
	}
	if err := repo.CreateWorkflow(context.Background(), scopeA, wf); err != nil {
		t.Fatal(err)
	}

	// Canceled context simulating process crash / context cancel mid-flight
	cancelledCtx, cancel := context.WithCancel(context.Background())
	cancel() // Cancel before executing

	_, err := repo.TransitionWorkflowTx(cancelledCtx, scopeA, "wf-crash-1", 1, domain.WorkflowContextBuilding, "ik-crash-test", "", nil)
	if err == nil {
		t.Error("expected error on cancelled context transition")
	}

	// Verify database is 100% clean: state remains PENDING, version remains 1, 0 events persisted
	checkWF, err := repo.GetWorkflow(context.Background(), scopeA, "wf-crash-1")
	if err != nil {
		t.Fatal(err)
	}
	if checkWF.State != domain.WorkflowPending {
		t.Errorf("state rolled back improperly: got %s, want PENDING", checkWF.State)
	}
	if checkWF.RowVersion != 1 {
		t.Errorf("row_version changed during aborted transaction: got %d, want 1", checkWF.RowVersion)
	}

	var events int
	_ = db.QueryRow("SELECT COUNT(*) FROM agent_workflow_events WHERE workflow_id = 'wf-crash-1'").Scan(&events)
	if events != 0 {
		t.Errorf("expected 0 events after aborted transaction, got %d", events)
	}
}

func TestAgentWorkflowRepository_RunsStepsToolCallsAndAttestations(t *testing.T) {
	db := newWorkflowDB(t)
	repo := New(db)
	ctx := context.Background()

	scopeA := scopeFor(t, "TENANT-A", auth.RoleOperator, auth.PermReadTenant)

	wf := &domain.AgentWorkflow{
		ID:             "wf-audit-1",
		IncidentID:     1001,
		ArtifactID:     101,
		ArtifactSHA256: "hash-a1",
		State:          domain.WorkflowContextBuilding,
		AgentName:      "SentinelCoordinator",
		AgentVersion:   "1.0.0",
		CorrelationID:  "corr-audit-1",
	}
	if err := repo.CreateWorkflow(ctx, scopeA, wf); err != nil {
		t.Fatal(err)
	}

	// 1. Record Run (with explicit provider/model fields and microUSD cost)
	provider := "Google"
	modelName := "gemini-2.5-flash"
	modelVersion := "2026-08"
	pricingVer := "gemini-pricing-v1"

	run := &domain.AgentRun{
		ID:                    "run-1",
		WorkflowID:            "wf-audit-1",
		AgentName:             "TriageAgent",
		AgentVersion:          "1.0.0",
		Provider:              &provider,
		ModelName:             &modelName,
		ModelVersion:          &modelVersion,
		Status:                "COMPLETED",
		InputTokens:           250,
		OutputTokens:          120,
		LatencyMs:             45,
		EstimatedCostMicroUSD: 50, // 50 micro-USD
		PricingVersion:        &pricingVer,
		StartedAt:             time.Now().UTC(),
	}
	if err := repo.RecordRun(ctx, scopeA, run); err != nil {
		t.Fatalf("record run: %v", err)
	}

	// 2. Record Step (structured, strictly non-CoT)
	step := &domain.AgentStep{
		ID:                     "step-1",
		RunID:                  "run-1",
		WorkflowID:             "wf-audit-1",
		StepNumber:             1,
		StepType:               domain.StepDecision,
		StateFrom:              domain.WorkflowContextBuilding,
		StateTo:                domain.WorkflowInvestigating,
		DecisionPayload:        `{"action":"PROCEED_INVESTIGATION","severity":"P2"}`,
		AuthorizedEvidenceRefs: []string{"FINDING-101", "RUNBOOK-RB-05"},
		StepStatus:             "COMPLETED",
		StepHash:               "sha256-step-hash-12345",
		LatencyMs:              15,
	}
	if err := repo.RecordStep(ctx, scopeA, step); err != nil {
		t.Fatalf("record step: %v", err)
	}

	// 3. Record Tool Call
	toolCall := &domain.AgentToolCall{
		ID:             "tool-1",
		StepID:         "step-1",
		WorkflowID:     "wf-audit-1",
		ToolName:       "lookup_finding",
		ToolScope:      domain.ToolScopeRead,
		InputRedacted:  `{"finding_id":"FINDING-101"}`,
		OutputRedacted: `{"code":"0802","severity":"BLOCKING"}`,
		IsError:        false,
		LatencyMs:      12,
	}
	if err := repo.RecordToolCall(ctx, scopeA, toolCall); err != nil {
		t.Fatalf("record tool call: %v", err)
	}

	// 4. Record Verification Attestation
	candidateID := int64(101)
	attestation := &domain.VerificationAttestation{
		ID:                    "attest-1",
		WorkflowID:            "wf-audit-1",
		VerifierAgent:         "VerifierAgent",
		CandidateArtifactID:   &candidateID,
		CandidateSHA256:       "hash-a1",
		FindingsCount:         1,
		BlockingFindingsCount: 1,
		Status:                "CONFIRMED",
		AttestationDigest:     "sha256-attestation-digest-12345",
	}
	if err := repo.RecordAttestation(ctx, scopeA, attestation); err != nil {
		t.Fatalf("record attestation: %v", err)
	}
}
