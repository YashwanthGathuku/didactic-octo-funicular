package toolgateway

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"sentinel-gateway/internal/policy"
)

var (
	ErrInvocationNotFound = errors.New("tool gateway: invocation not found")
)

// InvocationRecord represents a persisted tool invocation.
type InvocationRecord struct {
	ID                  string
	TenantID            string
	ToolID              string
	ToolVersion         string
	ManifestHash        string
	CallerType          string
	CallerID            string
	CallerAutonomyLevel int
	WorkflowID          string
	IdempotencyKey      string
	RequestHash         string
	Status              InvocationStatus
	PolicyDecisionID    string
	PolicyDecisionHash  string
	PolicyBundleHash    string
	InputHash           string
	OutputHash          string
	OutputPayload       string
	ErrorCode           string
	ErrorMessage        string
	DurationMs          int64
	ExecutionMode       string
	CreatedAt           time.Time
	CompletedAt         *time.Time
}

// ToolStore provides durable database storage and outbox event journaling for tool invocations.
type ToolStore struct {
	db *sql.DB
}

// NewToolStore creates a new ToolStore.
func NewToolStore(db *sql.DB) *ToolStore {
	return &ToolStore{db: db}
}

// RecordInvocation persists a new tool invocation record and emits a transactional outbox event.
func (s *ToolStore) RecordInvocation(ctx context.Context, rec *InvocationRecord) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin tx: %w", err)
	}
	defer tx.Rollback()

	if err := s.RecordInvocationTx(ctx, tx, rec); err != nil {
		return err
	}

	return tx.Commit()
}

// RecordInvocationTx persists a tool invocation within a caller's transaction.
func (s *ToolStore) RecordInvocationTx(ctx context.Context, tx *sql.Tx, rec *InvocationRecord) error {
	if rec.ID == "" || rec.TenantID == "" || rec.ToolID == "" || rec.IdempotencyKey == "" {
		return errors.New("missing required fields in invocation record")
	}
	if rec.CreatedAt.IsZero() {
		rec.CreatedAt = time.Now().UTC()
	}

	query := `
		INSERT INTO tool_invocations (
			id, tenant_id, tool_id, tool_version, manifest_hash,
			caller_type, caller_id, caller_autonomy_level, workflow_id,
			idempotency_key, request_hash, status, policy_decision_id,
			policy_decision_hash, policy_bundle_hash, input_hash, output_hash,
			output_payload, error_code, error_message, duration_ms,
			execution_mode, created_at, completed_at
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT (tenant_id, caller_id, tool_id, tool_version, idempotency_key) DO UPDATE SET
			status = excluded.status,
			output_hash = excluded.output_hash,
			output_payload = excluded.output_payload,
			error_code = excluded.error_code,
			error_message = excluded.error_message,
			duration_ms = excluded.duration_ms,
			completed_at = excluded.completed_at`

	var completedAt sql.NullTime
	if rec.CompletedAt != nil {
		completedAt = sql.NullTime{Time: *rec.CompletedAt, Valid: true}
	}

	_, err := tx.ExecContext(ctx, query,
		rec.ID, rec.TenantID, rec.ToolID, rec.ToolVersion, rec.ManifestHash,
		rec.CallerType, rec.CallerID, rec.CallerAutonomyLevel, rec.WorkflowID,
		rec.IdempotencyKey, rec.RequestHash, string(rec.Status), rec.PolicyDecisionID,
		rec.PolicyDecisionHash, rec.PolicyBundleHash, rec.InputHash, rec.OutputHash,
		rec.OutputPayload, rec.ErrorCode, rec.ErrorMessage, rec.DurationMs,
		rec.ExecutionMode, rec.CreatedAt, completedAt,
	)
	if err != nil {
		return fmt.Errorf("insert tool_invocation: %w", err)
	}

	// Emit universal transactional outbox event
	eventType := "TOOL_INVOCATION_SUCCEEDED"
	if rec.Status == StatusDenied {
		eventType = "TOOL_INVOCATION_DENIED"
	} else if rec.Status == StatusFailed || rec.Status == StatusTimedOut {
		eventType = "TOOL_INVOCATION_FAILED"
	}

	eventPayload := map[string]interface{}{
		"invocation_id":        rec.ID,
		"tenant_id":            rec.TenantID,
		"tool_id":              rec.ToolID,
		"tool_version":         rec.ToolVersion,
		"manifest_hash":        rec.ManifestHash,
		"caller_type":          rec.CallerType,
		"caller_id":            rec.CallerID,
		"status":               string(rec.Status),
		"policy_decision_hash": rec.PolicyDecisionHash,
		"input_hash":           rec.InputHash,
		"output_hash":          rec.OutputHash,
		"error_code":           rec.ErrorCode,
		"duration_ms":          rec.DurationMs,
	}
	if rec.WorkflowID != "" {
		eventPayload["workflow_id"] = rec.WorkflowID
	}

	payloadBytes, _ := policy.CanonicalJSON(eventPayload)
	dedupeKey := fmt.Sprintf("tool-inv-%s", rec.ID)

	outboxQuery := `
		INSERT OR IGNORE INTO outbox_events (
			tenant_id, event_type, subject_type, subject_id, payload, dedupe_key, created_at
		) VALUES (?, ?, 'TOOL_INVOCATION', 0, ?, ?, ?)`

	_, _ = tx.ExecContext(ctx, outboxQuery,
		rec.TenantID, eventType, string(payloadBytes), dedupeKey, rec.CreatedAt,
	)

	return nil
}

