package schedule

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"sentinel-gateway/internal/domain"
)

// What a breach means.
//
// Prompt 10 detected breaches and stopped there: the transition was recorded
// and nothing else happened. The owner and escalation policy every contract
// version carries were stored and never read, so the system knew who to tell
// and did not tell them.

// IncidentTypeMissingFile is the incident raised when an expectation breaches.
const IncidentTypeMissingFile = "MISSING_FILE"

// NotificationKindBreach is the notification intent written for a breach.
const NotificationKindBreach = "EXPECTATION_BREACHED"

// unassignedOwner is recorded when a contract version carries no owner.
//
// Contract validation requires one, so this can only be reached by a row that
// predates that validation or was written by hand. The incident is still
// raised, addressed to nobody, and says so -- dropping it would mean the
// deployments with the worst configuration are also the ones that get no
// alerts.
const unassignedOwner = "unassigned"

// Breach is the fact handed to an escalator.
//
// It carries the terms in force at the moment of the breach rather than
// identifiers to look them up by. An escalation that re-reads the contract
// would describe whatever the contract says when the alert is delivered, which
// is not necessarily what it said when the deadline passed.
type Breach struct {
	TenantID     string
	OccurrenceID int64
	IncidentID   int64
	ContractID   int64
	VersionID    int64
	Version      int
	FeedID       string
	BusinessDate Date
	DueAt        time.Time
	BreachedAt   time.Time
	DueLocal     string
	Timezone     string

	OwnerSubject       string
	EscalationPolicyID string
	Summary            string
}

// Escalator is notified of a breach inside the transaction that recorded it.
//
// The interface is narrow and takes the transaction on purpose. An escalator
// that ran after the commit could be lost to a crash in between, and the
// failure would be silent in exactly the way that matters -- the breach would
// be on record, and nobody would have been told.
//
// `internal/schedule` deliberately does not import `internal/jobs`: what a
// breach means beyond "open an incident and record who to tell" is the
// application's decision, not the scheduler's.
type Escalator interface {
	Breached(ctx context.Context, tx *sql.Tx, b Breach) error
}

// SetEscalator installs the escalation hook. Passing nil disables it, which
// leaves the incident and notification intent still written -- those are part
// of the domain, not an optional integration.
func (s *Store) SetEscalator(e Escalator) { s.escalator = e }

// escalate opens the incident, records the notification intent, and invokes the
// escalator, all inside the transition's transaction.
func (s *Store) escalate(ctx context.Context, tx *sql.Tx, tenantID string, occurrenceID int64, at time.Time) error {
	b, err := s.breachFacts(ctx, tx, tenantID, occurrenceID, at)
	if err != nil {
		return err
	}

	incidentID, created, err := s.openIncident(ctx, tx, b)
	if err != nil {
		return err
	}
	if !created {
		// The incident already exists, so this occurrence has been escalated
		// before. Writing a second notification would page someone twice for
		// one missing file, which is how an alert channel stops being read.
		return nil
	}
	b.IncidentID = incidentID

	if err := s.recordNotificationIntent(ctx, tx, b); err != nil {
		return err
	}
	if s.escalator == nil {
		return nil
	}
	return s.escalator.Breached(ctx, tx, b)
}

// breachFacts assembles the terms that were in force when the deadline passed.
func (s *Store) breachFacts(ctx context.Context, tx *sql.Tx, tenantID string, occurrenceID int64, at time.Time) (Breach, error) {
	b := Breach{TenantID: tenantID, OccurrenceID: occurrenceID, BreachedAt: at.UTC()}

	var (
		business  sql.NullTime
		versionID sql.NullInt64
		version   sql.NullInt64
		feedID    sql.NullString
		owner     sql.NullString
		policy    sql.NullString
		dueLocal  string
		zone      string
	)
	err := tx.QueryRowContext(ctx, s.rebind(`
		SELECT e.contract_id, e.business_date, e.expected_delivery_start,
		       e.due_local, e.timezone,
		       v.id, v.version, v.feed_id, v.owner_subject, v.escalation_policy_id
		FROM expectations e
		LEFT JOIN file_contract_versions v
		       ON v.id = e.contract_version_id AND v.tenant_id = e.tenant_id
		WHERE e.tenant_id = ? AND e.id = ?`), tenantID, occurrenceID).Scan(
		&b.ContractID, &business, &b.DueAt, &dueLocal, &zone,
		&versionID, &version, &feedID, &owner, &policy)
	if err != nil {
		return b, fmt.Errorf("read the breached occurrence %d: %w", occurrenceID, err)
	}

	if business.Valid {
		b.BusinessDate = DateOf(business.Time, time.UTC)
	}
	b.DueAt = b.DueAt.UTC()
	b.DueLocal, b.Timezone = dueLocal, zone
	b.VersionID = versionID.Int64
	b.Version = int(version.Int64)
	b.FeedID = feedID.String
	if b.FeedID == "" {
		b.FeedID = fmt.Sprintf("contract-%d", b.ContractID)
	}

	b.OwnerSubject = strings.TrimSpace(owner.String)
	if b.OwnerSubject == "" {
		b.OwnerSubject = unassignedOwner
	}
	b.EscalationPolicyID = strings.TrimSpace(policy.String)

	local := b.DueLocal
	if local != "" && b.Timezone != "" {
		local = fmt.Sprintf("%s %s", b.DueLocal, b.Timezone)
	} else {
		local = b.DueAt.Format(time.RFC3339)
	}
	b.Summary = fmt.Sprintf(
		"feed %s did not deliver its file for business date %s, due %s; the grace period and "+
			"breach threshold both elapsed with no matching arrival",
		b.FeedID, b.BusinessDate, local)
	return b, nil
}

