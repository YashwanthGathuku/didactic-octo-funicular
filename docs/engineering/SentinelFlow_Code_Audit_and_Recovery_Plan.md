# Sentinel Flow Code Audit and Recovery Plan

**Audit date:** August 14, 2026  
**Reviewed artifact:** `SentinelFlow-remediated_2.zip` and the accompanying research, roadmap, and implementation-prompt documents

## Executive verdict

Sentinel Flow has a strong product wedge and a visually compelling operations interface, but the attached code is currently a **demonstration prototype, not a deployable fintech platform**.

The most important problem is not missing functionality. It is that several screens and APIs present simulated data as if it were verified infrastructure behavior. In runtime checks, an unauthenticated HTTP request was reported as mTLS-verified, an empty instant-payment payload was reported as compliant and settled, an empty ACH file was released, and compliance and performance endpoints returned authoritative-looking claims that were not produced by real controls or measurements.

**Recommendation:** do not add more connectors, agents, formats, or executive dashboards yet. First complete a short “truth reset,” then build one production-shaped vertical slice: authenticated file arrival → immutable source object → expectation occurrence → deterministic validation → quarantine/release decision → evidence-backed UI.

This can still become an excellent job-seeking project. The strongest portfolio story is not “we built every fintech feature.” It is: **“I found and removed prototype theater, built a secure and measurable ingestion path, proved failure recovery and idempotency, and added a constrained evidence-grounded agent only where it improved operations.”**

## Scorecard

| Area | Current assessment | Evidence-based interpretation |
|---|---:|---|
| Product problem and positioning | B+ | The pre-ledger reliability gateway is specific, understandable, and relevant to financial operations. |
| UI and demonstration craft | B | The operations interface is attractive and broad, but much of its state is synthetic or falls back silently to mock data. |
| Core ingestion reliability | D | Full-file reads, polling, incomplete-upload risk, weak idempotency, and an empty ACH file being released are release blockers. |
| Security and tenant isolation | F | Authentication is optional, tenant isolation is absent, webhook secrets are returned and queryable, arbitrary webhook tests create SSRF risk, and mTLS status is fabricated. |
| Connector implementation | F | The Integration Hub and discovered resources are hardcoded; the edge agent uses ordinary HTTP while claiming mTLS. |
| Format correctness | D | Parsers are useful prototypes, but validation is incomplete and several accounting/format assumptions are unsafe for real financial files. |
| AI and agent implementation | D | The swarm is a scripted transcript, the evaluation validates static behavior, and the Go service can fabricate a perfect evaluation when Python is unavailable. |
| Performance evidence | D | Throughput, latency, worker-pool, SLA, and failover values are primarily hardcoded rather than measured. |
| Testing and reproducibility | C− / unverified | Tests exist, but the supplied environment and container definitions do not reproduce the claimed build. |
| Documentation accuracy | D | The roadmap is candid about features to remove, but the archive still exposes them and the README contains conflicting claims. |

## Audit scope and limitations

The review included:

- Static inspection of the Go gateway, React/Vite client, Python AI tier, edge agent, tests, container files, Compose files, schema/migrations, documentation, and configuration.
- Safe local execution of the supplied prebuilt gateway binary and requests against its HTTP API.
- Python evaluation execution both as documented and with a corrected import path.
- Reproducibility checks for Go, JavaScript, Python, containers, configuration, CI, and documented endpoints.

Limitations:

- The environment did not contain a Go toolchain, so the source-level Go test suite, `go vet`, and a clean Go build could not be executed.
- JavaScript dependencies were not installed and package download was unavailable, so the Vite/TypeScript build could not be independently reproduced.
- No external bank network, SFTPGo server, database, object store, secret manager, or payment rail was supplied. Claims involving them were therefore assessed from code and local behavior.
- Runtime checks used the supplied binary. A clean source build should be required before any release because a committed binary is not adequate build provenance.

## Verified runtime findings

