package domain

import (
	"errors"
	"testing"
	"time"
)

var now = time.Date(2026, 8, 15, 12, 0, 0, 0, time.UTC)

// ---------------------------------------------------------------------------
// Exhaustive transition coverage
// ---------------------------------------------------------------------------

// Every state pair is enumerated and checked against the declared table. This
// catches an edge added to the table without being considered, which a
// hand-written list of illegal cases would silently miss.
func TestArtifactTransitionMatrixIsExhaustive(t *testing.T) {
	legal := map[ArtifactState]map[ArtifactState]bool{
		ArtifactReceived:    {ArtifactValidating: true, ArtifactQuarantined: true},
		ArtifactValidating:  {ArtifactValidated: true, ArtifactQuarantined: true},
		ArtifactValidated:   {ArtifactApproved: true, ArtifactQuarantined: true, ArtifactRejected: true},
		ArtifactQuarantined: {ArtifactRejected: true},
		ArtifactApproved:    {ArtifactReleased: true, ArtifactRejected: true},
		ArtifactReleased:    {},
		ArtifactRejected:    {},
	}

	states := ArtifactStates()
	for _, from := range states {
		for _, to := range states {
			want := legal[from][to]
			got := CanTransitionArtifact(from, to)
			if got != want {
				t.Errorf("CanTransitionArtifact(%s -> %s) = %v, want %v", from, to, got, want)
			}
		}
	}
}

// The specific defect this model exists to prevent.
func TestArtifactCannotReachReleasedExceptFromApproved(t *testing.T) {
	for _, from := range ArtifactStates() {
		if from == ArtifactApproved {
			continue
		}
		if CanTransitionArtifact(from, ArtifactReleased) {
			t.Errorf("%s -> RELEASED must not be a legal edge", from)
		}
	}
	if !CanTransitionArtifact(ArtifactApproved, ArtifactReleased) {
		t.Errorf("APPROVED -> RELEASED must be legal")
	}
}

func TestArtifactTransitionToRefusesIllegalEdges(t *testing.T) {
	cases := []struct {
		name    string
		from    ArtifactState
		to      ArtifactState
		wantErr bool
	}{
		{"received to validating", ArtifactReceived, ArtifactValidating, false},
		{"received straight to released", ArtifactReceived, ArtifactReleased, true},
		{"quarantined to released", ArtifactQuarantined, ArtifactReleased, true},
		{"quarantined to validated", ArtifactQuarantined, ArtifactValidated, true},
		{"validated to approved", ArtifactValidated, ArtifactApproved, false},
		{"validated straight to released", ArtifactValidated, ArtifactReleased, true},
		{"approved to released", ArtifactApproved, ArtifactReleased, false},
		{"released is terminal", ArtifactReleased, ArtifactRejected, true},
		{"released cannot reopen", ArtifactReleased, ArtifactValidating, true},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			a := &Artifact{TenantID: "t1", State: tc.from, SHA256: "abc"}
			err := a.TransitionTo(tc.to, now)
			if tc.wantErr && err == nil {
				t.Fatalf("expected %s -> %s to be refused, artifact is now %s", tc.from, tc.to, a.State)
			}
			if !tc.wantErr && err != nil {
				t.Fatalf("expected %s -> %s to be allowed, got %v", tc.from, tc.to, err)
			}
			if tc.wantErr && a.State != tc.from {
				t.Errorf("refused transition still mutated state to %s", a.State)
			}
		})
	}
}

func TestTransitionRequiresTenant(t *testing.T) {
	a := &Artifact{State: ArtifactReceived} // no tenant
	if err := a.TransitionTo(ArtifactValidating, now); !errors.Is(err, ErrNoTenant) {
		t.Errorf("expected ErrNoTenant, got %v", err)
	}

	e := &ExpectationOccurrence{State: ExpectationPending}
	if err := e.TransitionTo(ExpectationDue, now); !errors.Is(err, ErrNoTenant) {
		t.Errorf("expectation: expected ErrNoTenant, got %v", err)
	}

	j := &IngestionJob{State: JobQueued}
	if err := j.TransitionTo(JobLeased, now); !errors.Is(err, ErrNoTenant) {
		t.Errorf("job: expected ErrNoTenant, got %v", err)
	}
}

func TestExpectationReachesBreachedWithoutAnyArrival(t *testing.T) {
	// The missing-file path: time alone drives this, with no external event.
	e := &ExpectationOccurrence{TenantID: "t1", State: ExpectationPending}
	for _, next := range []ExpectationState{ExpectationDue, ExpectationOverdue, ExpectationBreached} {
		if err := e.TransitionTo(next, now); err != nil {
			t.Fatalf("PENDING..BREACHED must be reachable: %v", err)
		}
	}
	if e.State != ExpectationBreached {
		t.Fatalf("expected BREACHED, got %s", e.State)
	}
	// A late file may still be recorded as arriving after breach.
	if err := e.TransitionTo(ExpectationArrived, now); err != nil {
		t.Errorf("a file arriving after breach must be recordable: %v", err)
	}
}

