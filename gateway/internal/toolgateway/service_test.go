package toolgateway

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"sync"
	"testing"
	"time"

	"sentinel-gateway/internal/policy"
	_ "modernc.org/sqlite"
)

func newToolTestDB(t *testing.T) *sql.DB {
	t.Helper()
	db, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatal(err)
	}
	db.SetMaxOpenConns(1)
	t.Cleanup(func() { db.Close() })

	schema := `
	CREATE TABLE tenants (id TEXT PRIMARY KEY, name TEXT NOT NULL);
	INSERT INTO tenants (id, name) VALUES ('TENANT-A', 'Tenant A'), ('TENANT-B', 'Tenant B');

	CREATE TABLE outbox_events (
		id             INTEGER PRIMARY KEY AUTOINCREMENT,
		tenant_id      TEXT NOT NULL REFERENCES tenants(id),
		event_type     TEXT NOT NULL,
		subject_type   TEXT NOT NULL,
		subject_id     INTEGER NOT NULL,
		payload        TEXT NOT NULL,
		dedupe_key     TEXT NOT NULL,
		attempt_count  INTEGER NOT NULL DEFAULT 0,
		max_attempts   INTEGER NOT NULL DEFAULT 10,
		run_after      TIMESTAMP,
		last_error     TEXT,
		delivered_at   TIMESTAMP,
		dead_at        TIMESTAMP,
		created_at     TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
		UNIQUE (tenant_id, dedupe_key)
	);

	CREATE TABLE tool_invocations (
		id                      TEXT PRIMARY KEY,
		tenant_id               TEXT NOT NULL REFERENCES tenants(id),
		tool_id                 TEXT NOT NULL,
		tool_version            TEXT NOT NULL,
		manifest_hash           TEXT NOT NULL,
		caller_type             TEXT NOT NULL,
		caller_id               TEXT NOT NULL,
		caller_autonomy_level   INTEGER NOT NULL DEFAULT 1,
		workflow_id             TEXT,
		idempotency_key         TEXT NOT NULL,
		request_hash            TEXT NOT NULL,
		status                  TEXT NOT NULL CHECK (status IN ('RECEIVED', 'AUTHORIZED', 'EXECUTING', 'SUCCEEDED', 'DENIED', 'FAILED', 'TIMED_OUT', 'UNCERTAIN')),
		policy_decision_id      TEXT,
		policy_decision_hash    TEXT,
		policy_bundle_hash      TEXT,
		input_hash              TEXT NOT NULL,
		output_hash             TEXT,
		output_payload          TEXT,
		error_code              TEXT,
		error_message           TEXT,
		duration_ms             INTEGER,
		execution_mode          TEXT NOT NULL DEFAULT 'LIVE',
		created_at              TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
		completed_at            TIMESTAMP,
		UNIQUE (tenant_id, caller_id, tool_id, tool_version, idempotency_key)
	);
	`
	if _, err := db.Exec(schema); err != nil {
		t.Fatal(err)
	}
	return db
}

func setupTestGateway(t *testing.T, db *sql.DB) (*ToolGatewayService, *Registry, *policy.PolicyEngine) {
	if t != nil {
		t.Helper()
	}
	reg := NewRegistry()
	_ = RegisterDefaultTools(reg, nil)

	// Pre-seed policy engine with foundational safety policies + tool policies
	policies := policy.SeedSafetyPolicies()

	// Add ALLOW policies for the 4 safe tools in ENTERPRISE layer
	policies = append(policies,
		&policy.PolicyDefinition{
			PolicyID:      "TEST-TOOL-INCIDENT-GET",
			Version:       1,
			Domain:        policy.DomainArtifact,
			Layer:         policy.LayerEnterprise,
			Priority:      100,
			Status:        policy.StatusActive,
			EffectiveFrom: policy.DefaultSafetyEffectiveDate,
			Action:        "GET_INCIDENT",
			Effect:        policy.DecisionAllow,
			ReasonCode:    "PERMITTED_INCIDENT_READ",
		},
		&policy.PolicyDefinition{
			PolicyID:      "TEST-TOOL-FINDINGS-LIST",
			Version:       1,
			Domain:        policy.DomainArtifact,
			Layer:         policy.LayerEnterprise,
			Priority:      100,
			Status:        policy.StatusActive,
			EffectiveFrom: policy.DefaultSafetyEffectiveDate,
			Action:        "LIST_FINDINGS",
			Effect:        policy.DecisionAllow,
			ReasonCode:    "PERMITTED_FINDINGS_READ",
		},
		&policy.PolicyDefinition{
			PolicyID:      "TEST-TOOL-ARTIFACT-GET",
			Version:       1,
			Domain:        policy.DomainArtifact,
			Layer:         policy.LayerEnterprise,
			Priority:      100,
			Status:        policy.StatusActive,
			EffectiveFrom: policy.DefaultSafetyEffectiveDate,
			Action:        "GET_ARTIFACT_METADATA",
			Effect:        policy.DecisionAllow,
			ReasonCode:    "PERMITTED_ARTIFACT_READ",
		},
		&policy.PolicyDefinition{
			PolicyID:      "TEST-TOOL-WORKFLOW-GET",
			Version:       1,
			Domain:        policy.DomainAgent,
			Layer:         policy.LayerEnterprise,
			Priority:      100,
			Status:        policy.StatusActive,
			EffectiveFrom: policy.DefaultSafetyEffectiveDate,
			Action:        "GET_WORKFLOW",
			Effect:        policy.DecisionAllow,
			ReasonCode:    "PERMITTED_WORKFLOW_READ",
		},
	)

	engine, err := policy.NewEngineWithBundle("bundle-tool-test", "1.0.0", policies)
	if err != nil {
		t.Fatalf("new engine: %v", err)
	}

	var store *ToolStore
	if db != nil {
		store = NewToolStore(db)
	}

	gw := NewToolGatewayService(reg, engine, store)
	return gw, reg, engine
}

