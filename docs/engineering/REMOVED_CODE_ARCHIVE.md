# Removed Code Archive — Backend (Go)

**Created by:** Prompt 01 (Truth reset and scope reduction)
**Source commit:** `cb09694`
**Date:** 14 August 2026

## What this file is

A verbatim, **non-executable** archive of every Go source file and route block deleted from the
gateway during Prompt 01. It exists so the removed work is readable without a git checkout.

- Nothing here is compiled, imported, routed, or reachable at runtime.
- This is a reference document, not a feature flag and not a disabled code path.
- The authoritative recovery mechanism is still git history at commit `cb09694`.

## Why these were removed

Each entry states its reason. In summary, every file below either returned a security,
compliance, settlement, or performance result that was a source constant rather than a
measurement or verified runtime fact, or exposed a control the product cannot yet honour
safely. The evidence for each is in `docs/engineering/CURRENT_STATE.md`.

## Where the good parts go

Several removed files contain patterns worth reusing. They are called out in
`docs/engineering/SCOPE.md` and are scheduled for reimplementation:

| Pattern | Removed from | Reimplemented in |
|---|---|---|
| Constant-time credential check, fails closed, logs denials | `vault.go` `authorizeDetokenize` | Prompt 04 (authz), Prompt 05 (SecretStore) |
| HMAC-SHA256 payload signing | `webhook.go` `computeSignature` | Prompt 05 (signed notification delivery) |
| Secret-reference indirection (never inline the secret) | `connector.go` `SecretReference` | Prompt 05 (SecretStore) |
| Honest demo labelling (`IsScriptedDemo`, `*Target`, `NOT_PROVISIONED`) | `failover.go` | Vocabulary rule in SCOPE.md |
| Capability/health/classification modelling for connectors | `connector.go` | Prompt 16 (connector platform) |

---

## `gateway/connector.go`

**Lines:** 398  ·  **Reason for removal:** Integration Hub was an in-memory struct literal presented as live connectivity; `/hub/edge/sync` returned `mTLSVerified: true` over plain HTTP with no client certificate (runtime-reproduced, CURRENT_STATE.md §6).

```go
package main

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"net/http"
	"sync"
	"time"

	"github.com/go-chi/chi/v5"
)

// Classification levels for discovered assets
type DataClassification string

const (
	ClassPublic       DataClassification = "PUBLIC"
	ClassInternal     DataClassification = "INTERNAL"
	ClassConfidential DataClassification = "CONFIDENTIAL"
	ClassRestricted   DataClassification = "RESTRICTED"
)

// ConnectionType represents supported connector protocols
type ConnectionType string

const (
	ConnSFTP       ConnectionType = "SFTP_SSH"
	ConnPostgres   ConnectionType = "POSTGRESQL"
	ConnRestAPI    ConnectionType = "REST_API"
	ConnSMBShared  ConnectionType = "SMB_NFS"
	ConnObjectS3   ConnectionType = "S3_OBJECT"
)

// SecretReference represents an OWASP-compliant decoupled credential pointer
type SecretReference struct {
	VaultKey     string    `json:"vaultKey"`     // e.g. "vault://secret/meridian/sftp-key"
	SecretType   string    `json:"secretType"`   // e.g. "SSH_KEY_ED25519", "DATABASE_PASSWORD"
	RotatedAt    time.Time `json:"rotatedAt"`
	SecretAgeDays int      `json:"secretAgeDays"`
	Status       string    `json:"status"`       // "CURRENT", "EXPIRING_SOON", "ROTATION_REQUIRED"
}

// Connection represents a customer edge connection endpoint
type Connection struct {
	ID                 string          `json:"id"`
	Name               string          `json:"name"`
	Type               ConnectionType  `json:"type"`
	HostAlias          string          `json:"hostAlias"` // Hostname alias (never internal IP if private)
	Port               int             `json:"port"`
	ServiceAccountUser string          `json:"serviceAccountUser"`
	AuthMethod         string          `json:"authMethod"` // "SSH_PUBLIC_KEY", "mTLS_CERTIFICATE", "OAUTH2_TOKEN"
	HostKeyFingerprint string          `json:"hostKeyFingerprint,omitempty"`
	PermittedPaths     []string        `json:"permittedPaths,omitempty"`
	DatabaseName       string          `json:"databaseName,omitempty"`
	SecretRef          SecretReference `json:"secretRef"` // Strictly sanitized: NEVER contains raw secrets
	Status             string          `json:"status"`    // "HEALTHY", "DEGRADED", "FAILED"
	LastSyncAt         time.Time       `json:"lastSyncAt"`
	ConnectorLatencyMs float64         `json:"connectorLatencyMs"`
	EdgeAgentID        string          `json:"edgeAgentId"`
}

// AssetField represents schema column details
type AssetField struct {
	Name        string `json:"name"`
	DataType    string `json:"dataType"`
	IsNullable  bool   `json:"isNullable"`
	IsMasked    bool   `json:"isMasked"`
	Description string `json:"description,omitempty"`
}

// CatalogAsset represents a normalized data resource across DB, SFTP, API, or Object storage
type CatalogAsset struct {
	ID               string             `json:"id"`
	ConnectionID     string             `json:"connectionId"`
	ConnectionName   string             `json:"connectionName"`
	AssetType        string             `json:"assetType"` // "FILE_DIRECTORY", "DATABASE_TABLE", "API_ENDPOINT", "OBJECT_PREFIX"
	LogicalPath      string             `json:"logicalPath"`
	QualifiedName    string             `json:"qualifiedName"`
	Classification   DataClassification `json:"classification"`
	Owner            string             `json:"owner"`
	Fields           []AssetField       `json:"fields"`
	RowCount         int64              `json:"rowCount"`
	ByteSize         int64              `json:"byteSize"`
	FreshnessSlaMin  int                `json:"freshnessSlaMin"`
	LastObservedAt   time.Time          `json:"lastObservedAt"`
	ValidationStatus string             `json:"validationStatus"` // "COMPLIANT", "DEVIATION_DETECTED"
}

// DataLineageEdge represents directional pipeline dependencies
type DataLineageEdge struct {
	ID             string `json:"id"`
	SourceAssetID  string `json:"sourceAssetId"`
	SourceName     string `json:"sourceName"`
	SourceType     string `json:"sourceType"`
	TargetAssetID  string `json:"targetAssetId"`
	TargetName     string `json:"targetName"`
	TargetType     string `json:"targetType"`
	Transformation string `json:"transformation"` // e.g. "Deterministic NACHA SIMD Parse", "DB ETL Sync"
}

// MaskedSampleRecord represents bounded, privacy-safe data preview
type MaskedSampleRecord struct {
	AssetID      string                   `json:"assetId"`
	QualifiedName string                  `json:"qualifiedName"`
	PreviewRows  []map[string]interface{} `json:"previewRows"`
	TotalRows    int64                    `json:"totalRows"`
	MaskedFields []string                 `json:"maskedFields"`
	AuditLogID   string                   `json:"auditLogId"`
}

// In-Memory Catalog Store with Institutional Seed Data
type CatalogStore struct {
	mu          sync.RWMutex
	Connections []Connection
	Assets      []CatalogAsset
	Lineage     []DataLineageEdge
}

var GlobalCatalog = &CatalogStore{
	Connections: []Connection{
		{
			ID:                 "CONN-SFTP-MERIDIAN",
			Name:               "Meridian Treasury SFTP Drop",
			Type:               ConnSFTP,
			HostAlias:          "sftp-prod-us-east.meridian.internal",
			Port:               2222,
			ServiceAccountUser: "svc_sentinel_ingest",
			AuthMethod:         "SSH_ED25519_KEY",
			HostKeyFingerprint: "SHA256:4a3F9cKz8eL1mQxWpTvBn9Rt6sDuFhJ2kLm8qWeRt7y",
			PermittedPaths:     []string{"/inbound/ach/", "/inbound/iso20022/"},
			SecretRef: SecretReference{
				VaultKey:      "vault://prod/treasury/sftp_agent_ed25519",
				SecretType:    "SSH_PRIVATE_KEY_POINTER",
				RotatedAt:     time.Now().Add(-48 * time.Hour),
				SecretAgeDays: 2,
				Status:        "CURRENT",
			},
			Status:             "HEALTHY",
			LastSyncAt:         time.Now().Add(-1 * time.Minute),
			ConnectorLatencyMs: 14.2,
			EdgeAgentID:        "EDGE-AGENT-MERIDIAN-VPC-01",
		},
		{
			ID:                 "CONN-PG-TREASURY",
			Name:               "Core Treasury PostgreSQL Cluster",
			Type:               ConnPostgres,
			HostAlias:          "pg-aurora-cluster.meridian.internal",
			Port:               5432,
			ServiceAccountUser: "svc_sentinel_readonly",
			AuthMethod:         "mTLS_X509_CERTIFICATE",
			DatabaseName:       "treasury_settlement_db",
			PermittedPaths:     []string{"public.settlement_batches", "public.counterparty_ledgers"},
			SecretRef: SecretReference{
				VaultKey:      "vault://prod/postgres/tls_cert_bundle",
				SecretType:    "mTLS_CERTIFICATE_POINTER",
				RotatedAt:     time.Now().Add(-120 * time.Hour),
				SecretAgeDays: 5,
				Status:        "CURRENT",
			},
			Status:             "HEALTHY",
			LastSyncAt:         time.Now().Add(-30 * time.Second),
			ConnectorLatencyMs: 3.8,
			EdgeAgentID:        "EDGE-AGENT-MERIDIAN-VPC-01",
		},
		{
			ID:                 "CONN-REST-SETTLEMENT",
			Name:               "Apex Clearing Settlement REST Gateway",
			Type:               ConnRestAPI,
			HostAlias:          "api.apexclearing.internal/v2",
			Port:               443,
			ServiceAccountUser: "svc_sentinel_settlement_oauth",
			AuthMethod:         "OAUTH2_MUTUAL_TLS",
			SecretRef: SecretReference{
				VaultKey:      "vault://prod/apex/oauth_client_secret",
				SecretType:    "OAUTH_SECRET_POINTER",
				RotatedAt:     time.Now().Add(-200 * time.Hour),
				SecretAgeDays: 8,
				Status:        "CURRENT",
			},
			Status:             "HEALTHY",
			LastSyncAt:         time.Now().Add(-2 * time.Minute),
			ConnectorLatencyMs: 24.5,
			EdgeAgentID:        "EDGE-AGENT-MERIDIAN-VPC-01",
		},
		{
			ID:                 "CONN-S3-ARCHIVE",
			Name:               "Immutable SEC 17a-4 S3 WORM Archive",
			Type:               ConnObjectS3,
			HostAlias:          "s3.us-east-1.amazonaws.com/meridian-sec17a4-worm",
			Port:               443,
			ServiceAccountUser: "arn:aws:iam::123456789012:role/SentinelEvidenceArchiver",
			AuthMethod:         "IAM_INSTANCE_PROFILE",
			PermittedPaths:     []string{"worm-vault/ach/2026/", "worm-vault/compliance/"},
			SecretRef: SecretReference{
				VaultKey:      "aws://iam/role/SentinelEvidenceArchiver",
				SecretType:    "IAM_ASSUME_ROLE",
				RotatedAt:     time.Now().Add(-12 * time.Hour),
				SecretAgeDays: 1,
				Status:        "CURRENT",
			},
			Status:             "HEALTHY",
			LastSyncAt:         time.Now().Add(-10 * time.Second),
			ConnectorLatencyMs: 38.1,
			EdgeAgentID:        "EDGE-AGENT-MERIDIAN-VPC-01",
		},
	},
	Assets: []CatalogAsset{
		{
			ID:               "ASSET-001",
			ConnectionID:     "CONN-SFTP-MERIDIAN",
			ConnectionName:   "Meridian Treasury SFTP Drop",
			AssetType:        "FILE_DIRECTORY",
			LogicalPath:      "/inbound/ach/commercial_payroll/",
			QualifiedName:    "sftp://meridian.internal/inbound/ach/*.ach",
			Classification:   ClassRestricted,
			Owner:            "Treasury Operations Team",
			Fields: []AssetField{
				{Name: "RecordType", DataType: "CHAR(1)", IsNullable: false, IsMasked: false, Description: "1=FileHeader, 5=BatchHeader, 6=EntryDetail, 8=BatchControl"},
				{Name: "RoutingNumber", DataType: "CHAR(9)", IsNullable: false, IsMasked: true, Description: "Receiving DFI Federal Reserve Routing Prefix"},
				{Name: "AccountNumber", DataType: "VARCHAR(17)", IsNullable: false, IsMasked: true, Description: "Counterparty Deposit Account"},
				{Name: "AmountInCents", DataType: "NUMERIC(10,0)", IsNullable: false, IsMasked: false, Description: "Transaction value in cents"},
			},
			RowCount:         10500,
			ByteSize:         987000,
			FreshnessSlaMin:  60,
			LastObservedAt:   time.Now().Add(-5 * time.Minute),
			ValidationStatus: "COMPLIANT",
		},
		{
			ID:               "ASSET-002",
			ConnectionID:     "CONN-PG-TREASURY",
			ConnectionName:   "Core Treasury PostgreSQL Cluster",
			AssetType:        "DATABASE_TABLE",
			LogicalPath:      "public.settlement_batches",
			QualifiedName:    "treasury_settlement_db.public.settlement_batches",
			Classification:   ClassConfidential,
			Owner:            "Settlement Platform Engineering",
			Fields: []AssetField{
				{Name: "batch_id", DataType: "UUID", IsNullable: false, IsMasked: false},
				{Name: "file_hash", DataType: "VARCHAR(64)", IsNullable: false, IsMasked: false},
				{Name: "total_credits", DataType: "NUMERIC(18,2)", IsNullable: false, IsMasked: false},
				{Name: "total_debits", DataType: "NUMERIC(18,2)", IsNullable: false, IsMasked: false},
				{Name: "settlement_date", DataType: "DATE", IsNullable: false, IsMasked: false},
				{Name: "status", DataType: "VARCHAR(20)", IsNullable: false, IsMasked: false},
			},
			RowCount:         482900,
			ByteSize:         142000000,
			FreshnessSlaMin:  120,
			LastObservedAt:   time.Now().Add(-1 * time.Minute),
			ValidationStatus: "COMPLIANT",
		},
		{
			ID:               "ASSET-003",
			ConnectionID:     "CONN-REST-SETTLEMENT",
			ConnectionName:   "Apex Clearing Settlement REST Gateway",
			AssetType:        "API_ENDPOINT",
			LogicalPath:      "POST /v2/settlements/reconcile",
			QualifiedName:    "api.apexclearing.internal/v2/settlements/reconcile",
			Classification:   ClassRestricted,
			Owner:            "Apex Interbank Settlement Service",
			Fields: []AssetField{
				{Name: "settlementId", DataType: "STRING", IsNullable: false, IsMasked: false},
				{Name: "counterpartyRouting", DataType: "STRING", IsNullable: false, IsMasked: true},
				{Name: "acknowledgedAmount", DataType: "FLOAT64", IsNullable: false, IsMasked: false},
			},
			RowCount:         1420,
			ByteSize:         85000,
			FreshnessSlaMin:  30,
			LastObservedAt:   time.Now().Add(-2 * time.Minute),
			ValidationStatus: "COMPLIANT",
		},
	},
	Lineage: []DataLineageEdge{
		{
			ID:             "LIN-001",
			SourceAssetID:  "ASSET-001",
			SourceName:     "Meridian Inbound SFTP (/inbound/ach/)",
			SourceType:     "SFTP_FILE",
			TargetAssetID:  "ASSET-002",
			TargetName:     "PostgreSQL (public.settlement_batches)",
			TargetType:     "DATABASE_TABLE",
			Transformation: "Deterministic Streaming NACHA 2025 Validation + Merkle Ledger Hash Commitment",
		},
		{
			ID:             "LIN-002",
			SourceAssetID:  "ASSET-002",
			SourceName:     "PostgreSQL (public.settlement_batches)",
			SourceType:     "DATABASE_TABLE",
			TargetAssetID:  "ASSET-003",
			TargetName:     "Apex Clearing REST API (/v2/settlements/reconcile)",
			TargetType:     "REST_API",
			Transformation: "Signed Outbound Webhook Pub/Sub Notification & Interbank Reconciliation",
		},
	},
}

// RegisterIntegrationHubRoutes configures all REST API endpoints for the Integration Hub.
func RegisterIntegrationHubRoutes(r chi.Router, db *sql.DB) {
	r.Route("/hub", func(r chi.Router) {
		// GET /api/v1/hub/connections
		r.Get("/connections", func(w http.ResponseWriter, r *http.Request) {
			GlobalCatalog.mu.RLock()
			defer GlobalCatalog.mu.RUnlock()
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(GlobalCatalog.Connections)
		})

		// GET /api/v1/hub/assets
		r.Get("/assets", func(w http.ResponseWriter, r *http.Request) {
			GlobalCatalog.mu.RLock()
			defer GlobalCatalog.mu.RUnlock()
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(GlobalCatalog.Assets)
		})

		// GET /api/v1/hub/assets/{id}/sample (Restricted PII-Masked Preview)
		r.Get("/assets/{id}/sample", func(w http.ResponseWriter, r *http.Request) {
			assetID := chi.URLParam(r, "id")
			GlobalCatalog.mu.RLock()
			defer GlobalCatalog.mu.RUnlock()

			var target *CatalogAsset
			for _, a := range GlobalCatalog.Assets {
				if a.ID == assetID {
					target = &a
					break
				}
			}

			if target == nil {
				http.Error(w, "asset not found", http.StatusNotFound)
				return
			}

			// Generate privacy-safe masked preview rows (Max 3 rows, masked PII)
			sample := MaskedSampleRecord{
				AssetID:       target.ID,
				QualifiedName: target.QualifiedName,
				MaskedFields:  []string{"RoutingNumber", "AccountNumber", "counterpartyRouting"},
				TotalRows:     target.RowCount,
				AuditLogID:    fmt.Sprintf("AUDIT-PREVIEW-%d", time.Now().UnixNano()),
				PreviewRows: []map[string]interface{}{
					{
						"row_id":        1,
						"RoutingNumber": "0210****8 (MASKED)",
						"AccountNumber": "********1842 (MASKED)",
						"AmountInCents": 245000,
						"Name":          "J*** D** (REDACTED)",
					},
					{
						"row_id":        2,
						"RoutingNumber": "0210****1 (MASKED)",
						"AccountNumber": "********9901 (MASKED)",
						"AmountInCents": 182000,
						"Name":          "A*** S**** (REDACTED)",
					},
				},
			}

			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(sample)
		})

		// GET /api/v1/hub/lineage
		r.Get("/lineage", func(w http.ResponseWriter, r *http.Request) {
			GlobalCatalog.mu.RLock()
			defer GlobalCatalog.mu.RUnlock()
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(map[string]interface{}{
				"nodes":   GlobalCatalog.Assets,
				"edges":   GlobalCatalog.Lineage,
				"version": "1.0-DAG",
			})
		})

		// POST /api/v1/hub/edge/sync (Edge Agent Outbound Metadata Heartbeat)
		r.Post("/edge/sync", func(w http.ResponseWriter, r *http.Request) {
			var body struct {
				EdgeAgentID string `json:"edgeAgentId"`
				Hostname    string `json:"hostname"`
				Status      string `json:"status"`
			}
			if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
				http.Error(w, "invalid heartbeat", http.StatusBadRequest)
				return
			}

			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(map[string]interface{}{
				"status":         "ACKNOWLEDGED",
				"edgeAgentId":    body.EdgeAgentID,
				"synchronizedAt": time.Now().UTC().Format(time.RFC3339),
				"mTLSVerified":   true,
			})
		})
	})
}
```

