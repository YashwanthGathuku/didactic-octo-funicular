// Package nacha validates ACH files and decides, under a versioned policy,
// whether an artifact may be released.
//
// Two things about this package are more important than its rule coverage.
//
// It fails closed. A parser error, zero records, a truncated file, an integer
// overflow, an unsupported code or a failed verification all quarantine the
// artifact. The defect this repository started from was the opposite: ingestion
// initialised every result to RELEASED and downgraded only on a positive
// finding, so a zero-byte file -- which produces no findings, because there is
// nothing to find -- was released as balanced.
//
// And it is honest about what it knows. The ACH file *format* is public: record
// length, record type codes, field positions, and the arithmetic that ties
// entries to their batch and file controls. The Nacha *Operating Rules* --
// which SEC codes are valid, how many addenda each permits, amount and timing
// constraints, return handling -- are licensed and are not available to this
// repository. Rules in the second category are declared with provenance
// Unverified and cannot be blocking. Inventing them would produce a validator
// that rejects legitimate files and accepts illegitimate ones with equal
// confidence.
package nacha

// Provenance records where a rule's authority comes from.
//
// This is the mechanism that keeps the validator honest. A rule is either
// derivable from the file in front of us, or it depends on a rule source this
// repository does not have.
type Provenance string

const (
	// ProvenanceFileFormat means the rule is checkable from the file itself:
	// its record layout, its declared totals, and the arithmetic relating them.
	// A file that fails one of these is internally inconsistent, which is true
	// regardless of which edition of the Operating Rules applies.
	ProvenanceFileFormat Provenance = "FILE_FORMAT"

	// ProvenanceUnverified means the rule requires the licensed Nacha Operating
	// Rules, which this repository does not have.
	//
	// Such a rule may be declared and may raise an INFO finding so an operator
	// can see it was considered. It must never be blocking, and
	// TestUnverifiedRulesAreNeverBlocking enforces that.
	ProvenanceUnverified Provenance = "UNVERIFIED_REQUIRES_LICENSED_RULES"
)

// Severity orders findings. Only Blocking prevents release.
type Severity string

const (
	// SeverityInfo records something observed. It never affects release.
	SeverityInfo Severity = "INFO"

	// SeverityWarning records something an operator should see. It does not
	// prevent release on its own; the policy decides.
	SeverityWarning Severity = "WARNING"

	// SeverityBlocking prevents release. Under every policy version.
	SeverityBlocking Severity = "BLOCKING"
)

// Rule is a validation rule with a stable identifier and a version.
//
// The version is on the rule, not the rule set, so a finding recorded last year
// still says which formulation of the rule produced it. Changing what a rule
// checks requires bumping its version; the existing findings keep their meaning.
type Rule struct {
	ID       string
	Version  string
	Severity Severity

	// Provenance is what the rule's authority rests on.
	Provenance Provenance

	// Description is shown to an operator. It states what was checked, not what
	// the caller should do about it.
	Description string

	// Citation names the source. For file-format rules it points at the
	// structural fact being checked. For unverified rules it names what would
	// be needed to verify it, so the gap is legible rather than implied.
	Citation string
}

// Blocking reports whether this rule prevents release.
func (r Rule) Blocking() bool { return r.Severity == SeverityBlocking }

