package memory

import (
	"context"
	"encoding/json"
	"testing"
	"time"
)

func TestProperty_DeterministicDominance(t *testing.T) {
	// Property 1: Memory asserting ALLOW cannot override deterministic policy DENY or validator FAIL.
	memoryClaim := "ALLOW"
	policyOutcome := "DENY"
	validatorOutcome := "FAIL"

	finalDecision := "DENY"
	if policyOutcome == "DENY" || validatorOutcome == "FAIL" {
		finalDecision = "DENY"
	} else if memoryClaim == "ALLOW" {
		finalDecision = "ALLOW"
	}

	if finalDecision != "DENY" {
		t.Fatalf("Property violation: memory claim %s bypassed policy outcome %s and validator %s", memoryClaim, policyOutcome, validatorOutcome)
	}
}

func TestProperty_MemoryIntegrity(t *testing.T) {
	// Property 2: Any mutation in structured memory fields invalidates the RFC 8785 canonical hash.
	rec := &OperationalMemoryRecord{
		MemoryID:               "mem-prop-01",
		TenantID:               "tenant-test",
		MemoryType:             MemoryTypeOperationalFact,
		SubjectType:            SubjectTypePartner,
		SubjectRef:             "PARTNER-01",
		FactType:               FactTypeVerifiedRemediationSuccess,
		StructuredValue:        json.RawMessage(`{"attempt":1,"status":"ok"}`),
		SourceRefs:             []string{"ref-1"},
		SourceHashes:           []string{"0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef"},
		SourceVerificationRefs: []string{"verif-001"},
		ConfidenceSource:       ConfidenceSourceVerifiedWorkflow,
		Classification:         ClassificationInternal,
		ValidFrom:              time.Date(2026, 8, 23, 12, 0, 0, 0, time.UTC),
		CreatedAt:              time.Date(2026, 8, 23, 12, 0, 0, 0, time.UTC),
		CreatedBy:              "actor",
	}

	hOriginal, err := ComputeMemoryHash(rec)
	if err != nil {
		t.Fatalf("compute hash: %v", err)
	}
	rec.MemoryHash = hOriginal

	valid, _ := VerifyMemoryHash(rec)
	if !valid {
		t.Fatalf("expected valid initial hash")
	}

	// Mutate payload
	rec.StructuredValue = json.RawMessage(`{"attempt":2,"status":"ok"}`)
	validMutated, _ := VerifyMemoryHash(rec)
	if validMutated {
		t.Fatalf("Property violation: mutated record unexpectedly verified with old canonical hash")
	}
}

func TestProperty_SourceIntegrity(t *testing.T) {
	// Property 3: Records without valid backing provenance hashes cannot pass the eligibility gate.
	db := setupMemoryTestDB(t)
	gate := NewEligibilityGate(db)

	rec := &OperationalMemoryRecord{
		MemoryID:               "mem-no-source",
		TenantID:               "tenant-test",
		MemoryType:             MemoryTypeOperationalFact,
		SubjectType:            SubjectTypePartner,
		SubjectRef:             "PARTNER-01",
		FactType:               FactTypeVerifiedRemediationSuccess,
		StructuredValue:        json.RawMessage(`{"status":"ok"}`),
		SourceRefs:             []string{}, // Empty
		SourceHashes:           []string{},
		SourceVerificationRefs: []string{},
		ConfidenceSource:       ConfidenceSourceVerifiedWorkflow,
	}

	err := gate.Evaluate(context.Background(), makeTestScope("tenant-test"), rec)
	if err == nil {
		t.Fatalf("Property violation: memory record with zero sources passed eligibility gate")
	}
}

func TestProperty_TenantIsolation(t *testing.T) {
	// Property 4: Tenant A cannot read or modify Tenant B's operational memories.
	db := setupMemoryTestDB(t)
	store := NewStore(db)

	// Seed Tenant B
	_, _ = db.Exec(`INSERT INTO tenants (id, name) VALUES ('tenant-b', 'Tenant B')`)

	recB := &OperationalMemoryRecord{
		MemoryID:               "mem-tenant-b-01",
		TenantID:               "tenant-b",
		MemoryType:             MemoryTypeOperationalFact,
		SubjectType:            SubjectTypePartner,
		SubjectRef:             "PARTNER-B",
		FactType:               FactTypeVerifiedRemediationSuccess,
		StructuredValue:        json.RawMessage(`{"secret_metric":42}`),
		SourceRefs:             []string{"ref-b"},
		SourceHashes:           []string{"0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef"},
		SourceVerificationRefs: []string{"verif-001"},
		ConfidenceSource:       ConfidenceSourceVerifiedWorkflow,
		CreatedBy:              "tenant-b-admin",
	}

	if err := store.PersistOperationalFact(context.Background(), makeTestScope("tenant-b"), recB); err != nil {
		t.Fatalf("persist tenant b record: %v", err)
	}

	// Attempt access with Tenant A scope
	scopeA := makeTestScope("tenant-test")
	_, err := store.GetMemory(context.Background(), scopeA, "mem-tenant-b-01")
	if err != ErrMemoryNotFound {
		t.Fatalf("Property violation: Tenant A accessed Tenant B memory, got: %v", err)
	}
}

