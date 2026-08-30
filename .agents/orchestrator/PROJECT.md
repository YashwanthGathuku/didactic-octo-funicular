# Project: SentinelFlow OpenTelemetry Tracing Integration

## Architecture
SentinelFlow connects a deterministic Go control-plane gateway with a Python multi-agent execution tier. Distributed tracing spans across both tiers using OpenTelemetry W3C trace context propagation (`traceparent` header), exporting to Google Cloud Trace in production while guaranteeing 100% offline test execution in local and CI environments.

- **Go Gateway (`gateway/`)**: Initiates distributed trace spans with `FormatW3CTraceParent()` (`00-{trace_id}-{span_id}-01`) and injects the `traceparent` header on outbound HTTP requests to the Python AI tier.
- **Python AI Tier (`ai-tier/`)**:
  - `ai-tier/observability/telemetry.py`: Environment-gated tracer initialization via `configure_agent_observability()` and `get_tracer()`.
  - Privacy/Security Boundary: Real span wrapper (`SanitizedSpan`) intercepts `set_attribute()` and `set_attributes()` to sanitize all keys and values via `sanitize_span_attributes()` before passing to OpenTelemetry SDK spans.
  - Trace Context Extraction: `TraceContextTextMapPropagator` extracts inbound W3C `traceparent` headers, joining AI-tier execution into the Go control plane trace.
  - Canonical Instrumentation: Standardized 5-span execution hierarchy across agents, model boundary, and tool gateway.

## Feature Inventory
| # | Feature | Description | Milestone | Source |
|---|---------|-------------|-----------|--------|
| 1 | Environment-Gated Tracer Configuration | `configure_agent_observability()` and `get_tracer()` gate `CloudTraceSpanExporter` behind `SENTINEL_OTEL_ENABLED="true"` with project ID resolution. When false/unset, returns `MockTracer` with zero network egress. | M1 | ORIGINAL_REQUEST §R1 |
| 2 | Sanitized Real Span Wrapper | `SanitizedSpan` wrapper intercepts `set_attribute()` and `set_attributes()` to redact PII (routing numbers, SSNs, NACHA, secrets) before export to Cloud Trace. | M1 | ORIGINAL_REQUEST §R2 |
| 3 | Dependency Pinning | Pin `opentelemetry-exporter-gcp-trace>=1.15.0,<2.0.0` in `ai-tier/requirements.txt` and `ai-tier/pyproject.toml`. | M1 | ORIGINAL_REQUEST §R5 |
| 4 | Outbound W3C Traceparent Injection (Go) | Go gateway injects standard `traceparent` headers on outbound requests in `gateway/agent_orchestrator.go` and `gateway/ai_client.go`. | M2 | ORIGINAL_REQUEST §R3 |
| 5 | Inbound W3C Traceparent Extraction (Python) | AI tier extracts `traceparent` headers via `TraceContextTextMapPropagator` so child spans inherit parent `trace_id`. | M2 | ORIGINAL_REQUEST §R3 |
| 6 | Canonical Span Instrumentation | Standardize 5 canonical spans: `sentinelflow.agent.invoke`, `sentinelflow.boundary.screen_input`, `sentinelflow.boundary.model_call`, `sentinelflow.boundary.screen_output`, `sentinelflow.toolgateway.execute`. | M2 | ORIGINAL_REQUEST §R4 |
| 7 | Unit & Integration Test Suite | Comprehensive tests in `ai-tier/tests/test_observability.py` covering disabled mode, PII sanitization wrapper, context extraction, and offline execution. | M3 | ORIGINAL_REQUEST §R5 |
| 8 | Capability Matrix Promotion | Update `live_agent_observability` in `docs/CAPABILITY_MATRIX.yaml` to `TESTED` with evidence and test commands. | M3 | ORIGINAL_REQUEST §Acceptance Criteria |

## Milestones
| # | Name | Scope | Dependencies | Status |
|---|------|-------|-------------|--------|
| 1 | M1: Telemetry Core & PII Sanitization | `ai-tier/observability/telemetry.py`, `ai-tier/requirements.txt`, `ai-tier/pyproject.toml` | none | PLANNED |
| 2 | M2: W3C Propagation & Canonical Spans | `gateway/agent_orchestrator.go`, `gateway/ai_client.go`, `ai-tier/main.py`, `ai-tier/guardrails/boundary.py`, `ai-tier/tools/gateway_client.py` | M1 | PLANNED |
| 3 | M3: Verification, E2E Tests & Matrix | `ai-tier/tests/test_observability.py`, `docs/CAPABILITY_MATRIX.yaml` | M1, M2 | PLANNED |

## Interface Contracts
### Go Gateway ↔ Python AI Tier
- Protocol: HTTP/REST
- Header: `traceparent: 00-{trace_id_32_hex}-{span_id_16_hex}-01`
- Backward Compatibility: Maintain `X-Trace-ID` and `X-Correlation-ID` alongside `traceparent`.

### Telemetry Module Public API
- `configure_agent_observability(project_id: Optional[str] = None) -> None`
- `get_tracer(instrument_name: str = "sentinelflow.default") -> Union[MockTracer, SanitizedTracer]`
- `sanitize_span_attributes(attributes: Dict[str, Any]) -> Dict[str, Any]`
- `extract_trace_context(headers: Dict[str, str]) -> Context`
- `inject_trace_context(headers: Dict[str, str], context: Optional[Context] = None) -> Dict[str, str]`

## Code Layout
- `ai-tier/observability/telemetry.py`: Core OpenTelemetry integration, PII sanitization wrapper, mock tracer fallback.
- `ai-tier/requirements.txt`: Python package requirements.
- `ai-tier/pyproject.toml`: Python project metadata and dependencies.
- `gateway/agent_orchestrator.go`: Go orchestrator outbound HTTP client calls.
- `gateway/ai_client.go`: Go AI client outbound HTTP calls.
- `ai-tier/main.py`: FastAPI application entry points and W3C traceparent extraction middleware/dependencies.
- `ai-tier/guardrails/boundary.py`: Guarded model boundary span instrumentation.
- `ai-tier/tools/gateway_client.py`: Tool gateway client span instrumentation.
- `ai-tier/tests/test_observability.py`: Observability test suite.
- `docs/CAPABILITY_MATRIX.yaml`: SentinelFlow system capability status matrix.
