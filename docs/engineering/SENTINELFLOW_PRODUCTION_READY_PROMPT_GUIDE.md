# Sentinel Flow — Production-Readiness Prompt Guide

**Purpose:** turn the audited prototype into a secure, measurable, recruiter-ready fintech engineering project.  
**Primary goal:** demonstrate the judgment and execution expected from a strong backend, platform, reliability, security, or applied-AI engineer.  
**Product goal:** reduce manual financial-file operations by detecting missing or unsafe files before downstream ledger and settlement jobs consume them.

> Important: software alone cannot make a product legally or organizationally “bank-grade.” Production approval also requires customer-specific risk assessment, legal review, licensed/current payment-format rules, incident procedures, vendor management, access reviews, penetration testing, operational ownership, and evidence from the deployed environment. This guide builds a **production-shaped system and verifiable release evidence** without making unsupported compliance claims.

---

## 1. Product decision before coding

Build a **web application first**, not a desktop application.

Recommended architecture:

| Layer | Choice | Reason |
|---|---|---|
| Operations UI | React + TypeScript | Strong portfolio surface; enterprise users can access it centrally. |
| Core API and workers | Go modular monolith | Efficient streaming, concurrency, predictable memory, simple deployment, strong backend signal. |
| Internal system of record | PostgreSQL | Transactions, locking, tenant controls, migrations, and job leases. This is Sentinel Flow's database, not a restriction on customer source connectors. |
| Immutable artifacts | S3-compatible object storage; MinIO locally | Raw financial files should not live in API responses or relational rows. |
| Async jobs | PostgreSQL job table + leases and transactional outbox | Reliable enough for the first release without prematurely adding Kafka/Redis. |
| Live updates | Server-Sent Events with replay cursor | Simpler than WebSockets for one-way operations updates. |
| AI analyst | Optional Python worker/service | Keeps model dependencies away from the deterministic processing path. |
| Authentication | OIDC/OAuth 2.0 with provider-neutral adapter | Enterprise-friendly; supports Entra/Okta later. Use a local OIDC provider for development. |
| Ingress | Authenticated upload first; finalized SFTP/object event second | Proves the pipeline before adding private-network complexity. |

Do not build microservices yet. Keep the Go code as a modular monolith with separate API and worker commands if necessary. Extract services only when independently measured scaling or ownership requirements justify it.

### Two separate database decisions

Do not mix these concerns:

1. **Sentinel Flow's internal database:** use PostgreSQL only. It stores tenants, contracts, expectations, jobs, findings, decisions, and audit metadata. Supporting several interchangeable internal databases would require multiple migration dialects, locking/lease implementations, transaction semantics, index strategies, and concurrency test matrices. That work does not improve the customer problem.
2. **Customer source databases:** support them through a connector SDK. The initial catalog can cover PostgreSQL, MySQL, MariaDB, SQL Server, Oracle, Snowflake, Amazon Redshift, Google BigQuery, and Databricks SQL. A connector becomes selectable as `AVAILABLE` only after its real driver and conformance suite pass.

The connection setup UI should be metadata-driven. Selecting a connector type returns a server-owned descriptor containing fields, authentication modes, default port, TLS requirements, safe connection templates, validation rules, and supported capabilities. Never display a saved password, private key, OAuth refresh token, service-account JSON, wallet, or complete connection URI.

Initial connector catalog:

| Connector | Connection fields shown to user | Authentication choices | Safe template shown by UI |
|---|---|---|---|
| PostgreSQL | host, port, database, username, SSL mode, CA/client-cert references | password, client certificate, cloud/IAM adapter later | `postgresql://<user>:<secret>@<host>:5432/<database>?sslmode=verify-full` |
| MySQL | host, port, database, username, TLS mode, CA/client-cert references | password, client certificate, cloud/IAM adapter later | `<user>:<secret>@tcp(<host>:3306)/<database>?tls=<profile>` |
| MariaDB | host, port, database, username, TLS mode, CA/client-cert references | password, client certificate | same protocol family as MySQL, but a separately tested capability profile |
| SQL Server | server, port/instance, database, username, encryption settings, CA reference | password, Entra/OAuth adapter later | `sqlserver://<user>:<secret>@<host>:1433?database=<database>&encrypt=true` |
| Oracle | host, port, service name or approved TNS alias, wallet reference, username | password, wallet/mTLS, external identity later | `oracle://<user>:<secret>@<host>:1521/<service>` |
| Snowflake | organization/account identifier, warehouse, database, schema, role | key pair, OAuth, password for local testing only | structured account parameters; do not force a JDBC-style URI |
| Amazon Redshift | host/workgroup, port, database, username, region | IAM/temporary credentials preferred, password | PostgreSQL-compatible template with a distinct capability/auth profile |
| Google BigQuery | project, billing project, dataset allowlist, location | workload identity/service-account secret reference | structured project/dataset settings; there is no normal database password URI |
| Databricks SQL | workspace host, HTTP path/warehouse, catalog, schema | OAuth/service principal preferred, PAT only as a secret reference | structured host and HTTP-path settings |

Allow an optional “paste connection URI” convenience only for connector types with a well-defined URI. Parse it immediately, separate credentials into the secret store, discard the raw URI, and show only a masked canonical summary. Structured fields are the primary interface.

### Version-one promise

