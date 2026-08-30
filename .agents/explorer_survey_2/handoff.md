# Handoff Report: W3C Trace Context Propagation Survey

**Agent**: `explorer_survey_2`  
**Working Directory**: `C:\Users\Gathu\Projects\fintech\.agents\explorer_survey_2`  
**Date**: 2026-08-28  
**Handoff Type**: Hard (Task Complete)

---

## 1. Observation

1. **Go Gateway Outbound Call Sites**:
   - `gateway/agent_orchestrator.go:128-135`:
     ```go
     url := fmt.Sprintf("%s/internal/agents/stage/run", o.aiTierURL)
     httpReq, err := http.NewRequestWithContext(ctx, "POST", url, bytes.NewReader(reqBytes))
     if err != nil {
         return nil, fmt.Errorf("create http request: %w", err)
     }
     httpReq.Header.Set("Content-Type", "application/json")
     httpReq.Header.Set("X-Sentinel-Tenant", req.TenantID)
     ```
     `ExecuteStage` creates the HTTP request and only sets `Content-Type` and `X-Sentinel-Tenant`. It does not set `traceparent`, `X-Correlation-ID`, or `X-Trace-ID`.
   - `gateway/ai_client.go:118-124`:
     ```go
     req.Header.Set("Content-Type", "application/json")
     req.Header.Set("X-Correlation-ID", env.CorrelationID)
     req.Header.Set("X-Sentinel-Tenant", env.TenantID)
     req.Header.Set("X-Idempotency-Key", idempotencyKey)
     if env.TraceID != "" {
         req.Header.Set("X-Trace-ID", env.TraceID)
     }
     ```
     `AIClient.TriageIncident` sets `X-Correlation-ID`, `X-Sentinel-Tenant`, `X-Idempotency-Key`, and `X-Trace-ID`, but does not set standard W3C `traceparent`.

2. **Go Gateway Inbound Tracing & W3C Utilities**:
   - `gateway/internal/telemetry/tracer.go:21,136-142`:
     ```go
     const TraceParentHeader = "traceparent"
     ...
     func (s *Span) FormatW3CTraceParent() string {
         tid := s.TraceID
         if len(tid) < 32 {
             tid = fmt.Sprintf("%032s", tid)
         }
         return fmt.Sprintf("00-%s-%s-01", tid, s.SpanID)
     }
     ```
   - `gateway/internal/telemetry/tracer.go:53-59`:
     ```go
     if tp := strings.TrimSpace(r.Header.Get(TraceParentHeader)); tp != "" {
         parts := strings.Split(tp, "-")
         if len(parts) == 4 && len(parts[1]) == 32 {
             return parts[1]
         }
     }
     ```
     `ExtractCorrelationID` already parses W3C `traceparent` headers and extracts the 32-hex trace ID into request context.

3. **Python AI Tier Inbound Endpoints**:
   - `ai-tier/main.py:165-450`:
     Endpoints include:
     - `POST /analyze` (`analyze_incident`)
     - `POST /orchestrate` (`orchestrate_agent_fleet`)
     - `POST /agents/diagnosis/run` (`run_diagnosis_agent`)
     - `POST /agents/workflows/run` (`run_multi_agent_workflow`)
     - `POST /internal/agents/stage/run` (`run_agent_stage`)
     - `POST /agents/verifier/run` (`run_verifier_critic`)
     - `GET /evals/run` (`get_evals_summary`)

4. **Python Telemetry Implementation**:
   - `ai-tier/observability/telemetry.py:58-119`:
     Currently provides in-memory `MockSpan`, `MockTracer`, `get_tracer()`, `sanitize_span_attributes()`, and `configure_agent_observability()`.
     `TraceContextTextMapPropagator` extraction is not yet integrated.

5. **Test Baseline Status**:
   - `pytest ai-tier/tests/ -q`: `111 passed, 35 warnings in 281.01s (0:04:41)`.
   - `go test ./internal/telemetry -v`: `PASS (0.630s)`.
   - `go test . -run "TestAgent" -v`: `PASS (2.796s)`.

---

## 2. Logic Chain

