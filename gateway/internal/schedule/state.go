package schedule

import (
	"fmt"
	"time"

	"sentinel-gateway/internal/domain"
)

// Evaluate returns the state an unmatched occurrence has reached at now.
//
// It is a total function of the window and the clock, with no reference to
// history, arrival events, prior state, or anything learned. That is the point:
// with no arrival ever recorded and every optional subsystem offline, a missing
// file still reaches BREACHED, because reaching BREACHED requires nothing to
// happen.
//
// The boundaries are half-open, [DueAt, GraceEndsAt) and so on, so an instant
// belongs to exactly one state and a file arriving exactly at the deadline is
// on time. Deadlines in this domain are stated as "by 09:00", and treating
// 09:00:00.000 as late is a distinction no partner agreement makes.
func Evaluate(w Window, now time.Time) domain.ExpectationState {
	switch {
	case now.Before(w.DueAt):
		return domain.ExpectationPending
	case now.Before(w.GraceEndsAt):
		return domain.ExpectationDue
	case now.Before(w.BreachesAt):
		return domain.ExpectationOverdue
	default:
		return domain.ExpectationBreached
	}
}

// ageOrder is the linear progression an unmatched occurrence walks as time
// passes. ARRIVED and WAIVED are absent because neither is reached by the clock.
var ageOrder = []domain.ExpectationState{
	domain.ExpectationPending,
	domain.ExpectationDue,
	domain.ExpectationOverdue,
	domain.ExpectationBreached,
}

func ageIndex(s domain.ExpectationState) int {
	for i, v := range ageOrder {
		if v == s {
			return i
		}
	}
	return -1
}

// PathTo returns the intermediate states an occurrence must pass through to get
// from one ageing state to another, excluding the starting state.
//
// A scheduler that has been down over a weekend finds occurrences still PENDING
// whose breach time passed on Friday. Writing BREACHED straight onto PENDING
// would be an illegal edge under domain.CanTransitionExpectation -- the state
// machine from Prompt 03 has no such transition, deliberately -- and forcing
// one would also lose the DUE and OVERDUE entries from the status history, so
// the record would show a file that was never due and then breached.
//
// Every intermediate transition is applied and recorded instead. The occurrence
// arrives at the same state; the trail explains how.
func PathTo(from, to domain.ExpectationState) ([]domain.ExpectationState, error) {
	i, j := ageIndex(from), ageIndex(to)
	if i < 0 {
		// ARRIVED and WAIVED are terminal. An occurrence in one of them is not
		// ageing any more and must not be moved by the clock.
		return nil, nil
	}
	if j < 0 {
		return nil, fmt.Errorf("%q is not a state reached by the passage of time", to)
	}
	if j <= i {
		return nil, nil
	}
	path := make([]domain.ExpectationState, 0, j-i)
	for k := i + 1; k <= j; k++ {
		if !domain.CanTransitionExpectation(ageOrder[k-1], ageOrder[k]) {
			return nil, fmt.Errorf("the expectation state machine has no edge %s -> %s",
				ageOrder[k-1], ageOrder[k])
		}
		path = append(path, ageOrder[k])
	}
	return path, nil
}

// Ageing reports whether a state is still subject to the clock.
func Ageing(s domain.ExpectationState) bool { return ageIndex(s) >= 0 }