These are observations from actual requests to the supplied gateway binary, not inferences from UI copy.

| Test | Observed result | Why it matters |
|---|---|---|
| Edge sync over ordinary unauthenticated HTTP | Response reported `mTLSVerified: true` | The server does not verify a client certificate before making the claim. This is a false security assertion. |
| Empty instant-payment validation payload | Reported compliant and `SETTLED_INSTANT`; generated amount, routing data, IDs, and network | A validator must fail closed on missing or invalid input. Fabricating settlement is dangerous. |
| Instant-payment metrics | Returned 1.42 ms average latency, 99.998% SLA compliance, and 12,500 TPS | These values are constants, not benchmark measurements. |
| Prometheus metrics | Reported eight active workers | The metric is hardcoded and does not represent a real worker pool. |
| Empty compliance database export | Reported chain integrity verified and named SEC/SOX/FINRA, Merkle-chain, Moov, and SIMD controls | The labels overstate implemented controls; the chain is neither a Merkle tree nor externally anchored WORM evidence. |
| Webhook creation and retrieval | Secret was stored and returned in plaintext | Secrets must not be recoverable through normal read APIs or the analytics surface. |
| SQL console query | `SELECT url, secret FROM webhook_subscriptions` returned webhook secrets | The “read-only” restriction does not prevent confidential data disclosure. |
| SLA board | Returned fixed 12.5% breach risk and a fixed countdown | The UI suggests live predictive operations when the values are not derived from current evidence. |
| Empty ACH ingest | Returned `RELEASED` and `isBalanced: true`; the parser exception was only a warning | Empty or unparseable financial files must be quarantined, never released. |
| Authentication with no configured token | API ran unauthenticated | A fintech service must fail closed or bind only to an explicitly local demo profile. |

## Release blockers — P0

### 1. Invalid or empty financial input can be released

`gateway/processor.go` initializes a result as `RELEASED`, reads the complete body, and treats a Moov ACH parser exception as a warning. The runtime test confirmed that an empty ACH file is released and considered balanced.

Required correction:

- Start every ingestion in `RECEIVED` or `VALIDATING`, never `RELEASED`.
- Require a recognized format, minimum structural records, successful parser completion, and a deterministic policy decision.
- Treat parser exceptions, zero records, truncation, duplicate identifiers, and unsupported versions as quarantine conditions.
- Encode release policy as explicit, versioned rules with tests that prove malformed inputs cannot reach release.

### 2. Security controls are asserted but not performed

`edge-agent/main.go` defaults to an `http://` URL and uses a plain `http.Client`. `gateway/connector.go` returns `mTLSVerified: true` without inspecting TLS state or a verified certificate chain.

Required correction:

- Remove the mTLS label and Edge Sync API from the public demo until real mutual TLS is implemented.
- When implemented, use a private CA, certificate rotation, certificate-to-tenant/workspace binding, expiry/revocation checks, and server-side inspection of verified peer certificates.
- Derive security-status fields exclusively from authenticated transport/session state. Never accept them from a client or constant.

### 3. Instant-payment validation fabricates successful settlement

`gateway/instant_payment.go` does not validate the submitted XML before producing success. It creates synthetic payment values and fixed performance metrics.

Required correction:

- Remove the module from version one, or relabel it as an isolated synthetic UI demonstration that cannot be confused with a payment control.
- If retained later, parse exact ISO 20022 message types, validate XSD and applicable market-practice rules, use decimal/integer minor units, and separate message validation from rail acceptance and settlement.
- Never synthesize `SETTLED` status. Settlement must come from an authenticated external event.

### 4. Authentication, authorization, and tenant boundaries are missing

`gateway/main.go` disables authentication when no token is configured. The data model and routes have no dependable tenant boundary. Approval and remediation endpoints trust caller-provided actor fields.

Required correction:

- Default to denying all non-health requests when authentication is not configured.
- Add workspace/tenant IDs to every business record and enforce them in repository methods, not only handlers.
- Use authenticated subject claims for actor identity. Ignore caller-supplied supervisor or approver identity.
- Add role-based permissions for configuration, review, approval, release, connector management, and evidence export.
- Require two distinct authenticated principals for dual-control workflows where claimed.

### 5. Secret handling and webhook testing are unsafe

Webhook secrets are generated from time-based values, stored in plaintext, returned through APIs, and readable through the SQL console. The webhook-test endpoint can call an arbitrary URL, creating a server-side request forgery risk.

Required correction:

- Generate secrets with a cryptographically secure random source; show the secret once and store only a protected form appropriate to the verification protocol.
- Use a real secret manager and persist only secret references.
- Remove arbitrary SQL and arbitrary URL webhook testing from version one.
- Later, use tenant-scoped destination allowlists, HTTPS only, DNS/IP revalidation, private/link-local/metadata-range blocking, redirect restrictions, egress policy, timeouts, body limits, and audit records.

### 6. AI and evaluation fallbacks report success without evidence

`ai-tier/swarm.py` produces a scripted transcript rather than an evidence-dependent multi-agent workflow. `gateway/main.go` can return a fabricated 5/5 evaluation if the Python evaluator is unavailable.

Required correction:

- Remove all success fallbacks. Return `UNAVAILABLE` or `NOT_EVALUATED` when a dependency cannot run.
- Replace the “swarm” with one read-only analyst after the deterministic pipeline is correct.
- Require typed evidence references, traceable runbook citations, prompt/tool logs, redaction, budget/time limits, and an explicit no-side-effects policy.
- Evaluate containment, unsupported assertions, evidence attribution, injection resistance, data minimization, and refusal behavior against a real system under test.

### 7. “Self-healing” is not bound to an authenticated approval

`gateway/healing.go` performs one hardcoded routing change and assigns high confidence. Its apply endpoint accepts caller-supplied supervisor identity and repaired content, then re-ingests the content without binding it to an immutable proposal or approval record.

Required correction:

- Remove automatic/self-healing from version one.
- Later, create immutable proposals containing the source hash, rule version, exact diff, rationale, risk, and expiry.
- Require approval by an authenticated, authorized second person; bind the approval cryptographically/transactionally to the proposal hash.
- Create a new derived artifact, never mutate the source, and rerun every deterministic validation before release.

### 8. Raw financial data is overexposed

`gateway/processor.go` retains entire raw contents and includes complete NACHA lines in findings. The LLM client can send raw input to a model.

Required correction:

- Store source files in encrypted object storage and pass opaque references through the application.
- Persist only necessary normalized fields, hashes, offsets, and redacted evidence excerpts.
- Establish field classification, masking, retention, deletion, and model-egress policies before accepting customer data.
- Default AI inputs to metadata and redacted evidence, with explicit tenant configuration for any broader use.

## Correctness and reliability risks — P1

### Integration Hub is a mock, not a connector platform

`gateway/connector.go`, `edge-agent/main.go`, and the Integration Hub UI contain hardcoded systems, resources, secret references, latency values, and sample data. This should not be described as database, API, SSH, or shared-path connectivity.

For the first real connector, choose exactly one:

1. A read-only PostgreSQL metadata connector, or
2. An SFTP/inbox connector that receives file-arrival events.

Do not start with arbitrary remote SQL, SSH shell access, or filesystem browsing. Those greatly expand credential, network, authorization, exfiltration, and support risk.

### Ingestion is not streaming or idempotent

The API and watcher use full-file reads; the watcher polls once per second and may process a file before upload completion. A rename failure is ignored. There is no dependable source-event idempotency key.

Required architecture:

- Arrival notification references an immutable object or a finalized file.
- Idempotency key includes tenant, connector, remote object identity/version, size, and content hash.
- A durable job record owns state transitions and retries.
- Validation streams through bounded buffers and records byte/line offsets.
- Repeated delivery returns the existing outcome without duplicating ledger entries or notifications.

