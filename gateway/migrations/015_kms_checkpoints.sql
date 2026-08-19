-- Cloud KMS Signed Evidence Checkpoints: periodic asymmetric signatures
-- on ledger checkpoint digests for external non-repudiation.

CREATE TABLE IF NOT EXISTS ledger_checkpoints (
    id              TEXT PRIMARY KEY,
    tenant_id       TEXT NOT NULL,
    sequence_start  INTEGER NOT NULL,
    sequence_end    INTEGER NOT NULL,
    entries_digest  TEXT NOT NULL,
    kms_key_version TEXT NOT NULL,
    signature       BLOB NOT NULL,
    algorithm       TEXT NOT NULL,
    created_at      TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    UNIQUE(tenant_id, sequence_end)
);
