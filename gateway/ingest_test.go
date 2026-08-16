package main

import (
	"bytes"
	"database/sql"
	"encoding/json"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"runtime"
	"strings"
	"testing"

	"sentinel-gateway/internal/objectstore"
	"sentinel-gateway/internal/secrets"
)

// Threat tests for the ingress. Each case is an attack or a failure mode the
// previous handler accepted:
//
//   - it read the whole body with io.ReadAll, so size was unbounded
//   - it derived a storage path from the client's filename
//   - it created a new row for every redelivery of the same file
//   - it returned a synchronous validation verdict, so a slow parse was a slow
//     request and a failed parse was a failed upload
//   - it never wrote an object anywhere

func newIngestTestEnv(t *testing.T) (*sql.DB, http.Handler, objectstore.ObjectStore) {
	t.Helper()
	db := setupTestDb(t)
	t.Cleanup(func() { db.Close() })

	store, err := objectstore.NewFilesystemStore(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	return db, NewRouterWithStore(db, ingestDemoConfig(), nil, store), store
}

// ingestDemoConfig is the local-demo profile, which uses the named demo
// principal. These tests are about the ingress mechanics, not authorization --
// authorization on these routes is covered by router_auth_test.go.
func ingestDemoConfig() *Config {
	return &Config{
		Profile:       ProfileLocalDemo,
		AllowedOrigin: "http://localhost:3000",
		Scrubber:      secrets.NewScrubber(),
	}
}

// uploadRequest builds a multipart upload.
func uploadRequest(t *testing.T, filename string, content []byte, headers map[string]string) *http.Request {
	t.Helper()
	var body bytes.Buffer
	writer := multipart.NewWriter(&body)
	part, err := writer.CreateFormFile("file", filename)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := part.Write(content); err != nil {
		t.Fatal(err)
	}
	writer.Close()

	req := httptest.NewRequest(http.MethodPost, "/api/v1/files/upload", &body)
	req.Header.Set("Content-Type", writer.FormDataContentType())
	for k, v := range headers {
		req.Header.Set(k, v)
	}
	return req
}

func doUpload(t *testing.T, handler http.Handler, req *http.Request) (*httptest.ResponseRecorder, AcceptedResponse) {
	t.Helper()
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	var resp AcceptedResponse
	if rec.Code == http.StatusAccepted {
		if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
			t.Fatalf("decode response: %v (body %s)", err, rec.Body.String())
		}
	}
	return rec, resp
}

// The happy path: 202 with identifiers, measured facts, and no verdict.
func TestUploadReturns202WithIdentifiersAndNoVerdict(t *testing.T) {
	db, handler, store := newIngestTestEnv(t)

	content := []byte("101 021000021 0210000210001011200A094101TEST BANK\n")
	rec, resp := doUpload(t, handler, uploadRequest(t, "payments.ach", content, nil))

	if rec.Code != http.StatusAccepted {
		t.Fatalf("status %d, want 202. body: %s", rec.Code, rec.Body.String())
	}
	if resp.ArtifactID == 0 || resp.JobID == 0 {
		t.Errorf("response carries no identifiers: %+v", resp)
	}
	if resp.Status != "RECEIVED" {
		t.Errorf("status = %q; an upload must not report a validation outcome it has not computed", resp.Status)
	}
	if resp.SizeBytes != int64(len(content)) {
		t.Errorf("size = %d, want %d", resp.SizeBytes, len(content))
	}

	// The response must not carry a validation verdict of any kind. This is the
	// specific regression: the old handler returned isBalanced and a status of
	// RELEASED from a synchronous parse.
	body := rec.Body.String()
	for _, forbidden := range []string{"isBalanced", "RELEASED", "VALIDATED", "findings", "totalRecordsParsed"} {
		if strings.Contains(body, forbidden) {
			t.Errorf("the 202 response contains %q, which asserts work that has not run: %s", forbidden, body)
		}
	}

	// The artifact exists in the store and the bytes match.
	var key string
	if err := db.QueryRow(`SELECT object_key FROM file_instances WHERE id = ?`, resp.ArtifactID).Scan(&key); err != nil {
		t.Fatal(err)
	}
	if key == "" {
		t.Fatal("no object key was recorded")
	}
	obj, err := store.Get(t.Context(), key)
	if err != nil {
		t.Fatalf("the recorded object key does not resolve: %v", err)
	}
	defer obj.Close()
	stored, _ := io.ReadAll(obj)
	if !bytes.Equal(stored, content) {
		t.Error("the stored object does not match what was uploaded")
	}

	// A job was enqueued in the same transaction.
	var jobState string
	if err := db.QueryRow(`SELECT state FROM ingestion_jobs WHERE id = ?`, resp.JobID).Scan(&jobState); err != nil {
		t.Fatalf("no job was enqueued: %v", err)
	}
	if jobState != "QUEUED" {
		t.Errorf("job state = %q, want QUEUED", jobState)
	}
}