func TestExpectationCannotSkipBackwards(t *testing.T) {
	e := &ExpectationOccurrence{TenantID: "t1", State: ExpectationArrived}
	if err := e.TransitionTo(ExpectationPending, now); err == nil {
		t.Errorf("ARRIVED is terminal; it must not return to PENDING")
	}
}

func TestJobRetryBudgetIsEnforced(t *testing.T) {
	j := &IngestionJob{TenantID: "t1", State: JobRetryable, AttemptCount: 3, MaxAttempts: 3}
	err := j.TransitionTo(JobQueued, now)
	if err == nil {
		t.Fatalf("a job past its retry budget must not requeue")
	}
	// It must still be able to die.
	if err := j.TransitionTo(JobDead, now); err != nil {
		t.Errorf("an exhausted job must be able to reach DEAD: %v", err)
	}
}

func TestJobPoisonReachesDeadWithoutBlocking(t *testing.T) {
	j := &IngestionJob{TenantID: "t1", State: JobQueued, MaxAttempts: 1}
	for _, next := range []JobState{JobLeased, JobRunning, JobDead} {
		if err := j.TransitionTo(next, now); err != nil {
			t.Fatalf("poison job path failed at %s: %v", next, err)
		}
	}
	if !IsTerminalJob(j.State) {
		t.Errorf("DEAD must be terminal")
	}
}

func TestDecisionTransitions(t *testing.T) {
	if !CanTransitionDecision(DecisionProposed, DecisionApproved) {
		t.Errorf("PROPOSED -> APPROVED must be legal")
	}
	if CanTransitionDecision(DecisionRejected, DecisionApproved) {
		t.Errorf("a rejected decision must not become approved")
	}
	if !CanTransitionDecision(DecisionApproved, DecisionExpired) {
		t.Errorf("an approval must be able to expire")
	}
	if CanTransitionDecision(DecisionExpired, DecisionApproved) {
		t.Errorf("an expired decision must not be revived")
	}
}

// ---------------------------------------------------------------------------
// Release authorization
// ---------------------------------------------------------------------------

func validRelease() ReleaseRequest {
	art := &Artifact{ID: 7, TenantID: "t1", State: ArtifactApproved, SHA256: "hash-abc"}
	run := &ValidationRun{
		ID: 11, TenantID: "t1", ArtifactID: 7,
		ParserOK: true, RecordsParsed: 120, CompletedAt: now,
	}
	dec := &PolicyDecision{
		ID: 21, TenantID: "t1", ArtifactID: 7, ValidationRunID: 11,
		PolicyVersion: "release-policy-v1", State: DecisionApproved,
		Outcome: ArtifactValidated, ArtifactSHA256: "hash-abc",
	}
	return ReleaseRequest{
		Artifact: art, Validation: run, Decision: dec,
		Approvals:       []Approval{{ID: 1, DecisionID: 21, ActorID: "alice"}},
		RequireApproval: true,
	}
}

func TestAuthorizeReleaseAcceptsAWellFormedRequest(t *testing.T) {
	if err := AuthorizeRelease(validRelease()); err != nil {
		t.Fatalf("a complete, approved release must be authorized: %v", err)
	}
}

func TestReleaseFromReceivedOrQuarantinedFails(t *testing.T) {
	for _, state := range []ArtifactState{ArtifactReceived, ArtifactQuarantined, ArtifactValidating, ArtifactValidated} {
		req := validRelease()
		req.Artifact.State = state
		err := AuthorizeRelease(req)
		if err == nil {
			t.Errorf("release from %s must be refused", state)
			continue
		}
		var te *TransitionError
		if !errors.As(err, &te) {
			t.Errorf("release from %s should fail as a TransitionError, got %T: %v", state, err, err)
		}
	}
}

func TestReleaseRequiresCompletedValidation(t *testing.T) {
	req := validRelease()
	req.Validation = nil
	if err := AuthorizeRelease(req); !errors.Is(err, ErrValidationMissing) {
		t.Errorf("expected ErrValidationMissing, got %v", err)
	}

	req = validRelease()
	req.Validation.ParserOK = false
	if err := AuthorizeRelease(req); !errors.Is(err, ErrValidationMissing) {
		t.Errorf("a failed parse must block release, got %v", err)
	}

	req = validRelease()
	req.Validation.RecordsParsed = 0
	if err := AuthorizeRelease(req); !errors.Is(err, ErrValidationMissing) {
		t.Errorf("zero parsed records must block release, got %v", err)
	}
}

