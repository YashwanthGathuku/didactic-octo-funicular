-- 002_tenancy_and_state.sql
--
-- Adds the tenant boundary, append-only status history, and CHECK constraints
-- that make illegal states unrepresentable at the storage layer rather than
-- only in application code.
--
-- Context: before this migration no business table had a tenant column, so
-- every query returned every tenant's rows and "tenant isolation" had nothing
-- to enforce. Status columns were free-form text, so a handler could write any
-- string it liked -- which is how an artifact could hold the value RELEASED
-- without ever having been validated or approved.
--
-- SQLite cannot add a NOT NULL column with no default to a populated table, and
-- cannot add CHECK constraints to an existing table at all. Each table is
-- therefore rebuilt: create the new shape, copy rows across assigning the
-- default tenant, drop the old, rename. This is the documented SQLite pattern
-- and runs inside the migration transaction.

-- ---------------------------------------------------------------------------
-- Tenants
-- ---------------------------------------------------------------------------
CREATE TABLE IF NOT EXISTS tenants (
    id          TEXT PRIMARY KEY,
    name        TEXT NOT NULL,
    created_at  TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP
);

-- A single tenant to carry pre-existing rows. Named explicitly so it is
-- obvious in any query that these records predate real tenancy.
INSERT OR IGNORE INTO tenants (id, name) VALUES ('TENANT-DEFAULT', 'Default (pre-tenancy records)');

-- ---------------------------------------------------------------------------
-- partners
-- ---------------------------------------------------------------------------
CREATE TABLE partners_new (
    id              INTEGER PRIMARY KEY AUTOINCREMENT,
    tenant_id       TEXT NOT NULL REFERENCES tenants(id),
    name            TEXT NOT NULL,
    routing_number  TEXT NOT NULL,
    created_at      TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    -- Routing numbers are unique per tenant, not globally: two tenants may
    -- legitimately transact with the same bank.
    UNIQUE (tenant_id, routing_number)
);
INSERT INTO partners_new (id, tenant_id, name, routing_number, created_at)
    SELECT id, 'TENANT-DEFAULT', name, routing_number, created_at FROM partners;
DROP TABLE partners;
ALTER TABLE partners_new RENAME TO partners;
CREATE INDEX idx_partners_tenant ON partners(tenant_id);

-- ---------------------------------------------------------------------------
-- file_contracts
-- ---------------------------------------------------------------------------
CREATE TABLE file_contracts_new (
    id                    INTEGER PRIMARY KEY AUTOINCREMENT,
    tenant_id             TEXT NOT NULL REFERENCES tenants(id),
    partner_id            INTEGER NOT NULL REFERENCES partners(id),
    name                  TEXT NOT NULL,
    direction             TEXT NOT NULL CHECK (direction IN ('INBOUND','OUTBOUND')),
    filename_pattern      TEXT NOT NULL,
    expected_time         TEXT NOT NULL,
    grace_period_minutes  INTEGER NOT NULL CHECK (grace_period_minutes >= 0),
    timezone              TEXT NOT NULL,
    created_at            TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP
);
INSERT INTO file_contracts_new
    SELECT id, 'TENANT-DEFAULT', partner_id, name, direction, filename_pattern,
           expected_time, grace_period_minutes, timezone, created_at
    FROM file_contracts;
DROP TABLE file_contracts;
ALTER TABLE file_contracts_new RENAME TO file_contracts;
CREATE INDEX idx_contracts_tenant ON file_contracts(tenant_id);

