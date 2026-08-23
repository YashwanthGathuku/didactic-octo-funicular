# SentinelFlow — Current State (P12.5 Truth Gate)

**State date:** 23 August 2026  
**Scope:** P12.5 surgical correction only; no P13 and no cloud deployment.  
**Closure:** **IMPLEMENTATION CLOSED / LOCAL VERIFICATION PENDING**

## Authority Model

- `ReturnRiskAssessment != FinancialDecision`
- `ReturnRiskAgent != ReturnAuthority`
- `HistoricalReturnPattern != CurrentTransactionTruth`
- `MemoryRecall != Evidence`
- `ReturnTaxonomyGuidance != LegalDecision`
- `ReturnRiskScore != ComplianceDecision`
- `RiskHigh != AutoRejectFinancialFile`
- `RiskLow != AutoReleaseFinancialFile`

## P12.5 Corrected State

| Area | Current state | Evidence |
|---|---|---|
| Return-risk live inference | All ReturnRiskAgent live inference enters the shared P09 `GuardedModelBoundary`; the agent contains no direct `google.genai.Client` call | `ai-tier/agents/return_risk.py`, `ai-tier/tests/test_return_risk_agent.py` |
| Model Armor required-mode block | A blocked pre-screen terminates before model construction/invocation; test asserts Gemini client call count is zero | `ai-tier/tests/test_return_risk_agent.py` |
| Provider truth | `LIVE` failures surface typed unavailable/failure; `LOCAL`/`DETERMINISTIC` permit deterministic output; `AUTO` uses common boundary fallback semantics | `ai-tier/agents/return_risk.py` |
| Gemini target | Governed executable provider target is `gemini-3.5-flash`; live external execution remains `IMPLEMENTED`, not `TESTED`, unless separately observed | `docs/CAPABILITY_MATRIX.yaml` |
| Return-rate monitoring values | Unauthorized `0.5%`; administrative `3.0%`; overall `15.0%` | `gateway/internal/returnrisk/types.go`, `docs/fixtures/return_risk_semantics.json` |
| R10 | Current public semantics: Originator not known and/or not authorized by Receiver; extended 60-calendar-day return-window representation; unauthorized return-rate category | `gateway/internal/returnrisk/taxonomy.go` |
| R11 | Authorization exists but the Entry is not in accordance with its terms; operational category is `AUTHORIZATION_TERMS`, while return-rate monitoring remains in the unauthorized family | `gateway/internal/returnrisk/types.go`, `taxonomy.go`, shared fixture |
| R16 | Current 2026 public semantics retained; regulatory-restricted threshold category has `threshold_applicable=false` and no invented percentage contribution | `gateway/internal/returnrisk/engine.go`, `p12_5_test.go` |
| Taxonomy scope | Representative MVP catalog only; explicitly not a complete ACH return-code catalog; R51 deliberately deferred | `taxonomy.go`, shared fixture |
| Source provenance | Every representative taxonomy entry carries public Nacha source ID/name/reference/retrieval date/verification state; R05/R07/R10/R11/R29 and R16 have code-specific source pins | `taxonomy.go`, `p12_5_test.go` |
| Regulatory language | Taxonomy returns typed operational guidance; unsupported absolute legal conclusions and model-side adjudication authority are excluded | `types.go`, `taxonomy.go` |
| Risk formula | Seven weighted features remain unchanged: CodeSeverity, Frequency7d, Frequency30d, PartnerReturnRate, RecentTrend, Exposure, SLA | `engine.go` |
| Context-only features | SameCodeRecurrence, VerifiedPriorOccurrences, SourceStrength remain diagnostic/contextual and are not silently added to the score | `types.go`, `engine.go` |
| Assessment hash | SHA-256 over SentinelFlow RFC 8785 `policy.CanonicalJSON` protected deterministic fields; volatile AssessmentID/ComputedAt excluded | `engine.go`, `p12_5_test.go` |
| CI regression gate | Explicit P12.5 Go tests, full Go race suite, Python unit/conformance suite, return-risk evals, master evals, and generated-submission drift check | `.github/workflows/ci.yml` |

## Submission Truth

`docs/CAPABILITY_MATRIX.yaml` is the status source for submission-facing claims. It distinguishes a **TESTED governed Gemini 3.5 provider path** from **IMPLEMENTED live Gemini 3.5 external execution**. `scripts/generate_docs.py` verifies the generated `docs/DEVPOST_SUBMISSION.md`; it does not claim to regenerate every repository document.

## P12.5 Local Verification Commands

```bash
cd gateway
go test ./internal/returnrisk/... -v
go test ./internal/...
cd ..
pytest ai-tier/tests/ -v
python ai-tier/evals/return_runner.py
python ai-tier/evals/runner.py
python scripts/generate_docs.py --check
```

The P12.5 code implementation is closed. Record **VERIFIED CLOSED** only after the local commands above pass without test weakening. External live-provider execution is not inferred from unit tests.
