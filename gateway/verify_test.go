package main

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"strings"
	"testing"

	_ "modernc.org/sqlite"
	"sentinel-gateway/internal/objectstore"
	"sentinel-gateway/internal/review"
)

func TestVerifyBeforeApproval(t *testing.T) {
	db, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	defer db.Close()

	if _, err := Migrate(db); err != nil {
		t.Fatalf("migrate: %v", err)
	}

	// Create filesystem object store in temporary test directory
	store, err := objectstore.NewFilesystemStore(t.TempDir())
	if err != nil {
		t.Fatalf("create test objectstore: %v", err)
	}
	ctx := context.Background()

	// A valid balanced NACHA sample content
	sample := GenerateNachaScenario(PresetBalancedPayroll)
	content := []byte(sample.Content)
	h := sha256.Sum256(content)
	sha256Hash := hex.EncodeToString(h[:])

	storageKey := "tenant-a/artifacts/101.ach"
	if _, err := store.Put(ctx, storageKey, strings.NewReader(sample.Content), int64(len(content))); err != nil {
		t.Fatalf("put object: %v", err)
	}

	// Calculate findings digest using review.Subject
	subj := review.Subject{Findings: []review.Finding{}}
	expectedFindingsDigest := subj.FindingsDigest()

	// Insert database records
	_, err = db.Exec(`
		INSERT OR IGNORE INTO tenants (id, name) VALUES ('tenant-a', 'Tenant Alpha'), ('tenant-b', 'Tenant Bravo');
		INSERT INTO file_instances (id, tenant_id, filename, storage_path, sha256_hash, size_bytes, status, received_at)
		VALUES (101, 'tenant-a', 'payroll.ach', ?, ?, ?, 'QUARANTINED', CURRENT_TIMESTAMP);
	`, storageKey, sha256Hash, len(content))
	if err != nil {
		t.Fatalf("insert file instance: %v", err)
	}

	t.Run("Independent Verification Passes On Untampered Artifact", func(t *testing.T) {
		res, err := VerifyBeforeApproval(ctx, db, store, "tenant-a", 101)
		if err != nil {
			t.Fatalf("VerifyBeforeApproval error: %v", err)
		}
		if !res.Passed {
			t.Errorf("expected verification to pass, got mismatch: %s", res.MismatchReason)
		}
		if res.ArtifactID != 101 {
			t.Errorf("expected artifact ID 101, got %d", res.ArtifactID)
		}
	})

	t.Run("Verification Fails On Content Hash Mismatch", func(t *testing.T) {
		// Tamper with database hash record
		_, _ = db.Exec("UPDATE file_instances SET sha256_hash = 'tampered_hash' WHERE id = 101")

		res, err := VerifyBeforeApproval(ctx, db, store, "tenant-a", 101)
		if err != nil {
			t.Fatalf("VerifyBeforeApproval error: %v", err)
		}
		if res.Passed {
			t.Errorf("expected verification to FAIL on tampered hash, but it passed")
		}
		if !strings.Contains(res.MismatchReason, "does not match recorded database hash") {
			t.Errorf("expected tamper reason in mismatch, got: %s", res.MismatchReason)
		}

		// Restore original hash
		_, _ = db.Exec("UPDATE file_instances SET sha256_hash = ? WHERE id = 101", sha256Hash)
	})

	t.Run("Verification Fails On Cross-Tenant Lookup", func(t *testing.T) {
		_, err := VerifyBeforeApproval(ctx, db, store, "tenant-b", 101)
		if err == nil {
			t.Errorf("expected error for cross-tenant artifact verification, got nil")
		}
	})

	t.Run("Verification Compares Against Recorded Findings Digest", func(t *testing.T) {
		// Link a validation run with matching findings digest
		_, err := db.Exec(`
			INSERT INTO validation_runs (id, tenant_id, file_instance_id, parser_name, parser_version, rule_pack_version, contract_id, contract_version, outcome, parser_ok, findings_digest, records_parsed, total_debits_minor, total_credits_minor, started_at, completed_at)
			VALUES (1, 'tenant-a', 101, 'nacha', '1.0.0', 'nacha-2025/1', 'contract-1', 'v1', 'VALIDATED', 1, ?, 10, 1000, 1000, CURRENT_TIMESTAMP, CURRENT_TIMESTAMP);
		`, expectedFindingsDigest)
		if err != nil {
			t.Fatalf("insert validation run: %v", err)
		}

		res, err := VerifyBeforeApproval(ctx, db, store, "tenant-a", 101)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if !res.Passed {
			t.Errorf("expected verification to pass with matching findings digest, failed: %s", res.MismatchReason)
		}
	})
}
