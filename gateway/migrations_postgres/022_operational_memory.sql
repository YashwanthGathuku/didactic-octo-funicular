-- 022_operational_memory.sql -- PostgreSQL dialect
-- Operational Memory Architecture & Persistence.

CREATE TABLE IF NOT EXISTS operational_memories (
    id                      TEXT PRIMARY KEY,
    tenant_id               TEXT NOT NULL REFERENCES tenants(id),
    memory_type             TEXT NOT NULL CHECK (memory_type IN ('M0_SESSION', 'M1_OPERATIONAL_FACT', 'M2_MANAGED_SEMANTIC', 'M3_STRUCTURED_PROFILE')),
    subject_type            TEXT NOT NULL CHECK (subject_type IN ('INCIDENT', 'PARTNER', 'REMEDIATION_PLAN', 'VALIDATION_RULE', 'TENANT_POLICY', 'FILE_FORMAT', 'ARTIFACT')),
    subject_ref             TEXT NOT NULL,
    fact_type               TEXT NOT NULL CHECK (fact_type IN (
        'VERIFIED_REMEDIATION_SUCCESS',
        'VERIFIED_FAILURE_PATTERN',
        'PARTNER_FILE_FORMAT_TOLERANCE',
        'OPERATIONAL_SLA_BREACH',
        'CANONICAL_RULE_AMENDMENT',
        'HUMAN_INVESTIGATION_OUTCOME',
        'DUAL_CONTROL_RELEASE_OUTCOME'
    )),
    structured_value        TEXT NOT NULL,
    confidence_source       TEXT NOT NULL CHECK (confidence_source IN (
        'DETERMINISTIC_DERIVED',
        'HUMAN_CONFIRMED',
        'VERIFIED_WORKFLOW',
        'MANAGED_MEMORY_SUGGESTION'
    )),
    classification          TEXT NOT NULL DEFAULT 'INTERNAL' CHECK (classification IN ('PUBLIC', 'INTERNAL', 'CONFIDENTIAL', 'RESTRICTED')),
    status                  TEXT NOT NULL DEFAULT 'ACTIVE' CHECK (status IN ('ACTIVE', 'EXPIRED', 'SUPERSEDED', 'INVALIDATED')),
    valid_from              TIMESTAMPTZ NOT NULL DEFAULT now(),
    expires_at              TIMESTAMPTZ,
    superseded_by           TEXT REFERENCES operational_memories(id),
    created_at              TIMESTAMPTZ NOT NULL DEFAULT now(),
    created_by              TEXT NOT NULL,
    memory_hash             TEXT NOT NULL,
    UNIQUE(tenant_id, memory_hash)
);

CREATE INDEX IF NOT EXISTS idx_operational_memories_tenant ON operational_memories(tenant_id);
CREATE INDEX IF NOT EXISTS idx_operational_memories_subject ON operational_memories(tenant_id, subject_type, subject_ref);
CREATE INDEX IF NOT EXISTS idx_operational_memories_fact ON operational_memories(tenant_id, fact_type);
CREATE INDEX IF NOT EXISTS idx_operational_memories_status_valid ON operational_memories(tenant_id, status, valid_from);
CREATE INDEX IF NOT EXISTS idx_operational_memories_hash ON operational_memories(memory_hash);

