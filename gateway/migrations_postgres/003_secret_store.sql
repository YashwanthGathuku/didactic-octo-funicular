-- 003_secret_store.sql -- PostgreSQL dialect
-- Credential storage and audit trail with Row-Level Security.

CREATE TABLE IF NOT EXISTS secret_versions (
    id            BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    secret_id     TEXT NOT NULL,
    tenant_id     TEXT NOT NULL REFERENCES tenants(id),
    name          TEXT NOT NULL,
    kind          TEXT NOT NULL CHECK (kind IN ('VERIFY','RETRIEVE')),
    version       INTEGER NOT NULL CHECK (version >= 1),
    fingerprint   TEXT NOT NULL,

    -- KindVerify: one-way. KindRetrieve: ciphertext under a key held outside this database.
    salt          BYTEA,
    digest        BYTEA,
    sealed        BYTEA,
    key_id        TEXT,

    created_at    TIMESTAMPTZ NOT NULL DEFAULT now(),
    created_by    TEXT NOT NULL,
    rotated_at    TIMESTAMPTZ,
    last_used_at  TIMESTAMPTZ,
    not_after     TIMESTAMPTZ,
    retired_at    TIMESTAMPTZ,

    UNIQUE (secret_id, version),
    UNIQUE (tenant_id, name, version),

    CHECK (
        (kind = 'VERIFY'   AND salt IS NOT NULL AND digest IS NOT NULL
                           AND sealed IS NULL AND key_id IS NULL)
     OR (kind = 'RETRIEVE' AND sealed IS NOT NULL AND key_id IS NOT NULL
                           AND salt IS NULL AND digest IS NULL)
    )
);

CREATE INDEX IF NOT EXISTS idx_secret_versions_lookup
    ON secret_versions(tenant_id, name, version);

CREATE TABLE IF NOT EXISTS secret_events (
    id           BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    tenant_id    TEXT NOT NULL REFERENCES tenants(id),
    secret_id    TEXT NOT NULL,
    name         TEXT NOT NULL,
    version      INTEGER NOT NULL,
    action       TEXT NOT NULL CHECK (action IN (
                     'SECRET_CREATED','SECRET_ROTATED','SECRET_RETIRED',
                     'SECRET_USED','SECRET_VERIFIED','SECRET_VERIFY_REJECTED')),
    actor        TEXT NOT NULL,
    fingerprint  TEXT NOT NULL DEFAULT '',
    created_at   TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX IF NOT EXISTS idx_secret_events_subject
    ON secret_events(tenant_id, name, id);

ALTER TABLE secret_versions ENABLE ROW LEVEL SECURITY;
ALTER TABLE secret_events   ENABLE ROW LEVEL SECURITY;

ALTER TABLE secret_versions FORCE ROW LEVEL SECURITY;
ALTER TABLE secret_events   FORCE ROW LEVEL SECURITY;

DROP POLICY IF EXISTS tenant_isolation_secret_versions ON secret_versions;
CREATE POLICY tenant_isolation_secret_versions ON secret_versions
    USING (tenant_id = current_setting('sentinel.tenant_id', true))
    WITH CHECK (tenant_id = current_setting('sentinel.tenant_id', true));

DROP POLICY IF EXISTS tenant_isolation_secret_events ON secret_events;
CREATE POLICY tenant_isolation_secret_events ON secret_events
    USING (tenant_id = current_setting('sentinel.tenant_id', true))
    WITH CHECK (tenant_id = current_setting('sentinel.tenant_id', true));

CREATE OR REPLACE FUNCTION secret_events_append_only() RETURNS TRIGGER AS $$
BEGIN
    RAISE EXCEPTION 'secret_events is append-only';
END;
$$ LANGUAGE plpgsql;

DROP TRIGGER IF EXISTS secret_events_no_change ON secret_events;
CREATE TRIGGER secret_events_no_change
    BEFORE UPDATE OR DELETE ON secret_events
    FOR EACH ROW EXECUTE FUNCTION secret_events_append_only();

CREATE OR REPLACE FUNCTION secret_material_is_immutable() RETURNS TRIGGER AS $$
BEGIN
    IF NEW.secret_id   IS DISTINCT FROM OLD.secret_id
    OR NEW.tenant_id   IS DISTINCT FROM OLD.tenant_id
    OR NEW.name        IS DISTINCT FROM OLD.name
    OR NEW.kind        IS DISTINCT FROM OLD.kind
    OR NEW.version     IS DISTINCT FROM OLD.version
    OR NEW.fingerprint IS DISTINCT FROM OLD.fingerprint
    OR NEW.salt        IS DISTINCT FROM OLD.salt
    OR NEW.digest      IS DISTINCT FROM OLD.digest
    OR NEW.sealed      IS DISTINCT FROM OLD.sealed
    OR NEW.key_id      IS DISTINCT FROM OLD.key_id
    OR NEW.created_at  IS DISTINCT FROM OLD.created_at
    OR NEW.created_by  IS DISTINCT FROM OLD.created_by THEN
        RAISE EXCEPTION 'secret material is immutable; rotate to create a new version';
    END IF;
    RETURN NEW;
END;
$$ LANGUAGE plpgsql;

DROP TRIGGER IF EXISTS secret_versions_material_immutable ON secret_versions;
CREATE TRIGGER secret_versions_material_immutable
    BEFORE UPDATE ON secret_versions
    FOR EACH ROW EXECUTE FUNCTION secret_material_is_immutable();