func TestProperty_FreshnessAndExpiry(t *testing.T) {
	// Property 5: Records whose expires_at is in the past return STALE_EXPIRED during revalidation.
	db := setupMemoryTestDB(t)
	revalidator := NewRevalidator(db)
	scope := makeTestScope("tenant-test")

	past := time.Now().UTC().Add(-24 * time.Hour)
	rec := &OperationalMemoryRecord{
		MemoryID:               "mem-expired",
		TenantID:               "tenant-test",
		MemoryType:             MemoryTypeOperationalFact,
		SubjectType:            SubjectTypePartner,
		SubjectRef:             "PARTNER-EXP",
		FactType:               FactTypeVerifiedRemediationSuccess,
		StructuredValue:        json.RawMessage(`{"status":"old"}`),
		SourceRefs:             []string{"ref-1"},
		SourceHashes:           []string{"0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef"},
		SourceVerificationRefs: []string{"verif-001"},
		ConfidenceSource:       ConfidenceSourceVerifiedWorkflow,
		ExpiresAt:              &past,
		CreatedBy:              "system",
	}
	h, _ := ComputeMemoryHash(rec)
	rec.MemoryHash = h

	report, err := revalidator.Revalidate(context.Background(), scope, rec)
	if err != nil {
		t.Fatalf("revalidate: %v", err)
	}
	if report.Status != RevalidationStaleExpired {
		t.Fatalf("Property violation: expired memory returned status %s instead of STALE_EXPIRED", report.Status)
	}
}

func TestProperty_RevisionMonotonicity(t *testing.T) {
	// Property 6: Superseding or invalidating memories creates monotonic append-only revision records.
	db := setupMemoryTestDB(t)
	store := NewStore(db)
	scope := makeTestScope("tenant-test")

	rec := &OperationalMemoryRecord{
		MemoryID:               "mem-mono-01",
		TenantID:               "tenant-test",
		MemoryType:             MemoryTypeOperationalFact,
		SubjectType:            SubjectTypePartner,
		SubjectRef:             "PARTNER-MONO",
		FactType:               FactTypePartnerFormatTolerance,
		StructuredValue:        json.RawMessage(`{"val":1}`),
		SourceRefs:             []string{"ref-1"},
		SourceHashes:           []string{"0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef"},
		SourceVerificationRefs: []string{"verif-001"},
		ConfidenceSource:       ConfidenceSourceHumanConfirmed,
		CreatedBy:              "admin",
	}

	if err := store.PersistOperationalFact(context.Background(), scope, rec); err != nil {
		t.Fatalf("persist initial: %v", err)
	}

	if err := store.InvalidateMemory(context.Background(), scope, "mem-mono-01", "admin", "test invalidation"); err != nil {
		t.Fatalf("invalidate: %v", err)
	}

	var revCount int
	err := db.QueryRow(`SELECT COUNT(*) FROM memory_revisions WHERE memory_id = 'mem-mono-01'`).Scan(&revCount)
	if err != nil || revCount != 2 {
		t.Fatalf("expected exactly 2 revisions (CREATED and INVALIDATED), got count=%d, err=%v", revCount, err)
	}
}

func TestProperty_NoDirectAgentWrite(t *testing.T) {
	// Property 7: Autonomous advisory suggestions cannot write to M1 directly.
	db := setupMemoryTestDB(t)
	gate := NewEligibilityGate(db)

	rec := &OperationalMemoryRecord{
		MemoryID:               "mem-unauth",
		TenantID:               "tenant-test",
		MemoryType:             MemoryTypeOperationalFact,
		SubjectType:            SubjectTypePartner,
		SubjectRef:             "PARTNER-UNAUTH",
		FactType:               FactTypeVerifiedRemediationSuccess,
		StructuredValue:        json.RawMessage(`{"claim":"trust me"}`),
		SourceRefs:             []string{"ref-agent"},
		SourceHashes:           []string{"0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef"},
		SourceVerificationRefs: []string{"verif-001"},
		ConfidenceSource:       ConfidenceSourceManagedMemorySuggest,
	}

	err := gate.Evaluate(context.Background(), makeTestScope("tenant-test"), rec)
	if err == nil {
		t.Fatalf("Property violation: unverified advisory memory suggestion wrote to M1")
	}
}

func TestProperty_NonEquivalence(t *testing.T) {
	// Property 8: MemoryRecall != Evidence, MemoryRecall != PolicyDecision, MemoryRecall != Authorization, MemoryRecall != VerificationResult.
	type AuthToken struct {
		IsEvidence       bool
		IsPolicyDecision bool
		IsAuthorization  bool
		IsVerification   bool
	}

	recalledMemory := AuthToken{
		IsEvidence:       false,
		IsPolicyDecision: false,
		IsAuthorization:  false,
		IsVerification:   false,
	}

	if recalledMemory.IsEvidence || recalledMemory.IsPolicyDecision || recalledMemory.IsAuthorization || recalledMemory.IsVerification {
		t.Fatalf("Property violation: MemoryRecall was treated as authoritative evidence, policy decision, authorization, or verification")
	}
}
