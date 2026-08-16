package nacha

import (
	"errors"
	"strings"
	"testing"
)

func validate(t *testing.T, file string) *Result {
	t.Helper()
	res, err := Validate(strings.NewReader(file))
	if err != nil {
		t.Fatalf("Validate returned an error for a file it should have reported on: %v", err)
	}
	return res
}

// The fixture generator is only useful if its routing numbers are real.
func TestFixtureRoutingNumbersAreValid(t *testing.T) {
	for _, r := range []string{routingOrigin, routingRDFI, routingRDFI2} {
		if err := ValidateRoutingCheckDigit(r); err != nil {
			t.Errorf("fixture routing number %s is invalid: %v", r, err)
		}
	}
}

// --- The regressions this package exists for ---

// An empty file must never be released, must never be reported balanced, and
// must produce a finding that says why.
//
// This is the defect the Prompt 00 baseline reproduced against the running
// system: POST /files/ingest-raw with an empty body returned
// {"status":"RELEASED","isBalanced":true}.
func TestEmptyFileIsQuarantinedAndSaysWhy(t *testing.T) {
	for _, empty := range []string{"", "\n", "   \n  \n", "\n\n\n"} {
		res := validate(t, empty)

		if res.ParserOK {
			t.Errorf("%q: ParserOK is true for a file with no records", empty)
		}
		if res.RecordsParsed != 0 {
			t.Errorf("%q: RecordsParsed = %d", empty, res.RecordsParsed)
		}
		if !res.HasBlocking() {
			t.Errorf("%q: no blocking finding was raised", empty)
		}

		decision := Decide(res, DefaultContract)
		if decision.Outcome != OutcomeQuarantined {
			t.Errorf("%q: outcome = %s, want QUARANTINED", empty, decision.Outcome)
		}
		if len(decision.Reasons) == 0 {
			t.Errorf("%q: quarantined with no stated reason", empty)
		}
	}
}

// The empty file must not report itself balanced. There is no isBalanced field
// any more, and this asserts the totals that replaced it are both zero and that
// nothing derives a verdict from their equality.
func TestEmptyFileTotalsAreZeroAndCarryNoVerdict(t *testing.T) {
	res := validate(t, "")
	if res.TotalDebitsMinor != 0 || res.TotalCreditsMinor != 0 {
		t.Errorf("totals are %d/%d for an empty file", res.TotalDebitsMinor, res.TotalCreditsMinor)
	}
	// Equality of two zeroes must not produce a validated outcome.
	if d := Decide(res, FeedContract{ID: "C1", Version: "1", RequireBalanced: true}); d.Outcome != OutcomeQuarantined {
		t.Error("an empty file satisfied a balance requirement by having two zero totals")
	}
}

// A parser exception must quarantine. In the original implementation the parser
// exception was the one finding branch that did not set QUARANTINED, and it was
// recorded at WARNING.
func TestUnparseableInputIsQuarantined(t *testing.T) {
	cases := map[string]string{
		"plain text":          "this is not an ACH file at all\n",
		"JSON":                `{"amount": 100, "account": "1234"}` + "\n",
		"binary":              string([]byte{0x00, 0x01, 0x02, 0xff}) + "\n",
		"short records":       "1\n5\n6\n8\n9\n",
		"no file header":      strings.Join(strings.Split(strings.TrimRight(validSingleBatch(), "\n"), "\n")[1:], "\n") + "\n",
		"truncated mid batch": truncateAfter(validSingleBatch(), 3),
		"no file control":     truncateAfter(validSingleBatch(), 5),
	}

	for name, input := range cases {
		t.Run(name, func(t *testing.T) {
			res := validate(t, input)
			decision := Decide(res, DefaultContract)
			if decision.Outcome != OutcomeQuarantined {
				t.Errorf("outcome = %s, want QUARANTINED. findings: %d", decision.Outcome, len(res.Findings))
			}
			if len(res.Findings) == 0 {
				t.Error("quarantined with no findings; the operator cannot act on this")
			}
		})
	}
}

