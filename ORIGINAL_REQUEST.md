# Original User Request

## Initial Request — 2026-08-27T23:48:45-04:00

Add a production-grade OpenTelemetry tracing integration to Google Cloud Trace for the SentinelFlow AI tier (`ai-tier/observability/telemetry.py`), connecting deterministic Go gateway operations and Python multi-agent execution into unified distributed traces without breaking offline test guarantees.

Codebase Context:
- Target File: `ai-tier/observability/telemetry.py`
- Dependencies: `opentelemetry-sdk==1.39.1`, `opentelemetry-api==1.39.1`, `opentelemetry-exporter-gcp-trace==1.15.0` (all Apache 2.0)
- Verified Exporter Class: `from opentelemetry.exporter.cloud_trace import CloudTraceSpanExporter`
- Capability Matrix Entry: `live_agent_observability` in `docs/CAPABILITY_MATRIX.yaml`

Requirements:
R1. Environment-Gated Observability Configuration:
`configure_agent_observability()` in `ai-tier/observability/telemetry.py` must inspect `SENTINEL_OTEL_ENABLED` (default "false") and `GOOGLE_CLOUD_PROJECT` (or default Google Cloud project resolution).
When `SENTINEL_OTEL_ENABLED` is not "true", behavior must be strictly identical to baseline: `get_tracer()` returns `MockTracer` and zero external network calls or exporter initialization occurs.
When `SENTINEL_OTEL_ENABLED="true"`, configure a real OpenTelemetry `TracerProvider` with a `BatchSpanProcessor` and `CloudTraceSpanExporter`, and `get_tracer()` returns a real tracer.

R2. Sanitized Real Span Wrapper (PII Leak Prevention):
Wrap the OpenTelemetry real span object so that `set_attribute()` and `set_attributes()` pass all keys and values through `sanitize_span_attributes()` before delegating to the underlying SDK span. In production, span attributes leave the process boundary to Google Cloud Trace; unsanitized attributes constitute a critical PII egress violation. Sanitization regexes must remain unmodified.

R3. W3C Trace Context Propagation Across Go Gateway and Python AI Tier:
Implement W3C `traceparent` header extraction on inbound agent-tier HTTP/gRPC requests so that AI-tier spans join the same distributed trace initiated by the Go control plane. Ensure the Go gateway injects standard `traceparent` headers on outbound agent calls (`gateway/agent_orchestrator.go` or HTTP client).

R4. Canonical Span Instrumentation:
Ensure the following span names are accurately instrumented across the execution lifecycle:
- `sentinelflow.agent.invoke`
- `sentinelflow.boundary.screen_input`
- `sentinelflow.boundary.model_call`
- `sentinelflow.boundary.screen_output`
- `sentinelflow.toolgateway.execute`

R5. Test Verification and Dependency Pinning:
Pin `opentelemetry-exporter-gcp-trace>=1.15.0,<2.0.0` in `ai-tier/requirements.txt` and `ai-tier/pyproject.toml`. Add comprehensive tests in `ai-tier/tests/test_observability.py` covering:
1. Disabled mode (`SENTINEL_OTEL_ENABLED="false"`) returns `MockTracer`.
2. Real-path wrapper sanitizes sensitive attributes (routing numbers, SSNs, financial payloads) using an in-memory/fake exporter.
3. Inbound `traceparent` header is extracted and produces child spans sharing the parent `trace_id`.
4. All existing tests pass: `pytest ai-tier/tests/ -v`.

Acceptance Criteria:
- Environment Gating & Zero Offline Regression: `SENTINEL_OTEL_ENABLED` unset or "false" produces MockTracer from get_tracer(); `pytest ai-tier/tests/ -v` passes 100% offline without requiring Google Cloud credentials or live endpoints.
- PII Sanitization Guarantee: Every attribute set on real spans is sanitized via `sanitize_span_attributes()` prior to export; unit tests assert account numbers/sensitive patterns are masked.
- Distributed Trace Continuity: W3C traceparent injected from Go is parsed via TraceContextTextMapPropagator and child spans inherit the trace ID; span names match exact required catalog.
- Dependency Hygiene: opentelemetry-exporter-gcp-trace pinned in `ai-tier/requirements.txt` and `ai-tier/pyproject.toml`; `live_agent_observability` in `docs/CAPABILITY_MATRIX.yaml` is promoted with test evidence.
