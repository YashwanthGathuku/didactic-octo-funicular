package executioncontrol

import (
	"errors"
	"testing"
	"time"
)

func mustController(t *testing.T, limits Limits) *Controller {
	t.Helper()
	c, err := NewController(limits)
	if err != nil {
		t.Fatal(err)
	}
	return c
}

func agentReq(now time.Time) CheckRequest {
	return CheckRequest{
		TenantID:   "tenant-A",
		WorkflowID: "wf-1",
		CallerID:   "DiagnosisAgent",
		CallerType: "AGENT",
		Now:        now,
	}
}

func TestController_GlobalKillSwitchFailsClosed(t *testing.T) {
	now := time.Now().UTC()
	c := mustController(t, Limits{MaxToolCalls: 10, MaxConcurrent: 2, MaxDuration: time.Minute})
	if err := c.SetKillSwitch(KillSwitch{
		Scope: ScopeGlobal, Enabled: true, Reason: "operator emergency stop", Generation: 1, UpdatedAt: now,
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := c.Acquire(agentReq(now)); !errors.Is(err, ErrKilled) {
		t.Fatalf("expected ErrKilled, got %v", err)
	}
}

func TestController_HumanRecoveryNotBlockedByAgentKillSwitch(t *testing.T) {
	now := time.Now().UTC()
	c := mustController(t, Limits{})
	if err := c.SetKillSwitch(KillSwitch{Scope: ScopeGlobal, Enabled: true, Reason: "stop agents", Generation: 1}); err != nil {
		t.Fatal(err)
	}
	req := agentReq(now)
	req.CallerType = "HUMAN"
	permit, err := c.Acquire(req)
	if err != nil {
		t.Fatalf("human recovery path must remain available: %v", err)
	}
	permit.Release()
}

func TestController_KillSwitchGenerationMonotonic(t *testing.T) {
	c := mustController(t, Limits{})
	first := KillSwitch{Scope: ScopeTenant, ScopeID: "tenant-A", Enabled: true, Generation: 4}
	if err := c.SetKillSwitch(first); err != nil {
		t.Fatal(err)
	}
	if err := c.SetKillSwitch(KillSwitch{Scope: ScopeTenant, ScopeID: "tenant-A", Enabled: false, Generation: 4}); !errors.Is(err, ErrStaleGeneration) {
		t.Fatalf("expected stale generation rejection, got %v", err)
	}
	if err := c.SetKillSwitch(KillSwitch{Scope: ScopeTenant, ScopeID: "tenant-A", Enabled: false, Generation: 5}); err != nil {
		t.Fatalf("newer disable should succeed: %v", err)
	}
}

func TestController_ToolCallBudget(t *testing.T) {
	now := time.Now().UTC()
	c := mustController(t, Limits{MaxToolCalls: 2})
	for i := 0; i < 2; i++ {
		permit, err := c.Acquire(agentReq(now.Add(time.Duration(i) * time.Second)))
		if err != nil {
			t.Fatalf("acquire %d: %v", i, err)
		}
		permit.Release()
	}
	if _, err := c.Acquire(agentReq(now.Add(3 * time.Second))); !errors.Is(err, ErrToolBudgetExhausted) {
		t.Fatalf("expected tool budget exhaustion, got %v", err)
	}
}

func TestController_ConcurrentBudget(t *testing.T) {
	now := time.Now().UTC()
	c := mustController(t, Limits{MaxConcurrent: 1})
	first, err := c.Acquire(agentReq(now))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := c.Acquire(agentReq(now)); !errors.Is(err, ErrConcurrentExceeded) {
		t.Fatalf("expected concurrent budget exhaustion, got %v", err)
	}
	first.Release()
	second, err := c.Acquire(agentReq(now))
	if err != nil {
		t.Fatalf("slot should be reusable after Release: %v", err)
	}
	second.Release()
}

func TestController_DurationBudget(t *testing.T) {
	started := time.Date(2026, 8, 23, 12, 0, 0, 0, time.UTC)
	c := mustController(t, Limits{})
	if err := c.ConfigureWorkflow("tenant-A", "wf-1", Limits{MaxDuration: 30 * time.Second}, started); err != nil {
		t.Fatal(err)
	}
	if _, err := c.Acquire(agentReq(started.Add(30 * time.Second))); !errors.Is(err, ErrDeadlineExceeded) {
		t.Fatalf("expected deadline exceeded, got %v", err)
	}
}

func TestController_ScopeIsolation(t *testing.T) {
	now := time.Now().UTC()
	c := mustController(t, Limits{})
	if err := c.SetKillSwitch(KillSwitch{Scope: ScopeTenant, ScopeID: "tenant-A", Enabled: true, Generation: 1}); err != nil {
		t.Fatal(err)
	}
	if _, err := c.Acquire(agentReq(now)); !errors.Is(err, ErrKilled) {
		t.Fatalf("tenant A should be killed, got %v", err)
	}
	reqB := agentReq(now)
	reqB.TenantID = "tenant-B"
	permit, err := c.Acquire(reqB)
	if err != nil {
		t.Fatalf("tenant B must remain isolated: %v", err)
	}
	permit.Release()
}

func TestController_ExpiredKillSwitchDoesNotBlock(t *testing.T) {
	now := time.Now().UTC()
	c := mustController(t, Limits{})
	if err := c.SetKillSwitch(KillSwitch{
		Scope: ScopeWorkflow, ScopeID: "wf-1", Enabled: true, Generation: 1, ExpiresAt: now.Add(-time.Second),
	}); err != nil {
		t.Fatal(err)
	}
	permit, err := c.Acquire(agentReq(now))
	if err != nil {
		t.Fatalf("expired switch should not block: %v", err)
	}
	permit.Release()
}
