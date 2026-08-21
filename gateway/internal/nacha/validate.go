package nacha

import (
	"bufio"
	"errors"
	"io"
	"sort"
	"strings"
)

// RecordLength is the fixed width of every ACH record.
const RecordLength = 94

// Record type codes, at position 1.
const (
	TypeFileHeader   = '1'
	TypeBatchHeader  = '5'
	TypeEntryDetail  = '6'
	TypeAddenda      = '7'
	TypeBatchControl = '8'
	TypeFileControl  = '9'
)

// maxRecords bounds a single artifact. At 94 bytes per record this is roughly
// 940 MB, well beyond the ingress limit; the bound exists so a malformed file
// cannot drive an unbounded loop even if it reaches this package by another
// path.
const maxRecords = 10_000_000

// Result is the outcome of validating one artifact.
//
// Every field is measured. There is no field that reports a verdict the
// validator did not compute, and there is deliberately no "isBalanced": whether
// a file must balance comes from the feed contract, not from this package. See
// Totals.
type Result struct {
	// ParserOK reports whether the file could be read as ACH at all. False
	// means the findings describe why, and no total below is meaningful.
	ParserOK bool `json:"parserOk"`

	RecordsParsed int `json:"recordsParsed"`
	BatchesParsed int `json:"batchesParsed"`
	EntriesParsed int `json:"entriesParsed"`
	AddendaParsed int `json:"addendaParsed"`

	Findings []Finding `json:"findings"`

	// Totals are the sums this validator computed from the entries, in minor
	// units. They are what was found, not what the file declared.
	TotalDebitsMinor  int64 `json:"totalDebitsMinor"`
	TotalCreditsMinor int64 `json:"totalCreditsMinor"`

	// DeclaredDebitsMinor and DeclaredCreditsMinor are what the file control
	// record claims. A caller comparing these to the totals above is checking
	// the file against itself; the findings already report any disagreement.
	DeclaredDebitsMinor  int64 `json:"declaredDebitsMinor"`
	DeclaredCreditsMinor int64 `json:"declaredCreditsMinor"`

	// FileID is the file header's originating identity fields, retained for
	// duplicate detection. Routing values here are redacted.
	OriginatorRedacted string `json:"originatorRedacted,omitempty"`
	FileCreationDate   string `json:"fileCreationDate,omitempty"`
	FileIDModifier     string `json:"fileIdModifier,omitempty"`

	// NotChecked lists the rules this validator did not evaluate because it
	// lacks an authoritative source. Reporting it is what stops silence from
	// implying coverage.
	NotChecked []Rule `json:"notChecked"`
}

// HasBlocking reports whether any finding prevents release.
func (r *Result) HasBlocking() bool {
	for _, f := range r.Findings {
		if f.Blocking() {
			return true
		}
	}
	return false
}

// batchState accumulates one batch while it is open.
type batchState struct {
	headerRecordNumber int
	headerByteOffset   int64
	batchNumber        string
	secCode            string

	entries         Count
	addenda         Count
	debits          Amount
	credits         Amount
	hash            EntryHash
	lastWasEntry    bool
	lastEntryRecord int
}

