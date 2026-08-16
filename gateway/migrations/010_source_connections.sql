-- Storing a customer source connection.
--
-- Stage 16.1 through 16.4 built the catalog, the descriptors, the validation,
-- the conformance suite and one real driver, and left the persistence between
-- them unbuilt -- so nothing was reachable. This is that layer.
--
-- The shape is dictated by one rule: a credential never lands in this schema.
-- Non-secret configuration goes in a column; every secret goes to the Prompt 05
-- secret store and only its *name* is recorded here. The Integration Hub this
-- replaces kept the webhook secret in the same table as everything else, which
-- is why three separate read paths returned it.

CREATE TABLE IF NOT EXISTS source_connections (
    id              INTEGER PRIMARY KEY AUTOINCREMENT,
    tenant_id       TEXT NOT NULL REFERENCES tenants(id),

    -- The catalog entry this connection is an instance of.
    connector_type  TEXT NOT NULL,
    display_name    TEXT NOT NULL,
    auth_mode       TEXT NOT NULL,

    -- The non-secret descriptor fields, as a JSON object keyed by field id.
    --
    -- JSON rather than a column per field because the field set is per
    -- connector and server-owned: adding Oracle would otherwise be a migration,
    -- and a migration per connector is how a catalog stops growing. Nothing
    -- here is a credential -- the write-only fields never reach this column,
    -- and a test asserts it.
    config_json     TEXT NOT NULL,

    -- The approved schemas, datasets or catalogs, one per row of a JSON array.
    -- Empty is refused at write time: a connection with no allowlist would read
    -- the whole database.
    resource_allowlist TEXT NOT NULL,

    -- Per-connection bounds, never above the platform's.
    max_rows        INTEGER NOT NULL DEFAULT 0,
    max_bytes       INTEGER NOT NULL DEFAULT 0,
    timeout_seconds INTEGER NOT NULL DEFAULT 0,

    -- Executions per minute. Per-execution bounds stop one query being
    -- unbounded; this stops an unbounded number of bounded queries.
    max_per_minute  INTEGER NOT NULL DEFAULT 60,

    -- Health, from real checks only.
    --
    -- NEVER_CHECKED is the default and is a distinct state from healthy. A UI
    -- that shows an untested connection as green is the defect this whole
    -- subsystem exists to prevent, so the schema refuses to let the two share
    -- a value.
    health_state    TEXT NOT NULL DEFAULT 'NEVER_CHECKED'
                      CHECK (health_state IN ('NEVER_CHECKED','HEALTHY','DEGRADED','FAILED')),
    health_checked_at   TIMESTAMP,
    health_error_class  TEXT,
    health_detail       TEXT,
    health_latency_ms   INTEGER,

    -- Which conformance evidence was in force when this connection was
    -- created. An operator reviewing an old connection needs to know what the
    -- driver had been verified against at the time, not what it claims now.
    conformance_commit  TEXT NOT NULL DEFAULT '',
    conformance_server  TEXT NOT NULL DEFAULT '',

    last_used_at    TIMESTAMP,
    created_by      TEXT NOT NULL,
    created_at      TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at      TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    row_version     INTEGER NOT NULL DEFAULT 0,

    -- One connection per name per tenant, so a display name is a usable
    -- reference in an alert.
    UNIQUE (tenant_id, display_name)
);

CREATE INDEX IF NOT EXISTS idx_source_connections_tenant
    ON source_connections(tenant_id, connector_type);

-- The link between a connection's credential field and its secret-store entry.
--
-- The secret's *name* is here; the secret is not. Reading one requires going
-- through internal/secrets, which returns the value only inside a callback and
-- stamps the access. Keeping the name in its own table rather than in
-- config_json means a query that dumps configuration cannot accidentally widen
-- into the credential graph.
CREATE TABLE IF NOT EXISTS source_connection_secrets (
    tenant_id       TEXT NOT NULL REFERENCES tenants(id),
    connection_id   INTEGER NOT NULL REFERENCES source_connections(id),
    field_id        TEXT NOT NULL,
    secret_name     TEXT NOT NULL,
    -- Reported to an operator so a credential the customer chose badly is
    -- visible without being displayed.
    weak            INTEGER NOT NULL DEFAULT 0,
    created_at      TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    rotated_at      TIMESTAMP,
    PRIMARY KEY (tenant_id, connection_id, field_id)
);

-- Every check, kept rather than only the latest.
--
-- A connection that is failing now is a different situation from one that has
-- been failing for a week, and the second is invisible if each check overwrites
-- the last. The row carries a sanitized error class, never the driver's own
-- message: driver errors routinely name the host, the account, and sometimes
-- the credential.
CREATE TABLE IF NOT EXISTS source_connection_health (
    id              INTEGER PRIMARY KEY AUTOINCREMENT,
    tenant_id       TEXT NOT NULL REFERENCES tenants(id),
    connection_id   INTEGER NOT NULL REFERENCES source_connections(id),
    state           TEXT NOT NULL
                      CHECK (state IN ('NEVER_CHECKED','HEALTHY','DEGRADED','FAILED')),
    error_class     TEXT,
    detail          TEXT,
    latency_ms      INTEGER,
    actor_id        TEXT NOT NULL,
    checked_at      TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP
);

CREATE INDEX IF NOT EXISTS idx_connection_health_recent
    ON source_connection_health(tenant_id, connection_id, checked_at);
