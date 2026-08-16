package main

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"
)

// IsSwiftMessage checks if the payload conforms to SWIFT MT block or tag formatting.
func IsSwiftMessage(content string) bool {
	trimmed := strings.TrimSpace(content)
	if strings.HasPrefix(trimmed, "{1:") || strings.HasPrefix(trimmed, "{4:") {
		return true
	}
	return strings.Contains(content, ":20:") && (strings.Contains(content, ":32A:") || strings.Contains(content, ":60F:") || strings.Contains(content, ":62F:"))
}

// ParseAndValidateSwift processes MT103 and MT940 financial messages.
func ParseAndValidateSwift(content string) (findings []ValidationFindingRecord, totalDebits float64, totalCredits float64, isBalanced bool) {
	isBalanced = true
	lines := strings.Split(content, "\n")

	isMT103 := strings.Contains(content, ":32A:")
	isMT940 := strings.Contains(content, ":60F:") || strings.Contains(content, ":62F:")

	if !isMT103 && !isMT940 {
		findings = append(findings, ValidationFindingRecord{
			Code:        "SWIFT_ERR_0001_UNKNOWN_MESSAGE_TYPE",
			Severity:    "ERROR",
			LineNumber:  1,
			Description: "Unable to identify standard SWIFT MT message category (expected MT103 or MT940 tags).",
		})
		return findings, 0, 0, false
	}

	if isMT103 {
		// Validate MT103 Single Customer Credit Transfer
		mandatoryTags := []string{":20:", ":32A:", ":50K:", ":59:", ":71A:"}
		for _, tag := range mandatoryTags {
			if !strings.Contains(content, tag) {
				findings = append(findings, ValidationFindingRecord{
					Code:        fmt.Sprintf("SWIFT_ERR_0103_MISSING_TAG_%s", strings.Trim(tag, ":")),
					Severity:    "FATAL",
					LineNumber:  1,
					Description: fmt.Sprintf("Mandatory SWIFT MT103 tag %s is missing from message body.", tag),
				})
				isBalanced = false
			}
		}

		// Extract Value Date and Settled Amount from :32A: (Format: YYMMDDCURRENCYAMOUNT)
		re32A := regexp.MustCompile(`:32A:(\d{6})([A-Z]{3})([0-9,]+)`)
		match := re32A.FindStringSubmatch(content)
		if len(match) == 4 {
			currency := match[2]
			amountStr := strings.Replace(match[3], ",", ".", 1)
			amt, err := strconv.ParseFloat(amountStr, 64)
			if err == nil {
				totalCredits = amt
			}
			if currency != "USD" && currency != "EUR" && currency != "GBP" && currency != "JPY" {
				findings = append(findings, ValidationFindingRecord{
					Code:        "SWIFT_ERR_0103_INVALID_CURRENCY",
					Severity:    "ERROR",
					LineNumber:  1,
					Description: fmt.Sprintf("Invalid or unsupported ISO 4217 settlement currency: %s", currency),
				})
			}
		}
	}

	if isMT940 {
		// Validate MT940 Customer Statement Message
		mandatoryTags := []string{":20:", ":25:", ":28C:", ":60F:", ":62F:"}
		for _, tag := range mandatoryTags {
			if !strings.Contains(content, tag) {
				findings = append(findings, ValidationFindingRecord{
					Code:        fmt.Sprintf("SWIFT_ERR_0940_MISSING_TAG_%s", strings.Trim(tag, ":")),
					Severity:    "FATAL",
					LineNumber:  1,
					Description: fmt.Sprintf("Mandatory SWIFT MT940 tag %s is missing from statement.", tag),
				})
				isBalanced = false
			}
		}

		// Verify Opening & Closing Balance Tags
		reBalance := regexp.MustCompile(`:(?:60F|62F):([CD])(\d{6})([A-Z]{3})([0-9,]+)`)
		balances := reBalance.FindAllStringSubmatch(content, -1)
		if len(balances) >= 2 {
			openAmtStr := strings.Replace(balances[0][4], ",", ".", 1)
			closeAmtStr := strings.Replace(balances[1][4], ",", ".", 1)
			openAmt, _ := strconv.ParseFloat(openAmtStr, 64)
			closeAmt, _ := strconv.ParseFloat(closeAmtStr, 64)
			totalCredits = closeAmt
			totalDebits = openAmt
		}
	}

	if len(lines) < 4 {
		findings = append(findings, ValidationFindingRecord{
			Code:        "SWIFT_ERR_0002_INCOMPLETE_BLOCK",
			Severity:    "ERROR",
			LineNumber:  len(lines),
			Description: "SWIFT message payload appears truncated (less than 4 lines).",
		})
		isBalanced = false
	}

	return findings, totalDebits, totalCredits, isBalanced && len(findings) == 0
}