// 1. Unregistered tool denied
func TestToolGateway_UnregisteredToolDenied(t *testing.T) {
	gw, _, _ := setupTestGateway(t, nil)
	ctx := context.Background()

	execCtx := &TrustedExecutionContext{
		RequestID:           "req-001",
		IdempotencyKey:      "idem-001",
		TenantID:            "TENANT-A",
		CallerID:            "agent-01",
		CallerCapabilities: []ToolCapability{CapIncidentRead},
		CallerAutonomyLevel: 2,
	}

	req := &ToolRequest{
		ToolID:         "unknown.tool.execute",
		Args:           json.RawMessage(`{}`),
		IdempotencyKey: "idem-001",
	}

	_, err := gw.Execute(ctx, execCtx, req, nil)
	if !errors.Is(err, ErrUnregisteredTool) {
		t.Errorf("expected ErrUnregisteredTool, got: %v", err)
	}
}

// 2. Unknown tool version denied
func TestToolGateway_UnknownVersionDenied(t *testing.T) {
	gw, _, _ := setupTestGateway(t, nil)
	ctx := context.Background()

	execCtx := &TrustedExecutionContext{
		RequestID:           "req-002",
		IdempotencyKey:      "idem-002",
		TenantID:            "TENANT-A",
		CallerID:            "agent-01",
		CallerCapabilities: []ToolCapability{CapIncidentRead},
		CallerAutonomyLevel: 2,
	}

	req := &ToolRequest{
		ToolID:         ToolIncidentGet,
		ToolVersion:    "99.9.9",
		Args:           json.RawMessage(`{"incident_id":"inc-101"}`),
		IdempotencyKey: "idem-002",
	}

	_, err := gw.Execute(ctx, execCtx, req, nil)
	if !errors.Is(err, ErrUnknownToolVersion) {
		t.Errorf("expected ErrUnknownToolVersion, got: %v", err)
	}
}

// 3. Duplicate registration denied
func TestToolGateway_DuplicateRegistrationDenied(t *testing.T) {
	_, reg, _ := setupTestGateway(t, nil)

	m := &ToolManifest{
		ToolID:               "custom.tool",
		Version:              "1.0.0",
		Description:          "Custom",
		Owner:                "test",
		Status:               ManifestStatusActive,
		PolicyDomain:         policy.DomainTool,
		PolicyAction:         "EXECUTE_CUSTOM",
		RequiredCapabilities: []ToolCapability{CapIncidentRead},
		SideEffectClass:      SideEffectReadOnly,
		Timeout:              5 * time.Second,
		MaxOutputBytes:       1024,
	}

	handler := func(ctx context.Context, execCtx *TrustedExecutionContext, args json.RawMessage) (json.RawMessage, error) {
		return json.RawMessage(`{}`), nil
	}

	if err := reg.Register(m, handler); err != nil {
		t.Fatalf("first registration failed: %v", err)
	}

	// Second registration of exact same (tool_id, version) must fail
	err := reg.Register(m, handler)
	if !errors.Is(err, ErrDuplicateToolRegistration) {
		t.Errorf("expected ErrDuplicateToolRegistration, got: %v", err)
	}
}

// 4. Missing / Invalid Tenant ID denied
func TestToolGateway_MissingTenantDenied(t *testing.T) {
	gw, _, _ := setupTestGateway(t, nil)
	ctx := context.Background()

	execCtx := &TrustedExecutionContext{
		RequestID:           "req-003",
		IdempotencyKey:      "idem-003",
		TenantID:            "", // Missing
		CallerID:            "agent-01",
		CallerCapabilities: []ToolCapability{CapIncidentRead},
	}

	req := &ToolRequest{
		ToolID:         ToolIncidentGet,
		Args:           json.RawMessage(`{"incident_id":"inc-101"}`),
		IdempotencyKey: "idem-003",
	}

	_, err := gw.Execute(ctx, execCtx, req, nil)
	if !errors.Is(err, ErrMissingTenantID) {
		t.Errorf("expected ErrMissingTenantID, got: %v", err)
	}
}

// 5. Capability authorization check (missing capability denied)
func TestToolGateway_UnauthorizedCapabilityDenied(t *testing.T) {
	gw, _, _ := setupTestGateway(t, nil)
	ctx := context.Background()

	execCtx := &TrustedExecutionContext{
		RequestID:           "req-004",
		IdempotencyKey:      "idem-004",
		TenantID:            "TENANT-A",
		CallerID:            "agent-01",
		CallerCapabilities: []ToolCapability{}, // Missing CapIncidentRead!
		CallerAutonomyLevel: 2,
	}

	req := &ToolRequest{
		ToolID:         ToolIncidentGet,
		Args:           json.RawMessage(`{"incident_id":"inc-101"}`),
		IdempotencyKey: "idem-004",
	}

	_, err := gw.Execute(ctx, execCtx, req, nil)
	if !errors.Is(err, ErrUnauthorizedCapability) {
		t.Errorf("expected ErrUnauthorizedCapability, got: %v", err)
	}
}

// 6. Autonomy bounds check (exceeded autonomy denied)
func TestToolGateway_AutonomyBoundsCheck(t *testing.T) {
	gw, reg, _ := setupTestGateway(t, nil)
	ctx := context.Background()

	m := &ToolManifest{
		ToolID:               "restricted.autonomy.tool",
		Version:              "1.0.0",
		PolicyAction:         "RESTRICTED_ACTION",
		RequiredCapabilities: []ToolCapability{CapIncidentRead},
		SideEffectClass:      SideEffectReadOnly,
		MinAutonomy:          1,
		MaxAutonomy:          2, // Max autonomy is 2
		Timeout:              5 * time.Second,
		MaxOutputBytes:       1024,
	}
	_ = reg.Register(m, func(ctx context.Context, execCtx *TrustedExecutionContext, args json.RawMessage) (json.RawMessage, error) {
		return json.RawMessage(`{}`), nil
	})

	execCtx := &TrustedExecutionContext{
		RequestID:           "req-005",
		IdempotencyKey:      "idem-005",
		TenantID:            "TENANT-A",
		CallerID:            "agent-01",
		CallerCapabilities: []ToolCapability{CapIncidentRead},
		CallerAutonomyLevel: 4, // Autonomy 4 exceeds max 2!
	}

	req := &ToolRequest{
		ToolID:         "restricted.autonomy.tool",
		Args:           json.RawMessage(`{}`),
		IdempotencyKey: "idem-005",
	}

	_, err := gw.Execute(ctx, execCtx, req, nil)
	if !errors.Is(err, ErrAutonomyExceeded) {
		t.Errorf("expected ErrAutonomyExceeded, got: %v", err)
	}
}

