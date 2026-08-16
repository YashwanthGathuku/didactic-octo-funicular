package nacha

import (
	"fmt"
	"strings"
	"unicode"
)

// Finding is one rule result against one artifact.
//
// It carries enough to investigate and nothing that would leak the payment
// instruction itself. The previous implementation stored the complete
// 94-character record in a `raw_data` column and returned it from
// GET /api/v1/incidents, which put account numbers, routing numbers and amounts
// into every response, log line and support export that touched an incident.
type Finding struct {
	RuleID      string     `json:"ruleId"`
	RuleVersion string     `json:"ruleVersion"`
	Severity    Severity   `json:"severity"`
	Provenance  Provenance `json:"provenance"`
	Description string     `json:"description"`

	// RecordNumber is the 1-based ordinal of the record in the file.
	RecordNumber int `json:"recordNumber"`

	// ByteOffset is where that record begins in the artifact. Together with
	// RecordNumber it locates the problem exactly, which is what an operator
	// needs, without reproducing the content.
	ByteOffset int64 `json:"byteOffset"`

	// FieldPositions names the 1-based character range within the record, when
	// the finding concerns a specific field.
	FieldStart int `json:"fieldStart,omitempty"`
	FieldEnd   int `json:"fieldEnd,omitempty"`

	// Evidence is a redacted excerpt. It is produced only by RedactEvidence and
	// never holds a complete record.
	Evidence string `json:"evidence,omitempty"`

	// Expected and Actual carry the two sides of an arithmetic disagreement.
	// Totals and counts are not payment instructions -- a mismatched batch
	// total is the finding -- so they appear in full.
	Expected string `json:"expected,omitempty"`
	Actual   string `json:"actual,omitempty"`
}

// Blocking reports whether this finding prevents release.
func (f Finding) Blocking() bool { return f.Severity == SeverityBlocking }

// maxEvidenceRunes bounds an excerpt. A field is at most 40 characters in this
// format; anything longer would be a record fragment.
const maxEvidenceRunes = 40

// RedactEvidence produces a safe excerpt of a field.
//
// The rules, in order of how much they matter:
//
//  1. A complete record is never evidence. The excerpt is bounded well below
//     the 94-character record length, so no combination of findings
//     reconstructs a line.
//  2. Digits are masked. Every numeric field in this format is an account
//     number, a routing number, an amount or a trace number, and the first
//     three are directly sensitive while the fourth correlates to them.
//  3. Structure is preserved. An operator needs to see that a field is the
//     wrong shape, so lengths and non-digit characters survive.
//
// A caller that wants to say "this field was wrong" passes the field; a caller
// that wants to say "these two totals disagree" uses Expected and Actual, which
// are not payment instructions.
func RedactEvidence(field string) string {
	if field == "" {
		return ""
	}
	runes := []rune(field)
	truncated := false
	if len(runes) > maxEvidenceRunes {
		runes = runes[:maxEvidenceRunes]
		truncated = true
	}

	var b strings.Builder
	for _, r := range runes {
		switch {
		case unicode.IsDigit(r):
			b.WriteRune('#')
		case r < 0x20 || r == 0x7f:
			// A control character is itself the finding, and must not reach a
			// log where it could forge a record or drive a terminal.
			b.WriteString("<ctrl>")
		case r > unicode.MaxASCII:
			b.WriteString("<non-ascii>")
		default:
			b.WriteRune(r)
		}
	}
	if truncated {
		b.WriteString("…")
	}
	return b.String()
}

// RedactRouting masks a routing number to its leading four digits.
//
// The leading four identify the Federal Reserve routing symbol, which is what
// an operator needs to recognise a counterparty. The remaining digits identify
// the institution and the check digit, and together with an account number are
// enough to originate a debit.
func RedactRouting(routing string) string {
	trimmed := strings.TrimSpace(routing)
	if len(trimmed) < 4 {
		return strings.Repeat("#", len(trimmed))
	}
	return trimmed[:4] + strings.Repeat("#", len(trimmed)-4)
}

// RedactAccount masks an account number to its last four characters.
//
// Last-four is the convention every operator already reads, and it is the
// smallest excerpt that lets someone confirm they are looking at the right
// record when they have the account in front of them by other means.
func RedactAccount(account string) string {
	trimmed := strings.TrimSpace(account)
	if len(trimmed) <= 4 {
		return strings.Repeat("#", len(trimmed))
	}
	return strings.Repeat("#", len(trimmed)-4) + trimmed[len(trimmed)-4:]
}

// newFinding builds a finding from a rule and a location.
func newFinding(rule Rule, recordNumber int, byteOffset int64) Finding {
	return Finding{
		RuleID:       rule.ID,
		RuleVersion:  rule.Version,
		Severity:     rule.Severity,
		Provenance:   rule.Provenance,
		Description:  rule.Description,
		RecordNumber: recordNumber,
		ByteOffset:   byteOffset,
	}
}

// withField adds a field location and its redacted excerpt.
func (f Finding) withField(start, end int, raw string) Finding {
	f.FieldStart = start
	f.FieldEnd = end
	f.Evidence = RedactEvidence(raw)
	return f
}

// withCounts records an arithmetic disagreement.
func (f Finding) withCounts(expected, actual int64) Finding {
	f.Expected = fmt.Sprintf("%d", expected)
	f.Actual = fmt.Sprintf("%d", actual)
	return f
}

// withAmounts records a disagreement between two minor-unit totals, rendered so
// an operator reads currency rather than cents.
func (f Finding) withAmounts(expected, actual Amount) Finding {
	f.Expected = expected.String()
	f.Actual = actual.String()
	return f
}

// withStrings records a disagreement between two non-sensitive values.
func (f Finding) withStrings(expected, actual string) Finding {
	f.Expected = expected
	f.Actual = actual
	return f
}
