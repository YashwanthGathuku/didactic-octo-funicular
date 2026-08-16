-- Feed contracts, business calendars, and materialized expectations.
--
-- Migration 002 created `file_contract_versions` and `expectations` and nothing
-- has ever written to either: no scheduler existed, so the only way an
-- expectation could appear was by hand. That is the silent-missing-file
-- problem in its purest form -- the table designed to make a missing file
-- visible was itself empty.
--
-- What 002 left out, and this migration adds:
--   * the schedule rule itself. A contract version said what time a file was
--     due but never which days it was due on.
--   * a business calendar. `calendar_id` was a free-text column with no table
--     behind it, so a holiday could not be represented at all and every partner
--     was late every Christmas.
--   * a breach threshold distinct from the grace period, so OVERDUE and
--     BREACHED are different states rather than the same instant.
--   * an owner and escalation reference, so a breach has a recipient.
--   * somewhere to record an ambiguous arrival rather than resolving it by
--     guessing.

-- ---------------------------------------------------------------------------
-- Business calendars
-- ---------------------------------------------------------------------------

-- A calendar is a published base plus tenant corrections.
--
-- The base rules live in Go (internal/schedule/calendar.go) rather than as
-- seeded rows, because a table of dates expires: the Federal Reserve publishes
-- observed dates only a few years ahead, and a scheduler that runs out of
-- holiday rows does not fail loudly. It marks Christmas Day a business day and
-- reports every partner as late.
--
-- Only the corrections are data, because only the corrections are local
-- knowledge.
CREATE TABLE IF NOT EXISTS business_calendars (
    tenant_id   TEXT NOT NULL REFERENCES tenants(id),
    calendar_id TEXT NOT NULL,
    name        TEXT NOT NULL,
    base        TEXT NOT NULL CHECK (base IN ('FEDERAL_RESERVE','WEEKDAYS','ALL_DAYS')),
    created_at  TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    PRIMARY KEY (tenant_id, calendar_id)
);

-- Tenant-specific overrides in both directions.
--
-- The two directions fail differently and both need to be possible. Closing a
-- day the base calls open suppresses an expectation, so a wrong closure hides a
-- missing file; opening a day the base calls closed creates an expectation, so
-- a wrong opening raises a false alarm. A reason is mandatory for both: an
-- unexplained closure is indistinguishable from a mistake at the moment someone
-- has to account for a file nobody was waiting for.
CREATE TABLE IF NOT EXISTS business_calendar_overrides (
    tenant_id     TEXT NOT NULL REFERENCES tenants(id),
    calendar_id   TEXT NOT NULL,
    calendar_date DATE NOT NULL,
    kind          TEXT NOT NULL CHECK (kind IN ('HOLIDAY','BUSINESS_DAY')),
    reason        TEXT NOT NULL CHECK (length(trim(reason)) > 0),
    created_at    TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    PRIMARY KEY (tenant_id, calendar_id, calendar_date),
    FOREIGN KEY (tenant_id, calendar_id)
        REFERENCES business_calendars(tenant_id, calendar_id)
);

-- ---------------------------------------------------------------------------
-- Contract version: the scheduling terms
-- ---------------------------------------------------------------------------

-- Which days the feed is expected on. See internal/schedule/rule.go for the
-- grammar. It is a small closed vocabulary rather than cron: cron cannot say
-- "the last day of the month" without a vendor extension, has no notion of a
-- business calendar, and is misread often enough that a wrong schedule -- which
-- fails by never materializing anything -- is a realistic outcome.
ALTER TABLE file_contract_versions ADD COLUMN schedule_rule TEXT NOT NULL DEFAULT 'EVERY_BUSINESS_DAY';

-- What to do when the rule names a day the calendar closes.
ALTER TABLE file_contract_versions ADD COLUMN nonbusiness_action TEXT NOT NULL DEFAULT 'SKIP';

-- How long after the grace period an occurrence becomes a breach.
--
-- Separate from grace_minutes because they answer different questions. Grace is
-- the tolerance the partner is given; the breach delay is how long the tenant
-- waits before treating lateness as an incident. Collapsing them into one
-- number makes OVERDUE unreachable, and OVERDUE is the state a human is
-- supposed to act on before it becomes a breach.
ALTER TABLE file_contract_versions ADD COLUMN breach_after_minutes INTEGER NOT NULL DEFAULT 60;

