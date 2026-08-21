# Governed Remediation & Immutable Candidate Generation (Phase P07)

## Executive Summary & Non-Negotiable Invariants

SentinelFlow Phase P07 implements **Governed Remediation & Immutable Candidate Generation** under the Sentinel Governed Autonomous Control Architecture (SGACA).

```
                        +---------------------------------------------+
                        |           Python AI Tier (ADK)              |
                        |              RemediationAgent               |
                        |       Autonomy Level A2 (Proposal Only)     |
                        +----------------------+----------------------+
                                               |
                                               | Structured RemediationPlan
                                               | (Allowlisted repair intent)
                                               v
+-----------------------------------------------------------------------------+
|                           Go Control Plane Authority                        |
|                                                                             |
|  1. Policy Engine Evaluation (SF-SAFE-004)                                  |
|  2. Immutable Parent Verification (SHA-256 TOCTOU check)                    |
|  3. Read Original Parent Bytes from ObjectStore (H_before)                  |
|  4. Deterministic NACHA Arithmetic Execution                                |
|  5. Construct Fixed-Width 94-Char Candidate Bytes                           |
|  6. Verify Original Parent Bytes Untouched (H_after == H_before)            |
|  7. Store Derived Candidate in ObjectStore                                  |
|  8. Record Artifact Derivation & Remediation Plan in DB                     |
|  9. Execute Authoritative Deterministic NACHA Validation                    |
| 10. Transition Workflow: AWAITING_VERIFICATION (or retry if attempt < 3)    |
+-----------------------------------------------------------------------------+
```

---

## 1. Core Architectural Laws

### Law #1: Authority Invariant
$$\text{AgentProposal} \neq \text{CandidateMutationAuthority} \implies \text{CandidateMutationAuthority} = \text{GoControlPlane}$$
- The AI tier (Python `RemediationAgent`) operates with **Autonomy Level A2 (Propose Candidate Creation)**.
- The Python model **never** calculates numeric control totals, never outputs raw NACHA bytes, never writes to object storage or databases, and has zero direct mutation authority.
- The Go Control Plane computes authoritative totals, generates 94-character fixed-width records, and manages candidate persistence.

### Law #2: Immutable Original Law
$$\forall i \in \{1, 2, 3\}, \quad \text{Parent}(\text{Candidate}_i) = \text{OriginalArtifact} \quad \land \quad \text{Candidate}_i = \text{Apply}(\text{Original}, \text{Plan}_i)$$
- Quarantined original artifacts are strictly read-only ($H_{before} == H_{after}$).
- In multi-attempt remediation scenarios (e.g. Attempt 2 after Attempt 1 fails), the repair is **always applied against the original quarantined parent artifact**, never against a prior candidate.

### Law #3: Bounded Attempt Law (Max 3)
- Go strictly bounds remediation to a maximum of **3 attempts** per workflow.
- Attempt counter is owned and incremented by Go.
- If attempt 3 fails revalidation, the workflow transitions immediately to `UNRESOLVED` with outcome `REMEDIATION_ATTEMPTS_EXHAUSTED`. Attempt 4 fails closed.

### Law #4: AI Failure Decoupling
$$\text{ProviderFailure} \implies \text{CandidateCreationFromAgent} = 0$$
- If the AI tier or Gemini API experiences an outage or returns an invalid proposal, Go transitions the workflow to `AGENT_UNAVAILABLE` or `UNRESOLVED`.
- Go **never** fabricates candidate artifacts from AI proposals when the AI tier is unavailable.

