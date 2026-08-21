package verification

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"sync"
	"testing"
)

// Property 1: DeterministicDominance
// Repeated or concurrent verification runs on the same candidate state MUST produce identical outcomes, checks, and verification hashes.
func TestProperty_DeterministicDominance(t *testing.T) {
	db, store, engine := setupTestDB(t)
	defer db.Close()
	ctx := context.Background()
	tenantID := "TENANT-PROP-01"
	wfID := "wf-prop-dominance"

	_, _ = db.Exec(`INSERT INTO tenants (id, name) VALUES (?, 'Tenant Prop 1')`, tenantID)

	parentNACHA := generateTestNACHA(true)
	candNACHA := generateTestNACHA(false)

	parentID, parentSHA, candID, candSHA, derivID, _ := seedVerificationScenario(
		t, db, store, tenantID, wfID, 1, parentNACHA, candNACHA, "VALIDATION_PASSED",
	)

	svc := NewService(db, store, engine)
	req := &VerificationRequest{
		TenantID:                tenantID,
		WorkflowID:              wfID,
		CandidateArtifactID:     candID,
		ExpectedCandidateSHA256: candSHA,
		ExpectedParentSHA256:    parentSHA,
		DerivationID:            derivID,
		PolicyBundleHash:        engine.GetBundleHash(),
	}

	res1, err := svc.VerifyCandidate(ctx, makeTestScope(tenantID), req)
	if err != nil {
		t.Fatalf("first verification failed: %v", err)
	}

	// Run concurrent verifications
	const numGoroutines = 5
	var wg sync.WaitGroup
	results := make([]*VerificationResult, numGoroutines)
	errorsList := make([]error, numGoroutines)

	for i := 0; i < numGoroutines; i++ {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			r, err := svc.VerifyCandidate(ctx, makeTestScope(tenantID), req)
			results[idx] = r
			errorsList[idx] = err
		}(i)
	}
	wg.Wait()

	for i, r := range results {
		if errorsList[i] != nil {
			t.Fatalf("concurrent run %d failed: %v", i, errorsList[i])
		}
		if r.DeterministicOutcome != res1.DeterministicOutcome {
			t.Errorf("outcome mismatch in run %d: got %s, want %s", i, r.DeterministicOutcome, res1.DeterministicOutcome)
		}
		if r.CandidateSHA256 != res1.CandidateSHA256 || r.ParentSHA256 != res1.ParentSHA256 {
			t.Errorf("sha mismatch in run %d", i)
		}
		if len(r.Checks) != len(res1.Checks) {
			t.Errorf("checks length mismatch in run %d", i)
		}
		for j, c := range r.Checks {
			if c.Type != res1.Checks[j].Type || c.Passed != res1.Checks[j].Passed {
				t.Errorf("check %d mismatch in run %d", j, i)
			}
		}
	}
	_ = parentID
}

