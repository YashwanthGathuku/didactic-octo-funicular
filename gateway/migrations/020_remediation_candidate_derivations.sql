-- Migration 020: Remediation Candidate Derivations & Attempt Persistence
-- Tracks structured remediation plans, candidate derivations, and attempt histories
-- with immutable parent linkage and strict max-attempt bounds.

CREATE TABLE IF NOT EXISTS remediation_plans (
    id                      TEXT PRIMARY KEY,
    workflow_id             TEXT NOT NULL REFERENCES agent_workflows(id),
    tenant_id               TEXT NOT NULL REFERENCES tenants(id),
    incident_id             INTEGER NOT NULL REFERENCES incidents(id),
    artifact_id             INTEGER NOT NULL REFERENCES file_instances(id),
    expected_parent_sha256  TEXT NOT NULL,
    attempt_number          INTEGER NOT NULL,
    plan_hash               TEXT NOT NULL,
    operations_json         TEXT NOT NULL,
    finding_refs_json       TEXT NOT NULL,
    confidence              TEXT NOT NULL DEFAULT 'HIGH',
    status                  TEXT NOT NULL DEFAULT 'RECEIVED',
    created_at              TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    UNIQUE(tenant_id, workflow_id, attempt_number)
);

CREATE TABLE IF NOT EXISTS artifact_derivations (
    id                      TEXT PRIMARY KEY,
    tenant_id               TEXT NOT NULL REFERENCES tenants(id),
    workflow_id             TEXT NOT NULL REFERENCES agent_workflows(id),
    remediation_plan_id     TEXT NOT NULL REFERENCES remediation_plans(id),
    attempt_number          INTEGER NOT NULL,
    parent_artifact_id      INTEGER NOT NULL REFERENCES file_instances(id),
    parent_sha256           TEXT NOT NULL,
    candidate_artifact_id   INTEGER NOT NULL REFERENCES file_instances(id),
    candidate_sha256        TEXT NOT NULL,
    remediation_plan_hash   TEXT NOT NULL,
    operation_types_json    TEXT NOT NULL,
    agent_name              TEXT NOT NULL DEFAULT 'RemediationAgent',
    agent_version           TEXT NOT NULL DEFAULT '1.0.0',
    policy_decision_id      TEXT,
    policy_decision_hash    TEXT,
    tool_manifest_hash      TEXT,
    validator_version       TEXT NOT NULL DEFAULT '1.0.0',
    validation_run_id       TEXT NOT NULL,
    validation_outcome      TEXT NOT NULL, -- 'VALIDATION_PASSED', 'VALIDATION_FAILED'
    findings_count          INTEGER NOT NULL DEFAULT 0,
    blocking_findings_count INTEGER NOT NULL DEFAULT 0,
    derivation_hash         TEXT NOT NULL,
    created_at              TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    UNIQUE(tenant_id, workflow_id, attempt_number)
);

CREATE INDEX IF NOT EXISTS idx_remediation_plans_workflow ON remediation_plans(tenant_id, workflow_id);
CREATE INDEX IF NOT EXISTS idx_artifact_derivations_parent ON artifact_derivations(tenant_id, parent_artifact_id);
CREATE INDEX IF NOT EXISTS idx_artifact_derivations_candidate ON artifact_derivations(tenant_id, candidate_artifact_id);
