-- 013_agent_memory.sql -- PostgreSQL dialect
-- Memory Bank: persistent cross-session agent memory for incidents,
-- partner history, and SLA trends. Scoped by tenant_id.

CREATE TABLE IF NOT EXISTS agent_memory (
    id            TEXT PRIMARY KEY,
    tenant_id     TEXT NOT NULL REFERENCES tenants(id),
    memory_type   TEXT NOT NULL,
    entity_id     TEXT NOT NULL,
    entity_type   TEXT NOT NULL,
    content       TEXT NOT NULL,
    confidence    DOUBLE PRECISION,
    created_at    TIMESTAMPTZ NOT NULL DEFAULT now(),
    expires_at    TIMESTAMPTZ,
    agent_id      TEXT NOT NULL REFERENCES agent_registry(id),
    invocation_id TEXT REFERENCES agent_invocations(id)
);

CREATE INDEX IF NOT EXISTS idx_agent_memory_tenant_entity
    ON agent_memory(tenant_id, entity_type, entity_id);

CREATE INDEX IF NOT EXISTS idx_agent_memory_type
    ON agent_memory(tenant_id, memory_type);

ALTER TABLE agent_memory ENABLE ROW LEVEL SECURITY;
ALTER TABLE agent_memory FORCE ROW LEVEL SECURITY;

DROP POLICY IF EXISTS tenant_isolation_agent_memory ON agent_memory;
CREATE POLICY tenant_isolation_agent_memory ON agent_memory
    USING (tenant_id = current_setting('sentinel.tenant_id', true))
    WITH CHECK (tenant_id = current_setting('sentinel.tenant_id', true));
