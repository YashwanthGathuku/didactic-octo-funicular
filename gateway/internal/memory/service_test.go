package memory

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"

	"sentinel-gateway/internal/auth"
	"sentinel-gateway/internal/jobs"
	"sentinel-gateway/internal/repository"

	_ "modernc.org/sqlite"
)

func setupMemoryTestDB(t *testing.T) *sql.DB {
	t.Helper()
	db, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatalf("open in-memory db: %v", err)
	}
	db.SetMaxOpenConns(1)

	schema := `
	CREATE TABLE tenants (id TEXT PRIMARY KEY, name TEXT NOT NULL);
	CREATE TABLE file_instances (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		tenant_id TEXT NOT NULL REFERENCES tenants(id),
		filename TEXT NOT NULL, storage_path TEXT NOT NULL,
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
		created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP
	);
	CREATE TABLE candidate_verifications (
		id TEXT PRIMARY KEY,
		tenant_id TEXT NOT NULL REFERENCES tenants(id),
		workflow_id TEXT NOT NULL REFERENCES agent_workflows(id),
		candidate_artifact_id INTEGER NOT NULL REFERENCES file_instances(id),
		candidate_sha256 TEXT NOT NULL,
		parent_artifact_id INTEGER NOT NULL REFERENCES file_instances(id),
		parent_sha256 TEXT NOT NULL,
		derivation_id TEXT NOT NULL,
		derivation_hash TEXT NOT NULL,
		remediation_plan_hash TEXT NOT NULL,
		p07_validation_run_id TEXT NOT NULL,
		p08_validation_run_id TEXT NOT NULL,
		rulepack_hash TEXT NOT NULL,
		policy_bundle_hash TEXT NOT NULL,
		deterministic_outcome TEXT NOT NULL,
		verification_hash TEXT NOT NULL,
		created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP
	);
	CREATE TABLE operational_memories (
		id TEXT PRIMARY KEY,
		tenant_id TEXT NOT NULL REFERENCES tenants(id),
		memory_type TEXT NOT NULL,
		subject_type TEXT NOT NULL,
		subject_ref TEXT NOT NULL,
		fact_type TEXT NOT NULL,
		structured_value TEXT NOT NULL,
		confidence_source TEXT NOT NULL,
		classification TEXT NOT NULL DEFAULT 'INTERNAL',
		status TEXT NOT NULL DEFAULT 'ACTIVE',
		valid_from TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
		expires_at TIMESTAMP,
		superseded_by TEXT REFERENCES operational_memories(id),
		created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
		created_by TEXT NOT NULL,
		memory_hash TEXT NOT NULL,
		UNIQUE(tenant_id, memory_hash)
	);
	CREATE TABLE memory_sources (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		memory_id TEXT NOT NULL REFERENCES operational_memories(id),
		tenant_id TEXT NOT NULL REFERENCES tenants(id),
		source_ref TEXT NOT NULL,
		source_hash TEXT NOT NULL,
		source_verification_ref TEXT,
		created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP
	);
	CREATE TABLE memory_revisions (
		id TEXT PRIMARY KEY,
		memory_id TEXT NOT NULL REFERENCES operational_memories(id),
		tenant_id TEXT NOT NULL REFERENCES tenants(id),
		revision_number INTEGER NOT NULL,
		previous_hash TEXT,
		new_hash TEXT NOT NULL,
		transition_type TEXT NOT NULL,
		reason TEXT NOT NULL,
		actor_id TEXT NOT NULL,
		created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
		UNIQUE(tenant_id, memory_id, revision_number)
	);
	`
	if _, err := db.Exec(schema); err != nil {
		t.Fatalf("setup test schema: %v", err)
	}

	// Seed tenant and verification
	_, err = db.Exec(`INSERT INTO tenants (id, name) VALUES ('tenant-test', 'Test Tenant')`)
	if err != nil {
		t.Fatalf("seed tenant: %v", err)
	}
	_, err = db.Exec(`
		INSERT INTO candidate_verifications (
			id, tenant_id, workflow_id, candidate_artifact_id, candidate_sha256,
			parent_artifact_id, parent_sha256, derivation_id, derivation_hash,
			remediation_plan_hash, p07_validation_run_id, p08_validation_run_id,
			rulepack_hash, policy_bundle_hash, deterministic_outcome, verification_hash
		) VALUES (
			'verif-001', 'tenant-test', 'wf-001', 2, 'candhash1234',
			1, 'parenthash1234', 'deriv-001', 'derivhash1234',
			'planhash1234', 'run-p07', 'run-p08',
			'rulepack-1', 'policy-1', 'PASS', 'verifhash1234'
		)`)
	if err != nil {
		t.Fatalf("seed verification: %v", err)
	}

	return db
}

