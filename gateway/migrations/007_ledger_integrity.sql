-- Evidence ledger: canonical serialisation and payload integrity.
--
-- The chain already had per-tenant sequence and predecessor uniqueness from
-- migration 002, which is what stopped concurrent appends forking it. What it
-- lacked was any way to verify a record's own contents: a row's payload could
-- be altered and only the chain hash would disagree, with no separate check on
-- the payload itself, and nothing recorded which serialisation produced a
-- given digest.

-- The digest of the record's canonical payload, computed separately from the
-- chain hash. It lets a verifier confirm a payload is unaltered without
-- re-deriving the whole record, and it lets an export redact further while
-- still carrying proof of what the original said.
ALTER TABLE audit_events ADD COLUMN payload_hash TEXT NOT NULL DEFAULT '';

-- Which serialisation rules produced this record's hashes.
--
-- Without it, changing how a record is canonicalised makes every prior record
-- fail verification and look like tampering. With it, a verifier can say "this
-- record was written under rules I do not implement" -- which is a different
-- and more useful statement than "this record is corrupt".
ALTER TABLE audit_events ADD COLUMN canonical_version TEXT NOT NULL DEFAULT '';

CREATE INDEX IF NOT EXISTS idx_audit_events_stream
    ON audit_events(tenant_id, sequence_no);

-- A record's correlation id ties together everything one request or one job
-- produced. Indexed because reconstructing an incident means querying by it.
CREATE INDEX IF NOT EXISTS idx_audit_events_correlation
    ON audit_events(tenant_id, correlation_id);

-- Rows written before this migration carry no payload hash or canonical
-- version, and they must not be silently treated as verifiable.
--
-- They are marked as belonging to a legacy serialisation. The verifier reports
-- a record whose canonical_version it does not implement as a break, with a
-- reason naming the version -- so a chain containing legacy rows reads as
-- "cannot be verified from here back", which is true, rather than as "intact",
-- which would not be.
UPDATE audit_events
SET canonical_version = 'ledger-canonical/0-legacy'
WHERE canonical_version = '';
