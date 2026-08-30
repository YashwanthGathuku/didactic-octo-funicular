# Canonical Span Instrumentation & Test Infrastructure Survey

**Date**: 2026-08-28  
**Surveyor**: `explorer_survey_3`  
**Target Project**: SentinelFlow AI Tier (`ai-tier/`) & Gateway (`gateway/`)  
**Scope**: Canonical Span Instrumentation, Test Infrastructure, Capability Matrix, and Offline Verification Guarantees.

---

## Executive Summary

This survey evaluates the observability architecture, canonical OpenTelemetry span instrumentation points, test infrastructure conventions, and capability promotion criteria for the SentinelFlow AI tier.

Key Findings:
1. **Canonical Spans**: The 5 required canonical span names (`sentinelflow.agent.invoke`, `sentinelflow.boundary.screen_input`, `sentinelflow.boundary.model_call`, `sentinelflow.boundary.screen_output`, `sentinelflow.toolgateway.execute`) map directly to distinct lifecycle phases across `ai-tier/guardrails/boundary.py`, `ai-tier/agents/`, `ai-tier/orchestrator/fleet.py`, and `ai-tier/tools/gateway_client.py`.
2. **Current Telemetry Status**: `ai-tier/observability/telemetry.py` provides a privacy-preserving baseline with `sanitize_span_attributes()`, `MockSpan`, and `MockTracer`. Real OpenTelemetry integration (`TracerProvider`, `CloudTraceSpanExporter`, `BatchSpanProcessor`, and sanitized real span wrapper) is environment-gated by `SENTINEL_OTEL_ENABLED`.
3. **W3C Context Propagation**: The Go gateway already possesses W3C `traceparent` formatting utilities in `gateway/internal/telemetry/tracer.go` (`FormatW3CTraceParent`). Propagating `traceparent` headers to outbound AI HTTP calls and extracting them in Python via `TraceContextTextMapPropagator` completes cross-language distributed trace continuity.
4. **Test Infrastructure**: `ai-tier/tests/` contains 25 test files with 111 passing tests running 100% offline under `pytest 8.3.4`. A dedicated `test_observability.py` test suite is needed to cover the 4 observability requirements (gating, sanitization, W3C propagation, and canonical span names).
5. **Capability Matrix**: `live_agent_observability` is currently `IMPLEMENTED` in `docs/CAPABILITY_MATRIX.yaml`. Adding automated unit/integration tests with `test_command: "pytest ai-tier/tests/test_observability.py -v"` fulfills the promotion requirement to `TESTED`.

---

## 1. Canonical Span Instrumentation Mapping

The 5 canonical spans correspond to specific operational boundaries in SentinelFlow:

| Canonical Span Name | Execution Lifecycle Phase | Exact Source Location | Required Attributes |
|---|---|---|---|
| `sentinelflow.agent.invoke` | Agent invocation / Reasoning stage dispatch | `ai-tier/agents/diagnosis.py`<br>`ai-tier/agents/commander.py`<br>`ai-tier/agents/policy_sla.py`<br>`ai-tier/agents/remediation.py`<br>`ai-tier/agents/verifier.py`<br>`ai-tier/agents/return_risk.py`<br>`ai-tier/agents/memory_agent.py`<br>`ai-tier/orchestrator/fleet.py` | `agent.name`, `tenant.id`, `workflow.id`, `incident.id`, `correlation.id`, `stage.type`, `autonomy_level`, `execution_source` |
| `sentinelflow.boundary.screen_input` | Pre-invocation Data Minimization & Model Armor Screening | `ai-tier/guardrails/boundary.py` (`GuardedModelBoundary.invoke`) | `tenant.id`, `correlation.id`, `guardrail.mode`, `guardrail.input_decision`, `pre_guardrail_input_hash`, `post_guardrail_input_hash` |
| `sentinelflow.boundary.model_call` | Governed LLM Inference (Gemini 3.5 Flash / Fallback) | `ai-tier/guardrails/boundary.py` (`GuardedModelBoundary.invoke`) | `gen_ai.system`, `gen_ai.request.model`, `model.name`, `provider`, `execution_source`, `prompt_tokens`, `completion_tokens`, `total_tokens`, `latency_ms` |
| `sentinelflow.boundary.screen_output` | Post-invocation Model Armor Screening, Schema Validation & Grounding | `ai-tier/guardrails/boundary.py` (`GuardedModelBoundary.invoke`) | `tenant.id`, `correlation.id`, `guardrail.output_decision`, `grounding.verdict`, `model_output_hash`, `post_guardrail_output_hash` |
| `sentinelflow.toolgateway.execute` | Tool Gateway Client Dispatch | `ai-tier/tools/gateway_client.py` (`ToolGatewayClient.execute_tool`)<br>`ai-tier/runtime/gateway_client.py` | `tool.id`, `tool.version`, `tenant.id`, `correlation.id`, `caller.id`, `caller.autonomy_level`, `idempotency_key`, `duration_ms`, `status` |

