package review

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"sentinel-gateway/internal/domain"
)

// Releasing.
//
// The original artifact is never modified. A release is a state transition on
// the artifact's row plus an auditable record; the stored object is immutable
// and this package has no path that could write to it.

// ReleaseResult reports what a release did.
type ReleaseResult struct {
	DecisionID int64     `json:"decisionId"`
	ArtifactID int64     `json:"artifactId"`
	ReleasedAt time.Time `json:"releasedAt"`
	ReleasedBy string    `json:"releasedBy"`
	Overridden bool      `json:"overridden"`
}

// Release moves an approved artifact to RELEASED.
//
// Every guard runs inside one transaction with an optimistic update, so a
// release and a concurrent rejection produce one legal outcome rather than a
// released artifact whose decision says REJECTED.
func (s *Store) Release(ctx context.Context, tenantID, actorID string, decisionID int64, current Subject) (*ReleaseResult, error) {
	if strings.TrimSpace(actorID) == "" {
		return nil, errors.New("a release requires an authenticated actor")
	}

	d, err := s.Get(ctx, tenantID, decisionID)
	if err != nil {
		return nil, err
	}

	// Freshness first. An approval that no longer describes what would be
	// released is the failure this whole mechanism exists to prevent, so it is
	// checked before anything else and expires the decision rather than merely
	// refusing this attempt -- leaving it approved would let the next caller
	// try again and again.
	if err := d.CheckFresh(current); err != nil {
		if xerr := s.expire(ctx, tenantID, d, err.Error()); xerr != nil {
			return nil, xerr
		}
		return nil, err
	}
	if d.Rejected() {
		return nil, fmt.Errorf("this decision was rejected and cannot be released")
	}
	if !d.Releasable() {
		return nil, fmt.Errorf("%w: %d of %d approvals held",
			ErrNotApproved, d.ApprovalsHeld(), d.RequiredApprovals)
	}

	return s.finalise(ctx, tenantID, actorID, d, current, false, nil)
}

// OverrideRequest is a manual release past the threshold or past blocking
// findings.
type OverrideRequest struct {
	DecisionID int64
	Reason     string
	Role       string
}

// Override releases despite the controls, on the record.
//
// It never rewrites the validation result. The findings stay exactly as the
// validator wrote them; what is recorded is that a named human released the
// artifact anyway, what they bypassed, and why. A mechanism that "resolved" the
// findings instead would destroy the evidence that the override was needed --
// and the next reader would see a clean file.
func (s *Store) Override(ctx context.Context, tenantID, actorID string, current Subject, req OverrideRequest) (*ReleaseResult, error) {
	if strings.TrimSpace(actorID) == "" {
		return nil, errors.New("an override requires an authenticated actor")
	}
	if err := ValidateOverrideReason(req.Reason); err != nil {
		return nil, err
	}

	policy, err := s.Policy(ctx, tenantID)
	if err != nil {
		return nil, err
	}
	if !policy.OverrideAllowed {
		return nil, ErrOverrideNotAllowed
	}

	d, err := s.Get(ctx, tenantID, req.DecisionID)
	if err != nil {
		return nil, err
	}

	// Freshness still applies. An override is permission to release *this*
	// file despite its findings -- not permission to release whatever the
	// artifact has become since.
	if err := d.CheckFresh(current); err != nil {
		if xerr := s.expire(ctx, tenantID, d, err.Error()); xerr != nil {
			return nil, xerr
		}
		return nil, err
	}
	if d.State == domain.DecisionRejected {
		// An override may bypass an absent approval. It may not bypass a
		// reviewer who looked at the file and refused it -- that is not a
		// control being unmet, it is a person saying no.
		return nil, fmt.Errorf("this decision was rejected by a reviewer and cannot be overridden")
	}

	bypassed := s.describeBypass(d, current)
	return s.finalise(ctx, tenantID, actorID, d, current, true, &overrideRecord{
		reason:   req.Reason,
		role:     req.Role,
		bypassed: bypassed,
	})
}

type overrideRecord struct {
	reason   string
	role     string
	bypassed string
}