func truncateAfter(file string, records int) string {
	lines := strings.Split(strings.TrimRight(file, "\n"), "\n")
	if records > len(lines) {
		records = len(lines)
	}
	return strings.Join(lines[:records], "\n") + "\n"
}

// --- Valid fixtures ---

func TestValidFilesAreValidated(t *testing.T) {
	cases := map[string]string{
		"single batch, balanced":   validSingleBatch(),
		"multi batch with addenda": validMultiBatch(),
		"credit only, unbalanced":  validUnbalancedCreditOnly(),
	}

	for name, file := range cases {
		t.Run(name, func(t *testing.T) {
			res := validate(t, file)
			if !res.ParserOK {
				t.Fatalf("a valid file did not parse. findings: %+v", res.Findings)
			}
			for _, f := range res.Findings {
				if f.Blocking() {
					t.Errorf("a valid file raised a blocking finding: %s at record %d (expected %s, actual %s)",
						f.RuleID, f.RecordNumber, f.Expected, f.Actual)
				}
			}
			if d := Decide(res, DefaultContract); d.Outcome != OutcomeValidated {
				t.Errorf("outcome = %s, want VALIDATED. reasons: %v", d.Outcome, d.Reasons)
			}
		})
	}
}

// Balance is a contract term, not a correctness signal. The same credit-only
// file is correct under one contract and quarantined under another, and neither
// outcome is a property of the file alone.
func TestBalanceRequirementComesFromTheContract(t *testing.T) {
	file := validUnbalancedCreditOnly()
	res := validate(t, file)

	permissive := Decide(res, FeedContract{ID: "PAYROLL", Version: "1.0"})
	if permissive.Outcome != OutcomeValidated {
		t.Errorf("a credit-only payroll file was quarantined under a contract that does not require balance: %v",
			permissive.Reasons)
	}

	strict := Decide(res, FeedContract{ID: "OFFSET", Version: "1.0", RequireBalanced: true})
	if strict.Outcome != OutcomeQuarantined {
		t.Error("an unbalanced file was validated under a contract requiring balance")
	}
	if strict.ContractID != "OFFSET" {
		t.Errorf("the decision does not name the contract it applied: %+v", strict)
	}

	// And a balanced file satisfies the strict contract.
	balanced := validate(t, validSingleBatch())
	if d := Decide(balanced, FeedContract{ID: "OFFSET", Version: "1.0", RequireBalanced: true}); d.Outcome != OutcomeValidated {
		t.Errorf("a balanced file failed a balance requirement: %v", d.Reasons)
	}
}

// --- Invalid fixtures. The acceptance criterion is that none reaches release. ---

// invalidFixtures maps a name to a file that must never validate.
func invalidFixtures() map[string]string {
	valid := validSingleBatch()
	return map[string]string{
		"empty":                         "",
		"whitespace only":               "   \n\n  \n",
		"wrong record length":           corruptLength(valid, 2),
		"invalid record type":           corrupt(valid, 2, 1, 1, "3"),
		"invalid routing check digit":   corrupt(valid, 3, 4, 12, "121000359"),
		"non-numeric amount":            corrupt(valid, 3, 30, 39, "12E4567890"),
		"mismatched batch entry count":  corrupt(valid, 5, 5, 10, "000009"),
		"mismatched batch entry hash":   corrupt(valid, 5, 11, 20, "0000000001"),
		"mismatched batch debit total":  corrupt(valid, 5, 21, 32, "000000000001"),
		"mismatched batch credit total": corrupt(valid, 5, 33, 44, "000000000001"),
		"mismatched file batch count":   corrupt(valid, 6, 2, 7, "000009"),
		"mismatched file entry count":   corrupt(valid, 6, 14, 21, "00000009"),
		"mismatched file entry hash":    corrupt(valid, 6, 22, 31, "0000000001"),
		"mismatched file debit total":   corrupt(valid, 6, 32, 43, "000000000001"),
		"mismatched file credit total":  corrupt(valid, 6, 44, 55, "000000000001"),
		"orphan addenda":                orphanAddenda(),
		"orphan entry":                  orphanEntry(),
		"truncated mid batch":           truncateAfter(valid, 3),
		"no file control":               truncateAfter(valid, 5),
		"malformed characters":          corrupt(valid, 3, 20, 20, "\x01"),
		"unreachable declared total":    unreachableDeclaredTotal(),
	}
}

