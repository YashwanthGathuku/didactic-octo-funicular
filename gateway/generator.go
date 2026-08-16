package main

import (
	"fmt"
	"strings"
	"time"
)

// Synthetic ACH fixtures, for demonstrating validator behaviour.
//
// The previous implementation built these as hand-written string literals and
// padded any line shorter than 94 characters -- but never truncated one that was
// longer. Five of its six presets emitted records of 95, 96, 102 or 103
// characters while its doc comment claimed "94-character fixed-width", and the
// preset named CORRUPTED_ENTRY_HASH was quarantined for malformed record widths
// rather than for the hash mismatch it advertised. Nothing caught it because
// the validator it fed did not check record length.
//
// Records are now assembled field by field at exact widths, so a preset
// demonstrates the defect it is named for and nothing else. Where a preset's
// defect *is* a wrong record width, that is the only thing wrong with it.

type GeneratorPreset string

const (
	PresetBalancedPayroll       GeneratorPreset = "BALANCED_PPD_PAYROLL"
	PresetUnbalancedCCD         GeneratorPreset = "UNBALANCED_CCD"
	PresetCorruptedEntryHash    GeneratorPreset = "CORRUPTED_ENTRY_HASH"
	PresetInvalidAbaRouting     GeneratorPreset = "INVALID_ABA_ROUTING"
	PresetRecordAlignmentError  GeneratorPreset = "RECORD_ALIGNMENT_ERROR"
	PresetMissingHeaderSequence GeneratorPreset = "MISSING_HEADER_SEQUENCE"
)

type GeneratedNachaResult struct {
	Preset   GeneratorPreset `json:"preset"`
	Filename string          `json:"filename"`
	Content  string          `json:"content"`

	// Description states what the fixture demonstrates. It describes the
	// single defect the preset introduces, so a reader can check the validator's
	// findings against it.
	Description string `json:"description"`

	// ExpectedRuleIDs names the rules this fixture should trigger. It is what
	// makes the generator checkable: a test can assert the validator raises
	// exactly these, which is how the previous mismatch would have been caught.
	ExpectedRuleIDs []string `json:"expectedRuleIds"`
}

// Fixture routing numbers, all with valid ABA check digits.
const (
	genOriginRouting = "021000021"
	genRDFI1         = "121000358"
	genRDFI2         = "011000015"
	genBadRouting    = "999999999" // check digit deliberately invalid
)

// numField right-justifies and zero-fills.
func numField(v int64, width int) string {
	s := fmt.Sprintf("%d", v)
	if len(s) > width {
		return s[len(s)-width:]
	}
	return strings.Repeat("0", width-len(s)) + s
}

// textField left-justifies and space-fills, truncating if too long.
//
// Truncating is what the previous implementation omitted, and it is why five of
// six presets were malformed.
func textField(s string, width int) string {
	if len(s) > width {
		return s[:width]
	}
	return s + strings.Repeat(" ", width-len(s))
}

type genEntry struct {
	transactionCode string // "22" credit, "27" debit
	routing         string
	account         string
	amountMinor     int64
}

// buildNachaFile assembles a syntactically exact file with correct controls.
//
// Every control value is computed from the records rather than written by hand,
// so a valid fixture is valid by construction and each invalid preset is a
// stated single change to it.
func buildNachaFile(secCode string, entries []genEntry, creationDate string) []string {
	var records []string

	// File header, 94 characters.
	records = append(records,
		"1"+"01"+
			" "+genRDFI1+ // 4-13 immediate destination
			" "+genOriginRouting+ // 14-23 immediate origin
			creationDate+"1200"+"A"+"094"+"10"+"1"+
			textField("DESTINATION BANK", 23)+
			textField("SENTINEL FLOW GATEWAY", 23)+
			textField("", 8))

	var (
		count   int64
		debits  int64
		credits int64
		hash    int64
	)

	// Batch header, 94 characters.
	records = append(records,
		"5"+"200"+textField("MERIDIAN CUSTODY", 16)+textField("", 20)+
			textField("1210000180", 10)+textField(secCode, 3)+
			textField("DIRECT DEP", 10)+creationDate+creationDate+"   "+"1"+
			genOriginRouting[:8]+numField(1, 7))

	for i, e := range entries {
		records = append(records,
			"6"+e.transactionCode+e.routing+textField(e.account, 17)+
				numField(e.amountMinor, 10)+textField(fmt.Sprintf("EMP-%05d", i+1), 15)+
				textField("EMPLOYEE NAME", 22)+"  "+"0"+
				genOriginRouting[:8]+numField(int64(i+1), 7))

		count++
		if e.transactionCode == "27" {
			debits += e.amountMinor
		} else {
			credits += e.amountMinor
		}
		var prefix int64
		fmt.Sscanf(e.routing[:8], "%d", &prefix)
		hash += prefix
	}

	// Batch control, 94 characters.
	records = append(records,
		"8"+"200"+numField(count, 6)+numField(hash%10_000_000_000, 10)+
			numField(debits, 12)+numField(credits, 12)+
			textField("1210000180", 10)+textField("", 19)+textField("", 6)+
			genOriginRouting[:8]+numField(1, 7))

	// File control, 94 characters.
	blocks := int64((len(records) + 1 + 9) / 10)
	records = append(records,
		"9"+numField(1, 6)+numField(blocks, 6)+numField(count, 8)+
			numField(hash%10_000_000_000, 10)+numField(debits, 12)+numField(credits, 12)+
			textField("", 39))

	return records
}

