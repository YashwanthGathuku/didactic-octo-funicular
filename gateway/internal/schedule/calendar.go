package schedule

import (
	"fmt"
	"sort"
	"strings"
	"time"
)

// DayKind classifies one calendar date.
type DayKind string

const (
	// KindBusiness is a day on which a file can be expected.
	KindBusiness DayKind = "BUSINESS"
	// KindWeekend is Saturday or Sunday under a calendar that excludes them.
	KindWeekend DayKind = "WEEKEND"
	// KindHoliday is a published holiday observed by the calendar's base.
	KindHoliday DayKind = "HOLIDAY"
	// KindOverrideClosed is a tenant-specific closure.
	KindOverrideClosed DayKind = "OVERRIDE_CLOSED"
	// KindOverrideOpen is a tenant-specific opening on a day the base closes.
	KindOverrideOpen DayKind = "OVERRIDE_OPEN"
)

// DayStatus is a classification with its reason.
//
// The reason is carried rather than derived later because it ends up in
// evidence: "no file was expected on 2026-07-03" is a claim someone will
// eventually dispute, and "the Federal Reserve Saturday rule left July 3 a
// business day and July 4 unobserved" is the answer.
type DayStatus struct {
	Date    Date
	Kind    DayKind
	Reason  string
	Holiday string // the holiday's name when Kind is KindHoliday
}

// Business reports whether work happens on this day.
func (s DayStatus) Business() bool {
	return s.Kind == KindBusiness || s.Kind == KindOverrideOpen
}

// Calendar decides which civil dates are business days.
type Calendar interface {
	// ID is the identifier stored on a contract version.
	ID() string
	// Classify returns the day's kind and the reason for it.
	Classify(d Date) DayStatus
}

// ---------------------------------------------------------------------------
// Base calendars
// ---------------------------------------------------------------------------

// Base names the published rule set a calendar builds on.
type Base string

const (
	// BaseFederalReserve observes weekends and the Federal Reserve Bank
	// holiday schedule. It is the correct base for ACH and Fedwire feeds.
	BaseFederalReserve Base = "FEDERAL_RESERVE"
	// BaseWeekdays observes weekends only, no holidays.
	BaseWeekdays Base = "WEEKDAYS"
	// BaseAllDays treats every date as a business day, for feeds that genuinely
	// arrive seven days a week.
	BaseAllDays Base = "ALL_DAYS"
)

// ParseBase validates a stored base name.
func ParseBase(s string) (Base, error) {
	switch Base(strings.ToUpper(strings.TrimSpace(s))) {
	case BaseFederalReserve:
		return BaseFederalReserve, nil
	case BaseWeekdays:
		return BaseWeekdays, nil
	case BaseAllDays:
		return BaseAllDays, nil
	}
	return "", fmt.Errorf("unknown calendar base %q", s)
}

// ---------------------------------------------------------------------------
// The Federal Reserve holiday rules
// ---------------------------------------------------------------------------

// The eleven standard Federal Reserve Bank holidays, expressed as the rules
// that generate them rather than as a list of dates.
//
// Rules, not dates, because a checked-in list of dates expires. The Federal
// Reserve publishes observed dates only a few years ahead, and a scheduler that
// stops knowing which days are holidays does not fail loudly -- it materializes
// occurrences on Christmas Day and reports every partner as late.
//
// The observance rule is the Federal Reserve's own, and it is not the federal
// employee rule:
//
//	For holidays falling on Saturday, Federal Reserve Bank offices are open
//	the preceding Friday. For holidays falling on Sunday, all Federal Reserve
//	Bank offices are closed the following Monday.
//
// Federal *employees* get the preceding Friday off for a Saturday holiday. Banks
// do not. Encoding the employee rule here would mark a normal Friday as closed
// and suppress a real expectation -- the failure that produces a silently
// missing file, which is the whole thing this package exists to prevent.
//
// SinceYear records when a holiday entered the schedule, so a date in a year
// before it existed is not retroactively made a holiday.
type fedHoliday struct {
	Name      string
	SinceYear int

	// Fixed-date holidays set month and day; the weekend rule applies to them.
	Month time.Month
	Day   int

	// Monday-and-Thursday holidays set an ordinal weekday instead. These can
	// never fall on a weekend, so the observance rule never applies.
	Weekday time.Weekday
	Nth     int // 1..4, or -1 for the last such weekday in the month
}

