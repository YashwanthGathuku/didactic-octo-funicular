# Survey: W3C Trace Context Propagation Across Go Gateway and Python AI Tier

**Survey Date**: 2026-08-28  
**Investigator**: `explorer_survey_2`  
**Target Codebases**: `gateway/` (Go Control Plane) & `ai-tier/` (Python Specialist Fleet & ADK Runtime)  
**Reference Document**: `ORIGINAL_REQUEST.md` (Requirements R1–R5)

---

## Executive Summary

This survey establishes the complete technical blueprint for implementing end-to-end distributed tracing using W3C Trace Context (`traceparent` header standard) across the deterministic Go gateway and the Python AI tier. 

Currently:
1. **Go Gateway**: The gateway already contains a W3C `traceparent` parser and formatter in `gateway/internal/telemetry/tracer.go` (`ExtractCorrelationID` and `Span.FormatW3CTraceParent()`). However, outbound HTTP calls in `gateway/agent_orchestrator.go` (`ExecuteStage`) and `gateway/ai_client.go` (`TriageIncident`, `RunEvals`) do not yet inject the `traceparent` HTTP header.
2. **Python AI Tier**: The AI tier entry points (`ai-tier/main.py`) receive HTTP requests via FastAPI and dispatch to specialist agents, Model Armor boundaries (`ai-tier/guardrails/boundary.py`), and orchestrator pipelines (`ai-tier/orchestrator/fleet.py`). Telemetry is currently mocked in `ai-tier/observability/telemetry.py` with `MockTracer` and `MockSpan`. W3C context extraction via OpenTelemetry's `TraceContextTextMapPropagator` must be integrated into `ai-tier/observability/telemetry.py` and connected at the request/handler boundaries so child spans inherit the Go control plane's `trace_id`.

---

## 1. Go Gateway Outbound Communication & Trace Context Injection

### 1.1 Outbound Call Sites to Python AI Tier

The Go control plane communicates with the Python AI tier over HTTP JSON REST endpoints across two primary client implementations:

| File & Location | Method / Invocation | Target AI Tier Endpoint | Request Envelope / Body | Current Headers |
|---|---|---|---|---|
| `gateway/agent_orchestrator.go:122–161` | `AgentOrchestrator.ExecuteStage(ctx, req)` | `POST /internal/agents/stage/run` | `AgentStageRequest` (JSON) | `Content-Type`, `X-Sentinel-Tenant` |
| `gateway/ai_client.go:78–167` | `AIClient.TriageIncident(ctx, env)` | `POST /analyze` | `AgentContextEnvelope` (JSON) | `Content-Type`, `X-Correlation-ID`, `X-Sentinel-Tenant`, `X-Idempotency-Key`, `X-Trace-ID` |
| `gateway/ai_client.go:170–198` | `AIClient.RunEvals(ctx)` | `POST /evals/run` | Empty / nil body | None (Default HTTP) |

### 1.2 Orchestrator Stage Execution Lifecycle

In `gateway/agent_orchestrator.go:324–848`, `AgentOrchestrator.RunWorkflow()` drives the multi-stage AI reasoning workflow under Go control-plane authority across the following stages:
1. `StageCommanderPlan` (`/internal/agents/stage/run`)
2. `StageParallelSpecialists` (`/internal/agents/stage/run`)
3. `StageCommanderSynthesis` (`/internal/agents/stage/run`)
4. `StageRemediationPlan` (`/internal/agents/stage/run` — up to 3 bounded attempts)
5. `StageVerifierCritic` (`/internal/agents/stage/run`)

Each stage executes via `o.ExecuteStage(ctx, req)`.

### 1.3 Existing Gateway Telemetry Capabilities

In `gateway/internal/telemetry/tracer.go`:
- **W3C Format Constants**:
  - `TraceParentHeader = "traceparent"` (line 21)
  - `CorrelationIDHeader = "X-Correlation-ID"` (line 18)
- **Inbound Extraction**: `ExtractCorrelationID(r *http.Request) string` (lines 46–62) extracts 32-hex trace IDs from incoming `traceparent` headers (`00-{trace_id}-{parent_id}-{flags}`) or `X-Correlation-ID`.
- **Span Struct & W3C Formatter**:
  ```go
  // FormatW3CTraceParent returns a W3C traceparent string: 00-{traceid}-{spanid}-01
  func (s *Span) FormatW3CTraceParent() string {
      tid := s.TraceID
      if len(tid) < 32 {
          tid = fmt.Sprintf("%032s", tid)
      }
      return fmt.Sprintf("00-%s-%s-01", tid, s.SpanID)
  }
  ```
