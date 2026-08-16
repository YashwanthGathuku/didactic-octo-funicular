package schedule

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"log"
	"sort"
	"time"

	"sentinel-gateway/internal/domain"
)

// DefaultHorizonDays is how far ahead occurrences are materialized.
//
// The horizon has to exceed the longest interval between scheduler runs by a
// wide margin, because an occurrence that was never written cannot become
// overdue -- a gap in materialization is a window in which a missing file is
// invisible, and it is invisible in exactly the way this subsystem exists to
// prevent. Fourteen days survives a long outage, a holiday weekend, and a
// deployment freeze together.
const DefaultHorizonDays = 14

// maxAdvanceBatch bounds one advancement pass.
//
// Unbounded would be simpler and would also mean the first run after a long
// outage holds a write transaction over every occurrence in the table. The
// remainder is picked up by the next tick.
const maxAdvanceBatch = 500

// SchedulerName is the actor recorded on transitions the scheduler performs.
// The `system:` prefix is the convention from internal/ledger: a process and a
// person must never be confusable in the audit trail.
const SchedulerName = "system:scheduler"

// MaterializeResult reports what one materialization pass did.
type MaterializeResult struct {
	Created  int
	Repinned int
	Skipped  int
	// Problems names contracts that could not be scheduled, with the reason.
	// They are reported rather than returned as an error: one unusable
	// contract must not stop every other tenant's schedule being written.
	Problems []string
}

// Materialize writes occurrences for every contract version over the horizon.
//
// Idempotent by construction. The occurrence table has a unique key on
// (tenant, contract, business date), and every insert is a conflict-do-nothing,
// so a restart mid-pass, a second scheduler running concurrently, and a pass
// that overlaps the previous one all converge on exactly one row per business
// date. No lease, no leader election, no advisory lock: the constraint is the
// coordination mechanism, and it is one that cannot be lost, expire, or be held
// by a process that has already died.
func (s *Store) Materialize(ctx context.Context, horizonDays int) (MaterializeResult, error) {
	var res MaterializeResult
	if horizonDays < 1 {
		return res, fmt.Errorf("materialization horizon must be at least one day, got %d", horizonDays)
	}

	versions, err := s.allVersions(ctx)
	if err != nil {
		return res, err
	}

	now := s.Now()
	calendars := map[string]Calendar{}

	// Group versions by contract so a contract's whole timeline is visible at
	// once: a horizon that spans a version change has to resolve each business
	// date to its own version, not to whichever version was loaded first.
	byContract := map[string][]Version{}
	var order []string
	for _, v := range versions {
		key := v.TenantID + "\x1e" + fmt.Sprint(v.ContractID)
		if _, ok := byContract[key]; !ok {
			order = append(order, key)
		}
		byContract[key] = append(byContract[key], v)
	}
	sort.Strings(order)

	for _, key := range order {
		group := byContract[key]
		tenantID := group[0].TenantID
		contractID := group[0].ContractID

		// The window starts today in the contract's own timezone. Starting
		// from a UTC "today" would skip or duplicate a day for every tenant
		// east of Greenwich for part of each day.
		loc := group[0].Location
		start := DateOf(now, loc)
		end := start.AddDays(horizonDays)

		// One pass per version, over the part of the horizon that version
		// governs, rather than one pass per day. Each version brings its own
		// rule, calendar and adjustment, so a horizon spanning a version change
		// is genuinely two schedules laid end to end -- and computing them
		// separately is also what stops a day-by-day loop re-scanning an
		// adjustment span for every date.
		for _, v := range group {
			from, to := start, end
			if v.EffectiveFrom.After(from) {
				from = v.EffectiveFrom
			}
			if v.EffectiveTo != nil {
				last := v.EffectiveTo.AddDays(-1) // the interval is half-open
				if last.Before(to) {
					to = last
				}
			}
			if to.Before(from) {
				continue // this version governs no day of the horizon
			}

			calKey := v.TenantID + "\x1e" + v.CalendarID
			cal, ok := calendars[calKey]
			if !ok {
				cal, err = s.Calendar(ctx, v.TenantID, v.CalendarID)
				if err != nil {
					res.Problems = append(res.Problems, fmt.Sprintf(
						"contract %d (tenant %s): %v", contractID, tenantID, err))
					break
				}
				calendars[calKey] = cal
			}

			slots, err := v.Rule.Slots(from, to, cal, v.Adjust)
			if err != nil {
				res.Problems = append(res.Problems, fmt.Sprintf(
					"contract %d (tenant %s): %v", contractID, tenantID, err))
				continue
			}
			for _, slot := range slots {
				// An adjusted slot can land on a date a different version
				// governs. The occurrence belongs to the version in force on
				// the day the file is actually expected, not on the day the
				// rule named.
				sv, ok := versionOn(group, slot.BusinessDate)
				if !ok || sv.ID != v.ID {
					continue
				}
				created, repinned, err := s.upsertOccurrence(ctx, sv, slot, now)
				if err != nil {
					res.Problems = append(res.Problems, fmt.Sprintf(
						"contract %d (tenant %s) on %s: %v", contractID, tenantID, slot.BusinessDate, err))
					continue
				}
				switch {
				case created:
					res.Created++
				case repinned:
					res.Repinned++
				default:
					res.Skipped++
				}
			}
		}
	}
	return res, nil
}

