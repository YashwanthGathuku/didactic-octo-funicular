# SentinelFlow Service-Level Objectives

> **STATUS**: All target values below are labelled **TARGET** and have not been
> validated in a defined production or staging environment. They must be
> confirmed through measured benchmarks before being treated as SLOs.

## 1. Availability

| Objective | Target | Measurement |
|-----------|--------|-------------|
| API endpoint availability | **TARGET** 99.9% monthly | `1 - (sentinel_http_requests_total{status=~"5.."} / sentinel_http_requests_total)` |
| `/health` endpoint availability | **TARGET** 99.95% monthly | Synthetic monitor on `/health` endpoint |
| Worker pool availability | **TARGET** 99.9% monthly | `sentinel_worker_saturation_ratio > 0` over rolling window |

## 2. Latency

| Objective | Target | Measurement |
|-----------|--------|-------------|
| API request latency (p50) | **TARGET** < 50ms | `sentinel_http_request_duration_seconds{quantile="0.5"}` |
| API request latency (p95) | **TARGET** < 200ms | `sentinel_http_request_duration_seconds{quantile="0.95"}` |
| API request latency (p99) | **TARGET** < 500ms | `sentinel_http_request_duration_seconds{quantile="0.99"}` |
| Arrival-to-job-visible latency (p95) | **TARGET** < 2s | `sentinel_arrival_to_job_visible_seconds{quantile="0.95"}` |
| Job queue wait (p95) | **TARGET** < 5s | `sentinel_job_queue_wait_seconds{quantile="0.95"}` |
| Processing duration (p95) | **TARGET** < 10s | `sentinel_job_processing_duration_seconds{quantile="0.95"}` |
| End-to-end decision latency (p95) | **TARGET** < 30s | `sentinel_decision_duration_seconds{quantile="0.95"}` |

## 3. Throughput

| Objective | Target | Measurement |
|-----------|--------|-------------|
| Streaming parse rate | **TARGET** > 100,000 records/sec | `sentinel_streaming_parse_rate_records_per_sec` |
| File ingestion rate | **TARGET** > 10 files/sec sustained | `rate(sentinel_pipeline_files_total[5m])` |
| Worker saturation headroom | **TARGET** < 80% sustained | `sentinel_worker_saturation_ratio < 0.80` |

## 4. Error Budget

| Objective | Target | Measurement |
|-----------|--------|-------------|
| Quarantine false-positive rate | **TARGET** < 0.1% of valid files | Manual review of quarantined files |
| Dependency error rate (database) | **TARGET** < 0.01% of operations | `rate(sentinel_dependency_errors_total{dependency="database"}[5m])` |
| Dependency error rate (object store) | **TARGET** < 0.01% of operations | `rate(sentinel_dependency_errors_total{dependency="object_store"}[5m])` |
| Dead job rate | **TARGET** < 0.5% of total jobs | `sentinel_jobs_state_count{state="dead"} / sum(sentinel_jobs_state_count)` |
| Backpressure rejection rate | **TARGET** < 1% of requests | `rate(sentinel_backpressure_rejections_total[5m]) / rate(sentinel_http_requests_total[5m])` |

## 5. SSE / Real-Time Delivery

| Objective | Target | Measurement |
|-----------|--------|-------------|
| SSE replay gap rate | **TARGET** < 0.1% of replay streams | `rate(sentinel_sse_replays_total{status="gap"}[5m])` |
| SSE dropped connection rate | **TARGET** < 1% of connections/hour | `rate(sentinel_sse_dropped_connections_total[1h])` |

## 6. Data Integrity

| Objective | Target | Measurement |
|-----------|--------|-------------|
| Audit chain integrity | 100% (non-negotiable) | `sentinel_audit_chain_height` monotonically increasing; chain validation on startup |
| Deterministic validation | 100% (non-negotiable) | Same input always produces same `Result` and `Decision` (tested, not measured at runtime) |

---

## Burn Rate Alerts (Recommended)

| Alert | Condition | Severity |
|-------|-----------|----------|
| High API error rate | > 5% of requests return 5xx over 5 minutes | Critical |
| Worker pool saturated | `sentinel_worker_saturation_ratio > 0.95` for > 2 minutes | Warning |
| Dead jobs accumulating | `rate(sentinel_jobs_state_count{state="dead"}[15m]) > 0` | Warning |
| Database dependency degraded | `rate(sentinel_dependency_errors_total{dependency="database"}[5m]) > 0.001` | Critical |
| SSE subscriber drop spike | `rate(sentinel_sse_dropped_connections_total[5m]) > 5` | Warning |
| Arrival-to-decision latency spike | `sentinel_decision_duration_seconds{quantile="0.99"} > 60` | Warning |

---

## Notes

1. **TARGET labels** indicate values derived from architectural expectations and
   industry norms, not from measured production data. They must be validated
   against benchmark output and production telemetry before promotion to SLOs.

2. **Benchmark harness** (`RunHarness` in `gateway/benchmark.go`) provides
   reproducible p50/p95/p99 latency, throughput, peak memory, and error counts.
   Run benchmarks with `go test -bench BenchmarkNachaParser_100k -benchtime 5s`
   and record raw output before updating any target value.

3. **Privacy constraint**: No SLO query uses tenant-identifying labels (tenant
   ID, filename, account data). All metrics use low-cardinality labels only
   (status code, normalized route, state name, dependency name).
