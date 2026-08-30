-- 005_redacted_findings.sql -- PostgreSQL dialect
-- Typed, redacted validation findings.

ALTER TABLE validation_findings DROP COLUMN IF EXISTS raw_data;
ALTER TABLE validation_findings ADD COLUMN IF NOT EXISTS provenance TEXT NOT NULL DEFAULT '';
ALTER TABLE validation_findings ADD COLUMN IF NOT EXISTS field_start INTEGER NOT NULL DEFAULT 0;
ALTER TABLE validation_findings ADD COLUMN IF NOT EXISTS field_end INTEGER NOT NULL DEFAULT 0;
ALTER TABLE validation_findings ADD COLUMN IF NOT EXISTS evidence_redacted TEXT NOT NULL DEFAULT '';
ALTER TABLE validation_findings ADD COLUMN IF NOT EXISTS expected_value TEXT NOT NULL DEFAULT '';
ALTER TABLE validation_findings ADD COLUMN IF NOT EXISTS actual_value TEXT NOT NULL DEFAULT '';

ALTER TABLE validation_findings DROP CONSTRAINT IF EXISTS validation_findings_severity_check;
ALTER TABLE validation_findings ADD CONSTRAINT validation_findings_severity_check
    CHECK (severity IN ('INFO','WARNING','BLOCKING'));

CREATE INDEX IF NOT EXISTS idx_findings_tenant ON validation_findings(tenant_id);
CREATE INDEX IF NOT EXISTS idx_findings_artifact ON validation_findings(tenant_id, file_instance_id);

ALTER TABLE validation_findings ENABLE ROW LEVEL SECURITY;
ALTER TABLE validation_findings FORCE ROW LEVEL SECURITY;

DROP POLICY IF EXISTS tenant_isolation_validation_findings ON validation_findings;
CREATE POLICY tenant_isolation_validation_findings ON validation_findings
    USING (tenant_id = current_setting('sentinel.tenant_id', true))
    WITH CHECK (tenant_id = current_setting('sentinel.tenant_id', true));

CREATE OR REPLACE FUNCTION validation_findings_append_only() RETURNS TRIGGER AS $$
BEGIN
    RAISE EXCEPTION 'validation_findings is append-only; re-validate to produce a new finding';
END;
$$ LANGUAGE plpgsql;

DROP TRIGGER IF EXISTS validation_findings_no_update ON validation_findings;
CREATE TRIGGER validation_findings_no_update
    BEFORE UPDATE ON validation_findings
    FOR EACH ROW EXECUTE FUNCTION validation_findings_append_only();