1. **Step 1 (Tracing Continuity Invariant)**: To establish distributed trace continuity between Go control plane operations and Python multi-agent execution, outgoing HTTP calls from Go must carry standard W3C `traceparent` headers (`00-{trace_id}-{span_id}-01`).
   - *References Observation 1 & 2*: Go gateway already has `Span.FormatW3CTraceParent()` and `ExtractCorrelationID` in `gateway/internal/telemetry/tracer.go`, but calls in `agent_orchestrator.go:ExecuteStage` and `ai_client.go:TriageIncident` omit the `traceparent` header.

2. **Step 2 (Trace Context Injection)**: Injecting `traceparent` in `ExecuteStage` and `TriageIncident` using `span.FormatW3CTraceParent()` or `telemetry.GetCorrelationID(ctx)` ensures every outbound HTTP request to `http://localhost:8000/internal/agents/stage/run` and `http://localhost:8000/analyze` carries the 32-character hex trace ID.
   - *References Observation 1 & 2*.

3. **Step 3 (Trace Context Extraction in Python)**: OpenTelemetry's `TraceContextTextMapPropagator().extract(carrier)` decodes the W3C `traceparent` header into an OpenTelemetry `Context` containing a remote `SpanContext`.
   - *References Observation 3 & 4*.

4. **Step 4 (Child Span Trace ID Inheritance)**: When AI tier spans (`sentinelflow.agent.invoke`, `sentinelflow.boundary.screen_input`, `sentinelflow.boundary.model_call`, `sentinelflow.boundary.screen_output`, `sentinelflow.toolgateway.execute`) are created in the context extracted by `TraceContextTextMapPropagator`, OpenTelemetry automatically assigns the exact `trace_id` from the Go control plane, linking the trace graph in Google Cloud Trace.
   - *References Observation 3, 4 & ORIGINAL_REQUEST.md Requirements R3, R4*.

5. **Step 5 (Dual-Mode & PII Guarantees)**: Gating the real OpenTelemetry provider behind `SENTINEL_OTEL_ENABLED="true"` ensures offline tests remain 100% functional without external dependencies (preserving `111 passed`), while wrapping real spans ensures all attributes are sanitized via `sanitize_span_attributes()` before leaving the process boundary.
   - *References Observation 4 & 5*.

---

## 3. Caveats

- **Network Environment**: In offline test environments (`SENTINEL_OTEL_ENABLED="false"` or unset), `MockTracer` is returned. Tests verifying `TraceContextTextMapPropagator` must test both:
  1. Unit extraction with `TraceContextTextMapPropagator` + in-memory tracer provider.
  2. Fallback / mock mode behavior.
- **Go Mock HTTP Servers in Tests**: Existing Go unit tests (e.g. `agent_orchestrator_test.go`) use `httptest.Server` to mock the AI tier; injecting `traceparent` headers does not break existing mock servers because headers are additive.

---

## 4. Conclusion

The survey confirms that W3C Trace Context Propagation across the Go gateway and Python AI tier is completely viable with minimal, surgical changes:
1. **Go Gateway**: Update `ExecuteStage` in `gateway/agent_orchestrator.go` and `TriageIncident` in `gateway/ai_client.go` to set `httpReq.Header.Set("traceparent", ...)` using `telemetry.Span.FormatW3CTraceParent()`.
2. **Python AI Tier**: Implement `extract_trace_context()` using `TraceContextTextMapPropagator` in `ai-tier/observability/telemetry.py`, attach context in FastAPI middleware / request handlers in `ai-tier/main.py`, and instrument canonical span names (`sentinelflow.agent.invoke`, `sentinelflow.boundary.screen_input`, `sentinelflow.boundary.model_call`, `sentinelflow.boundary.screen_output`, `sentinelflow.toolgateway.execute`).

Detailed survey report is preserved at `C:\Users\Gathu\Projects\fintech\.agents\explorer_survey_2\survey.md`.

---

## 5. Verification Method

1. **Inspect Survey File**:
   - `view_file` at `C:\Users\Gathu\Projects\fintech\.agents\explorer_survey_2\survey.md`.
2. **Verify Go Telemetry Tests**:
   - Run: `go test ./internal/telemetry -v` in `C:\Users\Gathu\Projects\fintech\gateway`.
3. **Verify Go Agent Tests**:
   - Run: `go test . -run "TestAgent" -v` in `C:\Users\Gathu\Projects\fintech\gateway`.
4. **Verify Python AI Tier Tests**:
   - Run: `pytest ai-tier/tests/ -q` in `C:\Users\Gathu\Projects\fintech`.