// describeBypass states what the override actually stepped around, in the
// system's terms, so a report does not have to infer it from the reason text.
func (s *Store) describeBypass(d *Decision, current Subject) string {
	var parts []string
	if held := d.ApprovalsHeld(); held < d.RequiredApprovals {
		parts = append(parts, fmt.Sprintf("dual control (%d of %d approvals)",
			held, d.RequiredApprovals))
	}
	if blocking := current.BlockingRuleIDs(); len(blocking) > 0 {
		parts = append(parts, fmt.Sprintf("blocking findings (%s)", strings.Join(blocking, ", ")))
	}
	if len(parts) == 0 {
		// An override recorded when nothing needed bypassing. It is still
		// recorded: an operator reaching for the override on a file that did
		// not need it is a signal about the workflow.
		return "nothing: the decision already satisfied its controls"
	}
	return strings.Join(parts, "; ")
}

// finalise performs the transition, the records and the event in one
// transaction.
func (s *Store) finalise(
	ctx context.Context, tenantID, actorID string,
	d *Decision, current Subject, overridden bool, ov *overrideRecord,
) (*ReleaseResult, error) {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, err
	}
	defer func() { _ = tx.Rollback() }()

	now := s.now().UTC()

	// The artifact's own state machine still governs. A release is only legal
	// from APPROVED, and the transition table from Prompt 03 is what says so --
	// this package does not get its own opinion about which states may release.
	var artifactState string
	if err := tx.QueryRowContext(ctx, s.rebind(
		`SELECT status FROM file_instances WHERE tenant_id = ? AND id = ?`),
		tenantID, d.ArtifactID).Scan(&artifactState); err != nil {
		return nil, err
	}

	from := domain.ArtifactState(artifactState)
	if from == domain.ArtifactReleased {
		return nil, fmt.Errorf("artifact %d is already released", d.ArtifactID)
	}
	// An override on a quarantined artifact has to move it through APPROVED
	// rather than jumping to RELEASED, because there is no such edge. The
	// intermediate transition is recorded, so the trail shows the artifact was
	// approved by override and then released rather than teleporting.
	path := []domain.ArtifactState{}
	switch from {
	case domain.ArtifactApproved:
		path = append(path, domain.ArtifactReleased)
	case domain.ArtifactValidated:
		path = append(path, domain.ArtifactApproved, domain.ArtifactReleased)
	case domain.ArtifactQuarantined:
		if !overridden {
			return nil, fmt.Errorf("artifact %d is quarantined; releasing it requires an explicit override",
				d.ArtifactID)
		}
		// QUARANTINED has no edge to APPROVED in the Prompt 03 table, and it
		// is deliberately not being added: a quarantined artifact that could
		// walk to RELEASED would make the quarantine advisory. The override is
		// recorded and the release is refused.
		return nil, fmt.Errorf(
			"artifact %d is quarantined and the state machine has no path from QUARANTINED to "+
				"RELEASED; quarantine is not overridable, only a validated artifact held by "+
				"dual control is", d.ArtifactID)
	default:
		return nil, &domain.TransitionError{
			Machine: "artifact", From: artifactState, To: string(domain.ArtifactReleased),
			Reason: "an artifact must be validated and approved before it can be released",
		}
	}

	prev := from
	for _, next := range path {
		if !domain.CanTransitionArtifact(prev, next) {
			return nil, &domain.TransitionError{
				Machine: "artifact", From: string(prev), To: string(next)}
		}
		res, err := tx.ExecContext(ctx, s.rebind(`
			UPDATE file_instances SET status = ?, updated_at = ?, row_version = row_version + 1
			WHERE tenant_id = ? AND id = ? AND status = ?`),
			string(next), now, tenantID, d.ArtifactID, string(prev))
		if err != nil {
			return nil, err
		}
		if n, _ := res.RowsAffected(); n == 0 {
			return nil, ErrConflict
		}

		reason := fmt.Sprintf("released under decision %d by %s", d.ID, actorID)
		if overridden {
			reason = fmt.Sprintf("released under decision %d by %s via manual override", d.ID, actorID)
		}
		if _, err := tx.ExecContext(ctx, s.rebind(`
			INSERT INTO status_history (tenant_id, object_type, object_id, from_state, to_state, actor_id, reason)
			VALUES (?, 'artifact', ?, ?, ?, ?, ?)`),
			tenantID, d.ArtifactID, string(prev), string(next), actorID, reason); err != nil {
			return nil, err
		}
		prev = next
	}

	res, err := tx.ExecContext(ctx, s.rebind(`
		UPDATE policy_decisions
		SET released_at = ?, released_by = ?, row_version = row_version + 1
		WHERE tenant_id = ? AND id = ? AND row_version = ? AND released_at IS NULL`),
		now, actorID, tenantID, d.ID, d.RowVersion)
	if err != nil {
		return nil, err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return nil, ErrConflict
	}

	action := ActionReleased
	payload := map[string]any{
		"decisionId":        d.ID,
		"artifactId":        d.ArtifactID,
		"validationRunId":   d.ValidationRunID,
		"policyVersion":     d.PolicyVersion,
		"artifactSha256":    d.ArtifactSHA256,
		"approvalsHeld":     d.ApprovalsHeld(),
		"requiredApprovals": d.RequiredApprovals,
		"overridden":        overridden,
	}

	if ov != nil {
		blocking := strings.Join(current.BlockingRuleIDs(), ",")
		if _, err := tx.ExecContext(ctx, s.rebind(`
			INSERT INTO release_overrides
				(tenant_id, decision_id, file_instance_id, actor_id, role, reason, bypassed,
				 approvals_held, approvals_required, blocking_rule_ids, integrity_digest, created_at)
			VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`),
			tenantID, d.ID, d.ArtifactID, actorID, ov.role, ov.reason, ov.bypassed,
			d.ApprovalsHeld(), d.RequiredApprovals, blocking, d.IntegrityDigest, now); err != nil {
			return nil, err
		}
		action = ActionOverride
		payload["bypassed"] = ov.bypassed
		payload["reason"] = ov.reason
		payload["blockingRuleIds"] = current.BlockingRuleIDs()
	}

	if err := s.emit(ctx, tx, Event{
		TenantID: tenantID, Action: action, Actor: actorID,
		DecisionID: d.ID, ArtifactID: d.ArtifactID, Payload: payload,
	}); err != nil {
		return nil, err
	}

	if err := tx.Commit(); err != nil {
		return nil, err
	}
	return &ReleaseResult{
		DecisionID: d.ID, ArtifactID: d.ArtifactID,
		ReleasedAt: now, ReleasedBy: actorID, Overridden: overridden,
	}, nil
}

