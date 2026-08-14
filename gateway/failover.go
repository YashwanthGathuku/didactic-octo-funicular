package main

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
)

// FailoverSimulationResult represents automated cross-region DR failover telemetry
type FailoverSimulationResult struct {
	SimulationID          string    `json:"simulationId"`
	PrimaryRegion         string    `json:"primaryRegion"`
	StandbyRegion         string    `json:"standbyRegion"`
	FailoverTriggerReason string    `json:"failoverTriggerReason"`
	IsScriptedDemo        bool      `json:"isScriptedDemo"`
	Disclaimer            string    `json:"disclaimer"`
	ElapsedMsScripted     float64   `json:"elapsedMsScripted"`
	RpoSecondsTarget      float64   `json:"rpoSecondsTarget"`      // TARGET, not measured
	RtoMillisecondsTarget float64   `json:"rtoMillisecondsTarget"` // TARGET, not measured
	ReplicatedBlocksCount int64     `json:"replicatedBlocksCount"`
	DataLossTransactionCount int   `json:"dataLossTransactionCount"`
	StandbyHealthStatus   string    `json:"standbyHealthStatus"` // "ACTIVE_PROMOTED", "SYNC_HEALTHY"
	Timestamp             time.Time `json:"timestamp"`
}

// SimulateCrossRegionFailover renders a SCRIPTED demonstration of a DR failover.
//
// HONESTY NOTE (2026-08-14): this function does not perform a failover. It
// sleeps for a fixed 42ms and then measures how long it slept. There is no
// second region, no replica, no replication stream, and no promotion. The
// previously advertised "RTO = 42.5ms / RPO = 0.00s, 100% Proven" was a
// measurement of time.Sleep(42ms) and a struct literal respectively.
//
// The fields below are therefore explicitly marked as scripted. Measuring a
// real RTO requires killing a real primary and timing a real promotion; until
// that exists, no RTO/RPO number from this codebase may be published.
func SimulateCrossRegionFailover() FailoverSimulationResult {
	start := time.Now()
	time.Sleep(42 * time.Millisecond) // scripted delay, NOT failover work
	elapsed := float64(time.Since(start).Microseconds()) / 1000.0

	return FailoverSimulationResult{
		IsScriptedDemo:     true,
		Disclaimer:         "SCRIPTED DEMONSTRATION. No failover occurred; no replica exists. RPO/RTO below are illustrative targets, not measurements.",
		ElapsedMsScripted:  elapsed,
		SimulationID:             fmt.Sprintf("DR-FAILOVER-%d", time.Now().Unix()),
		PrimaryRegion:            "us-east-1 (N. Virginia Active)",
		StandbyRegion:            "us-west-2 (Oregon Standby)",
		FailoverTriggerReason:    "SIMULATED_PRIMARY_DATACENTER_OUTAGE",
		RpoSecondsTarget:         0.00,
		RtoMillisecondsTarget:    42.5,
		ReplicatedBlocksCount:    0, // unknown: no replication stream exists
		DataLossTransactionCount: 0,
		StandbyHealthStatus:      "NOT_PROVISIONED",
		Timestamp:                time.Now().UTC(),
	}
}

// RegisterFailoverRoutes wires disaster recovery endpoints into Chi router
func RegisterFailoverRoutes(r chi.Router, db *sql.DB) {
	r.Route("/chaos/failover", func(r chi.Router) {
		// POST /api/v1/chaos/failover/simulate
		r.Post("/simulate", func(w http.ResponseWriter, r *http.Request) {
			result := SimulateCrossRegionFailover()
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(result)
		})
	})
}
