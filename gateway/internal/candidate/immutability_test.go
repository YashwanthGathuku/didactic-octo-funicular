package candidate

import (
	"bytes"
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"errors"
	"io"
	"testing"
	"time"

	"sentinel-gateway/internal/objectstore"
	"sentinel-gateway/internal/repository"
)

func setupTestService(t *testing.T) (*Service, *sql.DB, objectstore.ObjectStore) {
	db, store, engine := setupCandidateTestDB(t)
	svc := NewService(db, store, engine)
	return svc, db, store
}

func TestCandidateOncePersistedIsImmutable(t *testing.T) {
	svc, db, store := setupTestService(t)
	ctx := context.Background()

	validNacha := generateTestNACHA(false)
	validHash := sha256.Sum256([]byte(validNacha))
	validHashHex := hex.EncodeToString(validHash[:])

	parentKey, _ := objectstore.NewKey("t1", time.Now().UTC())
	_, _ = db.Exec(`INSERT INTO tenants (id, name) VALUES ('t1', 't1')`)
	_, _ = store.Put(ctx, parentKey, bytes.NewReader([]byte(validNacha)), int64(len(validNacha)))
	_, _ = db.Exec(`INSERT INTO file_instances (id, tenant_id, filename, storage_path, size_bytes, sha256_hash, status) VALUES (2, 't1', 'p_nacha', ?, ?, ?, 'QUARANTINED')`, parentKey, len(validNacha), validHashHex)

	req := &CandidateCreationRequest{
		TenantID:             "t1",
		WorkflowID:           "wf1",
		AttemptNumber:        1,
		ParentArtifactID:     2,
		ExpectedParentSHA256: validHashHex,
		PlanHash:             "plan123",
		AgentName:            "RemediationAgent",
		Operations: []RemediationOperation{
			{OperationType: OpRecomputeBatchControlTotal, TargetRef: "BATCH-1"},
			{OperationType: OpRecomputeFileControlTotal, TargetRef: "FILE_CONTROL"},
		},
	}

	res, err := svc.GenerateCandidate(ctx, repository.Scope{}, req)
	if err != nil {
		t.Fatalf("GenerateCandidate failed: %v", err)
	}
	if res.CandidateArtifactID == 0 {
		t.Fatalf("expected valid candidate artifact ID")
	}

	// Verify that attempting to overwrite the candidate in ObjectStore fails
	logicalPayload := "t1:wf1:1:" + validHashHex + ":plan123"
	hLogical := sha256.Sum256([]byte(logicalPayload))
	candidateKey, _ := objectstore.DeterministicKey("t1", hex.EncodeToString(hLogical[:]))

	_, putErr := store.Put(ctx, candidateKey, bytes.NewReader([]byte("TAMPERED_BYTES")), 14)
	if !errors.Is(putErr, objectstore.ErrObjectExists) {
		t.Errorf("Expected ErrObjectExists on candidate overwrite attempt, got %v", putErr)
	}
}

func TestOriginalImmutability_ObjectStoreReRead(t *testing.T) {
	svc, db, store := setupTestService(t)
	ctx := context.Background()

	validNacha := generateTestNACHA(true)
	validHash := sha256.Sum256([]byte(validNacha))
	validHashHex := hex.EncodeToString(validHash[:])

	parentPath := "/tenant/t1/parent_immut"
	_, _ = db.Exec(`INSERT INTO tenants (id, name) VALUES ('t1', 't1')`)
	_, _ = store.Put(ctx, parentPath, bytes.NewReader([]byte(validNacha)), int64(len(validNacha)))
	_, _ = db.Exec(`INSERT INTO file_instances (id, tenant_id, filename, storage_path, size_bytes, sha256_hash, status) VALUES (10, 't1', 'p_immut', ?, ?, ?, 'QUARANTINED')`, parentPath, len(validNacha), validHashHex)

	req := &CandidateCreationRequest{
		TenantID:             "t1",
		WorkflowID:           "wf-immut",
		AttemptNumber:        1,
		ParentArtifactID:     10,
		ExpectedParentSHA256: validHashHex,
		PlanHash:             "plan-immut",
		AgentName:            "RemediationAgent",
		Operations: []RemediationOperation{
			{OperationType: OpRecomputeBatchControlTotal, TargetRef: "BATCH-1"},
			{OperationType: OpRecomputeFileControlTotal, TargetRef: "FILE_CONTROL"},
		},
	}

	_, err := svc.GenerateCandidate(ctx, repository.Scope{}, req)
	if err != nil {
		t.Fatalf("GenerateCandidate failed: %v", err)
	}

	// Fresh re-read from ObjectStore after candidate creation
	rc, err := store.Get(ctx, parentPath)
	if err != nil {
		t.Fatalf("fresh read parent failed: %v", err)
	}
	defer rc.Close()
	bytesAfter, err := io.ReadAll(rc)
	if err != nil {
		t.Fatalf("read bytes after: %v", err)
	}

	hAfter := sha256.Sum256(bytesAfter)
	hAfterHex := hex.EncodeToString(hAfter[:])

	if hAfterHex != validHashHex {
		t.Fatalf("Original artifact modified in storage: before=%s, after=%s", validHashHex, hAfterHex)
	}
}

func TestDerivationRecordImmutability(t *testing.T) {
	_, db, _ := setupTestService(t)

	_, _ = db.Exec(`INSERT INTO tenants (id, name) VALUES ('t1', 't1')`)
	_, _ = db.Exec(`INSERT INTO artifact_derivations (
		id, tenant_id, workflow_id, remediation_plan_id, attempt_number, parent_artifact_id, parent_sha256,
		candidate_artifact_id, candidate_sha256, remediation_plan_hash, operation_types_json, derivation_hash
	) VALUES ('d1', 't1', 'w1', 'p1', 1, 1, 'sha1', 2, 'sha2', 'ph1', '[]', 'dh1')`)

	// Verify attempt to insert duplicate attempt fails with unique constraint
	_, err := db.Exec(`INSERT INTO artifact_derivations (
		id, tenant_id, workflow_id, remediation_plan_id, attempt_number, parent_artifact_id, parent_sha256,
		candidate_artifact_id, candidate_sha256, remediation_plan_hash, operation_types_json, derivation_hash
	) VALUES ('d2', 't1', 'w1', 'p2', 1, 1, 'sha1', 3, 'sha3', 'ph2', '[]', 'dh2')`)

	if err == nil {
		t.Fatalf("Expected unique constraint violation on duplicate attempt number for same workflow")
	}
}
