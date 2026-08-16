# Durable jobs, leases and the transactional outbox

**Established by:** Prompt 08
**Authority:** `gateway/internal/jobs/` and `gateway/worker.go` are the
implementation; this document describes them. Where they disagree, the code is
correct and this file is a defect.

## What this closes

`ingestion_jobs` and `job_attempts` were created by migration 002 in Prompt 03
and had **no consumer**. Ingest enqueued a row and nothing ever leased it, so
every uploaded artifact stayed `RECEIVED` forever. Validation only happened
because `ProcessFileBytes` ran it synchronously inside the request handler,
which is the behaviour Prompt 06 removed from the safe upload path.

That gap is now closed: `POST /api/v1/files/upload` returns 202, a worker leases
the job, reads the artifact from immutable storage, validates it, and records
the outcome — and a test asserts the artifact is `RECEIVED` before any worker
runs, so ingest cannot quietly go back to validating inline.

## The property everything rests on

A job is owned by exactly one worker at a time, and a crash at any point loses
no work and duplicates no side effect that matters.

Each failure mode below is invisible in normal operation, which is why each has
a test that constructs the race rather than asserting a flag.

| Property | Mechanism | Test |
|---|---|---|
| One owner per job | conditional claim on `row_version`, `SKIP LOCKED` on PostgreSQL | `TestFiftyWorkersRaceForOneJobAndExactlyOneWins` |
| Crash between commit and publish loses nothing | outbox written in the business transaction | `TestOutboxSurvivesACrashBetweenCommitAndPublish` |
| Stale worker cannot overwrite a newer completion | lease re-checked in the completing UPDATE | `TestStaleWorkerCannotOverwriteNewerCompletion` |
| Duplicate arrival is harmless | unique `(tenant_id, idempotency_key)` | `TestDuplicateArrivalProducesOneJob` |
| Duplicate outbox write is harmless | unique `(tenant_id, dedupe_key)` | `TestDuplicateOutboxWriteIsHarmless` |
| Poison job dies without blocking the queue | retry budget → `DEAD` | `TestPoisonJobReachesDeadWithoutBlockingTheQueue` |
| One tenant cannot consume all workers | quota enforced inside the claim | `TestOneTenantCannotConsumeAllWorkers` |

## Two databases, two claim strategies

PostgreSQL is the stated target and SQLite is what runs today. The claim
strategies genuinely differ, so both are implemented and both are tested.

**PostgreSQL** uses `SELECT ... FOR UPDATE SKIP LOCKED`. A row another
transaction holds is passed over rather than blocking, so N workers each take a
different row in one pass. Without it, workers serialise on the head of the
queue.

**SQLite** has no `SKIP LOCKED` and does not need one: it serialises writers, so
a conditional `UPDATE` guarded by `row_version` either matches one row or none.

The driver is passed to `jobs.New` explicitly rather than sniffed. A wrong guess
produces a queue that appears to work and races under load, which is the worst
possible failure for this component, so an unknown driver is refused.

## The per-tenant quota, and the bug that was in it

The first implementation counted a tenant's live leases, computed the saturated
set, and then claimed. `TestOneTenantCannotConsumeAllWorkers` failed on **both**
databases: four workers each counted two running jobs, each concluded there was
room, and each took a third.

That is a time-of-check-to-time-of-use gap, and the only fix is to make the
check atomic with taking the lease:

- **PostgreSQL** locks the tenant row (`SELECT ... FROM tenants ... FOR UPDATE`)
  before counting. Under `READ COMMITTED` a concurrent claimer's uncommitted
  lease is invisible, so counting alone undercounts. Locking the tenant
  serialises same-tenant claims — which is exactly the resource being bounded —
  while different tenants proceed in parallel.
- **SQLite** folds the count into the `UPDATE`'s `WHERE` clause. SQLite
  evaluates that while holding the write lock, so the check and the lease are
  one atomic step.

The pool still pre-filters saturated tenants, but only as a hint that saves a
pass. Correctness does not depend on it.

## The transactional outbox

An event is written in the same transaction as the business state it describes.
That is the whole point: a process that commits an artifact's validation and
then dies before publishing has still recorded the event, and the dispatcher
delivers it on the next pass. Publishing after commit loses the event on exactly
the failure it needs to survive.

The dispatcher is separate from the workers, for a reason worth stating: a
worker that published its own events would have to do so either inside its
transaction — making an external call part of a database transaction, holding
locks across a network — or after it, in a gap where a crash loses the event.

Ordering within the dispatcher is fetch, deliver, **then** mark. Marking before
delivering loses every event whose delivery then fails. So delivery may repeat,
and `dedupe_key` is provided so consumers can recognise a repeat. **At-least-once
is what this guarantees**; exactly-once across a process boundary is not
something any design here provides.

An undeliverable event is parked (`dead_at`), never deleted. A record that
something happened is not discarded because a consumer was down.

