package candidate

import (
	"strings"
	"testing"
)

// Helper to run operations
func runOps(t *testing.T, nacha string, ops []RemediationOperation) string {
	t.Helper()
	svc := &Service{}
	out, _, _, err := svc.applyDeterministicOperations([]byte(nacha), ops)
	if err != nil {
		t.Fatalf("applyDeterministicOperations failed: %v", err)
	}
	return string(out)
}

func pad(s string, width int) string {
	if len(s) > width {
		return s[:width]
	}
	return s + strings.Repeat(" ", width-len(s))
}

func padInt(n int64, width int) string {
	s := ""
	if n == 0 {
		s = "0"
	} else {
		res := []byte{}
		temp := n
		for temp > 0 {
			res = append([]byte{byte(temp%10) + '0'}, res...)
			temp /= 10
		}
		s = string(res)
	}
	if len(s) > width {
		return s[len(s)-width:]
	}
	return strings.Repeat("0", width-len(s)) + s
}

func TestVector_SingleBatch_KnownEntryHash(t *testing.T) {
	// routing numbers: 121000358, 011000015
	// hashes: 12100035, 01100001 -> sum = 13200036
	h := "101 121000358 0210000212603011200A094101" + pad("DEST", 23) + pad("ORIG", 23) + pad("", 8)
	bh := "5200" + pad("ORIG", 16) + pad("", 20) + pad("1234567890", 10) + "PPD" + pad("PAY", 10) + "260301260302   1021000020000001"
	e1 := "622121000358" + pad("1", 17) + padInt(100, 10) + pad("ID", 15) + pad("N1", 22) + "  0021000020000001"
	e2 := "622011000015" + pad("2", 17) + padInt(200, 10) + pad("ID", 15) + pad("N2", 22) + "  0021000020000002"
	bc := "8200000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000"
	fc := "9000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000"

	nacha := strings.Join([]string{h, bh, e1, e2, bc, fc}, "\n") + "\n"
	ops := []RemediationOperation{
		{OperationType: OpRecomputeBatchControlTotal, TargetRef: "BATCH-1"},
	}
	out := runOps(t, nacha, ops)
	lines := strings.Split(strings.TrimSuffix(out, "\n"), "\n")
	batchControl := lines[4]
	if batchControl[10:20] != "0013200036" {
		t.Errorf("Expected entry hash 0013200036, got %s", batchControl[10:20])
	}
}

func TestVector_MultipleBatches_KnownSums(t *testing.T) {
	h := "101 121000358 0210000212603011200A094101" + pad("DEST", 23) + pad("ORIG", 23) + pad("", 8)
	bh1 := "5200" + pad("ORIG", 16) + pad("", 20) + pad("1234567890", 10) + "PPD" + pad("PAY", 10) + "260301260302   1021000020000001"
	e1 := "622121000358" + pad("1", 17) + padInt(100, 10) + pad("ID", 15) + pad("N1", 22) + "  0021000020000001"
	bc1 := "8200000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000"
	bh2 := "5200" + pad("ORIG", 16) + pad("", 20) + pad("1234567890", 10) + "PPD" + pad("PAY", 10) + "260301260302   1021000020000002"
	e2 := "622011000015" + pad("2", 17) + padInt(200, 10) + pad("ID", 15) + pad("N2", 22) + "  0021000020000002"
	bc2 := "8200000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000"
	fc := "9000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000"
	nacha := strings.Join([]string{h, bh1, e1, bc1, bh2, e2, bc2, fc}, "\n") + "\n"
	ops := []RemediationOperation{
		{OperationType: OpRecomputeBatchControlTotal, TargetRef: "BATCH-1"},
		{OperationType: OpRecomputeBatchControlTotal, TargetRef: "BATCH-2"},
		{OperationType: OpRecomputeFileControlTotal, TargetRef: "FILE_CONTROL"},
	}
	out := runOps(t, nacha, ops)
	lines := strings.Split(strings.TrimSuffix(out, "\n"), "\n")
	if lines[3][10:20] != "0012100035" {
		t.Errorf("Expected batch 1 hash 0012100035, got %s", lines[3][10:20])
	}
	if lines[6][10:20] != "0001100001" {
		t.Errorf("Expected batch 2 hash 0001100001, got %s", lines[6][10:20])
	}
	if lines[7][21:31] != "0013200036" { // pos 22-31 is index 21:31
		t.Errorf("Expected file hash 0013200036, got %s", lines[7][21:31])
	}
}

