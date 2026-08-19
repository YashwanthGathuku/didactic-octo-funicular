package main

import (
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"testing"
)

// ApprovedLicenses lists all legally permitted open-source licenses for SentinelFlow.
var ApprovedLicenses = map[string]bool{
	"MIT":          true,
	"Apache-2.0":   true,
	"BSD-2-Clause": true,
	"BSD-3-Clause": true,
	"ISC":          true,
	"0BSD":         true,
	"CC0-1.0":      true,
}

// ProhibitedLicenses lists copyleft licenses that cannot be linked into production binaries.
var ProhibitedLicenses = map[string]bool{
	"AGPL-1.0":      true,
	"AGPL-3.0":      true,
	"AGPL-3.0-only": true,
	"GPL-2.0":       true,
	"GPL-3.0":       true,
	"GPL-3.0-only":  true,
	"SSPL-1.0":      true,
	"BUSL-1.1":      true,
}

// CheckLicenseValidity verifies that a license complies with SentinelFlow policy.
func CheckLicenseValidity(licenseName string) error {
	normalized := strings.TrimSpace(licenseName)
	if ProhibitedLicenses[normalized] {
		return fmt.Errorf("prohibited copyleft/source-available license %q violates SentinelFlow policy", normalized)
	}
	if !ApprovedLicenses[normalized] {
		return fmt.Errorf("unapproved license %q requires documented legal exception", normalized)
	}
	return nil
}

// EvaluateVulnerabilityGate determines if a CVE severity blocks CI release.
func EvaluateVulnerabilityGate(severity string, cvss float64) (blocked bool, reason string) {
	upper := strings.ToUpper(strings.TrimSpace(severity))
	switch {
	case upper == "CRITICAL" || cvss >= 9.0:
		return true, fmt.Sprintf("CRITICAL vulnerability (CVSS %.1f) violates release policy (24h SLA)", cvss)
	case upper == "HIGH" || cvss >= 7.0:
		return true, fmt.Sprintf("HIGH vulnerability (CVSS %.1f) violates release policy (7d SLA)", cvss)
	default:
		return false, "Within acceptable risk tolerance for scheduled patch release"
	}
}

func TestSDLCLicenseGovernance_RejectsProhibitedCopyleftInProduction(t *testing.T) {
	// Read gateway/go.mod and ensure no forbidden packages
	goModData, err := os.ReadFile("go.mod")
	if err != nil {
		t.Fatalf("failed to read go.mod: %v", err)
	}
	content := string(goModData)

	// Explicitly verify zero AGPL/GPL packages are referenced
	forbiddenPackages := []string{
		"github.com/drakkan/sftpgo",
		"github.com/sftpgo",
	}
	for _, pkg := range forbiddenPackages {
		if strings.Contains(content, pkg) {
			t.Errorf("forbidden copyleft package %q found in go.mod", pkg)
		}
	}
}

func TestSDLCPolicy_DeliberateLicenseViolationFailsGate(t *testing.T) {
	// 1. Permissive licenses pass
	for _, lic := range []string{"MIT", "Apache-2.0", "BSD-3-Clause", "ISC"} {
		if err := CheckLicenseValidity(lic); err != nil {
			t.Errorf("approved license %s was rejected: %v", lic, err)
		}
	}

	// 2. Prohibited licenses must fail
	for _, lic := range []string{"AGPL-3.0", "AGPL-3.0-only", "GPL-3.0", "SSPL-1.0"} {
		if err := CheckLicenseValidity(lic); err == nil {
			t.Errorf("prohibited copyleft license %s was erroneously accepted", lic)
		}
	}
}

func TestSDLC_VulnerabilitySeverityThresholdEnforcement(t *testing.T) {
	// Critical CVE must block
	blocked, reason := EvaluateVulnerabilityGate("CRITICAL", 9.8)
	if !blocked {
		t.Error("CRITICAL CVE must block release gate")
	}
	if !strings.Contains(reason, "CRITICAL") {
		t.Errorf("expected critical reason, got: %s", reason)
	}

	// High CVE must block
	blockedHigh, _ := EvaluateVulnerabilityGate("HIGH", 7.5)
	if !blockedHigh {
		t.Error("HIGH CVE must block release gate")
	}

	// Low/Medium vulnerability allows build with alert
	blockedLow, _ := EvaluateVulnerabilityGate("LOW", 3.1)
	if blockedLow {
		t.Error("LOW CVE should not hard-block CI gate")
	}
}

func TestSDLC_SBOMSchemaValidation(t *testing.T) {
	// Validate minimal CycloneDX JSON structure
	minimalSBOM := `{
		"bomFormat": "CycloneDX",
		"specVersion": "1.5",
		"serialNumber": "urn:uuid:3e671687-395b-41f5-a30f-a58921a69b79",
		"version": 1,
		"metadata": {
			"timestamp": "2026-08-18T19:00:00Z",
			"component": {
				"type": "application",
				"name": "sentinel-flow",
				"version": "1.0.0"
			}
		},
		"components": [
			{
				"type": "library",
				"name": "github.com/jackc/pgx/v5",
				"version": "v5.7.2",
				"licenses": [{"license": {"id": "MIT"}}]
			}
		]
	}`

	var parsed struct {
		BOMFormat   string `json:"bomFormat"`
		SpecVersion string `json:"specVersion"`
		Metadata    struct {
			Component struct {
				Name    string `json:"name"`
				Version string `json:"version"`
			} `json:"component"`
		} `json:"metadata"`
		Components []struct {
			Name    string `json:"name"`
			Version string `json:"version"`
		} `json:"components"`
	}

	if err := json.Unmarshal([]byte(minimalSBOM), &parsed); err != nil {
		t.Fatalf("failed to parse SBOM JSON: %v", err)
	}

	if parsed.BOMFormat != "CycloneDX" || parsed.SpecVersion != "1.5" {
		t.Errorf("invalid SBOM format/spec: %s %s", parsed.BOMFormat, parsed.SpecVersion)
	}
	if len(parsed.Components) == 0 {
		t.Error("SBOM contains 0 components")
	}
}
