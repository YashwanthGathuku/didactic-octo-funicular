// Package schedule is the deterministic scheduling core.
//
// It exists to solve one problem: a file that never arrives generates no event,
// so nothing that reacts to events can ever notice it is missing. The answer is
// to write the row before the file is due. An occurrence is materialized ahead
// of time and ages into OVERDUE and BREACHED by the passage of time alone.
//
// Everything here is deterministic. No AI, no heuristics, no history: with zero
// prior arrivals and every optional dependency offline, the schedule is fully
// determined by the contract version, the calendar, and the clock.
package schedule

import (
	"fmt"
	"time"
)

// Date is a civil date with no timezone and no instant.
//
// It is a distinct type rather than a time.Time because the two are constantly
// confused and the confusion is silent. "The file for 2025-03-09" is a business
// date; the moment it is due is an instant derived from that date, a local
// time, and a timezone -- and on 2025-03-09 in America/New_York that derivation
// has a one-hour hole in it. Keeping the two apart means the derivation has to
// be written down, which is where the DST rules live (see window.go).
type Date struct {
	Year  int
	Month time.Month
	Day   int
}

// NewDate builds a Date, normalising out-of-range components the way time.Date
// does: NewDate(2025, 1, 32) is 2025-02-01.
func NewDate(year int, month time.Month, day int) Date {
	t := time.Date(year, month, day, 0, 0, 0, 0, time.UTC)
	return Date{Year: t.Year(), Month: t.Month(), Day: t.Day()}
}

// DateOf returns the civil date of an instant as observed in loc.
//
// The location is required. There is no sensible default: the civil date of a
// given instant genuinely differs between zones, and picking UTC silently is
// how a file due at 23:00 in Sydney gets attributed to the previous day.
func DateOf(t time.Time, loc *time.Location) Date {
	local := t.In(loc)
	return Date{Year: local.Year(), Month: local.Month(), Day: local.Day()}
}

// ParseDate reads YYYY-MM-DD.
func ParseDate(s string) (Date, error) {
	t, err := time.Parse("2006-01-02", s)
	if err != nil {
		return Date{}, fmt.Errorf("business date %q is not YYYY-MM-DD: %w", s, err)
	}
	return Date{Year: t.Year(), Month: t.Month(), Day: t.Day()}, nil
}

// String renders YYYY-MM-DD, which is also the storage form.
func (d Date) String() string {
	return fmt.Sprintf("%04d-%02d-%02d", d.Year, int(d.Month), d.Day)
}

// Zero reports whether the date was never set.
func (d Date) Zero() bool { return d.Year == 0 && d.Month == 0 && d.Day == 0 }

// utc is the date's midnight in UTC, used only for arithmetic and comparison.
//
// It is deliberately unexported. UTC midnight is not the start of the business
// day in any zone but UTC, and exposing it invites exactly that mistake.
func (d Date) utc() time.Time {
	return time.Date(d.Year, d.Month, d.Day, 0, 0, 0, 0, time.UTC)
}

// AddDays moves by whole calendar days. Adding a day to 2025-03-09 gives
// 2025-03-10 regardless of that day being 23 hours long in New York.
func (d Date) AddDays(n int) Date {
	t := d.utc().AddDate(0, 0, n)
	return Date{Year: t.Year(), Month: t.Month(), Day: t.Day()}
}

// Weekday reports the day of week. Weekday is a property of the civil date and
// does not depend on any timezone.
func (d Date) Weekday() time.Weekday { return d.utc().Weekday() }

// Before, After and Equal compare civil dates.
func (d Date) Before(o Date) bool { return d.utc().Before(o.utc()) }
func (d Date) After(o Date) bool  { return d.utc().After(o.utc()) }
func (d Date) Equal(o Date) bool  { return d == o }

// DaysUntil returns the whole-day distance from d to o, negative if o precedes.
func (d Date) DaysUntil(o Date) int {
	return int(o.utc().Sub(d.utc()) / (24 * time.Hour))
}

// LastDayOfMonth returns the final day number of the date's month, leap years
// included.
func (d Date) LastDayOfMonth() int {
	// Day 0 of the next month is the last day of this one.
	return time.Date(d.Year, d.Month+1, 0, 0, 0, 0, 0, time.UTC).Day()
}

// IsWeekend reports Saturday or Sunday. It is a separate helper because the
// Federal Reserve's Saturday observance rule turns on it (see calendar.go) and
// the rule is easier to read when the test is named.
func (d Date) IsWeekend() bool {
	w := d.Weekday()
	return w == time.Saturday || w == time.Sunday
}
