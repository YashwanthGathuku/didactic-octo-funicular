package domain

import (
	"fmt"
	"time"
)

// AgentWorkflowState represents the lifecycle of an AI agent orchestration workflow.
//
// CRITICAL ARCHITECTURAL INVARIANT:
// AgentWorkflowState is completely decoupled from ArtifactState.
// An agent cannot transition an artifact; it can only transition its own workflow.
type AgentWorkflowState string

const (
	WorkflowPending             AgentWorkflowState = "PENDING"
	WorkflowContextBuilding     AgentWorkflowState = "CONTEXT_BUILDING"
	WorkflowInvestigating       AgentWorkflowState = "INVESTIGATING"
	WorkflowPlanning            AgentWorkflowState = "PLANNING"
	WorkflowRemediating         AgentWorkflowState = "REMEDIATING"
	WorkflowValidatingCandidate AgentWorkflowState = "VALIDATING_CANDIDATE"
	WorkflowRetrying            AgentWorkflowState = "RETRYING"
	WorkflowVerified            AgentWorkflowState = "VERIFIED"
	WorkflowUnresolved          AgentWorkflowState = "UNRESOLVED"
	WorkflowHumanReview         AgentWorkflowState = "HUMAN_REVIEW"
	WorkflowCompleted           AgentWorkflowState = "COMPLETED"
	WorkflowAgentUnavailable    AgentWorkflowState = "AGENT_UNAVAILABLE"
	WorkflowPolicyDenied        AgentWorkflowState = "POLICY_DENIED"
	WorkflowBudgetExhausted     AgentWorkflowState = "BUDGET_EXHAUSTED"
	WorkflowCancelled           AgentWorkflowState = "CANCELLED"
	WorkflowFailed              AgentWorkflowState = "FAILED"
)

var agentWorkflowTransitions = map[AgentWorkflowState][]AgentWorkflowState{
	WorkflowPending: {
		WorkflowContextBuilding,
		WorkflowAgentUnavailable,
		WorkflowPolicyDenied,
		WorkflowCancelled,
	},
	WorkflowContextBuilding: {
		WorkflowInvestigating,
		WorkflowAgentUnavailable,
		WorkflowPolicyDenied,
		WorkflowBudgetExhausted,
		WorkflowFailed,
		WorkflowCancelled,
	},
	WorkflowInvestigating: {
		WorkflowPlanning,
		WorkflowUnresolved,
		WorkflowHumanReview,
		WorkflowAgentUnavailable,
		WorkflowBudgetExhausted,
		WorkflowFailed,
		WorkflowCancelled,
	},
	WorkflowPlanning: {
		WorkflowRemediating,
		WorkflowHumanReview,
		WorkflowUnresolved,
		WorkflowPolicyDenied,
		WorkflowBudgetExhausted,
		WorkflowFailed,
		WorkflowCancelled,
	},
	WorkflowRemediating: {
		WorkflowValidatingCandidate,
		WorkflowRetrying,
		WorkflowHumanReview,
		WorkflowFailed,
		WorkflowCancelled,
		WorkflowBudgetExhausted,
	},
	WorkflowValidatingCandidate: {
		WorkflowVerified,
		WorkflowRetrying,
		WorkflowHumanReview,
		WorkflowUnresolved,
		WorkflowFailed,
		WorkflowCancelled,
	},
	WorkflowRetrying: {
		WorkflowContextBuilding,
		WorkflowInvestigating,
		WorkflowRemediating,
		WorkflowBudgetExhausted,
		WorkflowFailed,
		WorkflowCancelled,
	},
	WorkflowVerified: {
		WorkflowHumanReview,
		WorkflowCompleted,
	},
	WorkflowUnresolved: {
		WorkflowHumanReview,
		WorkflowCompleted,
		WorkflowCancelled,
	},
	WorkflowHumanReview: {
		WorkflowCompleted,
		WorkflowRemediating,
		WorkflowCancelled,
		WorkflowPolicyDenied,
	},
	// Terminal states:
	WorkflowCompleted:        {},
	WorkflowAgentUnavailable: {},
	WorkflowPolicyDenied:     {},
	WorkflowBudgetExhausted:  {},
	WorkflowCancelled:        {},
	WorkflowFailed:           {},
}