- **Context Attachment**: `WithCorrelationID(ctx, cid)` (line 78) and `GetCorrelationID(ctx)` (line 83).

### 1.4 Required Injection Points in Go Gateway

To ensure end-to-end trace continuity, standard `traceparent` headers must be injected in:

1. **`gateway/agent_orchestrator.go:ExecuteStage`**:
   ```go
   // Start or propagate span from ctx
   ctx, span := telemetry.StartSpan(ctx, "sentinelflow.gateway.stage_invoke")
   defer span.End()
   span.SetAttribute("stage_type", string(req.StageType))
   span.SetAttribute("workflow_id", req.WorkflowID)
   span.SetAttribute("tenant_id", req.TenantID)

   httpReq.Header.Set(telemetry.TraceParentHeader, span.FormatW3CTraceParent())
   httpReq.Header.Set(telemetry.CorrelationIDHeader, span.TraceID)
   if req.TraceID != "" {
       httpReq.Header.Set("X-Trace-ID", req.TraceID)
   }
   ```

2. **`gateway/ai_client.go:TriageIncident`**:
   ```go
   traceID := env.TraceID
   if traceID == "" {
       traceID = telemetry.GetCorrelationID(ctx)
   }
   if traceID == "" {
       traceID = env.CorrelationID
   }
   if len(traceID) < 32 {
       traceID = fmt.Sprintf("%032s", traceID)
   }
   spanID := telemetry.GenerateOpaqueID(8)
   traceParent := fmt.Sprintf("00-%s-%s-01", traceID, spanID)
   req.Header.Set("traceparent", traceParent)
   ```

3. **`gateway/ai_client.go:RunEvals`**:
   ```go
   traceID := telemetry.GetCorrelationID(ctx)
   if traceID != "" {
       if len(traceID) < 32 {
           traceID = fmt.Sprintf("%032s", traceID)
       }
       spanID := telemetry.GenerateOpaqueID(8)
       req.Header.Set("traceparent", fmt.Sprintf("00-%s-%s-01", traceID, spanID))
   }
   ```

---

## 2. Python AI Tier Entry Points, Handlers & Trace Context Extraction

### 2.1 Server Handlers in `ai-tier/main.py`

The FastAPI server exposes the following inbound endpoints:

| Endpoint | Handler Function | Input Request Model | Downstream Processor |
|---|---|---|---|
| `POST /analyze` | `analyze_incident` | `AgentContextEnvelope` \| `GatewayTriageRequest` | Model Armor $\rightarrow$ `generate_ai_analysis()` $\rightarrow$ Model Armor |
| `POST /orchestrate` | `orchestrate_agent_fleet` | `EvidenceEnvelope` | Model Armor $\rightarrow$ `generate_ai_analysis()` $\rightarrow$ Model Armor |
| `POST /agents/diagnosis/run` | `run_diagnosis_agent` | `AgentContextEnvelope` | Model Armor $\rightarrow$ `DiagnosisAgent.run()` |
| `POST /agents/workflows/run` | `run_multi_agent_workflow` | `AgentContextEnvelope` | Model Armor $\rightarrow$ `MultiAgentWorkflowOrchestrator.run_workflow()` |
| `POST /internal/agents/stage/run` | `run_agent_stage` | `AgentStageRequest` | `MultiAgentWorkflowOrchestrator.execute_stage()` |
| `POST /agents/verifier/run` | `run_verifier_critic` | `AgentStageRequest` \| `AgentContextEnvelope` | `VerifierAgent.run()` |
| `GET /evals/run` | `get_evals_summary` | None | `run_adversarial_evals()` |

### 2.2 Trace Context Extraction via `TraceContextTextMapPropagator`

OpenTelemetry provides `opentelemetry.trace.propagation.tracecontext.TraceContextTextMapPropagator` in `opentelemetry-api`.

#### Mechanism of Extraction:
1. An incoming HTTP request carrier (dictionary containing `{"traceparent": "00-4bf92f3577b34da6a3ce929d0e0e4736-00f067aa0ba902b7-01", ...}`) is passed to `TraceContextTextMapPropagator().extract(carrier)`.
2. The propagator decodes:
   - Version: `00`
   - Trace ID: `4bf92f3577b34da6a3ce929d0e0e4736` ($128\text{-bit integer}$)
   - Parent Span ID: `00f067aa0ba902b7` ($64\text{-bit integer}$)
   - Trace Flags: `01` (Sampled)