---

## `gateway/instant_payment.go`

**Lines:** 115  ·  **Reason for removal:** Returned `SETTLED_INSTANT`, `isCompliant: true`, an invented $150,000 amount and fixed 1.42ms / 99.998% / 12,500 TPS metrics without parsing the payload. Settlement is not a state in this product.

```go
package main

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
)

// InstantPaymentType represents FedNow or RTP networks
type InstantPaymentNetwork string

const (
	NetFedNow InstantPaymentNetwork = "FEDNOW"
	NetRTP    InstantPaymentNetwork = "THE_CLEARING_HOUSE_RTP"
)

// InstantPaymentTransaction represents a real-time ISO 20022 payment payload
type InstantPaymentTransaction struct {
	Network             InstantPaymentNetwork `json:"network"`
	MessageType         string                `json:"messageType"` // "pacs.008.001.08", "pacs.002.001.10"
	EndToEndID          string                `json:"endToEndId"`
	InstructionID       string                `json:"instructionId"`
	AmountUSD           float64               `json:"amountUsd"`
	DebtorAgentRouting  string                `json:"debtorAgentRouting"`
	CreditorAgentRouting string               `json:"creditorAgentRouting"`
	Status              string                `json:"status"` // "SETTLED_INSTANT", "QUARANTINED", "REJECTED_TIMEOUT"
	ValidationLatencyMs float64               `json:"validationLatencyMs"`
	SlaThresholdMs      float64               `json:"slaThresholdMs"`
	Timestamp           time.Time             `json:"timestamp"`
}

// ValidateInstantPaymentXml parses and validates instant ISO 20022 XML messages
func ValidateInstantPaymentXml(content string) (*InstantPaymentTransaction, []string) {
	start := time.Now()
	var findings []string

	network := NetFedNow
	if strings.Contains(content, "RTP") || strings.Contains(content, "ClearingHouse") {
		network = NetRTP
	}

	msgType := "pacs.008.001.08"
	if strings.Contains(content, "pacs.002") {
		msgType = "pacs.002.001.10"
	}

	tx := &InstantPaymentTransaction{
		Network:             network,
		MessageType:         msgType,
		EndToEndID:          fmt.Sprintf("FEDNOW-E2E-%d", time.Now().UnixNano()),
		InstructionID:       fmt.Sprintf("INSTR-INST-%d", time.Now().UnixNano()),
		AmountUSD:           150000.00,
		DebtorAgentRouting:  "021000021",
		CreditorAgentRouting: "121000358",
		Status:              "SETTLED_INSTANT",
		SlaThresholdMs:      2500.0, // FedNow 2.5s maximum settlement window
		Timestamp:           time.Now().UTC(),
	}

	// Verify Federal Reserve Mod10 routing
	if !ValidateRoutingMod10(tx.DebtorAgentRouting) {
		findings = append(findings, "Debtor routing ABA 021000021 failed Federal Reserve Mod10 validation.")
		tx.Status = "QUARANTINED"
	}

	tx.ValidationLatencyMs = float64(time.Since(start).Microseconds()) / 1000.0
	if tx.ValidationLatencyMs > tx.SlaThresholdMs {
		tx.Status = "REJECTED_TIMEOUT"
		findings = append(findings, "Validation latency exceeded 2,500ms Instant Payment SLA.")
	}

	return tx, findings
}

// RegisterInstantPaymentRoutes wires FedNow/RTP endpoints into Chi router
func RegisterInstantPaymentRoutes(r chi.Router, db *sql.DB) {
	r.Route("/instant-payments", func(r chi.Router) {
		// POST /api/v1/instant-payments/validate
		r.Post("/validate", func(w http.ResponseWriter, r *http.Request) {
			var body struct {
				PayloadXML string `json:"payloadXml"`
			}
			if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
				http.Error(w, "invalid request body", http.StatusBadRequest)
				return
			}

			tx, findings := ValidateInstantPaymentXml(body.PayloadXML)

			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(map[string]interface{}{
				"transaction":   tx,
				"findings":      findings,
				"isCompliant":   len(findings) == 0,
				"instantSlaMet": tx.ValidationLatencyMs <= tx.SlaThresholdMs,
			})
		})

		// GET /api/v1/instant-payments/metrics
		r.Get("/metrics", func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(map[string]interface{}{
				"supportedNetworks":        []string{"FEDNOW_SERVICE", "THE_CLEARING_HOUSE_RTP"},
				"averageValidationLatency": "1.42 ms",
				"slaComplianceRate":        "99.998%",
				"maxThroughputTps":         12500,
			})
		})
	})
}
```

---

## `gateway/vault.go`

**Lines:** 380  ·  **Reason for removal:** Tokenisation is not part of the v1 promise. Removed whole rather than left as an unused API surface that Prompt 05 will rewrite. `authorizeDetokenize` was the strongest control in the codebase and its pattern is carried forward.

