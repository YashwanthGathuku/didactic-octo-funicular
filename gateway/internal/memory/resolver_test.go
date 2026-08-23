package memory

import (
	"context"
	"database/sql"
	"encoding/json"
	"testing"
	"time"

	_ "modernc.org/sqlite"
	"sentinel-gateway/internal/jobs"
	"sentinel-gateway/internal/repository"
)

func setupTestDB(t *testing.T) *sql.DB {
	t.Helper()
	db, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}

	// Schema setup for memory tests
	schema := `
	CREATE TABLE tenants (id TEXT PRIMARY KEY);
	INSERT INTO tenants (id) VALUES ('TENANT-TEST'), ('TENANT-FOREIGN');

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
		valid_from TIMESTAMP NOT NULL,
		expires_at TIMESTAMP,
		superseded_by TEXT,
		created_at TIMESTAMP NOT NULL,
		created_by TEXT NOT NULL,
		memory_hash TEXT NOT NULL
	);

	CREATE TABLE memory_sources (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		memory_id TEXT NOT NULL REFERENCES operational_memories(id),
		tenant_id TEXT NOT NULL REFERENCES tenants(id),
		source_ref TEXT NOT NULL,
		source_hash TEXT NOT NULL,
		source_verification_ref TEXT,
		created_at TIMESTAMP NOT NULL
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
		created_at TIMESTAMP NOT NULL
	);

	CREATE TABLE candidate_verifications (
		id TEXT PRIMARY KEY,
		tenant_id TEXT NOT NULL,
		workflow_id TEXT NOT NULL,
		deterministic_outcome TEXT NOT NULL
	);

	CREATE TABLE incidents (
		id INTEGER PRIMARY KEY,
		tenant_id TEXT NOT NULL
	);
	`

	if _, err := db.Exec(schema); err != nil {
		t.Fatalf("setup test schema: %v", err)
	}

	return db
}

func TestSourceResolver_CleanResolutionAndEvidenceMinting(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()

	ctx := context.Background()
	scope := repository.Scope{}
	store := NewStore(db)

	// Persist an M1 fact
	rec := &OperationalMemoryRecord{
		MemoryID:               "MEM-100",
		TenantID:               "TENANT-TEST",
		MemoryType:             MemoryTypeOperationalFact,
		SubjectType:            SubjectTypePartner,
		SubjectRef:             "PARTNER-01",
		FactType:               FactTypeVerifiedRemediationSuccess,
		StructuredValue:        json.RawMessage(`{"remediation_type":"RECOMPUTE_BATCH_CONTROL_TOTAL"}`),
		SourceRefs:             []string{"FINDING-001"},
		SourceHashes:           []string{"0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef"},
		SourceVerificationRefs: []string{"VER-001"},
		ConfidenceSource:       ConfidenceSourceDeterministicDerived,
		Classification:         ClassificationInternal,
		Status:                 StatusActive,
		ValidFrom:              time.Now().UTC(),
		CreatedAt:              time.Now().UTC(),
		CreatedBy:              "operator_alice",
	}
	h, _ := ComputeMemoryHash(rec)
	rec.MemoryHash = h

	if err := store.PersistOperationalFact(ctx, scope, rec); err != nil {
		t.Fatalf("persist operational fact: %v", err)
	}

	// Insert mock verification
	db.Exec(`INSERT INTO candidate_verifications (id, tenant_id, workflow_id, deterministic_outcome) VALUES ('VER-001', 'TENANT-TEST', 'WF-01', 'PASS')`)

	// Resolve sources
	req := &ResolveMemorySourcesRequest{
		TenantID:   "TENANT-TEST",
		MemoryRef:  "MEM-HIT-999",
		SourceRefs: []string{"MEM-100", "VER-001", "FINDING-001"},
	}

	resolved, err := store.ResolveMemorySources(ctx, scope, req)
	if err != nil {
		t.Fatalf("resolve memory sources: %v", err)
	}

	if len(resolved.ValidSourceRefs) != 3 {
		t.Errorf("expected 3 valid sources, got %d: %v", len(resolved.ValidSourceRefs), resolved.ValidSourceRefs)
	}
	if len(resolved.EvidenceRefsMinted) != 3 {
		t.Errorf("expected 3 evidence refs minted, got %d: %v", len(resolved.EvidenceRefsMinted), resolved.EvidenceRefsMinted)
	}
	if resolved.ResolutionHash == "" {
		t.Errorf("expected non-empty resolution hash")
	}
}

