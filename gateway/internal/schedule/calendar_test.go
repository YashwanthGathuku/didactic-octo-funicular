package schedule

import (
	"testing"
	"time"
)

// The Federal Reserve's published observed holiday dates.
//
// These are entered from the Federal Reserve's published Bank holiday schedule
// and are the fixture the encoded rules are checked against. They are not
// derived from the rules -- a fixture derived from the code under test proves
// only that the code equals itself.
//
// 2026 and 2027 are included because they contain the cases that break a naive
// implementation: two holidays fall on a Saturday and, under the Reserve Banks'
// own rule, are not observed at all.
var publishedFedHolidays = map[int][]string{
	2024: {
		"2024-01-01", // New Year's Day
		"2024-01-15", // Martin Luther King, Jr. Day
		"2024-02-19", // Washington's Birthday
		"2024-05-27", // Memorial Day
		"2024-06-19", // Juneteenth
		"2024-07-04", // Independence Day
		"2024-09-02", // Labor Day
		"2024-10-14", // Columbus Day
		"2024-11-11", // Veterans Day
		"2024-11-28", // Thanksgiving Day
		"2024-12-25", // Christmas Day
	},
	2025: {
		"2025-01-01",
		"2025-01-20",
		"2025-02-17",
		"2025-05-26",
		"2025-06-19",
		"2025-07-04",
		"2025-09-01",
		"2025-10-13",
		"2025-11-11",
		"2025-11-27",
		"2025-12-25",
	},
	2026: {
		"2026-01-01",
		"2026-01-19",
		"2026-02-16",
		"2026-05-25",
		"2026-06-19",
		// Independence Day 2026 falls on Saturday 4 July. Reserve Bank offices
		// are open the preceding Friday and the holiday is not observed, so
		// July has no entry at all.
		"2026-09-07",
		"2026-10-12",
		"2026-11-11",
		"2026-11-26",
		"2026-12-25",
	},
	2027: {
		"2027-01-01",
		"2027-01-18",
		"2027-02-15",
		"2027-05-31",
		// Juneteenth 2027 falls on Saturday 19 June: not observed.
		"2027-07-05", // 4 July is a Sunday, so the Monday is observed
		"2027-09-06",
		"2027-10-11",
		"2027-11-11",
		"2027-11-25",
		// Christmas Day 2027 falls on Saturday 25 December: not observed.
	},
}

func TestFederalReserveHolidaysMatchThePublishedSchedule(t *testing.T) {
	cal := federalReserveCalendar{}

	for year, published := range publishedFedHolidays {
		want := map[string]bool{}
		for _, d := range published {
			want[d] = true
		}

		// Walk every day of the year, so an extra holiday is caught as well as
		// a missing one. Checking only the published dates would pass an
		// implementation that closes the whole of December.
		got := map[string]bool{}
		for d := NewDate(year, time.January, 1); d.Year == year; d = d.AddDays(1) {
			status := cal.Classify(d)
			if status.Kind == KindHoliday {
				got[d.String()] = true
			}
		}

		for d := range want {
			if !got[d] {
				t.Errorf("%d: %s is a published Federal Reserve holiday but the calendar calls it a business day", year, d)
			}
		}
		for d := range got {
			if !want[d] {
				t.Errorf("%d: the calendar closes on %s, which is not a published Federal Reserve holiday", year, d)
			}
		}
	}
}

// The Reserve Banks' Saturday rule is not the federal employee rule, and
// getting them the wrong way round suppresses a real expectation.
func TestSaturdayHolidayLeavesThePrecedingFridayOpen(t *testing.T) {
	cal := federalReserveCalendar{}

	// 4 July 2026 is a Saturday.
	july4 := NewDate(2026, time.July, 4)
	if july4.Weekday() != time.Saturday {
		t.Fatalf("fixture is wrong: 2026-07-04 is a %s", july4.Weekday())
	}

	friday := NewDate(2026, time.July, 3)
	status := cal.Classify(friday)
	if !status.Business() {
		t.Errorf("Friday 2026-07-03 must remain a business day (Reserve Bank offices are open "+
			"the Friday before a Saturday holiday); got %s: %s", status.Kind, status.Reason)
	}
	if s := cal.Classify(july4); s.Kind != KindWeekend {
		t.Errorf("Saturday 2026-07-04 should classify as a weekend, got %s", s.Kind)
	}
	// The following Monday is a normal business day: the Reserve Banks do not
	// roll a Saturday holiday forward.
	if s := cal.Classify(NewDate(2026, time.July, 6)); !s.Business() {
		t.Errorf("Monday 2026-07-06 must be a business day, got %s: %s", s.Kind, s.Reason)
	}
}

func TestSundayHolidayIsObservedTheFollowingMonday(t *testing.T) {
	cal := federalReserveCalendar{}

	july4 := NewDate(2027, time.July, 4)
	if july4.Weekday() != time.Sunday {
		t.Fatalf("fixture is wrong: 2027-07-04 is a %s", july4.Weekday())
	}
	monday := NewDate(2027, time.July, 5)
	s := cal.Classify(monday)
	if s.Kind != KindHoliday {
		t.Errorf("Monday 2027-07-05 must be observed as Independence Day, got %s", s.Kind)
	}
	if s.Holiday != "Independence Day" {
		t.Errorf("observed holiday name = %q, want Independence Day", s.Holiday)
	}
}

