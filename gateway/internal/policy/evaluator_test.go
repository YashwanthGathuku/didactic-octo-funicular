package policy

import (
	"sync"
	"testing"
	"time"
)

func sampleRequest() *PolicyEvaluationRequest {
	return &PolicyEvaluationRequest{
		RequestID: "req-test-001",
		TenantID:  "TENANT-A",
		Subject: PolicySubject{
			Type:          "AGENT",
			ID:            "agent-remediator",
			Roles:         []string{"REMEDIATION_SPECIALIST"},
			AutonomyLevel: 2,
			TenantID:      "TENANT-A",
		},
		Action: ActionCreateCandidate,
		Resource: PolicyResource{
			Type:           "ARTIFACT",
			ID:             "art-101",
			SHA256:         "sha256-art-101-original",
			State:          "QUARANTINED",
			Classification: "FINANCIAL_PAYLOAD",
			TenantID:       "TENANT-A",
		},
		Workflow: PolicyWorkflowContext{
			WorkflowID: "wf-101",
			State:      "REMEDIATING",
			Attempt:    1,
		},
		Environment: PolicyEnvironment{
			EvaluationTime: time.Date(2026, 8, 19, 12, 0, 0, 0, time.UTC),
			FleetMode:      "ADVISORY",
		},
		AuthoritativeAttributes: map[string]interface{}{
			"partner_id": "PARTNER-ALPHA",
		},
	}
}

func TestPolicyEngine_AllowWithObligations_StrengthenedSafetyRule(t *testing.T) {
	engine := NewEngineWithDefaults()
	req := sampleRequest()
	req.Action = ActionCreateCandidate

	dec, err := engine.Evaluate(req)
	if err != nil {
		t.Fatalf("evaluate: %v", err)
	}

	if dec.Decision != DecisionAllowWithObligations {
		t.Errorf("expected decision %s, got %s", DecisionAllowWithObligations, dec.Decision)
	}

	expectedObls := map[ObligationType]bool{
		ObligationCandidateOnly:             true,
		ObligationImmutableParentRequired:   true,
		ObligationSandboxOnly:               true,
		ObligationDeterministicRevalidation: true,
		ObligationAuditRequired:             true,
		ObligationMaxAttempts:               true,
	}

	for _, obl := range dec.Obligations {
		if !expectedObls[obl.Type] {
			t.Errorf("unexpected obligation %s", obl.Type)
		}
		if obl.Type == ObligationMaxAttempts {
			cnt, ok := obl.Parameters["count"]
			if !ok || cnt != 3 && cnt != float64(3) && cnt != int64(3) {
				t.Errorf("expected max attempts count 3, got %v", cnt)
			}
		}
		delete(expectedObls, obl.Type)
	}
	if len(expectedObls) > 0 {
		t.Errorf("missing expected obligations: %v", expectedObls)
	}

	if dec.DecisionHash == "" {
		t.Error("decision hash must not be empty")
	}

	// Executable decision check: ALLOW_WITH_OBLIGATIONS is NOT immediately executable
	if IsExecutableDecision(dec) {
		t.Error("ALLOW_WITH_OBLIGATIONS must not be immediately executable without satisfied obligations")
	}

	// Verify satisfaction checking
	satisfied := []ObligationType{
		ObligationCandidateOnly,
		ObligationImmutableParentRequired,
		ObligationSandboxOnly,
		ObligationDeterministicRevalidation,
		ObligationAuditRequired,
		ObligationMaxAttempts,
	}
	canExec, missing := CanExecuteWithSatisfiedObligations(dec, satisfied)
	if !canExec || len(missing) > 0 {
		t.Errorf("expected executable when all obligations satisfied, got missing: %v", missing)
	}
}

