package toolgateway

import (
	"encoding/json"
	"fmt"
	"regexp"
	"strings"

	"sentinel-gateway/internal/policy"
)

var (
	// Defense-in-depth regex patterns for unstructured text leak detection
	unmaskedSSNRegex = regexp.MustCompile(`\b\d{3}-\d{2}-\d{4}\b`)
	nineDigitRegex   = regexp.MustCompile(`\b\d{9}\b`)

	// Prohibited structured key names that must never cross the AI boundary
	// secret-scan-allow: list of prohibited secret key names for leak detector
	forbiddenSecretKeys = map[string]struct{}{
		"api_key":      {},
		"secret_key":   {},
		"private_key":  {},
		"password":     {},
		"auth_token":   {},
		"access_token": {},
		"db_password":  {},
		"kms_key":      {},
		"raw_secret":   {},
	}

	forbiddenRawFinancialKeys = map[string]struct{}{
		"raw_account_number": {},
		"unmasked_account":   {},
		"raw_pan":            {},
		"cvv":                {},
		"unmasked_ssn":       {},
		"raw_routing_number": {},
	}
)

// ValidateInput verifies the raw input arguments against size and JSON constraints.
func ValidateInput(args json.RawMessage, maxBytes int64) error {
	if maxBytes <= 0 {
		maxBytes = DefaultMaxInputBytes
	}
	if int64(len(args)) > maxBytes {
		return fmt.Errorf("%w: input size %d exceeds max %d bytes", ErrInputTooLarge, len(args), maxBytes)
	}
	if len(args) > 0 {
		if err := policy.ValidateJSONNoDuplicates(args); err != nil {
			return fmt.Errorf("%w: %v", ErrInputValidationFailed, err)
		}
	}
	return nil
}

// ValidateOutput verifies tool output against schema classifications, forbidden keys, and defense-in-depth leak checks.
func ValidateOutput(output json.RawMessage, manifest *ToolManifest, execCtx *TrustedExecutionContext) error {
	if int64(len(output)) > manifest.MaxOutputBytes {
		return fmt.Errorf("%w: output size %d exceeds max %d bytes", ErrOutputTooLarge, len(output), manifest.MaxOutputBytes)
	}

	if len(output) == 0 {
		return nil
	}

	var generic interface{}
	if err := json.Unmarshal(output, &generic); err != nil {
		return fmt.Errorf("%w: tool output is not valid JSON: %v", ErrOutputValidationFailed, err)
	}

	// Permitted classifications set
	allowedClassifications := make(map[DataClassification]struct{})
	for _, c := range manifest.AllowedOutputClassifications {
		allowedClassifications[c] = struct{}{}
	}
	if len(allowedClassifications) == 0 {
		for _, c := range manifest.DataClassifications {
			allowedClassifications[c] = struct{}{}
		}
	}

	// 1. Authoritative structured classification & forbidden key check
	if err := inspectStructuredOutput(generic, allowedClassifications, execCtx); err != nil {
		return err
	}

	// 2. Defense-in-depth regex & algorithmic verification
	outputStr := string(output)

	// SSN leak check
	if unmaskedSSNRegex.MatchString(outputStr) {
		return fmt.Errorf("%w: output contains unmasked SSN pattern", ErrOutputValidationFailed)
	}

	// Routing number leak check (valid 9-digit Mod-10 routing number)
	for _, match := range nineDigitRegex.FindAllString(outputStr, -1) {
		if isValidRoutingNumber(match) {
			// If output is not classified as internal/raw financial, valid routing number in output is blocked
			if _, ok := allowedClassifications[ClassificationFinancialSensitive]; !ok {
				return fmt.Errorf("%w: output contains unmasked 9-digit routing transit number (%s)", ErrOutputValidationFailed, match)
			}
		}
	}

	return nil
}

func inspectStructuredOutput(v interface{}, allowed map[DataClassification]struct{}, execCtx *TrustedExecutionContext) error {
	switch val := v.(type) {
	case map[string]interface{}:
		for k, item := range val {
			kLower := strings.ToLower(k)

			// Prohibit secret keys
			if _, isSecret := forbiddenSecretKeys[kLower]; isSecret {
				return fmt.Errorf("%w: output contains forbidden secret key %q", ErrOutputValidationFailed, k)
			}

			// Prohibit raw unredacted financial keys
			if _, isRaw := forbiddenRawFinancialKeys[kLower]; isRaw {
				return fmt.Errorf("%w: output contains unredacted financial key %q", ErrOutputValidationFailed, k)
			}

			// Check explicit data classification tag
			if kLower == "data_classification" || kLower == "classification" || kLower == "confidentiality" {
				if classStr, ok := item.(string); ok {
					classUpper := DataClassification(strings.ToUpper(classStr))

					// SECRET is never permitted across AI tier boundary
					if classUpper == ClassificationSecret && (execCtx == nil || execCtx.CallerType == CallerTypeAgent) {
						return fmt.Errorf("%w: field marked SECRET is strictly prohibited across AI boundary", ErrOutputValidationFailed)
					}

					// Verify classification is permitted by manifest
					if len(allowed) > 0 {
						if _, isAllowed := allowed[classUpper]; !isAllowed {
							return fmt.Errorf("%w: output classification %s is not permitted by manifest", ErrOutputValidationFailed, classUpper)
						}
					}
				}
			}

			if err := inspectStructuredOutput(item, allowed, execCtx); err != nil {
				return err
			}
		}
	case []interface{}:
		for _, item := range val {
			if err := inspectStructuredOutput(item, allowed, execCtx); err != nil {
				return err
			}
		}
	}
	return nil
}

// isValidRoutingNumber verifies an ABA routing number using the Federal Reserve Mod-10 checksum algorithm.
func isValidRoutingNumber(s string) bool {
	if len(s) != 9 {
		return false
	}
	var d [9]int
	for i := 0; i < 9; i++ {
		if s[i] < '0' || s[i] > '9' {
			return false
		}
		d[i] = int(s[i] - '0')
	}
	// Prefix range check: US Fedwire/ACH routing numbers must start with 01-12, 21-32, 61-72, or 80
	prefix := d[0]*10 + d[1]
	validPrefix := (prefix >= 1 && prefix <= 12) ||
		(prefix >= 21 && prefix <= 32) ||
		(prefix >= 61 && prefix <= 72) ||
		(prefix == 80)
	if !validPrefix {
		return false
	}

	sum := 3*(d[0]+d[3]+d[6]) + 7*(d[1]+d[4]+d[7]) + (d[2]+d[5]+d[8])
	return sum%10 == 0
}

// VerifyResourcePreconditions verifies that TOCTOU constraints hold prior to execution.
func VerifyResourcePreconditions(pre *ResourcePreconditions, execCtx *TrustedExecutionContext) error {
	if pre == nil {
		return nil
	}

	if pre.ExpectedArtifactSHA256 != "" && execCtx.ArtifactSHA256 != "" {
		if pre.ExpectedArtifactSHA256 != execCtx.ArtifactSHA256 {
			return fmt.Errorf("%w: artifact SHA-256 mismatch (expected %s, got %s)",
				ErrPreconditionFailed, pre.ExpectedArtifactSHA256, execCtx.ArtifactSHA256)
		}
	}

	if pre.ExpectedRowVersion > 0 && execCtx.ResourceVersion > 0 {
		if pre.ExpectedRowVersion != execCtx.ResourceVersion {
			return fmt.Errorf("%w: resource version mismatch (expected %d, got %d)",
				ErrPreconditionFailed, pre.ExpectedRowVersion, execCtx.ResourceVersion)
		}
	}

	return nil
}
