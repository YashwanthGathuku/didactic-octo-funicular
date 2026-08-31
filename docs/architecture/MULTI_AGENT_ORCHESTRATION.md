# SentinelFlow Phase P06.5 — Durable ADK Multi-Agent Orchestration Gate

**Status**: IMPLEMENTED / TESTED  
**Authoritative Architectural Specification**: SGACA (SentinelFlow Governed Agentic Control Architecture)  
**Classification**: Zero-Trust Pre-Ledger AI Multi-Agent Control Plane  

---

## 1. Executive Summary & Core Architectural Invariants

Phase P06.5 establishes SentinelFlow's durable, crash-resilient multi-agent control plane. It integrates real **Google Agent Development Kit (ADK)** runtime primitives (`Agent`, `ParallelAgent`, `InMemoryRunner`) with **SQLite-backed durable persistence**, crash-consistent event journaling, protected state binding caches, and fail-closed TOCTOU protection.

```
                                      +------------------------------------------+
                                      |         Trigger Event (Normalized)       |
                                      |   (ARTIFACT_QUARANTINED, SLA_AT_RISK)    |
                                      +------------------------------------------+
                                                           |
                                                           v
                                      +------------------------------------------+
                                      |     Durable Idempotency & Workflow State |
                                      |     (agent_workflows & event journal)    |
                                      +------------------------------------------+
                                                           |
                                                           v
                                      +------------------------------------------+
                                      |      IncidentCommanderAgent (Phase 1)    |
                                      |       -> Creates CommanderPlan           |
                                      +------------------------------------------+
                                                           |
                                                           v
                                      +------------------------------------------+
                                      |           Plan Validator                 |
                                      |    (Anti-Hallucination & Fixed Roster)   |
                                      +------------------------------------------+
                                                           |
                                  +------------------------+------------------------+
                                  | (ADK ParallelAgent Dispatch)                    | (ADK ParallelAgent Dispatch)
                                  v                                                 v
             +------------------------------------------+      +------------------------------------------+
             |           DiagnosisAgent (A1)            |      |           PolicySLAAgent (A1)            |
             |   • Root Cause & Findings Analysis       |      |   • Policy Engine Interpretation         |
             |   • Read-Only Tool Gateway Queries       |      |   • Deterministic SLA Computation        |
             +------------------------------------------+      +------------------------------------------+
                                  |                                                 |
                                  | (output_key="diagnosis_result")                 | (output_key="policy_sla_result")
                                  +------------------------+------------------------+
                                                           |
                                                           v
                                      +------------------------------------------+
                                      |      TOCTOU & Protected Bindings Gate    |
                                      |      • PolicyBundleHash check (fail-close)|
                                      |      • ArtifactHash check (fail-closed)  |
                                      +------------------------------------------+
                                                           |
                                                           v
                                      +------------------------------------------+
                                      |      IncidentCommanderAgent (Phase 2)    |
                                      |       -> Synthesizes Authoritative Plan  |
                                      +------------------------------------------+
                                                           |
                                                           v
                                      +------------------------------------------+
                                      |       Authoritative Workflow Outbox      |
                                      |       • READY_FOR_REMEDIATION            |
                                      |       • HUMAN_AUTHORIZATION_REQUIRED     |
                                      |       • POLICY_BLOCKED                   |
                                      |       • PARTIAL_SPECIALIST_FAILURE       |
                                      |       • UNRESOLVED                       |
                                      +------------------------------------------+
```

---

## 2. Real Google ADK Runtime Primitives & Introspection

All SentinelFlow agents instantiate and execute through authentic Google ADK runtime classes:

| Agent / Workflow Component | ADK Runtime Object | Model & Execution Mode | Output Key / Role |
| :--- | :--- | :--- | :--- |
| **`IncidentCommanderAgent`** | `google.adk.agents.Agent` (`LlmAgent`) | `gemini-3.5-flash` + `InMemoryRunner` | `output_key="commander_plan"` |
| **`DiagnosisAgent`** | `google.adk.agents.Agent` (`LlmAgent`) | `gemini-3.5-flash` + `InMemoryRunner` | `output_key="diagnosis_result"` |
| **`PolicySLAAgent`** | `google.adk.agents.Agent` (`LlmAgent`) | `gemini-3.5-flash` + `InMemoryRunner` | `output_key="policy_sla_result"` |
| **`VerifierAgent`** | `google.adk.agents.Agent` (`LlmAgent`) | `gemini-3.5-flash` + `InMemoryRunner` | `output_key="verification_result"` (Advisory Critic) |
| **`ParallelSpecialists`** | `google.adk.agents.ParallelAgent` | `sub_agents=[DiagnosisAgent, PolicySLAAgent]` | Disjoint output keys prevent state collision |

