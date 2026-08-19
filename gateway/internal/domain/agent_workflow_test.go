package domain

import (
	"testing"
)

func TestAgentWorkflowStateMachine_ValidTransitions(t *testing.T) {
	// Happy path lifecycle: PENDING -> CONTEXT_BUILDING -> INVESTIGATING -> PLANNING -> REMEDIATING -> VALIDATING_CANDIDATE -> VERIFIED -> HUMAN_REVIEW -> COMPLETED
	lifecycle := []AgentWorkflowState{
		WorkflowPending,
		WorkflowContextBuilding,
		WorkflowInvestigating,
		WorkflowPlanning,
		WorkflowRemediating,
		WorkflowValidatingCandidate,
		WorkflowVerified,
		WorkflowHumanReview,
		WorkflowCompleted,
	}

	for i := 0; i < len(lifecycle)-1; i++ {
		from := lifecycle[i]
		to := lifecycle[i+1]
		if !CanTransitionAgentWorkflow(from, to) {
			t.Errorf("expected valid transition %s -> %s", from, to)
		}
		next, err := TransitionAgentWorkflow(from, to)
		if err != nil {
			t.Errorf("unexpected error transitioning %s -> %s: %v", from, to, err)
		}
		if next != to {
			t.Errorf("expected state %s, got %s", to, next)
		}
	}
}

func TestAgentWorkflowStateMachine_Idempotency(t *testing.T) {
	// Any state transitioning to itself must succeed as an idempotent no-op
	for _, state := range AgentWorkflowStates() {
		if !CanTransitionAgentWorkflow(state, state) {
			t.Errorf("expected idempotent transition %s -> %s to be allowed", state, state)
		}
		next, err := TransitionAgentWorkflow(state, state)
		if err != nil {
			t.Errorf("unexpected error on idempotent self-transition %s -> %s: %v", state, state, err)
		}
		if next != state {
			t.Errorf("expected state %s, got %s", state, next)
		}
	}
}

func TestAgentWorkflowStateMachine_InvalidTransitions(t *testing.T) {
	invalidTransitions := [][2]AgentWorkflowState{
		{WorkflowCompleted, WorkflowInvestigating},
		{WorkflowCompleted, WorkflowPending},
		{WorkflowFailed, WorkflowPlanning},
		{WorkflowCancelled, WorkflowRemediating},
		{WorkflowBudgetExhausted, WorkflowContextBuilding},
		{WorkflowPolicyDenied, WorkflowInvestigating},
		{WorkflowAgentUnavailable, WorkflowVerified},
		{WorkflowPending, WorkflowCompleted},
		{WorkflowPending, WorkflowRemediating},
		{WorkflowInvestigating, WorkflowVerified},
	}

	for _, tc := range invalidTransitions {
		from, to := tc[0], tc[1]
		if CanTransitionAgentWorkflow(from, to) {
			t.Errorf("expected transition %s -> %s to be forbidden", from, to)
		}
		_, err := TransitionAgentWorkflow(from, to)
		if err == nil {
			t.Errorf("expected TransitionError for %s -> %s, got nil", from, to)
		}
	}
}

func TestAgentWorkflowStateMachine_TerminalStates(t *testing.T) {
	terminalStates := []AgentWorkflowState{
		WorkflowCompleted,
		WorkflowAgentUnavailable,
		WorkflowPolicyDenied,
		WorkflowBudgetExhausted,
		WorkflowCancelled,
		WorkflowFailed,
	}

	for _, s := range terminalStates {
		if !IsTerminalAgentWorkflow(s) {
			t.Errorf("expected state %s to be terminal", s)
		}
		// Must not transition to any other distinct state
		for _, target := range AgentWorkflowStates() {
			if target != s && CanTransitionAgentWorkflow(s, target) {
				t.Errorf("terminal state %s must not transition to %s", s, target)
			}
		}
	}
}

func TestAgentWorkflowState_DecoupledFromArtifactState(t *testing.T) {
	// Invariant: ArtifactState must remain strictly separated from AgentWorkflowState
	artifactStates := map[string]bool{
		"RECEIVED":    true,
		"VALIDATING":  true,
		"VALIDATED":   true,
		"QUARANTINED": true,
		"APPROVED":    true,
		"RELEASED":    true,
		"REJECTED":    true,
	}

	// Verify no agent workflow state collides with artifact states
	for _, ws := range AgentWorkflowStates() {
		if artifactStates[string(ws)] {
			t.Errorf("agent workflow state %s must not collide with artifact state", ws)
		}
	}
}
