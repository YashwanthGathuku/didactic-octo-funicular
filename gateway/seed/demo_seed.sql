-- Demo seed data. NOT a migration.
--
-- Applied only by `sentinel-gateway migrate seed-demo`, which refuses to run
-- outside the local-demo profile. These are fictional counterparties used to
-- populate a developer machine; they must never appear in a real deployment.

INSERT OR IGNORE INTO partners (name, routing_number) VALUES 
('Meridian Custody Bank', '021000018'),
('Atlantic Trust', '011000028'),
('Central Clearing Network', '000000000');

INSERT OR IGNORE INTO file_contracts (partner_id, name, direction, filename_pattern, expected_time, grace_period_minutes, timezone) VALUES
(1, 'Meridian End of Day NAV', 'INBOUND', '^MERIDIAN_NAV_.*\.csv$', '18:00:00', 30, 'America/New_York'),
(3, 'Central Clearing PM Delivery', 'INBOUND', '^CLEARING_PM_.*\.ach$', '16:45:00', 15, 'America/New_York');

-- Seed initial expectation for clearing network
INSERT OR IGNORE INTO expectations (contract_id, expected_delivery_start, expected_delivery_end, status) VALUES
(2, datetime('now', 'start of day', '+16 hours', '+30 minutes'), datetime('now', 'start of day', '+17 hours'), 'PENDING');