Introspection tests in [`ai-tier/tests/test_adk_introspection.py`](file:///c:/Users/Gathu/Projects/fintech/ai-tier/tests/test_adk_introspection.py) verify class membership, runner wrapping, and distinct output key bindings.

---

## 3. Durable Persistence & Crash Recovery

Workflow state and event history are persisted in SQLite storage ([`ai-tier/persistence/store.py`](file:///c:/Users/Gathu/Projects/fintech/ai-tier/persistence/store.py)), mirroring the Go Gateway's `016_agent_workflow_state.sql` schema:

1. **Durable Trigger Idempotency**:
   - Workflows are uniquely keyed by `(tenant_id, trigger_event_id, workflow_type)`. Duplicate event delivery after process restart resolves to the existing workflow record with zero duplicate execution.
2. **Protected State Binding Hash Verification**:
   Specialist results are cached and reused across restarts only when all 7 protected state bindings match:
   - `workflow_id`
   - `agent_name`
   - `agent_version` / `manifest_hash`
   - `input_context_hash`
   - `artifact_sha256`
   - `policy_bundle_hash`
   - `authorized_evidence_set_hash`
   - `tool_manifest_hash`
   If any protected binding changes, the cached entry is marked `STALE` and re-evaluated.
3. **Crash-Consistent Domain Event Journal**:
   Records discrete state machine events: `WORKFLOW_CREATED`, `COMMANDER_PLAN_ACCEPTED`, `SPECIALIST_STARTED`, `SPECIALIST_COMPLETED`, `SPECIALIST_FAILED`, `POLICY_CONTEXT_STALE`, `RESOURCE_CONTEXT_STALE`, `SYNTHESIS_COMPLETED`.

---

## 4. TOCTOU Invariants & Policy Authority

1. **Policy-Bundle TOCTOU Fail-Closed Invariant**:
   $$\text{PolicyBundleHash}(\text{plan}) \neq \text{PolicyBundleHash}(\text{current}) \implies \text{OldPlanNotActionable}$$
   If the governing policy bundle changes between planning and synthesis, the workflow records `POLICY_CONTEXT_STALE`, invalidates the plan, and fails closed with `outcome = "UNRESOLVED"`.
2. **Artifact Mutation TOCTOU Fail-Closed Invariant**:
   $$\text{ArtifactHash}(\text{plan}) \neq \text{ArtifactHash}(\text{current}) \implies \text{OldPlanNotActionable}$$
   If the quarantined artifact SHA-256 changes during execution, the workflow records `RESOURCE_CONTEXT_STALE` and fails closed with `outcome = "UNRESOLVED"`.
3. **Deterministic Policy Authority & Distinct Outcomes**:
   - $\text{PolicyDecision} == \text{DENY} \implies \text{outcome} = \text{"POLICY_BLOCKED"}$ (human attention attached for review only; human click cannot relax a safety DENY).
   - $\text{PolicyDecision} == \text{REQUIRE_HUMAN} \implies \text{outcome} = \text{"HUMAN_AUTHORIZATION_REQUIRED"}$.
   - $\text{PolicyDecision} == \text{ALLOW} \land \text{Eligible} \implies \text{outcome} = \text{"READY_FOR_REMEDIATION"}$.
4. **Deterministic SLA Timetable Computation**:
   `time_remaining_seconds` is computed deterministically from authoritative cutoff and evaluation timestamps rather than generated by LLM reasoning.

---

## 5. Precise Product Integrity & Decoupling Guarantees

- **Deterministic Pipeline Decoupling**: SentinelFlow deterministic NACHA ingestion, validation, quarantine, review, and evidence paths remain operational when the AI tier is unavailable (`AGENT_UNAVAILABLE`).
- **Autonomy Level A1**: Zero mutating or file release tools are exposed to the AI fleet. All actions operate strictly in a read-only investigation and planning capacity.

---

## 6. Language and Authority Boundary & Failure-Removal Invariant

### Authority Matrix by Language

| Architectural Responsibility | Authority Owner | Implementation Location | Invariant / Enforcement Mechanism |
| :--- | :--- | :--- | :--- |
| **Authoritative Workflow IDs & Creation** | **Go Control Plane** | [`gateway/agent_workflow_service.go`](file:///c:/Users/Gathu/Projects/fintech/gateway/agent_workflow_service.go) | Allocated strictly in Go; Python cannot invent IDs |
| **Durable Trigger Idempotency** | **Go Control Plane** | [`gateway/migrations/019_agent_workflow_trigger_idempotency.sql`](file:///c:/Users/Gathu/Projects/fintech/gateway/migrations/019_agent_workflow_trigger_idempotency.sql) | `UNIQUE(tenant_id, trigger_event_id, workflow_type)` |
| **Workflow State Machine & `row_version`** | **Go Control Plane** | [`gateway/internal/domain/agent_workflow.go`](file:///c:/Users/Gathu/Projects/fintech/gateway/internal/domain/agent_workflow.go) | Optimistic locking + transactional transitions |
| **Authoritative Event Journal** | **Go Control Plane** | `agent_workflow_events` (Go) | Append-only crash-consistent transaction journal |
| **7 Protected Binding Hash Freshness** | **Go Control Plane** | [`gateway/agent_workflow_service.go`](file:///c:/Users/Gathu/Projects/fintech/gateway/agent_workflow_service.go) | Go verifies hashes; Python cannot force stale reuse |
| **TOCTOU Policy & Resource Freshness** | **Go Control Plane** | [`gateway/agent_orchestrator.go`](file:///c:/Users/Gathu/Projects/fintech/gateway/agent_orchestrator.go) | `CheckTOCTOU` in Go fails closed to `UNRESOLVED` |
| **Deterministic Policy Engine Verdicts** | **Go Control Plane** | [`gateway/internal/policy/`](file:///c:/Users/Gathu/Projects/fintech/gateway/internal/policy/) | DENY -> POLICY_BLOCKED; REQUIRE_HUMAN -> HUMAN_REVIEW |
| **Tool Execution & Authorization** | **Go Control Plane** | [`gateway/internal/toolgateway/`](file:///c:/Users/Gathu/Projects/fintech/gateway/internal/toolgateway/) | 12-term access conjunction + server-injected scope |
| **Independent Verification & Dominance** | **Go Control Plane** | [`gateway/internal/verification/`](file:///c:/Users/Gathu/Projects/fintech/gateway/internal/verification/) | 12 deterministic checks dominate ADK critic opinions |
| **Critic Opinion Reasoning** | **Python Runtime** | [`ai-tier/agents/verifier.py`](file:///c:/Users/Gathu/Projects/fintech/ai-tier/agents/verifier.py) | Autonomy A1 read-only advisory critic review |
| **AI Reasoning & ADK Execution** | **Python Runtime** | [`ai-tier/agents/`](file:///c:/Users/Gathu/Projects/fintech/ai-tier/agents/) | Google ADK `Agent` & `ParallelAgent` execution |
| **Ephemeral Session Cache** | **Python Runtime** | [`ai-tier/persistence/store.py`](file:///c:/Users/Gathu/Projects/fintech/ai-tier/persistence/store.py) | Non-authoritative testing & ephemeral session cache |

### Formal Failure-Removal Invariant
$$\text{Remove}(\text{ai-tier}) \implies \text{SentinelFlow durable control plane remains internally consistent}.$$

Deleting or disabling the Python runtime leaves all Go workflow records, audit trails, and state machines intact. Python never directly opens SentinelFlow's authoritative database.

---

## 7. Integration with Independent Verification (Phase P08)

Orchestrated workflows culminating in remediation candidate generation hand off directly to **Phase P08 Independent Verification**:
- The ADK `VerifierAgent` acts as an independent second pair of eyes evaluating candidate findings without sharing state with the `RemediationAgent`.
- All critic verdicts are governed by the **Deterministic Dominance Equation** ($\text{DeterministicVerification} \succ \text{CriticOpinion}$).
- See [`docs/architecture/INDEPENDENT_VERIFICATION.md`](file:///c:/Users/Gathu/Projects/fintech/docs/architecture/INDEPENDENT_VERIFICATION.md) for full architectural specifications.


