package verification

import (
	"bytes"
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"fmt"
	"io"
	"strings"
	"sync"
	"testing"

	"sentinel-gateway/internal/auth"
	"sentinel-gateway/internal/objectstore"
	"sentinel-gateway/internal/policy"
	"sentinel-gateway/internal/repository"

	_ "modernc.org/sqlite"
)

type memoryTestStore struct {
	mu      sync.RWMutex
	objects map[string][]byte
}

func newMemoryTestStore() *memoryTestStore {
	return &memoryTestStore{
		objects: make(map[string][]byte),
	}
}

func (m *memoryTestStore) Put(ctx context.Context, key string, r io.Reader, limit int64) (objectstore.PutResult, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if _, exists := m.objects[key]; exists {
		return objectstore.PutResult{}, objectstore.ErrObjectExists
	}
	b, err := io.ReadAll(r)
	if err != nil {
		return objectstore.PutResult{}, err
	}
	m.objects[key] = b
	h := sha256.Sum256(b)
	return objectstore.PutResult{
		Key:       key,
		SizeBytes: int64(len(b)),
		SHA256:    hex.EncodeToString(h[:]),
	}, nil
}

func (m *memoryTestStore) Get(ctx context.Context, key string) (io.ReadCloser, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	b, exists := m.objects[key]
	if !exists {
		return nil, objectstore.ErrObjectNotFound
	}
	return io.NopCloser(bytes.NewReader(b)), nil
}

func (m *memoryTestStore) Stat(ctx context.Context, key string) (objectstore.ObjectInfo, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	b, exists := m.objects[key]
	if !exists {
		return objectstore.ObjectInfo{}, objectstore.ErrObjectNotFound
	}
	h := sha256.Sum256(b)
	return objectstore.ObjectInfo{
		Key:       key,
		SizeBytes: int64(len(b)),
		SHA256:    hex.EncodeToString(h[:]),
	}, nil
}

func (m *memoryTestStore) Tamper(key string, data []byte) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.objects[key] = data
}

