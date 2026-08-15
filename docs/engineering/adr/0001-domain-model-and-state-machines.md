# ADR 0001 — Domain model and state machines

**Status:** Accepted
**Date:** 15 August 2026
**Supersedes:** the implicit model in which status was a free-form string column

## Context

Before this decision there was no domain model. Status was a `VARCHAR` that any
handler could assign, and the release decision was a sequence of `if` statements
inside the ingestion function. Three consequences followed directly, all
reproduced at runtime and recorded in `CURRENT_STATE.md`:

1. A zero-byte file was persisted with `status = 'RELEASED'` because the result
   struct was initialised to that value and only downgraded on a positive
   finding.
2. Nothing distinguished "this file passed validation" from "a person decided
   this file may be used downstream". Ingestion did both.
3. No business table had a tenant column, so every query returned every
   tenant's rows and there was nothing for tenant isolation to enforce.

## Decision

### Transitions are data, and they live in one package

`gateway/internal/domain` declares four state machines — artifact, expectation,
job, decision — as explicit maps of legal edges. `TransitionTo` refuses any edge
the map does not contain. Handlers no longer assign status; they ask the domain
to move and handle refusal.

The artifact machine is the load-bearing one:

```
RECEIVED ──► VALIDATING ──► VALIDATED ──► APPROVED ──► RELEASED
    │             │              │            │
    └──► QUARANTINED ◄───────────┘            └──► REJECTED
              │
              └──► REJECTED
```

**There is no edge from RECEIVED or QUARANTINED to RELEASED.** The Prompt 01
defect is not merely fixed, it is unrepresentable: a caller cannot construct the
transition, and the database `CHECK` constraint refuses the value even if one
bypassed the domain entirely.

### Ingestion terminates at VALIDATED, never RELEASED

This is a deliberate behavioural change and it broke two tests, which were
updated rather than worked around. Ingestion is a machine deciding whether a
file is well-formed. Release is a statement that a file may be consumed by
downstream ledger processes. Those are different claims made by different
authorities, so they are different states with a human decision between them.

`AuthorizeRelease` is the single function that answers "may this be released",
and it requires all of:

- the artifact is in `APPROVED`
- a validation run exists, completed, parsed at least one record, and the parser
  succeeded
- no finding at or above `ERROR`
- a policy decision exists, carries a non-empty version, and is `APPROVED`
- the decision names this artifact's SHA-256, this artifact's ID, and this
  validation run's ID
- where policy requires it, an approval exists; where dual control is enabled,
  two *distinct* actor identities

The three identity bindings are the important part. An approval that does not
name the content it approved can be replayed against different bytes — which is
precisely how the removed self-healing endpoint could re-ingest arbitrary
content under a prior approval.

### Settlement is not a state

Money movement is performed by systems this product does not touch. It has no
state here, and `CHECK` constraints reject the value in every status column. A
test asserts this across tables, because the previous code returned
`SETTLED_INSTANT` from a validator that had parsed nothing.

### Tenancy is a column with teeth

`tenant_id` is `NOT NULL` on every business table, with a foreign key to
`tenants`. `TenantID` is a distinct Go type, and every `TransitionTo` refuses an
empty tenant.

**This is not yet tenant isolation, and the code says so.** The request path has
no identity to derive a tenant from until OIDC lands, so every write currently
uses `DefaultTenantID`. What exists today is the storage-level precondition: an
untenanted row cannot be written, so when authentication arrives there is
nothing to retrofit.

### Impossible states are rejected by the database, not only the application

Application-layer validation protects against mistakes. Storage-layer
constraints protect against bypass. Migration 002 adds:

- `CHECK` on every status column, restricted to the modelled set
- `UNIQUE (tenant_id, contract_id, business_date)` on occurrences, so two
  concurrent schedulers cannot both create one
- `UNIQUE (tenant_id, idempotency_key)` on jobs, so duplicate delivery collides
  instead of creating a second unit of work
- `UNIQUE (tenant_id, validation_run_id)` on decisions, so two concurrent
  finalizations cannot both produce an answer
- `UNIQUE (tenant_id, decision_id, actor_id)` on approvals, so one person cannot
  satisfy dual control by approving twice
- `UNIQUE (tenant_id, sequence_no)` and `UNIQUE (tenant_id, previous_hash)` on
  audit events, so a concurrent append loses on insert rather than forking the
  chain
- `CHECK (from_state <> to_state)` on status history

### History is append-only, enforced by trigger

`status_history` and `audit_events` reject `UPDATE` and `DELETE` via triggers.

This is a guard against the application, not against an attacker: anyone with
direct database access can drop a trigger. That is why hash-chain verification
by recomputation remains the real defence, and why the tamper tests now
explicitly drop the triggers first — they model an attacker who has already done
so. A separate test asserts the triggers themselves hold.

### Optimistic concurrency, not locks

Mutable rows carry `row_version`. Domain transitions increment it, and a refused
transition does not. Persistence compares-and-sets on it, so two callers holding
the same loaded row cannot both finalize: the loser's update matches zero rows.

## Consequences

**Good**

- The empty-file defect class cannot recur; it is unrepresentable at two layers.
- Release preconditions are one auditable function, not scattered conditionals.
- Duplicate delivery, double approval and concurrent finalization are database
  errors rather than silent data corruption.
- The schema is ready for real tenancy with no retrofit.

**Costs**

- Ingestion no longer produces `RELEASED`. Anything expecting that must go
  through approval. Two tests and the UI status mapping changed.
- Migration 002 rebuilds seven tables, because SQLite cannot add a `NOT NULL`
  column without a default or add `CHECK` to an existing table. It is
  transactional and tested against a legacy fixture, but it is not a cheap
  migration.
- Legacy rows with an unrecognised status are mapped to `QUARANTINED`. This
  fails closed: a row whose state we cannot interpret must not be treated as
  releasable.

**Deferred**

- The repository layer still issues SQL from `package main` rather than through
  a tenant-scoped repository interface. The constraint is enforced by the
  database today; enforcing it in a repository boundary is Prompt 04's work,
  alongside the identity that makes it meaningful.
- `validation_runs`, `policy_decisions`, `approvals`, `ingestion_jobs`,
  `job_attempts` and `notification_intents` exist as schema and domain types but
  are not yet written by the ingestion path. They are the substrate for Prompts
  07, 08 and 11.
- Money is still carried as `float64` in the API response struct. The database
  columns are integer minor units and the domain types are integer minor units;
  the remaining float is in the wire format and goes with Prompt 07.