> Sentinel Flow knows which financial files are expected, records finalized arrivals, validates NACHA files deterministically, quarantines unsafe inputs, and gives operators a traceable evidence path from source object to human release decision. An optional read-only AI analyst summarizes incidents using cited system evidence but cannot alter financial state.

### Explicitly out of version one

- Payment initiation, payment settlement, or claims of rail connectivity.
- Autonomous repair or autonomous release.
- Arbitrary SQL consoles, SSH shells, or unrestricted remote filesystem browsers.
- Production claims for BAI2, ISO 20022, and SWIFT validation.
- Predictive breach probabilities before adequate real history and prospective evaluation.
- “FIPS certified,” “SOC 2 compliant,” “bank-grade,” “zero trust,” “Merkle,” or other assurance labels without the required external evidence.
- Hardcoded benchmarks, SLA percentages, health values, security status, or compliance status.

---

## 2. How to use these prompts

1. Commit or tag the current repository so every deletion is recoverable.
2. Because this project uses Claude Code, put the standing instruction in Section 3 into the repository root as `CLAUDE.md`.
3. Give Claude Code this guide and the audit report, then run **one task prompt at a time**, in order. Do not say only “follow the entire guide” and allow one giant implementation session.
4. Create one branch or clearly scoped commit per prompt.
5. Do not accept “implemented” without the requested command output and acceptance evidence.
6. If a gate fails, fix it before moving to the next prompt.
7. Never combine a security boundary change and a major feature change in the same task.
8. Preserve the audit report and this guide under `docs/engineering/` in the repository.

Use this exact first message in Claude Code:

```text
Read CLAUDE.md, docs/engineering/SentinelFlow_Code_Audit_and_Recovery_Plan.md, and
docs/engineering/SENTINELFLOW_PRODUCTION_READY_PROMPT_GUIDE.md completely.

We will execute the guide one gated prompt at a time. Do not implement later prompts,
connectors, AI features, or adjacent improvements early. Start with Prompt 00 only.
Perform the required read-only baseline audit, run the available verification commands,
create CURRENT_STATE.md, and stop for my review. Do not change production code in this task.
```

Each coding session must end with:

```text
1. Outcome and architectural decision
2. Files changed
3. Tests added first and the failure they demonstrated
4. Verification commands and exact results
5. Security/privacy impact
6. Performance/concurrency impact
7. What remains unimplemented or unverified
8. The smallest safe next task
```

---

## 3. Standing repository instruction

Copy the following into the repository instruction file.

```markdown
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
```

---

## 4. Sequenced implementation prompts

## Prompt 00 — Establish a trustworthy baseline

```text
Work read-only first. Audit this repository against
docs/engineering/SentinelFlow_Code_Audit_and_Recovery_Plan.md.

Create docs/engineering/CURRENT_STATE.md containing:
- complete module and route inventory
- every hardcoded operational/security/compliance/performance result
- every production route or UI screen backed by synthetic data
- every place raw financial data, credentials, tokens, or secrets can be stored, returned,
  logged, queried, or sent externally
- build/runtime dependency graph
- exact Go/Node/Python/container versions currently required
- current tests mapped to production behavior
- a route-by-route authentication and tenant-isolation matrix

Run all currently documented verification commands without changing code. Mark each PASS,
FAIL, or NOT RUN with exact output. Compare the supplied binary behavior with source only
if a clean source build is available; otherwise do not treat the binary as source proof.

Do not fix anything in this task. Finish with a proposed deletion/change list ordered P0,
P1, P2 and identify any existing user work that must be preserved.

Acceptance:
- No code changes.
- Every claim has a file:line or runtime-request reference.
- Unknowns are labelled unknown.
```

## Prompt 01 — Truth reset and scope reduction

```text
Use CURRENT_STATE.md and the audit report. Remove from the shipped application, API routes,
tests, README claims, navigation, and production build:
- mock Integration Hub and fake edge sync
- instant-payment validation/settlement simulation
- arbitrary SQL console
- vault UI and plaintext webhook-secret retrieval
- scripted agent swarm
- self-healing apply endpoint
- chaos monkey and failover simulator
- executive deck and hardcoded benchmark/SLA panels
- fabricated AI evaluation fallback

Preserve only generally useful primitives whose behavior is real and tested. Git history is
the recovery mechanism; do not retain dead production routes behind undocumented flags.

Create docs/engineering/SCOPE.md with:
- one-sentence version-one promise
- implemented / experimental / planned capability table
- explicit non-goals
- vocabulary rules for quarantine, validation, approval, release, delivery, and settlement

Replace silent frontend mock fallbacks with an explicit error/degraded state. If a separate
demo profile remains, it must be compile/deploy-time explicit and display DEMO DATA on every
affected screen.

Tests:
- removed routes return 404
- backend failure cannot render mock healthy production state
- no production response contains fixed success/security/performance values

Acceptance:
- frontend and backend build from source
- repository search finds no removed navigation/routes/claims
- report exact line count removed and anything intentionally retained
```

## Prompt 02 — Reproducible build and secure configuration

