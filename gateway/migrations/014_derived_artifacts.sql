-- Derived Artifacts: remediation workflow creates new artifacts linked to
-- quarantined originals rather than mutating them.

ALTER TABLE file_instances ADD COLUMN derived_from TEXT REFERENCES file_instances(id);
ALTER TABLE file_instances ADD COLUMN derivation_reason TEXT;
ALTER TABLE file_instances ADD COLUMN derivation_agent_id TEXT;
