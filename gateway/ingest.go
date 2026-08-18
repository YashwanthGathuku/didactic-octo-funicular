package main

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"mime"
	"mime/multipart"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"

	"sentinel-gateway/internal/auth"
	"sentinel-gateway/internal/objectstore"
	"sentinel-gateway/internal/schedule"
)

// Safe ingress: an authenticated upload that streams to immutable storage.
//
// The path this replaces read the entire body into memory with io.ReadAll,
// stored the bytes in a database column, derived a storage path from the
// client's filename, and returned a full validation result synchronously. Each
// of those is addressed here, and the ordering below is deliberate: authorize,
// then bound, then measure, then store, then record, then enqueue. Nothing
// about the artifact is persisted until the artifact exists.

const (
	// MaxArtifactBytes bounds a single upload.
	//
	// A NACHA file is 94 bytes per record; 64 MiB is roughly 700,000 records,
	// far beyond any realistic single delivery. The number matters less than
	// the fact that there is one -- the previous handler had no limit on
	// ingest-raw at all and a 20 MiB form-parse limit on upload that silently
	// buffered to disk.
	MaxArtifactBytes int64 = 64 << 20

	// DefaultTenantQuotaBytes applies when a tenant has no explicit quota row.
	DefaultTenantQuotaBytes int64 = 10 << 30

	// DefaultTenantQuotaArtifacts bounds row count as well as bytes: a tenant
	// uploading a billion one-byte files exhausts the database rather than the
	// object store, and a byte quota alone would not notice.
	DefaultTenantQuotaArtifacts int64 = 1_000_000

	// maxMultipartHeaderBytes bounds the part headers, not the payload. A
	// client can otherwise send an unbounded stream of header fields before any
	// content arrives.
	maxMultipartHeaderBytes int64 = 32 << 10
)

// ingestConflictError distinguishes the two conflicting cases a caller must be
// able to tell apart.
type ingestConflict struct {
	Code    string
	Message string
	Detail  map[string]any
}

func (e *ingestConflict) Error() string { return e.Message }

// AcceptedResponse is returned by a successful upload.
//
// It carries identifiers and the measured facts, and no validation verdict:
// the API returns 202 and does not wait for validation, so any status here
// beyond "received" would be a claim about work that has not happened.
type AcceptedResponse struct {
	ArtifactID int64  `json:"artifactId"`
	JobID      int64  `json:"jobId"`
	Status     string `json:"status"`
	SHA256     string `json:"sha256"`
	SizeBytes  int64  `json:"sizeBytes"`
	MediaType  string `json:"mediaType"`
	Filename   string `json:"filename"`

	// FilenameNormalized reports that the stored name differs from the one
	// sent. A client seeing this should look at what it sent.
	FilenameNormalized bool `json:"filenameNormalized,omitempty"`

	// Duplicate reports that this content was already present and no new
	// artifact was created. The identifiers are the original's.
	Duplicate bool `json:"duplicate,omitempty"`

	IdempotencyKey string `json:"idempotencyKey"`

	// ExpectationMatch reports whether this arrival satisfied something that
	// was expected: ATTRIBUTED, AMBIGUOUS, DUPLICATE or UNEXPECTED.
	//
	// It is returned because it changes what the uploader should expect. An
	// AMBIGUOUS or DUPLICATE arrival clears nobody's missing-file alert until a
	// human resolves it, and a client that assumed otherwise would report a
	// delivery that the gateway does not consider made.
	ExpectationMatch  string `json:"expectationMatch,omitempty"`
	ExpectationID     int64  `json:"expectationId,omitempty"`
	ExpectationDetail string `json:"expectationDetail,omitempty"`
}