```go
package main

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"database/sql"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/go-chi/chi/v5"
)

// ---------------------------------------------------------------------------
// SECURITY REMEDIATION 2026-08-14
//
// Previous implementation had four defects, each individually a full break:
//
//  1. HARDCODED KEY. The HMAC secret was the literal string
//     "SENTINEL_FIPS140_HMAC_SECRET" compiled into the binary. Because the
//     tokenizer is deterministic, anyone with the source can precompute tokens
//     for the entire ABA routing space (~10^7 assigned) or the SSN space
//     (10^9) in seconds and invert every token. Keyed-hash tokenization is only
//     as strong as the key's secrecy over a low-entropy domain.
//
//  2. PLAINTEXT "VAULT". Tokens map[string]string stored the RAW value in
//     memory, unencrypted. The vault was a plaintext dictionary.
//
//  3. NO AUTHENTICATION ON DETOKENIZE. The only checks were
//     SupervisorID != "" and len(AuditReason) >= 10. AuthCertDigest was parsed
//     and never used. RequireApproval in the policy was never read. Any caller
//     could retrieve plaintext PII.
//
//  4. FALSE AUDIT CLAIM. The response asserted "auditLogged": true while never
//     writing to the audit ledger -- the worst possible failure for a product
//     whose value proposition is SEC 17a-4 evidentiary proof.
//
// Also fixed: fieldType[0:3] panicked on any fieldType shorter than 3 chars.
//
// NOTE ON LABELLING: the policy Algorithm field previously advertised
// "FPE_AES256" while the code did substring masking + HMAC. That is not
// format-preserving encryption. Real FPE requires FF1 or FF3-1 per NIST
// SP 800-38G. The label is now honest about what is actually implemented.
// FIPS 140 is a validation of a cryptographic module by an accredited lab --
// it cannot be asserted in a struct tag and that claim has been removed.
// ---------------------------------------------------------------------------

type TokenizationPolicy struct {
	TenantID        string   `json:"tenantId"`
	MaskedFields    []string `json:"maskedFields"`
	Algorithm       string   `json:"algorithm"`
	RetentionDays   int      `json:"retentionDays"`
	RequireApproval bool     `json:"requireApprovalForDetokenize"`
}

type TokenizedRecord struct {
	OriginalType string    `json:"originalType"`
	MaskedValue  string    `json:"maskedValue"`
	TokenKey     string    `json:"tokenKey"`
	TenantID     string    `json:"tenantId"`
	CreatedAt    time.Time `json:"createdAt"`
}

type vaultEntry struct {
	ciphertext []byte
	expiresAt  time.Time
}

type TokenVaultStore struct {
	mu       sync.RWMutex
	entries  map[string]vaultEntry
	Policies []TokenizationPolicy
}

var GlobalVault = &TokenVaultStore{
	entries: make(map[string]vaultEntry),
	Policies: []TokenizationPolicy{
		{
			TenantID:        "TENANT-MERIDIAN-PROD",
			MaskedFields:    []string{"RoutingNumber", "AccountNumber", "TaxIdentifier", "IndividualName"},
			Algorithm:       "HMAC_SHA256_TOKEN + AES-256-GCM_AT_REST",
			RetentionDays:   2555, // 7 years, SEC 17a-4(f)
			RequireApproval: true,
		},
		{
			TenantID:        "TENANT-APEX-GLOBAL",
			MaskedFields:    []string{"BeneficiaryAccount", "CounterpartyRouting"},
			Algorithm:       "HMAC_SHA256_TOKEN + AES-256-GCM_AT_REST",
			RetentionDays:   2555,
			RequireApproval: true,
		},
	},
}

var (
	vaultKeyOnce sync.Once
	vaultHMACKey []byte
	vaultAEADKey []byte
	vaultKeyErr  error
)

// loadVaultKeys reads key material from the environment. There is no default:
// if SENTINEL_VAULT_HMAC_KEY / SENTINEL_VAULT_AES_KEY are unset the vault
// refuses to operate rather than silently falling back to a known constant.
func loadVaultKeys() ([]byte, []byte, error) {
	vaultKeyOnce.Do(func() {
		h := os.Getenv("SENTINEL_VAULT_HMAC_KEY")
		a := os.Getenv("SENTINEL_VAULT_AES_KEY")
		if h == "" || a == "" {
			vaultKeyErr = errors.New("vault disabled: SENTINEL_VAULT_HMAC_KEY and SENTINEL_VAULT_AES_KEY must be set (32+ bytes, base64 or hex)")
			return
		}
		hk, err := decodeKey(h, 32)
		if err != nil {
			vaultKeyErr = fmt.Errorf("SENTINEL_VAULT_HMAC_KEY: %w", err)
			return
		}
		ak, err := decodeKey(a, 32)
		if err != nil {
			vaultKeyErr = fmt.Errorf("SENTINEL_VAULT_AES_KEY: %w", err)
			return
		}
		vaultHMACKey, vaultAEADKey = hk, ak
	})
	return vaultHMACKey, vaultAEADKey, vaultKeyErr
}

func decodeKey(s string, want int) ([]byte, error) {
	if b, err := hex.DecodeString(s); err == nil && len(b) >= want {
		return b[:want], nil
	}
	if b, err := base64.StdEncoding.DecodeString(s); err == nil && len(b) >= want {
		return b[:want], nil
	}
	return nil, fmt.Errorf("must decode (hex or base64) to at least %d bytes", want)
}

func sealValue(key, plaintext []byte) ([]byte, error) {
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, err
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, err
	}
	nonce := make([]byte, gcm.NonceSize())
	if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
		return nil, err
	}
	return gcm.Seal(nonce, nonce, plaintext, nil), nil
}

func openValue(key, ct []byte) ([]byte, error) {
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, err
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, err
	}
	if len(ct) < gcm.NonceSize() {
		return nil, errors.New("ciphertext too short")
	}
	return gcm.Open(nil, ct[:gcm.NonceSize()], ct[gcm.NonceSize():], nil)
}

func typeAbbrev(fieldType string) string {
	t := strings.ToUpper(strings.TrimSpace(fieldType))
	if t == "" {
		return "GEN"
	}
	if len(t) < 3 {
		return t + strings.Repeat("X", 3-len(t))
	}
	return t[:3]
}

func policyFor(tenantID string) *TokenizationPolicy {
	GlobalVault.mu.RLock()
	defer GlobalVault.mu.RUnlock()
	for i := range GlobalVault.Policies {
		if GlobalVault.Policies[i].TenantID == tenantID {
			return &GlobalVault.Policies[i]
		}
	}
	return nil
}

// TokenizeField produces a masked display value and a keyed token pointer, and
// stores the raw value AES-256-GCM encrypted with a retention deadline.
func TokenizeField(tenantID string, fieldType string, rawValue string) (TokenizedRecord, error) {
	hmacKey, aesKey, err := loadVaultKeys()
	if err != nil {
		return TokenizedRecord{}, err
	}

	trimmed := strings.TrimSpace(rawValue)
	masked := maskValue(fieldType, trimmed)

	h := hmac.New(sha256.New, hmacKey)
	h.Write([]byte(tenantID + ":" + fieldType + ":" + trimmed))
	tokenKey := "TOK-" + typeAbbrev(fieldType) + "-" + hex.EncodeToString(h.Sum(nil))[:12]

	ct, err := sealValue(aesKey, []byte(trimmed))
	if err != nil {
		return TokenizedRecord{}, fmt.Errorf("vault seal failed: %w", err)
	}

	retention := 2555
	if p := policyFor(tenantID); p != nil && p.RetentionDays > 0 {
		retention = p.RetentionDays
	}

	GlobalVault.mu.Lock()
	GlobalVault.entries[tokenKey] = vaultEntry{
		ciphertext: ct,
		expiresAt:  time.Now().UTC().AddDate(0, 0, retention),
	}
	GlobalVault.mu.Unlock()

	return TokenizedRecord{
		OriginalType: fieldType,
		MaskedValue:  masked,
		TokenKey:     tokenKey,
		TenantID:     tenantID,
		CreatedAt:    time.Now().UTC(),
	}, nil
}

func maskValue(fieldType, trimmed string) string {
	switch {
	case fieldType == "ROUTING_NUMBER" && len(trimmed) == 9:
		return trimmed[0:4] + "****" + trimmed[8:9]
	case fieldType == "ACCOUNT_NUMBER" && len(trimmed) >= 4:
		return strings.Repeat("*", len(trimmed)-4) + trimmed[len(trimmed)-4:]
	case fieldType == "INDIVIDUAL_NAME":
		parts := strings.Split(trimmed, " ")
		out := make([]string, 0, len(parts))
		for _, p := range parts {
			if len(p) > 1 {
				out = append(out, p[0:1]+strings.Repeat("*", len(p)-1))
			} else {
				out = append(out, p)
			}
		}
		return strings.Join(out, " ")
	default:
		return "******** (REDACTED)"
	}
}

// authorizeDetokenize enforces a shared-secret supervisor credential in constant
// time. This is a minimum bar, not a substitute for real mTLS client-cert or
// OIDC identity; it exists so the endpoint is not open to the world.
func authorizeDetokenize(r *http.Request) error {
	expected := os.Getenv("SENTINEL_SUPERVISOR_TOKEN")
	if expected == "" {
		return errors.New("detokenization disabled: SENTINEL_SUPERVISOR_TOKEN is not configured")
	}
	got := strings.TrimPrefix(r.Header.Get("Authorization"), "Bearer ")
	if subtle.ConstantTimeCompare([]byte(got), []byte(expected)) != 1 {
		return errors.New("detokenization denied: invalid supervisor credential")
	}
	return nil
}

func RegisterVaultRoutes(r chi.Router, db *sql.DB) {
	r.Route("/vault", func(r chi.Router) {
		r.Post("/tokenize", func(w http.ResponseWriter, r *http.Request) {
			var body struct {
				TenantID  string `json:"tenantId"`
				FieldType string `json:"fieldType"`
				RawValue  string `json:"rawValue"`
			}
			if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
				http.Error(w, "invalid request body", http.StatusBadRequest)
				return
			}
			record, err := TokenizeField(body.TenantID, body.FieldType, body.RawValue)
			if err != nil {
				http.Error(w, err.Error(), http.StatusServiceUnavailable)
				return
			}
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(record)
		})

		r.Get("/policies", func(w http.ResponseWriter, r *http.Request) {
			GlobalVault.mu.RLock()
			defer GlobalVault.mu.RUnlock()
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(GlobalVault.Policies)
		})

		r.Post("/detokenize", func(w http.ResponseWriter, r *http.Request) {
			if err := authorizeDetokenize(r); err != nil {
				// Log the denied attempt BEFORE returning.
				_, _ = AppendAuditEvent(db, "VAULT_DETOKENIZE_DENIED", "unauthenticated",
					map[string]interface{}{"reason": err.Error(), "remoteAddr": r.RemoteAddr})
				http.Error(w, err.Error(), http.StatusUnauthorized)
				return
			}

			var body struct {
				TokenKey     string `json:"tokenKey"`
				SupervisorID string `json:"supervisorId"`
				AuditReason  string `json:"auditReason"`
			}
			if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
				http.Error(w, "invalid detokenize request", http.StatusBadRequest)
				return
			}
			if body.SupervisorID == "" || len(body.AuditReason) < 10 {
				http.Error(w, "supervisor id and a justification of >=10 chars are required", http.StatusForbidden)
				return
			}

			GlobalVault.mu.RLock()
			entry, exists := GlobalVault.entries[body.TokenKey]
			GlobalVault.mu.RUnlock()
			if !exists {
				http.Error(w, "token not found", http.StatusNotFound)
				return
			}
			if time.Now().UTC().After(entry.expiresAt) {
				http.Error(w, "token past retention deadline", http.StatusGone)
				return
			}

			_, aesKey, err := loadVaultKeys()
			if err != nil {
				http.Error(w, err.Error(), http.StatusServiceUnavailable)
				return
			}
			plaintext, err := openValue(aesKey, entry.ciphertext)
			if err != nil {
				http.Error(w, "vault decryption failed", http.StatusInternalServerError)
				return
			}

			// Write the audit record and only claim auditLogged if it succeeded.
			ev, auditErr := AppendAuditEvent(db, "VAULT_DETOKENIZE", body.SupervisorID,
				map[string]interface{}{
					"tokenKey":    body.TokenKey,
					"auditReason": body.AuditReason,
					"remoteAddr":  r.RemoteAddr,
				})
			if auditErr != nil {
				// Fail closed: no audit record means no disclosure.
				http.Error(w, "refusing to disclose: audit ledger write failed", http.StatusInternalServerError)
				return
			}

			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]interface{}{
				"tokenKey":     body.TokenKey,
				"detokenized":  string(plaintext),
				"supervisorId": body.SupervisorID,
				"auditLogged":  true,
				"auditEventId": ev.ID,
				"auditHash":    ev.CurrentHash,
				"accessedAt":   time.Now().UTC().Format(time.RFC3339),
			})
		})
	})
}
```

---

## `gateway/agent_swarm.go`

**Lines:** 219  ·  **Reason for removal:** A scripted four-agent transcript with typed-in confidence values (0.96-1.00) presented as multi-agent deliberation. No model is invoked.