### Detailed Lifecycle Trace

```
Inbound HTTP/gRPC Request (Go Control Plane -> Python AI Tier)
  │
  ├─► [W3C TraceContext Extraction] (traceparent -> OpenTelemetry Context)
  │
  ├─► SPAN 1: `sentinelflow.agent.invoke` (Agent Entry Point)
  │     │
  │     ├─► SPAN 2: `sentinelflow.boundary.screen_input` (GuardedModelBoundary Pre-Screen)
  │     │     ├─ Data Minimization (masking 94-char NACHA, account, routing numbers)
  │     │     ├─ 4-Domain Trust Partitioning
  │     │     └─ Model Armor Screen Prompt (PII / Injection / Jailbreak)
  │     │
  │     ├─► SPAN 3: `sentinelflow.boundary.model_call` (Governed Invocation)
  │     │     ├─ Gemini 3.5 Flash (`generate_content`) or Fallback
  │     │     └─ Token usage & latency recording
  │     │
  │     ├─► SPAN 4: `sentinelflow.boundary.screen_output` (GuardedModelBoundary Post-Screen)
  │     │     ├─ Model Armor Screen Response (PII leakage defense)
  │     │     ├─ Structured Output Schema Validation (Pydantic)
  │     │     └─ Evidence Grounding Verification (ClaimedCitations ⊆ AuthorizedEvidenceSet)
  │     │
  │     └─► SPAN 5: `sentinelflow.toolgateway.execute` (Tool Dispatch)
  │           ├─ Client headers: X-Sentinel-Tenant, X-Trace-ID, X-Correlation-ID, traceparent
  │           └─ Go Tool Gateway authorization & execution
  │
  └─► Return Structured Response (DiagnosisRunResponse, StageResponse, etc.)
```

---

## 2. Test Infrastructure Analysis (`ai-tier/tests/`)

### Test Framework & Environment
- **Runner**: `pytest` 8.3.4 with `pytest-asyncio` 0.25.0
- **Python Version**: 3.13.5 (configured for `>=3.11` in `pyproject.toml`)
- **Execution Mode**: `asyncio_mode = "auto"`, `pythonpath = ["."]`
- **Baseline Test Count**: 111 automated tests across 25 test files (all 111 pass offline).

### Mocking & Isolation Strategy
1. **MockTracer & MockSpan**:
   - Implemented in `ai-tier/observability/telemetry.py` (lines 58–110).
   - In offline test mode (`SENTINEL_OTEL_ENABLED="false"` or unset), `get_tracer()` returns `MockTracer`.
   - Zero background threads, zero gRPC channels, and zero outbound network connections are initiated.
2. **Model Armor & Memory Mock Providers**:
   - `GoogleModelArmorProvider` supports offline test modes with explicit fault injection (`inject_fault("TIMEOUT")`, `inject_fault("UNAVAILABLE")`, `inject_fault("EXPLICIT_BLOCK")`).
   - `MockManagedMemoryProvider` (`ai-tier/memory/mock_provider.py`) provides in-memory multi-tenant memory isolation.
3. **Environment Isolation via Monkeypatch**:
   - Tests use `monkeypatch.setenv()` and `monkeypatch.delenv()` to toggle execution modes (`SENTINEL_AI_MODE`, `SENTINEL_PLATFORM_MODE`, `SENTINEL_OTEL_ENABLED`).
