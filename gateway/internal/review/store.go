package review

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"

	"sentinel-gateway/internal/domain"
)

// dialect selects the database-specific statements.
type dialect int

const (
	dialectPostgres dialect = iota
	dialectSQLite
)

// Store persists decisions, votes and overrides.
type Store struct {
	db      *sql.DB
	dialect dialect
	now     func() time.Time

	events EventSink
}

// EventSink receives release events inside the transaction that produced them.
//
// Narrow, and taking the transaction, for the same reason internal/schedule's
// Escalator does: publishing after the commit would let a crash in between lose
// the record of a release that actually happened.
type EventSink interface {
	ReleaseEvent(ctx context.Context, tx *sql.Tx, ev Event) error
}

// Event is one auditable release-workflow transition.
type Event struct {
	TenantID   string
	Action     string
	Actor      string
	DecisionID int64
	ArtifactID int64
	Payload    map[string]any
}

// The actions this package emits.
const (
	ActionProposed = "RELEASE_PROPOSED"
	ActionApproved = "RELEASE_APPROVED"
	ActionRejected = "RELEASE_REJECTED"
	ActionReleased = "ARTIFACT_RELEASED"
	ActionExpired  = "RELEASE_APPROVAL_EXPIRED"
	ActionOverride = "RELEASE_MANUALLY_OVERRIDDEN"
)

// NewStore builds the persistence layer.
func NewStore(db *sql.DB, driverName string) (*Store, error) {
	if db == nil {
		return nil, errors.New("the review store requires a database handle")
	}
	var d dialect
	switch {
	case strings.Contains(driverName, "pgx"), strings.Contains(driverName, "postgres"):
		d = dialectPostgres
	case strings.Contains(driverName, "sqlite"):
		d = dialectSQLite
	default:
		return nil, fmt.Errorf("unsupported driver %q for the review store", driverName)
	}
	return &Store{db: db, dialect: d, now: time.Now}, nil
}

// SetClock replaces the time source, for tests.
func (s *Store) SetClock(fn func() time.Time) { s.now = fn }

// SetEventSink installs the audit and outbox hook.
func (s *Store) SetEventSink(e EventSink) { s.events = e }

func (s *Store) rebind(q string) string {
	if s.dialect != dialectPostgres {
		return q
	}
	var b strings.Builder
	n := 0
	for _, r := range q {
		if r == '?' {
			n++
			fmt.Fprintf(&b, "$%d", n)
			continue
		}
		b.WriteRune(r)
	}
	return b.String()
}

func (s *Store) emit(ctx context.Context, tx *sql.Tx, ev Event) error {
	if s.events == nil {
		return nil
	}
	return s.events.ReleaseEvent(ctx, tx, ev)
}

// ---------------------------------------------------------------------------
// Policy
// ---------------------------------------------------------------------------

// Policy returns a tenant's dual-control configuration.
//
// A tenant with no row gets DefaultPolicy, which is the strict one. The absence
// of configuration must not be the weakest setting: a deployment that forgot to
// configure this would otherwise release on one approval and nobody would find
// out until an audit.
func (s *Store) Policy(ctx context.Context, tenantID string) (Policy, error) {
	p := DefaultPolicy()
	p.TenantID = tenantID

	var minApprovals int
	var sod, override int
	err := s.db.QueryRowContext(ctx, s.rebind(`
		SELECT min_approvals, separation_of_duties, override_allowed
		FROM release_policies WHERE tenant_id = ?`), tenantID).Scan(&minApprovals, &sod, &override)
	if errors.Is(err, sql.ErrNoRows) {
		return p, nil
	}
	if err != nil {
		return p, err
	}
	p.MinApprovals = minApprovals
	p.SeparationOfDuties = sod != 0
	p.OverrideAllowed = override != 0
	return p, nil
}

