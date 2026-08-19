package repository

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"sentinel-gateway/internal/domain"
)

var (
	// ErrWorkflowConflict is returned when an optimistic concurrency check fails.
	ErrWorkflowConflict = errors.New("agent workflow conflict: row_version mismatch")
)

// CreateWorkflow creates a new agent workflow record scoped to the tenant.
func (r *Repository) CreateWorkflow(ctx context.Context, s Scope, wf *domain.AgentWorkflow) error {
	if err := s.valid(); err != nil {
		return err
	}
	if wf.ID == "" {
		return errors.New("workflow id is required")
	}
	if wf.IncidentID <= 0 {
		return errors.New("invalid incident id")
	}
	if wf.ArtifactID <= 0 {
		return errors.New("invalid artifact id")
	}
	if wf.State == "" {
		wf.State = domain.WorkflowPending
	}
	if wf.WorkflowType == "" {
		wf.WorkflowType = "TRIAGE_AND_REMEDIATION"
	}
	if wf.CreatedAt.IsZero() {
		wf.CreatedAt = time.Now().UTC()
	}
	if wf.UpdatedAt.IsZero() {
		wf.UpdatedAt = wf.CreatedAt
	}

	wf.TenantID = s.tenantID
	wf.RowVersion = 1

	query := `
		INSERT INTO agent_workflows (
			id, tenant_id, incident_id, artifact_id, artifact_sha256,
			state, agent_name, agent_version, workflow_type,
			correlation_id, trace_id, row_version, error_detail,
			created_at, updated_at, started_at, completed_at
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`

	_, err := r.db.ExecContext(ctx, query,
		wf.ID, wf.TenantID, wf.IncidentID, wf.ArtifactID, wf.ArtifactSHA256,
		string(wf.State), wf.AgentName, wf.AgentVersion, wf.WorkflowType,
		wf.CorrelationID, wf.TraceID, wf.RowVersion, wf.ErrorDetail,
		wf.CreatedAt, wf.UpdatedAt, wf.StartedAt, wf.CompletedAt,
	)
	if err != nil {
		return fmt.Errorf("insert agent_workflow: %w", err)
	}
	return nil
}

// GetWorkflow fetches a workflow by ID within the tenant scope.
func (r *Repository) GetWorkflow(ctx context.Context, s Scope, workflowID string) (*domain.AgentWorkflow, error) {
	if err := s.valid(); err != nil {
		return nil, err
	}

	query := `
		SELECT id, tenant_id, incident_id, artifact_id, artifact_sha256,
		       state, agent_name, agent_version, workflow_type,
		       correlation_id, COALESCE(trace_id, ''), row_version, COALESCE(error_detail, ''),
		       created_at, updated_at, started_at, completed_at
		FROM agent_workflows
		WHERE id = ? AND tenant_id = ?`

	var (
		wf          domain.AgentWorkflow
		stStr       string
		traceID     string
		errDetail   string
		startedAt   sql.NullTime
		completedAt sql.NullTime
	)

	err := r.db.QueryRowContext(ctx, query, workflowID, s.tenantID).Scan(
		&wf.ID, &wf.TenantID, &wf.IncidentID, &wf.ArtifactID, &wf.ArtifactSHA256,
		&stStr, &wf.AgentName, &wf.AgentVersion, &wf.WorkflowType,
		&wf.CorrelationID, &traceID, &wf.RowVersion, &errDetail,
		&wf.CreatedAt, &wf.UpdatedAt, &startedAt, &completedAt,
	)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, ErrNotFound
		}
		return nil, fmt.Errorf("query agent_workflow: %w", err)
	}

	wf.State = domain.AgentWorkflowState(stStr)
	wf.TraceID = traceID
	wf.ErrorDetail = errDetail
	if startedAt.Valid {
		wf.StartedAt = &startedAt.Time
	}
	if completedAt.Valid {
		wf.CompletedAt = &completedAt.Time
	}

	return &wf, nil
}

