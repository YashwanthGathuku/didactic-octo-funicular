# SentinelFlow — All Things Agentic Hackathon Submission

## Project Overview
**SentinelFlow** is a governed AI Agent Control Plane built for **Gemini 3.5** and the **Google Agent Development Kit (ADK)** for high-assurance financial-file reliability, incident triage, and pre-ledger operational intelligence.

AI is intentionally separated from financial authority: deterministic Go services own validation, evidence, policy, risk scoring, verification, and state transitions; human dual control owns release decisions.

## Governed Specialist Fleet
1. **IncidentCommanderAgent** — bounded planning and synthesis.
2. **DiagnosisAgent** — evidence-grounded diagnostic hypotheses.
3. **PolicySLAAgent** — advisory explanation of deterministic policy/SLA context.
4. **MemoryAgent** — advisory historical retrieval; memory is not evidence.
5. **RemediationAgent** — proposal-only derived-artifact remediation.
6. **VerifierAgent** — verification critic; deterministic Go verification remains authoritative.
7. **ReturnRiskAgent** — A1 ACH return-intelligence specialist.

## Gemini & Guardrail Path
- **Gemini 3.5 Flash (`gemini-3.5-flash`)** is the executable provider target for the governed path.
- **GuardedModelBoundary** performs data minimization, trust partitioning, configured Model Armor pre/post screening, Pydantic validation, and evidence grounding around live inference.
- **LIVE** provider failure is surfaced as failure/unavailable; it is not silently relabeled as successful deterministic AI.
- **LOCAL/DETERMINISTIC** mode permits clearly labeled rule-grounded fallback.
- **AUTO** follows the common SentinelFlow boundary semantics.

## P12.5 Return Intelligence Truth Gate
- Public return-rate monitoring values represented by SentinelFlow: **0.5% unauthorized, 3.0% administrative, 15.0% overall**.
- R10 and R11 semantics are pinned by a shared public-semantics fixture; R11 participates in unauthorized-return-rate handling.
- R16 has **no invented percentage threshold**; threshold applicability is explicit and false for the regulatory-restricted category.
- The representative MVP taxonomy is **not a complete ACH return-code catalog**.
- Operational taxonomy guidance is not a legal decision, and a return-risk score is not a compliance decision.
- Assessment hashes reuse SentinelFlow's RFC 8785 canonical JSON implementation over deterministic protected fields; volatile record identity/time are excluded.

## Submission Capability Truth

| Capability | Status | Evidence | Description |
|---|---|---|---|
| `gemini_3_5_provider_path` | **TESTED** | `ai-tier/tests/test_return_risk_agent.py` | Governed Gemini 3.5 Flash provider path through GuardedModelBoundary; ReturnRiskAgent has no direct provider invocation path |
| `live_gemini_3_5` | **IMPLEMENTED** | `ai-tier/guardrails/boundary.py` | Gemini 3.5 Flash live invocation path is implemented; TESTED status requires separately observed external live-provider execution |
| `guarded_model_boundary` | **TESTED** | `ai-tier/guardrails/boundary.py` | Shared hardened model boundary with minimization, trust partitioning, pre/post screening, schema validation, grounding, and audit hashing |
| `deterministic_ach_return_intelligence` | **TESTED** | `gateway/internal/returnrisk/p12_5_test.go` | Seven-feature deterministic Go risk score, representative return-code taxonomy, explicit threshold applicability, and RFC 8785 protected assessment hashing |
| `return_risk_agent` | **TESTED** | `ai-tier/tests/test_return_risk_agent.py` | A1 read-only return-risk specialist uses only GuardedModelBoundary for live inference; LIVE failure cannot silently become deterministic success |
| `return_risk_public_semantics_fixture` | **TESTED** | `docs/fixtures/return_risk_semantics.json` | Shared Go/Python fixture pins public threshold values, R10/R11 semantics, unauthorized-rate family, R16 threshold non-applicability, and representative-catalog scope |
| `dual_control_release` | **TESTED** | `gateway/internal/review/review_test.go` | Separation-of-duties release requiring N approvals from distinct reviewers |
| `independent_verification` | **TESTED** | `gateway/internal/verification/service_test.go` | Independent Go verification re-reads ObjectStore bytes, re-runs validation, verifies derivation hashes, and enforces deterministic dominance |


## Authority Invariants

`ReturnRiskAssessment != FinancialDecision`

`MemoryRecall != Evidence`

`ReturnRiskScore != ComplianceDecision`

`RiskHigh != AutoRejectFinancialFile` and `RiskLow != AutoReleaseFinancialFile`
