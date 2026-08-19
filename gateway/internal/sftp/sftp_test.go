package sftp

import (
	"context"
	"database/sql"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	_ "modernc.org/sqlite"

	"sentinel-gateway/internal/jobs"
	"sentinel-gateway/internal/ledger"
	"sentinel-gateway/internal/objectstore"
)

func setupTestDB(t *testing.T) *sql.DB {
	t.Helper()
	db, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatal(err)
	}

	schema := `
	CREATE TABLE tenants (id TEXT PRIMARY KEY, name TEXT);
	INSERT INTO tenants (id, name) VALUES ('TENANT-SFTP', 'SFTP Tenant');

	CREATE TABLE file_instances (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		tenant_id TEXT NOT NULL REFERENCES tenants(id),
		expectation_id INTEGER,
		filename TEXT NOT NULL,
		storage_path TEXT NOT NULL,
		size_bytes INTEGER NOT NULL,
		sha256_hash TEXT NOT NULL,
		status TEXT NOT NULL,
		received_at TIMESTAMP NOT NULL,
		created_at TIMESTAMP NOT NULL,
		updated_at TIMESTAMP NOT NULL
	);
	CREATE INDEX idx_file_instances_tenant_hash ON file_instances(tenant_id, sha256_hash);

	CREATE TABLE ingestion_jobs (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		tenant_id TEXT NOT NULL REFERENCES tenants(id),
		file_instance_id INTEGER REFERENCES file_instances(id),
		kind TEXT NOT NULL DEFAULT 'VALIDATE_ARTIFACT',
		idempotency_key TEXT NOT NULL,
		state TEXT NOT NULL DEFAULT 'QUEUED',
		attempt_count INTEGER NOT NULL DEFAULT 0,
		max_attempts INTEGER NOT NULL DEFAULT 3,
		row_version INTEGER NOT NULL DEFAULT 1,
		lease_owner TEXT,
		lease_expires_at TIMESTAMP,
		last_heartbeat_at TIMESTAMP,
		run_after TIMESTAMP,
		terminal_reason TEXT,
		last_error TEXT,
		created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
		updated_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
		UNIQUE (tenant_id, idempotency_key)
	);

	CREATE TABLE audit_events (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		tenant_id TEXT NOT NULL REFERENCES tenants(id),
		sequence_no INTEGER NOT NULL,
		action TEXT NOT NULL,
		actor TEXT NOT NULL,
		object_type TEXT,
		object_id INTEGER,
		correlation_id TEXT,
		payload TEXT NOT NULL,
		previous_hash TEXT NOT NULL,
		current_hash TEXT NOT NULL,
		occurred_at TIMESTAMP NOT NULL,
		created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
		UNIQUE (tenant_id, sequence_no),
		UNIQUE (tenant_id, previous_hash)
	);
	`
	if _, err := db.Exec(schema); err != nil {
		t.Fatal(err)
	}
	return db
}

func TestEventValidation_RejectsInFlightOrPartialTransfers(t *testing.T) {
	// 1. In-flight upload (status 0)
	evtInFlight := FinalizedUploadEvent{
		Action:      "upload",
		Status:      0,
		VirtualPath: "/inbound/file.ach",
		SizeBytes:   100,
		SHA256Hash:  "e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855",
	}
	if err := evtInFlight.Validate(); err == nil {
		t.Error("expected error for in-flight transfer (status 0)")
	}

	// 2. Download action (not upload)
	evtDownload := FinalizedUploadEvent{
		Action:      "download",
		Status:      1,
		VirtualPath: "/inbound/file.ach",
		SizeBytes:   100,
		SHA256Hash:  "e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855",
	}
	if err := evtDownload.Validate(); err == nil {
		t.Error("expected error for download action")
	}

	// 3. Temporary file extension
	evtTmp := FinalizedUploadEvent{
		Action:      "upload",
		Status:      1,
		VirtualPath: "/inbound/.upload.tmp",
		SizeBytes:   100,
		SHA256Hash:  "e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855",
	}
	if err := evtTmp.Validate(); err == nil {
		t.Error("expected error for temporary file extension")
	}

	// 4. Valid finalized upload
	evtValid := FinalizedUploadEvent{
		Action:      "upload",
		Status:      1,
		VirtualPath: "/inbound/payroll_2026.ach",
		SizeBytes:   1024,
		SHA256Hash:  "e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855",
	}
	if err := evtValid.Validate(); err != nil {
		t.Errorf("unexpected error on valid event: %v", err)
	}
}

