-- 010_source_connections.sql -- PostgreSQL dialect
-- Customer source connections, secret links, and health checks.

CREATE TABLE IF NOT EXISTS source_connections (
    id                  BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    tenant_id           TEXT NOT NULL REFERENCES tenants(id),
    connector_type      TEXT NOT NULL,
    display_name        TEXT NOT NULL,
    auth_mode           TEXT NOT NULL,
    config_json         TEXT NOT NULL,
    resource_allowlist  TEXT NOT NULL,
    max_rows            INTEGER NOT NULL DEFAULT 0,
    max_bytes           BIGINT NOT NULL DEFAULT 0,
    timeout_seconds     INTEGER NOT NULL DEFAULT 0,
    max_per_minute      INTEGER NOT NULL DEFAULT 60,
    health_state        TEXT NOT NULL DEFAULT 'NEVER_CHECKED'
                          CHECK (health_state IN ('NEVER_CHECKED','HEALTHY','DEGRADED','FAILED')),
    health_checked_at   TIMESTAMPTZ,
    health_error_class  TEXT,
    health_detail       TEXT,
    health_latency_ms   INTEGER,
    conformance_commit  TEXT NOT NULL DEFAULT '',
    conformance_server  TEXT NOT NULL DEFAULT '',
    last_used_at        TIMESTAMPTZ,
    created_by          TEXT NOT NULL,
    created_at          TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at          TIMESTAMPTZ NOT NULL DEFAULT now(),
    row_version         INTEGER NOT NULL DEFAULT 0,
    UNIQUE (tenant_id, display_name)
);

CREATE INDEX IF NOT EXISTS idx_source_connections_tenant
    ON source_connections(tenant_id, connector_type);

CREATE TABLE IF NOT EXISTS source_connection_secrets (
    tenant_id       TEXT NOT NULL REFERENCES tenants(id),
    connection_id   BIGINT NOT NULL REFERENCES source_connections(id),
    field_id        TEXT NOT NULL,
    secret_name     TEXT NOT NULL,
    weak            INTEGER NOT NULL DEFAULT 0,
    created_at      TIMESTAMPTZ NOT NULL DEFAULT now(),
    rotated_at      TIMESTAMPTZ,
    PRIMARY KEY (tenant_id, connection_id, field_id)
);

CREATE TABLE IF NOT EXISTS source_connection_health (
    id              BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    tenant_id       TEXT NOT NULL REFERENCES tenants(id),
    connection_id   BIGINT NOT NULL REFERENCES source_connections(id),
    state           TEXT NOT NULL
                      CHECK (state IN ('NEVER_CHECKED','HEALTHY','DEGRADED','FAILED')),
    error_class     TEXT,
    detail          TEXT,
    latency_ms      INTEGER,
    actor_id        TEXT NOT NULL,
    checked_at      TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX IF NOT EXISTS idx_connection_health_recent
    ON source_connection_health(tenant_id, connection_id, checked_at);

ALTER TABLE source_connections        ENABLE ROW LEVEL SECURITY;
ALTER TABLE source_connection_secrets ENABLE ROW LEVEL SECURITY;
ALTER TABLE source_connection_health  ENABLE ROW LEVEL SECURITY;

ALTER TABLE source_connections        FORCE ROW LEVEL SECURITY;
ALTER TABLE source_connection_secrets FORCE ROW LEVEL SECURITY;
ALTER TABLE source_connection_health  FORCE ROW LEVEL SECURITY;

DROP POLICY IF EXISTS tenant_isolation_source_connections ON source_connections;
CREATE POLICY tenant_isolation_source_connections ON source_connections
    USING (tenant_id = current_setting('sentinel.tenant_id', true))
    WITH CHECK (tenant_id = current_setting('sentinel.tenant_id', true));

DROP POLICY IF EXISTS tenant_isolation_source_connection_secrets ON source_connection_secrets;
CREATE POLICY tenant_isolation_source_connection_secrets ON source_connection_secrets
    USING (tenant_id = current_setting('sentinel.tenant_id', true))
    WITH CHECK (tenant_id = current_setting('sentinel.tenant_id', true));

DROP POLICY IF EXISTS tenant_isolation_source_connection_health ON source_connection_health;
CREATE POLICY tenant_isolation_source_connection_health ON source_connection_health
    USING (tenant_id = current_setting('sentinel.tenant_id', true))
    WITH CHECK (tenant_id = current_setting('sentinel.tenant_id', true));
