# Handoff Report — Canonical Span Instrumentation & Test Infrastructure Survey

**Agent**: `explorer_survey_3`  
**Milestone**: Canonical Span Instrumentation and Test Infrastructure Survey  
**Date**: 2026-08-28  

---

## 1. Observation

1. **Telemetry Base Implementation (`ai-tier/observability/telemetry.py`)**:
   - Lines 21–32 define regex patterns for `NACHA_94_REGEX`, `ACCOUNT_REGEX`, `ROUTING_REGEX`, `SECRET_REGEX`, `CONTROL_CHARS_REGEX`, and `LINE_BREAK_REGEX`.
   - Lines 47–56 define `sanitize_span_attributes(attributes: Dict[str, Any]) -> Dict[str, Any]`.
   - Lines 58–80 define `MockSpan` and lines 82–101 define `MockTracer`.
   - Lines 106–110 define `get_tracer(instrument_name: str = "sentinelflow.default") -> MockTracer`.
   - Lines 113–119 define `configure_agent_observability()` setting `OTEL_INSTRUMENTATION_GENAI_CAPTURE_MESSAGE_CONTENT="NO_CONTENT"`, `ADK_CAPTURE_MESSAGE_CONTENT_IN_SPANS="false"`, and `OTEL_SEMCONV_STABILITY_OPT_IN="gen_ai_latest_experimental"`.

2. **Installed Dependencies**:
   - `python -c "import importlib.metadata; ..."` confirmed:
     - `opentelemetry-api`: `1.39.1`
     - `opentelemetry-sdk`: `1.39.1`
     - `opentelemetry-exporter-gcp-trace`: `1.15.0`
   - Verified that `from opentelemetry.exporter.cloud_trace import CloudTraceSpanExporter` and `from opentelemetry.trace.propagation.tracecontext import TraceContextTextMapPropagator` import cleanly.

3. **Guarded Model Boundary (`ai-tier/guardrails/boundary.py`)**:
   - `GuardedModelBoundary.invoke` (lines 137–514) executes a 7-step hardened lifecycle:
     - Step 1: Pre-invocation Data Minimization (lines 160–162)
     - Step 2: Active Evidence Set Initialization (lines 164–167)
     - Step 3: 4-Domain Trust Partitioning (lines 169–176)
     - Step 4: Pre-invocation Model Armor Screening (lines 178–224)
     - Step 5: Governed Model Invocation (`client.models.generate_content`) (lines 230–254)
     - Step 6: Post-invocation Model Armor Screening & Schema Validation (lines 256–327)
     - Step 7: Authoritative Evidence Grounding Verification (lines 328–342)
     - Step 8: Deterministic Rule-Grounded Fallback (lines 433–488)

4. **Tool Gateway Client (`ai-tier/tools/gateway_client.py`)**:
   - Lines 76–89 define `ToolGatewayContext` with `tenant_id`, `correlation_id`, `trace_id`, `caller_id`, `caller_autonomy_level`.
   - Lines 120–239 define `ToolGatewayClient.execute_tool()`, injecting `X-Sentinel-Tenant`, `X-Correlation-ID`, `X-Idempotency-Key`, and `X-Trace-ID`.

5. **W3C Trace Context Propagation in Go (`gateway/internal/telemetry/tracer.go` & `gateway/ai_client.go`)**:
   - `gateway/internal/telemetry/tracer.go` lines 20–21 define `TraceParentHeader = "traceparent"` and line 136 defines `func (s *Span) FormatW3CTraceParent() string`.
   - `gateway/ai_client.go` lines 122–124 set `X-Trace-ID` and `X-Correlation-ID` on outbound HTTP calls to the AI tier.

6. **Test Infrastructure & Existing Tests**:
   - `pytest ai-tier/tests/ -v` ran 111 tests in 214.69 seconds with 100% pass rate (`111 passed, 38 warnings`).
   - All tests run 100% offline with zero live Google Cloud API calls or credentials.
   - `test_observability.py` does not yet exist and is required to test real-path OpenTelemetry wrapper sanitization, gating, W3C extraction, and canonical spans.

7. **Capability Matrix Entry (`docs/CAPABILITY_MATRIX.yaml`)**:
   - Lines 376–379 record `live_agent_observability` with `status: IMPLEMENTED`, `evidence: ai-tier/observability/telemetry.py`.
   - Promotion to `TESTED` requires automated test coverage and the `test_command: "pytest ai-tier/tests/test_observability.py -v"`.

