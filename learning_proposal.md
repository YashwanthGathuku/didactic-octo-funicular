# SentinelFlow P06.5 Learning Proposal & Durable Multi-Agent Architectural Insights

## 1. ADK-Native vs ADK-Compatible Orchestration

**Pattern**: High-level agent frameworks often encourage wrapping entire multi-agent state machines in single black-box prompts or unconstrained transfer meshes where agents decide autonomously when to transfer control.

**Insight**: In zero-trust financial control planes, AI agents must be instantiated as genuine Google ADK `Agent` (`LlmAgent`) and `ParallelAgent` objects with discrete `output_key` bindings, but wrapped in a **deterministic outer orchestration shell**. This guarantees that:
- Automatic agent transfers cannot bypass the deterministic fixed roster allowlist (`["DiagnosisAgent", "PolicySLAAgent"]`).
- Specialist outputs are captured into typed Pydantic contracts rather than unparsed conversation history.
- The outer shell enforces mathematical evidence-grounding invariants across the entire delegation tree.

---

## 2. Durable Workflow Ownership & Authority Boundaries

**Pattern**: Multi-agent state is frequently held only in ephemeral Python memory or local process state, leading to catastrophic state loss and duplicate LLM execution upon process restarts.

**Insight**: Workflow identity, state transitions, domain events, and execution runs must be owned by durable persistence (SQLite/relational store) using optimistic locking (`row_version`).
- The Python AI tier executes bounded units of agent work.
- Every state transition emits a crash-consistent domain event (`WORKFLOW_CREATED`, `COMMANDER_PLAN_ACCEPTED`, `SPECIALIST_COMPLETED`, `SYNTHESIS_COMPLETED`).
- Duplicate event triggers resolve idempotently to the existing persisted workflow without re-running completed stages.

---

## 3. Restart-Safe Multi-Agent Execution & 7-Point Protected State Bindings

**Pattern**: In-memory caching of specialist results risks returning stale or invalid data if any governing context changed between execution attempts.

**Insight**: A cached specialist result is reusable across service restarts only when all 7 protected state bindings strictly match:
1. `workflow_id`
2. `agent_name`
3. `agent_version` / `manifest_hash`
4. `input_context_hash`
5. `artifact_sha256`
6. `policy_bundle_hash`
7. `authorized_evidence_set_hash`
If any protected binding changes, the entry is marked `STALE` and re-evaluated fail-closed.

---

## 4. Stale Policy and Resource Invalidation (TOCTOU Protection)

**Pattern**: Multi-agent planning and synthesis frequently suffer from Time-of-Check to Time-of-Use (TOCTOU) race conditions where policies or files change while specialists are executing.

**Insight**: SentinelFlow enforces two formal TOCTOU invariants:
- $\text{PolicyBundleHash}(\text{plan}) \neq \text{PolicyBundleHash}(\text{current}) \implies \text{OldPlanNotActionable}$: Synthesis fails closed with `outcome = "UNRESOLVED"` and emits `POLICY_CONTEXT_STALE`.
- $\text{ArtifactHash}(\text{plan}) \neq \text{ArtifactHash}(\text{current}) \implies \text{OldPlanNotActionable}$: Synthesis fails closed with `outcome = "UNRESOLVED"` and emits `RESOURCE_CONTEXT_STALE`.

---

## 5. Distinct DENY vs REQUIRE_HUMAN Authority Semantics

**Pattern**: AI assistants frequently treat policy engine `DENY` decisions as simple warnings or advisory flags that operators can click to approve or override.

**Insight**: Safety `DENY` decisions cannot be relaxed by lower policies or human approval.
- $\text{PolicyDecision} == \text{DENY} \implies \text{outcome} = \text{"POLICY_BLOCKED"}$. Human attention is attached strictly for investigation, not authorization.
- $\text{PolicyDecision} == \text{REQUIRE_HUMAN} \implies \text{outcome} = \text{"HUMAN_AUTHORIZATION_REQUIRED"}$. Dual-control human approval is permitted.
- If the AI model asserts `ALLOW` on a `DENY` file, the deterministic Policy Engine strictly dominates, and `agent_policy_disagreement_total` is incremented.
