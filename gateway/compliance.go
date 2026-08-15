package main

import (
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"time"
)

// CompliancePackage is an evidence export of the application hash chain and
// the ingestion counts derived from it.
//
// It deliberately carries NO regulatory claim. The previous version asserted
// "SEC Rule 17a-4 / SOX 404 / FINRA Rule 4511 Tamper-Evident Recordkeeping",
// named the engine "Sentinel-Merkle-Chain-v1.0", and described the validator as
// SIMD-accelerated. None of those were true: the structure is a linear hash
// chain with no external anchor, and a regulatory standard is met by an audited
// programme, not by a string constant in an export. See SCOPE.md §3 and §4.
type CompliancePackage struct {
	ExportType             string                 `json:"exportType"`
	GeneratedAtUtc         string                 `json:"generatedAtUtc"`
	EvidenceEngineVersion  string                 `json:"evidenceEngineVersion"`
	ChainIntegrityVerified bool                   `json:"chainIntegrityVerified"`
	GenesisHash            string                 `json:"genesisHash"`
	TipHash                string                 `json:"tipHash"`
	TotalDomainEvents      int                    `json:"totalDomainEvents"`
	BundleDigestSha256     string                 `json:"bundleDigestSha256"`
	Limitations            []string               `json:"limitations"`
	LedgerSummary          *LedgerSummary         `json:"ledgerSummary"`
	ValidationSummary      map[string]interface{} `json:"validationSummary"`
}

// GenerateCompliancePackage compiles an evidence export of the audit chain.
func GenerateCompliancePackage(db *sql.DB, tenantID string) (*CompliancePackage, error) {
	ledger, err := GetLedger(db, tenantID)
	if err != nil {
		return nil, fmt.Errorf("failed to get ledger: %w", err)
	}

	genesis := "0000000000000000000000000000000000000000000000000000000000000000"
	if len(ledger.Events) > 0 {
		genesis = ledger.Events[0].PreviousHash
	}

	var totalFiles, totalQuarantined, totalIncidents int
	_ = db.QueryRow("SELECT COUNT(*) FROM file_instances WHERE tenant_id = ?", tenantID).Scan(&totalFiles)
	_ = db.QueryRow("SELECT COUNT(*) FROM file_instances WHERE tenant_id = ? AND status = 'QUARANTINED'", tenantID).Scan(&totalQuarantined)
	_ = db.QueryRow("SELECT COUNT(*) FROM incidents WHERE tenant_id = ?", tenantID).Scan(&totalIncidents)

	now := time.Now().UTC().Format(time.RFC3339)
	pkg := &CompliancePackage{
		ExportType:             "APPLICATION_HASH_CHAIN_EVIDENCE_EXPORT",
		GeneratedAtUtc:         now,
		EvidenceEngineVersion:  "sentinel-application-hash-chain-v1",
		ChainIntegrityVerified: ledger.IsChainValid,
		GenesisHash:            genesis,
		TipHash:                ledger.LastEventHash,
		TotalDomainEvents:      ledger.TotalEvents,
		Limitations: []string{
			"This is a linear application hash chain, not a Merkle history tree. It provides no membership or consistency proofs.",
			"The chain has no external anchor or signed checkpoint, so it cannot prove absence of wholesale replacement by an actor with database write access.",
			"Chain append is not serialised; concurrent writers can fork the chain. Tracked for Prompt 09.",
			"Records are not tenant-scoped. No tenant boundary exists in this schema.",
			"This export asserts no regulatory or compliance status.",
		},
		LedgerSummary: ledger,
		ValidationSummary: map[string]interface{}{
			"totalTransmissionsIngested": totalFiles,
			"quarantinedTransmissions":   totalQuarantined,
			"activeOperationalIncidents": totalIncidents,
			"preFlightEngine":            "moov-io/ach + fixed-width record checks",
		},
	}

	// Compute a digest over the bundle.
	//
	// This is a SHA-256 digest, NOT a digital signature: it binds nothing to an
	// identity and anyone who can alter the bundle can recompute it. It detects
	// accidental corruption in transit, nothing more. Named accordingly.
	pkgBytes, _ := json.Marshal(pkg)
	hasher := sha256.New()
	hasher.Write(pkgBytes)
	pkg.BundleDigestSha256 = hex.EncodeToString(hasher.Sum(nil))

	return pkg, nil
}
