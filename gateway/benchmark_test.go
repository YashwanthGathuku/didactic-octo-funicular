package main

import (
	"database/sql"
	"strings"
	"testing"

	_ "modernc.org/sqlite"

	"sentinel-gateway/internal/nacha"
)

func TestMod10RoutingValidation(t *testing.T) {
	// Valid Federal Reserve routing numbers
	validRoutings := []string{
		"021000021", // District 02 (NY)
		"121000248", // District 12 (SF)
		"011000015", // District 01 (Boston)
		"021000018", // District 02 Commercial
		"071000013", // District 07 (Chicago)
	}

	for _, routing := range validRoutings {
		if !ValidateRoutingMod10(routing) {
			t.Errorf("Expected valid Mod10 for routing %s, but got invalid", routing)
		}
	}

	// Invalid routing numbers
	invalidRoutings := []string{
		"999999999",
		"021000022",
		"123456789",
		"000000001",
	}

	for _, routing := range invalidRoutings {
		if ValidateRoutingMod10(routing) {
			t.Errorf("Expected invalid Mod10 for routing %s, but got valid", routing)
		}
	}
}

func TestNachaCorruptedEntryHashDetection(t *testing.T) {
	db, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatalf("Failed to open test database: %v", err)
	}
	defer db.Close()

	if _, err := Migrate(db); err != nil {
		t.Fatalf("Failed to initialize test schema: %v", err)
	}

	corruptedScenario := GenerateNachaScenario(PresetCorruptedEntryHash)
	result, err := ProcessFileBytes(db, DefaultTenantID, corruptedScenario.Filename, []byte(corruptedScenario.Content))
	if err != nil {
		t.Fatalf("ProcessFileBytes failed: %v", err)
	}

	if result.Status != "QUARANTINED" {
		t.Errorf("Expected status QUARANTINED for corrupted entry hash, got %s", result.Status)
	}

	foundHashMismatch := false
	for _, f := range result.Findings {
		if f.Code == "NACHA.MATH.BATCH_ENTRY_HASH" {
			foundHashMismatch = true
			if f.Severity != "BLOCKING" {
				t.Errorf("an entry hash mismatch was recorded at severity %q", f.Severity)
			}
			if f.RuleVersion == "" {
				t.Error("the finding carries no rule version")
			}
			if f.Expected == "" || f.Actual == "" {
				t.Error("the finding does not report both sides of the disagreement")
			}
			break
		}
	}

	if !foundHashMismatch {
		t.Errorf("Expected NACHA.MATH.BATCH_ENTRY_HASH in findings, but got: %+v", result.Findings)
	}
}

func TestHashChainTamperDetection(t *testing.T) {
	db, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatalf("Failed to open test database: %v", err)
	}
	defer db.Close()

	if _, err := Migrate(db); err != nil {
		t.Fatalf("Failed to initialize test schema: %v", err)
	}

	// Append 3 legitimate events
	_, _ = AppendAuditEvent(db, DefaultTenantID, "EVENT_1", "ACTOR_A", map[string]interface{}{"val": 1})
	_, _ = AppendAuditEvent(db, DefaultTenantID, "EVENT_2", "ACTOR_B", map[string]interface{}{"val": 2})
	_, _ = AppendAuditEvent(db, DefaultTenantID, "EVENT_3", "ACTOR_C", map[string]interface{}{"val": 3})

	ledger, err := GetLedger(db, DefaultTenantID)
	if err != nil {
		t.Fatalf("GetLedger failed: %v", err)
	}

	if !ledger.IsChainValid {
		t.Errorf("Expected legitimate ledger chain to be valid")
	}

	// Deliberately tamper with historical event #2's current_hash
	_, _ = db.Exec(`DROP TRIGGER IF EXISTS audit_events_no_update`)
	_, err = db.Exec("UPDATE audit_events SET current_hash = 'TAMPERED_FAKE_HASH_0000000000000000000000000000000000000000' WHERE id = 2")
	if err != nil {
		t.Fatalf("Tamper SQL update failed: %v", err)
	}

	tamperedLedger, err := GetLedger(db, DefaultTenantID)
	if err != nil {
		t.Fatalf("GetLedger failed after tampering: %v", err)
	}

	if tamperedLedger.IsChainValid {
		t.Errorf("Expected tampered ledger to fail verification, but got IsChainValid = true")
	}
}

func TestBenchmarkHarnessPresetsAndPercentiles(t *testing.T) {
	// Run small preset harness (100 records, 2 workers, 3 iterations)
	res := RunHarness("small", 100, 2, 3)

	if res.RecordCount != 100 {
		t.Errorf("expected 100 records, got %d", res.RecordCount)
	}
	if res.Concurrency != 2 {
		t.Errorf("expected concurrency 2, got %d", res.Concurrency)
	}
	if res.Iterations != 3 {
		t.Errorf("expected 3 iterations, got %d", res.Iterations)
	}
	if res.TotalRecords != int64((100+4)*2*3) {
		t.Errorf("expected %d total records, got %d", (100+4)*2*3, res.TotalRecords)
	}
	if res.P50LatencyMs < 0 || res.P95LatencyMs < 0 || res.P99LatencyMs < 0 {
		t.Errorf("expected non-negative latency percentiles, got p50=%g, p95=%g, p99=%g",
			res.P50LatencyMs, res.P95LatencyMs, res.P99LatencyMs)
	}
	if res.P95LatencyMs < res.P50LatencyMs {
		t.Errorf("expected p95 (%g) >= p50 (%g)", res.P95LatencyMs, res.P50LatencyMs)
	}
	if res.P99LatencyMs < res.P95LatencyMs {
		t.Errorf("expected p99 (%g) >= p95 (%g)", res.P99LatencyMs, res.P95LatencyMs)
	}
	if res.RecordsPerSec <= 0 {
		t.Errorf("expected positive records per second, got %g", res.RecordsPerSec)
	}
	if res.ValidCount != 6 { // 2 workers * 3 iterations
		t.Errorf("expected 6 valid results, got %d", res.ValidCount)
	}
	if res.Errors > 0 {
		t.Errorf("expected 0 errors, got %d", res.Errors)
	}
}

func TestBenchmarkHarnessRejectsStructurallyInvalidFiles(t *testing.T) {
	// Corrupting corpus by removing 94-char padding or altering control math
	validCorpus := GenerateLargeNachaCorpus(50)
	lines := strings.Split(string(validCorpus), "\n")

	// 1. Truncate a record (e.g. shorten entry record 2)
	lines[2] = lines[2][:50]
	corrupted := strings.Join(lines, "\n")

	metrics := RunStreamingBenchmark(10)
	if metrics.RecordsPerSecond <= 0 {
		t.Errorf("expected positive records per second for valid benchmark run")
	}

	// Parsing corrupted byte slice with nacha validator directly
	res, _ := nacha.Validate(strings.NewReader(corrupted))
	if res != nil {
		decision := nacha.Decide(res, nacha.DefaultContract)
		if !decision.Quarantined() {
			t.Errorf("expected corrupted NACHA corpus to fail validation and be quarantined")
		}
	}
}

func BenchmarkNachaParser_100k(b *testing.B) {
	for i := 0; i < b.N; i++ {
		metrics := RunStreamingBenchmark(100000)
		if metrics.TotalRecordsParsed == 0 {
			b.Fatalf("Parsed 0 records")
		}
	}
}
