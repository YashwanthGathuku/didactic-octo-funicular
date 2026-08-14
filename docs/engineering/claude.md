# Sentinel Flow engineering contract

Sentinel Flow is a pre-ledger financial-file reliability gateway. Correctness, evidence,
tenant isolation, and failure behavior matter more than feature count or visual polish.

## Source of truth

Before changing code, read:
- docs/engineering/SentinelFlow_Code_Audit_and_Recovery_Plan.md
- docs/engineering/SENTINELFLOW_PRODUCTION_READY_PROMPT_GUIDE.md
- README.md
- the code and tests in the affected module

If documentation contradicts running code, report the contradiction. Do not silently
choose the more impressive interpretation.

## Non-negotiable invariants

1. Every financial input begins untrusted and unreleased.
2. Empty, partial, unparseable, unsupported, duplicate-conflicting, or unverifiable input
   fails closed into a typed quarantine state.
3. The original artifact is immutable. Repair creates a new derived artifact.
4. Deterministic parsing and release decisions do not depend on AI.
5. AI is read-only, evidence-grounded, and unable to release, repair, pay, notify an
   external party, execute SQL, use a shell, or change schedules.
6. Authentication is mandatory outside an explicitly named local demo profile.
7. Actor identity comes from authenticated claims, never request fields.
8. Every business record is tenant-scoped and repository queries enforce that scope.
9. Secrets are write-only references. They are never returned, logged, placed in metrics,
   stored in source, exposed to SQL/reporting, or sent to an AI model.
10. Security state is derived from verified runtime state. Never return mTLS=true,
    verified=true, compliant=true, healthy=true, or settled=true from constants.
11. Operational metrics are measured. Synthetic/demo values are isolated and labelled.
12. A missing dependency produces UNAVAILABLE or DEGRADED, never fabricated success.
13. State transitions are explicit, validated, persisted, and auditable.
14. Duplicate delivery and restart are normal conditions and must be idempotent.
15. Bounded concurrency and backpressure are mandatory; do not create unbounded
    goroutines, queues, request bodies, result sets, retries, or model calls.

## Architecture boundaries

- Go modular monolith: API, domain, repositories, ingestion, validation, jobs, outbox,
  authorization, and evidence ledger.
- PostgreSQL: durable metadata, leases, state, outbox, and audit indexes.
- S3-compatible storage: immutable source and derived artifacts.
- Python AI tier: optional asynchronous consumer; never part of deterministic ingestion.
- React UI: server-backed state only. Demo data must use an explicit demo build/profile and
  visible banner. Never silently fall back to mocks.

## Engineering method

- Inspect before editing. State the scope and out-of-scope items.
- Add a failing behavior test before fixing a defect.
- Prefer small interfaces and dependency injection at network, storage, clock, ID, and
  secret boundaries.
- Use integer minor units or a decimal library for money; never float32/float64.
- Store timestamps in UTC plus the source timezone/rule where business scheduling needs it.
- Do not invent payment-format rules. Cite licensed/current rule sources in rule metadata.
- Do not weaken a production control to make a test or demo pass.
- Never log raw financial file contents, credentials, tokens, authorization headers, or
  unredacted account/routing values.

## Required verification

Run the repository's pinned commands for:
- formatting and linting
- Go unit/integration/race tests
- TypeScript typecheck, unit tests, and production build
- Python lint/type/unit/evaluation tests when AI code changes
- database migration up/down/upgrade tests
- container build and clean-stack integration smoke test
- secret and dependency scanning

If a toolchain or dependency is unavailable, mark the check NOT RUN and explain why.
Never replace a missing check with a claimed pass.

## Completion report

Report the outcome, changed files, tests, command results, security/privacy impact,
concurrency/performance impact, remaining risks, and next task. Include no unsupported
percentage, latency, throughput, compliance, or correctness claim.
