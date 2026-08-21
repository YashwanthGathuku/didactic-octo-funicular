# AI Guardrails & Google Cloud Model Armor Architecture (P09)

## 1. Executive Summary & Architectural Role

SentinelFlow Phase P09 integrates **Google Cloud Model Armor** as an inline, content-safety boundary defense layer surrounding the Google ADK and Gemini AI tier (`gemini-3.5-flash`). 

Model Armor provides pre-invocation prompt screening (detecting direct/indirect prompt injection, jailbreaks, malicious URIs, and PII exfiltration) and post-invocation response screening (detecting secret leakage, hallucinated PII, and unsafe content).

```
[Agent Context Envelope]
             │
             ▼
   [Data Minimization Layer]  (Mask 10-17 digit ACCT, 9-digit ABA routing, 94-char NACHA records)
             │
             ▼
   [4-Domain Trust Partitioning] (SYSTEM_POLICY, TRUSTED_CONTEXT, UNTRUSTED_CONTENT, TOOL_OUTPUT)
             │
             ▼
   [Model Armor: SanitizeUserPrompt]
      ├── BLOCK / Injection Detected ──> PROMPT_SECURITY_BLOCKED (Gemini calls = 0)
      ├── UNAVAILABLE & REQUIRED ───────> GUARDRAIL_UNAVAILABLE (Go engine continues)
      └── ALLOW / SANITIZED
             │
             ▼
       [Gemini / ADK Model Invocation]
             │
             ▼
   [Model Armor: SanitizeModelResponse]
      ├── BLOCK / Leakage Detected ────> MODEL_RESPONSE_BLOCKED
      └── ALLOW / SANITIZED
             │
             ▼
       [Pydantic Schema Validation] (Reject damaged JSON -> MODEL_OUTPUT_INVALID)
             │
             ▼
       [Evidence Grounding Validator] (E_returned ⊆ E_authorized)
             │
             ▼
       [BoundaryAuditRecord & Hash Chain Output]
```

---

## 2. Formal Authority Separation Equation

Model Armor operates strictly as a content-filtering guardrail and **possesses zero financial authorization capability**. A clean pass through Model Armor is a necessary precondition for model invocation, but is entirely insufficient to authorize system state changes, file mutations, or financial releases.

$$\text{ModelArmorPass} \neq \text{Authorization}$$

$$\text{Authorization} = \text{Identity} \cap \text{Capability} \cap \text{ToolManifest} \cap \text{Policy} \cap \text{Obligations} \cap \text{ResourceFreshness}$$

### Authority Matrix:

| Capability / Responsibility | Go Control Plane | Python / ADK Agent Tier | Google Cloud Model Armor |
| :--- | :--- | :--- | :--- |
| **Financial Correctness & NACHA Validation** | **Authoritative (Exclusive)** | Non-Authoritative | Non-Authoritative |
| **Tool Gateway Execution & Access Control** | **Authoritative (Exclusive)** | Requestor Only | Non-Authoritative |
| **Workflow State Machine & Invariants** | **Authoritative (Exclusive)** | Non-Authoritative | Non-Authoritative |
| **Candidate Byte Generation & Hashing** | **Authoritative (Exclusive)** | Proposes Intent | Non-Authoritative |
| **Policy Engine & Rule Enforcement** | **Authoritative (Exclusive)** | Interprets Context | Non-Authoritative |
| **Release & Human Approval Gating** | **Authoritative (Exclusive)** | Strictly Denied | Strictly Denied |
| **Prompt Minimization & Trust Partitioning** | Boundary Provider | **Authoritative Owner** | Filter Target |
| **Prompt Injection & Safety Screening** | Non-Authoritative | Client Integrator | **Authoritative Filter** |
| **Output Leakage & Secret Screening** | Non-Authoritative | Client Integrator | **Authoritative Filter** |
| **Evidence Grounding Verification** | Outbox Recorder | **Authoritative Enforcer** | Non-Authoritative |

---

## 3. Google Cloud Model Armor Regional REST Execution

SentinelFlow communicates with Model Armor via regional REST execution endpoints authenticated with Google Application Default Credentials (ADC):

- **Regional Execution Endpoint**: `https://modelarmor.{location}.rep.googleapis.com/v1` (Default: `us-central1`)
- **Template Resource Path**: `projects/{project_id}/locations/{location}/templates/{template_id}`
- **User Prompt Screening RPC**: `POST .../templates/{template_id}:sanitizeUserPrompt`
- **Model Response Screening RPC**: `POST .../templates/{template_id}:sanitizeModelResponse`

### Dual-Screening Payloads

1. **User Prompt Screening (`sanitizeUserPrompt`)**:
   ```json
   {
     "userPromptData": {
       "text": "<untrusted_content>...</untrusted_content>"
     }
   }
   ```
2. **Model Response Screening (`sanitizeModelResponse`)**:
   ```json
   {
     "modelResponseData": {
       "text": "{\"classification\": \"ENTRY_HASH_MISMATCH\", ...}"
     },
     "userPromptData": {
       "text": "..."
     }
   }
   ```

---

## 4. Fail-Closed Operational Modes & Availability Isolation

SentinelFlow supports three distinct operational modes in `ModelArmorConfig`:

1. **`REQUIRED` (Production High-Assurance)**:
   - All prompts and responses **must** pass Model Armor screening.
   - If Model Armor is unreachable, times out, or returns HTTP 5xx, the AI boundary fails closed with `GUARDRAIL_UNAVAILABLE`.
   - **Zero unscreened prompts reach Gemini**.
   - **Deterministic Go financial engine continues operating normally** without AI assistance.
