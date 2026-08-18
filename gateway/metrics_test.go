package main

import (
	"database/sql"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	_ "modernc.org/sqlite"
)

func TestServePrometheusMetricsOutput(t *testing.T) {
	GlobalMetrics.RecordFileIngested("VALID", 512)
	GlobalMetrics.RecordFileIngested("QUARANTINED", 256)
	GlobalMetrics.SetActiveIncidents(3)
	GlobalMetrics.SetAuditChainHeight(15)
	GlobalMetrics.RecordMeasuredParseRate(12345.67)

	RecordHTTPRequest("GET", "/api/v1/artifacts", 200, 0.015)
	RecordPipelineCompletion("VALID", 1024, 10, 0.050)
	RecordDependencyOperation("database", "query", 0.002, nil)

	req := httptest.NewRequest("GET", "/metrics", nil)
	w := httptest.NewRecorder()
	ServePrometheusMetrics(w, req)

	resp := w.Result()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected status 200, got %d", resp.StatusCode)
	}

	body := w.Body.String()

	expectedMetrics := []string{
		"sentinel_uptime_seconds",
		"sentinel_files_ingested_total",
		"sentinel_bytes_ingested_total",
		"sentinel_active_incidents 3",
		"sentinel_audit_chain_height 15",
		"sentinel_streaming_parse_rate_records_per_sec 12345.67",
		"sentinel_pipeline_files_total",
		"sentinel_pipeline_bytes_total",
		"sentinel_http_requests_total",
		"sentinel_http_request_duration_seconds",
		"sentinel_dependency_request_duration_seconds",
	}

	for _, m := range expectedMetrics {
		if !strings.Contains(body, m) {
			t.Errorf("expected metrics output to contain %q; body:\n%s", m, body)
		}
	}
}

func TestRefreshDatabaseJobGauges(t *testing.T) {
	db, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatalf("failed to open test database: %v", err)
	}
	defer db.Close()

	if _, err := Migrate(db); err != nil {
		t.Fatalf("migration failed: %v", err)
	}

	// Insert test jobs using actual schema columns
	_, err = db.Exec(`
		INSERT INTO ingestion_jobs (tenant_id, kind, state, run_after, max_attempts, idempotency_key)
		VALUES
			('default', 'VALIDATE_ARTIFACT', 'QUEUED', CURRENT_TIMESTAMP, 3, 'key-1'),
			('default', 'VALIDATE_ARTIFACT', 'QUEUED', CURRENT_TIMESTAMP, 3, 'key-2'),
			('default', 'VALIDATE_ARTIFACT', 'RUNNING', CURRENT_TIMESTAMP, 3, 'key-3'),
			('default', 'VALIDATE_ARTIFACT', 'DEAD', CURRENT_TIMESTAMP, 3, 'key-4')
	`)
	if err != nil {
		t.Fatalf("failed to insert test jobs: %v", err)
	}

	RefreshDatabaseJobGauges(db)

	req := httptest.NewRequest("GET", "/metrics", nil)
	w := httptest.NewRecorder()
	ServePrometheusMetrics(w, req)

	body := w.Body.String()
	if !strings.Contains(body, `sentinel_jobs_state_count{state="queued"} 2`) {
		t.Errorf("expected 2 queued jobs in metrics output; body:\n%s", body)
	}
	if !strings.Contains(body, `sentinel_jobs_state_count{state="running"} 1`) {
		t.Errorf("expected 1 running job in metrics output; body:\n%s", body)
	}
	if !strings.Contains(body, `sentinel_jobs_state_count{state="dead"} 1`) {
		t.Errorf("expected 1 dead job in metrics output; body:\n%s", body)
	}
}