func TestWebhookAuthentication_EnforcesHMACSignature(t *testing.T) {
	secret := "test-sftp-webhook-secret-key-9912"
	now := time.Now().Unix()
	payload := []byte(`{"action":"upload","status":1}`)

	validSig := ComputeSignature(secret, now, payload)

	// Valid signature passes
	if err := VerifySignature(secret, validSig, now, payload, 5*time.Minute); err != nil {
		t.Errorf("valid signature failed verification: %v", err)
	}

	// Tampered payload fails
	tamperedPayload := []byte(`{"action":"upload","status":0}`)
	if err := VerifySignature(secret, validSig, now, tamperedPayload, 5*time.Minute); err == nil {
		t.Error("expected failure on tampered payload")
	}

	// Expired timestamp fails
	expiredTime := now - 600 // 10 minutes ago
	expiredSig := ComputeSignature(secret, expiredTime, payload)
	if err := VerifySignature(secret, expiredSig, expiredTime, payload, 5*time.Minute); err == nil {
		t.Error("expected failure on expired timestamp skew")
	}
}

func TestIngressService_IdempotentDeduplication(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()

	store, err := objectstore.NewFilesystemStore(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	queue, err := jobs.New(db, "sqlite")
	if err != nil {
		t.Fatal(err)
	}
	evidence, err := ledger.New(db, "sqlite")
	if err != nil {
		t.Fatal(err)
	}

	secret := "sftp-secret-01"
	service := NewIngressService(db, store, queue, evidence, secret)

	ctx := context.Background()
	now := time.Now().Unix()
	rawEvent := []byte(`{
		"event_id": "EVT-1001",
		"action": "upload",
		"status": 1,
		"timestamp": 1771448400000,
		"username": "partner_bank",
		"virtual_path": "/inbound/ach/payroll.ach",
		"fs_path": "/data/sftp/payroll.ach",
		"file_size": 2048,
		"sha256_hash": "e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855"
	}`)
	sig := ComputeSignature(secret, now, rawEvent)

	// First ingestion
	res1, err := service.HandleWebhook(ctx, "TENANT-SFTP", sig, now, rawEvent)
	if err != nil {
		t.Fatalf("first ingestion failed: %v", err)
	}
	if res1.Deduplicated {
		t.Errorf("first ingestion should not be deduplicated")
	}
	if res1.ArtifactID <= 0 || res1.JobID <= 0 {
		t.Errorf("invalid IDs: artifact=%d job=%d", res1.ArtifactID, res1.JobID)
	}

	// Duplicate arrival
	res2, err := service.HandleWebhook(ctx, "TENANT-SFTP", sig, now, rawEvent)
	if err != nil {
		t.Fatalf("duplicate ingestion failed: %v", err)
	}
	if !res2.Deduplicated {
		t.Errorf("duplicate arrival must be marked deduplicated")
	}
	if res2.ArtifactID != res1.ArtifactID || res2.JobID != res1.JobID {
		t.Errorf("duplicate arrival returned different identifiers: %+v vs %+v", res2, res1)
	}

	// Verify database contains exactly 1 file instance and 1 job
	var count int
	_ = db.QueryRow("SELECT COUNT(*) FROM file_instances WHERE tenant_id = 'TENANT-SFTP'").Scan(&count)
	if count != 1 {
		t.Errorf("expected 1 file_instance, found %d", count)
	}
}

func TestReconciliationScanner_RecoversMissedFiles(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()

	store, err := objectstore.NewFilesystemStore(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	queue, err := jobs.New(db, "sqlite")
	if err != nil {
		t.Fatal(err)
	}
	evidence, _ := ledger.New(db, "sqlite")

	service := NewIngressService(db, store, queue, evidence, "")
	scanner := NewReconciliationScanner(service, 50*time.Millisecond)

	tmpDir := t.TempDir()

	// 1. Create a finalized settled file
	filePath := filepath.Join(tmpDir, "batch_recovered.ach")
	if err := os.WriteFile(filePath, []byte("settled file content\n"), 0644); err != nil {
		t.Fatal(err)
	}

	// 2. Create an in-flight .tmp file (should be skipped)
	tmpFilePath := filepath.Join(tmpDir, ".upload_in_flight.tmp")
	_ = os.WriteFile(tmpFilePath, []byte("partial content"), 0644)

	// Wait past settle window
	time.Sleep(100 * time.Millisecond)

	ctx := context.Background()
	report, err := scanner.ScanDirectory(ctx, "TENANT-SFTP", tmpDir)
	if err != nil {
		t.Fatalf("reconciliation scan failed: %v", err)
	}

	if report.RecoveredCount != 1 {
		t.Errorf("expected 1 recovered file, got %d", report.RecoveredCount)
	}
	if report.SkippedCount != 1 {
		t.Errorf("expected 1 skipped .tmp file, got %d", report.SkippedCount)
	}

	// Second run should recover 0 new files (idempotent)
	report2, _ := scanner.ScanDirectory(ctx, "TENANT-SFTP", tmpDir)
	if report2.RecoveredCount != 0 {
		t.Errorf("second scan should recover 0 files, got %d", report2.RecoveredCount)
	}
}

func TestWinSCPAndOpenSSH_DiagnosticsAndScripting(t *testing.T) {
	// 1. WinSCP .filepart rejection in event validation
	evtFilePart := FinalizedUploadEvent{
		Action:      "upload",
		Status:      1,
		VirtualPath: "/inbound/payroll.ach.filepart",
		SizeBytes:   1024,
		SHA256Hash:  "e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855",
	}
	if err := evtFilePart.Validate(); err == nil {
		t.Error("expected validation rejection on WinSCP .filepart in-flight file")
	}

	// 2. OpenSSH .part rejection
	evtPart := FinalizedUploadEvent{
		Action:      "upload",
		Status:      1,
		VirtualPath: "/inbound/payroll.ach.part",
		SizeBytes:   1024,
		SHA256Hash:  "e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855",
	}
	if err := evtPart.Validate(); err == nil {
		t.Error("expected validation rejection on OpenSSH .part in-flight file")
	}

	// 3. Inspect PuTTY PPK key
	ppkData := []byte("PuTTY-User-Key-File-3: ssh-rsa\nEncryption: none\nComment: bny-treasury-key\n")
	resPPK := InspectSSHKey(ppkData)
	if resPPK.Format != KeyFormatPuTTYv3 || !resPPK.WinSCPDirect || resPPK.OpenSSHDirect {
		t.Errorf("unexpected PPK inspection outcome: %+v", resPPK)
	}

	// 4. Inspect OpenSSH PEM key
	opensshData := []byte("-----BEGIN OPENSSH PRIVATE KEY-----\nb3BlbnNzaC1rZXktdjEAAAAABG5vbmU...\n-----END OPENSSH PRIVATE KEY-----\n")
	resOpenSSH := InspectSSHKey(opensshData)
	if resOpenSSH.Format != KeyFormatOpenSSH || !resOpenSSH.OpenSSHDirect || resOpenSSH.WinSCPDirect {
		t.Errorf("unexpected OpenSSH inspection outcome: %+v", resOpenSSH)
	}

	// 5. Stale lock detection
	tmpDir := t.TempDir()
	staleLockPath := filepath.Join(tmpDir, "payroll_dropped.ach.filepart")
	_ = os.WriteFile(staleLockPath, []byte("stale lock content"), 0644)

	locks, err := ScanStaleLocks(tmpDir, 1*time.Millisecond)
	if err != nil {
		t.Fatal(err)
	}
	if len(locks) != 1 || locks[0].ClientType != "WinSCP" {
		t.Errorf("expected 1 WinSCP lock, found %+v", locks)
	}

	// 6. WinSCP script generation
	script := GenerateWinSCPScript("sftp.meridian.bank", 2222, "svc_treasury", "C:\\keys\\treasury.ppk", "D:\\Outbound", "/inbound/ach")
	if !strings.Contains(script, "open sftp://svc_treasury@sftp.meridian.bank:2222/") || !strings.Contains(script, "-resumesupport=on") {
		t.Errorf("malformed WinSCP script: %s", script)
	}
}