```text
Make a clean checkout reproducible.

Tasks:
- choose and pin one supported Go version across go.mod, CI, containers, and docs
- pin Node and Python versions and lock dependencies
- create the missing Python dependency manifest
- remove committed binaries and generated databases
- create one authoritative local Compose stack
- use PostgreSQL and S3-compatible object storage consistently
- add versioned migrations and a migration command
- implement typed configuration loading with startup validation
- honor all configured service URLs; remove hardcoded localhost addresses
- store local development state in mounted volumes
- add health endpoints for process liveness and readiness endpoints for dependencies
- create .env.example containing names and safe descriptions only, no credentials

Security behavior:
- production profile refuses to start without authentication, encryption/secret-provider,
  database, and object-storage configuration
- local demo profile binds to loopback by default and is visibly labelled
- no default production passwords or tokens

Tests:
- migrate empty database
- upgrade from previous schema fixture
- start, create state, restart, confirm state remains
- invalid/missing production configuration fails startup
- containers communicate using configured service names

Acceptance:
- one documented command builds and starts a clean stack
- CI can execute the same command
- README contains no unsupported badge or test count
```

## Prompt 03 — Domain model and state machines

```text
Design and implement the minimum domain model before adding more handlers.

Entities:
- tenant/workspace
- partner
- feed contract and immutable contract version
- expectation occurrence
- source artifact
- ingestion job and attempt
- validation finding
- policy decision
- review/approval record
- derived artifact
- notification intent
- audit event

Define explicit state machines and legal transitions for expectation, artifact, job, and
decision states. Examples:
expectation: PENDING -> DUE -> OVERDUE -> BREACHED, or -> ARRIVED
artifact: RECEIVED -> VALIDATING -> QUARANTINED | VALIDATED -> APPROVED -> RELEASED
job: QUEUED -> LEASED -> RUNNING -> SUCCEEDED | RETRYABLE | DEAD

Requirements:
- state transitions occur in domain methods, not arbitrary SQL/handlers
- database constraints prevent impossible combinations
- status history is append-only
- release requires a successful validation result and versioned policy decision
- settlement is not a state in this product
- tenant_id is non-null on every business table

Create an ADR explaining the model and transition invariants.

Tests:
- table-driven legal and illegal transitions
- release from RECEIVED/QUARANTINED fails
- caller cannot skip approval where policy requires it
- concurrent attempts cannot both finalize the same transition
```

## Prompt 04 — Mandatory authentication, authorization, and tenant isolation

```text
Implement provider-neutral OIDC authentication for the web application and API.

Requirements:
- Authorization Code + PKCE for browser login
- secure, HttpOnly, SameSite cookies or a documented safe token architecture
- issuer, audience, expiry, nonce/state, and signature validation
- CSRF protection for cookie-authenticated mutations
- authenticated subject mapped to tenant memberships and roles
- roles: viewer, operator, reviewer, tenant_admin; platform_admin is isolated
- actor IDs derive only from verified identity/session claims
- authorization enforced in service/repository boundaries, not only UI/route middleware
- tenant filtering is mandatory in every repository method
- PostgreSQL row-level security as defense in depth where practical
- structured audit events for login, denial, role/configuration changes, and sensitive reads

Create a route-permission matrix and automated authorization tests.

Adversarial tests:
- no token, expired token, wrong issuer/audience
- forged actor/supervisor request field
- horizontal cross-tenant object ID access
- vertical privilege escalation
- tenant-admin attempt to access platform scope
- CSRF attempt

Acceptance:
- the API fails closed when auth is missing or misconfigured
- two test tenants cannot read, infer, update, or enumerate each other's records
- UI contains no security decision that the API does not independently enforce
```

## Prompt 05 — Secret management and network egress controls

```text
Replace every stored or returned credential with a secret reference abstraction.

Implement:
- SecretStore interface with local development and production adapter contracts
- cryptographically secure secret generation where the application must generate one
- write-once display; subsequent reads return metadata and reference only
- rotation metadata and last-used timestamp without exposing values
- automatic redaction in logs, traces, errors, API responses, and support exports
- startup scan/test proving no default or literal secrets in source/config fixtures

Remove arbitrary webhook test URLs. If webhook delivery is required later, implement:
- tenant-approved HTTPS destination allowlist
- DNS resolution and IP revalidation
- blocking of loopback, private, link-local, multicast, and cloud metadata addresses
- redirect restrictions, timeout, size limit, rate limit, egress proxy/policy
- HMAC signature and rotation without returning stored secret
- delivery through durable notification jobs, not request handlers

Tests:
- secret never appears in GET/list/API errors/logs/traces/SQL reporting fixtures
- SSRF attempts using redirects, decimal/hex IPs, DNS rebinding fixtures, IPv6, localhost,
  private ranges, and metadata IPs are denied
- rotation preserves overlap policy and audit evidence
```

## Prompt 06 — Immutable artifact storage and safe ingress

```text
Implement the first real ingress: authenticated multipart upload that streams directly to
an immutable S3-compatible object.

Requirements:
- bounded request size and per-tenant quota
- filename normalization; object keys generated by the server, never from a path supplied
  by the client
- streaming SHA-256, byte count, media sniffing, and optional detached-signature metadata
- do not load the full file into memory
- source object cannot be overwritten; derived objects use new IDs/keys
- persist source metadata and enqueue validation in the same reliable workflow
- idempotency key uses tenant, source identity, size, and content hash
- conflicting reuse of an idempotency key returns a typed conflict
- API returns 202 with job/artifact identifiers; it does not wait for full validation
- raw object access uses short-lived authorized URLs or a streaming proxy with audit

Threat tests:
- empty file, oversized file, path traversal filename, duplicate, conflicting duplicate,
  interrupted upload, content-length mismatch, decompression/archive bomb if archives are
  supported, and unauthorized object access

Performance test:
- representative large fixture proves bounded memory; report measured peak RSS and the
  command/environment used, without turning the result into a universal claim
```

