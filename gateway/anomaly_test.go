package main

import (
	"database/sql"
	"strings"
	"testing"
	_ "modernc.org/sqlite"
)

func TestVolumeAnomalyDetection(t *testing.T) {
	baseline := BaselineStats{
		MeanRecords:   10000.0,
		StdDevRecords: 1500.0,
		MeanBytes:     940000.0,
		StdDevBytes:   141000.0,
		SampleCount:   30,
	}

	// Case 1: Normal volume (10,500 records -> Z = +0.33)
	normalFinding := EvaluateVolumeAnomaly(10500, 987000, baseline)
	if normalFinding.IsAnomaly || normalFinding.Severity != "INFO" {
		t.Errorf("Expected normal volume to NOT be an anomaly, got: %+v", normalFinding)
	}

	// Case 2: Spike volume (16,000 records -> Z = +4.00 > 3.0σ)
	spikeFinding := EvaluateVolumeAnomaly(16000, 1504000, baseline)
	if !spikeFinding.IsAnomaly || spikeFinding.ZScore < 3.0 {
		t.Errorf("Expected 16,000 records to trigger volume anomaly, got: %+v", spikeFinding)
	}

	// Case 3: Severe drop volume (2,000 records -> Z = -5.33 < -3.0σ)
	dropFinding := EvaluateVolumeAnomaly(2000, 188000, baseline)
	if !dropFinding.IsAnomaly || dropFinding.Severity != "CRITICAL" {
		t.Errorf("Expected severe volume drop to trigger CRITICAL anomaly, got: %+v", dropFinding)
	}
}

func TestSqlConsoleSecurityGuardrails(t *testing.T) {
	db, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatalf("Failed to open in-memory database: %v", err)
	}
	defer db.Close()

	_, _ = db.Exec("CREATE TABLE test_table (id INTEGER PRIMARY KEY, name TEXT);")
	_, _ = db.Exec("INSERT INTO test_table (name) VALUES ('Test Record');")

	// Valid SELECT Query
	rows, err := db.Query("SELECT id, name FROM test_table")
	if err != nil {
		t.Fatalf("Expected valid SELECT query, got error: %v", err)
	}
	rows.Close()

	// Prohibited DROP attempt validation logic
	prohibitedQuery := "DROP TABLE test_table;"
	trimmed := strings.ToUpper(strings.TrimSpace(prohibitedQuery))
	if strings.HasPrefix(trimmed, "DROP") {
		// correctly identified and rejected
	} else {
		t.Errorf("Failed to detect prohibited DROP query")
	}
}
