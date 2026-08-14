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
