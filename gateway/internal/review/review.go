// Package review is human review and dual-control release.
//
// It exists because a validated artifact is not a released one. Validation says
// the file parses and satisfies the rules that could be checked; release is a
// decision by a person to send money. This package is the boundary between the
// two, and everything in it is built so the boundary cannot be crossed by
// accident, by one person acting alone, or by a decision that no longer
// describes what would be released.
//
// There is no self-healing. Nothing here repairs a file, re-runs validation to
// get a better answer, or resolves a finding. A human either accepts what the
// validator found or explicitly overrides it, on the record.
package review

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"sort"
	"strconv"
	"strings"
	"time"

	"sentinel-gateway/internal/domain"
)

// DigestVersion identifies the integrity-digest rules.
//
// Versioned for the same reason the ledger's canonical form is: changing how
// the digest is computed would make every existing approval look stale, which
// is indistinguishable from an attack. With a version, a decision computed
// under rules this build does not implement is reported as such.
const DigestVersion = "release-integrity/1"

// sep is the ASCII record separator, which cannot appear in any component.
//
// A printable separator would let two different decisions digest identically:
// "a|b" + "c" and "a" + "b|c" concatenate the same way, and the components here
// include rule ids and severities that a caller partly controls.
const sep = "\x1e"

var (
	// ErrNotFound is returned for an unknown decision and for one belonging to
	// another tenant, with identical text.
	ErrNotFound = errors.New("decision not found in this tenant")

	// ErrStale is returned when the artifact, findings, policy or contract have
	// changed since the decision was proposed.
	ErrStale = errors.New("the decision no longer describes what would be released")

	// ErrSelfApproval is returned when separation of duties refuses a vote.
	ErrSelfApproval = errors.New("separation of duties: this person cannot also approve")

	// ErrAlreadyVoted is returned when one person votes twice.
	ErrAlreadyVoted = errors.New("this person has already voted on this decision")

	// ErrNotApproved is returned when a release is attempted below the
	// threshold.
	ErrNotApproved = errors.New("this decision does not hold enough approvals to release")

	// ErrOverrideNotAllowed is returned when a tenant has disabled overrides.
	ErrOverrideNotAllowed = errors.New("this tenant does not permit manual override")

	// ErrConflict is returned when a concurrent action changed the decision
	// first.
	ErrConflict = errors.New("the decision changed while this action was being applied")
)

// Policy is a tenant's dual-control configuration.
//
// The zero value is not a usable policy; DefaultPolicy is the strict one that
// applies when a tenant has no row. A deployment that forgets to configure this
// is stricter than intended rather than weaker, which is the only safe
// direction for a control whose absence is invisible.
type Policy struct {
	TenantID           string `json:"-"`
	MinApprovals       int    `json:"minApprovals"`
	SeparationOfDuties bool   `json:"separationOfDuties"`
	OverrideAllowed    bool   `json:"overrideAllowed"`
}

// DefaultPolicy is two distinct approvers, separation of duties on, override
// permitted.
func DefaultPolicy() Policy {
	return Policy{MinApprovals: 2, SeparationOfDuties: true, OverrideAllowed: true}
}

// Validate refuses a policy that cannot be satisfied or that is not a control.
func (p Policy) Validate() error {
	if p.MinApprovals < 1 {
		return fmt.Errorf("a release policy needs at least one approval, got %d", p.MinApprovals)
	}
	if p.MinApprovals > 8 {
		// Not a security limit; a usability one. A threshold nobody can meet is
		// a threshold that gets overridden every time, which is worse than a
		// lower one that is actually followed.
		return fmt.Errorf("a release policy requiring %d approvals cannot realistically be met",
			p.MinApprovals)
	}
	return nil
}

// Finding is the part of a validation finding that binds a decision.
//
// Only the rule id and severity, never the evidence. Two things follow: the
// digest is stable against a change in how evidence is redacted, and a decision
// record carries nothing derived from file content.
type Finding struct {
	RuleID   string
	Severity string
}