// corruptLength shortens a record so it is no longer 94 characters.
func corruptLength(file string, recordNumber int) string {
	lines := strings.Split(strings.TrimRight(file, "\n"), "\n")
	lines[recordNumber-1] = lines[recordNumber-1][:80]
	return strings.Join(lines, "\n") + "\n"
}

// orphanAddenda places an addenda record where no entry precedes it.
func orphanAddenda() string {
	lines := strings.Split(strings.TrimRight(validSingleBatch(), "\n"), "\n")
	addenda := "7" + "05" + text("ORPHANED ADDENDA", 80) + pad(1, 4) + pad(1, 7)
	// Insert immediately after the batch header, before any entry.
	out := append([]string{}, lines[:2]...)
	out = append(out, addenda)
	out = append(out, lines[2:]...)
	return strings.Join(out, "\n") + "\n"
}

// orphanEntry places an entry detail record outside any batch.
func orphanEntry() string {
	lines := strings.Split(strings.TrimRight(validSingleBatch(), "\n"), "\n")
	entry := lines[2] // a real entry record
	// Insert between the file header and the batch header.
	out := append([]string{}, lines[:1]...)
	out = append(out, entry)
	out = append(out, lines[1:]...)
	return strings.Join(out, "\n") + "\n"
}

// unreachableDeclaredTotal declares a file credit total the entries cannot sum
// to.
//
// An earlier version of this fixture was called "amount overflow" and claimed
// to drive the minor-unit accumulator past its range. It did not: 32 entries at
// the ten-character field maximum sum to about 3.2e11, and int64 holds 9.2e18.
// It passed only because of the corrupted control total, so the test asserted
// one thing and demonstrated another. The overflow path is exercised directly
// by TestAccumulatorOverflowIsBlocking below, where the boundary can actually
// be reached.
func unreachableDeclaredTotal() string {
	entries := make([]entrySpec, 0, 32)
	for i := 0; i < 32; i++ {
		entries = append(entries, entrySpec{
			transactionCode: "22", routing: routingRDFI,
			account: "9999999999", amountMinor: 9_999_999_999,
		})
	}
	file := buildFile([]batchSpec{{secCode: "PPD", number: 1, entries: entries}})
	return corrupt(file, len(strings.Split(strings.TrimRight(file, "\n"), "\n")), 44, 55, "999999999999")
}

// The headline acceptance criterion.
func TestNoInvalidFixtureCanReachRelease(t *testing.T) {
	for name, file := range invalidFixtures() {
		t.Run(name, func(t *testing.T) {
			res := validate(t, file)
			decision := Decide(res, DefaultContract)

			if decision.Outcome != OutcomeQuarantined {
				t.Fatalf("outcome = %s, want QUARANTINED.\nfindings: %+v", decision.Outcome, res.Findings)
			}
			if len(decision.Reasons) == 0 {
				t.Error("quarantined with no stated reason")
			}
			if len(decision.BlockingRuleIDs) == 0 && res.ParserOK {
				t.Error("quarantined with no blocking rule identified")
			}
		})
	}
}

