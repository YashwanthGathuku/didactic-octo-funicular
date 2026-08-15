// Package domain holds the entities and state machines of Sentinel Flow.
//
// It has no database, HTTP, or filesystem dependency. Transitions are decided
// here and nowhere else, so a handler cannot invent a status by assigning a
// string, which is how a zero-byte file previously reached RELEASED.
package domain

import "fmt"

// ---------------------------------------------------------------------------
// Artifact
// ---------------------------------------------------------------------------

// ArtifactState is the lifecycle of a received financial file.
//
// RELEASED is reachable only from APPROVED, and APPROVED only from VALIDATED.
// There is no edge from RECEIVED or QUARANTINED to RELEASED, so the defect
// fixed in Prompt 01 is now unrepresentable rather than merely absent.
//
// Settlement is deliberately not a state. Money movement is performed by
// systems this product does not touch, so it has no state to model here.
type ArtifactState string

const (
	ArtifactReceived    ArtifactState = "RECEIVED"
	ArtifactValidating  ArtifactState = "VALIDATING"
	ArtifactValidated   ArtifactState = "VALIDATED"
	ArtifactQuarantined ArtifactState = "QUARANTINED"
	ArtifactApproved    ArtifactState = "APPROVED"
	ArtifactReleased    ArtifactState = "RELEASED"
	ArtifactRejected    ArtifactState = "REJECTED"
)

// ---------------------------------------------------------------------------
// Expectation occurrence
// ---------------------------------------------------------------------------

// ExpectationState tracks a file that is supposed to arrive.
//
// The occurrence exists before any arrival event, which is what makes a missing
// file detectable at all: BREACHED is reached by the passage of time, not by
// anything the partner does.
type ExpectationState string

const (
	ExpectationPending  ExpectationState = "PENDING"
	ExpectationDue      ExpectationState = "DUE"
	ExpectationOverdue  ExpectationState = "OVERDUE"
	ExpectationBreached ExpectationState = "BREACHED"
	ExpectationArrived  ExpectationState = "ARRIVED"
	ExpectationWaived   ExpectationState = "WAIVED"
)

// ---------------------------------------------------------------------------
// Ingestion job
// ---------------------------------------------------------------------------

// JobState is the lifecycle of a unit of durable work.
//
// RETRYABLE returns to QUEUED; DEAD is terminal and must not block the queue.
// Leases and heartbeats are Prompt 08; this defines the states they move
// between.
type JobState string

const (
	JobQueued    JobState = "QUEUED"
	JobLeased    JobState = "LEASED"
	JobRunning   JobState = "RUNNING"
	JobSucceeded JobState = "SUCCEEDED"
	JobRetryable JobState = "RETRYABLE"
	JobDead      JobState = "DEAD"
	JobCancelled JobState = "CANCELLED"
)

// ---------------------------------------------------------------------------
// Policy decision
// ---------------------------------------------------------------------------

// DecisionState is the lifecycle of a release decision.
//
// EXPIRED exists because an approval is bound to a specific artifact hash,
// validation run and policy version. If any of those change, the approval no
// longer describes what would be released.
type DecisionState string

const (
	DecisionProposed DecisionState = "PROPOSED"
	DecisionApproved DecisionState = "APPROVED"
	DecisionRejected DecisionState = "REJECTED"
	DecisionExpired  DecisionState = "EXPIRED"
)

// ---------------------------------------------------------------------------
// Transition tables
// ---------------------------------------------------------------------------

