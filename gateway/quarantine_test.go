package main

import (
	"bytes"
	"database/sql"
	"encoding/json"
	"fmt"
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

func TestEmptyFileReportsNoBalanceVerdict(t *testing.T) {
	res := mustProcess(t, "empty.ach", "")

	// The isBalanced field this once asserted against is gone. 0 debits ==
	// 0 credits is arithmetically true and operationally meaningless, and more
	// importantly whether a file must balance is a term of the feed contract
	// rather than a property of the file -- a credit-only payroll file never
	// balances and is entirely correct. What is asserted now is that an empty
	// file produces no records, no totals, and no verdict.
	if res.TotalRecordsParsed != 0 {
		t.Errorf("expected 0 records parsed for an empty file, got %d", res.TotalRecordsParsed)
	}
	if res.TotalDebitsMinor != 0 || res.TotalCreditsMinor != 0 {
		t.Errorf("an empty file reported totals %d/%d", res.TotalDebitsMinor, res.TotalCreditsMinor)
	}
	if res.PolicyVersion == "" {
		t.Error("the outcome carries no policy version; a decision without one is an opinion")
	}
	if len(res.QuarantineReasons) == 0 {
		t.Error("an empty file was quarantined with no stated reason")
	}
}

func TestWhitespaceOnlyFileIsQuarantined(t *testing.T) {
	res := mustProcess(t, "blank.ach", "\n\n   \n\t\n")

	if res.Status != "QUARANTINED" {
		t.Errorf("expected QUARANTINED for a whitespace-only file, got %q", res.Status)
	}
	if res.TotalDebitsMinor != 0 || res.TotalCreditsMinor != 0 {
		t.Errorf("a whitespace-only file reported totals %d/%d", res.TotalDebitsMinor, res.TotalCreditsMinor)
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

// TestParserExceptionIsNotAdvisory guards the specific regression: a file that
// cannot be read must carry a disqualifying severity, not WARNING. A finding
// that cannot block a release is decoration.
//
// The rule id changed with the registry introduced in Prompt 07: an empty file
// now raises NACHA.STRUCT.EMPTY, which is more precise than the parser
// exception it replaces -- "the parser threw" and "the file has no records" are
// different facts, and only the second is true of an empty file.
func TestParserExceptionIsNotAdvisory(t *testing.T) {
	res := mustProcess(t, "empty.ach", "")

	for _, f := range res.Findings {
		if f.Code != "NACHA.STRUCT.EMPTY" {
			continue
		}
		if f.Severity != "BLOCKING" {
			t.Errorf("an empty file was recorded at non-blocking severity %q", f.Severity)
		}
		if f.RuleVersion == "" || f.Provenance == "" {
			t.Errorf("the finding is not traceable: version=%q provenance=%q", f.RuleVersion, f.Provenance)
		}
		return
	}
	t.Errorf("expected a NACHA.STRUCT.EMPTY finding for an empty file, got %+v", res.Findings)
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

// Baseline item P0-11: raw financial record content must not be stored or
// returned.
//
// `validation_findings.raw_data` held the complete 94-character ACH record --
// account number, routing number, amount and trace number -- and
// GET /api/v1/incidents selected it and returned it. Every response, log line,
// support export and AI triage request that touched an incident carried it.
//
// This asserts the leak is closed at the source: nothing the processor writes
// contains a record from the file it processed.
func TestNoRawRecordContentIsStoredOrReturned(t *testing.T) {
	db := setupTestDb(t)
	defer db.Close()

	scenario := GenerateNachaScenario(PresetCorruptedEntryHash)
	res, err := ProcessFileBytes(db, DefaultTenantID, scenario.Filename, []byte(scenario.Content))
	if err != nil {
		t.Fatal(err)
	}
	if len(res.Findings) == 0 {
		t.Fatal("the fixture produced no findings, so this test would pass vacuously")
	}

	records := strings.Split(strings.TrimRight(scenario.Content, "\n"), "\n")

	// 1. Nothing in the returned result.
	returned, err := json.Marshal(res)
	if err != nil {
		t.Fatal(err)
	}
	for i, record := range records {
		if len(record) >= 20 && bytes.Contains(returned, []byte(record)) {
			t.Errorf("the ingestion result contains record %d in full", i+1)
		}
	}
	// Not even the account numbers on their own.
	for _, sensitive := range []string{"12345678901234567", "98765432101234567", "55544433321234567"} {
		if bytes.Contains(returned, []byte(sensitive)) {
			t.Errorf("the ingestion result contains the account number %s", sensitive)
		}
	}

	// 2. Nothing in any column of any row of the database.
	dump := dumpAllCells(t, db)
	for i, record := range records {
		if len(record) >= 20 && strings.Contains(dump, record) {
			t.Errorf("the database stores record %d in full", i+1)
		}
	}
	for _, sensitive := range []string{"12345678901234567", "98765432101234567", "55544433321234567"} {
		if strings.Contains(dump, sensitive) {
			t.Errorf("the database stores the account number %s", sensitive)
		}
	}
}

// dumpAllCells renders every value in every table as text, so a column added
// later is covered without editing this helper.
func dumpAllCells(t *testing.T, db *sql.DB) string {
	t.Helper()

	rows, err := db.Query(`SELECT name FROM sqlite_master WHERE type='table' AND name NOT LIKE 'sqlite_%'`)
	if err != nil {
		t.Fatal(err)
	}
	var tables []string
	for rows.Next() {
		var n string
		if err := rows.Scan(&n); err != nil {
			t.Fatal(err)
		}
		tables = append(tables, n)
	}
	rows.Close()

	var out strings.Builder
	for _, table := range tables {
		r, err := db.Query(`SELECT * FROM "` + table + `"`)
		if err != nil {
			continue
		}
		cols, _ := r.Columns()
		for r.Next() {
			cells := make([]any, len(cols))
			ptrs := make([]any, len(cols))
			for i := range cells {
				ptrs[i] = &cells[i]
			}
			if err := r.Scan(ptrs...); err != nil {
				continue
			}
			for i, c := range cells {
				fmt.Fprintf(&out, "%s.%s=%v\n", table, cols[i], c)
			}
		}
		r.Close()
	}
	return out.String()
}
