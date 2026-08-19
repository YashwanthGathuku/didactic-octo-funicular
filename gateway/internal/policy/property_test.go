package policy

import (
	"fmt"
	"math/rand"
	"testing"
	"time"
)

// TestProperty_MonotonicRestrictionInvariant verifies that adding any more restrictive policy
// can never make the overall decision less restrictive.
func TestProperty_MonotonicRestrictionInvariant(t *testing.T) {
	evalTime := time.Date(2026, 8, 19, 12, 0, 0, 0, time.UTC)
	basePolicies := SeedSafetyPolicies() // Includes SF-SAFE-001 through SF-SAFE-006

	engine, err := NewEngine(basePolicies)
	if err != nil {
		t.Fatal(err)
	}

	req := &PolicyEvaluationRequest{
		RequestID: "req-prop-1",
		TenantID:  "TENANT-A",
		Subject: PolicySubject{
			Type:     "AGENT",
			ID:       "agent-attacker",
			TenantID: "TENANT-A",
		},
		Action: ActionModifyOriginalArtifact, // Prohibited by SF-SAFE-001
		Resource: PolicyResource{
			Type:     "ARTIFACT",
			ID:       "art-101",
			TenantID: "TENANT-A",
		},
		Environment: PolicyEnvironment{
			EvaluationTime: evalTime,
		},
	}

	initialDec, err := engine.Evaluate(req)
	if err != nil {
		t.Fatal(err)
	}
	if initialDec.Decision != DecisionDeny {
		t.Fatalf("expected initial baseline decision DENY, got %s", initialDec.Decision)
	}

	// Add 50 randomized tenant/partner/enterprise ALLOW policies targeting this action
	rng := rand.New(rand.NewSource(42))
	layers := []PolicyLayer{LayerEnterprise, LayerTenant, LayerPartner}

	for i := 0; i < 50; i++ {
		layer := layers[rng.Intn(len(layers))]
		priority := rng.Intn(1000)

		allowPolicy := &PolicyDefinition{
			PolicyID:      fmt.Sprintf("RAND-ALLOW-%d", i),
			Version:       1,
			Domain:        DomainArtifact,
			Layer:         layer,
			Priority:      priority,
			Status:        StatusActive,
			EffectiveFrom: DefaultSafetyEffectiveDate,
			TenantID:      strPtr("TENANT-A"),
			Action:        ActionModifyOriginalArtifact,
			SubjectConstraints: SubjectConstraint{
				Type: "*",
			},
			ResourceConstraints: ResourceConstraint{
				Type: "*",
			},
			Effect:     DecisionAllow,
			ReasonCode: fmt.Sprintf("RANDOM_ALLOW_%d", i),
		}

		basePolicies = append(basePolicies, allowPolicy)
		updatedEngine, err := NewEngine(basePolicies)
		if err != nil {
			t.Fatal(err)
		}

		newDec, err := updatedEngine.Evaluate(req)
		if err != nil {
			t.Fatal(err)
		}

		// Invariant check: MUST REMAIN DENY
		if newDec.Decision != DecisionDeny {
			t.Fatalf("INVARIANT VIOLATION on iteration %d: adding ALLOW policy %s in layer %s made decision %s!",
				i, allowPolicy.PolicyID, layer, newDec.Decision)
		}
	}
}

