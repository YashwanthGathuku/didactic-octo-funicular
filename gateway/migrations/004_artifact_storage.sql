-- Immutable artifact storage and safe ingress.
--
-- Before this migration, `storage_path` held the literal string
-- "/s3/incoming/" concatenated with the client's filename, no object was ever
-- written anywhere, and the file body lived in a database column. Three
-- separate problems: the path was derived from client input, the artifact did
-- not exist, and the bytes were in the wrong store.

-- The object key is server-generated and is the authoritative location of the
-- artifact. `storage_path` is retained for the rows that predate this and is no
-- longer written by the ingest path.
ALTER TABLE file_instances ADD COLUMN object_key TEXT;

-- The client's filename as received, and the sanitised form actually used for
-- display. Keeping both means a support question about "the file I uploaded"
-- can be answered, while nothing renders the raw value.
ALTER TABLE file_instances ADD COLUMN original_filename TEXT;

-- Whether normalisation changed the name. A true here is worth looking at: a
-- legitimate client does not send control characters or path separators.
ALTER TABLE file_instances ADD COLUMN filename_was_normalized INTEGER NOT NULL DEFAULT 0;

-- Sniffed from the leading bytes at ingest, not from the client's declared
-- Content-Type, which is a claim rather than a measurement.
ALTER TABLE file_instances ADD COLUMN media_type TEXT;

-- derived_from_id already exists from migration 002: a repair creates a new row
-- pointing at its source, and the source is never modified.

CREATE INDEX IF NOT EXISTS idx_file_instances_object_key
    ON file_instances(tenant_id, object_key);

-- One artifact per (tenant, content hash, size). Duplicate delivery of
-- identical content is a normal condition -- a partner retries, a watcher sees
-- the same file twice -- and must not create a second artifact.
CREATE UNIQUE INDEX IF NOT EXISTS uq_file_instances_content
    ON file_instances(tenant_id, sha256_hash, size_bytes);

-- Per-tenant ingest quota.
--
-- A quota is a control, not a billing feature: without one, any authenticated
-- tenant can exhaust shared storage for every other tenant. Absent a row, the
-- defaults in code apply; a row makes the limit explicit and auditable.
CREATE TABLE IF NOT EXISTS tenant_quotas (
    tenant_id       TEXT PRIMARY KEY REFERENCES tenants(id),
    max_total_bytes INTEGER NOT NULL CHECK (max_total_bytes > 0),
    max_artifacts   INTEGER NOT NULL CHECK (max_artifacts > 0),
    updated_at      TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP
);

-- Idempotency records for ingest.
--
-- The fingerprint is what makes a conflict detectable: the same key presented
-- with different content is a client bug or an attack, and returning the first
-- artifact for it would attribute one file's identity to another's bytes.
CREATE TABLE IF NOT EXISTS ingest_idempotency (
    tenant_id        TEXT NOT NULL REFERENCES tenants(id),
    idempotency_key  TEXT NOT NULL,
    fingerprint      TEXT NOT NULL,
    file_instance_id INTEGER NOT NULL REFERENCES file_instances(id),
    job_id           INTEGER REFERENCES ingestion_jobs(id),
    created_at       TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    PRIMARY KEY (tenant_id, idempotency_key)
);

-- An idempotency record is a statement about what happened. Rewriting one would
-- let a second upload claim the first one's identity.
CREATE TRIGGER IF NOT EXISTS ingest_idempotency_no_update
BEFORE UPDATE ON ingest_idempotency
BEGIN
    SELECT RAISE(ABORT, 'ingest_idempotency is append-only');
END;

-- Every read of raw artifact bytes is recorded.
--
-- Raw financial file content is the most sensitive data this system holds. An
-- access log that exists only in an HTTP server's file is not evidence; this
-- table is queryable, tenant-scoped and append-only.
CREATE TABLE IF NOT EXISTS artifact_access_log (
    id               INTEGER PRIMARY KEY AUTOINCREMENT,
    tenant_id        TEXT NOT NULL REFERENCES tenants(id),
    file_instance_id INTEGER NOT NULL REFERENCES file_instances(id),
    actor_id         TEXT NOT NULL,
    action           TEXT NOT NULL CHECK (action IN ('DOWNLOAD','DOWNLOAD_DENIED')),
    bytes_served     INTEGER NOT NULL DEFAULT 0 CHECK (bytes_served >= 0),
    occurred_at      TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP
);

CREATE INDEX IF NOT EXISTS idx_artifact_access_subject
    ON artifact_access_log(tenant_id, file_instance_id, id);

CREATE TRIGGER IF NOT EXISTS artifact_access_log_no_update
BEFORE UPDATE ON artifact_access_log
BEGIN
    SELECT RAISE(ABORT, 'artifact_access_log is append-only');
END;

CREATE TRIGGER IF NOT EXISTS artifact_access_log_no_delete
BEFORE DELETE ON artifact_access_log
BEGIN
    SELECT RAISE(ABORT, 'artifact_access_log is append-only');
END;
