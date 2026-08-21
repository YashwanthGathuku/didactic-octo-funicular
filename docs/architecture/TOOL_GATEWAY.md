# Governed Tool & Action Gateway (SGACA Phase P04)

## 1. Architectural Invariant & Executive Overview

The **SentinelFlow Tool & Action Gateway** is the exclusive, authoritative enforcement boundary between callers (autonomous agents, human operators, external systems, and future analytics workbenches) and executable system capabilities.

```
+-------------------------------------------------------------------------------+
|                             CALLER / AI AGENT TIER                            |
|        (May reason within pre-evaluated capability envelopes; READ-ONLY)      |
+---------------------------------------+---------------------------------------+
                                        |
                                        | ToolRequest (Untrusted Args + IdempotencyKey)
                                        v
+-------------------------------------------------------------------------------+
|                       SENTINEL TOOL & ACTION GATEWAY                          |
|                                                                               |
|  1. Trusted Context Verification (Server-Injected Tenant, Autonomy, Roles)    |
|  2. Versioned Tool Registry Lookup & Immutable Manifest Hash Verification      |
|  3. Context Tool Allowlist & Capability Authorization Check                   |
|  4. Shadow Mode Invariant Check (Side-effect blocking)                        |
|  5. Input Argument Canonical Hashing & Idempotency / Singleflight Lock        |
|  6. TOCTOU Resource Precondition Verification (Artifact Hash, Version)        |
|  7. Strict JSON & Size Validation (Max Input Bytes Guard)                     |
|  8. Authoritative Policy Engine Evaluation (Deterministic 4-Layer Envelope)   |
|  9. Capability Prohibition Mapping (IsCapabilityProhibited Conjunction)       |
| 10. Pre-Execution Obligation Verification (CANDIDATE_ONLY, DUAL_CONTROL, etc) |
| 11. Bounded Isolated Execution (Context Timeout, Panic Recovery, Rate Limits)  |
| 12. Output Schema Validation, Redaction Scan, Post-Obligations & Outbox Log    |
+-------------------+-----------------------+-----------------------------------+
                    |                       |
                    v                       v
+-----------------------------+ +-----------------------------+
|   DETERMINISTIC POLICY      | |   IMMUTABLE OUTBOX & STORE  |
|   (P03 / RFC 8785 Engine)   | |   (tool_invocations table)  |
+-----------------------------+ +-----------------------------+
```

### The Fundamental Access Equation

Every tool invocation requires satisfying a strict 12-term conjunction with zero bypasses:

$$\text{EffectiveToolAccess} = \text{RegisteredTool} \cap \text{CallerCapability} \cap \text{ContextToolAllowlist} \cap \text{TenantAuthorization} \cap \text{PolicyPermission} \cap \text{ResourceScope} \cap \text{ObligationSatisfaction}$$

If any term in the conjunction evaluates to `false`, the Gateway **fails closed immediately** without executing the tool handler.

---

## 2. Core Architectural Components

### 2.1 Trusted Execution Context vs. Untrusted Model Arguments

To eliminate prompt injection and context spoofing attacks:
- **`TrustedExecutionContext`** is constructed and verified strictly on the server.
- The AI tier and callers **cannot override or inject** `tenant_id`, `caller_id`, `caller_roles`, `caller_autonomy_level`, `allowed_tools`, or `execution_mode`.
- Model arguments (`ToolRequest.Args`) are treated as untrusted data and validated against strict JSON schema bounds and size limits.

### 2.2 Side-Effect Classification & Financial Guardrail

Tools are categorized by explicit side-effect classes:
1. `READ_ONLY`: No system state changes; returns sanitized data.
2. `INTERNAL_STATE_WRITE`: Updates internal workflow/incident operational tracking.
3. `CANDIDATE_SANDBOX_WRITE`: Generates candidate/derived artifacts in isolated storage without modifying originals.
4. `REVERSIBLE_EXTERNAL`: Calls external APIs with compensating rollback actions.
5. `IRREVERSIBLE_EXTERNAL`: Unreversible external side effects (e.g. notifications).
6. `IRREVERSIBLE_FINANCIAL`: Unreversible ledger movements or wire transfers.

> [!CAUTION]
> **Safety Invariant SF-GATEWAY-001**: No tool with side-effect class `IRREVERSIBLE_FINANCIAL` may be registered for autonomous agent execution (`MaxAutonomy > 0`). Any attempt to register an irreversible financial tool for agent execution fails immediately at gateway startup (`ErrIrreversibleFinancialAgent`).

### 2.3 Immutable Manifests & RFC 8785 Canonical Hashing

Every registered tool is described by an immutable `ToolManifest` whose cryptographic digest is computed using RFC 8785 JSON Canonicalization Scheme:

$$\text{manifest\_hash} = \text{SHA256}(\text{CanonicalJSON}(\text{ToolManifest}))$$

Re-registration of existing `(tool_id, version)` tuples is strictly rejected (`ErrDuplicateToolRegistration`).

### 2.4 Idempotency Coordinator & Singleflight Concurrency

The Gateway provides durable, tenant-scoped idempotency:
- Composite Key: `(tenant_id, caller_id, tool_id, tool_version, idempotency_key)`
- If a request is received with an existing idempotency key and **identical payload hash**, the gateway replays the authoritative cached result without re-executing the handler.
- If a request is received with an existing idempotency key but a **different payload hash**, the gateway returns `ErrIdempotencyConflict`.
- If concurrent requests arrive simultaneously with the same key, the **singleflight coordinator** ensures exactly **one** handler execution runs while concurrent callers wait and receive the authoritative response.