## Prompt 07 — Fail-closed NACHA validation and policy engine

```text
Make NACHA the only production-claimed financial format.

Start with failing regression tests proving the current empty-file release bug and parser
exception behavior. Then implement a streaming validation pipeline.

Requirements:
- minimum file structure and exact record-length/record-type validation
- file header/control, batch header/control, entry/addenda relationships
- per-batch and file-level entry/addenda counts, hash totals, debit/credit totals
- ABA routing check-digit validation
- duplicate file/reference detection where the supported rule set permits it
- integer minor units or a decimal type for amounts
- PGP/signature policy separated from format parsing
- typed findings containing rule ID/version, severity, redacted evidence, record/byte offset
- no complete NACHA line in findings, logs, metrics, AI inputs, or default UI
- parser error, zero records, truncation, overflow, unsupported code/version, and failed
  verification quarantine the artifact
- explicit versioned release policy that maps findings to VALIDATED or QUARANTINED
- balanced/unbalanced handling comes from the feed contract; do not equate debits=credits
  with universal correctness

Use licensed/current Nacha rules for production rule semantics. Where the repository lacks
an authoritative rule source, mark the rule unverified and do not invent it.

Fixtures:
- valid single- and multi-batch files
- balanced and contract-authorized unbalanced files
- empty, truncated, wrong length, invalid trace/routing, mismatched counts/hash/totals,
  orphan addenda, duplicate, overflow, malformed characters, and tampered signature

Acceptance:
- no invalid fixture can reach RELEASED
- every finding links to a rule version and redacted offset
- results are deterministic across repeated runs
```

## Prompt 08 — Durable asynchronous jobs, idempotency, and outbox

```text
Implement a PostgreSQL-backed worker system; do not add Kafka or Redis yet.

Requirements:
- bounded worker count and per-tenant concurrency quota
- job leases using transaction-safe locking (for example SKIP LOCKED with carefully tested
  semantics), heartbeat, expiry, retry budget, exponential backoff with jitter
- idempotent handlers and a unique business idempotency key
- attempt records and typed terminal/dead-letter state
- transactional outbox for audit/SSE/notification events
- separate dispatcher that marks delivery only after success
- graceful shutdown stops leasing, completes or safely abandons active work, and releases
  resources
- backpressure returns a clear overload state rather than spawning unbounded work
- no external network call inside the transaction that commits business state

Concurrency tests:
- 50 workers race for one job; exactly one owns the valid lease
- process dies after business commit but before event publish; outbox delivers later once
- lease expires mid-work; stale worker cannot overwrite newer completion
- duplicate arrival and duplicate outbox delivery are harmless
- poison job reaches dead state without blocking the queue
- one tenant cannot consume all workers

Run Go race tests and a database integration stress test. Report observed results, not
hardcoded throughput.
```

## Prompt 09 — Concurrency-safe evidence ledger

```text
Repair the audit/evidence ledger.

Requirements:
- append is serialized per tenant/evidence stream in one transaction
- unique sequence and predecessor constraints prevent forks
- canonical event serialization is versioned
- each event includes tenant, actor/service identity, action, object type/id, timestamp,
  request/correlation ID, previous hash, payload hash, and redacted metadata
- verification detects mutation, deletion, reordering, and broken predecessor links
- audit rows are append-only to application roles
- sensitive source contents and secrets are never copied into audit payloads
- periodic verification job records a result; optional external signed checkpoint is
  designed but not falsely claimed if absent

Language rule:
Call this an application hash chain until a real Merkle structure/external anchoring exists.
Do not call a SHA-256 digest a digital signature.

Tests:
- concurrent append stress test has one linear sequence
- mutation/deletion/reorder detection
- tenant streams cannot reference each other
- redaction and maximum-payload tests
```

## Prompt 10 — Feed contracts, calendars, and materialized expectations

```text
Build the deterministic scheduling core that solves the silent-missing-file problem.

Feed contract versions include:
- tenant, partner, feed ID, direction
- safe filename/object matcher
- format=NACHA for production
- expected local time and IANA timezone
- grace period
- business calendar ID and schedule rule
- contract-authorized balanced/unbalanced mode
- owner and escalation policy reference
- activation interval and version

Materialize future expectation occurrences ahead of time. A missing file must have a row
that can become overdue even when no arrival event exists.

Requirements:
- deterministic PENDING/DUE/OVERDUE/BREACHED transitions
- idempotent generation across restart and concurrent schedulers
- arrival matching records ambiguity rather than choosing silently
- contract editing creates a new version; historical occurrence resolves to the version
  active on that date
- Federal Reserve/business calendars use verified published dates/rules and allow
  tenant-specific overrides
- timezone and DST behavior is explicit

Tests:
- on-time, grace, late, never arrives, arrives after breach
- zero history and AI fully offline
- spring-forward/fall-back, month/year boundary, observed holiday, override
- restart and two concurrent schedulers produce one occurrence
- ambiguous filename match requires review
```

## Prompt 11 — Human review and dual-control release

