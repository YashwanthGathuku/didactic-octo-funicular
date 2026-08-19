package policy

import (
	"fmt"
	"testing"
	"time"
)

func BenchmarkPolicyEvaluation_SingleThreaded(b *testing.B) {
	// Seed safety policies + 50 custom enterprise/tenant policies
	policies := SeedSafetyPolicies()
	for i := 0; i < 50; i++ {
		policies = append(policies, &PolicyDefinition{
			PolicyID:      fmt.Sprintf("BENCH-RULE-%d", i),
			Version:       1,
			Domain:        DomainTool,
			Layer:         LayerEnterprise,
			Priority:      10,
			Status:        StatusActive,
			EffectiveFrom: DefaultSafetyEffectiveDate,
			Action:        fmt.Sprintf("ACTION_%d", i),
			Effect:        DecisionAllow,
			ReasonCode:    "BENCHMARK_ALLOW",
		})
	}

	engine, err := NewEngine(policies)
	if err != nil {
		b.Fatal(err)
	}

	req := &PolicyEvaluationRequest{
		RequestID: "bench-req-1",
		TenantID:  "TENANT-A",
		Subject: PolicySubject{
			Type:          "AGENT",
			ID:            "bench-agent",
			Roles:         []string{"SPECIALIST"},
			AutonomyLevel: 2,
			TenantID:      "TENANT-A",
		},
		Action: ActionCreateCandidate,
		Resource: PolicyResource{
			Type:     "ARTIFACT",
			ID:       "art-bench",
			TenantID: "TENANT-A",
		},
		Environment: PolicyEnvironment{
			EvaluationTime: time.Date(2026, 8, 19, 12, 0, 0, 0, time.UTC),
			FleetMode:      "ACTIVE",
		},
	}

	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		_, err := engine.Evaluate(req)
		if err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkPolicyEvaluation_Parallel(b *testing.B) {
	policies := SeedSafetyPolicies()
	for i := 0; i < 50; i++ {
		policies = append(policies, &PolicyDefinition{
			PolicyID:      fmt.Sprintf("BENCH-RULE-PAR-%d", i),
			Version:       1,
			Domain:        DomainTool,
			Layer:         LayerEnterprise,
			Priority:      10,
			Status:        StatusActive,
			EffectiveFrom: DefaultSafetyEffectiveDate,
			Action:        fmt.Sprintf("ACTION_PAR_%d", i),
			Effect:        DecisionAllow,
			ReasonCode:    "BENCHMARK_ALLOW",
		})
	}

	engine, err := NewEngine(policies)
	if err != nil {
		b.Fatal(err)
	}

	req := &PolicyEvaluationRequest{
		RequestID: "bench-req-par",
		TenantID:  "TENANT-A",
		Subject: PolicySubject{
			Type:          "AGENT",
			ID:            "bench-agent",
			Roles:         []string{"SPECIALIST"},
			AutonomyLevel: 2,
			TenantID:      "TENANT-A",
		},
		Action: ActionCreateCandidate,
		Resource: PolicyResource{
			Type:     "ARTIFACT",
			ID:       "art-bench",
			TenantID: "TENANT-A",
		},
		Environment: PolicyEnvironment{
			EvaluationTime: time.Date(2026, 8, 19, 12, 0, 0, 0, time.UTC),
			FleetMode:      "ACTIVE",
		},
	}

	b.ResetTimer()
	b.ReportAllocs()

	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			_, err := engine.Evaluate(req)
			if err != nil {
				b.Fatal(err)
			}
		}
	})
}
