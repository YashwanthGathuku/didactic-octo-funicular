# Sentinel Flow — Scope

**Established by:** Prompt 01 (Truth reset and scope reduction)
**Date:** 14 August 2026

This document is the authority on what Sentinel Flow claims to do. If code, README, UI copy, or a
demo asserts a capability that is not marked **Implemented** below, the assertion is a defect.

---

## 1. Version-one promise

> Sentinel Flow knows which financial files are expected, records finalized arrivals, validates
> NACHA files deterministically, quarantines unsafe inputs, and gives operators a traceable
> evidence path from source object to human release decision.

Everything else is out of scope until that sentence is true end to end, measurable, and covered by
tests.

---

## 2. Capability status

Status meanings are strict:

- **Implemented** — the behaviour exists, is exercised by a test, and no part of its result is a
  source constant.
- **Partial** — real behaviour exists but a named gap makes it unsafe to rely on.
- **Experimental** — code exists and may be useful, but it is not production-claimed, not covered
  by conformance fixtures, and must never gate a release decision.
- **Planned** — not built. Named here only so nobody claims it early.

### Ingestion and validation

| Capability | Status | Notes |
|---|---|---|
| Authenticated multipart upload | Implemented | Prompt 06. `/files/upload` streams to storage under a size and tenant quota bound. The legacy `/files/ingest-raw` still buffers; see NACHA_VALIDATION.md. |
| SHA-256 content hashing | Implemented | Real digest over the received bytes. |
| ABA routing Mod10 check digit | Implemented | `ValidateRoutingMod10`, covered by `TestMod10RoutingValidation`. |
| NACHA record-length and structure checks | Partial | Record width, entry hash, and batch control arithmetic are checked. This is not the full Nacha rule set; see Prompt 07. |
| **Fail-closed on empty / unparseable input** | Implemented | Prompt 01 fixed this. A zero-byte or unparseable file now quarantines. Covered by `quarantine_test.go`. |
| Quarantine on validation findings | Implemented | Any finding at or above `ERROR` prevents release. |
| Versioned release policy | Implemented | Prompt 07. `internal/nacha` `PolicyVersion`, recorded on every decision. |
| ISO 20022 XML parsing | **Experimental** | Small XML projection only. No XSD, no market-practice validation, money handled as `float64`. Must not be production-claimed. |
| BAI2 parsing | **Experimental** | Record and transaction-code logic is superficial. |
| SWIFT MT103 / MT940 parsing | **Experimental** | Regex and tag checks, not message validation. |
| Duplicate / idempotent delivery | Implemented | Prompt 06 dedupes by content hash at ingest; Prompt 08 made job delivery and outbox dispatch idempotent. |
| Immutable object storage | Implemented | Prompt 06. Filesystem and S3 adapters share one conformance suite; keys are server-assigned. |

### Scheduling and expectations

| Capability | Status | Notes |
|---|---|---|
| Materialized expectation occurrences | Implemented | Prompt 10. Written ahead of time, so a file that never arrives has a row that ages into OVERDUE and BREACHED. |
| Versioned feed contracts | Implemented | Prompt 10. Editing terms creates a version; a historical occurrence resolves to the version active on its business date. |
| Federal Reserve business calendar | Implemented | Prompt 10. Encoded as the published rules, including the Reserve Banks' Saturday rule, with mandatory-reason tenant overrides. No network fetch of the published calendar occurs. |
| Explicit timezone and DST handling | Implemented | Prompt 10. The spring-forward gap and fall-back ambiguity are resolved deterministically and the resolution is persisted. |
| Arrival matching with recorded ambiguity | Implemented | Prompt 10. An arrival that could satisfy more than one occurrence attributes nothing and records every candidate for review. |
| Breach incident and escalation record | Implemented | A breach opens an incident, records a notification intent addressed to the contract owner, and publishes an outbox event, all in the transition's transaction. |
| Breach notification delivery | **Planned** | The obligation is durable and attributed; no channel drains the queue, so an operator reads the incident list rather than being paged. |
| Review resolution for ambiguous arrivals | Partial | `ResolveCandidate` accepts or rejects with a required actor and reason; no HTTP route calls it yet. |
| Waiving an expectation | Partial | `Waive` requires an actor and reason and resolves the incident; no HTTP route, and no second-signature approval. |
| Contract and calendar version *history* | Implemented | Prompt 12. `GET /contracts/{id}/versions`, rendered with the version in effect marked by the server's clock. |
| Contract and calendar management API | **Planned** | No authenticated route creates or amends a contract, version, calendar or override. Reading them is done; writing them is not. |
| Outbound feed delivery | **Planned** | `direction` is stored and validated; nothing sends a file. An OUTBOUND contract materializes occurrences that only an arrival could satisfy. |
| Predictive breach risk | **Non-goal** | Removed in Prompt 01 and not reintroduced. |