// TransitionWorkflowTx executes an atomic, crash-consistent, and idempotent state transition
// within a single database transaction, recording the domain event in agent_workflow_events.
func (r *Repository) TransitionWorkflowTx(
	ctx context.Context,
	s Scope,
	workflowID string,
	expectedVersion int,
	nextState domain.AgentWorkflowState,
	idempotencyKey string,
	errorDetail string,
	eventPayload map[string]interface{},
) (*domain.AgentWorkflow, error) {
	if err := s.valid(); err != nil {
		return nil, err
	}
	if idempotencyKey == "" {
		return nil, errors.New("idempotency_key is required")
	}

	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, fmt.Errorf("begin tx: %w", err)
	}
	defer tx.Rollback()

	// 1. Check if this exact idempotency key was already committed (idempotent replay)
	var existingEventID string
	err = tx.QueryRowContext(ctx, `
		SELECT id FROM agent_workflow_events
		WHERE tenant_id = ? AND workflow_id = ? AND idempotency_key = ?`,
		s.tenantID, workflowID, idempotencyKey,
	).Scan(&existingEventID)

	if err == nil {
		// Found existing event with same key: return current workflow state without re-transitioning
		_ = tx.Rollback()
		return r.GetWorkflow(ctx, s, workflowID)
	} else if !errors.Is(err, sql.ErrNoRows) {
		return nil, fmt.Errorf("check idempotency key: %w", err)
	}

	// 2. Fetch current workflow row with lock
	query := `
		SELECT id, tenant_id, incident_id, artifact_id, artifact_sha256,
		       state, agent_name, agent_version, workflow_type,
		       correlation_id, COALESCE(trace_id, ''), row_version, COALESCE(error_detail, ''),
		       created_at, updated_at, started_at, completed_at
		FROM agent_workflows
		WHERE id = ? AND tenant_id = ?`

	var (
		wf          domain.AgentWorkflow
		stStr       string
		traceID     string
		curErrDet   string
		startedAt   sql.NullTime
		completedAt sql.NullTime
	)

	err = tx.QueryRowContext(ctx, query, workflowID, s.tenantID).Scan(
		&wf.ID, &wf.TenantID, &wf.IncidentID, &wf.ArtifactID, &wf.ArtifactSHA256,
		&stStr, &wf.AgentName, &wf.AgentVersion, &wf.WorkflowType,
		&wf.CorrelationID, &traceID, &wf.RowVersion, &curErrDet,
		&wf.CreatedAt, &wf.UpdatedAt, &startedAt, &completedAt,
	)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, ErrNotFound
		}
		return nil, fmt.Errorf("query agent_workflow for update: %w", err)
	}

	wf.State = domain.AgentWorkflowState(stStr)
	wf.TraceID = traceID
	wf.ErrorDetail = curErrDet
	if startedAt.Valid {
		wf.StartedAt = &startedAt.Time
	}
	if completedAt.Valid {
		wf.CompletedAt = &completedAt.Time
	}

	// 3. Check optimistic concurrency version
	if wf.RowVersion != expectedVersion {
		return nil, ErrWorkflowConflict
	}

	// 4. Validate transition in domain state machine
	newState, err := domain.TransitionAgentWorkflow(wf.State, nextState)
	if err != nil {
		return nil, err
	}

	now := time.Now().UTC()
	var newCompletedAt *time.Time
	if domain.IsTerminalAgentWorkflow(newState) {
		newCompletedAt = &now
	}

	var newStartedAt *time.Time
	if wf.StartedAt == nil && newState != domain.WorkflowPending {
		newStartedAt = &now
	} else {
		newStartedAt = wf.StartedAt
	}

	// 5. Update agent_workflows
	updateQuery := `
		UPDATE agent_workflows
		SET state = ?,
		    row_version = row_version + 1,
		    error_detail = ?,
		    updated_at = ?,
		    started_at = COALESCE(started_at, ?),
		    completed_at = COALESCE(completed_at, ?)
		WHERE id = ? AND tenant_id = ? AND row_version = ?`

	res, err := tx.ExecContext(ctx, updateQuery,
		string(newState), errorDetail, now, newStartedAt, newCompletedAt,
		workflowID, s.tenantID, expectedVersion,
	)
	if err != nil {
		return nil, fmt.Errorf("update agent_workflow in tx: %w", err)
	}
	affected, _ := res.RowsAffected()
	if affected == 0 {
		return nil, ErrWorkflowConflict
	}

	// 6. Insert into agent_workflow_events within same atomic transaction
	eventPayloadJSON, _ := json.Marshal(eventPayload)
	eventID := fmt.Sprintf("wev-%s-%s", workflowID, idempotencyKey)
	eventInsert := `
		INSERT INTO agent_workflow_events (
			id, workflow_id, tenant_id, idempotency_key, event_type,
			state_from, state_to, row_version, payload, created_at
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`

	_, err = tx.ExecContext(ctx, eventInsert,
		eventID, workflowID, s.tenantID, idempotencyKey, "AGENT_WORKFLOW_STATE_CHANGED",
		string(wf.State), string(newState), expectedVersion+1, string(eventPayloadJSON), now,
	)
	if err != nil {
		return nil, fmt.Errorf("insert agent_workflow_event: %w", err)
	}

	// 7. Commit transaction
	if err := tx.Commit(); err != nil {
		return nil, fmt.Errorf("commit transition tx: %w", err)
	}

	wf.State = newState
	wf.RowVersion = expectedVersion + 1
	wf.ErrorDetail = errorDetail
	wf.UpdatedAt = now
	wf.StartedAt = newStartedAt
	wf.CompletedAt = newCompletedAt

	return &wf, nil
}