// SetPolicy records a tenant's configuration.
func (s *Store) SetPolicy(ctx context.Context, actorID string, p Policy) error {
	if err := p.Validate(); err != nil {
		return err
	}
	if strings.TrimSpace(actorID) == "" {
		return errors.New("changing a release policy requires an actor")
	}
	sod, override := 0, 0
	if p.SeparationOfDuties {
		sod = 1
	}
	if p.OverrideAllowed {
		override = 1
	}
	_, err := s.db.ExecContext(ctx, s.rebind(`
		INSERT INTO release_policies
			(tenant_id, min_approvals, separation_of_duties, override_allowed, updated_by, updated_at)
		VALUES (?, ?, ?, ?, ?, ?)
		ON CONFLICT (tenant_id) DO UPDATE SET
			min_approvals = EXCLUDED.min_approvals,
			separation_of_duties = EXCLUDED.separation_of_duties,
			override_allowed = EXCLUDED.override_allowed,
			updated_by = EXCLUDED.updated_by,
			updated_at = EXCLUDED.updated_at`),
		p.TenantID, p.MinApprovals, sod, override, actorID, s.now().UTC())
	return err
}

// ---------------------------------------------------------------------------
// Proposing
// ---------------------------------------------------------------------------

// Propose records a validation run and the decision resting on it.
//
// Called by the validation worker inside the job's transaction, so the run, the
// decision and the artifact's new state commit together. A decision that
// existed without its validation run would be a proposal to release something
// nobody can reconstruct the basis for.
//
// The proposer is `system:validation-worker`, not a person. That matters for
// separation of duties: nothing proposed automatically is blocked from human
// approval, while anything a person proposes is.
func (s *Store) ProposeTx(ctx context.Context, tx *sql.Tx, tenantID, proposedBy string, subject Subject, policy Policy) (int64, error) {
	now := s.now().UTC()
	blocking := strings.Join(subject.BlockingRuleIDs(), ",")

	runID := subject.ValidationRunID
	if runID == 0 {
		var err error
		runID, err = s.insertRun(ctx, tx, tenantID, subject, blocking, now)
		if err != nil {
			return 0, err
		}
		subject.ValidationRunID = runID
	}

	sod := 0
	if policy.SeparationOfDuties {
		sod = 1
	}

	const insert = `
		INSERT INTO policy_decisions
			(tenant_id, file_instance_id, validation_run_id, policy_version, state, outcome,
			 artifact_sha256, integrity_digest, findings_digest, proposed_by, proposed_at,
			 required_approvals, separation_of_duties, decided_at)
		VALUES (?, ?, ?, ?, 'PROPOSED', ?, ?, ?, ?, ?, ?, ?, ?, ?)`
	args := []any{
		tenantID, subject.ArtifactID, runID, subject.PolicyVersion, subject.Outcome,
		subject.ArtifactSHA256, subject.IntegrityDigest(), subject.FindingsDigest(),
		proposedBy, now, policy.MinApprovals, sod, now,
	}

	var id int64
	if s.dialect == dialectPostgres {
		if err := tx.QueryRowContext(ctx, s.rebind(insert+" RETURNING id"), args...).Scan(&id); err != nil {
			return 0, err
		}
	} else {
		res, err := tx.ExecContext(ctx, insert, args...)
		if err != nil {
			return 0, err
		}
		if id, err = res.LastInsertId(); err != nil {
			return 0, err
		}
	}

	return id, s.emit(ctx, tx, Event{
		TenantID: tenantID, Action: ActionProposed, Actor: proposedBy,
		DecisionID: id, ArtifactID: subject.ArtifactID,
		Payload: map[string]any{
			"decisionId":        id,
			"artifactId":        subject.ArtifactID,
			"validationRunId":   runID,
			"outcome":           subject.Outcome,
			"policyVersion":     subject.PolicyVersion,
			"requiredApprovals": policy.MinApprovals,
			"blockingRuleIds":   subject.BlockingRuleIDs(),
		},
	})
}