### Customer source connectors

Sentinel Flow reading a *customer's* database. PostgreSQL remains Sentinel
Flow's own system of record and is unaffected by anything in this section.

| Capability | Status | Notes |
|---|---|---|
| Connector contracts and capability model | Implemented | Prompt 16.1. `internal/connectors`. |
| Metadata-driven catalog of nine connectors | Implemented | All nine carry a reviewed field model. Eight are PLANNED with the reason stated. |
| Server-owned descriptors driving one generic wizard | Implemented | Prompt 16.2. The browser knows nothing about any specific database. |
| Write-only credential handling | Implemented | Secrets are a separate type holding sealed values; no read path returns one, and marshalling refuses. |
| Connection-string paste, split and discard | Implemented | Offered only where the provider defines a URI. Never stored, echoed or logged. |
| Shared conformance suite | Implemented | Prompt 16.3. 21 black-box checks against a real disposable fixture; a skip counts as a failure. |
| PostgreSQL driver | Implemented | Prompt 16.4. Passes all 21 checks against real PostgreSQL 16 in CI. |
| MySQL, MariaDB, SQL Server, Oracle, Snowflake, Redshift, BigQuery, Databricks | **Planned** | No driver. Visible in the catalog, not selectable, and no connection can be attempted. |
| Storing a connection | Implemented | Migration 010. Credentials go to the secret store; only the reference is in the connection tables, asserted by an exhaustive dump. |
| Runtime conformance evidence | Implemented | The run publishes an artefact the deployment carries and the binary validates: a record that is incomplete, stale, or built against a different driver version leaves the connector unselectable. |
| Approved query templates | **Planned** | The approval model exists; no templates ship and no route defines one. |
| Per-connector rate and concurrency limits | Implemented | Per-execution row, byte, time and cursor bounds, plus a per-minute fixed window whose refusals are audited. Per process, so replicas each permit the rate. |
| Connection lifecycle audit events | Implemented | Create, test, rotate and delete are written to the evidence chain, carrying identifiers and classifications only — no credential and no hostname. |
| Creating a connection from the wizard | **Planned** | The wizard renders the server's fields; submitting still goes through the API rather than the screen. |
| Arbitrary SQL from browser or AI | **Non-goal** | There is no entry point that accepts a statement. Not restricted — absent. |

### Human review and release

| Capability | Status | Notes |
|---|---|---|
| Review queue with redacted references | Implemented | Prompt 11. Tenant-scoped; identifiers and digests only, no evidence. |
| Dual control, configurable per tenant | Implemented | Default is two distinct approvers with separation of duties; an unconfigured tenant gets the strict policy. |
| Approval expiry on any change | Implemented | One integrity digest over artifact, findings, policy, rule pack, contract and run. The error names what changed. |
| Separation of authorities | Implemented | reviewer approves; release_supervisor overrides; tenant_admin configures. No role holds both configure and override. |
| Auditable release with an outbox event | Implemented | Every transition reaches the evidence chain and the outbox in the transaction that produced it. |
| Manual override, separately reportable | Implemented | Its own table, a mandatory 20-character justification, and it never rewrites validation results. |
| Releasing a quarantined artifact | **Non-goal** | No path exists, by approval or override. Quarantine would otherwise be advisory. |
| Review UI | Implemented | Prompt 12. `src/components/ops/ReviewQueue.tsx`: approve, reject, release and override, each behind a confirmation that restates the consequence; a 409 renders as a conflict and the queue re-reads. |
| Notification that a decision needs review | **Planned** | The outbox event is published and nothing subscribes. |
| Time-based approval expiry | **Planned** | An approval expires when its subject changes, not because it is old. |