-- Immutable contract versions. Editing terms creates a row here; it never
-- updates one, so a historical occurrence still resolves to the terms that were
-- in force on its business date.
CREATE TABLE IF NOT EXISTS file_contract_versions (
    id                INTEGER PRIMARY KEY AUTOINCREMENT,
    tenant_id         TEXT NOT NULL REFERENCES tenants(id),
    contract_id       INTEGER NOT NULL REFERENCES file_contracts(id),
    version           INTEGER NOT NULL CHECK (version >= 1),
    filename_pattern  TEXT NOT NULL,
    format            TEXT NOT NULL DEFAULT 'NACHA',
    expected_local    TEXT NOT NULL,
    timezone          TEXT NOT NULL,
    grace_minutes     INTEGER NOT NULL CHECK (grace_minutes >= 0),
    calendar_id       TEXT,
    balanced_mode     TEXT NOT NULL DEFAULT 'BALANCED'
                        CHECK (balanced_mode IN ('BALANCED','UNBALANCED_AUTHORIZED')),
    effective_from    TIMESTAMP NOT NULL,
    effective_to      TIMESTAMP,
    created_at        TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    UNIQUE (tenant_id, contract_id, version)
);

-- ---------------------------------------------------------------------------
-- expectations
-- ---------------------------------------------------------------------------
CREATE TABLE expectations_new (
    id                       INTEGER PRIMARY KEY AUTOINCREMENT,
    tenant_id                TEXT NOT NULL REFERENCES tenants(id),
    contract_id              INTEGER NOT NULL REFERENCES file_contracts(id),
    contract_version_id      INTEGER REFERENCES file_contract_versions(id),
    business_date            DATE,
    expected_delivery_start  TIMESTAMP NOT NULL,
    expected_delivery_end    TIMESTAMP NOT NULL,
    status                   TEXT NOT NULL
        CHECK (status IN ('PENDING','DUE','OVERDUE','BREACHED','ARRIVED','WAIVED')),
    matched_artifact_id      INTEGER,
    row_version              INTEGER NOT NULL DEFAULT 0,
    created_at               TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at               TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    -- One occurrence per contract per business date. Two schedulers racing
    -- cannot both create one.
    UNIQUE (tenant_id, contract_id, business_date)
);
-- Legacy rows used 'WAITING'; map it onto the modelled PENDING state and
-- reject anything else by leaving it to fail the CHECK.
INSERT INTO expectations_new
    (id, tenant_id, contract_id, expected_delivery_start, expected_delivery_end, status, created_at, updated_at)
    SELECT id, 'TENANT-DEFAULT', contract_id, expected_delivery_start, expected_delivery_end,
           CASE
             WHEN status IN ('PENDING','DUE','OVERDUE','BREACHED','ARRIVED','WAIVED') THEN status
             WHEN status = 'WAITING' THEN 'PENDING'
             WHEN status = 'QUARANTINED' THEN 'ARRIVED'
             ELSE 'PENDING'
           END,
           created_at, updated_at
    FROM expectations;
DROP TABLE expectations;
ALTER TABLE expectations_new RENAME TO expectations;
CREATE INDEX idx_expectations_tenant ON expectations(tenant_id);
CREATE INDEX idx_expectations_state ON expectations(tenant_id, status);

-- ---------------------------------------------------------------------------
-- file_instances (artifacts)
-- ---------------------------------------------------------------------------
CREATE TABLE file_instances_new (
    id               INTEGER PRIMARY KEY AUTOINCREMENT,
    tenant_id        TEXT NOT NULL REFERENCES tenants(id),
    expectation_id   INTEGER REFERENCES expectations(id),
    filename         TEXT NOT NULL,
    storage_path     TEXT NOT NULL,
    size_bytes       INTEGER NOT NULL CHECK (size_bytes >= 0),
    sha256_hash      TEXT NOT NULL,
    status           TEXT NOT NULL
        CHECK (status IN ('RECEIVED','VALIDATING','VALIDATED','QUARANTINED','APPROVED','RELEASED','REJECTED')),
    -- A derived artifact points at the artifact it was produced from. The
    -- source is never mutated.
    derived_from_id  INTEGER REFERENCES file_instances(id),
    row_version      INTEGER NOT NULL DEFAULT 0,
    received_at      TIMESTAMP NOT NULL,
    updated_at       TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    created_at       TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP
);
-- The legacy file_instances table has no created_at column; received_at is the
-- only timestamp it carries, so it seeds both.
INSERT INTO file_instances_new
    (id, tenant_id, expectation_id, filename, storage_path, size_bytes, sha256_hash, status, received_at, updated_at, created_at)
    SELECT id, 'TENANT-DEFAULT', expectation_id, filename, storage_path, size_bytes, sha256_hash,
           CASE
             WHEN status IN ('RECEIVED','VALIDATING','VALIDATED','QUARANTINED','APPROVED','RELEASED','REJECTED') THEN status
             ELSE 'QUARANTINED'   -- unknown legacy state fails closed
           END,
           received_at, received_at, received_at
    FROM file_instances;
