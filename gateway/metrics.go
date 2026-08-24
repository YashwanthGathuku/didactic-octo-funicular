package main

import (
	"database/sql"
	"math"
	"net/http"
	"sync/atomic"
	"time"

	"sentinel-gateway/internal/telemetry"
)

// MetricRegistry is the process-wide metrics registry.
var MetricRegistry = telemetry.NewRegistry()

// Global Prometheus metrics
var (
	// System / Process
	metricUptime = MetricRegistry.RegisterGauge(telemetry.NewGauge(
		"sentinel_uptime_seconds", "Total seconds the gateway process has been running"))

	// Legacy / Compatibility counters
	metricFilesIngestedTotal = MetricRegistry.RegisterCounter(telemetry.NewCounter(
		"sentinel_files_ingested_total", "Total count of financial files ingested"))
	metricBytesIngestedTotal = MetricRegistry.RegisterCounter(telemetry.NewCounter(
		"sentinel_bytes_ingested_total", "Total volume of payload bytes processed"))
	metricActiveIncidents = MetricRegistry.RegisterGauge(telemetry.NewGauge(
		"sentinel_active_incidents", "Total count of active pre-ledger incidents"))
	metricAuditChainHeight = MetricRegistry.RegisterGauge(telemetry.NewGauge(
		"sentinel_audit_chain_height", "Current height of the append-only application hash chain"))
	metricStreamingParseRate = MetricRegistry.RegisterGauge(telemetry.NewGauge(
		"sentinel_streaming_parse_rate_records_per_sec", "Last MEASURED streaming parse velocity (-1 = never measured)"))

	// Pipeline / Financial file metrics
	metricPipelineFiles = MetricRegistry.RegisterCounter(telemetry.NewCounter(
		"sentinel_pipeline_files_total", "Count of processed files by terminal status"))
	metricPipelineBytes = MetricRegistry.RegisterCounter(telemetry.NewCounter(
		"sentinel_pipeline_bytes_total", "Volume of processed bytes by terminal status"))
	metricPipelineRecords = MetricRegistry.RegisterCounter(telemetry.NewCounter(
		"sentinel_pipeline_records_total", "Count of parsed NACHA records by terminal status"))

	// Latency histograms
	metricArrivalToJobVisible = MetricRegistry.RegisterHistogram(telemetry.NewHistogram(
		"sentinel_arrival_to_job_visible_seconds", "Duration from finalized upload arrival to ingestion job visibility", nil))
	metricJobQueueWait = MetricRegistry.RegisterHistogram(telemetry.NewHistogram(
		"sentinel_job_queue_wait_seconds", "Duration a job spent queued before being leased by a worker", nil))
	metricJobProcessingDuration = MetricRegistry.RegisterHistogram(telemetry.NewHistogram(
		"sentinel_job_processing_duration_seconds", "Duration of job handler execution by job kind", nil))
	metricDecisionDuration = MetricRegistry.RegisterHistogram(telemetry.NewHistogram(
		"sentinel_decision_duration_seconds", "End-to-end duration from arrival to terminal validation or review decision", nil))

	// Jobs & Worker state
	metricJobsStateCount = MetricRegistry.RegisterGauge(telemetry.NewGauge(
		"sentinel_jobs_state_count", "Current count of ingestion jobs in each lifecycle state"))
	metricWorkerSaturation = MetricRegistry.RegisterGauge(telemetry.NewGauge(
		"sentinel_worker_saturation_ratio", "Ratio of active worker leases to total configured worker capacity"))
	metricBackpressureRejections = MetricRegistry.RegisterCounter(telemetry.NewCounter(
		"sentinel_backpressure_rejections_total", "Total requests or jobs rejected due to capacity limits or backpressure"))

	// API / HTTP metrics
	metricHTTPRequestsTotal = MetricRegistry.RegisterCounter(telemetry.NewCounter(
		"sentinel_http_requests_total", "Total HTTP requests served by normalized route and status code"))
	metricHTTPRequestDuration = MetricRegistry.RegisterHistogram(telemetry.NewHistogram(
		"sentinel_http_request_duration_seconds", "HTTP request duration in seconds by normalized route", nil))

	// External Dependency metrics
	metricDependencyDuration = MetricRegistry.RegisterHistogram(telemetry.NewHistogram(
		"sentinel_dependency_request_duration_seconds", "Duration of operations on external dependencies (database, object store)", nil))
	metricDependencyErrors = MetricRegistry.RegisterCounter(telemetry.NewCounter(
		"sentinel_dependency_errors_total", "Count of errors encountered when calling external dependencies"))

	// SSE stream metrics
	metricSSESubscribersActive = MetricRegistry.RegisterGauge(telemetry.NewGauge(
		"sentinel_sse_subscribers_active", "Current count of active Server-Sent Events subscribers"))
	metricSSEReplays = MetricRegistry.RegisterCounter(telemetry.NewCounter(
		"sentinel_sse_replays_total", "Count of SSE replay streams served by status (normal vs gap)"))
	metricSSEDroppedConnections = MetricRegistry.RegisterCounter(telemetry.NewCounter(
		"sentinel_sse_dropped_connections_total", "Count of SSE connections closed abnormally or timed out"))
)