func TestSourceResolver_RejectsForeignTenantAndInvalidatedSources(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()

	ctx := context.Background()
	scope := repository.Scope{}
	store := NewStore(db)

	// Persist an active M1 fact for foreign tenant
	recForeign := &OperationalMemoryRecord{
		MemoryID:               "MEM-FOREIGN",
		TenantID:               "TENANT-FOREIGN",
		MemoryType:             MemoryTypeOperationalFact,
		SubjectType:            SubjectTypePartner,
		SubjectRef:             "PARTNER-02",
		FactType:               FactTypeVerifiedRemediationSuccess,
		StructuredValue:        json.RawMessage(`{"remediation_type":"RECOMPUTE_BATCH_CONTROL_TOTAL"}`),
		SourceRefs:             []string{"FINDING-002"},
		SourceHashes:           []string{"0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef"},
		SourceVerificationRefs: []string{"VER-FOREIGN"},
		ConfidenceSource:       ConfidenceSourceDeterministicDerived,
		Classification:         ClassificationInternal,
		Status:                 StatusActive,
		ValidFrom:              time.Now().UTC(),
		CreatedAt:              time.Now().UTC(),
		CreatedBy:              "operator_bob",
	}
	hForeign, _ := ComputeMemoryHash(recForeign)
	recForeign.MemoryHash = hForeign
	store.PersistOperationalFact(ctx, scope, recForeign)

	// Persist an invalidated M1 fact for TENANT-TEST
	recInvalid := &OperationalMemoryRecord{
		MemoryID:               "MEM-INVALID",
		TenantID:               "TENANT-TEST",
		MemoryType:             MemoryTypeOperationalFact,
		SubjectType:            SubjectTypePartner,
		SubjectRef:             "PARTNER-01",
		FactType:               FactTypeVerifiedFailurePattern,
		StructuredValue:        json.RawMessage(`{"pattern":"CORRUPTED_ENTRY"}`),
		SourceRefs:             []string{"FINDING-003"},
		SourceHashes:           []string{"0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef"},
		SourceVerificationRefs: []string{"VER-003"},
		ConfidenceSource:       ConfidenceSourceDeterministicDerived,
		Classification:         ClassificationInternal,
		Status:                 StatusActive,
		ValidFrom:              time.Now().UTC(),
		CreatedAt:              time.Now().UTC(),
		CreatedBy:              "operator_alice",
	}
	hInvalid, _ := ComputeMemoryHash(recInvalid)
	recInvalid.MemoryHash = hInvalid
	store.PersistOperationalFact(ctx, scope, recInvalid)
	store.InvalidateMemory(ctx, scope, "MEM-INVALID", "operator_alice", "Correction")

	// Attempt resolution from TENANT-TEST
	req := &ResolveMemorySourcesRequest{
		TenantID:   "TENANT-TEST",
		MemoryRef:  "MEM-HIT-888",
		SourceRefs: []string{"MEM-FOREIGN", "MEM-INVALID", "NONEXISTENT-999"},
	}

	resolved, err := store.ResolveMemorySources(ctx, scope, req)
	if err != nil {
		t.Fatalf("resolve memory sources: %v", err)
	}

	if len(resolved.ValidSourceRefs) != 0 {
		t.Errorf("expected 0 valid sources, got %d", len(resolved.ValidSourceRefs))
	}
	if len(resolved.InvalidSourceRefs) != 3 {
		t.Errorf("expected 3 invalid sources, got %d: %v", len(resolved.InvalidSourceRefs), resolved.InvalidSourceRefs)
	}
	if len(resolved.EvidenceRefsMinted) != 0 {
		t.Errorf("expected 0 evidence refs minted, got %d", len(resolved.EvidenceRefsMinted))
	}
}

func TestSourceResolver_FreshnessPolicyExpiration(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()

	ctx := context.Background()
	scope := repository.Scope{}
	store := NewStore(db)

	// Persist an SLA breach fact from 20 days ago (SLA DefaultTTL is 14 days)
	oldTime := time.Now().UTC().Add(-20 * 24 * time.Hour)
	recSLA := &OperationalMemoryRecord{
		MemoryID:               "MEM-SLA-OLD",
		TenantID:               "TENANT-TEST",
		MemoryType:             MemoryTypeOperationalFact,
		SubjectType:            SubjectTypePartner,
		SubjectRef:             "PARTNER-01",
		FactType:               FactTypeOperationalSLABreach,
		StructuredValue:        json.RawMessage(`{"sla_type":"WINDOW_BREACH","seconds_exceeded":3600}`),
		SourceRefs:             []string{"INC-001"},
		SourceHashes:           []string{"0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef"},
		SourceVerificationRefs: []string{"VER-001"},
		ConfidenceSource:       ConfidenceSourceDeterministicDerived,
		Classification:         ClassificationInternal,
		Status:                 StatusActive,
		ValidFrom:              oldTime,
		CreatedAt:              oldTime,
		CreatedBy:              "system",
	}
	h, _ := ComputeMemoryHash(recSLA)
	recSLA.MemoryHash = h
	store.PersistOperationalFact(ctx, scope, recSLA)

	// Resolve sources
	req := &ResolveMemorySourcesRequest{
		TenantID:   "TENANT-TEST",
		MemoryRef:  "MEM-HIT-777",
		SourceRefs: []string{"MEM-SLA-OLD"},
	}

	resolved, err := store.ResolveMemorySources(ctx, scope, req)
	if err != nil {
		t.Fatalf("resolve memory sources: %v", err)
	}

	if len(resolved.ValidSourceRefs) != 0 {
		t.Errorf("expected 0 valid sources for expired SLA fact, got %d", len(resolved.ValidSourceRefs))
	}
	if len(resolved.StaleSourceRefs) != 1 || resolved.StaleSourceRefs[0] != "MEM-SLA-OLD" {
		t.Errorf("expected MEM-SLA-OLD in stale sources, got: %v", resolved.StaleSourceRefs)
	}
}