// 7. Policy DENY blocks tool execution
func TestToolGateway_PolicyDenyBlocksExecution(t *testing.T) {
	gw, reg, engine := setupTestGateway(t, nil)
	ctx := context.Background()

	// Register dangerous tool
	m := &ToolManifest{
		ToolID:               "dangerous.modify.original",
		Version:              "1.0.0",
		PolicyDomain:         policy.DomainArtifact,
		PolicyAction:         policy.ActionModifyOriginalArtifact,
		RequiredCapabilities: []ToolCapability{CapIncidentRead},
		SideEffectClass:      SideEffectInternalStateWrite,
		MinAutonomy:          1,
		MaxAutonomy:          4,
		Timeout:              5 * time.Second,
		MaxOutputBytes:       1024,
	}
	_ = reg.Register(m, func(ctx context.Context, execCtx *TrustedExecutionContext, args json.RawMessage) (json.RawMessage, error) {
		return json.RawMessage(`{"status":"modified"}`), nil
	})

	// Add Tenant-layer policy trying to ALLOW modify original (SF-SAFE-001 in safety layer must DENY)
	policies := append(policy.SeedSafetyPolicies(), &policy.PolicyDefinition{
		PolicyID:      "TENANT-ALLOW-MODIFY",
		Version:       1,
		Domain:        policy.DomainArtifact,
		Layer:         policy.LayerTenant,
		Priority:      100,
		Status:        policy.StatusActive,
		EffectiveFrom: policy.DefaultSafetyEffectiveDate,
		Action:        policy.ActionModifyOriginalArtifact,
		Effect:        policy.DecisionAllow,
		ReasonCode:    "TENANT_OVERRIDE_ATTEMPT",
	})
	_ = engine.SetBundle(policies)

	execCtx := &TrustedExecutionContext{
		RequestID:           "req-006",
		IdempotencyKey:      "idem-006",
		TenantID:            "TENANT-A",
		CallerID:            "agent-01",
		CallerCapabilities: []ToolCapability{CapIncidentRead},
		CallerAutonomyLevel: 2,
	}

	req := &ToolRequest{
		ToolID:         "dangerous.modify.original",
		Args:           json.RawMessage(`{}`),
		IdempotencyKey: "idem-006",
	}

	_, err := gw.Execute(ctx, execCtx, req, nil)
	if !errors.Is(err, ErrPolicyDenial) {
		t.Errorf("expected ErrPolicyDenial, got: %v", err)
	}
}

// 8. REQUIRE_HUMAN policy blocks autonomous tool execution
func TestToolGateway_RequireHumanBlocksExecution(t *testing.T) {
	gw, reg, engine := setupTestGateway(t, nil)
	ctx := context.Background()

	m := &ToolManifest{
		ToolID:               "high.value.payment.dispatch",
		Version:              "1.0.0",
		PolicyDomain:         policy.DomainRelease,
		PolicyAction:         "DISPATCH_PAYMENT",
		RequiredCapabilities: []ToolCapability{CapIncidentRead},
		SideEffectClass:      SideEffectInternalStateWrite,
		MinAutonomy:          1,
		MaxAutonomy:          4,
		Timeout:              5 * time.Second,
		MaxOutputBytes:       1024,
	}
	_ = reg.Register(m, func(ctx context.Context, execCtx *TrustedExecutionContext, args json.RawMessage) (json.RawMessage, error) {
		return json.RawMessage(`{}`), nil
	})

	policies := append(policy.SeedSafetyPolicies(), &policy.PolicyDefinition{
		PolicyID:      "ENTERPRISE-REQUIRE-HUMAN-DISPATCH",
		Version:       1,
		Domain:        policy.DomainRelease,
		Layer:         policy.LayerEnterprise,
		Priority:      100,
		Status:        policy.StatusActive,
		EffectiveFrom: policy.DefaultSafetyEffectiveDate,
		Action:        "DISPATCH_PAYMENT",
		Effect:        policy.DecisionRequireHuman,
		ReasonCode:    "HUMAN_APPROVAL_MANDATORY",
	})
	_ = engine.SetBundle(policies)

	execCtx := &TrustedExecutionContext{
		RequestID:           "req-007",
		IdempotencyKey:      "idem-007",
		TenantID:            "TENANT-A",
		CallerID:            "agent-01",
		CallerCapabilities: []ToolCapability{CapIncidentRead},
		CallerAutonomyLevel: 2,
	}

	req := &ToolRequest{
		ToolID:         "high.value.payment.dispatch",
		Args:           json.RawMessage(`{}`),
		IdempotencyKey: "idem-007",
	}

	_, err := gw.Execute(ctx, execCtx, req, nil)
	if !errors.Is(err, ErrRequireHumanReview) {
		t.Errorf("expected ErrRequireHumanReview, got: %v", err)
	}
}