### The audit chain can fork under concurrency

`gateway/ledger.go` selects the last hash and then inserts a new record without a transaction or lock. Concurrent writers can select the same predecessor and create branches. The structure is a hash chain, not a Merkle tree, and it has no external checkpoint.

Required correction:

- Serialize append per tenant/stream using a transaction and database locking strategy.
- Add a unique sequence/predecessor constraint.
- Run a verification job and publish signed checkpoints to an independent store if tamper-evidence is claimed.
- Use precise language: “application hash chain” until stronger evidence exists.

### Rule and incident attribution is hardcoded

The processor uses a fixed expectation ID, fixed storage path, and NACHA-specific incident description even when other parsers or causes are involved.

Required correction:

- Resolve an actual expectation occurrence based on tenant, source, filename/object key, schedule, date window, and format.
- Use typed cause codes and format-specific evidence.
- Separate raw observations, deterministic findings, policy decisions, and operational incidents.

### Format validators are prototype-grade

- **NACHA:** global totals and last-batch control handling are insufficient for multi-batch semantics; a simple debit-equals-credit flag misrepresents valid balanced and unbalanced arrangements.
- **ISO 20022:** money uses `float64`; there is no XSD or market-practice validation and only a small XML projection is parsed.
- **BAI2:** record and transaction-code logic is superficial; debit/credit classification is based on string matching.
- **SWIFT:** regular-expression/tag checks are not production message validation.

Recommendation: make NACHA the only fully claimed version-one format. Label all others “experimental parser previews” or remove them until supported with licensed/current rule sources and conformance fixtures.

## Build and reproducibility defects

| Defect | Impact | Correction |
|---|---|---|
| `go.mod` requires Go 1.26.4 while the gateway container uses Go 1.22 | Clean container build is incompatible | Pin one supported Go version across local, CI, and container builds. |
| AI container copies a missing `ai-tier/requirements.txt` | Container build fails before fallback logic | Add a locked dependency file and build it in CI. |
| Root Compose omits SFTPGo, PostgreSQL, and MinIO | Documented stack is not instantiated | Provide one authoritative local stack or remove the claims. |
| Secondary Compose includes PostgreSQL/MinIO, but the app opens SQLite | Infrastructure is disconnected from runtime | Choose one database for version one and integrate/test it end to end. |
| `AI_TIER_URL` is supplied but Go hardcodes `127.0.0.1` | AI container cannot be reached from the gateway container | Centralize validated configuration and test Compose networking. |
| SQLite file is outside the mounted data volume | State is lost when the container is replaced | Store it in the mounted path or use PostgreSQL. |
| Migration runs opportunistically and is not versioned | Upgrades are not reproducible | Use versioned, idempotent migrations with upgrade/rollback tests. |
| No CI workflow despite a CI badge | Badge is unsupported | Add CI or remove the badge. |
| No license file despite an MIT badge | Distribution terms are unclear | Add the intended license after dependency/license review or remove the badge. |
| README says 26/26 while badge says 38/38 | Test claim is inconsistent | Publish CI-generated counts or avoid fixed counts. |
| README benchmark route does not match implementation | Documentation is stale | Generate or test API documentation against registered routes. |
| Prebuilt 20 MB gateway binary is committed | Weak provenance and repository bloat | Remove it; produce signed checksummed release artifacts from CI. |
| UI hardcodes localhost and sends no authorization header | Deployed/authenticated setup breaks | Use runtime configuration and a real session/token client. |
| UI silently falls back to synthetic data | Operational failure can look healthy | Show explicit demo mode and dependency errors; never silently substitute production state. |

## Research and claim corrections

### Predictive SLA model

The research document contains useful ideas—quantile forecasts, conformal calibration, multiple-testing control, and drift monitoring—but several claims need correction before implementation:

- A split-conformal interval does not automatically provide a calibrated cumulative distribution or a direct probability of deadline breach.
- Sixty observations from the same weekday require roughly sixty weeks, not a 90-day synthetic history.
- Twenty-one training observations are too sparse for a dependable per-feed gradient-boosted quantile model plus calibration.
- Repeatedly checking a stored quantile forecast does not, by itself, create a continuously updated breach probability.
- Positive dependence among feeds cannot be assumed solely because partners share infrastructure; the conditions behind the selected false-discovery-rate procedure must be justified.
- Synthetic calibration can verify pipeline mechanics, but it is not evidence of customer-data predictive performance.
- A single 30-day holdout makes a narrow 86–94% coverage acceptance band unstable and highly granular.

Version-one SLA status should therefore be deterministic: expected-by time, grace period, arrival state, last successful observation, and explicit escalation timers. Add predictive risk as advisory only after enough real, tenant-specific history exists and the model is evaluated prospectively.

### External institutional claims

The accompanying BNY material mixes supportable current facts with unsupported framework and outcome claims. A current BNY publication supports more than 220 AI solutions and 140 digital employees, but the project should remove or directly source the named “RRR framework,” multi-agent research tracks, quantified productivity/outcome claims, and attributed bank use cases before publishing them. The Carnegie Mellon announcement supports a $10 million partnership focused on AI education, research, governance, trust, and accountability; it does not substantiate every framework described in the project report.

Relevant sources:

