package domain

import (
	"errors"
	"time"
)

// TenantID scopes every business record. It is a distinct type so a bare string
// cannot be passed where a tenant is required, and so an empty value is
// detectable at compile-adjacent boundaries rather than silently matching all
// rows.
type TenantID string

// ErrNoTenant is returned whenever a tenant scope is missing. A query without a
// tenant is a cross-tenant read waiting to happen, so it is refused rather than
// defaulted.
var ErrNoTenant = errors.New("tenant scope is required")

// Valid reports whether the tenant is usable as a scope.
func (t TenantID) Valid() bool { return string(t) != "" }

// Severity of a validation finding. Ordered: anything at or above SeverityError
// blocks release.
type Severity string

const (
	SeverityInfo     Severity = "INFO"
	SeverityWarning  Severity = "WARNING"
	SeverityError    Severity = "ERROR"
	SeverityCritical Severity = "CRITICAL"
	SeverityFatal    Severity = "FATAL"
)

// Blocking reports whether a finding of this severity disqualifies release.
func (s Severity) Blocking() bool {
	switch s {
	case SeverityError, SeverityCritical, SeverityFatal:
		return true
	default:
		return false
	}
}

// Finding is one deterministic rule result against an artifact.
//
// EvidenceRedacted holds a bounded excerpt, never a whole record: a complete
// NACHA line carries routing and account fields, and findings are returned by
// the API and shown in the UI.
type Finding struct {
	ID               int64
	TenantID         TenantID
	ArtifactID       int64
	RuleID           string
	RuleVersion      string
	Severity         Severity
	Message          string
	ByteOffset       int64
	RecordNumber     int
	EvidenceRedacted string
	CreatedAt        time.Time
}

// ValidationRun records one deterministic evaluation of one artifact.
type ValidationRun struct {
	ID             int64
	TenantID       TenantID
	ArtifactID     int64
	ParserName     string
	ParserVersion  string
	RulePackVer    string
	ParserOK       bool
	RecordsParsed  int
	StartedAt      time.Time
	CompletedAt    time.Time
	Findings       []Finding
	TotalDebitsMin int64 // integer minor units; never float
	TotalCreditsMi int64
}

// HasBlockingFinding reports whether any finding disqualifies release.
func (v *ValidationRun) HasBlockingFinding() bool {
	for _, f := range v.Findings {
		if f.Severity.Blocking() {
			return true
		}
	}
	return false
}

// Artifact is an immutable received file. Repair never mutates an artifact; it
// produces a new one whose DerivedFromID points here.
type Artifact struct {
	ID            int64
	TenantID      TenantID
	OccurrenceID  *int64
	Filename      string
	ObjectKey     string
	SizeBytes     int64
	SHA256        string
	State         ArtifactState
	DerivedFromID *int64
	ReceivedAt    time.Time
	UpdatedAt     time.Time
	Version       int // optimistic concurrency
}

// PolicyDecision binds a release decision to exactly what was decided upon.
//
// The three hashes matter: an approval that does not name the artifact hash,
// the validation run and the policy version can be replayed against different
// content, which is how the removed self-healing endpoint could re-ingest
// arbitrary bytes under a prior approval.
type PolicyDecision struct {
	ID              int64
	TenantID        TenantID
	ArtifactID      int64
	ValidationRunID int64
	PolicyVersion   string
	State           DecisionState
	Outcome         ArtifactState // VALIDATED or QUARANTINED
	ArtifactSHA256  string
	Reason          string
	DecidedAt       time.Time
}

// Approval is one authenticated human's decision. ActorID must come from
// verified session claims; a request field is not an identity.
type Approval struct {
	ID         int64
	TenantID   TenantID
	DecisionID int64
	ActorID    string
	Role       string
	Reason     string
	ApprovedAt time.Time
}