// Every finding must be traceable to a versioned rule and a location.
func TestEveryFindingCarriesARuleVersionAndOffset(t *testing.T) {
	for name, file := range invalidFixtures() {
		res := validate(t, file)
		for _, f := range res.Findings {
			if f.RuleID == "" {
				t.Errorf("%s: a finding has no rule id", name)
			}
			if f.RuleVersion == "" {
				t.Errorf("%s: finding %s has no rule version", name, f.RuleID)
			}
			if f.Provenance == "" {
				t.Errorf("%s: finding %s has no provenance", name, f.RuleID)
			}
			if f.ByteOffset < 0 {
				t.Errorf("%s: finding %s has a negative byte offset", name, f.RuleID)
			}
		}
	}
}

// Determinism: two runs over the same bytes must produce identical findings.
// Evidence that changes between runs is not evidence.
func TestValidationIsDeterministic(t *testing.T) {
	for name, file := range invalidFixtures() {
		first := validate(t, file)
		for i := 0; i < 5; i++ {
			again := validate(t, file)
			if len(first.Findings) != len(again.Findings) {
				t.Fatalf("%s: run %d produced %d findings, first run produced %d",
					name, i, len(again.Findings), len(first.Findings))
			}
			for j := range first.Findings {
				if first.Findings[j] != again.Findings[j] {
					t.Errorf("%s: finding %d differs between runs:\n  %+v\n  %+v",
						name, j, first.Findings[j], again.Findings[j])
				}
			}
			if first.TotalDebitsMinor != again.TotalDebitsMinor ||
				first.TotalCreditsMinor != again.TotalCreditsMinor {
				t.Errorf("%s: totals differ between runs", name)
			}
		}
	}
}

// --- Redaction ---

// No finding, over any fixture, may contain a complete record or an
// unredacted account or routing number.
func TestNoFindingContainsRawRecordContent(t *testing.T) {
	all := invalidFixtures()
	all["valid single batch"] = validSingleBatch()
	all["valid multi batch"] = validMultiBatch()

	for name, file := range all {
		res := validate(t, file)
		records := strings.Split(strings.TrimRight(file, "\n"), "\n")

		for _, f := range res.Findings {
			blob := f.Description + "|" + f.Evidence + "|" + f.Expected + "|" + f.Actual

			// No complete record.
			for _, record := range records {
				if len(record) >= 20 && strings.Contains(blob, record) {
					t.Errorf("%s: finding %s contains a complete record", name, f.RuleID)
				}
			}
			// No evidence long enough to be a record fragment of consequence.
			if len([]rune(f.Evidence)) > maxEvidenceRunes+1 {
				t.Errorf("%s: finding %s carries %d characters of evidence", name, f.RuleID, len(f.Evidence))
			}
			// No unredacted routing or account number.
			for _, sensitive := range []string{routingRDFI, routingRDFI2, "1234567890", "0987654321"} {
				if strings.Contains(f.Evidence, sensitive) {
					t.Errorf("%s: finding %s evidence contains %q unredacted: %q",
						name, f.RuleID, sensitive, f.Evidence)
				}
			}
		}
	}
}

func TestRedactionHelpers(t *testing.T) {
	if got := RedactRouting("121000358"); got != "1210#####" {
		t.Errorf("RedactRouting = %q", got)
	}
	if got := RedactAccount("1234567890"); got != "######7890" {
		t.Errorf("RedactAccount = %q", got)
	}
	if got := RedactAccount("12"); got != "##" {
		t.Errorf("RedactAccount on a short value = %q", got)
	}
	if got := RedactEvidence("ACCT1234567"); strings.ContainsAny(got, "0123456789") {
		t.Errorf("RedactEvidence left digits: %q", got)
	}
	// A control character must not survive into a log line.
	if got := RedactEvidence("a\x00b\nc"); strings.ContainsAny(got, "\x00\n") {
		t.Errorf("RedactEvidence left a control character: %q", got)
	}
	// Length is bounded well below a record.
	long := RedactEvidence(strings.Repeat("x", 500))
	if len([]rune(long)) > maxEvidenceRunes+1 {
		t.Errorf("RedactEvidence produced %d characters", len([]rune(long)))
	}
}

// --- Rule registry properties ---