func TestVector_LargeEntryHash_10DigitTruncation(t *testing.T) {
	// routing sum > 10,000,000,000
	h := "101 121000358 0210000212603011200A094101" + pad("DEST", 23) + pad("ORIG", 23) + pad("", 8)
	bh := "5200" + pad("ORIG", 16) + pad("", 20) + pad("1234567890", 10) + "PPD" + pad("PAY", 10) + "260301260302   1021000020000001"

	// Create 15 entries with routing 99999999
	var entries []string
	for i := 0; i < 15; i++ {
		e := "622999999999" + pad("1", 17) + padInt(10, 10) + pad("ID", 15) + pad("N", 22) + "  0021000020000001"
		entries = append(entries, e)
	}

	bc := "8200000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000"
	fc := "9000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000"

	all := []string{h, bh}
	all = append(all, entries...)
	all = append(all, bc, fc)

	nacha := strings.Join(all, "\n") + "\n"
	ops := []RemediationOperation{
		{OperationType: OpRecomputeBatchControlTotal, TargetRef: "BATCH-1"},
		{OperationType: OpRecomputeFileControlTotal, TargetRef: "FILE_CONTROL"},
	}
	out := runOps(t, nacha, ops)
	lines := strings.Split(strings.TrimSuffix(out, "\n"), "\n")
	batchControl := lines[17]
	fileControl := lines[18]

	// 15 * 99999999 = 1499999985
	// mod 10^10 = 1499999985 -> padded to 10 is 1499999985
	expected := "1499999985"
	if batchControl[10:20] != expected {
		t.Errorf("Expected %s, got %s", expected, batchControl[10:20])
	}
	if fileControl[21:31] != expected {
		t.Errorf("Expected %s, got %s", expected, fileControl[21:31])
	}
}

func TestVector_AddendaCounts(t *testing.T) {
	h := "101 121000358 0210000212603011200A094101" + pad("DEST", 23) + pad("ORIG", 23) + pad("", 8)
	bh := "5200" + pad("ORIG", 16) + pad("", 20) + pad("1234567890", 10) + "PPD" + pad("PAY", 10) + "260301260302   1021000020000001"
	e1 := "622121000358" + pad("1", 17) + padInt(100, 10) + pad("ID", 15) + pad("N1", 22) + "  0021000020000001"
	a1 := "705" + pad("ADDENDA1", 80) + "00010000001"
	a2 := "705" + pad("ADDENDA2", 80) + "00020000001"
	e2 := "622011000015" + pad("2", 17) + padInt(200, 10) + pad("ID", 15) + pad("N2", 22) + "  0021000020000002"
	a3 := "705" + pad("ADDENDA3", 80) + "00010000002"
	bc := "8200000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000"

	nacha := strings.Join([]string{h, bh, e1, a1, a2, e2, a3, bc}, "\n") + "\n"
	ops := []RemediationOperation{
		{OperationType: OpRecomputeBatchControlTotal, TargetRef: "BATCH-1"},
	}
	out := runOps(t, nacha, ops)
	lines := strings.Split(strings.TrimSuffix(out, "\n"), "\n")
	batchControl := lines[7]
	// entry count is at pos 4-10
	if batchControl[4:10] != "000005" {
		t.Errorf("Expected entry count 000005, got %s", batchControl[4:10])
	}
}

func TestVector_DebitOnlyBatch(t *testing.T) {
	h := "101 121000358 0210000212603011200A094101" + pad("DEST", 23) + pad("ORIG", 23) + pad("", 8)
	bh := "5200" + pad("ORIG", 16) + pad("", 20) + pad("1234567890", 10) + "PPD" + pad("PAY", 10) + "260301260302   1021000020000001"
	e1 := "627121000358" + pad("1", 17) + padInt(100, 10) + pad("ID", 15) + pad("N1", 22) + "  0021000020000001"
	e2 := "627121000358" + pad("1", 17) + padInt(200, 10) + pad("ID", 15) + pad("N1", 22) + "  0021000020000001"
	bc := "8200000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000"

	nacha := strings.Join([]string{h, bh, e1, e2, bc}, "\n") + "\n"
	ops := []RemediationOperation{
		{OperationType: OpRecomputeBatchControlTotal, TargetRef: "BATCH-1"},
	}
	out := runOps(t, nacha, ops)
	lines := strings.Split(strings.TrimSuffix(out, "\n"), "\n")
	batchControl := lines[4]
	if batchControl[20:32] != "000000000300" { // debits
		t.Errorf("Expected debits 000000000300, got %s", batchControl[20:32])
	}
	if batchControl[32:44] != "000000000000" { // credits
		t.Errorf("Expected credits 000000000000, got %s", batchControl[32:44])
	}
}