func makeTestScope(tenantID string) repository.Scope {
	p := &auth.Principal{
		Subject: "test-user",
		Memberships: []auth.Membership{
			{TenantID: tenantID, Roles: []auth.Role{auth.RoleOperator}},
		},
	}
	scope, _ := repository.NewScope(p, tenantID, auth.PermReadTenant)
	return scope
}

func TestComputeMemoryHash_CanonicalDeterminism(t *testing.T) {
	fact := RemediationSuccessFact{
		WorkflowID:           "wf-001",
		IncidentID:           100,
		ParentArtifactSHA:    "abc",
		CandidateArtifactSHA: "def",
		AppliedOperations:    []string{"OP_B", "OP_A"},
	}
	valJSON, _ := json.Marshal(fact)

	m1 := &OperationalMemoryRecord{
		MemoryID:               "mem-001",
		TenantID:               "tenant-test",
		MemoryType:             MemoryTypeOperationalFact,
		SubjectType:            SubjectTypePartner,
		SubjectRef:             "PARTNER-01",
		FactType:               FactTypeVerifiedRemediationSuccess,
		StructuredValue:        valJSON,
		SourceRefs:             []string{"ref-z", "ref-a"},
		SourceHashes:           []string{"hash-z", "hash-a"},
		SourceVerificationRefs: []string{"verif-001"},
		ConfidenceSource:       ConfidenceSourceVerifiedWorkflow,
		Classification:         ClassificationInternal,
		ValidFrom:              time.Date(2026, 8, 23, 12, 0, 0, 0, time.UTC),
		CreatedAt:              time.Date(2026, 8, 23, 12, 0, 0, 0, time.UTC),
		CreatedBy:              "test-actor",
	}

	h1, err := ComputeMemoryHash(m1)
	if err != nil {
		t.Fatalf("compute hash m1: %v", err)
	}

	// Permute array order in m2
	m2 := &OperationalMemoryRecord{
		MemoryID:               "mem-001",
		TenantID:               "tenant-test",
		MemoryType:             MemoryTypeOperationalFact,
		SubjectType:            SubjectTypePartner,
		SubjectRef:             "PARTNER-01",
		FactType:               FactTypeVerifiedRemediationSuccess,
		StructuredValue:        valJSON,
		SourceRefs:             []string{"ref-a", "ref-z"},
		SourceHashes:           []string{"hash-a", "hash-z"},
		SourceVerificationRefs: []string{"verif-001"},
		ConfidenceSource:       ConfidenceSourceVerifiedWorkflow,
		Classification:         ClassificationInternal,
		ValidFrom:              time.Date(2026, 8, 23, 12, 0, 0, 0, time.UTC),
		CreatedAt:              time.Date(2026, 8, 23, 12, 0, 0, 0, time.UTC),
		CreatedBy:              "test-actor",
	}

	h2, err := ComputeMemoryHash(m2)
	if err != nil {
		t.Fatalf("compute hash m2: %v", err)
	}

	if h1 != h2 {
		t.Fatalf("expected identical canonical hash despite array permutation, got %s vs %s", h1, h2)
	}
}