// A rule that cannot be verified must never block a release. Enforcing an
// invented rule is worse than not enforcing it: it rejects legitimate files
// with the same confidence as illegitimate ones.
func TestUnverifiedRulesAreNeverBlocking(t *testing.T) {
	for _, r := range AllRules {
		if r.Provenance == ProvenanceUnverified && r.Blocking() {
			t.Errorf("%s is blocking but has no authoritative source", r.ID)
		}
		if r.Provenance == "" {
			t.Errorf("%s declares no provenance", r.ID)
		}
		if r.Citation == "" {
			t.Errorf("%s has no citation", r.ID)
		}
		if r.Version == "" {
			t.Errorf("%s has no version", r.ID)
		}
	}
}

func TestRuleIDsAreUnique(t *testing.T) {
	seen := map[string]bool{}
	for _, r := range AllRules {
		if seen[r.ID] {
			t.Errorf("duplicate rule id %s", r.ID)
		}
		seen[r.ID] = true
	}
}

// A result must report what it did not check, so silence does not imply
// coverage.
func TestResultsReportWhatWasNotChecked(t *testing.T) {
	res := validate(t, validSingleBatch())
	if len(res.NotChecked) == 0 {
		t.Fatal("a result claims to have checked everything")
	}
	for _, r := range res.NotChecked {
		if r.Provenance != ProvenanceUnverified {
			t.Errorf("%s is listed as not checked but claims a verifiable provenance", r.ID)
		}
	}

	decision := Decide(res, DefaultContract)
	if len(decision.NotCheckedRuleIDs) != len(res.NotChecked) {
		t.Error("the decision does not carry forward what was not checked")
	}
	if !strings.Contains(decision.Summary(), "not checked") {
		t.Errorf("the summary does not qualify the outcome: %s", decision.Summary())
	}
}

// A decision must name the policy that produced it.
func TestDecisionsAreVersioned(t *testing.T) {
	for _, file := range []string{validSingleBatch(), ""} {
		d := Decide(validate(t, file), DefaultContract)
		if d.PolicyVersion != PolicyVersion {
			t.Errorf("decision carries policy version %q, want %q", d.PolicyVersion, PolicyVersion)
		}
	}
}

// --- Money ---

func TestAmountArithmeticRefusesToWrap(t *testing.T) {
	const max = Amount(9223372036854775807)
	if _, err := max.Add(1); err == nil {
		t.Error("Amount.Add wrapped on overflow")
	}
	if sum, err := Amount(150000).Add(Amount(250000)); err != nil || sum != 400000 {
		t.Errorf("Add = %d, %v", sum, err)
	}
}

func TestAmountRendersWithoutFloatingPoint(t *testing.T) {
	cases := map[Amount]string{
		0: "0.00", 1: "0.01", 99: "0.99", 100: "1.00",
		150000: "1500.00", 9_999_999_999: "99999999.99",
		-2550: "-25.50",
	}
	for amount, want := range cases {
		if got := amount.String(); got != want {
			t.Errorf("Amount(%d).String() = %q, want %q", int64(amount), got, want)
		}
	}
}

func TestParseAmountRejectsNonNumeric(t *testing.T) {
	for _, bad := range []string{"12E4567890", "abcdefghij", "12.34", "-1234"} {
		if _, err := ParseAmount(bad); err == nil {
			t.Errorf("ParseAmount(%q) succeeded", bad)
		}
	}
	if v, err := ParseAmount("0000150000"); err != nil || v != 150000 {
		t.Errorf("ParseAmount = %d, %v", v, err)
	}
	if v, err := ParseAmount("          "); err != nil || v != 0 {
		t.Errorf("an all-space amount field = %d, %v", v, err)
	}
}

