package main

import (
	"database/sql"
	"encoding/json"
	"strings"
	"testing"

	_ "modernc.org/sqlite"
)

func setupTestDb(t *testing.T) *sql.DB {
	db, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatalf("Failed to open in-memory sqlite: %v", err)
	}

	// Build the schema with the real migrator rather than a copy maintained by
	// hand. The hand-written copy drifted from the migrations the moment
	// tenant_id was added, so tests passed against a schema production did not
	// have.
	if _, err := Migrate(db); err != nil {
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

	// Ingestion ends at VALIDATED, not RELEASED. Release requires a versioned
	// policy decision and, where policy demands it, human approval -- none of
	// which ingestion performs. Before the domain model existed this function
	// returned RELEASED directly, which meant a file was marked usable
	// downstream without anyone deciding it should be.
	if res.Status != "VALIDATED" {
		for _, f := range res.Findings {
			t.Logf("Finding: %s - %s (line %d)", f.Code, f.Description, f.LineNumber)
		}
		t.Errorf("Expected status VALIDATED for valid NACHA, got %s", res.Status)
	}
	if res.Status == "RELEASED" {
		t.Errorf("ingestion must never release on its own")
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

func TestE2E_AuditChainAndEvidenceExport(t *testing.T) {
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
		t.Errorf("Expected valid application hash chain, err=%v", err)
	}

	pkg, err := GenerateCompliancePackage(db)
	if err != nil {
		t.Fatalf("GenerateCompliancePackage failed: %v", err)
	}

	if !pkg.ChainIntegrityVerified {
		t.Errorf("Expected evidence export chain integrity to be verified")
	}

	// The export must carry no regulatory or assurance claim. This assertion
	// previously required the string "SEC Rule 17a-4" to be PRESENT, which made
	// the test a guarantee that an unsupported compliance claim kept shipping.
	//
	// Limitations are excluded from the scan: that field is where the export
	// says what it is not ("not a Merkle history tree"), and naming the thing
	// you are disclaiming is the point of a disclaimer.
	if len(pkg.Limitations) == 0 {
		t.Errorf("evidence export must state its limitations")
	}
	scanned := *pkg
	scanned.Limitations = nil
	blob, err := json.Marshal(&scanned)
	if err != nil {
		t.Fatalf("failed to marshal evidence export: %v", err)
	}
	for _, banned := range []string{"SEC Rule 17a-4", "SOX 404", "FINRA", "Merkle", "SIMD", "Simd", "FIPS", "WORM"} {
		if strings.Contains(string(blob), banned) {
			t.Errorf("evidence export contains unsupported assurance claim %q", banned)
		}
	}
}
