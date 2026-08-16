package schedule

import (
	"fmt"
	"sort"
	"strconv"
	"strings"
	"time"
)

// Adjust says what to do when a rule names a date the calendar closes.
type Adjust string

const (
	// AdjustSkip drops the occurrence. Nothing is expected that day.
	AdjustSkip Adjust = "SKIP"
	// AdjustPreceding moves it to the previous business day. This is the
	// common banking convention for a month-end feed.
	AdjustPreceding Adjust = "PRECEDING"
	// AdjustFollowing moves it to the next business day.
	AdjustFollowing Adjust = "FOLLOWING"
)

// ParseAdjust validates a stored adjustment.
func ParseAdjust(s string) (Adjust, error) {
	switch Adjust(strings.ToUpper(strings.TrimSpace(s))) {
	case AdjustSkip:
		return AdjustSkip, nil
	case AdjustPreceding:
		return AdjustPreceding, nil
	case AdjustFollowing:
		return AdjustFollowing, nil
	}
	return "", fmt.Errorf("unknown non-business-day action %q", s)
}

// RuleKind is the family a schedule rule belongs to.
type RuleKind string

const (
	// KindEveryBusinessDay expects a file on every day the calendar opens.
	KindEveryBusinessDay RuleKind = "EVERY_BUSINESS_DAY"
	// KindWeekly expects a file on named weekdays.
	KindWeekly RuleKind = "WEEKLY"
	// KindMonthly expects a file on named days of the month, where LAST means
	// the final calendar day.
	KindMonthly RuleKind = "MONTHLY"
)

// Rule is a parsed schedule rule.
//
// The stored form is a small closed grammar rather than a cron expression.
// Cron cannot express "the last day of the month" without a vendor extension,
// has no notion of a business calendar at all, and its five-field form is read
// wrongly by most people who read it. The failure mode of a misread schedule is
// an expectation that never materializes, which is silent.
//
//	EVERY_BUSINESS_DAY
//	WEEKLY:MON,WED,FRI
//	MONTHLY:1,15
//	MONTHLY:LAST
type Rule struct {
	Kind     RuleKind
	Weekdays []time.Weekday // KindWeekly
	Days     []int          // KindMonthly; -1 means the last day of the month
	raw      string
}

// String returns the canonical stored form.
func (r Rule) String() string { return r.raw }

var weekdayNames = map[string]time.Weekday{
	"SUN": time.Sunday, "MON": time.Monday, "TUE": time.Tuesday,
	"WED": time.Wednesday, "THU": time.Thursday, "FRI": time.Friday,
	"SAT": time.Saturday,
}

var weekdayAbbrev = [...]string{"SUN", "MON", "TUE", "WED", "THU", "FRI", "SAT"}