// 9. Fake agent-provided obligation proof is rejected; only infrastructure evidence is trusted
func TestToolGateway_FakeObligationProofRejected(t *testing.T) {
	gw, reg, engine := setupTestGateway(t, nil)
	ctx := context.Background()

	m := &ToolManifest{
		ToolID:               "dual.control.action",
		Version:              "1.0.0",
		PolicyDomain:         policy.DomainRelease,
		PolicyAction:         "DUAL_CONTROL_ACTION",
		RequiredCapabilities: []ToolCapability{CapIncidentRead},
		SideEffectClass:      SideEffectInternalStateWrite,
		MinAutonomy:          1,
		MaxAutonomy:          4,
		Timeout:              5 * time.Second,
		MaxOutputBytes:       1024,
	}
	_ = reg.Register(m, func(ctx context.Context, execCtx *TrustedExecutionContext, args json.RawMessage) (json.RawMessage, error) {
		return json.RawMessage(`{}`), nil
	})

	policies := append(policy.SeedSafetyPolicies(), &policy.PolicyDefinition{
		PolicyID:      "REQUIRE-DUAL-CONTROL-POLICY",
		Version:       1,
		Domain:        policy.DomainRelease,
		Layer:         policy.LayerEnterprise,
		Priority:      100,
		Status:        policy.StatusActive,
		EffectiveFrom: policy.DefaultSafetyEffectiveDate,
		Action:        "DUAL_CONTROL_ACTION",
		Effect:        policy.DecisionAllowWithObligations,
		Obligations: []policy.Obligation{
			{Type: policy.ObligationDualControl},
		},
		ReasonCode: "DUAL_CONTROL_MANDATORY",
	})
	_ = engine.SetBundle(policies)

	execCtx := &TrustedExecutionContext{
		RequestID:           "req-008",
		IdempotencyKey:      "idem-008",
		TenantID:            "TENANT-A",
		CallerID:            "agent-01",
		CallerCapabilities: []ToolCapability{CapIncidentRead},
		CallerAutonomyLevel: 2,
	}

	// Model tries to claim satisfied: true in args
	req := &ToolRequest{
		ToolID:         "dual.control.action",
		Args:           json.RawMessage(`{"satisfied": true, "dual_control": true}`),
		IdempotencyKey: "idem-008",
	}

	// No infrastructure evidence passed in map
	_, err := gw.Execute(ctx, execCtx, req, nil)
	if !errors.Is(err, ErrUnsatisfiedObligation) {
		t.Errorf("expected ErrUnsatisfiedObligation for fake model proof, got: %v", err)
	}

	// Now pass authoritative infrastructure evidence in evidence map with a fresh idempotency key
	evidence := map[policy.ObligationType]*ObligationEvidence{
		policy.ObligationDualControl: {
			Type:        policy.ObligationDualControl,
			Satisfied:   true,
			EvidenceRef: "review-record-999",
			VerifiedAt:  time.Now().UTC(),
		},
	}
	execCtxValid := *execCtx
	execCtxValid.IdempotencyKey = "idem-008-valid"
	reqValid := *req
	reqValid.IdempotencyKey = "idem-008-valid"

	resp, err := gw.Execute(ctx, &execCtxValid, &reqValid, evidence)
	if err != nil {
		t.Fatalf("execution with valid evidence failed: %v", err)
	}
	if resp.Status != StatusSucceeded {
		t.Errorf("expected StatusSucceeded, got: %s", resp.Status)
	}
}

// 10. Idempotency: same key + same request replays; same key + different request conflicts
func TestToolGateway_IdempotencyReplayAndConflict(t *testing.T) {
	db := newToolTestDB(t)
	gw, _, _ := setupTestGateway(t, db)
	ctx := context.Background()

	execCtx := &TrustedExecutionContext{
		RequestID:           "req-009",
		IdempotencyKey:      "idem-fixed-009",
		TenantID:            "TENANT-A",
		CallerID:            "agent-01",
		CallerCapabilities: []ToolCapability{CapIncidentRead},
		CallerAutonomyLevel: 2,
	}

	req1 := &ToolRequest{
		ToolID:         ToolIncidentGet,
		Args:           json.RawMessage(`{"incident_id":"inc-100"}`),
		IdempotencyKey: "idem-fixed-009",
	}

	resp1, err := gw.Execute(ctx, execCtx, req1, nil)
	if err != nil {
		t.Fatalf("first execution failed: %v", err)
	}

	// 1. Same request + same key -> Replay identical response
	respReplay, err := gw.Execute(ctx, execCtx, req1, nil)
	if err != nil {
		t.Fatalf("replay failed: %v", err)
	}
	if respReplay.InvocationID != resp1.InvocationID {
		t.Errorf("expected replayed invocation ID %s, got %s", resp1.InvocationID, respReplay.InvocationID)
	}

	// 2. Different request + same key -> Conflict
	reqDifferent := &ToolRequest{
		ToolID:         ToolIncidentGet,
		Args:           json.RawMessage(`{"incident_id":"inc-DIFFERENT"}`),
		IdempotencyKey: "idem-fixed-009",
	}

	_, err = gw.Execute(ctx, execCtx, reqDifferent, nil)
	if !errors.Is(err, ErrIdempotencyConflict) {
		t.Errorf("expected ErrIdempotencyConflict on payload mismatch, got: %v", err)
	}
}

// 11. Concurrent duplicate requests: single execution, others receive result
func TestToolGateway_ConcurrentDuplicateRequestsSingleExecution(t *testing.T) {
	gw, reg, _ := setupTestGateway(t, nil)
	ctx := context.Background()

	var executionCount int
	var mu sync.Mutex

	m := &ToolManifest{
		ToolID:               "slow.counter.tool",
		Version:              "1.0.0",
		PolicyAction:         "GET_INCIDENT", // Permitted by pre-seeded policy
		RequiredCapabilities: []ToolCapability{CapIncidentRead},
		SideEffectClass:      SideEffectReadOnly,
		Timeout:              5 * time.Second,
		MaxOutputBytes:       1024,
	}

	_ = reg.Register(m, func(ctx context.Context, execCtx *TrustedExecutionContext, args json.RawMessage) (json.RawMessage, error) {
		time.Sleep(50 * time.Millisecond) // Simulate slow operation
		mu.Lock()
		executionCount++
		mu.Unlock()
		return json.RawMessage(`{"result":"ok"}`), nil
	})

	const numCallers = 10
	var wg sync.WaitGroup
	responses := make([]*ToolResponse, numCallers)
	errs := make([]error, numCallers)

	for i := 0; i < numCallers; i++ {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			execCtx := &TrustedExecutionContext{
				RequestID:           fmt.Sprintf("req-conc-%d", idx),
				IdempotencyKey:      "shared-concurrent-key",
				TenantID:            "TENANT-A",
				CallerID:            "agent-01",
				CallerCapabilities: []ToolCapability{CapIncidentRead},
				CallerAutonomyLevel: 2,
			}
			req := &ToolRequest{
				ToolID:         "slow.counter.tool",
				Args:           json.RawMessage(`{"field":"same"}`),
				IdempotencyKey: "shared-concurrent-key",
			}
			resp, err := gw.Execute(ctx, execCtx, req, nil)
			responses[idx] = resp
			errs[idx] = err
		}(i)
	}

	wg.Wait()

	for i, err := range errs {
		if err != nil {
			t.Errorf("caller %d failed: %v", i, err)
		}
	}

	// Verify exactly 1 execution occurred
	mu.Lock()
	count := executionCount
	mu.Unlock()

	if count != 1 {
		t.Errorf("expected exactly 1 handler execution for concurrent duplicate requests, got %d", count)
	}
}

