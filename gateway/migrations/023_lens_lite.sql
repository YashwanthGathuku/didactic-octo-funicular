-- 023_lens_lite.sql
-- Governed, read-only analytics metadata for SentinelFlow Lens Lite.
-- Financial authority remains in the existing deterministic control plane.

-- Composite uniqueness lets SQLite enforce tenant-bound references from Lens.
CREATE UNIQUE INDEX IF NOT EXISTS idx_incidents_tenant_id_unique ON incidents(tenant_id, id);

CREATE TABLE IF NOT EXISTS lens_return_events (
    id            TEXT PRIMARY KEY,
    tenant_id     TEXT NOT NULL REFERENCES tenants(id),
    occurred_at   TIMESTAMP NOT NULL,
    partner_id    TEXT NOT NULL,
    return_code   TEXT NOT NULL,
    amount_cents  INTEGER NOT NULL CHECK (amount_cents >= 0),
    source_type   TEXT NOT NULL CHECK (source_type IN ('SYNTHETIC_DEMO','CURATED_IMPORT')),
    verified      INTEGER NOT NULL DEFAULT 0 CHECK (verified IN (0,1)),
    incident_id   INTEGER,
    created_at    TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
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
    created_at  TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at  TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    UNIQUE (tenant_id, id)
);

CREATE TABLE IF NOT EXISTS lens_investigation_nodes (
    id                  TEXT PRIMARY KEY,
    investigation_id    TEXT NOT NULL,
    tenant_id            TEXT NOT NULL REFERENCES tenants(id),
    parent_node_id       TEXT,
    question             TEXT NOT NULL,
    query_intent_json    TEXT NOT NULL,
    query_hash           TEXT NOT NULL,
    result_hash          TEXT NOT NULL,
    chart_spec_json      TEXT NOT NULL,
    evidence_refs_json   TEXT NOT NULL DEFAULT '[]',
    created_by           TEXT NOT NULL,
    created_at           TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    UNIQUE (tenant_id, id),
    FOREIGN KEY (tenant_id, investigation_id) REFERENCES lens_investigations(tenant_id, id),
    FOREIGN KEY (tenant_id, parent_node_id) REFERENCES lens_investigation_nodes(tenant_id, id)
);
CREATE INDEX IF NOT EXISTS idx_lens_nodes_investigation ON lens_investigation_nodes(tenant_id, investigation_id, created_at);

CREATE TRIGGER IF NOT EXISTS lens_investigation_nodes_no_update
BEFORE UPDATE ON lens_investigation_nodes
BEGIN
    SELECT RAISE(ABORT, 'lens investigation nodes are append-only');
END;

CREATE TRIGGER IF NOT EXISTS lens_investigation_nodes_no_delete
BEFORE DELETE ON lens_investigation_nodes
BEGIN
    SELECT RAISE(ABORT, 'lens investigation nodes are append-only');
END;
