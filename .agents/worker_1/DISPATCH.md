## 2026-08-28T04:12:00Z

Implement the complete, production-grade OpenTelemetry tracing integration across the Python AI tier and Go Gateway according to requirements R1-R5:

1. ai-tier/observability/telemetry.py:
   - Environment-Gated Observability:
     - configure_agent_observability(project_id: Optional[str] = None) must inspect SENTINEL_OTEL_ENABLED (default "false" if unset).
     - When SENTINEL_OTEL_ENABLED is NOT "true", keep baseline behavior strictly identical: get_tracer() returns MockTracer and zero external network calls or exporter initialization occurs.
     - When SENTINEL_OTEL_ENABLED="true", configure a real OpenTelemetry TracerProvider with BatchSpanProcessor and CloudTraceSpanExporter (using from opentelemetry.exporter.cloud_trace import CloudTraceSpanExporter). Resolve project ID via parameter, GOOGLE_CLOUD_PROJECT env var, or google.auth.default().
     - get_tracer(instrument_name) must return MockTracer when disabled, and a wrapped real tracer (SanitizedTracer) when enabled.
   - PII Sanitization Span Wrapper:
     - Wrap OpenTelemetry real span (SanitizedSpan) such that set_attribute(key, value) and set_attributes(attributes) pass all keys and values through sanitize_span_attributes() before delegating to the underlying SDK span.
     - Support MockSpan.set_attributes() for interface parity.
     - Sanitization regexes must remain unmodified.
   - W3C Trace Context Helpers:
     - Implement extract_trace_context(headers: Dict[str, str]) and inject_trace_context(headers: Dict[str, str], context=None) using TraceContextTextMapPropagator.

2. Cross-Tier W3C Propagation & Canonical Spans:
   - Go Gateway (gateway/agent_orchestrator.go and gateway/ai_client.go): Inject standard W3C traceparent header on outbound HTTP calls to Python AI tier using span.FormatW3CTraceParent() while preserving existing headers (X-Trace-ID, etc.).
   - Python AI Tier (ai-tier/main.py, ai-tier/guardrails/boundary.py, ai-tier/tools/gateway_client.py):
     - Ensure incoming traceparent header is extracted in inbound request handlers so spans attach to the Go trace context.
     - Ensure the 5 canonical span names are instrumented:
       - sentinelflow.agent.invoke
       - sentinelflow.boundary.screen_input
       - sentinelflow.boundary.model_call
       - sentinelflow.boundary.screen_output
       - sentinelflow.toolgateway.execute

3. Dependency Declarations:
   - In ai-tier/requirements.txt and ai-tier/pyproject.toml, pin:
     opentelemetry-exporter-gcp-trace>=1.15.0,<2.0.0
     (and opentelemetry-api>=1.39.1,<2.0.0, opentelemetry-sdk>=1.39.1,<2.0.0).

4. Comprehensive Tests (ai-tier/tests/test_observability.py):
   - Add tests covering:
     a. Disabled mode (SENTINEL_OTEL_ENABLED="false" or unset) returns MockTracer and makes zero network calls.
     b. Real-path wrapper sanitizes sensitive attributes (routing numbers, SSNs, financial payloads) using InMemorySpanExporter.
     c. Inbound traceparent header is extracted and produces child spans sharing the parent trace_id.
     d. All 5 canonical span names and context hierarchy.
   - Run verification commands:
     - pytest ai-tier/tests/ -v (must pass 100% offline).
     - go test ./internal/telemetry -v and go test . -v (Go gateway tests pass).

5. Capability Matrix (docs/CAPABILITY_MATRIX.yaml):
   - Update live_agent_observability to TESTED with evidence and test commands.
