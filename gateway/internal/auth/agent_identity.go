package auth

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"os"
	"strings"
)

var (
	ErrUnknownAgentIdentity = errors.New("unknown or untrusted agent identity principal")
	ErrCrossProjectIdentity = errors.New("cross-project agent identity forbidden")
	ErrIdentitySpoofing     = errors.New("client-supplied agent identity spoofing detected")
	ErrAgentNotRegistered   = errors.New("agent identity is not in the fixed canonical roster")
	ErrUnauthorizedToolCall = errors.New("authenticated agent is not authorized for requested tool capability")
	ErrMissingAgentIdentity = errors.New("missing agent identity in managed ingress")
)

type contextKey string

const agentIdentityKey contextKey = "sentinelflow.agent_identity"

// AutonomyLevel defines the maximum autonomy of a specialist. It is not an
// authorization grant; every tool invocation is still evaluated by ToolGateway.
type AutonomyLevel string

const (
	AutonomyA1 AutonomyLevel = "A1" // read-only advisory / synthesis / evaluation
	AutonomyA2 AutonomyLevel = "A2" // proposal-only remediation intent
)

// RegisteredAgentIdentity is SentinelFlow's internal fixed-roster identity.
// Principal is populated only after either a local test fixture is parsed or a
// managed IAP assertion is cryptographically verified.
type RegisteredAgentIdentity struct {
	AgentName           string        `json:"agent_name"`
	Version             string        `json:"version"`
	AutonomyLevel       AutonomyLevel `json:"autonomy_level"`
	Principal           string        `json:"principal"`
	ProjectID           string        `json:"project_id"`
	AllowedCapabilities []string      `json:"allowed_capabilities"`
	DeniedCapabilities  []string      `json:"denied_capabilities"`
}

var commonDenied = []string{
	"artifact.release",
	"incident.approve",
	"ledger.mutate",
	"database.raw_sql",
	"system.shell",
	"agent.create_dynamic",
	"artifact.write_direct",
}

func rosterAgent(name string, level AutonomyLevel, allowed, additionalDenied []string) RegisteredAgentIdentity {
	denied := append([]string(nil), commonDenied...)
	denied = append(denied, additionalDenied...)
	return RegisteredAgentIdentity{
		AgentName:           name,
		Version:             "1.0.0",
		AutonomyLevel:       level,
		AllowedCapabilities: append([]string(nil), allowed...),
		DeniedCapabilities:  denied,
	}
}

// FixedCanonicalRoster is application authority. Google Agent Registry is
// inventory/discovery and never automatically expands this roster.
var FixedCanonicalRoster = map[string]RegisteredAgentIdentity{
	"IncidentCommanderAgent": rosterAgent(
		"IncidentCommanderAgent", AutonomyA1,
		[]string{"incident.get", "workflow.get", "artifact.metadata.get", "validation.findings.list_redacted", "lens.query"},
		[]string{"remediation.candidate.create"},
	),
	"DiagnosisAgent": rosterAgent(
		"DiagnosisAgent", AutonomyA1,
		[]string{"incident.get", "validation.findings.list_redacted", "artifact.metadata.get", "workflow.get", "memory.retrieve"},
		[]string{"remediation.candidate.create"},
	),
	"PolicySLAAgent": rosterAgent(
		"PolicySLAAgent", AutonomyA1,
		[]string{"incident.get", "workflow.get", "artifact.metadata.get", "memory.profile.get"},
		[]string{"remediation.candidate.create"},
	),
	"MemoryAgent": rosterAgent(
		"MemoryAgent", AutonomyA1,
		[]string{"incident.get", "workflow.get", "artifact.metadata.get", "memory.retrieve", "memory.profile.get"},
		[]string{
			"remediation.candidate.create", "memory.write_direct", "evidence.mint_authoritative",
			"source.validate_authoritative", "policy.override", "candidate.verify",
		},
	),
	"RemediationAgent": rosterAgent(
		"RemediationAgent", AutonomyA2,
		[]string{"incident.get", "validation.findings.list_redacted", "artifact.metadata.get", "workflow.get", "memory.retrieve", "remediation.candidate.create"},
		nil,
	),
	"VerifierAgent": rosterAgent(
		"VerifierAgent", AutonomyA1,
		[]string{"incident.get", "validation.findings.list_redacted", "artifact.metadata.get", "workflow.get", "verification.result.get"},
		[]string{"remediation.candidate.create"},
	),
	"ReturnRiskAgent": rosterAgent(
		"ReturnRiskAgent", AutonomyA1,
		[]string{"incident.get", "workflow.get", "memory.retrieve", "returnrisk.result.get", "lens.query"},
		[]string{"remediation.candidate.create"},
	),
}

// AgentIdentityValidator maps an already authenticated workload plus an
// internal fixed-roster specialist name to capability metadata.
type AgentIdentityValidator struct {
	expectedProjectID string
}

func NewAgentIdentityValidator(expectedProjectID string) *AgentIdentityValidator {
	return &AgentIdentityValidator{expectedProjectID: expectedProjectID}
}