// ParseRule reads the stored form.
func ParseRule(s string) (Rule, error) {
	s = strings.ToUpper(strings.TrimSpace(s))
	if s == "" {
		return Rule{}, fmt.Errorf("a contract version needs a schedule rule")
	}
	head, tail, hasTail := strings.Cut(s, ":")

	switch RuleKind(head) {
	case KindEveryBusinessDay:
		if hasTail {
			return Rule{}, fmt.Errorf("EVERY_BUSINESS_DAY takes no argument, got %q", tail)
		}
		return Rule{Kind: KindEveryBusinessDay, raw: string(KindEveryBusinessDay)}, nil

	case KindWeekly:
		if !hasTail {
			return Rule{}, fmt.Errorf("WEEKLY needs at least one weekday, e.g. WEEKLY:MON")
		}
		seen := map[time.Weekday]bool{}
		var days []time.Weekday
		for _, part := range strings.Split(tail, ",") {
			wd, ok := weekdayNames[strings.TrimSpace(part)]
			if !ok {
				return Rule{}, fmt.Errorf("WEEKLY: %q is not a weekday abbreviation", part)
			}
			if seen[wd] {
				return Rule{}, fmt.Errorf("WEEKLY: %s is listed twice", part)
			}
			seen[wd] = true
			days = append(days, wd)
		}
		sort.Slice(days, func(i, j int) bool { return days[i] < days[j] })
		names := make([]string, len(days))
		for i, d := range days {
			names[i] = weekdayAbbrev[d]
		}
		return Rule{Kind: KindWeekly, Weekdays: days, raw: "WEEKLY:" + strings.Join(names, ",")}, nil

	case KindMonthly:
		if !hasTail {
			return Rule{}, fmt.Errorf("MONTHLY needs at least one day, e.g. MONTHLY:1 or MONTHLY:LAST")
		}
		seen := map[int]bool{}
		var days []int
		for _, part := range strings.Split(tail, ",") {
			part = strings.TrimSpace(part)
			if part == "LAST" {
				if seen[-1] {
					return Rule{}, fmt.Errorf("MONTHLY: LAST is listed twice")
				}
				seen[-1] = true
				days = append(days, -1)
				continue
			}
			n, err := strconv.Atoi(part)
			if err != nil {
				return Rule{}, fmt.Errorf("MONTHLY: %q is neither a day number nor LAST", part)
			}
			// 29, 30 and 31 are refused rather than clamped. A contract that
			// says "the 31st" in a 30-day month has an unanswerable question in
			// it, and clamping answers it silently -- the partner and the
			// gateway then disagree about which day the file was due, in the
			// months where it matters most. LAST states the intent exactly.
			if n < 1 || n > 28 {
				return Rule{}, fmt.Errorf(
					"MONTHLY: day %d is out of range; use 1-28, or LAST for month end "+
						"(29-31 are refused because they do not exist in every month)", n)
			}
			if seen[n] {
				return Rule{}, fmt.Errorf("MONTHLY: day %d is listed twice", n)
			}
			seen[n] = true
			days = append(days, n)
		}
		sort.Ints(days)
		parts := make([]string, len(days))
		for i, d := range days {
			if d == -1 {
				parts[i] = "LAST"
			} else {
				parts[i] = strconv.Itoa(d)
			}
		}
		return Rule{Kind: KindMonthly, Days: days, raw: "MONTHLY:" + strings.Join(parts, ",")}, nil
	}
	return Rule{}, fmt.Errorf("unknown schedule rule %q", s)
}

// Slot is one date the schedule produces.
type Slot struct {
	// BusinessDate is the date the occurrence is keyed by and the date the
	// file is actually expected on.
	BusinessDate Date
	// NominalDate is the date the rule named before calendar adjustment. It
	// equals BusinessDate when no adjustment was applied.
	NominalDate Date
	// Note explains any adjustment or collision, and is empty when neither
	// happened. It is persisted with the occurrence.
	Note string
}