func TestJuneteenthIsNotAppliedBeforeItExisted(t *testing.T) {
	cal := federalReserveCalendar{}

	// 19 June 2019 was a Wednesday, and Juneteenth was not a federal holiday.
	d := NewDate(2019, time.June, 19)
	if d.Weekday() != time.Wednesday {
		t.Fatalf("fixture is wrong: 2019-06-19 is a %s", d.Weekday())
	}
	if s := cal.Classify(d); !s.Business() {
		t.Errorf("2019-06-19 predates Juneteenth as a federal holiday and must be a business day, got %s", s.Kind)
	}
	// 2022 was the first year Reserve Bank offices actually closed for it.
	if s := cal.Classify(NewDate(2022, time.June, 20)); s.Kind != KindHoliday {
		t.Errorf("2022-06-20 must be the observed Juneteenth (19 June 2022 was a Sunday), got %s", s.Kind)
	}
}

func TestTenantOverridesBothOpenAndCloseDays(t *testing.T) {
	base, err := BaseCalendar(BaseFederalReserve)
	if err != nil {
		t.Fatal(err)
	}
	cal, err := NewOverlayCalendar("acme", base, []Override{
		{Date: NewDate(2025, time.December, 26), Open: false, Reason: "office closed between Christmas and New Year"},
		{Date: NewDate(2025, time.November, 27), Open: true, Reason: "partner delivers on Thanksgiving by agreement"},
	})
	if err != nil {
		t.Fatal(err)
	}

	// A normal business day the tenant closes.
	closed := cal.Classify(NewDate(2025, time.December, 26))
	if closed.Business() {
		t.Error("2025-12-26 was overridden closed and must not be a business day")
	}
	if closed.Kind != KindOverrideClosed {
		t.Errorf("kind = %s, want %s", closed.Kind, KindOverrideClosed)
	}

	// A published holiday the tenant opens.
	open := cal.Classify(NewDate(2025, time.November, 27))
	if !open.Business() {
		t.Error("2025-11-27 was overridden open and must be a business day")
	}
	if open.Kind != KindOverrideOpen {
		t.Errorf("kind = %s, want %s", open.Kind, KindOverrideOpen)
	}

	// Everything else still follows the base.
	if s := cal.Classify(NewDate(2025, time.December, 25)); s.Kind != KindHoliday {
		t.Errorf("Christmas Day is unaffected by the overrides, got %s", s.Kind)
	}
}

func TestAnOverrideWithoutAReasonIsRefused(t *testing.T) {
	base, _ := BaseCalendar(BaseWeekdays)
	_, err := NewOverlayCalendar("acme", base, []Override{
		{Date: NewDate(2025, time.July, 7), Open: false},
	})
	if err == nil {
		t.Fatal("an override with no reason must be refused: an unexplained closure suppresses " +
			"an expectation and is indistinguishable from a mistake")
	}
}

func TestNextAndPreviousBusinessDaySkipHolidayWeekends(t *testing.T) {
	cal := federalReserveCalendar{}

	// Thanksgiving 2025 is Thursday 27 November. The next business day is the
	// Friday -- the Reserve Banks do not close for the day after.
	next, ok := NextBusinessDay(cal, NewDate(2025, time.November, 27))
	if !ok || next.String() != "2025-11-28" {
		t.Errorf("next business day after Thanksgiving 2025 = %s (ok=%v), want 2025-11-28", next, ok)
	}

	// Christmas Day 2025 is a Thursday; the 26th is a Friday and open.
	prev, ok := PreviousBusinessDay(cal, NewDate(2025, time.December, 25))
	if !ok || prev.String() != "2025-12-24" {
		t.Errorf("previous business day before Christmas 2025 = %s (ok=%v), want 2025-12-24", prev, ok)
	}

	// Across a holiday Monday: Monday 2025-01-20 is MLK Day, so the previous
	// business day from Tuesday is the preceding Friday.
	prev, ok = PreviousBusinessDay(cal, NewDate(2025, time.January, 21))
	if !ok || prev.String() != "2025-01-17" {
		t.Errorf("previous business day before 2025-01-21 = %s (ok=%v), want 2025-01-17", prev, ok)
	}
}

// A calendar that closes every day must not hang the scheduler.
func TestBusinessDaySearchIsBounded(t *testing.T) {
	base, _ := BaseCalendar(BaseWeekdays)
	var overrides []Override
	for d := NewDate(2025, time.January, 1); d.Year == 2025; d = d.AddDays(1) {
		overrides = append(overrides, Override{Date: d, Open: false, Reason: "site shut for the year"})
	}
	cal, err := NewOverlayCalendar("shut", base, overrides)
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := NextBusinessDay(cal, NewDate(2025, time.June, 1)); ok {
		t.Error("a calendar with no business days must report that it found none, not return one")
	}
}

func TestLastDayOfMonthHandlesLeapYears(t *testing.T) {
	for _, tc := range []struct {
		date Date
		want int
	}{
		{NewDate(2024, time.February, 1), 29}, // leap
		{NewDate(2025, time.February, 1), 28},
		{NewDate(2000, time.February, 1), 29}, // divisible by 400
		{NewDate(1900, time.February, 1), 28}, // divisible by 100, not 400
		{NewDate(2025, time.December, 1), 31},
		{NewDate(2025, time.April, 1), 30},
	} {
		if got := tc.date.LastDayOfMonth(); got != tc.want {
			t.Errorf("%s: last day = %d, want %d", tc.date, got, tc.want)
		}
	}
}
