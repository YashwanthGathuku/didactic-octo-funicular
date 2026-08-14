CREATE TABLE partners (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    name VARCHAR(255) NOT NULL,
    routing_number VARCHAR(9) UNIQUE NOT NULL,
    created_at DATETIME DEFAULT CURRENT_TIMESTAMP
);

CREATE TABLE file_contracts (
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

CREATE TABLE expectations (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    contract_id INTEGER,
    expected_delivery_start DATETIME NOT NULL,
    expected_delivery_end DATETIME NOT NULL,
    status VARCHAR(50) NOT NULL, -- PENDING, ARRIVED, OVERDUE
    created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
    updated_at DATETIME DEFAULT CURRENT_TIMESTAMP,
    FOREIGN KEY(contract_id) REFERENCES file_contracts(id)
);

CREATE TABLE file_instances (
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

CREATE TABLE incidents (
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

CREATE TABLE validation_findings (
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

CREATE TABLE audit_events (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    event_type VARCHAR(100) NOT NULL,
    actor VARCHAR(100) NOT NULL,
    payload TEXT NOT NULL,
    previous_hash VARCHAR(64) NOT NULL,
    current_hash VARCHAR(64) NOT NULL,
    created_at DATETIME DEFAULT CURRENT_TIMESTAMP
);

-- Seed Initial Data
INSERT INTO partners (name, routing_number) VALUES 
('Meridian Custody Bank', '021000018'),
('Atlantic Trust', '011000028'),
('Central Clearing Network', '000000000');

INSERT INTO file_contracts (partner_id, name, direction, filename_pattern, expected_time, grace_period_minutes, timezone) VALUES
(1, 'Meridian End of Day NAV', 'INBOUND', '^MERIDIAN_NAV_.*\.csv$', '18:00:00', 30, 'America/New_York'),
(3, 'Central Clearing PM Delivery', 'INBOUND', '^CLEARING_PM_.*\.ach$', '16:45:00', 15, 'America/New_York');

-- Seed initial expectation for clearing network
INSERT INTO expectations (contract_id, expected_delivery_start, expected_delivery_end, status) VALUES
(2, datetime('now', 'start of day', '+16 hours', '+30 minutes'), datetime('now', 'start of day', '+17 hours'), 'PENDING');