var fedHolidays = []fedHoliday{
	{Name: "New Year's Day", Month: time.January, Day: 1, SinceYear: 1870},
	// Made a federal holiday in 1983, first observed in 1986.
	{Name: "Birthday of Martin Luther King, Jr.", Month: time.January, Weekday: time.Monday, Nth: 3, SinceYear: 1986},
	// Uniform Monday Holiday Act moved this to the third Monday from 1971.
	{Name: "Washington's Birthday", Month: time.February, Weekday: time.Monday, Nth: 3, SinceYear: 1971},
	{Name: "Memorial Day", Month: time.May, Weekday: time.Monday, Nth: -1, SinceYear: 1971},
	// Enacted 17 June 2021. June 19 2021 fell on a Saturday, so under the
	// Federal Reserve rule above the first date on which Reserve Bank offices
	// actually closed for it was Monday 20 June 2022.
	{Name: "Juneteenth National Independence Day", Month: time.June, Day: 19, SinceYear: 2021},
	{Name: "Independence Day", Month: time.July, Day: 4, SinceYear: 1870},
	{Name: "Labor Day", Month: time.September, Weekday: time.Monday, Nth: 1, SinceYear: 1894},
	{Name: "Columbus Day", Month: time.October, Weekday: time.Monday, Nth: 2, SinceYear: 1971},
	{Name: "Veterans Day", Month: time.November, Day: 11, SinceYear: 1971},
	{Name: "Thanksgiving Day", Month: time.November, Weekday: time.Thursday, Nth: 4, SinceYear: 1870},
	{Name: "Christmas Day", Month: time.December, Day: 25, SinceYear: 1870},
}

// nthWeekday returns the date of the nth given weekday in a month, or the last
// one when n is -1.
func nthWeekday(year int, month time.Month, wd time.Weekday, n int) Date {
	if n < 0 {
		last := NewDate(year, month, 1)
		last = NewDate(year, month, last.LastDayOfMonth())
		back := (int(last.Weekday()) - int(wd) + 7) % 7
		return last.AddDays(-back)
	}
	first := NewDate(year, month, 1)
	forward := (int(wd) - int(first.Weekday()) + 7) % 7
	return first.AddDays(forward + 7*(n-1))
}

// observedFederalHolidays returns the dates on which Federal Reserve Bank
// offices are closed in a given year, keyed by date string.
func observedFederalHolidays(year int) map[string]string {
	out := make(map[string]string, len(fedHolidays))
	for _, h := range fedHolidays {
		if year < h.SinceYear {
			continue
		}
		var d Date
		if h.Nth != 0 {
			d = nthWeekday(year, h.Month, h.Weekday, h.Nth)
		} else {
			d = NewDate(year, h.Month, h.Day)
			switch d.Weekday() {
			case time.Saturday:
				// Reserve Bank offices are open the preceding Friday, and the
				// holiday is simply not observed. No date is added.
				continue
			case time.Sunday:
				d = d.AddDays(1)
			}
		}
		out[d.String()] = h.Name
	}
	// New Year's Day of the following year is observed on 31 December of this
	// one only under the federal employee rule, which the Reserve Banks do not
	// follow. It is deliberately absent.
	return out
}

// federalReserveCalendar observes weekends and the Reserve Bank holidays.
type federalReserveCalendar struct{}

func (federalReserveCalendar) ID() string { return string(BaseFederalReserve) }

func (federalReserveCalendar) Classify(d Date) DayStatus {
	if d.IsWeekend() {
		return DayStatus{Date: d, Kind: KindWeekend, Reason: d.Weekday().String()}
	}
	if name, ok := observedFederalHolidays(d.Year)[d.String()]; ok {
		return DayStatus{
			Date: d, Kind: KindHoliday, Holiday: name,
			Reason: "Federal Reserve Bank holiday: " + name,
		}
	}
	return DayStatus{Date: d, Kind: KindBusiness, Reason: "weekday, no observed holiday"}
}

type weekdayCalendar struct{}

func (weekdayCalendar) ID() string { return string(BaseWeekdays) }

func (weekdayCalendar) Classify(d Date) DayStatus {
	if d.IsWeekend() {
		return DayStatus{Date: d, Kind: KindWeekend, Reason: d.Weekday().String()}
	}
	return DayStatus{Date: d, Kind: KindBusiness, Reason: "weekday"}
}

