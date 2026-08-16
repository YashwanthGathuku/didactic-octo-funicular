# The evidence ledger

**Established by:** Prompt 09
**Authority:** `gateway/internal/ledger/` is the implementation; this document
describes it. Where they disagree, the code is correct and this file is a defect.

## What this is, and what it is not

It is an **application hash chain**. Each record carries the digest of its
predecessor, so altering or removing one breaks every record after it.

It is **not a Merkle tree**. There is no tree, no branch hashing, no inclusion
proof — the structure is a linear predecessor chain. Earlier code exported a
metric named `sentinel_merkle_chain_height` for exactly this structure, which
Prompt 01 corrected.

A **SHA-256 digest is not a digital signature**. Nothing here is signed.

A test enforces the vocabulary: `TestNoMerkleOrSignatureClaimsInThisPackage`
fails if an identifier like `MerkleRoot` or `SignedCheckpoint` appears in the
implementation.

## The concurrency defect this fixes

The previous `AppendAuditEvent` read the chain head on one connection, computed
the hash outside any transaction, and inserted on another. Two concurrent
appends could read the same predecessor. The per-tenant unique constraints on
`(tenant_id, sequence_no)` and `(tenant_id, previous_hash)` stopped that forking
the chain — but the loser received a constraint error and **its audit record was
simply dropped**. Under the worker pool added in Prompt 08, that is not
theoretical.

The whole read-compute-write is now one transaction, serialised per tenant:

- **PostgreSQL** locks the tenant row. Two appends for one tenant queue behind
  each other while different tenants proceed in parallel — one linear sequence
  per tenant, not one globally.
- **SQLite** relies on its single writer, with the unique constraints as the
  backstop on both.

A loser retries against the new head rather than being dropped.
`TestConcurrentAppendsProduceOneLinearSequence` runs 24 writers × 8 appends and
requires all 192 to land in one verifiable chain, on both databases.

## What a record contains

Tenant, sequence number, action, actor, object type and id, correlation id,
timestamp, payload, payload hash, previous hash, current hash, canonical
version. None is optional in practice: a record that cannot say who did what to
which object, under which request, is not evidence.

`Actor` is prefixed `system:` for a process and comes from verified claims for a
human, so the two can never be confused when reading the trail.

## Canonical serialisation is versioned

`ledger-canonical/1`, stored on every record. Changing how a record is
serialised changes every digest computed afterwards — without the version, a
serialisation change makes the entire prior chain fail verification and look
like tampering. With it, the verifier can say "this record was written under
rules I do not implement", which is a different and more useful statement than
"this record is corrupt".

Migration 007 marks pre-existing rows `ledger-canonical/0-legacy`, so a chain
containing them reads as "cannot be verified from here back" — which is true —
rather than as intact, which would not be.

Two details in the canonical form are load-bearing:

**Field separator.** Fields are joined with ASCII `RS` (0x1e), which cannot
appear in any field. A printable separator would let two different records
canonicalise identically: `"a|b"` + `"c"` and `"a"` + `"b|c"` concatenate the
same way. `TestCanonicalRecordSeparatorsAreUnambiguous` asserts this.

**Timestamp precision.** Timestamps are truncated to microseconds before
hashing. PostgreSQL's `TIMESTAMPTZ` holds microseconds, so a Go time carrying
nanoseconds loses precision on the round trip and the recomputed hash disagrees
— the verifier correctly reports it as tampering. All 192 concurrently written
records failed verification on PostgreSQL until this was fixed, while passing on
SQLite, which stores the formatted string verbatim.

Truncating before hashing keeps the verifier **strict**. The alternative —
comparing timestamps with a tolerance — would make a rewritten timestamp
undetectable within that tolerance.

## Verification

`Verify` detects mutation, deletion, reordering and broken predecessor links.
Those are four names for one underlying check: every record's stored hash must
equal the hash recomputed from its own fields, its `previous_hash` must equal
its predecessor's `current_hash`, and sequence numbers must be dense and
ascending from 1.

| Attack | Detected by |
|---|---|
| Payload rewritten | recomputed payload hash |
| Actor or any other field rewritten | recomputed record hash |
| Record deleted | sequence gap |
| Records reordered | sequence order and link |
| Link rewritten | predecessor mismatch |
| Record relabelled to an unknown canonical version | version check |

That last one matters: an unknown version is **reported**, not skipped. Skipping
would let an attacker exempt a record from checking by relabelling it.

`GetLedger` reports per-record status. Records before the break are `VERIFIED`;
the break is `BROKEN`; everything after is `UNVERIFIABLE_AFTER_BREAK`, because
every subsequent hash was computed from the broken one.

### The tamper tests drop the append-only triggers

Every tamper test failed with `audit_events is append-only` until a helper
dropped the triggers first. Weakening the trigger to make the tests pass would
have removed a production control to satisfy a test; dropping it inside the test
models the threat the hash chain exists to detect — someone who already has
database access. That the triggers hold is asserted separately in
`TestAuditEventsAreAppendOnlyToTheApplication`.

One tamper fixture also had to change: setting a record's `previous_hash` to the
genesis hash is refused by `UNIQUE (tenant_id, previous_hash)`, because record 1
already claims that predecessor. That constraint preventing a fork **is** the
constraint working, so the fixture uses an unclaimed hash instead.

