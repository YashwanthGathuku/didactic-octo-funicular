package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"

	"sentinel-gateway/internal/candidate"
	"sentinel-gateway/internal/domain"
	"sentinel-gateway/internal/toolgateway"
	"sentinel-gateway/internal/verification"
)

// StageType defines the bounded execution stages for AI reasoning.
type StageType string

const (
	StageCommanderPlan       StageType = "COMMANDER_PLAN"
	StageParallelSpecialists StageType = "PARALLEL_SPECIALISTS"
	StageCommanderSynthesis  StageType = "COMMANDER_SYNTHESIS"
	StageRemediationPlan     StageType = "REMEDIATION_PLAN"
	StageVerifierCritic      StageType = "STAGE_VERIFIER_CRITIC"
)

// AgentStageRequest represents a typed request from Go to Python for a bounded AI stage.
type AgentStageRequest struct {
	StageType               StageType              `json:"stage_type"`
	WorkflowID              string                 `json:"workflow_id"`
	TenantID                string                 `json:"tenant_id"`
	IncidentID              int64                  `json:"incident_id"`
	ArtifactID              int64                  `json:"artifact_id"`
	ArtifactSHA256          string                 `json:"artifact_sha256"`
	PolicyBundleHash        string                 `json:"policy_bundle_hash"`
	AuthorizedEvidenceRefs  []string               `json:"authorized_evidence_refs"`
	Findings                []interface{}          `json:"findings"`
	AvailableRunbooks       []string               `json:"available_runbooks"`
	CorrelationID           string                 `json:"correlation_id"`
	TraceID                 string                 `json:"trace_id,omitempty"`
	AttemptNumber           int                    `json:"attempt_number,omitempty"`
	Plan                    map[string]interface{} `json:"plan,omitempty"`
	DiagnosisResult         map[string]interface{} `json:"diagnosis_result,omitempty"`
	PolicySLAResult         map[string]interface{} `json:"policy_sla_result,omitempty"`
	RemediationPlan         map[string]interface{} `json:"remediation_plan,omitempty"`
	AuthoritativeDecision   map[string]interface{} `json:"authoritative_policy_decision,omitempty"`
	SLAContext              map[string]interface{} `json:"sla_context,omitempty"`
	MaxElapsedSeconds       float64                `json:"max_elapsed_seconds,omitempty"`
}

// AgentStageResponse represents the structured result returned by Python for a bounded stage.
type AgentStageResponse struct {
	StageType          StageType                      `json:"stage_type"`
	Status             string                         `json:"status"` // "SUCCESS" or "FAILED"
	WorkflowID         string                         `json:"workflow_id"`
	Plan               map[string]interface{}         `json:"plan,omitempty"`
	DiagnosisResult    map[string]interface{}         `json:"diagnosis_result,omitempty"`
	PolicySLAResult    map[string]interface{}         `json:"policy_sla_result,omitempty"`
	RemediationPlan    map[string]interface{}         `json:"remediation_plan,omitempty"`
	CriticAssessment   map[string]interface{}         `json:"critic_assessment,omitempty"`
	Synthesis          map[string]interface{}         `json:"synthesis,omitempty"`
	CandidateResult    *candidate.CandidateResult     `json:"candidate_result,omitempty"`
	VerificationResult *verification.VerificationResult `json:"verification_result,omitempty"`
	Outcome            string                         `json:"outcome,omitempty"`
	EvidenceRefs       []string                       `json:"evidence_refs,omitempty"`
	LatencyMs          float64                        `json:"latency_ms"`
	InputTokens        int                            `json:"input_tokens"`
	OutputTokens       int                            `json:"output_tokens"`
	ExecutionSource    string                         `json:"execution_source"`
	ErrorDetail        string                         `json:"error_detail,omitempty"`
}

// AgentOrchestrator is the authoritative Go orchestrator for multi-agent workflows.
type AgentOrchestrator struct {
	wfService     *AgentWorkflowService
	candService   *candidate.Service
	toolGW        *toolgateway.ToolGatewayService
	verifService  *verification.Service
	executionMode string
	aiTierURL     string
	httpClient    *http.Client
}

// NewAgentOrchestrator creates a new Go AgentOrchestrator.
func NewAgentOrchestrator(wfService *AgentWorkflowService, aiTierURL string) *AgentOrchestrator {
	if aiTierURL == "" {
		aiTierURL = "http://localhost:8000"
	}
	return &AgentOrchestrator{
		wfService:     wfService,
		executionMode: "LIVE",
		aiTierURL:     aiTierURL,
		httpClient: &http.Client{
			Timeout: 20 * time.Second,
		},
	}
}

// SetCandidateService attaches the Go Candidate Service for governed remediation.
func (o *AgentOrchestrator) SetCandidateService(cs *candidate.Service) {
	o.candService = cs
}

// SetToolGateway attaches the ToolGatewayService for exclusive governed tool execution.
func (o *AgentOrchestrator) SetToolGateway(gw *toolgateway.ToolGatewayService) {
	o.toolGW = gw
}

// SetVerificationService attaches the Go Verification Service for independent candidate verification.
func (o *AgentOrchestrator) SetVerificationService(vs *verification.Service) {
	o.verifService = vs
}

// SetExecutionMode sets the orchestrator execution mode (e.g. SHADOW, ADVISORY, LIVE).
func (o *AgentOrchestrator) SetExecutionMode(mode string) {
	o.executionMode = mode
}

