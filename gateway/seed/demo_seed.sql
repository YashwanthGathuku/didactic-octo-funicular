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
       'MERIDIAN_NAV_*.csv', '18:00:00', 30, 'America/New_York'
FROM partners p
WHERE p.tenant_id = 'TENANT-DEMO' AND p.routing_number = '021000018'
  AND NOT EXISTS (
      SELECT 1 FROM file_contracts c
      WHERE c.tenant_id = 'TENANT-DEMO' AND c.name = 'Meridian End of Day NAV'
  );

INSERT INTO file_contracts
    (tenant_id, partner_id, name, direction, filename_pattern, expected_time, grace_period_minutes, timezone)
SELECT 'TENANT-DEMO', p.id, 'Central Clearing PM Delivery', 'INBOUND',
       'CLEARING_PM_*.ach', '16:45:00', 15, 'America/New_York'
FROM partners p
WHERE p.tenant_id = 'TENANT-DEMO' AND p.routing_number = '000000000'
  AND NOT EXISTS (
      SELECT 1 FROM file_contracts c
      WHERE c.tenant_id = 'TENANT-DEMO' AND c.name = 'Central Clearing PM Delivery'
  );

-- A business calendar for the demo tenant.
--
-- Without one the scheduler refuses to materialize -- deliberately, since
-- defaulting to a weekday calendar would put a Christmas Day expectation in the
-- queue. The demo therefore has to configure one like any other deployment.
INSERT OR IGNORE INTO business_calendars (tenant_id, calendar_id, name, base)
VALUES ('TENANT-DEMO', 'fed', 'Federal Reserve Bank holidays', 'FEDERAL_RESERVE');

-- Contract versions carry the scheduling terms. The filename patterns are the
-- glob form internal/schedule accepts, not regular expressions: the previous
-- seed used `^MERIDIAN_NAV_.*\.csv$`, which no component in this repository
-- has ever been able to match against.
--
-- The date token on the clearing feed is what lets an arrival be attributed to
-- a specific business date rather than to whichever occurrence happens to be
-- open.
INSERT INTO file_contract_versions
    (tenant_id, contract_id, version, feed_id, direction, filename_pattern, format,
     expected_local, timezone, grace_minutes, breach_after_minutes, calendar_id,
     schedule_rule, nonbusiness_action, balanced_mode, owner_subject,
     escalation_policy_id, effective_from)
SELECT 'TENANT-DEMO', c.id, 1, 'MERIDIAN-NAV', 'INBOUND',
       'MERIDIAN_NAV_*.csv', 'NACHA',
       '18:00:00', 'America/New_York', 30, 60, 'fed',
       'EVERY_BUSINESS_DAY', 'SKIP', 'BALANCED', 'demo-operator@example.invalid',
       'demo/escalation-policy', date('now', '-30 days')
FROM file_contracts c
WHERE c.tenant_id = 'TENANT-DEMO' AND c.name = 'Meridian End of Day NAV'
  AND NOT EXISTS (
      SELECT 1 FROM file_contract_versions v
      WHERE v.tenant_id = 'TENANT-DEMO' AND v.contract_id = c.id
  );

INSERT INTO file_contract_versions
    (tenant_id, contract_id, version, feed_id, direction, filename_pattern, format,
     expected_local, timezone, grace_minutes, breach_after_minutes, calendar_id,
     schedule_rule, nonbusiness_action, balanced_mode, owner_subject,
     escalation_policy_id, effective_from)
SELECT 'TENANT-DEMO', c.id, 1, 'CLEARING-PM', 'INBOUND',
       'CLEARING_PM_{YYYY}{MM}{DD}.ach', 'NACHA',
       '16:45:00', 'America/New_York', 15, 45, 'fed',
       'EVERY_BUSINESS_DAY', 'SKIP', 'BALANCED', 'demo-operator@example.invalid',
       'demo/escalation-policy', date('now', '-30 days')
FROM file_contracts c
WHERE c.tenant_id = 'TENANT-DEMO' AND c.name = 'Central Clearing PM Delivery'
  AND NOT EXISTS (
      SELECT 1 FROM file_contract_versions v
      WHERE v.tenant_id = 'TENANT-DEMO' AND v.contract_id = c.id
  );

-- No expectation rows are seeded.
--
-- The previous seed inserted one by hand, with deadlines computed by SQLite
-- date arithmetic in UTC rather than in the contract's timezone. That row was
-- indistinguishable in the board from a scheduled one while being produced by a
-- completely different mechanism -- so a demo could show a working schedule
-- while the scheduler was doing nothing at all. Occurrences now come only from
-- internal/schedule, on the first pass after the process starts.