// GetInvocationByIdempotency retrieves an existing invocation record by tenant, caller, tool ID, version, and idempotency key.
func (s *ToolStore) GetInvocationByIdempotency(
	ctx context.Context,
	tenantID, callerID, toolID, toolVersion, idempotencyKey string,
) (*InvocationRecord, error) {
	query := `
		SELECT id, tenant_id, tool_id, tool_version, manifest_hash,
		       caller_type, caller_id, caller_autonomy_level, workflow_id,
		       idempotency_key, request_hash, status, policy_decision_id,
		       policy_decision_hash, policy_bundle_hash, input_hash, output_hash,
		       COALESCE(output_payload, ''), COALESCE(error_code, ''), COALESCE(error_message, ''),
		       COALESCE(duration_ms, 0), execution_mode, created_at, completed_at
		FROM tool_invocations
		WHERE tenant_id = ? AND caller_id = ? AND tool_id = ? AND tool_version = ? AND idempotency_key = ?`

	var (
		rec         InvocationRecord
		statusStr   string
		completedAt sql.NullTime
		workflowID  sql.NullString
		policyDecID sql.NullString
		policyDecH  sql.NullString
		policyBndH  sql.NullString
		outputHash  sql.NullString
	)

	err := s.db.QueryRowContext(ctx, query, tenantID, callerID, toolID, toolVersion, idempotencyKey).Scan(
		&rec.ID, &rec.TenantID, &rec.ToolID, &rec.ToolVersion, &rec.ManifestHash,
		&rec.CallerType, &rec.CallerID, &rec.CallerAutonomyLevel, &workflowID,
		&rec.IdempotencyKey, &rec.RequestHash, &statusStr, &policyDecID,
		&policyDecH, &policyBndH, &rec.InputHash, &outputHash,
		&rec.OutputPayload, &rec.ErrorCode, &rec.ErrorMessage,
		&rec.DurationMs, &rec.ExecutionMode, &rec.CreatedAt, &completedAt,
	)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, ErrInvocationNotFound
		}
		return nil, fmt.Errorf("query tool_invocation: %w", err)
	}

	rec.Status = InvocationStatus(statusStr)
	if completedAt.Valid {
		rec.CompletedAt = &completedAt.Time
	}
	if workflowID.Valid {
		rec.WorkflowID = workflowID.String
	}
	if policyDecID.Valid {
		rec.PolicyDecisionID = policyDecID.String
	}
	if policyDecH.Valid {
		rec.PolicyDecisionHash = policyDecH.String
	}
	if policyBndH.Valid {
		rec.PolicyBundleHash = policyBndH.String
	}
	if outputHash.Valid {
		rec.OutputHash = outputHash.String
	}

	return &rec, nil
}

// ToToolResponse converts a persisted record into a ToolResponse.
func (rec *InvocationRecord) ToToolResponse() *ToolResponse {
	resp := &ToolResponse{
		InvocationID:       rec.ID,
		ToolID:             rec.ToolID,
		ToolVersion:        rec.ToolVersion,
		Status:             rec.Status,
		Duration:           time.Duration(rec.DurationMs) * time.Millisecond,
		PolicyDecisionHash: rec.PolicyDecisionHash,
		PolicyBundleHash:   rec.PolicyBundleHash,
		ManifestHash:       rec.ManifestHash,
		OutputHash:         rec.OutputHash,
		Timestamp:          rec.CreatedAt,
	}

	if rec.OutputPayload != "" {
		resp.Output = json.RawMessage(rec.OutputPayload)
		resp.OutputBytes = len(rec.OutputPayload)
	}

	if rec.ErrorCode != "" || rec.ErrorMessage != "" {
		resp.Error = &ToolError{
			Code:    rec.ErrorCode,
			Message: rec.ErrorMessage,
		}
	}

	return resp
}