// Property 2: CandidateIntegrity
// If candidate bytes in object store do not match the derivation candidate SHA-256, verification MUST detect corruption.
func TestProperty_CandidateIntegrity(t *testing.T) {
	db, store, engine := setupTestDB(t)
	defer db.Close()
	ctx := context.Background()
	tenantID := "TENANT-PROP-02"
	wfID := "wf-prop-cand-integrity"

	_, _ = db.Exec(`INSERT INTO tenants (id, name) VALUES (?, 'Tenant Prop 2')`, tenantID)

	parentNACHA := generateTestNACHA(true)
	candNACHA := generateTestNACHA(false)

	_, parentSHA, candID, candSHA, derivID, _ := seedVerificationScenario(
		t, db, store, tenantID, wfID, 1, parentNACHA, candNACHA, "VALIDATION_PASSED",
	)

	// Corrupt candidate bytes in store
	candKey := fmt.Sprintf("cand-%s-%s", tenantID, candSHA[:8])
	corrupted := []byte(candNACHA)
	corrupted[0] = 'X'
	store.Tamper(candKey, corrupted)

	svc := NewService(db, store, engine)
	req := &VerificationRequest{
		TenantID:                tenantID,
		WorkflowID:              wfID,
		CandidateArtifactID:     candID,
		ExpectedCandidateSHA256: candSHA,
		ExpectedParentSHA256:    parentSHA,
		DerivationID:            derivID,
		PolicyBundleHash:        engine.GetBundleHash(),
	}

	res, err := svc.VerifyCandidate(ctx, makeTestScope(tenantID), req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if res.DeterministicOutcome != OutcomeCorruptionDetected {
		t.Fatalf("expected OutcomeCorruptionDetected on candidate mutation, got %s", res.DeterministicOutcome)
	}

	var candCheck *VerificationCheck
	for i := range res.Checks {
		if res.Checks[i].Type == CheckCandidateHashMatch {
			candCheck = &res.Checks[i]
			break
		}
	}
	if candCheck == nil || candCheck.Passed {
		t.Errorf("CheckCandidateHashMatch expected to fail")
	}
}

// Property 3: ParentIntegrity
// If parent bytes in object store do not match the derivation parent SHA-256, verification MUST detect corruption.
func TestProperty_ParentIntegrity(t *testing.T) {
	db, store, engine := setupTestDB(t)
	defer db.Close()
	ctx := context.Background()
	tenantID := "TENANT-PROP-03"
	wfID := "wf-prop-parent-integrity"

	_, _ = db.Exec(`INSERT INTO tenants (id, name) VALUES (?, 'Tenant Prop 3')`, tenantID)

	parentNACHA := generateTestNACHA(true)
	candNACHA := generateTestNACHA(false)

	_, parentSHA, candID, candSHA, derivID, _ := seedVerificationScenario(
		t, db, store, tenantID, wfID, 1, parentNACHA, candNACHA, "VALIDATION_PASSED",
	)

	// Corrupt parent bytes in store
	parentKey := fmt.Sprintf("parent-%s-%s", tenantID, parentSHA[:8])
	corrupted := []byte(parentNACHA)
	corrupted[len(corrupted)-2] = 'Z'
	store.Tamper(parentKey, corrupted)

	svc := NewService(db, store, engine)
	req := &VerificationRequest{
		TenantID:                tenantID,
		WorkflowID:              wfID,
		CandidateArtifactID:     candID,
		ExpectedCandidateSHA256: candSHA,
		ExpectedParentSHA256:    parentSHA,
		DerivationID:            derivID,
		PolicyBundleHash:        engine.GetBundleHash(),
	}

	res, err := svc.VerifyCandidate(ctx, makeTestScope(tenantID), req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if res.DeterministicOutcome != OutcomeCorruptionDetected {
		t.Fatalf("expected OutcomeCorruptionDetected on parent mutation, got %s", res.DeterministicOutcome)
	}

	var parentCheck *VerificationCheck
	for i := range res.Checks {
		if res.Checks[i].Type == CheckParentHashMatch {
			parentCheck = &res.Checks[i]
			break
		}
	}
	if parentCheck == nil || parentCheck.Passed {
		t.Errorf("CheckParentHashMatch expected to fail")
	}
}

// Property 4: DerivationIntegrity
// If derivation hash or derivation fields are tampered with, verification MUST detect corruption or fail.
func TestProperty_DerivationIntegrity(t *testing.T) {
	db, store, engine := setupTestDB(t)
	defer db.Close()
	ctx := context.Background()
	tenantID := "TENANT-PROP-04"
	wfID := "wf-prop-deriv-integrity"

	_, _ = db.Exec(`INSERT INTO tenants (id, name) VALUES (?, 'Tenant Prop 4')`, tenantID)

	parentNACHA := generateTestNACHA(true)
	candNACHA := generateTestNACHA(false)

	_, parentSHA, candID, candSHA, derivID, _ := seedVerificationScenario(
		t, db, store, tenantID, wfID, 1, parentNACHA, candNACHA, "VALIDATION_PASSED",
	)

	// Tamper remediation_plan_hash inside artifact_derivations without updating derivation_hash
	_, err := db.Exec(`UPDATE artifact_derivations SET remediation_plan_hash = 'tampered-plan-hash' WHERE id = ?`, derivID)
	if err != nil {
		t.Fatalf("tamper plan hash: %v", err)
	}

	svc := NewService(db, store, engine)
	req := &VerificationRequest{
		TenantID:                tenantID,
		WorkflowID:              wfID,
		CandidateArtifactID:     candID,
		ExpectedCandidateSHA256: candSHA,
		ExpectedParentSHA256:    parentSHA,
		DerivationID:            derivID,
		PolicyBundleHash:        engine.GetBundleHash(),
	}

	res, err := svc.VerifyCandidate(ctx, makeTestScope(tenantID), req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if res.DeterministicOutcome != OutcomeCorruptionDetected && res.DeterministicOutcome != OutcomeFail {
		t.Fatalf("expected corruption/fail outcome on derivation tampering, got %s", res.DeterministicOutcome)
	}

	var derivCheck *VerificationCheck
	for i := range res.Checks {
		if res.Checks[i].Type == CheckDerivationHashMatch {
			derivCheck = &res.Checks[i]
			break
		}
	}
	if derivCheck == nil || derivCheck.Passed {
		t.Errorf("CheckDerivationHashMatch expected to fail")
	}
}

// Property 5: PolicyFreshness
// If policy bundle hash is changed/stale, verification MUST return OutcomeStale.
func TestProperty_PolicyFreshness(t *testing.T) {
	db, store, engine := setupTestDB(t)
	defer db.Close()
	ctx := context.Background()
	tenantID := "TENANT-PROP-05"
	wfID := "wf-prop-policy-freshness"

	_, _ = db.Exec(`INSERT INTO tenants (id, name) VALUES (?, 'Tenant Prop 5')`, tenantID)

	parentNACHA := generateTestNACHA(true)
	candNACHA := generateTestNACHA(false)

	_, parentSHA, candID, candSHA, derivID, _ := seedVerificationScenario(
		t, db, store, tenantID, wfID, 1, parentNACHA, candNACHA, "VALIDATION_PASSED",
	)

	svc := NewService(db, store, engine)
	req := &VerificationRequest{
		TenantID:                tenantID,
		WorkflowID:              wfID,
		CandidateArtifactID:     candID,
		ExpectedCandidateSHA256: candSHA,
		ExpectedParentSHA256:    parentSHA,
		DerivationID:            derivID,
		PolicyBundleHash:        "non-matching-stale-policy-bundle-digest",
	}

	res, err := svc.VerifyCandidate(ctx, makeTestScope(tenantID), req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if res.DeterministicOutcome != OutcomeStale {
		t.Fatalf("expected OutcomeStale, got %s", res.DeterministicOutcome)
	}

	var polCheck *VerificationCheck
	for i := range res.Checks {
		if res.Checks[i].Type == CheckPolicyContextFresh {
			polCheck = &res.Checks[i]
			break
		}
	}
	if polCheck == nil || polCheck.Passed {
		t.Errorf("CheckPolicyContextFresh expected to fail")
	}
}

// Property 6: VerificationImmutability
// Verification MUST NEVER mutate parent bytes, candidate bytes, or existing derivation records.
func TestProperty_VerificationImmutability(t *testing.T) {
	db, store, engine := setupTestDB(t)
	defer db.Close()
	ctx := context.Background()
	tenantID := "TENANT-PROP-06"
	wfID := "wf-prop-immutability"

	_, _ = db.Exec(`INSERT INTO tenants (id, name) VALUES (?, 'Tenant Prop 6')`, tenantID)

	parentNACHA := generateTestNACHA(true)
	candNACHA := generateTestNACHA(false)

	parentID, parentSHA, candID, candSHA, derivID, derivHash := seedVerificationScenario(
		t, db, store, tenantID, wfID, 1, parentNACHA, candNACHA, "VALIDATION_PASSED",
	)

	parentKey := fmt.Sprintf("parent-%s-%s", tenantID, parentSHA[:8])
	candKey := fmt.Sprintf("cand-%s-%s", tenantID, candSHA[:8])

	svc := NewService(db, store, engine)
	req := &VerificationRequest{
		TenantID:                tenantID,
		WorkflowID:              wfID,
		CandidateArtifactID:     candID,
		ExpectedCandidateSHA256: candSHA,
		ExpectedParentSHA256:    parentSHA,
		DerivationID:            derivID,
		PolicyBundleHash:        engine.GetBundleHash(),
	}

	_, err := svc.VerifyCandidate(ctx, makeTestScope(tenantID), req)
	if err != nil {
		t.Fatalf("verification failed: %v", err)
	}

	// Verify parent bytes in store unchanged
	rcP, _ := store.Get(ctx, parentKey)
	defer rcP.Close()
	pBytesAfter, _ := io.ReadAll(rcP)
	hPAfter := sha256.Sum256(pBytesAfter)
	if hex.EncodeToString(hPAfter[:]) != parentSHA {
		t.Errorf("parent bytes were mutated during verification!")
	}

	// Verify candidate bytes in store unchanged
	rcC, _ := store.Get(ctx, candKey)
	defer rcC.Close()
	cBytesAfter, _ := io.ReadAll(rcC)
	hCAfter := sha256.Sum256(cBytesAfter)
	if hex.EncodeToString(hCAfter[:]) != candSHA {
		t.Errorf("candidate bytes were mutated during verification!")
	}

	// Verify derivation record unchanged in DB
	var dbDerivHash string
	_ = db.QueryRowContext(ctx, `SELECT derivation_hash FROM artifact_derivations WHERE id = ?`, derivID).Scan(&dbDerivHash)
	if dbDerivHash != derivHash {
		t.Errorf("derivation record was mutated during verification!")
	}
	_ = parentID
}

// Property 7: Idempotency
// Multiple executions of VerifyCandidate on the same inputs succeed idempotently without error.
func TestProperty_Idempotency(t *testing.T) {
	db, store, engine := setupTestDB(t)
	defer db.Close()
	ctx := context.Background()
	tenantID := "TENANT-PROP-07"
	wfID := "wf-prop-idempotency"

	_, _ = db.Exec(`INSERT INTO tenants (id, name) VALUES (?, 'Tenant Prop 7')`, tenantID)

	parentNACHA := generateTestNACHA(true)
	candNACHA := generateTestNACHA(false)

	_, parentSHA, candID, candSHA, derivID, _ := seedVerificationScenario(
		t, db, store, tenantID, wfID, 1, parentNACHA, candNACHA, "VALIDATION_PASSED",
	)

	svc := NewService(db, store, engine)
	req := &VerificationRequest{
		TenantID:                tenantID,
		WorkflowID:              wfID,
		CandidateArtifactID:     candID,
		ExpectedCandidateSHA256: candSHA,
		ExpectedParentSHA256:    parentSHA,
		DerivationID:            derivID,
		PolicyBundleHash:        engine.GetBundleHash(),
	}

	res1, err1 := svc.VerifyCandidate(ctx, makeTestScope(tenantID), req)
	if err1 != nil {
		t.Fatalf("first verification failed: %v", err1)
	}

	res2, err2 := svc.VerifyCandidate(ctx, makeTestScope(tenantID), req)
	if err2 != nil {
		t.Fatalf("second idempotent verification failed: %v", err2)
	}

	if res1.DeterministicOutcome != res2.DeterministicOutcome {
		t.Errorf("idempotency outcome mismatch: %s != %s", res1.DeterministicOutcome, res2.DeterministicOutcome)
	}
	if res1.CandidateSHA256 != res2.CandidateSHA256 || res1.ParentSHA256 != res2.ParentSHA256 {
		t.Errorf("idempotency sha mismatch")
	}

	// Verify that candidate_verifications table still has exactly 1 row for this workflow/candidate
	var rowCount int
	_ = db.QueryRowContext(ctx, `SELECT count(*) FROM candidate_verifications WHERE tenant_id = ? AND workflow_id = ?`, tenantID, wfID).Scan(&rowCount)
	if rowCount != 1 {
		t.Errorf("expected 1 record in candidate_verifications, got %d", rowCount)
	}
}

// Property 8: NoReleaseAuthority
// VerifyCandidate MUST NEVER directly transition file_instances status to RELEASED or APPROVED.
func TestProperty_NoReleaseAuthority(t *testing.T) {
	db, store, engine := setupTestDB(t)
	defer db.Close()
	ctx := context.Background()
	tenantID := "TENANT-PROP-08"
	wfID := "wf-prop-no-release"

	_, _ = db.Exec(`INSERT INTO tenants (id, name) VALUES (?, 'Tenant Prop 8')`, tenantID)

	parentNACHA := generateTestNACHA(true)
	candNACHA := generateTestNACHA(false)

	parentID, parentSHA, candID, candSHA, derivID, _ := seedVerificationScenario(
		t, db, store, tenantID, wfID, 1, parentNACHA, candNACHA, "VALIDATION_PASSED",
	)

	svc := NewService(db, store, engine)
	req := &VerificationRequest{
		TenantID:                tenantID,
		WorkflowID:              wfID,
		CandidateArtifactID:     candID,
		ExpectedCandidateSHA256: candSHA,
		ExpectedParentSHA256:    parentSHA,
		DerivationID:            derivID,
		PolicyBundleHash:        engine.GetBundleHash(),
	}

	res, err := svc.VerifyCandidate(ctx, makeTestScope(tenantID), req)
	if err != nil {
		t.Fatalf("verification failed: %v", err)
	}
	if res.DeterministicOutcome != OutcomePass {
		t.Fatalf("expected OutcomePass, got %s", res.DeterministicOutcome)
	}

	// Assert candidate status in DB remains CANDIDATE (never mutated to RELEASED or APPROVED)
	var candStatus string
	_ = db.QueryRowContext(ctx, `SELECT status FROM file_instances WHERE id = ?`, candID).Scan(&candStatus)
	if candStatus == "RELEASED" || candStatus == "APPROVED" {
		t.Fatalf("CRITICAL SAFETY VIOLATION: candidate file_instance was mutated to %s by verification!", candStatus)
	}
	if candStatus != "CANDIDATE" {
		t.Errorf("candidate status unexpectedly modified to %s", candStatus)
	}

	// Assert parent status in DB remains QUARANTINED
	var parentStatus string
	_ = db.QueryRowContext(ctx, `SELECT status FROM file_instances WHERE id = ?`, parentID).Scan(&parentStatus)
	if parentStatus == "RELEASED" || parentStatus == "APPROVED" {
		t.Fatalf("CRITICAL SAFETY VIOLATION: parent file_instance was mutated to %s by verification!", parentStatus)
	}
	if parentStatus != "QUARANTINED" {
		t.Errorf("parent status unexpectedly modified to %s", parentStatus)
	}
}
