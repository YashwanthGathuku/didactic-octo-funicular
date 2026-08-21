package toolgateway

import (
	"context"
	"encoding/json"
	"fmt"
	"testing"
	"time"

	"sentinel-gateway/internal/policy"
)

func BenchmarkToolGateway_AuthorizeAndExecute(b *testing.B) {
	reg := NewRegistry()
	_ = RegisterDefaultTools(reg, nil)

	policies := append(policy.SeedSafetyPolicies(), &policy.PolicyDefinition{
		PolicyID:      "BENCH-ALLOW-INCIDENT-GET",
		Version:       1,
		Domain:        policy.DomainArtifact,
		Layer:         policy.LayerEnterprise,
		Priority:      100,
		Status:        policy.StatusActive,
		EffectiveFrom: policy.DefaultSafetyEffectiveDate,
		Action:        "GET_INCIDENT",
		Effect:        policy.DecisionAllow,
		ReasonCode:    "PERMITTED_BENCH",
	})
	engine, _ := policy.NewEngineWithBundle("bundle-bench", "1.0.0", policies)
	gw := NewToolGatewayService(reg, engine, nil)

	ctx := context.Background()
	reqArgs := json.RawMessage(`{"incident_id":"inc-bench-101"}`)

	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		execCtx := &TrustedExecutionContext{
			RequestID:           fmt.Sprintf("bench-req-%d", i),
			IdempotencyKey:      fmt.Sprintf("bench-idem-%d", i),
			TenantID:            "TENANT-A",
			CallerID:            "agent-bench",
			CallerCapabilities: []ToolCapability{CapIncidentRead},
			CallerAutonomyLevel: 2,
			Timestamp:           time.Now().UTC(),
		}
		req := &ToolRequest{
			ToolID:         ToolIncidentGet,
			Args:           reqArgs,
			IdempotencyKey: execCtx.IdempotencyKey,
		}

		resp, err := gw.Execute(ctx, execCtx, req, nil)
		if err != nil || resp.Status != StatusSucceeded {
			b.Fatalf("benchmark execution failed: %v", err)
		}
	}
}

func BenchmarkToolGateway_RegistryLookup(b *testing.B) {
	reg := NewRegistry()
	_ = RegisterDefaultTools(reg, nil)

	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		tool, err := reg.Lookup(ToolIncidentGet, "1.0.0")
		if err != nil || tool == nil {
			b.Fatalf("lookup failed: %v", err)
		}
	}
}

func BenchmarkToolGateway_IdempotencyLookup(b *testing.B) {
	coord := NewIdempotencyCoordinator()
	ctx := context.Background()
	hash := "a1b2c3d4e5f6"

	// Pre-populate
	resp := &ToolResponse{InvocationID: "inv-001", Status: StatusSucceeded}
	coord.RecordDurableResult("TENANT-A", "agent-01", ToolIncidentGet, "1.0.0", "idem-bench", hash, resp)

	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		r, _, err := coord.CheckOrLock(ctx, "TENANT-A", "agent-01", ToolIncidentGet, "1.0.0", "idem-bench", hash)
		if err != nil || r == nil {
			b.Fatalf("idempotency check failed: %v", err)
		}
	}
}
