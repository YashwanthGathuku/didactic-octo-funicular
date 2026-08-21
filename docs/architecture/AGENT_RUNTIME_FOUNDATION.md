# SentinelFlow Phase P05 — Gemini 3.5 + Google ADK Agent Runtime Foundation

**Status**: IMPLEMENTED / TESTED  
**Authoritative Architectural Specification**: SGACA (SentinelFlow Governed Agentic Control Architecture)  
**Classification**: Zero-Trust Pre-Ledger AI Control Plane  

---

## 1. Executive Summary & Core Architectural Invariants

Phase P05 establishes the governed AI runtime foundation for SentinelFlow, implementing the **DiagnosisAgent** powered by **Google Gemini 3.5 Flash** and the **Google Agent Development Kit (ADK)**. 

### Core Architectural Invariants
1. **$\text{AgentRecommendation} \neq \text{Permission}$**: The AI model operates strictly in an advisory capacity (Autonomy Level A1: Investigate / Recommend Only). All system state transitions, file releases, and ledger modifications require cryptographic verification and identity-bound dual-control approval with cryptographic artifact and policy integrity binding.
2. **Strict Tool Gateway Conjunction Gate**: The AI agent never communicates directly with databases, object stores, or clearing networks. Every tool invocation passes through the 12-term conjunction gate in `gateway/internal/toolgateway/` and the deterministic Policy Decision Engine (Phase P03).
3. **Parameter Segregation**: The model supplies *only* business semantic keys (e.g. `incident_id`, `artifact_id`, `workflow_id`). Tenant isolation, caller identity, autonomy level, and policy evaluation context are injected exclusively by the server-side trusted execution envelope.
4. **Authoritative Evidence Grounding ($\text{ReturnedEvidenceRefs} \subseteq \text{AuthorizedEvidenceSet}$)**: Hypotheses and diagnostic conclusions must cite only authorized evidence IDs (`FINDING-*`, `RUNBOOK-*`, `METRIC-*`, `EVID-*`) explicitly present in the execution context or returned by verified Tool Gateway tools. Fabricated citations trigger immediate fail-closed `GROUNDING_VIOLATION`.
5. **Deterministic Decoupling & Failure Independence**: If the Gemini API or AI tier is offline, unconfigured, or timing out (`AGENT_UNAVAILABLE`), pre-ledger NACHA validation, fail-closed quarantine, dual-control release, and hash-chained ledgering continue operating with zero degradation.

---

## 2. End-to-End Control Plane Architecture

```
                                    +------------------------------------------+
                                    |         Ingestion / Incident Trigger     |
                                    +------------------------------------------+
                                                         |
                                                         v
                                    +------------------------------------------+
                                    |     AgentContextEnvelope (Server Side)   |
                                    |  (TenantID, CorrelationID, AllowedTools) |
                                    +------------------------------------------+
                                                         |
                                                         v
+---------------------------------------------------------------------------------------------------------------+
|                                            AI TIER (Google ADK)                                               |
|                                                                                                               |
|  [ Prompt Trust Partitioner ] -----------------------------------------------------------------------------+  |
|    • Domain 1: SYSTEM_POLICY (Immutable directives & mandatory disclaimer)                                 |  |
|    • Domain 2: TRUSTED_CONTEXT (Server-injected metadata & budget)                                         |  |
|    • Domain 3: UNTRUSTED_FINANCIAL_CONTENT (Fenced & pre-redacted findings)                                |  |
|    • Domain 4: TOOL_OUTPUT (Governed tool responses)                                                       |  |
|                                                                                                            |  |
|  [ DiagnosisAgent (Gemini 3.5 Flash) ] <-------------------------------------------------------------------+  |
|    |                                                                                                          |
|    +---> Tool Call Request: {"incident_id": "101"} (Business Parameters Only)                                |
|    |        |                                                                                                 |
|    |        v                                                                                                 |
|    |     [ SentinelToolAdapter & ToolGatewayClient ]                                                          |
|    |        |                                                                                                 |
+----+--------+-------------------------------------------------------------------------------------------------+
              | HTTP POST /api/v1/tools/execute
              | Headers: X-Sentinel-Tenant, X-Correlation-ID, X-Request-ID, X-Idempotency-Key
              v
+---------------------------------------------------------------------------------------------------------------+
|                                      TOOL & ACTION GATEWAY (Go)                                               |
|                                                                                                               |
|  [ 12-Step Access Conjunction Gate ]                                                                          |
|    1. Validate Context & Invariants    5. Singleflight Idempotency Check    9. Policy Prohibition Check       |
|    2. Registry Lookup (Manifest)       6. TOCTOU Precondition Check        10. Pre-Execution Obligations      |
|    3. Capability Allowlist Check       7. Max Input Size (256 KB)          11. Bounded Timeout & Recovery     |
|    4. Shadow Mode Verification         8. Authoritative Policy Engine      12. Output Scrubbing & Mod-10 Check|
|                                                                                                               |
|  [ Registered READ_ONLY Tool Execution ]                                                                      |
|    • incident.get                 • artifact.metadata.get                                                     |
|    • validation.findings.list_redacted • workflow.get                                                         |
+---------------------------------------------------------------------------------------------------------------+
              |
              | Typed / Redacted Output (DataClassification: METADATA_ONLY / REDACTED_FINDINGS)
              v
+---------------------------------------------------------------------------------------------------------------+
|                                            AI TIER (Google ADK)                                               |
|                                                                                                               |
|  [ Monotonic Evidence Expansion ] ---> AuthorizedEvidenceSet.expand_from_tool_result()                        |
|                                                                                                               |
|  [ Structured Output Generation ] ---> DiagnosisOutput (JSON Schema)                                          |
|                                                                                                               |
|  [ Evidence Grounding Verifier ] --------------------------------------------------------------------------+  |
|    • Subset Invariant Check: ClaimedEvidence ⊆ AuthorizedEvidenceSet                                       |  |
|    • Fail-Closed Gate: Fabricated citations (e.g. FINDING-999999) -> REJECT                                |  |
|                                                                                                            |  |
|  [ Transactional Journaling ] -------> AuditMetadata + Hashes -> Response to Gateway Bridge               |  |
+------------------------------------------------------------------------------------------------------------+--+
```