// 12. Bounded timeout check
func TestToolGateway_TimeoutBounded(t *testing.T) {
	gw, reg, _ := setupTestGateway(t, nil)
	ctx := context.Background()

	m := &ToolManifest{
		ToolID:               "timeout.tool",
		Version:              "1.0.0",
		PolicyAction:         "GET_INCIDENT",
		RequiredCapabilities: []ToolCapability{CapIncidentRead},
		SideEffectClass:      SideEffectReadOnly,
		Timeout:              50 * time.Millisecond, // Strict 50ms timeout
		MaxOutputBytes:       1024,
	}

	_ = reg.Register(m, func(ctx context.Context, execCtx *TrustedExecutionContext, args json.RawMessage) (json.RawMessage, error) {
		select {
		case <-time.After(200 * time.Millisecond):
			return json.RawMessage(`{}`), nil
		case <-ctx.Done():
			return nil, ctx.Err()
		}
	})

	execCtx := &TrustedExecutionContext{
		RequestID:           "req-010",
		IdempotencyKey:      "idem-010",
		TenantID:            "TENANT-A",
		CallerID:            "agent-01",
		CallerCapabilities: []ToolCapability{CapIncidentRead},
		CallerAutonomyLevel: 2,
	}

	req := &ToolRequest{
		ToolID:         "timeout.tool",
		Args:           json.RawMessage(`{}`),
		IdempotencyKey: "idem-010",
	}

	_, err := gw.Execute(ctx, execCtx, req, nil)
	if !errors.Is(err, ErrToolExecutionTimeout) {
		t.Errorf("expected ErrToolExecutionTimeout, got: %v", err)
	}
}

// 13. Panic recovery isolation
func TestToolGateway_PanicRecoveryIsolation(t *testing.T) {
	gw, reg, _ := setupTestGateway(t, nil)
	ctx := context.Background()

	m := &ToolManifest{
		ToolID:               "panicking.tool",
		Version:              "1.0.0",
		PolicyAction:         "GET_INCIDENT",
		RequiredCapabilities: []ToolCapability{CapIncidentRead},
		SideEffectClass:      SideEffectReadOnly,
		Timeout:              5 * time.Second,
		MaxOutputBytes:       1024,
	}

	_ = reg.Register(m, func(ctx context.Context, execCtx *TrustedExecutionContext, args json.RawMessage) (json.RawMessage, error) {
		panic("unexpected internal nil pointer")
	})

	execCtx := &TrustedExecutionContext{
		RequestID:           "req-011",
		IdempotencyKey:      "idem-011",
		TenantID:            "TENANT-A",
		CallerID:            "agent-01",
		CallerCapabilities: []ToolCapability{CapIncidentRead},
		CallerAutonomyLevel: 2,
	}

	req := &ToolRequest{
		ToolID:         "panicking.tool",
		Args:           json.RawMessage(`{}`),
		IdempotencyKey: "idem-011",
	}

	_, err := gw.Execute(ctx, execCtx, req, nil)
	if !errors.Is(err, ErrToolPanicRecovered) {
		t.Errorf("expected ErrToolPanicRecovered, got: %v", err)
	}
}

// 14. Shadow Mode blocks prohibited side-effects
func TestToolGateway_ShadowModeBlocksSideEffects(t *testing.T) {
	gw, reg, _ := setupTestGateway(t, nil)
	ctx := context.Background()

	m := &ToolManifest{
		ToolID:               "stateful.write.tool",
		Version:              "1.0.0",
		PolicyAction:         "GET_INCIDENT",
		RequiredCapabilities: []ToolCapability{CapIncidentRead},
		SideEffectClass:      SideEffectInternalStateWrite, // Write class
		ShadowModeAllowed:    false,
		Timeout:              5 * time.Second,
		MaxOutputBytes:       1024,
	}

	_ = reg.Register(m, func(ctx context.Context, execCtx *TrustedExecutionContext, args json.RawMessage) (json.RawMessage, error) {
		return json.RawMessage(`{}`), nil
	})

	execCtx := &TrustedExecutionContext{
		RequestID:           "req-012",
		IdempotencyKey:      "idem-012",
		TenantID:            "TENANT-A",
		CallerID:            "agent-01",
		CallerCapabilities: []ToolCapability{CapIncidentRead},
		CallerAutonomyLevel: 2,
		ExecutionMode:       "SHADOW", // In SHADOW mode!
	}

	req := &ToolRequest{
		ToolID:         "stateful.write.tool",
		Args:           json.RawMessage(`{}`),
		IdempotencyKey: "idem-012",
	}

	_, err := gw.Execute(ctx, execCtx, req, nil)
	if !errors.Is(err, ErrShadowModeProhibited) {
		t.Errorf("expected ErrShadowModeProhibited in SHADOW mode, got: %v", err)
	}
}

// 15. Irreversible financial tool registration for agents is rejected at startup
func TestToolGateway_IrreversibleFinancialToolRegistrationFails(t *testing.T) {
	reg := NewRegistry()

	m := &ToolManifest{
		ToolID:               "bank.wire.transfer",
		Version:              "1.0.0",
		PolicyAction:         "WIRE_TRANSFER",
		RequiredCapabilities: []ToolCapability{CapIncidentRead},
		SideEffectClass:      SideEffectIrreversibleFinancial, // Irreversible financial!
		MaxAutonomy:          3,                               // Agent autonomy > 0
		Timeout:              5 * time.Second,
		MaxOutputBytes:       1024,
	}

	err := reg.Register(m, func(ctx context.Context, execCtx *TrustedExecutionContext, args json.RawMessage) (json.RawMessage, error) {
		return json.RawMessage(`{}`), nil
	})

	if !errors.Is(err, ErrIrreversibleFinancialAgent) {
		t.Errorf("expected ErrIrreversibleFinancialAgent on registering financial tool, got: %v", err)
	}
}

