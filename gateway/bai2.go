package main

import (
	"sentinel-gateway/internal/nacha"

	"strconv"
	"strings"
)

// ValidateBai2File parses and verifies BAI2 cash balance and transaction records.
func ValidateBai2File(content []byte) ([]ValidationFindingRecord, float64, float64, int, bool) {
	findings := make([]ValidationFindingRecord, 0)
	var totalDebits float64 = 0
	var totalCredits float64 = 0
	var recordCount = 0

	lines := strings.Split(strings.ReplaceAll(string(content), "\r\n", "\n"), "\n")
	var validLines []string
	for _, l := range lines {
		if strings.TrimSpace(l) != "" {
			validLines = append(validLines, strings.TrimSpace(l))
		}
	}
	recordCount = len(validLines)

	if len(validLines) == 0 {
		findings = append(findings, ValidationFindingRecord{
			Code:        "BAI_ERR_0001_EMPTY_FILE",
			Description: "BAI2 file is empty (0 records found).",
			Severity:    "FATAL",
			LineNumber:  1,
		})
		return findings, 0, 0, 0, false
	}

	// Line 1 must start with 01 (File Header)
	if !strings.HasPrefix(validLines[0], "01,") {
		findings = append(findings, ValidationFindingRecord{
			Code:        "BAI_ERR_0002_MISSING_FILE_HEADER",
			Description: "First record must be Record 01 (File Header).",
			Severity:    "FATAL",
			LineNumber:  1,
			Evidence:    nacha.RedactEvidence(validLines[0]),
		})
	}

	// Last Line must start with 99 (File Trailer)
	lastLine := validLines[len(validLines)-1]
	if !strings.HasPrefix(lastLine, "99,") {
		findings = append(findings, ValidationFindingRecord{
			Code:        "BAI_ERR_0003_MISSING_FILE_TRAILER",
			Description: "Last record must be Record 99 (File Trailer).",
			Severity:    "CRITICAL",
			LineNumber:  len(validLines),
			Evidence:    nacha.RedactEvidence(lastLine),
		})
	}

	// Parse transaction amounts in Record 16
	for _, line := range validLines {
		if strings.HasPrefix(line, "16,") {
			parts := strings.Split(line, ",")
			if len(parts) >= 3 {
				// Amount in cents / minor currency unit
				if amtCents, err := strconv.ParseInt(parts[2], 10, 64); err == nil {
					amtUsd := float64(amtCents) / 100.0
					if strings.Contains(parts[1], "4") || strings.Contains(parts[1], "5") {
						totalDebits += amtUsd
					} else {
						totalCredits += amtUsd
					}
				}
			}
		}
	}

	isBalanced := (totalDebits == totalCredits)
	return findings, totalDebits, totalCredits, recordCount, isBalanced
}
