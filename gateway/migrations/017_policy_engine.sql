-- Deterministic Policy Engine (P03/P03.5): versioned policy definitions, bundles, and decisions.
-- Independent of LLM/AI reasoning. Policy decisions are immutable and replayable.

CREATE TABLE IF NOT EXISTS policy_definitions (
    policy_id            TEXT NOT NULL,
    version              INTEGER NOT NULL,
    domain               TEXT NOT NULL,
    layer                TEXT NOT NULL,
    priority             INTEGER NOT NULL DEFAULT 100,
    status               TEXT NOT NULL CHECK (status IN ('DRAFT','ACTIVE','RETIRED')),
    effective_from       TIMESTAMP NOT NULL,
    effective_to         TIMESTAMP,
    tenant_id            TEXT REFERENCES tenants(id),
    partner_id           TEXT,
    action               TEXT NOT NULL,
    subject_constraints  TEXT NOT NULL,
    resource_constraints TEXT NOT NULL,
    conditions           TEXT NOT NULL DEFAULT '{}',
    effect               TEXT NOT NULL CHECK (effect IN ('ALLOW','DENY','ALLOW_WITH_OBLIGATIONS','REQUIRE_HUMAN')),
    obligations          TEXT NOT NULL DEFAULT '[]',
    prohibitions         TEXT NOT NULL DEFAULT '[]',
    reason_code          TEXT NOT NULL,
    source_reference     TEXT,
    created_at           TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    content_hash         TEXT NOT NULL,
    PRIMARY KEY (policy_id, version)
);

CREATE INDEX IF NOT EXISTS idx_policy_defs_lookup ON policy_definitions(status, domain, action);
CREATE INDEX IF NOT EXISTS idx_policy_defs_tenant ON policy_definitions(tenant_id, status);

CREATE TABLE IF NOT EXISTS policy_bundle_versions (
    id              TEXT PRIMARY KEY,
    bundle_name     TEXT NOT NULL,
    version         TEXT NOT NULL,
    bundle_hash     TEXT NOT NULL,
    policies_count  INTEGER NOT NULL,
    created_at      TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    status          TEXT NOT NULL DEFAULT 'ACTIVE'
);

CREATE TABLE IF NOT EXISTS agent_policy_decisions (
    id                     TEXT PRIMARY KEY,
    request_id             TEXT NOT NULL,
    tenant_id              TEXT NOT NULL REFERENCES tenants(id),
    workflow_id            TEXT,
    decision               TEXT NOT NULL CHECK (decision IN ('ALLOW','DENY','ALLOW_WITH_OBLIGATIONS','REQUIRE_HUMAN')),
    action                 TEXT NOT NULL,
    reason_codes           TEXT NOT NULL,
    obligations            TEXT NOT NULL,
    prohibitions           TEXT NOT NULL,
    matched_policy_refs    TEXT NOT NULL,
    policy_bundle_id       TEXT NOT NULL DEFAULT 'bundle-sentinel-default',
    policy_bundle_version  TEXT NOT NULL DEFAULT '1.0.0',
    policy_bundle_hash     TEXT NOT NULL,
    manifest               TEXT NOT NULL DEFAULT '[]',
    evaluated_context_hash TEXT NOT NULL,
    evaluated_at           TIMESTAMP NOT NULL,
    evaluator_version      TEXT NOT NULL,
    decision_hash          TEXT NOT NULL,
    created_at             TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP
);

CREATE INDEX IF NOT EXISTS idx_agent_policy_decisions_req ON agent_policy_decisions(tenant_id, request_id);
CREATE INDEX IF NOT EXISTS idx_agent_policy_decisions_wf ON agent_policy_decisions(tenant_id, workflow_id);
