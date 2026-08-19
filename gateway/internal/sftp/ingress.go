package sftp

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"sentinel-gateway/internal/jobs"
	"sentinel-gateway/internal/ledger"
	"sentinel-gateway/internal/objectstore"
)

// IngressResult is the outcome of a webhook or reconciliation ingestion.
type IngressResult struct {
	ArtifactID   int64  `json:"artifact_id"`
	JobID        int64  `json:"job_id"`
	Deduplicated bool   `json:"deduplicated"`
	Filename     string `json:"filename"`
	SHA256Hash   string `json:"sha256_hash"`
	Status       string `json:"status"`
}

// IngressService processes finalized SFTP arrival events into SentinelFlow.
type IngressService struct {
	db            *sql.DB
	objectStore   objectstore.ObjectStore
	queue         *jobs.Queue
	evidence      *ledger.Ledger
	webhookSecret string
}

// NewIngressService builds the SFTP ingress service.
func NewIngressService(
	db *sql.DB,
	store objectstore.ObjectStore,
	q *jobs.Queue,
	ev *ledger.Ledger,
	secret string,
) *IngressService {
	return &IngressService{
		db:            db,
		objectStore:   store,
		queue:         q,
		evidence:      ev,
		webhookSecret: secret,
	}
}

// HandleWebhook processes an incoming authenticated SFTP finalized upload notification.
func (s *IngressService) HandleWebhook(
	ctx context.Context,
	tenantID string,
	signature string,
	timestamp int64,
	rawBody []byte,
) (*IngressResult, error) {
	if tenantID == "" {
		return nil, errors.New("missing tenant ID")
	}

	// 1. Authenticate webhook signature if secret is configured
	if s.webhookSecret != "" {
		if err := VerifySignature(s.webhookSecret, signature, timestamp, rawBody, 5*time.Minute); err != nil {
			return nil, fmt.Errorf("authentication failed: %w", err)
		}
	}

	// 2. Decode and validate finalized event
	var event FinalizedUploadEvent
	if err := json.Unmarshal(rawBody, &event); err != nil {
		return nil, fmt.Errorf("malformed event json: %w", err)
	}
	if err := event.Validate(); err != nil {
		return nil, fmt.Errorf("event validation rejected: %w", err)
	}

	dedupeKey := event.DedupeKey(tenantID)

	// 3. Check for duplicate arrival (Idempotency)
	var existingArtifactID int64
	var existingStatus string
	err := s.db.QueryRowContext(ctx, `
		SELECT id, status FROM file_instances 
		WHERE tenant_id = ? AND sha256_hash = ?
	`, tenantID, event.SHA256Hash).Scan(&existingArtifactID, &existingStatus)

	if err == nil && existingArtifactID > 0 {
		var existingJobID int64
		_ = s.db.QueryRowContext(ctx, `
			SELECT id FROM ingestion_jobs 
			WHERE tenant_id = ? AND file_instance_id = ?
		`, tenantID, existingArtifactID).Scan(&existingJobID)

		return &IngressResult{
			ArtifactID:   existingArtifactID,
			JobID:        existingJobID,
			Deduplicated: true,
			Filename:     event.Filename(),
			SHA256Hash:   event.SHA256Hash,
			Status:       existingStatus,
		}, nil
	}

	// 4. Record new file instance and job in a transaction
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, fmt.Errorf("begin transaction: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	storagePath := event.FSPath
	if storagePath == "" {
		storagePath = event.VirtualPath
	}

	res, err := tx.ExecContext(ctx, `
		INSERT INTO file_instances (
			tenant_id, filename, storage_path, size_bytes, sha256_hash, status, received_at, created_at, updated_at
		) VALUES (?, ?, ?, ?, ?, 'RECEIVED', CURRENT_TIMESTAMP, CURRENT_TIMESTAMP, CURRENT_TIMESTAMP)
	`, tenantID, event.Filename(), storagePath, event.SizeBytes, event.SHA256Hash)
	if err != nil {
		return nil, fmt.Errorf("insert file_instance: %w", err)
	}

	artifactID, err := res.LastInsertId()
	if err != nil {
		return nil, fmt.Errorf("get artifact id: %w", err)
	}

	// Enqueue durable validation job
	jobID, err := s.queue.EnqueueTx(ctx, tx, jobs.EnqueueRequest{
		TenantID:       tenantID,
		Kind:           "VALIDATE_ARTIFACT",
		IdempotencyKey: dedupeKey,
		ArtifactID:     artifactID,
	})
	if err != nil {
		return nil, fmt.Errorf("enqueue ingestion job: %w", err)
	}

	if err := tx.Commit(); err != nil {
		return nil, fmt.Errorf("commit ingress: %w", err)
	}

	// 5. Stamp immutable audit ledger evidence
	if s.evidence != nil {
		_, _ = s.evidence.Append(ctx, ledger.AppendRequest{
			TenantID:   tenantID,
			Action:     "SFTP_FINALIZED_UPLOAD_INGESTED",
			Actor:      "SFTP_INGRESS_SERVICE",
			ObjectType: "file_instance",
			ObjectID:   artifactID,
			Payload: map[string]any{
				"artifactId":  artifactID,
				"jobId":       jobID,
				"filename":    event.Filename(),
				"sha256":      event.SHA256Hash,
				"sizeBytes":   event.SizeBytes,
				"virtualPath": event.VirtualPath,
				"username":    event.Username,
			},
		})
	}

	return &IngressResult{
		ArtifactID:   artifactID,
		JobID:        jobID,
		Deduplicated: false,
		Filename:     event.Filename(),
		SHA256Hash:   event.SHA256Hash,
		Status:       "RECEIVED",
	}, nil
}