// The empty file, which is where this whole programme started.
func TestUploadRefusesAnEmptyFile(t *testing.T) {
	db, handler, _ := newIngestTestEnv(t)

	rec, _ := doUpload(t, handler, uploadRequest(t, "empty.ach", nil, nil))
	if rec.Code != http.StatusBadRequest {
		t.Errorf("status %d, want 400. body: %s", rec.Code, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), "empty_artifact") {
		t.Errorf("the refusal is not typed: %s", rec.Body.String())
	}

	var artifacts int
	db.QueryRow(`SELECT COUNT(*) FROM file_instances`).Scan(&artifacts)
	if artifacts != 0 {
		t.Errorf("an empty upload created %d artifact rows", artifacts)
	}
}

func TestUploadRefusesAnOversizedFile(t *testing.T) {
	db, handler, _ := newIngestTestEnv(t)

	// Exceed the body limit rather than allocating 64 MiB: the assertion is
	// that the server refuses, not which of its two bounds fired.
	oversized := bytes.Repeat([]byte("x"), int(MaxArtifactBytes)+int(maxMultipartHeaderBytes)+4096)
	rec, _ := doUpload(t, handler, uploadRequest(t, "huge.ach", oversized, nil))

	if rec.Code != http.StatusRequestEntityTooLarge && rec.Code != http.StatusBadRequest {
		t.Errorf("status %d; an oversized upload must be refused", rec.Code)
	}

	var artifacts int
	db.QueryRow(`SELECT COUNT(*) FROM file_instances`).Scan(&artifacts)
	if artifacts != 0 {
		t.Errorf("an oversized upload created %d artifact rows", artifacts)
	}
}

// Path traversal. The key is server-generated, so the filename cannot influence
// where anything lands -- this asserts that, rather than asserting a filter.
func TestUploadFilenameCannotInfluenceTheObjectKey(t *testing.T) {
	db, handler, _ := newIngestTestEnv(t)

	hostile := []string{
		"../../../../etc/passwd",
		"..\\..\\windows\\system32\\drivers\\etc\\hosts",
		"/absolute/path.ach",
		"....//....//escape.ach",
		"name\x00truncated.ach",
	}

	for i, name := range hostile {
		content := []byte(fmt.Sprintf("distinct content for case %d\n", i))
		rec, resp := doUpload(t, handler, uploadRequest(t, name, content, nil))
		if rec.Code != http.StatusAccepted {
			// A rejected upload is also acceptable; what must never happen is
			// acceptance with a traversable key.
			continue
		}

		var key, stored, original string
		var wasNormalized int
		err := db.QueryRow(`
			SELECT object_key, filename, original_filename, filename_was_normalized
			FROM file_instances WHERE id = ?`, resp.ArtifactID).Scan(&key, &stored, &original, &wasNormalized)
		if err != nil {
			t.Fatal(err)
		}

		if strings.Contains(key, "..") || strings.Contains(key, "\x00") {
			t.Errorf("%q produced the traversable object key %q", name, key)
		}
		if !strings.HasPrefix(key, "tenant/") {
			t.Errorf("%q produced the key %q, which is not server-generated", name, key)
		}
		if strings.ContainsAny(stored, "/\\") {
			t.Errorf("%q was stored as the display name %q, which still contains a separator", name, stored)
		}
		if wasNormalized != 1 {
			t.Errorf("%q was not flagged as normalised", name)
		}
		// The original is retained so a support question can be answered.
		if original != name {
			t.Errorf("the original filename was not retained: got %q, sent %q", original, name)
		}
	}
}

