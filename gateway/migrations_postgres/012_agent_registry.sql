-- 012_agent_registry.sql -- PostgreSQL dialect
-- Agent Registry: lifecycle metadata and versioning for AI agent fleet.

CREATE TABLE IF NOT EXISTS agent_registry (
    id            TEXT PRIMARY KEY,
    tenant_id     TEXT NOT NULL REFERENCES tenants(id),
    agent_type    TEXT NOT NULL,
    display_name  TEXT NOT NULL,
    description   TEXT,
    model_id      TEXT NOT NULL,
    version       TEXT NOT NULL,
    status        TEXT NOT NULL DEFAULT 'ACTIVE',
    tool_scopes   TEXT NOT NULL,
    created_at    TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at    TIMESTAMPTZ NOT NULL DEFAULT now(),
    config_hash   TEXT NOT NULL
);

CREATE INDEX IF NOT EXISTS idx_agent_registry_tenant ON agent_registry(tenant_id);

CREATE TABLE IF NOT EXISTS agent_invocations (
    id                  TEXT PRIMARY KEY,
    agent_id            TEXT NOT NULL REFERENCES agent_registry(id),
    incident_id         TEXT,
    tenant_id           TEXT NOT NULL REFERENCES tenants(id),
    started_at          TIMESTAMPTZ NOT NULL,
    completed_at        TIMESTAMPTZ,
    status              TEXT NOT NULL DEFAULT 'RUNNING',
    input_hash          TEXT NOT NULL,
    output_hash         TEXT,
    token_count         INTEGER,
    latency_ms          INTEGER,
    model_armor_verdict TEXT
);

CREATE INDEX IF NOT EXISTS idx_agent_invocations_tenant ON agent_invocations(tenant_id);
CREATE INDEX IF NOT EXISTS idx_agent_invocations_agent ON agent_invocations(agent_id);

ALTER TABLE agent_registry    ENABLE ROW LEVEL SECURITY;
ALTER TABLE agent_invocations ENABLE ROW LEVEL SECURITY;

ALTER TABLE agent_registry    FORCE ROW LEVEL SECURITY;
ALTER TABLE agent_invocations FORCE ROW LEVEL SECURITY;

DROP POLICY IF EXISTS tenant_isolation_agent_registry ON agent_registry;
CREATE POLICY tenant_isolation_agent_registry ON agent_registry
    USING (tenant_id = current_setting('sentinel.tenant_id', true))
    WITH CHECK (tenant_id = current_setting('sentinel.tenant_id', true));

DROP POLICY IF EXISTS tenant_isolation_agent_invocations ON agent_invocations;
CREATE POLICY tenant_isolation_agent_invocations ON agent_invocations
    USING (tenant_id = current_setting('sentinel.tenant_id', true))
    WITH CHECK (tenant_id = current_setting('sentinel.tenant_id', true));