func TestIngestionBridge_IdempotencyReplayAndConflict(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()

	ctx := context.Background()
	store := NewStore(db)
	deliverer := NewIngestionBridgeDeliverer(store)

	ev := jobs.PendingEvent{
		OutboxEvent: jobs.OutboxEvent{
			ID:        1,
			EventType: "MEMORY_EVENT_ELIGIBLE",
			TenantID:  "TENANT-TEST",
			DedupeKey: "dedup-12345678",
		},
		Payload: jobs.RawPayload(`{
			"tenant_id": "TENANT-TEST",
			"workflow_id": "WF-01",
			"incident_id": 1,
			"subject_type": "PARTNER",
			"subject_ref": "PARTNER-01",
			"fact_type": "VERIFIED_REMEDIATION_SUCCESS",
			"confidence_source": "DETERMINISTIC_DERIVED",
			"source_refs": ["FINDING-001"],
			"source_hashes": ["0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef"],
			"source_verification_refs": ["VER-001"],
			"structured_value": {"remediation_type": "RECOMPUTE_BATCH_CONTROL_TOTAL"},
			"created_by": "system"
		}`),
	}

	// 1. First delivery succeeds
	if err := deliverer.Deliver(ctx, ev); err != nil {
		t.Fatalf("initial delivery failed: %v", err)
	}

	// 2. Replay with identical payload succeeds idempotently
	if err := deliverer.Deliver(ctx, ev); err != nil {
		t.Fatalf("idempotent replay failed: %v", err)
	}

	// 3. Replay with DIFFERENT payload produces ErrIdempotencyConflict
	evConflicting := ev
	evConflicting.Payload = jobs.RawPayload(`{
		"tenant_id": "TENANT-TEST",
		"workflow_id": "WF-01",
		"incident_id": 1,
		"subject_type": "PARTNER",
		"subject_ref": "PARTNER-01",
		"fact_type": "VERIFIED_REMEDIATION_SUCCESS",
		"confidence_source": "DETERMINISTIC_DERIVED",
		"source_refs": ["FINDING-001"],
		"source_hashes": ["0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef"],
		"source_verification_refs": ["VER-001"],
		"structured_value": {"remediation_type": "DIFFERENT_REPAIR_OPERATION"},
		"created_by": "system"
	}`)

	errConflict := deliverer.Deliver(ctx, evConflicting)
	if errConflict == nil {
		t.Fatalf("expected conflict error on altered event payload, got nil")
	}
}

func TestExportToEnvelope_EnforcesClassificationAndFactPolicy(t *testing.T) {
	recRestricted := &OperationalMemoryRecord{
		MemoryID:        "MEM-RESTRICTED",
		TenantID:        "TENANT-TEST",
		MemoryType:      MemoryTypeOperationalFact,
		SubjectType:     SubjectTypePartner,
		SubjectRef:      "PARTNER-01",
		FactType:        FactTypeVerifiedRemediationSuccess,
		StructuredValue: json.RawMessage(`{"remediation_type":"TEST"}`),
		SourceRefs:      []string{"FINDING-001"},
		SourceHashes:    []string{"0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef"},
		Classification:  ClassificationRestricted,
	}

	policy := ManagedMemoryExportPolicy{
		TenantID:          "TENANT-TEST",
		AllowedFactTypes:  []FactType{FactTypeVerifiedRemediationSuccess},
		MaxClassification: ClassificationInternal,
	}

	_, err := ExportToEnvelope(recRestricted, policy)
	if err == nil {
		t.Fatalf("expected error exporting RESTRICTED memory, got nil")
	}

	recInternal := recRestricted
	recInternal.Classification = ClassificationInternal
	envelope, err := ExportToEnvelope(recInternal, policy)
	if err != nil {
		t.Fatalf("unexpected error exporting INTERNAL memory: %v", err)
	}

	if envelope["provenance_digest"] == "" {
		t.Errorf("expected non-empty provenance digest on export envelope")
	}
}
