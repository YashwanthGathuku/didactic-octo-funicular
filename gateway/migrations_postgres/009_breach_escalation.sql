-- 009_breach_escalation.sql -- PostgreSQL dialect
-- Breach escalation, notifications, and waivers.

CREATE UNIQUE INDEX IF NOT EXISTS idx_incidents_occurrence_type
    ON incidents(tenant_id, expectation_id, type)
    WHERE expectation_id IS NOT NULL;

ALTER TABLE incidents ADD COLUMN IF NOT EXISTS contract_version_id BIGINT REFERENCES file_contract_versions(id);
ALTER TABLE incidents ADD COLUMN IF NOT EXISTS summary TEXT NOT NULL DEFAULT '';
ALTER TABLE incidents ADD COLUMN IF NOT EXISTS owner_subject TEXT NOT NULL DEFAULT '';
ALTER TABLE incidents ADD COLUMN IF NOT EXISTS escalation_policy_id TEXT NOT NULL DEFAULT '';
ALTER TABLE incidents ADD COLUMN IF NOT EXISTS resolved_at TIMESTAMPTZ;
ALTER TABLE incidents ADD COLUMN IF NOT EXISTS resolved_by TEXT;

CREATE INDEX IF NOT EXISTS idx_incidents_open
    ON incidents(tenant_id, status, created_at);

ALTER TABLE notification_intents ADD COLUMN IF NOT EXISTS dedupe_key TEXT NOT NULL DEFAULT '';
ALTER TABLE notification_intents ADD COLUMN IF NOT EXISTS recipient TEXT NOT NULL DEFAULT '';
ALTER TABLE notification_intents ADD COLUMN IF NOT EXISTS escalation_policy_id TEXT NOT NULL DEFAULT '';
ALTER TABLE notification_intents ADD COLUMN IF NOT EXISTS attempt_count INTEGER NOT NULL DEFAULT 0;
ALTER TABLE notification_intents ADD COLUMN IF NOT EXISTS last_error TEXT;

CREATE UNIQUE INDEX IF NOT EXISTS idx_notification_dedupe
    ON notification_intents(tenant_id, dedupe_key)
    WHERE dedupe_key <> '';

CREATE INDEX IF NOT EXISTS idx_notification_undelivered
    ON notification_intents(tenant_id, delivered_at);

ALTER TABLE expectations ADD COLUMN IF NOT EXISTS waived_by TEXT;
ALTER TABLE expectations ADD COLUMN IF NOT EXISTS waived_reason TEXT;
ALTER TABLE expectations ADD COLUMN IF NOT EXISTS waived_at TIMESTAMPTZ;
