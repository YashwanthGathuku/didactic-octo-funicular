-- Migration 021: Candidate Verifications, Verification Checks & Critic Assessments
-- Records independent, authoritative verification results, check outcomes, and critic assessments for remediation candidates.

CREATE TABLE IF NOT EXISTS candidate_verifications (
    id                      TEXT PRIMARY KEY,
    tenant_id               TEXT NOT NULL REFERENCES tenants(id),
    workflow_id             TEXT NOT NULL REFERENCES agent_workflows(id),
    candidate_artifact_id   INTEGER NOT NULL REFERENCES file_instances(id),
    candidate_sha256        TEXT NOT NULL,
    parent_artifact_id      INTEGER NOT NULL REFERENCES file_instances(id),
    parent_sha256           TEXT NOT NULL,
    derivation_id           TEXT NOT NULL REFERENCES artifact_derivations(id),
    derivation_hash         TEXT NOT NULL,
    remediation_plan_hash   TEXT NOT NULL,
    p07_validation_run_id   TEXT NOT NULL,
    p08_validation_run_id   TEXT NOT NULL,
    validator_version       TEXT NOT NULL DEFAULT '1.0.0',
    rulepack_hash           TEXT NOT NULL,
    policy_bundle_hash      TEXT NOT NULL,
    deterministic_outcome   TEXT NOT NULL, -- 'PASS', 'FAIL', 'STALE', 'CORRUPTION_DETECTED', 'ERROR'
    verification_hash       TEXT NOT NULL,
    created_at              TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    UNIQUE(tenant_id, workflow_id, candidate_artifact_id)
);

CREATE TABLE IF NOT EXISTS verification_checks (
    id                      INTEGER PRIMARY KEY AUTOINCREMENT,
    verification_id         TEXT NOT NULL REFERENCES candidate_verifications(id),
    tenant_id               TEXT NOT NULL REFERENCES tenants(id),
    check_type              TEXT NOT NULL,
    passed                  INTEGER NOT NULL CHECK(passed IN (0, 1)),
    message                 TEXT NOT NULL,
    expected_value          TEXT NOT NULL,
    actual_value            TEXT NOT NULL,
    created_at              TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP
);

CREATE TABLE IF NOT EXISTS critic_assessments (
    id                      TEXT PRIMARY KEY,
    tenant_id               TEXT NOT NULL REFERENCES tenants(id),
    workflow_id             TEXT NOT NULL REFERENCES agent_workflows(id),
    candidate_artifact_id   INTEGER NOT NULL REFERENCES file_instances(id),
    verifier_agent          TEXT NOT NULL,
    assessment_status       TEXT NOT NULL,
    risk_level              TEXT NOT NULL DEFAULT 'LOW',
    reasoning               TEXT NOT NULL,
    assessment_hash         TEXT NOT NULL,
    created_at              TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP
);

CREATE INDEX IF NOT EXISTS idx_candidate_verifications_wf ON candidate_verifications(tenant_id, workflow_id);
CREATE INDEX IF NOT EXISTS idx_candidate_verifications_cand ON candidate_verifications(tenant_id, candidate_artifact_id);
CREATE INDEX IF NOT EXISTS idx_verification_checks_verif ON verification_checks(tenant_id, verification_id);
CREATE INDEX IF NOT EXISTS idx_critic_assessments_wf ON critic_assessments(tenant_id, workflow_id);