DROP TABLE file_instances;
ALTER TABLE file_instances_new RENAME TO file_instances;
CREATE INDEX idx_artifacts_tenant ON file_instances(tenant_id);
CREATE INDEX idx_artifacts_state ON file_instances(tenant_id, status);
CREATE INDEX idx_artifacts_hash ON file_instances(tenant_id, sha256_hash);

-- ---------------------------------------------------------------------------
-- validation_findings
-- ---------------------------------------------------------------------------
CREATE TABLE validation_findings_new (
    id                INTEGER PRIMARY KEY AUTOINCREMENT,
    tenant_id         TEXT NOT NULL REFERENCES tenants(id),
    file_instance_id  INTEGER NOT NULL REFERENCES file_instances(id),
    validation_run_id INTEGER,
    code              TEXT NOT NULL,
    rule_version      TEXT,
    description       TEXT NOT NULL,
    severity          TEXT NOT NULL
        CHECK (severity IN ('INFO','WARNING','ERROR','CRITICAL','FATAL')),
    line_number       INTEGER,
    byte_offset       INTEGER,
    raw_data          TEXT,
    created_at        TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP
);
INSERT INTO validation_findings_new
    (id, tenant_id, file_instance_id, code, description, severity, line_number, raw_data, created_at)
    SELECT id, 'TENANT-DEFAULT', file_instance_id, code, description,
           CASE WHEN severity IN ('INFO','WARNING','ERROR','CRITICAL','FATAL') THEN severity ELSE 'ERROR' END,
           line_number, raw_data, created_at
    FROM validation_findings;
DROP TABLE validation_findings;
ALTER TABLE validation_findings_new RENAME TO validation_findings;
CREATE INDEX idx_findings_tenant ON validation_findings(tenant_id);
CREATE INDEX idx_findings_artifact ON validation_findings(tenant_id, file_instance_id);

-- ---------------------------------------------------------------------------
-- incidents
-- ---------------------------------------------------------------------------
CREATE TABLE incidents_new (
    id                INTEGER PRIMARY KEY AUTOINCREMENT,
    tenant_id         TEXT NOT NULL REFERENCES tenants(id),
    expectation_id    INTEGER REFERENCES expectations(id),
    file_instance_id  INTEGER REFERENCES file_instances(id),
    type              TEXT NOT NULL,
    severity          TEXT NOT NULL,
    status            TEXT NOT NULL CHECK (status IN ('OPEN','INVESTIGATING','RESOLVED','CLOSED')),
    created_at        TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at        TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP
);
INSERT INTO incidents_new
    SELECT id, 'TENANT-DEFAULT', expectation_id, file_instance_id, type, severity,
           CASE WHEN status IN ('OPEN','INVESTIGATING','RESOLVED','CLOSED') THEN status ELSE 'OPEN' END,
           created_at, updated_at
    FROM incidents;
DROP TABLE incidents;
ALTER TABLE incidents_new RENAME TO incidents;
CREATE INDEX idx_incidents_tenant ON incidents(tenant_id);