// Overrides lists manual overrides, newest first.
//
// A separate report because that is what "separately reportable" means: an
// auditor asks for the overrides and gets them, without needing to know which
// flag on which table to look at.
func (s *Store) Overrides(ctx context.Context, tenantID string, limit int) ([]OverrideRecord, error) {
	if limit <= 0 || limit > 500 {
		limit = 100
	}
	rows, err := s.db.QueryContext(ctx, s.rebind(`
		SELECT id, decision_id, file_instance_id, actor_id, role, reason, bypassed,
		       approvals_held, approvals_required, blocking_rule_ids, created_at
		FROM release_overrides WHERE tenant_id = ?
		ORDER BY created_at DESC, id DESC LIMIT ?`), tenantID, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []OverrideRecord
	for rows.Next() {
		var r OverrideRecord
		var blocking string
		var at time.Time
		if err := rows.Scan(&r.ID, &r.DecisionID, &r.ArtifactID, &r.ActorID, &r.Role,
			&r.Reason, &r.Bypassed, &r.ApprovalsHeld, &r.ApprovalsRequired, &blocking, &at); err != nil {
			return nil, err
		}
		if blocking != "" {
			r.BlockingRuleIDs = strings.Split(blocking, ",")
		}
		r.CreatedAt = at.UTC()
		out = append(out, r)
	}
	return out, rows.Err()
}

// OverrideRecord is one manual override, as reported.
type OverrideRecord struct {
	ID                int64     `json:"id"`
	DecisionID        int64     `json:"decisionId"`
	ArtifactID        int64     `json:"artifactId"`
	ActorID           string    `json:"actorId"`
	Role              string    `json:"role"`
	Reason            string    `json:"reason"`
	Bypassed          string    `json:"bypassed"`
	ApprovalsHeld     int       `json:"approvalsHeld"`
	ApprovalsRequired int       `json:"approvalsRequired"`
	BlockingRuleIDs   []string  `json:"blockingRuleIds,omitempty"`
	CreatedAt         time.Time `json:"createdAt"`
}