// ingestUpload is the handler for POST /api/v1/files/upload.
func ingestUpload(db *sql.DB, store objectstore.ObjectStore) http.HandlerFunc {
	// Built once per handler rather than per request. A failure here means the
	// driver name is wrong, which is a programming error, not a runtime
	// condition -- so it is logged and matching is disabled rather than
	// failing every upload.
	scheduler, serr := schedulerFor(db)
	if serr != nil {
		log.Printf("ingest: expectation matching disabled: %v", serr)
		scheduler = nil
	}
	return func(w http.ResponseWriter, r *http.Request) {
		scope, serr := resolveScope(r, auth.PermUploadArtifact)
		if serr != nil {
			serr.write(w)
			return
		}
		tenantID := scope.TenantID()

		// Bound the request body before anything reads from it. MaxBytesReader
		// makes the limit the server's, not the client's Content-Length claim,
		// so a lying header cannot buy extra bytes.
		//
		// The slack above MaxArtifactBytes covers multipart framing; the
		// payload itself is bounded separately and exactly, inside Put.
		r.Body = http.MaxBytesReader(w, r.Body, MaxArtifactBytes+maxMultipartHeaderBytes)

		part, declaredName, err := openUploadPart(r)
		if err != nil {
			writeIngestError(w, http.StatusBadRequest, "invalid_upload", err.Error())
			return
		}
		defer part.Close()

		filename, normalized := objectstore.NormalizeFilename(declaredName)

		// Quota is checked before the bytes are accepted. Checking afterwards
		// would mean storing an object in order to discover it is not allowed.
		if err := checkTenantQuota(r.Context(), db, tenantID); err != nil {
			writeIngestError(w, http.StatusInsufficientStorage, "quota_exceeded", err.Error())
			return
		}

		uploadStart := time.Now()
		key, err := objectstore.NewKey(tenantID, uploadStart)
		if err != nil {
			writeIngestError(w, http.StatusInternalServerError, "internal_error", "could not allocate an object key")
			return
		}

		putStart := time.Now()
		put, err := store.Put(r.Context(), key, part, MaxArtifactBytes)
		RecordDependencyOperation("object_store", "put", time.Since(putStart).Seconds(), err)
		if err != nil {
			writeStoreError(w, err)
			return
		}

		// The declared length, when the client sent one, must match what
		// arrived. A mismatch means a truncated or padded upload, and a
		// truncated NACHA file parses.
		if declared := declaredLength(r); declared > 0 && declared != put.SizeBytes {
			writeIngestError(w, http.StatusBadRequest, "size_mismatch",
				fmt.Sprintf("declared %d bytes, received %d", declared, put.SizeBytes))
			return
		}

		fingerprint := contentFingerprint(tenantID, filename, put.SizeBytes, put.SHA256)
		idempotencyKey := strings.TrimSpace(r.Header.Get("Idempotency-Key"))
		clientSupplied := idempotencyKey != ""
		if !clientSupplied {
			// Absent a client key, the content itself is the key, so redelivery
			// of identical content is idempotent by default.
			idempotencyKey = fingerprint
		}

		resp, conflict, err := recordIngest(r.Context(), db, ingestRecord{
			TenantID:       tenantID,
			ActorID:        scope.ActorID(),
			Filename:       filename,
			OriginalName:   declaredName,
			Normalized:     normalized,
			Put:            put,
			Fingerprint:    fingerprint,
			IdempotencyKey: idempotencyKey,
			ClientSupplied: clientSupplied,
			Scheduler:      scheduler,
		})
		if conflict != nil {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusConflict)
			_ = json.NewEncoder(w).Encode(map[string]any{
				"error": conflict.Code,
				// "detail" is the human-readable sentence everywhere in this
				// API; the structured facts about the colliding artifact move
				// under "conflict" so the two do not share a key.
				"detail":   conflict.Message,
				"conflict": conflict.Detail,
			})
			return
		}
		if err != nil {
			log.Printf("ingest: recording artifact failed for tenant %s: %v", tenantID, err)
			writeIngestError(w, http.StatusInternalServerError, "internal_error", "could not record the artifact")
			return
		}

		metricArrivalToJobVisible.Observe(time.Since(uploadStart).Seconds())
		GlobalMetrics.RecordFileIngested("RECEIVED", int(put.SizeBytes))

		w.Header().Set("Content-Type", "application/json")
		// 202: the artifact is stored and validation is queued. It has not run.
		w.WriteHeader(http.StatusAccepted)
		_ = json.NewEncoder(w).Encode(resp)
	}
}