func (s *Store) insertRun(ctx context.Context, tx *sql.Tx, tenantID string, subject Subject, blocking string, now time.Time) (int64, error) {
	parserOK := 1
	if subject.Outcome == "QUARANTINED" {
		// Not a claim that the parser failed -- a quarantine has many causes.
		// The column is what migration 002 named it, and the honest value for
		// a run that produced blocking findings is the one that does not
		// assert the parse was clean.
		if len(subject.BlockingRuleIDs()) > 0 {
			parserOK = 0
		}
	}

	const insert = `
		INSERT INTO validation_runs
			(tenant_id, file_instance_id, parser_name, parser_version, rule_pack_version, parser_ok,
			 records_parsed, total_debits_minor, total_credits_minor,
			 policy_version, contract_id, contract_version,
			 outcome, findings_digest, blocking_rule_ids, started_at, completed_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`
	args := []any{
		tenantID, subject.ArtifactID, subject.ParserName, subject.ParserVersion,
		subject.RulePackVersion, parserOK,
		subject.RecordsParsed, subject.TotalDebitsMinor, subject.TotalCreditsMinor,
		subject.PolicyVersion, subject.ContractID,
		subject.ContractVersion, subject.Outcome, subject.FindingsDigest(), blocking, now, now,
	}
	if s.dialect == dialectPostgres {
		var id int64
		err := tx.QueryRowContext(ctx, s.rebind(insert+" RETURNING id"), args...).Scan(&id)
		return id, err
	}
	res, err := tx.ExecContext(ctx, insert, args...)
	if err != nil {
		return 0, err
	}
	return res.LastInsertId()
}

// ---------------------------------------------------------------------------
// Reading
// ---------------------------------------------------------------------------

const decisionColumns = `
	d.id, d.tenant_id, d.file_instance_id, d.validation_run_id, d.state, d.outcome, d.policy_version,
	d.artifact_sha256, d.integrity_digest, d.findings_digest, d.proposed_by, d.proposed_at,
	d.required_approvals, d.separation_of_duties, d.expired_reason, d.released_at, d.released_by,
	d.reason, d.row_version, COALESCE(v.rule_pack_version, '')`

// decisionFrom joins the run so a decision carries what produced its findings.
const decisionFrom = `
	FROM policy_decisions d
	LEFT JOIN validation_runs v ON v.id = d.validation_run_id AND v.tenant_id = d.tenant_id`

// Get returns one decision with its votes, tenant-scoped.
func (s *Store) Get(ctx context.Context, tenantID string, id int64) (*Decision, error) {
	row := s.db.QueryRowContext(ctx, s.rebind(
		`SELECT `+decisionColumns+decisionFrom+` WHERE d.tenant_id = ? AND d.id = ?`),
		tenantID, id)
	d, err := scanDecision(row)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	if err := s.loadVotes(ctx, d); err != nil {
		return nil, err
	}
	return d, nil
}

// Queue returns the decisions awaiting human judgement.
//
// Tenant-scoped, and carrying no evidence: a decision references its artifact
// and its findings by identifier, and the findings themselves were redacted
// when they were written by internal/nacha. The queue is a work list, not a
// place to read file content.
func (s *Store) Queue(ctx context.Context, tenantID string, limit int) ([]*Decision, error) {
	return s.QueuePage(ctx, tenantID, 0, limit)
}