// TestProperty_RequireHumanCannotProduceAllow verifies that if a rule requires human review,
// adding lower layer ALLOW policies can NEVER relax it to ALLOW.
func TestProperty_RequireHumanCannotProduceAllow(t *testing.T) {
	evalTime := time.Date(2026, 8, 19, 12, 0, 0, 0, time.UTC)
	policies := append(SeedSafetyPolicies(), &PolicyDefinition{
		PolicyID:      "ENT-HUMAN-RULE",
		Version:       1,
		Domain:        DomainRemediation,
		Layer:         LayerEnterprise,
		Priority:      10,
		Status:        StatusActive,
		EffectiveFrom: DefaultSafetyEffectiveDate,
		Action:        "SPECIAL_FINANCIAL_ACTION",
		Effect:        DecisionRequireHuman,
		ReasonCode:    "DUAL_CONTROL_MANDATORY",
	})

	req := &PolicyEvaluationRequest{
		RequestID: "req-human-prop",
		TenantID:  "TENANT-A",
		Subject: PolicySubject{
			Type:     "AGENT",
			ID:       "agent-1",
			TenantID: "TENANT-A",
		},
		Action: "SPECIAL_FINANCIAL_ACTION",
		Resource: PolicyResource{
			Type:     "ARTIFACT",
			ID:       "art-1",
			TenantID: "TENANT-A",
		},
		Environment: PolicyEnvironment{
			EvaluationTime: evalTime,
		},
	}

	engine, err := NewEngine(policies)
	if err != nil {
		t.Fatal(err)
	}

	dec, err := engine.Evaluate(req)
	if err != nil || dec.Decision != DecisionRequireHuman {
		t.Fatalf("expected REQUIRE_HUMAN, got %s (err: %v)", dec.Decision, err)
	}

	// Add 20 Tenant & Partner ALLOW rules
	for i := 0; i < 20; i++ {
		policies = append(policies, &PolicyDefinition{
			PolicyID:      fmt.Sprintf("TENANT-RELAX-%d", i),
			Version:       1,
			Domain:        DomainRemediation,
			Layer:         LayerTenant,
			Priority:      i * 10,
			Status:        StatusActive,
			EffectiveFrom: DefaultSafetyEffectiveDate,
			TenantID:      strPtr("TENANT-A"),
			Action:        "SPECIAL_FINANCIAL_ACTION",
			Effect:        DecisionAllow,
			ReasonCode:    "TENANT_RELAX",
		})

		updatedEngine, err := NewEngine(policies)
		if err != nil {
			t.Fatal(err)
		}

		res, err := updatedEngine.Evaluate(req)
		if err != nil {
			t.Fatal(err)
		}
		if res.Decision == DecisionAllow || res.Decision == DecisionAllowWithObligations {
			t.Fatalf("INVARIANT VIOLATION: Tenant ALLOW relaxed Enterprise REQUIRE_HUMAN to %s", res.Decision)
		}
	}
}

// TestProperty_ObligationAndProhibitionPreservation verifies that adding rules
// can NEVER remove or subtract existing obligations or prohibitions.
func TestProperty_ObligationAndProhibitionPreservation(t *testing.T) {
	evalTime := time.Date(2026, 8, 19, 12, 0, 0, 0, time.UTC)
	basePolicies := SeedSafetyPolicies()
	engine, err := NewEngine(basePolicies)
	if err != nil {
		t.Fatal(err)
	}

	req := &PolicyEvaluationRequest{
		RequestID: "req-obl-preserve",
		TenantID:  "TENANT-A",
		Subject: PolicySubject{
			Type:     "AGENT",
			ID:       "agent-1",
			TenantID: "TENANT-A",
		},
		Action: ActionCreateCandidate,
		Resource: PolicyResource{
			Type:     "ARTIFACT",
			ID:       "art-1",
			TenantID: "TENANT-A",
		},
		Environment: PolicyEnvironment{
			EvaluationTime: evalTime,
		},
	}

	initialDec, err := engine.Evaluate(req)
	if err != nil {
		t.Fatal(err)
	}

	initialOblTypes := make(map[ObligationType]struct{})
	for _, o := range initialDec.Obligations {
		initialOblTypes[o.Type] = struct{}{}
	}

	// Add 30 additional rules with new obligations
	for i := 0; i < 30; i++ {
		basePolicies = append(basePolicies, &PolicyDefinition{
			PolicyID:      fmt.Sprintf("NEW-OBL-RULE-%d", i),
			Version:       1,
			Domain:        DomainRemediation,
			Layer:         LayerTenant,
			Priority:      i,
			Status:        StatusActive,
			EffectiveFrom: DefaultSafetyEffectiveDate,
			TenantID:      strPtr("TENANT-A"),
			Action:        ActionCreateCandidate,
			Effect:        DecisionAllowWithObligations,
			Obligations: []Obligation{
				{Type: ObligationType(fmt.Sprintf("CUSTOM_OBL_%d", i))},
			},
			ReasonCode: fmt.Sprintf("REASON_%d", i),
		})

		updatedEngine, err := NewEngine(basePolicies)
		if err != nil {
			t.Fatal(err)
		}

		newDec, err := updatedEngine.Evaluate(req)
		if err != nil {
			t.Fatal(err)
		}

		// Verify all initial obligations are still present
		currentOblTypes := make(map[ObligationType]struct{})
		for _, o := range newDec.Obligations {
			currentOblTypes[o.Type] = struct{}{}
		}

		for initType := range initialOblTypes {
			if _, exists := currentOblTypes[initType]; !exists {
				t.Fatalf("INVARIANT VIOLATION: Obligation %s was lost on iteration %d!", initType, i)
			}
		}
	}
}