func TestEligibilityGate_RejectsInvalidMemoryType(t *testing.T) {
	db := setupMemoryTestDB(t)
	gate := NewEligibilityGate(db)

	m := &OperationalMemoryRecord{
		MemoryID:               "mem-001",
		TenantID:               "tenant-test",
		MemoryType:             MemoryTypeManagedSemantic, // Not M1
		SubjectType:            SubjectTypePartner,
		SubjectRef:             "PARTNER-01",
		FactType:               FactTypeVerifiedRemediationSuccess,
		StructuredValue:        json.RawMessage(`{"status":"ok"}`),
		SourceRefs:             []string{"ref-1"},
		SourceHashes:           []string{"0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef"},
		SourceVerificationRefs: []string{"verif-001"},
		ConfidenceSource:       ConfidenceSourceVerifiedWorkflow,
	}

	err := gate.Evaluate(context.Background(), makeTestScope("tenant-test"), m)
	if err == nil || !strings.Contains(err.Error(), "only M1_OPERATIONAL_FACT") {
		t.Fatalf("expected rejection of non-M1 memory type, got: %v", err)
	}
}

func TestEligibilityGate_RejectsSecrets(t *testing.T) {
	db := setupMemoryTestDB(t)
	gate := NewEligibilityGate(db)

	m := &OperationalMemoryRecord{
		MemoryID:               "mem-001",
		TenantID:               "tenant-test",
		MemoryType:             MemoryTypeOperationalFact,
		SubjectType:            SubjectTypePartner,
		SubjectRef:             "PARTNER-01",
		FactType:               FactTypeVerifiedRemediationSuccess,
		StructuredValue:        json.RawMessage(`{"api_key": "sk_live_1234567890abcdef1234"}`),
		SourceRefs:             []string{"ref-1"},
		SourceHashes:           []string{"0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef"},
		SourceVerificationRefs: []string{"verif-001"},
		ConfidenceSource:       ConfidenceSourceVerifiedWorkflow,
	}

	err := gate.Evaluate(context.Background(), makeTestScope("tenant-test"), m)
	if err != ErrSecretDetected {
		t.Fatalf("expected ErrSecretDetected, got: %v", err)
	}
}

func TestEligibilityGate_RejectsRawNACHARecord(t *testing.T) {
	db := setupMemoryTestDB(t)
	gate := NewEligibilityGate(db)

	nacha94 := "6221210003581234567890          00000500000918273645JOHN DOE              0121000350000001"
	m := &OperationalMemoryRecord{
		MemoryID:               "mem-001",
		TenantID:               "tenant-test",
		MemoryType:             MemoryTypeOperationalFact,
		SubjectType:            SubjectTypePartner,
		SubjectRef:             "PARTNER-01",
		FactType:               FactTypeVerifiedRemediationSuccess,
		StructuredValue:        json.RawMessage(`{"raw_line": "` + nacha94 + `"}`),
		SourceRefs:             []string{"ref-1"},
		SourceHashes:           []string{"0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef"},
		SourceVerificationRefs: []string{"verif-001"},
		ConfidenceSource:       ConfidenceSourceVerifiedWorkflow,
	}

	err := gate.Evaluate(context.Background(), makeTestScope("tenant-test"), m)
	if err == nil || !errors.Is(err, ErrPIIDetected) {
		t.Fatalf("expected ErrPIIDetected rejection, got: %v", err)
	}
}

func TestEligibilityGate_RejectsUnverifiedConfidence(t *testing.T) {
	db := setupMemoryTestDB(t)
	gate := NewEligibilityGate(db)

	m := &OperationalMemoryRecord{
		MemoryID:               "mem-001",
		TenantID:               "tenant-test",
		MemoryType:             MemoryTypeOperationalFact,
		SubjectType:            SubjectTypePartner,
		SubjectRef:             "PARTNER-01",
		FactType:               FactTypeVerifiedRemediationSuccess,
		StructuredValue:        json.RawMessage(`{"status":"ok"}`),
		SourceRefs:             []string{"ref-1"},
		SourceHashes:           []string{"0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef"},
		SourceVerificationRefs: []string{"verif-001"},
		ConfidenceSource:       ConfidenceSourceManagedMemorySuggest,
	}

	err := gate.Evaluate(context.Background(), makeTestScope("tenant-test"), m)
	if err == nil || !strings.Contains(err.Error(), "cannot directly write M1 facts") {
		t.Fatalf("expected rejection of MANAGED_MEMORY_SUGGESTION confidence source, got: %v", err)
	}
}

