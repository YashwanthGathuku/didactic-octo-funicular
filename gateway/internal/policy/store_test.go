package policy

import (
	"context"
	"database/sql"
	"errors"
	"testing"
	"time"

	_ "modernc.org/sqlite"
)

func newPolicyTestDB(t *testing.T) *sql.DB {
	t.Helper()
	db, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatal(err)
	}
	db.SetMaxOpenConns(1)
	t.Cleanup(func() { db.Close() })

	schema := `
	CREATE TABLE tenants (id TEXT PRIMARY KEY, name TEXT NOT NULL);
	INSERT INTO tenants (id, name) VALUES ('TENANT-A', 'Tenant A'), ('TENANT-B', 'Tenant B');

	CREATE TABLE policy_definitions (
		policy_id            TEXT NOT NULL,
		version              INTEGER NOT NULL,
		domain               TEXT NOT NULL,
		layer                TEXT NOT NULL,
		priority             INTEGER NOT NULL DEFAULT 100,
		status               TEXT NOT NULL CHECK (status IN ('DRAFT','ACTIVE','RETIRED')),
		effective_from       TIMESTAMP NOT NULL,
		effective_to         TIMESTAMP,
		tenant_id            TEXT REFERENCES tenants(id),
		partner_id           TEXT,
		action               TEXT NOT NULL,
		subject_constraints  TEXT NOT NULL,
		resource_constraints TEXT NOT NULL,
		conditions           TEXT NOT NULL DEFAULT '{}',
		effect               TEXT NOT NULL CHECK (effect IN ('ALLOW','DENY','ALLOW_WITH_OBLIGATIONS','REQUIRE_HUMAN')),
		obligations          TEXT NOT NULL DEFAULT '[]',
		prohibitions         TEXT NOT NULL DEFAULT '[]',
		reason_code          TEXT NOT NULL,
		source_reference     TEXT,
		created_at           TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
		content_hash         TEXT NOT NULL,
		PRIMARY KEY (policy_id, version)
	);

	CREATE TABLE agent_workflows (
		id TEXT PRIMARY KEY,
		tenant_id TEXT NOT NULL REFERENCES tenants(id)
	);
	INSERT INTO agent_workflows (id, tenant_id) VALUES ('wf-101', 'TENANT-A');

	CREATE TABLE agent_workflow_events (
		id              TEXT PRIMARY KEY,
		workflow_id     TEXT NOT NULL REFERENCES agent_workflows(id),
		tenant_id       TEXT NOT NULL REFERENCES tenants(id),
		idempotency_key TEXT NOT NULL,
		event_type      TEXT NOT NULL,
		state_from      TEXT NOT NULL,
		state_to        TEXT NOT NULL,
		row_version     INTEGER NOT NULL,
		payload         TEXT NOT NULL,
		created_at      TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
		UNIQUE(tenant_id, workflow_id, idempotency_key)
	);

	CREATE TABLE agent_policy_decisions (
		id                     TEXT PRIMARY KEY,
		request_id             TEXT NOT NULL,
		tenant_id              TEXT NOT NULL REFERENCES tenants(id),
		workflow_id            TEXT,
		decision               TEXT NOT NULL CHECK (decision IN ('ALLOW','DENY','ALLOW_WITH_OBLIGATIONS','REQUIRE_HUMAN')),
		action                 TEXT NOT NULL,
		reason_codes           TEXT NOT NULL,
		obligations            TEXT NOT NULL,
		prohibitions           TEXT NOT NULL,
		matched_policy_refs    TEXT NOT NULL,
		policy_bundle_id       TEXT NOT NULL DEFAULT 'bundle-sentinel-default',
		policy_bundle_version  TEXT NOT NULL DEFAULT '1.0.0',
		policy_bundle_hash     TEXT NOT NULL,
		manifest               TEXT NOT NULL DEFAULT '[]',
		evaluated_context_hash TEXT NOT NULL,
		evaluated_at           TIMESTAMP NOT NULL,
		evaluator_version      TEXT NOT NULL,
		decision_hash          TEXT NOT NULL,
		created_at             TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP
	);
	`

	if _, err := db.Exec(schema); err != nil {
		t.Fatal(err)
	}
	return db
}

