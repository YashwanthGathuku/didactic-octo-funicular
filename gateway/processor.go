package main

import (
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"fmt"
	"io"
	"mime/multipart"
	"strconv"
	"strings"
	"time"

	"github.com/moov-io/ach"
)

type ValidationFindingRecord struct {
	ID            int64  `json:"id"`
	Code          string `json:"code"`
	Description   string `json:"description"`
	Severity      string `json:"severity"`
	LineNumber    int    `json:"lineNumber"`
	RawData       string `json:"rawData"`
	RuleReference string `json:"ruleReference"`
}

type IngestionResult struct {
	FileID             int64                     `json:"fileId"`
	Filename           string                    `json:"filename"`
	Hash               string                    `json:"hash"`
	SizeBytes          int64                     `json:"sizeBytes"`
	Status             string                    `json:"status"` // QUARANTINED, RELEASED
	TotalRecordsParsed int                       `json:"totalRecordsParsed"`
	TotalDebitsUsd     float64                   `json:"totalDebitsUsd"`
	TotalCreditsUsd    float64                   `json:"totalCreditsUsd"`
	CalculatedHash     string                    `json:"calculatedHash"`
	ExpectedHash       string                    `json:"expectedHash"`
	IsBalanced         bool                      `json:"isBalanced"`
	Findings           []ValidationFindingRecord `json:"findings"`
	RawContent         string                    `json:"rawContent,omitempty"`
	IncidentID         *int64                    `json:"incidentId,omitempty"`
}

// ValidateRoutingMod10 verifies Federal Reserve Modulo 10 check digit on ABA routing numbers.
func ValidateRoutingMod10(routing string) bool {
	if len(routing) != 9 {
		return false
	}
	weights := []int{3, 7, 1, 3, 7, 1, 3, 7}
	sum := 0
	for i := 0; i < 8; i++ {
		d, err := strconv.Atoi(string(routing[i]))
		if err != nil {
			return false
		}
		sum += d * weights[i]
	}
	checkDigit, err := strconv.Atoi(string(routing[8]))
	if err != nil {
		return false
	}
	expected := (10 - (sum % 10)) % 10
	return checkDigit == expected
}

// hasBlockingFinding reports whether any finding disqualifies the artifact from
// release. FATAL, CRITICAL and ERROR block; WARNING and INFO are advisory.
//
// Severity strings are still free-form here. Prompt 07 replaces them with a
// typed severity and a versioned release policy.
func hasBlockingFinding(findings []ValidationFindingRecord) bool {
	for _, f := range findings {
		switch f.Severity {
		case "FATAL", "CRITICAL", "ERROR":
			return true
		}
	}
	return false
}

