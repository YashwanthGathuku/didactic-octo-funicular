-- 018_tool_gateway.sql -- PostgreSQL dialect
-- Governed Tool & Action Gateway: durable tool invocation tracking and audit.

CREATE TABLE IF NOT EXISTS tool_invocations (
    id                      TEXT PRIMARY KEY,
    tenant_id               TEXT NOT NULL REFERENCES tenants(id),
    tool_id                 TEXT NOT NULL,
    tool_version            TEXT NOT NULL,
    manifest_hash           TEXT NOT NULL,
    caller_type             TEXT NOT NULL CHECK (caller_type IN ('AGENT', 'HUMAN', 'SYSTEM', 'API')),
    caller_id               TEXT NOT NULL,
    caller_autonomy_level   INTEGER NOT NULL DEFAULT 1,
    workflow_id             TEXT,
    idempotency_key         TEXT NOT NULL,
    request_hash            TEXT NOT NULL,
    status                  TEXT NOT NULL CHECK (status IN ('RECEIVED', 'AUTHORIZED', 'EXECUTING', 'SUCCEEDED', 'DENIED', 'FAILED', 'TIMED_OUT', 'UNCERTAIN')),
    policy_decision_id      TEXT,
    policy_decision_hash    TEXT,
    policy_bundle_hash      TEXT,
    input_hash              TEXT NOT NULL,
    output_hash             TEXT,
    output_payload          TEXT,
    error_code              TEXT,
    error_message           TEXT,
    duration_ms             INTEGER,
    execution_mode          TEXT NOT NULL DEFAULT 'LIVE',
    created_at              TIMESTAMPTZ NOT NULL DEFAULT now(),
    completed_at            TIMESTAMPTZ,
    UNIQUE (tenant_id, caller_id, tool_id, tool_version, idempotency_key)
);

CREATE INDEX IF NOT EXISTS idx_tool_invocations_tenant_created
    ON tool_invocations(tenant_id, created_at);

CREATE INDEX IF NOT EXISTS idx_tool_invocations_workflow
    ON tool_invocations(workflow_id);

CREATE INDEX IF NOT EXISTS idx_tool_invocations_status
    ON tool_invocations(tenant_id, status);

ALTER TABLE tool_invocations ENABLE ROW LEVEL SECURITY;
ALTER TABLE tool_invocations FORCE ROW LEVEL SECURITY;

DROP POLICY IF EXISTS tenant_isolation_tool_invocations ON tool_invocations;
CREATE POLICY tenant_isolation_tool_invocations ON tool_invocations
    USING (tenant_id = current_setting('sentinel.tenant_id', true))
    WITH CHECK (tenant_id = current_setting('sentinel.tenant_id', true));
