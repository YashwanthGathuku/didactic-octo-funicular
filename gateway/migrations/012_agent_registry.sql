-- Agent Registry: lifecycle metadata and versioning for AI agent fleet.
-- Each agent has a declared type, model, version, and explicit tool scope list.

CREATE TABLE IF NOT EXISTS agent_registry (
    id            TEXT PRIMARY KEY,
    tenant_id     TEXT NOT NULL,
    agent_type    TEXT NOT NULL,
    display_name  TEXT NOT NULL,
    description   TEXT,
    model_id      TEXT NOT NULL,
    version       TEXT NOT NULL,
    status        TEXT NOT NULL DEFAULT 'ACTIVE',
    tool_scopes   TEXT NOT NULL,
    created_at    TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at    TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    config_hash   TEXT NOT NULL
);

CREATE TABLE IF NOT EXISTS agent_invocations (
    id              TEXT PRIMARY KEY,
    agent_id        TEXT NOT NULL REFERENCES agent_registry(id),
    incident_id     TEXT,
    tenant_id       TEXT NOT NULL,
    started_at      TIMESTAMP NOT NULL,
    completed_at    TIMESTAMP,
    status          TEXT NOT NULL DEFAULT 'RUNNING',
    input_hash      TEXT NOT NULL,
    output_hash     TEXT,
    token_count     INTEGER,
    latency_ms      INTEGER,
    model_armor_verdict TEXT
);

CREATE INDEX IF NOT EXISTS idx_agent_invocations_tenant ON agent_invocations(tenant_id);
CREATE INDEX IF NOT EXISTS idx_agent_invocations_agent ON agent_invocations(agent_id);