```go
package main

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"net/http"
	"sync"
	"time"

	"github.com/go-chi/chi/v5"
)

// AgentRole defines specialized autonomous agents in the financial swarm
type AgentRole string

const (
	RoleLeadSupervisor   AgentRole = "LEAD_SUPERVISOR"
	RoleFormatValidator  AgentRole = "FORMAT_VALIDATOR"
	RoleLineageRecon     AgentRole = "LINEAGE_RECON"
	RoleAuditCompliance  AgentRole = "AUDIT_COMPLIANCE"
)

// AgentMessage represents an inter-agent reasoning or tool communication step
type AgentMessage struct {
	ID             string    `json:"id"`
	AgentRole      AgentRole `json:"agentRole"`
	AgentName      string    `json:"agentName"`
	StepType       string    `json:"stepType"` // "THOUGHT", "TOOL_CALL", "OBSERVATION", "CONCLUSION"
	Content        string    `json:"content"`
	ToolName       string    `json:"toolName,omitempty"`
	ToolParameters string    `json:"toolParameters,omitempty"`
	Confidence     float64   `json:"confidence"`
	Timestamp      time.Time `json:"timestamp"`
}

// SwarmSession represents an active multi-agent deliberation session
type SwarmSession struct {
	SessionID         string         `json:"sessionId"`
	IncidentID        int64          `json:"incidentId"`
	FileID            int64          `json:"fileId"`
	Status            string         `json:"status"` // "DELIBERATING", "CONSENSUS_REACHED", "AWAITING_SUPERVISOR"
	ConsensusAction   string         `json:"consensusAction"`
	ConsensusSeverity string         `json:"consensusSeverity"`
	ConfidenceScore   float64        `json:"confidenceScore"`
	Messages          []AgentMessage `json:"messages"`
	StartedAt         time.Time      `json:"startedAt"`
	CompletedAt       time.Time      `json:"completedAt,omitempty"`
}

type SwarmStore struct {
	mu       sync.RWMutex
	Sessions map[string]*SwarmSession
}

var GlobalSwarmStore = &SwarmStore{
	Sessions: make(map[string]*SwarmSession),
}

// RunAgentSwarm executes an orchestrated multi-agent deliberation
func RunAgentSwarm(incidentID int64, fileID int64, rawFindings []string, rawData string) *SwarmSession {
	sessionID := fmt.Sprintf("SWARM-%d-%d", incidentID, time.Now().Unix())
	now := time.Now().UTC()

	session := &SwarmSession{
		SessionID:       sessionID,
		IncidentID:      incidentID,
		FileID:          fileID,
		Status:          "DELIBERATING",
		StartedAt:       now,
		ConfidenceScore: 0.96,
		Messages:        []AgentMessage{},
	}

	// 1. Lead Supervisor initializes triage plan
	session.Messages = append(session.Messages, AgentMessage{
		ID:         fmt.Sprintf("MSG-%d-1", time.Now().UnixNano()),
		AgentRole:  RoleLeadSupervisor,
		AgentName:  "Astra Lead Supervisor",
		StepType:   "THOUGHT",
		Content:    fmt.Sprintf("Initiating multi-agent incident triage for Incident #%d (File #%d). Dispatching parallel verification tasks to FormatValidator, LineageRecon, and AuditCompliance agents.", incidentID, fileID),
		Confidence: 0.98,
		Timestamp:  now,
	})

	// 2. Format Validator executes line-by-line inspection
	session.Messages = append(session.Messages, AgentMessage{
		ID:             fmt.Sprintf("MSG-%d-2", time.Now().UnixNano()),
		AgentRole:      RoleFormatValidator,
		AgentName:      "Syntax & Mod10 Inspector",
		StepType:       "TOOL_CALL",
		ToolName:       "parseAndValidateNachaLine",
		ToolParameters: `{"line": "6220210000218420000245000999888800John Doe                 0021000020000001", "recordType": 6}`,
		Content:        "Calling deterministic Federal Reserve Mod10 checksum parser on Entry Detail records.",
		Confidence:     0.99,
		Timestamp:      now.Add(120 * time.Millisecond),
	})

	session.Messages = append(session.Messages, AgentMessage{
		ID:         fmt.Sprintf("MSG-%d-3", time.Now().UnixNano()),
		AgentRole:  RoleFormatValidator,
		AgentName:  "Syntax & Mod10 Inspector",
		StepType:   "OBSERVATION",
		Content:    "Identified Federal Reserve Mod10 routing error: Prefix '021000021' fails weights [3,7,1,3,7,1,3,7] check digit formula. Calculated 8 != expected 1.",
		Confidence: 0.99,
		Timestamp:  now.Add(240 * time.Millisecond),
	})

	// 3. Lineage Recon assesses downstream blast radius
	session.Messages = append(session.Messages, AgentMessage{
		ID:             fmt.Sprintf("MSG-%d-4", time.Now().UnixNano()),
		AgentRole:      RoleLineageRecon,
		AgentName:      "Settlement Lineage Recon",
		StepType:       "TOOL_CALL",
		ToolName:       "query_downstream_dependencies",
		ToolParameters: `{"sourceAsset": "ASSET-001", "targetDB": "public.settlement_batches"}`,
		Content:        "Inspecting catalog lineage to verify if corrupted batch has reached PostgreSQL staging ledger.",
		Confidence:     0.97,
		Timestamp:      now.Add(350 * time.Millisecond),
	})

	session.Messages = append(session.Messages, AgentMessage{
		ID:         fmt.Sprintf("MSG-%d-5", time.Now().UnixNano()),
		AgentRole:  RoleLineageRecon,
		AgentName:  "Settlement Lineage Recon",
		StepType:   "OBSERVATION",
		Content:    "Blast Radius Assessment: Zero downstream contamination. Sentinel Gateway isolated payload at ingress boundary. Core PostgreSQL settlement ledger public.settlement_batches remains intact.",
		Confidence: 0.99,
		Timestamp:  now.Add(480 * time.Millisecond),
	})

	// 4. Audit Compliance verifies Merkle proof
	session.Messages = append(session.Messages, AgentMessage{
		ID:         fmt.Sprintf("MSG-%d-6", time.Now().UnixNano()),
		AgentRole:  RoleAuditCompliance,
		AgentName:  "SEC 17a-4 Audit Defense",
		StepType:   "CONCLUSION",
		Content:    "Cryptographic SHA-256 evidence package created and committed to Merkle ledger block. Ready for SOX 404 audit submission.",
		Confidence: 1.00,
		Timestamp:  now.Add(600 * time.Millisecond),
	})

	// 5. Lead Supervisor reaches consensus
	session.Status = "CONSENSUS_REACHED"
	session.ConsensusAction = "QUARANTINE_AND_DISPATCH_CORRECTED_RESEND_NOTICE"
	session.ConsensusSeverity = "CRITICAL"
	session.CompletedAt = now.Add(750 * time.Millisecond)

	session.Messages = append(session.Messages, AgentMessage{
		ID:         fmt.Sprintf("MSG-%d-7", time.Now().UnixNano()),
		AgentRole:  RoleLeadSupervisor,
		AgentName:  "Astra Lead Supervisor",
		StepType:   "CONCLUSION",
		Content:    "Consensus finalized (Confidence: 98.4%). Action: Contain batch in Dead-Letter Quarantine, dispatch Nacha Article 2 remediation notice to Meridian Custody Bank, require Tier-3 human supervisor dual-control sign-off.",
		Confidence: 0.984,
		Timestamp:  session.CompletedAt,
	})

	GlobalSwarmStore.mu.Lock()
	GlobalSwarmStore.Sessions[session.SessionID] = session
	GlobalSwarmStore.mu.Unlock()

	return session
}

// RegisterSwarmRoutes wires the multi-agent swarm endpoints into Chi router
func RegisterSwarmRoutes(r chi.Router, db *sql.DB) {
	r.Route("/swarm", func(r chi.Router) {
		// POST /api/v1/swarm/deliberate (Trigger multi-agent swarm)
		r.Post("/deliberate", func(w http.ResponseWriter, r *http.Request) {
			var body struct {
				IncidentID  int64    `json:"incidentId"`
				FileID      int64    `json:"fileId"`
				RawFindings []string `json:"findings"`
				RawData     string   `json:"rawData"`
			}

			if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
				http.Error(w, "invalid request body", http.StatusBadRequest)
				return
			}

			session := RunAgentSwarm(body.IncidentID, body.FileID, body.RawFindings, body.RawData)

			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(session)
		})

		// GET /api/v1/swarm/sessions
		r.Get("/sessions", func(w http.ResponseWriter, r *http.Request) {
			GlobalSwarmStore.mu.RLock()
			defer GlobalSwarmStore.mu.RUnlock()

			var list []*SwarmSession
			for _, s := range GlobalSwarmStore.Sessions {
				list = append(list, s)
			}

			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(list)
		})

		// GET /api/v1/swarm/sessions/{id}
		r.Get("/sessions/{id}", func(w http.ResponseWriter, r *http.Request) {
			sessionID := chi.URLParam(r, "id")
			GlobalSwarmStore.mu.RLock()
			defer GlobalSwarmStore.mu.RUnlock()

			session, exists := GlobalSwarmStore.Sessions[sessionID]
			if !exists {
				http.Error(w, "swarm session not found", http.StatusNotFound)
				return
			}

			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(session)
		})
	})
}
```

---

## `gateway/healing.go`

**Lines:** 196  ·  **Reason for removal:** `/healing/apply` accepted a caller-supplied `supervisorId` and arbitrary `repairedContent`, then re-ingested it without binding to any immutable proposal or approval record. `ConfidenceScore: 0.995` was a literal.

```go
package main

import (
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
)

// RepairPatch represents an atomic modification chunk for a corrupted financial file
type RepairPatch struct {
	LineNumber    int    `json:"lineNumber"`
	OriginalText  string `json:"originalText"`
	RepairedText  string `json:"repairedText"`
	RepairReason  string `json:"repairReason"`
	CalculatedFix string `json:"calculatedFix"`
}

// SelfHealingProposal represents a complete proposed file repair with dry-run verification
type SelfHealingProposal struct {
	ProposalID          string        `json:"proposalId"`
	IncidentID          int64         `json:"incidentId"`
	FileID              int64         `json:"fileId"`
	OriginalSha256      string        `json:"originalSha256"`
	RepairedSha256      string        `json:"repairedSha256"`
	Status              string        `json:"status"` // "PROPOSED", "DRY_RUN_PASSED", "SUPERVISOR_APPROVED", "RE_INGESTED"
	ConfidenceScore     float64       `json:"confidenceScore"`
	Patches             []RepairPatch `json:"patches"`
	RepairedFullContent string        `json:"repairedFullContent"`
	DryRunSummary       string        `json:"dryRunSummary"`
	CreatedAt           time.Time     `json:"createdAt"`
}

// GenerateSelfHealingProposal analyzes corrupted NACHA lines and proposes deterministic fixes
func GenerateSelfHealingProposal(incidentID int64, fileID int64, rawContent string) *SelfHealingProposal {
	lines := strings.Split(strings.ReplaceAll(rawContent, "\r\n", "\n"), "\n")
	var patches []RepairPatch
	var repairedLines []string

	hasHeader := false
	var totalEntryHash int64 = 0

	for i, line := range lines {
		trimmed := strings.TrimRight(line, " \t\r")
		if len(trimmed) == 0 {
			continue
		}

		recordType := trimmed[0:1]
		if recordType == "1" {
			hasHeader = true
			repairedLines = append(repairedLines, trimmed)
		} else if recordType == "6" {
			// Extract 8-digit routing prefix for entry hash calculation
			if len(trimmed) >= 12 {
				routingPrefix := trimmed[3:11]
				if val, err := strconv.ParseInt(routingPrefix, 10, 64); err == nil {
					totalEntryHash += val
				}
			}

			// Check for Mod10 check digit correction on Entry Detail
			if len(trimmed) >= 12 && trimmed[3:12] == "021000021" {
				// Correct check digit from 1 to 8 based on Federal Reserve Mod10
				fixedLine := trimmed[0:11] + "8" + trimmed[12:]
				patches = append(patches, RepairPatch{
					LineNumber:    i + 1,
					OriginalText:  trimmed,
					RepairedText:  fixedLine,
					RepairReason:  "Federal Reserve Mod10 Check Digit Mismatch",
					CalculatedFix: "Replaced erroneous check digit '1' with calculated Mod10 digit '8' for ABA prefix 02100002",
				})
				repairedLines = append(repairedLines, fixedLine)
			} else {
				repairedLines = append(repairedLines, trimmed)
			}
		} else if recordType == "8" {
			// Calculate exact 10-digit Entry Hash modulo 10,000,000,000
			modEntryHash := totalEntryHash % 10000000000
			expectedHashStr := fmt.Sprintf("%010d", modEntryHash)

			if len(trimmed) >= 20 {
				currentHashStr := trimmed[10:20]
				if currentHashStr != expectedHashStr {
					// Patch the Batch Control Entry Hash
					fixedControl := trimmed[0:10] + expectedHashStr + trimmed[20:]
					patches = append(patches, RepairPatch{
						LineNumber:    i + 1,
						OriginalText:  trimmed,
						RepairedText:  fixedControl,
						RepairReason:  "Batch Control Record 8 Entry Hash Miscalculation",
						CalculatedFix: fmt.Sprintf("Updated Entry Hash from %s to mathematically verified sum %s", currentHashStr, expectedHashStr),
					})
					repairedLines = append(repairedLines, fixedControl)
				} else {
					repairedLines = append(repairedLines, trimmed)
				}
			} else {
				repairedLines = append(repairedLines, trimmed)
			}
		} else {
			repairedLines = append(repairedLines, trimmed)
		}
	}

	// Calculate original and repaired SHA-256 digests
	origHasher := sha256.New()
	origHasher.Write([]byte(rawContent))
	origSha := hex.EncodeToString(origHasher.Sum(nil))

	repairedFull := strings.Join(repairedLines, "\n") + "\n"
	repHasher := sha256.New()
	repHasher.Write([]byte(repairedFull))
	repSha := hex.EncodeToString(repHasher.Sum(nil))

	statusStr := "DRY_RUN_PASSED"
	if !hasHeader {
		statusStr = "HEADER_REQUIRED"
	}

	return &SelfHealingProposal{
		ProposalID:          fmt.Sprintf("HEAL-%d-%d", incidentID, time.Now().Unix()),
		IncidentID:          incidentID,
		FileID:              fileID,
		OriginalSha256:      origSha,
		RepairedSha256:      repSha,
		Status:              statusStr,
		ConfidenceScore:     0.995,
		Patches:             patches,
		RepairedFullContent: repairedFull,
		DryRunSummary:       fmt.Sprintf("Dry-Run Validation Succeeded: %d patches applied. Mod10 routing and Entry Hash sums verified against Nacha 2025 specification.", len(patches)),
		CreatedAt:           time.Now().UTC(),
	}
}

// RegisterSelfHealingRoutes registers HTTP endpoints for self-healing file repairs
func RegisterSelfHealingRoutes(r chi.Router, db *sql.DB) {
	r.Route("/healing", func(r chi.Router) {
		// POST /api/v1/healing/propose (Generate self-healing patch)
		r.Post("/propose", func(w http.ResponseWriter, r *http.Request) {
			var body struct {
				IncidentID int64  `json:"incidentId"`
				FileID     int64  `json:"fileId"`
				RawContent string `json:"rawContent"`
			}
			if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
				http.Error(w, "invalid request body", http.StatusBadRequest)
				return
			}

			proposal := GenerateSelfHealingProposal(body.IncidentID, body.FileID, body.RawContent)

			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(proposal)
		})

		// POST /api/v1/healing/apply (Supervisor Dual-Control Approval & Re-Ingest)
		r.Post("/apply", func(w http.ResponseWriter, r *http.Request) {
			var body struct {
				ProposalID      string `json:"proposalId"`
				SupervisorID    string `json:"supervisorId"`
				ApprovalNote    string `json:"approvalNote"`
				RepairedContent string `json:"repairedContent"`
			}
			if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
				http.Error(w, "invalid approval payload", http.StatusBadRequest)
				return
			}

			// Ingest repaired payload directly into processor
			result, err := ProcessFileBytes(db, "REPAIRED_NACHA_BATCH.ach", []byte(body.RepairedContent))
			if err != nil {
				http.Error(w, err.Error(), http.StatusInternalServerError)
				return
			}

			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(map[string]interface{}{
				"status":        "HEALED_AND_INGESTED",
				"proposalId":    body.ProposalID,
				"supervisorId":  body.SupervisorID,
				"ingestionId":   result.FileID,
				"fileStatus":    result.Status,
				"findingsCount": len(result.Findings),
				"executedAt":    time.Now().UTC().Format(time.RFC3339),
			})
		})
	})
}
```

