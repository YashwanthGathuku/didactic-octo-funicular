package main

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"sentinel-gateway/internal/auth"
	"sentinel-gateway/internal/domain"
	"sentinel-gateway/internal/repository"
)

// AgentWorkflowService manages the authoritative lifecycle, persistence, and audit logging
// for the AI Agent Control Plane in Go.
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

// GetOrCreateWorkflowByTrigger implements durable trigger idempotency.
// Returns the existing workflow if (tenantID, triggerEventID, workflowType) already exists,
// or creates a new one in state PENDING.
func (s *AgentWorkflowService) GetOrCreateWorkflowByTrigger(
	ctx context.Context,
	tenantID string,
	triggerEventID string,
	workflowType string,
	incidentID int64,
	artifactID int64,
	artifactSHA256 string,
	policyBundleHash string,
	evidenceSetHash string,
	corrID string,
	traceID string,
) (*domain.AgentWorkflow, bool, error) {
	if tenantID == "" {
		return nil, false, errors.New("tenant_id is required")
	}
	if triggerEventID == "" {
		return nil, false, errors.New("trigger_event_id is required")
	}
	if workflowType == "" {
		workflowType = "ARTIFACT_QUARANTINED"
	}

	scope := s.scopeForTenant(tenantID, "system/agent-coordinator")

	// 1. Try to find existing workflow by trigger
	existing, err := s.repo.GetWorkflowByTrigger(ctx, scope, triggerEventID, workflowType)
	if err == nil {
		return existing, false, nil
	} else if !errors.Is(err, repository.ErrNotFound) {
		return nil, false, fmt.Errorf("lookup workflow by trigger: %w", err)
	}

	// 2. Allocate authoritative workflow ID in Go
	wfID := fmt.Sprintf("wf-%s-%s-%s", tenantID, strings.ToLower(triggerEventID), strings.ToLower(workflowType))
	if corrID == "" {
		corrID = fmt.Sprintf("corr-%s-%s", tenantID, triggerEventID)
	}

	wf := &domain.AgentWorkflow{
		ID:                        wfID,
		TenantID:                  tenantID,
		IncidentID:                incidentID,
		ArtifactID:                artifactID,
		ArtifactSHA256:            artifactSHA256,
		State:                     domain.WorkflowPending,
		AgentName:                 "IncidentCommanderAgent",
		AgentVersion:              "1.0.0",
		WorkflowType:              workflowType,
		TriggerEventID:            triggerEventID,
		PolicyBundleHash:          policyBundleHash,
		AuthorizedEvidenceSetHash: evidenceSetHash,
		CorrelationID:             corrID,
		TraceID:                   traceID,
		RowVersion:                1,
		CreatedAt:                 time.Now().UTC(),
		UpdatedAt:                 time.Now().UTC(),
	}

	if err := s.repo.CreateWorkflow(ctx, scope, wf); err != nil {
		// In case of a race condition on insert, retry lookup
		if existing, lookupErr := s.repo.GetWorkflowByTrigger(ctx, scope, triggerEventID, workflowType); lookupErr == nil {
			return existing, false, nil
		}
		return nil, false, fmt.Errorf("create agent workflow: %w", err)
	}

	// 3. Append creation event to domain event journal & audit ledger
	_ = s.RecordWorkflowEvent(ctx, tenantID, wfID, fmt.Sprintf("ev-created-%s", wfID), "WORKFLOW_CREATED", domain.WorkflowPending, domain.WorkflowPending, 1, map[string]interface{}{
		"triggerEventId": triggerEventID,
		"workflowType":   workflowType,
		"incidentId":     incidentID,
		"artifactId":     artifactID,
	})

	_, _ = AppendAuditEvent(s.db, tenantID, "AGENT_WORKFLOW_CREATED", wf.AgentName, map[string]interface{}{
		"workflowId":     wf.ID,
		"triggerEventId": triggerEventID,
		"incidentId":     wf.IncidentID,
		"artifactId":     wf.ArtifactID,
		"artifactSha256": wf.ArtifactSHA256,
		"state":          string(wf.State),
		"agentVersion":   wf.AgentVersion,
		"correlationId":  wf.CorrelationID,
	})

	return wf, true, nil
}

