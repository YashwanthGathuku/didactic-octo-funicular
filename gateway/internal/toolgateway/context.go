package toolgateway

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"sort"
	"time"

	"sentinel-gateway/internal/policy"
)

// TrustedExecutionContext represents the immutable, server-generated context for tool execution.
// Models / callers cannot supply or override tenant_id, caller identity, roles, autonomy, or policy state.
type TrustedExecutionContext struct {
	RequestID             string           `json:"request_id"`
	IdempotencyKey        string           `json:"idempotency_key"`
	CorrelationID         string           `json:"correlation_id"`
	TraceID               string           `json:"trace_id"`
	TenantID              string           `json:"tenant_id"`
	CallerType            string           `json:"caller_type"` // "AGENT", "HUMAN", "SYSTEM", "API"
	CallerID              string           `json:"caller_id"`
	CallerRoles           []string         `json:"caller_roles"`
	CallerCapabilities    []ToolCapability `json:"caller_capabilities"`
	CallerAutonomyLevel   int              `json:"caller_autonomy_level"`
	WorkflowID            string           `json:"workflow_id,omitempty"`
	IncidentID            string           `json:"incident_id,omitempty"`
	ArtifactID            string           `json:"artifact_id,omitempty"`
	ArtifactSHA256        string           `json:"artifact_sha256,omitempty"`
	ResourceVersion       int              `json:"resource_version,omitempty"`
	AllowedTools          []string         `json:"allowed_tools"` // Context allowlist
	ExecutionMode         string           `json:"execution_mode"` // "SHADOW", "ADVISORY", "LIVE"
	Timestamp             time.Time        `json:"timestamp"`
}

// ResourcePreconditions defines TOCTOU verification constraints that must hold at execution time.
type ResourcePreconditions struct {
	ExpectedArtifactSHA256 string `json:"expected_artifact_sha256,omitempty"`
	ExpectedRowVersion     int    `json:"expected_row_version,omitempty"`
	ExpectedWorkflowState  string `json:"expected_workflow_state,omitempty"`
	ExpectedPolicyBundle   string `json:"expected_policy_bundle_hash,omitempty"`
}

// Validate checks that the execution context contains all mandatory trusted fields.
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
		return true // No explicit allowlist restriction
	}
	for _, t := range ctx.AllowedTools {
		if t == toolID || t == "*" {
			return true
		}
	}
	return false
}

// CanonicalHash computes the RFC 8785 SHA-256 hash of the trusted context.
func (ctx *TrustedExecutionContext) CanonicalHash() (string, error) {
	roles := make([]string, len(ctx.CallerRoles))
	copy(roles, ctx.CallerRoles)
	sort.Strings(roles)

	caps := make([]string, len(ctx.CallerCapabilities))
	for i, c := range ctx.CallerCapabilities {
		caps[i] = string(c)
	}
	sort.Strings(caps)

	allowed := make([]string, len(ctx.AllowedTools))
	copy(allowed, ctx.AllowedTools)
	sort.Strings(allowed)

	payload := map[string]interface{}{
		"schema_version":        "1.0",
		"request_id":            ctx.RequestID,
		"idempotency_key":       ctx.IdempotencyKey,
		"correlation_id":        ctx.CorrelationID,
		"trace_id":              ctx.TraceID,
		"tenant_id":             ctx.TenantID,
		"caller_type":           ctx.CallerType,
		"caller_id":             ctx.CallerID,
		"caller_roles":          roles,
		"caller_capabilities":   caps,
		"caller_autonomy_level": ctx.CallerAutonomyLevel,
		"workflow_id":           ctx.WorkflowID,
		"incident_id":           ctx.IncidentID,
		"artifact_id":           ctx.ArtifactID,
		"artifact_sha256":       ctx.ArtifactSHA256,
		"resource_version":      ctx.ResourceVersion,
		"allowed_tools":         allowed,
		"execution_mode":        ctx.ExecutionMode,
		"timestamp":             ctx.Timestamp.Format(time.RFC3339),
	}

	canonicalBytes, err := policy.CanonicalJSON(payload)
	if err != nil {
		return "", err
	}
	h := sha256.Sum256(canonicalBytes)
	return hex.EncodeToString(h[:]), nil
}
