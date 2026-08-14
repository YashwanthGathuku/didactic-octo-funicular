package main

import (
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"time"
)

type CompliancePackage struct {
	RegulatoryStandard    string                 `json:"regulatoryStandard"`
	GeneratedAtUtc        string                 `json:"generatedAtUtc"`
	TenantIdentifier      string                 `json:"tenantIdentifier"`
	AuditEngineVersion    string                 `json:"auditEngineVersion"`
	ChainIntegrityVerified bool                  `json:"chainIntegrityVerified"`
	GenesisHash           string                 `json:"genesisHash"`
	TipHash               string                 `json:"tipHash"`
	TotalDomainEvents     int                    `json:"totalDomainEvents"`
	BundleSignatureSha256 string                 `json:"bundleSignatureSha256"`
	LedgerSummary         *LedgerSummary         `json:"ledgerSummary"`
	ValidationSummary     map[string]interface{} `json:"validationSummary"`
}

// GenerateCompliancePackage compiles an immutable regulatory audit export.
func GenerateCompliancePackage(db *sql.DB) (*CompliancePackage, error) {
	ledger, err := GetLedger(db)
	if err != nil {
		return nil, fmt.Errorf("failed to get ledger: %w", err)
	}

	genesis := "0000000000000000000000000000000000000000000000000000000000000000"
	if len(ledger.Events) > 0 {
		genesis = ledger.Events[0].PreviousHash
	}

	var totalFiles, totalQuarantined, totalIncidents int
	_ = db.QueryRow("SELECT COUNT(*) FROM file_instances").Scan(&totalFiles)
	_ = db.QueryRow("SELECT COUNT(*) FROM file_instances WHERE status = 'QUARANTINED'").Scan(&totalQuarantined)
	_ = db.QueryRow("SELECT COUNT(*) FROM incidents").Scan(&totalIncidents)

	now := time.Now().UTC().Format(time.RFC3339)
	pkg := &CompliancePackage{
		RegulatoryStandard:     "SEC Rule 17a-4 / SOX 404 / FINRA Rule 4511 Tamper-Evident Recordkeeping",
		GeneratedAtUtc:         now,
		TenantIdentifier:       "SENTINEL-TENANT-MERIDIAN-CUSTODY-001",
		AuditEngineVersion:     "Sentinel-Merkle-Chain-v1.0",
		ChainIntegrityVerified: ledger.IsChainValid,
		GenesisHash:            genesis,
		TipHash:                ledger.LastEventHash,
		TotalDomainEvents:      ledger.TotalEvents,
		LedgerSummary:          ledger,
		ValidationSummary: map[string]interface{}{
			"totalTransmissionsIngested": totalFiles,
			"quarantinedTransmissions":   totalQuarantined,
			"activeOperationalIncidents": totalIncidents,
			"preFlightEngine":           "Moov ACH + Simd94-Record-Validator",
		},
	}

	// Compute bundle signature
	pkgBytes, _ := json.Marshal(pkg)
	hasher := sha256.New()
	hasher.Write(pkgBytes)
	pkg.BundleSignatureSha256 = hex.EncodeToString(hasher.Sum(nil))

	return pkg, nil
}