// TestProperty_DeterministicReproducibility verifies that evaluating 1000 times
// with identical inputs produces 1000 byte-identical decisions and identical hashes.
func TestProperty_DeterministicReproducibility(t *testing.T) {
	engine := NewEngineWithDefaults()
	req := &PolicyEvaluationRequest{
		RequestID: "req-rep-1000",
		TenantID:  "TENANT-A",
		Subject: PolicySubject{
			Type:          "AGENT",
			ID:            "agent-1",
			Roles:         []string{"TRIAGE_ANALYST"},
			AutonomyLevel: 2,
			TenantID:      "TENANT-A",
		},
		Action: ActionCreateCandidate,
		Resource: PolicyResource{
			Type:     "ARTIFACT",
			ID:       "art-101",
			TenantID: "TENANT-A",
		},
		Environment: PolicyEnvironment{
			EvaluationTime: time.Date(2026, 8, 19, 12, 0, 0, 0, time.UTC),
			FleetMode:      "ADVISORY",
		},
	}

	firstDec, err := engine.Evaluate(req)
	if err != nil {
		t.Fatal(err)
	}

	for i := 0; i < 1000; i++ {
		nextDec, err := engine.Evaluate(req)
		if err != nil {
			t.Fatal(err)
		}
		if nextDec.Decision != firstDec.Decision {
			t.Fatalf("decision mismatch at iter %d: %s vs %s", i, nextDec.Decision, firstDec.Decision)
		}
		if nextDec.DecisionHash != firstDec.DecisionHash {
			t.Fatalf("decision hash mismatch at iter %d: %s vs %s", i, nextDec.DecisionHash, firstDec.DecisionHash)
		}
		if nextDec.EvaluatedContextHash != firstDec.EvaluatedContextHash {
			t.Fatalf("context hash mismatch at iter %d", i)
		}
	}
}

// FuzzPolicyEvaluation executes fuzz testing with arbitrary strings and autonomy levels.
func FuzzPolicyEvaluation(f *testing.F) {
	f.Add("TENANT-A", "AGENT", "agent-1", "MODIFY_ORIGINAL_ARTIFACT", "ARTIFACT", "art-1", 2)
	f.Add("TENANT-B", "USER", "user-1", "CREATE_CANDIDATE", "ARTIFACT", "art-2", 0)
	f.Add("TENANT-X", "SYSTEM", "sys-1", "UNKNOWN_ACTION", "SECRET", "sec-1", 5)

	engine := NewEngineWithDefaults()
	evalTime := time.Date(2026, 8, 19, 12, 0, 0, 0, time.UTC)

	f.Fuzz(func(t *testing.T, tenantID, subjType, subjID, action, resType, resID string, autonomy int) {
		if tenantID == "" || action == "" {
			return
		}

		req := &PolicyEvaluationRequest{
			RequestID: "fuzz-req",
			TenantID:  tenantID,
			Subject: PolicySubject{
				Type:          subjType,
				ID:            subjID,
				AutonomyLevel: autonomy,
				TenantID:      tenantID,
			},
			Action: action,
			Resource: PolicyResource{
				Type:     resType,
				ID:       resID,
				TenantID: tenantID,
			},
			Environment: PolicyEnvironment{
				EvaluationTime: evalTime,
			},
		}

		dec, err := engine.Evaluate(req)
		if err != nil {
			t.Fatalf("evaluator returned unexpected error on valid request: %v", err)
		}

		// Ensure decision is strictly one of the 4 legal decisions
		switch dec.Decision {
		case DecisionAllow, DecisionDeny, DecisionAllowWithObligations, DecisionRequireHuman:
			// Valid
		default:
			t.Fatalf("illegal decision returned by fuzzer: %q", dec.Decision)
		}

		if dec.DecisionHash == "" || dec.EvaluatedContextHash == "" {
			t.Fatal("decision or context hash cannot be empty")
		}
	})
}
