package toolgateway

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"runtime/debug"
	"time"

	"sentinel-gateway/internal/policy"
)

// ToolGatewayService coordinates the governed execution of registered tools.
// It is the exclusive enforcement boundary between callers/agents and system capabilities.
type ToolGatewayService struct {
	registry     *Registry
	policyEngine *policy.PolicyEngine
	store        *ToolStore
	idempotency  *IdempotencyCoordinator
}

// NewToolGatewayService creates a new ToolGatewayService.
func NewToolGatewayService(
	registry *Registry,
	policyEngine *policy.PolicyEngine,
	store *ToolStore,
) *ToolGatewayService {
	return &ToolGatewayService{
		registry:     registry,
		policyEngine: policyEngine,
		store:        store,
		idempotency:  NewIdempotencyCoordinator(),
	}
}

// Execute handles the complete 12-step governed tool execution lifecycle.
func (s *ToolGatewayService) Execute(
	ctx context.Context,
	execCtx *TrustedExecutionContext,
	req *ToolRequest,
	evidence map[policy.ObligationType]*ObligationEvidence,
) (*ToolResponse, error) {
	startTime := time.Now().UTC()

	// 1. Identity & Tenant Authorization
	if execCtx == nil {
		return nil, errors.New("execution context is required")
	}
	if err := execCtx.Validate(); err != nil {
		return nil, err
	}
	if req == nil {
		return nil, errors.New("tool request is required")
	}

	// 2. Registry Lookup & Manifest Verification
	tool, err := s.registry.Lookup(req.ToolID, req.ToolVersion)
	if err != nil {
		return nil, err
	}
	m := tool.Manifest

	// 3. Caller Capability & Context Allowlist Verification
	if !execCtx.IsToolAllowed(m.ToolID) {
		return nil, fmt.Errorf("%w: tool %s not allowed for caller", ErrToolNotInAllowlist, m.ToolID)
	}
	for _, reqCap := range m.RequiredCapabilities {
		if !execCtx.HasCapability(reqCap) {
			return nil, fmt.Errorf("%w: missing %s", ErrUnauthorizedCapability, reqCap)
		}
	}
	// Autonomy bounds apply specifically to AGENT callers
	if execCtx.CallerType == CallerTypeAgent || execCtx.CallerType == "" {
		if execCtx.CallerAutonomyLevel < m.MinAutonomy {
			return nil, fmt.Errorf("%w: caller autonomy %d < min %d", ErrAutonomyInsufficient, execCtx.CallerAutonomyLevel, m.MinAutonomy)
		}
		if m.MaxAutonomy > 0 && execCtx.CallerAutonomyLevel > m.MaxAutonomy {
			return nil, fmt.Errorf("%w: caller autonomy %d > max %d", ErrAutonomyExceeded, execCtx.CallerAutonomyLevel, m.MaxAutonomy)
		}
	}

	// 4. Shadow Mode Enforcement
	if execCtx.ExecutionMode == "SHADOW" {
		if !m.ShadowModeAllowed || m.SideEffectClass != SideEffectReadOnly {
			return nil, fmt.Errorf("%w: tool %s has side effect %s in SHADOW mode", ErrShadowModeProhibited, m.ToolID, m.SideEffectClass)
		}
	}

	// 5. Input Hash & Idempotency Check
	inputHash, err := req.ComputeArgsHash()
	if err != nil {
		return nil, fmt.Errorf("compute input hash: %w", err)
	}

	cachedResp, unlock, err := s.idempotency.CheckOrLock(
		ctx, execCtx.TenantID, execCtx.CallerID, m.ToolID, m.Version, req.IdempotencyKey, inputHash,
	)
	if err != nil {
		return nil, err
	}
	if cachedResp != nil {
		// Replay existing idempotent result
		return cachedResp, nil
	}

	// Check durable persistence if not in memory
	if s.store != nil {
		durableRec, err := s.store.GetInvocationByIdempotency(ctx, execCtx.TenantID, execCtx.CallerID, m.ToolID, m.Version, req.IdempotencyKey)
		if err == nil && durableRec != nil {
			if durableRec.RequestHash != inputHash {
				if unlock != nil {
					unlock(nil, ErrIdempotencyConflict)
				}
				return nil, fmt.Errorf("%w: key %s already used with different payload", ErrIdempotencyConflict, req.IdempotencyKey)
			}
			// If already reached a terminal state, replay authoritative result without re-executing handler
			switch durableRec.Status {
			case StatusSucceeded, StatusDenied, StatusFailed, StatusTimedOut:
				resp := durableRec.ToToolResponse()
				s.idempotency.RecordDurableResult(execCtx.TenantID, execCtx.CallerID, m.ToolID, m.Version, req.IdempotencyKey, inputHash, resp)
				if unlock != nil {
					unlock(resp, nil)
				}
				return resp, nil
			case StatusExecuting, StatusReceived, StatusAuthorized:
				// Previous process crashed mid-execution.
				// For READ_ONLY tools, deterministic recovery safely proceeds to re-execute and finalize.
				// For side-effectful tools, preserve UNCERTAIN recovery semantics.
				if m.SideEffectClass != SideEffectReadOnly {
					errUncertain := fmt.Errorf("%w: prior execution interrupted in status %s", ErrPreconditionFailed, durableRec.Status)
					if unlock != nil {
						unlock(nil, errUncertain)
					}
					return nil, errUncertain
				}
			}
		}
	}

	invocationID := fmt.Sprintf("inv-%s-%d", execCtx.RequestID, time.Now().UnixNano())

	// 6. Resource Preconditions (TOCTOU)
	if err := VerifyResourcePreconditions(req.ResourcePreconditions, execCtx); err != nil {
		if unlock != nil {
			unlock(nil, err)
		}
		return nil, err
	}

	// 7. Input Schema & Size Validation
	if err := ValidateInput(req.Args, DefaultMaxInputBytes); err != nil {
		if unlock != nil {
			unlock(nil, err)
		}
		return nil, err
	}

	// 8. Authoritative Policy Evaluation
	var policyDecision *policy.PolicyDecision
	if s.policyEngine != nil {
		policyReq := &policy.PolicyEvaluationRequest{
			RequestID: execCtx.RequestID,
			TenantID:  execCtx.TenantID,
			Subject: policy.PolicySubject{
				Type:          execCtx.CallerType,
				ID:            execCtx.CallerID,
				Roles:         execCtx.CallerRoles,
				AutonomyLevel: execCtx.CallerAutonomyLevel,
				TenantID:      execCtx.TenantID,
			},
			Action: m.PolicyAction,
			Resource: policy.PolicyResource{
				Type:     string(m.PolicyDomain),
				ID:       m.ToolID,
				SHA256:   execCtx.ArtifactSHA256,
				TenantID: execCtx.TenantID,
			},
			Workflow: policy.PolicyWorkflowContext{
				WorkflowID: execCtx.WorkflowID,
				Attempt:    execCtx.ResourceVersion,
			},
			Environment: policy.PolicyEnvironment{
				EvaluationTime: startTime,
				FleetMode:      execCtx.ExecutionMode,
			},
		}

		policyDecision, err = s.policyEngine.Evaluate(policyReq)
		if err != nil {
			if unlock != nil {
				unlock(nil, err)
			}
			return nil, fmt.Errorf("policy evaluation: %w", err)
		}

		// Enforce executable decision semantics
		if policyDecision.Decision == policy.DecisionDeny {
			s.recordDeniedInvocation(ctx, invocationID, execCtx, m, inputHash, policyDecision, startTime)
			if unlock != nil {
				unlock(nil, ErrPolicyDenial)
			}
			return nil, fmt.Errorf("%w: %v", ErrPolicyDenial, policyDecision.ReasonCodes)
		}
		if policyDecision.Decision == policy.DecisionRequireHuman {
			s.recordDeniedInvocation(ctx, invocationID, execCtx, m, inputHash, policyDecision, startTime)
			if unlock != nil {
				unlock(nil, ErrRequireHumanReview)
			}
			return nil, fmt.Errorf("%w: %v", ErrRequireHumanReview, policyDecision.ReasonCodes)
		}

		// 9. Prohibition Verification
		for _, reqCap := range m.RequiredCapabilities {
			if isProh, prohType := policy.IsCapabilityProhibited(policy.ToolCapability(reqCap), policyDecision.Prohibitions); isProh {
				s.recordDeniedInvocation(ctx, invocationID, execCtx, m, inputHash, policyDecision, startTime)
				if unlock != nil {
					unlock(nil, ErrCapabilityProhibited)
				}
				return nil, fmt.Errorf("%w: capability %s prohibited by policy (%s)", ErrCapabilityProhibited, reqCap, prohType)
			}
		}

		// 10. Pre-Execution Obligation Verification
		for _, obl := range policyDecision.Obligations {
			if IsPreExecutionObligation(obl.Type) {
				if err := VerifyPreExecution(ctx, obl, execCtx, m, evidence); err != nil {
					s.recordDeniedInvocation(ctx, invocationID, execCtx, m, inputHash, policyDecision, startTime)
					if unlock != nil {
						unlock(nil, err)
					}
					return nil, err
				}
			}
		}
	}

	// 11. Bounded Execution with Timeout & Panic Isolation
	timeoutCtx, cancel := context.WithTimeout(ctx, m.Timeout)
	defer cancel()

	var (
		rawOutput   json.RawMessage
		execErr     error
		execStatus  = StatusSucceeded
		errCode     string
		errMessage  string
	)

	func() {
		defer func() {
			if r := recover(); r != nil {
				execErr = fmt.Errorf("%w: %v", ErrToolPanicRecovered, r)
				execStatus = StatusFailed
				errCode = "TOOL_PANIC"
				errMessage = "Internal tool error"
				_ = debug.Stack() // isolated internally
			}
		}()

		rawOutput, execErr = tool.Handler(timeoutCtx, execCtx, req.Args)
	}()

	duration := time.Since(startTime)

	if execErr != nil {
		if errors.Is(timeoutCtx.Err(), context.DeadlineExceeded) {
			execStatus = StatusTimedOut
			execErr = ErrToolExecutionTimeout
			errCode = "TIMEOUT"
			errMessage = "Tool execution exceeded maximum allowed time"
		} else if execStatus != StatusFailed {
			execStatus = StatusFailed
			errCode = "EXECUTION_ERROR"
			errMessage = execErr.Error()
		}
	}

	// 12. Output Validation & Post-Execution Obligations
	var outputHash string
	if execStatus == StatusSucceeded {
		if err := ValidateOutput(rawOutput, m, execCtx); err != nil {
			execStatus = StatusFailed
			execErr = err
			errCode = "OUTPUT_VALIDATION_FAILED"
			errMessage = err.Error()
			rawOutput = nil
		} else {
			respDummy := &ToolResponse{Output: rawOutput}
			outputHash, _ = respDummy.ComputeOutputHash()

			// Post-execution obligations verification
			if policyDecision != nil {
				for _, obl := range policyDecision.Obligations {
					if !IsPreExecutionObligation(obl.Type) {
						if err := VerifyPostExecution(ctx, obl, execCtx, respDummy, evidence); err != nil {
							execStatus = StatusFailed
							execErr = err
							errCode = "POST_OBLIGATION_FAILED"
							errMessage = err.Error()
							rawOutput = nil
							break
						}
					}
				}
			}
		}
	}

	// Build authoritative ToolResponse
	resp := &ToolResponse{
		InvocationID: invocationID,
		ToolID:       m.ToolID,
		ToolVersion:  m.Version,
		Status:       execStatus,
		Output:       rawOutput,
		OutputBytes:  len(rawOutput),
		Duration:     duration,
		ManifestHash: m.ManifestHash,
		OutputHash:   outputHash,
		Timestamp:    startTime,
	}
	if policyDecision != nil {
		resp.PolicyDecisionHash = policyDecision.DecisionHash
		resp.PolicyBundleHash = policyDecision.PolicyBundleHash
	}
	if execErr != nil {
		resp.Error = &ToolError{
			Code:    errCode,
			Message: errMessage,
		}
	}

	// Durable persistence & outbox event
	if s.store != nil {
		rec := &InvocationRecord{
			ID:                  invocationID,
			TenantID:            execCtx.TenantID,
			ToolID:              m.ToolID,
			ToolVersion:         m.Version,
			ManifestHash:        m.ManifestHash,
			CallerType:          execCtx.CallerType,
			CallerID:            execCtx.CallerID,
			CallerAutonomyLevel: execCtx.CallerAutonomyLevel,
			WorkflowID:          execCtx.WorkflowID,
			IdempotencyKey:      req.IdempotencyKey,
			RequestHash:         inputHash,
			Status:              execStatus,
			InputHash:           inputHash,
			OutputHash:          outputHash,
			OutputPayload:       string(rawOutput),
			ErrorCode:           errCode,
			ErrorMessage:        errMessage,
			DurationMs:          duration.Milliseconds(),
			ExecutionMode:       execCtx.ExecutionMode,
			CreatedAt:           startTime,
		}
		if policyDecision != nil {
			rec.PolicyDecisionID = policyDecision.DecisionID
			rec.PolicyDecisionHash = policyDecision.DecisionHash
			rec.PolicyBundleHash = policyDecision.PolicyBundleHash
		}
		completedAt := time.Now().UTC()
		rec.CompletedAt = &completedAt

		_ = s.store.RecordInvocation(ctx, rec)
	}

	if unlock != nil {
		unlock(resp, execErr)
	}

	return resp, execErr
}