func TestStore_PersistAndGetMemory(t *testing.T) {
	db := setupMemoryTestDB(t)
	store := NewStore(db)
	scope := makeTestScope("tenant-test")

	rec := &OperationalMemoryRecord{
		MemoryID:               "mem-001",
		TenantID:               "tenant-test",
		MemoryType:             MemoryTypeOperationalFact,
		SubjectType:            SubjectTypePartner,
		SubjectRef:             "PARTNER-01",
		FactType:               FactTypeVerifiedRemediationSuccess,
		StructuredValue:        json.RawMessage(`{"status":"remediated","attempt":1}`),
		SourceRefs:             []string{"ref-1"},
		SourceHashes:           []string{"0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef"},
		SourceVerificationRefs: []string{"verif-001"},
		ConfidenceSource:       ConfidenceSourceVerifiedWorkflow,
		Classification:         ClassificationInternal,
		Status:                 StatusActive,
		ValidFrom:              time.Now().UTC(),
		CreatedAt:              time.Now().UTC(),
		CreatedBy:              "test-runner",
	}

	if err := store.PersistOperationalFact(context.Background(), scope, rec); err != nil {
		t.Fatalf("persist operational fact: %v", err)
	}

	loaded, err := store.GetMemory(context.Background(), scope, "mem-001")
	if err != nil {
		t.Fatalf("get memory: %v", err)
	}

	if loaded.MemoryID != "mem-001" || loaded.TenantID != "tenant-test" {
		t.Fatalf("loaded memory mismatch: %+v", loaded)
	}
	if len(loaded.SourceRefs) != 1 || loaded.SourceRefs[0] != "ref-1" {
		t.Fatalf("loaded source refs mismatch: %v", loaded.SourceRefs)
	}
}

func TestStore_ListMemoriesForSubject(t *testing.T) {
	db := setupMemoryTestDB(t)
	store := NewStore(db)
	scope := makeTestScope("tenant-test")

	for i := 1; i <= 3; i++ {
		rec := &OperationalMemoryRecord{
			MemoryID:               fmt.Sprintf("mem-00%d", i),
			TenantID:               "tenant-test",
			MemoryType:             MemoryTypeOperationalFact,
			SubjectType:            SubjectTypePartner,
			SubjectRef:             "PARTNER-RECURRENT",
			FactType:               FactTypeVerifiedRemediationSuccess,
			StructuredValue:        json.RawMessage(fmt.Sprintf(`{"attempt":%d}`, i)),
			SourceRefs:             []string{fmt.Sprintf("ref-%d", i)},
			SourceHashes:           []string{"0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef"},
			SourceVerificationRefs: []string{"verif-001"},
			ConfidenceSource:       ConfidenceSourceVerifiedWorkflow,
			Classification:         ClassificationInternal,
			Status:                 StatusActive,
			ValidFrom:              time.Now().UTC(),
			CreatedAt:              time.Now().UTC().Add(time.Duration(i) * time.Minute),
			CreatedBy:              "test-runner",
		}
		if err := store.PersistOperationalFact(context.Background(), scope, rec); err != nil {
			t.Fatalf("persist fact %d: %v", i, err)
		}
	}

	list, err := store.ListMemoriesForSubject(context.Background(), scope, SubjectTypePartner, "PARTNER-RECURRENT")
	if err != nil {
		t.Fatalf("list memories: %v", err)
	}
	if len(list) != 3 {
		t.Fatalf("expected 3 memories, got %d", len(list))
	}
}