## Backoff

Exponential with **full jitter**, capped at 15 minutes. The jitter is not
cosmetic: without it a batch of jobs that failed together retries together,
reproducing the load that caused the failure at exactly the wrong moment.
`TestBackoffIsJitteredAndBounded` requires 200 samples to produce at least 50
distinct delays.

## Graceful shutdown

`Stop` clears the leasing flag first, then waits. The two halves are separate
deliberately: clearing the flag stops new claims immediately so the pool drains
rather than picking up work it will then abandon; waiting lets held jobs
complete so their leases are released properly rather than left to expire, which
would delay their retry by the whole lease duration on the next process.

In `main.go` the HTTP server shuts down **before** the pool drains, so no request
is answered by a process that can no longer do the work it promises. A final
dispatch pass runs after the pool stops, so events from the last jobs are
delivered rather than waiting for the next process.

If the drain times out, `Stop` says so rather than reporting a clean shutdown
that did not happen.

## Backpressure

`Pool.EnqueueTx` refuses with `ErrOverloaded` above `MaxQueueDepth`. An accepted
job nobody will run is worse than a refused one: the caller believes it was
scheduled.

Every field of `PoolConfig` is a bound and there is deliberately no "unlimited"
setting. A pool whose heartbeat interval is not shorter than the lease duration
is refused at construction, because a healthy worker's lease would expire under
it.

## Idempotent handlers

At-least-once delivery makes handler idempotency a requirement, not a nicety.
`validateArtifactHandler` short-circuits when the artifact is already
`VALIDATED` or `QUARANTINED`, and replaces findings rather than appending them,
so a retry after a partial write does not leave two copies.

A storage failure retries; a malformed file quarantines. Quarantining an
artifact because the object store was briefly unavailable would blame the file
for an outage.

## A test-harness defect found on the way

`setupTestDb` used `sqlite ":memory:"`. An in-memory SQLite database belongs to
a single connection — open a second and it gets its own empty database. With
`database/sql` pooling that is invisible until a test raises `MaxOpenConns`, at
which point a concurrency test sees "no such table" or, worse, passes because
each goroutine is looking at a different empty queue.

It is now file-backed with `busy_timeout`. Every concurrency test in this
repository written before this change was running against per-connection
databases.

## Observed results

The stress test reports what it observed and asserts nothing about duration or
throughput. This machine's numbers are not a claim about any other machine, and
a hardcoded threshold would be either meaningless or flaky.

```
observed: 40 artifacts settled in 106ms;
          jobs claimed=40 succeeded=40 failed=0 leases-lost=0
```

What it does assert: every artifact settled, none reached `RELEASED`, exactly
one job succeeded per artifact, exactly one outbox event per artifact, and no
artifact has duplicated findings from a retry.

## Verification

```
gofmt PASS · vet PASS · go test PASS · go test -race PASS · tsc PASS · npm build PASS
internal/jobs: 12 properties × 2 backends, all PASS
  - SQLite via the real migrations
  - PostgreSQL 16 via a real server, SKIP LOCKED exercised
Container builds NOT RUN: no Docker daemon in this environment.
```

CI runs the jobs suite against a PostgreSQL service container **and asserts the
PostgreSQL half was not skipped**, because a concurrency test that quietly does
not run is worse than no test.

## What is not done

1. **The application still runs on SQLite.** The queue is correct on both, and
   PostgreSQL is verified against a real server, but the running process opens
   SQLite and `startWorkers` passes `"sqlite"`. The job tables are also absent
   from `migrations_postgres/`, which is honest — the port has not happened — but
   means the PostgreSQL schema in that file is not yet complete for this
   subsystem.
2. **`POST /files/ingest-raw` still validates synchronously.** It does not
   enqueue. Two paths now exist: `/files/upload` is asynchronous and durable,
   `/files/ingest-raw` is neither. The UI uses the second.
3. **The only deliverer is the audit ledger.** SSE broadcast and external
   notification are the obvious next consumers; `/api/v1/stream` still emits
   only a connect heartbeat, so wiring the dispatcher to it is Prompt 12.
4. **No dead-letter surface.** A `DEAD` job and a parked outbox event are both
   queryable but nothing lists them, alerts on them, or offers a retry. An
   operator would have to run SQL.
5. **No scheduled sweeper.** Jobs whose leases expire are reclaimed by the next
   claim, which is fine while workers are running, but nothing reclaims them if
   the pool is stopped and a job was left `LEASED`.
6. **Per-tenant quotas are not configurable through any interface.**
   `tenant_job_quotas` exists and is read; nothing writes it but SQL.
7. **The lease duration and pool size are not tuned.** They are conservative
   defaults, and this repository has no measured throughput to tune against. A
   plausible-looking figure chosen without measurement is exactly the kind of
   claim this programme exists to remove.
