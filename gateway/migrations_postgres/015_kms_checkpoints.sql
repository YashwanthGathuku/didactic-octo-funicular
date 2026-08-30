-- 015_kms_checkpoints.sql -- PostgreSQL dialect
-- Cloud KMS Signed Evidence Checkpoints: periodic asymmetric signatures
-- on ledger checkpoint digests for external non-repudiation.

CREATE TABLE IF NOT EXISTS ledger_checkpoints (
    id              TEXT PRIMARY KEY,
    tenant_id       TEXT NOT NULL REFERENCES tenants(id),
    sequence_start  BIGINT NOT NULL,
    sequence_end    BIGINT NOT NULL,
    entries_digest  TEXT NOT NULL,
    kms_key_version TEXT NOT NULL,
    signature       BYTEA NOT NULL,
    algorithm       TEXT NOT NULL,
    created_at      TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE(tenant_id, sequence_end)
);

CREATE INDEX IF NOT EXISTS idx_ledger_checkpoints_tenant ON ledger_checkpoints(tenant_id);

ALTER TABLE ledger_checkpoints ENABLE ROW LEVEL SECURITY;
ALTER TABLE ledger_checkpoints FORCE ROW LEVEL SECURITY;

DROP POLICY IF EXISTS tenant_isolation_ledger_checkpoints ON ledger_checkpoints;
CREATE POLICY tenant_isolation_ledger_checkpoints ON ledger_checkpoints
    USING (tenant_id = current_setting('sentinel.tenant_id', true))
    WITH CHECK (tenant_id = current_setting('sentinel.tenant_id', true));
