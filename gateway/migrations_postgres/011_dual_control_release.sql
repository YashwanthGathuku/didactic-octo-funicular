-- 011_dual_control_release.sql -- PostgreSQL dialect
-- Human review and dual-control release.

CREATE TABLE IF NOT EXISTS release_policies (
    tenant_id            TEXT PRIMARY KEY REFERENCES tenants(id),
    min_approvals        INTEGER NOT NULL DEFAULT 2 CHECK (min_approvals >= 1),
    separation_of_duties INTEGER NOT NULL DEFAULT 1,
    override_allowed     INTEGER NOT NULL DEFAULT 1,
    updated_by           TEXT NOT NULL DEFAULT '',
    updated_at           TIMESTAMPTZ NOT NULL DEFAULT now()
);

ALTER TABLE policy_decisions ADD COLUMN IF NOT EXISTS row_version INTEGER NOT NULL DEFAULT 0;
ALTER TABLE policy_decisions ADD COLUMN IF NOT EXISTS integrity_digest TEXT NOT NULL DEFAULT '';
ALTER TABLE policy_decisions ADD COLUMN IF NOT EXISTS findings_digest TEXT NOT NULL DEFAULT '';
ALTER TABLE policy_decisions ADD COLUMN IF NOT EXISTS proposed_by TEXT NOT NULL DEFAULT '';
ALTER TABLE policy_decisions ADD COLUMN IF NOT EXISTS proposed_at TIMESTAMPTZ;
ALTER TABLE policy_decisions ADD COLUMN IF NOT EXISTS required_approvals INTEGER NOT NULL DEFAULT 2;
ALTER TABLE policy_decisions ADD COLUMN IF NOT EXISTS separation_of_duties INTEGER NOT NULL DEFAULT 1;
ALTER TABLE policy_decisions ADD COLUMN IF NOT EXISTS contract_version_id BIGINT;
ALTER TABLE policy_decisions ADD COLUMN IF NOT EXISTS expired_reason TEXT;
ALTER TABLE policy_decisions ADD COLUMN IF NOT EXISTS released_at TIMESTAMPTZ;
ALTER TABLE policy_decisions ADD COLUMN IF NOT EXISTS released_by TEXT;

CREATE INDEX IF NOT EXISTS idx_decisions_queue
    ON policy_decisions(tenant_id, state, decided_at);

ALTER TABLE approvals ADD COLUMN IF NOT EXISTS vote TEXT NOT NULL DEFAULT 'APPROVE'
    CHECK (vote IN ('APPROVE','REJECT'));
ALTER TABLE approvals ADD COLUMN IF NOT EXISTS integrity_digest TEXT NOT NULL DEFAULT '';

CREATE TABLE IF NOT EXISTS release_overrides (
    id                 BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    tenant_id          TEXT NOT NULL REFERENCES tenants(id),
    decision_id        BIGINT NOT NULL REFERENCES policy_decisions(id),
    file_instance_id   BIGINT NOT NULL REFERENCES file_instances(id),
    actor_id           TEXT NOT NULL CHECK (length(actor_id) > 0),
    role               TEXT NOT NULL,
    reason             TEXT NOT NULL CHECK (length(trim(reason)) >= 20),
    bypassed           TEXT NOT NULL,
    approvals_held     INTEGER NOT NULL DEFAULT 0,
    approvals_required INTEGER NOT NULL,
    blocking_rule_ids  TEXT NOT NULL DEFAULT '',
    integrity_digest   TEXT NOT NULL DEFAULT '',
    created_at         TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE (tenant_id, decision_id)
);

CREATE INDEX IF NOT EXISTS idx_overrides_recent
    ON release_overrides(tenant_id, created_at);

ALTER TABLE validation_runs ADD COLUMN IF NOT EXISTS policy_version TEXT NOT NULL DEFAULT '';
ALTER TABLE validation_runs ADD COLUMN IF NOT EXISTS contract_id TEXT NOT NULL DEFAULT '';
ALTER TABLE validation_runs ADD COLUMN IF NOT EXISTS contract_version TEXT NOT NULL DEFAULT '';
ALTER TABLE validation_runs ADD COLUMN IF NOT EXISTS outcome TEXT NOT NULL DEFAULT '';
ALTER TABLE validation_runs ADD COLUMN IF NOT EXISTS findings_digest TEXT NOT NULL DEFAULT '';
ALTER TABLE validation_runs ADD COLUMN IF NOT EXISTS blocking_rule_ids TEXT NOT NULL DEFAULT '';

CREATE INDEX IF NOT EXISTS idx_runs_artifact
    ON validation_runs(tenant_id, file_instance_id, started_at);

ALTER TABLE release_policies  ENABLE ROW LEVEL SECURITY;
ALTER TABLE release_overrides ENABLE ROW LEVEL SECURITY;

ALTER TABLE release_policies  FORCE ROW LEVEL SECURITY;
ALTER TABLE release_overrides FORCE ROW LEVEL SECURITY;

DROP POLICY IF EXISTS tenant_isolation_release_policies ON release_policies;
CREATE POLICY tenant_isolation_release_policies ON release_policies
    USING (tenant_id = current_setting('sentinel.tenant_id', true))
    WITH CHECK (tenant_id = current_setting('sentinel.tenant_id', true));

DROP POLICY IF EXISTS tenant_isolation_release_overrides ON release_overrides;
CREATE POLICY tenant_isolation_release_overrides ON release_overrides
    USING (tenant_id = current_setting('sentinel.tenant_id', true))
    WITH CHECK (tenant_id = current_setting('sentinel.tenant_id', true));