// CreateWorkflow initializes a new durable agent workflow (backward compatibility).
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
	trigID := fmt.Sprintf("trig-inc-%d-%d", incidentID, time.Now().UnixNano())
	wf, _, err := s.GetOrCreateWorkflowByTrigger(
		ctx, tenantID, trigID, "TRIAGE_AND_REMEDIATION", incidentID, artifactID, artifactSHA256,
		"default/1", "ev-hash-default", corrID, traceID,
	)
	return wf, err
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
		"workflowId":      workflowID,
		"incidentId":      current.IncidentID,
		"artifactId":      current.ArtifactID,
		"stateFrom":       string(stateFrom),
		"stateTo":         string(nextState),
		"expectedVersion": expectedVersion,
		"rationale":       rationale,
		"errorDetail":     errorDetail,
	}

	updated, err := s.repo.TransitionWorkflowTx(ctx, scope, workflowID, expectedVersion, nextState, idempotencyKey, errorDetail, payload)
	if err != nil {
		return nil, err
	}

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

// RecordWorkflowEvent records a domain event in agent_workflow_events.
func (s *AgentWorkflowService) RecordWorkflowEvent(
	ctx context.Context,
	tenantID string,
	workflowID string,
	idempotencyKey string,
	eventType string,
	stateFrom domain.AgentWorkflowState,
	stateTo domain.AgentWorkflowState,
	rowVersion int,
	payload map[string]interface{},
) error {
	scope := s.scopeForTenant(tenantID, "system/agent-coordinator")
	payloadBytes, _ := json.Marshal(payload)
	event := &domain.AgentWorkflowEvent{
		ID:             fmt.Sprintf("wev-%s-%s", workflowID, idempotencyKey),
		WorkflowID:     workflowID,
		TenantID:       tenantID,
		IdempotencyKey: idempotencyKey,
		EventType:      eventType,
		StateFrom:      stateFrom,
		StateTo:        stateTo,
		RowVersion:     rowVersion,
		Payload:        string(payloadBytes),
		CreatedAt:      time.Now().UTC(),
	}
	return s.repo.RecordWorkflowEvent(ctx, scope, event)
}

// RecordRun records an agent execution run in agent_runs.
func (s *AgentWorkflowService) RecordRun(ctx context.Context, tenantID string, run *domain.AgentRun) error {
	scope := s.scopeForTenant(tenantID, "system/agent-coordinator")
	return s.repo.RecordRun(ctx, scope, run)
}

// RecordStep records a structured execution step in agent_steps.
func (s *AgentWorkflowService) RecordStep(ctx context.Context, tenantID string, step *domain.AgentStep) error {
	scope := s.scopeForTenant(tenantID, "system/agent-coordinator")
	return s.repo.RecordStep(ctx, scope, step)
}

// GetSteps fetches all steps for a workflow.
func (s *AgentWorkflowService) GetSteps(ctx context.Context, tenantID, workflowID string) ([]domain.AgentStep, error) {
	scope := s.scopeForTenant(tenantID, "system/agent-coordinator")
	return s.repo.GetSteps(ctx, scope, workflowID)
}

// GetEvents fetches all events for a workflow.
func (s *AgentWorkflowService) GetEvents(ctx context.Context, tenantID, workflowID string) ([]domain.AgentWorkflowEvent, error) {
	scope := s.scopeForTenant(tenantID, "system/agent-coordinator")
	return s.repo.GetEvents(ctx, scope, workflowID)
}

