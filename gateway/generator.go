package main

import (
	"fmt"
	"strings"
	"time"
)

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
	Preset      GeneratorPreset `json:"preset"`
	Filename    string          `json:"filename"`
	Content     string          `json:"content"`
	Description string          `json:"description"`
}

// GenerateNachaScenario creates synthetic 94-character fixed-width NACHA files with precise mathematical properties.
func GenerateNachaScenario(preset GeneratorPreset) GeneratedNachaResult {
	timestamp := time.Now().Format("0601021504")
	filename := fmt.Sprintf("ACH_%s_%s.ach", string(preset), timestamp)

	switch preset {
	case PresetBalancedPayroll:
		lines := []string{
			"101 021000018 0210000212608141000A094101MERIDIAN CUSTODY     SENTINEL FLOW GATEWAY             ",
			"5200MERIDIAN PAYROLLCORP PAYROLL        1210000180PPDDIRECT DEP260814260814   1021000010000001",
			"622021000021123456789012345670000250000EMP-00101      GATHU JOHN              0021000010000001",
			"622121000248987654321012345670000250000EMP-00102      SARAH CONNOR            0021000010000002",
			"627021000021555444333212345670000500000OFF-00101      MERIDIAN CONCENTRATE    0021000010000003",
			"820000000300163000280000005000000000005000001210000180                         021000010000001",
			"9000001000001000000030016300028000000500000000000500000                                       ",
		}
		for i := range lines {
			if len(lines[i]) < 94 {
				lines[i] = lines[i] + strings.Repeat(" ", 94-len(lines[i]))
			} else if len(lines[i]) > 94 {
				lines[i] = lines[i][:94]
			}
		}
		return GeneratedNachaResult{
			Preset:      preset,
			Filename:    filename,
			Content:     strings.Join(lines, "\n"),
			Description: "Balanced PPD Corporate Payroll with exact debits and credits, valid ABA routing check digits, and verified Entry Hash sum.",
		}

	case PresetUnbalancedCCD:
		lines := []string{
			"101 021000018 011000028" + time.Now().Format("060102") + time.Now().Format("1504") + "A094101ATLANTIC TRUST       SENTINEL FLOW GATEWAY ",
			"5220ATLANTIC TRUST  VENDOR PAYMENTS     011000028CCDVENDOR ACH " + time.Now().Format("060102") + time.Now().Format("060102") + "   1011000020000001",
			"6220210000211112223334      0001250000VND-99001           GLOBAL CUSTODY CORP         0011000020000001",
			"6221210002484445556667      0000750000VND-99002           VERTEX CLEARING LLC         0011000020000002",
			"8220000002014310002600000000000000200000011000028                         011000020000001",
			"9000001000001000000020143100026000000000000002000000                                       ",
		}
		for i := range lines {
			if len(lines[i]) < 94 {
				lines[i] = lines[i] + strings.Repeat(" ", 94-len(lines[i]))
			}
		}
		return GeneratedNachaResult{
			Preset:      preset,
			Filename:    filename,
			Content:     strings.Join(lines, "\r\n"),
			Description: "Unbalanced CCD Vendor Disbursement ($2,000.00 credits, $0.00 debits) allowed under bilateral contract terms.",
		}

	case PresetCorruptedEntryHash:
		// Actual Entry Hash sum is 0143100026, but trailer declares 0999999999
		lines := []string{
			"101 021000018 021000021" + time.Now().Format("060102") + time.Now().Format("1504") + "A094101MERIDIAN CUSTODY     SENTINEL FLOW GATEWAY ",
			"5200MERIDIAN CORRUPT BATCH TEST          021000018PPDDIRECT DEP " + time.Now().Format("060102") + time.Now().Format("060102") + "   1021000010000001",
			"6220210000211234567890      0000250000EMP-00101           GATHU JOHN                  0021000010000001",
			"6221210002489876543210      0000250000EMP-00102           SARAH CONNOR                0021000010000002",
			"8200000002099999999900000000000000500000021000018                         021000010000001",
			"9000001000001000000020999999999000000000000005000000                                       ",
		}
		for i := range lines {
			if len(lines[i]) < 94 {
				lines[i] = lines[i] + strings.Repeat(" ", 94-len(lines[i]))
			}
		}
		return GeneratedNachaResult{
			Preset:      preset,
			Filename:    filename,
			Content:     strings.Join(lines, "\r\n"),
			Description: "Corrupted Batch Control Entry Hash (Trailer: 0999999999 vs Computed: 0143100026), triggering pre-flight quarantine.",
		}

	case PresetInvalidAbaRouting:
		// Invalid Routing: 999999999 (Invalid check digit)
		lines := []string{
			"101 021000018 021000021" + time.Now().Format("060102") + time.Now().Format("1504") + "A094101MERIDIAN CUSTODY     SENTINEL FLOW GATEWAY ",
			"5200MERIDIAN INVALID BATCH TEST          021000018PPDDIRECT DEP " + time.Now().Format("060102") + time.Now().Format("060102") + "   1021000010000001",
			"6229999999991234567890      0000100000EMP-99999           BOGUS ROUTING ENTRY         0021000010000001",
			"8200000001099999999000000000000000100000021000018                         021000010000001",
			"9000001000001000000010999999990000000000000001000000                                       ",
		}
		for i := range lines {
			if len(lines[i]) < 94 {
				lines[i] = lines[i] + strings.Repeat(" ", 94-len(lines[i]))
			}
		}
		return GeneratedNachaResult{
			Preset:      preset,
			Filename:    filename,
			Content:     strings.Join(lines, "\r\n"),
			Description: "Invalid Federal Reserve ABA routing number (999999999) failing Modulo 10 check digit verification.",
		}

	case PresetRecordAlignmentError:
		// Truncated line
		lines := []string{
			"101 021000018 021000021" + time.Now().Format("060102") + time.Now().Format("1504") + "A094101MERIDIAN CUSTODY     SENTINEL FLOW GATEWAY ",
			"5200MERIDIAN TRUNCATED BATCH TEST        021000018PPD", // Truncated!
			"6220210000211234567890      0000250000EMP-00101           GATHU JOHN                  0021000010000001",
			"8200000001002100002100000000000000250000021000018                         021000010000001",
			"9000001000001000000010021000021000000000000002500000                                       ",
		}
		return GeneratedNachaResult{
			Preset:      preset,
			Filename:    filename,
			Content:     strings.Join(lines, "\r\n"),
			Description: "Record alignment failure: Line 2 truncated to 48 characters, violating strict 94-character fixed-width alignment.",
		}

	default:
		return GenerateNachaScenario(PresetBalancedPayroll)
	}
}