type allDaysCalendar struct{}

func (allDaysCalendar) ID() string { return string(BaseAllDays) }

func (allDaysCalendar) Classify(d Date) DayStatus {
	return DayStatus{Date: d, Kind: KindBusiness, Reason: "calendar observes every day"}
}

// BaseCalendar returns the published calendar for a base.
func BaseCalendar(b Base) (Calendar, error) {
	switch b {
	case BaseFederalReserve:
		return federalReserveCalendar{}, nil
	case BaseWeekdays:
		return weekdayCalendar{}, nil
	case BaseAllDays:
		return allDaysCalendar{}, nil
	}
	return nil, fmt.Errorf("unknown calendar base %q", b)
}

// ---------------------------------------------------------------------------
// Tenant overrides
// ---------------------------------------------------------------------------

// Override is one tenant-specific correction to a base calendar.
//
// Both directions are needed and they are not symmetric in importance. Closing
// a day the base calls open suppresses an expectation, so a wrong one hides a
// missing file; opening a day the base calls closed creates one, so a wrong one
// raises a false alarm. Both carry a mandatory reason for that reason.
type Override struct {
	Date   Date
	Open   bool
	Reason string
}

// OverlayCalendar applies tenant overrides on top of a published base.
type OverlayCalendar struct {
	id        string
	base      Calendar
	overrides map[string]Override
}

// NewOverlayCalendar builds a calendar from a base and a set of overrides.
//
// An override with no reason is refused. An unexplained closure is
// indistinguishable from a mistake at the moment someone has to defend a
// missed file, and by then the person who added it has left.
func NewOverlayCalendar(id string, base Calendar, overrides []Override) (*OverlayCalendar, error) {
	if id == "" {
		return nil, fmt.Errorf("a calendar needs an id")
	}
	if base == nil {
		return nil, fmt.Errorf("calendar %q has no base", id)
	}
	m := make(map[string]Override, len(overrides))
	for _, o := range overrides {
		if o.Date.Zero() {
			return nil, fmt.Errorf("calendar %q has an override with no date", id)
		}
		if strings.TrimSpace(o.Reason) == "" {
			return nil, fmt.Errorf("calendar %q override on %s has no reason", id, o.Date)
		}
		if _, dup := m[o.Date.String()]; dup {
			return nil, fmt.Errorf("calendar %q has two overrides for %s", id, o.Date)
		}
		m[o.Date.String()] = o
	}
	return &OverlayCalendar{id: id, base: base, overrides: m}, nil
}

// ID returns the calendar identifier.
func (c *OverlayCalendar) ID() string { return c.id }

// Classify applies the override if one exists, otherwise the base.
func (c *OverlayCalendar) Classify(d Date) DayStatus {
	if o, ok := c.overrides[d.String()]; ok {
		kind := KindOverrideClosed
		if o.Open {
			kind = KindOverrideOpen
		}
		return DayStatus{
			Date: d, Kind: kind,
			Reason: "tenant override: " + o.Reason,
		}
	}
	return c.base.Classify(d)
}

// Overrides returns the configured overrides in date order, for reporting.
func (c *OverlayCalendar) Overrides() []Override {
	out := make([]Override, 0, len(c.overrides))
	for _, o := range c.overrides {
		out = append(out, o)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Date.Before(out[j].Date) })
	return out
}

// NextBusinessDay returns the first business day strictly after d.
//
// The search is bounded. An unbounded walk over a misconfigured calendar that
// declares every day closed does not return, and a scheduler that does not
// return is a scheduler that stops noticing missing files.
func NextBusinessDay(cal Calendar, d Date) (Date, bool) {
	for i := 1; i <= maxCalendarSearchDays; i++ {
		c := d.AddDays(i)
		if cal.Classify(c).Business() {
			return c, true
		}
	}
	return Date{}, false
}

// PreviousBusinessDay returns the last business day strictly before d.
func PreviousBusinessDay(cal Calendar, d Date) (Date, bool) {
	for i := 1; i <= maxCalendarSearchDays; i++ {
		c := d.AddDays(-i)
		if cal.Classify(c).Business() {
			return c, true
		}
	}
	return Date{}, false
}

// maxCalendarSearchDays bounds adjustment searches. The longest run of
// non-business days a sane calendar produces is a handful; 30 leaves room for a
// deliberate multi-week closure and still terminates.
const maxCalendarSearchDays = 30