func TestStore_SupersedeMemory(t *testing.T) {
	db := setupMemoryTestDB(t)
	store := NewStore(db)
	scope := makeTestScope("tenant-test")

	rec1 := &OperationalMemoryRecord{
		MemoryID:               "mem-v1",
		TenantID:               "tenant-test",
		MemoryType:             MemoryTypeOperationalFact,
		SubjectType:            SubjectTypePartner,
		SubjectRef:             "PARTNER-UPDATE",
		FactType:               FactTypePartnerFormatTolerance,
		StructuredValue:        json.RawMessage(`{"cutoff":"16:00"}`),
		SourceRefs:             []string{"ref-1"},
		SourceHashes:           []string{"0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef"},
		SourceVerificationRefs: []string{"verif-001"},
		ConfidenceSource:       ConfidenceSourceHumanConfirmed,
		CreatedBy:              "admin",
	}
	if err := store.PersistOperationalFact(context.Background(), scope, rec1); err != nil {
		t.Fatalf("persist v1: %v", err)
	}

	rec2 := &OperationalMemoryRecord{
		MemoryID:               "mem-v2",
		TenantID:               "tenant-test",
		MemoryType:             MemoryTypeOperationalFact,
		SubjectType:            SubjectTypePartner,
		SubjectRef:             "PARTNER-UPDATE",
		FactType:               FactTypePartnerFormatTolerance,
		StructuredValue:        json.RawMessage(`{"cutoff":"17:00"}`),
		SourceRefs:             []string{"ref-2"},
		SourceHashes:           []string{"0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef"},
		SourceVerificationRefs: []string{"verif-001"},
		ConfidenceSource:       ConfidenceSourceHumanConfirmed,
		CreatedBy:              "admin",
	}

	if err := store.SupersedeMemory(context.Background(), scope, "mem-v1", rec2, "admin", "Updated partner cutoff to 17:00"); err != nil {
		t.Fatalf("supersede memory: %v", err)
	}

	oldRec, err := store.GetMemory(context.Background(), scope, "mem-v1")
	if err != nil {
		t.Fatalf("get old record: %v", err)
	}
	if oldRec.Status != StatusSuperseded {
		t.Fatalf("expected old record to be SUPERSEDED, got %s", oldRec.Status)
	}
	if oldRec.SupersededBy == nil || *oldRec.SupersededBy != "mem-v2" {
		t.Fatalf("expected superseded_by = mem-v2, got %v", oldRec.SupersededBy)
	}
}

func TestStore_InvalidateMemory(t *testing.T) {
	db := setupMemoryTestDB(t)
	store := NewStore(db)
	scope := makeTestScope("tenant-test")

	rec := &OperationalMemoryRecord{
		MemoryID:               "mem-invalid-target",
		TenantID:               "tenant-test",
		MemoryType:             MemoryTypeOperationalFact,
		SubjectType:            SubjectTypePartner,
		SubjectRef:             "PARTNER-INV",
		FactType:               FactTypeVerifiedFailurePattern,
		StructuredValue:        json.RawMessage(`{"pattern":"deprecated"}`),
		SourceRefs:             []string{"ref-1"},
		SourceHashes:           []string{"0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef"},
		SourceVerificationRefs: []string{"verif-001"},
		ConfidenceSource:       ConfidenceSourceVerifiedWorkflow,
		CreatedBy:              "system",
	}
	if err := store.PersistOperationalFact(context.Background(), scope, rec); err != nil {
		t.Fatalf("persist fact: %v", err)
	}

	if err := store.InvalidateMemory(context.Background(), scope, "mem-invalid-target", "admin", "Observation was erroneous"); err != nil {
		t.Fatalf("invalidate memory: %v", err)
	}

	loaded, err := store.GetMemory(context.Background(), scope, "mem-invalid-target")
	if err != nil {
		t.Fatalf("get invalidated memory: %v", err)
	}
	if loaded.Status != StatusInvalidated {
		t.Fatalf("expected status INVALIDATED, got %s", loaded.Status)
	}
}