// AgentWorkflowStates returns every legal agent workflow state.
func AgentWorkflowStates() []AgentWorkflowState {
	out := make([]AgentWorkflowState, 0, len(agentWorkflowTransitions))
	for s := range agentWorkflowTransitions {
		out = append(out, s)
	}
	return out
}

// CanTransitionAgentWorkflow reports whether the transition from -> to is valid.
// Transitions from a state to itself are allowed and treated as idempotent no-ops.
func CanTransitionAgentWorkflow(from, to AgentWorkflowState) bool {
	if from == to {
		return true // Idempotent self-transition
	}
	return allowed(agentWorkflowTransitions, from, to)
}

// TransitionAgentWorkflow validates and applies the state transition.
func TransitionAgentWorkflow(from, to AgentWorkflowState) (AgentWorkflowState, error) {
	if from == to {
		return to, nil // Idempotent no-op
	}
	if !CanTransitionAgentWorkflow(from, to) {
		return from, &TransitionError{
			Machine: "AgentWorkflow",
			From:    string(from),
			To:      string(to),
			Reason:  fmt.Sprintf("illegal transition from %s to %s", from, to),
		}
	}
	return to, nil
}

// IsTerminalAgentWorkflow reports whether no transition leaves this state.
func IsTerminalAgentWorkflow(s AgentWorkflowState) bool {
	return len(agentWorkflowTransitions[s]) == 0
}

// ---------------------------------------------------------------------------
// Agent Workflow Entities
// ---------------------------------------------------------------------------

// AgentWorkflow represents a persistent multi-agent orchestration session.
type AgentWorkflow struct {
	ID             string             `json:"id"`
	TenantID       string             `json:"tenantId"`
	IncidentID     int64              `json:"incidentId"`
	ArtifactID     int64              `json:"artifactId"`
	ArtifactSHA256 string             `json:"artifactSha256"`
	State          AgentWorkflowState `json:"state"`
	AgentName      string             `json:"agentName"`
	AgentVersion   string             `json:"agentVersion"`
	WorkflowType   string             `json:"workflowType"`
	CorrelationID  string             `json:"correlationId"`
	TraceID        string             `json:"traceId,omitempty"`
	RowVersion     int                `json:"rowVersion"`
	ErrorDetail    string             `json:"errorDetail,omitempty"`
	CreatedAt      time.Time          `json:"createdAt"`
	UpdatedAt      time.Time          `json:"updatedAt"`
	StartedAt      *time.Time         `json:"startedAt,omitempty"`
	CompletedAt    *time.Time         `json:"completedAt,omitempty"`
}

// AgentWorkflowEvent records an immutable, idempotent state transition event.
type AgentWorkflowEvent struct {
	ID             string             `json:"id"`
	WorkflowID     string             `json:"workflowId"`
	TenantID       string             `json:"tenantId"`
	IdempotencyKey string             `json:"idempotencyKey"`
	EventType      string             `json:"eventType"`
	StateFrom      AgentWorkflowState `json:"stateFrom"`
	StateTo        AgentWorkflowState `json:"stateTo"`
	RowVersion     int                `json:"rowVersion"`
	Payload        string             `json:"payload"`
	CreatedAt      time.Time          `json:"createdAt"`
}

