-- PostgreSQL schema with row-level security.
--
-- This is the dialect-specific counterpart to the SQLite migrations. It exists
-- for one reason SQLite cannot provide: RLS makes tenant isolation a property
-- the database enforces, independent of whether application code remembers to
-- add a WHERE clause.
--
-- The application connects as a NOSUPERUSER role and sets
-- `sentinel.tenant_id` per transaction. Every policy below filters on that
-- setting, so a query with no tenant set returns nothing rather than
-- everything -- the failure mode is starvation, not disclosure.
--
-- Superusers and table owners bypass RLS by default, which is why the
-- application role must own neither.

CREATE TABLE IF NOT EXISTS tenants (
    id          TEXT PRIMARY KEY,
    name        TEXT NOT NULL,
    created_at  TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE IF NOT EXISTS partners (
    id              BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    tenant_id       TEXT NOT NULL REFERENCES tenants(id),
    name            TEXT NOT NULL,
    routing_number  TEXT NOT NULL,
    created_at      TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE (tenant_id, routing_number)
);

CREATE TABLE IF NOT EXISTS file_instances (
    id              BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    tenant_id       TEXT NOT NULL REFERENCES tenants(id),
    filename        TEXT NOT NULL,
    storage_path    TEXT NOT NULL,
    size_bytes      BIGINT NOT NULL CHECK (size_bytes >= 0),
    sha256_hash     TEXT NOT NULL,
    status          TEXT NOT NULL
        CHECK (status IN ('RECEIVED','VALIDATING','VALIDATED','QUARANTINED','APPROVED','RELEASED','REJECTED')),
    derived_from_id BIGINT REFERENCES file_instances(id),
    row_version     INTEGER NOT NULL DEFAULT 0,
    received_at     TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at      TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE IF NOT EXISTS incidents (
    id                BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    tenant_id         TEXT NOT NULL REFERENCES tenants(id),
    file_instance_id  BIGINT REFERENCES file_instances(id),
    type              TEXT NOT NULL,
    severity          TEXT NOT NULL,
    status            TEXT NOT NULL CHECK (status IN ('OPEN','INVESTIGATING','RESOLVED','CLOSED')),
    created_at        TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at        TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE IF NOT EXISTS audit_events (
    id             BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    tenant_id      TEXT NOT NULL REFERENCES tenants(id),
    sequence_no    BIGINT NOT NULL,
    event_type     TEXT NOT NULL,
    actor          TEXT NOT NULL,
    payload        TEXT NOT NULL,
    previous_hash  TEXT NOT NULL,
    current_hash   TEXT NOT NULL,
    created_at     TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE (tenant_id, sequence_no),
    UNIQUE (tenant_id, previous_hash)
);

-- ---------------------------------------------------------------------------
-- Row-level security
-- ---------------------------------------------------------------------------
--
-- current_setting(..., true) returns NULL when unset rather than raising, and
-- a NULL comparison is false, so an unset tenant matches no rows. That is the
-- desired failure: a forgotten scope yields nothing.

ALTER TABLE partners       ENABLE ROW LEVEL SECURITY;
ALTER TABLE file_instances ENABLE ROW LEVEL SECURITY;
ALTER TABLE incidents      ENABLE ROW LEVEL SECURITY;
ALTER TABLE audit_events   ENABLE ROW LEVEL SECURITY;

-- FORCE applies the policy to the table owner too, so ownership is not an
-- accidental bypass.
ALTER TABLE partners       FORCE ROW LEVEL SECURITY;
ALTER TABLE file_instances FORCE ROW LEVEL SECURITY;
ALTER TABLE incidents      FORCE ROW LEVEL SECURITY;
ALTER TABLE audit_events   FORCE ROW LEVEL SECURITY;

CREATE POLICY tenant_isolation_partners ON partners
    USING (tenant_id = current_setting('sentinel.tenant_id', true))
    WITH CHECK (tenant_id = current_setting('sentinel.tenant_id', true));

CREATE POLICY tenant_isolation_file_instances ON file_instances
    USING (tenant_id = current_setting('sentinel.tenant_id', true))
    WITH CHECK (tenant_id = current_setting('sentinel.tenant_id', true));

CREATE POLICY tenant_isolation_incidents ON incidents
    USING (tenant_id = current_setting('sentinel.tenant_id', true))
    WITH CHECK (tenant_id = current_setting('sentinel.tenant_id', true));

CREATE POLICY tenant_isolation_audit_events ON audit_events
    USING (tenant_id = current_setting('sentinel.tenant_id', true))
    WITH CHECK (tenant_id = current_setting('sentinel.tenant_id', true));

-- WITH CHECK on every policy is what stops a write into another tenant. USING
-- alone would filter reads while still permitting an INSERT that names a
-- foreign tenant_id.

-- The tenants table itself is readable by the application so it can resolve
-- names, but never writable by it.
GRANT SELECT ON tenants TO PUBLIC;

-- ---------------------------------------------------------------------------
-- Credential storage
-- ---------------------------------------------------------------------------
--
-- The PostgreSQL counterpart of migrations/003_secret_store.sql. It carries the
-- same guarantee -- no column holds a credential in a readable form -- and adds
-- the one SQLite cannot: row-level security, so a query that forgets its tenant
-- returns nothing rather than every tenant's credential metadata.

CREATE TABLE IF NOT EXISTS secret_versions (
    id            BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    secret_id     TEXT NOT NULL,
    tenant_id     TEXT NOT NULL,
    name          TEXT NOT NULL,
    kind          TEXT NOT NULL CHECK (kind IN ('VERIFY','RETRIEVE')),
    version       INTEGER NOT NULL CHECK (version >= 1),
    fingerprint   TEXT NOT NULL,

    -- KindVerify: one-way. KindRetrieve: ciphertext under a key held outside
    -- this database.
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
    tenant_id    TEXT NOT NULL,
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

CREATE POLICY tenant_isolation_secret_versions ON secret_versions
    USING (tenant_id = current_setting('sentinel.tenant_id', true))
    WITH CHECK (tenant_id = current_setting('sentinel.tenant_id', true));

CREATE POLICY tenant_isolation_secret_events ON secret_events
    USING (tenant_id = current_setting('sentinel.tenant_id', true))
    WITH CHECK (tenant_id = current_setting('sentinel.tenant_id', true));

-- Append-only enforcement, and immutability of credential material.
--
-- SQLite expresses these as BEFORE triggers with RAISE(ABORT); PostgreSQL needs
-- a trigger function. The property enforced is identical: rotation appends a
-- version, it never rewrites one, and the evidence of a rotation cannot be
-- edited afterwards.
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