// ReleaseRequest carries everything the release rule must inspect.
type ReleaseRequest struct {
	Artifact        *Artifact
	Validation      *ValidationRun
	Decision        *PolicyDecision
	Approvals       []Approval
	RequireDual     bool
	RequireApproval bool
}

var (
	// ErrValidationMissing is returned when no successful validation exists.
	ErrValidationMissing = errors.New("release requires a completed validation run")
	// ErrDecisionMissing is returned when no versioned policy decision exists.
	ErrDecisionMissing = errors.New("release requires a versioned policy decision")
	// ErrApprovalRequired is returned when policy requires human approval.
	ErrApprovalRequired = errors.New("release requires an approval and none was recorded")
	// ErrDualControl is returned when two distinct approvers are required.
	ErrDualControl = errors.New("release requires two distinct authorized approvers")
	// ErrStaleApproval is returned when the approval no longer describes the
	// artifact, validation run, or policy version being released.
	ErrStaleApproval = errors.New("approval does not match the artifact being released")
)

// AuthorizeRelease decides whether an artifact may move to RELEASED.
//
// This is the single place that answers the question. It is deliberately strict
// and returns a typed reason for every refusal so the refusal is auditable.
func AuthorizeRelease(req ReleaseRequest) error {
	if req.Artifact == nil {
		return errors.New("release requires an artifact")
	}
	if !req.Artifact.TenantID.Valid() {
		return ErrNoTenant
	}

	// The state machine is the first gate: RECEIVED and QUARANTINED have no
	// edge to RELEASED at all.
	if !CanTransitionArtifact(req.Artifact.State, ArtifactReleased) {
		return &TransitionError{
			Machine: "artifact",
			From:    string(req.Artifact.State),
			To:      string(ArtifactReleased),
			Reason:  "only an APPROVED artifact may be released",
		}
	}

	if req.Validation == nil || req.Validation.CompletedAt.IsZero() {
		return ErrValidationMissing
	}
	if !req.Validation.ParserOK || req.Validation.RecordsParsed == 0 {
		return ErrValidationMissing
	}
	if req.Validation.HasBlockingFinding() {
		return errors.New("release refused: validation produced a blocking finding")
	}

	if req.Decision == nil {
		return ErrDecisionMissing
	}
	if req.Decision.PolicyVersion == "" {
		return ErrDecisionMissing
	}
	if req.Decision.State != DecisionApproved {
		return errors.New("release requires an APPROVED policy decision, got " + string(req.Decision.State))
	}
	// Bind the decision to this exact content and this exact validation run.
	if req.Decision.ArtifactSHA256 != req.Artifact.SHA256 {
		return ErrStaleApproval
	}
	if req.Decision.ArtifactID != req.Artifact.ID {
		return ErrStaleApproval
	}
	if req.Decision.ValidationRunID != req.Validation.ID {
		return ErrStaleApproval
	}

	if req.RequireApproval && len(req.Approvals) == 0 {
		return ErrApprovalRequired
	}

	if req.RequireDual {
		distinct := map[string]struct{}{}
		for _, a := range req.Approvals {
			if a.ActorID == "" {
				return errors.New("approval carries no actor identity")
			}
			if a.DecisionID != req.Decision.ID {
				return ErrStaleApproval
			}
			distinct[a.ActorID] = struct{}{}
		}
		if len(distinct) < 2 {
			return ErrDualControl
		}
	}

	return nil
}

// TransitionTo moves an artifact, refusing any edge the machine does not
// define. Callers must persist Version for optimistic concurrency; two
// concurrent callers cannot both finalize because the persisted compare-and-set
// on Version will fail for the loser.
func (a *Artifact) TransitionTo(next ArtifactState, at time.Time) error {
	if !a.TenantID.Valid() {
		return ErrNoTenant
	}
	if !CanTransitionArtifact(a.State, next) {
		return &TransitionError{Machine: "artifact", From: string(a.State), To: string(next)}
	}
	a.State = next
	a.UpdatedAt = at
	a.Version++
	return nil
}