// EvaluateResultFreshness verifies whether a specialist result is reusable across all 7 protected state bindings:
// 1. workflow_id
// 2. agent_name
// 3. manifest_hash
// 4. input_context_hash
// 5. artifact_sha256
// 6. policy_bundle_hash
// 7. authorized_evidence_set_hash
func (s *AgentWorkflowService) EvaluateResultFreshness(
	ctx context.Context,
	tenantID string,
	workflowID string,
	agentName string,
	expectedManifestHash string,
	expectedInputHash string,
	currentArtifactSHA string,
	currentPolicyBundleHash string,
	currentEvidenceSetHash string,
) (*domain.AgentStep, bool, error) {
	steps, err := s.GetSteps(ctx, tenantID, workflowID)
	if err != nil {
		return nil, false, err
	}

	for _, step := range steps {
		if step.StepType != domain.StepDecision && step.StepType != domain.StepModelInvocation {
			continue
		}
		if step.DecisionPayload == "" {
			continue
		}

		var payload map[string]interface{}
		if err := json.Unmarshal([]byte(step.DecisionPayload), &payload); err != nil {
			continue
		}

		if payload["agent_name"] != agentName {
			continue
		}

		// Check 7 protected state bindings
		manifestMatch := payload["manifest_hash"] == expectedManifestHash
		inputMatch := payload["input_context_hash"] == expectedInputHash
		artifactMatch := payload["artifact_sha256"] == currentArtifactSHA
		policyMatch := payload["policy_bundle_hash"] == currentPolicyBundleHash
		evidenceMatch := payload["authorized_evidence_set_hash"] == currentEvidenceSetHash

		if manifestMatch && inputMatch && artifactMatch && policyMatch && evidenceMatch {
			return &step, true, nil
		}
	}

	return nil, false, nil
}

// CheckTOCTOU verifies that the plan's policy bundle and artifact SHA match current Go authoritative truth.
// Returns (valid, violationType, err)
func (s *AgentWorkflowService) CheckTOCTOU(
	planPolicyBundleHash string,
	currentPolicyBundleHash string,
	planArtifactSHA string,
	currentArtifactSHA string,
) (bool, string) {
	if planPolicyBundleHash != "" && currentPolicyBundleHash != "" && planPolicyBundleHash != currentPolicyBundleHash {
		return false, "POLICY_CONTEXT_STALE"
	}
	if planArtifactSHA != "" && currentArtifactSHA != "" && planArtifactSHA != currentArtifactSHA {
		return false, "RESOURCE_CONTEXT_STALE"
	}
	return true, ""
}

// RecordRemediationPlan persists a structured remediation plan under tenant scope.
func (s *AgentWorkflowService) RecordRemediationPlan(ctx context.Context, tenantID string, plan *domain.RemediationPlanRecord) error {
	scope := s.scopeForTenant(tenantID, "system/remediation-service")
	return s.repo.RecordRemediationPlan(ctx, scope, plan)
}

// GetRemediationPlan fetches a remediation plan under tenant scope.
func (s *AgentWorkflowService) GetRemediationPlan(ctx context.Context, tenantID, planID string) (*domain.RemediationPlanRecord, error) {
	scope := s.scopeForTenant(tenantID, "system/remediation-service")
	return s.repo.GetRemediationPlan(ctx, scope, planID)
}

// RecordArtifactDerivation persists an immutable candidate derivation manifest.
func (s *AgentWorkflowService) RecordArtifactDerivation(ctx context.Context, tenantID string, d *domain.ArtifactDerivationRecord) error {
	scope := s.scopeForTenant(tenantID, "system/remediation-service")
	return s.repo.RecordArtifactDerivation(ctx, scope, d)
}

// GetArtifactDerivations retrieves all candidate derivation attempts for a workflow.
func (s *AgentWorkflowService) GetArtifactDerivations(ctx context.Context, tenantID, workflowID string) ([]domain.ArtifactDerivationRecord, error) {
	scope := s.scopeForTenant(tenantID, "system/remediation-service")
	return s.repo.GetArtifactDerivations(ctx, scope, workflowID)
}
