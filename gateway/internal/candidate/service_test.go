package candidate

import (
	"bytes"
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"strings"
	"testing"
	"time"

	"sentinel-gateway/internal/auth"
	"sentinel-gateway/internal/objectstore"
	"sentinel-gateway/internal/policy"
	"sentinel-gateway/internal/repository"

	_ "modernc.org/sqlite"
)

// Helper to create test in-memory SQLite and ObjectStore
func setupCandidateTestDB(t *testing.T) (*sql.DB, objectstore.ObjectStore, *policy.PolicyEngine) {
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
	`
	if _, err := db.Exec(schema); err != nil {
		t.Fatalf("exec schema: %v", err)
	}

	store, err := objectstore.NewFilesystemStore(t.TempDir())
	if err != nil {
		t.Fatalf("create filesystem store: %v", err)
	}

	// Seed safety policies into Policy Engine
	engine := policy.NewEngineWithDefaults()

	return db, store, engine
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

// Generates a valid or deliberately quarantined NACHA file
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

	// Hash sum: 12100035 + 01100001 = 13200036
	declaredDebits := int64(15000)
	if corruptBatchDebits {
		declaredDebits = int64(0) // Mismatched batch control debits
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

func TestCandidateService_GenerateCandidate_SuccessAndRevalidation(t *testing.T) {
	db, store, engine := setupCandidateTestDB(t)
	defer db.Close()
	ctx := context.Background()
	tenantID := "TENANT-CAND-01"

	// 1. Seed tenant and quarantined parent artifact
	_, err := db.Exec(`INSERT INTO tenants (id, name) VALUES ('TENANT-CAND-01', 'Tenant Cand')`)
	if err != nil {
		t.Fatalf("insert tenant: %v", err)
	}

	nachaText := generateTestNACHA(true) // Corrupted batch & file control
	nachaBytes := []byte(nachaText)
	parentSHABytes := sha256.Sum256(nachaBytes)
	parentSHAHex := hex.EncodeToString(parentSHABytes[:])

	parentKey, _ := objectstore.NewKey(tenantID, time.Now().UTC())
	putRes, err := store.Put(ctx, parentKey, bytes.NewReader(nachaBytes), int64(len(nachaBytes)))
	if err != nil {
		t.Fatalf("put parent artifact: %v", err)
	}

	res, err := db.Exec(`
		INSERT INTO file_instances (tenant_id, filename, storage_path, size_bytes, sha256_hash, status)
		VALUES (?, 'payroll_quarantined.ach', ?, ?, ?, 'QUARANTINED')`,
		tenantID, parentKey, len(nachaBytes), parentSHAHex,
	)
	if err != nil {
		t.Fatalf("insert file_instance: %v", err)
	}
	parentArtifactID, _ := res.LastInsertId()

	resInc, _ := db.Exec(`INSERT INTO incidents (tenant_id, file_instance_id, type, severity, status) VALUES (?, ?, 'VALIDATION_FAILED', 'HIGH', 'OPEN')`, tenantID, parentArtifactID)
	incidentID, _ := resInc.LastInsertId()

	svc := NewService(db, store, engine)
	scope := makeTestScope(tenantID)

	req := &CandidateCreationRequest{
		TenantID:             tenantID,
		WorkflowID:           "wf-cand-001",
		IncidentID:           incidentID,
		ParentArtifactID:     parentArtifactID,
		ExpectedParentSHA256: parentSHAHex,
		AttemptNumber:        1,
		PlanHash:             "plan-hash-12345",
		Operations: []RemediationOperation{
			{
				OperationType: OpRecomputeBatchControlTotal,
				TargetRef:     "BATCH-1",
				FindingRefs:   []string{"FINDING-001"},
				Rationale:     "Recompute batch debit total from entry records",
			},
			{
				OperationType: OpRecomputeFileControlTotal,
				TargetRef:     "FILE_CONTROL",
				FindingRefs:   []string{"FINDING-002"},
				Rationale:     "Recompute file debit total and block count",
			},
		},
		FindingRefs:        []string{"FINDING-001", "FINDING-002"},
		Confidence:         "HIGH",
		AgentName:          "RemediationAgent",
		AgentVersion:       "1.0.0",
		PolicyDecisionID:   "POL-DEC-001",
		PolicyDecisionHash: "pol-hash-001",
		ToolManifestHash:   "tool-man-hash-001",
	}

	candRes, err := svc.GenerateCandidate(ctx, scope, req)
	if err != nil {
		t.Fatalf("generate candidate: %v", err)
	}

	if candRes.ValidationOutcome != "VALIDATION_PASSED" {
		t.Errorf("expected validation outcome VALIDATION_PASSED, got %s (findings: %+v)", candRes.ValidationOutcome, candRes.Findings)
	}
	if candRes.BlockingFindingsCount != 0 {
		t.Errorf("expected 0 blocking findings, got %d", candRes.BlockingFindingsCount)
	}
	if candRes.CandidateSHA256 == parentSHAHex {
		t.Errorf("candidate SHA-256 must differ from parent SHA-256")
	}

	// Verify original artifact remains untouched in ObjectStore
	rc, err := store.Get(ctx, parentKey)
	if err != nil {
		t.Fatalf("get parent from store: %v", err)
	}
	defer rc.Close()
	readOriginal, _ := io.ReadAll(rc)
	hOrig := sha256.Sum256(readOriginal)
	readOriginalHash := hex.EncodeToString(hOrig[:])
	if readOriginalHash != putRes.SHA256 {
		t.Fatalf("original artifact was mutated in storage!")
	}
}

func TestCandidateService_ParentSHA256Mismatch_FailsClosed(t *testing.T) {
	db, store, engine := setupCandidateTestDB(t)
	defer db.Close()
	ctx := context.Background()
	tenantID := "TENANT-CAND-01"

	_, _ = db.Exec(`INSERT INTO tenants (id, name) VALUES ('TENANT-CAND-01', 'Tenant Cand')`)
	res, _ := db.Exec(`INSERT INTO file_instances (tenant_id, filename, storage_path, size_bytes, sha256_hash, status) VALUES ('TENANT-CAND-01', 'file.ach', 'key-1', 100, 'actual-sha-256', 'QUARANTINED')`)
	parentArtifactID, _ := res.LastInsertId()

	svc := NewService(db, store, engine)
	scope := makeTestScope(tenantID)

	req := &CandidateCreationRequest{
		TenantID:             tenantID,
		WorkflowID:           "wf-toctou-001",
		IncidentID:           1,
		ParentArtifactID:     parentArtifactID,
		ExpectedParentSHA256: "stale-sha-256-does-not-match",
		AttemptNumber:        1,
		PlanHash:             "plan-hash-1",
		Operations:           []RemediationOperation{{OperationType: OpRecomputeBatchControlTotal, TargetRef: "BATCH-1"}},
	}

	_, err := svc.GenerateCandidate(ctx, scope, req)
	if err == nil || !errors.Is(err, ErrParentSHA256Mismatch) {
		t.Fatalf("expected ErrParentSHA256Mismatch, got %v", err)
	}
}

func TestCandidateService_MaxAttemptsExceeded(t *testing.T) {
	db, store, engine := setupCandidateTestDB(t)
	defer db.Close()
	ctx := context.Background()
	tenantID := "TENANT-CAND-01"

	svc := NewService(db, store, engine)
	scope := makeTestScope(tenantID)

	req := &CandidateCreationRequest{
		TenantID:             tenantID,
		WorkflowID:           "wf-attempt4-001",
		IncidentID:           1,
		ParentArtifactID:     1,
		ExpectedParentSHA256: "sha-1",
		AttemptNumber:        4, // Attempt 4 exceeds max 3
		PlanHash:             "plan-hash-4",
		Operations:           []RemediationOperation{{OperationType: OpRecomputeBatchControlTotal, TargetRef: "BATCH-1"}},
	}

	_, err := svc.GenerateCandidate(ctx, scope, req)
	if err == nil || !errors.Is(err, ErrMaxAttemptsExceeded) {
		t.Fatalf("expected ErrMaxAttemptsExceeded on attempt 4, got %v", err)
	}
}