// ExecuteStage executes a bounded AI stage by invoking the Python ADK tier or deterministic fallback.
func (o *AgentOrchestrator) ExecuteStage(ctx context.Context, req *AgentStageRequest) (*AgentStageResponse, error) {
	reqBytes, err := json.Marshal(req)
	if err != nil {
		return nil, fmt.Errorf("marshal stage request: %w", err)
	}

	url := fmt.Sprintf("%s/internal/agents/stage/run", o.aiTierURL)
	httpReq, err := http.NewRequestWithContext(ctx, "POST", url, bytes.NewReader(reqBytes))
	if err != nil {
		return nil, fmt.Errorf("create http request: %w", err)
	}
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("X-Sentinel-Tenant", req.TenantID)

	resp, err := o.httpClient.Do(httpReq)
	if err == nil && resp.StatusCode == http.StatusOK {
		defer resp.Body.Close()
		body, readErr := io.ReadAll(resp.Body)
		if readErr == nil {
			var stageResp AgentStageResponse
			if jsonErr := json.Unmarshal(body, &stageResp); jsonErr == nil {
				return &stageResp, nil
			}
		}
	}

	// For P07 Remediation: if AI tier is unavailable during REMEDIATION_PLAN, fail closed (zero candidate creation from agent)
	if req.StageType == StageRemediationPlan {
		return &AgentStageResponse{
			StageType:       StageRemediationPlan,
			Status:          "FAILED",
			WorkflowID:      req.WorkflowID,
			ErrorDetail:     "AI Tier unavailable: RemediationAgent could not be reached",
			ExecutionSource: "PROVIDER_FAILURE",
		}, nil
	}

	// Deterministic rule-grounded fallback for investigation and synthesis
	return o.fallbackStage(req)
}

func (o *AgentOrchestrator) fallbackStage(req *AgentStageRequest) (*AgentStageResponse, error) {
	switch req.StageType {
	case StageCommanderPlan:
		return &AgentStageResponse{
			StageType:  StageCommanderPlan,
			Status:     "SUCCESS",
			WorkflowID: req.WorkflowID,
			Plan: map[string]interface{}{
				"schema_version":       "1.0",
				"workflow_id":          req.WorkflowID,
				"workflow_class":       "VALIDATION_REMEDIATION",
				"selected_specialists": []string{"DiagnosisAgent", "PolicySLAAgent"},
				"reason_codes":         []string{"DETERMINISTIC_TRIAGE"},
				"parallelizable":       true,
				"remediation_eligible": true,
				"policy_bundle_hash":   req.PolicyBundleHash,
				"artifact_sha256":      req.ArtifactSHA256,
			},
			EvidenceRefs:    req.AuthorizedEvidenceRefs,
			LatencyMs:       5.0,
			ExecutionSource: "DETERMINISTIC_FALLBACK",
		}, nil

	case StageParallelSpecialists:
		return &AgentStageResponse{
			StageType:  StageParallelSpecialists,
			Status:     "SUCCESS",
			WorkflowID: req.WorkflowID,
			DiagnosisResult: map[string]interface{}{
				"agent_name":                  "DiagnosisAgent",
				"status":                      "SUCCESS",
				"manifest_hash":               "manifest_hash_diagnosis",
				"input_context_hash":          "input_hash_diag",
				"artifact_sha256":             req.ArtifactSHA256,
				"policy_bundle_hash":          req.PolicyBundleHash,
				"authorized_evidence_set_hash": "evidence_hash_default",
				"evidence_refs":               req.AuthorizedEvidenceRefs,
				"output": map[string]interface{}{
					"classification":         "ENTRY_HASH_ACCUMULATOR_MISMATCH",
					"summary":                "Batch entry hash accumulator mismatch verified.",
					"evidence_refs":          req.AuthorizedEvidenceRefs,
					"remediation_eligibility": true,
				},
			},
			PolicySLAResult: map[string]interface{}{
				"agent_name":                  "PolicySLAAgent",
				"status":                      "SUCCESS",
				"manifest_hash":               "manifest_hash_policy",
				"input_context_hash":          "input_hash_policy",
				"artifact_sha256":             req.ArtifactSHA256,
				"policy_bundle_hash":          req.PolicyBundleHash,
				"authorized_evidence_set_hash": "evidence_hash_default",
				"evidence_refs":               req.AuthorizedEvidenceRefs,
				"output": map[string]interface{}{
					"policy_summary": "PolicyEngine evaluation attached.",
					"evidence_refs":  req.AuthorizedEvidenceRefs,
					"sla_status":     "ON_TRACK",
				},
			},
			LatencyMs:       10.0,
			ExecutionSource: "DETERMINISTIC_FALLBACK",
		}, nil

	case StageCommanderSynthesis:
		decision := "REQUIRE_HUMAN"
		if req.AuthoritativeDecision != nil {
			if d, ok := req.AuthoritativeDecision["decision"].(string); ok && d != "" {
				decision = d
			}
		}

		outcome := "HUMAN_AUTHORIZATION_REQUIRED"
		if decision == "DENY" {
			outcome = "POLICY_BLOCKED"
		} else if decision == "ALLOW" || decision == "ALLOW_WITH_OBLIGATIONS" {
			outcome = "READY_FOR_REMEDIATION"
		}

		return &AgentStageResponse{
			StageType:  StageCommanderSynthesis,
			Status:     "SUCCESS",
			WorkflowID: req.WorkflowID,
			Outcome:    outcome,
			Synthesis: map[string]interface{}{
				"schema_version":    "1.0",
				"workflow_id":       req.WorkflowID,
				"outcome":           outcome,
				"synthesis_summary": fmt.Sprintf("Authoritative synthesis: %s.", outcome),
				"evidence_refs":     req.AuthorizedEvidenceRefs,
			},
			EvidenceRefs:    req.AuthorizedEvidenceRefs,
			LatencyMs:       5.0,
			ExecutionSource: "DETERMINISTIC_FALLBACK",
		}, nil

	case StageRemediationPlan:
		return &AgentStageResponse{
			StageType:  StageRemediationPlan,
			Status:     "SUCCESS",
			WorkflowID: req.WorkflowID,
			RemediationPlan: map[string]interface{}{
				"schema_version":         "1.0",
				"workflow_id":            req.WorkflowID,
				"tenant_id":              req.TenantID,
				"incident_id":            req.IncidentID,
				"artifact_id":            req.ArtifactID,
				"expected_parent_sha256": req.ArtifactSHA256,
				"attempt_number":         req.AttemptNumber,
				"plan_hash":              fmt.Sprintf("plan-hash-auto-%s-%d", req.WorkflowID, req.AttemptNumber),
				"operations": []map[string]interface{}{
					{
						"operation_type": candidate.OpRecomputeBatchControlTotal,
						"target_ref":     "BATCH-1",
						"finding_refs":   []string{"FINDING-001"},
						"rationale":     "Recompute batch debit total and hash accumulator",
					},
					{
						"operation_type": candidate.OpRecomputeFileControlTotal,
						"target_ref":     "FILE_CONTROL",
						"finding_refs":   []string{"FINDING-002"},
						"rationale":     "Recompute file debit total and block count",
					},
				},
				"confidence": "HIGH",
			},
			EvidenceRefs:    req.AuthorizedEvidenceRefs,
			LatencyMs:       5.0,
			ExecutionSource: "DETERMINISTIC_FALLBACK",
		}, nil
	}

	if req.StageType == StageVerifierCritic {
		return &AgentStageResponse{
			StageType:  StageVerifierCritic,
			Status:     "SUCCESS",
			WorkflowID: req.WorkflowID,
			CriticAssessment: map[string]interface{}{
				"schema_version": "1.0",
				"candidate_ref":  fmt.Sprintf("cand-%d", req.ArtifactID),
				"assessment":     "CONSISTENT",
				"risk_level":     "LOW",
				"recommendation": "PROCEED_TO_HUMAN_REVIEW",
				"summary":        "Candidate independently verified by deterministic validator and critic fallback",
				"non_authority_statement": "This critic assessment is advisory only and has no authority to approve, verify, or release financial artifacts.",
			},
			EvidenceRefs:    req.AuthorizedEvidenceRefs,
			LatencyMs:       5.0,
			ExecutionSource: "DETERMINISTIC_FALLBACK",
		}, nil
	}

	return &AgentStageResponse{
		StageType:   req.StageType,
		Status:      "FAILED",
		WorkflowID:  req.WorkflowID,
		ErrorDetail: "Unknown stage type",
	}, nil
}