// openUploadPart returns the file part without buffering the payload.
//
// r.ParseMultipartForm is deliberately not used: it reads every part into
// memory or a temporary file up to its limit before the handler sees anything,
// which is the behaviour this replaces. multipart.Reader hands back a streaming
// part instead.
func openUploadPart(r *http.Request) (io.ReadCloser, string, error) {
	contentType := r.Header.Get("Content-Type")
	mediaType, params, err := mime.ParseMediaType(contentType)
	if err != nil {
		return nil, "", fmt.Errorf("unreadable Content-Type")
	}
	if !strings.HasPrefix(mediaType, "multipart/") {
		return nil, "", fmt.Errorf("expected a multipart upload, got %q", mediaType)
	}
	boundary, ok := params["boundary"]
	if !ok {
		return nil, "", fmt.Errorf("multipart upload has no boundary")
	}

	reader := multipart.NewReader(r.Body, boundary)
	for {
		part, err := reader.NextPart()
		if errors.Is(err, io.EOF) {
			return nil, "", fmt.Errorf("no 'file' part in the upload")
		}
		if err != nil {
			return nil, "", fmt.Errorf("malformed multipart upload")
		}
		if part.FormName() == "file" {
			return part, declaredFilename(part), nil
		}
		part.Close()
	}
}

// declaredFilename recovers the filename the client actually sent.
//
// multipart.Part.FileName() passes the value through filepath.Base before
// returning it, so a client sending "../../etc/passwd" reaches the handler as
// "passwd". That is a sound default and it means path traversal is already
// defeated twice over -- but it also means the audit trail would record a name
// the client never sent, and "the client attempted a traversal" is exactly the
// sort of thing an investigation needs.
//
// This reads the raw Content-Disposition parameter instead. The value is
// treated as hostile and is normalised before anything uses it; only the
// unmodified form is stored, and only as a record of what arrived.
func declaredFilename(part *multipart.Part) string {
	raw := part.Header.Get("Content-Disposition")
	if raw == "" {
		return part.FileName()
	}
	_, params, err := mime.ParseMediaType(raw)
	if err != nil {
		return part.FileName()
	}
	if name, ok := params["filename"]; ok && name != "" {
		// Bounded: this string is stored and appears in logs and exports.
		if len(name) > 1024 {
			name = name[:1024]
		}
		return name
	}
	return part.FileName()
}

// declaredLength reads a client-supplied length, when one was sent.
//
// It is used only to detect a mismatch. It never bounds anything: a limit
// derived from a value the client controls is not a limit.
func declaredLength(r *http.Request) int64 {
	raw := strings.TrimSpace(r.Header.Get("X-Artifact-Length"))
	if raw == "" {
		return 0
	}
	var n int64
	if _, err := fmt.Sscanf(raw, "%d", &n); err != nil || n < 0 {
		return 0
	}
	return n
}

// contentFingerprint identifies an artifact by what it is, not by what it is
// called.
//
// Tenant, name, size and content hash, as the guide requires. The name is
// included because the same bytes delivered under two names are two deliveries
// -- a partner sending yesterday's file again under today's name is a real
// event that must not be silently collapsed.
func contentFingerprint(tenantID, filename string, size int64, sha256Hex string) string {
	h := sha256.New()
	fmt.Fprintf(h, "sentinel/ingest-fingerprint/v1\x00%s\x00%s\x00%d\x00%s", tenantID, filename, size, sha256Hex)
	return hex.EncodeToString(h.Sum(nil))
}

type ingestRecord struct {
	TenantID       string
	ActorID        string
	Filename       string
	OriginalName   string
	Normalized     bool
	Put            objectstore.PutResult
	Fingerprint    string
	IdempotencyKey string
	ClientSupplied bool

	// Scheduler attributes the arrival to an expectation inside the same
	// transaction. Nil disables matching, which is what a deployment with no
	// contracts configured amounts to.
	Scheduler *schedule.Store
}