// versionOn picks the version governing a date from a contract's timeline.
func versionOn(group []Version, d Date) (Version, bool) {
	var best Version
	found := false
	for _, v := range group {
		if !v.ActiveOn(d) {
			continue
		}
		if !found || v.EffectiveFrom.After(best.EffectiveFrom) ||
			(v.EffectiveFrom.Equal(best.EffectiveFrom) && v.Version > best.Version) {
			best, found = v, true
		}
	}
	return best, found
}

// upsertOccurrence inserts one occurrence, or re-points a future one whose
// governing version has changed.
func (s *Store) upsertOccurrence(ctx context.Context, v Version, slot Slot, now time.Time) (created, repinned bool, err error) {
	w, err := WindowFor(slot.BusinessDate, v.Timing())
	if err != nil {
		return false, false, err
	}
	note := joinNotes(slot.Note, w.Note)

	res, err := s.db.ExecContext(ctx, s.rebind(`
		INSERT INTO expectations
			(tenant_id, contract_id, contract_version_id, business_date,
			 expected_delivery_start, expected_delivery_end, breach_at,
			 status, schedule_note, due_local, timezone, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, 'PENDING', ?, ?, ?, ?, ?)
		ON CONFLICT (tenant_id, contract_id, business_date) DO NOTHING`),
		v.TenantID, v.ContractID, v.ID, slot.BusinessDate.utc(),
		w.DueAt, w.GraceEndsAt, w.BreachesAt,
		note, v.ExpectedLocal.String(), v.Timezone, now, now)
	if err != nil {
		return false, false, err
	}
	if n, err := res.RowsAffected(); err == nil && n > 0 {
		return true, false, nil
	}

	// The row already exists. It is updated only when the governing version
	// has changed AND the occurrence is untouched and still in the future.
	//
	// Every clause of that guard is load-bearing. Re-pinning an occurrence
	// whose deadline has passed would move a deadline that has already been
	// judged against; re-pinning a matched one would contradict an arrival
	// already attributed; re-pinning one under review would discard the
	// question a human was asked.
	upd, err := s.db.ExecContext(ctx, s.rebind(`
		UPDATE expectations
		SET contract_version_id = ?, expected_delivery_start = ?, expected_delivery_end = ?,
		    breach_at = ?, schedule_note = ?, due_local = ?, timezone = ?,
		    updated_at = ?, row_version = row_version + 1
		WHERE tenant_id = ? AND contract_id = ? AND business_date = ?
		  AND status = 'PENDING'
		  AND matched_artifact_id IS NULL
		  AND review_required = 0
		  AND expected_delivery_start > ?
		  AND contract_version_id <> ?`),
		v.ID, w.DueAt, w.GraceEndsAt, w.BreachesAt, note, v.ExpectedLocal.String(), v.Timezone,
		now, v.TenantID, v.ContractID, slot.BusinessDate.utc(), now, v.ID)
	if err != nil {
		return false, false, err
	}
	if n, err := upd.RowsAffected(); err == nil && n > 0 {
		return false, true, nil
	}
	return false, false, nil
}

