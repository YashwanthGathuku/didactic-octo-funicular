package toolgateway

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"sort"
	"strings"
	"time"

	"sentinel-gateway/internal/policy"
)

var (
	ErrAgentExecutionDisabled = errors.New("agent execution disabled by trusted control plane")
	ErrAgentBudgetExhausted   = errors.New("agent execution budget exhausted")
	ErrAgentDeadlineExceeded  = errors.New("agent workflow execution deadline exceeded")
)

// TrustedExecutionContext represents the immutable, server-generated context for tool execution.
// Models/callers cannot supply or override tenant ID, caller identity, roles, autonomy, policy state,
// kill switches, or workflow budgets.
type TrustedExecutionContext struct {
	RequestID             string           `json:"request_id"`
	IdempotencyKey        string           `json:"idempotency_key"`
	CorrelationID         string           `json:"correlation_id"`
	TraceID               string           `json:"trace_id"`
	TenantID              string           `json:"tenant_id"`
	CallerType            string           `json:"caller_type"` // AGENT, HUMAN, SERVICE, DETERMINISTIC_CONTROL, API
	CallerID              string           `json:"caller_id"`
	CallerRoles           []string         `json:"caller_roles"`
	CallerCapabilities    []ToolCapability `json:"caller_capabilities"`
	CallerAutonomyLevel   int              `json:"caller_autonomy_level"`
	WorkflowID            string           `json:"workflow_id,omitempty"`
	IncidentID            string           `json:"incident_id,omitempty"`
	ArtifactID            string           `json:"artifact_id,omitempty"`
	ArtifactSHA256        string           `json:"artifact_sha256,omitempty"`
	ResourceVersion       int              `json:"resource_version,omitempty"`
	AllowedTools          []string         `json:"allowed_tools"`
	ExecutionMode         string           `json:"execution_mode"` // SHADOW, ADVISORY, LIVE
	Timestamp             time.Time        `json:"timestamp"`

	// P13-15 trusted operational controls. These fields are injected by the Go
	// control plane and are checked before ToolGateway performs any lookup,
	// policy evaluation, idempotency mutation or tool execution.
	AgentExecutionDisabled bool      `json:"agent_execution_disabled,omitempty"`
	ExecutionDisableReason string    `json:"execution_disable_reason,omitempty"`
	KillSwitchGeneration   uint64    `json:"kill_switch_generation,omitempty"`
	ToolCallOrdinal        uint64    `json:"tool_call_ordinal,omitempty"`
	MaxToolCalls           uint64    `json:"max_tool_calls,omitempty"`
	WorkflowStartedAt      time.Time `json:"workflow_started_at,omitempty"`
	MaxWorkflowDurationSec int64     `json:"max_workflow_duration_sec,omitempty"`
}

// ResourcePreconditions defines TOCTOU verification constraints that must hold at execution time.
type ResourcePreconditions struct {
	ExpectedArtifactSHA256 string `json:"expected_artifact_sha256,omitempty"`
	ExpectedRowVersion     int    `json:"expected_row_version,omitempty"`
	ExpectedWorkflowState  string `json:"expected_workflow_state,omitempty"`
	ExpectedPolicyBundle   string `json:"expected_policy_bundle_hash,omitempty"`
}

