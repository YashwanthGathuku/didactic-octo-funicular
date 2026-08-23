package returnrisk

import (
	"context"
	"testing"
	"time"

	"sentinel-gateway/internal/auth"
	"sentinel-gateway/internal/repository"
)

func newTestScope(tenantID string) repository.Scope {
	p := &auth.Principal{
		Subject: "test-user",
		Memberships: []auth.Membership{
			{TenantID: tenantID, Roles: []auth.Role{auth.RoleOperator}},
		},
	}
	scope, _ := repository.NewScope(p, tenantID, auth.PermReadTenant)
	return scope
}

func TestDeterministicRiskEngine_Vectors(t *testing.T) {
	engine, err := NewDeterministicRiskEngine(DefaultEngineConfig())
	if err != nil {
		t.Fatalf("failed to create engine: %v", err)
	}

	scope := newTestScope("TENANT-ACME")
	ctx := context.Background()

	// Vector 1: Low Risk (Routine NSF, Minor Amount, Clean History)
	v1Event := ReturnEvent{
		ReturnEventID:      "ret-001",
		TenantID:           "TENANT-ACME",
		WorkflowID:         "wf-001",
		ReturnCode:         "R01",
		AmountCents:        25000, // $250.00
		Timestamp:          time.Now().UTC(),
		VerificationStatus: VerificationStatusVerified,
	}
	v1History := HistoricalReturnContext{
		TotalReturns7d:         0,
		TotalReturns30d:        2,
		PartnerTotalEntries30d: 1000,
		PartnerTotalReturns30d: 8,
		SameCodeCount30d:       1,
		VerifiedPriorCount:     1,
		RecentTrendVelocity:    0.0,
	}
	v1SLA := SLAContext{
		RemainingDuration: 36 * time.Hour,
		IsBreached:        false,
	}

	res1, err := engine.CalculateRisk(ctx, scope, v1Event, v1History, v1SLA)
	if err != nil {
		t.Fatalf("Vector 1 failed: %v", err)
	}
	if res1.RiskTier != RiskTierLow {
		t.Errorf("Vector 1 expected LOW tier, got %s (score=%f)", res1.RiskTier, res1.RiskScore)
	}
	if res1.RiskScore >= 30.0 {
		t.Errorf("Vector 1 expected score < 30.0, got %f", res1.RiskScore)
	}

	// Vector 4: Severe Risk (Customer Advises Unauthorized / Fraud, Heavy Spike, Breached SLA)
	v4Event := ReturnEvent{
		ReturnEventID:      "ret-004",
		TenantID:           "TENANT-ACME",
		WorkflowID:         "wf-004",
		ReturnCode:         "R10",
		AmountCents:        32000000, // $320,000.00
		Timestamp:          time.Now().UTC(),
		VerificationStatus: VerificationStatusVerified,
	}
	v4History := HistoricalReturnContext{
		TotalReturns7d:         35,
		TotalReturns30d:        55,
		PartnerTotalEntries30d: 1000,
		PartnerTotalReturns30d: 12,
		SameCodeCount30d:       15,
		VerifiedPriorCount:     5,
		RecentTrendVelocity:    1.2,
	}
	v4SLA := SLAContext{
		RemainingDuration: 0,
		IsBreached:        true,
	}

	res4, err := engine.CalculateRisk(ctx, scope, v4Event, v4History, v4SLA)
	if err != nil {
		t.Fatalf("Vector 4 failed: %v", err)
	}
	if res4.RiskTier != RiskTierSevere {
		t.Errorf("Vector 4 expected SEVERE tier, got %s (score=%f)", res4.RiskTier, res4.RiskScore)
	}
	if res4.RiskScore < 80.0 {
		t.Errorf("Vector 4 expected score >= 80.0, got %f", res4.RiskScore)
	}
	if len(res4.PrimaryDrivers) == 0 {
		t.Errorf("Vector 4 expected non-empty primary drivers")
	}
}

func TestDeterministicRiskEngine_TenantIsolation(t *testing.T) {
	engine, err := NewDeterministicRiskEngine(DefaultEngineConfig())
	if err != nil {
		t.Fatalf("failed to create engine: %v", err)
	}

	scope := newTestScope("TENANT-ALPHA")
	ctx := context.Background()

	event := ReturnEvent{
		ReturnEventID: "ret-002",
		TenantID:     "TENANT-BETA", // Mismatch
		ReturnCode:   "R01",
		AmountCents:  5000,
	}

	_, err = engine.CalculateRisk(ctx, scope, event, HistoricalReturnContext{}, SLAContext{})
	if err == nil {
		t.Fatalf("expected error on tenant mismatch, got nil")
	}
}

func TestDeterministicRiskEngine_UnknownReturnCode(t *testing.T) {
	engine, err := NewDeterministicRiskEngine(DefaultEngineConfig())
	if err != nil {
		t.Fatalf("failed to create engine: %v", err)
	}

	scope := newTestScope("TENANT-ACME")
	ctx := context.Background()

	event := ReturnEvent{
		ReturnEventID: "ret-003",
		TenantID:     "TENANT-ACME",
		ReturnCode:   "R999_INVALID",
		AmountCents:  5000,
	}

	_, err = engine.CalculateRisk(ctx, scope, event, HistoricalReturnContext{}, SLAContext{})
	if err == nil {
		t.Fatalf("expected error on unknown return code, got nil")
	}
}

func TestDeterministicRiskEngine_Repeatability(t *testing.T) {
	engine, err := NewDeterministicRiskEngine(DefaultEngineConfig())
	if err != nil {
		t.Fatalf("failed to create engine: %v", err)
	}

	scope := newTestScope("TENANT-ACME")
	ctx := context.Background()

	event := ReturnEvent{
		ReturnEventID: "ret-repeat",
		TenantID:     "TENANT-ACME",
		ReturnCode:   "R03",
		AmountCents:  1500000,
	}
	hist := HistoricalReturnContext{
		TotalReturns7d:  5,
		TotalReturns30d: 15,
	}
	sla := SLAContext{
		RemainingDuration: 4 * time.Hour,
	}

	resA, err := engine.CalculateRisk(ctx, scope, event, hist, sla)
	if err != nil {
		t.Fatalf("calc A failed: %v", err)
	}

	resB, err := engine.CalculateRisk(ctx, scope, event, hist, sla)
	if err != nil {
		t.Fatalf("calc B failed: %v", err)
	}

	if resA.RiskScore != resB.RiskScore {
		t.Errorf("expected identical risk score, got %f vs %f", resA.RiskScore, resB.RiskScore)
	}
	if resA.RiskTier != resB.RiskTier {
		t.Errorf("expected identical risk tier, got %s vs %s", resA.RiskTier, resB.RiskTier)
	}
	if resA.FeatureVector != resB.FeatureVector {
		t.Errorf("expected identical feature vector")
	}
	if resA.AssessmentHash != resB.AssessmentHash {
		t.Errorf("expected identical assessment hash, got %s vs %s", resA.AssessmentHash, resB.AssessmentHash)
	}
}
