-- Agent Workflow State: persistent, resumable, tenant-scoped state machine
-- for the AI Agent Control Plane. Completely separate from ArtifactState.

CREATE TABLE IF NOT EXISTS agent_workflows (
    id              TEXT PRIMARY KEY,
    tenant_id       TEXT NOT NULL REFERENCES tenants(id),
    incident_id     INTEGER NOT NULL REFERENCES incidents(id),
    artifact_id     INTEGER NOT NULL REFERENCES file_instances(id),
    artifact_sha256 TEXT NOT NULL,
    state           TEXT NOT NULL CHECK (state IN (
        'PENDING',
        'CONTEXT_BUILDING',
        'INVESTIGATING',
        'PLANNING',
        'REMEDIATING',
        'VALIDATING_CANDIDATE',
        'RETRYING',
        'VERIFIED',
        'UNRESOLVED',
        'HUMAN_REVIEW',
        'COMPLETED',
        'AGENT_UNAVAILABLE',
        'POLICY_DENIED',
        'BUDGET_EXHAUSTED',
        'CANCELLED',
        'FAILED'
    )),
    agent_name      TEXT NOT NULL,
    agent_version   TEXT NOT NULL,
    workflow_type   TEXT NOT NULL DEFAULT 'TRIAGE_AND_REMEDIATION',
    correlation_id  TEXT NOT NULL,
    trace_id        TEXT,
    row_version     INTEGER NOT NULL DEFAULT 0,
    error_detail    TEXT,
    created_at      TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at      TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    started_at      TIMESTAMP,
    completed_at    TIMESTAMP,
    UNIQUE(tenant_id, id)
);

CREATE INDEX IF NOT EXISTS idx_agent_workflows_tenant ON agent_workflows(tenant_id);
CREATE INDEX IF NOT EXISTS idx_agent_workflows_incident ON agent_workflows(tenant_id, incident_id);
CREATE INDEX IF NOT EXISTS idx_agent_workflows_state ON agent_workflows(tenant_id, state);

-- Transition Events: crash-consistent and idempotent domain event journal
CREATE TABLE IF NOT EXISTS agent_workflow_events (
    id              TEXT PRIMARY KEY,
    workflow_id     TEXT NOT NULL REFERENCES agent_workflows(id),
    tenant_id       TEXT NOT NULL REFERENCES tenants(id),
    idempotency_key TEXT NOT NULL,
    event_type      TEXT NOT NULL,
    state_from      TEXT NOT NULL,
    state_to        TEXT NOT NULL,
    row_version     INTEGER NOT NULL,
    payload         TEXT NOT NULL,
    created_at      TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    UNIQUE(tenant_id, workflow_id, idempotency_key)
);

CREATE INDEX IF NOT EXISTS idx_agent_workflow_events_wf ON agent_workflow_events(tenant_id, workflow_id);

CREATE TABLE IF NOT EXISTS agent_runs (
    id                      TEXT PRIMARY KEY,
    workflow_id             TEXT NOT NULL REFERENCES agent_workflows(id),
    tenant_id               TEXT NOT NULL REFERENCES tenants(id),
    agent_name              TEXT NOT NULL,
    agent_version           TEXT NOT NULL,
    provider                TEXT,
    model_name              TEXT,
    model_version           TEXT,
    status                  TEXT NOT NULL CHECK (status IN ('RUNNING', 'COMPLETED', 'FAILED', 'CANCELLED')),
    input_tokens            INTEGER NOT NULL DEFAULT 0,
    output_tokens           INTEGER NOT NULL DEFAULT 0,
    latency_ms              INTEGER NOT NULL DEFAULT 0,
    estimated_cost_microusd BIGINT NOT NULL DEFAULT 0,
    pricing_version         TEXT,
    error_message           TEXT,
    started_at              TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    completed_at            TIMESTAMP
);

CREATE INDEX IF NOT EXISTS idx_agent_runs_workflow ON agent_runs(tenant_id, workflow_id);

-- Structured execution steps (strictly NO private model chain-of-thought persistence)
CREATE TABLE IF NOT EXISTS agent_steps (
    id                       TEXT PRIMARY KEY,
    run_id                   TEXT NOT NULL REFERENCES agent_runs(id),
    workflow_id              TEXT NOT NULL REFERENCES agent_workflows(id),
    tenant_id                TEXT NOT NULL REFERENCES tenants(id),
    step_number              INTEGER NOT NULL,
    step_type                TEXT NOT NULL CHECK (step_type IN (
        'CONTEXT_BUILD',
        'MODEL_INVOCATION',
        'DECISION',
        'TOOL_REQUEST',
        'TOOL_RESULT',
        'HANDOFF',
        'POLICY_CHECK',
        'VALIDATION',
        'VERIFICATION',
        'HUMAN_REVIEW'
    )),
    state_from               TEXT NOT NULL,
    state_to                 TEXT NOT NULL,
    decision_payload         TEXT,
    authorized_evidence_refs TEXT,
    step_status              TEXT NOT NULL DEFAULT 'COMPLETED',
    step_hash                TEXT,
    latency_ms               INTEGER NOT NULL DEFAULT 0,
    created_at               TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP
);

CREATE INDEX IF NOT EXISTS idx_agent_steps_workflow ON agent_steps(tenant_id, workflow_id);

CREATE TABLE IF NOT EXISTS agent_tool_calls (
    id              TEXT PRIMARY KEY,
    step_id         TEXT NOT NULL REFERENCES agent_steps(id),
    workflow_id     TEXT NOT NULL REFERENCES agent_workflows(id),
    tenant_id       TEXT NOT NULL REFERENCES tenants(id),
    tool_name       TEXT NOT NULL,
    tool_scope      TEXT NOT NULL CHECK (tool_scope IN ('READ', 'WRITE')),
    input_redacted  TEXT NOT NULL,
    output_redacted TEXT NOT NULL,
    is_error        INTEGER NOT NULL DEFAULT 0 CHECK (is_error IN (0,1)),
    latency_ms      INTEGER NOT NULL DEFAULT 0,
    executed_at     TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP
);

CREATE INDEX IF NOT EXISTS idx_agent_tool_calls_workflow ON agent_tool_calls(tenant_id, workflow_id);

CREATE TABLE IF NOT EXISTS verification_attestations (
    id                      TEXT PRIMARY KEY,
    workflow_id             TEXT NOT NULL REFERENCES agent_workflows(id),
    tenant_id               TEXT NOT NULL REFERENCES tenants(id),
    verifier_agent          TEXT NOT NULL,
    candidate_artifact_id   INTEGER REFERENCES file_instances(id),
    candidate_sha256        TEXT NOT NULL,
    findings_count          INTEGER NOT NULL DEFAULT 0,
    blocking_findings_count INTEGER NOT NULL DEFAULT 0,
    status                  TEXT NOT NULL CHECK (status IN ('CONFIRMED', 'DISPUTED', 'PARTIAL')),
    attestation_digest      TEXT NOT NULL,
    created_at              TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP
);

CREATE INDEX IF NOT EXISTS idx_verification_attestations_workflow ON verification_attestations(tenant_id, workflow_id);