---

## `gateway/failover.go`

**Lines:** 73  ·  **Reason for removal:** Scripted DR simulation. Already honestly labelled `IsScriptedDemo: true`; removed because a simulator is not a resilience control. Prompt 14 proves recovery with integration tests and runbooks instead.

```go
package main

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
)

// FailoverSimulationResult represents automated cross-region DR failover telemetry
type FailoverSimulationResult struct {
	SimulationID          string    `json:"simulationId"`
	PrimaryRegion         string    `json:"primaryRegion"`
	StandbyRegion         string    `json:"standbyRegion"`
	FailoverTriggerReason string    `json:"failoverTriggerReason"`
	IsScriptedDemo        bool      `json:"isScriptedDemo"`
	Disclaimer            string    `json:"disclaimer"`
	ElapsedMsScripted     float64   `json:"elapsedMsScripted"`
	RpoSecondsTarget      float64   `json:"rpoSecondsTarget"`      // TARGET, not measured
	RtoMillisecondsTarget float64   `json:"rtoMillisecondsTarget"` // TARGET, not measured
	ReplicatedBlocksCount int64     `json:"replicatedBlocksCount"`
	DataLossTransactionCount int   `json:"dataLossTransactionCount"`
	StandbyHealthStatus   string    `json:"standbyHealthStatus"` // "ACTIVE_PROMOTED", "SYNC_HEALTHY"
	Timestamp             time.Time `json:"timestamp"`
}

// SimulateCrossRegionFailover renders a SCRIPTED demonstration of a DR failover.
//
// HONESTY NOTE (2026-08-14): this function does not perform a failover. It
// sleeps for a fixed 42ms and then measures how long it slept. There is no
// second region, no replica, no replication stream, and no promotion. The
// previously advertised "RTO = 42.5ms / RPO = 0.00s, 100% Proven" was a
// measurement of time.Sleep(42ms) and a struct literal respectively.
//
// The fields below are therefore explicitly marked as scripted. Measuring a
// real RTO requires killing a real primary and timing a real promotion; until
// that exists, no RTO/RPO number from this codebase may be published.
func SimulateCrossRegionFailover() FailoverSimulationResult {
	start := time.Now()
	time.Sleep(42 * time.Millisecond) // scripted delay, NOT failover work
	elapsed := float64(time.Since(start).Microseconds()) / 1000.0

	return FailoverSimulationResult{
		IsScriptedDemo:     true,
		Disclaimer:         "SCRIPTED DEMONSTRATION. No failover occurred; no replica exists. RPO/RTO below are illustrative targets, not measurements.",
		ElapsedMsScripted:  elapsed,
		SimulationID:             fmt.Sprintf("DR-FAILOVER-%d", time.Now().Unix()),
		PrimaryRegion:            "us-east-1 (N. Virginia Active)",
		StandbyRegion:            "us-west-2 (Oregon Standby)",
		FailoverTriggerReason:    "SIMULATED_PRIMARY_DATACENTER_OUTAGE",
		RpoSecondsTarget:         0.00,
		RtoMillisecondsTarget:    42.5,
		ReplicatedBlocksCount:    0, // unknown: no replication stream exists
		DataLossTransactionCount: 0,
		StandbyHealthStatus:      "NOT_PROVISIONED",
		Timestamp:                time.Now().UTC(),
	}
}

// RegisterFailoverRoutes wires disaster recovery endpoints into Chi router
func RegisterFailoverRoutes(r chi.Router, db *sql.DB) {
	r.Route("/chaos/failover", func(r chi.Router) {
		// POST /api/v1/chaos/failover/simulate
		r.Post("/simulate", func(w http.ResponseWriter, r *http.Request) {
			result := SimulateCrossRegionFailover()
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(result)
		})
	})
}
```

---

## `gateway/webhook.go`

**Lines:** 114  ·  **Reason for removal:** Delivery ran from a request handler to any caller-supplied URL with no allowlist, no private/metadata-range block and no redirect limit. Secrets were generated from `time.Now().UnixNano()`. Prompt 05 rebuilds this as durable notification jobs.

```go
package main

import (
	"bytes"
	"crypto/hmac"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/http"
	"time"
)

type WebhookSubscription struct {
	ID        int64     `json:"id"`
	URL       string    `json:"url"`
	Secret    string    `json:"secret"`
	Events    []string  `json:"events"`
	Status    string    `json:"status"` // ACTIVE, DISABLED
	CreatedAt time.Time `json:"createdAt"`
}

type WebhookDeliveryEvent struct {
	EventID       string      `json:"eventId"`
	EventType     string      `json:"eventType"`
	TimestampUtc  string      `json:"timestampUtc"`
	TenantID      string      `json:"tenantId"`
	PayloadDigest string      `json:"payloadDigest"`
	Data          interface{} `json:"data"`
}

type WebhookDeliveryLog struct {
	ID           int64     `json:"id"`
	WebhookID    int64     `json:"webhookId"`
	EventType    string    `json:"eventType"`
	ResponseCode int       `json:"responseCode"`
	Status       string    `json:"status"` // DELIVERED, FAILED
	DeliveredAt  time.Time `json:"deliveredAt"`
	DurationMs   int64     `json:"durationMs"`
}

// ComputeWebhookHmac generates the cryptographic HMAC-SHA256 signature for downstream validation.
func ComputeWebhookHmac(payload []byte, secret string) string {
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write(payload)
	return hex.EncodeToString(mac.Sum(nil))
}

// DispatchWebhookEvent sends an asynchronous HTTP POST with HMAC signature header.
func DispatchWebhookEvent(url string, secret string, event WebhookDeliveryEvent) (*WebhookDeliveryLog, error) {
	payloadBytes, err := json.Marshal(event)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal webhook event: %w", err)
	}

	sig := ComputeWebhookHmac(payloadBytes, secret)

	req, err := http.NewRequest("POST", url, bytes.NewBuffer(payloadBytes))
	if err != nil {
		return nil, fmt.Errorf("failed to create webhook request: %w", err)
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("User-Agent", "Sentinel-Flow-Webhook-Dispatcher/1.0")
	req.Header.Set("X-Sentinel-Signature", fmt.Sprintf("sha256=%s", sig))
	req.Header.Set("X-Sentinel-Event", event.EventType)
	req.Header.Set("X-Sentinel-Event-Id", event.EventID)

	client := &http.Client{Timeout: 5 * time.Second}
	startTime := time.Now()

	resp, err := client.Do(req)
	duration := time.Since(startTime).Milliseconds()

	if err != nil {
		return &WebhookDeliveryLog{
			EventType:    event.EventType,
			ResponseCode: 0,
			Status:       "FAILED",
			DeliveredAt:  time.Now().UTC(),
			DurationMs:   duration,
		}, err
	}
	defer resp.Body.Close()

	status := "DELIVERED"
	if resp.StatusCode >= 400 {
		status = "FAILED"
	}

	return &WebhookDeliveryLog{
		EventType:    event.EventType,
		ResponseCode: resp.StatusCode,
		Status:       status,
		DeliveredAt:  time.Now().UTC(),
		DurationMs:   duration,
	}, nil
}

// InitWebhookSchema ensures webhook subscription table exists in SQLite.
func InitWebhookSchema(db *sql.DB) error {
	_, err := db.Exec(`
		CREATE TABLE IF NOT EXISTS webhook_subscriptions (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			url TEXT NOT NULL,
			secret TEXT NOT NULL,
			events TEXT NOT NULL DEFAULT '["ALL"]',
			status TEXT NOT NULL DEFAULT 'ACTIVE',
			created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
		);
	`)
	return err
}
```

---

## `gateway/drift.go`

**Lines:** 111  ·  **Reason for removal:** The KS statistics are real (`kstest.go`, retained) but the served report envelope hardcoded `ConfidenceScore: 0.978`, `ReportID: DRIFT-REP-20260814` and a fixed partner name.

```go
package main

import (
	"database/sql"
	"encoding/json"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
)

// DriftMetric represents statistical field distribution shifts
type DriftMetric struct {
	FieldName         string  `json:"fieldName"`
	MetricType        string  `json:"metricType"` // "NUMERICAL_VALUE", "CATEGORICAL_RATIO", "NULL_RATE"
	BaselineMean      float64 `json:"baselineMean"`
	CurrentMean       float64 `json:"currentMean"`
	DivergenceScore   float64 `json:"divergenceScore"` // Normalized Kolmogorov-Smirnov or Chi-Sq D-statistic
	Status            string  `json:"status"`          // "STABLE", "MODERATE_DRIFT", "SEVERE_DRIFT"
	IsSignificant     bool    `json:"isSignificant"`
	Explanation       string  `json:"explanation"`
}

// DriftProfileReport represents comprehensive institutional schema & volume drift analysis
type DriftProfileReport struct {
	ReportID             string        `json:"reportId"`
	PartnerID            string        `json:"partnerId"`
	PartnerName          string        `json:"partnerName"`
	EvaluationWindowDays int           `json:"evaluationWindowDays"`
	OverallDriftStatus   string        `json:"overallDriftStatus"` // "HEALTHY_STABLE", "WARNING_DRIFT_DETECTED"
	ConfidenceScore      float64       `json:"confidenceScore"`
	Metrics              []DriftMetric `json:"metrics"`
	EvaluatedAt          time.Time     `json:"evaluatedAt"`
}

// CalculateDriftMetrics computes distribution divergence against contractual baselines
func CalculateDriftMetrics() DriftProfileReport {
	metrics := []DriftMetric{
		{
			FieldName:       "TransactionAmountCents",
			MetricType:      "NUMERICAL_VALUE",
			BaselineMean:    2450.00,
			CurrentMean:     2485.50,
			DivergenceScore: 0.042,
			Status:          "STABLE",
			IsSignificant:   false,
			Explanation:     "Median transaction ticket size ($24.85) is within standard ±5% historical band.",
		},
		{
			FieldName:       "SecClassCode_CCD_Ratio",
			MetricType:      "CATEGORICAL_RATIO",
			BaselineMean:    0.85,
			CurrentMean:     0.84,
			DivergenceScore: 0.015,
			Status:          "STABLE",
			IsSignificant:   false,
			Explanation:     "Corporate Credit or Debit (CCD) code ratio (84%) aligns with commercial payroll profile.",
		},
		{
			FieldName:       "DiscretionaryData_NullRate",
			MetricType:      "NULL_RATE",
			BaselineMean:    0.02,
			CurrentMean:     0.18,
			DivergenceScore: 0.380,
			Status:          "MODERATE_DRIFT",
			IsSignificant:   true,
			Explanation:     "Discretionary data null rate rose from 2.0% to 18.0%. Counterparty may have updated their upstream ERP extraction schema.",
		},
		{
			FieldName:       "HourlyArrivalKurtosis",
			MetricType:      "NUMERICAL_VALUE",
			BaselineMean:    3.10,
			CurrentMean:     3.15,
			DivergenceScore: 0.020,
			Status:          "STABLE",
			IsSignificant:   false,
			Explanation:     "Transmission window arrival timing kurtosis shows normal distribution around 14:00 UTC cutoff.",
		},
	}

	overallStatus := "HEALTHY_STABLE"
	for _, m := range metrics {
		if m.IsSignificant {
			overallStatus = "WARNING_DRIFT_DETECTED"
			break
		}
	}

	return DriftProfileReport{
		ReportID:             "DRIFT-REP-20260814",
		PartnerID:            "PARTNER-MERIDIAN-01",
		PartnerName:          "Meridian Custody Bank",
		EvaluationWindowDays: 30,
		OverallDriftStatus:   overallStatus,
		ConfidenceScore:      0.978,
		Metrics:              metrics,
		EvaluatedAt:          time.Now().UTC(),
	}
}

// RegisterDriftRoutes wires continuous schema & volume drift endpoints into Chi router
func RegisterDriftRoutes(r chi.Router, db *sql.DB) {
	r.Route("/analytics/drift", func(r chi.Router) {
		// GET /api/v1/analytics/drift
		r.Get("/", func(w http.ResponseWriter, r *http.Request) {
			report := CalculateDriftMetrics()
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(report)
		})
	})
}
```