3. It constructs an OpenTelemetry `Context` containing a remote `SpanContext`.
4. When `tracer.start_as_current_span(name, context=extracted_context)` is invoked:
   - The created span inherits `trace_id = 0x4bf92f3577b34da6a3ce929d0e0e4736`.
   - The created span sets `parent_span_id = 0x00f067aa0ba902b7`.
   - The created span is assigned a fresh local span ID.

#### Implementation Architecture in `ai-tier/observability/telemetry.py`:

```python
from opentelemetry import context as otel_context
from opentelemetry.trace.propagation.tracecontext import TraceContextTextMapPropagator

def extract_trace_context(
    carrier: Optional[Dict[str, str]] = None,
    traceparent: Optional[str] = None,
    trace_id: Optional[str] = None,
) -> Optional[Any]:
    """Extract OpenTelemetry context from W3C traceparent or carrier dictionary."""
    headers: Dict[str, str] = {}
    if carrier:
        headers.update({k.lower(): str(v) for k, v in carrier.items()})
    if traceparent:
        headers["traceparent"] = traceparent
    elif "traceparent" not in headers and trace_id:
        # Fallback synthesis for requests carrying only trace_id
        clean_tid = trace_id.replace("-", "").lower()
        if len(clean_tid) < 32:
            clean_tid = clean_tid.zfill(32)
        headers["traceparent"] = f"00-{clean_tid[:32]}-0000000000000001-01"

    if "traceparent" in headers:
        try:
            return TraceContextTextMapPropagator().extract(carrier=headers)
        except Exception as e:
            logger.debug("Failed to extract traceparent: %s", e)
            return None
    return None
```

#### FastAPI Middleware Integration in `ai-tier/main.py`:

```python
from fastapi import Request
from observability.telemetry import extract_trace_context
from opentelemetry import context as otel_context

@app.middleware("http")
async def otel_trace_propagation_middleware(request: Request, call_next):
    carrier = dict(request.headers)
    ctx = extract_trace_context(carrier)
    if ctx is not None:
        token = otel_context.attach(ctx)
        try:
            response = await call_next(request)
            return response
        finally:
            otel_context.detach(token)
    return await call_next(request)
```

---

## 3. Data Structures, Carrier Mappings, and Context Propagators

### 3.1 Data Structures Inventory

| System Component | Struct / Class | File Location | Tracing-Relevant Fields |
|---|---|---|---|
| **Go Gateway** | `telemetry.Span` | `gateway/internal/telemetry/tracer.go:27–37` | `TraceID`, `SpanID`, `ParentID`, `Name`, `Attributes`, `FormatW3CTraceParent()` |
| **Go Gateway** | `AgentStageRequest` | `gateway/agent_orchestrator.go:30–51` | `TraceID string`, `CorrelationID string`, `WorkflowID string`, `TenantID string` |
| **Go Gateway** | `AgentContextEnvelope` | `gateway/agent_context.go:42–61` | `TraceID string`, `CorrelationID string`, `WorkflowID string`, `TenantID string` |
| **Go Gateway** | `domain.AgentWorkflow` | `gateway/internal/domain/agent_workflow.go:13–35` | `TraceID string`, `CorrelationID string`, `TriggerEventID string` |
| **Python AI Tier** | `AgentContextEnvelope` | `ai-tier/models/envelope.py:44–73` | `trace_id: str`, `correlation_id: str`, `workflow_id: str`, `tenant_id: str` |
| **Python AI Tier** | `AgentStageRequest` | `ai-tier/contracts/orchestration.py:213–253` | `trace_id: Optional[str]`, `correlation_id: str`, `workflow_id: str` |
| **Python AI Tier** | `AgentHandoffEnvelope` | `ai-tier/contracts/orchestration.py:38–65` | `trace_id: str`, `correlation_id: str`, `workflow_id: str` |
| **Python AI Tier** | `MockSpan` / `MockTracer` | `ai-tier/observability/telemetry.py:58–110` | `name`, `attributes`, `status`, `start_as_current_span()` |
| **Python AI Tier** | `SanitizedSpanWrapper` | `ai-tier/observability/telemetry.py` (to build) | Real `Span` wrapper delegating `set_attribute` / `set_attributes` to `sanitize_span_attributes` |

### 3.2 Carrier Mappings

1. **HTTP Wire Format (Standard W3C)**:
   - Header: `traceparent: 00-{32-hex-trace-id}-{16-hex-span-id}-{2-hex-flags}`
   - Header: `tracestate: [vendor-specific key-value pairs]`
   - Header: `X-Correlation-ID: [opaque string]`
   - Header: `X-Trace-ID: [32-hex string]`
   - Header: `X-Sentinel-Tenant: [tenant identifier]`
   - Header: `X-Idempotency-Key: [unique idempotency key]`

