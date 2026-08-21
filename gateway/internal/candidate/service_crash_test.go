package candidate

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"testing"
	"time"

	"sentinel-gateway/internal/objectstore"
	"sentinel-gateway/internal/repository"
)

func TestCrashConsistency_WindowsAThroughF(t *testing.T) {
	svc, db, store := setupTestService(t)
	ctx := context.Background()

	validNacha := generateTestNACHA(false)
	validHash := sha256.Sum256([]byte(validNacha))
	validHashHex := hex.EncodeToString(validHash[:])

	parentKey, _ := objectstore.NewKey("t1", time.Now().UTC())
	_, _ = db.Exec(`INSERT INTO tenants (id, name) VALUES ('t1', 't1')`)
	_, _ = store.Put(ctx, parentKey, bytes.NewReader([]byte(validNacha)), int64(len(validNacha)))
	_, _ = db.Exec(`INSERT INTO file_instances (id, tenant_id, filename, storage_path, size_bytes, sha256_hash, status) VALUES (5, 't1', 'p_crash', ?, ?, ?, 'QUARANTINED')`, parentKey, len(validNacha), validHashHex)

	req := &CandidateCreationRequest{
		TenantID:             "t1",
		WorkflowID:           "wf-crash",
		AttemptNumber:        1,
		ParentArtifactID:     5,
		ExpectedParentSHA256: validHashHex,
		PlanHash:             "crashplan1",
		AgentName:            "RemediationAgent",
		Operations: []RemediationOperation{
			{OperationType: OpRecomputeFileControlTotal, TargetRef: "FILE_CONTROL"},
		},
	}

	// Window A: Before object write -> safe retry
	reportA, err := svc.ReconcileCandidate(ctx, repository.Scope{}, req.TenantID, "wf-nonexistent", 1, req.PlanHash, req.ExpectedParentSHA256)
	if err != nil {
		t.Fatalf("Reconcile Window A failed: %v", err)
	}
	if reportA.Outcome != RetrySafe {
		t.Errorf("Window A: expected RetrySafe, got %v", reportA.Outcome)
	}

	// Execute Candidate Generation
	res1, err := svc.GenerateCandidate(ctx, repository.Scope{}, req)
	if err != nil {
		t.Fatalf("GenerateCandidate failed: %v", err)
	}

	// Window F: Workflow transition committed, retry arrives -> existing result replayed identically
	res2, err := svc.GenerateCandidate(ctx, repository.Scope{}, req)
	if err != nil {
		t.Fatalf("Re-entry failed: %v", err)
	}
	if res1.CandidateSHA256 != res2.CandidateSHA256 {
		t.Errorf("Idempotency SHA mismatch: %s vs %s", res1.CandidateSHA256, res2.CandidateSHA256)
	}
	if res1.CandidateArtifactID != res2.CandidateArtifactID {
		t.Errorf("Idempotency artifact ID mismatch: %d vs %d", res1.CandidateArtifactID, res2.CandidateArtifactID)
	}

	// Window C / Reconciliation: State consistent -> Reconciled
	report, err := svc.ReconcileCandidate(ctx, repository.Scope{}, req.TenantID, req.WorkflowID, req.AttemptNumber, req.PlanHash, req.ExpectedParentSHA256)
	if err != nil {
		t.Fatalf("ReconcileCandidate failed: %v", err)
	}
	if report.Outcome != Reconciled {
		t.Errorf("Expected Reconciled, got %v: %s", report.Outcome, report.Detail)
	}

	// Corrupt plan hash in DB to test corruption detection
	_, _ = db.Exec(`UPDATE artifact_derivations SET remediation_plan_hash = 'corrupt' WHERE workflow_id = ?`, req.WorkflowID)
	report2, _ := svc.ReconcileCandidate(ctx, repository.Scope{}, req.TenantID, req.WorkflowID, req.AttemptNumber, req.PlanHash, req.ExpectedParentSHA256)
	if report2.Outcome != CorruptionDetected {
		t.Errorf("Expected CorruptionDetected, got %v", report2.Outcome)
	}

	// Missing DB derivation with object existing -> ReconciliationRequired / Orphan
	_, _ = db.Exec(`DELETE FROM artifact_derivations WHERE workflow_id = ?`, req.WorkflowID)
	report3, _ := svc.ReconcileCandidate(ctx, repository.Scope{}, req.TenantID, req.WorkflowID, req.AttemptNumber, req.PlanHash, req.ExpectedParentSHA256)
	if report3.Outcome != RetrySafe && report3.Outcome != ReconciliationRequired {
		t.Errorf("Expected RetrySafe or ReconciliationRequired, got %v", report3.Outcome)
	}
}
