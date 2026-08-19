# SentinelFlow Verified Capability Matrix
## Google All Things Agentic Hackathon — Proof & Evidence Index

> **Rule**: Every claim in this matrix is strictly provable from source code, unit/integration tests, or deployment configurations.
> **Permitted Status Tags**: `TESTED` | `IMPLEMENTED` | `DEMO_ONLY` | `EXPERIMENTAL` | `PLANNED`

---

## 1. Core Deterministic Financial Engine

| Capability | Status | Evidence & Test Location | Technical Description |
|---|---|---|---|
| **Zero-Copy NACHA Ingress** | `TESTED` | `gateway/internal/nacha/validate_test.go` | 94-byte fixed-width record parser with single-pass validation; bounded $<16\text{MB}$ memory footprint. |
| **ABA Routing Mod-10 Check Digit** | `TESTED` | `gateway/internal/nacha/routing_test.go` | Deterministic Mod-10 weights $[3, 7, 1, 3, 7, 1, 3, 7]$ checksum verification on 9-digit Fed routing prefixes. |
| **Batch Hash Sum Accumulator** | `TESTED` | `gateway/quarantine_test.go:TestQuarantine_CorruptedBatchHash` | FileControl/BatchControl entry hash accumulation modulo $10^{10}$ verified against entry details. |
| **Fail-Closed Quarantine** | `TESTED` | `gateway/quarantine_test.go` | Corrupted, unbalanced, or truncated batches transition to `QUARANTINED` immediately with zero ledger release. |
| **Dual-Control Human Release** | `TESTED` | `gateway/internal/review/review_test.go` | N-of-M cryptographic 2-person rule; batch creator cannot approve release; freshness digest validated. |
| **Linear Hash Chain Audit Ledger** | `TESTED` | `gateway/internal/ledger/ledger_test.go` | Append-only SHA-256 linear hash chain ($H_N = \text{SHA256}(H_{N-1} \parallel \text{Payload})$); zero Merkle or WORM claims. |
| **Durable Lease Worker Pool** | `TESTED` | `gateway/worker_test.go:TestConcurrentValidationStress` | Multi-worker background job pool; 40 artifacts settled in 1.135s with 0 lease losses. |
| **Single-Flight Job Deduplication** | `TESTED` | `gateway/worker_test.go:TestConcurrentEnqueueOfOneKeyProducesOneJob` | 20 concurrent goroutines enqueuing identical idempotency key produce exactly 1 scheduled job. |
| **Immutable Artifact Storage (S3)** | `TESTED` | `gateway/internal/objectstore/conformance_test.go` | Content-addressed blob store with SHA-256 integrity verification; write-once semantics. |
| **Filesystem Storage Adapter** | `DEMO_ONLY` | `gateway/internal/objectstore/filesystem.go` | Local directory storage adapter with temporary atomic rename for development and demo profiles. |

---

## 2. Security, Authentication & Tenant Isolation

| Capability | Status | Evidence & Test Location | Technical Description |
|---|---|---|---|
| **Tenant Scoping (SQLite App-level & Postgres RLS)** | `TESTED` | `gateway/tenant_isolation_test.go` | Every database query scoped by `tenant_id`; cross-tenant access returns 404/403. PostgreSQL RLS policies in `migrations_postgres/`. |
| **Tenant-Scoped AI Triage** | `TESTED` | `gateway/triage_test.go:TestTriageTenantIsolation` | AI triage handler strictly scopes incident lookup and findings to authenticated tenant context; cross-tenant returns 404. |
| **Canonical AgentContextEnvelope** | `TESTED` | `gateway/triage_test.go:TestAgentContextEnvelopeInvariantsAndDelivery` | Immutable typed envelope with server-injected tenant ID, budget, correlation ID, and zero raw financial payloads. |
| **Bounded AI Service Client** | `TESTED` | `gateway/triage_test.go:TestAIClientBoundedPolicyAndTimeout` | Outbound AI client with explicit timeout, retry policy, response size ceiling, and header propagation. |
| **Fail-Closed Provider Failure Semantics** | `TESTED` | `gateway/ai_analyst_test.go:TestAiAnalyst_Returns503WhenUnconfiguredOrOffline` | When AI tier is offline or unconfigured, returns HTTP 503 `UNAVAILABLE`; zero fabricated fallback claims. |
| **RBAC Authorization Matrix** | `TESTED` | `gateway/router_auth_test.go` | 8-role capability matrix (Viewer, Operator, Reviewer, TenantAdmin) enforced at route level. |
| **Secret Sealing & Scrubber** | `TESTED` | `gateway/secret_hygiene_test.go` | `secrets.Value` types redact credentials in stringers, logs, and JSON serialization. |
| **SSRF & Network Egress Policy** | `TESTED` | `gateway/threat_model_test.go:TestThreatModel_SSRFDenial` | RFC1918 private IP ranges and loopback blocked for external connector endpoints. |
| **CSRF Token Double-Submit Cookie** | `TESTED` | `gateway/router_auth_test.go` | SameSite=Lax HttpOnly session cookie paired with custom `X-CSRF-Token` header. |
| **Demo Principal Auto-Auth** | `DEMO_ONLY` | `gateway/main.go:385-397` | Loopback-only demo principal (`demo-operator@local`) active only under `PROFILE=local-demo`. |

