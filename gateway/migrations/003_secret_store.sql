-- Credential storage.
--
-- The governing property is that no column in this file can hold a credential
-- in a form anything can read. A KindVerify secret is a salted digest, which is
-- one-way. A KindRetrieve secret is AES-256-GCM ciphertext under a key that is
-- supplied through the environment and never written to this database, so a
-- backup, a replica, or a read-only reporting user yields ciphertext alone.
--
-- The removed webhook subsystem stored its signing secrets as plain text in a
-- column that a SQL console endpoint would happily select. That is the failure
-- this schema is shaped to prevent.

CREATE TABLE IF NOT EXISTS secret_versions (
    id            INTEGER PRIMARY KEY AUTOINCREMENT,
    secret_id     TEXT NOT NULL,
    tenant_id     TEXT NOT NULL,
    name          TEXT NOT NULL,
    kind          TEXT NOT NULL CHECK (kind IN ('VERIFY','RETRIEVE')),
    version       INTEGER NOT NULL CHECK (version >= 1),

    -- A non-reversible handle used to correlate audit records with the
    -- credential they concern. Safe to read; safe to export.
    fingerprint   TEXT NOT NULL,

    -- Exactly one of these two representations is populated, enforced below.
    -- KindVerify: salt + digest, one-way.
    salt          BLOB,
    digest        BLOB,
    -- KindRetrieve: ciphertext, plus the id of the key that sealed it so a key
    -- rotation can identify which rows still need re-sealing.
    sealed        BLOB,
    key_id        TEXT,

    created_at    TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    created_by    TEXT NOT NULL,
    rotated_at    TIMESTAMP,
    last_used_at  TIMESTAMP,

    -- Set on a superseded version for the duration of its rotation overlap.
    not_after     TIMESTAMP,
    retired_at    TIMESTAMP,

    -- One row per version of a secret, and one secret per name per tenant.
    UNIQUE (secret_id, version),
    UNIQUE (tenant_id, name, version),

    -- A row must be one kind or the other, fully populated, never both and
    -- never neither. Without this a bug could write a NULL digest and produce a
    -- version that verifies nothing while appearing active.
    CHECK (
        (kind = 'VERIFY'   AND salt IS NOT NULL AND digest IS NOT NULL
                           AND sealed IS NULL AND key_id IS NULL)
     OR (kind = 'RETRIEVE' AND sealed IS NOT NULL AND key_id IS NOT NULL
                           AND salt IS NULL AND digest IS NULL)
    )
);

CREATE INDEX IF NOT EXISTS idx_secret_versions_lookup
    ON secret_versions(tenant_id, name, version);

-- Rotation evidence. An auditor asking when a credential last changed, and by
-- whom, must be answerable without anyone handling the credential -- which is
-- why this table records a fingerprint and never a value.
CREATE TABLE IF NOT EXISTS secret_events (
    id           INTEGER PRIMARY KEY AUTOINCREMENT,
    tenant_id    TEXT NOT NULL,
    secret_id    TEXT NOT NULL,
    name         TEXT NOT NULL,
    version      INTEGER NOT NULL,
    action       TEXT NOT NULL CHECK (action IN (
                     'SECRET_CREATED','SECRET_ROTATED','SECRET_RETIRED',
                     'SECRET_USED','SECRET_VERIFIED','SECRET_VERIFY_REJECTED')),
    actor        TEXT NOT NULL,
    fingerprint  TEXT NOT NULL DEFAULT '',
    created_at   TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP
);

CREATE INDEX IF NOT EXISTS idx_secret_events_subject
    ON secret_events(tenant_id, name, id);

-- Append-only, for the same reason status_history and audit_events are: a
-- rotation record that can be edited is not evidence of anything.
CREATE TRIGGER IF NOT EXISTS secret_events_no_update
BEFORE UPDATE ON secret_events
BEGIN
    SELECT RAISE(ABORT, 'secret_events is append-only');
END;

CREATE TRIGGER IF NOT EXISTS secret_events_no_delete
BEFORE DELETE ON secret_events
BEGIN
    SELECT RAISE(ABORT, 'secret_events is append-only');
END;

-- A stored credential is never rewritten in place. Rotation appends a new
-- version; only the mutable lifecycle columns may change on an existing row.
-- Without this, "rotation" could be implemented as an UPDATE that destroys the
-- record of what the previous credential was and when it was valid.
CREATE TRIGGER IF NOT EXISTS secret_versions_material_is_immutable
BEFORE UPDATE OF secret_id, tenant_id, name, kind, version, fingerprint,
                 salt, digest, sealed, key_id, created_at, created_by
ON secret_versions
BEGIN
    SELECT RAISE(ABORT, 'secret material is immutable; rotate to create a new version');
END;