### Evidence and audit

| Capability | Status | Notes |
|---|---|---|
| Application hash chain (append) | Implemented | Prompt 09 made the read-compute-write one transaction, serialised per tenant. Still an application hash chain, not a Merkle tree. |
| Tamper detection by recomputation | Implemented | `GetLedger` recomputes each row's hash; detects payload, actor, event-type and timestamp edits. Covered by `TestLedgerDetectsContentTampering` and `TestLedgerDetectsActorTampering`. |
| Evidence export | Implemented | Exports the chain and its verification result. Carries no regulatory claim. |
| External anchoring / signed checkpoints | Planned | Required before any tamper-evidence claim beyond "application hash chain". |

### Statistics

| Capability | Status | Notes |
|---|---|---|
| Two-sample Kolmogorov–Smirnov test | Implemented | `kstest.go`. Exact D, Stephens (1970) correction, theta-transformed series cross-validated to 1e-17. Twelve tests. **Not wired to any production decision.** |
| Benjamini–Hochberg FDR control | Implemented | `kstest.go`, covered by `TestBenjaminiHochbergControlsFDR`. Not wired to production. |
| Robust median/MAD anomaly detection | Implemented | `robust_anomaly.go`. 50% breakdown, handles MAD=0, refuses thin history. Not wired to production. |
| Predictive breach probability | **Non-goal for v1** | See §3. |

### Security

| Capability | Status | Notes |
|---|---|---|
| PGP detached-signature verification | Implemented | Fails closed on missing keyring, unknown signer, or mismatch. Test signs real bytes then flips one. |
| SSH public-key parsing | Implemented | RFC 4253 wire parsing, algorithm-match enforcement, 2048-bit floor. |
| API authentication | Implemented | Prompt 04. OIDC token verification, fail-closed in every direction; the "unset token means no authentication" branch is gone. The local-demo profile is the single, announced exception. |
| Browser login (Authorization Code + PKCE) | **Partial** | Implemented in `internal/auth/pkce.go` and **wired to no route**, so outside the demo profile a browser has no way to obtain a session. |
| Authorization / roles | Implemented | Prompt 04, extended by Prompt 11. Six roles; no role holds both configure and override. `GET /session` reports what a caller actually holds. |
| Tenant isolation | Implemented | Prompt 04. Every business table carries `tenant_id`; the scope is derived from verified claims and proven end to end over HTTP. PostgreSQL row-level security is verified against a real server and not deployed on one. |
| Secret management | Implemented | Prompt 05. AES-GCM `secrets.Value`; no method returns a stored value and `MarshalJSON` refuses rather than emitting `{}`. |
| CSRF on cookie-authenticated mutations | Implemented | Double-submit against the readable CSRF cookie. Prompt 12 corrected this: it had compared the header against the HttpOnly session cookie, which no browser can read. |

### Operations

| Capability | Status | Notes |
|---|---|---|
| Prometheus metrics | Partial | Counters are real. The parse-rate gauge reports `-1` until genuinely measured — keep that pattern. |
| Server-Sent Events endpoint | Implemented | Prompt 12 replaced the in-memory broadcaster — which was untenanted, had no cursor, and had no publisher — with a tenant-scoped reader over `outbox_events`. `Last-Event-ID` replays without duplicates; a cursor older than the retained window produces a `gap` event rather than a silent restart. |
| Inbox watcher | **Partial — unsafe** | Polls once per second and reads files that may still be uploading. Prompt 17. |
| Health / readiness | Implemented | `/health` is liveness only; `/ready` probes the database and reports each dependency's real state. `GET /service-health` adds queue and outbox depth from database state, and a dependency nobody probed reports UNKNOWN, never OK. |
| Server-backed operations UI | Implemented | Prompt 12. Six screens, one typed transport, explicit unavailable/forbidden/stale/partial states, and no path that renders a value the server did not send. |
| Server-side pagination on every list | Implemented | Keyset, not OFFSET. The limit ceiling is enforced at the server: `limit=100000` returns 200 rows. |
| Measured performance figures | **None** | No performance number may appear in README, UI, or API until a reproducible result artifact exists (Prompt 13). |