// ProcessFileBytes processes raw file contents, calculates SHA256, validates NACHA, and persists to DB.
func ProcessFileBytes(db *sql.DB, filename string, content []byte) (*IngestionResult, error) {
	// Every financial input begins untrusted and unreleased. RELEASED is only
	// ever reached through the explicit terminal decision below.
	result := &IngestionResult{
		Filename:   filename,
		Status:     "RECEIVED",
		Findings:   make([]ValidationFindingRecord, 0),
		RawContent: string(content),
		SizeBytes:  int64(len(content)),
	}

	// 1. Calculate SHA-256
	hasher := sha256.New()
	hasher.Write(content)
	result.Hash = hex.EncodeToString(hasher.Sum(nil))

	trimmed := strings.TrimSpace(string(content))

	// parserSucceeded records whether the format parser could read the file at
	// all. Only the NACHA branch sets it false today; the experimental parsers
	// report through findings instead.
	parserSucceeded := true

	// 2. Multi-Format Detection: ISO 20022 XML vs BAI2 vs SWIFT MT vs NACHA ACH
	if strings.HasPrefix(trimmed, "<?xml") || strings.HasPrefix(trimmed, "<Document") || strings.HasSuffix(strings.ToLower(filename), ".xml") {
		isoFindings, debits, credits, count, balanced := ValidateIso20022Xml(content)
		result.Findings = append(result.Findings, isoFindings...)
		result.TotalDebitsUsd = debits
		result.TotalCreditsUsd = credits
		result.TotalRecordsParsed = count
		result.IsBalanced = balanced
		if len(isoFindings) > 0 {
			result.Status = "QUARANTINED"
		}
	} else if strings.HasPrefix(trimmed, "01,") || strings.HasSuffix(strings.ToLower(filename), ".bai") || strings.HasSuffix(strings.ToLower(filename), ".bai2") {
		baiFindings, debits, credits, count, balanced := ValidateBai2File(content)
		result.Findings = append(result.Findings, baiFindings...)
		result.TotalDebitsUsd = debits
		result.TotalCreditsUsd = credits
		result.TotalRecordsParsed = count
		result.IsBalanced = balanced
		if len(baiFindings) > 0 {
			result.Status = "QUARANTINED"
		}
	} else if IsSwiftMessage(string(content)) || strings.HasSuffix(strings.ToLower(filename), ".swift") || strings.HasSuffix(strings.ToLower(filename), ".mt103") || strings.HasSuffix(strings.ToLower(filename), ".mt940") {
		swiftFindings, debits, credits, balanced := ParseAndValidateSwift(string(content))
		result.Findings = append(result.Findings, swiftFindings...)
		result.TotalDebitsUsd = debits
		result.TotalCreditsUsd = credits
		result.TotalRecordsParsed = len(strings.Split(string(content), "\n"))
		result.IsBalanced = balanced
		if len(swiftFindings) > 0 {
			result.Status = "QUARANTINED"
		}
	} else {
		// 3. Custom Line-by-Line NACHA verification
		lines := strings.Split(strings.ReplaceAll(string(content), "\r\n", "\n"), "\n")
		var validLines []string
		for _, l := range lines {
			if strings.TrimSpace(l) != "" {
				validLines = append(validLines, l)
			}
		}

		var entryHashSum int64 = 0
		var batchDebits int64 = 0
		var batchCredits int64 = 0
		var expectedEntryHash string = ""
		var declaredBatchDebits int64 = 0
		var declaredBatchCredits int64 = 0
		var totalRecords = len(validLines)
		result.TotalRecordsParsed = totalRecords

		for idx, line := range validLines {
			lineNum := idx + 1
			if len(line) != 94 {
				result.Findings = append(result.Findings, ValidationFindingRecord{
					Code:          "ACH_ERR_0001_INVALID_RECORD_LENGTH",
					Description:   fmt.Sprintf("Record at line %d has length %d, expected exactly 94 characters.", lineNum, len(line)),
					Severity:      "FATAL",
					LineNumber:    lineNum,
					RawData:       line,
					RuleReference: "Nacha Operating Rules 2025, Section 3.1: Standard File Layout",
				})
				result.Status = "QUARANTINED"
			}

			if len(line) > 0 {
				recordType := string(line[0])
				switch recordType {
				case "6": // Entry Detail
					if len(line) >= 12 {
						routing := line[3:12]
						if !ValidateRoutingMod10(routing) {
							result.Findings = append(result.Findings, ValidationFindingRecord{
								Code:          "ACH_ERR_0602_INVALID_ABA_CHECK_DIGIT",
								Description:   fmt.Sprintf("Receiving DFI Identification '%s' at line %d failed Federal Reserve Modulo 10 check digit verification.", routing, lineNum),
								Severity:      "ERROR",
								LineNumber:    lineNum,
								RawData:       line,
								RuleReference: "Nacha Operating Rules 2025, Appendix 1: ABA Routing Check Digits",
							})
							result.Status = "QUARANTINED"
						}
						// Add first 8 digits to Entry Hash
						if r8, err := strconv.ParseInt(routing[:8], 10, 64); err == nil {
							entryHashSum += r8
						}
					}
					if len(line) >= 39 {
						txCode := line[1:3]
						amt, _ := strconv.ParseInt(line[29:39], 10, 64)
						if txCode == "27" || txCode == "37" || txCode == "55" {
							batchDebits += amt
						} else {
							batchCredits += amt
						}
					}
				case "8": // Batch Control
					if len(line) >= 44 {
						expectedEntryHash = strings.TrimSpace(line[10:20])
						declaredBatchDebits, _ = strconv.ParseInt(line[20:32], 10, 64)
						declaredBatchCredits, _ = strconv.ParseInt(line[32:44], 10, 64)
					}
				}
			}
		}

		calcHashStr := fmt.Sprintf("%010d", entryHashSum%10000000000)
		result.CalculatedHash = calcHashStr
		result.ExpectedHash = expectedEntryHash
		result.TotalDebitsUsd = float64(batchDebits) / 100.0
		result.TotalCreditsUsd = float64(batchCredits) / 100.0
		result.IsBalanced = (batchDebits == batchCredits)

		if expectedEntryHash != "" && calcHashStr != expectedEntryHash {
			result.Findings = append(result.Findings, ValidationFindingRecord{
				Code:          "ACH_ERR_0802_HASH_MISMATCH",
				Description:   fmt.Sprintf("Batch Control Entry Hash mismatch. Declared trailer: %s, Sum of entry 8-digit routing numbers: %s.", expectedEntryHash, calcHashStr),
				Severity:      "CRITICAL",
				LineNumber:    len(validLines) - 1,
				RawData:       fmt.Sprintf("EntryHash: %s vs Calc: %s", expectedEntryHash, calcHashStr),
				RuleReference: "Nacha Operating Rules 2025, Section 3.2.1: Entry Hash Field Validation",
			})
			result.Status = "QUARANTINED"
		}

		if declaredBatchDebits > 0 && declaredBatchDebits != batchDebits {
			result.Findings = append(result.Findings, ValidationFindingRecord{
				Code:          "ACH_ERR_0803_CONTROL_OUT_OF_BALANCE",
				Description:   fmt.Sprintf("Batch Control total debits mismatch. Declared: $%.2f, Actual sum of entries: $%.2f.", float64(declaredBatchDebits)/100.0, float64(batchDebits)/100.0),
				Severity:      "CRITICAL",
				LineNumber:    len(validLines) - 1,
				RawData:       fmt.Sprintf("Debits declared %d vs sum %d", declaredBatchDebits, batchDebits),
				RuleReference: "Nacha Operating Rules 2025, Section 3.2.2: Batch Control Arithmetic Verification",
			})
			result.Status = "QUARANTINED"
		}

		if declaredBatchCredits > 0 && declaredBatchCredits != batchCredits {
			result.Findings = append(result.Findings, ValidationFindingRecord{
				Code:          "ACH_ERR_0804_CREDIT_CONTROL_MISMATCH",
				Description:   fmt.Sprintf("Batch Control total credits mismatch. Declared: $%.2f, Actual sum of entries: $%.2f.", float64(declaredBatchCredits)/100.0, float64(batchCredits)/100.0),
				Severity:      "CRITICAL",
				LineNumber:    len(validLines) - 1,
				RawData:       fmt.Sprintf("Credits declared %d vs sum %d", declaredBatchCredits, batchCredits),
				RuleReference: "Nacha Operating Rules 2025, Section 3.2.2: Batch Control Arithmetic Verification",
			})
			result.Status = "QUARANTINED"
		}

		// Also parse with Moov ACH if format allows.
		//
		// A parser that cannot read the file is disqualifying, not advisory. This
		// finding was previously recorded at WARNING and was the only finding
		// branch that did not affect the release decision, which is how a zero-byte
		// file reached RELEASED.
		reader := ach.NewReader(strings.NewReader(string(content)))
		if _, err := reader.Read(); err != nil {
			parserSucceeded = false
			if len(result.Findings) == 0 {
				result.Findings = append(result.Findings, ValidationFindingRecord{
					Code:          "ACH_ERR_0099_PARSER_EXCEPTION",
					Description:   fmt.Sprintf("Moov ACH Parser reported: %v", err),
					Severity:      "FATAL",
					LineNumber:    1,
					RawData:       "",
					RuleReference: "Nacha Standard Specification 2025",
				})
			}
		}
	}

	// Terminal release decision.
	//
	// Deliberately minimal fail-closed behaviour, not the versioned policy
	// engine (Prompt 07). It can only make the outcome stricter: a status
	// already set to QUARANTINED above is never promoted back to RELEASED.
	if result.TotalRecordsParsed == 0 {
		// debits == credits over zero records is arithmetically true and
		// operationally meaningless: it asserts a property of records that do
		// not exist.
		result.IsBalanced = false
	}
	if !parserSucceeded || result.TotalRecordsParsed == 0 || hasBlockingFinding(result.Findings) {
		result.Status = "QUARANTINED"
	} else if result.Status == "RECEIVED" {
		result.Status = "RELEASED"
	}

	// 3. Persist to Database
	now := time.Now().UTC().Format(time.RFC3339)
	res, err := db.Exec(`
		INSERT INTO file_instances (expectation_id, filename, storage_path, size_bytes, sha256_hash, status, received_at)
		VALUES (1, ?, ?, ?, ?, ?, ?)
	`, filename, "/s3/incoming/"+filename, len(content), result.Hash, result.Status, now)
	if err != nil {
		return nil, fmt.Errorf("failed to save file instance: %w", err)
	}

	fileID, _ := res.LastInsertId()
	result.FileID = fileID

	// Insert findings
	for i := range result.Findings {
		f := &result.Findings[i]
		fRes, _ := db.Exec(`
			INSERT INTO validation_findings (file_instance_id, code, description, severity, line_number, raw_data, created_at)
			VALUES (?, ?, ?, ?, ?, ?, ?)
		`, fileID, f.Code, f.Description, f.Severity, f.LineNumber, f.RawData, now)
		fID, _ := fRes.LastInsertId()
		f.ID = fID
	}

	// If quarantined, create incident
	if result.Status == "QUARANTINED" {
		incRes, err := db.Exec(`
			INSERT INTO incidents (expectation_id, file_instance_id, type, severity, status, created_at, updated_at)
			VALUES (1, ?, 'NACHA_ENTRY_HASH_MISMATCH', 'CRITICAL', 'OPEN', ?, ?)
		`, fileID, now, now)
		if err == nil {
			incID, _ := incRes.LastInsertId()
			result.IncidentID = &incID
		}
	}

	// 4. Append to Audit Ledger
	_, _ = AppendAuditEvent(db, "FILE_INGESTED", "SENTINEL_GATEWAY_WORKER", map[string]interface{}{
		"fileId":         fileID,
		"filename":       filename,
		"sha256":         result.Hash,
		"sizeBytes":      len(content),
		"status":         result.Status,
		"findingsCount":  len(result.Findings),
		"totalDebitsUsd": result.TotalDebitsUsd,
	})

	return result, nil
}

// ProcessFile streams the uploaded file, computes its SHA-256 hash, and parses NACHA.
func ProcessFile(db *sql.DB, file multipart.File, header *multipart.FileHeader) (*IngestionResult, error) {
	content, err := io.ReadAll(file)
	if err != nil {
		return nil, fmt.Errorf("failed to read upload: %w", err)
	}
	return ProcessFileBytes(db, header.Filename, content)
}
