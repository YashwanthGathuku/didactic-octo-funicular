package auth

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"strings"
)

var (
	ErrUnknownAgentIdentity = errors.New("unknown or untrusted agent identity principal")
	ErrCrossProjectIdentity = errors.New("cross-project agent identity forbidden")
	ErrIdentitySpoofing     = errors.New("client-supplied agent identity spoofing detected")
	ErrAgentNotRegistered   = errors.New("agent identity is not in the fixed canonical roster")
	ErrUnauthorizedToolCall = errors.New("authenticated agent is not authorized for requested tool capability")
	ErrMissingAgentIdentity = errors.New("missing agent identity header in managed ingress")
)

type contextKey string

const (
	agentIdentityKey contextKey = "sentinelflow.agent_identity"
)

// AutonomyLevel defines the autonomy tier of an agent.
type AutonomyLevel string

const (
	AutonomyA1 AutonomyLevel = "A1" // Read-only advisory, synthesis, evaluation
	AutonomyA2 AutonomyLevel = "A2" // Proposal-only (e.g. declarative remediation candidate plans)
)

// RegisteredAgentIdentity represents a validated agent within SentinelFlow's fixed roster.
type RegisteredAgentIdentity struct {
	AgentName          string        `json:"agent_name"`
	Version            string        `json:"version"`
	AutonomyLevel      AutonomyLevel `json:"autonomy_level"`
	Principal          string        `json:"principal"`
	ProjectID          string        `json:"project_id"`
	AllowedCapabilities []string     `json:"allowed_capabilities"`
	DeniedCapabilities  []string     `json:"denied_capabilities"`
}

// FixedCanonicalRoster defines SentinelFlow's immutable fixed roster of 6 agents.
// Invariant: RegistryContains(agent) != SentinelFlowRosterAllows(agent)
var FixedCanonicalRoster = map[string]RegisteredAgentIdentity{
	"IncidentCommanderAgent": {
		AgentName:     "IncidentCommanderAgent",
		Version:       "1.0.0",
		AutonomyLevel: AutonomyA1,
		AllowedCapabilities: []string{
			"incident.get",
			"workflow.get",
			"artifact.metadata.get",
			"validation.findings.list_redacted",
		},
		DeniedCapabilities: []string{
			"artifact.release",
			"incident.approve",
			"ledger.mutate",
			"database.raw_sql",
			"system.shell",
			"agent.create_dynamic",
			"artifact.write_direct",
			"remediation.candidate.create",
		},
	},
	"DiagnosisAgent": {
		AgentName:     "DiagnosisAgent",
		Version:       "1.0.0",
		AutonomyLevel: AutonomyA1,
		AllowedCapabilities: []string{
			"incident.get",
			"validation.findings.list_redacted",
			"artifact.metadata.get",
			"workflow.get",
			"memory.retrieve",
		},
		DeniedCapabilities: []string{
			"artifact.release",
			"incident.approve",
			"ledger.mutate",
			"database.raw_sql",
			"system.shell",
			"agent.create_dynamic",
			"artifact.write_direct",
			"remediation.candidate.create",
		},
	},
	"PolicySLAAgent": {
		AgentName:     "PolicySLAAgent",
		Version:       "1.0.0",
		AutonomyLevel: AutonomyA1,
		AllowedCapabilities: []string{
			"incident.get",
			"workflow.get",
			"artifact.metadata.get",
			"memory.profile.get",
		},
		DeniedCapabilities: []string{
			"artifact.release",
			"incident.approve",
			"ledger.mutate",
			"database.raw_sql",
			"system.shell",
			"agent.create_dynamic",
			"artifact.write_direct",
			"remediation.candidate.create",
		},
	},
	"MemoryAgent": {
		AgentName:     "MemoryAgent",
		Version:       "1.0.0",
		AutonomyLevel: AutonomyA1,
		AllowedCapabilities: []string{
			"incident.get",
			"workflow.get",
			"artifact.metadata.get",
			"memory.retrieve",
			"memory.profile.get",
		},
		DeniedCapabilities: []string{
			"artifact.release",
			"incident.approve",
			"ledger.mutate",
			"database.raw_sql",
			"system.shell",
			"agent.create_dynamic",
			"artifact.write_direct",
			"remediation.candidate.create",
			"memory.write_direct",
			"evidence.mint_authoritative",
			"source.validate_authoritative",
			"policy.override",
			"candidate.verify",
		},
	},
	"RemediationAgent": {
		AgentName:     "RemediationAgent",
		Version:       "1.0.0",
		AutonomyLevel: AutonomyA2,
		AllowedCapabilities: []string{
			"incident.get",
			"validation.findings.list_redacted",
			"artifact.metadata.get",
			"workflow.get",
			"memory.retrieve",
			"remediation.candidate.create",
		},
		DeniedCapabilities: []string{
			"artifact.release",
			"incident.approve",
			"ledger.mutate",
			"database.raw_sql",
			"system.shell",
			"agent.create_dynamic",
			"artifact.write_direct",
		},
	},
	"VerifierAgent": {
		AgentName:     "VerifierAgent",
		Version:       "1.0.0",
		AutonomyLevel: AutonomyA1,
		AllowedCapabilities: []string{
			"incident.get",
			"validation.findings.list_redacted",
			"artifact.metadata.get",
			"workflow.get",
			"verification.result.get",
		},
		DeniedCapabilities: []string{
			"artifact.release",
			"incident.approve",
			"ledger.mutate",
			"database.raw_sql",
			"system.shell",
			"agent.create_dynamic",
			"artifact.write_direct",
			"remediation.candidate.create",
		},
	},
}