// Legal transitions. A state absent from a value set is terminal.
//
// These are data, not code, so the complete set of legal edges is reviewable in
// one place and the test suite can enumerate every illegal edge by difference
// rather than by a hand-written list that drifts.
var (
	artifactTransitions = map[ArtifactState][]ArtifactState{
		ArtifactReceived:    {ArtifactValidating, ArtifactQuarantined},
		ArtifactValidating:  {ArtifactValidated, ArtifactQuarantined},
		ArtifactValidated:   {ArtifactApproved, ArtifactQuarantined, ArtifactRejected},
		ArtifactQuarantined: {ArtifactRejected},
		ArtifactApproved:    {ArtifactReleased, ArtifactRejected},
		ArtifactReleased:    {},
		ArtifactRejected:    {},
	}

	expectationTransitions = map[ExpectationState][]ExpectationState{
		ExpectationPending:  {ExpectationDue, ExpectationArrived, ExpectationWaived},
		ExpectationDue:      {ExpectationOverdue, ExpectationArrived, ExpectationWaived},
		ExpectationOverdue:  {ExpectationBreached, ExpectationArrived, ExpectationWaived},
		ExpectationBreached: {ExpectationArrived, ExpectationWaived},
		ExpectationArrived:  {},
		ExpectationWaived:   {},
	}

	jobTransitions = map[JobState][]JobState{
		JobQueued:    {JobLeased, JobCancelled},
		JobLeased:    {JobRunning, JobRetryable, JobCancelled},
		JobRunning:   {JobSucceeded, JobRetryable, JobDead},
		JobRetryable: {JobQueued, JobDead},
		JobSucceeded: {},
		JobDead:      {},
		JobCancelled: {},
	}

	decisionTransitions = map[DecisionState][]DecisionState{
		DecisionProposed: {DecisionApproved, DecisionRejected, DecisionExpired},
		DecisionApproved: {DecisionExpired},
		DecisionRejected: {},
		DecisionExpired:  {},
	}
)

// TransitionError describes a refused transition. It names both states so the
// audit trail records what was attempted, not merely that something failed.
type TransitionError struct {
	Machine string
	From    string
	To      string
	Reason  string
}

func (e *TransitionError) Error() string {
	if e.Reason != "" {
		return fmt.Sprintf("illegal %s transition %s -> %s: %s", e.Machine, e.From, e.To, e.Reason)
	}
	return fmt.Sprintf("illegal %s transition %s -> %s", e.Machine, e.From, e.To)
}

func allowed[S ~string](table map[S][]S, from, to S) bool {
	for _, next := range table[from] {
		if next == to {
			return true
		}
	}
	return false
}

// ---------------------------------------------------------------------------
// Guarded transitions
// ---------------------------------------------------------------------------

// ArtifactStates returns every legal artifact state.
func ArtifactStates() []ArtifactState {
	out := make([]ArtifactState, 0, len(artifactTransitions))
	for s := range artifactTransitions {
		out = append(out, s)
	}
	return out
}

// ExpectationStates returns every legal expectation state.
func ExpectationStates() []ExpectationState {
	out := make([]ExpectationState, 0, len(expectationTransitions))
	for s := range expectationTransitions {
		out = append(out, s)
	}
	return out
}

// JobStates returns every legal job state.
func JobStates() []JobState {
	out := make([]JobState, 0, len(jobTransitions))
	for s := range jobTransitions {
		out = append(out, s)
	}
	return out
}

// DecisionStates returns every legal decision state.
func DecisionStates() []DecisionState {
	out := make([]DecisionState, 0, len(decisionTransitions))
	for s := range decisionTransitions {
		out = append(out, s)
	}
	return out
}

// CanTransitionArtifact reports whether the edge exists, without side effects.
func CanTransitionArtifact(from, to ArtifactState) bool {
	return allowed(artifactTransitions, from, to)
}

// CanTransitionExpectation reports whether the edge exists.
func CanTransitionExpectation(from, to ExpectationState) bool {
	return allowed(expectationTransitions, from, to)
}

// CanTransitionJob reports whether the edge exists.
func CanTransitionJob(from, to JobState) bool { return allowed(jobTransitions, from, to) }

// CanTransitionDecision reports whether the edge exists.
func CanTransitionDecision(from, to DecisionState) bool {
	return allowed(decisionTransitions, from, to)
}

// IsTerminalArtifact reports whether no transition leaves this state.
func IsTerminalArtifact(s ArtifactState) bool { return len(artifactTransitions[s]) == 0 }

// IsTerminalJob reports whether no transition leaves this state.
func IsTerminalJob(s JobState) bool { return len(jobTransitions[s]) == 0 }