func TestRoutingCheckDigit(t *testing.T) {
	for _, valid := range []string{"021000021", "121000358", "011000015"} {
		if err := ValidateRoutingCheckDigit(valid); err != nil {
			t.Errorf("%s was rejected: %v", valid, err)
		}
	}
	for _, invalid := range []string{"021000022", "123456789", "00000000", "abcdefghi", ""} {
		if err := ValidateRoutingCheckDigit(invalid); err == nil {
			t.Errorf("%s was accepted", invalid)
		}
	}
}

// --- Streaming ---

// Validation must not hold the artifact. This bounds the check to the parse
// itself, which is what the streaming design is for.
func TestValidationDoesNotAccumulateTheFile(t *testing.T) {
	entries := make([]entrySpec, 0, 4000)
	for i := 0; i < 4000; i++ {
		entries = append(entries, entrySpec{
			transactionCode: "22", routing: routingRDFI,
			account: "1234567890", amountMinor: int64(100 + i),
		})
	}
	file := buildFile([]batchSpec{{secCode: "PPD", number: 1, entries: entries}})

	res := validate(t, file)
	if !res.ParserOK {
		t.Fatalf("a large valid file did not parse: %+v", res.Findings)
	}
	if res.EntriesParsed != 4000 {
		t.Errorf("EntriesParsed = %d, want 4000", res.EntriesParsed)
	}
	for _, f := range res.Findings {
		if f.Blocking() {
			t.Errorf("a large valid file raised %s (expected %s, actual %s)", f.RuleID, f.Expected, f.Actual)
		}
	}
}

// Overflow is a blocking condition rather than a wrap.
//
// It cannot be reached with a realistic fixture -- a ten-character amount field
// caps a single entry near 1e10, so overflowing an int64 would need on the order
// of a billion entries. The accumulator is therefore driven to its boundary
// directly, which is the only honest way to demonstrate this path: a fixture
// that claims to overflow and does not is worse than no fixture.
func TestAccumulatorOverflowIsBlocking(t *testing.T) {
	var findings []Finding
	note := func(f Finding) { findings = append(findings, f) }
	overflowed := false
	noteOverflow := func(record int, offset int64) {
		if !overflowed {
			overflowed = true
			note(newFinding(RuleAmountOverflow, record, offset))
		}
	}

	// A batch whose credit accumulator is one entry away from the limit.
	batch := &batchState{credits: Amount(9223372036854775807 - 1000)}

	entry := "6" + "22" + routingRDFI + text("9999999999", 17) +
		pad(9_999_999_999, 10) + text("INDIVIDUAL", 15) +
		text("NAME", 22) + "  " + "0" + routingOrigin[:8] + pad(1, 7)
	if len(entry) != RecordLength {
		t.Fatalf("the test entry is %d characters, not %d", len(entry), RecordLength)
	}

	err := readEntry(entry, 3, 200, batch, note, noteOverflow)
	if err == nil {
		t.Fatal("an entry that overflows the accumulator was accepted")
	}
	if !errors.Is(err, ErrOverflow) {
		t.Fatalf("got %v, want ErrOverflow", err)
	}

	var found bool
	for _, f := range findings {
		if f.RuleID == RuleAmountOverflow.ID {
			found = true
			if !f.Blocking() {
				t.Error("the overflow finding is not blocking; a wrapped total could balance a file that does not")
			}
		}
	}
	if !found {
		t.Errorf("no overflow finding was raised; findings: %+v", findings)
	}

	// And the accumulator was not left holding a wrapped value.
	if batch.credits < 0 {
		t.Errorf("the credit accumulator wrapped to %d", batch.credits)
	}
}

// A result containing an overflow finding must quarantine.
func TestOverflowFindingQuarantines(t *testing.T) {
	res := &Result{
		ParserOK:      true,
		RecordsParsed: 10,
		EntriesParsed: 4,
		Findings:      []Finding{newFinding(RuleAmountOverflow, 3, 200)},
	}
	if d := Decide(res, DefaultContract); d.Outcome != OutcomeQuarantined {
		t.Errorf("outcome = %s, want QUARANTINED", d.Outcome)
	}
}