---

## 3. Prompt Trust Partitioning

Prompts are partitioned into **4 disjoint security domains** to eliminate prompt injection and model confusion:

| Domain | Trust Level | Description | Mutability |
| :--- | :--- | :--- | :--- |
| **`SYSTEM_POLICY`** | Tier 1 (Highest) | Immutable system directives, read-only scope, zero-release invariant, mandatory disclaimer. | Immutable (Codebase) |
| **`TRUSTED_CONTEXT`** | Tier 2 | Server-injected authenticated tenant ID, incident ID, correlation ID, budget limits, allowed tools. | Immutable (Server Context) |
| **`UNTRUSTED_FINANCIAL_CONTENT`** | Tier 3 | Counterparty filenames, line numbers, and pre-redacted validation finding descriptions fenced in XML. | Untrusted Data |
| **`TOOL_OUTPUT`** | Tier 4 | Typed outputs returned by the Tool Gateway during execution. | Governed Monotonic |

---

## 4. Evidence Grounding Verification

The AI tier enforces a mathematical subset invariant before any diagnostic report can be stored or presented to operators:

$$\text{ClaimedEvidence}(\mathcal{O}) = \left( \bigcup_{h \in \mathcal{O}.\text{hypotheses}} h.\text{evidence\_refs} \right) \cup \mathcal{O}.\text{evidence\_refs}$$

$$\text{IsGrounded}(\mathcal{O}) \iff \text{ClaimedEvidence}(\mathcal{O}) \subseteq \text{AuthorizedEvidenceSet}_N$$

1. $\text{AuthorizedEvidenceSet}_0$ is initialized from `envelope.findings`, `envelope.available_runbooks`, and `envelope.telemetry_summary`.
2. As tools execute via the Tool Gateway, evidence tokens are monotonically appended.
3. The LLM cannot invent or mint evidence tokens. Any citation outside the set triggers `GROUNDING_VIOLATION` / `UNGROUNDED_REJECTED`.

---

## 5. Security & Threat Mitigation

| Threat Vector | Attack Mechanism | Preventive & Detective Controls | Containment State |
| :--- | :--- | :--- | :--- |
| **Indirect Prompt Injection** | Injection inside finding descriptions or filenames | Prompt Trust Partitioning + Delimiter Sandboxing + Model Armor | Injected directives treated as raw text; zero release tokens minted |
| **Privilege Escalation** | Claiming file is approved or released | Tool Registry contains zero mutating tools; Policy Layer 20 Deny | Ignored by deterministic worker; file remains `QUARANTINED` |
| **Fake Finding Citations** | Hallucinating `FINDING-999999` to justify release | Evidence Grounding Verifier subset check | Output rejected with `GROUNDING_VIOLATION` |
| **Tool Scope Escalation** | Calling tools outside agent permission scope | 12-term Conjunction Gate + Server-injected caller identity | Gateway returns `403 POLICY_DENIED`; handler uncalled |
| **Multi-Tenant IDOR** | Injecting foreign `tenant_id` in arguments | Model parameters stripped; SQL queries bound to server `TenantID` | Data queries scoped strictly to $1; leaks blocked |

---

## 6. Verification Evidence

- **Unit & Property Tests (`pytest ai-tier/tests/`)**: 17/17 tests passing (100% pass rate).
- **Adversarial Security Evaluations (`python ai-tier/evals/runner.py`)**: 15/15 attack scenarios passing (100% pass rate, 92/92 checks).
- **Go Tool Gateway Suite (`go test ./internal/toolgateway/...`)**: 25/25 tests passing, 0 data races, 89,667 fuzz iterations.
- **Go Core Packages (`go test ./internal/...`)**: 16/16 packages passing with 0 failures.
- **Frontend Test Suite (`npm test`)**: 14/14 tests passing.
- **Documentation Drift (`scripts/generate_docs.py --check`)**: 0 drift against `CAPABILITY_MATRIX.yaml`.
