-- Memory Bank: persistent cross-session agent memory for incidents,
-- partner history, and SLA trends. Scoped by tenant_id.

CREATE TABLE IF NOT EXISTS agent_memory (
    id            TEXT PRIMARY KEY,
    tenant_id     TEXT NOT NULL,
    memory_type   TEXT NOT NULL,
    entity_id     TEXT NOT NULL,
    entity_type   TEXT NOT NULL,
    content       TEXT NOT NULL,
    confidence    REAL,
    created_at    TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    expires_at    TIMESTAMP,
    agent_id      TEXT NOT NULL REFERENCES agent_registry(id),
    invocation_id TEXT REFERENCES agent_invocations(id)
);

CREATE INDEX IF NOT EXISTS idx_agent_memory_tenant_entity ON agent_memory(tenant_id, entity_type, entity_id);
CREATE INDEX IF NOT EXISTS idx_agent_memory_type ON agent_memory(tenant_id, memory_type);
