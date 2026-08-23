# Governed ACH Return Intelligence Architecture (Phase P12)

## 1. Overview & Objectives

SentinelFlow Phase P12 introduces **Governed ACH Return Intelligence**, empowering financial operations teams to proactively detect, analyze, and manage Automated Clearing House (ACH) returns while strictly preserving every **Strict Governed AI Control Architecture (SGACA)** authority and security boundary.

In enterprise banking and payment processing, ACH return codes (such as `R01` for Insufficient Funds, `R03` for No Account, `R05`/`R10` for Unauthorized Transactions, and `R16` for Account Frozen / OFAC Sanctions) carry strict regulatory return windows, Nacha network thresholds (e.g. 0.5% unauthorized return rate ceiling), and operational SLA cutoffs. 

SentinelFlow P12 establishes a deterministic, mathematically grounded, and cryptographically audited intelligence engine paired with an Autonomy Level A1 advisory specialist (`ReturnRiskAgent`).

---

## 2. Fundamental SGACA Invariants

The return risk subsystem is governed by the following immutable mathematical and architectural invariants:

$$\text{ReturnRiskAssessment} \neq \text{FinancialDecision} \land \text{ReturnRiskAgent} \neq \text{ReturnAuthority}$$

$$\text{HistoricalReturnPattern} \neq \text{CurrentTransactionTruth} \land \text{MemoryRecall} \neq \text{Evidence}$$

$$\text{RiskHigh} \neq \text{AutoRejectFinancialFile} \land \text{RiskLow} \neq \text{AutoReleaseFinancialFile}$$

```
+-----------------------------------------------------------------------------------+
|                        SGACA P12 Authority Invariants                             |
+-----------------------------------------------------------------------------------+
| 1. Deterministic Dominance: Go Engine Score/Tier strictly dominates AI Tier output|
| 2. Read-Only / Non-Authority Mandate: ReturnRiskAgent has ZERO release/mutate power|
| 3. Disjoint Grounding: EvidenceRefs ∩ MemoryRefs = ∅                              |
| 4. Input Minimization: Masked 4-Domain Prompt Trust Partitioning                  |
| 5. Zero Cloud Cost Offline Baseline: 100% testable with deterministic fallback    |
+-----------------------------------------------------------------------------------+
```

---

## 3. Layered Architecture

```
[ Incoming Return Event (Synthetic / Webhook) ]
                 │
                 ▼
 [ Go Gateway: gateway/internal/returnrisk/ ]
   ├── ReturnCodeRegistry lookup (Taxonomy: ACH-RETURN-TAXONOMY-V1)
   ├── Historical Context extraction (7d/30d volume, partner return rate)
   ├── Deterministic feature normalization (N_code, N_freq7, N_partner, etc.)
   ├── Weighted RiskScore calculation [0-100] & Tier assignment (LOW, MEDIUM, HIGH, SEVERE)
   ├── RFC 8785 Canonical AssessmentHash generation
   └── Tool Gateway / API Context Marshalling
                 │
                 ▼ (Server-Injected Context Envelope)
 [ Python AI Tier: ai-tier/agents/return_risk.py ]
   ├── GuardedModelBoundary (P09) & 4-Domain Prompt Trust Partitioning
   ├── Disjoint Grounding Filter: EvidenceRefs != MemoryRefs
   ├── Deterministic Dominance Override: Model prose CANNOT change Go score or tier
   └── ReturnRiskAssessment generation (Advisory Explanation Only)
                 │
                 ▼
 [ Go Control Plane & Incident Commander ]
   ├── Commander synthesizes findings (Evidence Set union)
   ├── Policy Engine evaluates rulepack (RiskHigh != AutoReject; RiskLow != AutoRelease)
   └── On confirmed resolution: M1 Fact emitted to gateway/internal/memory
```

---

## 4. Authoritative ACH Return Code Taxonomy (`ACH-RETURN-TAXONOMY-V1`)

The deterministic engine implements an authoritative taxonomy catalog covering 11 representative NACHA return codes:

