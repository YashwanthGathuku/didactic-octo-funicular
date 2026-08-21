package toolgateway

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"sort"
	"time"

	"sentinel-gateway/internal/policy"
)

// ToolManifest is the versioned, immutable specification for an executable tool.
type ToolManifest struct {
	ToolID                       string               `json:"tool_id"`
	Version                      string               `json:"version"`
	Description                  string               `json:"description"`
	Owner                        string               `json:"owner"`
	Status                       ManifestStatus       `json:"status"`
	PolicyDomain                 policy.PolicyDomain  `json:"policy_domain"`
	PolicyAction                 string               `json:"policy_action"`
	RequiredCapabilities         []ToolCapability     `json:"required_capabilities"`
	SideEffectClass              SideEffectClass      `json:"side_effect_class"`
	MinAutonomy                  int                  `json:"min_autonomy"`
	MaxAutonomy                  int                  `json:"max_autonomy"`
	InputContract                string               `json:"input_contract"`
	InputSchemaHash              string               `json:"input_schema_hash"`
	OutputContract               string               `json:"output_contract"`
	OutputSchemaHash             string               `json:"output_schema_hash"`
	Timeout                      time.Duration        `json:"timeout"`
	MaxOutputBytes               int64                `json:"max_output_bytes"`
	DataClassifications          []DataClassification `json:"data_classifications"`
	AllowedOutputClassifications []DataClassification `json:"allowed_output_classifications"`
	IdempotencyRequired          bool                 `json:"idempotency_required"`
	VerificationRequired         bool                 `json:"verification_required"`
	ShadowModeAllowed            bool                 `json:"shadow_mode_allowed"`
	AllowedExecutionModes        []string             `json:"allowed_execution_modes"`
	ManifestHash                 string               `json:"manifest_hash"`
}

// Validate verifies that a ToolManifest conforms to all security and schema invariants.
func (m *ToolManifest) Validate() error {
	if m.ToolID == "" {
		return fmt.Errorf("%w: tool_id is required", ErrInvalidManifest)
	}
	if m.Version == "" {
		return fmt.Errorf("%w: version is required", ErrInvalidManifest)
	}
	if m.PolicyAction == "" {
		return fmt.Errorf("%w: policy_action is required", ErrInvalidManifest)
	}
	if m.SideEffectClass == "" {
		return fmt.Errorf("%w: side_effect_class is required", ErrInvalidManifest)
	}
	if m.SideEffectClass == SideEffectIrreversibleFinancial && m.MaxAutonomy > 0 {
		return fmt.Errorf("%w: tool %s has side effect %s with max autonomy %d",
			ErrIrreversibleFinancialAgent, m.ToolID, m.SideEffectClass, m.MaxAutonomy)
	}
	if m.MinAutonomy < 0 || m.MaxAutonomy < m.MinAutonomy {
		return fmt.Errorf("%w: invalid autonomy bounds [%d, %d]", ErrInvalidManifest, m.MinAutonomy, m.MaxAutonomy)
	}
	if m.Timeout <= 0 {
		m.Timeout = DefaultToolTimeout
	}
	if m.Timeout > MaxToolTimeout {
		return fmt.Errorf("%w: timeout %v exceeds maximum %v", ErrInvalidManifest, m.Timeout, MaxToolTimeout)
	}
	if m.MaxOutputBytes <= 0 {
		m.MaxOutputBytes = DefaultMaxOutputBytes
	}
	if m.Status == "" {
		m.Status = ManifestStatusActive
	}
	// If AllowedOutputClassifications is empty, default to DataClassifications
	if len(m.AllowedOutputClassifications) == 0 && len(m.DataClassifications) > 0 {
		m.AllowedOutputClassifications = m.DataClassifications
	}
	return nil
}

// ComputeManifestHash computes the RFC 8785 canonical SHA-256 hash of a ToolManifest.
func (m *ToolManifest) ComputeManifestHash() (string, error) {
	caps := make([]string, len(m.RequiredCapabilities))
	for i, c := range m.RequiredCapabilities {
		caps[i] = string(c)
	}
	sort.Strings(caps)

	modes := make([]string, len(m.AllowedExecutionModes))
	copy(modes, m.AllowedExecutionModes)
	sort.Strings(modes)

	classifications := make([]string, len(m.DataClassifications))
	for i, c := range m.DataClassifications {
		classifications[i] = string(c)
	}
	sort.Strings(classifications)

	allowedOut := make([]string, len(m.AllowedOutputClassifications))
	for i, c := range m.AllowedOutputClassifications {
		allowedOut[i] = string(c)
	}
	sort.Strings(allowedOut)

	payload := map[string]interface{}{
		"schema_version":                 "1.0",
		"tool_id":                        m.ToolID,
		"version":                        m.Version,
		"description":                    m.Description,
		"owner":                          m.Owner,
		"status":                         string(m.Status),
		"policy_domain":                  string(m.PolicyDomain),
		"policy_action":                  m.PolicyAction,
		"required_capabilities":          caps,
		"side_effect_class":              string(m.SideEffectClass),
		"min_autonomy":                   m.MinAutonomy,
		"max_autonomy":                   m.MaxAutonomy,
		"input_contract":                 m.InputContract,
		"input_schema_hash":              m.InputSchemaHash,
		"output_contract":                m.OutputContract,
		"output_schema_hash":             m.OutputSchemaHash,
		"timeout_ms":                     m.Timeout.Milliseconds(),
		"max_output_bytes":               m.MaxOutputBytes,
		"data_classifications":           classifications,
		"allowed_output_classifications": allowedOut,
		"idempotency_required":           m.IdempotencyRequired,
		"verification_required":          m.VerificationRequired,
		"shadow_mode_allowed":            m.ShadowModeAllowed,
		"allowed_execution_modes":        modes,
	}

	canonicalBytes, err := policy.CanonicalJSON(payload)
	if err != nil {
		return "", fmt.Errorf("canonicalize manifest: %w", err)
	}

	h := sha256.Sum256(canonicalBytes)
	return hex.EncodeToString(h[:]), nil
}
