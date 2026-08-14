package main

import (
	"strings"
	"testing"
)

func TestValidSwiftMT103(t *testing.T) {
	sampleMT103 := `{1:F01MERIUS33AXXX0000000000}{2:I103ATLTAUS33XXXXN}{4:
:20:TRX-20260814-001
:23B:CRED
:32A:260814USD250000,00
:50K:/1234567890
MERIDIAN CUSTODY TREASURY
:59:/9876543210
APEX SETTLEMENT CORP
:71A:SHA
-}`

	if !IsSwiftMessage(sampleMT103) {
		t.Fatalf("Expected sample to be detected as SWIFT message")
	}

	findings, debits, credits, balanced := ParseAndValidateSwift(sampleMT103)

	if len(findings) > 0 {
		t.Errorf("Expected 0 findings for valid MT103, got %d: %v", len(findings), findings)
	}

	if credits != 250000.00 {
		t.Errorf("Expected settled amount $250,000.00, got %.2f", credits)
	}

	if !balanced {
		t.Errorf("Expected balanced MT103 transaction")
	}
	_ = debits
}

func TestCorruptedSwiftMT103MissingTag(t *testing.T) {
	// Missing mandatory :59: Beneficiary Customer
	corruptedMT103 := `{1:F01MERIUS33AXXX0000000000}{2:I103ATLTAUS33XXXXN}{4:
:20:TRX-20260814-002
:23B:CRED
:32A:260814USD100000,00
:50K:/1234567890
MERIDIAN CUSTODY
:71A:SHA
-}`

	findings, _, _, balanced := ParseAndValidateSwift(corruptedMT103)

	if len(findings) == 0 {
		t.Errorf("Expected findings for missing tag :59:, got 0")
	}

	hasMissingTag59 := false
	for _, f := range findings {
		if strings.Contains(f.Code, "59") {
			hasMissingTag59 = true
			break
		}
	}

	if !hasMissingTag59 {
		t.Errorf("Expected finding for missing :59:, got %v", findings)
	}

	if balanced {
		t.Errorf("Expected unbalanced/invalid state for incomplete MT103")
	}
}

func TestValidSwiftMT940Statement(t *testing.T) {
	sampleMT940 := `{1:F01MERIUS33AXXX0000000000}{2:I940ATLTAUS33XXXXN}{4:
:20:STMT-20260814
:25:ACC-9988776655
:28C:00001/001
:60F:C260814USD1000000,00
:61:2608140814CR250000,00NTRFNONREF//REF-001
:62F:C260814USD1250000,00
-}`

	findings, debits, credits, balanced := ParseAndValidateSwift(sampleMT940)

	if len(findings) > 0 {
		t.Errorf("Expected 0 findings for valid MT940 statement, got %d: %v", len(findings), findings)
	}

	if debits != 1000000.00 || credits != 1250000.00 {
		t.Errorf("Expected opening balance $1M and closing $1.25M, got debits=%.2f credits=%.2f", debits, credits)
	}

	if !balanced {
		t.Errorf("Expected balanced state for valid MT940")
	}
}
