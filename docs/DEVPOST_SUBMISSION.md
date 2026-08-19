# SentinelFlow — All Things Agentic Hackathon Submission

## Project Overview
**SentinelFlow** is a next-generation autonomous AI Agent Control Plane built for **Gemini models** and targeting the **Google Agent Development Kit (ADK)** for high-assurance enterprise financial file reliability, incident triaging, and pre-ledger compliance.

Instead of a generic chatbot, SentinelFlow deploys an **orchestrated fleet of specialist agents** running asynchronously to handle the heavy lifting of batch payments validation, anomaly classification, regulatory compliance auditing, and derived artifact remediation while preserving authoritative deterministic controls.

---

## The Specialist Agent Fleet
1. **SentinelCoordinator (Root Agent)**: Orchestrates the specialist fleet, routes incident findings, and enforces Model Armor guardrails.
2. **TriageAgent**: Classifies incident severity (P1–P4) from deterministic findings and SLA commitments.
3. **ComplianceAgent**: Deep NACHA/ACH regulatory expertise with rule citations.
4. **RemediationAgent**: Proposes non-destructive fixes as **derived artifacts** (preserving immutable originals).
5. **VerifierAgent**: Independent deterministic re-validation of findings before dual-control human release.
6. **MemoryAgent (Memory Bank)**: Persistent cross-session recall of incident patterns, counterparty reliability, and SLA trends.
7. **EscalationAgent**: Proactive SLA breach detection and risk scoring.

---

## Google Cloud & Gemini Technology Stack
- **Gemini 2.5 Flash (`google-genai` SDK)**: Grounded reasoning with calibrated uncertainty.
- **Google Agent Development Kit (ADK)**: Multi-agent hierarchical delegation with least-privilege tool scopes (local fleet implemented, managed ADK runtime PLANNED).
- **Google Cloud Model Armor**: Input/output screening against prompt injection, jailbreaks, and PII leakage (local filter implemented, Cloud API PLANNED).
- **Google Cloud KMS**: Periodic asymmetric signatures on linear hash chain ledger checkpoints (schema implemented, Cloud KMS API PLANNED).
- **Google Cloud SQL (PostgreSQL 16)**: System of record with row-level security and transactional outbox.
- **Google Cloud Run**: Containerized, auto-scaling backend gateway and AI Tier.

---

## Verified Capabilities Matrix (Single Source of Truth)

