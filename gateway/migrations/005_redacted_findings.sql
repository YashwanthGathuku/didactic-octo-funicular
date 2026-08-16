-- Typed, redacted validation findings.
--
-- This migration closes the last place raw financial record content was stored.
-- `validation_findings.raw_data` held the complete 94-character ACH record --
-- account number, routing number, amount and trace number -- and
-- GET /api/v1/incidents selected it and returned it. Every response, log line,
-- support export and AI triage request that touched an incident carried it.
--
-- The engineering contract is explicit: never log or expose raw financial file
-- contents or unredacted account and routing values. That column was a standing
-- violation, and dropping the write path without removing what is already
-- stored would leave the violation in place for existing rows.
--
-- The table is rebuilt rather than altered, for two reasons that both require
-- it. `raw_data` is dropped entirely, so no future query can select it back.
-- And the severity CHECK from migration 002 admits INFO/WARNING/ERROR/CRITICAL/
-- FATAL, a five-level scale where only two levels were ever acted on; the rule
-- set now has three levels with one meaning -- BLOCKING prevents release --
-- which the old constraint would reject.

CREATE TABLE validation_findings_v2 (
    id                INTEGER PRIMARY KEY AUTOINCREMENT,
    tenant_id         TEXT NOT NULL REFERENCES tenants(id),
    file_instance_id  INTEGER NOT NULL REFERENCES file_instances(id),
    validation_run_id INTEGER,

    code              TEXT NOT NULL,
    rule_version      TEXT NOT NULL DEFAULT '',

    -- Where the rule's authority comes from. A finding raised by a rule this
    -- repository cannot verify against a licensed source must be
    -- distinguishable from one it can.
    provenance        TEXT NOT NULL DEFAULT '',

    description       TEXT NOT NULL,

    -- Three levels, one of which has a defined consequence. The five-level
    -- scale it replaces had ERROR, CRITICAL and FATAL all meaning "bad" with
    -- no rule saying which of them stopped a release.
    severity          TEXT NOT NULL CHECK (severity IN ('INFO','WARNING','BLOCKING')),

    -- Location, not content. Together these identify the record and the field
    -- exactly, which is what an operator needs to act.
    line_number       INTEGER,
    byte_offset       INTEGER NOT NULL DEFAULT 0,
    field_start       INTEGER NOT NULL DEFAULT 0,
    field_end         INTEGER NOT NULL DEFAULT 0,

    -- A redacted excerpt. Digits are masked and the length is bounded well
    -- below the 94-character record width, so no combination of findings
    -- reconstructs a line.
    evidence_redacted TEXT NOT NULL DEFAULT '',

    -- The two sides of an arithmetic disagreement. A mismatched batch total is
    -- the finding itself, and a total is not a payment instruction, so these
    -- are stored in full.
    expected_value    TEXT NOT NULL DEFAULT '',
    actual_value      TEXT NOT NULL DEFAULT '',

    created_at        TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP
);

-- Existing findings are carried over without their raw_data.
--
-- Legacy severities collapse upward: ERROR, CRITICAL and FATAL all become
-- BLOCKING. That is the fail-closed direction. A legacy finding whose severity
-- is unrecognisable also becomes BLOCKING rather than being dropped or
-- downgraded -- an unreadable verdict about a financial file is not a pass.
INSERT INTO validation_findings_v2
    (id, tenant_id, file_instance_id, validation_run_id, code, rule_version,
     provenance, description, severity, line_number, byte_offset, created_at)
SELECT
    id, tenant_id, file_instance_id, validation_run_id, code,
    COALESCE(rule_version, ''),
    'LEGACY_PRE_RULE_REGISTRY',
    description,
    CASE severity
        WHEN 'INFO'    THEN 'INFO'
        WHEN 'WARNING' THEN 'WARNING'
        ELSE 'BLOCKING'
    END,
    line_number,
    COALESCE(byte_offset, 0),
    created_at
FROM validation_findings;

DROP TABLE validation_findings;
ALTER TABLE validation_findings_v2 RENAME TO validation_findings;

CREATE INDEX IF NOT EXISTS idx_findings_tenant ON validation_findings(tenant_id);
CREATE INDEX IF NOT EXISTS idx_findings_artifact ON validation_findings(tenant_id, file_instance_id);

-- A finding is a statement about an artifact at a point in time, and a release
-- decision rests on it. Amending one after the fact changes the evidence
-- retrospectively.
CREATE TRIGGER IF NOT EXISTS validation_findings_no_update
BEFORE UPDATE ON validation_findings
BEGIN
    SELECT RAISE(ABORT, 'validation_findings is append-only; re-validate to produce a new finding');
END;