// RecordRun records a single agent run within a workflow with typed telemetry.
func (r *Repository) RecordRun(ctx context.Context, s Scope, run *domain.AgentRun) error {
	if err := s.valid(); err != nil {
		return err
	}
	if run.ID == "" || run.WorkflowID == "" {
		return errors.New("run id and workflow id are required")
	}

	run.TenantID = s.tenantID
	if run.StartedAt.IsZero() {
		run.StartedAt = time.Now().UTC()
	}

	query := `
		INSERT INTO agent_runs (
			id, workflow_id, tenant_id, agent_name, agent_version,
			provider, model_name, model_version, status,
			input_tokens, output_tokens, latency_ms, estimated_cost_microusd,
			pricing_version, error_message, started_at, completed_at
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`

	_, err := r.db.ExecContext(ctx, query,
		run.ID, run.WorkflowID, run.TenantID, run.AgentName, run.AgentVersion,
		run.Provider, run.ModelName, run.ModelVersion, run.Status,
		run.InputTokens, run.OutputTokens, run.LatencyMs, run.EstimatedCostMicroUSD,
		run.PricingVersion, run.ErrorMessage, run.StartedAt, run.CompletedAt,
	)
	if err != nil {
		return fmt.Errorf("insert agent_run: %w", err)
	}
	return nil
}

// RecordStep records an agent step within a workflow (strictly structured, non-CoT).
func (r *Repository) RecordStep(ctx context.Context, s Scope, step *domain.AgentStep) error {
	if err := s.valid(); err != nil {
		return err
	}
	if step.ID == "" || step.RunID == "" || step.WorkflowID == "" {
		return errors.New("step id, run id and workflow id are required")
	}

	step.TenantID = s.tenantID
	if step.CreatedAt.IsZero() {
		step.CreatedAt = time.Now().UTC()
	}
	if step.StepStatus == "" {
		step.StepStatus = "COMPLETED"
	}

	evidenceJoined := strings.Join(step.AuthorizedEvidenceRefs, ",")

	query := `
		INSERT INTO agent_steps (
			id, run_id, workflow_id, tenant_id, step_number,
			step_type, state_from, state_to, decision_payload,
			authorized_evidence_refs, step_status, step_hash, latency_ms, created_at
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`

	_, err := r.db.ExecContext(ctx, query,
		step.ID, step.RunID, step.WorkflowID, step.TenantID, step.StepNumber,
		string(step.StepType), string(step.StateFrom), string(step.StateTo),
		step.DecisionPayload, evidenceJoined, step.StepStatus, step.StepHash,
		step.LatencyMs, step.CreatedAt,
	)
	if err != nil {
		return fmt.Errorf("insert agent_step: %w", err)
	}
	return nil
}

// RecordToolCall records an agent tool execution.
func (r *Repository) RecordToolCall(ctx context.Context, s Scope, call *domain.AgentToolCall) error {
	if err := s.valid(); err != nil {
		return err
	}
	if call.ID == "" || call.StepID == "" || call.WorkflowID == "" {
		return errors.New("call id, step id and workflow id are required")
	}

	call.TenantID = s.tenantID
	if call.ExecutedAt.IsZero() {
		call.ExecutedAt = time.Now().UTC()
	}

	isErrInt := 0
	if call.IsError {
		isErrInt = 1
	}

	query := `
		INSERT INTO agent_tool_calls (
			id, step_id, workflow_id, tenant_id, tool_name,
			tool_scope, input_redacted, output_redacted, is_error,
			latency_ms, executed_at
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`

	_, err := r.db.ExecContext(ctx, query,
		call.ID, call.StepID, call.WorkflowID, call.TenantID, call.ToolName,
		string(call.ToolScope), call.InputRedacted, call.OutputRedacted, isErrInt,
		call.LatencyMs, call.ExecutedAt,
	)
	if err != nil {
		return fmt.Errorf("insert agent_tool_call: %w", err)
	}
	return nil
}

// RecordAttestation records an independent verification attestation.
func (r *Repository) RecordAttestation(ctx context.Context, s Scope, att *domain.VerificationAttestation) error {
	if err := s.valid(); err != nil {
		return err
	}
	if att.ID == "" || att.WorkflowID == "" {
		return errors.New("attestation id and workflow id are required")
	}

	att.TenantID = s.tenantID
	if att.CreatedAt.IsZero() {
		att.CreatedAt = time.Now().UTC()
	}

	query := `
		INSERT INTO verification_attestations (
			id, workflow_id, tenant_id, verifier_agent,
			candidate_artifact_id, candidate_sha256, findings_count,
			blocking_findings_count, status, attestation_digest, created_at
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`

	_, err := r.db.ExecContext(ctx, query,
		att.ID, att.WorkflowID, att.TenantID, att.VerifierAgent,
		att.CandidateArtifactID, att.CandidateSHA256, att.FindingsCount,
		att.BlockingFindingsCount, att.Status, att.AttestationDigest, att.CreatedAt,
	)
	if err != nil {
		return fmt.Errorf("insert verification_attestation: %w", err)
	}
	return nil
}
