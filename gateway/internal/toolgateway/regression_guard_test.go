package toolgateway

import (
	"testing"

	"sentinel-gateway/internal/policy"
)

// TestArchitecturalGuard_NoDirectPrivilegedCapabilitiesExposedToAgents verifies that no registered
// tool or capability exposed to agents grants direct SQL, shell, filesystem mutation, secret exfiltration,
// or autonomous release authority.
func TestArchitecturalGuard_NoDirectPrivilegedCapabilitiesExposedToAgents(t *testing.T) {
	reg := NewRegistry()
	_ = RegisterDefaultTools(reg, nil)

	manifests := reg.ListManifests()

	prohibitedActions := map[string]struct{}{
		policy.ActionModifyOriginalArtifact: {},
		policy.ActionReleaseArtifact:        {},
		policy.ActionApproveRelease:         {},
		"EXECUTE_SQL":                       {},
		"RUN_SHELL":                         {},
		"HTTP_REQUEST":                      {},
		"READ_SECRET":                       {},
		"WIRE_TRANSFER":                     {},
	}

	for _, m := range manifests {
		// Invariant 1: No registered tool may expose a prohibited action
		if _, prohibited := prohibitedActions[m.PolicyAction]; prohibited {
			t.Errorf("SECURITY REGRESSION: Tool %s exposes prohibited action %s", m.ToolID, m.PolicyAction)
		}

		// Invariant 2: No registered tool may have IRREVERSIBLE_FINANCIAL side effect
		if m.SideEffectClass == SideEffectIrreversibleFinancial {
			t.Errorf("SECURITY REGRESSION: Tool %s has SideEffectIrreversibleFinancial", m.ToolID)
		}

		// Invariant 3: Permitted side effects are READ_ONLY or CANDIDATE_SANDBOX_WRITE (A2 only)
		if m.SideEffectClass != SideEffectReadOnly && m.SideEffectClass != SideEffectCandidateSandboxWrite {
			t.Errorf("SECURITY REGRESSION: Tool %s has unpermitted side effect %s", m.ToolID, m.SideEffectClass)
		}
		if m.SideEffectClass == SideEffectCandidateSandboxWrite && m.MaxAutonomy > 2 {
			t.Errorf("SECURITY REGRESSION: Sandbox write tool %s permits excessive autonomy (%d > 2)", m.ToolID, m.MaxAutonomy)
		}

		// Invariant 4: No tool may permit autonomy level 5 (unconstrained irreversible autonomy)
		if m.MaxAutonomy >= 5 {
			t.Errorf("SECURITY REGRESSION: Tool %s permits Level 5 autonomy (%d)", m.ToolID, m.MaxAutonomy)
		}
	}
}

// TestArchitecturalGuard_ProhibitionMappingCompleteness verifies that all dangerous capabilities
// are mapped to explicit policy ProhibitionTypes.
func TestArchitecturalGuard_ProhibitionMappingCompleteness(t *testing.T) {
	dangerousCapabilities := []policy.ToolCapability{
		"direct_artifact_edit",
		"release_file",
		"approve_release",
		"execute_sql",
		"access_secret",
		"cross_tenant_access",
	}

	for _, cap := range dangerousCapabilities {
		blockedProhibitions, exists := policy.CapabilityProhibitions[cap]
		if !exists || len(blockedProhibitions) == 0 {
			t.Errorf("SECURITY REGRESSION: Dangerous capability %s has no mapped ProhibitionTypes", cap)
		}
	}
}
