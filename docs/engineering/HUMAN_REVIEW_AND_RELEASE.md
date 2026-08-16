# Human review and dual-control release

**Established by:** Prompt 11
**Authority:** `gateway/internal/review/` is the implementation; this document
describes it. Where they disagree, the code is correct and this file is a defect.

## What this is

A validated artifact is not a released one. Validation says the file parses and
satisfies the rules that could be checked; release is a decision by a person to
send money. This package is the boundary, built so it cannot be crossed by
accident, by one person acting alone, or by a decision that no longer describes
what would be released.

**There is no self-healing.** Nothing here repairs a file, re-runs validation to
get a better answer, or resolves a finding. A human accepts what the validator
found or explicitly overrides it, on the record.

## What Prompt 03 left

`policy_decisions`, `approvals` and `validation_runs` were created by migration
002 and nothing had ever written to any of them. The separation-of-duties
constraint — `UNIQUE (tenant_id, decision_id, actor_id)` — has been correct and
unreachable since it was written, because no code path created a decision to
approve.

The one route that existed, `POST /incidents/{id}/approve`, resolves an
incident. It has never released anything. It also, before Prompt 04, read
`actor` from the request body and defaulted it to the literal
`TREASURY_SUPERVISOR_01`.

## The integrity digest

An approval expires when the artifact, the findings, the policy, the rule pack,
the governing contract, or the validation run change. That is enforced by **one
digest over all of them**, recomputed at vote and release time and compared.

One digest rather than six field comparisons is deliberate: six checks means one
somebody forgets, and the one forgotten is the one that matters. The digest is
versioned (`release-integrity/1`) for the same reason the ledger's canonical form
is — changing how it is computed would make every existing approval look stale,
which is indistinguishable from an attack.

Components are joined with ASCII `RS` (0x1e) and assembled by an explicit
`strings.Join`, not a format string. The first draft used `fmt.Fprintf` with
nineteen verbs; adding the rule-pack field shifted every argument after it and
`go vet` caught the arity mismatch. A format string of that length is a place
where a future field silently produces a digest that is stable across the change
it was supposed to detect.

**The findings digest covers rule id and severity only** — never the evidence.
Two things follow: the digest is stable when evidence redaction changes, and a
decision record carries nothing derived from file content. Findings are sorted
before digesting, so a change in query plan does not expire every approval.

The **rule pack version** is in the digest alongside the policy version. They
change independently and for different reasons: the policy says which severities
block, the rule pack says what the severities are. An approval given under one
rule pack does not describe what the next one would find, even with the policy
unchanged.

`CheckFresh` names what changed. "The approval expired" is not actionable; "the
validation findings changed since it was approved" is.

## Dual control

| | Default |
|---|---|
| `min_approvals` | 2 |
| `separation_of_duties` | on |
| `override_allowed` | on |

**A tenant with no configuration gets the strict policy.** The absence of
configuration must not be the weakest setting: a deployment that forgot this
would otherwise release on one approval and nobody would find out until an
audit.

The threshold is **copied onto the decision** when it is proposed, not read at
release time. Changing the policy must not retroactively satisfy a decision
proposed under a stricter one — that would let an operator lower the threshold
and release something already half-approved.

A vote counts towards the threshold only when it approves, was cast against the
decision's **current** integrity digest, and — under separation of duties — was
not cast by the proposer. `ApprovalsHeld` applies all three in one place, and
counts distinct actors, so a caller assembling a `Decision` by hand cannot count
one person twice. The `UNIQUE` constraint is what holds under a race.

**One rejection is enough.** A release requires agreement, so a reviewer who
examined the file and refused it is not outvoted by two who accepted it — the
disagreement itself is the signal.

The proposer is `system:validation-worker`, so nothing proposed automatically is
excluded from human approval. Anything a *person* proposes is.

## Three authorities, three roles

The Prompt 04 matrix already refused to give `tenant_admin` the ability to
approve, on the grounds that administering a tenant and approving the movement
of money are different authorities. This extends that:

| Role | Approve | Override | Configure dual control |
|---|---|---|---|
| viewer | ✗ | ✗ | ✗ |
| operator | ✗ | ✗ | ✗ |
| reviewer | ✓ | ✗ | ✗ |
| tenant_admin | ✗ | ✗ | ✓ |
| **release_supervisor** | ✓ | ✓ | ✗ |

