package nacha

import (
	"fmt"
	"strings"
)

// Fixtures are built by a generator rather than checked in as literal files.
//
// The reason is that a hand-written fixture with a hand-computed entry hash
// drifts the moment anything changes, and the usual fix is to adjust the
// expected value until the test passes -- which turns the arithmetic check into
// a check that the test agrees with itself. Here the generator computes the
// control records the same way the format specifies, so a valid fixture is
// valid by construction, and every invalid fixture is produced by making one
// stated change to a valid one.

// pad right-justifies and zero-fills a number into a fixed width.
func pad(n int64, width int) string {
	s := fmt.Sprintf("%d", n)
	if len(s) > width {
		return s[len(s)-width:]
	}
	return strings.Repeat("0", width-len(s)) + s
}

// text left-justifies and space-fills into a fixed width.
func text(s string, width int) string {
	if len(s) > width {
		return s[:width]
	}
	return s + strings.Repeat(" ", width-len(s))
}

// Routing numbers with valid ABA check digits, verified by
// TestFixtureRoutingNumbersAreValid.
const (
	routingOrigin = "021000021"
	routingRDFI   = "121000358"
	routingRDFI2  = "011000015"
)

type entrySpec struct {
	transactionCode string // 22 credit / 27 debit
	routing         string
	account         string
	amountMinor     int64
	addendaCount    int
}

type batchSpec struct {
	secCode string
	number  int
	entries []entrySpec
}

// buildFile assembles a complete ACH file with correct control records.
func buildFile(batches []batchSpec) string {
	var records []string

	// File header. Positions: 1 type, 2-3 priority, 4-13 immediate destination,
	// 14-23 immediate origin, 24-29 creation date, 30-33 creation time,
	// 34 file id modifier, 35-37 record size, 38-39 blocking factor,
	// 40 format code, 41-63 destination name, 64-86 origin name, 87-94 reference.
	header := "1" + "01" +
		" " + routingRDFI + text("", 0) + // 4-13: leading space + 9 digits
		" " + routingOrigin +
		"260301" + "1200" + "A" + "094" + "10" + "1" +
		text("DESTINATION BANK", 23) + text("ORIGIN COMPANY", 23) + text("", 8)
	records = append(records, header)

	var (
		fileEntries int64
		fileDebits  int64
		fileCredits int64
		fileHash    int64
	)

	for _, b := range batches {
		var (
			batchEntries int64
			batchDebits  int64
			batchCredits int64
			batchHash    int64
		)

		// Batch header: 1 type, 2-4 service class, 5-20 company name,
		// 21-40 discretionary, 41-50 company id, 51-53 SEC code,
		// 54-63 entry description, 64-69 descriptive date, 70-75 effective date,
		// 76-78 settlement, 79 originator status, 80-87 ODFI, 88-94 batch number.
		bh := "5" + "200" + text("ORIGIN COMPANY", 16) + text("", 20) +
			text("1234567890", 10) + text(b.secCode, 3) + text("PAYROLL", 10) +
			"260301" + "260302" + "   " + "1" + routingOrigin[:8] + pad(int64(b.number), 7)
		records = append(records, bh)

		for i, e := range b.entries {
			// Entry detail: 1 type, 2-3 transaction code, 4-11 RDFI routing
			// (8 digits), 12 check digit, 13-29 account, 30-39 amount,
			// 40-54 individual id, 55-76 individual name, 77-78 discretionary,
			// 79 addenda indicator, 80-94 trace number.
			addendaIndicator := "0"
			if e.addendaCount > 0 {
				addendaIndicator = "1"
			}
			ed := "6" + e.transactionCode + e.routing + text(e.account, 17) +
				pad(e.amountMinor, 10) + text("INDIVID"+pad(int64(i), 3), 15) +
				text("EMPLOYEE NAME", 22) + "  " + addendaIndicator +
				routingOrigin[:8] + pad(int64(i+1), 7)
			records = append(records, ed)

			batchEntries++
			if IsDebitTransaction(e.transactionCode) {
				batchDebits += e.amountMinor
			} else {
				batchCredits += e.amountMinor
			}
			prefix := int64(0)
			fmt.Sscanf(e.routing[:8], "%d", &prefix)
			batchHash += prefix

			for a := 0; a < e.addendaCount; a++ {
				ad := "7" + "05" + text("ADDENDA INFORMATION", 80) +
					pad(int64(a+1), 4) + pad(int64(i+1), 7)
				records = append(records, ad)
				batchEntries++
			}
		}

		// Batch control: 1 type, 2-4 service class, 5-10 entry/addenda count,
		// 11-20 entry hash, 21-32 total debits, 33-44 total credits,
		// 45-54 company id, 55-73 MAC + reserved, 74-79 reserved,
		// 80-87 ODFI, 88-94 batch number.
		bc := "8" + "200" + pad(batchEntries, 6) + pad(batchHash%entryHashModulus, 10) +
			pad(batchDebits, 12) + pad(batchCredits, 12) +
			text("1234567890", 10) + text("", 19) + text("", 6) +
			routingOrigin[:8] + pad(int64(b.number), 7)
		records = append(records, bc)

		fileEntries += batchEntries
		fileDebits += batchDebits
		fileCredits += batchCredits
		fileHash += batchHash
	}

	// File control: 1 type, 2-7 batch count, 8-13 block count,
	// 14-21 entry/addenda count, 22-31 entry hash, 32-43 total debits,
	// 44-55 total credits, 56-94 reserved.
	blockCount := (len(records) + 1 + 9) / 10
	fc := "9" + pad(int64(len(batches)), 6) + pad(int64(blockCount), 6) +
		pad(fileEntries, 8) + pad(fileHash%entryHashModulus, 10) +
		pad(fileDebits, 12) + pad(fileCredits, 12) + text("", 39)
	records = append(records, fc)

	return strings.Join(records, "\n") + "\n"
}

