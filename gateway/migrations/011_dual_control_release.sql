-- Human review and dual-control release.
--
-- Migration 002 created `policy_decisions`, `approvals` and `validation_runs`,
-- and nothing has ever written to any of them. The separation-of-duties
-- constraint on `approvals` -- UNIQUE (tenant_id, decision_id, actor_id) -- has
-- been correct and unreachable since it was written, because no code path
-- created a decision to approve.
--
-- The one route that did exist, POST /incidents/{id}/approve, resolves an
-- incident. It has never released anything.

-- ---------------------------------------------------------------------------
-- Per-tenant release policy
-- ---------------------------------------------------------------------------

-- Dual control is configurable because it is a commercial and regulatory
-- choice, not a technical one: a tenant moving payroll for three people and one
-- moving a national ACH file want different answers.
--
-- The defaults are the strict ones. A tenant with no row gets two approvals and
-- separation of duties, so a deployment that forgets to configure this is
-- stricter than intended rather than weaker.
CREATE TABLE IF NOT EXISTS release_policies (
    tenant_id            TEXT PRIMARY KEY REFERENCES tenants(id),

    -- How many distinct people must approve before a release is permitted.
    min_approvals        INTEGER NOT NULL DEFAULT 2 CHECK (min_approvals >= 1),

    -- Whether the person who proposed a release may also approve it. With this
    -- on, the proposer's approval does not count towards the threshold.
    separation_of_duties INTEGER NOT NULL DEFAULT 1,

    -- Whether a manual override may bypass the threshold at all. A tenant that
    -- turns this off cannot be talked into a release by an operator under
    -- pressure, which is the situation overrides exist for and also the
    -- situation they are most abused in.
    override_allowed     INTEGER NOT NULL DEFAULT 1,

    updated_by           TEXT NOT NULL DEFAULT '',
    updated_at           TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP
);

-- ---------------------------------------------------------------------------
-- Decisions
-- ---------------------------------------------------------------------------

-- Optimistic concurrency. Two reviewers acting at the same instant must produce
-- one legal outcome, not a last-writer-wins overwrite of the other's decision.
ALTER TABLE policy_decisions ADD COLUMN row_version INTEGER NOT NULL DEFAULT 0;

-- What the decision was made about, as a single digest.
--
-- This is what makes "an approval expires if the source, findings, policy or
-- proposed artifact change" enforceable rather than aspirational. The digest
-- covers the artifact's bytes, the validation run, the policy version, the
-- governing contract version, and every finding's rule id and severity.
-- Re-deriving it at release time and comparing is one check that catches all
-- four kinds of change; storing the parts separately would mean four checks,
-- and the one somebody forgets is the one that matters.
ALTER TABLE policy_decisions ADD COLUMN integrity_digest TEXT NOT NULL DEFAULT '';

-- The findings digest on its own, so a report can say *which* part changed
-- without recomputing everything.
ALTER TABLE policy_decisions ADD COLUMN findings_digest TEXT NOT NULL DEFAULT '';

-- Who proposed the release, and when. Separation of duties needs to know.
ALTER TABLE policy_decisions ADD COLUMN proposed_by TEXT NOT NULL DEFAULT '';
ALTER TABLE policy_decisions ADD COLUMN proposed_at TIMESTAMP;

-- The threshold in force when the decision was created.
--
-- Copied rather than read from release_policies at release time. Changing the
-- policy must not retroactively satisfy a decision that was proposed under a
-- stricter one -- that would let an operator lower the threshold and release
-- something already half-approved.
ALTER TABLE policy_decisions ADD COLUMN required_approvals INTEGER NOT NULL DEFAULT 2;
ALTER TABLE policy_decisions ADD COLUMN separation_of_duties INTEGER NOT NULL DEFAULT 1;

-- The contract version whose terms governed the validation this decision rests
-- on. An approval reviewed months later has to state the terms that applied.
ALTER TABLE policy_decisions ADD COLUMN contract_version_id INTEGER;

-- Why the decision is no longer live, when it is not.
ALTER TABLE policy_decisions ADD COLUMN expired_reason TEXT;