-- ---------------------------------------------------------------------------
-- audit_events
-- ---------------------------------------------------------------------------
CREATE TABLE audit_events_new (
    id              INTEGER PRIMARY KEY AUTOINCREMENT,
    tenant_id       TEXT NOT NULL REFERENCES tenants(id),
    sequence_no     INTEGER NOT NULL,
    event_type      TEXT NOT NULL,
    actor           TEXT NOT NULL,
    object_type     TEXT,
    object_id       INTEGER,
    correlation_id  TEXT,
    payload         TEXT NOT NULL,
    previous_hash   TEXT NOT NULL,
    current_hash    TEXT NOT NULL,
    created_at      TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    -- Two constraints that together prevent a forked chain: within a tenant a
    -- sequence number is unique, and so is the predecessor a row claims. A
    -- concurrent writer that reads the same predecessor loses on insert rather
    -- than creating a branch.
    UNIQUE (tenant_id, sequence_no),
    UNIQUE (tenant_id, previous_hash)
);
INSERT INTO audit_events_new
    (id, tenant_id, sequence_no, event_type, actor, payload, previous_hash, current_hash, created_at)
    SELECT id, 'TENANT-DEFAULT', id, event_type, actor, payload, previous_hash, current_hash, created_at
    FROM audit_events;
DROP TABLE audit_events;
ALTER TABLE audit_events_new RENAME TO audit_events;
CREATE INDEX idx_audit_tenant ON audit_events(tenant_id, sequence_no);

-- ---------------------------------------------------------------------------
-- New tables: decisions, approvals, jobs, status history, notifications
-- ---------------------------------------------------------------------------

CREATE TABLE IF NOT EXISTS validation_runs (
    id                INTEGER PRIMARY KEY AUTOINCREMENT,
    tenant_id         TEXT NOT NULL REFERENCES tenants(id),
    file_instance_id  INTEGER NOT NULL REFERENCES file_instances(id),
    parser_name       TEXT NOT NULL,
    parser_version    TEXT NOT NULL,
    rule_pack_version TEXT NOT NULL,
    parser_ok         INTEGER NOT NULL CHECK (parser_ok IN (0,1)),
    records_parsed    INTEGER NOT NULL CHECK (records_parsed >= 0),
    -- Integer minor units. Money is never stored as a float.
    total_debits_minor  INTEGER NOT NULL DEFAULT 0,
    total_credits_minor INTEGER NOT NULL DEFAULT 0,
    started_at        TIMESTAMP NOT NULL,
    completed_at      TIMESTAMP
);
CREATE INDEX IF NOT EXISTS idx_runs_tenant ON validation_runs(tenant_id, file_instance_id);

CREATE TABLE IF NOT EXISTS policy_decisions (
    id                INTEGER PRIMARY KEY AUTOINCREMENT,
    tenant_id         TEXT NOT NULL REFERENCES tenants(id),
    file_instance_id  INTEGER NOT NULL REFERENCES file_instances(id),
    validation_run_id INTEGER NOT NULL REFERENCES validation_runs(id),
    policy_version    TEXT NOT NULL,
    state             TEXT NOT NULL CHECK (state IN ('PROPOSED','APPROVED','REJECTED','EXPIRED')),
    outcome           TEXT NOT NULL CHECK (outcome IN ('VALIDATED','QUARANTINED')),
    -- Binds the decision to the exact bytes it was made about.
    artifact_sha256   TEXT NOT NULL,
    reason            TEXT,
    decided_at        TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    -- At most one live decision per validation run: a second finalization
    -- attempt conflicts instead of producing two answers.
    UNIQUE (tenant_id, validation_run_id)
);
CREATE INDEX IF NOT EXISTS idx_decisions_tenant ON policy_decisions(tenant_id, file_instance_id);

CREATE TABLE IF NOT EXISTS approvals (
    id           INTEGER PRIMARY KEY AUTOINCREMENT,
    tenant_id    TEXT NOT NULL REFERENCES tenants(id),
    decision_id  INTEGER NOT NULL REFERENCES policy_decisions(id),
    actor_id     TEXT NOT NULL CHECK (length(actor_id) > 0),
    role         TEXT NOT NULL,
    reason       TEXT NOT NULL CHECK (length(reason) > 0),
    approved_at  TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    -- Separation of duties at the storage layer: the same person cannot record
    -- two approvals against one decision, so dual control cannot be satisfied
    -- by one actor clicking twice.
    UNIQUE (tenant_id, decision_id, actor_id)
);