4. **Planned Test Suite (`test_observability.py`)**:
   - Test 1: Disabled mode (`SENTINEL_OTEL_ENABLED="false"`) returns `MockTracer` and avoids any exporter initialization.
   - Test 2: Real-path wrapper sanitizes sensitive attributes (routing numbers, SSNs, financial payloads) using `InMemorySpanExporter`.
   - Test 3: Inbound `traceparent` extraction extracts parent `trace_id` into child spans.
   - Test 4: All 5 canonical spans are generated with sanitized attributes during full agent invocation lifecycle.

---

## 3. Capability Matrix Analysis (`docs/CAPABILITY_MATRIX.yaml`)

### Current State
In `docs/CAPABILITY_MATRIX.yaml`:
```yaml
  agent_observability:
    status: TESTED
    evidence: ai-tier/observability/telemetry.py
    description: "Privacy-preserving OpenTelemetry instrumentation separates allowed trace metadata from prompt payloads; live Cloud Trace proof remains separate"
    test_command: "pytest ai-tier/tests/test_platform_runtime.py -v"

  live_agent_observability:
    status: IMPLEMENTED
    evidence: ai-tier/observability/telemetry.py
    description: "Cloud-compatible tracing instrumentation exists; TESTED requires an observed managed trace ID from Agent Runtime"
```

### Promotion Requirements to `TESTED`
Per SentinelFlow capability governance (lines 4–9 of `CAPABILITY_MATRIX.yaml`), `TESTED` status requires:
1. Production-grade code implementation with Cloud Trace integration.
2. Comprehensive automated unit and integration tests covering real-path initialization, PII attribute sanitization, W3C trace context extraction, and canonical span names.
3. Updating `docs/CAPABILITY_MATRIX.yaml` entry `live_agent_observability`:
   - Set `status: TESTED`
   - Set `test_command: "pytest ai-tier/tests/test_observability.py -v"`
   - Reference `evidence: ai-tier/observability/telemetry.py`

---

## 4. Verification Requirements & Configuration

### Exact Test Commands
- **Full AI Tier Suite**: `pytest ai-tier/tests/ -v`
- **Observability Tests**: `pytest ai-tier/tests/test_observability.py -v`
- **Platform Runtime Tests**: `pytest ai-tier/tests/test_platform_runtime.py -v`
- **Adversarial Platform Evals**: `python ai-tier/evals/platform_runner.py`
- **Go Gateway Telemetry Suite**: `go test -v ./internal/telemetry/`

### Environment Variables & Defaults

| Variable | Default Value | Purpose |
|---|---|---|
| `SENTINEL_OTEL_ENABLED` | `"false"` | Master switch: when `"true"`, enables real OpenTelemetry SDK with `CloudTraceSpanExporter` |
| `GOOGLE_CLOUD_PROJECT` | Dynamic / `telos-agent` | Target Google Cloud project for Cloud Trace export |
| `OTEL_INSTRUMENTATION_GENAI_CAPTURE_MESSAGE_CONTENT` | `"NO_CONTENT"` | Guarantees raw prompt / completion text is omitted from GenAI spans |
| `ADK_CAPTURE_MESSAGE_CONTENT_IN_SPANS` | `"false"` | Disables prompt capture in Google ADK tracing |
| `OTEL_SEMCONV_STABILITY_OPT_IN` | `"gen_ai_latest_experimental"` | Opts into standard GenAI OpenTelemetry semantic conventions |
| `SENTINEL_AI_MODE` | `"auto"` | Controls AI tier execution (`auto`, `live`, `local`) |

### Offline Verification Guarantees
- **Zero-Network Invariant**: Unset or `"false"` `SENTINEL_OTEL_ENABLED` ensures no GCP credentials or endpoints are queried.
- **PII Leak Prevention**: All span attributes pass through `sanitize_span_attributes()` before leaving the process:
  - Masking NACHA 94-char records -> `[NACHA_RECORD_REDACTED]`
  - Masking account numbers (10–17 digits) -> `[ACCOUNT_REDACTED]`
  - Masking ABA routing numbers (9 digits) -> `[ROUTING_REDACTED]`
  - Masking secrets & API tokens -> `[SECRET_REDACTED]`
  - Stripping control characters and collapsing newlines to prevent log/trace record injection.