---

## `gateway/anomaly.go`

**Lines:** 80  ·  **Reason for removal:** `EvaluateVolumeAnomaly` computed a real z-score against a hardcoded `DefaultBaseline`, and the route fed it two hardcoded literals. `robust_anomaly.go` (median/MAD, retained) has no dependency on this file.

```go
package main

import (
	"fmt"
	"math"
)

type BaselineStats struct {
	MeanRecords float64 `json:"meanRecords"`
	StdDevRecords float64 `json:"stdDevRecords"`
	MeanBytes float64 `json:"meanBytes"`
	StdDevBytes float64 `json:"stdDevBytes"`
	SampleCount int `json:"sampleCount"`
}

type VolumeAnomalyFinding struct {
	IsAnomaly bool `json:"isAnomaly"`
	ZScore float64 `json:"zScore"`
	MetricName string `json:"metricName"`
	ActualValue float64 `json:"actualValue"`
	ExpectedMean float64 `json:"expectedMean"`
	DeviationPct float64 `json:"deviationPct"`
	Severity string `json:"severity"`
	Explanation string `json:"explanation"`
}

// Default baseline for Meridian Commercial ACH (e.g. ~10,000 records ± 1,500)
var DefaultBaseline = BaselineStats{
	MeanRecords:   10000.0,
	StdDevRecords: 1500.0,
	MeanBytes:     940000.0,
	StdDevBytes:   141000.0,
	SampleCount:   30,
}

// EvaluateVolumeAnomaly compares actual file metrics against rolling statistical baselines.
func EvaluateVolumeAnomaly(actualRecords int, actualBytes int64, baseline BaselineStats) *VolumeAnomalyFinding {
	if baseline.StdDevRecords <= 0 {
		baseline.StdDevRecords = 1.0
	}

	zScore := (float64(actualRecords) - baseline.MeanRecords) / baseline.StdDevRecords
	absZ := math.Abs(zScore)
	deviationPct := ((float64(actualRecords) - baseline.MeanRecords) / baseline.MeanRecords) * 100.0

	// 3-Sigma threshold (99.7% confidence interval)
	if absZ >= 3.0 {
		severity := "WARNING"
		if absZ >= 5.0 {
			severity = "CRITICAL"
		}

		direction := "spike"
		if zScore < 0 {
			direction = "drop"
		}

		return &VolumeAnomalyFinding{
			IsAnomaly:    true,
			ZScore:       math.Round(zScore*100) / 100,
			MetricName:   "Record Count",
			ActualValue:  float64(actualRecords),
			ExpectedMean: baseline.MeanRecords,
			DeviationPct: math.Round(deviationPct*10) / 10,
			Severity:     severity,
			Explanation:  fmt.Sprintf("Statistical %s detected: %d records deviates by %.1f%% (|Z|=%.2f > 3.0σ baseline).", direction, actualRecords, math.Abs(deviationPct), absZ),
		}
	}

	return &VolumeAnomalyFinding{
		IsAnomaly:    false,
		ZScore:       math.Round(zScore*100) / 100,
		MetricName:   "Record Count",
		ActualValue:  float64(actualRecords),
		ExpectedMean: baseline.MeanRecords,
		DeviationPct: math.Round(deviationPct*10) / 10,
		Severity:     "INFO",
		Explanation:  "File volume conforms within expected 3-Sigma statistical baseline.",
	}
}
```

---

# Removed test files

These tests asserted that simulated behaviour was present, so they were deleted with the
features they covered. Retaining them would have re-encoded the mock as expected behaviour.

---

## `gateway/connector_test.go`

**Lines:** 110  ·  **Reason for removal:** Asserted the hardcoded Integration Hub catalog and its masked preview literals were returned.

```go
package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/go-chi/chi/v5"
)

func TestIntegrationHubSanitizedConnections(t *testing.T) {
	r := chi.NewRouter()
	RegisterIntegrationHubRoutes(r, nil)

	req := httptest.NewRequest("GET", "/hub/connections", nil)
	rr := httptest.NewRecorder()

	r.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("Expected HTTP 200 from /hub/connections, got %d", rr.Code)
	}

	body := rr.Body.String()

	// Strict OWASP Secrets Management Check:
	// Verify that raw passwords, raw private keys, and decrypted tokens NEVER appear in the response payload
	forbiddenKeywords := []string{"password", "BEGIN RSA PRIVATE KEY", "BEGIN OPENSSH PRIVATE KEY", "client_secret_value"}
	for _, kw := range forbiddenKeywords {
		if strings.Contains(strings.ToLower(body), strings.ToLower(kw)) {
			t.Errorf("Security Violation: Found forbidden secret keyword '%s' in connection payload", kw)
		}
	}

	var conns []Connection
	if err := json.Unmarshal([]byte(body), &conns); err != nil {
		t.Fatalf("Failed to parse connections JSON: %v", err)
	}

	if len(conns) < 3 {
		t.Errorf("Expected at least 3 registered connections, got %d", len(conns))
	}

	// Verify secret reference is present as a decoupled pointer
	for _, c := range conns {
		if !strings.HasPrefix(c.SecretRef.VaultKey, "vault://") && !strings.HasPrefix(c.SecretRef.VaultKey, "aws://") {
			t.Errorf("Expected decoupled Vault/AWS secret pointer, got %s", c.SecretRef.VaultKey)
		}
	}
}

func TestCatalogAssetsAndMaskedPreview(t *testing.T) {
	r := chi.NewRouter()
	RegisterIntegrationHubRoutes(r, nil)

	// Test GET /hub/assets
	req := httptest.NewRequest("GET", "/hub/assets", nil)
	rr := httptest.NewRecorder()
	r.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("Expected HTTP 200, got %d", rr.Code)
	}

	var assets []CatalogAsset
	if err := json.Unmarshal(rr.Body.Bytes(), &assets); err != nil {
		t.Fatalf("Failed to parse assets: %v", err)
	}
	if len(assets) < 3 {
		t.Errorf("Expected at least 3 catalog assets, got %d", len(assets))
	}

	// Test GET /hub/assets/ASSET-001/sample (Masked PII)
	reqSample := httptest.NewRequest("GET", "/hub/assets/ASSET-001/sample", nil)
	rrSample := httptest.NewRecorder()
	r.ServeHTTP(rrSample, reqSample)

	if rrSample.Code != http.StatusOK {
		t.Fatalf("Expected HTTP 200 for masked sample, got %d", rrSample.Code)
	}

	sampleBody := rrSample.Body.String()
	if !strings.Contains(sampleBody, "(MASKED)") && !strings.Contains(sampleBody, "(REDACTED)") {
		t.Errorf("Expected PII masking in sample payload, got: %s", sampleBody)
	}
}

func TestDataLineageGraph(t *testing.T) {
	r := chi.NewRouter()
	RegisterIntegrationHubRoutes(r, nil)

	req := httptest.NewRequest("GET", "/hub/lineage", nil)
	rr := httptest.NewRecorder()
	r.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("Expected HTTP 200 for lineage, got %d", rr.Code)
	}

	var lineageMap map[string]interface{}
	if err := json.Unmarshal(rr.Body.Bytes(), &lineageMap); err != nil {
		t.Fatalf("Failed to parse lineage DAG: %v", err)
	}

	if lineageMap["edges"] == nil || lineageMap["nodes"] == nil {
		t.Errorf("Lineage DAG missing nodes or edges")
	}
}
```

---

## `gateway/agent_swarm_test.go`

**Lines:** 57  ·  **Reason for removal:** Asserted the scripted swarm transcript.

```go
package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/go-chi/chi/v5"
)

func TestMultiAgentSwarmDeliberation(t *testing.T) {
	r := chi.NewRouter()
	RegisterSwarmRoutes(r, nil)

	payload := `{"incidentId": 101, "fileId": 501, "findings": ["INVALID_MOD10_ROUTING"], "rawData": "6220210000218420000245000999888800John Doe"}`
	req := httptest.NewRequest("POST", "/swarm/deliberate", strings.NewReader(payload))
	req.Header.Set("Content-Type", "application/json")
	rr := httptest.NewRecorder()

	r.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("Expected HTTP 200 from /swarm/deliberate, got %d", rr.Code)
	}

	var session SwarmSession
	if err := json.Unmarshal(rr.Body.Bytes(), &session); err != nil {
		t.Fatalf("Failed to parse swarm session: %v", err)
	}

	if session.Status != "CONSENSUS_REACHED" {
		t.Errorf("Expected status CONSENSUS_REACHED, got %s", session.Status)
	}

	if len(session.Messages) < 5 {
		t.Errorf("Expected at least 5 inter-agent messages, got %d", len(session.Messages))
	}

	if session.ConfidenceScore < 0.90 {
		t.Errorf("Expected confidence >= 0.90, got %.2f", session.ConfidenceScore)
	}

	// Verify all 4 agent roles participated
	rolesFound := make(map[AgentRole]bool)
	for _, m := range session.Messages {
		rolesFound[m.AgentRole] = true
	}

	expectedRoles := []AgentRole{RoleLeadSupervisor, RoleFormatValidator, RoleLineageRecon, RoleAuditCompliance}
	for _, er := range expectedRoles {
		if !rolesFound[er] {
			t.Errorf("Agent role %s did not participate in swarm deliberation", er)
		}
	}
}
```

---

## `gateway/healing_test.go`

**Lines:** 71  ·  **Reason for removal:** Covered the self-healing proposal and the hardcoded drift envelope.

```go
package main

import (
	"strings"
	"testing"
)

func TestSelfHealingProposalGeneration(t *testing.T) {
	// Sample corrupted NACHA with invalid Mod10 digit (021000021 instead of 021000028)
	corruptedContent := "101 021000021 1234567892608141430A094101MERIDIAN CUSTODY        SENTINEL FLOW          \n" +
		"5200PAYROLL   CORP INC        0001234567PPDDIRECT PAY260814260814   1021000020000001\n" +
		"6220210000218420000245000999888800John Doe                 0021000020000001\n" +
		"820000000100021000020000002450000000000000000001234567                         021000020000001\n" +
		"900000100000100000001000210000200000024500000000000000                         \n"

	proposal := GenerateSelfHealingProposal(101, 501, corruptedContent)

	if proposal == nil {
		t.Fatalf("Expected non-nil self-healing proposal")
	}

	if proposal.Status != "DRY_RUN_PASSED" {
		t.Errorf("Expected status DRY_RUN_PASSED, got %s", proposal.Status)
	}

	if len(proposal.Patches) == 0 {
		t.Errorf("Expected at least 1 patch to be proposed, got 0")
	}

	// Verify that the proposed patch fixed the Mod10 routing digit to 8
	hasMod10Fix := false
	for _, p := range proposal.Patches {
		if strings.Contains(p.RepairedText, "021000028") {
			hasMod10Fix = true
			break
		}
	}

	if !hasMod10Fix {
		t.Errorf("Proposed patches did not include Mod10 routing correction to 021000028")
	}

	if proposal.OriginalSha256 == proposal.RepairedSha256 {
		t.Errorf("Expected different SHA-256 hashes after repair")
	}
}

func TestDriftMetricsCalculation(t *testing.T) {
	report := CalculateDriftMetrics()

	if report.PartnerID != "PARTNER-MERIDIAN-01" {
		t.Errorf("Expected partner ID PARTNER-MERIDIAN-01, got %s", report.PartnerID)
	}

	if len(report.Metrics) < 3 {
		t.Errorf("Expected at least 3 drift metrics, got %d", len(report.Metrics))
	}

	// Verify detection of DiscretionaryData null rate drift
	hasNullDrift := false
	for _, m := range report.Metrics {
		if m.FieldName == "DiscretionaryData_NullRate" && m.IsSignificant {
			hasNullDrift = true
			break
		}
	}

	if !hasNullDrift {
		t.Errorf("Failed to detect significant drift in DiscretionaryData_NullRate")
	}
}
```

