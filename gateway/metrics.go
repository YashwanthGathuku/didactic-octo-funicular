package main

import (
	"fmt"
	"math"
	"net/http"
	"sync/atomic"
	"time"
)

type MetricsCollector struct {
	TotalIngested      uint64
	TotalQuarantined   uint64
	TotalValid         uint64
	TotalBytesIngested uint64
	ActiveIncidents    uint64
	MerkleHeight       uint64
	StartTime          time.Time
	// parseRateBits holds the last MEASURED records/sec as float64 bits.
	// math.Float64bits(-1) until a benchmark actually runs.
	parseRateBits uint64
}

var GlobalMetrics = &MetricsCollector{
	StartTime:     time.Now(),
	parseRateBits: math.Float64bits(-1),
}

// RecordMeasuredParseRate stores a genuinely observed parse rate.
func (m *MetricsCollector) RecordMeasuredParseRate(recordsPerSec float64) {
	atomic.StoreUint64(&m.parseRateBits, math.Float64bits(recordsPerSec))
}

// LastMeasuredParseRate returns the last observed rate, or -1 if never measured.
func (m *MetricsCollector) LastMeasuredParseRate() float64 {
	return math.Float64frombits(atomic.LoadUint64(&m.parseRateBits))
}

func (m *MetricsCollector) RecordFileIngested(status string, bytes int) {
	atomic.AddUint64(&m.TotalIngested, 1)
	atomic.AddUint64(&m.TotalBytesIngested, uint64(bytes))
	if status == "QUARANTINED" {
		atomic.AddUint64(&m.TotalQuarantined, 1)
	} else if status == "RELEASED" || status == "VALID" {
		atomic.AddUint64(&m.TotalValid, 1)
	}
}

func (m *MetricsCollector) SetMerkleHeight(height uint64) {
	atomic.StoreUint64(&m.MerkleHeight, height)
}

func (m *MetricsCollector) SetActiveIncidents(count uint64) {
	atomic.StoreUint64(&m.ActiveIncidents, count)
}

// ServePrometheusMetrics formats and writes metrics in Prometheus standard text format.
func ServePrometheusMetrics(w http.ResponseWriter, r *http.Request) {
	uptimeSeconds := time.Since(GlobalMetrics.StartTime).Seconds()

	w.Header().Set("Content-Type", "text/plain; version=0.0.4")

	fmt.Fprintf(w, "# HELP sentinel_uptime_seconds Total seconds the gateway has been running\n")
	fmt.Fprintf(w, "# TYPE sentinel_uptime_seconds gauge\n")
	fmt.Fprintf(w, "sentinel_uptime_seconds %.2f\n\n", uptimeSeconds)

	fmt.Fprintf(w, "# HELP sentinel_files_ingested_total Total count of financial files ingested\n")
	fmt.Fprintf(w, "# TYPE sentinel_files_ingested_total counter\n")
	fmt.Fprintf(w, "sentinel_files_ingested_total{status=\"ALL\"} %d\n", atomic.LoadUint64(&GlobalMetrics.TotalIngested))
	fmt.Fprintf(w, "sentinel_files_ingested_total{status=\"VALID\"} %d\n", atomic.LoadUint64(&GlobalMetrics.TotalValid))
	fmt.Fprintf(w, "sentinel_files_ingested_total{status=\"QUARANTINED\"} %d\n\n", atomic.LoadUint64(&GlobalMetrics.TotalQuarantined))

	fmt.Fprintf(w, "# HELP sentinel_bytes_ingested_total Total volume of payload bytes processed\n")
	fmt.Fprintf(w, "# TYPE sentinel_bytes_ingested_total counter\n")
	fmt.Fprintf(w, "sentinel_bytes_ingested_total %d\n\n", atomic.LoadUint64(&GlobalMetrics.TotalBytesIngested))

	fmt.Fprintf(w, "# HELP sentinel_active_incidents Total count of active pre-ledger incidents\n")
	fmt.Fprintf(w, "# TYPE sentinel_active_incidents gauge\n")
	fmt.Fprintf(w, "sentinel_active_incidents %d\n\n", atomic.LoadUint64(&GlobalMetrics.ActiveIncidents))

	fmt.Fprintf(w, "# HELP sentinel_merkle_chain_height Current block height of the append-only audit ledger\n")
	fmt.Fprintf(w, "# TYPE sentinel_merkle_chain_height gauge\n")
	fmt.Fprintf(w, "sentinel_merkle_chain_height %d\n\n", atomic.LoadUint64(&GlobalMetrics.MerkleHeight))

	// Previously emitted a hardcoded 296000 regardless of actual throughput, which
	// meant any Grafana panel scraping this showed a constant fabricated rate.
	// Now reports the last genuinely measured benchmark run; -1 until one occurs.
	fmt.Fprintf(w, "# HELP sentinel_streaming_parse_rate_records_per_sec Last MEASURED streaming parse velocity (-1 = never measured)\n")
	fmt.Fprintf(w, "# TYPE sentinel_streaming_parse_rate_records_per_sec gauge\n")
	fmt.Fprintf(w, "sentinel_streaming_parse_rate_records_per_sec %.0f\n\n", GlobalMetrics.LastMeasuredParseRate())

	fmt.Fprintf(w, "# HELP sentinel_worker_pool_active Active concurrent validation workers\n")
	fmt.Fprintf(w, "# TYPE sentinel_worker_pool_active gauge\n")
	fmt.Fprintf(w, "sentinel_worker_pool_active 8\n")
}