2. **`OBSERVE` (Staging / Advisory Telemetry)**:
   - Prompts and responses are screened asynchronously or with advisory logging.
   - Guardrail errors log security telemetry without blocking fallback pipelines.
3. **`DISABLED` (Offline Unit Testing Only)**:
   - Guardrail screening is bypassed for local isolated test suites.

### Fault Decoupling:
A complete outage of Model Armor or Google Gemini **never halts the Go Control Plane**. Ingestion, NACHA validation, database updates, quarantine isolation, and deterministic audit journaling continue with zero interruption.

---

## 5. The 8-Step Guarded Model Boundary Lifecycle

The `GuardedModelBoundary` wrapper (`ai-tier/guardrails/boundary.py`) executes an 8-step lifecycle for every agent invocation across the fleet:

1. **Pre-Invocation Data Minimization**:
   - Redacts 10–17 digit account numbers to `[ACCOUNT_REDACTED]`.
   - Redacts 9-digit ABA routing numbers to `[ROUTING_REDACTED]`.
   - Redacts 94-character fixed-width NACHA record lines to `[NACHA_RECORD_REDACTED]`.
2. **4-Domain Prompt Trust Partitioning**:
   - `Domain 1: SYSTEM_POLICY`: Immutable, high-priority role and safety rules.
   - `Domain 2: TRUSTED_CONTEXT`: Authenticated workflow and tenant metadata.
   - `Domain 3: UNTRUSTED_FINANCIAL_CONTENT`: Minimised validation findings and semantic diffs fenced with `<untrusted_content>` tags.
   - `Domain 4: TOOL_OUTPUT`: Read-only deterministic check results.
3. **Input Content Hashing**:
   - Computes SHA-256 hashes of pre-guardrail and post-guardrail prompts for provenance.
4. **Pre-Invocation Model Armor Screening**:
   - Dispatches prompt to `:sanitizeUserPrompt`.
   - If blocked $\implies$ returns `PROMPT_SECURITY_BLOCKED` immediately (**0 Gemini API calls**).
   - If unavailable and mode is `REQUIRED` $\implies$ returns `GUARDRAIL_UNAVAILABLE`.
5. **Governed Model Invocation**:
   - Invokes `gemini-3.5-flash` via Google ADK / `google-genai` with strict `response_schema`.
6. **Post-Invocation Model Armor Screening**:
   - Dispatches model response to `:sanitizeModelResponse`.
   - If blocked (e.g. secret leakage or unredacted PII) $\implies$ returns `MODEL_RESPONSE_BLOCKED`.
7. **Pydantic Schema Validation & Evidence Grounding**:
   - Validates JSON structure against target Pydantic schema (returns `MODEL_OUTPUT_INVALID` on corruption).
   - Enforces $E_{\text{returned}} \subseteq E_{\text{authorized}}$ via `EvidenceGroundingVerifier` (returns `GROUNDING_VIOLATION` on ungrounded citations).
8. **Deterministic Fallback & Audit Journaling**:
   - If model invocation or guardrail fails and fallback is enabled, executes deterministic rule baseline.
   - Emits `BoundaryAuditRecord` capturing pre/post hashes, token counts, latency, and guardrail verdicts.

---

## 6. Identity-Bound Dual-Control Approval Gating

In accordance with SentinelFlow safety invariants:
- **Dual-Control Reality**: The Go Control Plane enforces `"Identity-bound dual-control approval with cryptographic artifact and policy integrity binding."`
- **Release Gating Reality**: When a candidate has passed deterministic verification but the CriticAgent (VerifierAgent) raises a high-risk concern (`CONCERN` / `HUMAN_INVESTIGATION_REQUIRED`), the workflow transitions to `WorkflowHumanReview` (`HUMAN_INVESTIGATION_REQUIRED`).
- **Investigation $\neq$ Release Approval**: A candidate under human investigation cannot be released until two distinct authorized human operators review and approve the candidate.

---

## 7. Adversarial Evaluation & Red-Team Test Suite (95 Scenarios)

The unified adversarial evaluation suite (`ai-tier/evals/runner.py`) validates all 5 phases across 95 adversarial scenarios:

| Evaluation Phase | Scenario Count | Scope & Invariants Tested | Pass Rate |
| :--- | :--- | :--- | :--- |
| **P05: Single-Agent Diagnosis** | 14 Scenarios | Prompt injection, tool manipulation, secret leakage, read-only disclaimers | **100.0%** |
| **P06: Multi-Agent Orchestration** | 16 Scenarios | Roster spoofing, loop delegation, policy override, partial failure isolation | **100.0%** |
| **P07: Governed Remediation** | 20 Scenarios | Original byte tampering, arbitrary patches, attempt limits, idempotency | **100.0%** |
| **P08: Independent Verification** | 20 Scenarios | Candidate corruption, derivation integrity, deterministic dominance, policy freshness | **100.0%** |
| **P09: Model Armor Boundary** | 25 Scenarios | Jailbreaks, metadata SSRF, homoglyphs, fail-closed timeouts, output secret interception | **100.0%** |
| **Total Fleet Evals** | **95 Scenarios** | **Comprehensive Autonomous Defense-in-Depth** | **100.0% (312/312 Checks)** |

---

## 8. Capability Matrix Integration

The following capabilities are formally verified and tracked in `docs/CAPABILITY_MATRIX.yaml`:

- `model_armor_client`: Regional REST execution client with Google ADC token management, fault injection, and fail-closed handling.
- `guarded_model_boundary`: 8-step hardened wrapper with data minimization, 4-domain partitioning, pre/post screening, schema enforcement, and audit hashing.
- `ai_guardrails_evals`: 25 adversarial Model Armor red-team evaluation scenarios passing at 100%.
