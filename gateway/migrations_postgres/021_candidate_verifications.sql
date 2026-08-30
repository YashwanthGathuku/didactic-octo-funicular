-- 021_candidate_verifications.sql -- PostgreSQL dialect
-- Candidate Verifications, Verification Checks & Critic Assessments.

CREATE TABLE IF NOT EXISTS candidate_verifications (
    id                      TEXT PRIMARY KEY,
    tenant_id               TEXT NOT NULL REFERENCES tenants(id),
    workflow_id             TEXT NOT NULL REFERENCES agent_workflows(id),
    candidate_artifact_id   BIGINT NOT NULL REFERENCES file_instances(id),
    candidate_sha256        TEXT NOT NULL,
    parent_artifact_id      BIGINT NOT NULL REFERENCES file_instances(id),
    parent_sha256           TEXT NOT NULL,
    derivation_id           TEXT NOT NULL REFERENCES artifact_derivations(id),
    derivation_hash         TEXT NOT NULL,
    remediation_plan_hash   TEXT NOT NULL,
    p07_validation_run_id   TEXT NOT NULL,
    p08_validation_run_id   TEXT NOT NULL,
    validator_version       TEXT NOT NULL DEFAULT '1.0.0',
    rulepack_hash           TEXT NOT NULL,
    policy_bundle_hash      TEXT NOT NULL,
    deterministic_outcome   TEXT NOT NULL,
    verification_hash       TEXT NOT NULL,
    created_at              TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE(tenant_id, workflow_id, candidate_artifact_id)
);

CREATE INDEX IF NOT EXISTS idx_candidate_verifications_wf
    ON candidate_verifications(tenant_id, workflow_id);

CREATE INDEX IF NOT EXISTS idx_candidate_verifications_cand
    ON candidate_verifications(tenant_id, candidate_artifact_id);

CREATE TABLE IF NOT EXISTS verification_checks (
    id                      BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    verification_id         TEXT NOT NULL REFERENCES candidate_verifications(id),
    tenant_id               TEXT NOT NULL REFERENCES tenants(id),
    check_type              TEXT NOT NULL,
    passed                  INTEGER NOT NULL CHECK(passed IN (0, 1)),
    message                 TEXT NOT NULL,
    expected_value          TEXT NOT NULL,
    actual_value            TEXT NOT NULL,
    created_at              TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX IF NOT EXISTS idx_verification_checks_verif
    ON verification_checks(tenant_id, verification_id);

CREATE TABLE IF NOT EXISTS critic_assessments (
    id                      TEXT PRIMARY KEY,
    tenant_id               TEXT NOT NULL REFERENCES tenants(id),
    workflow_id             TEXT NOT NULL REFERENCES agent_workflows(id),
    candidate_artifact_id   BIGINT NOT NULL REFERENCES file_instances(id),
    verifier_agent          TEXT NOT NULL,
    assessment_status       TEXT NOT NULL,
    risk_level              TEXT NOT NULL DEFAULT 'LOW',
    reasoning               TEXT NOT NULL,
    assessment_hash         TEXT NOT NULL,
    created_at              TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX IF NOT EXISTS idx_critic_assessments_wf
    ON critic_assessments(tenant_id, workflow_id);

ALTER TABLE candidate_verifications ENABLE ROW LEVEL SECURITY;
ALTER TABLE verification_checks     ENABLE ROW LEVEL SECURITY;
ALTER TABLE critic_assessments      ENABLE ROW LEVEL SECURITY;

ALTER TABLE candidate_verifications FORCE ROW LEVEL SECURITY;
ALTER TABLE verification_checks     FORCE ROW LEVEL SECURITY;
ALTER TABLE critic_assessments      FORCE ROW LEVEL SECURITY;

DROP POLICY IF EXISTS tenant_isolation_candidate_verifications ON candidate_verifications;
CREATE POLICY tenant_isolation_candidate_verifications ON candidate_verifications
    USING (tenant_id = current_setting('sentinel.tenant_id', true))
    WITH CHECK (tenant_id = current_setting('sentinel.tenant_id', true));

DROP POLICY IF EXISTS tenant_isolation_verification_checks ON verification_checks;
CREATE POLICY tenant_isolation_verification_checks ON verification_checks
    USING (tenant_id = current_setting('sentinel.tenant_id', true))
    WITH CHECK (tenant_id = current_setting('sentinel.tenant_id', true));

DROP POLICY IF EXISTS tenant_isolation_critic_assessments ON critic_assessments;
CREATE POLICY tenant_isolation_critic_assessments ON critic_assessments
    USING (tenant_id = current_setting('sentinel.tenant_id', true))
    WITH CHECK (tenant_id = current_setting('sentinel.tenant_id', true));