```text
Implement a real human review workflow without self-healing.

Requirements:
- review queue is tenant-scoped and contains redacted findings/evidence references
- reviewer action uses authenticated subject; request cannot supply actor identity
- approval includes decision reason, policy version, source artifact hash, validation run,
  and optimistic concurrency version
- configurable dual control requires two distinct authorized people
- proposer cannot be the second approver when separation of duties is enabled
- approval expires if source, findings, policy, or proposed derived artifact changes
- release creates an auditable transition and an outbox event
- original artifact remains immutable
- manual override is explicit, reason-required, strongly authorized, and separately
  reportable; it never rewrites validation results

Tests:
- forged actor ID ignored/rejected
- same person cannot satisfy two-person control
- stale approval cannot release changed content
- simultaneous approve/reject produces one legal outcome
- unauthorized role and cross-tenant attempts fail
```

## Prompt 12 — Server-backed operations UI

```text
Refactor the React UI so every production view is backed by authenticated server data.

Screens:
- expected feed/status board
- artifact and validation result
- quarantine/review queue
- evidence timeline
- contract management and version history
- measured service health/queue status

Requirements:
- generated/typed API client and centralized auth/error handling
- pagination and filters performed server-side
- SSE updates with last-event cursor and reconnect/replay
- explicit loading, empty, permission-denied, unavailable, stale, and partial-data states
- no silent mock fallback
- sensitive fields masked by API policy, not only CSS
- timestamps show source timezone and UTC on inspection
- accessible keyboard and screen-reader behavior for review actions
- confirmation and concurrency-conflict handling for consequential decisions
- demo mode is a separate visible profile

Tests:
- backend outage displays unavailable, not healthy demo data
- expired session and permission denial
- SSE disconnect/replay without duplicates
- stale decision conflict
- large paginated dataset does not load all rows into browser memory
```

## Prompt 13 — Observability, SLOs, and measured performance

```text
Instrument the real pipeline using OpenTelemetry-compatible traces, structured logs, and
Prometheus metrics. Delete every constant operational metric.

Measure:
- finalized-arrival to job-visible latency
- queue wait, processing duration, and end-to-end decision duration
- files/bytes/records processed with success/quarantine/failure labels
- active/leased/retry/dead jobs from database state
- API latency/error rate by normalized route, never by tenant/file ID label
- database/object-store dependency latency and errors
- SSE subscriber/replay health
- worker saturation and backpressure

Security/privacy:
- low-cardinality labels
- no tenant names, filenames, account data, tokens, secrets, raw queries, or file content
- correlation IDs are opaque

Create an SLO document with target values explicitly labelled TARGET until measured in a
defined environment. Add a benchmark harness for representative file sizes and concurrency.
Record commit, hardware/container limits, dataset generator version, command, raw output,
peak memory, p50/p95/p99, errors, and saturation point.

Tests:
- metrics change when work occurs
- worker gauge equals real lease/worker state
- telemetry redaction test
- high-cardinality regression test
- benchmark rejects structurally invalid generated NACHA files

Never put a result in README or UI until the reproducible result artifact exists.
```

## Prompt 14 — Failure recovery and operational runbooks

```text
Prove resilience through tests and runbooks, not a “failover simulator” UI.

Create integration scenarios for:
- API restart during upload acceptance
- worker kill before processing, during processing, after business commit, and before
  outbox delivery
- PostgreSQL unavailable/slow and restored
- object storage unavailable/slow and restored
- duplicate arrival burst
- partial/finalized SFTP-style arrival event ordering
- disk full/quota exceeded
- malformed poison file
- notification destination unavailable
- AI provider unavailable

For each scenario define expected state, recovery mechanism, data-loss tolerance, duplicate
tolerance, alert, and operator action. Automate what can be safely reproduced and write
short runbooks for the rest.

Requirements:
- deterministic ingestion continues when AI is unavailable
- no fabricated success during dependency failure
- readiness reflects inability to serve critical operations
- retries are bounded and visible
- recovery leaves an explainable audit trail

Report measured recovery time from the test environment as evidence, not a production SLA.
```

## Prompt 15 — Read-only evidence-grounded AI analyst

```text
Add one AI incident analyst only after Prompts 00-14 pass.

The analyst may read only:
- tenant-scoped incident metadata
- redacted deterministic findings
- expectation and prior occurrence history
- approved runbook passages
- measured system telemetry summaries

It may produce only a typed recommendation:
- concise incident summary
- hypotheses ranked with calibrated/qualitative uncertainty
- evidence IDs supporting each claim
- missing evidence and questions for the operator
- relevant runbook passage IDs
- recommended human next actions
- explicit statement that it made no system change

It must not receive tools for release, approval, repair, notification, connector writes,
arbitrary SQL, shell, secrets, or unrestricted object reads. It runs asynchronously and
stores model/provider/prompt/schema versions, redacted inputs, output, latency, token usage,
cost, and policy decision. Provider failure produces UNAVAILABLE; never a synthetic answer.

Security tests/evals:
- prompt injection inside filename, file content, finding text, and runbook
- request to reveal secrets or cross-tenant data
- nonexistent evidence citations
- unsupported settlement/compliance claims
- excessive confidence with missing evidence
- output-schema violation, timeout, rate limit, and provider outage
- repeated-run consistency for invariant facts

Acceptance:
- every citation resolves to authorized evidence
- unsupported citations fail evaluation
- the analyst cannot mutate any application state
- deterministic processing and UI continue when AI is disabled

Map the risk register and evaluation evidence to the NIST AI RMF Generative AI Profile;
do not claim certification or compliance.
```