// ---------------------------------------------------------------------------
// Advancement
// ---------------------------------------------------------------------------

// AdvanceResult reports what one advancement pass did.
type AdvanceResult struct {
	Due      int
	Overdue  int
	Breached int
	// Contended counts occurrences another scheduler advanced first. It is
	// reported rather than hidden because a persistently high count means two
	// schedulers are duplicating work, which is correct but wasteful.
	Contended int
}

// Advance moves every ageing occurrence to the state the clock says it is in.
//
// This is the mechanism that makes a missing file visible. It reads no arrival
// events and consults nothing external: an occurrence whose deadline has passed
// and which nothing has matched moves DUE, then OVERDUE, then BREACHED purely
// because time passed.
//
// Two schedulers running this concurrently is safe and produces one set of
// transitions. Each step is a conditional update on the occurrence's
// row_version, so the loser of a race changes nothing and records nothing.
func (s *Store) Advance(ctx context.Context) (AdvanceResult, error) {
	var res AdvanceResult
	now := s.Now()

	rows, err := s.db.QueryContext(ctx, s.rebind(`
		SELECT id, tenant_id, status, row_version,
		       expected_delivery_start, expected_delivery_end, breach_at, business_date
		FROM expectations
		WHERE status IN ('PENDING','DUE','OVERDUE')
		  AND expected_delivery_start <= ?
		ORDER BY expected_delivery_start
		LIMIT ?`), now, maxAdvanceBatch)
	if err != nil {
		return res, err
	}

	type candidate struct {
		id       int64
		tenant   string
		state    domain.ExpectationState
		version  int64
		window   Window
		business Date
	}
	var candidates []candidate

	for rows.Next() {
		var (
			c        candidate
			state    string
			due      time.Time
			grace    time.Time
			breach   sql.NullTime
			business sql.NullTime
		)
		if err := rows.Scan(&c.id, &c.tenant, &state, &c.version, &due, &grace, &breach, &business); err != nil {
			rows.Close()
			return res, err
		}
		c.state = domain.ExpectationState(state)
		c.window = Window{DueAt: due.UTC(), GraceEndsAt: grace.UTC()}
		if breach.Valid {
			c.window.BreachesAt = breach.Time.UTC()
		} else {
			// Occurrences written before migration 008 have no breach instant.
			// Treating a missing one as "breaches when grace ends" would
			// declare a breach on rows that were never configured for one;
			// treating it as never breaching would hide them. The grace end is
			// used and the row stops at OVERDUE, which is the state a human
			// acts on, without asserting a contractual breach the contract
			// never specified.
			c.window.BreachesAt = grace.UTC()
		}
		if business.Valid {
			c.business = DateOf(business.Time, time.UTC)
		}
		candidates = append(candidates, c)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return res, err
	}

	for _, c := range candidates {
		target := Evaluate(c.window, now)
		if c.window.BreachesAt.Equal(c.window.GraceEndsAt) && target == domain.ExpectationBreached {
			// The legacy case described above: no configured breach instant,
			// so the occurrence is held at OVERDUE.
			target = domain.ExpectationOverdue
		}
		path, err := PathTo(c.state, target)
		if err != nil {
			return res, err
		}
		for _, next := range path {
			moved, err := s.transition(ctx, c.id, c.tenant, c.state, next, c.version, now, c.business)
			if err != nil {
				return res, err
			}
			if !moved {
				res.Contended++
				break
			}
			c.state = next
			c.version++
			switch next {
			case domain.ExpectationDue:
				res.Due++
			case domain.ExpectationOverdue:
				res.Overdue++
			case domain.ExpectationBreached:
				res.Breached++
			}
		}
	}
	return res, nil
}