// Validate reads an ACH file and reports what it found.
//
// It streams: records are read one at a time through a bounded scanner and
// nothing accumulates but counts and totals. A 64 MiB artifact does not become
// a 64 MiB allocation.
//
// It never returns an error for a malformed file. A file that cannot be parsed
// is a Result with ParserOK false and findings explaining why -- an error return
// would make "unparseable" indistinguishable from "the disk failed", and a
// caller might retry one and not the other.
func Validate(r io.Reader) (*Result, error) {
	result := &Result{
		Findings:   []Finding{},
		NotChecked: UnverifiedRules(),
	}

	scanner := bufio.NewScanner(r)
	// A record is 94 bytes. The buffer is generous enough to read a
	// pathologically long line and report it, rather than erroring out of the
	// scanner with a condition the caller cannot distinguish from I/O failure.
	scanner.Buffer(make([]byte, 0, 4096), 1<<20)

	var (
		byteOffset     int64
		recordNumber   int
		sawFileHeader  bool
		sawFileControl bool

		current *batchState

		fileEntries Count
		fileAddenda Count
		fileDebits  Amount
		fileCredits Amount
		fileHash    EntryHash
		batchCount  Count

		overflowed bool
	)

	// note records a finding once; overflow is reported once rather than per
	// record, because a file that overflows does so on every subsequent entry.
	note := func(f Finding) { result.Findings = append(result.Findings, f) }
	noteOverflow := func(recordNumber int, offset int64) {
		if !overflowed {
			overflowed = true
			note(newFinding(RuleAmountOverflow, recordNumber, offset))
		}
	}

	for scanner.Scan() {
		line := scanner.Text()
		lineStart := byteOffset
		byteOffset += int64(len(line)) + 1 // +1 for the line terminator
		recordNumber++

		if recordNumber > maxRecords {
			note(newFinding(RuleRecordLength, recordNumber, lineStart).
				withStrings("at most 10,000,000 records", "more"))
			break
		}

		// Blank lines between records are a common artefact of transfer and are
		// not records. They are skipped rather than counted, and a file of
		// nothing but blank lines is still empty.
		if strings.TrimSpace(line) == "" {
			recordNumber--
			continue
		}

		if len(line) != RecordLength {
			note(newFinding(RuleRecordLength, recordNumber, lineStart).
				withCounts(RecordLength, int64(len(line))))
			// The record cannot be field-decoded, so nothing further is
			// attempted on it. Decoding by position from a short line would
			// read the wrong fields and report confident nonsense.
			continue
		}
		if idx := firstNonPrintable(line); idx >= 0 {
			note(newFinding(RuleCharacterSet, recordNumber, lineStart).
				withField(idx+1, idx+1, line[idx:idx+1]))
			continue
		}

		result.RecordsParsed++

		switch line[0] {
		case TypeFileHeader:
			sawFileHeader = true
			if recordNumber != 1 {
				note(newFinding(RuleFileHeaderMissing, recordNumber, lineStart).
					withStrings("record 1", "record "+itoa(recordNumber)))
			}
			// Positions 14-23 are the immediate origin; 24-29 the creation
			// date; 34 the file ID modifier. The origin is routing-shaped, so
			// it is redacted before it is retained.
			result.OriginatorRedacted = RedactRouting(strings.TrimSpace(field(line, 14, 23)))
			result.FileCreationDate = strings.TrimSpace(field(line, 24, 29))
			result.FileIDModifier = strings.TrimSpace(field(line, 34, 34))

		case TypeBatchHeader:
			if current != nil {
				// A batch header inside an open batch means the previous batch
				// was never closed.
				note(newFinding(RuleTruncated, current.headerRecordNumber, current.headerByteOffset))
			}
			current = &batchState{
				headerRecordNumber: recordNumber,
				headerByteOffset:   lineStart,
				batchNumber:        strings.TrimSpace(field(line, 88, 94)),
				secCode:            strings.TrimSpace(field(line, 51, 53)),
			}

		case TypeEntryDetail:
			if current == nil {
				note(newFinding(RuleOrphanEntry, recordNumber, lineStart))
				continue
			}
			result.EntriesParsed++
			if e := readEntry(line, recordNumber, lineStart, current, note, noteOverflow); e != nil {
				// readEntry has already recorded the finding.
				continue
			}

		case TypeAddenda:
			if current == nil {
				note(newFinding(RuleOrphanAddenda, recordNumber, lineStart))
				continue
			}
			if !current.lastWasEntry {
				note(newFinding(RuleOrphanAddenda, recordNumber, lineStart))
				continue
			}
			result.AddendaParsed++
			var err error
			if current.addenda, err = current.addenda.Add(1); err != nil {
				noteOverflow(recordNumber, lineStart)
			}

		case TypeBatchControl:
			if current == nil {
				note(newFinding(RuleOrphanEntry, recordNumber, lineStart).
					withStrings("a batch header before this control", "none"))
				continue
			}
			closeBatch(line, recordNumber, lineStart, current, note)
			result.BatchesParsed++

			var err error
			if batchCount, err = batchCount.Add(1); err != nil {
				noteOverflow(recordNumber, lineStart)
			}
			if fileEntries, err = fileEntries.Add(current.entries); err != nil {
				noteOverflow(recordNumber, lineStart)
			}
			if fileAddenda, err = fileAddenda.Add(current.addenda); err != nil {
				noteOverflow(recordNumber, lineStart)
			}
			if fileDebits, err = fileDebits.Add(current.debits); err != nil {
				noteOverflow(recordNumber, lineStart)
			}
			if fileCredits, err = fileCredits.Add(current.credits); err != nil {
				noteOverflow(recordNumber, lineStart)
			}
			fileHash += current.hash
			current = nil

		case TypeFileControl:
			sawFileControl = true
			checkFileControl(line, recordNumber, lineStart, fileControlExpectation{
				batches: batchCount,
				entries: fileEntries + fileAddenda,
				debits:  fileDebits,
				credits: fileCredits,
				hash:    fileHash,
			}, note)

		default:
			note(newFinding(RuleRecordType, recordNumber, lineStart).
				withField(1, 1, line[0:1]))
		}

		// Track adjacency for the addenda relationship check.
		if current != nil {
			current.lastWasEntry = line[0] == TypeEntryDetail
			if current.lastWasEntry {
				current.lastEntryRecord = recordNumber
			}
		}
	}

	if err := scanner.Err(); err != nil {
		// A read failure is a genuine error: the artifact may be fine and the
		// storage may not be. The caller must be able to tell that apart from
		// an invalid file, so it is returned rather than turned into a finding.
		if errors.Is(err, bufio.ErrTooLong) {
			note(newFinding(RuleRecordLength, recordNumber+1, byteOffset).
				withStrings("94 characters", "a line exceeding 1 MiB"))
		} else {
			return nil, err
		}
	}

	// Terminal structural checks.
	if result.RecordsParsed == 0 {
		note(newFinding(RuleFileEmpty, 0, 0))
		result.ParserOK = false
		finalise(result, fileDebits, fileCredits)
		return result, nil
	}
	if !sawFileHeader {
		note(newFinding(RuleFileHeaderMissing, 1, 0))
	}
	if current != nil {
		note(newFinding(RuleTruncated, current.headerRecordNumber, current.headerByteOffset))
	}
	if !sawFileControl {
		note(newFinding(RuleFileControlMissing, recordNumber, byteOffset))
	}

	// ParserOK means the file was structurally readable as ACH. Arithmetic
	// disagreements do not make it unreadable -- they make it wrong, which the
	// findings say.
	result.ParserOK = sawFileHeader && sawFileControl && result.RecordsParsed > 0
	finalise(result, fileDebits, fileCredits)
	return result, nil
}