// ValidatePrincipal parses legacy/local fixture principal formats for unit
// tests and compatibility. IT IS NOT CRYPTOGRAPHIC ATTESTATION. Managed ingress
// must use ManagedAgentIdentityMiddleware from iap_verifier.go.
func (v *AgentIdentityValidator) ValidatePrincipal(principal string) (*RegisteredAgentIdentity, error) {
	if strings.TrimSpace(principal) == "" {
		return nil, ErrMissingAgentIdentity
	}
	trimmed := strings.TrimSpace(principal)
	var agentName, projectID string

	switch {
	case strings.HasPrefix(trimmed, "test-agent:"):
		agentName = strings.TrimPrefix(trimmed, "test-agent:")
		projectID = v.expectedProjectID
	case strings.HasPrefix(trimmed, "spiffe://"):
		// Legacy P11 fixture format retained only so historical tests and evidence
		// remain reproducible. Real Google Agent Identity uses a resource-bound
		// agents.* trust domain and is verified by IAP, not this parser.
		rest := strings.TrimPrefix(trimmed, "spiffe://")
		parts := strings.Split(rest, "/")
		if len(parts) >= 3 && parts[1] == "agent" {
			hostParts := strings.Split(parts[0], ".")
			if len(hostParts) > 0 {
				projectID = hostParts[0]
			}
			agentName = parts[2]
		}
	case strings.HasPrefix(trimmed, "serviceAccount:"):
		rest := strings.TrimPrefix(trimmed, "serviceAccount:")
		parts := strings.Split(rest, "@")
		if len(parts) == 2 {
			agentName = resolveAgentNameFromSlug(parts[0])
			domainParts := strings.Split(parts[1], ".")
			if len(domainParts) > 0 {
				projectID = domainParts[0]
			}
		}
	}

	agentName = normalizeAgentName(agentName)
	if agentName == "" {
		return nil, fmt.Errorf("%w: cannot parse fixture identity", ErrUnknownAgentIdentity)
	}
	if v.expectedProjectID != "" && projectID != "" && projectID != v.expectedProjectID {
		return nil, fmt.Errorf("%w: principal project %q != expected %q", ErrCrossProjectIdentity, projectID, v.expectedProjectID)
	}
	identity, ok := FixedCanonicalRoster[agentName]
	if !ok {
		return nil, fmt.Errorf("%w: agent %q", ErrAgentNotRegistered, agentName)
	}
	res := identity
	res.Principal = trimmed
	res.ProjectID = projectID
	return &res, nil
}

// AuthorizeCapability enforces the fixed-roster capability ceiling after
// workload authentication. Identity validity alone is insufficient.
func (v *AgentIdentityValidator) AuthorizeCapability(identity *RegisteredAgentIdentity, capability string) error {
	if identity == nil {
		return ErrMissingAgentIdentity
	}
	for _, denied := range identity.DeniedCapabilities {
		if denied == capability {
			return fmt.Errorf("%w: capability %q explicitly denied for %q", ErrUnauthorizedToolCall, capability, identity.AgentName)
		}
	}
	for _, allowed := range identity.AllowedCapabilities {
		if allowed == capability {
			return nil
		}
	}
	return fmt.Errorf("%w: capability %q not allowlisted for %q", ErrUnauthorizedToolCall, capability, identity.AgentName)
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
	case "returnrisk", "returnriskagent", "return-risk", "return_risk":
		return "ReturnRiskAgent"
	default:
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

// AgentIdentityMiddleware is deliberately local-test-only. Previous P11 code
// trusted a client-authored identity header; that must never be used in a
// managed deployment. Managed mode fails closed and requires the signed-IAP
// middleware from iap_verifier.go.
func (v *AgentIdentityValidator) AgentIdentityMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.EqualFold(strings.TrimSpace(os.Getenv("SENTINEL_PLATFORM_MODE")), "managed") {
			http.Error(w, `{"error":"legacy identity middleware disabled in managed mode"}`, http.StatusUnauthorized)
			return
		}
		principal := strings.TrimSpace(r.Header.Get("X-Sentinel-Test-Principal"))
		if !strings.HasPrefix(principal, "test-agent:") {
			http.Error(w, `{"error":"local test identity required"}`, http.StatusUnauthorized)
			return
		}
		identity, err := v.ValidatePrincipal(principal)
		if err != nil {
			http.Error(w, `{"error":"invalid local test identity"}`, http.StatusForbidden)
			return
		}
		ctx := ContextWithAgentIdentity(r.Context(), identity)
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

func ContextWithAgentIdentity(ctx context.Context, identity *RegisteredAgentIdentity) context.Context {
	return context.WithValue(ctx, agentIdentityKey, identity)
}

func AgentIdentityFromContext(ctx context.Context) (*RegisteredAgentIdentity, bool) {
	val, ok := ctx.Value(agentIdentityKey).(*RegisteredAgentIdentity)
	return val, ok
}