## Prompt 16 — Multi-database connector platform

```text
Do not restore the old hardcoded Integration Hub. Build a metadata-driven connector
platform for customer source databases while retaining PostgreSQL as Sentinel Flow's own
internal system of record.

Work in four independently reviewable stages. Stop after each stage and obtain approval
before proceeding. Do not claim or enable a connector until its real integration and
conformance suite pass.

Stage 16.1 — connector contracts and catalog

Create a connector interface with:
- ValidateConfig (structural checks only; never logs secrets)
- TestConnection (real, bounded, read-only check)
- DiscoverResources (approved metadata only)
- ExecuteTemplate (administrator-approved parameterized query only)
- Health (real timestamped result and sanitized error category)
- Close/cancellation

Create a capability model because databases do not share identical behavior:
- schemas/catalogs, parameter style, identifier quoting, cursor/stream support
- read-only transaction support
- statement cancellation/timeout
- TLS verification modes
- authentication modes
- maximum row/byte/page limits
- metadata and aggregate-query capabilities

Initial catalog entries:
PostgreSQL, MySQL, MariaDB, SQL Server, Oracle, Snowflake, Amazon Redshift,
Google BigQuery, and Databricks SQL.

Every catalog entry has status:
PLANNED | IMPLEMENTING | AVAILABLE | DEGRADED | DISABLED.
Only conformance-tested connectors may be AVAILABLE.

Stage 16.2 — metadata-driven connection UI

Build one generic connection wizard whose fields come from server-owned descriptors.
Selecting a connector displays only the fields and authentication methods that apply:
- host/port/database/service/account/project/warehouse/catalog/schema as applicable
- username only when applicable
- password, private key, wallet, OAuth, service-account, or PAT as write-only secret input
- TLS/CA/client-certificate settings with secure defaults
- approved schemas/datasets/catalogs and row/byte/time limits

Display a safe connection template with placeholders, never a saved complete connection
string. After save, show a masked canonical summary and secret reference metadata only.

If “paste connection URI” is supported, parse it immediately, extract credentials into
the SecretStore, discard the raw value, and never place it in logs, audit payloads, browser
storage, metrics, or API reads. Do not offer URI paste for providers such as BigQuery where
structured identity/project configuration is the correct model.

Stage 16.3 — shared conformance suite

Build a reusable black-box suite that every driver must pass against a real disposable
database/account fixture:
- valid TLS connection and rejection of untrusted certificates
- authentication success/failure with error redaction
- resource discovery limited to allowlisted resources
- parameter binding and identifier allowlisting
- read-only enforcement and denial of DDL/DML/multi-statement execution
- timeout, cancellation, row limit, byte limit, cursor pagination
- connection-pool exhaustion and recovery
- secret rotation/revocation
- cross-tenant connector access denial
- audit evidence without credential/query-result leakage
- cleanup with no goroutine/connection leak

Record connector name, server/driver version, test commit, test time, and result artifact.

Stage 16.4 — real drivers

Implement adapters in this order because each adds a distinct protocol/auth surface:
1. PostgreSQL
2. MySQL and MariaDB as separate tested capability profiles
3. SQL Server
4. Snowflake
5. Oracle
6. Amazon Redshift
7. Google BigQuery
8. Databricks SQL

Use official supported drivers/SDKs with pinned versions and license review. Reuse the
shared interface and tests; do not hide provider differences behind lowest-common-
denominator behavior. Each implementation receives its own small task/session, threat
review, test fixture, and completion report.

Connector model:
- tenant and connector ID
- capability allowlist
- secret reference
- approved hosts/ports/database
- TLS mode and verified certificate metadata
- health based on real checks with timestamp/error class
- resource allowlist
- per-connector rate/concurrency/time/row/byte limits
- audit events and last-used metadata

All connectors:
- metadata/schema discovery only for approved schemas
- administrator-defined parameterized query templates
- least-privilege source identity plus database/provider read-only controls and timeout
- no arbitrary SQL from browser or AI
- no DDL/DML, multi-statement execution, extension calls, file/network functions, or
  unbounded result sets
- cursor pagination and explicit column classification/masking
- return approved aggregates/metadata by default, not raw financial rows

Threat tests:
- SQL injection through parameters/identifiers
- attempt to execute write/DDL or multi-statement SQL
- TLS downgrade or certificate-validation bypass
- oversized/slow result
- credential/connection error redaction
- cross-tenant connector ID
- access to undeclared schema/table
- cancellation and pool exhaustion
- provider-specific privilege or external-function escape

Acceptance:
- revoking the connector secret/role stops access
- UI accurately distinguishes never-checked, healthy, degraded, and failed
- no secret or raw connection string is retrievable
- every selectable connector has a real passing conformance artifact
- unimplemented/failed connectors remain visibly disabled rather than returning mock data
```

## Prompt 17 — Finalized SFTP ingress and private edge design