func setupTestDB(t *testing.T) (*sql.DB, *memoryTestStore, *policy.PolicyEngine) {
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
		rulepack_hash TEXT NOT NULL,
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
		passed INTEGER NOT NULL CHECK(passed IN (0, 1)),
		message TEXT NOT NULL,
		expected_value TEXT NOT NULL,
		actual_value TEXT NOT NULL,
		created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP
	);
	`

	if _, err := db.Exec(schema); err != nil {
		t.Fatalf("exec schema: %v", err)
	}

	store := newMemoryTestStore()
	engine := policy.NewEngineWithDefaults()
	return db, store, engine
}

func makeTestScope(tenantID string) repository.Scope {
	p := &auth.Principal{
		Subject: "test-verifier",
		Memberships: []auth.Membership{
			{TenantID: tenantID, Roles: []auth.Role{auth.RoleOperator}},
		},
	}
	scope, _ := repository.NewScope(p, tenantID, auth.PermReadTenant)
	return scope
}

func padNum(n int64, width int) string {
	s := fmt.Sprintf("%d", n)
	if len(s) > width {
		return s[len(s)-width:]
	}
	return strings.Repeat("0", width-len(s)) + s
}

func padText(s string, width int) string {
	if len(s) > width {
		return s[:width]
	}
	return s + strings.Repeat(" ", width-len(s))
}

func generateTestNACHA(corruptBatchDebits bool) string {
	routingOrigin := "021000021"
	routingRDFI := "121000358"
	routingRDFI2 := "011000015"

	header := "101 " + routingRDFI + " " + routingOrigin + "2603011200A094101" +
		padText("DESTINATION BANK", 23) + padText("ORIGIN COMPANY", 23) + padText("", 8)

	bh := "5200" + padText("ORIGIN COMPANY", 16) + padText("", 20) +
		padText("1234567890", 10) + padText("PPD", 3) + padText("PAYROLL", 10) +
		"260301260302   1" + routingOrigin[:8] + padNum(1, 7)

	ed1 := "622" + routingRDFI + padText("1234567890", 17) +
		padNum(15000, 10) + padText("INDIVID001", 15) +
		padText("EMPLOYEE ONE", 22) + "  0" +
		routingOrigin[:8] + padNum(1, 7)

	ed2 := "627" + routingRDFI2 + padText("0987654321", 17) +
		padNum(15000, 10) + padText("INDIVID002", 15) +
		padText("EMPLOYEE TWO", 22) + "  0" +
		routingOrigin[:8] + padNum(2, 7)

	declaredDebits := int64(15000)
	if corruptBatchDebits {
		declaredDebits = int64(0)
	}

	bc := "8200" + padNum(2, 6) + padNum(13200036, 10) +
		padNum(declaredDebits, 12) + padNum(15000, 12) +
		padText("1234567890", 10) + padText("", 19) + padText("", 6) +
		routingOrigin[:8] + padNum(1, 7)

	declaredFileDebits := int64(15000)
	if corruptBatchDebits {
		declaredFileDebits = int64(0)
	}

	fc := "9" + padNum(1, 6) + padNum(1, 6) +
		padNum(2, 8) + padNum(13200036, 10) +
		padNum(declaredFileDebits, 12) + padNum(15000, 12) + padText("", 39)

	records := []string{header, bh, ed1, ed2, bc, fc}
	return strings.Join(records, "\n") + "\n"
}

func seedVerificationScenario(
	t *testing.T,
	db *sql.DB,
	store objectstore.ObjectStore,
	tenantID string,
	wfID string,
	attempt int,
	parentNACHA string,
	candNACHA string,
	validationOutcome string,
) (int64, string, int64, string, string, string) {
	t.Helper()
	ctx := context.Background()

	parentBytes := []byte(parentNACHA)
	hParent := sha256.Sum256(parentBytes)
	parentSHAHex := hex.EncodeToString(hParent[:])
	parentKey := fmt.Sprintf("parent-%s-%s", tenantID, parentSHAHex[:8])
	_, _ = store.Put(ctx, parentKey, strings.NewReader(parentNACHA), int64(len(parentBytes)))

	resP, err := db.Exec(`INSERT INTO file_instances (tenant_id, filename, storage_path, size_bytes, sha256_hash, status)
		VALUES (?, 'parent.ach', ?, ?, ?, 'QUARANTINED')`, tenantID, parentKey, len(parentBytes), parentSHAHex)
	if err != nil {
		t.Fatalf("insert parent file_instance: %v", err)
	}
	parentID, _ := resP.LastInsertId()

	candBytes := []byte(candNACHA)
	hCand := sha256.Sum256(candBytes)
	candSHAHex := hex.EncodeToString(hCand[:])
	candKey := fmt.Sprintf("cand-%s-%s", tenantID, candSHAHex[:8])
	_, _ = store.Put(ctx, candKey, strings.NewReader(candNACHA), int64(len(candBytes)))

	resC, err := db.Exec(`INSERT INTO file_instances (tenant_id, filename, storage_path, size_bytes, sha256_hash, status, derived_from)
		VALUES (?, 'candidate.ach', ?, ?, ?, 'CANDIDATE', ?)`, tenantID, candKey, len(candBytes), candSHAHex, fmt.Sprintf("%d", parentID))
	if err != nil {
		t.Fatalf("insert candidate file_instance: %v", err)
	}
	candID, _ := resC.LastInsertId()

	planID := fmt.Sprintf("plan-%s-%d", wfID, attempt)
	planHash := "plan-hash-12345"
	_, _ = db.Exec(`INSERT INTO remediation_plans (id, workflow_id, tenant_id, incident_id, artifact_id, expected_parent_sha256, attempt_number, plan_hash, operations_json, finding_refs_json, confidence, status)
		VALUES (?, ?, ?, 1, ?, ?, ?, ?, '[]', '[]', 'HIGH', 'APPLIED')`,
		planID, wfID, tenantID, parentID, parentSHAHex, attempt, planHash)

	derivID := fmt.Sprintf("deriv-%s-%d", wfID, attempt)
	derivHash := ComputeDerivationHash(wfID, attempt, parentSHAHex, candID, candSHAHex, planHash)

	_, err = db.Exec(`INSERT INTO artifact_derivations (
		id, tenant_id, workflow_id, remediation_plan_id, attempt_number,
		parent_artifact_id, parent_sha256, candidate_artifact_id, candidate_sha256,
		remediation_plan_hash, operation_types_json, agent_name, agent_version,
		policy_decision_id, policy_decision_hash, tool_manifest_hash,
		validator_version, validation_run_id, validation_outcome,
		findings_count, blocking_findings_count, derivation_hash
	) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, '[]', 'RemediationAgent', '1.0.0', 'dec-1', 'pol-hash-1', 'tool-hash-1', '1.0.0', ?, ?, 0, 0, ?)`,
		derivID, tenantID, wfID, planID, attempt,
		parentID, parentSHAHex, candID, candSHAHex,
		planHash, fmt.Sprintf("vrun-p07-%s-%d", wfID, attempt), validationOutcome, derivHash,
	)
	if err != nil {
		t.Fatalf("insert artifact_derivations: %v", err)
	}

	return parentID, parentSHAHex, candID, candSHAHex, derivID, derivHash
}

func TestVerifyCandidate_CleanPass(t *testing.T) {
	db, store, engine := setupTestDB(t)
	defer db.Close()
	ctx := context.Background()
	tenantID := "TENANT-CLEAN"
	wfID := "wf-clean-01"

	_, _ = db.Exec(`INSERT INTO tenants (id, name) VALUES (?, 'Tenant Clean')`, tenantID)

	parentNACHA := generateTestNACHA(true) // parent was invalid
	candNACHA := generateTestNACHA(false)  // candidate is repaired & valid

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
		CallerID:                "test-runner",
	}

	res, err := svc.VerifyCandidate(ctx, makeTestScope(tenantID), req)
	if err != nil {
		t.Fatalf("VerifyCandidate returned unexpected error: %v", err)
	}

	if res.DeterministicOutcome != OutcomePass {
		t.Fatalf("expected DeterministicOutcome %q, got %q", OutcomePass, res.DeterministicOutcome)
	}

	if res.ParentArtifactID != parentID || res.ParentSHA256 != parentSHA {
		t.Errorf("parent artifact metadata mismatch: got id=%d sha=%s", res.ParentArtifactID, res.ParentSHA256)
	}
	if res.CandidateArtifactID != candID || res.CandidateSHA256 != candSHA {
		t.Errorf("candidate artifact metadata mismatch: got id=%d sha=%s", res.CandidateArtifactID, res.CandidateSHA256)
	}
	if res.VerificationHash == "" {
		t.Errorf("expected non-empty VerificationHash")
	}

	// Verify all checks passed
	if len(res.Checks) == 0 {
		t.Fatalf("expected non-empty Checks slice")
	}
	for _, check := range res.Checks {
		if !check.Passed {
			t.Errorf("check %s unexpectedly failed: %s (expected=%s, actual=%s)", check.Type, check.Message, check.Expected, check.Actual)
		}
	}

	// Verify DB record exists
	var count int
	err = db.QueryRowContext(ctx, `SELECT count(*) FROM candidate_verifications WHERE tenant_id = ? AND workflow_id = ?`, tenantID, wfID).Scan(&count)
	if err != nil || count != 1 {
		t.Fatalf("candidate_verifications not persisted properly (count=%d, err=%v)", count, err)
	}
}

func TestVerifyCandidate_ParentSHACorruption(t *testing.T) {
	db, store, engine := setupTestDB(t)
	defer db.Close()
	ctx := context.Background()
	tenantID := "TENANT-CORRUPT-PARENT"
	wfID := "wf-corrupt-parent"

	_, _ = db.Exec(`INSERT INTO tenants (id, name) VALUES (?, 'Tenant Corrupt Parent')`, tenantID)

	parentNACHA := generateTestNACHA(true)
	candNACHA := generateTestNACHA(false)

	_, parentSHA, candID, candSHA, derivID, _ := seedVerificationScenario(
		t, db, store, tenantID, wfID, 1, parentNACHA, candNACHA, "VALIDATION_PASSED",
	)

	// Tamper parent bytes in object store directly
	parentKey := fmt.Sprintf("parent-%s-%s", tenantID, parentSHA[:8])
	store.Tamper(parentKey, []byte("TAMPERED_PARENT_BYTES_CORRUPTED"))

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
		t.Fatalf("unexpected execution error: %v", err)
	}

	if res.DeterministicOutcome != OutcomeCorruptionDetected {
		t.Fatalf("expected %q, got %q", OutcomeCorruptionDetected, res.DeterministicOutcome)
	}

	foundCheck := false
	for _, c := range res.Checks {
		if c.Type == CheckParentHashMatch {
			foundCheck = true
			if c.Passed {
				t.Errorf("expected CheckParentHashMatch to fail")
			}
		}
	}
	if !foundCheck {
		t.Errorf("CheckParentHashMatch was not recorded")
	}
}

func TestVerifyCandidate_CandidateSHACorruption(t *testing.T) {
	db, store, engine := setupTestDB(t)
	defer db.Close()
	ctx := context.Background()
	tenantID := "TENANT-CORRUPT-CAND"
	wfID := "wf-corrupt-cand"

	_, _ = db.Exec(`INSERT INTO tenants (id, name) VALUES (?, 'Tenant Corrupt Cand')`, tenantID)

	parentNACHA := generateTestNACHA(true)
	candNACHA := generateTestNACHA(false)

	_, parentSHA, candID, candSHA, derivID, _ := seedVerificationScenario(
		t, db, store, tenantID, wfID, 1, parentNACHA, candNACHA, "VALIDATION_PASSED",
	)

	// Tamper candidate bytes in object store directly
	candKey := fmt.Sprintf("cand-%s-%s", tenantID, candSHA[:8])
	store.Tamper(candKey, []byte("TAMPERED_CANDIDATE_BYTES_CORRUPTED"))

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
		t.Fatalf("unexpected execution error: %v", err)
	}

	if res.DeterministicOutcome != OutcomeCorruptionDetected {
		t.Fatalf("expected %q, got %q", OutcomeCorruptionDetected, res.DeterministicOutcome)
	}

	foundCheck := false
	for _, c := range res.Checks {
		if c.Type == CheckCandidateHashMatch {
			foundCheck = true
			if c.Passed {
				t.Errorf("expected CheckCandidateHashMatch to fail")
			}
		}
	}
	if !foundCheck {
		t.Errorf("CheckCandidateHashMatch was not recorded")
	}
}

func TestVerifyCandidate_DerivationHashMismatch(t *testing.T) {
	db, store, engine := setupTestDB(t)
	defer db.Close()
	ctx := context.Background()
	tenantID := "TENANT-DERIV-MISMATCH"
	wfID := "wf-deriv-mismatch"

	_, _ = db.Exec(`INSERT INTO tenants (id, name) VALUES (?, 'Tenant Deriv Mismatch')`, tenantID)

	parentNACHA := generateTestNACHA(true)
	candNACHA := generateTestNACHA(false)

	_, parentSHA, candID, candSHA, derivID, _ := seedVerificationScenario(
		t, db, store, tenantID, wfID, 1, parentNACHA, candNACHA, "VALIDATION_PASSED",
	)

	// Tamper derivation hash in DB
	_, err := db.Exec(`UPDATE artifact_derivations SET derivation_hash = 'tampered-derivation-hash' WHERE id = ?`, derivID)
	if err != nil {
		t.Fatalf("tamper derivation_hash: %v", err)
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
		t.Fatalf("expected outcome corruption/fail, got %q", res.DeterministicOutcome)
	}

	foundCheck := false
	for _, c := range res.Checks {
		if c.Type == CheckDerivationHashMatch {
			foundCheck = true
			if c.Passed {
				t.Errorf("expected CheckDerivationHashMatch to fail")
			}
		}
	}
	if !foundCheck {
		t.Errorf("CheckDerivationHashMatch was not recorded")
	}
}

func TestVerifyCandidate_PolicyMismatch(t *testing.T) {
	db, store, engine := setupTestDB(t)
	defer db.Close()
	ctx := context.Background()
	tenantID := "TENANT-POLICY-MISMATCH"
	wfID := "wf-policy-mismatch"

	_, _ = db.Exec(`INSERT INTO tenants (id, name) VALUES (?, 'Tenant Policy Mismatch')`, tenantID)

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
		PolicyBundleHash:        "stale-bundle-hash-9999", // Deliberately mismatched
	}

	res, err := svc.VerifyCandidate(ctx, makeTestScope(tenantID), req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if res.DeterministicOutcome != OutcomeStale {
		t.Fatalf("expected OutcomeStale, got %q", res.DeterministicOutcome)
	}

	foundCheck := false
	for _, c := range res.Checks {
		if c.Type == CheckPolicyContextFresh {
			foundCheck = true
			if c.Passed {
				t.Errorf("expected CheckPolicyContextFresh to fail")
			}
		}
	}
	if !foundCheck {
		t.Errorf("CheckPolicyContextFresh was not recorded")
	}
}

func TestVerifyCandidate_StoredValidationPass_RecomputedFail(t *testing.T) {
	db, store, engine := setupTestDB(t)
	defer db.Close()
	ctx := context.Background()
	tenantID := "TENANT-VAL-MISMATCH"
	wfID := "wf-val-mismatch"

	_, _ = db.Exec(`INSERT INTO tenants (id, name) VALUES (?, 'Tenant Val Mismatch')`, tenantID)

	parentNACHA := generateTestNACHA(true)
	candNACHAWithErrors := generateTestNACHA(true) // Candidate still has errors!

	// Seed derivation with falsely recorded VALIDATION_PASSED
	_, parentSHA, candID, candSHA, derivID, _ := seedVerificationScenario(
		t, db, store, tenantID, wfID, 1, parentNACHA, candNACHAWithErrors, "VALIDATION_PASSED",
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
		t.Fatalf("unexpected error: %v", err)
	}

	if res.DeterministicOutcome != OutcomeFail {
		t.Fatalf("expected OutcomeFail, got %q", res.DeterministicOutcome)
	}

	// Assert CheckValidatorPass and CheckValidationResultMatch both failed
	valPassChecked := false
	valMatchChecked := false
	for _, c := range res.Checks {
		if c.Type == CheckValidatorPass {
			valPassChecked = true
			if c.Passed {
				t.Errorf("expected CheckValidatorPass to fail")
			}
		}
		if c.Type == CheckValidationResultMatch {
			valMatchChecked = true
			if c.Passed {
				t.Errorf("expected CheckValidationResultMatch to fail")
			}
		}
	}
	if !valPassChecked || !valMatchChecked {
		t.Errorf("expected both validation checks to be recorded (valPass=%v, valMatch=%v)", valPassChecked, valMatchChecked)
	}
}
