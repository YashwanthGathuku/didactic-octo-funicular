package schedule

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	"sentinel-gateway/internal/domain"
)

// MatchOutcome says what happened when an arriving artifact was matched.
type MatchOutcome string

const (
	// MatchAttributed means exactly one open occurrence matched and was
	// marked ARRIVED.
	MatchAttributed MatchOutcome = "ATTRIBUTED"
	// MatchAmbiguous means more than one occurrence could have been satisfied.
	// Nothing was attributed and every candidate was recorded for review.
	MatchAmbiguous MatchOutcome = "AMBIGUOUS"
	// MatchDuplicate means the only matching occurrence was already satisfied
	// by an earlier artifact.
	MatchDuplicate MatchOutcome = "DUPLICATE"
	// MatchUnexpected means nothing was expecting this file. It is not an
	// error: an unexpected file is a real thing that happens, and it is still
	// stored, validated and quarantined on its own merits.
	MatchUnexpected MatchOutcome = "UNEXPECTED"
)

// MatchResult is the full record of a matching attempt.
type MatchResult struct {
	Outcome    MatchOutcome
	Occurrence int64   // set when Outcome is MatchAttributed
	Candidates []int64 // occurrence ids recorded for review
	Reason     string
}

// matchLookbackDays and matchLookaheadDays bound which occurrences an arrival
// is considered against.
//
// A partner sending Monday's file on Wednesday is ordinary. A partner sending a
// file that satisfies an expectation from three months ago is not, and matching
// it would silently close a breach that has already been reported and acted on.
// The window is deliberately narrow in the forward direction: a file arriving
// two days before it is due is far more likely to be the wrong file than an
// early one.
const (
	matchLookbackDays  = 10
	matchLookaheadDays = 2
)

// MatchArrival attributes an arriving artifact to an expected occurrence.
//
// It runs inside the caller's transaction so the attribution commits with the
// artifact record. Attributing in a separate transaction would allow a crash
// between the two, leaving a file that arrived and an expectation that says it
// did not -- which reads, from every report, as a missing file.
//
// The central rule: when the arrival could satisfy more than one occurrence,
// nothing is attributed. Picking the closest deadline is the obvious heuristic
// and it is wrong in the way that matters -- if the guess is wrong, one
// occurrence is marked ARRIVED and a genuinely missing file is recorded as
// having been delivered. The system would then be asserting, with an audit
// trail behind it, that a file it never saw arrived. Recording both candidates
// and asking leaves the other occurrence ageing towards breach, which is the
// truthful state.
func (s *Store) MatchArrival(
	ctx context.Context, tx *sql.Tx,
	tenantID string, artifactID int64, filename string, arrivedAt time.Time,
) (MatchResult, error) {
	arrivedAt = arrivedAt.UTC()
	from := DateOf(arrivedAt, time.UTC).AddDays(-matchLookbackDays)
	to := DateOf(arrivedAt, time.UTC).AddDays(matchLookaheadDays)

	rows, err := tx.QueryContext(ctx, s.rebind(`
		SELECT e.id, e.status, e.row_version, e.business_date, e.matched_artifact_id,
		       v.filename_pattern, v.feed_id
		FROM expectations e
		JOIN file_contract_versions v
		  ON v.id = e.contract_version_id AND v.tenant_id = e.tenant_id
		WHERE e.tenant_id = ?
		  AND e.business_date >= ? AND e.business_date <= ?
		  AND e.status IN ('PENDING','DUE','OVERDUE','BREACHED','ARRIVED')
		ORDER BY e.business_date, e.id`),
		tenantID, from.utc(), to.utc())
	if err != nil {
		return MatchResult{}, err
	}

	type occ struct {
		id       int64
		state    domain.ExpectationState
		version  int64
		business Date
		feedID   string
	}
	var open, satisfied []occ

	for rows.Next() {
		var (
			o        occ
			state    string
			business sql.NullTime
			matched  sql.NullInt64
			pattern  string
		)
		if err := rows.Scan(&o.id, &state, &o.version, &business, &matched, &pattern, &o.feedID); err != nil {
			rows.Close()
			return MatchResult{}, err
		}
		if !business.Valid {
			// A pre-008 occurrence with no business date cannot have its
			// pattern's date tokens resolved, so it is not a candidate. It is
			// skipped rather than matched loosely: a loose match here is the
			// silent wrong attribution this function exists to avoid.
			continue
		}
		o.state = domain.ExpectationState(state)
		o.business = DateOf(business.Time, time.UTC)

		p, err := ParsePattern(pattern)
		if err != nil {
			// A stored pattern that no longer parses is a configuration fault,
			// not an arrival fault. It must not silently exclude the
			// occurrence from matching, so it is surfaced.
			rows.Close()
			return MatchResult{}, fmt.Errorf("occurrence %d has an unusable filename pattern: %w", o.id, err)
		}
		if !p.Match(filename, o.business) {
			continue
		}
		if o.state == domain.ExpectationArrived {
			satisfied = append(satisfied, o)
		} else {
			open = append(open, o)
		}
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return MatchResult{}, err
	}

	switch {
	case len(open) == 1:
		o := open[0]
		reason := fmt.Sprintf("artifact %d (%s) matched feed %s for business date %s",
			artifactID, filename, o.feedID, o.business)
		moved, err := s.attribute(ctx, tx, tenantID, o.id, o.state, o.version, artifactID, arrivedAt, reason)
		if err != nil {
			return MatchResult{}, err
		}
		if !moved {
			// Another transaction attributed it first. The arrival is a
			// duplicate rather than a match, and is recorded as one.
			if err := s.recordCandidate(ctx, tx, tenantID, o.id, artifactID, filename,
				"occurrence was satisfied by another artifact concurrently"); err != nil {
				return MatchResult{}, err
			}
			return MatchResult{Outcome: MatchDuplicate, Candidates: []int64{o.id},
				Reason: "the occurrence was satisfied concurrently by another arrival"}, nil
		}
		return MatchResult{Outcome: MatchAttributed, Occurrence: o.id, Reason: reason}, nil

	case len(open) > 1:
		ids := make([]int64, 0, len(open))
		dates := make([]string, 0, len(open))
		for _, o := range open {
			ids = append(ids, o.id)
			dates = append(dates, o.business.String())
		}
		reason := fmt.Sprintf(
			"%q matches %d open expectations (business dates %v); a human must decide which it satisfies",
			filename, len(open), dates)
		for _, o := range open {
			if err := s.recordCandidate(ctx, tx, tenantID, o.id, artifactID, filename, reason); err != nil {
				return MatchResult{}, err
			}
		}
		return MatchResult{Outcome: MatchAmbiguous, Candidates: ids, Reason: reason}, nil

	case len(satisfied) > 0:
		ids := make([]int64, 0, len(satisfied))
		reason := fmt.Sprintf(
			"%q matches an expectation already satisfied by an earlier artifact", filename)
		for _, o := range satisfied {
			ids = append(ids, o.id)
			if err := s.recordCandidate(ctx, tx, tenantID, o.id, artifactID, filename, reason); err != nil {
				return MatchResult{}, err
			}
		}
		return MatchResult{Outcome: MatchDuplicate, Candidates: ids, Reason: reason}, nil
	}

	return MatchResult{
		Outcome: MatchUnexpected,
		Reason:  fmt.Sprintf("no open expectation matches %q", filename),
	}, nil
}