// recordIngest persists the artifact and enqueues validation in one
// transaction.
//
// Both or neither. An artifact with no job is a file that arrived and will
// never be looked at; a job with no artifact is a worker that will fail
// forever. The single transaction is what makes "persist source metadata and
// enqueue validation in the same reliable workflow" true rather than intended.
func recordIngest(ctx context.Context, db *sql.DB, rec ingestRecord) (*AcceptedResponse, *ingestConflict, error) {
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return nil, nil, err
	}
	defer func() { _ = tx.Rollback() }()

	// 1. A previously seen idempotency key.
	var priorFingerprint string
	var priorArtifact int64
	var priorJob sql.NullInt64
	err = tx.QueryRowContext(ctx, `
		SELECT fingerprint, file_instance_id, job_id
		FROM ingest_idempotency
		WHERE tenant_id = ? AND idempotency_key = ?`,
		rec.TenantID, rec.IdempotencyKey).Scan(&priorFingerprint, &priorArtifact, &priorJob)

	switch {
	case err == nil && priorFingerprint == rec.Fingerprint:
		// A replay of the same request. Return the original identifiers.
		return &AcceptedResponse{
			ArtifactID: priorArtifact, JobID: priorJob.Int64, Status: "RECEIVED",
			SHA256: rec.Put.SHA256, SizeBytes: rec.Put.SizeBytes, MediaType: rec.Put.MediaType,
			Filename: rec.Filename, Duplicate: true, IdempotencyKey: rec.IdempotencyKey,
		}, nil, nil

	case err == nil:
		// The same key with different content. Returning the first artifact
		// would attribute one file's identity to another's bytes.
		return nil, &ingestConflict{
			Code:    "idempotency_key_conflict",
			Message: "this Idempotency-Key was already used for different content",
			Detail: map[string]any{
				"idempotencyKey":     rec.IdempotencyKey,
				"existingArtifactId": priorArtifact,
			},
		}, nil

	case !errors.Is(err, sql.ErrNoRows):
		return nil, nil, err
	}

	// 2. The same content, delivered again without the same key. This is the
	// ordinary retry: the artifact already exists and no second one is made.
	var existingID int64
	err = tx.QueryRowContext(ctx, `
		SELECT id FROM file_instances
		WHERE tenant_id = ? AND sha256_hash = ? AND size_bytes = ?`,
		rec.TenantID, rec.Put.SHA256, rec.Put.SizeBytes).Scan(&existingID)
	if err == nil {
		var jobID sql.NullInt64
		_ = tx.QueryRowContext(ctx,
			`SELECT id FROM ingestion_jobs WHERE tenant_id = ? AND file_instance_id = ?`,
			rec.TenantID, existingID).Scan(&jobID)

		if _, err := tx.ExecContext(ctx, `
			INSERT INTO ingest_idempotency (tenant_id, idempotency_key, fingerprint, file_instance_id, job_id)
			VALUES (?, ?, ?, ?, ?)`,
			rec.TenantID, rec.IdempotencyKey, rec.Fingerprint, existingID, jobID); err != nil {
			return nil, nil, err
		}
		if err := tx.Commit(); err != nil {
			return nil, nil, err
		}
		return &AcceptedResponse{
			ArtifactID: existingID, JobID: jobID.Int64, Status: "RECEIVED",
			SHA256: rec.Put.SHA256, SizeBytes: rec.Put.SizeBytes, MediaType: rec.Put.MediaType,
			Filename: rec.Filename, Duplicate: true, IdempotencyKey: rec.IdempotencyKey,
		}, nil, nil
	}
	if !errors.Is(err, sql.ErrNoRows) {
		return nil, nil, err
	}

	// 3. A new artifact. It begins RECEIVED -- untrusted and unreleased.
	now := time.Now().UTC().Format(time.RFC3339)
	res, err := tx.ExecContext(ctx, `
		INSERT INTO file_instances
			(tenant_id, expectation_id, filename, storage_path, object_key, original_filename,
			 filename_was_normalized, media_type, size_bytes, sha256_hash, status, received_at, updated_at)
		VALUES (?, NULL, ?, '', ?, ?, ?, ?, ?, ?, 'RECEIVED', ?, ?)`,
		rec.TenantID, rec.Filename, rec.Put.Key, rec.OriginalName,
		boolToInt(rec.Normalized), rec.Put.MediaType, rec.Put.SizeBytes, rec.Put.SHA256, now, now)
	if err != nil {
		return nil, nil, err
	}
	artifactID, err := res.LastInsertId()
	if err != nil {
		return nil, nil, err
	}

	jobRes, err := tx.ExecContext(ctx, `
		INSERT INTO ingestion_jobs (tenant_id, file_instance_id, idempotency_key, state)
		VALUES (?, ?, ?, 'QUEUED')`,
		rec.TenantID, artifactID, rec.IdempotencyKey)
	if err != nil {
		return nil, nil, err
	}
	jobID, _ := jobRes.LastInsertId()

	if _, err := tx.ExecContext(ctx, `
		INSERT INTO ingest_idempotency (tenant_id, idempotency_key, fingerprint, file_instance_id, job_id)
		VALUES (?, ?, ?, ?, ?)`,
		rec.TenantID, rec.IdempotencyKey, rec.Fingerprint, artifactID, jobID); err != nil {
		return nil, nil, err
	}

	if _, err := tx.ExecContext(ctx, `
		INSERT INTO status_history (tenant_id, object_type, object_id, from_state, to_state, actor_id, reason)
		VALUES (?, 'artifact', ?, 'NONE', 'RECEIVED', ?, 'upload accepted')`,
		rec.TenantID, artifactID, rec.ActorID); err != nil {
		return nil, nil, err
	}

	// Attribute the arrival to whatever was expecting it, in this same
	// transaction. Doing it afterwards would allow a crash in between, leaving
	// a file that arrived and an expectation saying it did not -- which reads,
	// from every report, as a missing file.
	match := schedule.MatchResult{Outcome: schedule.MatchUnexpected}
	if rec.Scheduler != nil {
		match = matchArrivalToExpectation(ctx, tx, rec.Scheduler, rec.TenantID, artifactID, rec.Filename)
	}

	if err := tx.Commit(); err != nil {
		return nil, nil, err
	}

	return &AcceptedResponse{
		ArtifactID: artifactID, JobID: jobID, Status: "RECEIVED",
		SHA256: rec.Put.SHA256, SizeBytes: rec.Put.SizeBytes, MediaType: rec.Put.MediaType,
		Filename: rec.Filename, FilenameNormalized: rec.Normalized,
		IdempotencyKey: rec.IdempotencyKey,
		// Reported to the uploader because it changes what they should expect:
		// an AMBIGUOUS or DUPLICATE arrival is not going to clear anyone's
		// missing-file alert until a human resolves it.
		ExpectationMatch:  string(match.Outcome),
		ExpectationID:     match.Occurrence,
		ExpectationDetail: match.Reason,
	}, nil, nil
}

