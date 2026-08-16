# Feed contracts, calendars, and materialized expectations

**Established by:** Prompt 10
**Authority:** `gateway/internal/schedule/` is the implementation; this document
describes it. Where they disagree, the code is correct and this file is a defect.

## The problem

A file that never arrives generates no event. Everything downstream of ingest
reacts to arrivals, so a partner who simply stops sending is invisible: there is
nothing to process, nothing to validate, nothing to quarantine, and no alert to
raise. The failure presents as silence, which is indistinguishable from a quiet
day.

The answer is to write the row before the file is due. An **occurrence** is
created ahead of time and ages into `OVERDUE` and `BREACHED` by the passage of
time alone. Nothing has to happen for a missing file to be detected; the
detection *is* nothing happening.

Prompt 03 created `expectations` and `file_contract_versions` and nothing ever
wrote to either. The table designed to make a missing file visible was itself
empty.

## Determinism

Nothing in `internal/schedule` imports the AI tier, reads past arrivals, or
consults a model. With zero history and every optional subsystem offline, the
schedule is fully determined by the contract version, the calendar, and the
clock. `TestScheduleIsDeterministicWithZeroHistory` asserts that two identically
configured contracts produce byte-identical windows.

## What a contract version carries

Immutable, and identified by `(tenant, contract, version)`:

| | |
|---|---|
| tenant, partner, feed id, direction | who, with whom, which feed, which way |
| filename matcher | a glob, not a regex — see below |
| format | `NACHA` only; the others are experimental (SCOPE.md) |
| expected local time + IANA timezone | `09:00` in `America/New_York`, not `13:00Z` |
| grace period, breach delay | two different numbers, see below |
| business calendar id, schedule rule, non-business-day action | which days |
| balanced mode | `BALANCED` or `UNBALANCED_AUTHORIZED` |
| owner, escalation policy reference | who gets told |
| effective interval, version number | when these terms applied |

**Editing terms creates a version; it never updates a row.** A historical
occurrence stores the version id it was materialized under, and `VersionOn`
resolves any business date to the version in force on that date. Changing
today's terms cannot alter whether last month's file was late. Backdating a
version over an existing one is refused outright — it would rewrite which terms
governed dates that have already been judged and possibly already breached.

The effective interval is **half-open**, `[from, to)`. A closed interval would
make the last day of the old version and the first day of the new one the same
day, and an occurrence on that day would resolve to whichever row the database
returned first.

### Grace and breach are two numbers

Grace is the tolerance the partner is given. The breach delay is how long the
tenant waits before treating lateness as an incident. Collapsing them into one
makes `OVERDUE` unreachable — the occurrence would pass from `DUE` straight to
`BREACHED` at a single instant, and the state that exists to be acted on before
it becomes a breach would never be observed by a scheduler running at any finite
interval. A zero breach delay is refused for that reason.

## The Federal Reserve calendar

Encoded as **rules, not a table of dates**. The Federal Reserve publishes
observed dates only a few years ahead, and a scheduler that runs out of holiday
rows does not fail loudly — it marks Christmas Day a business day and reports
every partner as late.

The observance rule is the Reserve Banks' own, and it is **not** the federal
employee rule:

> For holidays falling on Saturday, Federal Reserve Bank offices are open the
> preceding Friday. For holidays falling on Sunday, all Federal Reserve Bank
> offices are closed the following Monday.

Federal *employees* get the preceding Friday off for a Saturday holiday; banks
do not. Encoding the employee rule would mark a normal Friday closed and
suppress a real expectation — producing the silently missing file this whole
subsystem exists to prevent.

Two consequences that catch naive implementations, and that the tests pin:

- **4 July 2026 is a Saturday.** It is not observed at all. Friday 3 July stays
  a business day and Monday 6 July is normal.
- **25 December 2027 is a Saturday**, likewise unobserved, as is Juneteenth 2027.

`TestFederalReserveHolidaysMatchThePublishedSchedule` checks the encoded rules
against a hand-entered fixture of the published dates for 2024–2027, walking
every day of each year so an *extra* holiday fails as loudly as a missing one.

**Not verified:** no network fetch of the Federal Reserve's published calendar
occurs. The fixture is hand-entered from the published schedule; years beyond it
are rule-derived, which is stated rather than claimed as published.

Juneteenth carries `SinceYear: 2021`, so a date before the holiday existed is
not retroactively made one. Its first *observed* Reserve Bank closure was Monday
20 June 2022, because 19 June 2021 fell on a Saturday.

### Tenant overrides

Both directions, each with a mandatory reason. Closing a day the base calls open
suppresses an expectation, so a wrong closure hides a missing file; opening a day
the base calls closed creates one, so a wrong opening raises a false alarm. An
override with no reason is refused — an unexplained closure is indistinguishable
from a mistake at the moment someone has to account for a file nobody was
waiting for.