// validSingleBatch is a balanced single-batch file: one debit offsetting one
// credit.
func validSingleBatch() string {
	return buildFile([]batchSpec{{
		secCode: "PPD", number: 1,
		entries: []entrySpec{
			{transactionCode: "22", routing: routingRDFI, account: "1234567890", amountMinor: 150000},
			{transactionCode: "27", routing: routingRDFI2, account: "0987654321", amountMinor: 150000},
		},
	}})
}

// validMultiBatch spans two batches and includes addenda.
func validMultiBatch() string {
	return buildFile([]batchSpec{
		{
			secCode: "PPD", number: 1,
			entries: []entrySpec{
				{transactionCode: "22", routing: routingRDFI, account: "1111111111", amountMinor: 250000},
				{transactionCode: "22", routing: routingRDFI2, account: "2222222222", amountMinor: 125050, addendaCount: 1},
			},
		},
		{
			secCode: "CCD", number: 2,
			entries: []entrySpec{
				{transactionCode: "27", routing: routingRDFI, account: "3333333333", amountMinor: 375050},
			},
		},
	})
}

// validUnbalancedCreditOnly is a payroll-shaped file: credits only, no
// offsetting debit. It is entirely correct under a contract that does not
// require balance, which is the point of the fixture.
func validUnbalancedCreditOnly() string {
	return buildFile([]batchSpec{{
		secCode: "PPD", number: 1,
		entries: []entrySpec{
			{transactionCode: "22", routing: routingRDFI, account: "4444444444", amountMinor: 320000},
			{transactionCode: "22", routing: routingRDFI2, account: "5555555555", amountMinor: 280000},
		},
	}})
}

// corrupt replaces a character range in the nth record (1-based).
func corrupt(file string, recordNumber, start, end int, replacement string) string {
	lines := strings.Split(strings.TrimRight(file, "\n"), "\n")
	if recordNumber < 1 || recordNumber > len(lines) {
		panic("fixture: record out of range")
	}
	line := lines[recordNumber-1]
	lines[recordNumber-1] = line[:start-1] + replacement + line[end:]
	return strings.Join(lines, "\n") + "\n"
}
