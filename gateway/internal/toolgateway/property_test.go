package toolgateway

import (
	"context"
	"encoding/json"
	"math/rand/v2"
	"testing"
	"time"

	"sentinel-gateway/internal/policy"
)

// TestProperty_AddingProhibitionNeverIncreasesAuthority tests that adding prohibitions
// or removing capabilities monotonically restricts or preserves the denial boundary.
func TestProperty_AddingProhibitionNeverIncreasesAuthority(t *testing.T) {
	gw, reg, engine := setupTestGateway(t, nil)
	ctx := context.Background()

	m := &ToolManifest{
		ToolID:               "prop.test.tool",
		Version:              "1.0.0",
		PolicyAction:         "GET_INCIDENT",
		RequiredCapabilities: []ToolCapability{CapIncidentRead},
		SideEffectClass:      SideEffectReadOnly,
		Timeout:              5 * time.Second,
		MaxOutputBytes:       1024,
	}
	_ = reg.Register(m, func(ctx context.Context, execCtx *TrustedExecutionContext, args json.RawMessage) (json.RawMessage, error) {
		return json.RawMessage(`{"status":"ok"}`), nil
	})

	baseExecCtx := &TrustedExecutionContext{
		RequestID:           "prop-req-001",
		IdempotencyKey:      "prop-idem-001",
		TenantID:            "TENANT-A",
		CallerID:            "agent-01",
		CallerCapabilities:  []ToolCapability{CapIncidentRead},
		CallerAutonomyLevel: 2,
	}

	req := &ToolRequest{
		ToolID:         "prop.test.tool",
		Args:           json.RawMessage(`{}`),
		IdempotencyKey: "prop-idem-001",
	}

	// 1. Initial request without prohibitions succeeds
	resp1, err1 := gw.Execute(ctx, baseExecCtx, req, nil)
	if err1 != nil || resp1.Status != StatusSucceeded {
		t.Fatalf("baseline execution failed: %v", err1)
	}

	// 2. Introduce random prohibitions into policy layer across iterations
	for i := 0; i < 30; i++ {
		prohPolicies := append(policy.SeedSafetyPolicies(), &policy.PolicyDefinition{
			PolicyID:      "RANDOM-PROH-POLICY",
			Version:       1,
			Domain:        policy.DomainArtifact,
			Layer:         policy.LayerEnterprise,
			Priority:      100,
			Status:        policy.StatusActive,
			EffectiveFrom: policy.DefaultSafetyEffectiveDate,
			Action:        "GET_INCIDENT",
			Effect:        policy.DecisionAllow,
			Prohibitions: []policy.Prohibition{
				{Type: policy.ProhibitionType(policy.ProhibitionMutateOriginal)},
				{Type: policy.ProhibitionType(policy.ProhibitionRelease)},
			},
			ReasonCode: "RESTRICTIVE_PROHIBITIONS",
		})
		_ = engine.SetBundle(prohPolicies)

		execCtxRestricted := *baseExecCtx
		execCtxRestricted.IdempotencyKey = "idem-restricted-" + string(rune('a'+i))
		reqRestricted := *req
		reqRestricted.IdempotencyKey = execCtxRestricted.IdempotencyKey

		// If caller capability is removed:
		if rand.Float64() < 0.5 {
			execCtxRestricted.CallerCapabilities = []ToolCapability{}
		}

		_, err := gw.Execute(ctx, &execCtxRestricted, &reqRestricted, nil)
		if len(execCtxRestricted.CallerCapabilities) == 0 && err == nil {
			t.Fatalf("INVARIANT VIOLATION: Execution succeeded with removed capabilities!")
		}
	}
}

// TestProperty_ToolRequestCannotAlterTrustedContext proves that untrusted ToolRequest arguments
// cannot override or inject tenant, caller autonomy, or identity.
func TestProperty_ToolRequestCannotAlterTrustedContext(t *testing.T) {
	gw, _, _ := setupTestGateway(t, nil)
	ctx := context.Background()

	execCtx := &TrustedExecutionContext{
		RequestID:           "prop-req-002",
		IdempotencyKey:      "prop-idem-002",
		TenantID:            "TENANT-A",
		CallerID:            "agent-01",
		CallerCapabilities:  []ToolCapability{CapIncidentRead},
		CallerAutonomyLevel: 2,
	}

	// Adversarial input trying to inject foreign tenant and autonomy level 5
	adversarialArgs := json.RawMessage(`{
		"tenant_id": "TENANT-B",
		"caller_id": "root-admin",
		"caller_autonomy_level": 5,
		"caller_roles": ["SUPER_ADMIN"],
		"incident_id": "inc-100"
	}`)

	req := &ToolRequest{
		ToolID:         ToolIncidentGet,
		Args:           adversarialArgs,
		IdempotencyKey: "prop-idem-002",
	}

	resp, err := gw.Execute(ctx, execCtx, req, nil)
	if err != nil {
		t.Fatalf("execution failed: %v", err)
	}

	var output map[string]interface{}
	_ = json.Unmarshal(resp.Output, &output)

	// Verify tenant in output is still the trusted TENANT-A, not the injected TENANT-B
	if output["tenant_id"] != "TENANT-A" {
		t.Errorf("SECURITY BREACH: Model args altered tenant ID! Got: %v, Want: TENANT-A", output["tenant_id"])
	}
}

func FuzzToolRequestArgs(f *testing.F) {
	gw, _, _ := setupTestGateway(nil, nil)
	ctx := context.Background()

	// Seed inputs
	f.Add([]byte(`{"incident_id":"inc-100"}`))
	f.Add([]byte(`{}`))
	f.Add([]byte(`{"invalid": [1, 2, 3]}`))
	f.Add([]byte(`{"nested": {"field": "value", "count": 42}}`))
	f.Add([]byte(`[1, 2, 3]`))

	f.Fuzz(func(t *testing.T, args []byte) {
		execCtx := &TrustedExecutionContext{
			RequestID:           "fuzz-req",
			IdempotencyKey:      "fuzz-idem-" + string(rune(len(args))),
			TenantID:            "TENANT-A",
			CallerID:            "agent-01",
			CallerCapabilities:  []ToolCapability{CapIncidentRead},
			CallerAutonomyLevel: 2,
		}

		req := &ToolRequest{
			ToolID:         ToolIncidentGet,
			Args:           json.RawMessage(args),
			IdempotencyKey: execCtx.IdempotencyKey,
		}

		// The gateway must never panic or crash on arbitrary fuzzed inputs
		_, _ = gw.Execute(ctx, execCtx, req, nil)
	})
}