// 16. Output validation: sensitive unmasked SSN pattern is blocked
func TestToolGateway_OutputRedactionValidation(t *testing.T) {
	gw, reg, _ := setupTestGateway(t, nil)
	ctx := context.Background()

	m := &ToolManifest{
		ToolID:               "buggy.tool.leaking.ssn",
		Version:              "1.0.0",
		PolicyAction:         "GET_INCIDENT",
		RequiredCapabilities: []ToolCapability{CapIncidentRead},
		SideEffectClass:      SideEffectReadOnly,
		Timeout:              5 * time.Second,
		MaxOutputBytes:       1024,
	}

	_ = reg.Register(m, func(ctx context.Context, execCtx *TrustedExecutionContext, args json.RawMessage) (json.RawMessage, error) {
		return json.RawMessage(`{"customer_ssn":"123-45-6789"}`), nil
	})

	execCtx := &TrustedExecutionContext{
		RequestID:           "req-013",
		IdempotencyKey:      "idem-013",
		TenantID:            "TENANT-A",
		CallerID:            "agent-01",
		CallerCapabilities: []ToolCapability{CapIncidentRead},
		CallerAutonomyLevel: 2,
	}

	req := &ToolRequest{
		ToolID:         "buggy.tool.leaking.ssn",
		Args:           json.RawMessage(`{}`),
		IdempotencyKey: "idem-013",
	}

	_, err := gw.Execute(ctx, execCtx, req, nil)
	if !errors.Is(err, ErrOutputValidationFailed) {
		t.Errorf("expected ErrOutputValidationFailed for unmasked SSN, got: %v", err)
	}
}

// 17. Successful execution records durable invocation & outbox event atomically
func TestToolGateway_DurablePersistenceAndOutboxJournaling(t *testing.T) {
	db := newToolTestDB(t)
	gw, _, _ := setupTestGateway(t, db)
	ctx := context.Background()

	execCtx := &TrustedExecutionContext{
		RequestID:           "req-014",
		IdempotencyKey:      "idem-014",
		TenantID:            "TENANT-A",
		CallerID:            "agent-01",
		CallerCapabilities: []ToolCapability{CapIncidentRead},
		CallerAutonomyLevel: 2,
		WorkflowID:          "wf-999",
	}

	req := &ToolRequest{
		ToolID:         ToolIncidentGet,
		Args:           json.RawMessage(`{"incident_id":"inc-100"}`),
		IdempotencyKey: "idem-014",
	}

	resp, err := gw.Execute(ctx, execCtx, req, nil)
	if err != nil {
		t.Fatalf("execution failed: %v", err)
	}
	if resp.Status != StatusSucceeded {
		t.Errorf("expected StatusSucceeded, got: %s", resp.Status)
	}

	// Verify durable tool_invocations row
	var invCount int
	err = db.QueryRowContext(ctx, "SELECT count(*) FROM tool_invocations WHERE idempotency_key = 'idem-014'").Scan(&invCount)
	if err != nil || invCount != 1 {
		t.Fatalf("expected 1 tool_invocations row, got %d (err: %v)", invCount, err)
	}

	// Verify outbox_events row
	var outboxCount int
	err = db.QueryRowContext(ctx, "SELECT count(*) FROM outbox_events WHERE dedupe_key = ?", fmt.Sprintf("tool-inv-%s", resp.InvocationID)).Scan(&outboxCount)
	if err != nil || outboxCount != 1 {
		t.Fatalf("expected 1 outbox_events row, got %d (err: %v)", outboxCount, err)
	}
}

// 18. Durable idempotency across gateway restart proves exactly 1 handler execution
func TestToolGateway_DurableRestartIdempotency_ProvesSingleExecution(t *testing.T) {
	db := newToolTestDB(t)
	ctx := context.Background()

	var handlerExecutionCount int
	var countMu sync.Mutex

	handler := func(ctx context.Context, execCtx *TrustedExecutionContext, args json.RawMessage) (json.RawMessage, error) {
		countMu.Lock()
		handlerExecutionCount++
		countMu.Unlock()
		return json.RawMessage(`{"incident_id":"inc-restart-101","status":"QUARANTINED","data_classification":"METADATA_ONLY"}`), nil
	}

	// 1. Start Process/Instance 1
	reg1 := NewRegistry()
	m := &ToolManifest{
		ToolID:                       "restart.test.tool",
		Version:                      "1.0.0",
		PolicyDomain:                 policy.DomainArtifact,
		PolicyAction:                 policy.ActionGetIncident,
		RequiredCapabilities:         []ToolCapability{CapIncidentRead},
		SideEffectClass:              SideEffectReadOnly,
		Timeout:                      5 * time.Second,
		MaxOutputBytes:               1024,
		DataClassifications:          []DataClassification{ClassificationMetadataOnly},
		AllowedOutputClassifications: []DataClassification{ClassificationMetadataOnly},
	}
	_ = reg1.Register(m, handler)
	engine1 := policy.NewEngineWithDefaults()
	store1 := NewToolStore(db)
	gw1 := NewToolGatewayService(reg1, engine1, store1)

	execCtx := &TrustedExecutionContext{
		RequestID:           "req-restart-01",
		IdempotencyKey:      "idem-restart-key-101",
		TenantID:            "TENANT-A",
		CallerID:            "agent-01",
		CallerCapabilities: []ToolCapability{CapIncidentRead},
		CallerAutonomyLevel: 2,
	}

	req := &ToolRequest{
		ToolID:         "restart.test.tool",
		Args:           json.RawMessage(`{"incident_id":"inc-restart-101"}`),
		IdempotencyKey: "idem-restart-key-101",
	}

	resp1, err := gw1.Execute(ctx, execCtx, req, nil)
	if err != nil || resp1.Status != StatusSucceeded {
		t.Fatalf("instance 1 execution failed: %v", err)
	}

	countMu.Lock()
	countAfterFirst := handlerExecutionCount
	countMu.Unlock()
	if countAfterFirst != 1 {
		t.Fatalf("expected handler to execute exactly 1 time, got %d", countAfterFirst)
	}

	// 2. Simulate Process Restart: Destroy gw1, create brand new Instance 2 with empty memory
	reg2 := NewRegistry()
	_ = reg2.Register(m, handler)
	engine2 := policy.NewEngineWithDefaults()
	store2 := NewToolStore(db)
	gw2 := NewToolGatewayService(reg2, engine2, store2)

	// Replay same request on new instance
	resp2, err := gw2.Execute(ctx, execCtx, req, nil)
	if err != nil {
		t.Fatalf("instance 2 replay failed: %v", err)
	}
	if resp2.InvocationID != resp1.InvocationID {
		t.Errorf("expected replayed invocation ID %s, got %s", resp1.InvocationID, resp2.InvocationID)
	}
	if resp2.Status != StatusSucceeded {
		t.Errorf("expected StatusSucceeded, got %s", resp2.Status)
	}

	// Prove handler execution count remains 1
	countMu.Lock()
	countAfterRestart := handlerExecutionCount
	countMu.Unlock()
	if countAfterRestart != 1 {
		t.Errorf("DURABLE IDEMPOTENCY VIOLATION: handler executed again after restart (count: %d, want: 1)", countAfterRestart)
	}
}

