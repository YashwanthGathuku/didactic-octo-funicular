# Runbook RB-07: AI Incident Analyst Provider Outage

## 1. Failure Scenario
- **Failure Mode**: Upstream AI model provider (e.g. OpenAI / Anthropic / Gemini API) experiences an outage, rate limit (HTTP 429), or network timeout.
- **Affected Components**: AI analysis tier (Prompt 15 consumer), `/api/v1/ready` probe `aiTier` check.

## 2. Expected State & System Behavior
- **Core Ingestion 100% Unaffected**:
  - Deterministic file ingestion, NACHA parsing, policy decision rules, database writes, outbox delivery, and dual-control human approvals proceed with 100% normal operation.
  - Zero payment processing traffic is blocked or delayed.
- **Readiness Probes**:
  - `/api/v1/ready` reports `ready: true` because AI is an optional, read-only analyst and never in the critical validation path.
  - Check details state: `checks: {"aiTier": {"status": "NOT_CONFIGURED" | "UNAVAILABLE", "required": false}}`.
- **AI Analyst Requests**:
  - Requests to the AI analyst endpoint return **HTTP 503 UNAVAILABLE** with clear error explanations rather than hallucinated or fabricated responses.

## 3. Guarantees & Tolerances
- **Data Loss Tolerance**: 0 (zero loss). No business state or financial decision relies on AI output.
- **Safety Invariant**: AI tier is strictly read-only and lacks credentials or tools to release, quarantine, or mutate financial state.

## 4. Telemetry & Alerts
- **Prometheus Metric**: `sentinel_dependency_errors_total{dependency="ai_tier"}` > 0.
- **Readiness Probe**: `aiTier.required` remains `false`.

## 5. Operator Action & Remediation
1. **Verify Deterministic Pipeline Continuity**:
   ```bash
   curl -s http://localhost:8080/api/v1/ready | jq .
   ```
2. **Inspect AI Model Rate Limits & Quotas**:
   Check upstream provider dashboard for quota limits or regional API degradation.
3. **Disable AI Tier in Gateway Config If Outage is Prolonged**:
   Set `SENTINEL_AI_TIER_URL=""` to stop outgoing probe attempts until provider resolves the issue.
