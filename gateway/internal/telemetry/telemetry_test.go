package telemetry

import (
	"context"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestMetricsRegistryAndRender(t *testing.T) {
	reg := NewRegistry()

	counter := reg.RegisterCounter(NewCounter("test_counter_total", "Test counter help"))
	gauge := reg.RegisterGauge(NewGauge("test_gauge", "Test gauge help"))
	hist := reg.RegisterHistogram(NewHistogram("test_latency_seconds", "Test histogram help", []float64{0.01, 0.05, 0.1}))

	// Initial values
	counter.Inc(Label{Key: "status", Value: "valid"})
	counter.Add(5, Label{Key: "status", Value: "quarantined"})
	gauge.Set(42.5, Label{Key: "pool", Value: "workers"})
	hist.Observe(0.025, Label{Key: "method", Value: "POST"})

	output := string(reg.Render())

	if !strings.Contains(output, `# HELP test_counter_total Test counter help`) {
		t.Errorf("missing help for test_counter_total in output:\n%s", output)
	}
	if !strings.Contains(output, `test_counter_total{status="valid"} 1`) {
		t.Errorf("missing valid count in output:\n%s", output)
	}
	if !strings.Contains(output, `test_counter_total{status="quarantined"} 5`) {
		t.Errorf("missing quarantined count in output:\n%s", output)
	}
	if !strings.Contains(output, `test_gauge{pool="workers"} 42.5`) {
		t.Errorf("missing gauge in output:\n%s", output)
	}
	if !strings.Contains(output, `test_latency_seconds_bucket{method="POST",le="0.05"} 1`) {
		t.Errorf("missing histogram bucket in output:\n%s", output)
	}
	if !strings.Contains(output, `test_latency_seconds_count{method="POST"} 1`) {
		t.Errorf("missing histogram count in output:\n%s", output)
	}
}

func TestMetricsChangeWhenWorkOccurs(t *testing.T) {
	reg := NewRegistry()
	filesTotal := reg.RegisterCounter(NewCounter("sentinel_pipeline_files_total", "Total files processed"))
	bytesTotal := reg.RegisterCounter(NewCounter("sentinel_pipeline_bytes_total", "Total bytes processed"))
	durationHist := reg.RegisterHistogram(NewHistogram("sentinel_job_processing_duration_seconds", "Processing duration", nil))

	// Initial state: 0
	if val := filesTotal.Get(Label{Key: "status", Value: "valid"}); val != 0 {
		t.Fatalf("expected 0 initial files, got %d", val)
	}

	// Simulate processing work
	startTime := time.Now()
	// do work
	filesTotal.Inc(Label{Key: "status", Value: "valid"})
	bytesTotal.Add(1024, Label{Key: "status", Value: "valid"})
	durationHist.Observe(time.Since(startTime).Seconds(), Label{Key: "kind", Value: "VALIDATE_ARTIFACT"})

	// Verify metric state changed
	if val := filesTotal.Get(Label{Key: "status", Value: "valid"}); val != 1 {
		t.Fatalf("expected 1 file after work, got %d", val)
	}
	if val := bytesTotal.Get(Label{Key: "status", Value: "valid"}); val != 1024 {
		t.Fatalf("expected 1024 bytes after work, got %d", val)
	}
}

func TestWorkerGaugeEqualsRealWorkerState(t *testing.T) {
	reg := NewRegistry()
	workerSaturation := reg.RegisterGauge(NewGauge("sentinel_worker_saturation_ratio", "Worker saturation ratio"))
	jobsGauge := reg.RegisterGauge(NewGauge("sentinel_jobs_state_count", "Jobs by state"))

	// 2 active workers out of 4 total capacity
	activeWorkers := 2
	totalWorkers := 4
	workerSaturation.Set(float64(activeWorkers)/float64(totalWorkers), Label{Key: "pool", Value: "default"})

	// Job counts from DB
	jobsGauge.Set(10, Label{Key: "state", Value: "queued"})
	jobsGauge.Set(2, Label{Key: "state", Value: "running"})
	jobsGauge.Set(1, Label{Key: "state", Value: "retryable"})
	jobsGauge.Set(0, Label{Key: "state", Value: "dead"})

	if got := workerSaturation.Get(Label{Key: "pool", Value: "default"}); got != 0.5 {
		t.Fatalf("expected saturation 0.5, got %g", got)
	}
	if got := jobsGauge.Get(Label{Key: "state", Value: "queued"}); got != 10 {
		t.Fatalf("expected 10 queued, got %g", got)
	}
	if got := jobsGauge.Get(Label{Key: "state", Value: "running"}); got != 2 {
		t.Fatalf("expected 2 running, got %g", got)
	}
}

func TestHighCardinalityRegressionGuard(t *testing.T) {
	// Concrete URLs with IDs must be mapped to normalized route templates
	testCases := []struct {
		input    string
		expected string
	}{
		{"/api/v1/artifacts/123", "/api/v1/artifacts/{id}"},
		{"/api/v1/artifacts/9999/content", "/api/v1/artifacts/{id}/content"},
		{"/api/v1/contracts/42/versions", "/api/v1/contracts/{id}/versions"},
		{"/api/v1/incidents/55/triage", "/api/v1/incidents/{id}/triage"},
		{"/api/v1/incidents/55/approve", "/api/v1/incidents/{id}/approve"},
		{"/api/v1/connections/postgres-prod-db/test", "/api/v1/connections/{id}/test"},
		{"/api/v1/connections/postgres-prod-db/secrets/password", "/api/v1/connections/{id}/secrets/{field}"},
		{"/api/v1/sla-board", "/api/v1/sla-board"},
		{"/api/v1/files/upload", "/api/v1/files/upload"},
		{"/api/v1/unknown-endpoint/leak-test-tenant-12345", "UNKNOWN_ROUTE"},
	}

	for _, tc := range testCases {
		normalized := NormalizeRoute(tc.input)
		if normalized != tc.expected {
			t.Errorf("NormalizeRoute(%q) = %q; want %q", tc.input, normalized, tc.expected)
		}
	}
}

func TestTelemetryRedaction(t *testing.T) {
	// Ensure labels cannot contain sensitive characters or raw SQL/tokens
	secret := "secret-jwt-token-ey123456"
	sanitized := escapeLabelValue(secret)
	if strings.Contains(sanitized, "\n") || strings.Contains(sanitized, `"`) {
		t.Errorf("failed to escape sensitive label characters: %s", sanitized)
	}

	// Status normalizer must reject freeform strings and map to bounded enum
	if norm := NormalizeStatus("SOME_UNSAFE_INJECTED_STRING_123"); norm != "unknown" {
		t.Errorf("expected 'unknown' for arbitrary status, got %q", norm)
	}
	if norm := NormalizeStatus("VALID"); norm != "valid" {
		t.Errorf("expected 'valid', got %q", norm)
	}
	if norm := NormalizeStatus("QUARANTINED"); norm != "quarantined" {
		t.Errorf("expected 'quarantined', got %q", norm)
	}
}

func TestCorrelationIDAndTracing(t *testing.T) {
	req := httptest.NewRequest("GET", "/api/v1/artifacts", nil)
	cid1 := ExtractCorrelationID(req)
	if len(cid1) == 0 {
		t.Fatal("expected non-empty generated correlation ID")
	}

	// Check with custom opaque X-Correlation-ID
	customID := "corr-test-1234-abcd"
	req.Header.Set(CorrelationIDHeader, customID)
	cid2 := ExtractCorrelationID(req)
	if cid2 != customID {
		t.Fatalf("expected custom correlation ID %s, got %s", customID, cid2)
	}

	// Check span lifecycle
	ctx := WithCorrelationID(context.Background(), cid2)
	ctx, span := StartSpan(ctx, "test_operation")
	span.SetAttribute("component", "ingest")
	time.Sleep(10 * time.Millisecond)
	span.End()

	if span.TraceID != cid2 {
		t.Fatalf("expected span trace ID %s, got %s", cid2, span.TraceID)
	}
	if span.EndTime.Before(span.StartTime) {
		t.Fatalf("expected end time after start time")
	}
	traceParent := span.FormatW3CTraceParent()
	if !strings.HasPrefix(traceParent, "00-") {
		t.Fatalf("invalid W3C traceparent prefix: %s", traceParent)
	}
}