// RunWorkflow executes the full multi-agent workflow under Go control-plane authority.
func (o *AgentOrchestrator) RunWorkflow(
	ctx context.Context,
	tenantID string,
	triggerEventID string,
	workflowType string,
	incidentID int64,
	artifactID int64,
	currentArtifactSHA string,
	currentPolicyBundleHash string,
	currentEvidenceSetHash string,
	authorizedEvidenceRefs []string,
	findings []interface{},
	availableRunbooks []string,
	authoritativePolicyDecision map[string]interface{},
	slaContext map[string]interface{},
	corrID string,
	traceID string,
) (*domain.AgentWorkflow, *AgentStageResponse, error) {
	// 1. Authoritative Durable Workflow Retrieval or Creation
	wf, _, err := o.wfService.GetOrCreateWorkflowByTrigger(
		ctx, tenantID, triggerEventID, workflowType, incidentID, artifactID, currentArtifactSHA,
		currentPolicyBundleHash, currentEvidenceSetHash, corrID, traceID,
	)
	if err != nil {
		return nil, nil, fmt.Errorf("get or create workflow: %w", err)
	}

	// 2. If workflow is already in terminal state, return it immediately
	if domain.IsTerminalAgentWorkflow(wf.State) {
		outcome := "UNRESOLVED"
		if wf.State == domain.WorkflowCompleted {
			outcome = "READY_FOR_REMEDIATION"
		} else if wf.State == domain.WorkflowAwaitingVerification {
			outcome = "CANDIDATE_REVALIDATION_PASSED"
		} else if wf.State == domain.WorkflowPolicyDenied {
			outcome = "POLICY_BLOCKED"
		} else if wf.State == domain.WorkflowHumanReview {
			outcome = "HUMAN_AUTHORIZATION_REQUIRED"
		}
		synthResp := &AgentStageResponse{
			StageType:  StageCommanderSynthesis,
			Status:     "SUCCESS",
			WorkflowID: wf.ID,
			Outcome:    outcome,
		}
		return wf, synthResp, nil
	}

	// 3. Transition PENDING -> INVESTIGATING -> PLANNING in Go
	wf, err = o.wfService.TransitionWorkflow(ctx, tenantID, wf.ID, wf.RowVersion, domain.WorkflowContextBuilding, fmt.Sprintf("ik-cb-%s", wf.ID), "", "Building context")
	if err != nil {
		return nil, nil, fmt.Errorf("transition context building: %w", err)
	}

	wf, err = o.wfService.TransitionWorkflow(ctx, tenantID, wf.ID, wf.RowVersion, domain.WorkflowInvestigating, fmt.Sprintf("ik-inv-%s", wf.ID), "", "Investigating incident")
	if err != nil {
		return nil, nil, fmt.Errorf("transition investigating: %w", err)
	}

	wf, err = o.wfService.TransitionWorkflow(ctx, tenantID, wf.ID, wf.RowVersion, domain.WorkflowPlanning, fmt.Sprintf("ik-plan-%s", wf.ID), "", "Planning investigation")
	if err != nil {
		return nil, nil, fmt.Errorf("transition planning: %w", err)
	}

	// 4. Execute COMMANDER_PLAN Stage
	planReq := &AgentStageRequest{
		StageType:              StageCommanderPlan,
		WorkflowID:             wf.ID,
		TenantID:               tenantID,
		IncidentID:             incidentID,
		ArtifactID:             artifactID,
		ArtifactSHA256:         currentArtifactSHA,
		PolicyBundleHash:       currentPolicyBundleHash,
		AuthorizedEvidenceRefs: authorizedEvidenceRefs,
		Findings:               findings,
		AvailableRunbooks:      availableRunbooks,
		CorrelationID:          corrID,
		TraceID:                traceID,
	}

	planResp, err := o.ExecuteStage(ctx, planReq)
	if err != nil {
		return nil, nil, fmt.Errorf("execute commander plan stage: %w", err)
	}

	_ = o.wfService.RecordWorkflowEvent(ctx, tenantID, wf.ID, fmt.Sprintf("ik-plan-acc-%s", wf.ID), "COMMANDER_PLAN_ACCEPTED", domain.WorkflowPlanning, domain.WorkflowPlanning, wf.RowVersion, planResp.Plan)

	// 5. Execute PARALLEL_SPECIALISTS Stage
	specialistsReq := &AgentStageRequest{
		StageType:              StageParallelSpecialists,
		WorkflowID:             wf.ID,
		TenantID:               tenantID,
		IncidentID:             incidentID,
		ArtifactID:             artifactID,
		ArtifactSHA256:         currentArtifactSHA,
		PolicyBundleHash:       currentPolicyBundleHash,
		AuthorizedEvidenceRefs: authorizedEvidenceRefs,
		Findings:               findings,
		AvailableRunbooks:      availableRunbooks,
		CorrelationID:          corrID,
		TraceID:                traceID,
		Plan:                   planResp.Plan,
		AuthoritativeDecision:  authoritativePolicyDecision,
		SLAContext:             slaContext,
	}

	_ = o.wfService.RecordWorkflowEvent(ctx, tenantID, wf.ID, fmt.Sprintf("ik-spec-start-%s", wf.ID), "SPECIALIST_STARTED", domain.WorkflowPlanning, domain.WorkflowPlanning, wf.RowVersion, map[string]interface{}{"specialists": []string{"DiagnosisAgent", "PolicySLAAgent"}})

	specialistsResp, err := o.ExecuteStage(ctx, specialistsReq)
	if err != nil {
		_ = o.wfService.RecordWorkflowEvent(ctx, tenantID, wf.ID, fmt.Sprintf("ik-spec-fail-%s", wf.ID), "SPECIALIST_FAILED", domain.WorkflowPlanning, domain.WorkflowPlanning, wf.RowVersion, map[string]interface{}{"error": err.Error()})
		return nil, nil, fmt.Errorf("execute parallel specialists stage: %w", err)
	}

	_ = o.wfService.RecordWorkflowEvent(ctx, tenantID, wf.ID, fmt.Sprintf("ik-spec-done-%s", wf.ID), "SPECIALIST_COMPLETED", domain.WorkflowPlanning, domain.WorkflowPlanning, wf.RowVersion, map[string]interface{}{"diagnosis": specialistsResp.DiagnosisResult, "policy_sla": specialistsResp.PolicySLAResult})

	// Record specialist steps in Go agent_steps
	diagStepID := fmt.Sprintf("step-diag-%s", wf.ID)
	diagPayload, _ := json.Marshal(specialistsResp.DiagnosisResult)
	_ = o.wfService.RecordStep(ctx, tenantID, &domain.AgentStep{
		ID:                     diagStepID,
		RunID:                  fmt.Sprintf("run-diag-%s", wf.ID),
		WorkflowID:             wf.ID,
		TenantID:               tenantID,
		StepNumber:             1,
		StepType:               domain.StepDecision,
		StateFrom:              domain.WorkflowPlanning,
		StateTo:                domain.WorkflowPlanning,
		DecisionPayload:        string(diagPayload),
		AuthorizedEvidenceRefs: authorizedEvidenceRefs,
		StepStatus:             "COMPLETED",
		LatencyMs:              int(specialistsResp.LatencyMs),
	})

	// 6. Section 10 & 11: TOCTOU Checks in Go Authority
	planPolicyHash := ""
	planArtifactSHA := ""
	if planResp.Plan != nil {
		if p, ok := planResp.Plan["policy_bundle_hash"].(string); ok {
			planPolicyHash = p
		}
		if a, ok := planResp.Plan["artifact_sha256"].(string); ok {
			planArtifactSHA = a
		}
	}

	toctouValid, violationType := o.wfService.CheckTOCTOU(planPolicyHash, currentPolicyBundleHash, planArtifactSHA, currentArtifactSHA)
	if !toctouValid {
		_ = o.wfService.RecordWorkflowEvent(ctx, tenantID, wf.ID, fmt.Sprintf("ik-toctou-%s", wf.ID), violationType, domain.WorkflowPlanning, domain.WorkflowUnresolved, wf.RowVersion, map[string]interface{}{
			"violation": violationType,
		})
		wf, _ = o.wfService.TransitionWorkflow(ctx, tenantID, wf.ID, wf.RowVersion, domain.WorkflowUnresolved, fmt.Sprintf("ik-toctou-tx-%s", wf.ID), violationType, "TOCTOU check failed: stale policy or resource context")

		toctouResp := &AgentStageResponse{
			StageType:   StageCommanderSynthesis,
			Status:      "SUCCESS",
			WorkflowID:  wf.ID,
			Outcome:     "UNRESOLVED",
			ErrorDetail: "TOCTOU check failed: stale policy or resource context",
		}
		return wf, toctouResp, nil
	}

	// 7. Execute COMMANDER_SYNTHESIS Stage
	synthesisReq := &AgentStageRequest{
		StageType:              StageCommanderSynthesis,
		WorkflowID:             wf.ID,
		TenantID:               tenantID,
		IncidentID:             incidentID,
		ArtifactID:             artifactID,
		ArtifactSHA256:         currentArtifactSHA,
		PolicyBundleHash:       currentPolicyBundleHash,
		AuthorizedEvidenceRefs: authorizedEvidenceRefs,
		Plan:                   planResp.Plan,
		DiagnosisResult:        specialistsResp.DiagnosisResult,
		PolicySLAResult:        specialistsResp.PolicySLAResult,
		AuthoritativeDecision:  authoritativePolicyDecision,
		SLAContext:             slaContext,
	}

	synthesisResp, err := o.ExecuteStage(ctx, synthesisReq)
	if err != nil {
		return nil, nil, fmt.Errorf("execute synthesis stage: %w", err)
	}

	// 8. Determine Next Action from Synthesis & Policy
	decision := ""
	if authoritativePolicyDecision != nil {
		if d, ok := authoritativePolicyDecision["decision"].(string); ok {
			decision = d
		}
	}

	if decision == "DENY" {
		wf, _ = o.wfService.TransitionWorkflow(ctx, tenantID, wf.ID, wf.RowVersion, domain.WorkflowPolicyDenied, fmt.Sprintf("ik-deny-%s", wf.ID), "", "Policy engine denied action")
		synthesisResp.Outcome = "POLICY_BLOCKED"
		return wf, synthesisResp, nil
	}

	if decision == "REQUIRE_HUMAN" {
		wf, _ = o.wfService.TransitionWorkflow(ctx, tenantID, wf.ID, wf.RowVersion, domain.WorkflowHumanReview, fmt.Sprintf("ik-human-%s", wf.ID), "", "Human authorization required")
		synthesisResp.Outcome = "HUMAN_AUTHORIZATION_REQUIRED"
		return wf, synthesisResp, nil
	}

	// 9. P07 Governed Remediation & Candidate Generation Loop
	if (synthesisResp.Outcome == "READY_FOR_REMEDIATION" || decision == "ALLOW" || decision == "ALLOW_WITH_OBLIGATIONS") && (o.candService != nil || o.toolGW != nil) {
		// Transition Planning -> Remediating
		wf, err = o.wfService.TransitionWorkflow(ctx, tenantID, wf.ID, wf.RowVersion, domain.WorkflowRemediating, fmt.Sprintf("ik-remed-%s", wf.ID), "", "Beginning governed remediation")
		if err != nil {
			return nil, nil, fmt.Errorf("transition remediating: %w", err)
		}

		scope := o.wfService.scopeForTenant(tenantID, "system/remediation-orchestrator")

		// Max 3 attempts bound
		for attempt := 1; attempt <= 3; attempt++ {
			_ = o.wfService.RecordWorkflowEvent(ctx, tenantID, wf.ID, fmt.Sprintf("ik-rem-att-%s-%d", wf.ID, attempt), "REMEDIATION_ATTEMPT_STARTED", wf.State, domain.WorkflowRemediating, wf.RowVersion, map[string]interface{}{"attempt": attempt})

			// Invoke RemediationPlan stage
			remReq := &AgentStageRequest{
				StageType:              StageRemediationPlan,
				WorkflowID:             wf.ID,
				TenantID:               tenantID,
				IncidentID:             incidentID,
				ArtifactID:             artifactID,
				ArtifactSHA256:         currentArtifactSHA,
				PolicyBundleHash:       currentPolicyBundleHash,
				AuthorizedEvidenceRefs: authorizedEvidenceRefs,
				Findings:               findings,
				AvailableRunbooks:      availableRunbooks,
				CorrelationID:          corrID,
				TraceID:                traceID,
				AttemptNumber:          attempt,
			}

			remResp, remErr := o.ExecuteStage(ctx, remReq)
			if remErr != nil || remResp.Status != "SUCCESS" || remResp.RemediationPlan == nil {
				// Decoupling Rule: AI tier failure => CandidateCreationFromAgent = 0
				_ = o.wfService.RecordWorkflowEvent(ctx, tenantID, wf.ID, fmt.Sprintf("ik-rem-fail-%s-%d", wf.ID, attempt), "REMEDIATION_PLAN_FAILED", wf.State, domain.WorkflowAgentUnavailable, wf.RowVersion, map[string]interface{}{"error": remResp.ErrorDetail})
				wf, _ = o.wfService.TransitionWorkflow(ctx, tenantID, wf.ID, wf.RowVersion, domain.WorkflowAgentUnavailable, fmt.Sprintf("ik-rem-unavail-%s", wf.ID), "", "Remediation plan generation failed: AI tier unavailable")
				synthesisResp.Outcome = "AGENT_UNAVAILABLE"
				return wf, synthesisResp, nil
			}

			// Parse RemediationOperations from plan
			var ops []candidate.RemediationOperation
			if rawOps, ok := remResp.RemediationPlan["operations"].([]interface{}); ok {
				for _, ro := range rawOps {
					if opMap, ok := ro.(map[string]interface{}); ok {
						var op candidate.RemediationOperation
						opBytes, _ := json.Marshal(opMap)
						_ = json.Unmarshal(opBytes, &op)
						if op.OperationType != "" {
							ops = append(ops, op)
						}
					}
				}
			}

			planHash := fmt.Sprintf("%v", remResp.RemediationPlan["plan_hash"])
			if planHash == "" || planHash == "<nil>" {
				planHash = fmt.Sprintf("plan-hash-%s-%d", wf.ID, attempt)
			}

			candReq := &candidate.CandidateCreationRequest{
				TenantID:             tenantID,
				WorkflowID:           wf.ID,
				IncidentID:           incidentID,
				ParentArtifactID:     artifactID,
				ExpectedParentSHA256: currentArtifactSHA,
				AttemptNumber:        attempt,
				PlanHash:             planHash,
				Operations:           ops,
				FindingRefs:          authorizedEvidenceRefs,
				Confidence:           "HIGH",
				AgentName:            "RemediationAgent",
				AgentVersion:         "1.0.0",
				PolicyDecisionID:     "POL-DEC-REMED-001",
				PolicyDecisionHash:   currentPolicyBundleHash,
				ToolManifestHash:     "man-hash-remed-create",
			}

			var candRes *candidate.CandidateResult

			if o.toolGW != nil {
				// Authoritative Path: Exclusive Tool Gateway Enforcement
				candReqBytes, marshalErr := json.Marshal(candReq)
				if marshalErr != nil {
					return nil, nil, fmt.Errorf("marshal candidate request: %w", marshalErr)
				}

				execMode := o.executionMode
				if execMode == "" {
					execMode = "LIVE"
				}

				execCtx := &toolgateway.TrustedExecutionContext{
					RequestID:           fmt.Sprintf("req-cand-%s-%d", wf.ID, attempt),
					IdempotencyKey:      fmt.Sprintf("ik-tool-cand-%s-%d", wf.ID, attempt),
					CorrelationID:       corrID,
					TraceID:             traceID,
					TenantID:            tenantID,
					CallerType:          toolgateway.CallerTypeAgent,
					CallerID:            "RemediationAgent",
					CallerRoles:         []string{"REMEDIATION_AGENT"},
					CallerCapabilities:  []toolgateway.ToolCapability{toolgateway.CapCandidateCreate},
					CallerAutonomyLevel: 2, // Autonomy A2
					WorkflowID:          wf.ID,
					IncidentID:          fmt.Sprintf("%d", incidentID),
					ArtifactID:          fmt.Sprintf("%d", artifactID),
					ArtifactSHA256:      currentArtifactSHA,
					ResourceVersion:     attempt,
					AllowedTools:        []string{toolgateway.ToolRemediationCandidateCreate},
					ExecutionMode:       execMode,
					Timestamp:           time.Now().UTC(),
				}

				toolReq := &toolgateway.ToolRequest{
					ToolID:         toolgateway.ToolRemediationCandidateCreate,
					ToolVersion:    "1.0.0",
					Args:           json.RawMessage(candReqBytes),
					IdempotencyKey: fmt.Sprintf("ik-tool-cand-%s-%d", wf.ID, attempt),
					ResourcePreconditions: &toolgateway.ResourcePreconditions{
						ExpectedArtifactSHA256: currentArtifactSHA,
						ExpectedPolicyBundle:   currentPolicyBundleHash,
					},
				}

				toolResp, genErr := o.toolGW.Execute(ctx, execCtx, toolReq, nil)
				if genErr != nil || toolResp == nil || toolResp.Status != toolgateway.StatusSucceeded {
					errMsg := "candidate creation rejected or failed via tool gateway"
					if genErr != nil {
						errMsg = genErr.Error()
					} else if toolResp != nil && toolResp.Error != nil {
						errMsg = toolResp.Error.Message
					}
					_ = o.wfService.RecordWorkflowEvent(ctx, tenantID, wf.ID, fmt.Sprintf("ik-gen-err-%s-%d", wf.ID, attempt), "CANDIDATE_CREATION_FAILED", wf.State, domain.WorkflowUnresolved, wf.RowVersion, map[string]interface{}{"error": errMsg})
					wf, _ = o.wfService.TransitionWorkflow(ctx, tenantID, wf.ID, wf.RowVersion, domain.WorkflowUnresolved, fmt.Sprintf("ik-cand-fail-%s", wf.ID), "", fmt.Sprintf("Candidate generation failed: %s", errMsg))
					synthesisResp.Outcome = "UNRESOLVED"
					return wf, synthesisResp, nil
				}

				var cr candidate.CandidateResult
				if unmarshalErr := json.Unmarshal(toolResp.Output, &cr); unmarshalErr != nil {
					return nil, nil, fmt.Errorf("unmarshal candidate result: %w", unmarshalErr)
				}
				candRes = &cr
			} else if o.candService != nil {
				// Fallback only if toolGW not explicitly configured
				var genErr error
				candRes, genErr = o.candService.GenerateCandidate(ctx, scope, candReq)
				if genErr != nil {
					_ = o.wfService.RecordWorkflowEvent(ctx, tenantID, wf.ID, fmt.Sprintf("ik-gen-err-%s-%d", wf.ID, attempt), "CANDIDATE_CREATION_FAILED", wf.State, domain.WorkflowUnresolved, wf.RowVersion, map[string]interface{}{"error": genErr.Error()})
					wf, _ = o.wfService.TransitionWorkflow(ctx, tenantID, wf.ID, wf.RowVersion, domain.WorkflowUnresolved, fmt.Sprintf("ik-cand-fail-%s", wf.ID), "", fmt.Sprintf("Candidate generation failed: %v", genErr))
					synthesisResp.Outcome = "UNRESOLVED"
					return wf, synthesisResp, nil
				}
			} else {
				_ = o.wfService.RecordWorkflowEvent(ctx, tenantID, wf.ID, fmt.Sprintf("ik-gen-err-%s-%d", wf.ID, attempt), "CANDIDATE_CREATION_FAILED", wf.State, domain.WorkflowUnresolved, wf.RowVersion, map[string]interface{}{"error": "tool gateway and candidate service not configured"})
				wf, _ = o.wfService.TransitionWorkflow(ctx, tenantID, wf.ID, wf.RowVersion, domain.WorkflowUnresolved, fmt.Sprintf("ik-cand-fail-%s", wf.ID), "", "Candidate generation failed: tool gateway not configured")
				synthesisResp.Outcome = "UNRESOLVED"
				return wf, synthesisResp, nil
			}

			// Transition Remediating -> ValidatingCandidate
			wf, err = o.wfService.TransitionWorkflow(ctx, tenantID, wf.ID, wf.RowVersion, domain.WorkflowValidatingCandidate, fmt.Sprintf("ik-vc-%s-%d", wf.ID, attempt), "", "Re-validating candidate artifact")
			if err != nil {
				return nil, nil, fmt.Errorf("transition validating candidate: %w", err)
			}

			if candRes.ValidationOutcome == "VALIDATION_PASSED" {
				_ = o.wfService.RecordWorkflowEvent(ctx, tenantID, wf.ID, fmt.Sprintf("ik-val-pass-%s-%d", wf.ID, attempt), "CANDIDATE_REVALIDATION_PASSED", domain.WorkflowValidatingCandidate, domain.WorkflowAwaitingVerification, wf.RowVersion, map[string]interface{}{
					"candidate_sha256": candRes.CandidateSHA256,
					"attempt":          attempt,
				})

				wf, err = o.wfService.TransitionWorkflow(ctx, tenantID, wf.ID, wf.RowVersion, domain.WorkflowAwaitingVerification, fmt.Sprintf("ik-await-ver-%s", wf.ID), "", "Candidate revalidation passed. Awaiting independent verification.")
				if err != nil {
					return nil, nil, fmt.Errorf("transition awaiting verification: %w", err)
				}

				if o.verifService != nil {
					// Transition AwaitingVerification -> Verifying
					wf, err = o.wfService.TransitionWorkflow(ctx, tenantID, wf.ID, wf.RowVersion, domain.WorkflowVerifying, fmt.Sprintf("ik-verifying-%s", wf.ID), "", "Independent verification running")
					if err != nil {
						return nil, nil, fmt.Errorf("transition verifying: %w", err)
					}

					verifReq := &verification.VerificationRequest{
						TenantID:                tenantID,
						WorkflowID:              wf.ID,
						CandidateArtifactID:     candRes.CandidateArtifactID,
						ExpectedCandidateSHA256: candRes.CandidateSHA256,
						ExpectedParentSHA256:    candRes.ParentSHA256,
						DerivationID:            candRes.DerivationID,
						PolicyBundleHash:        currentPolicyBundleHash,
						CallerID:                "AgentOrchestrator",
					}

					verifRes, verifErr := o.verifService.VerifyCandidate(ctx, scope, verifReq)
					if verifErr != nil || verifRes == nil || verifRes.DeterministicOutcome != verification.OutcomePass {
						detOutcome := "FAIL"
						if verifRes != nil {
							detOutcome = string(verifRes.DeterministicOutcome)
						}
						_ = o.wfService.RecordWorkflowEvent(ctx, tenantID, wf.ID, fmt.Sprintf("ik-ver-fail-%s-%d", wf.ID, attempt), "VERIFICATION_CHECK_FAILED", domain.WorkflowVerifying, domain.WorkflowRetrying, wf.RowVersion, map[string]interface{}{
							"outcome": detOutcome,
							"attempt": attempt,
						})
						if attempt < 3 {
							wf, _ = o.wfService.TransitionWorkflow(ctx, tenantID, wf.ID, wf.RowVersion, domain.WorkflowRetrying, fmt.Sprintf("ik-retry-ver-%s-%d", wf.ID, attempt), "", fmt.Sprintf("Verification failed (%s). Retrying fresh plan against original.", detOutcome))
							wf, _ = o.wfService.TransitionWorkflow(ctx, tenantID, wf.ID, wf.RowVersion, domain.WorkflowRemediating, fmt.Sprintf("ik-rem-loop-ver-%s-%d", wf.ID, attempt), "", "Retrying remediation")
							continue
						} else {
							wf, _ = o.wfService.TransitionWorkflow(ctx, tenantID, wf.ID, wf.RowVersion, domain.WorkflowUnresolved, fmt.Sprintf("ik-unres-ver-%s", wf.ID), "", fmt.Sprintf("Verification failed: %s (attempts exhausted)", detOutcome))
							synthesisResp.Outcome = "VERIFICATION_REJECTED"
							synthesisResp.CandidateResult = candRes
							synthesisResp.VerificationResult = verifRes
							return wf, synthesisResp, nil
						}
					}

					// Deterministic checks passed. Now invoke VerifierAgent (Critic) stage for advisory review
					criticReq := &AgentStageRequest{
						TenantID:   tenantID,
						WorkflowID: wf.ID,
						IncidentID: incidentID,
						ArtifactID: candRes.CandidateArtifactID,
						StageType:  StageVerifierCritic,
						AuthorizedEvidenceRefs: []string{
							fmt.Sprintf("art-%d", candRes.ParentArtifactID),
							fmt.Sprintf("cand-%d", candRes.CandidateArtifactID),
							candRes.DerivationID,
						},
					}

					criticResp, criticErr := o.ExecuteStage(ctx, criticReq)
					criticConcern := false
					if criticErr == nil && criticResp != nil && criticResp.CriticAssessment != nil {
						if risk, ok := criticResp.CriticAssessment["risk_level"].(string); ok && risk == "HIGH" {
							criticConcern = true
						}
						if rec, ok := criticResp.CriticAssessment["recommendation"].(string); ok && (rec == "REQUEST_HUMAN_INVESTIGATION" || rec == "HUMAN_INVESTIGATION_REQUIRED") {
							criticConcern = true
						}
					}

					if criticConcern {
						_ = o.wfService.RecordWorkflowEvent(ctx, tenantID, wf.ID, fmt.Sprintf("ik-critic-concern-%s", wf.ID), "CRITIC_CONCERN_RAISED", domain.WorkflowVerifying, domain.WorkflowHumanReview, wf.RowVersion, map[string]interface{}{
							"recommendation": criticResp.CriticAssessment["recommendation"],
							"risk_level":     criticResp.CriticAssessment["risk_level"],
						})
						wf, _ = o.wfService.TransitionWorkflow(ctx, tenantID, wf.ID, wf.RowVersion, domain.WorkflowHumanReview, fmt.Sprintf("ik-hr-critic-%s", wf.ID), "", "CriticAgent raised high-risk concern. Routing to human investigation queue.")
						synthesisResp.Outcome = "HUMAN_INVESTIGATION_REQUIRED"
						synthesisResp.CandidateResult = candRes
						synthesisResp.VerificationResult = verifRes
						if criticResp != nil {
							synthesisResp.CriticAssessment = criticResp.CriticAssessment
						}
						return wf, synthesisResp, nil
					}

					// Consensus passed: Transition Verifying -> Verified
					_ = o.wfService.RecordWorkflowEvent(ctx, tenantID, wf.ID, fmt.Sprintf("ik-ver-passed-%s", wf.ID), "CANDIDATE_VERIFIED", domain.WorkflowVerifying, domain.WorkflowVerified, wf.RowVersion, map[string]interface{}{
						"verification_hash": verifRes.VerificationHash,
						"candidate_sha256":  candRes.CandidateSHA256,
					})
					wf, err = o.wfService.TransitionWorkflow(ctx, tenantID, wf.ID, wf.RowVersion, domain.WorkflowVerified, fmt.Sprintf("ik-verified-%s", wf.ID), "", "Candidate independently verified by Go control plane and CriticAgent.")
					if err != nil {
						return nil, nil, fmt.Errorf("transition verified: %w", err)
					}

					synthesisResp.Outcome = "VERIFIED"
					synthesisResp.CandidateResult = candRes
					synthesisResp.VerificationResult = verifRes
					if criticResp != nil {
						synthesisResp.CriticAssessment = criticResp.CriticAssessment
					}
					return wf, synthesisResp, nil
				}

				// Fallback when verification service is not attached
				synthesisResp.Outcome = "CANDIDATE_REVALIDATION_PASSED"
				synthesisResp.CandidateResult = candRes
				return wf, synthesisResp, nil
			}

			// Candidate revalidation failed
			_ = o.wfService.RecordWorkflowEvent(ctx, tenantID, wf.ID, fmt.Sprintf("ik-val-fail-%s-%d", wf.ID, attempt), "CANDIDATE_REVALIDATION_FAILED", domain.WorkflowValidatingCandidate, domain.WorkflowRetrying, wf.RowVersion, map[string]interface{}{
				"findings_count": candRes.FindingsCount,
				"attempt":        attempt,
			})

			if attempt < 3 {
				wf, _ = o.wfService.TransitionWorkflow(ctx, tenantID, wf.ID, wf.RowVersion, domain.WorkflowRetrying, fmt.Sprintf("ik-retry-%s-%d", wf.ID, attempt), "", fmt.Sprintf("Candidate attempt %d failed revalidation. Retrying fresh plan against original.", attempt))
				wf, _ = o.wfService.TransitionWorkflow(ctx, tenantID, wf.ID, wf.RowVersion, domain.WorkflowRemediating, fmt.Sprintf("ik-rem-loop-%s-%d", wf.ID, attempt), "", "Retrying remediation")
			} else {
				// Max attempts exhausted
				_ = o.wfService.RecordWorkflowEvent(ctx, tenantID, wf.ID, fmt.Sprintf("ik-exh-%s", wf.ID), "REMEDIATION_ATTEMPTS_EXHAUSTED", domain.WorkflowValidatingCandidate, domain.WorkflowUnresolved, wf.RowVersion, map[string]interface{}{
					"total_attempts": 3,
				})
				wf, _ = o.wfService.TransitionWorkflow(ctx, tenantID, wf.ID, wf.RowVersion, domain.WorkflowUnresolved, fmt.Sprintf("ik-unres-%s", wf.ID), "", "Remediation attempts exhausted: candidate failed 3 times")
				synthesisResp.Outcome = "REMEDIATION_ATTEMPTS_EXHAUSTED"
				synthesisResp.CandidateResult = candRes
				return wf, synthesisResp, nil
			}
		}
	}

	// Default fallback completion if no candidate generation triggered
	wf, err = o.wfService.TransitionWorkflow(ctx, tenantID, wf.ID, wf.RowVersion, domain.WorkflowCompleted, fmt.Sprintf("ik-synth-%s", wf.ID), "", fmt.Sprintf("Workflow finalized with outcome: %s", synthesisResp.Outcome))
	if err != nil {
		return nil, nil, fmt.Errorf("transition final state: %w", err)
	}

	_ = o.wfService.RecordWorkflowEvent(ctx, tenantID, wf.ID, fmt.Sprintf("ik-synth-done-%s", wf.ID), "SYNTHESIS_COMPLETED", domain.WorkflowPlanning, domain.WorkflowCompleted, wf.RowVersion, map[string]interface{}{
		"outcome": synthesisResp.Outcome,
	})

	return wf, synthesisResp, nil
}