ALTER TABLE policy_decisions ADD COLUMN released_at TIMESTAMP;
ALTER TABLE policy_decisions ADD COLUMN released_by TEXT;

CREATE INDEX IF NOT EXISTS idx_decisions_queue
    ON policy_decisions(tenant_id, state, decided_at);

-- ---------------------------------------------------------------------------
-- Votes
-- ---------------------------------------------------------------------------

-- A rejection is recorded the same way an approval is.
--
-- The table was named `approvals` and could only hold one side, so a reviewer
-- who examined a file and refused it left no record -- and the next reviewer
-- had no way to know it had already been looked at. The vote column makes the
-- refusal evidence.
ALTER TABLE approvals ADD COLUMN vote TEXT NOT NULL DEFAULT 'APPROVE'
    CHECK (vote IN ('APPROVE','REJECT'));

-- The digest the reviewer actually saw.
--
-- A reviewer approves a specific state of the world. If the artifact's findings
-- change after they voted, their vote no longer describes what would be
-- released, and it must not be counted. Recording what they saw is what lets
-- that be checked rather than assumed.
ALTER TABLE approvals ADD COLUMN integrity_digest TEXT NOT NULL DEFAULT '';

-- ---------------------------------------------------------------------------
-- Overrides
-- ---------------------------------------------------------------------------

-- A manual override is a separate table, not a flag on the decision.
--
-- "Separately reportable" is the requirement, and a flag buried in a decision
-- row is not separately reportable in any useful sense -- it is found only by
-- someone who already suspects it. A table can be listed, counted, and put in
-- front of an auditor without anyone needing to know it exists.
--
-- An override never rewrites the validation result. The findings stay exactly
-- as they were; the override records that a human released the artifact anyway,
-- and who.
CREATE TABLE IF NOT EXISTS release_overrides (
    id                INTEGER PRIMARY KEY AUTOINCREMENT,
    tenant_id         TEXT NOT NULL REFERENCES tenants(id),
    decision_id       INTEGER NOT NULL REFERENCES policy_decisions(id),
    file_instance_id  INTEGER NOT NULL REFERENCES file_instances(id),

    actor_id          TEXT NOT NULL CHECK (length(actor_id) > 0),
    role              TEXT NOT NULL,
    reason            TEXT NOT NULL CHECK (length(trim(reason)) >= 20),

    -- What was bypassed, in the reviewer's own terms and in the system's.
    bypassed          TEXT NOT NULL,
    approvals_held    INTEGER NOT NULL DEFAULT 0,
    approvals_required INTEGER NOT NULL,

    -- The blocking findings at the moment of override, so a later reader sees
    -- what the override was made against rather than the current state.
    blocking_rule_ids TEXT NOT NULL DEFAULT '',
    integrity_digest  TEXT NOT NULL DEFAULT '',

    created_at        TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    UNIQUE (tenant_id, decision_id)
);

CREATE INDEX IF NOT EXISTS idx_overrides_recent
    ON release_overrides(tenant_id, created_at);

-- ---------------------------------------------------------------------------
-- Validation runs
-- ---------------------------------------------------------------------------

-- The run's decision inputs, so a decision can name the run it rests on and a
-- reader can reconstruct it.
ALTER TABLE validation_runs ADD COLUMN policy_version TEXT NOT NULL DEFAULT '';
ALTER TABLE validation_runs ADD COLUMN contract_id TEXT NOT NULL DEFAULT '';
ALTER TABLE validation_runs ADD COLUMN contract_version TEXT NOT NULL DEFAULT '';
ALTER TABLE validation_runs ADD COLUMN outcome TEXT NOT NULL DEFAULT '';
ALTER TABLE validation_runs ADD COLUMN findings_digest TEXT NOT NULL DEFAULT '';
ALTER TABLE validation_runs ADD COLUMN blocking_rule_ids TEXT NOT NULL DEFAULT '';

CREATE INDEX IF NOT EXISTS idx_runs_artifact
    ON validation_runs(tenant_id, file_instance_id, started_at);