// openIncident raises one incident per occurrence, reporting whether it created
// it.
//
// Idempotent through the unique index from migration 009 rather than a
// read-then-write: two schedulers can reach this concurrently, and a check
// followed by an insert would let both pass the check.
//
// The conflict target repeats the index's WHERE predicate because the index is
// partial -- `expectation_id` is nullable -- and both databases refuse to match
// a partial index unless the predicate is restated.
func (s *Store) openIncident(ctx context.Context, tx *sql.Tx, b Breach) (int64, bool, error) {
	var versionID any
	if b.VersionID != 0 {
		versionID = b.VersionID
	}

	if s.dialect == dialectPostgres {
		var id int64
		err := tx.QueryRowContext(ctx, s.rebind(`
			INSERT INTO incidents
				(tenant_id, expectation_id, type, severity, status, contract_version_id,
				 summary, owner_subject, escalation_policy_id, created_at, updated_at)
			VALUES (?, ?, ?, 'HIGH', 'OPEN', ?, ?, ?, ?, ?, ?)
			ON CONFLICT (tenant_id, expectation_id, type) WHERE expectation_id IS NOT NULL
			DO NOTHING
			RETURNING id`),
			b.TenantID, b.OccurrenceID, IncidentTypeMissingFile, versionID,
			b.Summary, b.OwnerSubject, b.EscalationPolicyID, b.BreachedAt, b.BreachedAt).Scan(&id)
		if errors.Is(err, sql.ErrNoRows) {
			return 0, false, nil
		}
		return id, err == nil, err
	}

	res, err := tx.ExecContext(ctx, `
		INSERT INTO incidents
			(tenant_id, expectation_id, type, severity, status, contract_version_id,
			 summary, owner_subject, escalation_policy_id, created_at, updated_at)
		VALUES (?, ?, ?, 'HIGH', 'OPEN', ?, ?, ?, ?, ?, ?)
		ON CONFLICT (tenant_id, expectation_id, type) WHERE expectation_id IS NOT NULL
		DO NOTHING`,
		b.TenantID, b.OccurrenceID, IncidentTypeMissingFile, versionID,
		b.Summary, b.OwnerSubject, b.EscalationPolicyID, b.BreachedAt, b.BreachedAt)
	if err != nil {
		return 0, false, err
	}
	n, err := res.RowsAffected()
	if err != nil || n == 0 {
		return 0, false, err
	}
	id, err := res.LastInsertId()
	return id, true, err
}

// recordNotificationIntent writes the obligation to tell someone.
//
// The payload carries identifiers and the deadline, never a filename pattern,
// a record, or anything derived from file content. A notification is the most
// widely distributed artifact this system produces, so it is the worst place
// for anything sensitive to appear.
func (s *Store) recordNotificationIntent(ctx context.Context, tx *sql.Tx, b Breach) error {
	payload, err := json.Marshal(map[string]any{
		"incidentId":         b.IncidentID,
		"expectationId":      b.OccurrenceID,
		"contractId":         b.ContractID,
		"contractVersion":    b.Version,
		"feedId":             b.FeedID,
		"businessDate":       b.BusinessDate.String(),
		"dueAt":              b.DueAt.Format(time.RFC3339),
		"dueLocal":           b.DueLocal,
		"timezone":           b.Timezone,
		"breachedAt":         b.BreachedAt.Format(time.RFC3339),
		"summary":            b.Summary,
		"escalationPolicyId": b.EscalationPolicyID,
	})
	if err != nil {
		return err
	}

	_, err = tx.ExecContext(ctx, s.rebind(`
		INSERT INTO notification_intents
			(tenant_id, kind, subject_type, subject_id, payload, dedupe_key,
			 recipient, escalation_policy_id, created_at)
		VALUES (?, ?, 'expectation', ?, ?, ?, ?, ?, ?)
		ON CONFLICT (tenant_id, dedupe_key) WHERE dedupe_key <> '' DO NOTHING`),
		b.TenantID, NotificationKindBreach, b.OccurrenceID, string(payload),
		fmt.Sprintf("breach-%d", b.OccurrenceID),
		b.OwnerSubject, b.EscalationPolicyID, b.BreachedAt)
	return err
}

