package schedule

import (
	"fmt"
	"strconv"
	"strings"
	"time"
)

// LocalTime is a wall-clock time of day in a contract's own timezone.
//
// It is stored and reasoned about separately from any instant, because the
// contract genuinely says "09:00 Eastern", not "13:00 UTC". Those two are the
// same statement for eight months of the year and different for the other four,
// and storing the UTC form makes a file an hour late every spring with nothing
// in the record to explain it.
type LocalTime struct {
	Hour, Minute, Second int
}

// ParseLocalTime reads HH:MM or HH:MM:SS.
func ParseLocalTime(s string) (LocalTime, error) {
	s = strings.TrimSpace(s)
	parts := strings.Split(s, ":")
	if len(parts) != 2 && len(parts) != 3 {
		return LocalTime{}, fmt.Errorf("expected local time %q to be HH:MM or HH:MM:SS", s)
	}
	nums := make([]int, 3)
	for i, p := range parts {
		if len(p) != 2 {
			return LocalTime{}, fmt.Errorf("expected local time %q to be zero-padded, e.g. 09:00", s)
		}
		n, err := strconv.Atoi(p)
		if err != nil {
			return LocalTime{}, fmt.Errorf("expected local time %q to be numeric: %w", s, err)
		}
		nums[i] = n
	}
	lt := LocalTime{Hour: nums[0], Minute: nums[1], Second: nums[2]}
	// 24:00:00 is deliberately refused. It is a legal ISO 8601 spelling of
	// midnight, but it denotes the end of the named day and every reader takes
	// it for the start -- an off-by-one-day deadline that only shows up when a
	// file is judged late.
	if lt.Hour < 0 || lt.Hour > 23 || lt.Minute < 0 || lt.Minute > 59 || lt.Second < 0 || lt.Second > 59 {
		return LocalTime{}, fmt.Errorf("local time %q is out of range", s)
	}
	return lt, nil
}

// String renders HH:MM:SS.
func (l LocalTime) String() string {
	return fmt.Sprintf("%02d:%02d:%02d", l.Hour, l.Minute, l.Second)
}

// LoadLocation resolves an IANA timezone name, refusing the abbreviations.
//
// "EST" and "UTC-5" are refused even though Go accepts "EST": an abbreviation
// names a fixed offset, not a zone, so it does not observe DST at all. A
// contract stored as EST is an hour wrong for eight months of the year, and the
// bug presents as a partner who is mysteriously always early.
func LoadLocation(name string) (*time.Location, error) {
	name = strings.TrimSpace(name)
	if name == "" {
		return nil, fmt.Errorf("a contract version needs an IANA timezone, e.g. America/New_York")
	}
	if name != "UTC" && !strings.Contains(name, "/") {
		return nil, fmt.Errorf(
			"timezone %q is not an IANA name; use a region name such as America/New_York "+
				"(an abbreviation names a fixed offset and does not observe daylight saving)", name)
	}
	loc, err := time.LoadLocation(name)
	if err != nil {
		return nil, fmt.Errorf("unknown timezone %q: %w", name, err)
	}
	return loc, nil
}

// ---------------------------------------------------------------------------
// Resolving a wall-clock time on a date that may not have one
// ---------------------------------------------------------------------------

// Resolution records how a local time mapped onto an instant.
type Resolution string

const (
	// ResolutionExact means the wall-clock time occurred once on that date.
	ResolutionExact Resolution = "EXACT"
	// ResolutionGap means the wall-clock time did not occur: the clock jumped
	// over it going into daylight saving.
	ResolutionGap Resolution = "DST_GAP"
	// ResolutionAmbiguous means the wall-clock time occurred twice, coming out
	// of daylight saving, and the earlier instant was taken.
	ResolutionAmbiguous Resolution = "DST_AMBIGUOUS"
)

