# Data Sovereignty & Geographic Residency Architecture

## 1. Overview & Threat Model

Enterprise financial systems operate under strict regulatory and jurisdictional data protection mandates (e.g., GDPR Chapter V in the EU, RBI cross-border transaction localization circulars in India, and national privacy frameworks). In agentic AI architectures, autonomous agents interact with localized production data—such as NACHA files, operational memory facts, and investigation contexts.

### The Threat
Without explicit geographic scope enforcement, a multi-tenant or multi-region system poses severe compliance risks:
1. **Cross-Border Model Invocation**: An EU tenant's financial data or diagnostic finding could be routed to an LLM inference endpoint located in an unapproved region (e.g., `us-central1`).
2. **Cross-Border Memory Leakage**: Long-term operational memory facts extracted in one jurisdiction could be ingested into or retrieved from an out-of-region managed Memory Bank.
3. **Tenant Policy Bypass / Misconfiguration**: A compromised or misconfigured tenant-level configuration might attempt to authorize cross-border data transfer, bypassing organizational or regulatory requirements.

---

## 2. The Layer-20 Safety Precedence Model

SentinelFlow enforces a strict hierarchical policy evaluation model across five distinct layers (defined in `gateway/internal/policy/types.go`):

| Layer | Numerical Precedence | Authority Scope |
| :--- | :--- | :--- |
| `NETWORK_EXTERNAL` | 10 | Perimeter network & edge transport boundaries |
| `SENTINEL_SAFETY` | **20** | **Foundational system invariants (immutable system boot laws)** |
| `ENTERPRISE` | 30 | Organization-wide compliance & governance rules |
| `TENANT` | 40 | Tenant-specific operational preferences & workflows |
| `PARTNER` | 50 | Counterparty & partner file rules |

### SF-SAFE-007 As an Unoverridable Boot Invariant
Data sovereignty is codified as **`SF-SAFE-007`** within `LayerSentinelSafety` (Precedence 20):
- **Policy ID**: `SF-SAFE-007`
- **Domain**: `ENTERPRISE_ACTION`
- **Layer**: `SENTINEL_SAFETY` (Precedence 20)
- **Effect**: `DENY`
- **Reason Code**: `DATA_SOVEREIGNTY_VIOLATION`
- **Action**: `CROSS_REGION_DATA_TRANSFER`

Because `SENTINEL_SAFETY` (precedence 20) strictly dominates `ENTERPRISE` (30) and `TENANT` (40) in `PolicyEngine.Evaluate()`, no tenant configuration or enterprise policy can override `SF-SAFE-007`. If a tenant-layer policy sets `Effect: ALLOW` for a cross-region transfer, `SF-SAFE-007`'s safety `DENY` deterministically dominates. Furthermore, `SF-SAFE-007` is registered in `MandatorySafetyPolicyIDs`; the SentinelFlow policy engine **refuses to boot** if `SF-SAFE-007` is absent or corrupted.

---

## 3. Enforcement Architecture & Boundary Verification

Sovereignty checks are enforced at two distinct boundaries before any payload processing or outbound network transport occurs.

```
                  ┌────────────────────────────────────────────────────────┐
                  │                 SentinelFlow Gateway                   │
                  │   Tenant: DataRegion="europe-west1"                    │
                  │           AllowedRegions=["europe-west1"]              │
                  └──────────────────────────┬─────────────────────────────┘
                                             │
                       ┌─────────────────────┴─────────────────────┐
                       │                                           │
                       ▼                                           ▼
         ┌───────────────────────────┐               ┌───────────────────────────┐
         │   GuardedModelBoundary    │               │  GoogleMemoryBankProvider │
         │   (ai-tier/guardrails)    │               │     (ai-tier/memory)      │
         └─────────────┬─────────────┘               └─────────────┬─────────────┘
                       │                                           │
            [Step 0: Region Check]                      [_check_sovereignty()]
            Target: "us-central1"                       Target: "us-central1"
            Allowed: ["europe-west1"]                   Allowed: ["europe-west1"]
                       │                                           │
                       ▼ [MISMATCH]                                ▼ [MISMATCH]
         ┌───────────────────────────┐               ┌───────────────────────────┐
         │  Typed Failure Raised:    │               │  Typed Exception Raised:  │
         │ DATA_SOVEREIGNTY_VIOLATION│               │DataSovereigntyViolation   │
         │ - Zero model invocation   │               │ - Zero HTTP bytes sent    │
         │ - Zero data minimization  │               │ - Zero memory leaked      │
         │ - No fallback relabeling  │               │ - No fallback relabeling  │
         └───────────────────────────┘               └───────────────────────────┘
```

### 1. Model Boundary (`GuardedModelBoundary`)
- **Location**: `ai-tier/guardrails/boundary.py`
- **Mechanism**: `GuardedModelBoundary.__init__(..., location=...)` captures the model endpoint region. At `invoke(envelope, ..., allowed_regions=[...])`, Step 0 checks whether `self.location` is contained in `allowed_regions`.
- **Fail-Closed Semantics**: If a mismatch occurs, execution immediately terminates returning `GuardedInvocationResult(success=False, error_code="DATA_SOVEREIGNTY_VIOLATION", ...)`.
- **No Silent Fallback**: Guardrail failures surface explicitly as failures and are **never** silently converted to local/deterministic success.

### 2. Memory Boundary (`GoogleMemoryBankProvider`)
- **Location**: `ai-tier/memory/google_provider.py`
- **Mechanism**: `GoogleMemoryBankProvider(location=..., allowed_regions=[...])` gates `ingest_event()` and `retrieve_memories()` via `_check_sovereignty()`.
- **Fail-Closed Semantics**: Raises `DataSovereigntyViolationError` before any HTTP request or token acquisition takes place.

---

## 4. Adversarial Verification Matrix

The data sovereignty control plane is verified by the dedicated test suite `ai-tier/evals/sovereignty_runner.py` executing against `ai-tier/evals/adversarial_sovereignty.json`:

| Scenario ID | Attack Vector | Expected Containment | Security Invariant |
| :--- | :--- | :--- | :--- |
| `ADV-SOV-001` | EU tenant fact routed to `us-central1` model endpoint | `GuardedModelBoundary` rejects at Step 0 with `DATA_SOVEREIGNTY_VIOLATION`; model invocation source marked `NOT_RUN` | SF-SAFE-007 Pre-Invocation Isolation |
| `ADV-SOV-002` | India-localized payment data routed cross-border | `GuardedModelBoundary` halts before data minimization; `post_guardrail_input_hash` is empty; zero bytes cross boundary | SF-SAFE-007 Zero Data Exfiltration |
| `ADV-SOV-003` | Tenant-level policy attempts to override SF-SAFE-007 | `PolicyEngine` enforces Layer 20 dominance (`SENTINEL_SAFETY` > `TENANT`); safety `DENY` beats tenant `ALLOW` | Layer Precedence Hierarchy |
| `ADV-SOV-004` | Memory ingest of in-region fact into out-of-region Memory Bank | `GoogleMemoryBankProvider` raises `DataSovereigntyViolationError` prior to HTTP dispatch | Memory Boundary Residency |

---

## 5. Scope Disclaimer

SentinelFlow is an engineering demonstration and staging prototype for autonomous financial operations and policy enforcement. The data sovereignty control mechanisms described herein represent architectural control models, runtime boundary gates, and automated test fixtures. They do not constitute formal legal certification, regulatory compliance attestation, or jurisdictional audit approval. Institution-specific cross-border data transfer legality and compliance determinations remain the exclusive responsibility of authorizing human compliance officers.