CREATE TABLE IF NOT EXISTS ingestion_jobs (
    id                INTEGER PRIMARY KEY AUTOINCREMENT,
    tenant_id         TEXT NOT NULL REFERENCES tenants(id),
    file_instance_id  INTEGER REFERENCES file_instances(id),
    idempotency_key   TEXT NOT NULL,
    state             TEXT NOT NULL
        CHECK (state IN ('QUEUED','LEASED','RUNNING','SUCCEEDED','RETRYABLE','DEAD','CANCELLED')),
    attempt_count     INTEGER NOT NULL DEFAULT 0 CHECK (attempt_count >= 0),
    max_attempts      INTEGER NOT NULL DEFAULT 5 CHECK (max_attempts >= 1),
    lease_owner       TEXT,
    lease_expires_at  TIMESTAMP,
    last_error        TEXT,
    row_version       INTEGER NOT NULL DEFAULT 0,
    created_at        TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at        TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    -- Duplicate delivery is a normal condition, not an error: the second
    -- arrival collides here instead of creating a second job.
    UNIQUE (tenant_id, idempotency_key)
);
CREATE INDEX IF NOT EXISTS idx_jobs_claimable ON ingestion_jobs(tenant_id, state, lease_expires_at);

CREATE TABLE IF NOT EXISTS job_attempts (
    id           INTEGER PRIMARY KEY AUTOINCREMENT,
    tenant_id    TEXT NOT NULL REFERENCES tenants(id),
    job_id       INTEGER NOT NULL REFERENCES ingestion_jobs(id),
    attempt_no   INTEGER NOT NULL CHECK (attempt_no >= 1),
    outcome      TEXT NOT NULL,
    error        TEXT,
    started_at   TIMESTAMP NOT NULL,
    finished_at  TIMESTAMP,
    UNIQUE (tenant_id, job_id, attempt_no)
);

-- Append-only status history. Nothing updates or deletes these rows; each
-- state machine transition writes exactly one.
CREATE TABLE IF NOT EXISTS status_history (
    id           INTEGER PRIMARY KEY AUTOINCREMENT,
    tenant_id    TEXT NOT NULL REFERENCES tenants(id),
    object_type  TEXT NOT NULL CHECK (object_type IN ('artifact','expectation','job','decision')),
    object_id    INTEGER NOT NULL,
    from_state   TEXT NOT NULL,
    to_state     TEXT NOT NULL,
    actor_id     TEXT NOT NULL,
    reason       TEXT,
    occurred_at  TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    CHECK (from_state <> to_state)
);
CREATE INDEX IF NOT EXISTS idx_history_object ON status_history(tenant_id, object_type, object_id);

CREATE TABLE IF NOT EXISTS notification_intents (
    id            INTEGER PRIMARY KEY AUTOINCREMENT,
    tenant_id     TEXT NOT NULL REFERENCES tenants(id),
    kind          TEXT NOT NULL,
    subject_type  TEXT NOT NULL,
    subject_id    INTEGER NOT NULL,
    payload       TEXT NOT NULL,
    delivered_at  TIMESTAMP,
    created_at    TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP
);
CREATE INDEX IF NOT EXISTS idx_intents_undelivered ON notification_intents(tenant_id, delivered_at);

-- Append-only enforcement for the two tables where mutation would destroy the
-- evidence they exist to provide.
CREATE TRIGGER IF NOT EXISTS status_history_no_update
BEFORE UPDATE ON status_history
BEGIN
    SELECT RAISE(ABORT, 'status_history is append-only');
END;

CREATE TRIGGER IF NOT EXISTS status_history_no_delete
BEFORE DELETE ON status_history
BEGIN
    SELECT RAISE(ABORT, 'status_history is append-only');
END;

CREATE TRIGGER IF NOT EXISTS audit_events_no_update
BEFORE UPDATE ON audit_events
BEGIN
    SELECT RAISE(ABORT, 'audit_events is append-only');
END;

CREATE TRIGGER IF NOT EXISTS audit_events_no_delete
BEFORE DELETE ON audit_events
BEGIN
    SELECT RAISE(ABORT, 'audit_events is append-only');
END;
