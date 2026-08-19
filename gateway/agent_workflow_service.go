package main

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	"sentinel-gateway/internal/auth"
	"sentinel-gateway/internal/domain"
	"sentinel-gateway/internal/repository"
)

// AgentWorkflowService manages the lifecycle, persistence, and audit logging
// for the AI Agent Control Plane.
type AgentWorkflowService struct {
	db   *sql.DB
	repo *repository.Repository
}

// NewAgentWorkflowService creates a new AgentWorkflowService.
func NewAgentWorkflowService(db *sql.DB) *AgentWorkflowService {
	return &AgentWorkflowService{
		db:   db,
		repo: repository.New(db),
	}
}

// scopeForTenant builds an authorized Scope for a tenant operation.
func (s *AgentWorkflowService) scopeForTenant(tenantID, actorID string) repository.Scope {
	if actorID == "" {
		actorID = "system/agent-coordinator"
	}
	p := &auth.Principal{
		Subject: actorID,
		Memberships: []auth.Membership{
			{
				TenantID: tenantID,
				Roles:    []auth.Role{auth.RoleOperator},
			},
		},
	}
	scope, _ := repository.NewScope(p, tenantID, auth.PermReadTenant)
	return scope
}

// CreateWorkflow initializes a new durable agent workflow and logs the creation in the audit ledger.
func (s *AgentWorkflowService) CreateWorkflow(
	ctx context.Context,
	tenantID string,
	incidentID int64,
	artifactID int64,
	artifactSHA256 string,
	agentName string,
	agentVersion string,
	corrID string,
	traceID string,
) (*domain.AgentWorkflow, error) {
	if tenantID == "" {
		return nil, errors.New("tenant_id is required")
	}
	if incidentID <= 0 {
		return nil, errors.New("incident_id must be greater than zero")
	}
	if artifactID <= 0 {
		return nil, errors.New("artifact_id must be greater than zero")
	}

	wfID := fmt.Sprintf("wf-%s-%d-%d", tenantID, incidentID, time.Now().UnixNano())
	if agentName == "" {
		agentName = "SentinelCoordinator"
	}
	if agentVersion == "" {
		agentVersion = "1.0.0"
	}
	if corrID == "" {
		corrID = fmt.Sprintf("corr-%s-%d", tenantID, time.Now().UnixNano())
	}

	wf := &domain.AgentWorkflow{
		ID:             wfID,
		TenantID:       tenantID,
		IncidentID:     incidentID,
		ArtifactID:     artifactID,
		ArtifactSHA256: artifactSHA256,
		State:          domain.WorkflowPending,
		AgentName:      agentName,
		AgentVersion:   agentVersion,
		WorkflowType:   "TRIAGE_AND_REMEDIATION",
		CorrelationID:  corrID,
		TraceID:        traceID,
		RowVersion:     1,
		CreatedAt:      time.Now().UTC(),
		UpdatedAt:      time.Now().UTC(),
	}

	scope := s.scopeForTenant(tenantID, "system/agent-coordinator")
	if err := s.repo.CreateWorkflow(ctx, scope, wf); err != nil {
		return nil, fmt.Errorf("create agent workflow: %w", err)
	}

	// Append creation event to immutable linear hash chain ledger
	_, _ = AppendAuditEvent(s.db, tenantID, "AGENT_WORKFLOW_CREATED", agentName, map[string]interface{}{
		"workflowId":     wf.ID,
		"incidentId":     wf.IncidentID,
		"artifactId":     wf.ArtifactID,
		"artifactSha256": wf.ArtifactSHA256,
		"state":          string(wf.State),
		"agentVersion":   wf.AgentVersion,
		"correlationId":  wf.CorrelationID,
	})

	return wf, nil
}

// GetWorkflow fetches a workflow by ID within a tenant boundary.
func (s *AgentWorkflowService) GetWorkflow(ctx context.Context, tenantID, workflowID string) (*domain.AgentWorkflow, error) {
	scope := s.scopeForTenant(tenantID, "system/agent-coordinator")
	return s.repo.GetWorkflow(ctx, scope, workflowID)
}

// TransitionWorkflow atomically advances workflow state with optimistic concurrency,
// transaction-level crash-consistency, and idempotency key deduplication.
func (s *AgentWorkflowService) TransitionWorkflow(
	ctx context.Context,
	tenantID string,
	workflowID string,
	expectedVersion int,
	nextState domain.AgentWorkflowState,
	idempotencyKey string,
	errorDetail string,
	rationale string,
) (*domain.AgentWorkflow, error) {
	scope := s.scopeForTenant(tenantID, "system/agent-coordinator")

	current, err := s.repo.GetWorkflow(ctx, scope, workflowID)
	if err != nil {
		return nil, err
	}

	stateFrom := current.State
	if idempotencyKey == "" {
		idempotencyKey = fmt.Sprintf("ik-%s-%s-%d", workflowID, nextState, expectedVersion)
	}

	payload := map[string]interface{}{
		"workflowId":     workflowID,
		"incidentId":     current.IncidentID,
		"artifactId":     current.ArtifactID,
		"stateFrom":      string(stateFrom),
		"stateTo":        string(nextState),
		"expectedVersion": expectedVersion,
		"rationale":      rationale,
		"errorDetail":    errorDetail,
	}

	// Perform atomic, crash-consistent, idempotent transition in a single database transaction
	updated, err := s.repo.TransitionWorkflowTx(ctx, scope, workflowID, expectedVersion, nextState, idempotencyKey, errorDetail, payload)
	if err != nil {
		return nil, err
	}

	// Append state transition audit event if state actually changed
	if stateFrom != updated.State {
		_, _ = AppendAuditEvent(s.db, tenantID, "AGENT_WORKFLOW_TRANSITIONED", updated.AgentName, map[string]interface{}{
			"workflowId":     workflowID,
			"incidentId":     updated.IncidentID,
			"artifactId":     updated.ArtifactID,
			"stateFrom":      string(stateFrom),
			"stateTo":        string(updated.State),
			"rowVersion":     updated.RowVersion,
			"idempotencyKey": idempotencyKey,
			"rationale":      rationale,
			"errorDetail":    errorDetail,
		})
	}

	return updated, nil
}