func TestVector_CreditOnlyBatch(t *testing.T) {
	h := "101 121000358 0210000212603011200A094101" + pad("DEST", 23) + pad("ORIG", 23) + pad("", 8)
	bh := "5200" + pad("ORIG", 16) + pad("", 20) + pad("1234567890", 10) + "PPD" + pad("PAY", 10) + "260301260302   1021000020000001"
	e1 := "622121000358" + pad("1", 17) + padInt(150, 10) + pad("ID", 15) + pad("N1", 22) + "  0021000020000001"
	e2 := "622121000358" + pad("1", 17) + padInt(250, 10) + pad("ID", 15) + pad("N1", 22) + "  0021000020000001"
	bc := "8200000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000"

	nacha := strings.Join([]string{h, bh, e1, e2, bc}, "\n") + "\n"
	ops := []RemediationOperation{
		{OperationType: OpRecomputeBatchControlTotal, TargetRef: "BATCH-1"},
	}
	out := runOps(t, nacha, ops)
	lines := strings.Split(strings.TrimSuffix(out, "\n"), "\n")
	batchControl := lines[4]
	if batchControl[20:32] != "000000000000" { // debits
		t.Errorf("Expected debits 000000000000, got %s", batchControl[20:32])
	}
	if batchControl[32:44] != "000000000400" { // credits
		t.Errorf("Expected credits 000000000400, got %s", batchControl[32:44])
	}
}

func TestVector_MixedBatch(t *testing.T) {
	h := "101 121000358 0210000212603011200A094101" + pad("DEST", 23) + pad("ORIG", 23) + pad("", 8)
	bh := "5200" + pad("ORIG", 16) + pad("", 20) + pad("1234567890", 10) + "PPD" + pad("PAY", 10) + "260301260302   1021000020000001"
	e1 := "622121000358" + pad("1", 17) + padInt(150, 10) + pad("ID", 15) + pad("N1", 22) + "  0021000020000001"
	e2 := "627121000358" + pad("1", 17) + padInt(250, 10) + pad("ID", 15) + pad("N1", 22) + "  0021000020000001"
	bc := "8200000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000"

	nacha := strings.Join([]string{h, bh, e1, e2, bc}, "\n") + "\n"
	ops := []RemediationOperation{
		{OperationType: OpRecomputeBatchControlTotal, TargetRef: "BATCH-1"},
	}
	out := runOps(t, nacha, ops)
	lines := strings.Split(strings.TrimSuffix(out, "\n"), "\n")
	batchControl := lines[4]
	if batchControl[20:32] != "000000000250" { // debits
		t.Errorf("Expected debits 000000000250, got %s", batchControl[20:32])
	}
	if batchControl[32:44] != "000000000150" { // credits
		t.Errorf("Expected credits 000000000150, got %s", batchControl[32:44])
	}
}

func TestVector_FilePaddingAndBlockCount(t *testing.T) {
	h := "101 121000358 0210000212603011200A094101" + pad("DEST", 23) + pad("ORIG", 23) + pad("", 8)
	bh := "5200" + pad("ORIG", 16) + pad("", 20) + pad("1234567890", 10) + "PPD" + pad("PAY", 10) + "260301260302   1021000020000001"
	e1 := "622121000358" + pad("1", 17) + padInt(150, 10) + pad("ID", 15) + pad("N1", 22) + "  0021000020000001"
	e2 := "627121000358" + pad("1", 17) + padInt(250, 10) + pad("ID", 15) + pad("N1", 22) + "  0021000020000001"
	bc := "8200000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000"
	fc := "9000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000"

	// 6 lines -> block count = 1
	nacha := strings.Join([]string{h, bh, e1, e2, bc, fc}, "\n") + "\n"
	ops := []RemediationOperation{
		{OperationType: OpRecomputeFileControlTotal, TargetRef: "FILE_CONTROL"},
	}
	out := runOps(t, nacha, ops)
	lines := strings.Split(strings.TrimSuffix(out, "\n"), "\n")
	fileControl := lines[5]
	if fileControl[7:13] != "000001" {
		t.Errorf("Expected block count 1, got %s", fileControl[7:13])
	}

	// Add 5 padding lines to make it 11 -> block count = 2
	all := []string{h, bh, e1, e2, bc, fc}
	for i := 0; i < 5; i++ {
		all = append(all, strings.Repeat("9", 94))
	}
	nacha = strings.Join(all, "\n") + "\n"
	out = runOps(t, nacha, ops)
	lines = strings.Split(strings.TrimSuffix(out, "\n"), "\n")
	fileControl = lines[5]
	if fileControl[7:13] != "000002" {
		t.Errorf("Expected block count 2, got %s", fileControl[7:13])
	}
}