// The rule set.
//
// Every blocking rule below is ProvenanceFileFormat: it checks the file against
// itself. That is a deliberately narrow foundation, and it is a real one -- a
// file whose batch control disagrees with the entries it contains is wrong
// under any edition of the rules.
var (
	// --- Structure ---

	RuleFileEmpty = Rule{
		ID: "NACHA.STRUCT.EMPTY", Version: "1.0.0", Severity: SeverityBlocking,
		Provenance:  ProvenanceFileFormat,
		Description: "The artifact contains no records.",
		Citation:    "An ACH file consists of at least a file header and a file control record; a file with neither is not an ACH file.",
	}

	RuleRecordLength = Rule{
		ID: "NACHA.STRUCT.RECORD_LENGTH", Version: "1.0.0", Severity: SeverityBlocking,
		Provenance:  ProvenanceFileFormat,
		Description: "Every ACH record is exactly 94 characters.",
		Citation:    "ACH file format: fixed-width 94-character records. A record of any other length cannot be field-decoded.",
	}

	RuleRecordType = Rule{
		ID: "NACHA.STRUCT.RECORD_TYPE", Version: "1.0.0", Severity: SeverityBlocking,
		Provenance:  ProvenanceFileFormat,
		Description: "The record type code is not one of 1, 5, 6, 7, 8, 9.",
		Citation:    "ACH file format: record type code occupies position 1 and takes one of six defined values.",
	}

	RuleCharacterSet = Rule{
		ID: "NACHA.STRUCT.CHARACTER_SET", Version: "1.0.0", Severity: SeverityBlocking,
		Provenance:  ProvenanceFileFormat,
		Description: "A record contains a character outside the printable ASCII range.",
		Citation:    "ACH file format: records are fixed-width printable characters. A control byte cannot be positionally decoded.",
	}

	RuleFileHeaderMissing = Rule{
		ID: "NACHA.STRUCT.NO_FILE_HEADER", Version: "1.0.0", Severity: SeverityBlocking,
		Provenance:  ProvenanceFileFormat,
		Description: "The file does not begin with a file header (type 1) record.",
		Citation:    "ACH file format: the file header is the first record of the file.",
	}

	RuleFileControlMissing = Rule{
		ID: "NACHA.STRUCT.NO_FILE_CONTROL", Version: "1.0.0", Severity: SeverityBlocking,
		Provenance:  ProvenanceFileFormat,
		Description: "The file has no file control (type 9) record; it is truncated.",
		Citation:    "ACH file format: the file control record terminates the file. Its absence means the file is incomplete.",
	}

	RuleTruncated = Rule{
		ID: "NACHA.STRUCT.TRUNCATED", Version: "1.0.0", Severity: SeverityBlocking,
		Provenance:  ProvenanceFileFormat,
		Description: "The file ends inside a batch: a batch header has no matching batch control.",
		Citation:    "ACH file format: every batch header (type 5) is closed by a batch control (type 8).",
	}

	// --- Batch and entry relationships ---

	RuleOrphanEntry = Rule{
		ID: "NACHA.STRUCT.ORPHAN_ENTRY", Version: "1.0.0", Severity: SeverityBlocking,
		Provenance:  ProvenanceFileFormat,
		Description: "An entry detail record appears outside any batch.",
		Citation:    "ACH file format: entry detail records occur between a batch header and its batch control.",
	}

	RuleOrphanAddenda = Rule{
		ID: "NACHA.STRUCT.ORPHAN_ADDENDA", Version: "1.0.0", Severity: SeverityBlocking,
		Provenance:  ProvenanceFileFormat,
		Description: "An addenda record does not follow an entry detail record.",
		Citation:    "ACH file format: an addenda record extends the entry detail record it follows.",
	}

	RuleBatchNumberSequence = Rule{
		ID: "NACHA.STRUCT.BATCH_NUMBER", Version: "1.0.0", Severity: SeverityWarning,
		Provenance:  ProvenanceFileFormat,
		Description: "The batch control's batch number does not match its batch header.",
		Citation:    "ACH file format: the batch number field is identical in a batch's header and control records.",
	}

	// --- Arithmetic. These are the substance of the validator. ---

	RuleBatchEntryCount = Rule{
		ID: "NACHA.MATH.BATCH_ENTRY_COUNT", Version: "1.0.0", Severity: SeverityBlocking,
		Provenance:  ProvenanceFileFormat,
		Description: "The batch control's entry/addenda count does not match the records counted in the batch.",
		Citation:    "ACH file format: batch control positions 5-10 declare the count of entry detail and addenda records in the batch.",
	}

	RuleBatchEntryHash = Rule{
		ID: "NACHA.MATH.BATCH_ENTRY_HASH", Version: "1.0.0", Severity: SeverityBlocking,
		Provenance:  ProvenanceFileFormat,
		Description: "The batch control's entry hash does not match the sum computed from the entries.",
		Citation:    "ACH file format: the entry hash is the sum of the first eight digits of each entry's RDFI routing number, truncated to the low-order ten digits.",
	}

	RuleBatchDebitTotal = Rule{
		ID: "NACHA.MATH.BATCH_DEBIT_TOTAL", Version: "1.0.0", Severity: SeverityBlocking,
		Provenance:  ProvenanceFileFormat,
		Description: "The batch control's total debit amount does not match the sum of its debit entries.",
		Citation:    "ACH file format: batch control positions 21-32 declare the total debit entry dollar amount for the batch.",
	}

	RuleBatchCreditTotal = Rule{
		ID: "NACHA.MATH.BATCH_CREDIT_TOTAL", Version: "1.0.0", Severity: SeverityBlocking,
		Provenance:  ProvenanceFileFormat,
		Description: "The batch control's total credit amount does not match the sum of its credit entries.",
		Citation:    "ACH file format: batch control positions 33-44 declare the total credit entry dollar amount for the batch.",
	}

	RuleFileBatchCount = Rule{
		ID: "NACHA.MATH.FILE_BATCH_COUNT", Version: "1.0.0", Severity: SeverityBlocking,
		Provenance:  ProvenanceFileFormat,
		Description: "The file control's batch count does not match the batches present.",
		Citation:    "ACH file format: file control positions 2-7 declare the number of batch control records in the file.",
	}

	RuleFileEntryCount = Rule{
		ID: "NACHA.MATH.FILE_ENTRY_COUNT", Version: "1.0.0", Severity: SeverityBlocking,
		Provenance:  ProvenanceFileFormat,
		Description: "The file control's entry/addenda count does not match the records counted across all batches.",
		Citation:    "ACH file format: file control positions 14-21 declare the total entry detail and addenda count for the file.",
	}

	RuleFileEntryHash = Rule{
		ID: "NACHA.MATH.FILE_ENTRY_HASH", Version: "1.0.0", Severity: SeverityBlocking,
		Provenance:  ProvenanceFileFormat,
		Description: "The file control's entry hash does not match the sum of the batch hashes.",
		Citation:    "ACH file format: the file entry hash is the sum of the batch entry hashes, truncated to the low-order ten digits.",
	}

	RuleFileDebitTotal = Rule{
		ID: "NACHA.MATH.FILE_DEBIT_TOTAL", Version: "1.0.0", Severity: SeverityBlocking,
		Provenance:  ProvenanceFileFormat,
		Description: "The file control's total debit amount does not match the sum of the batch debit totals.",
		Citation:    "ACH file format: file control positions 32-43 declare the total debit dollar amount in the file.",
	}

	RuleFileCreditTotal = Rule{
		ID: "NACHA.MATH.FILE_CREDIT_TOTAL", Version: "1.0.0", Severity: SeverityBlocking,
		Provenance:  ProvenanceFileFormat,
		Description: "The file control's total credit amount does not match the sum of the batch credit totals.",
		Citation:    "ACH file format: file control positions 44-55 declare the total credit dollar amount in the file.",
	}

	RuleAmountOverflow = Rule{
		ID: "NACHA.MATH.OVERFLOW", Version: "1.0.0", Severity: SeverityBlocking,
		Provenance:  ProvenanceFileFormat,
		Description: "An amount or total exceeded the representable range during accumulation.",
		Citation:    "Arithmetic on a signed 64-bit minor-unit accumulator. A wrapped total would silently balance a file that does not.",
	}

	RuleNumericField = Rule{
		ID: "NACHA.STRUCT.NUMERIC_FIELD", Version: "1.0.0", Severity: SeverityBlocking,
		Provenance:  ProvenanceFileFormat,
		Description: "A field defined as numeric contains a non-numeric character.",
		Citation:    "ACH file format: amount, count and hash fields are right-justified, zero-filled numerics.",
	}

	// --- Routing numbers ---

	RuleRoutingCheckDigit = Rule{
		ID: "NACHA.ROUTING.CHECK_DIGIT", Version: "1.0.0", Severity: SeverityBlocking,
		Provenance:  ProvenanceFileFormat,
		Description: "A routing number fails the ABA check-digit calculation.",
		Citation:    "ABA routing number check digit: 3(d1+d4+d7) + 7(d2+d5+d8) + 1(d3+d6+d9) mod 10 = 0. This is arithmetic on the number itself, not a rule-set requirement.",
	}

	// --- Rules that require the licensed Operating Rules ---
	//
	// Each is declared so the gap is visible in the rule registry rather than
	// implied by absence. None is blocking, and none may become blocking
	// without a licensed source.

	RuleSECCodeSupported = Rule{
		ID: "NACHA.RULES.SEC_CODE", Version: "0.1.0", Severity: SeverityInfo,
		Provenance:  ProvenanceUnverified,
		Description: "Whether the batch's Standard Entry Class code is valid and permitted for this originator was NOT checked.",
		Citation:    "Requires the current Nacha Operating Rules, which name the valid SEC codes and their eligibility conditions. Not available to this repository.",
	}

	RuleAddendaLimit = Rule{
		ID: "NACHA.RULES.ADDENDA_LIMIT", Version: "0.1.0", Severity: SeverityInfo,
		Provenance:  ProvenanceUnverified,
		Description: "Whether the number of addenda records is permitted for this SEC code was NOT checked.",
		Citation:    "The addenda limit varies by SEC code and is defined in the Nacha Operating Rules. Not available to this repository.",
	}

	RuleEffectiveDate = Rule{
		ID: "NACHA.RULES.EFFECTIVE_DATE", Version: "0.1.0", Severity: SeverityInfo,
		Provenance:  ProvenanceUnverified,
		Description: "Whether the effective entry date is a valid banking day within the permitted window was NOT checked.",
		Citation:    "Settlement timing windows and banking-day calendars are defined in the Nacha Operating Rules. Not available to this repository.",
	}

	RuleAmountLimit = Rule{
		ID: "NACHA.RULES.AMOUNT_LIMIT", Version: "0.1.0", Severity: SeverityInfo,
		Provenance:  ProvenanceUnverified,
		Description: "Whether entry amounts fall within per-SEC-code limits was NOT checked.",
		Citation:    "Per-SEC-code amount limits are defined in the Nacha Operating Rules. Not available to this repository.",
	}
)

// AllRules is the registry. It exists so the rule set can be listed, versioned
// and audited without reading the validator, and so a test can assert
// properties across every rule at once.
var AllRules = []Rule{
	RuleFileEmpty, RuleRecordLength, RuleRecordType, RuleCharacterSet,
	RuleFileHeaderMissing, RuleFileControlMissing, RuleTruncated,
	RuleOrphanEntry, RuleOrphanAddenda, RuleBatchNumberSequence,
	RuleBatchEntryCount, RuleBatchEntryHash, RuleBatchDebitTotal, RuleBatchCreditTotal,
	RuleFileBatchCount, RuleFileEntryCount, RuleFileEntryHash,
	RuleFileDebitTotal, RuleFileCreditTotal,
	RuleAmountOverflow, RuleNumericField, RuleRoutingCheckDigit,
	RuleSECCodeSupported, RuleAddendaLimit, RuleEffectiveDate, RuleAmountLimit,
}

// UnverifiedRules lists what this validator does not check, so a caller can
// report the gap rather than let silence imply coverage.
func UnverifiedRules() []Rule {
	var out []Rule
	for _, r := range AllRules {
		if r.Provenance == ProvenanceUnverified {
			out = append(out, r)
		}
	}
	return out
}