### Law #5: P07/P08 Verification Gate Boundary
- Successful candidate revalidation in Phase P07 terminates at state `AWAITING_VERIFICATION` with outcome `CANDIDATE_REVALIDATION_PASSED`.
- Phase P07 does **not** declare `VERIFIED`, `APPROVED`, or `RELEASED`. Instead, it hands off to **Phase P08 Independent Verification** ([`INDEPENDENT_VERIFICATION.md`](file:///c:/Users/Gathu/Projects/fintech/docs/architecture/INDEPENDENT_VERIFICATION.md)), where the Go Verification Service re-reads candidate bytes from ObjectStore, executes 12 deterministic integrity checks, evaluates the ADK `VerifierAgent` (Critic) opinion under deterministic dominance, and enforces the dual-control boundary (`Verified != HumanApproved != Released`).

---

## 2. Allowlisted Remediation Operations

| Operation Type | Target Ref | Behavior |
| :--- | :--- | :--- |
| `RECOMPUTE_BATCH_CONTROL_TOTAL` | `BATCH-n` | Recomputes batch entry count, entry hash accumulator (least 10 digits sum of routing numbers), total debits, and total credits directly from parsed Type 6 entry detail records. |
| `RECOMPUTE_FILE_CONTROL_TOTAL` | `FILE_CONTROL` | Recomputes file batch count, block count (ceiling of total records / 10), entry count, entry hash accumulator, total debits, and total credits. |

---

## 3. Data Model & Derivation Schema

```sql
-- Remediation Plans Table
CREATE TABLE remediation_plans (
    id TEXT PRIMARY KEY,
    workflow_id TEXT NOT NULL,
    tenant_id TEXT NOT NULL,
    incident_id INTEGER NOT NULL,
    artifact_id INTEGER NOT NULL,
    expected_parent_sha256 TEXT NOT NULL,
    attempt_number INTEGER NOT NULL,
    plan_hash TEXT NOT NULL,
    operations_json TEXT NOT NULL,
    finding_refs_json TEXT NOT NULL,
    confidence TEXT NOT NULL,
    status TEXT NOT NULL,
    created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP
);

-- Immutable Derivation Ledger
CREATE TABLE artifact_derivations (
    id TEXT PRIMARY KEY,
    tenant_id TEXT NOT NULL,
    workflow_id TEXT NOT NULL,
    remediation_plan_id TEXT NOT NULL REFERENCES remediation_plans(id),
    attempt_number INTEGER NOT NULL,
    parent_artifact_id INTEGER NOT NULL REFERENCES file_instances(id),
    parent_sha256 TEXT NOT NULL,
    candidate_artifact_id INTEGER NOT NULL REFERENCES file_instances(id),
    candidate_sha256 TEXT NOT NULL,
    remediation_plan_hash TEXT NOT NULL,
    operation_types_json TEXT NOT NULL,
    agent_name TEXT NOT NULL,
    agent_version TEXT NOT NULL,
    policy_decision_id TEXT,
    policy_decision_hash TEXT,
    tool_manifest_hash TEXT,
    validator_version TEXT NOT NULL,
    validation_run_id TEXT NOT NULL,
    validation_outcome TEXT NOT NULL,
    findings_count INTEGER NOT NULL DEFAULT 0,
    blocking_findings_count INTEGER NOT NULL DEFAULT 0,
    derivation_hash TEXT NOT NULL,
    created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    UNIQUE(tenant_id, workflow_id, attempt_number)
);
```

---

## 4. Policy Engine Obligation: `SF-SAFE-004`

Every candidate generation operation is evaluated by the Go Policy Engine under rule `SF-SAFE-004`:
- **Domain**: `REMEDIATION`
- **Action**: `CREATE_CANDIDATE`
- **Effect**: `ALLOW_WITH_OBLIGATIONS`
- **Obligations Enforced**:
  1. `CandidateOnly`: May only create `CANDIDATE` status artifacts.
  2. `ImmutableParentRequired`: Asserts parent SHA-256 matches before and after write.
  3. `SandboxOnly`: Writes only to sandboxed candidate storage paths.
  4. `DeterministicRevalidation`: Immediate execution of authoritative NACHA validator on candidate bytes.
  5. `AuditRequired`: Derivation manifest recorded in `artifact_derivations`.
  6. `MaxAttempts`: Count $\le 3$.

---

## 5. Handoff to Independent Verification (Phase P08)

Once candidate revalidation passes and the workflow transitions to `AWAITING_VERIFICATION`, the candidate enters the **Independent Verification Control Plane (Phase P08)**:
1. **Physical Byte Re-Read**: The Go Verification Service re-reads raw candidate bytes from ObjectStore by SHA-256 address to confirm storage persistence and digest immutability.
2. **12 Typed Deterministic Checks**: Go independently verifies parent immutability, candidate 94-byte NACHA record alignment, derivation ledger hash integrity, dual-run zero-copy NACHA parsing, and active policy bundle freshness.
3. **Critic Review & Deterministic Dominance**: The read-only ADK `VerifierAgent` reviews structured evidence under prompt trust partitioning; deterministic checks dominate critic opinions in all conflict scenarios.
4. **Dual-Control Human Release**: Passing candidate artifacts transition to `VERIFIED` and are enqueued for 2-of-M dual-control human authorization before any bank transmission or ledger release occurs.

See [`docs/architecture/INDEPENDENT_VERIFICATION.md`](file:///c:/Users/Gathu/Projects/fintech/docs/architecture/INDEPENDENT_VERIFICATION.md) for full architectural specifications.

