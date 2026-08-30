-- 014_derived_artifacts.sql -- PostgreSQL dialect
-- Derived Artifacts: remediation workflow creates new artifacts linked to
-- quarantined originals rather than mutating them.

ALTER TABLE file_instances ADD COLUMN IF NOT EXISTS derived_from BIGINT REFERENCES file_instances(id);
ALTER TABLE file_instances ADD COLUMN IF NOT EXISTS derivation_reason TEXT;
ALTER TABLE file_instances ADD COLUMN IF NOT EXISTS derivation_agent_id TEXT;