func finalise(result *Result, debits, credits Amount) {
	result.TotalDebitsMinor = debits.Minor()
	result.TotalCreditsMinor = credits.Minor()

	// Deterministic ordering: by record, then by rule id. Two runs over the
	// same bytes must produce byte-identical findings, or the evidence is not
	// evidence.
	sort.SliceStable(result.Findings, func(i, j int) bool {
		if result.Findings[i].RecordNumber != result.Findings[j].RecordNumber {
			return result.Findings[i].RecordNumber < result.Findings[j].RecordNumber
		}
		return result.Findings[i].RuleID < result.Findings[j].RuleID
	})
}

// readEntry accumulates one entry detail record into its batch.
func readEntry(line string, recordNumber int, offset int64, batch *batchState,
	note func(Finding), noteOverflow func(int, int64)) error {

	// Positions 4-12 are the RDFI routing number including its check digit.
	routing := field(line, 4, 12)
	if err := ValidateRoutingCheckDigit(routing); err != nil {
		note(newFinding(RuleRoutingCheckDigit, recordNumber, offset).
			withField(4, 12, RedactRouting(routing)))
	}

	// Positions 30-39 are the amount, in minor units.
	amount, err := ParseAmount(field(line, 30, 39))
	if err != nil {
		if errors.Is(err, ErrNotNumeric) {
			note(newFinding(RuleNumericField, recordNumber, offset).
				withField(30, 39, field(line, 30, 39)))
		} else {
			noteOverflow(recordNumber, offset)
		}
		return err
	}

	// Position 2-3 is the transaction code; its second digit distinguishes
	// credits (2x) from debits (2x/3x per the format's code ranges).
	if IsDebitTransaction(field(line, 2, 3)) {
		if batch.debits, err = batch.debits.Add(amount); err != nil {
			noteOverflow(recordNumber, offset)
			return err
		}
	} else {
		if batch.credits, err = batch.credits.Add(amount); err != nil {
			noteOverflow(recordNumber, offset)
			return err
		}
	}

	if batch.hash, err = batch.hash.AddRouting(routing); err != nil {
		if errors.Is(err, ErrOverflow) {
			noteOverflow(recordNumber, offset)
			return err
		}
		note(newFinding(RuleNumericField, recordNumber, offset).
			withField(4, 12, RedactRouting(routing)))
	}

	if batch.entries, err = batch.entries.Add(1); err != nil {
		noteOverflow(recordNumber, offset)
		return err
	}
	return nil
}