- [BNY — The Intelligent Investor: Insights on AI-forward investing](https://www.bny.com/content/dam/bnymellonwealth/pdf-library/reports/the-intelligent-investor-insights-on-ai-forward-investing.pdf)
- [Carnegie Mellon University — BNY and CMU partnership](https://www.cmu.edu/news/stories/archives/2025/september/bny-and-carnegie-mellon-university-join-forces-to-advance-ai-education-and-research)
- [SEC filing containing current BNY disclosures](https://www.sec.gov/Archives/edgar/data/1390777/000119312526092500/d52987ddef14a.htm)

### SFTPGo licensing

The roadmap’s assumption that a separate unmodified SFTPGo service broadly removes licensing obligations is too simple. SFTPGo’s official guidance states that architecture, coupling, and the nature of a managed-file-transfer service matter; containers alone do not determine whether a separate service is derivative or subject to commercial terms. Select the deployment model only after reviewing the exact integration and obtaining legal advice for commercial use.

- [SFTPGo licensing and compliance guidance](https://sftpgo.com/compliance)
- [SFTPGo repository and license](https://github.com/drakkan/sftpgo)

## What is worth preserving

The audit is not a recommendation to discard the project. Preserve these parts:

- The core positioning: a pre-ledger reliability gateway for files and operational deadlines.
- The visual design language and operations-oriented UI structure.
- Modular route organization in the Go gateway.
- Real SHA-256 hashing and the ABA-routing Mod10 implementation.
- Use of the Moov ACH library as one layer of NACHA parsing.
- Fail-closed PGP detached-signature verification and tamper tests.
- Improved SSH public-key parsing.
- Hash-chain content verification as a starting point, after fixing concurrent append semantics and claims.
- Median/MAD and KS/BH experimental code and tests, clearly separated from production decisions.
- The roadmap’s own recommendation to remove vault, swarm, chaos, DR, executive, benchmark, and mock integration surfaces.

## Strongest implementation sequence

### Phase A — Truth reset (2–3 focused days)

Goal: every visible claim corresponds to implemented behavior.

- Remove or feature-flag the Integration Hub, instant payments, vault, agent swarm, chaos monkey, failover simulator, executive deck, arbitrary SQL console, self-healing, benchmark modal, and predictive breach percentage.
- Add an unmistakable `DEMO_DATA` banner if any synthetic corpus remains.
- Delete fabricated success fallbacks, hardcoded operational metrics, unsupported compliance labels, stale badges, and the prebuilt binary.
- Correct README endpoint, test, architecture, and support claims.
- Make authentication fail closed outside a named local demo profile.

Verification checkpoint:

- A repository-wide search finds no hardcoded success metrics or simulated security/compliance fields on production routes.
- When a dependency fails, the UI shows `Unavailable`, never mock healthy state.

### Phase B — Reproducible secure foundation (3–5 days)

Goal: one command builds and starts the same stack CI tests.

- Align Go, Node, and Python versions; lock dependencies.
- Choose PostgreSQL for durable metadata and MinIO/S3-compatible storage for immutable source objects.
- Add versioned migrations, health/readiness checks, structured logs, tracing IDs, and configuration validation.
- Add a simple OIDC/session integration or a well-defined local authentication adapter; enforce tenant and role boundaries in repositories.
- Store secrets in a local development secret provider and define production adapters by interface.
- Add CI for formatting, linting, unit tests, race tests, dependency scanning, container builds, and integration tests.

Verification checkpoint:

- A clean checkout builds without committed binaries.
- CI starts the stack, migrates an empty database, runs tests, and restarts without losing state.
- Unauthenticated and cross-tenant access tests fail closed.

### Phase C — One real vertical slice (approximately 2 weeks)

Goal: process one real NACHA file-arrival flow end to end.

- Use one finalized inbox/SFTP event or an authenticated upload that writes an immutable object first.
- Create a versioned expectation template and a concrete expectation occurrence for the business date.
- Create an idempotent ingestion job using source identity and content hash.
- Stream the object through bounded buffers; validate file, batch, entry, control, hash/signature, and policy rules.
- Persist redacted evidence with offsets, quarantine invalid files, and release only through an explicit versioned policy.
- Publish state changes through an outbox; update the UI from API/SSE state with replay cursors.
- Keep the original artifact immutable and make every derived/repaired artifact separately addressable.

Verification checkpoint:

- Empty, truncated, duplicate, partially uploaded, tampered, malformed, multi-batch, balanced, and unbalanced fixtures have explicit expected outcomes.
- Redelivery produces no duplicate job, ledger append, or notification.
- A reviewer can trace every UI claim to a source object, rule version, finding, decision, and audit event.

### Phase D — Measured performance and resilience (approximately 1 week)

Goal: make performance claims reproducible.

- Add bounded worker pools, database-backed leases, retry budgets, dead-letter state, cursor pagination, connection pools, and backpressure.
- Benchmark representative 1 MB, 100 MB, and 1 GB inputs with documented hardware and concurrency.
- Measure arrival-to-visible, validation duration, queue wait, API p50/p95/p99, throughput, memory, duplicate rate, and recovery time.
- Test kill/restart, lease expiry, duplicate events, slow storage, database contention, partial uploads, and notification failure.

Verification checkpoint:

- The dashboard is populated only from recorded telemetry.
- Published numbers link to a repeatable benchmark command, dataset, environment, commit, and raw result artifact.

### Phase E — One constrained AI analyst (approximately 1 week)

Goal: add demonstrable AI value without allowing autonomous financial actions.

- Provide a read-only tool surface for incident metadata, redacted findings, historical occurrences, and approved runbooks.
- Require typed JSON output with hypothesis, evidence IDs, missing evidence, confidence rationale, and recommended human action.
- Make citations resolve to stored records/runbook passages and reject nonexistent evidence IDs.
- Add prompt-injection fixtures in file metadata/content, PII leakage checks, hallucinated-citation checks, budget/time limits, and provider-outage behavior.
- Never grant release, repair, payment, connector-write, shell, or arbitrary SQL tools.

Verification checkpoint:

- The agent cannot change system state.
- Unsupported claims and invalid citations fail evaluation.
- Provider unavailability produces a clear degraded state while deterministic processing continues.

### Phase F — Connector expansion (later)

Goal: extend the proven control plane without turning the product into a remote administration platform.

- Add a connector SDK with typed capabilities, health, secret references, egress rules, schema/resource discovery, pagination, rate limits, and audit events.
- First database connector: PostgreSQL metadata and parameterized, administrator-approved read templates—not arbitrary SQL.
- First file connector: SFTP/object-storage event integration—not SSH shell access.
- Implement the edge agent only for private-network customers who require it, with real mTLS, short-lived credentials, signed configuration, upgrade policy, and least-privilege resource allowlists.
- Display metadata and approved aggregates by default; require explicit policy for raw rows/files.

Verification checkpoint:

- Connector compromise cannot cross tenants or access undeclared destinations.
- Revoking a credential or certificate stops access promptly and is visible in audit evidence.

## Version-one scope: keep, defer, remove

| Keep and finish | Defer until the core is proven | Remove from current public build |
|---|---|---|
| Authenticated NACHA upload/finalized arrival | Read-only PostgreSQL connector | Mock Integration Hub |
| Immutable object and content hash | Edge agent with real mTLS | Instant-payment settlement simulator |
| Expectation occurrence and deterministic deadline state | ISO 20022/BAI2/SWIFT production claims | Vault UI with fake secret references |
| Streaming deterministic validation | Predictive SLA risk | Arbitrary SQL console |
| Quarantine/release policy | Human-approved repair proposals | Agent swarm scripted transcript |
| Idempotent durable jobs | External hash checkpoints | Self-healing apply endpoint |
| Redacted evidence and audit chain | Advanced compliance exports | Chaos monkey and fake failover |
| Server-backed operations UI | Additional databases/APIs/shared paths | Hardcoded benchmark/SLA metrics |
| One read-only evidence-grounded analyst | Executive reporting | Fabricated perfect eval fallback |

## Recruiter-ready definition of done

The project is ready to promote when a reviewer can clone it and verify all of the following:

1. One documented command builds and starts the system from source.
2. Authentication is mandatory and two test tenants cannot access each other’s objects, jobs, findings, or events.
3. A real NACHA fixture arrives through the supported ingress and is stored immutably.
4. An empty, malformed, duplicate, truncated, or tampered file cannot be released.
5. Reprocessing or duplicate delivery is idempotent.
6. Every UI state traces to database/object-store evidence; dependency failures never become synthetic success.
7. A restart during processing recovers the job exactly once or safely retries it.
8. Performance numbers are reproduced by a checked-in benchmark harness and include hardware, dataset, concurrency, and commit metadata.
9. The AI analyst is read-only, cites resolvable evidence, has no release/repair authority, and degrades cleanly when unavailable.
10. CI independently builds containers, runs security/correctness/integration tests, and publishes results.
11. The README clearly separates implemented, experimental, and planned capabilities.
12. A five-minute demo tells one coherent story: expected file → anomaly/quarantine → evidence → human decision → immutable audit trail.

## Recommended portfolio narrative

Use this framing:

> Sentinel Flow is a production-shaped pre-ledger reliability gateway. It ingests finalized financial files, validates them deterministically with an idempotent streaming pipeline, quarantines unsafe inputs, and gives operations teams a traceable evidence path from source artifact to release decision. A read-only AI analyst summarizes incidents using cited system evidence but cannot alter financial state.

Avoid claiming autonomous remediation, bank-grade compliance, payment settlement, verified mTLS, multi-format production support, or measured high throughput until the corresponding controls and evidence exist.

## Immediate next actions

1. Freeze feature additions.
2. Complete Phase A and make the README honest.
3. Repair the build and secure defaults.
4. Implement the single NACHA vertical slice.
5. Add failure, concurrency, and idempotency tests before performance tuning.
6. Add one constrained analyst only after the evidence model is trustworthy.
7. Add database/API/private-network connectors only after the connector threat model and tenant boundaries pass review.

That sequence produces a smaller project with substantially stronger engineering signal than the current broad prototype.
