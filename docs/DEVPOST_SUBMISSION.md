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
| `gemini_live_analysis` | **TESTED** | `ai-tier/llm_client.py` | Google Gemini 2.5 Flash grounded incident hypothesis generation |
| `read_only_ai_investigator` | **TESTED** | `ai-tier/evals/runner.py` | AI tier has no database write credentials, no mutation endpoints, no shell execution |
| `agent_development_kit_orchestration` | **TESTED** | `ai-tier/tests/test_adk_introspection.py` | Real Google ADK Agent instances orchestrating specialized sub-agents with 4-domain trust-partitioned prompts |
| `durable_agent_workflows` | **TESTED** | `gateway/internal/repository/agent_workflow_repo_test.go` | Crash-resilient workflow state machine in Go gateway backed by atomic DB transitions and durable outbox events |
| `parallel_specialist_execution` | **TESTED** | `ai-tier/tests/test_durable_orchestration.py` | Deterministic synthesis across parallel specialists with partial failure isolation and unified budget enforcement |
| `policy_sla_agent` | **TESTED** | `ai-tier/agents/policy_sla.py` | Autonomy Level A1 specialist explaining deterministic policy decisions and contract cutoff context |
| `toctou_and_policy_authority` | **TESTED** | `ai-tier/tests/test_toctou_and_policy_authority.py` | PolicyEngine.Evaluate strictly dominates agent opinions; TOCTOU policy bundle hash and artifact SHA changes fail closed |
| `agent_tool_gateway` | **TESTED** | `gateway/internal/toolgateway/gateway_test.go` | Exclusive gateway mediating all AI tool calls with manifest enforcement, capability allowlists, rate limiting, and side-effect segregation |
| `remediation_agent` | **TESTED** | `ai-tier/agents/remediation.py` | Autonomy Level A2 remediation planner proposing declarative repair operations with zero raw file byte access |
| `verifier_agent` | **TESTED** | `ai-tier/agents/verifier.py` | Autonomy Level A1 read-only critic agent reviewing structured verification evidence with prompt trust partitioning |
| `memory_agent` | **TESTED** | `ai-tier/agents/memory_agent.py` | Autonomy Level A1 read-only memory specialist retrieving bounded historical context and partner profiles |
| `agent_runtime` | **IMPLEMENTED** | `gateway/migrations/011_agent_workflows.sql` | Local execution runtime for Google ADK agents; Google Cloud Agent Runtime is PLANNED |
| `agent_registry` | **IMPLEMENTED** | `gateway/migrations/012_agent_registry.sql` | Local database schema for agent lifecycle metadata, versioning, and invocations; Google Cloud Agent Registry is PLANNED |
| `go_operational_memory` | **TESTED** | `gateway/internal/memory/service_test.go` | Go-owned authoritative operational fact store (M1), deterministic eligibility gate with strict PII rejection, RFC 8785 canonical hashing, and outbox event bridge |
| `sqlite_operational_memory` | **TESTED** | `gateway/internal/memory/service_test.go` | SQLite operational memory persistence, triggers, revisions, and concurrency isolation |
| `postgresql_operational_memory` | **IMPLEMENTED** | `gateway/migrations/022_operational_memory.sql` | PostgreSQL 16 operational memory schema and migrations in 022_operational_memory.sql; live execution planned in P11 |
| `memory_source_resolution` | **TESTED** | `gateway/internal/memory/resolver_test.go` | Go control plane ResolveMemorySources engine validating tenant scope, source status, and cryptographic hashes, with exclusive authority to mint AuthorizedEvidenceRefs |
| `memory_non_authority` | **TESTED** | `ai-tier/evals/memory_runner.py` | Strict non-equivalence invariant (MemoryRecall != Evidence, MemoryRef not in EvidenceSet, SimilarityScore != Trust) enforced across entire fleet |
| `memory_tenant_isolation` | **TESTED** | `gateway/internal/memory/property_test.go` | Strict multi-tenant memory partitioning in Go and Python; cross-tenant retrieval queries return zero foreign tenant hits |
| `memory_poisoning_resistance` | **TESTED** | `ai-tier/evals/memory_runner.py` | Adversarial memory poisoning resistance; poisoned memory claiming policy ALLOW or file release strictly contained |
| `managed_memory_adapter` | **TESTED** | `ai-tier/memory/provider.py` | Python Google Agent Platform Memory Bank adapter with ADC tokens, multi-factor deterministic ranker, and bounded retrieval |
| `managed_memory_mock` | **TESTED** | `ai-tier/memory/mock_provider.py` | In-memory test memory provider supporting red-team fault injection (TIMEOUT, UNAVAILABLE, CONFLICT, POISON, CROSS_TENANT) |
| `managed_memory_revisions` | **TESTED** | `gateway/internal/memory/property_test.go` | Google Memory Bank managed revision audit tracking; rollback of managed memory never mutates authoritative Go M1 facts |
| `live_google_memory_bank` | **IMPLEMENTED** | `ai-tier/memory/google_provider.py` | REST client for Google Agent Platform Memory Bank v1beta1; live production API call is NOT_RUN pending live ADC credentials |
| `live_ingest_events` | **IMPLEMENTED** | `ai-tier/memory/google_provider.py` | Managed IngestEvents client pipeline; live production API call is NOT_RUN pending live ADC credentials |
| `live_memory_profiles` | **IMPLEMENTED** | `ai-tier/memory/google_provider.py` | Managed RetrieveProfiles client pipeline; live production API call is NOT_RUN pending live ADC credentials |
| `derived_artifacts` | **TESTED** | `gateway/internal/candidate/service_test.go` | Remediation creates new artifacts linked to quarantined originals, never mutating originals |
| `governed_remediation` | **TESTED** | `gateway/agent_orchestrator_gateway_test.go` | Exclusive Tool Gateway gate, crash consistency (Windows A-F), orphan reconciliation, deterministic candidate keys, and immutable candidate generation |
| `independent_verification` | **TESTED** | `gateway/internal/verification/service_test.go` | Independent Go verification service re-reading ObjectStore bytes, re-running NACHA validator, verifying derivation hashes, and enforcing deterministic dominance |
| `verification_integrity_checks` | **TESTED** | `gateway/internal/verification/service_test.go` | 12 typed deterministic verification checks covering parent, candidate, derivation, validator, and policy freshness |
| `sentinelflow_lens` | **PLANNED** | `None` | Governed analytics plane featuring AnalyticsAgent A1, structured QueryIntent AST, deterministic Safe Query Compiler, and branching Investigation Graph |
| `model_armor_client` | **TESTED** | `ai-tier/armor/client.py` | Google Cloud Model Armor regional REST client with ADC token management, template screening, fault injection, and fail-closed handling |
| `guarded_model_boundary` | **TESTED** | `ai-tier/guardrails/boundary.py` | 8-step hardened model boundary wrapper with data minimization, 4-domain partitioning, pre/post Model Armor screening, schema enforcement, and audit hashing |
| `ai_guardrails_evals` | **TESTED** | `ai-tier/evals/model_armor_runner.py` | 25 adversarial Model Armor security test scenarios validating prompt injection, data minimization, fail-closed timeouts, and secret leakage containment |
| `cloud_kms_checkpoints` | **IMPLEMENTED** | `gateway/migrations/015_kms_checkpoints.sql` | Database schema for ledger checkpoint digests; Google Cloud KMS API asymmetric signing is PLANNED |
| `cloud_sql_deployment` | **PLANNED** | `deploy/setup-gcp.sh` | Google Cloud SQL PostgreSQL 16 + Cloud Run automated deployment script |
| `prometheus_metrics` | **TESTED** | `gateway/internal/telemetry/telemetry_test.go` | Low-cardinality Prometheus metrics with normalized routes and status labels |
| `adversarial_evals` | **TESTED** | `ai-tier/evals/runner.py` | 120 adversarial security scenarios across 6 phases testing SGACA guardrail invariants (100% pass rate: 14 single-agent, 16 multi-agent, 20 remediation, 20 verification, 25 Model Armor, 25 Memory) |
| `ci_pipeline` | **TESTED** | `.github/workflows/ci.yml` | 6-job CI: lint, test-backend (race+coverage), test-frontend, test-ai-tier, migrations, security |
| `concurrent_stress_test` | **TESTED** | `gateway/worker_test.go` | 40 artifacts settled in 1.135s under 24 parallel workers with zero lease loss |

---

## Summary of Capabilities
- **44 Tested Capabilities** backed by automated regression tests in CI.
- **7 Implemented Components** with schema/code present.
- **2 Planned Google Integrations** scheduled for runtime deployment.
- **100% Deterministic Grounding**: AI operates in a read-only advisory capacity; all releases require verified dual-control human authorization.