CREATE TABLE IF NOT EXISTS memory_sources (
    id                      BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    memory_id               TEXT NOT NULL REFERENCES operational_memories(id),
    tenant_id               TEXT NOT NULL REFERENCES tenants(id),
    source_ref              TEXT NOT NULL,
    source_hash             TEXT NOT NULL,
    source_verification_ref TEXT,
    created_at              TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX IF NOT EXISTS idx_memory_sources_mem ON memory_sources(tenant_id, memory_id);
CREATE INDEX IF NOT EXISTS idx_memory_sources_ref ON memory_sources(tenant_id, source_ref);
CREATE INDEX IF NOT EXISTS idx_memory_sources_hash ON memory_sources(source_hash);

CREATE TABLE IF NOT EXISTS memory_revisions (
    id                      TEXT PRIMARY KEY,
    memory_id               TEXT NOT NULL REFERENCES operational_memories(id),
    tenant_id               TEXT NOT NULL REFERENCES tenants(id),
    revision_number         INTEGER NOT NULL,
    previous_hash           TEXT,
    new_hash                TEXT NOT NULL,
    transition_type         TEXT NOT NULL CHECK (transition_type IN ('CREATED', 'SUPERSEDED', 'EXPIRED', 'INVALIDATED')),
    reason                  TEXT NOT NULL,
    actor_id                TEXT NOT NULL,
    created_at              TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE(tenant_id, memory_id, revision_number)
);

CREATE INDEX IF NOT EXISTS idx_memory_revisions_mem ON memory_revisions(tenant_id, memory_id);

ALTER TABLE operational_memories ENABLE ROW LEVEL SECURITY;
ALTER TABLE memory_sources       ENABLE ROW LEVEL SECURITY;
ALTER TABLE memory_revisions     ENABLE ROW LEVEL SECURITY;

ALTER TABLE operational_memories FORCE ROW LEVEL SECURITY;
ALTER TABLE memory_sources       FORCE ROW LEVEL SECURITY;
ALTER TABLE memory_revisions     FORCE ROW LEVEL SECURITY;

DROP POLICY IF EXISTS tenant_isolation_operational_memories ON operational_memories;
CREATE POLICY tenant_isolation_operational_memories ON operational_memories
    USING (tenant_id = current_setting('sentinel.tenant_id', true))
    WITH CHECK (tenant_id = current_setting('sentinel.tenant_id', true));

DROP POLICY IF EXISTS tenant_isolation_memory_sources ON memory_sources;
CREATE POLICY tenant_isolation_memory_sources ON memory_sources
    USING (tenant_id = current_setting('sentinel.tenant_id', true))
    WITH CHECK (tenant_id = current_setting('sentinel.tenant_id', true));

DROP POLICY IF EXISTS tenant_isolation_memory_revisions ON memory_revisions;
CREATE POLICY tenant_isolation_memory_revisions ON memory_revisions
    USING (tenant_id = current_setting('sentinel.tenant_id', true))
    WITH CHECK (tenant_id = current_setting('sentinel.tenant_id', true));

CREATE OR REPLACE FUNCTION operational_memories_core_is_immutable() RETURNS TRIGGER AS $$
BEGIN
    IF NEW.tenant_id         IS DISTINCT FROM OLD.tenant_id
    OR NEW.memory_type       IS DISTINCT FROM OLD.memory_type
    OR NEW.subject_type      IS DISTINCT FROM OLD.subject_type
    OR NEW.subject_ref       IS DISTINCT FROM OLD.subject_ref
    OR NEW.fact_type         IS DISTINCT FROM OLD.fact_type
    OR NEW.structured_value  IS DISTINCT FROM OLD.structured_value
    OR NEW.confidence_source IS DISTINCT FROM OLD.confidence_source
    OR NEW.created_at        IS DISTINCT FROM OLD.created_at
    OR NEW.created_by        IS DISTINCT FROM OLD.created_by
    OR NEW.memory_hash       IS DISTINCT FROM OLD.memory_hash THEN
        RAISE EXCEPTION 'operational_memories core fact data and cryptographic hashes are immutable';
    END IF;
    RETURN NEW;
END;
$$ LANGUAGE plpgsql;

DROP TRIGGER IF EXISTS operational_memories_core_immutable ON operational_memories;
CREATE TRIGGER operational_memories_core_immutable
    BEFORE UPDATE ON operational_memories
    FOR EACH ROW EXECUTE FUNCTION operational_memories_core_is_immutable();

CREATE OR REPLACE FUNCTION memory_sources_append_only() RETURNS TRIGGER AS $$
BEGIN
    RAISE EXCEPTION 'memory_sources is append-only';
END;
$$ LANGUAGE plpgsql;

DROP TRIGGER IF EXISTS memory_sources_no_delete ON memory_sources;
CREATE TRIGGER memory_sources_no_delete
    BEFORE DELETE ON memory_sources
    FOR EACH ROW EXECUTE FUNCTION memory_sources_append_only();

CREATE OR REPLACE FUNCTION memory_revisions_append_only() RETURNS TRIGGER AS $$
BEGIN
    RAISE EXCEPTION 'memory_revisions is append-only';
END;
$$ LANGUAGE plpgsql;

DROP TRIGGER IF EXISTS memory_revisions_no_delete ON memory_revisions;
CREATE TRIGGER memory_revisions_no_delete
    BEFORE DELETE ON memory_revisions
    FOR EACH ROW EXECUTE FUNCTION memory_revisions_append_only();