// AgentRun represents a single agent execution invocation within a workflow.
type AgentRun struct {
	ID                    string     `json:"id"`
	WorkflowID            string     `json:"workflowId"`
	TenantID              string     `json:"tenantId"`
	AgentName             string     `json:"agentName"`
	AgentVersion          string     `json:"agentVersion"`
	Provider              *string    `json:"provider,omitempty"`
	ModelName             *string    `json:"modelName,omitempty"`
	ModelVersion          *string    `json:"modelVersion,omitempty"`
	Status                string     `json:"status"` // RUNNING, COMPLETED, FAILED, CANCELLED
	InputTokens           int        `json:"inputTokens"`
	OutputTokens          int        `json:"outputTokens"`
	LatencyMs             int        `json:"latencyMs"`
	EstimatedCostMicroUSD int64      `json:"estimatedCostMicroUSD"`
	PricingVersion        *string    `json:"pricingVersion,omitempty"`
	ErrorMessage          string     `json:"errorMessage,omitempty"`
	StartedAt             time.Time  `json:"startedAt"`
	CompletedAt           *time.Time `json:"completedAt,omitempty"`
}

// StepType describes structured, auditable agent execution step types.
// (Strict invariant: Chain-of-thought is never stored or requested).
type StepType string

const (
	StepContextBuild     StepType = "CONTEXT_BUILD"
	StepModelInvocation  StepType = "MODEL_INVOCATION"
	StepDecision         StepType = "DECISION"
	StepToolRequest      StepType = "TOOL_REQUEST"
	StepToolResult       StepType = "TOOL_RESULT"
	StepHandoff          StepType = "HANDOFF"
	StepPolicyCheck      StepType = "POLICY_CHECK"
	StepValidation       StepType = "VALIDATION"
	StepVerification     StepType = "VERIFICATION"
	StepHumanReview      StepType = "HUMAN_REVIEW"
)

// AgentStep records a discrete structured execution step without CoT.
type AgentStep struct {
	ID                     string             `json:"id"`
	RunID                  string             `json:"runId"`
	WorkflowID             string             `json:"workflowId"`
	TenantID               string             `json:"tenantId"`
	StepNumber             int                `json:"stepNumber"`
	StepType               StepType           `json:"stepType"`
	StateFrom              AgentWorkflowState `json:"stateFrom"`
	StateTo                AgentWorkflowState `json:"stateTo"`
	DecisionPayload        string             `json:"decisionPayload,omitempty"`
	AuthorizedEvidenceRefs []string           `json:"evidenceRefs,omitempty"`
	StepStatus             string             `json:"stepStatus"`
	StepHash               string             `json:"stepHash,omitempty"`
	LatencyMs              int                `json:"latencyMs"`
	CreatedAt              time.Time          `json:"createdAt"`
}

// ToolScope defines the permission boundary of a tool.
type ToolScope string

const (
	ToolScopeRead  ToolScope = "READ"
	ToolScopeWrite ToolScope = "WRITE"
)

// AgentToolCall records an authorized tool execution by an agent.
type AgentToolCall struct {
	ID             string    `json:"id"`
	StepID         string    `json:"stepId"`
	WorkflowID     string    `json:"workflowId"`
	TenantID       string    `json:"tenantId"`
	ToolName       string    `json:"toolName"`
	ToolScope      ToolScope `json:"toolScope"`
	InputRedacted  string    `json:"inputRedacted"`
	OutputRedacted string    `json:"outputRedacted"`
	IsError        bool      `json:"isError"`
	LatencyMs      int       `json:"latencyMs"`
	ExecutedAt     time.Time `json:"executedAt"`
}

// VerificationAttestation captures an independent verification verdict.
type VerificationAttestation struct {
	ID                    string    `json:"id"`
	WorkflowID            string    `json:"workflowId"`
	TenantID              string    `json:"tenantId"`
	VerifierAgent         string    `json:"verifierAgent"`
	CandidateArtifactID   *int64    `json:"candidateArtifactId,omitempty"`
	CandidateSHA256       string    `json:"candidateSha256"`
	FindingsCount         int       `json:"findingsCount"`
	BlockingFindingsCount int       `json:"blockingFindingsCount"`
	Status                string    `json:"status"` // CONFIRMED, DISPUTED, PARTIAL
	AttestationDigest     string    `json:"attestationDigest"`
	CreatedAt             time.Time `json:"createdAt"`
}
