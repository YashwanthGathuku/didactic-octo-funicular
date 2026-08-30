-- 007_ledger_integrity.sql -- PostgreSQL dialect
-- Evidence ledger: canonical serialisation and payload integrity.

ALTER TABLE audit_events ADD COLUMN IF NOT EXISTS payload_hash TEXT NOT NULL DEFAULT '';
ALTER TABLE audit_events ADD COLUMN IF NOT EXISTS canonical_version TEXT NOT NULL DEFAULT '';

CREATE INDEX IF NOT EXISTS idx_audit_events_stream
    ON audit_events(tenant_id, sequence_no);

CREATE INDEX IF NOT EXISTS idx_audit_events_correlation
    ON audit_events(tenant_id, correlation_id);

UPDATE audit_events
SET canonical_version = 'ledger-canonical/0-legacy'
WHERE canonical_version = '';