// AgentIdentityValidator validates Google Agent Identity and maps it to SentinelFlow's fixed roster.
type AgentIdentityValidator struct {
	expectedProjectID string
}

// NewAgentIdentityValidator creates an AgentIdentityValidator bound to expected project.
func NewAgentIdentityValidator(expectedProjectID string) *AgentIdentityValidator {
	return &AgentIdentityValidator{
		expectedProjectID: expectedProjectID,
	}
}

// ValidatePrincipal extracts, verifies, and maps an incoming Agent Identity principal.
func (v *AgentIdentityValidator) ValidatePrincipal(principal string) (*RegisteredAgentIdentity, error) {
	if principal == "" {
		return nil, ErrMissingAgentIdentity
	}

	trimmed := strings.TrimSpace(principal)

	// Expected formats:
	// 1. SPIFFE: spiffe://<project-id>.iam.gserviceaccount.com/agent/<agent-name>
	// 2. ServiceAccount: serviceAccount:sentinelflow-<agent-slug>@<project-id>.iam.gserviceaccount.com
	// 3. Local/Test prefix: test-agent:<agent-name>

	var agentName, projectID string

	if strings.HasPrefix(trimmed, "spiffe://") {
		// Parse SPIFFE ID
		rest := strings.TrimPrefix(trimmed, "spiffe://")
		parts := strings.Split(rest, "/")
		if len(parts) >= 3 && parts[1] == "agent" {
			hostParts := strings.Split(parts[0], ".")
			if len(hostParts) > 0 {
				projectID = hostParts[0]
			}
			agentName = parts[2]
		}
	} else if strings.HasPrefix(trimmed, "serviceAccount:") {
		rest := strings.TrimPrefix(trimmed, "serviceAccount:")
		parts := strings.Split(rest, "@")
		if len(parts) == 2 {
			saName := parts[0]
			domainParts := strings.Split(parts[1], ".")
			if len(domainParts) > 0 {
				projectID = domainParts[0]
			}
			agentName = resolveAgentNameFromSlug(saName)
		}
	} else if strings.HasPrefix(trimmed, "test-agent:") {
		agentName = strings.TrimPrefix(trimmed, "test-agent:")
		projectID = v.expectedProjectID
	}

	// Normalize agent name
	agentName = normalizeAgentName(agentName)

	if agentName == "" {
		return nil, fmt.Errorf("%w: cannot parse agent identity from %q", ErrUnknownAgentIdentity, principal)
	}

	// Project boundary assertion
	if v.expectedProjectID != "" && projectID != "" && projectID != v.expectedProjectID {
		return nil, fmt.Errorf("%w: principal project %q != expected %q", ErrCrossProjectIdentity, projectID, v.expectedProjectID)
	}

	// Look up in fixed canonical roster
	identity, ok := FixedCanonicalRoster[agentName]
	if !ok {
		return nil, fmt.Errorf("%w: agent %q is not in the fixed canonical roster", ErrAgentNotRegistered, agentName)
	}

	res := identity
	res.Principal = trimmed
	res.ProjectID = projectID
	return &res, nil
}

