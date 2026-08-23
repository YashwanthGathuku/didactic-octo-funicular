-- Migration 022: Operational Memory Architecture & Persistence
-- Authoritative, Go-owned M1 operational memory store with cryptographic provenance,
-- immutable source linkage, and audit revision ledger.

-- 1. Master Operational Memories Table (M1 Operational Facts)
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
    valid_from              TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    expires_at              TIMESTAMP,
    superseded_by           TEXT REFERENCES operational_memories(id),
    created_at              TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    created_by              TEXT NOT NULL,
    memory_hash             TEXT NOT NULL,
    UNIQUE(tenant_id, memory_hash)
);

-- Performance & Multitenant Isolation Indexes
CREATE INDEX IF NOT EXISTS idx_operational_memories_tenant ON operational_memories(tenant_id);
CREATE INDEX IF NOT EXISTS idx_operational_memories_subject ON operational_memories(tenant_id, subject_type, subject_ref);
CREATE INDEX IF NOT EXISTS idx_operational_memories_fact ON operational_memories(tenant_id, fact_type);
CREATE INDEX IF NOT EXISTS idx_operational_memories_status_valid ON operational_memories(tenant_id, status, valid_from);
CREATE INDEX IF NOT EXISTS idx_operational_memories_hash ON operational_memories(memory_hash);

-- 2. Memory Provenance Sources Table
CREATE TABLE IF NOT EXISTS memory_sources (
    id                      INTEGER PRIMARY KEY AUTOINCREMENT,
    memory_id               TEXT NOT NULL REFERENCES operational_memories(id),
    tenant_id               TEXT NOT NULL REFERENCES tenants(id),
    source_ref              TEXT NOT NULL,
    source_hash             TEXT NOT NULL,
    source_verification_ref TEXT,
    created_at              TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP
);

CREATE INDEX IF NOT EXISTS idx_memory_sources_mem ON memory_sources(tenant_id, memory_id);
CREATE INDEX IF NOT EXISTS idx_memory_sources_ref ON memory_sources(tenant_id, source_ref);
CREATE INDEX IF NOT EXISTS idx_memory_sources_hash ON memory_sources(source_hash);

-- 3. Memory Revision Audit Ledger
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
    created_at              TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    UNIQUE(tenant_id, memory_id, revision_number)
);

CREATE INDEX IF NOT EXISTS idx_memory_revisions_mem ON memory_revisions(tenant_id, memory_id);

-- 4. Invariant Triggers
-- Prevent in-place mutation of immutable core fact data (only lifecycle fields: status, expires_at, superseded_by may transition)
CREATE TRIGGER IF NOT EXISTS operational_memories_core_immutable
BEFORE UPDATE OF tenant_id, memory_type, subject_type, subject_ref, fact_type, structured_value, confidence_source, created_at, created_by, memory_hash
ON operational_memories
BEGIN
    SELECT RAISE(ABORT, 'operational_memories core fact data and cryptographic hashes are immutable');
END;

-- Ensure memory_sources is append-only
CREATE TRIGGER IF NOT EXISTS memory_sources_no_delete
BEFORE DELETE ON memory_sources
BEGIN
    SELECT RAISE(ABORT, 'memory_sources is append-only');
END;

-- Ensure memory_revisions is append-only
CREATE TRIGGER IF NOT EXISTS memory_revisions_no_delete
BEFORE DELETE ON memory_revisions
BEGIN
    SELECT RAISE(ABORT, 'memory_revisions is append-only');
END;
