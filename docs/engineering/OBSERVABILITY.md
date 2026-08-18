# SentinelFlow Observability Architecture & Metrics Catalogue

## 1. Overview & Principles

SentinelFlow instruments critical pre-ledger financial file processing with Prometheus metrics, OpenTelemetry-compatible distributed tracing with correlation IDs, and structured JSON logs.

### Non-Negotiable Privacy & Cardinality Invariants
1. **Zero Financial / Sensitive Leakage**: Telemetry never records raw account numbers, routing numbers, monetary amounts, SQL queries, authorization tokens, secrets, or file payloads.
2. **Strictly Low-Cardinality Labels**: Metric labels are restricted to bounded enums (e.g. HTTP status codes `200`, `400`, `500`; normalized routes `/api/v1/artifacts/{id}`; job kinds `VALIDATE_ARTIFACT`; state names `QUEUED`, `RUNNING`, `DEAD`). Dynamic values such as tenant IDs, user IDs, file instance IDs, or trace IDs are strictly forbidden as Prometheus labels.
3. **Opaque Correlation IDs**: Traces and request logs use opaque correlation IDs (e.g., UUIDv4 or W3C `traceparent`), propagated across internal pipelines and returned via `X-Correlation-ID` header.
4. **Zero Unmeasured Constants**: Metric values are derived directly from runtime system state, atomic counters, database queries, and benchmarks.

---

## 2. Prometheus Metrics Catalogue

All metrics are exposed at the `GET /metrics` endpoint in standard Prometheus 0.0.4 text format.

### Financial Pipeline & Processing Metrics
| Metric Name | Type | Description | Labels |
|-------------|------|-------------|--------|
| `sentinel_pipeline_files_total` | Counter | Count of processed financial files by terminal outcome | `status` (`VALIDATED`, `QUARANTINED`, `FAILED`) |
| `sentinel_pipeline_bytes_total` | Counter | Volume of processed bytes by terminal outcome | `status` (`VALIDATED`, `QUARANTINED`, `FAILED`) |
| `sentinel_pipeline_records_total` | Counter | Count of parsed NACHA records by terminal outcome | `status` (`VALIDATED`, `QUARANTINED`, `FAILED`) |
| `sentinel_streaming_parse_rate_records_per_sec` | Gauge | Last observed parsing velocity in records/second (-1 if unmeasured) | None |

### Latency Histograms
| Metric Name | Type | Description | Buckets (seconds) | Labels |
|-------------|------|-------------|-------------------|--------|
| `sentinel_arrival_to_job_visible_seconds` | Histogram | Duration from upload arrival completion to job record commit | Default (0.005 to 10s) | None |
| `sentinel_job_queue_wait_seconds` | Histogram | Duration a job spent in `QUEUED` state prior to worker lease | Default (0.005 to 10s) | None |
| `sentinel_job_processing_duration_seconds` | Histogram | Execution duration of worker job handler | Default (0.005 to 10s) | `kind` (`VALIDATE_ARTIFACT`, etc.) |
| `sentinel_decision_duration_seconds` | Histogram | End-to-end duration from file ingress to terminal release/quarantine decision | Default (0.005 to 10s) | `status` (`VALIDATED`, `QUARANTINED`, `FAILED`) |

### Ingestion Queue & Worker Pool Saturation
| Metric Name | Type | Description | Labels |
|-------------|------|-------------|--------|
| `sentinel_jobs_state_count` | Gauge | Live count of ingestion jobs queried directly from database | `state` (`queued`, `leased`, `running`, `succeeded`, `retryable`, `dead`, `cancelled`) |
| `sentinel_worker_saturation_ratio` | Gauge | Ratio of active executing workers to total configured pool capacity (0.0 - 1.0) | None |
| `sentinel_backpressure_rejections_total` | Counter | Total requests or job enqueues rejected due to capacity limits | `reason` (`worker_pool_full`, `tenant_quota_exceeded`, etc.) |

### HTTP & Ingress Metrics
| Metric Name | Type | Description | Labels |
|-------------|------|-------------|--------|
| `sentinel_http_requests_total` | Counter | Total HTTP requests handled by normalized route and status | `method`, `route`, `status` |
| `sentinel_http_request_duration_seconds` | Histogram | Latency distribution of HTTP requests by normalized route | `method`, `route` |

*Note: Routes are normalized (e.g. `/api/v1/artifacts/:id` -> `/api/v1/artifacts/{id}`) to prevent unbounded label explosion.*

### External Dependency Health
| Metric Name | Type | Description | Labels |
|-------------|------|-------------|--------|
| `sentinel_dependency_request_duration_seconds` | Histogram | Latency of calls to external dependencies | `dependency` (`database`, `object_store`), `operation` (`query`, `exec`, `get`, `put`, `delete`) |
| `sentinel_dependency_errors_total` | Counter | Total failed operations against external dependencies | `dependency` (`database`, `object_store`), `operation` |

### Real-Time SSE Stream Health
| Metric Name | Type | Description | Labels |
|-------------|------|-------------|--------|
| `sentinel_sse_subscribers_active` | Gauge | Current number of active SSE event stream clients | None |
| `sentinel_sse_replays_total` | Counter | Number of historical event replay streams served | `status` (`normal`, `gap`) |
| `sentinel_sse_dropped_connections_total` | Counter | Number of SSE connections disconnected abnormally | None |

### System & Integrity Telemetry
| Metric Name | Type | Description | Labels |
|-------------|------|-------------|--------|
| `sentinel_uptime_seconds` | Gauge | Seconds elapsed since gateway process start | None |
| `sentinel_active_incidents` | Gauge | Current count of unresolved pre-ledger incidents | None |
| `sentinel_audit_chain_height` | Gauge | Height of the immutable SHA-256 audit ledger | None |

---

## 3. Distributed Tracing & Correlation IDs

SentinelFlow implements an OpenTelemetry-compatible tracing layer in `internal/telemetry/tracer.go`.

### Trace Context Propagation
- Ingress requests are inspected for W3C `traceparent` or incoming `X-Correlation-ID`.
- If missing, a secure random 32-character hex ID is generated.
- The correlation ID is attached to the Go `context.Context` and injected into every log message, worker job execution, and HTTP response header (`X-Correlation-ID`).

### Span Lifecycle
```go
tracer := telemetry.GlobalTracer()
ctx, span := tracer.StartSpan(ctx, "validate_artifact",
    telemetry.WithAttribute("job_kind", "VALIDATE_ARTIFACT"),
)
defer span.End()
```

---

## 4. Benchmark Harness & Performance Verification

Reproducible performance benchmarking is implemented in `gateway/benchmark.go`:
- **Corpus Generator**: `GenerateLargeNachaCorpus(recordCount)` generates valid, strictly 94-character fixed-width NACHA ACH files with accurate batch and file hashes, credit/debit accumulators, and valid ABA routing numbers.
- **Harness**: `RunHarness(preset, recordCount, concurrency, iterations)` executes concurrent streaming validations across presets (`small`: 100, `medium`: 10,000, `large`: 100,000) and computes p50, p95, p99 latencies, throughput MB/s, records/sec, and peak allocation metrics.