func boolToInt(b bool) int {
	if b {
		return 1
	}
	return 0
}

// checkTenantQuota refuses an upload from a tenant at its limit.
func checkTenantQuota(ctx context.Context, db *sql.DB, tenantID string) error {
	maxBytes, maxArtifacts := DefaultTenantQuotaBytes, DefaultTenantQuotaArtifacts
	var b, a int64
	if err := db.QueryRowContext(ctx,
		`SELECT max_total_bytes, max_artifacts FROM tenant_quotas WHERE tenant_id = ?`,
		tenantID).Scan(&b, &a); err == nil {
		maxBytes, maxArtifacts = b, a
	} else if !errors.Is(err, sql.ErrNoRows) {
		return fmt.Errorf("quota lookup failed")
	}

	var usedBytes, usedCount sql.NullInt64
	if err := db.QueryRowContext(ctx,
		`SELECT COALESCE(SUM(size_bytes),0), COUNT(*) FROM file_instances WHERE tenant_id = ?`,
		tenantID).Scan(&usedBytes, &usedCount); err != nil {
		return fmt.Errorf("quota lookup failed")
	}

	// The check is per-tenant and the message reports only this tenant's own
	// numbers: another tenant's usage is not disclosed, and neither is the
	// shared total.
	if usedBytes.Int64 >= maxBytes {
		return fmt.Errorf("tenant storage quota reached: %d of %d bytes used", usedBytes.Int64, maxBytes)
	}
	if usedCount.Int64 >= maxArtifacts {
		return fmt.Errorf("tenant artifact quota reached: %d of %d artifacts", usedCount.Int64, maxArtifacts)
	}
	return nil
}

