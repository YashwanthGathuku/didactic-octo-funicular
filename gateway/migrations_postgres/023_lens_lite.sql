-- 023_lens_lite.sql -- PostgreSQL dialect
-- Governed, read-only analytics metadata for SentinelFlow Lens Lite.
-- Financial authority remains in the existing deterministic control plane.

-- Composite uniqueness lets PostgreSQL enforce tenant-bound references from Lens.
CREATE UNIQUE INDEX IF NOT EXISTS idx_incidents_tenant_id_unique ON incidents(tenant_id, id);

CREATE TABLE IF NOT EXISTS lens_return_events (
    id            TEXT PRIMARY KEY,
    tenant_id     TEXT NOT NULL REFERENCES tenants(id),
    occurred_at   TIMESTAMPTZ NOT NULL,
    partner_id    TEXT NOT NULL,
    return_code   TEXT NOT NULL,
    amount_cents  BIGINT NOT NULL CHECK (amount_cents >= 0),
    source_type   TEXT NOT NULL CHECK (source_type IN ('SYNTHETIC_DEMO','CURATED_IMPORT')),
    verified      INTEGER NOT NULL DEFAULT 0 CHECK (verified IN (0,1)),
    incident_id   BIGINT,
    created_at    TIMESTAMPTZ NOT NULL DEFAULT now(),
    CHECK (NOT (source_type = 'SYNTHETIC_DEMO' AND verified = 1)),
    FOREIGN KEY (tenant_id, incident_id) REFERENCES incidents(tenant_id, id)
);

CREATE INDEX IF NOT EXISTS idx_lens_returns_tenant_time ON lens_return_events(tenant_id, occurred_at);
CREATE INDEX IF NOT EXISTS idx_lens_returns_partner ON lens_return_events(tenant_id, partner_id, occurred_at);
CREATE INDEX IF NOT EXISTS idx_lens_returns_code ON lens_return_events(tenant_id, return_code, occurred_at);

CREATE TABLE IF NOT EXISTS lens_investigations (
    id          TEXT PRIMARY KEY,
    tenant_id   TEXT NOT NULL REFERENCES tenants(id),
    title       TEXT NOT NULL,
    created_by  TEXT NOT NULL,
    created_at  TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at  TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE (tenant_id, id)
);

CREATE TABLE IF NOT EXISTS lens_investigation_nodes (
    id                  TEXT PRIMARY KEY,
    investigation_id    TEXT NOT NULL,
    tenant_id           TEXT NOT NULL REFERENCES tenants(id),
    parent_node_id      TEXT,
    question            TEXT NOT NULL,
    query_intent_json   TEXT NOT NULL,
    query_hash          TEXT NOT NULL,
    result_hash         TEXT NOT NULL,
    chart_spec_json     TEXT NOT NULL,
    evidence_refs_json  TEXT NOT NULL DEFAULT '[]',
    created_by          TEXT NOT NULL,
    created_at          TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE (tenant_id, id),
    FOREIGN KEY (tenant_id, investigation_id) REFERENCES lens_investigations(tenant_id, id),
    FOREIGN KEY (tenant_id, parent_node_id) REFERENCES lens_investigation_nodes(tenant_id, id)
);

CREATE INDEX IF NOT EXISTS idx_lens_nodes_investigation ON lens_investigation_nodes(tenant_id, investigation_id, created_at);

ALTER TABLE lens_return_events        ENABLE ROW LEVEL SECURITY;
ALTER TABLE lens_investigations       ENABLE ROW LEVEL SECURITY;
ALTER TABLE lens_investigation_nodes  ENABLE ROW LEVEL SECURITY;

ALTER TABLE lens_return_events        FORCE ROW LEVEL SECURITY;
ALTER TABLE lens_investigations       FORCE ROW LEVEL SECURITY;
ALTER TABLE lens_investigation_nodes  FORCE ROW LEVEL SECURITY;

DROP POLICY IF EXISTS tenant_isolation_lens_return_events ON lens_return_events;
CREATE POLICY tenant_isolation_lens_return_events ON lens_return_events
    USING (tenant_id = current_setting('sentinel.tenant_id', true))
    WITH CHECK (tenant_id = current_setting('sentinel.tenant_id', true));

DROP POLICY IF EXISTS tenant_isolation_lens_investigations ON lens_investigations;
CREATE POLICY tenant_isolation_lens_investigations ON lens_investigations
    USING (tenant_id = current_setting('sentinel.tenant_id', true))
    WITH CHECK (tenant_id = current_setting('sentinel.tenant_id', true));

DROP POLICY IF EXISTS tenant_isolation_lens_investigation_nodes ON lens_investigation_nodes;
CREATE POLICY tenant_isolation_lens_investigation_nodes ON lens_investigation_nodes
    USING (tenant_id = current_setting('sentinel.tenant_id', true))
    WITH CHECK (tenant_id = current_setting('sentinel.tenant_id', true));

CREATE OR REPLACE FUNCTION lens_investigation_nodes_append_only() RETURNS TRIGGER AS $$
BEGIN
    RAISE EXCEPTION 'lens investigation nodes are append-only';
END;
$$ LANGUAGE plpgsql;

DROP TRIGGER IF EXISTS lens_investigation_nodes_no_change ON lens_investigation_nodes;
CREATE TRIGGER lens_investigation_nodes_no_change
    BEFORE UPDATE OR DELETE ON lens_investigation_nodes
    FOR EACH ROW EXECUTE FUNCTION lens_investigation_nodes_append_only();