---

## 2. Logic Chain

1. **From Observations 1 & 2**:
   - The required OpenTelemetry SDK packages (`opentelemetry-api 1.39.1`, `opentelemetry-sdk 1.39.1`, `opentelemetry-exporter-gcp-trace 1.15.0`) are already installed in the environment.
   - `ai-tier/observability/telemetry.py` can be upgraded with an environment-gated `configure_agent_observability()` and real `TracerProvider` with `CloudTraceSpanExporter` when `SENTINEL_OTEL_ENABLED="true"`, while maintaining 100% identical baseline `MockTracer` behavior when `SENTINEL_OTEL_ENABLED` is false or unset.

2. **From Observations 1, 3, & 4**:
   - The 5 canonical spans align directly with the execution flow:
     - Root agent invocation -> `sentinelflow.agent.invoke`
     - Pre-screening in `GuardedModelBoundary` -> `sentinelflow.boundary.screen_input`
     - Model execution in `GuardedModelBoundary` -> `sentinelflow.boundary.model_call`
     - Post-screening in `GuardedModelBoundary` -> `sentinelflow.boundary.screen_output`
     - Tool Gateway dispatch in `ToolGatewayClient` -> `sentinelflow.toolgateway.execute`
   - In all spans, attribute sanitization via `sanitize_span_attributes()` ensures no raw NACHA, account numbers, routing numbers, or secrets leave the process boundary.

3. **From Observations 4 & 5**:
   - Cross-tier distributed tracing requires the Go gateway to inject `traceparent: 00-{trace_id}-{span_id}-01` on outbound requests to the AI tier.
   - Inbound AI tier requests parse `traceparent` using `TraceContextTextMapPropagator().extract(carrier=headers)`, attaching child spans to the Go control plane trace.

4. **From Observations 6 & 7**:
   - Creating `ai-tier/tests/test_observability.py` with in-memory exporter fixtures satisfies offline verification requirements and provides the required test evidence to promote `live_agent_observability` from `IMPLEMENTED` to `TESTED` in `docs/CAPABILITY_MATRIX.yaml`.

---

## 3. Caveats

- **Live GCP Trace Exporter Verification**: While `CloudTraceSpanExporter` is imported and verified, live transmission to Google Cloud Trace endpoints requires active GCP credentials (`GOOGLE_APPLICATION_CREDENTIALS` or ADC) and a configured project ID. In offline CI/CD test runs, this is mocked via `InMemorySpanExporter` and `MockTracer`.
- **Go Gateway Outbound `traceparent`**: Adding `req.Header.Set("traceparent", ...)` in `gateway/ai_client.go` and `gateway/agent_orchestrator.go` should preserve backward compatibility with existing `X-Trace-ID` and `X-Correlation-ID` headers.

---

## 4. Conclusion

The SentinelFlow AI tier and Go gateway have all architectural prerequisites in place for a production-grade OpenTelemetry Cloud Trace integration. The 5 canonical spans are mapped to clear lifecycle locations, dependencies are available, and the test suite provides complete offline guarantees. Promoting `live_agent_observability` in `docs/CAPABILITY_MATRIX.yaml` to `TESTED` requires the proposed `test_observability.py` test suite.

---

## 5. Verification Method

To independently verify the survey findings:

1. **Verify Python & Dependencies**:
   ```bash
   python -c "import importlib.metadata; print([importlib.metadata.version(p) for p in ['opentelemetry-api', 'opentelemetry-sdk', 'opentelemetry-exporter-gcp-trace']])"
   ```
   *Expected Output*: `['1.39.1', '1.39.1', '1.15.0']`

2. **Verify Existing AI Tier Test Suite**:
   ```bash
   pytest ai-tier/tests/ -v
   ```
   *Expected Output*: `111 passed` (zero failures, 100% offline).

3. **Verify Capability Matrix Entry**:
   Inspect `docs/CAPABILITY_MATRIX.yaml` lines 376–379 to confirm `live_agent_observability` status and description.

4. **Invalidation Conditions**:
   - Any failure in `pytest ai-tier/tests/ -v` without credentials.
   - Failure to import `CloudTraceSpanExporter` from `opentelemetry.exporter.cloud_trace`.
