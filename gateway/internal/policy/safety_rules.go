package policy

import (
	"errors"
	"fmt"
	"time"
)

var (
	ErrSafetyBootstrapFailed = errors.New("mandatory Sentinel safety policy bootstrap failed: safety bundle incomplete or corrupted")
)

// DefaultSafetyEffectiveDate is the baseline effective time for foundational safety rules.
var DefaultSafetyEffectiveDate = time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)

// MandatorySafetyPolicyIDs contains the non-negotiable safety rules that must exist at engine boot.
var MandatorySafetyPolicyIDs = []string{
	"SF-SAFE-001",
	"SF-SAFE-002",
	"SF-SAFE-003",
	"SF-SAFE-004",
	"SF-SAFE-005",
	"SF-SAFE-006",
}

// SeedSafetyPolicies returns the foundational, immutable SentinelFlow safety policies.
func SeedSafetyPolicies() []*PolicyDefinition {
	rules := []*PolicyDefinition{
		// SF-SAFE-001: Original quarantined artifacts are strictly immutable
		{
			PolicyID:      "SF-SAFE-001",
			Version:       1,
			Domain:        DomainArtifact,
			Layer:         LayerSentinelSafety,
			Priority:      100,
			Status:        StatusActive,
			EffectiveFrom: DefaultSafetyEffectiveDate,
			Action:        ActionModifyOriginalArtifact,
			SubjectConstraints: SubjectConstraint{
				Type: "*",
			},
			ResourceConstraints: ResourceConstraint{
				Type: "ARTIFACT",
			},
			Effect: DecisionDeny,
			Prohibitions: []Prohibition{
				{Type: ProhibitionMutateOriginal, Description: "Direct modification of original financial artifacts is prohibited"},
			},
			ReasonCode:      "IMMUTABLE_ORIGINALS_ENFORCED",
			SourceReference: "SGACA Architectural Law #2",
			CreatedAt:       DefaultSafetyEffectiveDate,
		},
		// SF-SAFE-002: Agents cannot execute file release or bank transmission
		{
			PolicyID:      "SF-SAFE-002",
			Version:       1,
			Domain:        DomainRelease,
			Layer:         LayerSentinelSafety,
			Priority:      100,
			Status:        StatusActive,
			EffectiveFrom: DefaultSafetyEffectiveDate,
			Action:        ActionReleaseArtifact,
			SubjectConstraints: SubjectConstraint{
				Type: "AGENT",
			},
			ResourceConstraints: ResourceConstraint{
				Type: "ARTIFACT",
			},
			Effect: DecisionDeny,
			Prohibitions: []Prohibition{
				{Type: ProhibitionRelease, Description: "Autonomous release of financial artifacts by AI agents is prohibited"},
			},
			ReasonCode:      "AGENT_RELEASE_FORBIDDEN",
			SourceReference: "SGACA Architectural Law #11",
			CreatedAt:       DefaultSafetyEffectiveDate,
		},
		// SF-SAFE-003: Agents cannot approve releases (dual-control requires distinct authorized humans)
		{
			PolicyID:      "SF-SAFE-003",
			Version:       1,
			Domain:        DomainRelease,
			Layer:         LayerSentinelSafety,
			Priority:      100,
			Status:        StatusActive,
			EffectiveFrom: DefaultSafetyEffectiveDate,
			Action:        ActionApproveRelease,
			SubjectConstraints: SubjectConstraint{
				Type: "AGENT",
			},
			ResourceConstraints: ResourceConstraint{
				Type: "ARTIFACT",
			},
			Effect: DecisionDeny,
			Prohibitions: []Prohibition{
				{Type: ProhibitionApprove, Description: "Autonomous approval of financial artifacts by AI agents is prohibited"},
			},
			ReasonCode:      "AGENT_APPROVAL_FORBIDDEN",
			SourceReference: "SGACA Architectural Law #8",
			CreatedAt:       DefaultSafetyEffectiveDate,
		},
		// SF-SAFE-004: Remediation proposals are permitted only as derived candidate artifacts with strict safety obligations
		{
			PolicyID:      "SF-SAFE-004",
			Version:       1,
			Domain:        DomainRemediation,
			Layer:         LayerSentinelSafety,
			Priority:      100,
			Status:        StatusActive,
			EffectiveFrom: DefaultSafetyEffectiveDate,
			Action:        ActionCreateCandidate,
			SubjectConstraints: SubjectConstraint{
				Type: "AGENT",
			},
			ResourceConstraints: ResourceConstraint{
				Type: "ARTIFACT",
			},
			Effect: DecisionAllowWithObligations,
			Obligations: []Obligation{
				{Type: ObligationCandidateOnly},
				{Type: ObligationImmutableParentRequired},
				{Type: ObligationSandboxOnly},
				{Type: ObligationDeterministicRevalidation},
				{Type: ObligationAuditRequired},
				{
					Type: ObligationMaxAttempts,
					Parameters: map[string]interface{}{
						"count": 3,
					},
				},
			},
			ReasonCode:      "DERIVED_CANDIDATE_WITH_SAFETY_OBLIGATIONS",
			SourceReference: "SGACA Architectural Law #2",
			CreatedAt:       DefaultSafetyEffectiveDate,
		},
		// SF-SAFE-005: High autonomy / irreversible financial authority (A5) is denied to agents
		{
			PolicyID:      "SF-SAFE-005",
			Version:       1,
			Domain:        DomainEnterpriseAction,
			Layer:         LayerSentinelSafety,
			Priority:      100,
			Status:        StatusActive,
			EffectiveFrom: DefaultSafetyEffectiveDate,
			Action:        "*",
			SubjectConstraints: SubjectConstraint{
				Type:        "AGENT",
				MinAutonomy: 5,
			},
			ResourceConstraints: ResourceConstraint{
				Type: "*",
			},
			Effect: DecisionDeny,
			Prohibitions: []Prohibition{
				{Type: ProhibitionIrreversibleFinancialAuthority, Description: "Irreversible financial authority (Autonomy Level A5) is prohibited for agents"},
			},
			ReasonCode:      "AUTONOMOUS_IRREVERSIBLE_ACTION_FORBIDDEN",
			SourceReference: "SGACA Fundamental Equation 2.1",
			CreatedAt:       DefaultSafetyEffectiveDate,
		},
		// SF-SAFE-006: Cross-tenant access is forbidden independent of all other policies
		{
			PolicyID:      "SF-SAFE-006",
			Version:       1,
			Domain:        DomainAgent,
			Layer:         LayerSentinelSafety,
			Priority:      1000, // Maximum safety priority
			Status:        StatusActive,
			EffectiveFrom: DefaultSafetyEffectiveDate,
			Action:        ActionCrossTenantQuery,
			SubjectConstraints: SubjectConstraint{
				Type: "*",
			},
			ResourceConstraints: ResourceConstraint{
				Type: "*",
			},
			Effect: DecisionDeny,
			Prohibitions: []Prohibition{
				{Type: ProhibitionCrossTenantAccess, Description: "Cross-tenant data queries or actions are strictly prohibited"},
			},
			ReasonCode:      "CROSS_TENANT_FORBIDDEN",
			SourceReference: "SGACA Architectural Law #6",
			CreatedAt:       DefaultSafetyEffectiveDate,
		},
	}

	for _, r := range rules {
		if r.ContentHash == "" {
			r.ContentHash = ComputePolicyContentHash(r)
		}
	}
	return rules
}

// ValidateSafetyBootstrap checks that all mandatory safety rules exist, are active, and have valid hashes.
func ValidateSafetyBootstrap(policies []*PolicyDefinition) error {
	byID := make(map[string]*PolicyDefinition)
	for _, p := range policies {
		if p.Status == StatusActive {
			byID[p.PolicyID] = p
		}
	}

	for _, reqID := range MandatorySafetyPolicyIDs {
		p, exists := byID[reqID]
		if !exists {
			return fmt.Errorf("%w: missing required active safety policy %s", ErrSafetyBootstrapFailed, reqID)
		}
		if p.Layer != LayerSentinelSafety {
			return fmt.Errorf("%w: policy %s must be in layer SENTINEL_SAFETY (got %s)", ErrSafetyBootstrapFailed, reqID, p.Layer)
		}
		computedHash := ComputePolicyContentHash(p)
		if p.ContentHash != "" && p.ContentHash != computedHash {
			return fmt.Errorf("%w: policy %s content hash mismatch (stored: %s, computed: %s)", ErrSafetyBootstrapFailed, reqID, p.ContentHash, computedHash)
		}
	}
	return nil
}