// Validate checks that the execution context contains all mandatory trusted fields and that agent
// kill-switch/budget constraints hold. Human/service recovery operations deliberately do not inherit
// an agent kill switch; their own RBAC/policy checks still apply normally.
func (ctx *TrustedExecutionContext) Validate() error {
	if ctx.TenantID == "" {
		return ErrMissingTenantID
	}
	if ctx.RequestID == "" {
		return fmt.Errorf("%w: request_id is required", ErrInputValidationFailed)
	}
	if ctx.IdempotencyKey == "" {
		return fmt.Errorf("%w: idempotency_key is required", ErrInputValidationFailed)
	}
	if ctx.CallerID == "" {
		return fmt.Errorf("%w: caller_id is required", ErrInputValidationFailed)
	}
	if ctx.CallerType == "" {
		ctx.CallerType = "AGENT"
	}
	if ctx.ExecutionMode == "" {
		ctx.ExecutionMode = "LIVE"
	}
	if ctx.Timestamp.IsZero() {
		ctx.Timestamp = time.Now().UTC()
	}

	if strings.EqualFold(ctx.CallerType, "AGENT") {
		if ctx.AgentExecutionDisabled {
			reason := strings.TrimSpace(ctx.ExecutionDisableReason)
			if reason == "" {
				reason = "operator/control-plane kill switch"
			}
			return fmt.Errorf("%w: %s (generation=%d)", ErrAgentExecutionDisabled, reason, ctx.KillSwitchGeneration)
		}
		if ctx.MaxToolCalls > 0 {
			// ToolCallOrdinal is one-based for an attempted logical invocation. An
			// ordinal greater than the trusted maximum is rejected before any tool
			// side effect can occur.
			if ctx.ToolCallOrdinal == 0 {
				return fmt.Errorf("%w: tool_call_ordinal required when max_tool_calls is configured", ErrInputValidationFailed)
			}
			if ctx.ToolCallOrdinal > ctx.MaxToolCalls {
				return fmt.Errorf("%w: ordinal=%d max=%d", ErrAgentBudgetExhausted, ctx.ToolCallOrdinal, ctx.MaxToolCalls)
			}
		}
		if ctx.MaxWorkflowDurationSec < 0 {
			return fmt.Errorf("%w: max_workflow_duration_sec cannot be negative", ErrInputValidationFailed)
		}
		if ctx.MaxWorkflowDurationSec > 0 {
			if ctx.WorkflowStartedAt.IsZero() {
				return fmt.Errorf("%w: workflow_started_at required when duration budget is configured", ErrInputValidationFailed)
			}
			deadline := ctx.WorkflowStartedAt.Add(time.Duration(ctx.MaxWorkflowDurationSec) * time.Second)
			if !ctx.Timestamp.Before(deadline) {
				return fmt.Errorf("%w: deadline=%s", ErrAgentDeadlineExceeded, deadline.UTC().Format(time.RFC3339Nano))
			}
		}
	}
	return nil
}

// HasCapability checks if the trusted context holds the requested capability.
func (ctx *TrustedExecutionContext) HasCapability(cap ToolCapability) bool {
	for _, c := range ctx.CallerCapabilities {
		if c == cap {
			return true
		}
	}
	return false
}

// IsToolAllowed checks if the tool is in the context's allowed tool list (if non-empty).
func (ctx *TrustedExecutionContext) IsToolAllowed(toolID string) bool {
	if len(ctx.AllowedTools) == 0 {
		return true
	}
	for _, t := range ctx.AllowedTools {
		if t == toolID || t == "*" {
			return true
		}
	}
	return false
}

// CanonicalHash computes the RFC 8785 SHA-256 hash of the trusted context, including the operational
// controls that were authoritative at invocation time.
func (ctx *TrustedExecutionContext) CanonicalHash() (string, error) {
	roles := append([]string(nil), ctx.CallerRoles...)
	sort.Strings(roles)

	caps := make([]string, len(ctx.CallerCapabilities))
	for i, c := range ctx.CallerCapabilities {
		caps[i] = string(c)
	}
	sort.Strings(caps)

	allowed := append([]string(nil), ctx.AllowedTools...)
	sort.Strings(allowed)

	payload := map[string]interface{}{
		"schema_version":             "1.1",
		"request_id":                 ctx.RequestID,
		"idempotency_key":            ctx.IdempotencyKey,
		"correlation_id":             ctx.CorrelationID,
		"trace_id":                   ctx.TraceID,
		"tenant_id":                  ctx.TenantID,
		"caller_type":                ctx.CallerType,
		"caller_id":                  ctx.CallerID,
		"caller_roles":               roles,
		"caller_capabilities":        caps,
		"caller_autonomy_level":      ctx.CallerAutonomyLevel,
		"workflow_id":                ctx.WorkflowID,
		"incident_id":                ctx.IncidentID,
		"artifact_id":                ctx.ArtifactID,
		"artifact_sha256":            ctx.ArtifactSHA256,
		"resource_version":           ctx.ResourceVersion,
		"allowed_tools":              allowed,
		"execution_mode":             ctx.ExecutionMode,
		"timestamp":                  ctx.Timestamp.UTC().Format(time.RFC3339Nano),
		"agent_execution_disabled":   ctx.AgentExecutionDisabled,
		"execution_disable_reason":   ctx.ExecutionDisableReason,
		"kill_switch_generation":     ctx.KillSwitchGeneration,
		"tool_call_ordinal":          ctx.ToolCallOrdinal,
		"max_tool_calls":             ctx.MaxToolCalls,
		"workflow_started_at":         "",
		"max_workflow_duration_sec":  ctx.MaxWorkflowDurationSec,
	}
	if !ctx.WorkflowStartedAt.IsZero() {
		payload["workflow_started_at"] = ctx.WorkflowStartedAt.UTC().Format(time.RFC3339Nano)
	}

	canonicalBytes, err := policy.CanonicalJSON(payload)
	if err != nil {
		return "", err
	}
	h := sha256.Sum256(canonicalBytes)
	return hex.EncodeToString(h[:]), nil
}