| Code | Label | Normalized Category | Severity | Retry Characteristic | Return Window | Nacha Threshold | Base Severity |
| :--- | :--- | :--- | :--- | :--- | :--- | :--- | :--- |
| **R01** | `INSUFFICIENT_FUNDS` | `INSUFFICIENT_FUNDS` | `MEDIUM` | `RETRYABLE_ONCE` | 2 Banking Days | 15% Overall | 45.0 |
| **R02** | `ACCOUNT_CLOSED` | `ACCOUNT_STATUS` | `HIGH` | `NON_RETRYABLE` | 2 Banking Days | 3% Admin | 50.0 |
| **R03** | `NO_ACCOUNT_UNABLE_TO_LOCATE` | `ACCOUNT_DATA` | `HIGH` | `RETRYABLE_WITH_CORRECTION` | 2 Banking Days | 3% Admin | 55.0 |
| **R04** | `INVALID_ACCOUNT_STRUCTURE` | `ACCOUNT_DATA` | `HIGH` | `RETRYABLE_WITH_CORRECTION` | 2 Banking Days | 3% Admin | 55.0 |
| **R05** | `UNAUTHORIZED_CONSUMER_CORPORATE_SEC` | `UNAUTHORIZED` | `CRITICAL` | `NON_RETRYABLE` | 60 Calendar Days | 0.5% Unauthorized | 90.0 |
| **R07** | `AUTHORIZATION_REVOKED` | `UNAUTHORIZED` | `CRITICAL` | `PROHIBITED` | 60 Calendar Days | 0.5% Unauthorized | 85.0 |
| **R08** | `PAYMENT_STOPPED` | `ADMINISTRATIVE` | `HIGH` | `NON_RETRYABLE` | 2 Banking Days | 15% Overall | 60.0 |
| **R10** | `CUSTOMER_ADVISES_UNAUTHORIZED` | `UNAUTHORIZED` | `CRITICAL` | `PROHIBITED` | 60 Calendar Days | 0.5% Unauthorized | 95.0 |
| **R16** | `ACCOUNT_FROZEN_OR_OFAC` | `OFAC_RESTRICTED` | `CRITICAL` | `PROHIBITED` | 2 Banking Days | Regulatory Restricted | 85.0 |
| **R20** | `NON_TRANSACTION_ACCOUNT` | `ACCOUNT_STATUS` | `HIGH` | `RETRYABLE_WITH_CORRECTION` | 2 Banking Days | 15% Overall | 50.0 |
| **R29** | `CORPORATE_CUSTOMER_NOT_AUTHORIZED` | `UNAUTHORIZED` | `CRITICAL` | `PROHIBITED` | 2 Banking Days | 0.5% Unauthorized | 90.0 |

---

## 5. Deterministic Risk Score Formula

The Go Control Plane computes a normalized, bounded risk score $S \in [0.0, 100.0]$:

$$\text{RawRisk} = w_{\text{code}} N_{\text{code}} + w_{\text{freq7}} N_{\text{freq7}} + w_{\text{freq30}} N_{\text{freq30}} + w_{\text{partner}} N_{\text{partner}} + w_{\text{trend}} N_{\text{trend}} + w_{\text{exposure}} N_{\text{exposure}} + w_{\text{sla}} N_{\text{sla}}$$

Where all weights sum to exactly $1.00$:
- $w_{\text{code}} = 0.30$ (Return Code Severity)
- $w_{\text{freq7}} = 0.15$ (7-day Velocity Surge)
- $w_{\text{freq30}} = 0.10$ (30-day Cumulative Returns)
- $w_{\text{partner}} = 0.15$ (Partner Return Rate vs Regulatory Ceiling)
- $w_{\text{trend}} = 0.10$ (Recent Acceleration Ratio)
- $w_{\text{exposure}} = 0.10$ (Dollar Exposure Bucket)
- $w_{\text{sla}} = 0.10$ (SLA Proximity / Breach Probability)

### Discrete Risk Tiers
- **`LOW`**: $[0.0, 29.99]$
- **`MEDIUM`**: $[30.0, 59.99]$
- **`HIGH`**: $[60.0, 79.99]$
- **`SEVERE`**: $[80.0, 100.0]$

---

## 6. Fixed 7-Agent Canonical Roster

With Phase P12, SentinelFlow's immutable fixed roster expands from 6 to 7 agents:

1. `IncidentCommanderAgent` (Autonomy A1)
2. `DiagnosisAgent` (Autonomy A1)
3. `PolicySLAAgent` (Autonomy A1)
4. `MemoryAgent` (Autonomy A1)
5. `RemediationAgent` (Autonomy A2 — Proposal Only)
6. `VerifierAgent` (Autonomy A1 — Independent Critic)
7. `ReturnRiskAgent` (Autonomy A1 — Return Risk Intelligence)

---

## 7. Operational Memory (M1) Integration

Upon confirmed operational resolution of an ACH return incident, the Go engine emits an authoritative `FactTypeVerifiedFailurePattern` record to M1 Operational Memory via `gateway/internal/memory/service.go`.

This record contains:
- `assessment_id` and `assessment_hash` (RFC 8785 canonical hash)
- `partner_ref`, `return_code`, `risk_score`, and `risk_tier`
- `primary_drivers` contributing to the risk calculation
- `verifier_ref` and dual-control operational sign-off timestamp

---

## 8. Adversarial Evaluation Harness

The P12 Return Risk Evaluation Suite introduces **20 adversarial scenarios** (`ADV-RET-001` through `ADV-RET-020`) validating:
- Catalog schema enforcement and unknown return code fail-closed behavior
- Deterministic risk dominance (blocking model downgrade from `SEVERE` to `LOW`)
- Numeric score immutability
- Memory Non-Authority (poisoned memory cannot waive R01 surge or approve release)
- Autonomy Level A1 containment (blocking release, approval, ledger mutation, SQL execution, remediation writing, and dynamic agent spawning)
- Strict multi-tenant isolation and cross-tenant leakage prevention
- Freshness ceilings and stale pattern expiration
- Disjoint evidence and memory taxonomy enforcement
- Cold-start uncertainty calibration

Across the entire SentinelFlow platform, the master adversarial suite now evaluates **165 scenarios across 8 phases with a 100% pass rate**.