// 19. Output security: Structured field marked SECRET is strictly rejected
func TestToolGateway_OutputSecurity_ProhibitsSecretClassification(t *testing.T) {
	gw, reg, _ := setupTestGateway(t, nil)
	ctx := context.Background()

	m := &ToolManifest{
		ToolID:                       "leaking.secret.class.tool",
		Version:                      "1.0.0",
		PolicyAction:                 "GET_INCIDENT",
		RequiredCapabilities:         []ToolCapability{CapIncidentRead},
		SideEffectClass:              SideEffectReadOnly,
		Timeout:                      5 * time.Second,
		MaxOutputBytes:               1024,
		DataClassifications:          []DataClassification{ClassificationMetadataOnly},
		AllowedOutputClassifications: []DataClassification{ClassificationMetadataOnly, ClassificationSecret},
	}
	_ = reg.Register(m, func(ctx context.Context, execCtx *TrustedExecutionContext, args json.RawMessage) (json.RawMessage, error) {
		return json.RawMessage(`{"incident_id":"inc-1","data_classification":"SECRET","secret_token":"xyz"}`), nil
	})

	execCtx := &TrustedExecutionContext{
		RequestID:           "req-sec-01",
		IdempotencyKey:      "idem-sec-01",
		TenantID:            "TENANT-A",
		CallerType:          CallerTypeAgent,
		CallerID:            "agent-01",
		CallerCapabilities: []ToolCapability{CapIncidentRead},
		CallerAutonomyLevel: 2,
	}
	req := &ToolRequest{
		ToolID:         "leaking.secret.class.tool",
		Args:           json.RawMessage(`{}`),
		IdempotencyKey: "idem-sec-01",
	}

	_, err := gw.Execute(ctx, execCtx, req, nil)
	if !errors.Is(err, ErrOutputValidationFailed) {
		t.Errorf("expected ErrOutputValidationFailed for SECRET classification across AI boundary, got: %v", err)
	}
}

// 20. Output security: Unpermitted PII classification is rejected
func TestToolGateway_OutputSecurity_ProhibitsUnpermittedPII(t *testing.T) {
	gw, reg, _ := setupTestGateway(t, nil)
	ctx := context.Background()

	m := &ToolManifest{
		ToolID:                       "unpermitted.pii.tool",
		Version:                      "1.0.0",
		PolicyAction:                 "GET_INCIDENT",
		RequiredCapabilities:         []ToolCapability{CapIncidentRead},
		SideEffectClass:              SideEffectReadOnly,
		Timeout:                      5 * time.Second,
		MaxOutputBytes:               1024,
		DataClassifications:          []DataClassification{ClassificationMetadataOnly},
		AllowedOutputClassifications: []DataClassification{ClassificationMetadataOnly}, // PII not allowed
	}
	_ = reg.Register(m, func(ctx context.Context, execCtx *TrustedExecutionContext, args json.RawMessage) (json.RawMessage, error) {
		return json.RawMessage(`{"incident_id":"inc-1","data_classification":"PII","customer_name":"Alice"}`), nil
	})

	execCtx := &TrustedExecutionContext{
		RequestID:           "req-pii-01",
		IdempotencyKey:      "idem-pii-01",
		TenantID:            "TENANT-A",
		CallerType:          CallerTypeAgent,
		CallerID:            "agent-01",
		CallerCapabilities: []ToolCapability{CapIncidentRead},
		CallerAutonomyLevel: 2,
	}
	req := &ToolRequest{
		ToolID:         "unpermitted.pii.tool",
		Args:           json.RawMessage(`{}`),
		IdempotencyKey: "idem-pii-01",
	}

	_, err := gw.Execute(ctx, execCtx, req, nil)
	if !errors.Is(err, ErrOutputValidationFailed) {
		t.Errorf("expected ErrOutputValidationFailed for unpermitted PII, got: %v", err)
	}
}

// 21. Output security: Forbidden secret keys are rejected
func TestToolGateway_OutputSecurity_ProhibitsForbiddenSecretKeys(t *testing.T) {
	gw, reg, _ := setupTestGateway(t, nil)
	ctx := context.Background()

	m := &ToolManifest{
		ToolID:                       "leaking.secret.key.tool",
		Version:                      "1.0.0",
		PolicyAction:                 "GET_INCIDENT",
		RequiredCapabilities:         []ToolCapability{CapIncidentRead},
		SideEffectClass:              SideEffectReadOnly,
		Timeout:                      5 * time.Second,
		MaxOutputBytes:               1024,
		DataClassifications:          []DataClassification{ClassificationMetadataOnly},
		AllowedOutputClassifications: []DataClassification{ClassificationMetadataOnly},
	}
	_ = reg.Register(m, func(ctx context.Context, execCtx *TrustedExecutionContext, args json.RawMessage) (json.RawMessage, error) {
		return json.RawMessage(`{"incident_id":"inc-1","api_key":"sk-secret-123"}`), nil
	})

	execCtx := &TrustedExecutionContext{
		RequestID:           "req-key-01",
		IdempotencyKey:      "idem-key-01",
		TenantID:            "TENANT-A",
		CallerType:          CallerTypeAgent,
		CallerID:            "agent-01",
		CallerCapabilities: []ToolCapability{CapIncidentRead},
		CallerAutonomyLevel: 2,
	}
	req := &ToolRequest{
		ToolID:         "leaking.secret.key.tool",
		Args:           json.RawMessage(`{}`),
		IdempotencyKey: "idem-key-01",
	}

	_, err := gw.Execute(ctx, execCtx, req, nil)
	if !errors.Is(err, ErrOutputValidationFailed) {
		t.Errorf("expected ErrOutputValidationFailed for secret key, got: %v", err)
	}
}