func TestIngestionBridgeDeliverer_DeliversEligibleEvent(t *testing.T) {
	db := setupMemoryTestDB(t)
	store := NewStore(db)
	deliverer := NewIngestionBridgeDeliverer(store)

	payloadObj := map[string]interface{}{
		"tenant_id":                "tenant-test",
		"workflow_id":              "wf-001",
		"incident_id":              101,
		"subject_type":             string(SubjectTypePartner),
		"subject_ref":              "PARTNER-OUTBOX",
		"fact_type":                string(FactTypeVerifiedRemediationSuccess),
		"confidence_source":        string(ConfidenceSourceVerifiedWorkflow),
		"source_refs":              []string{"ref-outbox-1"},
		"source_hashes":            []string{"0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef"},
		"source_verification_refs": []string{"verif-001"},
		"structured_value":         map[string]interface{}{"status": "remediated"},
		"created_by":               "worker",
	}
	payloadBytes, _ := json.Marshal(payloadObj)

	ev := jobs.PendingEvent{
		OutboxEvent: jobs.OutboxEvent{
			ID:        1,
			EventType: "MEMORY_EVENT_ELIGIBLE",
			TenantID:  "tenant-test",
			DedupeKey: "outbox1234567890",
		},
		Payload: jobs.RawPayload(payloadBytes),
	}

	if err := deliverer.Deliver(context.Background(), ev); err != nil {
		t.Fatalf("deliver memory event: %v", err)
	}

	scope := makeTestScope("tenant-test")
	list, err := store.ListMemoriesForSubject(context.Background(), scope, SubjectTypePartner, "PARTNER-OUTBOX")
	if err != nil {
		t.Fatalf("list memories after delivery: %v", err)
	}
	if len(list) != 1 {
		t.Fatalf("expected 1 delivered memory, got %d", len(list))
	}
}

func TestRevalidator_AuthoritativePass(t *testing.T) {
	db := setupMemoryTestDB(t)
	store := NewStore(db)
	revalidator := NewRevalidator(db)
	scope := makeTestScope("tenant-test")

	rec := &OperationalMemoryRecord{
		MemoryID:               "mem-reval-01",
		TenantID:               "tenant-test",
		MemoryType:             MemoryTypeOperationalFact,
		SubjectType:            SubjectTypePartner,
		SubjectRef:             "PARTNER-REVAL",
		FactType:               FactTypeVerifiedRemediationSuccess,
		StructuredValue:        json.RawMessage(`{"status":"verified"}`),
		SourceRefs:             []string{"ref-1"},
		SourceHashes:           []string{"0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef"},
		SourceVerificationRefs: []string{"verif-001"},
		ConfidenceSource:       ConfidenceSourceVerifiedWorkflow,
		CreatedBy:              "system",
	}

	if err := store.PersistOperationalFact(context.Background(), scope, rec); err != nil {
		t.Fatalf("persist operational fact: %v", err)
	}

	report, err := revalidator.Revalidate(context.Background(), scope, rec)
	if err != nil {
		t.Fatalf("revalidate: %v", err)
	}
	if report.Status != RevalidationAuthoritativeVerified {
		t.Fatalf("expected AUTHORITATIVE_VERIFIED, got %s: %s", report.Status, report.Detail)
	}
}

func TestRevalidator_DetectsHashTamper(t *testing.T) {
	db := setupMemoryTestDB(t)
	revalidator := NewRevalidator(db)
	scope := makeTestScope("tenant-test")

	rec := &OperationalMemoryRecord{
		MemoryID:               "mem-tampered",
		TenantID:               "tenant-test",
		MemoryType:             MemoryTypeOperationalFact,
		SubjectType:            SubjectTypePartner,
		SubjectRef:             "PARTNER-TAMPER",
		FactType:               FactTypeVerifiedRemediationSuccess,
		StructuredValue:        json.RawMessage(`{"status":"tampered"}`),
		SourceRefs:             []string{"ref-1"},
		SourceHashes:           []string{"0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef"},
		SourceVerificationRefs: []string{"verif-001"},
		ConfidenceSource:       ConfidenceSourceVerifiedWorkflow,
		CreatedBy:              "system",
		MemoryHash:             "fake_tampered_hash_value_1234567890",
	}

	report, err := revalidator.Revalidate(context.Background(), scope, rec)
	if err != nil {
		t.Fatalf("revalidate: %v", err)
	}
	if report.Status != RevalidationTamperedRejected {
		t.Fatalf("expected TAMPERED_REJECTED, got %s", report.Status)
	}
}