```text
First perform a documented SFTPGo license and integration decision. Do not assume separate
containers settle AGPL/commercial obligations. Record legal review as REQUIRED for any
commercial deployment.

Integration spike:
- run unmodified SFTPGo locally
- capture a real finalized-upload event and document its exact payload
- determine whether event delivery is at-least-once and how completion is represented
- verify user, source, path/object identity, size, timestamp, success, and hash availability
- design replay/reconciliation when a webhook is lost
- do not process filesystem files before upload finalization

Only after the spike, implement authenticated event ingestion, event idempotency, object
copy/reference, reconciliation, and audit evidence.

For future private-network customers, write an edge-agent design before coding:
- outbound-only control connection where possible
- real TLS 1.2+ and mutual certificate verification
- device enrollment bound to tenant and connector
- short-lived/rotatable certificates, revocation, expiry monitoring
- signed configuration and update provenance
- resource/host allowlist; no general shell
- SFTP/file/database capabilities implemented separately with least privilege
- local metadata/redaction policy and bounded spool for outages

Tests must prove an ordinary HTTP client or untrusted certificate can never produce
mTLSVerified=true. Derive that field from verified transport state only.
```

## Prompt 18 — Secure SDLC and supply-chain evidence

```text
Create a CI pipeline aligned to secure-development practices, without claiming that CI
alone creates compliance.

Required gates:
- formatting, linting, type checking
- Go unit/integration/race tests
- frontend unit tests and production build
- Python lint/type/test/eval when applicable
- migration tests and clean-stack smoke test
- secret scanning
- dependency and container vulnerability scanning with documented severity policy
- SBOM for release artifacts
- license inventory and policy exceptions
- reproducible container builds pinned by digest where appropriate
- signed release artifacts/provenance where the platform supports it
- protected production configuration; no secrets in CI logs/artifacts

Create SECURITY.md, vulnerability-reporting instructions, supported-version policy,
dependency-update policy, and a release checklist. Map engineering evidence to OWASP ASVS
5.0 Level 2 as a target and NIST SSDF practices. Record PASS/PARTIAL/NOT APPLICABLE with
artifact links; do not claim formal certification.

Acceptance:
- a deliberately vulnerable fixture or policy violation fails the relevant gate
- released image maps to source commit and SBOM
- all required checks are branch-protection-ready
```

## Prompt 19 — Threat model and independent security test plan

```text
Create docs/security/THREAT_MODEL.md using data-flow diagrams and trust boundaries for:
browser, API, worker, PostgreSQL, object storage, OIDC provider, optional AI provider,
connector target, optional SFTPGo, and optional edge agent.

Inventory assets: financial files, normalized metadata, credentials, tenant membership,
approval authority, audit evidence, model inputs/outputs, logs, and release events.

Analyze at least:
- cross-tenant access and IDOR
- broken authorization and forged approval identity
- upload/path/content attacks
- parser denial of service and integer overflow
- duplicate/replay/race conditions
- SSRF and connector pivoting
- SQL injection and unsafe query templates
- secret leakage
- audit-log tampering
- supply-chain compromise
- prompt injection/data exfiltration through AI
- insider misuse and separation-of-duties failure
- backup/restore and deletion/retention failure

For every threat record precondition, impact, preventive/detective controls, test, residual
risk, owner, and status. Produce a penetration-test scope and safe test-data plan. Do not
mark risks closed unless an automated test or review artifact exists.
```

## Prompt 20 — Portfolio, demo, and recruiter evidence

```text
Prepare the repository to attract fintech/platform/security employers without exaggeration.

Create:
- a concise README with problem, architecture, one vertical-slice demo, threat model,
  measured benchmark method/results, screenshots, and exact local-start instructions
- docs/DECISIONS.md linking architecture decision records
- docs/SECURITY_AND_PRIVACY.md describing controls, limitations, test data, retention,
  redaction, and responsible disclosure
- docs/OPERATIONS.md linking SLOs, alerts, backup/restore, and failure runbooks
- docs/AI_SAFETY.md describing the read-only boundary and eval results
- a five-minute demo script
- a 90-second screen-recording script
- an interview deep-dive with five tradeoff decisions and three defects found/fixed

Demo story:
1. A feed expectation exists before the file arrives.
2. A malformed/empty/tampered NACHA file arrives.
3. It is streamed, hashed, and quarantined with redacted evidence.
4. Duplicate delivery is deduplicated.
5. The UI updates from real SSE state.
6. A reviewer inspects evidence and follows the controlled workflow.
7. The read-only analyst cites evidence and a runbook but cannot release the file.
8. A restart/retry test proves recovery.
9. A benchmark page links to reproducible raw results.

README rules:
- label implemented, experimental, planned, and out-of-scope separately
- no bank/customer logos without permission
- no invented user counts, savings, compliance, throughput, or accuracy
- use “designed toward” or “mapped to” for framework work, not “certified/compliant”
- include the exact environment for every measured number

Provide truthful resume bullets in the form:
action + difficult constraint + measured result + verification method.
If a result has not been measured, leave a placeholder such as [MEASURED_P95] rather than
inventing a number.
```

## Prompt 21 — Final release-readiness gate