---

## `gateway/vault_test.go`

**Lines:** 80  ·  **Reason for removal:** Covered vault tokenisation, the fabricated FedNow settlement, and the scripted DR failover.

```go
package main

import (
	"strings"
	"testing"
)

func TestTokenizationAndMasking(t *testing.T) {
	// Vault fails closed without key material; provision test keys.
	t.Setenv("SENTINEL_VAULT_HMAC_KEY", "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef")
	t.Setenv("SENTINEL_VAULT_AES_KEY", "fedcba9876543210fedcba9876543210fedcba9876543210fedcba9876543210")

	// 1. Test ABA Routing Number Tokenization
	routingRecord, _ := TokenizeField("TENANT-MERIDIAN-PROD", "ROUTING_NUMBER", "021000021")
	if routingRecord.MaskedValue != "0210****1" {
		t.Errorf("Expected masked routing 0210****1, got %s", routingRecord.MaskedValue)
	}
	if !strings.HasPrefix(routingRecord.TokenKey, "TOK-ROU-") {
		t.Errorf("Expected token key prefix TOK-ROU-, got %s", routingRecord.TokenKey)
	}

	// 2. Test Account Number Tokenization
	acctRecord, _ := TokenizeField("TENANT-MERIDIAN-PROD", "ACCOUNT_NUMBER", "12345678901842")
	if !strings.HasSuffix(acctRecord.MaskedValue, "1842") || !strings.HasPrefix(acctRecord.MaskedValue, "*") {
		t.Errorf("Expected format-preserving account mask, got %s", acctRecord.MaskedValue)
	}

	// 3. Test Individual Name Redaction
	nameRecord, _ := TokenizeField("TENANT-MERIDIAN-PROD", "INDIVIDUAL_NAME", "Johnathan Alexander Doe")
	if !strings.Contains(nameRecord.MaskedValue, "J***") {
		t.Errorf("Expected initial character preserved with asterisk mask, got %s", nameRecord.MaskedValue)
	}
}

func TestInstantPaymentFedNowValidation(t *testing.T) {
	sampleFedNowXml := `<?xml version="1.0" encoding="UTF-8"?>
<Document xmlns="urn:iso:std:iso:20022:tech:xsd:pacs.008.001.08">
  <FIToFICstmrCdtTrf>
    <GrpHdr>
      <MsgId>FEDNOW-2026-MSG-001</MsgId>
      <CreDtTm>2026-08-14T10:00:00Z</CreDtTm>
      <NbOfTxs>1</NbOfTxs>
      <SttlmInf><SttlmMtd>CLRG</SttlmMtd></SttlmInf>
    </GrpHdr>
    <CdtTrfTxInf>
      <PmtId><EndToEndId>E2E-FEDNOW-8891</EndToEndId></PmtId>
      <IntrBkSttlmAmt Ccy="USD">150000.00</IntrBkSttlmAmt>
    </CdtTrfTxInf>
  </FIToFICstmrCdtTrf>
</Document>`

	tx, _ := ValidateInstantPaymentXml(sampleFedNowXml)

	if tx.Network != NetFedNow {
		t.Errorf("Expected FEDNOW network, got %s", tx.Network)
	}

	if tx.ValidationLatencyMs > tx.SlaThresholdMs {
		t.Errorf("Validation latency %.2f ms exceeded SLA threshold %.2f ms", tx.ValidationLatencyMs, tx.SlaThresholdMs)
	}
}

func TestDisasterRecoveryFailoverSimulation(t *testing.T) {
	result := SimulateCrossRegionFailover()

	if !result.IsScriptedDemo || result.Disclaimer == "" {
		t.Errorf("Failover result must be explicitly marked as a scripted demo, got %+v", result)
	}
	if result.RpoSecondsTarget != 0.00 {
		t.Errorf("Expected RPO TARGET = 0.00, got %.2f", result.RpoSecondsTarget)
	}

	if result.DataLossTransactionCount != 0 {
		t.Errorf("Expected 0 lost transactions in simulated failover, got %d", result.DataLossTransactionCount)
	}

	if result.StandbyHealthStatus != "NOT_PROVISIONED" {
		t.Errorf("Expected standby status NOT_PROVISIONED (no replica exists), got %s", result.StandbyHealthStatus)
	}
}
```

---

## `gateway/anomaly_test.go`

**Lines:** 63  ·  **Reason for removal:** Covered the hardcoded-baseline anomaly evaluation and the deleted SQL console guardrails.

```go
package main

import (
	"database/sql"
	"strings"
	"testing"
	_ "modernc.org/sqlite"
)

func TestVolumeAnomalyDetection(t *testing.T) {
	baseline := BaselineStats{
		MeanRecords:   10000.0,
		StdDevRecords: 1500.0,
		MeanBytes:     940000.0,
		StdDevBytes:   141000.0,
		SampleCount:   30,
	}

	// Case 1: Normal volume (10,500 records -> Z = +0.33)
	normalFinding := EvaluateVolumeAnomaly(10500, 987000, baseline)
	if normalFinding.IsAnomaly || normalFinding.Severity != "INFO" {
		t.Errorf("Expected normal volume to NOT be an anomaly, got: %+v", normalFinding)
	}

	// Case 2: Spike volume (16,000 records -> Z = +4.00 > 3.0σ)
	spikeFinding := EvaluateVolumeAnomaly(16000, 1504000, baseline)
	if !spikeFinding.IsAnomaly || spikeFinding.ZScore < 3.0 {
		t.Errorf("Expected 16,000 records to trigger volume anomaly, got: %+v", spikeFinding)
	}

	// Case 3: Severe drop volume (2,000 records -> Z = -5.33 < -3.0σ)
	dropFinding := EvaluateVolumeAnomaly(2000, 188000, baseline)
	if !dropFinding.IsAnomaly || dropFinding.Severity != "CRITICAL" {
		t.Errorf("Expected severe volume drop to trigger CRITICAL anomaly, got: %+v", dropFinding)
	}
}

func TestSqlConsoleSecurityGuardrails(t *testing.T) {
	db, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatalf("Failed to open in-memory database: %v", err)
	}
	defer db.Close()

	_, _ = db.Exec("CREATE TABLE test_table (id INTEGER PRIMARY KEY, name TEXT);")
	_, _ = db.Exec("INSERT INTO test_table (name) VALUES ('Test Record');")

	// Valid SELECT Query
	rows, err := db.Query("SELECT id, name FROM test_table")
	if err != nil {
		t.Fatalf("Expected valid SELECT query, got error: %v", err)
	}
	rows.Close()

	// Prohibited DROP attempt validation logic
	prohibitedQuery := "DROP TABLE test_table;"
	trimmed := strings.ToUpper(strings.TrimSpace(prohibitedQuery))
	if strings.HasPrefix(trimmed, "DROP") {
		// correctly identified and rejected
	} else {
		t.Errorf("Failed to detect prohibited DROP query")
	}
}
```

---

## `gateway/webhook_test.go`

**Lines:** 59  ·  **Reason for removal:** Covered HMAC dispatch for the deleted webhook surface.

```go
package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestWebhookHmacComputationAndDispatch(t *testing.T) {
	secret := "test-secret-key-9988"
	event := WebhookDeliveryEvent{
		EventID:       "EVT-TEST-001",
		EventType:     "FILE_VALIDATED_AND_RELEASED",
		TimestampUtc:  "2026-08-14T10:00:00Z",
		TenantID:      "TENANT-DEFAULT",
		PayloadDigest: "e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855",
		Data: map[string]interface{}{
			"filename": "MERIDIAN_ACH_20260814.ach",
			"status":   "RELEASED",
		},
	}

	payloadBytes, _ := json.Marshal(event)
	expectedSig := ComputeWebhookHmac(payloadBytes, secret)

	if len(expectedSig) != 64 {
		t.Errorf("Expected 64-char hex HMAC signature, got length %d", len(expectedSig))
	}

	// Create test HTTP receiver server
	var receivedSignature string
	var receivedEventHeader string
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		receivedSignature = r.Header.Get("X-Sentinel-Signature")
		receivedEventHeader = r.Header.Get("X-Sentinel-Event")
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{"received": true}`))
	}))
	defer ts.Close()

	log, err := DispatchWebhookEvent(ts.URL, secret, event)
	if err != nil {
		t.Fatalf("Unexpected webhook dispatch error: %v", err)
	}

	if log.Status != "DELIVERED" || log.ResponseCode != 200 {
		t.Errorf("Expected DELIVERED 200, got status=%s code=%d", log.Status, log.ResponseCode)
	}

	if receivedEventHeader != "FILE_VALIDATED_AND_RELEASED" {
		t.Errorf("Expected event header FILE_VALIDATED_AND_RELEASED, got %s", receivedEventHeader)
	}

	if !strings.HasSuffix(receivedSignature, expectedSig) {
		t.Errorf("Expected signature header to contain %s, got %s", expectedSig, receivedSignature)
	}
}
```

---

# Removed route blocks from `gateway/main.go`

Extracted verbatim at commit `cb09694` before deletion.

---

### `POST /api/v1/sql/query` — arbitrary SQL console

**Origin:** `gateway/main.go:720-795` (76 lines)  ·  **Reason:** A read-only handle prevents writes but not column selection: `SELECT url, secret FROM webhook_subscriptions` returned live secrets (runtime-reproduced).

```go
		// POST /api/v1/sql/query (Read-only query runner)
		r.Post("/sql/query", func(w http.ResponseWriter, r *http.Request) {
			var body struct {
				Query string `json:"query"`
			}
			if err := json.NewDecoder(r.Body).Decode(&body); err != nil || body.Query == "" {
				http.Error(w, "missing query in request body", http.StatusBadRequest)
				return
			}

			trimmedQuery := strings.TrimSpace(strings.ToUpper(body.Query))
			if !strings.HasPrefix(trimmedQuery, "SELECT") && !strings.HasPrefix(trimmedQuery, "EXPLAIN") && !strings.HasPrefix(trimmedQuery, "PRAGMA") {
				http.Error(w, "permission denied: only read-only SELECT/EXPLAIN queries are permitted in audit console", http.StatusForbidden)
				return
			}

			// Block mutation keywords
			forbidden := []string{"DROP", "DELETE", "UPDATE", "INSERT", "ALTER", "CREATE", "TRUNCATE",
				"REPLACE", "ATTACH", "DETACH", "VACUUM", "REINDEX", "LOAD_EXTENSION", "WRITABLE_SCHEMA"}
			for _, word := range forbidden {
				pattern := `\b` + word + `\b`
				if matched, _ := regexp.MatchString(pattern, trimmedQuery); matched {
					http.Error(w, fmt.Sprintf("permission denied: mutating keyword '%s' is prohibited", word), http.StatusForbidden)
					return
				}
			}

			// Hard timeout: an unbounded cross join is a trivial DoS otherwise.
			ctx, cancel := context.WithTimeout(r.Context(), 10*time.Second)
			defer cancel()

			startTime := time.Now()
			rows, err := roDB.QueryContext(ctx, body.Query)
			if err != nil {
				http.Error(w, fmt.Sprintf("SQL error: %v", err), http.StatusBadRequest)
				return
			}
			defer rows.Close()

			cols, err := rows.Columns()
			if err != nil {
				http.Error(w, err.Error(), http.StatusInternalServerError)
				return
			}

			var results [][]interface{}
			for rows.Next() {
				colValues := make([]interface{}, len(cols))
				colPointers := make([]interface{}, len(cols))
				for i := range colValues {
					colPointers[i] = &colValues[i]
				}

				if err := rows.Scan(colPointers...); err == nil {
					for i, val := range colValues {
						if b, ok := val.([]byte); ok {
							colValues[i] = string(b)
						}
					}
					results = append(results, colValues)
				}
			}
			if results == nil {
				results = [][]interface{}{}
			}

			durationMs := float64(time.Since(startTime).Microseconds()) / 1000.0

			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(map[string]interface{}{
				"columns":    cols,
				"rows":       results,
				"rowCount":   len(results),
				"durationMs": durationMs,
			})
		})
```

### `GET /api/v1/webhooks` — plaintext secret listing

**Origin:** `gateway/main.go:625-649` (25 lines)  ·  **Reason:** Returned the stored secret to every caller.

```go
		// GET /api/v1/webhooks
		r.Get("/webhooks", func(w http.ResponseWriter, r *http.Request) {
			rows, err := db.Query("SELECT id, url, secret, events, status, created_at FROM webhook_subscriptions ORDER BY id DESC")
			if err != nil {
				http.Error(w, err.Error(), http.StatusInternalServerError)
				return
			}
			defer rows.Close()

			var webhooks []WebhookSubscription
			for rows.Next() {
				var wh WebhookSubscription
				var eventsJson, createdAtStr string
				if err := rows.Scan(&wh.ID, &wh.URL, &wh.Secret, &eventsJson, &wh.Status, &createdAtStr); err == nil {
					_ = json.Unmarshal([]byte(eventsJson), &wh.Events)
					wh.CreatedAt, _ = time.Parse(time.RFC3339, createdAtStr)
					webhooks = append(webhooks, wh)
				}
			}
			if webhooks == nil {
				webhooks = []WebhookSubscription{}
			}
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(webhooks)
		})
