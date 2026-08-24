package toolgateway

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"time"

	"sentinel-gateway/internal/policy"
)

// ToolRequest represents an incoming request to invoke a registered tool.
// Args contains untrusted arguments provided by the caller/model.
type ToolRequest struct {
	ToolID                string                 `json:"tool_id"`
	ToolVersion           string                 `json:"tool_version,omitempty"` // If empty, resolves to active version
	Args                  json.RawMessage        `json:"args"`
	IdempotencyKey        string                 `json:"idempotency_key"`
	ResourcePreconditions *ResourcePreconditions `json:"resource_preconditions,omitempty"`
}

// ComputeArgsHash computes the RFC 8785 canonical hash of the raw arguments.
func (r *ToolRequest) ComputeArgsHash() (string, error) {
	if len(r.Args) == 0 {
		h := sha256.Sum256([]byte("{}"))
		return hex.EncodeToString(h[:]), nil
	}
	canonicalBytes, err := policy.CanonicalJSON(r.Args)
	if err != nil {
		return "", fmt.Errorf("canonicalize args: %w", err)
	}
	h := sha256.Sum256(canonicalBytes)
	return hex.EncodeToString(h[:]), nil
}

// ToolError represents a structured, safe error returned from tool execution.
// Sensitive internals, credentials, or stack traces are never exposed to the AI tier.
type ToolError struct {
	Code    string                 `json:"code"`
	Message string                 `json:"message"`
	Details map[string]interface{} `json:"details,omitempty"`
}

func (e *ToolError) Error() string {
	return fmt.Sprintf("[%s] %s", e.Code, e.Message)
}

// ToolResponse represents the complete, verified outcome of a tool execution.
type ToolResponse struct {
	InvocationID       string           `json:"invocation_id"`
	ToolID             string           `json:"tool_id"`
	ToolVersion        string           `json:"tool_version"`
	Status             InvocationStatus `json:"status"`
	Output             json.RawMessage  `json:"output,omitempty"`
	Error              *ToolError       `json:"error,omitempty"`
	OutputBytes        int              `json:"output_bytes"`
	Duration           time.Duration    `json:"duration"`
	PolicyDecisionHash string           `json:"policy_decision_hash,omitempty"`
	PolicyBundleHash   string           `json:"policy_bundle_hash,omitempty"`
	ManifestHash       string           `json:"manifest_hash,omitempty"`
	OutputHash         string           `json:"output_hash,omitempty"`
	Timestamp          time.Time        `json:"timestamp"`
}

// ComputeOutputHash computes the RFC 8785 canonical hash of the tool output.
func (r *ToolResponse) ComputeOutputHash() (string, error) {
	if len(r.Output) == 0 {
		return "", nil
	}
	canonicalBytes, err := policy.CanonicalJSON(r.Output)
	if err != nil {
		return "", err
	}
	h := sha256.Sum256(canonicalBytes)
	return hex.EncodeToString(h[:]), nil
}
