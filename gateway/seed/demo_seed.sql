-- Demo seed data. NOT a migration.
--
-- Applied only by `sentinel-gateway migrate seed-demo`, which refuses to run
-- outside the local-demo profile. These are fictional counterparties used to
-- populate a developer machine; they must never appear in a real deployment.
--
-- Idempotency is expressed as WHERE NOT EXISTS rather than INSERT OR IGNORE.
-- OR IGNORE suppresses every constraint violation, including NOT NULL, so when
-- tenant_id became mandatory this file silently inserted nothing and reported
-- success. A guard that names the condition cannot hide a different failure.

INSERT INTO tenants (id, name)
SELECT 'TENANT-DEMO', 'Demo Tenant (local development only)'
WHERE NOT EXISTS (SELECT 1 FROM tenants WHERE id = 'TENANT-DEMO');

INSERT INTO partners (tenant_id, name, routing_number)
SELECT 'TENANT-DEMO', 'Meridian Custody Bank', '021000018'
WHERE NOT EXISTS (
    SELECT 1 FROM partners WHERE tenant_id = 'TENANT-DEMO' AND routing_number = '021000018'
);

INSERT INTO partners (tenant_id, name, routing_number)
SELECT 'TENANT-DEMO', 'Atlantic Trust', '011000028'
WHERE NOT EXISTS (
    SELECT 1 FROM partners WHERE tenant_id = 'TENANT-DEMO' AND routing_number = '011000028'
);

INSERT INTO partners (tenant_id, name, routing_number)
SELECT 'TENANT-DEMO', 'Central Clearing Network', '000000000'
WHERE NOT EXISTS (
    SELECT 1 FROM partners WHERE tenant_id = 'TENANT-DEMO' AND routing_number = '000000000'
);

INSERT INTO file_contracts
    (tenant_id, partner_id, name, direction, filename_pattern, expected_time, grace_period_minutes, timezone)
SELECT 'TENANT-DEMO', p.id, 'Meridian End of Day NAV', 'INBOUND',
       '^MERIDIAN_NAV_.*\.csv$', '18:00:00', 30, 'America/New_York'
FROM partners p
WHERE p.tenant_id = 'TENANT-DEMO' AND p.routing_number = '021000018'
  AND NOT EXISTS (
      SELECT 1 FROM file_contracts c
      WHERE c.tenant_id = 'TENANT-DEMO' AND c.name = 'Meridian End of Day NAV'
  );

INSERT INTO file_contracts
    (tenant_id, partner_id, name, direction, filename_pattern, expected_time, grace_period_minutes, timezone)
SELECT 'TENANT-DEMO', p.id, 'Central Clearing PM Delivery', 'INBOUND',
       '^CLEARING_PM_.*\.ach$', '16:45:00', 15, 'America/New_York'
FROM partners p
WHERE p.tenant_id = 'TENANT-DEMO' AND p.routing_number = '000000000'
  AND NOT EXISTS (
      SELECT 1 FROM file_contracts c
      WHERE c.tenant_id = 'TENANT-DEMO' AND c.name = 'Central Clearing PM Delivery'
  );

-- One occurrence due today, so a developer machine has something to look at.
INSERT INTO expectations
    (tenant_id, contract_id, business_date, expected_delivery_start, expected_delivery_end, status)
SELECT 'TENANT-DEMO', c.id, date('now'),
       datetime('now', 'start of day', '+16 hours', '+30 minutes'),
       datetime('now', 'start of day', '+17 hours'),
       'PENDING'
FROM file_contracts c
WHERE c.tenant_id = 'TENANT-DEMO' AND c.name = 'Central Clearing PM Delivery'
  AND NOT EXISTS (
      SELECT 1 FROM expectations e
      WHERE e.tenant_id = 'TENANT-DEMO' AND e.contract_id = c.id AND e.business_date = date('now')
  );