### 2.5 Obligation Engine & Dual-Control Verification

Policy obligations are classified into pre-execution and post-execution phases:
- **Pre-execution**: `CANDIDATE_ONLY`, `IMMUTABLE_PARENT_REQUIRED`, `SANDBOX_ONLY`, `MAX_ATTEMPTS`, `DUAL_CONTROL`, `EXACT_ARTIFACT_HASH`.
- **Post-execution**: `AUDIT_REQUIRED`, `DETERMINISTIC_REVALIDATION`.
- Model-supplied `"satisfied": true` payloads are ignored. Obligations are verified exclusively against trusted infrastructure `ObligationEvidence`.

### 2.6 Authoritative Typed Output Security & AI Boundary

To protect data confidentiality across trust boundaries:
- **Authoritative Classification**: Tools declare `DataClassifications` and `AllowedOutputClassifications` (`PUBLIC`, `INTERNAL`, `CONFIDENTIAL`, `FINANCIAL_SENSITIVE`, `PII`, `SECRET`, `METADATA_ONLY`, `REDACTED_FINDINGS`).
- **AI Tier Boundary Law**: Structured outputs marked `SECRET` are strictly prohibited from crossing into the AI tier and fail closed immediately (`ErrOutputValidationFailed`).
- **Forbidden Key Scans**: Raw secrets (`api_key`, `private_key`, etc.) and unredacted financial account keys (`raw_account_number`, `raw_pan`, `cvv`) are rejected.
- **Defense-in-Depth Filter**: Unmasked SSNs (`\b\d{3}-\d{2}-\d{4}\b`) and ABA Routing Transit Numbers passing Federal Reserve Mod-10 checksums are blocked from exiting non-sensitive tool boundaries.

### 2.7 Caller Autonomy & Non-Agent Authority Semantics

- **`AGENT` Callers**: Bound by `[MinAutonomy, MaxAutonomy]` constraints.
- **Non-Agent Callers (`HUMAN`, `SERVICE`, `DETERMINISTIC_CONTROL`, `API`)**: Exempt from artificial `A0-A4` agent autonomy level checks, but **never receive implicit unrestricted authority**. Every non-agent invocation strictly requires verified tenant isolation, required capabilities, allowlist inclusion, and deterministic policy permission.

---

## 3. Initial Registered Safe Tool Proof Set (P04)

Phase P04 registers a verified proof set of safe, bounded `READ_ONLY` tools:

| Tool ID | Action | Required Capability | Max Autonomy | Description |
| :--- | :--- | :--- | :--- | :--- |
| `incident.get` | `GET_INCIDENT` | `INCIDENT_READ` | 4 | Retrieves quarantined file incident metadata within verified tenant boundaries. |
| `validation.findings.list_redacted` | `LIST_FINDINGS` | `FINDINGS_READ_REDACTED` | 4 | Lists deterministic NACHA validation findings with sensitive payload fields redacted. |
| `artifact.metadata.get` | `GET_ARTIFACT_METADATA` | `ARTIFACT_METADATA_READ` | 4 | Retrieves immutable artifact metadata (SHA-256 digest, classification, quarantined state). |
| `workflow.get` | `GET_WORKFLOW` | `WORKFLOW_READ` | 4 | Retrieves agent workflow execution status and step timeline. |

---

## 4. Universal Persistence & Outbox Journaling

Tool execution is recorded in the universal `tool_invocations` table (Migration `018_tool_gateway.sql`):
- Tracks: `id`, `tenant_id`, `tool_id`, `tool_version`, `manifest_hash`, `caller_type`, `caller_id`, `caller_autonomy_level`, `workflow_id`, `idempotency_key`, `request_hash`, `status`, `policy_decision_id`, `policy_decision_hash`, `policy_bundle_hash`, `input_hash`, `output_hash`, `error_code`, `duration_ms`, `created_at`, `completed_at`.
- Uniqueness Constraint: `UNIQUE (tenant_id, caller_id, tool_id, tool_version, idempotency_key)` guarantees restart-safe durable logical idempotency across process crashes.
- Emits universal transactional outbox events:
  - `TOOL_INVOCATION_SUCCEEDED`
  - `TOOL_INVOCATION_DENIED`
  - `TOOL_INVOCATION_FAILED`
- Outbox payloads contain **strictly identifiers and SHA-256 hashes**; raw financial payloads and sensitive data are excluded from event streams.

---

## 5. Measured Performance & Verification Summary

### Benchmarks (Intel Core i5-8300H @ 2.30GHz)
- **Registry Lookup**: **37.50 ns/op** (0 B/op, 0 allocs/op) $\rightarrow$ **26.6M lookups/sec**
- **Idempotency Lookup**: **367.6 ns/op** (128 B/op, 6 allocs/op) $\rightarrow$ **2.72M checks/sec**
- **Full Governed Execution**: **104 µs/op** (including 4-layer policy engine evaluation, RFC 8785 canonical hashing, input/output validation, and handler execution) $\rightarrow$ **~9,600 full authorized executions/sec per core**

### Fuzz & Race Testing
- **Fuzz Campaign**: **89,667 iterations (18,738 execs/sec)** across 8 workers with 0 failures.
- **Race Detector**: **0 data races** (`go test -race ./internal/toolgateway/...`).
- **Security Invariants**: 25/25 verified automated test cases.
