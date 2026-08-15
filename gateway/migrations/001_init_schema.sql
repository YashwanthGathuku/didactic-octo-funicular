-- 001_init_schema.sql -- baseline schema.
--
-- DDL only. Seed data was previously appended to this file, which meant every
-- deployment created three demo partners and two demo contracts, and re-running
-- the file duplicated them. Demo data now lives in a separate seed that only the
-- local-demo profile may apply (`sentinel-gateway migrate seed-demo`).
--
-- IF NOT EXISTS makes this safe to apply to a database created by the previous
-- opportunistic startup migration, which produced these tables without any
-- schema_migrations record.

CREATE TABLE IF NOT EXISTS partners (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    name VARCHAR(255) NOT NULL,
    routing_number VARCHAR(9) UNIQUE NOT NULL,
    created_at DATETIME DEFAULT CURRENT_TIMESTAMP
);

CREATE TABLE IF NOT EXISTS file_contracts (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    partner_id INTEGER,
    name VARCHAR(255) NOT NULL,
    direction VARCHAR(50) NOT NULL, -- INBOUND or OUTBOUND
    filename_pattern VARCHAR(255) NOT NULL,
    expected_time TIME NOT NULL,
    grace_period_minutes INTEGER NOT NULL,
    timezone VARCHAR(50) NOT NULL,
    created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
    FOREIGN KEY(partner_id) REFERENCES partners(id)
);

CREATE TABLE IF NOT EXISTS expectations (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    contract_id INTEGER,
    expected_delivery_start DATETIME NOT NULL,
    expected_delivery_end DATETIME NOT NULL,
    status VARCHAR(50) NOT NULL, -- PENDING, ARRIVED, OVERDUE
    created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
    updated_at DATETIME DEFAULT CURRENT_TIMESTAMP,
    FOREIGN KEY(contract_id) REFERENCES file_contracts(id)
);

CREATE TABLE IF NOT EXISTS file_instances (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    expectation_id INTEGER,
    filename VARCHAR(255) NOT NULL,
    storage_path VARCHAR(512) NOT NULL,
    size_bytes BIGINT NOT NULL,
    sha256_hash VARCHAR(64) NOT NULL,
    status VARCHAR(50) NOT NULL, -- QUARANTINED, RELEASED, REJECTED
    received_at DATETIME DEFAULT CURRENT_TIMESTAMP,
    FOREIGN KEY(expectation_id) REFERENCES expectations(id)
);

CREATE TABLE IF NOT EXISTS incidents (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    expectation_id INTEGER,
    file_instance_id INTEGER,
    type VARCHAR(50) NOT NULL, -- MISSING_FILE, MALFORMED_FILE, SLA_BREACH
    severity VARCHAR(50) NOT NULL, -- HIGH, CRITICAL, INFO
    status VARCHAR(50) NOT NULL, -- OPEN, INVESTIGATING, RESOLVED
    created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
    updated_at DATETIME DEFAULT CURRENT_TIMESTAMP,
    FOREIGN KEY(expectation_id) REFERENCES expectations(id),
    FOREIGN KEY(file_instance_id) REFERENCES file_instances(id)
);

CREATE TABLE IF NOT EXISTS validation_findings (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    file_instance_id INTEGER,
    code VARCHAR(100) NOT NULL,
    description TEXT NOT NULL,
    severity VARCHAR(50) NOT NULL,
    line_number INTEGER,
    raw_data TEXT,
    created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
    FOREIGN KEY(file_instance_id) REFERENCES file_instances(id)
);

CREATE TABLE IF NOT EXISTS audit_events (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    event_type VARCHAR(100) NOT NULL,
    actor VARCHAR(100) NOT NULL,
    payload TEXT NOT NULL,
    previous_hash VARCHAR(64) NOT NULL,
    current_hash VARCHAR(64) NOT NULL,
    created_at DATETIME DEFAULT CURRENT_TIMESTAMP
);