```

### `POST /api/v1/webhooks` — secret creation

**Origin:** `gateway/main.go:651-682` (32 lines)  ·  **Reason:** Returned the secret in the response and generated it from `time.Now().UnixNano()`.

```go
		// POST /api/v1/webhooks
		r.Post("/webhooks", func(w http.ResponseWriter, r *http.Request) {
			var body struct {
				URL    string   `json:"url"`
				Secret string   `json:"secret"`
				Events []string `json:"events"`
			}
			if err := json.NewDecoder(r.Body).Decode(&body); err != nil || body.URL == "" {
				http.Error(w, "invalid webhook payload", http.StatusBadRequest)
				return
			}
			if body.Secret == "" {
				body.Secret = "whsec_" + fmt.Sprintf("%x", time.Now().UnixNano())
			}
			if len(body.Events) == 0 {
				body.Events = []string{"ALL"}
			}
			eventsJson, _ := json.Marshal(body.Events)
			res, err := db.Exec("INSERT INTO webhook_subscriptions (url, secret, events, status) VALUES (?, ?, ?, 'ACTIVE')", body.URL, body.Secret, string(eventsJson))
			if err != nil {
				http.Error(w, err.Error(), http.StatusInternalServerError)
				return
			}
			id, _ := res.LastInsertId()
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(map[string]interface{}{
				"id":     id,
				"url":    body.URL,
				"secret": body.Secret,
				"status": "ACTIVE",
			})
		})
```

### `POST /api/v1/webhooks/test` — SSRF sink

**Origin:** `gateway/main.go:684-718` (35 lines)  ·  **Reason:** Fetched any caller-supplied URL. A request to 169.254.169.254 was issued by the application.

```go
		// POST /api/v1/webhooks/test
		r.Post("/webhooks/test", func(w http.ResponseWriter, r *http.Request) {
			var body struct {
				URL    string `json:"url"`
				Secret string `json:"secret"`
			}
			_ = json.NewDecoder(r.Body).Decode(&body)
			if body.URL == "" {
				http.Error(w, "missing URL", http.StatusBadRequest)
				return
			}
			if body.Secret == "" {
				body.Secret = "whsec_test_secret"
			}
			event := WebhookDeliveryEvent{
				EventID:       fmt.Sprintf("EVT-TEST-%d", time.Now().Unix()),
				EventType:     "GATEWAY_PING_TEST",
				TimestampUtc:  time.Now().UTC().Format(time.RFC3339),
				TenantID:      "TENANT-DEFAULT",
				PayloadDigest: "0000000000000000000000000000000000000000000000000000000000000000",
				Data: map[string]interface{}{
					"message": "Sentinel Flow Webhook Ping Confirmation",
					"gateway": "Sentinel Flow v1.0.0",
				},
			}
			logRes, err := DispatchWebhookEvent(body.URL, body.Secret, event)
			if err != nil {
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(http.StatusBadGateway)
				json.NewEncoder(w).Encode(logRes)
				return
			}
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(logRes)
		})
```

### `POST /api/v1/chaos/trigger` — scripted incident injection

**Origin:** `gateway/main.go:807-849` (43 lines)  ·  **Reason:** Wrote a fabricated SLA-breach audit event including a hardcoded 4.8ms recovery latency.

```go
		// POST Chaos Trigger
		r.Post("/chaos/trigger", func(w http.ResponseWriter, r *http.Request) {
			var body struct {
				Scenario string `json:"scenario"` // MISSING_FILE, WORKER_CRASH, RESET
			}
			_ = json.NewDecoder(r.Body).Decode(&body)

			switch body.Scenario {
			case "MISSING_FILE":
				// Mark expectation as OVERDUE and create incident
				_, _ = db.Exec("UPDATE expectations SET status = 'OVERDUE' WHERE id = 1")
				now := time.Now().UTC().Format(time.RFC3339)
				res, _ := db.Exec(`
					INSERT INTO incidents (expectation_id, type, severity, status, created_at, updated_at)
					VALUES (1, 'MISSING_FILE_DEADLINE', 'CRITICAL', 'OPEN', ?, ?)
				`, now, now)
				incID, _ := res.LastInsertId()

				_, _ = AppendAuditEvent(db, "SLA_BREACH_DETECTED", "DEADLINE_SCHEDULER_DAEMON", map[string]interface{}{
					"incidentId":  incID,
					"partner":     "Central Clearing Network",
					"cutoffTime":  "16:45:00 UTC",
					"explanation": "Expected delivery window expired +15m grace window without file arrival.",
				})

				w.Header().Set("Content-Type", "application/json")
				w.Write([]byte(fmt.Sprintf(`{"status": "TRIGGERED", "scenario": "MISSING_FILE", "incidentId": %d}`, incID)))
				return

			case "WORKER_CRASH":
				_, _ = AppendAuditEvent(db, "WORKER_CRASH_RECOVERY", "WATCHDOG_DAEMON", map[string]interface{}{
					"signal":          "SIGKILL",
					"reacquiredLease": true,
					"recoveryLatencyMs": 4.8,
				})
				w.Header().Set("Content-Type", "application/json")
				w.Write([]byte(`{"status": "TRIGGERED", "scenario": "WORKER_CRASH"}`))
				return

			default:
				http.Error(w, "Unknown chaos scenario", http.StatusBadRequest)
			}
		})
```

### `GET /api/v1/analytics/anomalies` — hardcoded evaluation

**Origin:** `gateway/main.go:797-805` (9 lines)  ·  **Reason:** Passed two literals into the detector on every call.

```go
		// GET /api/v1/analytics/anomalies
		r.Get("/analytics/anomalies", func(w http.ResponseWriter, r *http.Request) {
			finding := EvaluateVolumeAnomaly(15200, 1428800, DefaultBaseline)
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(map[string]interface{}{
				"baseline":          DefaultBaseline,
				"currentEvaluation": finding,
			})
		})
```

### Fabricated AI triage fallback

**Origin:** `gateway/main.go:466-489` (24 lines)  ·  **Reason:** Invented a summary, two Nacha citations, two proposed actions, confidence 0.94, and a fixed token/cost block whenever the AI tier was unreachable — which is the container default.

```go
			} else {
				// Fallback offline deterministic model
				aiRes = AnalystResponse{
					Summary: fmt.Sprintf("Automated Eliza 2.0 triage on Incident #%d identified 10-digit Entry Hash mismatch and out-of-balance control records.", incID),
					Citations: []string{
						"Nacha Operating Rules 2025, Article Two, Subsection 2.2.1: Entry Hash Verification",
						"Runbook RB-ACH-01: Hash Mismatch Counterparty Escalation",
					},
					ProposedActions: []ActionProposal{
						{Type: "REQUEST_PARTNER_RESEND", Description: "Draft formal notice to partner operations demanding re-transmission with corrected trailer controls."},
						{Type: "SUPERVISOR_SIGN_OFF", Description: "Require dual-control authorization before applying any exceptional settlement waiver."},
					},
					Confidence:   0.94,
					AgentVersion: "Eliza 2.0 RRR Standard",
				}
			}

			aiRes.AgentVersion = "Eliza 2.0 RRR Agentic AI"
			aiRes.Metrics = map[string]interface{}{
				"durationMs":       128,
				"inputTokens":      420,
				"outputTokens":     195,
				"estimatedCostUsd": 0.00042,
			}
```

### Fabricated AI evaluation fallback

**Origin:** `gateway/main.go:563-573` (11 lines)  ·  **Reason:** Returned passRatePct 100.0 with 5/5 passed whenever the Python evaluator was unreachable.

```go
			// Fallback if Python tier not reachable
			w.Header().Set("Content-Type", "application/json")
			w.Write([]byte(`{
				"suite": "Eliza 2.0 Adversarial Prompt Injection & Guardrail Eval",
				"totalTests": 5,
				"passedTests": 5,
				"passRatePct": 100.0,
				"unauthorizedExecutions": 0,
				"averageLatencyMs": 14.2,
				"evaluatedAtUtc": "` + time.Now().UTC().Format(time.RFC3339) + `"
			}`))
```

### `GET /api/v1/benchmark/run` — production benchmark route

**Origin:** `gateway/main.go:539-551` (13 lines)  ·  **Reason:** Benchmark harness moved to test-only scope; Prompt 13 builds the reproducible harness.

```go
		// GET Benchmark Run
		r.Get("/benchmark/run", func(w http.ResponseWriter, r *http.Request) {
			recStr := r.URL.Query().Get("records")
			recCount := 25000
			if recStr != "" {
				if n, err := strconv.Atoi(recStr); err == nil && n > 0 {
					recCount = n
				}
			}
			metrics := RunStreamingBenchmark(recCount)
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(metrics)
		})
```

### SLA breach-risk constants

**Origin:** `gateway/main.go:229-235` (7 lines)  ·  **Reason:** BreachRiskPct 98.4/12.5 and CountdownMinutes -15/23 assigned by an if-statement on status.

```go
				if exp.Status == "OVERDUE" {
					exp.BreachRiskPct = 98.4
					exp.CountdownMinutes = -15
				} else {
					exp.BreachRiskPct = 12.5
					exp.CountdownMinutes = 23
				}
```



---

## Prompt 12 — the in-memory event broadcaster

**Origin:** `gateway/stream.go` (99 lines, whole file) · **Source commit:** `2d9107b`
**Reason:** an untenanted in-process fan-out with no cursor, no persistence and no
publisher. `GlobalBroadcaster.Broadcast` had no caller anywhere in the tree, so the
only thing `GET /api/v1/stream` ever emitted was a literal
`{"status":"CONNECTED","stream":"SENTINEL_REALTIME_BUS"}` — a live-looking
connection carrying no events. Had a publisher ever been wired up, every
subscriber would have received every tenant's events: the fan-out iterated its
whole client map and had no notion of who was listening.

Replaced by a reader over `outbox_events`, which is durable, tenant-scoped,
immutable by trigger, and carries a monotonic id that is exactly a last-event
cursor.

```go
package main

import (
	"encoding/json"
	"fmt"
	"net/http"
	"sync"
	"time"

	"github.com/go-chi/chi/v5"
)

type StreamEvent struct {
	ID        string      `json:"id"`
	Type      string      `json:"type"` // "FILE_INGESTED", "ANOMALY_DETECTED", "LEDGER_COMMITTED", "SWARM_MESSAGE"
	Data      interface{} `json:"data"`
	Timestamp time.Time   `json:"timestamp"`
}

type EventBroadcaster struct {
	mu      sync.RWMutex
	clients map[chan StreamEvent]bool
}

var GlobalBroadcaster = &EventBroadcaster{
	clients: make(map[chan StreamEvent]bool),
}

func (b *EventBroadcaster) Subscribe() chan StreamEvent {
	b.mu.Lock()
	defer b.mu.Unlock()
	ch := make(chan StreamEvent, 10)
	b.clients[ch] = true
	return ch
}

func (b *EventBroadcaster) Unsubscribe(ch chan StreamEvent) {
	b.mu.Lock()
	defer b.mu.Unlock()
	delete(b.clients, ch)
	close(ch)
}

func (b *EventBroadcaster) Broadcast(eventType string, data interface{}) {
	b.mu.RLock()
	defer b.mu.RUnlock()

	event := StreamEvent{
		ID:        fmt.Sprintf("EVT-%d", time.Now().UnixNano()),
		Type:      eventType,
		Data:      data,
		Timestamp: time.Now().UTC(),
	}

	for ch := range b.clients {
		select {
		case ch <- event:
		default:
		}
	}
}

// RegisterStreamRoutes wires Server-Sent Events into Chi router
func RegisterStreamRoutes(r chi.Router) {
	r.Get("/stream", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		w.Header().Set("Cache-Control", "no-cache")
		w.Header().Set("Connection", "keep-alive")

		flusher, ok := w.(http.Flusher)
		if !ok {
			http.Error(w, "Streaming unsupported", http.StatusInternalServerError)
			return
		}

		ch := GlobalBroadcaster.Subscribe()
		defer GlobalBroadcaster.Unsubscribe(ch)

		// Send initial heartbeat
		initBytes, _ := json.Marshal(map[string]string{"status": "CONNECTED", "stream": "SENTINEL_REALTIME_BUS"})
		fmt.Fprintf(w, "event: connected\ndata: %s\n\n", string(initBytes))
		flusher.Flush()

		notify := r.Context().Done()
		for {
			select {
			case <-notify:
				return
			case event, ok := <-ch:
				if !ok {
					return
				}
				dataBytes, _ := json.Marshal(event)
				fmt.Fprintf(w, "event: %s\ndata: %s\n\n", event.Type, string(dataBytes))
				flusher.Flush()
			}
		}
	})
}
```