```text
Perform a release-candidate review. Fix nothing during the first pass.

Create a matrix with PASS, FAIL, PARTIAL, NOT RUN, or NOT APPLICABLE and evidence links for:
- clean build and dependency locks
- migrations and restart persistence
- authentication, authorization, tenant isolation, and CSRF
- secret handling and egress restrictions
- upload/object immutability and data redaction
- NACHA fail-closed validation fixtures
- state-machine and dual-control rules
- job idempotency, concurrency, retry, and outbox recovery
- audit-chain concurrency and verification
- deterministic scheduling/timezone/calendar cases
- UI degraded/permission/stale/replay states
- telemetry correctness and reproducible performance results
- failure-recovery scenarios and runbooks
- AI read-only boundary and adversarial evals
- connector permission/SSRF/SQL tests if connectors are included
- dependency, container, license, SBOM, and release-provenance checks
- backup restore test and data-retention/deletion behavior
- threat-model residual-risk review
- README/demo claim traceability

For every README, UI, API, metric, and demo claim, point to its computation or evidence.
Classify unsupported claims as REMOVE, RELABEL AS TARGET, or BLOCK RELEASE.

Release only if all P0 controls pass and every NOT RUN item has an explicit owner and
accepted reason. “The code looks right” is not evidence. Finish with the top five residual
risks and the exact scope this release is safe to demonstrate—not a claim that it is safe
for live customer funds or production financial data.
```

---

## 5. Coding-assistant task wrapper

Prefix any smaller task with this wrapper so the assistant does not expand scope or hide uncertainty:

```text
Read AGENTS.md and the audit/recovery plan first.

Before editing, respond with:
1. the goal in one sentence
2. the exact acceptance tests
3. files/modules likely affected
4. security, privacy, concurrency, and tenant-boundary risks
5. explicit out-of-scope items

Then inspect the existing implementation. Add a failing regression/behavior test before
the fix whenever practical. Implement the smallest coherent change. Do not add adjacent
features. Do not substitute mocks or hardcoded success when infrastructure is unavailable.

At completion, provide the standard eight-part report from the project guide and paste the
verification results. If anything cannot be verified, mark it NOT RUN and stop short of a
production-ready claim.
```

---

## 6. Suggested 8-week job-focused build plan

This schedule prioritizes visible engineering signal rather than maximum feature count.

| Week | Outcome | Prompts |
|---:|---|---|
| 1 | Honest, reproducible, smaller codebase | 00–02 |
| 2 | Domain state machines, auth, tenant isolation | 03–04 |
| 3 | Secret boundary, immutable streaming upload | 05–06 |
| 4 | Fail-closed NACHA validator | 07 |
| 5 | Durable jobs, idempotency, evidence ledger | 08–09 |
| 6 | Expectations, calendars, review workflow, UI | 10–12 |
| 7 | Observability, benchmark, recovery proof | 13–14 |
| 8 | Read-only analyst, secure CI, portfolio/demo | 15, 18, 20–21 |

Prompts 16–17—database connectors, SFTPGo, and an edge agent—remain stretch work after the core release gate passes. Build the entire connector catalog and dynamic connection wizard first, but enable each database only after its real driver passes the same conformance suite. Several honestly disabled connectors are more credible than nine simulated “healthy” connections. A safe SFTP integration is more valuable than SSH shell access.

---

## 7. What will actually attract employers

Recruiters may notice the UI, but experienced interviewers will remember the engineering evidence:

- You found that an empty ACH file was being released and wrote a fail-closed validator.
- You replaced fake mTLS with verified transport identity—or removed the claim until it existed.
- You designed tenant isolation and proved it with adversarial tests.
- You streamed large inputs with bounded memory and published reproducible measurements.
- You proved job idempotency and recovery across process failure.
- You prevented an AI system from having financial side effects and evaluated prompt injection and evidence fabrication.
- You made the README less impressive-sounding but more trustworthy.

Strong interview sentence:

> I deliberately reduced the feature count. The original prototype simulated connectors, security, performance, and AI orchestration. I rebuilt one operationally painful path end to end, made every visible claim traceable to evidence, and proved the failure and concurrency behavior rather than only the happy path.

---

## 8. Standards used as engineering references

Use these as control and evidence references, not as self-certification:

- [OWASP Application Security Verification Standard 5.0](https://owasp.org/www-project-application-security-verification-standard/) — use Level 2 as an application-security verification target appropriate to systems handling sensitive data.
- [NIST SP 800-218, Secure Software Development Framework](https://csrc.nist.gov/pubs/sp/800/218/final) — organize secure-development practices and evidence. As of this guide, 1.1 is final and 1.2 is an initial public draft; do not cite the draft as final.
- [NIST AI RMF Generative AI Profile](https://www.nist.gov/publications/artificial-intelligence-risk-management-framework-generative-artificial-intelligence) — structure the AI risk register, evaluation, monitoring, and human-oversight evidence.
- [FFIEC Architecture, Infrastructure, and Operations guidance](https://www.ffiec.gov/news/press-releases/2021/pr-06-30) — understand the governance, architecture, infrastructure, and operational concerns financial institutions evaluate.

Exact regulatory and contractual requirements depend on the product role, customer, data, jurisdiction, and payment rails. PCI DSS is not automatically in scope unless cardholder-data systems are involved. NACHA rule use and SFTPGo commercial architecture require appropriate license/legal review.

---

## 9. Final scope discipline

Do not try to automate “everything enterprises do manually.” That is too broad to build, test, explain, or trust.

Automate this narrow, expensive loop exceptionally well:

1. Know what file is expected.
2. Know whether a finalized file arrived.
3. Prove which immutable object was examined.
4. Validate it deterministically.
5. Quarantine unsafe input.
6. Explain the evidence to an operator.
7. Control the human decision.
8. Recover safely from duplicates, crashes, and dependency failures.
9. Measure the time and labor removed.

That is already a serious fintech platform project—and a much stronger job signal than a broad set of unverified dashboards and agents.
