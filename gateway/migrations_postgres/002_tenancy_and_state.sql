-- 002_tenancy_and_state.sql -- PostgreSQL dialect
-- Adds the tenant boundary, append-only status history, and CHECK constraints.

CREATE TABLE IF NOT EXISTS file_contracts (
    id                    BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    tenant_id             TEXT NOT NULL REFERENCES tenants(id),
    partner_id            BIGINT NOT NULL REFERENCES partners(id),
    name                  TEXT NOT NULL,
    direction             TEXT NOT NULL CHECK (direction IN ('INBOUND','OUTBOUND')),
    filename_pattern      TEXT NOT NULL,
    expected_time         TEXT NOT NULL,
    grace_period_minutes  INTEGER NOT NULL CHECK (grace_period_minutes >= 0),
    timezone              TEXT NOT NULL,
    created_at            TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX IF NOT EXISTS idx_contracts_tenant ON file_contracts(tenant_id);

CREATE TABLE IF NOT EXISTS file_contract_versions (
    id                BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    tenant_id         TEXT NOT NULL REFERENCES tenants(id),
    contract_id       BIGINT NOT NULL REFERENCES file_contracts(id),
    version           INTEGER NOT NULL CHECK (version >= 1),
    filename_pattern  TEXT NOT NULL,
    format            TEXT NOT NULL DEFAULT 'NACHA',
    expected_local    TEXT NOT NULL,
    timezone          TEXT NOT NULL,
    grace_minutes     INTEGER NOT NULL CHECK (grace_minutes >= 0),
    calendar_id       TEXT,
    balanced_mode     TEXT NOT NULL DEFAULT 'BALANCED'
                        CHECK (balanced_mode IN ('BALANCED','UNBALANCED_AUTHORIZED')),
    effective_from    TIMESTAMPTZ NOT NULL,
    effective_to      TIMESTAMPTZ,
    created_at        TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE (tenant_id, contract_id, version)
);

CREATE TABLE IF NOT EXISTS expectations (
    id                       BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    tenant_id                TEXT NOT NULL REFERENCES tenants(id),
    contract_id              BIGINT NOT NULL REFERENCES file_contracts(id),
    contract_version_id      BIGINT REFERENCES file_contract_versions(id),
    business_date            DATE,
    expected_delivery_start  TIMESTAMPTZ NOT NULL,
    expected_delivery_end    TIMESTAMPTZ NOT NULL,
    status                   TEXT NOT NULL
        CHECK (status IN ('PENDING','DUE','OVERDUE','BREACHED','ARRIVED','WAIVED')),
    matched_artifact_id      BIGINT REFERENCES file_instances(id),
    row_version              INTEGER NOT NULL DEFAULT 0,
    created_at               TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at               TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE (tenant_id, contract_id, business_date)
);

CREATE INDEX IF NOT EXISTS idx_expectations_tenant ON expectations(tenant_id);
CREATE INDEX IF NOT EXISTS idx_expectations_state ON expectations(tenant_id, status);

CREATE TABLE IF NOT EXISTS validation_findings (
    id                BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    tenant_id         TEXT NOT NULL REFERENCES tenants(id),
    file_instance_id  BIGINT NOT NULL REFERENCES file_instances(id),
    validation_run_id BIGINT,
    code              TEXT NOT NULL,
    rule_version      TEXT,
    description       TEXT NOT NULL,
    severity          TEXT NOT NULL
        CHECK (severity IN ('INFO','WARNING','ERROR','CRITICAL','FATAL')),
    line_number       INTEGER,
    byte_offset       INTEGER,
    raw_data          TEXT,
    created_at        TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX IF NOT EXISTS idx_findings_tenant ON validation_findings(tenant_id);
CREATE INDEX IF NOT EXISTS idx_findings_artifact ON validation_findings(tenant_id, file_instance_id);

CREATE TABLE IF NOT EXISTS validation_runs (
    id                  BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    tenant_id           TEXT NOT NULL REFERENCES tenants(id),
    file_instance_id    BIGINT NOT NULL REFERENCES file_instances(id),
    parser_name         TEXT NOT NULL,
    parser_version      TEXT NOT NULL,
    rule_pack_version   TEXT NOT NULL,
    parser_ok           INTEGER NOT NULL CHECK (parser_ok IN (0,1)),
    records_parsed      INTEGER NOT NULL CHECK (records_parsed >= 0),
    total_debits_minor  BIGINT NOT NULL DEFAULT 0,
    total_credits_minor BIGINT NOT NULL DEFAULT 0,
    started_at          TIMESTAMPTZ NOT NULL,
    completed_at        TIMESTAMPTZ
);

CREATE INDEX IF NOT EXISTS idx_runs_tenant ON validation_runs(tenant_id, file_instance_id);

CREATE TABLE IF NOT EXISTS policy_decisions (
    id                BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    tenant_id         TEXT NOT NULL REFERENCES tenants(id),
    file_instance_id  BIGINT NOT NULL REFERENCES file_instances(id),
    validation_run_id BIGINT NOT NULL REFERENCES validation_runs(id),
    policy_version    TEXT NOT NULL,
    state             TEXT NOT NULL CHECK (state IN ('PROPOSED','APPROVED','REJECTED','EXPIRED')),
    outcome           TEXT NOT NULL CHECK (outcome IN ('VALIDATED','QUARANTINED')),
    artifact_sha256   TEXT NOT NULL,
    reason            TEXT,
    decided_at        TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE (tenant_id, validation_run_id)
);

CREATE INDEX IF NOT EXISTS idx_decisions_tenant ON policy_decisions(tenant_id, file_instance_id);

CREATE TABLE IF NOT EXISTS approvals (
    id           BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    tenant_id    TEXT NOT NULL REFERENCES tenants(id),
    decision_id  BIGINT NOT NULL REFERENCES policy_decisions(id),
    actor_id     TEXT NOT NULL CHECK (length(actor_id) > 0),
    role         TEXT NOT NULL,
    reason       TEXT NOT NULL CHECK (length(reason) > 0),
    approved_at  TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE (tenant_id, decision_id, actor_id)
);

CREATE TABLE IF NOT EXISTS ingestion_jobs (
    id                BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    tenant_id         TEXT NOT NULL REFERENCES tenants(id),
    file_instance_id  BIGINT REFERENCES file_instances(id),
    idempotency_key   TEXT NOT NULL,
    state             TEXT NOT NULL
        CHECK (state IN ('QUEUED','LEASED','RUNNING','SUCCEEDED','RETRYABLE','DEAD','CANCELLED')),
    attempt_count     INTEGER NOT NULL DEFAULT 0 CHECK (attempt_count >= 0),
    max_attempts      INTEGER NOT NULL DEFAULT 5 CHECK (max_attempts >= 1),
    lease_owner       TEXT,
    lease_expires_at  TIMESTAMPTZ,
    last_error        TEXT,
    row_version       INTEGER NOT NULL DEFAULT 0,
    created_at        TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at        TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE (tenant_id, idempotency_key)
);

CREATE INDEX IF NOT EXISTS idx_jobs_claimable ON ingestion_jobs(tenant_id, state, lease_expires_at);

CREATE TABLE IF NOT EXISTS job_attempts (
    id           BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    tenant_id    TEXT NOT NULL REFERENCES tenants(id),
    job_id       BIGINT NOT NULL REFERENCES ingestion_jobs(id),
    attempt_no   INTEGER NOT NULL CHECK (attempt_no >= 1),
    outcome      TEXT NOT NULL,
    error        TEXT,
    started_at   TIMESTAMPTZ NOT NULL,
    finished_at  TIMESTAMPTZ,
    UNIQUE (tenant_id, job_id, attempt_no)
);

CREATE TABLE IF NOT EXISTS status_history (
    id           BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    tenant_id    TEXT NOT NULL REFERENCES tenants(id),
    object_type  TEXT NOT NULL CHECK (object_type IN ('artifact','expectation','job','decision')),
    object_id    BIGINT NOT NULL,
    from_state   TEXT NOT NULL,
    to_state     TEXT NOT NULL,
    actor_id     TEXT NOT NULL,
    reason       TEXT,
    occurred_at  TIMESTAMPTZ NOT NULL DEFAULT now(),
    CHECK (from_state <> to_state)
);

CREATE INDEX IF NOT EXISTS idx_history_object ON status_history(tenant_id, object_type, object_id);

CREATE TABLE IF NOT EXISTS notification_intents (
    id            BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    tenant_id     TEXT NOT NULL REFERENCES tenants(id),
    kind          TEXT NOT NULL,
    subject_type  TEXT NOT NULL,
    subject_id    BIGINT NOT NULL,
    payload       TEXT NOT NULL,
    delivered_at  TIMESTAMPTZ,
    created_at    TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX IF NOT EXISTS idx_intents_undelivered ON notification_intents(tenant_id, delivered_at);

-- Row-level security
ALTER TABLE file_contracts         ENABLE ROW LEVEL SECURITY;
ALTER TABLE file_contract_versions ENABLE ROW LEVEL SECURITY;
ALTER TABLE expectations           ENABLE ROW LEVEL SECURITY;
ALTER TABLE validation_findings    ENABLE ROW LEVEL SECURITY;
ALTER TABLE validation_runs        ENABLE ROW LEVEL SECURITY;
ALTER TABLE policy_decisions       ENABLE ROW LEVEL SECURITY;
ALTER TABLE approvals              ENABLE ROW LEVEL SECURITY;
ALTER TABLE ingestion_jobs         ENABLE ROW LEVEL SECURITY;
ALTER TABLE job_attempts           ENABLE ROW LEVEL SECURITY;
ALTER TABLE status_history         ENABLE ROW LEVEL SECURITY;
ALTER TABLE notification_intents   ENABLE ROW LEVEL SECURITY;

ALTER TABLE file_contracts         FORCE ROW LEVEL SECURITY;
ALTER TABLE file_contract_versions FORCE ROW LEVEL SECURITY;
ALTER TABLE expectations           FORCE ROW LEVEL SECURITY;
ALTER TABLE validation_findings    FORCE ROW LEVEL SECURITY;
ALTER TABLE validation_runs        FORCE ROW LEVEL SECURITY;
ALTER TABLE policy_decisions       FORCE ROW LEVEL SECURITY;
ALTER TABLE approvals              FORCE ROW LEVEL SECURITY;
ALTER TABLE ingestion_jobs         FORCE ROW LEVEL SECURITY;
ALTER TABLE job_attempts           FORCE ROW LEVEL SECURITY;
ALTER TABLE status_history         FORCE ROW LEVEL SECURITY;
ALTER TABLE notification_intents   FORCE ROW LEVEL SECURITY;

DROP POLICY IF EXISTS tenant_isolation_file_contracts ON file_contracts;
CREATE POLICY tenant_isolation_file_contracts ON file_contracts
    USING (tenant_id = current_setting('sentinel.tenant_id', true))
    WITH CHECK (tenant_id = current_setting('sentinel.tenant_id', true));

DROP POLICY IF EXISTS tenant_isolation_file_contract_versions ON file_contract_versions;
CREATE POLICY tenant_isolation_file_contract_versions ON file_contract_versions
    USING (tenant_id = current_setting('sentinel.tenant_id', true))
    WITH CHECK (tenant_id = current_setting('sentinel.tenant_id', true));

DROP POLICY IF EXISTS tenant_isolation_expectations ON expectations;
CREATE POLICY tenant_isolation_expectations ON expectations
    USING (tenant_id = current_setting('sentinel.tenant_id', true))
    WITH CHECK (tenant_id = current_setting('sentinel.tenant_id', true));

DROP POLICY IF EXISTS tenant_isolation_validation_findings ON validation_findings;
CREATE POLICY tenant_isolation_validation_findings ON validation_findings
    USING (tenant_id = current_setting('sentinel.tenant_id', true))
    WITH CHECK (tenant_id = current_setting('sentinel.tenant_id', true));

DROP POLICY IF EXISTS tenant_isolation_validation_runs ON validation_runs;
CREATE POLICY tenant_isolation_validation_runs ON validation_runs
    USING (tenant_id = current_setting('sentinel.tenant_id', true))
    WITH CHECK (tenant_id = current_setting('sentinel.tenant_id', true));

DROP POLICY IF EXISTS tenant_isolation_policy_decisions ON policy_decisions;
CREATE POLICY tenant_isolation_policy_decisions ON policy_decisions
    USING (tenant_id = current_setting('sentinel.tenant_id', true))
    WITH CHECK (tenant_id = current_setting('sentinel.tenant_id', true));

DROP POLICY IF EXISTS tenant_isolation_approvals ON approvals;
CREATE POLICY tenant_isolation_approvals ON approvals
    USING (tenant_id = current_setting('sentinel.tenant_id', true))
    WITH CHECK (tenant_id = current_setting('sentinel.tenant_id', true));

DROP POLICY IF EXISTS tenant_isolation_ingestion_jobs ON ingestion_jobs;
CREATE POLICY tenant_isolation_ingestion_jobs ON ingestion_jobs
    USING (tenant_id = current_setting('sentinel.tenant_id', true))
    WITH CHECK (tenant_id = current_setting('sentinel.tenant_id', true));

DROP POLICY IF EXISTS tenant_isolation_job_attempts ON job_attempts;
CREATE POLICY tenant_isolation_job_attempts ON job_attempts
    USING (tenant_id = current_setting('sentinel.tenant_id', true))
    WITH CHECK (tenant_id = current_setting('sentinel.tenant_id', true));

DROP POLICY IF EXISTS tenant_isolation_status_history ON status_history;
CREATE POLICY tenant_isolation_status_history ON status_history
    USING (tenant_id = current_setting('sentinel.tenant_id', true))
    WITH CHECK (tenant_id = current_setting('sentinel.tenant_id', true));

DROP POLICY IF EXISTS tenant_isolation_notification_intents ON notification_intents;
CREATE POLICY tenant_isolation_notification_intents ON notification_intents
    USING (tenant_id = current_setting('sentinel.tenant_id', true))
    WITH CHECK (tenant_id = current_setting('sentinel.tenant_id', true));

-- Append-only triggers
CREATE OR REPLACE FUNCTION status_history_append_only() RETURNS TRIGGER AS $$
BEGIN
    RAISE EXCEPTION 'status_history is append-only';
END;
$$ LANGUAGE plpgsql;

DROP TRIGGER IF EXISTS status_history_no_change ON status_history;
CREATE TRIGGER status_history_no_change
    BEFORE UPDATE OR DELETE ON status_history
    FOR EACH ROW EXECUTE FUNCTION status_history_append_only();

CREATE OR REPLACE FUNCTION audit_events_append_only() RETURNS TRIGGER AS $$
BEGIN
    RAISE EXCEPTION 'audit_events is append-only';
END;
$$ LANGUAGE plpgsql;

DROP TRIGGER IF EXISTS audit_events_no_change ON audit_events;
CREATE TRIGGER audit_events_no_change
    BEFORE UPDATE OR DELETE ON audit_events
    FOR EACH ROW EXECUTE FUNCTION audit_events_append_only();
