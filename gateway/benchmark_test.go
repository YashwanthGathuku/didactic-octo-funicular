package main

import (
	"database/sql"
	_ "modernc.org/sqlite"
	"testing"
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

	// The rule id changed with the rule registry introduced in Prompt 07. The
	// old ACH_ERR_0802_HASH_MISMATCH carried no version and no provenance, so a
	// finding recorded under it could not say which formulation produced it.
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
	// Drop the append-only guard first: this test models an attacker with direct
	// database access, which is the only actor who could mutate these rows.
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

func BenchmarkNachaParser_100k(b *testing.B) {
	for i := 0; i < b.N; i++ {
		metrics := RunStreamingBenchmark(100000)
		if metrics.TotalRecordsParsed == 0 {
			b.Fatalf("Parsed 0 records")
		}
	}
}