// Slots returns every occurrence whose *business date* falls in [from, to],
// inclusive.
//
// The window is stated in business dates, not nominal ones. A nominal date
// outside the window that adjusts into it is included; a nominal date inside it
// that adjusts out is not. This is the reading materialization needs -- "which
// files am I expecting between these two dates" -- and it is why the scan below
// runs over a widened range before clipping.
//
// The result is sorted by business date and contains no duplicates. Both
// properties matter: two nominal dates can adjust onto one business day (a
// MONTHLY:LAST rule with PRECEDING over a long holiday weekend, for instance),
// and the occurrence table permits one row per contract per business date. The
// collision is merged here, with a note, rather than being left for a unique
// constraint to reject at insert time -- a constraint violation is not a place
// to discover that two of a contract's deliveries landed on the same day.
func (r Rule) Slots(from, to Date, cal Calendar, adjust Adjust) ([]Slot, error) {
	if to.Before(from) {
		return nil, fmt.Errorf("schedule window ends (%s) before it starts (%s)", to, from)
	}
	if cal == nil {
		return nil, fmt.Errorf("a schedule needs a calendar")
	}

	// EVERY_BUSINESS_DAY nominates every date, so the calendar is the whole
	// filter and adjustment is meaningless for it. Honouring FOLLOWING here
	// would move Saturday and Sunday onto Monday, which already has its own
	// occurrence: the rule would collapse a weekend into a phantom triple
	// delivery. The adjustment is overridden rather than rejected, because a
	// stored contract that pairs the two is a configuration mistake and not a
	// reason to stop scheduling that partner altogether.
	if r.Kind == KindEveryBusinessDay {
		adjust = AdjustSkip
	}

	// Nominal dates are collected from a window widened by one adjustment
	// span. A date just outside the window can adjust into it, and dropping it
	// would leave a hole exactly at the window edge -- which, because the
	// scheduler materializes in rolling windows, is a hole that appears on
	// every run boundary.
	span := 0
	if adjust != AdjustSkip {
		span = maxCalendarSearchDays
	}

	byDate := make(map[string]Slot)
	order := make([]string, 0)

	add := func(s Slot) {
		key := s.BusinessDate.String()
		if existing, ok := byDate[key]; ok {
			if existing.NominalDate.Equal(s.NominalDate) {
				return
			}
			merged := existing
			merged.Note = joinNotes(existing.Note, fmt.Sprintf(
				"schedule collision: %s and %s both resolve to this business day",
				existing.NominalDate, s.NominalDate))
			byDate[key] = merged
			return
		}
		byDate[key] = s
		order = append(order, key)
	}

	for d := from.AddDays(-span); !d.After(to.AddDays(span)); d = d.AddDays(1) {
		if !r.names(d) {
			continue
		}
		status := cal.Classify(d)
		slot := Slot{BusinessDate: d, NominalDate: d}

		if !status.Business() {
			switch adjust {
			case AdjustSkip:
				continue
			case AdjustPreceding:
				moved, ok := PreviousBusinessDay(cal, d)
				if !ok {
					return nil, fmt.Errorf(
						"calendar %q has no business day within %d days before %s",
						cal.ID(), maxCalendarSearchDays, d)
				}
				slot.BusinessDate = moved
			case AdjustFollowing:
				moved, ok := NextBusinessDay(cal, d)
				if !ok {
					return nil, fmt.Errorf(
						"calendar %q has no business day within %d days after %s",
						cal.ID(), maxCalendarSearchDays, d)
				}
				slot.BusinessDate = moved
			default:
				return nil, fmt.Errorf("unknown non-business-day action %q", adjust)
			}
			slot.Note = fmt.Sprintf("nominal %s is not a business day (%s); moved %s to %s",
				d, status.Reason, strings.ToLower(string(adjust)), slot.BusinessDate)
		}

		// Only now clip to the requested window, on the adjusted date.
		if slot.BusinessDate.Before(from) || slot.BusinessDate.After(to) {
			continue
		}
		add(slot)
	}

	sort.Strings(order)
	out := make([]Slot, 0, len(order))
	for _, k := range order {
		out = append(out, byDate[k])
	}
	sort.Slice(out, func(i, j int) bool { return out[i].BusinessDate.Before(out[j].BusinessDate) })
	return out, nil
}

// names reports whether the rule nominates this civil date, before any calendar
// adjustment. EVERY_BUSINESS_DAY nominates every date and lets the calendar do
// the filtering, which is why it ignores the adjustment entirely.
func (r Rule) names(d Date) bool {
	switch r.Kind {
	case KindEveryBusinessDay:
		return true
	case KindWeekly:
		for _, wd := range r.Weekdays {
			if d.Weekday() == wd {
				return true
			}
		}
		return false
	case KindMonthly:
		last := d.LastDayOfMonth()
		for _, n := range r.Days {
			if n == -1 && d.Day == last {
				return true
			}
			if n == d.Day {
				return true
			}
		}
		return false
	}
	return false
}

func joinNotes(a, b string) string {
	switch {
	case a == "":
		return b
	case b == "":
		return a
	default:
		return a + "; " + b
	}
}