type MetricsCollector struct {
	TotalIngested      uint64
	TotalQuarantined   uint64
	TotalValid         uint64
	TotalBytesIngested uint64
	ActiveIncidents    uint64
	AuditChainHeight   uint64
	StartTime          time.Time
	parseRateBits      uint64
}

var GlobalMetrics = &MetricsCollector{
	StartTime:     time.Now(),
	parseRateBits: math.Float64bits(-1),
}

// RecordMeasuredParseRate stores a genuinely observed parse rate from benchmark harness.
func (m *MetricsCollector) RecordMeasuredParseRate(recordsPerSec float64) {
	atomic.StoreUint64(&m.parseRateBits, math.Float64bits(recordsPerSec))
	metricStreamingParseRate.Set(recordsPerSec)
}

// LastMeasuredParseRate returns the last observed rate, or -1 if never measured.
func (m *MetricsCollector) LastMeasuredParseRate() float64 {
	return math.Float64frombits(atomic.LoadUint64(&m.parseRateBits))
}

// RecordFileIngested records a processed financial file.
func (m *MetricsCollector) RecordFileIngested(status string, bytes int) {
	atomic.AddUint64(&m.TotalIngested, 1)
	atomic.AddUint64(&m.TotalBytesIngested, uint64(bytes))
	metricFilesIngestedTotal.Inc(telemetry.Label{Key: "status", Value: "ALL"})
	metricBytesIngestedTotal.Add(uint64(bytes))

	normStatus := telemetry.NormalizeStatus(status)
	switch status {
	case "QUARANTINED":
		atomic.AddUint64(&m.TotalQuarantined, 1)
		metricFilesIngestedTotal.Inc(telemetry.Label{Key: "status", Value: "QUARANTINED"})
	case "RELEASED", "VALID":
		atomic.AddUint64(&m.TotalValid, 1)
		metricFilesIngestedTotal.Inc(telemetry.Label{Key: "status", Value: "VALID"})
	}

	metricPipelineFiles.Inc(telemetry.Label{Key: "status", Value: normStatus})
	metricPipelineBytes.Add(uint64(bytes), telemetry.Label{Key: "status", Value: normStatus})
}

// RecordPipelineCompletion records the end-to-end outcome of validating an artifact.
func RecordPipelineCompletion(status string, bytes int64, records int, decisionDurationSec float64) {
	normStatus := telemetry.NormalizeStatus(status)
	metricPipelineFiles.Inc(telemetry.Label{Key: "status", Value: normStatus})
	if bytes > 0 {
		metricPipelineBytes.Add(uint64(bytes), telemetry.Label{Key: "status", Value: normStatus})
	}
	if records > 0 {
		metricPipelineRecords.Add(uint64(records), telemetry.Label{Key: "status", Value: normStatus})
	}
	if decisionDurationSec > 0 {
		metricDecisionDuration.Observe(decisionDurationSec)
	}
}