func TestPolicyEngine_SafetyDeny_ModifyOriginalArtifact(t *testing.T) {
	engine := NewEngineWithDefaults()
	req := sampleRequest()
	req.Action = ActionModifyOriginalArtifact

	dec, err := engine.Evaluate(req)
	if err != nil {
		t.Fatalf("evaluate: %v", err)
	}

	if dec.Decision != DecisionDeny {
		t.Errorf("expected decision DENY for MODIFY_ORIGINAL_ARTIFACT, got %s", dec.Decision)
	}

	foundProhibition := false
	for _, p := range dec.Prohibitions {
		if p.Type == ProhibitionMutateOriginal {
			foundProhibition = true
			break
		}
	}
	if !foundProhibition {
		t.Errorf("expected prohibition %s, got %v", ProhibitionMutateOriginal, dec.Prohibitions)
	}

	// Capability check for P04 Tool Gateway
	isBlocked, blockedType := IsCapabilityProhibited(CapabilityDirectArtifactEdit, dec.Prohibitions)
	if !isBlocked || blockedType != ProhibitionMutateOriginal {
		t.Errorf("expected CapabilityDirectArtifactEdit to be blocked by ProhibitionMutateOriginal, got %v (%s)", isBlocked, blockedType)
	}
}

func TestPolicyEngine_SafetyDenyBeatsTenantAllow(t *testing.T) {
	policies := SeedSafetyPolicies()

	// Tenant attempts to allow original artifact mutation
	tenantAllow := &PolicyDefinition{
		PolicyID:      "TENANT-CUSTOM-001",
		Version:       1,
		Domain:        DomainArtifact,
		Layer:         LayerTenant,
		Priority:      999, // High priority inside tenant layer
		Status:        StatusActive,
		EffectiveFrom: DefaultSafetyEffectiveDate,
		TenantID:      strPtr("TENANT-A"),
		Action:        ActionModifyOriginalArtifact,
		SubjectConstraints: SubjectConstraint{
			Type: "*",
		},
		ResourceConstraints: ResourceConstraint{
			Type: "ARTIFACT",
		},
		Effect:     DecisionAllow,
		ReasonCode: "TENANT_PERMITS_DIRECT_EDIT",
	}
	policies = append(policies, tenantAllow)

	engine, err := NewEngine(policies)
	if err != nil {
		t.Fatalf("new engine: %v", err)
	}

	req := sampleRequest()
	req.Action = ActionModifyOriginalArtifact

	dec, err := engine.Evaluate(req)
	if err != nil {
		t.Fatalf("evaluate: %v", err)
	}

	// Safety DENY MUST dominate Tenant ALLOW
	if dec.Decision != DecisionDeny {
		t.Errorf("CRITICAL SAFETY BREACH: tenant allow overrode safety deny! Got decision: %s", dec.Decision)
	}
}

func TestPolicyEngine_SafetyBootstrapValidation(t *testing.T) {
	// 1. Omit SF-SAFE-001 -> must fail engine construction
	corrupted := []*PolicyDefinition{
		{
			PolicyID:      "SF-SAFE-002",
			Version:       1,
			Domain:        DomainRelease,
			Layer:         LayerSentinelSafety,
			Status:        StatusActive,
			EffectiveFrom: DefaultSafetyEffectiveDate,
			Action:        ActionReleaseArtifact,
			Effect:        DecisionDeny,
			ReasonCode:    "SAFE_002",
		},
	}

	_, err := NewEngine(corrupted)
	if err == nil {
		t.Error("expected error when mandatory safety policy SF-SAFE-001 is missing")
	}

	// 2. Tampered content hash -> must fail bootstrap
	tampered := SeedSafetyPolicies()
	tampered[0].ContentHash = "tampered_corrupted_hash"

	_, err = NewEngine(tampered)
	if err == nil {
		t.Error("expected error when safety policy content hash is tampered")
	}
}