A calendar that is missing is an error, never a default. Falling back to a
weekday calendar would put a Christmas Day expectation in the queue and show the
operator who mistyped the id a plausible schedule rather than a failure.

## Timezones and daylight saving

The contract says "09:00 Eastern", not "13:00 UTC". Those are the same statement
for eight months of the year and different for the other four; storing the UTC
form makes a file an hour late every spring with nothing in the record to
explain it.

Timezone abbreviations are refused. `EST` names a fixed offset and does not
observe daylight saving, so a contract stored that way is an hour wrong for two
thirds of the year and the bug presents as a partner who is mysteriously always
early.

`time.Date` silently normalises both awkward cases and documents that it does
not guarantee which side it lands on. Observed on this build, 02:30 on
2025-03-09 in `America/New_York` comes back as **01:30 EST** — an hour *earlier*
than contracted — and another tzdata build may answer 03:30 EDT. Neither answer
is announced and neither is stable.

Both cases are resolved explicitly and the resolution is persisted with the
occurrence:

- **Spring-forward gap.** The contracted time does not exist. The deadline
  becomes the instant the local clock reaches the far side of the jump — 03:00
  for a contracted 02:30, found by bisecting the UTC offset. It is the first
  moment at which the contracted time can be said to have passed, so it is the
  strictest defensible reading and never moves a deadline later than the partner
  expects.
- **Fall-back ambiguity.** The contracted time occurs twice. The earlier instant
  is taken: a file arriving during the repeated hour is on time under the later
  reading and late under the earlier one, and treating a doubtful case as late
  raises a reviewable alert instead of silently passing one.

The bisection bounds are the previous and following **noon**, not midnight —
some zones shift at midnight (`Asia/Beirut`, `America/Santiago` have both done
so), and a midnight bound would itself be the non-existent wall time, normalised
by `time.Date` onto the far side of the very transition being searched for.

**Grace is added in absolute time, not clock time.** Sixty minutes is sixty
minutes. On a fall-back date a wall-clock addition would give the partner 120
real minutes; on a spring-forward date, none.
`TestGraceIsAbsoluteTimeAcrossADstTransition` pins this by asserting the grace
ends at `01:30 EST` — the *second* 01:30 — where a clock-face addition would
have landed on `01:30 EDT`, one real hour earlier.

## Schedule rules

A small closed grammar, not cron:

```
EVERY_BUSINESS_DAY
WEEKLY:MON,WED,FRI
MONTHLY:1,15
MONTHLY:LAST
```

Cron cannot express "the last day of the month" without a vendor extension, has
no notion of a business calendar, and is misread often enough that a wrong
schedule — which fails by never materializing anything — is a realistic outcome.

`MONTHLY:29`, `:30` and `:31` are **refused rather than clamped**. A contract
that says "the 31st" has an unanswerable question in it for four months of the
year, and clamping answers it silently — the partner and the gateway then
disagree about which day the file was due, in exactly the months where it
matters. `LAST` states the intent.

The non-business-day action is `SKIP`, `PRECEDING` or `FOLLOWING`. It is
overridden to `SKIP` for `EVERY_BUSINESS_DAY`, which nominates every date and
lets the calendar do the filtering: honouring `FOLLOWING` there would move
Saturday and Sunday onto Monday, which already has its own occurrence, turning a
weekend into a phantom triple delivery.

Two nominal dates can adjust onto one business day. The occurrence table permits
one row per contract per date, so the collision is **merged with a note** rather
than left for a unique constraint to reject — a constraint violation is not a
place to discover that two of a contract's deliveries landed on the same day.

`Slots(from, to)` clips on the **business date**, not the nominal one: a date
outside the window that adjusts into it is included. Dropping it would leave a
hole at every rolling-window boundary, and the scheduler materializes in rolling
windows.

## Materialization is idempotent by construction

The occurrence table has a unique key on `(tenant, contract, business_date)` and
every insert is a conflict-do-nothing. A restart mid-pass, two schedulers running
concurrently, and a pass overlapping the previous one all converge on exactly one
row per business date.

**No lease, no leader election, no advisory lock.** The constraint is the
coordination mechanism, and unlike a lease it cannot be lost, expire, or be held
by a process that has already died.
`TestTwoConcurrentSchedulersProduceOneOccurrence` runs six schedulers against one
database on both backends.

A **future, unmatched, `PENDING`** occurrence is re-pointed when its governing
version changes. Every clause of that guard is load-bearing: re-pinning one whose
deadline has passed would move a deadline already judged against; re-pinning a
matched one would contradict an arrival already attributed; re-pinning one under
review would discard the question a human was asked.