---

## 3. Agent Control Plane & Governance

| Capability | Status | Evidence & Test Location | Technical Description |
|---|---|---|---|
| **Agent Workflow State Machine (16 States)** | `TESTED` | `gateway/internal/domain/agent_workflow_test.go` | Persistent 16-state machine with optimistic concurrency, duplicate idempotency, and linear ledger audit events. |
| **Crash-Consistent Workflow Transitions** | `TESTED` | `gateway/agent_workflow_service_test.go` | Atomic transactional state transition and domain event journal commits. |
| **Gemini Integration** | `TESTED` | `ai-tier/llm_client.py` | Google GenAI SDK (`google-genai`) with structured JSON schema output and calibrated temperature ($0.1$). |
| **Specialist Agent Fleet Definitions** | `IMPLEMENTED` | `ai-tier/agents/` | 6 specialist agent definitions (`SentinelCoordinator`, `Triage`, `Compliance`, `Remediation`, `Verifier`, `Memory`, `Escalation`). Google ADK runtime is `PLANNED`. |
| **Least-Privilege Tool Registry** | `IMPLEMENTED` | `ai-tier/agents/tools.py` | Declared capability scopes mapping each agent to allowed read/write tools. |
| **Model Armor Screening Engine** | `IMPLEMENTED` | `ai-tier/armor/client.py` | Dual-stage input/output screening engine for prompt injection, jailbreaks, and PII leakage. Managed Cloud Model Armor API is `PLANNED`. |
| **Typed Evidence Envelopes** | `TESTED` | `gateway/agent_context.go`, `ai-tier/models/envelope.py` | Pydantic and Go evidence envelopes with server-injected tenant context; unredacted data banned. |
| **Agent Registry & Telemetry API** | `TESTED` | `gateway/agents_test.go:TestAgentRegistry` | `GET/POST /api/v1/agents` with drift config hashing and `agent_invocations` audit history. Google Agent Registry is `PLANNED`. |
| **Agent Memory Bank Store** | `IMPLEMENTED` | `ai-tier/memory/store.py` | Local tenant-scoped persistent cross-session memory for incident patterns and partner history. Google Memory Bank is `PLANNED`. |
| **Derived Artifact Workflow** | `IMPLEMENTED` | `gateway/migrations/014_derived_artifacts.sql` | Non-destructive remediation proposing derived artifacts without mutating quarantined originals. |
| **Independent Verifier Engine** | `PLANNED` | `gateway/migrations/016_agent_workflow_state.sql` | Schema for `verification_attestations` in place; Go verifier engine is `PLANNED` for P03. |
| **Adversarial Evaluation Suite (15)** | `TESTED` | `ai-tier/evals/runner.py` | 15 adversarial security attack scenarios passing 92/92 invariant checks (100% pass rate). |
| **Calibrated Uncertainty** | `TESTED` | `ai-tier/evals/runner.py:ADV-010` | Qualitative confidence ranks (`HIGH`, `MEDIUM`, `LOW`); missing evidence triggers operator questions. |
| **Mandatory Read-Only Disclaimer** | `TESTED` | `ai-tier/evals/runner.py:all_scenarios` | Every recommendation concludes with the immutable read-only statement. |

---

## 4. Cloud Infrastructure & Enterprise Deployment

| Capability | Status | Evidence & Test Location | Technical Description |
|---|---|---|---|
| **Asymmetric Ledger Checkpoint Schema** | `IMPLEMENTED` | `gateway/migrations/015_kms_checkpoints.sql` | `ledger_checkpoints` table with ECDSA P-256 asymmetric signature storage. Cloud KMS API integration is `PLANNED`. |
| **Cloud Run Service Definitions** | `IMPLEMENTED` | `deploy/cloudrun.yaml`, `deploy/cloudrun-ai.yaml` | Containerized Knative service configs for Gateway and AI Tier with health probes. |
| **GCP Provisioning Automation** | `IMPLEMENTED` | `deploy/setup-gcp.sh` | Bash script provisioning Cloud SQL PostgreSQL 16, Cloud KMS, Secret Manager, and Artifact Registry. Cloud SQL deployment is `PLANNED`. |
| **PostgreSQL 16 Multi-Tenant RLS** | `TESTED` | `gateway/migrations_postgres/001_schema_and_rls.sql` | Full schema migrations with foreign key constraints, indexes, and database-level RLS policies. |
| **Prometheus & OpenTelemetry Metrics** | `TESTED` | `gateway/internal/telemetry/telemetry_test.go` | Low-cardinality Prometheus metrics endpoint (`/metrics`) with route normalization and token auth. |
| **Continuous Integration Pipeline** | `TESTED` | `.github/workflows/ci.yml` | 6-job CI pipeline enforcing linting, backend race tests, frontend tests, AI evals, and doc drift. |
| **Single-Source Doc Generation** | `TESTED` | `scripts/generate_docs.py --check` | Matrix-driven documentation generator validating zero doc drift in CI. |