// AuthorizeCapability checks whether the authenticated agent is authorized for a specific capability.
// Formal Invariant: AgentIdentityValid != ToolAuthorization
func (v *AgentIdentityValidator) AuthorizeCapability(identity *RegisteredAgentIdentity, capability string) error {
	if identity == nil {
		return ErrMissingAgentIdentity
	}

	// Check explicit denies first
	for _, denied := range identity.DeniedCapabilities {
		if denied == capability {
			return fmt.Errorf("%w: capability %q is explicitly denied for agent %q", ErrUnauthorizedToolCall, capability, identity.AgentName)
		}
	}

	// Check allowed capabilities
	for _, allowed := range identity.AllowedCapabilities {
		if allowed == capability {
			return nil
		}
	}

	return fmt.Errorf("%w: capability %q is not in allowed capabilities for agent %q", ErrUnauthorizedToolCall, capability, identity.AgentName)
}

func normalizeAgentName(raw string) string {
	lower := strings.ToLower(strings.TrimSpace(raw))
	switch lower {
	case "commander", "incidentcommander", "incidentcommanderagent", "incident-commander":
		return "IncidentCommanderAgent"
	case "diagnosis", "diagnosisagent", "diagnostician":
		return "DiagnosisAgent"
	case "policysla", "policyslaagent", "policy-sla", "policy_sla":
		return "PolicySLAAgent"
	case "memory", "memoryagent":
		return "MemoryAgent"
	case "remediation", "remediationagent", "remediator":
		return "RemediationAgent"
	case "verifier", "verifieragent", "critic", "criticagent":
		return "VerifierAgent"
	default:
		// Exact match check
		for canonical := range FixedCanonicalRoster {
			if strings.EqualFold(canonical, raw) {
				return canonical
			}
		}
		return ""
	}
}

func resolveAgentNameFromSlug(slug string) string {
	slug = strings.TrimPrefix(slug, "sentinelflow-")
	slug = strings.TrimSuffix(slug, "-sa")
	return normalizeAgentName(slug)
}

// AgentIdentityMiddleware extracts and validates the Agent Identity header on managed ingress routes.
func (v *AgentIdentityValidator) AgentIdentityMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		principal := r.Header.Get("X-Agent-Identity-Principal")
		if principal == "" {
			principal = r.Header.Get("X-Goog-Authenticated-User-Email")
		}
		if principal == "" {
			http.Error(w, `{"error":"missing agent identity header"}`, http.StatusUnauthorized)
			return
		}

		identity, err := v.ValidatePrincipal(principal)
		if err != nil {
			http.Error(w, fmt.Sprintf(`{"error":%q}`, err.Error()), http.StatusForbidden)
			return
		}

		ctx := ContextWithAgentIdentity(r.Context(), identity)
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

// Context helpers
func ContextWithAgentIdentity(ctx context.Context, identity *RegisteredAgentIdentity) context.Context {
	return context.WithValue(ctx, agentIdentityKey, identity)
}

func AgentIdentityFromContext(ctx context.Context) (*RegisteredAgentIdentity, bool) {
	val, ok := ctx.Value(agentIdentityKey).(*RegisteredAgentIdentity)
	return val, ok
}