// ResolveLocal maps a wall-clock time on a civil date to a single instant, and
// says which of the three cases applied.
//
// Go's time.Date silently normalises both awkward cases and documents that it
// does not guarantee which side it lands on. Observed on this build, 02:30 on
// 2025-03-09 in America/New_York comes back as 01:30 EST -- an hour *earlier*
// than contracted, not later, and a build with different tzdata may answer
// 03:30 EDT instead. For 01:30 on a fall-back date it returns one of the two
// instants without saying which. Neither answer is announced and neither is
// stable, which is the problem: a deadline that moves by an hour twice a year
// with nothing in the record is indistinguishable from a partner who missed it.
//
// The two cases are resolved explicitly:
//
//   - Gap. The deadline becomes the instant the local clock reaches the far
//     side of the jump -- 03:00 for a contracted 02:30, not 03:30. It is the
//     first moment at which the contracted time can be said to have passed, so
//     it is the strictest defensible reading and it never moves a deadline
//     later than the partner would expect.
//   - Ambiguous. The earlier of the two instants is taken, which is again the
//     stricter reading: a file arriving during the repeated hour is on time
//     under the later instant and late under the earlier one, and treating a
//     doubtful case as late raises a reviewable alert instead of silently
//     passing one.
//
// Either way the caller receives the Resolution and persists it, so the record
// says which rule was applied rather than leaving it to be re-derived.
func ResolveLocal(d Date, lt LocalTime, loc *time.Location) (time.Time, Resolution) {
	t := time.Date(d.Year, d.Month, d.Day, lt.Hour, lt.Minute, lt.Second, 0, loc)

	if !sameWallClock(t, d, lt) {
		// The requested wall clock does not exist on this date. Find the
		// transition instant by bisection on the UTC offset: the offset is a
		// step function of time, so the first instant carrying the
		// post-transition offset is exactly the moment the clock jumped.
		//
		// The bounds are the previous and following noon rather than midnight,
		// because some zones shift at midnight (Asia/Beirut, America/Santiago
		// have both done so) and a midnight bound would itself be a
		// non-existent wall time -- normalised by time.Date onto the far side
		// of the very transition being searched for, leaving both bounds in
		// the same offset and the search with nothing to find.
		lo := time.Date(d.Year, d.Month, d.Day, 12, 0, 0, 0, loc).Add(-24 * time.Hour)
		hi := lo.Add(48 * time.Hour)
		_, loOff := lo.Zone()
		// 48h in nanoseconds needs 48 halvings; 64 leaves margin and
		// terminates unconditionally, which a `for hi.Sub(lo) > 0` loop does
		// not once the gap reaches one nanosecond.
		for range 64 {
			mid := lo.Add(hi.Sub(lo) / 2)
			if mid.Equal(lo) || mid.Equal(hi) {
				break
			}
			if _, off := mid.Zone(); off == loOff {
				lo = mid
			} else {
				hi = mid
			}
		}
		return hi.UTC(), ResolutionGap
	}

	// The wall clock exists. If it also exists one hour earlier in absolute
	// time, this date is coming out of daylight saving and the requested time
	// occurs twice; take the earlier instant.
	if earlier := t.Add(-time.Hour); sameWallClock(earlier, d, lt) {
		return earlier.UTC(), ResolutionAmbiguous
	}
	// Symmetrically, t may already be the earlier of the two.
	if later := t.Add(time.Hour); sameWallClock(later, d, lt) {
		return t.UTC(), ResolutionAmbiguous
	}
	return t.UTC(), ResolutionExact
}

func sameWallClock(t time.Time, d Date, lt LocalTime) bool {
	l := t.In(t.Location())
	return l.Year() == d.Year && l.Month() == d.Month && l.Day() == d.Day &&
		l.Hour() == lt.Hour && l.Minute() == lt.Minute && l.Second() == lt.Second
}

// ---------------------------------------------------------------------------
// The deadline window
// ---------------------------------------------------------------------------

// Window is the set of instants that decide an occurrence's state.
//
// All three are stored in UTC. The business date and timezone are stored beside
// them so the local reading can be reproduced, which is what an operator needs
// when a partner disputes a breach.
type Window struct {
	BusinessDate Date
	DueAt        time.Time
	GraceEndsAt  time.Time
	BreachesAt   time.Time
	Resolution   Resolution
	Note         string
}

// Timing is the part of a contract version that produces a window.
type Timing struct {
	ExpectedLocal LocalTime
	Location      *time.Location
	GraceMinutes  int
	BreachMinutes int
}

// WindowFor derives the deadline instants for one business date.
//
// Grace and breach are added in absolute time, not wall-clock time. A 60-minute
// grace period is sixty minutes; on a fall-back date, adding an hour to the
// wall clock would give a partner 120 real minutes, and on a spring-forward
// date it would give them none. Absolute arithmetic is the only reading under
// which "one hour" means the same thing on all 365 days.
func WindowFor(d Date, t Timing) (Window, error) {
	if t.Location == nil {
		return Window{}, fmt.Errorf("business date %s has no timezone", d)
	}
	if t.GraceMinutes < 0 {
		return Window{}, fmt.Errorf("grace period cannot be negative, got %d", t.GraceMinutes)
	}
	if t.BreachMinutes < 0 {
		return Window{}, fmt.Errorf("breach delay cannot be negative, got %d", t.BreachMinutes)
	}

	due, res := ResolveLocal(d, t.ExpectedLocal, t.Location)
	w := Window{
		BusinessDate: d,
		DueAt:        due,
		GraceEndsAt:  due.Add(time.Duration(t.GraceMinutes) * time.Minute),
		Resolution:   res,
	}
	w.BreachesAt = w.GraceEndsAt.Add(time.Duration(t.BreachMinutes) * time.Minute)

	switch res {
	case ResolutionGap:
		w.Note = fmt.Sprintf(
			"local %s does not exist on %s in %s (clocks moved forward); deadline set to the first instant past the gap, %s local",
			t.ExpectedLocal, d, t.Location, due.In(t.Location).Format("15:04:05"))
	case ResolutionAmbiguous:
		w.Note = fmt.Sprintf(
			"local %s occurs twice on %s in %s (clocks moved back); the earlier instant was used",
			t.ExpectedLocal, d, t.Location)
	}
	return w, nil
}
