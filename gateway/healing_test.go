package main

import (
	"strings"
	"testing"
)

func TestSelfHealingProposalGeneration(t *testing.T) {
	// Sample corrupted NACHA with invalid Mod10 digit (021000021 instead of 021000028)
	corruptedContent := "101 021000021 1234567892608141430A094101MERIDIAN CUSTODY        SENTINEL FLOW          \n" +
		"5200PAYROLL   CORP INC        0001234567PPDDIRECT PAY260814260814   1021000020000001\n" +
		"6220210000218420000245000999888800John Doe                 0021000020000001\n" +
		"820000000100021000020000002450000000000000000001234567                         021000020000001\n" +
		"900000100000100000001000210000200000024500000000000000                         \n"

	proposal := GenerateSelfHealingProposal(101, 501, corruptedContent)

	if proposal == nil {
		t.Fatalf("Expected non-nil self-healing proposal")
	}

	if proposal.Status != "DRY_RUN_PASSED" {
		t.Errorf("Expected status DRY_RUN_PASSED, got %s", proposal.Status)
	}

	if len(proposal.Patches) == 0 {
		t.Errorf("Expected at least 1 patch to be proposed, got 0")
	}

	// Verify that the proposed patch fixed the Mod10 routing digit to 8
	hasMod10Fix := false
	for _, p := range proposal.Patches {
		if strings.Contains(p.RepairedText, "021000028") {
			hasMod10Fix = true
			break
		}
	}

	if !hasMod10Fix {
		t.Errorf("Proposed patches did not include Mod10 routing correction to 021000028")
	}

	if proposal.OriginalSha256 == proposal.RepairedSha256 {
		t.Errorf("Expected different SHA-256 hashes after repair")
	}
}

func TestDriftMetricsCalculation(t *testing.T) {
	report := CalculateDriftMetrics()

	if report.PartnerID != "PARTNER-MERIDIAN-01" {
		t.Errorf("Expected partner ID PARTNER-MERIDIAN-01, got %s", report.PartnerID)
	}

	if len(report.Metrics) < 3 {
		t.Errorf("Expected at least 3 drift metrics, got %d", len(report.Metrics))
	}

	// Verify detection of DiscretionaryData null rate drift
	hasNullDrift := false
	for _, m := range report.Metrics {
		if m.FieldName == "DiscretionaryData_NullRate" && m.IsSignificant {
			hasNullDrift = true
			break
		}
	}

	if !hasNullDrift {
		t.Errorf("Failed to detect significant drift in DiscretionaryData_NullRate")
	}
}