// 22. Output security: Unmasked 9-digit ABA Routing Number is rejected
func TestToolGateway_OutputSecurity_ProhibitsUnmaskedRoutingNumber(t *testing.T) {
	gw, reg, _ := setupTestGateway(t, nil)
	ctx := context.Background()

	m := &ToolManifest{
		ToolID:                       "leaking.routing.tool",
		Version:                      "1.0.0",
		PolicyAction:                 "GET_INCIDENT",
		RequiredCapabilities:         []ToolCapability{CapIncidentRead},
		SideEffectClass:              SideEffectReadOnly,
		Timeout:                      5 * time.Second,
		MaxOutputBytes:               1024,
		DataClassifications:          []DataClassification{ClassificationMetadataOnly},
		AllowedOutputClassifications: []DataClassification{ClassificationMetadataOnly},
	}
	// 021000021 is a valid Federal Reserve Bank of New York routing number passing Mod-10
	_ = reg.Register(m, func(ctx context.Context, execCtx *TrustedExecutionContext, args json.RawMessage) (json.RawMessage, error) {
		return json.RawMessage(`{"incident_id":"inc-1","bank_routing":"021000021"}`), nil
	})

	execCtx := &TrustedExecutionContext{
		RequestID:           "req-rt-01",
		IdempotencyKey:      "idem-rt-01",
		TenantID:            "TENANT-A",
		CallerType:          CallerTypeAgent,
		CallerID:            "agent-01",
		CallerCapabilities: []ToolCapability{CapIncidentRead},
		CallerAutonomyLevel: 2,
	}
	req := &ToolRequest{
		ToolID:         "leaking.routing.tool",
		Args:           json.RawMessage(`{}`),
		IdempotencyKey: "idem-rt-01",
	}

	_, err := gw.Execute(ctx, execCtx, req, nil)
	if !errors.Is(err, ErrOutputValidationFailed) {
		t.Errorf("expected ErrOutputValidationFailed for unmasked routing number, got: %v", err)
	}
}

// 23. Output security: Harmless numeric strings (timestamps, zip codes, ids, hashes) do not false positive
func TestToolGateway_OutputSecurity_HarmlessNumericStringsPass(t *testing.T) {
	gw, reg, _ := setupTestGateway(t, nil)
	ctx := context.Background()

	m := &ToolManifest{
		ToolID:                       "safe.numeric.tool",
		Version:                      "1.0.0",
		PolicyAction:                 "GET_INCIDENT",
		RequiredCapabilities:         []ToolCapability{CapIncidentRead},
		SideEffectClass:              SideEffectReadOnly,
		Timeout:                      5 * time.Second,
		MaxOutputBytes:               4096,
		DataClassifications:          []DataClassification{ClassificationMetadataOnly},
		AllowedOutputClassifications: []DataClassification{ClassificationMetadataOnly},
	}
	_ = reg.Register(m, func(ctx context.Context, execCtx *TrustedExecutionContext, args json.RawMessage) (json.RawMessage, error) {
		return json.RawMessage(`{
			"incident_id": "inc-9999",
			"created_at": "2026-08-19T15:00:00Z",
			"unix_timestamp": 1787096991033,
			"zip_code": "10001",
			"sha256": "e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855",
			"attempt_count": 1,
			"data_classification": "METADATA_ONLY"
		}`), nil
	})

	execCtx := &TrustedExecutionContext{
		RequestID:           "req-safe-01",
		IdempotencyKey:      "idem-safe-01",
		TenantID:            "TENANT-A",
		CallerType:          CallerTypeAgent,
		CallerID:            "agent-01",
		CallerCapabilities: []ToolCapability{CapIncidentRead},
		CallerAutonomyLevel: 2,
	}
	req := &ToolRequest{
		ToolID:         "safe.numeric.tool",
		Args:           json.RawMessage(`{}`),
		IdempotencyKey: "idem-safe-01",
	}

	resp, err := gw.Execute(ctx, execCtx, req, nil)
	if err != nil {
		t.Fatalf("expected harmless numeric strings to pass without false positive, got error: %v", err)
	}
	if resp.Status != StatusSucceeded {
		t.Errorf("expected StatusSucceeded, got: %s", resp.Status)
	}
}

// 24. Non-Agent caller semantics: HUMAN, SERVICE, DETERMINISTIC_CONTROL
func TestToolGateway_NonAgentCallerSemantics(t *testing.T) {
	gw, _, _ := setupTestGateway(t, nil)
	ctx := context.Background()

	// 1. Human caller with valid capabilities and tenant succeeds without agent autonomy check
	humanCtx := &TrustedExecutionContext{
		RequestID:           "req-human-01",
		IdempotencyKey:      "idem-human-01",
		TenantID:            "TENANT-A",
		CallerType:          CallerTypeHuman,
		CallerID:            "user-ops-analyst-42",
		CallerRoles:         []string{"OPERATOR", "ANALYST"},
		CallerCapabilities: []ToolCapability{CapIncidentRead},
		CallerAutonomyLevel: 0, // Humans do not have agent autonomy levels
	}
	req := &ToolRequest{
		ToolID:         ToolIncidentGet,
		Args:           json.RawMessage(`{"incident_id":"inc-100"}`),
		IdempotencyKey: "idem-human-01",
	}

	resp, err := gw.Execute(ctx, humanCtx, req, nil)
	if err != nil {
		t.Fatalf("human execution failed: %v", err)
	}
	if resp.Status != StatusSucceeded {
		t.Errorf("expected StatusSucceeded for human caller, got: %s", resp.Status)
	}

	// 2. Service caller lacking capability still fails closed
	unauthorizedServiceCtx := &TrustedExecutionContext{
		RequestID:           "req-svc-01",
		IdempotencyKey:      "idem-svc-01",
		TenantID:            "TENANT-A",
		CallerType:          CallerTypeService,
		CallerID:            "cron-worker-01",
		CallerCapabilities: []ToolCapability{}, // Missing CapIncidentRead
	}
	_, err = gw.Execute(ctx, unauthorizedServiceCtx, req, nil)
	if !errors.Is(err, ErrUnauthorizedCapability) {
		t.Errorf("expected ErrUnauthorizedCapability for service caller lacking capability, got: %v", err)
	}
}