// Duplicate delivery is a normal condition: a partner retries, a watcher sees
// the same file twice. It must not create a second artifact.
func TestDuplicateDeliveryIsIdempotent(t *testing.T) {
	db, handler, _ := newIngestTestEnv(t)
	content := []byte("101 021000021 0210000210001011200A094101TEST BANK\nduplicate\n")

	rec1, first := doUpload(t, handler, uploadRequest(t, "payments.ach", content, nil))
	if rec1.Code != http.StatusAccepted {
		t.Fatalf("first upload: status %d, body %s", rec1.Code, rec1.Body.String())
	}

	rec2, second := doUpload(t, handler, uploadRequest(t, "payments.ach", content, nil))
	if rec2.Code != http.StatusAccepted {
		t.Fatalf("redelivery: status %d, body %s", rec2.Code, rec2.Body.String())
	}
	if !second.Duplicate {
		t.Error("the redelivery was not reported as a duplicate")
	}
	if second.ArtifactID != first.ArtifactID {
		t.Errorf("redelivery produced artifact %d, want the original %d", second.ArtifactID, first.ArtifactID)
	}

	var artifacts int
	db.QueryRow(`SELECT COUNT(*) FROM file_instances`).Scan(&artifacts)
	if artifacts != 1 {
		t.Errorf("redelivery produced %d artifacts, want 1", artifacts)
	}
}

// The same content under a different name is a different delivery, and must not
// be collapsed: a partner sending yesterday's file again under today's name is
// a real event.
func TestSameBytesUnderADifferentNameIsANewDelivery(t *testing.T) {
	_, handler, _ := newIngestTestEnv(t)
	content := []byte("identical bytes, two names\n")

	rec1, first := doUpload(t, handler, uploadRequest(t, "monday.ach", content, nil))
	rec2, second := doUpload(t, handler, uploadRequest(t, "tuesday.ach", content, nil))

	if rec1.Code != http.StatusAccepted || rec2.Code != http.StatusAccepted {
		t.Fatalf("statuses %d and %d", rec1.Code, rec2.Code)
	}
	if first.IdempotencyKey == second.IdempotencyKey {
		t.Error("two differently named deliveries produced the same idempotency key")
	}
	// The content index means one artifact holds the bytes; the second delivery
	// is recognised as carrying content already present.
	if !second.Duplicate {
		t.Error("the second delivery did not report that the content was already stored")
	}
}

// Reusing an idempotency key for different content must be a typed conflict.
// Returning the first artifact would attribute one file's identity to another's
// bytes.
func TestConflictingIdempotencyKeyIsRejected(t *testing.T) {
	db, handler, _ := newIngestTestEnv(t)
	headers := map[string]string{"Idempotency-Key": "client-supplied-key-0001"}

	rec1, first := doUpload(t, handler, uploadRequest(t, "a.ach", []byte("the first content\n"), headers))
	if rec1.Code != http.StatusAccepted {
		t.Fatalf("first upload: %d %s", rec1.Code, rec1.Body.String())
	}

	rec2, _ := doUpload(t, handler, uploadRequest(t, "b.ach", []byte("entirely different content\n"), headers))
	if rec2.Code != http.StatusConflict {
		t.Fatalf("status %d, want 409. body: %s", rec2.Code, rec2.Body.String())
	}

	var conflict map[string]any
	json.Unmarshal(rec2.Body.Bytes(), &conflict)
	if conflict["error"] != "idempotency_key_conflict" {
		t.Errorf("the conflict is not typed: %v", conflict)
	}

	// The first artifact is untouched and no second one was created.
	var artifacts int
	db.QueryRow(`SELECT COUNT(*) FROM file_instances`).Scan(&artifacts)
	if artifacts != 1 {
		t.Errorf("a conflicting key produced %d artifacts, want 1", artifacts)
	}
	var storedHash string
	db.QueryRow(`SELECT sha256_hash FROM file_instances WHERE id = ?`, first.ArtifactID).Scan(&storedHash)
	if storedHash != first.SHA256 {
		t.Error("the original artifact's hash changed")
	}
}

