package main

import (
	"database/sql"
	"strings"
	"testing"

	_ "modernc.org/sqlite"
)

func setupTestDb(t *testing.T) *sql.DB {
	db, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatalf("Failed to open in-memory sqlite: %v", err)
	}

	schema := `
	CREATE TABLE partners (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		name TEXT NOT NULL,
		routing_number TEXT NOT NULL,
		created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
	);
	CREATE TABLE file_contracts (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		partner_id INTEGER NOT NULL,
		name TEXT NOT NULL,
		direction TEXT NOT NULL,
		filename_pattern TEXT NOT NULL,
		expected_time TEXT NOT NULL,
		grace_period_minutes INTEGER NOT NULL DEFAULT 15,
		timezone TEXT NOT NULL DEFAULT 'America/New_York'
	);
	CREATE TABLE expectations (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		contract_id INTEGER NOT NULL,
		window_start TIMESTAMP NOT NULL,
		window_end TIMESTAMP NOT NULL,
		status TEXT NOT NULL DEFAULT 'WAITING',
		created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
	);
	CREATE TABLE file_instances (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		expectation_id INTEGER,
		filename TEXT NOT NULL,
		storage_path TEXT NOT NULL,
		size_bytes INTEGER NOT NULL,
		sha256_hash TEXT NOT NULL,
		status TEXT NOT NULL,
		received_at TIMESTAMP NOT NULL,
		created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
	);
	CREATE TABLE validation_findings (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		file_instance_id INTEGER NOT NULL,
		code TEXT NOT NULL,
		description TEXT NOT NULL,
		severity TEXT NOT NULL,
		line_number INTEGER,
		raw_data TEXT,
		created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
	);
	CREATE TABLE incidents (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		expectation_id INTEGER,
		file_instance_id INTEGER,
		type TEXT NOT NULL,
		severity TEXT NOT NULL,
		status TEXT NOT NULL DEFAULT 'OPEN',
		created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
	);
	CREATE TABLE audit_events (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		event_type TEXT NOT NULL,
		actor TEXT NOT NULL,
		payload TEXT NOT NULL,
		previous_hash TEXT NOT NULL,
		current_hash TEXT NOT NULL,
		created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
	);
	`
	if _, err := db.Exec(schema); err != nil {
		t.Fatalf("Failed to initialize test schema: %v", err)
	}

	return db
}

func TestE2E_ValidNachaPipeline(t *testing.T) {
	db := setupTestDb(t)
	defer db.Close()

	scenario := GenerateNachaScenario(PresetBalancedPayroll)
	res, err := ProcessFileBytes(db, scenario.Filename, []byte(scenario.Content))
	if err != nil {
		t.Fatalf("ProcessFileBytes failed: %v", err)
	}

	if res.Status != "RELEASED" {
		for _, f := range res.Findings {
			t.Logf("Finding: %s - %s (line %d)", f.Code, f.Description, f.LineNumber)
		}
		t.Errorf("Expected status RELEASED for valid NACHA, got %s", res.Status)
	}
	if len(res.Findings) != 0 {
		for _, f := range res.Findings {
			t.Logf("Remaining Finding: %s - %s (line %d)", f.Code, f.Description, f.LineNumber)
		}
		t.Errorf("Expected 0 findings for valid NACHA, got %d", len(res.Findings))
	}
	if !res.IsBalanced {
		t.Errorf("Expected valid NACHA to be balanced")
	}
}

func TestE2E_CorruptedHashQuarantine(t *testing.T) {
	db := setupTestDb(t)
	defer db.Close()

	scenario := GenerateNachaScenario(PresetCorruptedEntryHash)
	res, err := ProcessFileBytes(db, scenario.Filename, []byte(scenario.Content))
	if err != nil {
		t.Fatalf("ProcessFileBytes failed: %v", err)
	}

	if res.Status != "QUARANTINED" {
		t.Errorf("Expected status QUARANTINED for corrupted entry hash, got %s", res.Status)
	}
	if len(res.Findings) == 0 {
		t.Errorf("Expected findings for corrupted hash, got 0")
	}
}

func TestE2E_InvalidAbaRoutingQuarantine(t *testing.T) {
	db := setupTestDb(t)
	defer db.Close()

	scenario := GenerateNachaScenario(PresetInvalidAbaRouting)
	res, err := ProcessFileBytes(db, scenario.Filename, []byte(scenario.Content))
	if err != nil {
		t.Fatalf("ProcessFileBytes failed: %v", err)
	}

	if res.Status != "QUARANTINED" {
		t.Errorf("Expected status QUARANTINED for invalid ABA, got %s", res.Status)
	}
}

func TestE2E_Iso20022XmlValidation(t *testing.T) {
	sampleXml := `<?xml version="1.0" encoding="UTF-8"?>
	<Document xmlns="urn:iso:std:iso:20022:tech:xsd:pacs.008.001.08">
		<FIToFICstmrCdtTrf>
			<GrpHdr>
				<MsgId>MSG-2026-0814-001</MsgId>
				<CreDtTm>2026-08-14T10:00:00Z</CreDtTm>
				<NbOfTxs>1</NbOfTxs>
				<TtlIntrBkSttlmAmt>50000.00</TtlIntrBkSttlmAmt>
				<IntrBkSttlmDt>2026-08-14</IntrBkSttlmDt>
			</GrpHdr>
			<CdtTrfTxInf>
				<PmtId>
					<EndToEndId>E2E-CITI-001</EndToEndId>
				</PmtId>
				<IntrBkSttlmAmt>50000.00</IntrBkSttlmAmt>
			</CdtTrfTxInf>
		</FIToFICstmrCdtTrf>
	</Document>`

	findings, _, credits, count, _ := ValidateIso20022Xml([]byte(sampleXml))
	if len(findings) != 0 {
		t.Errorf("Expected 0 findings for valid ISO 20022 XML, got %d", len(findings))
	}
	if credits != 50000.00 {
		t.Errorf("Expected 50000.00 credits, got %.2f", credits)
	}
	if count != 1 {
		t.Errorf("Expected 1 transaction, got %d", count)
	}
}

func TestE2E_MerkleLedgerAndCompliancePackage(t *testing.T) {
	db := setupTestDb(t)
	defer db.Close()

	_, err := AppendAuditEvent(db, "FILE_INGESTED", "SYSTEM_PROCESSOR", map[string]interface{}{
		"filename": "test.ach",
		"sha256":   "e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855",
	})
	if err != nil {
		t.Fatalf("Failed to append audit event: %v", err)
	}

	ledger, err := GetLedger(db)
	if err != nil || !ledger.IsChainValid {
		t.Errorf("Expected valid Merkle chain, err=%v", err)
	}

	pkg, err := GenerateCompliancePackage(db)
	if err != nil {
		t.Fatalf("GenerateCompliancePackage failed: %v", err)
	}

	if !strings.Contains(pkg.RegulatoryStandard, "SEC Rule 17a-4") {
		t.Errorf("Unexpected compliance standard: %s", pkg.RegulatoryStandard)
	}
	if !pkg.ChainIntegrityVerified {
		t.Errorf("Expected compliance package chain integrity to be verified")
	}
}