-- Who is accountable, and under which escalation policy. An expectation with no
-- owner produces an alert with no recipient.
ALTER TABLE file_contract_versions ADD COLUMN owner_subject TEXT NOT NULL DEFAULT '';
ALTER TABLE file_contract_versions ADD COLUMN escalation_policy_id TEXT NOT NULL DEFAULT '';

-- The tenant's own name for the feed and the partner it belongs to.
--
-- A partner is commonly the counterparty for several feeds, so "a file from
-- Acme is late" does not say which file. The feed id is what appears in an
-- alert.
ALTER TABLE file_contract_versions ADD COLUMN feed_id TEXT NOT NULL DEFAULT '';
ALTER TABLE file_contract_versions ADD COLUMN direction TEXT NOT NULL DEFAULT 'INBOUND';

CREATE INDEX IF NOT EXISTS idx_contract_versions_active
    ON file_contract_versions(tenant_id, contract_id, effective_from);

-- ---------------------------------------------------------------------------
-- Occurrences
-- ---------------------------------------------------------------------------

-- The instant at which lateness becomes a breach. `expected_delivery_end` from
-- 002 is the end of the grace period; this is the escalation threshold.
ALTER TABLE expectations ADD COLUMN breach_at TIMESTAMP;

-- Why this occurrence's window is what it is: a calendar adjustment, a schedule
-- collision, or a daylight-saving transition. Written when the occurrence is
-- materialized, because it cannot be re-derived later -- the calendar overrides
-- and the contract version can both change afterwards.
ALTER TABLE expectations ADD COLUMN schedule_note TEXT NOT NULL DEFAULT '';

-- The clock's reading of the local deadline, kept alongside the UTC instants.
-- An operator disputing a breach asks "what time was it due, their time", and
-- recomputing the answer from a version that has since been superseded gives
-- the wrong one.
ALTER TABLE expectations ADD COLUMN due_local TEXT NOT NULL DEFAULT '';
ALTER TABLE expectations ADD COLUMN timezone TEXT NOT NULL DEFAULT '';

-- When the matching artifact was attributed, distinct from when the occurrence
-- row was last touched.
ALTER TABLE expectations ADD COLUMN matched_at TIMESTAMP;

-- An arrival could not be attributed to exactly one occurrence and a human must
-- decide.
--
-- The occurrence deliberately keeps its ageing state while this is set. It has
-- not been shown to have arrived, so freezing it would let a wrong guess stop
-- the clock on the file that did not come.
ALTER TABLE expectations ADD COLUMN review_required INTEGER NOT NULL DEFAULT 0;

CREATE INDEX IF NOT EXISTS idx_expectations_due
    ON expectations(tenant_id, status, expected_delivery_start);

CREATE INDEX IF NOT EXISTS idx_expectations_business_date
    ON expectations(tenant_id, business_date);

-- ---------------------------------------------------------------------------
-- Ambiguous arrivals
-- ---------------------------------------------------------------------------

-- Every occurrence an arriving artifact could have satisfied.
--
-- When exactly one open occurrence matches, the artifact is attributed and no
-- row is written here. When several match, or when the only match is already
-- satisfied by an earlier artifact, one row per candidate is written and
-- nothing is attributed. Choosing the closest deadline would be a guess, and a
-- wrong guess marks one occurrence arrived and leaves a real missing file
-- looking satisfied -- the exact failure this subsystem exists to prevent, now
-- with an audit record asserting it did not happen.
CREATE TABLE IF NOT EXISTS expectation_match_candidates (
    id               INTEGER PRIMARY KEY AUTOINCREMENT,
    tenant_id        TEXT NOT NULL REFERENCES tenants(id),
    expectation_id   INTEGER NOT NULL REFERENCES expectations(id),
    file_instance_id INTEGER NOT NULL REFERENCES file_instances(id),
    filename         TEXT NOT NULL,
    reason           TEXT NOT NULL,
    resolution       TEXT NOT NULL DEFAULT 'REVIEW_REQUIRED'
                       CHECK (resolution IN ('REVIEW_REQUIRED','ACCEPTED','REJECTED')),
    resolved_by      TEXT,
    resolved_at      TIMESTAMP,
    created_at       TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    -- One candidate row per artifact per occurrence. Re-running the matcher
    -- after a restart must not multiply the review queue.
    UNIQUE (tenant_id, expectation_id, file_instance_id)
);

CREATE INDEX IF NOT EXISTS idx_match_candidates_open
    ON expectation_match_candidates(tenant_id, resolution);