func TestStore_PolicyLifecycleAndImmutability(t *testing.T) {
	db := newPolicyTestDB(t)
	store := NewStore(db)
	ctx := context.Background()

	p1 := &PolicyDefinition{
		PolicyID:      "TEST-POL-01",
		Version:       1,
		Domain:        DomainTool,
		Layer:         LayerEnterprise,
		Priority:      100,
		Status:        StatusActive,
		EffectiveFrom: time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC),
		Action:        "EXECUTE_TOOL",
		SubjectConstraints: SubjectConstraint{
			Type: "AGENT",
		},
		ResourceConstraints: ResourceConstraint{
			Type: "TOOL",
		},
		Effect:     DecisionAllow,
		ReasonCode: "TOOL_PERMITTED",
	}

	// 1. Store Version 1
	if err := store.StorePolicyDefinition(ctx, p1); err != nil {
		t.Fatalf("store policy v1: %v", err)
	}

	// 2. Fetch Version 1
	fetched1, err := store.GetPolicyDefinition(ctx, "TEST-POL-01", 1)
	if err != nil {
		t.Fatalf("get policy v1: %v", err)
	}
	if fetched1.PolicyID != "TEST-POL-01" || fetched1.Version != 1 {
		t.Errorf("unexpected fetched policy: %+v", fetched1)
	}
	if fetched1.ContentHash != p1.ContentHash {
		t.Errorf("content hash mismatch: %s vs %s", fetched1.ContentHash, p1.ContentHash)
	}

	// 3. Attempt in-place mutation / re-store of active version 1 (must fail)
	err = store.StorePolicyDefinition(ctx, p1)
	if !errors.Is(err, ErrActivePolicyImmutable) {
		t.Fatalf("expected ErrActivePolicyImmutable on re-storing active v1, got: %v", err)
	}

	// 4. Create Version 2 (immutable evolution)
	p2 := &PolicyDefinition{
		PolicyID:      "TEST-POL-01",
		Version:       2,
		Domain:        DomainTool,
		Layer:         LayerEnterprise,
		Priority:      100,
		Status:        StatusActive,
		EffectiveFrom: time.Date(2026, 6, 1, 0, 0, 0, 0, time.UTC),
		Action:        "EXECUTE_TOOL",
		Effect:        DecisionAllowWithObligations,
		Obligations: []Obligation{
			{Type: ObligationAuditRequired},
		},
		ReasonCode: "TOOL_PERMITTED_WITH_AUDIT",
	}

	if err := store.StorePolicyDefinition(ctx, p2); err != nil {
		t.Fatalf("store policy v2: %v", err)
	}

	// 5. Verify both v1 and v2 exist independently
	v1Check, err := store.GetPolicyDefinition(ctx, "TEST-POL-01", 1)
	if err != nil || v1Check.Effect != DecisionAllow {
		t.Errorf("v1 corrupted after v2 insertion: %+v", v1Check)
	}
	v2Check, err := store.GetPolicyDefinition(ctx, "TEST-POL-01", 2)
	if err != nil || v2Check.Effect != DecisionAllowWithObligations {
		t.Errorf("v2 corrupted: %+v", v2Check)
	}
}

func TestStore_DecisionPersistenceAndOutboxEventRollback(t *testing.T) {
	db := newPolicyTestDB(t)
	store := NewStore(db)
	ctx := context.Background()

	evalTime := time.Date(2026, 8, 19, 12, 0, 0, 0, time.UTC)
	dec := &PolicyDecision{
		DecisionID:          "pdec-test-999",
		RequestID:           "req-999",
		Decision:            DecisionAllowWithObligations,
		Action:              ActionCreateCandidate,
		ReasonCodes:         []string{"DERIVED_CANDIDATE_ONLY"},
		PolicyBundleID:      "bundle-sentinel-default",
		PolicyBundleVersion: "1.0.0",
		Obligations: []Obligation{
			{Type: ObligationCandidateOnly},
			{Type: ObligationDeterministicRevalidation},
		},
		Prohibitions: []Prohibition{
			{Type: ProhibitionMutateOriginal, Description: "Mutating original is forbidden"},
		},
		MatchedPolicyRefs:    []string{"SF-SAFE-004:v1"},
		PolicyBundleHash:     "sha256-bundle-hash",
		EvaluatedContextHash: "sha256-context-hash",
		EvaluatedAt:          evalTime,
		EvaluatorVersion:     EvaluatorVersion,
	}

	// 1. Successful atomic transaction
	if err := store.RecordDecision(ctx, "TENANT-A", "wf-101", dec); err != nil {
		t.Fatalf("record decision: %v", err)
	}

	// Read by Tenant A -> OK
	fetched, err := store.GetDecision(ctx, "TENANT-A", "pdec-test-999")
	if err != nil {
		t.Fatalf("get decision same tenant: %v", err)
	}
	if fetched.Decision != DecisionAllowWithObligations {
		t.Errorf("expected decision %s, got %s", DecisionAllowWithObligations, fetched.Decision)
	}
	if len(fetched.Obligations) != 2 || fetched.Obligations[0].Type != ObligationCandidateOnly {
		t.Errorf("unexpected fetched obligations: %+v", fetched.Obligations)
	}

	// Verify outbox/workflow event was created
	var eventCount int
	err = db.QueryRowContext(ctx, "SELECT count(*) FROM agent_workflow_events WHERE idempotency_key = ?", dec.DecisionID).Scan(&eventCount)
	if err != nil || eventCount != 1 {
		t.Fatalf("expected 1 workflow event for decision, got %d (err: %v)", eventCount, err)
	}

	// 2. Transaction Rollback Fault-Injection Test
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		t.Fatal(err)
	}

	decRollback := &PolicyDecision{
		DecisionID:          "pdec-rollback-123",
		RequestID:           "req-rollback",
		Decision:            DecisionDeny,
		Action:              ActionReleaseArtifact,
		PolicyBundleHash:    "hash-1",
		EvaluatedAt:         evalTime,
		EvaluatorVersion:    EvaluatorVersion,
	}

	if err := store.RecordDecisionTx(ctx, tx, "TENANT-A", "wf-101", decRollback); err != nil {
		t.Fatalf("record decision tx: %v", err)
	}

	// Force explicit rollback
	if err := tx.Rollback(); err != nil {
		t.Fatal(err)
	}

	// Prove neither the decision nor the event exists
	_, err = store.GetDecision(ctx, "TENANT-A", "pdec-rollback-123")
	if !errors.Is(err, ErrDecisionNotFound) {
		t.Errorf("expected ErrDecisionNotFound after rollback, got %v", err)
	}

	var rbEventCount int
	_ = db.QueryRowContext(ctx, "SELECT count(*) FROM agent_workflow_events WHERE idempotency_key = 'pdec-rollback-123'").Scan(&rbEventCount)
	if rbEventCount != 0 {
		t.Errorf("expected 0 workflow events after rollback, got %d", rbEventCount)
	}
}
