-- 006_jobs_and_outbox.sql -- PostgreSQL dialect
-- Durable jobs, quotas, and transactional outbox.

ALTER TABLE ingestion_jobs ADD COLUMN IF NOT EXISTS run_after TIMESTAMPTZ;
ALTER TABLE ingestion_jobs ADD COLUMN IF NOT EXISTS last_heartbeat_at TIMESTAMPTZ;
ALTER TABLE ingestion_jobs ADD COLUMN IF NOT EXISTS kind TEXT NOT NULL DEFAULT 'VALIDATE_ARTIFACT';
ALTER TABLE ingestion_jobs ADD COLUMN IF NOT EXISTS terminal_reason TEXT;

CREATE INDEX IF NOT EXISTS idx_jobs_runnable
    ON ingestion_jobs(state, run_after, tenant_id);

CREATE TABLE IF NOT EXISTS tenant_job_quotas (
    tenant_id       TEXT PRIMARY KEY REFERENCES tenants(id),
    max_concurrent  INTEGER NOT NULL CHECK (max_concurrent > 0),
    updated_at      TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE IF NOT EXISTS outbox_events (
    id             BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    tenant_id      TEXT NOT NULL REFERENCES tenants(id),
    event_type     TEXT NOT NULL,
    subject_type   TEXT NOT NULL,
    subject_id     BIGINT NOT NULL,
    payload        TEXT NOT NULL,
    dedupe_key     TEXT NOT NULL,
    attempt_count  INTEGER NOT NULL DEFAULT 0 CHECK (attempt_count >= 0),
    max_attempts   INTEGER NOT NULL DEFAULT 10 CHECK (max_attempts >= 1),
    run_after      TIMESTAMPTZ,
    last_error     TEXT,
    delivered_at   TIMESTAMPTZ,
    dead_at        TIMESTAMPTZ,
    created_at     TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE (tenant_id, dedupe_key)
);

CREATE INDEX IF NOT EXISTS idx_outbox_undelivered
    ON outbox_events(delivered_at, dead_at, run_after, id);

ALTER TABLE tenant_job_quotas ENABLE ROW LEVEL SECURITY;
ALTER TABLE outbox_events     ENABLE ROW LEVEL SECURITY;

ALTER TABLE tenant_job_quotas FORCE ROW LEVEL SECURITY;
ALTER TABLE outbox_events     FORCE ROW LEVEL SECURITY;

DROP POLICY IF EXISTS tenant_isolation_tenant_job_quotas ON tenant_job_quotas;
CREATE POLICY tenant_isolation_tenant_job_quotas ON tenant_job_quotas
    USING (tenant_id = current_setting('sentinel.tenant_id', true))
    WITH CHECK (tenant_id = current_setting('sentinel.tenant_id', true));

DROP POLICY IF EXISTS tenant_isolation_outbox_events ON outbox_events;
CREATE POLICY tenant_isolation_outbox_events ON outbox_events
    USING (tenant_id = current_setting('sentinel.tenant_id', true))
    WITH CHECK (tenant_id = current_setting('sentinel.tenant_id', true));

CREATE OR REPLACE FUNCTION outbox_events_content_is_immutable() RETURNS TRIGGER AS $$
BEGIN
    IF NEW.tenant_id    IS DISTINCT FROM OLD.tenant_id
    OR NEW.event_type   IS DISTINCT FROM OLD.event_type
    OR NEW.subject_type IS DISTINCT FROM OLD.subject_type
    OR NEW.subject_id   IS DISTINCT FROM OLD.subject_id
    OR NEW.payload      IS DISTINCT FROM OLD.payload
    OR NEW.dedupe_key   IS DISTINCT FROM OLD.dedupe_key
    OR NEW.created_at   IS DISTINCT FROM OLD.created_at THEN
        RAISE EXCEPTION 'outbox event content is immutable; delivery bookkeeping may change';
    END IF;
    RETURN NEW;
END;
$$ LANGUAGE plpgsql;

DROP TRIGGER IF EXISTS outbox_events_content_immutable ON outbox_events;
CREATE TRIGGER outbox_events_content_immutable
    BEFORE UPDATE ON outbox_events
    FOR EACH ROW EXECUTE FUNCTION outbox_events_content_is_immutable();

CREATE OR REPLACE FUNCTION job_attempts_no_delete() RETURNS TRIGGER AS $$
BEGIN
    RAISE EXCEPTION 'job_attempts is append-only';
END;
$$ LANGUAGE plpgsql;

DROP TRIGGER IF EXISTS job_attempts_no_delete ON job_attempts;
CREATE TRIGGER job_attempts_no_delete
    BEFORE DELETE ON job_attempts
    FOR EACH ROW EXECUTE FUNCTION job_attempts_no_delete();