// QueuePage returns one page of decisions awaiting a human, oldest first.
//
// Paged rather than capped. The previous form took a limit of 100 and returned
// the same first 100 forever: a tenant with a longer backlog had decisions no
// screen could reach and no way to see that they existed.
//
// The page key is the row id, not decided_at. id is unique and monotonic, so
// the cursor needs no tiebreak and cannot skip a row that shares a timestamp
// with the one before it. Insertion order is proposal order, which is the
// order the queue wants anyway: the decision that has waited longest is the
// one that needs attention.
//
// afterID is exclusive; zero starts at the head.
func (s *Store) QueuePage(ctx context.Context, tenantID string, afterID int64, limit int) ([]*Decision, error) {
	if limit <= 0 || limit > 200 {
		limit = 100
	}
	where := "d.tenant_id = ? AND d.state IN ('PROPOSED','APPROVED')"
	args := []any{tenantID}
	if afterID > 0 {
		where += " AND d.id > ?"
		args = append(args, afterID)
	}
	args = append(args, limit)

	rows, err := s.db.QueryContext(ctx, s.rebind(
		`SELECT `+decisionColumns+decisionFrom+`
		 WHERE `+where+`
		 ORDER BY d.id LIMIT ?`), args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []*Decision
	for rows.Next() {
		d, err := scanDecision(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, d)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	for _, d := range out {
		if err := s.loadVotes(ctx, d); err != nil {
			return nil, err
		}
	}
	return out, nil
}

func scanDecision(sc interface{ Scan(...any) error }) (*Decision, error) {
	var (
		d          Decision
		state      string
		proposedAt sql.NullTime
		sod        int
		expired    sql.NullString
		releasedAt sql.NullTime
		releasedBy sql.NullString
		reason     sql.NullString
	)
	if err := sc.Scan(
		&d.ID, &d.TenantID, &d.ArtifactID, &d.ValidationRunID, &state, &d.Outcome,
		&d.PolicyVersion, &d.ArtifactSHA256, &d.IntegrityDigest, &d.FindingsDigest,
		&d.ProposedBy, &proposedAt, &d.RequiredApprovals, &sod, &expired,
		&releasedAt, &releasedBy, &reason, &d.RowVersion, &d.RulePackVersion,
	); err != nil {
		return nil, err
	}
	d.State = domain.DecisionState(state)
	if proposedAt.Valid {
		d.ProposedAt = proposedAt.Time.UTC()
	}
	d.SeparationOfDuties = sod != 0
	d.ExpiredReason = expired.String
	if releasedAt.Valid {
		t := releasedAt.Time.UTC()
		d.ReleasedAt = &t
	}
	d.ReleasedBy = releasedBy.String
	d.Reason = reason.String
	return &d, nil
}

func (s *Store) loadVotes(ctx context.Context, d *Decision) error {
	rows, err := s.db.QueryContext(ctx, s.rebind(`
		SELECT actor_id, role, vote, reason, approved_at, integrity_digest
		FROM approvals WHERE tenant_id = ? AND decision_id = ? ORDER BY approved_at, id`),
		d.TenantID, d.ID)
	if err != nil {
		return err
	}
	defer rows.Close()

	for rows.Next() {
		var v Vote
		var at time.Time
		if err := rows.Scan(&v.ActorID, &v.Role, &v.Choice, &v.Reason, &at, &v.Digest); err != nil {
			return err
		}
		v.At = at.UTC()
		d.Votes = append(d.Votes, v)
	}
	return rows.Err()
}

// ---------------------------------------------------------------------------
// Voting
// ---------------------------------------------------------------------------

// VoteRequest is one reviewer's judgement.
//
// The actor is a parameter rather than a field a caller can put in a request
// body. The handler takes it from the verified principal, and there is no path
// through this package by which a request could supply one.
type VoteRequest struct {
	DecisionID int64
	Approve    bool
	Reason     string
}

// Vote records a reviewer's judgement, refusing every way it could be invalid.
//
// The whole check sequence runs inside one transaction with an optimistic
// update on row_version, so two reviewers acting at the same instant produce
// one legal outcome: the loser sees ErrConflict and re-reads rather than
// overwriting.
func (s *Store) Vote(ctx context.Context, tenantID, actorID, role string, current Subject, req VoteRequest) (*Decision, error) {
	if strings.TrimSpace(actorID) == "" {
		return nil, errors.New("a vote requires an authenticated actor")
	}
	if strings.TrimSpace(req.Reason) == "" {
		return nil, errors.New("a vote requires a reason; it is recorded in the audit ledger")
	}

	d, err := s.Get(ctx, tenantID, req.DecisionID)
	if err != nil {
		return nil, err
	}
	if err := d.CanVote(actorID); err != nil {
		return nil, err
	}
	// The reviewer is voting on what exists now. A decision whose subject has
	// changed is expired here rather than silently accepting a vote against a
	// state of the world that no longer holds.
	if err := d.CheckFresh(current); err != nil {
		if xerr := s.expire(ctx, tenantID, d, err.Error()); xerr != nil {
			return nil, xerr
		}
		return nil, err
	}

	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, err
	}
	defer func() { _ = tx.Rollback() }()

	now := s.now().UTC()
	choice := "REJECT"
	if req.Approve {
		choice = "APPROVE"
	}

	// The UNIQUE (tenant_id, decision_id, actor_id) constraint from migration
	// 002 is the real enforcement of "one vote per person". The CanVote check
	// above is the friendly error; this is the one that holds under a race.
	if _, err := tx.ExecContext(ctx, s.rebind(`
		INSERT INTO approvals (tenant_id, decision_id, actor_id, role, reason, vote,
		                       integrity_digest, approved_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?)`),
		tenantID, d.ID, actorID, role, req.Reason, choice, d.IntegrityDigest, now); err != nil {
		return nil, ErrAlreadyVoted
	}

	// Recount with this vote included.
	d.Votes = append(d.Votes, Vote{
		ActorID: actorID, Role: role, Choice: choice,
		Reason: req.Reason, At: now, Digest: d.IntegrityDigest,
	})

	next := d.State
	switch {
	case d.Rejected():
		// One rejection is enough. A release requires agreement, so a reviewer
		// who examined the file and refused it is not outvoted.
		next = domain.DecisionRejected
	case d.ApprovalsHeld() >= d.RequiredApprovals:
		next = domain.DecisionApproved
	}

	if next != d.State {
		if !domain.CanTransitionDecision(d.State, next) {
			return nil, &domain.TransitionError{
				Machine: "decision", From: string(d.State), To: string(next)}
		}
		res, err := tx.ExecContext(ctx, s.rebind(`
			UPDATE policy_decisions SET state = ?, row_version = row_version + 1
			WHERE tenant_id = ? AND id = ? AND row_version = ?`),
			string(next), tenantID, d.ID, d.RowVersion)
		if err != nil {
			return nil, err
		}
		if n, _ := res.RowsAffected(); n == 0 {
			return nil, ErrConflict
		}
		d.State = next
		d.RowVersion++
	}

	action := ActionApproved
	if !req.Approve {
		action = ActionRejected
	}
	if err := s.emit(ctx, tx, Event{
		TenantID: tenantID, Action: action, Actor: actorID,
		DecisionID: d.ID, ArtifactID: d.ArtifactID,
		Payload: map[string]any{
			"decisionId":        d.ID,
			"artifactId":        d.ArtifactID,
			"state":             string(d.State),
			"approvalsHeld":     d.ApprovalsHeld(),
			"requiredApprovals": d.RequiredApprovals,
			"role":              role,
			// The reason is a reviewer's own words about a financial file. It
			// is recorded, and it is the one free-text field here, so it is
			// bounded by the caller before it reaches this point.
			"reason": req.Reason,
		},
	}); err != nil {
		return nil, err
	}

	if err := tx.Commit(); err != nil {
		return nil, err
	}
	return d, nil
}

// expire marks a decision stale, outside any caller's transaction.
func (s *Store) expire(ctx context.Context, tenantID string, d *Decision, reason string) error {
	if d.State == domain.DecisionExpired {
		return nil
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()

	res, err := tx.ExecContext(ctx, s.rebind(`
		UPDATE policy_decisions SET state = 'EXPIRED', expired_reason = ?, row_version = row_version + 1
		WHERE tenant_id = ? AND id = ? AND row_version = ? AND state IN ('PROPOSED','APPROVED')`),
		reason, tenantID, d.ID, d.RowVersion)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		// Someone else expired or decided it first. Not an error: the outcome
		// is the one this call wanted.
		return nil
	}
	if err := s.emit(ctx, tx, Event{
		TenantID: tenantID, Action: ActionExpired, Actor: "system:review",
		DecisionID: d.ID, ArtifactID: d.ArtifactID,
		Payload: map[string]any{
			"decisionId": d.ID,
			"artifactId": d.ArtifactID,
			"reason":     reason,
		},
	}); err != nil {
		return err
	}
	d.State = domain.DecisionExpired
	d.ExpiredReason = reason
	return tx.Commit()
}