// Subject is everything a decision is made about.
//
// The digest over this is what makes an approval expire when any of it changes.
// Computing one digest over all four rather than checking four fields
// separately is deliberate: four checks means one somebody forgets, and the one
// forgotten is the one that matters.
type Subject struct {
	ArtifactID      int64
	ArtifactSHA256  string
	ValidationRunID int64
	PolicyVersion   string
	ContractID      string
	ContractVersion string
	Outcome         string
	Findings        []Finding

	// What produced the findings.
	//
	// The rule pack is in the digest as well as the policy version, because
	// they change independently and for different reasons: the policy says
	// which severities block, the rule pack says what the severities are. An
	// approval given under one rule pack does not describe what the next one
	// would find, even when the policy is unchanged.
	ParserName      string
	ParserVersion   string
	RulePackVersion string

	// What the run measured. These are recorded on the run rather than folded
	// into the digest: a record count is a consequence of the bytes, which the
	// artifact hash already covers, so including it would add nothing and would
	// expire approvals whenever the counting changed.
	RecordsParsed     int
	TotalDebitsMinor  int64
	TotalCreditsMinor int64
}

// FindingsDigest is the digest of the findings alone.
//
// Kept separate so a report can say which part of a decision changed without
// recomputing everything -- "the findings changed" and "the policy changed"
// call for very different responses.
func (s Subject) FindingsDigest() string {
	sorted := make([]Finding, len(s.Findings))
	copy(sorted, s.Findings)
	// Sorted, because the order findings come back from a query is not part of
	// what a reviewer approved. An unsorted digest would expire approvals on a
	// query-plan change.
	sort.Slice(sorted, func(i, j int) bool {
		if sorted[i].RuleID != sorted[j].RuleID {
			return sorted[i].RuleID < sorted[j].RuleID
		}
		return sorted[i].Severity < sorted[j].Severity
	})

	h := sha256.New()
	h.Write([]byte(DigestVersion + sep + "findings"))
	for _, f := range sorted {
		h.Write([]byte(sep + f.RuleID + sep + f.Severity))
	}
	return hex.EncodeToString(h.Sum(nil))
}

// IntegrityDigest is the digest of everything a decision rests on.
//
// The components are joined explicitly rather than formatted, because a format
// string with nineteen verbs is a place where adding a field silently shifts
// every argument after it -- and the failure would be a digest that is stable
// across a change it was supposed to detect.
func (s Subject) IntegrityDigest() string {
	parts := []string{
		DigestVersion,
		strconv.FormatInt(s.ArtifactID, 10),
		s.ArtifactSHA256,
		strconv.FormatInt(s.ValidationRunID, 10),
		s.PolicyVersion,
		s.RulePackVersion,
		s.ContractID,
		s.ContractVersion,
		s.Outcome,
		s.FindingsDigest(),
	}
	h := sha256.New()
	h.Write([]byte(strings.Join(parts, sep)))
	return hex.EncodeToString(h.Sum(nil))
}

// BlockingRuleIDs returns the rules that prevent an unattended release.
func (s Subject) BlockingRuleIDs() []string {
	var out []string
	for _, f := range s.Findings {
		if strings.EqualFold(f.Severity, "BLOCKING") {
			out = append(out, f.RuleID)
		}
	}
	sort.Strings(out)
	return out
}

// Decision is a proposed release awaiting human judgement.
type Decision struct {
	ID              int64                `json:"id"`
	TenantID        string               `json:"-"`
	ArtifactID      int64                `json:"artifactId"`
	ValidationRunID int64                `json:"validationRunId"`
	State           domain.DecisionState `json:"state"`
	Outcome         string               `json:"outcome"`

	PolicyVersion   string `json:"policyVersion"`
	RulePackVersion string `json:"rulePackVersion"`
	ArtifactSHA256  string `json:"artifactSha256"`
	IntegrityDigest string `json:"integrityDigest"`
	FindingsDigest  string `json:"findingsDigest"`

	ProposedBy string    `json:"proposedBy"`
	ProposedAt time.Time `json:"proposedAt"`

	RequiredApprovals  int  `json:"requiredApprovals"`
	SeparationOfDuties bool `json:"separationOfDuties"`

	Votes []Vote `json:"votes"`

	ExpiredReason string     `json:"expiredReason,omitempty"`
	ReleasedAt    *time.Time `json:"releasedAt,omitempty"`
	ReleasedBy    string     `json:"releasedBy,omitempty"`

	Reason     string `json:"reason,omitempty"`
	RowVersion int64  `json:"rowVersion"`
}