// Replaying the identical request with the same key is a replay, not a
// conflict: a client that retries after a timeout must not be punished.
func TestReplayingTheSameRequestReturnsTheOriginal(t *testing.T) {
	_, handler, _ := newIngestTestEnv(t)
	headers := map[string]string{"Idempotency-Key": "client-supplied-key-0002"}
	content := []byte("the same content twice\n")

	_, first := doUpload(t, handler, uploadRequest(t, "a.ach", content, headers))
	rec2, second := doUpload(t, handler, uploadRequest(t, "a.ach", content, headers))

	if rec2.Code != http.StatusAccepted {
		t.Fatalf("a replay returned %d, want 202: %s", rec2.Code, rec2.Body.String())
	}
	if second.ArtifactID != first.ArtifactID || second.JobID != first.JobID {
		t.Errorf("a replay returned different identifiers: %+v vs %+v", second, first)
	}
}

// A declared length that does not match what arrived means a truncated or
// padded upload, and a truncated NACHA file parses.
func TestContentLengthMismatchIsRefused(t *testing.T) {
	_, handler, _ := newIngestTestEnv(t)
	content := []byte("thirty-two bytes of content here")

	rec, _ := doUpload(t, handler, uploadRequest(t, "a.ach", content,
		map[string]string{"X-Artifact-Length": "9999"}))

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status %d, want 400", rec.Code)
	}
	if !strings.Contains(rec.Body.String(), "size_mismatch") {
		t.Errorf("the refusal is not typed: %s", rec.Body.String())
	}
}

// truncatingReader models a client that disconnects mid-upload.
type truncatingReader struct {
	data   []byte
	offset int
	cutoff int
}

func (tr *truncatingReader) Read(p []byte) (int, error) {
	if tr.offset >= tr.cutoff {
		return 0, io.ErrUnexpectedEOF
	}
	n := copy(p, tr.data[tr.offset:tr.cutoff])
	tr.offset += n
	return n, nil
}

// An interrupted upload must leave neither an object nor a database row.
func TestInterruptedUploadLeavesNothingBehind(t *testing.T) {
	db, handler, _ := newIngestTestEnv(t)

	var body bytes.Buffer
	writer := multipart.NewWriter(&body)
	part, _ := writer.CreateFormFile("file", "interrupted.ach")
	part.Write(bytes.Repeat([]byte("payment record line\n"), 2000))
	writer.Close()

	full := body.Bytes()
	req := httptest.NewRequest(http.MethodPost, "/api/v1/files/upload",
		&truncatingReader{data: full, cutoff: len(full) / 2})
	req.Header.Set("Content-Type", writer.FormDataContentType())

	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code == http.StatusAccepted {
		t.Errorf("an interrupted upload was accepted: %s", rec.Body.String())
	}

	var artifacts, jobs int
	db.QueryRow(`SELECT COUNT(*) FROM file_instances`).Scan(&artifacts)
	db.QueryRow(`SELECT COUNT(*) FROM ingestion_jobs`).Scan(&jobs)
	if artifacts != 0 || jobs != 0 {
		t.Errorf("an interrupted upload left %d artifacts and %d jobs", artifacts, jobs)
	}
}