The horizon is 14 days. It has to exceed the longest gap between scheduler runs
by a wide margin — an occurrence that was never written cannot become overdue, so
a gap in materialization is a window in which a missing file is invisible.

## Advancement walks every state

A scheduler that was down over a weekend finds occurrences still `PENDING` whose
breach time passed on Friday. Writing `BREACHED` straight onto `PENDING` would be
an illegal edge under the Prompt 03 state machine, and would also lose the `DUE`
and `OVERDUE` history entries — the record would show a file that was never due
and then breached. Every intermediate transition is applied and recorded instead.

State is a total function of the window and the clock, with half-open boundaries
so an instant belongs to exactly one state:

| | |
|---|---|
| `now < DueAt` | PENDING |
| `DueAt ≤ now < GraceEndsAt` | DUE |
| `GraceEndsAt ≤ now < BreachesAt` | OVERDUE |
| `BreachesAt ≤ now` | BREACHED |

A file arriving exactly at the deadline is on time. Agreements say "by 09:00";
none makes 09:00:00.000 late.

Each step is a conditional update on the occurrence's `row_version`, so
concurrent schedulers produce one set of transitions and the loser records
nothing. `TestTwoConcurrentSchedulersProduceOneSetOfTransitions` asserts no
duplicate history row exists.

## Arrival matching records ambiguity rather than resolving it

The filename matcher is a glob with date tokens (`{YYYY}{MM}{DD}`, `{YY}`,
`{JJJ}`), never a regular expression. Patterns are tenant configuration written
by operations staff reading a partner's spec sheet: `ACH_*.txt` is checkable by
eye. A pattern of nothing but wildcards is refused, because the resulting
behaviour — every arrival ambiguous against every open occurrence — is
indistinguishable from the system being broken. Matching is case-insensitive:
partners change filename case without telling anyone, and a case-sensitive miss
presents as a missing file, an alert about the wrong thing that also hides the
arrival.

The matcher is iterative with one backtrack point per star.
`TestPatternMatchDoesNotBlowUpOnAdversarialInput` runs `a*a*a*a*a*a*a*b` against
4000 `a`s, which is exponential in the usual recursive formulation.

**The central rule: when an arrival could satisfy more than one occurrence,
nothing is attributed.** Picking the closest deadline is the obvious heuristic
and it is wrong in the way that matters — a wrong guess marks one occurrence
`ARRIVED` and leaves a genuinely missing file recorded as delivered. The system
would then assert, with an audit trail behind it, that a file it never saw
arrived. Every candidate is recorded in `expectation_match_candidates` and a
human decides.

An occurrence under review **keeps ageing**. It has not been shown to have
arrived, so freezing it would let a wrong guess stop the clock on the file that
did not come. `TestAmbiguousOccurrenceStillBreaches` pins this.

Matching runs **inside the ingest transaction**. Attributing separately would
allow a crash in between, leaving a file that arrived and an expectation saying
it did not — which reads, from every report, as a missing file.

Other outcomes: a second file for an already-satisfied occurrence is a
`DUPLICATE` review, not a replacement attribution. A file nothing was expecting
is `UNEXPECTED`, which is not an error — it is stored, validated and quarantined
on its own merits. A late arrival after breach is attributed and the history
records `BREACHED -> ARRIVED` with a "late arrival after breach" reason, because
the breach happened and a report of final states would otherwise show a clean
day.

The match window is 10 days back and 2 forward. A partner sending Monday's file
on Wednesday is ordinary; one satisfying a three-month-old expectation is not,
and matching it would silently close a breach already reported and acted on.

## Contract terms now reach validation

`nacha.DefaultContract` was applied to every artifact of every tenant, which
meant either failing the counterparties authorised to send unbalanced files or
passing the ones that are not. `contractForArtifact` resolves the terms from the
contract version governing the occurrence the artifact satisfied. An artifact
matching no expectation still validates under the default, and `ContractID` stays
empty so "no contract was applied" remains visible rather than inferred from
silence.

## The demo seed

Previously it wrote regex-style patterns (`^MERIDIAN_NAV_.*\.csv$`) that nothing
in this repository can match, and inserted an expectation row by hand with
deadlines computed by SQLite date arithmetic in UTC rather than in the contract's
timezone. A developer would have seen a populated board produced by a mechanism
that is not the scheduler — the most misleading possible demo of a scheduler. It
now seeds a calendar and contract versions only; occurrences come from the
scheduler alone, and `TestDemoSeedProducesAMaterializableSchedule` asserts both.

## What a breach now does

Detection without escalation is a system that knows a file is missing and does
not say so. Every step below happens **inside the transaction that records the
`BREACHED` transition**, so a crash cannot leave the breach on record with
nobody informed:

1. **An incident opens**, one per occurrence, idempotent through a partial
   unique index on `(tenant, expectation_id, type)`. Idempotent through the
   constraint rather than a read-then-write, because two schedulers can reach
   this at once and a check followed by an insert lets both through. A duplicate
   alert is how an operator's queue stops being read.
2. **A notification intent is written**, addressed to the contract version's
   `owner_subject` under its `escalation_policy_id`. The recipient and policy are
   explicit columns, not fields inside the payload: a routing decision made by
   digging through free-form JSON is a routing decision nobody can audit.
3. **An outbox event is published**, which the Prompt 08 dispatcher carries into
   the evidence ledger. Publishing rather than delivering is the point — calling
   a notification service from inside a write transaction would hold it open
   across a network call and would alert on a transaction that may still roll
   back.

The incident copies the owner and policy rather than joining to them. A feed's
owner changes, and an incident records who was responsible *then*; re-deriving
it later would silently rewrite the answer.

The escalator is a narrow interface taking the transaction, and
`internal/schedule` does not import `internal/jobs`: what a breach means beyond
"open an incident and record who to tell" is the application's decision.
`TestAFailingEscalatorRollsBackTheTransition` asserts the whole thing is atomic —
if escalation fails, the occurrence is not left BREACHED.

The alert payload carries identifiers and deadlines only. A notification is the
most widely distributed artifact this system produces, so it is the worst place
for anything derived from file content; the ledger's own payload rules refuse
one that carries it, which the wiring test relies on as a second check.

## Verification

```
gofmt PASS · vet PASS · go test PASS · go test -race PASS
internal/schedule: 60 test functions; the 21 database-backed ones run
                  against SQLite and real PostgreSQL 16
Federal Reserve rules checked against published dates for 2024, 2025, 2026, 2027
migrations 001–008 apply and are idempotent through the real command path
Container builds NOT RUN: no Docker daemon in this environment
```

## What is not done

1. **No HTTP surface for contract or calendar management.** Contracts,
   versions, calendars and overrides are created through the `internal/schedule`
   API, exercised by tests and the demo seed. There is no authenticated route to
   create or amend one, so a real deployment configures feeds by direct database
   access. Prompt 12 lists contract management and version history as screens;
   the routes behind them do not exist yet.
2. ~~**Nothing consumes a breach.**~~ **Closed.** See "What a breach now does"
   below. A breach opens an incident, records a notification intent addressed to
   the contract version's owner under its escalation policy, and publishes an
   outbox event that reaches the evidence ledger — all inside the transaction
   that recorded the transition. What remains undone is the **last mile**: no
   channel actually delivers the notification. `PendingNotifications` returns
   the queue and nothing drains it, so an operator must read the incident list
   rather than being paged. That is a smaller and more honest gap than the
   original — the obligation is durable and attributed, it is just not yet
   posted anywhere.
3. ~~**No review resolution path.**~~ **Closed.** `ResolveCandidate` accepts or
   rejects a candidate with a required actor and reason. Accepting attributes
   the artifact and closes the same artifact's other candidates; rejecting
   leaves the occurrence ageing. Still missing: an HTTP route to call it (see
   item 1) and any notion of who is *permitted* to resolve a review beyond the
   tenant check.
4. ~~**`WAIVED` is unreachable.**~~ **Closed.** `Waive` requires an actor and a
   reason, refuses an already-arrived occurrence, writes the status history, and
   resolves the occurrence's open incident. Still missing: the same HTTP route
   gap, and no approval step — one person can waive without a second signature.
5. **The application still runs on SQLite.** The scheduler is verified against a
   real PostgreSQL 16 server, and `migrations_postgres/` carries none of these
   tables — the PostgreSQL half of the suite builds the schema itself. This is
   the fifth subsystem in that position: RLS, secret storage, the job queue, the
   evidence ledger, and now scheduling. The port is overdue as its own piece of
   work.
6. **The scheduler is not leader-elected.** Running two processes is safe and
   produces one set of rows, but both do the whole scan every minute. That is
   correct and wasteful; at a few hundred contracts it does not matter, and
   there is no production deployment to measure.
7. **No outbound feed handling.** `Direction` is stored and validated;
   `OUTBOUND` contracts materialize occurrences exactly like inbound ones and
   nothing sends anything. An outbound occurrence can therefore only ever be
   satisfied by a file arriving, which is not what outbound means.
8. **Calendar overrides have no expiry or audit.** A tenant override applies
   forever once written, is not versioned, and changing one silently changes
   future materialization. Contract versions were given this treatment;
   calendars were not.
9. **The materialization horizon and run interval are defaults, not
   measurements.** Fourteen days and one minute are reasoned from failure cost,
   not observed from a production load.