// RecordHTTPRequest records an incoming HTTP request duration and status.
func RecordHTTPRequest(method, route string, statusCode int, durationSec float64) {
	normRoute := telemetry.NormalizeRoute(route)
	statusStr := ""
	switch {
	case statusCode >= 200 && statusCode < 300:
		statusStr = "2xx"
	case statusCode >= 300 && statusCode < 400:
		statusStr = "3xx"
	case statusCode >= 400 && statusCode < 500:
		statusStr = "4xx"
	case statusCode >= 500:
		statusStr = "5xx"
	default:
		statusStr = "other"
	}

	metricHTTPRequestsTotal.Inc(
		telemetry.Label{Key: "method", Value: method},
		telemetry.Label{Key: "route", Value: normRoute},
		telemetry.Label{Key: "status_code", Value: statusStr},
	)
	metricHTTPRequestDuration.Observe(durationSec,
		telemetry.Label{Key: "method", Value: method},
		telemetry.Label{Key: "route", Value: normRoute},
	)
}

// RecordDependencyOperation records a call to a database or object store.
func RecordDependencyOperation(dependency, operation string, durationSec float64, err error) {
	metricDependencyDuration.Observe(durationSec,
		telemetry.Label{Key: "dependency", Value: dependency},
		telemetry.Label{Key: "operation", Value: operation},
	)
	if err != nil {
		metricDependencyErrors.Inc(
			telemetry.Label{Key: "dependency", Value: dependency},
			telemetry.Label{Key: "operation", Value: operation},
		)
	}
}

// SetAuditChainHeight updates the hash chain height gauge.
func (m *MetricsCollector) SetAuditChainHeight(height uint64) {
	atomic.StoreUint64(&m.AuditChainHeight, height)
	metricAuditChainHeight.Set(float64(height))
}

// SetActiveIncidents updates the active incidents gauge.
func (m *MetricsCollector) SetActiveIncidents(count uint64) {
	atomic.StoreUint64(&m.ActiveIncidents, count)
	metricActiveIncidents.Set(float64(count))
}

// RefreshDatabaseJobGauges updates the job state gauges directly from database counts.
func RefreshDatabaseJobGauges(db *sql.DB) {
	if db == nil {
		return
	}
	rows, err := db.Query(`SELECT state, COUNT(*) FROM ingestion_jobs GROUP BY state`)
	if err != nil {
		return
	}
	defer rows.Close()

	counts := map[string]float64{
		"queued":    0,
		"leased":    0,
		"running":   0,
		"retryable": 0,
		"dead":      0,
	}

	for rows.Next() {
		var state string
		var count float64
		if err := rows.Scan(&state, &count); err == nil {
			norm := telemetry.NormalizeStatus(state)
			counts[norm] += count
		}
	}

	for state, count := range counts {
		metricJobsStateCount.Set(count, telemetry.Label{Key: "state", Value: state})
	}
}

// ServePrometheusMetrics formats and writes metrics in Prometheus standard text format 0.0.4.
func ServePrometheusMetrics(w http.ResponseWriter, r *http.Request) {
	uptimeSeconds := time.Since(GlobalMetrics.StartTime).Seconds()
	metricUptime.Set(uptimeSeconds)

	// Ensure legacy gauges reflect collector state
	metricActiveIncidents.Set(float64(atomic.LoadUint64(&GlobalMetrics.ActiveIncidents)))
	metricAuditChainHeight.Set(float64(atomic.LoadUint64(&GlobalMetrics.AuditChainHeight)))
	metricStreamingParseRate.Set(GlobalMetrics.LastMeasuredParseRate())

	w.Header().Set("Content-Type", "text/plain; version=0.0.4")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(MetricRegistry.Render())
}
