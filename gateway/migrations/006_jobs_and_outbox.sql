-- Durable jobs and a transactional outbox.
--
-- `ingestion_jobs` and `job_attempts` were created by migration 002 and have
-- had no consumer since: ingest enqueues a row and nothing ever leases it, so
-- every uploaded artifact stays RECEIVED forever. This migration adds what a
-- worker needs to claim work safely and what a dispatcher needs to deliver
-- events exactly once in effect.

-- Scheduling. A job that failed becomes visible again only after its backoff.
ALTER TABLE ingestion_jobs ADD COLUMN run_after TIMESTAMP;

-- The heartbeat is separate from the lease expiry so a stalled worker is
-- distinguishable from a slow one: the lease says when the claim lapses, the
-- heartbeat says when the owner was last alive.
ALTER TABLE ingestion_jobs ADD COLUMN last_heartbeat_at TIMESTAMP;

-- Job kind, so one queue can carry more than validation without a second
-- table. Existing rows are validation jobs.
ALTER TABLE ingestion_jobs ADD COLUMN kind TEXT NOT NULL DEFAULT 'VALIDATE_ARTIFACT';

-- Why a job reached a terminal state. A DEAD job with no stated reason is an
-- operator's dead end.
ALTER TABLE ingestion_jobs ADD COLUMN terminal_reason TEXT;

CREATE INDEX IF NOT EXISTS idx_jobs_runnable
    ON ingestion_jobs(state, run_after, tenant_id);

-- Per-tenant concurrency quota.
--
-- Without it one tenant's backlog occupies every worker and every other
-- tenant's files stop being validated. That is a shared-fate failure between
-- customers, which is the same class of defect as a missing tenant filter --
-- it just presents as an outage rather than a disclosure.
CREATE TABLE IF NOT EXISTS tenant_job_quotas (
    tenant_id       TEXT PRIMARY KEY REFERENCES tenants(id),
    max_concurrent  INTEGER NOT NULL CHECK (max_concurrent > 0),
    updated_at      TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP
);

-- The transactional outbox.
--
-- An event is written in the same transaction as the business state it
-- describes. That is the whole point: a process that commits an artifact's
-- validation and then dies before publishing has still recorded the event, and
-- the dispatcher delivers it on the next pass. The alternative -- publish after
-- commit -- loses the event on exactly the failure it needs to survive.
--
-- Nothing in this table is delivered inside the committing transaction, and no
-- external network call happens there either. The dispatcher is a separate pass
-- over rows that are already durable.
CREATE TABLE IF NOT EXISTS outbox_events (
    id             INTEGER PRIMARY KEY AUTOINCREMENT,
    tenant_id      TEXT NOT NULL REFERENCES tenants(id),

    -- What the event is about, so a consumer can route without parsing the
    -- payload.
    event_type     TEXT NOT NULL,
    subject_type   TEXT NOT NULL,
    subject_id     INTEGER NOT NULL,

    -- The payload carries no raw financial content. Findings are already
    -- redacted by internal/nacha before they reach here.
    payload        TEXT NOT NULL,

    -- A stable identity for the event. A duplicate delivery is harmless because
    -- a consumer can recognise one, and a duplicate *write* is impossible
    -- because of the uniqueness constraint below.
    dedupe_key     TEXT NOT NULL,

    attempt_count  INTEGER NOT NULL DEFAULT 0 CHECK (attempt_count >= 0),
    max_attempts   INTEGER NOT NULL DEFAULT 10 CHECK (max_attempts >= 1),
    run_after      TIMESTAMP,
    last_error     TEXT,

    -- delivered_at is set only after the delivery succeeded. A dispatcher that
    -- marked delivery first would lose every event whose delivery then failed.
    delivered_at   TIMESTAMP,
    dead_at        TIMESTAMP,

    created_at     TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,

    UNIQUE (tenant_id, dedupe_key)
);

CREATE INDEX IF NOT EXISTS idx_outbox_undelivered
    ON outbox_events(delivered_at, dead_at, run_after, id);

-- An outbox row is a record that something happened. Its subject and payload
-- are immutable; only the delivery bookkeeping may change.
CREATE TRIGGER IF NOT EXISTS outbox_events_content_is_immutable
BEFORE UPDATE OF tenant_id, event_type, subject_type, subject_id, payload, dedupe_key, created_at
ON outbox_events
BEGIN
    SELECT RAISE(ABORT, 'outbox event content is immutable; delivery bookkeeping may change');
END;

-- Attempt records are evidence of what the system tried. Rewriting one hides a
-- failure that happened.
CREATE TRIGGER IF NOT EXISTS job_attempts_no_delete
BEFORE DELETE ON job_attempts
BEGIN
    SELECT RAISE(ABORT, 'job_attempts is append-only');
END;
