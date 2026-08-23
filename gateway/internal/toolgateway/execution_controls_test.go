package toolgateway

import (
	"errors"
	"testing"
	"time"
)

func baseTrustedContext(now time.Time) *TrustedExecutionContext {
	return &TrustedExecutionContext{
		RequestID:      "req-1",
		IdempotencyKey: "idem-1",
		TenantID:       "tenant-A",
		CallerType:     "AGENT",
		CallerID:       "DiagnosisAgent",
		ExecutionMode:  "LIVE",
		Timestamp:      now,
	}
}

func TestTrustedExecutionContext_AgentKillSwitchFailsClosed(t *testing.T) {
	now := time.Now().UTC()
	ctx := baseTrustedContext(now)
	ctx.AgentExecutionDisabled = true
	ctx.ExecutionDisableReason = "operator emergency stop"
	ctx.KillSwitchGeneration = 7
	if err := ctx.Validate(); !errors.Is(err, ErrAgentExecutionDisabled) {
		t.Fatalf("expected ErrAgentExecutionDisabled, got %v", err)
	}
}

func TestTrustedExecutionContext_HumanRecoveryBypassesAgentKillSwitch(t *testing.T) {
	ctx := baseTrustedContext(time.Now().UTC())
	ctx.CallerType = "HUMAN"
	ctx.AgentExecutionDisabled = true
	ctx.ExecutionDisableReason = "stop agents"
	if err := ctx.Validate(); err != nil {
		t.Fatalf("human recovery context should remain valid: %v", err)
	}
}

func TestTrustedExecutionContext_ToolBudget(t *testing.T) {
	ctx := baseTrustedContext(time.Now().UTC())
	ctx.MaxToolCalls = 3
	ctx.ToolCallOrdinal = 4
	if err := ctx.Validate(); !errors.Is(err, ErrAgentBudgetExhausted) {
		t.Fatalf("expected budget exhaustion, got %v", err)
	}

	ctx.ToolCallOrdinal = 3
	if err := ctx.Validate(); err != nil {
		t.Fatalf("final allowed ordinal should pass: %v", err)
	}
}

func TestTrustedExecutionContext_DurationBudget(t *testing.T) {
	started := time.Date(2026, 8, 23, 12, 0, 0, 0, time.UTC)
	ctx := baseTrustedContext(started.Add(10 * time.Second))
	ctx.WorkflowStartedAt = started
	ctx.MaxWorkflowDurationSec = 10
	if err := ctx.Validate(); !errors.Is(err, ErrAgentDeadlineExceeded) {
		t.Fatalf("expected deadline at exact boundary, got %v", err)
	}

	ctx.Timestamp = started.Add(9 * time.Second)
	if err := ctx.Validate(); err != nil {
		t.Fatalf("pre-deadline execution should pass: %v", err)
	}
}

func TestTrustedExecutionContext_BudgetFieldsBoundIntoCanonicalHash(t *testing.T) {
	now := time.Date(2026, 8, 23, 12, 0, 0, 0, time.UTC)
	a := baseTrustedContext(now)
	a.MaxToolCalls = 3
	a.ToolCallOrdinal = 1
	b := *a
	b.MaxToolCalls = 4

	ha, err := a.CanonicalHash()
	if err != nil {
		t.Fatal(err)
	}
	hb, err := b.CanonicalHash()
	if err != nil {
		t.Fatal(err)
	}
	if ha == hb {
		t.Fatal("changing trusted execution budget must change canonical context hash")
	}
}