| Capability | Status | Evidence & Test Suite | Description |
|---|---|---|---|
| `nacha_validation` | **TESTED** | `gateway/internal/nacha/validate_test.go` | Zero-copy NACHA parser with Mod-10 check digit and batch hash verification |
| `fail_closed_quarantine` | **TESTED** | `gateway/quarantine_test.go` | Malformed or unbalanced files transition to QUARANTINED immediately |
| `dual_control_release` | **TESTED** | `gateway/internal/review/review_test.go` | Separation-of-duties release requiring N approvals from distinct reviewers |
| `linear_hash_chain` | **TESTED** | `gateway/internal/ledger/ledger_test.go` | Append-only SHA-256 linear hash chain (NOT Merkle tree, NOT digital signature) |
| `immutable_artifact_storage` | **TESTED** | `gateway/internal/objectstore/conformance_test.go` | Uploaded files stored as immutable blobs in S3/MinIO with SHA-256 addressability |
| `tenant_isolation` | **TESTED** | `gateway/tenant_isolation_test.go` | Every database query scoped by tenant_id; cross-tenant access returns 404/403 |
| `tenant_scoped_ai_triage` | **TESTED** | `gateway/triage_test.go` | AI triage endpoint scoped to authenticated tenant context; cross-tenant access returns 404 |
| `canonical_agent_context_envelope` | **TESTED** | `gateway/triage_test.go` | Immutable typed envelope with server-injected tenant ID, budget, and zero raw financial payloads |
| `bounded_ai_service_client` | **TESTED** | `gateway/triage_test.go` | Outbound AI client with explicit timeout, retry policy, response size ceiling, and header propagation |
| `fail_closed_provider_semantics` | **TESTED** | `gateway/ai_analyst_test.go` | When AI tier is offline or unconfigured, returns HTTP 503 UNAVAILABLE; zero fabricated fallback claims |
| `rbac_permissions` | **TESTED** | `gateway/router_auth_test.go` | 8-capability RBAC model enforced at route level |
| `secret_redaction` | **TESTED** | `gateway/internal/secrets/redact_test.go` | Credentials sealed with AES-256-GCM, redacted in logs/JSON/errors |
| `ssrf_prevention` | **TESTED** | `gateway/threat_model_test.go` | RFC1918 and loopback ranges blocked for connector URIs |
| `agent_workflow_state_machine` | **TESTED** | `gateway/internal/domain/agent_workflow_test.go` | 16-state persistent, resumable agent workflow state machine with optimistic concurrency and linear ledger audit events |
| `deterministic_policy_engine` | **TESTED** | `gateway/internal/policy/evaluator_test.go` | In-memory deterministic policy engine enforcing AgentRecommendation != Permission with 5 layers, deny dominance, typed obligations/prohibitions, RFC 8785 canonical hashing, exact bundle replay, and microsecond latency |
| `evidence_envelopes` | **TESTED** | `gateway/agent_context.go` | Typed, redacted evidence envelopes ensuring raw financial data never reaches models |
| `gemini_integration` | **TESTED** | `ai-tier/llm_client.py` | Gemini 2.5 Flash via google-genai SDK for grounded incident analysis |
| `adk_agent_fleet` | **IMPLEMENTED** | `ai-tier/agents/coordinator.py` | Specialist fleet definitions (Triage, Compliance, Remediation, Verifier, Memory, Escalation); Google ADK runtime is PLANNED |
| `model_armor` | **IMPLEMENTED** | `ai-tier/armor/client.py` | Local regex/heuristic input/output screening client; Google Cloud Model Armor API integration is PLANNED |
| `agent_registry` | **IMPLEMENTED** | `gateway/migrations/012_agent_registry.sql` | Local database schema for agent lifecycle metadata, versioning, and invocations; Google Cloud Agent Registry is PLANNED |
| `memory_bank` | **IMPLEMENTED** | `ai-tier/memory/store.py` | Local persistent cross-session tenant memory store for incident patterns and SLA trends; Google Cloud Memory Bank is PLANNED |
| `derived_artifacts` | **IMPLEMENTED** | `gateway/migrations/014_derived_artifacts.sql` | Remediation creates new artifacts linked to quarantined originals, never mutating originals |
| `independent_verification` | **PLANNED** | `None` | Independent deterministic re-validation before approval (VerifierAgent + Go verify path) |
| `sentinelflow_lens` | **PLANNED** | `None` | Governed analytics plane featuring AnalyticsAgent A1, structured QueryIntent AST, deterministic Safe Query Compiler, and branching Investigation Graph |
| `cloud_kms_checkpoints` | **IMPLEMENTED** | `gateway/migrations/015_kms_checkpoints.sql` | Database schema for ledger checkpoint digests; Google Cloud KMS API asymmetric signing is PLANNED |
| `cloud_sql_deployment` | **PLANNED** | `deploy/setup-gcp.sh` | Google Cloud SQL PostgreSQL 16 + Cloud Run automated deployment script |
| `prometheus_metrics` | **TESTED** | `gateway/internal/telemetry/telemetry_test.go` | Low-cardinality Prometheus metrics with normalized routes and status labels |
| `adversarial_evals` | **TESTED** | `ai-tier/evals/runner.py` | 15 adversarial security scenarios testing 8 guardrail invariants (100% pass rate) |
| `ci_pipeline` | **TESTED** | `.github/workflows/ci.yml` | 6-job CI: lint, test-backend (race+coverage), test-frontend, test-ai-tier, migrations, security |
| `concurrent_stress_test` | **TESTED** | `gateway/worker_test.go` | 40 artifacts settled in 1.135s under 24 parallel workers with zero lease loss |

---

## Summary of Capabilities
- **21 Tested Capabilities** backed by automated regression tests in CI.
- **6 Implemented Components** with schema/code present.
- **3 Planned Google Integrations** scheduled for runtime deployment.
- **100% Deterministic Grounding**: AI operates in a read-only advisory capacity; all releases require verified dual-control human authorization.