// replaceField substitutes a 1-based inclusive character range, preserving the
// record's width.
func replaceField(record string, start, end int, value string) string {
	width := end - start + 1
	return record[:start-1] + textField(value, width) + record[end:]
}

// GenerateNachaScenario creates a synthetic fixture demonstrating one defect.
func GenerateNachaScenario(preset GeneratorPreset) GeneratedNachaResult {
	timestamp := time.Now().Format("0601021504")
	creationDate := time.Now().Format("060102")
	filename := fmt.Sprintf("ACH_%s_%s.ach", string(preset), timestamp)

	// A balanced payroll: two credits offset by one debit.
	balanced := []genEntry{
		{transactionCode: "22", routing: genRDFI1, account: "12345678901234567", amountMinor: 250000},
		{transactionCode: "22", routing: genRDFI2, account: "98765432101234567", amountMinor: 250000},
		{transactionCode: "27", routing: genRDFI1, account: "55544433321234567", amountMinor: 500000},
	}

	result := GeneratedNachaResult{Preset: preset, Filename: filename}

	switch preset {
	case PresetUnbalancedCCD:
		records := buildNachaFile("CCD", []genEntry{
			{transactionCode: "22", routing: genRDFI1, account: "11122233344455566", amountMinor: 125000},
			{transactionCode: "22", routing: genRDFI2, account: "44455566677788899", amountMinor: 75000},
		}, creationDate)
		result.Content = strings.Join(records, "\n") + "\n"
		result.Description = "Structurally valid CCD vendor disbursement with credits only and no offsetting debit. " +
			"Whether this is acceptable is a term of the feed contract, not a property of the file: " +
			"under a contract that does not require balance it is entirely correct."
		result.ExpectedRuleIDs = []string{}

	case PresetCorruptedEntryHash:
		records := buildNachaFile("PPD", balanced, creationDate)
		// Batch control positions 11-20 hold the entry hash.
		records[len(records)-2] = replaceField(records[len(records)-2], 11, 20, "0999999999")
		result.Content = strings.Join(records, "\n") + "\n"
		result.Description = "Batch control declares an entry hash of 0999999999 that does not match the sum computed from its entries. " +
			"Every other field is correct, so this is the only finding."
		result.ExpectedRuleIDs = []string{"NACHA.MATH.BATCH_ENTRY_HASH"}

	case PresetInvalidAbaRouting:
		entries := append([]genEntry{}, balanced...)
		entries[0].routing = genBadRouting
		records := buildNachaFile("PPD", entries, creationDate)
		result.Content = strings.Join(records, "\n") + "\n"
		result.Description = "One entry carries the routing number 999999999, which fails the ABA check-digit calculation. " +
			"The control records are computed from the file as written, so the check digit is the only finding."
		result.ExpectedRuleIDs = []string{"NACHA.ROUTING.CHECK_DIGIT"}

	case PresetRecordAlignmentError:
		records := buildNachaFile("PPD", balanced, creationDate)
		// Truncate the batch header only. Its defect is its width.
		records[1] = records[1][:48]
		result.Content = strings.Join(records, "\n") + "\n"
		result.Description = "The batch header is 48 characters rather than 94. A record of the wrong width cannot be field-decoded, " +
			"so the records that depend on it are reported too."
		result.ExpectedRuleIDs = []string{"NACHA.STRUCT.RECORD_LENGTH"}

	case PresetMissingHeaderSequence:
		records := buildNachaFile("PPD", balanced, creationDate)
		// Drop the file header.
		result.Content = strings.Join(records[1:], "\n") + "\n"
		result.Description = "The file header record is absent, so the file does not begin as an ACH file does."
		result.ExpectedRuleIDs = []string{"NACHA.STRUCT.NO_FILE_HEADER"}

	default: // PresetBalancedPayroll
		result.Preset = PresetBalancedPayroll
		records := buildNachaFile("PPD", balanced, creationDate)
		result.Content = strings.Join(records, "\n") + "\n"
		result.Description = "Balanced PPD payroll: debits equal credits, every routing number carries a valid check digit, " +
			"and every control record agrees with the entries it summarises."
		result.ExpectedRuleIDs = []string{}
	}

	return result
}