`release_supervisor` is a new role rather than a permission added to an existing
one. Giving override to `reviewer` would make it indistinguishable from
approval; giving it to `tenant_admin` would let one account both configure the
control and step around it. `TestApprovingOverridingAndConfiguringAreDifferentAuthorities`
asserts that **no role holds both** configure and override — otherwise "lower
the threshold, then meet it" would be an alternative to writing a justification.

## Release

The artifact's own state machine still governs; this package does not get its
own opinion about which states may release. `VALIDATED → APPROVED → RELEASED`
walks both edges and records both, so the trail shows the path rather than a
teleport.

**A quarantined artifact cannot be released at all** — not by approval, not by
override. `QUARANTINED` has no edge to `APPROVED` in the Prompt 03 table, and
adding one was considered and rejected: a quarantined artifact that could walk to
`RELEASED` would make quarantine advisory. The override is refused with that
stated.

Every guard runs inside one transaction with an optimistic update on
`row_version`, so concurrent releases produce exactly one release and exactly one
`RELEASED` history row. The original stored object is never touched; this package
has no path that could write to it.

## Manual override

A **separate table**, not a flag on the decision. "Separately reportable" is the
requirement, and a flag buried in a decision row is found only by someone who
already suspects it. A table can be listed, counted, and put in front of an
auditor.

It records who, under which role, what was bypassed in the system's own terms
(`dual control (0 of 2 approvals); blocking findings (NACHA.BATCH.CONTROL)`), the
blocking rules at that moment, and a justification of at least 20 characters. The
length floor is crude and effective: "ok" and "urgent" are not justifications,
and a floor makes someone write a sentence.

**An override never rewrites the validation result.** The findings stay exactly
as the validator wrote them. A mechanism that "resolved" them would destroy the
evidence that the override was needed, and the next reader would see a clean
file.

**An override cannot bypass a rejection.** It may bypass an absent approval —
that is a control being unmet. A reviewer who looked at the file and said no is
not an unmet control; it is a person saying no.

A tenant can forbid overrides entirely, and one that does cannot be talked into a
release by an operator under pressure — the situation overrides exist for, and
the situation they are most abused in.

## Every transition is evidence

Proposals, votes, releases, expiries, overrides and policy changes all reach the
append-only evidence chain **and** the outbox, in the transaction that produced
them. A release that reached one and not the other would be a release the rest of
the system did not know about, or one with no evidence.

Changing dual control is itself a control change, so it is recorded too.

## Verification

```
gofmt PASS · vet PASS · go test PASS · go test -race PASS
internal/review: 17 test functions × SQLite + real PostgreSQL 16
migrations 001–011 apply and are idempotent through the real command path
Container builds NOT RUN: no Docker daemon in this environment
```

## What is not done

1. **No UI.** There is no review queue screen, no approve/reject control, and no
   override form. Every route exists and nothing in the browser calls them.
   Prompt 12 is the operations UI.
2. **No notification when a decision needs review.** A decision is proposed and
   sits in the queue until somebody looks. The outbox event is published and
   nothing subscribes to it — the same last-mile gap as the breach notifications
   from Prompt 10.
3. **`RoleReleaseSupervisor` is not issued by anything.** The role exists in the
   matrix and no identity provider mapping grants it, so in practice no one can
   override yet. That is the safe direction and it is still incomplete.
4. **No approval expiry by time.** An approval expires when its subject changes.
   It does not expire because it is three weeks old, and a decision approved
   before a long weekend is still approved afterwards.
5. **The proposer is always the worker.** `ProposeTx` takes a proposer and the
   only caller passes `system:validation-worker`, so the separation-of-duties
   path that excludes a *human* proposer is exercised by tests and not by any
   production flow.
6. **No derived-artifact repair path.** The guide's "proposed derived artifact"
   is covered in the digest by the source artifact's hash; there is no repair
   flow that produces a derived artifact to approve, so that half is untested
   against reality.
7. **The application still runs on SQLite.** Seventh subsystem verified against
   a real PostgreSQL server and not deployed on one.