// attribute marks one occurrence ARRIVED, under optimistic concurrency.
func (s *Store) attribute(
	ctx context.Context, tx *sql.Tx, tenantID string,
	id int64, from domain.ExpectationState, rowVersion int64,
	artifactID int64, at time.Time, reason string,
) (bool, error) {
	if !domain.CanTransitionExpectation(from, domain.ExpectationArrived) {
		return false, &domain.TransitionError{
			Machine: "expectation", From: string(from), To: string(domain.ExpectationArrived)}
	}

	res, err := tx.ExecContext(ctx, s.rebind(`
		UPDATE expectations
		SET status = 'ARRIVED', matched_artifact_id = ?, matched_at = ?,
		    updated_at = ?, row_version = row_version + 1
		WHERE id = ? AND tenant_id = ? AND status = ? AND row_version = ?
		  AND matched_artifact_id IS NULL`),
		artifactID, at, at, id, tenantID, string(from), rowVersion)
	if err != nil {
		return false, err
	}
	n, err := res.RowsAffected()
	if err != nil {
		return false, err
	}
	if n == 0 {
		return false, nil
	}

	// An arrival that satisfies a BREACHED occurrence is worth naming as such.
	// The breach happened; the file turning up later does not unhappen it, and
	// a report that shows only the final state would show a clean day.
	if from == domain.ExpectationBreached {
		reason = "late arrival after breach: " + reason
	}
	if _, err := tx.ExecContext(ctx, s.rebind(`
		INSERT INTO status_history (tenant_id, object_type, object_id, from_state, to_state, actor_id, reason)
		VALUES (?, 'expectation', ?, ?, 'ARRIVED', ?, ?)`),
		tenantID, id, string(from), SchedulerName, reason); err != nil {
		return false, err
	}

	// Point the artifact back at the occurrence it satisfied, so the link is
	// navigable from either end.
	if _, err := tx.ExecContext(ctx, s.rebind(`
		UPDATE file_instances SET expectation_id = ? WHERE tenant_id = ? AND id = ?`),
		id, tenantID, artifactID); err != nil {
		return false, err
	}
	return true, nil
}

// recordCandidate writes one reviewable candidate and flags the occurrence.
//
// The occurrence keeps its ageing state. That is deliberate and it is the whole
// point of the ambiguity path: an occurrence whose arrival is in doubt has not
// been shown to have arrived, so it must continue towards OVERDUE and BREACHED.
// Freezing it, or moving it to a state of its own, would mean a wrong guess
// about which file arrived also stops the clock on the one that did not.
func (s *Store) recordCandidate(
	ctx context.Context, tx *sql.Tx, tenantID string,
	occurrenceID, artifactID int64, filename, reason string,
) error {
	if _, err := tx.ExecContext(ctx, s.rebind(`
		INSERT INTO expectation_match_candidates
			(tenant_id, expectation_id, file_instance_id, filename, reason, resolution)
		VALUES (?, ?, ?, ?, ?, 'REVIEW_REQUIRED')
		ON CONFLICT (tenant_id, expectation_id, file_instance_id) DO NOTHING`),
		tenantID, occurrenceID, artifactID, filename, reason); err != nil {
		return err
	}
	_, err := tx.ExecContext(ctx, s.rebind(`
		UPDATE expectations SET review_required = 1, updated_at = ?
		WHERE tenant_id = ? AND id = ?`), s.Now(), tenantID, occurrenceID)
	return err
}

// OpenReviews counts arrivals awaiting a human decision, for the operations
// view and for tests.
func (s *Store) OpenReviews(ctx context.Context, tenantID string) (int, error) {
	var n int
	err := s.db.QueryRowContext(ctx, s.rebind(`
		SELECT COUNT(*) FROM expectation_match_candidates
		WHERE tenant_id = ? AND resolution = 'REVIEW_REQUIRED'`), tenantID).Scan(&n)
	return n, err
}