// ---------------------------------------------------------------------------
// Resolving an ambiguous arrival
// ---------------------------------------------------------------------------

// ErrCandidateResolved is returned when a candidate has already been decided.
var ErrCandidateResolved = errors.New("this match candidate has already been resolved")

// ResolveCandidate records a human's decision about an ambiguous arrival.
//
// Accepting attributes the artifact to the occurrence and marks every other
// candidate for that artifact rejected: the file satisfied one expectation, so
// the remaining questions are answered by the same decision. Rejecting leaves
// the occurrence ageing, which is correct -- the file was not this feed's.
//
// The actor is required. A candidate that was resolved by nobody is a state
// change with no accountable party, and this is the one place in the scheduler
// where a person overrides what the system could not determine.
func (s *Store) ResolveCandidate(ctx context.Context, tenantID string, candidateID int64, accept bool, actor, reason string) error {
	if strings.TrimSpace(actor) == "" {
		return errors.New("resolving a match candidate requires an actor")
	}
	if strings.TrimSpace(reason) == "" {
		return errors.New("resolving a match candidate requires a reason")
	}

	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()

	now := s.Now()
	var (
		occurrenceID int64
		artifactID   int64
		filename     string
		resolution   string
	)
	err = tx.QueryRowContext(ctx, s.rebind(`
		SELECT expectation_id, file_instance_id, filename, resolution
		FROM expectation_match_candidates
		WHERE tenant_id = ? AND id = ?`), tenantID, candidateID).
		Scan(&occurrenceID, &artifactID, &filename, &resolution)
	if errors.Is(err, sql.ErrNoRows) {
		return fmt.Errorf("tenant %s has no match candidate %d", tenantID, candidateID)
	}
	if err != nil {
		return err
	}
	if resolution != "REVIEW_REQUIRED" {
		return fmt.Errorf("candidate %d is %s: %w", candidateID, resolution, ErrCandidateResolved)
	}

	decision := "REJECTED"
	if accept {
		decision = "ACCEPTED"
	}
	if _, err := tx.ExecContext(ctx, s.rebind(`
		UPDATE expectation_match_candidates
		SET resolution = ?, resolved_by = ?, resolved_at = ?
		WHERE tenant_id = ? AND id = ? AND resolution = 'REVIEW_REQUIRED'`),
		decision, actor, now, tenantID, candidateID); err != nil {
		return err
	}

	if accept {
		var state string
		var rowVersion int64
		if err := tx.QueryRowContext(ctx, s.rebind(
			`SELECT status, row_version FROM expectations WHERE tenant_id = ? AND id = ?`),
			tenantID, occurrenceID).Scan(&state, &rowVersion); err != nil {
			return err
		}
		from := domain.ExpectationState(state)
		if from == domain.ExpectationArrived {
			return fmt.Errorf("occurrence %d is already satisfied", occurrenceID)
		}
		moved, err := s.attribute(ctx, tx, tenantID, occurrenceID, from, rowVersion, artifactID, now,
			fmt.Sprintf("%s attributed %q to this expectation on review: %s", actor, filename, reason))
		if err != nil {
			return err
		}
		if !moved {
			return fmt.Errorf("occurrence %d changed while the review was being resolved", occurrenceID)
		}

		// The same artifact cannot also satisfy the other candidates it
		// matched, so those questions are answered too. Leaving them open
		// would put a resolved decision back in the reviewer's queue.
		if _, err := tx.ExecContext(ctx, s.rebind(`
			UPDATE expectation_match_candidates
			SET resolution = 'REJECTED', resolved_by = ?, resolved_at = ?
			WHERE tenant_id = ? AND file_instance_id = ? AND id <> ? AND resolution = 'REVIEW_REQUIRED'`),
			actor, now, tenantID, artifactID, candidateID); err != nil {
			return err
		}
	}

	// Clear the review flag from any occurrence with no open candidates left.
	// The flag means "a human still has to decide", so it must not outlive the
	// decision.
	if _, err := tx.ExecContext(ctx, s.rebind(`
		UPDATE expectations SET review_required = 0, updated_at = ?
		WHERE tenant_id = ? AND review_required = 1
		  AND NOT EXISTS (
		      SELECT 1 FROM expectation_match_candidates c
		      WHERE c.tenant_id = expectations.tenant_id
		        AND c.expectation_id = expectations.id
		        AND c.resolution = 'REVIEW_REQUIRED')`),
		now, tenantID); err != nil {
		return err
	}

	return tx.Commit()
}