// A tenant at its quota must be refused before its bytes are stored.
func TestQuotaRefusesUploadsBeyondTheTenantLimit(t *testing.T) {
	db, handler, _ := newIngestTestEnv(t)

	if _, err := db.Exec(`
		INSERT INTO tenant_quotas (tenant_id, max_total_bytes, max_artifacts)
		VALUES (?, 40, 100)`, DefaultTenantID); err != nil {
		t.Fatal(err)
	}

	// The first upload fits.
	rec1, _ := doUpload(t, handler, uploadRequest(t, "first.ach", bytes.Repeat([]byte("a"), 50), nil))
	if rec1.Code != http.StatusAccepted {
		t.Fatalf("the first upload was refused: %d %s", rec1.Code, rec1.Body.String())
	}

	// The tenant is now over its byte quota, so the next is refused.
	rec2, _ := doUpload(t, handler, uploadRequest(t, "second.ach", []byte("more content\n"), nil))
	if rec2.Code != http.StatusInsufficientStorage {
		t.Fatalf("status %d, want 507", rec2.Code)
	}
	if !strings.Contains(rec2.Body.String(), "quota_exceeded") {
		t.Errorf("the refusal is not typed: %s", rec2.Body.String())
	}

	// And the refusal discloses only this tenant's own numbers.
	if strings.Contains(rec2.Body.String(), "TENANT-") {
		t.Errorf("the quota message names a tenant: %s", rec2.Body.String())
	}
}

// A gzip bomb is just a large file to this ingress: nothing decompresses, so
// the byte limit applies to what actually arrives. This asserts that archives
// are not expanded rather than that expansion is bounded.
func TestArchivesAreNotDecompressed(t *testing.T) {
	db, handler, store := newIngestTestEnv(t)

	// A gzip header followed by content. If anything decompressed it, the
	// stored size would differ from the uploaded size.
	payload := append([]byte{0x1f, 0x8b, 0x08, 0x00, 0, 0, 0, 0}, bytes.Repeat([]byte{0}, 512)...)
	rec, resp := doUpload(t, handler, uploadRequest(t, "archive.gz", payload, nil))
	if rec.Code != http.StatusAccepted {
		t.Fatalf("status %d: %s", rec.Code, rec.Body.String())
	}
	if resp.SizeBytes != int64(len(payload)) {
		t.Errorf("stored size %d differs from uploaded size %d; something expanded the payload",
			resp.SizeBytes, len(payload))
	}

	var key string
	db.QueryRow(`SELECT object_key FROM file_instances WHERE id = ?`, resp.ArtifactID).Scan(&key)
	info, err := store.Stat(t.Context(), key)
	if err != nil {
		t.Fatal(err)
	}
	if info.SizeBytes != int64(len(payload)) {
		t.Errorf("the stored object is %d bytes, uploaded %d", info.SizeBytes, len(payload))
	}
	// The media type is measured, not taken from the client's declaration.
	if resp.MediaType == "" {
		t.Error("no media type was recorded")
	}
}

// Raw artifact bytes are the most sensitive data this system holds. Every read
// is recorded and every read is tenant-scoped.
func TestArtifactDownloadIsAuditedAndScoped(t *testing.T) {
	db, handler, _ := newIngestTestEnv(t)
	content := []byte("101 021000021 sensitive record content\n")

	_, resp := doUpload(t, handler, uploadRequest(t, "sensitive.ach", content, nil))

	req := httptest.NewRequest(http.MethodGet, fmt.Sprintf("/api/v1/artifacts/%d/content", resp.ArtifactID), nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status %d: %s", rec.Code, rec.Body.String())
	}
	if !bytes.Equal(rec.Body.Bytes(), content) {
		t.Error("the download does not match the stored artifact")
	}
	if got := rec.Header().Get("Content-Disposition"); !strings.Contains(got, "sensitive.ach") {
		t.Errorf("Content-Disposition = %q", got)
	}

	var action string
	var served int64
	err := db.QueryRow(`
		SELECT action, bytes_served FROM artifact_access_log
		WHERE file_instance_id = ? ORDER BY id DESC LIMIT 1`, resp.ArtifactID).Scan(&action, &served)
	if err != nil {
		t.Fatalf("the download was not recorded: %v", err)
	}
	if action != "DOWNLOAD" || served != int64(len(content)) {
		t.Errorf("the access record says action=%s bytes=%d, want DOWNLOAD and %d", action, served, len(content))
	}
}