// closeBatch compares a batch control record against what was counted.
func closeBatch(line string, recordNumber int, offset int64, batch *batchState, note func(Finding)) {
	// Positions 5-10: entry/addenda count.
	declaredCount, err := ParseCount(field(line, 5, 10))
	if err != nil {
		note(newFinding(RuleNumericField, recordNumber, offset).withField(5, 10, field(line, 5, 10)))
	} else if want := batch.entries + batch.addenda; Count(declaredCount) != want {
		note(newFinding(RuleBatchEntryCount, recordNumber, offset).
			withCounts(int64(declaredCount), int64(want)))
	}

	// Positions 11-20: entry hash.
	declaredHash, err := ParseCount(field(line, 11, 20))
	if err != nil {
		note(newFinding(RuleNumericField, recordNumber, offset).withField(11, 20, field(line, 11, 20)))
	} else if int64(declaredHash) != batch.hash.Truncated() {
		note(newFinding(RuleBatchEntryHash, recordNumber, offset).
			withCounts(int64(declaredHash), batch.hash.Truncated()))
	}

	// Positions 21-32: total debits. 33-44: total credits.
	declaredDebits, err := ParseAmount(field(line, 21, 32))
	if err != nil {
		note(newFinding(RuleNumericField, recordNumber, offset).withField(21, 32, field(line, 21, 32)))
	} else if declaredDebits != batch.debits {
		note(newFinding(RuleBatchDebitTotal, recordNumber, offset).
			withAmounts(declaredDebits, batch.debits))
	}

	declaredCredits, err := ParseAmount(field(line, 33, 44))
	if err != nil {
		note(newFinding(RuleNumericField, recordNumber, offset).withField(33, 44, field(line, 33, 44)))
	} else if declaredCredits != batch.credits {
		note(newFinding(RuleBatchCreditTotal, recordNumber, offset).
			withAmounts(declaredCredits, batch.credits))
	}

	// Positions 88-94: batch number, which must match the header's.
	if controlNumber := strings.TrimSpace(field(line, 88, 94)); controlNumber != batch.batchNumber {
		note(newFinding(RuleBatchNumberSequence, recordNumber, offset).
			withStrings(batch.batchNumber, controlNumber))
	}
}

