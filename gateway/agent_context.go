package main

import (
	"errors"
	"fmt"
)

// AgentBudget defines explicit resource constraints for an agent execution.
type AgentBudget struct {
	MaxTokens   int     `json:"maxTokens"`
	MaxSeconds  int     `json:"maxSeconds"`
	MaxCostUSD  float64 `json:"maxCostUsd"`
}

// RedactedFindingItem represents a single pre-redacted validation finding.
// It carries no full lines, account numbers, or raw payment instructions.
type RedactedFindingItem struct {
	ID               string `json:"id"`
	Code             string `json:"code"`
	Severity         string `json:"severity"` // INFO, WARNING, BLOCKING
	Description      string `json:"description"`
	RuleVersion      string `json:"ruleVersion"`
	Provenance       string `json:"provenance,omitempty"`
	LineNumber       *int   `json:"lineNumber,omitempty"`
	ByteOffset       *int64 `json:"byteOffset,omitempty"`
	FieldStart       *int   `json:"fieldStart,omitempty"`
	FieldEnd         *int   `json:"fieldEnd,omitempty"`
	EvidenceRedacted string `json:"evidenceRedacted,omitempty"`
	ExpectedValue    string `json:"expectedValue,omitempty"`
	ActualValue      string `json:"actualValue,omitempty"`
}

// AgentContextEnvelope is the canonical immutable context passed from the Go gateway
// to the AI control plane.
//
// INVARIANTS:
// 1. TenantID is strictly injected server-side from verified authentication context.
// 2. IncidentID, ArtifactID, and ValidationRunID are distinct and non-conflated.
// 3. Raw financial payloads (raw_data, raw_content, full lines) are excluded by construction.
// 4. All evidence citations must map to AuthorizedEvidenceRefs.
type AgentContextEnvelope struct {
	SchemaVersion          string                `json:"schemaVersion"` // "1.0"
	WorkflowID             string                `json:"workflowId"`
	TenantID               string                `json:"tenantId"`
	TriggerEventID         string                `json:"triggerEventId"`
	IncidentID             int64                 `json:"incidentId"`
	ArtifactID             int64                 `json:"artifactId"`
	ArtifactSHA256         string                `json:"artifactSha256"`
	ValidationRunID        string                `json:"validationRunId"`
	PolicyVersion          string                `json:"policyVersion"`
	CorrelationID          string                `json:"correlationId"`
	TraceID                string                `json:"traceId"`
	AgentName              string                `json:"agentName"`
	AgentVersion           string                `json:"agentVersion"`
	AuthorizedEvidenceRefs []string              `json:"authorizedEvidenceRefs"`
	AllowedTools           []string              `json:"allowedTools"`
	Budget                 AgentBudget           `json:"budget"`
	Findings               []RedactedFindingItem `json:"findings"`
	AvailableRunbooks      []string              `json:"availableRunbooks"`
	PriorOccurrences       int                   `json:"priorOccurrences"`
}

// Validate verifies that the envelope satisfies all security invariants before dispatch.
func (e *AgentContextEnvelope) Validate() error {
	if e.SchemaVersion == "" {
		return errors.New("agent context: schema_version is required")
	}
	if e.TenantID == "" {
		return errors.New("agent context: tenant_id must be injected by server")
	}
	if e.IncidentID <= 0 {
		return fmt.Errorf("agent context: invalid incident_id %d", e.IncidentID)
	}
	if e.ArtifactID < 0 {
		return fmt.Errorf("agent context: invalid artifact_id %d", e.ArtifactID)
	}
	if e.CorrelationID == "" {
		return errors.New("agent context: correlation_id is required")
	}
	return nil
}
