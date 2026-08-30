-- 008_scheduling.sql -- PostgreSQL dialect
-- Feed contracts, business calendars, and materialized expectations.

CREATE TABLE IF NOT EXISTS business_calendars (
    tenant_id   TEXT NOT NULL REFERENCES tenants(id),
    calendar_id TEXT NOT NULL,
    name        TEXT NOT NULL,
    base        TEXT NOT NULL CHECK (base IN ('FEDERAL_RESERVE','WEEKDAYS','ALL_DAYS')),
    created_at  TIMESTAMPTZ NOT NULL DEFAULT now(),
    PRIMARY KEY (tenant_id, calendar_id)
);

CREATE TABLE IF NOT EXISTS business_calendar_overrides (
    tenant_id     TEXT NOT NULL REFERENCES tenants(id),
    calendar_id   TEXT NOT NULL,
    calendar_date DATE NOT NULL,
    kind          TEXT NOT NULL CHECK (kind IN ('HOLIDAY','BUSINESS_DAY')),
    reason        TEXT NOT NULL CHECK (length(trim(reason)) > 0),
    created_at    TIMESTAMPTZ NOT NULL DEFAULT now(),
    PRIMARY KEY (tenant_id, calendar_id, calendar_date),
    FOREIGN KEY (tenant_id, calendar_id)
        REFERENCES business_calendars(tenant_id, calendar_id)
);

ALTER TABLE file_contract_versions ADD COLUMN IF NOT EXISTS schedule_rule TEXT NOT NULL DEFAULT 'EVERY_BUSINESS_DAY';
ALTER TABLE file_contract_versions ADD COLUMN IF NOT EXISTS nonbusiness_action TEXT NOT NULL DEFAULT 'SKIP';
ALTER TABLE file_contract_versions ADD COLUMN IF NOT EXISTS breach_after_minutes INTEGER NOT NULL DEFAULT 60;
ALTER TABLE file_contract_versions ADD COLUMN IF NOT EXISTS owner_subject TEXT NOT NULL DEFAULT '';
ALTER TABLE file_contract_versions ADD COLUMN IF NOT EXISTS escalation_policy_id TEXT NOT NULL DEFAULT '';
ALTER TABLE file_contract_versions ADD COLUMN IF NOT EXISTS feed_id TEXT NOT NULL DEFAULT '';
ALTER TABLE file_contract_versions ADD COLUMN IF NOT EXISTS direction TEXT NOT NULL DEFAULT 'INBOUND';

CREATE INDEX IF NOT EXISTS idx_contract_versions_active
    ON file_contract_versions(tenant_id, contract_id, effective_from);

ALTER TABLE expectations ADD COLUMN IF NOT EXISTS breach_at TIMESTAMPTZ;
ALTER TABLE expectations ADD COLUMN IF NOT EXISTS schedule_note TEXT NOT NULL DEFAULT '';
ALTER TABLE expectations ADD COLUMN IF NOT EXISTS due_local TEXT NOT NULL DEFAULT '';
ALTER TABLE expectations ADD COLUMN IF NOT EXISTS timezone TEXT NOT NULL DEFAULT '';
ALTER TABLE expectations ADD COLUMN IF NOT EXISTS matched_at TIMESTAMPTZ;
ALTER TABLE expectations ADD COLUMN IF NOT EXISTS review_required INTEGER NOT NULL DEFAULT 0;

CREATE INDEX IF NOT EXISTS idx_expectations_due
    ON expectations(tenant_id, status, expected_delivery_start);

CREATE INDEX IF NOT EXISTS idx_expectations_business_date
    ON expectations(tenant_id, business_date);

CREATE TABLE IF NOT EXISTS expectation_match_candidates (
    id               BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    tenant_id        TEXT NOT NULL REFERENCES tenants(id),
    expectation_id   BIGINT NOT NULL REFERENCES expectations(id),
    file_instance_id BIGINT NOT NULL REFERENCES file_instances(id),
    filename         TEXT NOT NULL,
    reason           TEXT NOT NULL,
    resolution       TEXT NOT NULL DEFAULT 'REVIEW_REQUIRED'
                       CHECK (resolution IN ('REVIEW_REQUIRED','ACCEPTED','REJECTED')),
    resolved_by      TEXT,
    resolved_at      TIMESTAMPTZ,
    created_at       TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE (tenant_id, expectation_id, file_instance_id)
);

CREATE INDEX IF NOT EXISTS idx_match_candidates_open
    ON expectation_match_candidates(tenant_id, resolution);

ALTER TABLE business_calendars            ENABLE ROW LEVEL SECURITY;
ALTER TABLE business_calendar_overrides  ENABLE ROW LEVEL SECURITY;
ALTER TABLE expectation_match_candidates  ENABLE ROW LEVEL SECURITY;

ALTER TABLE business_calendars            FORCE ROW LEVEL SECURITY;
ALTER TABLE business_calendar_overrides  FORCE ROW LEVEL SECURITY;
ALTER TABLE expectation_match_candidates  FORCE ROW LEVEL SECURITY;

DROP POLICY IF EXISTS tenant_isolation_business_calendars ON business_calendars;
CREATE POLICY tenant_isolation_business_calendars ON business_calendars
    USING (tenant_id = current_setting('sentinel.tenant_id', true))
    WITH CHECK (tenant_id = current_setting('sentinel.tenant_id', true));

DROP POLICY IF EXISTS tenant_isolation_business_calendar_overrides ON business_calendar_overrides;
CREATE POLICY tenant_isolation_business_calendar_overrides ON business_calendar_overrides
    USING (tenant_id = current_setting('sentinel.tenant_id', true))
    WITH CHECK (tenant_id = current_setting('sentinel.tenant_id', true));

DROP POLICY IF EXISTS tenant_isolation_expectation_match_candidates ON expectation_match_candidates;
CREATE POLICY tenant_isolation_expectation_match_candidates ON expectation_match_candidates
    USING (tenant_id = current_setting('sentinel.tenant_id', true))
    WITH CHECK (tenant_id = current_setting('sentinel.tenant_id', true));