type fileControlExpectation struct {
	batches Count
	entries Count
	debits  Amount
	credits Amount
	hash    EntryHash
}

// checkFileControl compares the file control record against the whole file.
func checkFileControl(line string, recordNumber int, offset int64, want fileControlExpectation, note func(Finding)) {
	// Positions 2-7: batch count.
	if declared, err := ParseCount(field(line, 2, 7)); err != nil {
		note(newFinding(RuleNumericField, recordNumber, offset).withField(2, 7, field(line, 2, 7)))
	} else if declared != want.batches {
		note(newFinding(RuleFileBatchCount, recordNumber, offset).
			withCounts(int64(declared), int64(want.batches)))
	}

	// Positions 14-21: entry/addenda count.
	if declared, err := ParseCount(field(line, 14, 21)); err != nil {
		note(newFinding(RuleNumericField, recordNumber, offset).withField(14, 21, field(line, 14, 21)))
	} else if declared != want.entries {
		note(newFinding(RuleFileEntryCount, recordNumber, offset).
			withCounts(int64(declared), int64(want.entries)))
	}

	// Positions 22-31: entry hash.
	if declared, err := ParseCount(field(line, 22, 31)); err != nil {
		note(newFinding(RuleNumericField, recordNumber, offset).withField(22, 31, field(line, 22, 31)))
	} else if int64(declared) != want.hash.Truncated() {
		note(newFinding(RuleFileEntryHash, recordNumber, offset).
			withCounts(int64(declared), want.hash.Truncated()))
	}

	// Positions 32-43: total debits. 44-55: total credits.
	if declared, err := ParseAmount(field(line, 32, 43)); err != nil {
		note(newFinding(RuleNumericField, recordNumber, offset).withField(32, 43, field(line, 32, 43)))
	} else if declared != want.debits {
		note(newFinding(RuleFileDebitTotal, recordNumber, offset).
			withAmounts(declared, want.debits))
	}

	if declared, err := ParseAmount(field(line, 44, 55)); err != nil {
		note(newFinding(RuleNumericField, recordNumber, offset).withField(44, 55, field(line, 44, 55)))
	} else if declared != want.credits {
		note(newFinding(RuleFileCreditTotal, recordNumber, offset).
			withAmounts(declared, want.credits))
	}
}

// field extracts a 1-based inclusive character range. The caller has already
// confirmed the record is exactly RecordLength.
func field(line string, start, end int) string {
	if start < 1 || end > len(line) || start > end {
		return ""
	}
	return line[start-1 : end]
}

// IsDebitTransaction reads the transaction code's direction.
//
// In this format, codes in the 20s are credits and codes in the 30s are debits
// for savings, while 22/23/27/28 and 32/33/37/38 split checking similarly. The
// distinguishing digit is the second: 7 and 8 are debits within a series, as
// are the 5x prefixes. This is the format's own encoding, not a rule-set
// requirement.
func IsDebitTransaction(code string) bool {
	trimmed := strings.TrimSpace(code)
	if len(trimmed) != 2 {
		return false
	}
	switch trimmed {
	case "27", "28", "29", "37", "38", "39", "47", "48", "49", "55", "56":
		return true
	default:
		return false
	}
}

// firstNonPrintable returns the index of the first character outside printable
// ASCII, or -1.
func firstNonPrintable(line string) int {
	for i := 0; i < len(line); i++ {
		if line[i] < 0x20 || line[i] > 0x7e {
			return i
		}
	}
	return -1
}

func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	neg := n < 0
	if neg {
		n = -n
	}
	var digits []byte
	for n > 0 {
		digits = append([]byte{byte('0' + n%10)}, digits...)
		n /= 10
	}
	if neg {
		return "-" + string(digits)
	}
	return string(digits)
}