// ---------------------------------------------------------------------------
// Waiving an occurrence
// ---------------------------------------------------------------------------

// Waive records that an occurrence should not have been expected.
//
// WAIVED was modelled by Prompt 03 and unreachable until now: an occurrence
// everyone agreed was wrong still breached, and the only remedy was editing the
// row by hand -- which leaves no record of who decided or why.
//
// An actor and a reason are both required. Waiving is the one operation that
// makes a missing-file alert go away without a file arriving, so it is the one
// that most needs an accountable party attached.
func (s *Store) Waive(ctx context.Context, tenantID string, occurrenceID int64, actor, reason string) error {
	if strings.TrimSpace(actor) == "" {
		return errors.New("waiving an expectation requires an actor")
	}
	if strings.TrimSpace(reason) == "" {
		return errors.New("waiving an expectation requires a reason")
	}

	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()

	var state string
	var rowVersion int64
	if err := tx.QueryRowContext(ctx, s.rebind(
		`SELECT status, row_version FROM expectations WHERE tenant_id = ? AND id = ?`),
		tenantID, occurrenceID).Scan(&state, &rowVersion); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return fmt.Errorf("tenant %s has no expectation %d", tenantID, occurrenceID)
		}
		return err
	}

	from := domain.ExpectationState(state)
	if !domain.CanTransitionExpectation(from, domain.ExpectationWaived) {
		return &domain.TransitionError{
			Machine: "expectation", From: state, To: string(domain.ExpectationWaived),
			Reason: "an occurrence that has already arrived or been waived cannot be waived",
		}
	}

	now := s.Now()
	res, err := tx.ExecContext(ctx, s.rebind(`
		UPDATE expectations
		SET status = 'WAIVED', waived_by = ?, waived_reason = ?, waived_at = ?,
		    updated_at = ?, row_version = row_version + 1
		WHERE id = ? AND tenant_id = ? AND status = ? AND row_version = ?`),
		actor, reason, now, now, occurrenceID, tenantID, state, rowVersion)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return fmt.Errorf("occurrence %d changed while it was being waived", occurrenceID)
	}

	if _, err := tx.ExecContext(ctx, s.rebind(`
		INSERT INTO status_history (tenant_id, object_type, object_id, from_state, to_state, actor_id, reason)
		VALUES (?, 'expectation', ?, ?, 'WAIVED', ?, ?)`),
		tenantID, occurrenceID, state, actor, reason); err != nil {
		return err
	}

	// A waived occurrence's incident is resolved with it. Leaving it open would
	// keep an alert on the board for a file the tenant has decided not to
	// expect.
	if _, err := tx.ExecContext(ctx, s.rebind(`
		UPDATE incidents
		SET status = 'RESOLVED', resolved_at = ?, resolved_by = ?, updated_at = ?
		WHERE tenant_id = ? AND expectation_id = ? AND status IN ('OPEN','INVESTIGATING')`),
		now, actor, now, tenantID, occurrenceID); err != nil {
		return err
	}

	return tx.Commit()
}

// OpenIncidents counts unresolved incidents, for the operations view and tests.
func (s *Store) OpenIncidents(ctx context.Context, tenantID string) (int, error) {
	var n int
	err := s.db.QueryRowContext(ctx, s.rebind(`
		SELECT COUNT(*) FROM incidents
		WHERE tenant_id = ? AND status IN ('OPEN','INVESTIGATING')`), tenantID).Scan(&n)
	return n, err
}

// PendingNotifications returns undelivered intents, oldest first.
//
// Exposed so a dispatcher can be built against it and so tests can assert that
// a breach produced exactly one obligation to tell someone.
func (s *Store) PendingNotifications(ctx context.Context, tenantID string, limit int) ([]Notification, error) {
	if limit <= 0 {
		limit = 100
	}
	rows, err := s.db.QueryContext(ctx, s.rebind(`
		SELECT id, kind, subject_id, recipient, escalation_policy_id, payload, created_at
		FROM notification_intents
		WHERE tenant_id = ? AND delivered_at IS NULL
		ORDER BY created_at, id
		LIMIT ?`), tenantID, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []Notification
	for rows.Next() {
		var n Notification
		if err := rows.Scan(&n.ID, &n.Kind, &n.SubjectID, &n.Recipient,
			&n.EscalationPolicyID, &n.Payload, &n.CreatedAt); err != nil {
			return nil, err
		}
		n.TenantID = tenantID
		out = append(out, n)
	}
	return out, rows.Err()
}

// Notification is one undelivered obligation to tell someone something.
type Notification struct {
	ID                 int64
	TenantID           string
	Kind               string
	SubjectID          int64
	Recipient          string
	EscalationPolicyID string
	Payload            string
	CreatedAt          time.Time
}