// transition applies one state change with optimistic concurrency and records
// it in the append-only status history.
//
// The history row is written in the same transaction as the state change. A
// transition without its history entry is a state that changed with no record
// of when or why, which is the condition the status history exists to rule out.
func (s *Store) transition(
	ctx context.Context,
	id int64, tenantID string,
	from, to domain.ExpectationState,
	rowVersion int64, now time.Time, business Date,
) (bool, error) {
	if !domain.CanTransitionExpectation(from, to) {
		return false, &domain.TransitionError{Machine: "expectation", From: string(from), To: string(to)}
	}

	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return false, err
	}
	defer func() { _ = tx.Rollback() }()

	res, err := tx.ExecContext(ctx, s.rebind(`
		UPDATE expectations
		SET status = ?, updated_at = ?, row_version = row_version + 1
		WHERE id = ? AND tenant_id = ? AND status = ? AND row_version = ?`),
		string(to), now, id, tenantID, string(from), rowVersion)
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

	reason := fmt.Sprintf("scheduled deadline for business date %s reached %s", business, to)
	if _, err := tx.ExecContext(ctx, s.rebind(`
		INSERT INTO status_history (tenant_id, object_type, object_id, from_state, to_state, actor_id, reason)
		VALUES (?, 'expectation', ?, ?, ?, ?, ?)`),
		tenantID, id, string(from), string(to), SchedulerName, reason); err != nil {
		return false, err
	}

	// A breach opens an incident and records who has to be told, in this same
	// transaction. Escalating after the commit would let a crash in between
	// leave the breach on record with nobody informed -- silent in exactly the
	// way this subsystem exists to prevent.
	if to == domain.ExpectationBreached {
		if err := s.escalate(ctx, tx, tenantID, id, now); err != nil {
			return false, err
		}
	}

	return true, tx.Commit()
}

// ---------------------------------------------------------------------------
// The loop
// ---------------------------------------------------------------------------

// RunConfig bounds the scheduler loop.
type RunConfig struct {
	Interval    time.Duration
	HorizonDays int
}

// DefaultRunConfig returns the shipped defaults.
//
// A one-minute interval is chosen from what it costs to be wrong: the interval
// is the maximum delay between an occurrence breaching and anyone being able to
// see it, and the pass is two indexed queries plus a bounded number of small
// transactions.
func DefaultRunConfig() RunConfig {
	return RunConfig{Interval: time.Minute, HorizonDays: DefaultHorizonDays}
}

// Run materializes and advances until the context is cancelled.
//
// Both operations run on every tick, materialization first. The order matters
// on the first tick after a deployment: advancing before materializing would
// leave today's occurrences unwritten for one interval, and if that deployment
// happened to land after a deadline, the missing file would go unreported for
// as long as the interval.
func (s *Store) Run(ctx context.Context, cfg RunConfig) {
	if cfg.Interval <= 0 {
		cfg.Interval = time.Minute
	}
	if cfg.HorizonDays < 1 {
		cfg.HorizonDays = DefaultHorizonDays
	}

	tick := time.NewTicker(cfg.Interval)
	defer tick.Stop()

	for {
		s.runOnce(ctx, cfg)
		select {
		case <-ctx.Done():
			return
		case <-tick.C:
		}
	}
}

func (s *Store) runOnce(ctx context.Context, cfg RunConfig) {
	mres, err := s.Materialize(ctx, cfg.HorizonDays)
	if err != nil && !errors.Is(err, context.Canceled) {
		log.Printf("scheduler: materialize: %v", err)
	}
	for _, p := range mres.Problems {
		// Reported every pass rather than once. A contract that cannot be
		// scheduled is a feed nobody is watching, and a message that appeared
		// only in the log line following a deployment is a message nobody saw.
		log.Printf("scheduler: cannot schedule %s", p)
	}
	ares, err := s.Advance(ctx)
	if err != nil && !errors.Is(err, context.Canceled) {
		log.Printf("scheduler: advance: %v", err)
	}
	if mres.Created > 0 || ares.Breached > 0 || ares.Overdue > 0 {
		log.Printf("scheduler: %d occurrences created, %d due, %d overdue, %d breached",
			mres.Created, ares.Due, ares.Overdue, ares.Breached)
	}
}