func TestReleaseRefusedOnBlockingFinding(t *testing.T) {
	req := validRelease()
	req.Validation.Findings = []Finding{{Severity: SeverityError, RuleID: "ACH_0001"}}
	if err := AuthorizeRelease(req); err == nil {
		t.Errorf("a blocking finding must prevent release")
	}

	// Advisory findings must not.
	req = validRelease()
	req.Validation.Findings = []Finding{{Severity: SeverityWarning}, {Severity: SeverityInfo}}
	if err := AuthorizeRelease(req); err != nil {
		t.Errorf("advisory findings must not block release: %v", err)
	}
}

func TestReleaseRequiresVersionedPolicyDecision(t *testing.T) {
	req := validRelease()
	req.Decision = nil
	if err := AuthorizeRelease(req); !errors.Is(err, ErrDecisionMissing) {
		t.Errorf("expected ErrDecisionMissing, got %v", err)
	}

	req = validRelease()
	req.Decision.PolicyVersion = ""
	if err := AuthorizeRelease(req); !errors.Is(err, ErrDecisionMissing) {
		t.Errorf("an unversioned policy decision must not authorize release, got %v", err)
	}

	req = validRelease()
	req.Decision.State = DecisionProposed
	if err := AuthorizeRelease(req); err == nil {
		t.Errorf("a merely proposed decision must not authorize release")
	}
}

// An approval must describe the exact bytes being released.
func TestStaleApprovalCannotReleaseChangedContent(t *testing.T) {
	req := validRelease()
	req.Artifact.SHA256 = "hash-DIFFERENT"
	if err := AuthorizeRelease(req); !errors.Is(err, ErrStaleApproval) {
		t.Errorf("content changed after approval must be refused, got %v", err)
	}

	req = validRelease()
	req.Decision.ValidationRunID = 999
	if err := AuthorizeRelease(req); !errors.Is(err, ErrStaleApproval) {
		t.Errorf("a decision bound to another validation run must be refused, got %v", err)
	}
}

func TestApprovalCannotBeSkippedWhenPolicyRequiresIt(t *testing.T) {
	req := validRelease()
	req.RequireApproval = true
	req.Approvals = nil
	if err := AuthorizeRelease(req); !errors.Is(err, ErrApprovalRequired) {
		t.Errorf("expected ErrApprovalRequired, got %v", err)
	}
}

func TestDualControlRequiresTwoDistinctPeople(t *testing.T) {
	req := validRelease()
	req.RequireDual = true
	req.Approvals = []Approval{
		{ID: 1, DecisionID: 21, ActorID: "alice"},
		{ID: 2, DecisionID: 21, ActorID: "alice"}, // same person twice
	}
	if err := AuthorizeRelease(req); !errors.Is(err, ErrDualControl) {
		t.Errorf("one person approving twice must not satisfy dual control, got %v", err)
	}

	req.Approvals[1].ActorID = "bob"
	if err := AuthorizeRelease(req); err != nil {
		t.Errorf("two distinct approvers must satisfy dual control: %v", err)
	}
}

func TestApprovalWithoutActorIdentityIsRefused(t *testing.T) {
	req := validRelease()
	req.RequireDual = true
	req.Approvals = []Approval{
		{ID: 1, DecisionID: 21, ActorID: "alice"},
		{ID: 2, DecisionID: 21, ActorID: ""},
	}
	if err := AuthorizeRelease(req); err == nil {
		t.Errorf("an approval with no actor identity must be refused")
	}
}

func TestReleaseRequiresTenant(t *testing.T) {
	req := validRelease()
	req.Artifact.TenantID = ""
	if err := AuthorizeRelease(req); !errors.Is(err, ErrNoTenant) {
		t.Errorf("expected ErrNoTenant, got %v", err)
	}
}

// Optimistic concurrency: two callers holding the same loaded artifact both
// compute a transition, but the persisted compare-and-set on Version means only
// one can win. This asserts the version actually advances so the second write
// has something to conflict against.
func TestTransitionAdvancesVersionForOptimisticConcurrency(t *testing.T) {
	a := &Artifact{TenantID: "t1", State: ArtifactValidated, SHA256: "h", Version: 4}
	if err := a.TransitionTo(ArtifactApproved, now); err != nil {
		t.Fatal(err)
	}
	if a.Version != 5 {
		t.Errorf("expected version 5 after transition, got %d", a.Version)
	}

	// A refused transition must not consume a version either.
	before := a.Version
	_ = a.TransitionTo(ArtifactValidating, now)
	if a.Version != before {
		t.Errorf("a refused transition must not advance the version")
	}
}

func TestSeverityBlocking(t *testing.T) {
	for _, s := range []Severity{SeverityError, SeverityCritical, SeverityFatal} {
		if !s.Blocking() {
			t.Errorf("%s must block release", s)
		}
	}
	for _, s := range []Severity{SeverityInfo, SeverityWarning} {
		if s.Blocking() {
			t.Errorf("%s must not block release", s)
		}
	}
}