// Vote is one reviewer's recorded judgement.
type Vote struct {
	ActorID string    `json:"actorId"`
	Role    string    `json:"role"`
	Choice  string    `json:"choice"` // APPROVE | REJECT
	Reason  string    `json:"reason"`
	At      time.Time `json:"at"`
	// Digest is what the reviewer saw. A vote cast against a state of the world
	// that no longer holds does not count.
	Digest string `json:"-"`
}

// ApprovalsHeld counts the votes that still count towards the threshold.
//
// A vote is counted when it approves, was cast against the decision's current
// integrity digest, and -- under separation of duties -- was not cast by the
// proposer. Each exclusion is a distinct control and each is applied here so
// there is one place that decides whether a release is permitted.
func (d Decision) ApprovalsHeld() int {
	seen := map[string]bool{}
	for _, v := range d.Votes {
		if v.Choice != "APPROVE" {
			continue
		}
		if v.Digest != "" && v.Digest != d.IntegrityDigest {
			continue
		}
		if d.SeparationOfDuties && v.ActorID == d.ProposedBy {
			continue
		}
		// Distinct people. The storage layer already refuses a second row for
		// one actor; this makes the rule true in the type as well, so a caller
		// assembling a Decision by hand cannot count one person twice.
		seen[v.ActorID] = true
	}
	return len(seen)
}

// Rejected reports whether any live vote refuses the release.
//
// One rejection is enough. A release requires agreement, so a reviewer who
// examined the file and refused it is not outvoted by two who accepted it --
// the disagreement itself is the signal.
func (d Decision) Rejected() bool {
	for _, v := range d.Votes {
		if v.Choice == "REJECT" && (v.Digest == "" || v.Digest == d.IntegrityDigest) {
			return true
		}
	}
	return false
}

// Releasable reports whether the decision may be released without an override.
func (d Decision) Releasable() bool {
	return d.State == domain.DecisionApproved &&
		!d.Rejected() &&
		d.ApprovalsHeld() >= d.RequiredApprovals
}

// CheckFresh compares a decision against the subject as it is now.
//
// Returns ErrStale with the part that changed named, because "the approval
// expired" is not actionable and "the findings changed since it was approved"
// is.
func (d Decision) CheckFresh(current Subject) error {
	if d.IntegrityDigest == current.IntegrityDigest() {
		return nil
	}
	switch {
	case d.ArtifactSHA256 != current.ArtifactSHA256:
		return fmt.Errorf("%w: the artifact's content changed", ErrStale)
	case d.FindingsDigest != current.FindingsDigest():
		return fmt.Errorf("%w: the validation findings changed", ErrStale)
	case d.PolicyVersion != current.PolicyVersion:
		return fmt.Errorf("%w: the release policy version changed from %s to %s",
			ErrStale, d.PolicyVersion, current.PolicyVersion)
	case d.RulePackVersion != "" && d.RulePackVersion != current.RulePackVersion:
		return fmt.Errorf("%w: the validation rule pack changed from %s to %s",
			ErrStale, d.RulePackVersion, current.RulePackVersion)
	case d.ValidationRunID != current.ValidationRunID:
		return fmt.Errorf("%w: the artifact was revalidated", ErrStale)
	default:
		return fmt.Errorf("%w: the governing contract or outcome changed", ErrStale)
	}
}

// CanVote reports whether an actor may cast a vote on this decision.
func (d Decision) CanVote(actorID string) error {
	if d.State != domain.DecisionProposed && d.State != domain.DecisionApproved {
		return fmt.Errorf("this decision is %s and cannot be voted on", d.State)
	}
	if d.SeparationOfDuties && actorID == d.ProposedBy {
		return ErrSelfApproval
	}
	for _, v := range d.Votes {
		if v.ActorID == actorID {
			return ErrAlreadyVoted
		}
	}
	return nil
}

// minOverrideReason is the shortest acceptable override justification.
//
// An override is the one action that releases a file the validator refused. A
// one-word reason -- "ok", "urgent" -- is not a justification, and a length
// floor is a crude but effective way to make someone write a sentence. It is
// not a substitute for review; it is a prompt to think.
const minOverrideReason = 20

// ValidateOverrideReason refuses a justification too short to be one.
func ValidateOverrideReason(reason string) error {
	if len(strings.TrimSpace(reason)) < minOverrideReason {
		return fmt.Errorf(
			"a manual override requires a reason of at least %d characters explaining why the "+
				"blocking findings are acceptable; this one is %d",
			minOverrideReason, len(strings.TrimSpace(reason)))
	}
	return nil
}