## Payload rules

Sensitive material is refused, not redacted. `Append` rejects a payload whose
key names a credential (`secret`, `token`, `apiKey`, `password`, `accountNumber`,
`privateKey`, `rawData`, …) or whose value has the shape of an ACH record — 94+
characters, printable, predominantly digits.

This is a backstop. The control is that callers pass identifiers and
already-redacted findings: `internal/nacha` redacts where a finding is produced
and `internal/secrets` makes credentials unprintable. What this catches is the
caller who has not read either.

Payloads are bounded at 16 KiB. An unbounded payload is a storage vector and an
invitation to copy a file body into the ledger; the limit forces a reference.

The rule caught a real fixture: `TestLedgerDetectsActorTampering` used a payload
keyed `token`, so the append was refused, no record was written, and an empty
chain verified as intact — **the test had been passing for the wrong reason**.
The error is checked now.

## Periodic verification, and the anchoring gap

`RunVerifier` walks every tenant's chain hourly and records each result **into
the chain itself**, so a verification is subject to the same append-only
guarantees as everything else. Failures are recorded too: recording only
successes makes the trail's silence ambiguous between "not checked" and "checked
and broken", which call for very different responses.

**External anchoring is designed and not implemented, and is never claimed.**

`Checkpoint.Anchored` is a field that is always `false`, asserted by
`TestCheckpointsAreNeverClaimedAsAnchored`, and every checkpoint carries
`AnchorGapStatement` so an export cannot render one without its qualification.
`LedgerSummary` carries it too.

Why it matters, stated plainly: this chain proves internal consistency, not
authenticity. The party that can rewrite the rows is the party that can
recompute the digests. An operator with database write access can produce a
chain that verifies perfectly and describes events that never happened.
`Verify` returning `Intact: true` means **consistent**, not **trustworthy**.

The design that would close it: publish a checkpoint — tenant, sequence, head
hash, timestamp — somewhere this system's operator cannot rewrite. Signed by a
key held outside the deployment, or written to an append-only external log.
Rewriting history then requires compromising both. That needs a key or a service
this repository does not have.

## A SQLite deployment defect found on the way

The gateway opened `sql.Open("sqlite", cfg.DatabaseURL)` with **no pragmas at
all** — no busy timeout, no WAL. Under any concurrency that produces `database
is locked` errors the application cannot handle. It now sets `busy_timeout`,
`journal_mode=WAL` and `foreign_keys`.

A fourth setting was tried and **rejected**: `_txlock=immediate`. It fixes the
ledger's specific problem — a deferred transaction that reads then writes must
upgrade its lock, and SQLite refuses to *wait* on an upgrade, returning
`SQLITE_BUSY` immediately so `busy_timeout` does not apply. But it also breaks
the worker pool: a job handler's transaction stays open for the whole handler,
so one long-running handler holds the only write lock and every other worker
blocks at `BEGIN`, including workers that would have served a different tenant.
The per-tenant concurrency test from Prompt 08 caught it — the quiet tenant's job
never ran at all.

The ledger's bounded retry with jittered backoff handles the upgrade case
instead: 40 attempts, growing to 40 ms. That budget covers the whole queue
draining, not one collision, and it is sized from observed behaviour rather than
guessed.

A second defect: the adapter cached the ledger in a package variable, which held
whichever `*sql.DB` it saw first. In a test binary that is a database since
closed — every ledger test after the first failed with `sql: database is closed`.
It is constructed per call now.

## Verification

```
gofmt PASS · vet PASS · go test PASS · go test -race PASS
502 tests · 18 ledger properties × SQLite + real PostgreSQL 16
migrations 001–007 apply and are idempotent through the real command path
Container builds NOT RUN: no Docker daemon in this environment

observed: 24 concurrent writers produced one linear chain of 192 records
```

## What is not done

1. **The application still runs on SQLite.** The ledger is correct on both and
   PostgreSQL is verified against a real server, but `ledgerFor` passes
   `"sqlite"` and `migrations_postgres/` has no `payload_hash` or
   `canonical_version` column. This is the fourth subsystem verified on a
   database the product does not use — RLS, secret storage, the job queue, and
   now the ledger. The PostgreSQL port is overdue as its own piece of work.
2. **No append-only triggers in the PostgreSQL schema.** The SQLite migrations
   have them; the PostgreSQL counterpart does not, so
   `TestAuditEventsAreAppendOnlyToTheApplication` is SQLite-only and says so.
3. **No external anchoring.** See above. Designed, not built, never claimed.
4. **`AppendTx` has no callers.** The atomic path — an audit record committing
   with the state it describes — exists and is unused; every current call site
   appends in its own transaction, so a crash between the business commit and
   the audit append loses the record. The outbox covers this for validation
   events; it does not for the approval and triage handlers.
5. **The hourly verification interval is a default, not a measurement.** A full
   walk costs one pass over a tenant's records; the right interval depends on
   chain length and there is no production chain to measure.
6. **`auditSubject` infers the object from payload keys.** It recognises
   `artifactId`, `fileId`, `incidentId` and falls back to `system`. That is
   accurate rather than a guess, but the right fix is for call sites to state
   the subject explicitly.