---

## 3. Explicit non-goals

Sentinel Flow does **not** do these things, and no surface may imply otherwise:

1. **Payment initiation, settlement, or rail connectivity.** Settlement is not a state in this
   product (§4).
2. **Autonomous repair or autonomous release.** Every release is a human decision.
3. **Arbitrary SQL consoles, SSH shells, or remote filesystem browsers.**
4. **Production support for BAI2, ISO 20022, or SWIFT.** Experimental only.
5. **Predictive breach probability.** Deterministic deadline state only. A calibrated probability
   requires real per-feed history and prospective evaluation that does not exist yet.
6. **Assurance labels** — "FIPS certified", "SOC 2 compliant", "bank-grade", "zero trust",
   "Merkle", "WORM", "SEC 17a-4 compliant". None of these are earned by code alone.
7. **Multi-agent AI orchestration.** One read-only analyst, after the deterministic pipeline is
   correct (Prompt 15).
8. **Live connector platform.** Prompt 16, and only per-connector after conformance tests pass.

---

## 4. Vocabulary rules

These words have exactly one meaning in this codebase, its API, its UI, and its documentation.
Using them loosely is a defect.

| Term | Means | Does **not** mean |
|---|---|---|
| **Delivery** | A file arrived and its upload is finalized. Nothing about its contents is known. | That the file is usable, valid, or accepted. |
| **Validation** | Deterministic rule evaluation against the file's bytes, producing typed findings. | A human judgement, an approval, or an AI opinion. |
| **Quarantine** | A terminal-until-reviewed state meaning the artifact must not be consumed downstream. Applied on any finding at or above `ERROR`, on parser failure, on zero records, and on unverifiable input. | That the file is deleted, or that it is merely flagged. |
| **Approval** | A named, authenticated human recorded a decision against a specific artifact hash, validation run, and policy version. | That the file moved anywhere. |
| **Release** | The artifact is marked usable by downstream ledger processes. Requires successful validation **and** a policy decision. | Payment, settlement, transmission, or any external effect. |
| **Settlement** | **Not a state in this product.** Money movement is performed by systems Sentinel Flow does not touch. | Anything Sentinel Flow can observe, assert, or cause. |

Two supporting rules:

- **"Application hash chain", never "Merkle".** The ledger is a linear predecessor chain. It has no
  history tree, no membership proofs, no consistency proofs, and no external anchor. A SHA-256
  digest is not a digital signature.
- **Demo and synthetic data must be labelled at the point of display.** If a screen or response
  contains generated data, it says so where the operator reads it. A dependency failure renders as
  `Unavailable` or `Degraded` — never as healthy state, and never as a silent empty list.

---

## 5. Patterns carried forward from removed code

Prompt 01 deleted roughly 5,900 lines. These patterns from that code are worth reusing and are
archived verbatim in `REMOVED_CODE_ARCHIVE.md` and `REMOVED_CODE_ARCHIVE_UI.md`:

| Pattern | Was in | Reuse in |
|---|---|---|
| Constant-time credential comparison, refuses to operate unconfigured, logs denied attempts before returning | `vault.go` `authorizeDetokenize` | Prompt 04, Prompt 05 |
| HMAC-SHA256 payload signing | `webhook.go` | Prompt 05 |
| Secret **reference** indirection — store a pointer, never the value | `connector.go` `SecretReference` | Prompt 05 |
| Honest demo labelling: `IsScriptedDemo: true`, `*Target` suffix on unmeasured figures, `NOT_PROVISIONED` for absent dependencies | `failover.go` | Everywhere |
| Measured-or-sentinel metrics: report `-1` until a real measurement exists | `metrics.go` (retained) | Prompt 13 |
| Capability, health and data-classification modelling | `connector.go` | Prompt 16 |