// serveArtifactContent streams raw artifact bytes through an audited proxy.
//
// A short-lived signed URL was the alternative. A streaming proxy is chosen
// because it keeps the object store unreachable from the internet, and because
// every byte served is attributable to a verified actor -- a signed URL, once
// issued, is a bearer token that can be forwarded, and the store cannot tell
// who eventually used it.
func serveArtifactContent(db *sql.DB, store objectstore.ObjectStore) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		// Reading raw financial content requires evidence access, which is a
		// higher bar than listing artifacts.
		scope, serr := resolveScope(r, auth.PermReadEvidence)
		if serr != nil {
			serr.write(w)
			return
		}

		artifactID, perr := artifactIDFromPath(r)
		if perr != nil {
			writeIngestError(w, http.StatusBadRequest, "invalid_id", "artifact id must be a positive integer")
			return
		}
		var objectKey sql.NullString
		var filename string
		var size int64
		err := db.QueryRowContext(r.Context(), `
			SELECT object_key, filename, size_bytes FROM file_instances
			WHERE tenant_id = ? AND id = ?`,
			scope.TenantID(), artifactID).Scan(&objectKey, &filename, &size)

		if errors.Is(err, sql.ErrNoRows) {
			// Byte-identical to a nonexistent id: a cross-tenant probe must not
			// confirm that an artifact exists elsewhere.
			writeIngestError(w, http.StatusNotFound, "not_found", "artifact not found")
			return
		}
		if err != nil {
			writeIngestError(w, http.StatusInternalServerError, "internal_error", "could not read the artifact record")
			return
		}
		if !objectKey.Valid || objectKey.String == "" {
			// Rows that predate this ingest path have no object. Say so rather
			// than serving an empty body that looks like an empty file.
			writeIngestError(w, http.StatusGone, "no_stored_object",
				"this artifact predates immutable storage and its bytes were never retained")
			return
		}

		body, err := store.Get(r.Context(), objectKey.String)
		if err != nil {
			recordArtifactAccess(r.Context(), db, scope.TenantID(), artifactID, scope.ActorID(), "DOWNLOAD_DENIED", 0)
			writeIngestError(w, http.StatusNotFound, "not_found", "artifact not found")
			return
		}
		defer body.Close()

		w.Header().Set("Content-Type", "application/octet-stream")
		// The filename is already normalised, and it is quoted so a name
		// containing a quote cannot break out of the header.
		w.Header().Set("Content-Disposition",
			fmt.Sprintf("attachment; filename=%q", strings.ReplaceAll(filename, `"`, "")))
		w.Header().Set("X-Content-Type-Options", "nosniff")

		served, copyErr := io.Copy(w, body)
		recordArtifactAccess(r.Context(), db, scope.TenantID(), artifactID, scope.ActorID(), "DOWNLOAD", served)
		if copyErr != nil {
			// The status is already written; the truncated response is what the
			// client sees, and the access record shows how much was served.
			log.Printf("artifact %d: download interrupted after %d bytes", artifactID, served)
		}
	}
}

// recordArtifactAccess writes the audit row. A failure to record is logged and
// does not fail the request: the bytes have already been served, and pretending
// otherwise would be a second lie on top of a missing record.
func recordArtifactAccess(ctx context.Context, db *sql.DB, tenantID string, artifactID int64, actor, action string, bytes int64) {
	if _, err := db.ExecContext(ctx, `
		INSERT INTO artifact_access_log (tenant_id, file_instance_id, actor_id, action, bytes_served)
		VALUES (?, ?, ?, ?, ?)`, tenantID, artifactID, actor, action, bytes); err != nil {
		log.Printf("artifact access log write failed for artifact %d: %v", artifactID, err)
	}
}

// artifactIDFromPath parses the path parameter, refusing anything that is not
// a positive integer rather than letting a parse failure become id 0.
func artifactIDFromPath(r *http.Request) (int64, error) {
	raw := chi.URLParam(r, "id")
	id, err := strconv.ParseInt(raw, 10, 64)
	if err != nil || id <= 0 {
		return 0, fmt.Errorf("invalid artifact id")
	}
	return id, nil
}

func writeIngestError(w http.ResponseWriter, status int, code, message string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(map[string]string{"error": code, "detail": message})
}

// writeStoreError maps a storage failure to a status, keeping each refusal
// distinguishable to the client that caused it.
func writeStoreError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, objectstore.ErrEmpty):
		writeIngestError(w, http.StatusBadRequest, "empty_artifact",
			"the upload contained no bytes; an empty file is never accepted")
	case errors.Is(err, objectstore.ErrTooLarge):
		writeIngestError(w, http.StatusRequestEntityTooLarge, "artifact_too_large",
			fmt.Sprintf("the upload exceeds the %d byte limit", MaxArtifactBytes))
	case errors.Is(err, objectstore.ErrObjectExists):
		// Keys are server-generated from 128 random bits, so this is a bug
		// rather than a client condition.
		writeIngestError(w, http.StatusInternalServerError, "internal_error", "object key collision")
	default:
		writeIngestError(w, http.StatusBadRequest, "upload_failed",
			"the upload did not complete; nothing was stored")
	}
}