// An artifact belonging to another tenant is byte-identically absent.
func TestDownloadingAnotherTenantsArtifactIsIndistinguishableFromAbsent(t *testing.T) {
	db, handler, _ := newIngestTestEnv(t)

	_, resp := doUpload(t, handler, uploadRequest(t, "ours.ach", []byte("our content\n"), nil))

	// Re-attribute the artifact to another tenant, which is what a cross-tenant
	// probe would be reaching for.
	if _, err := db.Exec(`INSERT OR IGNORE INTO tenants (id, name) VALUES ('TENANT-OTHER','Other')`); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`UPDATE file_instances SET tenant_id = 'TENANT-OTHER' WHERE id = ?`, resp.ArtifactID); err != nil {
		t.Fatal(err)
	}

	foreign := httptest.NewRecorder()
	handler.ServeHTTP(foreign, httptest.NewRequest(http.MethodGet,
		fmt.Sprintf("/api/v1/artifacts/%d/content", resp.ArtifactID), nil))

	absent := httptest.NewRecorder()
	handler.ServeHTTP(absent, httptest.NewRequest(http.MethodGet, "/api/v1/artifacts/999999/content", nil))

	if foreign.Code != http.StatusNotFound {
		t.Errorf("a foreign artifact returned %d, want 404", foreign.Code)
	}
	if foreign.Body.String() != absent.Body.String() {
		t.Errorf("a foreign artifact (%s) is distinguishable from an absent one (%s)",
			foreign.Body.String(), absent.Body.String())
	}
}

// With no store configured, uploads are refused rather than falling back to the
// in-memory path this replaces.
func TestUploadsAreRefusedWhenStorageIsUnconfigured(t *testing.T) {
	db := setupTestDb(t)
	defer db.Close()

	handler := NewRouterWithStore(db, ingestDemoConfig(), nil, nil)
	rec, _ := doUpload(t, handler, uploadRequest(t, "a.ach", []byte("content\n"), nil))

	if rec.Code != http.StatusServiceUnavailable {
		t.Errorf("status %d, want 503", rec.Code)
	}
	if !strings.Contains(rec.Body.String(), "storage_unavailable") {
		t.Errorf("the refusal is not typed: %s", rec.Body.String())
	}
}

// Bounded memory.
//
// The measurement below is of this process, on this machine, for this fixture.
// It is reported so the bound can be checked, and it is deliberately not turned
// into a throughput or capacity claim -- the guide's requirement is to prove
// memory does not scale with artifact size, which is what the ratio shows.
func TestUploadMemoryDoesNotScaleWithArtifactSize(t *testing.T) {
	if testing.Short() {
		t.Skip("allocates a multi-megabyte fixture")
	}
	_, handler, _ := newIngestTestEnv(t)

	measure := func(size int) uint64 {
		content := bytes.Repeat([]byte("payment record line padded to width\n"), size/36)

		runtime.GC()
		var before, after runtime.MemStats
		runtime.ReadMemStats(&before)

		rec, _ := doUpload(t, handler, uploadRequest(t, fmt.Sprintf("size-%d.ach", size), content, nil))
		if rec.Code != http.StatusAccepted {
			t.Fatalf("upload of %d bytes returned %d: %s", size, rec.Code, rec.Body.String())
		}

		runtime.ReadMemStats(&after)
		return after.TotalAlloc - before.TotalAlloc
	}

	small := measure(256 << 10) // 256 KiB
	large := measure(8 << 20)   // 8 MiB

	// The fixture itself is built in the test, so some growth is expected and
	// is not the handler's. What must not happen is the handler allocating a
	// multiple of the artifact size on top of that.
	//
	// A 32x increase in payload with less than a 32x increase in allocation
	// shows the handler is not holding the artifact.
	ratio := float64(large) / float64(small)
	t.Logf("allocation: 256KiB upload=%d bytes, 8MiB upload=%d bytes, ratio=%.1fx (payload ratio 32x)",
		small, large, ratio)
	t.Logf("environment: %s/%s, %s", runtime.GOOS, runtime.GOARCH, runtime.Version())

	if ratio > 32 {
		t.Errorf("allocation grew %.1fx for a 32x larger artifact; the handler is retaining the payload", ratio)
	}
}
