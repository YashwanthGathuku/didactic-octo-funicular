package main

import (
	"strings"
	"testing"
)

// These tests pin the single most dangerous behaviour in the ingestion path:
// unusable input must never reach RELEASED.
//
// Before the Prompt 01 fix, ProcessFileBytes initialised every result to
// RELEASED and downgraded only on a positive finding. The Moov ACH parser
// exception was recorded at severity WARNING and was the one finding branch
// that did not set QUARANTINED, so a zero-byte file was released and reported
// balanced. Reproduced against a clean source build in
// docs/engineering/CURRENT_STATE.md §7.

// mustProcess runs the ingestion path and fails the test on a transport-level
// error, so each test body can focus on the release decision itself.
func mustProcess(t *testing.T, filename, content string) *IngestionResult {
	t.Helper()
	db := setupTestDb(t)
	defer db.Close()

	res, err := ProcessFileBytes(db, DefaultTenantID, filename, []byte(content))
	if err != nil {
		t.Fatalf("ProcessFileBytes(%q) returned an error: %v", filename, err)
	}
	if res == nil {
		t.Fatalf("ProcessFileBytes(%q) returned a nil result", filename)
	}
	return res
}

func TestEmptyFileIsQuarantined(t *testing.T) {
	res := mustProcess(t, "empty.ach", "")

	if res.Status == "RELEASED" {
		t.Errorf("a zero-byte file was RELEASED; empty input must fail closed")
	}
	if res.Status != "QUARANTINED" {
		t.Errorf("expected QUARANTINED for a zero-byte file, got %q", res.Status)
	}
	if len(res.Findings) == 0 {
		t.Errorf("expected at least one finding explaining the quarantine, got 0")
	}
}

func TestEmptyFileIsNotReportedBalanced(t *testing.T) {
	res := mustProcess(t, "empty.ach", "")

	// 0 debits == 0 credits is arithmetically true and operationally meaningless.
	// Reporting isBalanced on a file with no records asserts a property of
	// records that do not exist.
	if res.IsBalanced {
		t.Errorf("a file with %d parsed records was reported isBalanced=true", res.TotalRecordsParsed)
	}
	if res.TotalRecordsParsed != 0 {
		t.Errorf("expected 0 records parsed for an empty file, got %d", res.TotalRecordsParsed)
	}
}

func TestWhitespaceOnlyFileIsQuarantined(t *testing.T) {
	res := mustProcess(t, "blank.ach", "\n\n   \n\t\n")

	if res.Status != "QUARANTINED" {
		t.Errorf("expected QUARANTINED for a whitespace-only file, got %q", res.Status)
	}
	if res.IsBalanced {
		t.Errorf("a whitespace-only file was reported isBalanced=true")
	}
}

func TestParserExceptionQuarantines(t *testing.T) {
	// Structurally wrong: right record width, but not a NACHA file. The Moov
	// reader rejects it, and that rejection alone must be disqualifying.
	junk := strings.Repeat("X", 94)
	res := mustProcess(t, "garbage.ach", junk+"\n")

	if res.Status == "RELEASED" {
		t.Errorf("input the parser could not read was RELEASED")
	}
	if res.Status != "QUARANTINED" {
		t.Errorf("expected QUARANTINED on parser failure, got %q", res.Status)
	}
}

func TestTruncatedFileIsQuarantined(t *testing.T) {
	// A file header and nothing else: no batches, no control record.
	scenario := GenerateNachaScenario(PresetBalancedPayroll)
	lines := strings.Split(strings.TrimSpace(scenario.Content), "\n")
	if len(lines) < 2 {
		t.Fatalf("fixture too short to truncate: %d lines", len(lines))
	}

	res := mustProcess(t, "truncated.ach", lines[0]+"\n")

	if res.Status != "QUARANTINED" {
		t.Errorf("expected QUARANTINED for a truncated file, got %q", res.Status)
	}
}

// TestParserExceptionIsNotAdvisory guards the specific regression: the parser
// exception must carry a disqualifying severity, not WARNING. A finding that
// cannot block a release is decoration.
func TestParserExceptionIsNotAdvisory(t *testing.T) {
	res := mustProcess(t, "empty.ach", "")

	for _, f := range res.Findings {
		if f.Code == "ACH_ERR_0099_PARSER_EXCEPTION" {
			if f.Severity == "WARNING" || f.Severity == "INFO" {
				t.Errorf("parser exception recorded at non-blocking severity %q", f.Severity)
			}
			return
		}
	}
	t.Errorf("expected a parser-exception finding for an empty file, got none")
}

// TestValidNachaReachesValidated is the counterweight: failing closed must not
// mean failing always. If this breaks, the fix is too broad.
//
// The success terminus is VALIDATED. Ingestion does not release; release needs
// a policy decision and approval it has no authority to make.
func TestValidNachaReachesValidated(t *testing.T) {
	scenario := GenerateNachaScenario(PresetBalancedPayroll)
	res := mustProcess(t, scenario.Filename, scenario.Content)

	if res.Status != "VALIDATED" {
		for _, f := range res.Findings {
			t.Logf("finding: %s (severity %s, line %d) %s", f.Code, f.Severity, f.LineNumber, f.Description)
		}
		t.Errorf("expected VALIDATED for a valid NACHA fixture, got %q", res.Status)
	}
	if res.Status == "RELEASED" {
		t.Errorf("ingestion released a file without a policy decision or approval")
	}
	if res.TotalRecordsParsed == 0 {
		t.Errorf("expected a valid fixture to parse at least one record")
	}
}