2. **In-Memory Payload Format**:
   - In Go struct fields: `req.TraceID`, `env.TraceID`
   - In Python Pydantic fields: `req.trace_id`, `envelope.trace_id`, `handoff.trace_id`

### 3.3 Canonical Span Instrumentation Catalog (Requirement R4)

The OpenTelemetry integration must instrument the following five canonical span names:

| Canonical Span Name | Execution Boundary | Source Location in AI Tier | Required Span Attributes (Sanitized) |
|---|---|---|---|
| `sentinelflow.agent.invoke` | AI Agent stage reasoning & dispatch | `ai-tier/orchestrator/fleet.py` & `ai-tier/agents/*.py` | `agent.name`, `tenant.id`, `workflow.id`, `incident.id`, `stage.type` |
| `sentinelflow.boundary.screen_input` | Pre-invocation Model Armor & data minimization | `ai-tier/guardrails/boundary.py:138–200` & `ai-tier/main.py:231, 279, 366, 403` | `guardrail.type`, `guardrail.decision`, `tenant.id` |
| `sentinelflow.boundary.model_call` | LLM invocation (Gemini API or fallback) | `ai-tier/guardrails/boundary.py:230–250` & `ai-tier/llm_client.py:126–220` | `gen_ai.system`, `gen_ai.request.model`, `gen_ai.usage.input_tokens`, `gen_ai.usage.output_tokens` |
| `sentinelflow.boundary.screen_output` | Post-invocation Model Armor & grounding verification | `ai-tier/guardrails/boundary.py:255–340` & `ai-tier/main.py:255, 317` | `guardrail.type`, `guardrail.decision`, `grounding.verdict` |
| `sentinelflow.toolgateway.execute` | Governed tool execution | `ai-tier/agents/tools.py` & `gateway/internal/toolgateway/service.go` | `tool.id`, `tool.version`, `tenant.id`, `caller.id` |

---

## 4. Verification & Testing Requirements

To satisfy Requirement R5 and acceptance criteria:
1. **Disabled Mode Test**: When `SENTINEL_OTEL_ENABLED="false"` or unset:
   - `get_tracer()` returns `MockTracer`.
   - Zero GCP Cloud Trace SDK network calls or exporter initialization occurs.
   - `pytest ai-tier/tests/ -v` passes 100% offline.
2. **PII Sanitization Test**:
   - `SanitizedSpanWrapper.set_attribute()` and `set_attributes()` pass all values through `sanitize_span_attributes()`.
   - Tests assert that routing numbers (`123456789`), account numbers (`123456789012`), NACHA 94-char records, and secret tokens are masked to `[ROUTING_REDACTED]`, `[ACCOUNT_REDACTED]`, `[NACHA_RECORD_REDACTED]`, `[SECRET_REDACTED]`.
3. **Trace Context Propagation Test**:
   - Given an inbound request with `traceparent: 00-4bf92f3577b34da6a3ce929d0e0e4736-00f067aa0ba902b7-01`:
   - Extraction via `TraceContextTextMapPropagator` produces spans where `span.context.trace_id == 0x4bf92f3577b34da6a3ce929d0e0e4736`.
   - Child span ID is distinct and parent span ID matches `0x00f067aa0ba902b7`.
4. **All Existing Suites Pass**:
   - `pytest ai-tier/tests/ -q` (all 111 baseline tests pass).
   - `go test ./internal/telemetry -v` and `go test . -run "TestAgent" -v` pass.

---

## 5. Conclusion & Recommendations

1. **Go Gateway**: Update `ExecuteStage` in `gateway/agent_orchestrator.go` and `TriageIncident` in `gateway/ai_client.go` to inject standard `traceparent` headers using `telemetry.Span.FormatW3CTraceParent()` or `ExtractCorrelationID`/`GetCorrelationID`.
2. **Python AI Tier**: Enhance `ai-tier/observability/telemetry.py` with:
   - `configure_agent_observability()` supporting `SENTINEL_OTEL_ENABLED="true"` $\rightarrow$ `CloudTraceSpanExporter` + `BatchSpanProcessor`.
   - `SanitizedSpanWrapper` wrapping OpenTelemetry spans to sanitize all attributes.
   - `extract_trace_context()` using `TraceContextTextMapPropagator`.
   - Add FastAPI middleware in `ai-tier/main.py`.
   - Pin `opentelemetry-exporter-gcp-trace>=1.15.0,<2.0.0` in `ai-tier/requirements.txt` and `ai-tier/pyproject.toml`.
   - Add comprehensive tests in `ai-tier/tests/test_observability.py`.