func TestPolicyEngine_ExactImmutableBundleReplay(t *testing.T) {
	evalTime := time.Date(2026, 8, 19, 12, 0, 0, 0, time.UTC)
	req := sampleRequest()
	req.Environment.EvaluationTime = evalTime

	// 1. Create Bundle B1
	policiesB1 := SeedSafetyPolicies()
	engine, err := NewEngineWithBundle("bundle-sentinel-b1", "1.0.0", policiesB1)
	if err != nil {
		t.Fatal(err)
	}

	// 2. Evaluate historical request R against B1
	dec1, err := engine.Evaluate(req)
	if err != nil {
		t.Fatal(err)
	}
	b1Manifest := engine.GetActiveBundle().Manifest

	// 3. Create later backdated policy and activate Bundle B2
	backdatedPolicy := &PolicyDefinition{
		PolicyID:      "BACKDATED-ENTERPRISE-RULE",
		Version:       1,
		Domain:        DomainRemediation,
		Layer:         LayerEnterprise,
		Priority:      99,
		Status:        StatusActive,
		EffectiveFrom: time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC), // Backdated before R!
		Action:        ActionCreateCandidate,
		SubjectConstraints: SubjectConstraint{
			Type: "AGENT",
		},
		ResourceConstraints: ResourceConstraint{
			Type: "ARTIFACT",
		},
		Effect:     DecisionDeny, // Attempts to deny candidate creation
		ReasonCode: "LATER_ADDED_DENY",
	}

	policiesB2 := append(SeedSafetyPolicies(), backdatedPolicy)
	if err := engine.SetBundleWithID("bundle-sentinel-b2", "2.0.0", policiesB2); err != nil {
		t.Fatal(err)
	}

	// In current engine (B2), request R would now be DENIED
	decB2, err := engine.Evaluate(req)
	if err != nil {
		t.Fatal(err)
	}
	if decB2.Decision != DecisionDeny {
		t.Fatalf("expected B2 to evaluate to DENY, got %s", decB2.Decision)
	}

	// 4. Replay historical request R using the exact immutable B1 manifest + original policies
	replayDec, err := ReplayEvaluation(b1Manifest, policiesB1, req)
	if err != nil {
		t.Fatalf("replay evaluation failed: %v", err)
	}

	// 5. Prove replayed decision and decision hash remain 100% byte-identical to dec1
	if replayDec.Decision != dec1.Decision {
		t.Errorf("replay decision mismatch: %s vs original %s", replayDec.Decision, dec1.Decision)
	}
	if replayDec.DecisionHash != dec1.DecisionHash {
		t.Errorf("replay decision hash mismatch: %s vs original %s", replayDec.DecisionHash, dec1.DecisionHash)
	}
	if replayDec.PolicyBundleHash != dec1.PolicyBundleHash {
		t.Errorf("replay bundle hash mismatch: %s vs original %s", replayDec.PolicyBundleHash, dec1.PolicyBundleHash)
	}
}

func TestPolicyEngine_AtomicBundleActivationConcurrency(t *testing.T) {
	policies1 := SeedSafetyPolicies()
	engine, err := NewEngineWithBundle("bundle-v1", "1.0.0", policies1)
	if err != nil {
		t.Fatal(err)
	}

	policies2 := SeedSafetyPolicies()
	policies2 = append(policies2, &PolicyDefinition{
		PolicyID:      "ENT-CONCUR-001",
		Version:       1,
		Domain:        DomainTool,
		Layer:         LayerEnterprise,
		Priority:      10,
		Status:        StatusActive,
		EffectiveFrom: DefaultSafetyEffectiveDate,
		Action:        "CONCURRENT_ACTION",
		Effect:        DecisionAllow,
		ReasonCode:    "ENT_CONCURRENT_ALLOW",
	})

	var wg sync.WaitGroup
	req := sampleRequest()
	req.Action = ActionCreateCandidate

	// Run 100 parallel readers
	for i := 0; i < 100; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := 0; j < 50; j++ {
				dec, err := engine.Evaluate(req)
				if err != nil {
					t.Errorf("concurrent evaluate: %v", err)
					return
				}
				// Every decision must be bound to either bundle-v1 or bundle-v2, never a corrupt state
				if dec.PolicyBundleID != "bundle-v1" && dec.PolicyBundleID != "bundle-v2" {
					t.Errorf("invalid bundle ID seen in flight: %s", dec.PolicyBundleID)
				}
			}
		}()
	}

	// Concurrently swap active bundle
	wg.Add(1)
	go func() {
		defer wg.Done()
		time.Sleep(2 * time.Millisecond)
		_ = engine.SetBundleWithID("bundle-v2", "2.0.0", policies2)
	}()

	wg.Wait()
}

func strPtr(s string) *string {
	return &s
}
