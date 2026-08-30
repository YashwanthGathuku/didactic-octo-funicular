-- 004_artifact_storage.sql -- PostgreSQL dialect
-- Immutable artifact storage, quotas, idempotency, and access logging.

ALTER TABLE file_instances ADD COLUMN IF NOT EXISTS object_key TEXT;
ALTER TABLE file_instances ADD COLUMN IF NOT EXISTS original_filename TEXT;
ALTER TABLE file_instances ADD COLUMN IF NOT EXISTS filename_was_normalized BOOLEAN NOT NULL DEFAULT FALSE;
ALTER TABLE file_instances ADD COLUMN IF NOT EXISTS media_type TEXT;

CREATE INDEX IF NOT EXISTS idx_file_instances_object_key
    ON file_instances(tenant_id, object_key);

CREATE UNIQUE INDEX IF NOT EXISTS uq_file_instances_content
    ON file_instances(tenant_id, sha256_hash, size_bytes);

CREATE TABLE IF NOT EXISTS tenant_quotas (
    tenant_id       TEXT PRIMARY KEY REFERENCES tenants(id),
    max_total_bytes BIGINT NOT NULL CHECK (max_total_bytes > 0),
    max_artifacts   BIGINT NOT NULL CHECK (max_artifacts > 0),
    updated_at      TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE IF NOT EXISTS ingest_idempotency (
    tenant_id        TEXT NOT NULL REFERENCES tenants(id),
    idempotency_key  TEXT NOT NULL,
    fingerprint      TEXT NOT NULL,
    file_instance_id BIGINT NOT NULL REFERENCES file_instances(id),
    job_id           BIGINT REFERENCES ingestion_jobs(id),
    created_at       TIMESTAMPTZ NOT NULL DEFAULT now(),
    PRIMARY KEY (tenant_id, idempotency_key)
);

CREATE TABLE IF NOT EXISTS artifact_access_log (
    id               BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    tenant_id        TEXT NOT NULL REFERENCES tenants(id),
    file_instance_id BIGINT NOT NULL REFERENCES file_instances(id),
    actor_id         TEXT NOT NULL,
    action           TEXT NOT NULL CHECK (action IN ('DOWNLOAD','DOWNLOAD_DENIED')),
    bytes_served     BIGINT NOT NULL DEFAULT 0 CHECK (bytes_served >= 0),
    occurred_at      TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX IF NOT EXISTS idx_artifact_access_subject
    ON artifact_access_log(tenant_id, file_instance_id, id);

ALTER TABLE tenant_quotas       ENABLE ROW LEVEL SECURITY;
ALTER TABLE ingest_idempotency  ENABLE ROW LEVEL SECURITY;
ALTER TABLE artifact_access_log ENABLE ROW LEVEL SECURITY;

ALTER TABLE tenant_quotas       FORCE ROW LEVEL SECURITY;
ALTER TABLE ingest_idempotency  FORCE ROW LEVEL SECURITY;
ALTER TABLE artifact_access_log FORCE ROW LEVEL SECURITY;

DROP POLICY IF EXISTS tenant_isolation_tenant_quotas ON tenant_quotas;
CREATE POLICY tenant_isolation_tenant_quotas ON tenant_quotas
    USING (tenant_id = current_setting('sentinel.tenant_id', true))
    WITH CHECK (tenant_id = current_setting('sentinel.tenant_id', true));

DROP POLICY IF EXISTS tenant_isolation_ingest_idempotency ON ingest_idempotency;
CREATE POLICY tenant_isolation_ingest_idempotency ON ingest_idempotency
    USING (tenant_id = current_setting('sentinel.tenant_id', true))
    WITH CHECK (tenant_id = current_setting('sentinel.tenant_id', true));

DROP POLICY IF EXISTS tenant_isolation_artifact_access_log ON artifact_access_log;
CREATE POLICY tenant_isolation_artifact_access_log ON artifact_access_log
    USING (tenant_id = current_setting('sentinel.tenant_id', true))
    WITH CHECK (tenant_id = current_setting('sentinel.tenant_id', true));

CREATE OR REPLACE FUNCTION ingest_idempotency_append_only() RETURNS TRIGGER AS $$
BEGIN
    RAISE EXCEPTION 'ingest_idempotency is append-only';
END;
$$ LANGUAGE plpgsql;

DROP TRIGGER IF EXISTS ingest_idempotency_no_update ON ingest_idempotency;
CREATE TRIGGER ingest_idempotency_no_update
    BEFORE UPDATE ON ingest_idempotency
    FOR EACH ROW EXECUTE FUNCTION ingest_idempotency_append_only();

CREATE OR REPLACE FUNCTION artifact_access_log_append_only() RETURNS TRIGGER AS $$
BEGIN
    RAISE EXCEPTION 'artifact_access_log is append-only';
END;
$$ LANGUAGE plpgsql;

DROP TRIGGER IF EXISTS artifact_access_log_no_change ON artifact_access_log;
CREATE TRIGGER artifact_access_log_no_change
    BEFORE UPDATE OR DELETE ON artifact_access_log
    FOR EACH ROW EXECUTE FUNCTION artifact_access_log_append_only();
