# Governed ACH Return Intelligence Architecture — P12.5 Truth Gate

## Scope

P12.5 is a surgical correction to Phase P12. It does not add a subsystem, start P13, or deploy cloud infrastructure.

## Governing Invariants

`ReturnRiskAssessment != FinancialDecision`

`ReturnRiskAgent != ReturnAuthority`

`HistoricalReturnPattern != CurrentTransactionTruth`

`MemoryRecall != Evidence`

`ReturnTaxonomyGuidance != LegalDecision`

`ReturnRiskScore != ComplianceDecision`

`RiskHigh != AutoRejectFinancialFile` and `RiskLow != AutoReleaseFinancialFile`

## Governed Live Path

```text
Go deterministic return-risk result
        |
        v
ReturnRiskAgent
        |
        v
P09 GuardedModelBoundary
        |
        +--> data minimization
        +--> four-domain trust partition
        +--> Model Armor input screening (when configured)
        +--> Gemini 3.5 Flash structured inference
        +--> Model Armor output screening (when configured)
        +--> Pydantic schema validation
        +--> AuthorizedEvidenceSet grounding
        |
        v
ReturnRiskAgent deterministic-dominance check
        |
        v
Advisory ReturnRiskAssessment
```

`ReturnRiskAgent` does not instantiate or call `google.genai.Client`. The shared boundary is the sole provider path for this agent.

### Execution truth

- `LOCAL` / `DETERMINISTIC`: deterministic rule-grounded advisory output is permitted and labeled `LOCAL_ADK_DETERMINISTIC`.
- `LIVE`: boundary/provider failure is surfaced as typed failure (`GUARDRAIL_BLOCKED` or `PROVIDER_UNAVAILABLE` semantics); there is no silent deterministic success.
- `AUTO`: follows the existing common `GuardedModelBoundary` provider/fallback semantics. A deterministic fallback remains labeled deterministic, never live.

## Deterministic Risk Score

The authoritative Go score remains unchanged and uses exactly seven weighted features:

\[
R = 0.30C + 0.15F_7 + 0.10F_{30} + 0.15P + 0.10T + 0.10E + 0.10S
\]

Where:

- `C` = CodeSeverity
- `F7` = Frequency7d
- `F30` = Frequency30d
- `P` = PartnerReturnRate relative to an applicable public monitoring level
- `T` = RecentTrend
- `E` = Exposure
- `S` = SLA proximity

The following remain contextual/diagnostic and are not silently added to the score:

- `SameCodeRecurrence`
- `VerifiedPriorOccurrences`
- `SourceStrength`

## Public Return-Rate Monitoring Values

SentinelFlow's shared semantics fixture pins:

- unauthorized: `0.005` (0.5%)
- administrative: `0.030` (3.0%)
- overall: `0.150` (15.0%)

These are operational monitoring inputs. A threshold comparison does not itself create a compliance or financial decision.

## Representative MVP Taxonomy

The P12.5 catalog is representative, not complete. R51 is deliberately deferred from the MVP catalog.

| Code | Represented operational semantics | Window | Monitoring category |
|---|---|---|---|
| R01 | Insufficient funds | Standard 2 banking days | Overall 15% |
| R02 | Account closed | Standard 2 banking days | Administrative 3% |
| R03 | No account / unable to locate | Standard 2 banking days | Administrative 3% |
| R04 | Invalid account number structure | Standard 2 banking days | Administrative 3% |
| R05 | Unauthorized consumer debit using corporate SEC code | Extended 60 calendar days | Unauthorized 0.5% |
| R07 | Authorization revoked | Extended 60 calendar days | Unauthorized 0.5% |
| R08 | Payment stopped | Standard 2 banking days | Overall 15% |
| R10 | Originator not known to Receiver and/or Originator not authorized by Receiver to debit account | Extended 60 calendar days | Unauthorized 0.5% |
| R11 | Entry not in accordance with terms of authorization | Extended 60 calendar days | Unauthorized 0.5% |
| R16 | Current 2026 Account Frozen / Entry Returned Per OFAC Instruction semantics | Standard 2 banking days | Regulatory restricted; percentage N/A |
| R20 | Non-transaction account | Standard 2 banking days | Overall 15% |
| R29 | Corporate customer advises not authorized | Standard 2 banking days | Unauthorized 0.5% |

### R16 special handling

`ThresholdRegulatoryRestricted` does not fall through to a made-up comparison value. The risk feature vector exposes:

```text
partner_return_rate_threshold = 0
partner_return_rate_threshold_applicable = false
partner_return_rate contribution = 0
```

Code severity and the other six weighted features continue to drive the deterministic risk posture.

## Operational Guidance, Not Legal Adjudication

Taxonomy entries use typed guidance such as:

- `REVIEW_REQUIRED`
- `COMPLIANCE_REVIEW_REQUIRED`
- `AUTHORIZATION_REVIEW_REQUIRED`
- `DO_NOT_AUTOMATICALLY_REINITIATE`
- `CORRECTION_REQUIRED`
- `STANDARD_EXCEPTION_REVIEW`

Unsupported absolute legal conclusions were removed from the taxonomy. Every represented code carries public-source provenance metadata (`source_id`, `source_name`, `reference`, retrieval date, semantics-verified flag).

## Assessment Hash

P12.5 reuses `gateway/internal/policy/CanonicalJSON`, SentinelFlow's RFC 8785 JSON Canonicalization Scheme implementation.

The protected hash is:

```text
SHA256(RFC8785_CanonicalJSON({
  tenant_id,
  workflow_id,
  return_event_id,
  return_code,
  risk_score,
  risk_tier,
  contributions,
  feature_vector,
  engine_version
}))
```

`AssessmentID` and `ComputedAt` are record metadata and are deliberately excluded, so repeated calculations with the same protected inputs produce the same `AssessmentHash` while each stored assessment can retain a unique record identity.

## Submission Model Truth

The executable governed provider target is `gemini-3.5-flash`.

The capability matrix distinguishes:

- `gemini_3_5_provider_path: TESTED`
- `live_gemini_3_5: IMPLEMENTED`

The latter is not promoted to `TESTED` merely because mocks/unit tests exercise the provider path; an actual external live call requires separate evidence.

## Verification Gates

```bash
cd gateway && go test ./internal/returnrisk/... -v
cd gateway && go test ./internal/...
pytest ai-tier/tests/ -v
python ai-tier/evals/return_runner.py
python ai-tier/evals/runner.py
python scripts/generate_docs.py --check
```

No P12.5 claim should be upgraded by weakening these gates.
