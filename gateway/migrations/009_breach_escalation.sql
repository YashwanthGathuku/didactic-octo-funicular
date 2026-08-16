-- Making a breach mean something.
--
-- Prompt 10 detected breaches and recorded the transition, and that was all.
-- No incident opened, no notification was written, and the `owner_subject` and
-- `escalation_policy_id` every contract version carries were stored and never
-- read. A breach nobody is told about is only marginally better than a breach
-- nobody detects: the evidence exists, and the person who needed it does not
-- know to look.

-- ---------------------------------------------------------------------------
-- Incidents
-- ---------------------------------------------------------------------------

-- One incident per occurrence per type.
--
-- Without this the advancement pass would open a second incident on any retry,
-- and an operator's queue would fill with duplicates of the same missing file --
-- which is how an alerting system stops being read.
--
-- The index is partial because `expectation_id` is nullable: an incident raised
-- against an artifact rather than an expectation has no occurrence to be unique
-- against, and NULLs would otherwise collide differently on each database.
CREATE UNIQUE INDEX IF NOT EXISTS idx_incidents_occurrence_type
    ON incidents(tenant_id, expectation_id, type)
    WHERE expectation_id IS NOT NULL;

-- Which contract version's terms the incident was raised under, so an incident
-- reviewed months later states the deadline that actually applied rather than
-- whatever the contract says now.
ALTER TABLE incidents ADD COLUMN contract_version_id INTEGER REFERENCES file_contract_versions(id);

-- The human-readable account of what happened. An incident whose only content
-- is a type and a severity requires the reader to reconstruct the case from
-- other tables.
ALTER TABLE incidents ADD COLUMN summary TEXT NOT NULL DEFAULT '';

-- Who is accountable, copied from the contract version in force at the moment
-- of the breach.
--
-- Copied rather than joined on purpose. The owner of a feed changes, and an
-- incident is a record of who was responsible *then*; re-deriving it later
-- would silently rewrite the answer.
ALTER TABLE incidents ADD COLUMN owner_subject TEXT NOT NULL DEFAULT '';
ALTER TABLE incidents ADD COLUMN escalation_policy_id TEXT NOT NULL DEFAULT '';

ALTER TABLE incidents ADD COLUMN resolved_at TIMESTAMP;
ALTER TABLE incidents ADD COLUMN resolved_by TEXT;

CREATE INDEX IF NOT EXISTS idx_incidents_open
    ON incidents(tenant_id, status, created_at);

-- ---------------------------------------------------------------------------
-- Notification intents
-- ---------------------------------------------------------------------------

-- The intent is written in the same transaction as the state change and
-- dispatched separately, so a crash between "the breach happened" and "someone
-- was told" cannot lose the obligation to tell them.

-- Stable identity, so a retried transition produces one intent rather than a
-- second alert about the same missing file.
ALTER TABLE notification_intents ADD COLUMN dedupe_key TEXT NOT NULL DEFAULT '';

-- The recipient and the policy under which they are being told.
--
-- Explicit columns rather than fields inside the JSON payload: a dispatcher has
-- to be able to route without parsing, and a routing decision made by digging
-- through a free-form payload is a routing decision nobody can audit.
ALTER TABLE notification_intents ADD COLUMN recipient TEXT NOT NULL DEFAULT '';
ALTER TABLE notification_intents ADD COLUMN escalation_policy_id TEXT NOT NULL DEFAULT '';

-- Delivery accounting. A dispatcher that cannot record a failure cannot
-- distinguish "not sent yet" from "tried and could not".
ALTER TABLE notification_intents ADD COLUMN attempt_count INTEGER NOT NULL DEFAULT 0;
ALTER TABLE notification_intents ADD COLUMN last_error TEXT;

CREATE UNIQUE INDEX IF NOT EXISTS idx_notification_dedupe
    ON notification_intents(tenant_id, dedupe_key)
    WHERE dedupe_key <> '';

CREATE INDEX IF NOT EXISTS idx_notification_undelivered
    ON notification_intents(tenant_id, delivered_at);

-- ---------------------------------------------------------------------------
-- Waivers
-- ---------------------------------------------------------------------------

-- WAIVED was modelled by Prompt 03 and unreachable: an occurrence everyone
-- agreed should never have existed still breached, and the only way to stop it
-- was to edit the row by hand.
--
-- A waiver requires an actor and a reason. A file that was expected and then
-- forgiven is a decision someone made, and an unattributed one is
-- indistinguishable from a bug that swallowed an alert.
ALTER TABLE expectations ADD COLUMN waived_by TEXT;
ALTER TABLE expectations ADD COLUMN waived_reason TEXT;
ALTER TABLE expectations ADD COLUMN waived_at TIMESTAMP;