func (s *ToolGatewayService) recordDeniedInvocation(
	ctx context.Context,
	invocationID string,
	execCtx *TrustedExecutionContext,
	m *ToolManifest,
	inputHash string,
	policyDecision *policy.PolicyDecision,
	startTime time.Time,
) {
	if s.store == nil {
		return
	}
	rec := &InvocationRecord{
		ID:                  invocationID,
		TenantID:            execCtx.TenantID,
		ToolID:              m.ToolID,
		ToolVersion:         m.Version,
		ManifestHash:        m.ManifestHash,
		CallerType:          execCtx.CallerType,
		CallerID:            execCtx.CallerID,
		CallerAutonomyLevel: execCtx.CallerAutonomyLevel,
		WorkflowID:          execCtx.WorkflowID,
		IdempotencyKey:      fmt.Sprintf("denied-%s", invocationID),
		RequestHash:         inputHash,
		Status:              StatusDenied,
		InputHash:           inputHash,
		ErrorCode:           "POLICY_DENIED",
		ErrorMessage:        fmt.Sprintf("Decision: %s", policyDecision.Decision),
		DurationMs:          time.Since(startTime).Milliseconds(),
		ExecutionMode:       execCtx.ExecutionMode,
		CreatedAt:           startTime,
	}
	if policyDecision != nil {
		rec.PolicyDecisionID = policyDecision.DecisionID
		rec.PolicyDecisionHash = policyDecision.DecisionHash
		rec.PolicyBundleHash = policyDecision.PolicyBundleHash
	}
	completedAt := time.Now().UTC()
	rec.CompletedAt = &completedAt
	_ = s.store.RecordInvocation(ctx, rec)
}
