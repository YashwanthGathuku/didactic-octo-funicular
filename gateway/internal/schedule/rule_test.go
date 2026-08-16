package schedule

import (
	"testing"
	"time"
)

func fedCalendar(t *testing.T) Calendar {
	t.Helper()
	cal, err := BaseCalendar(BaseFederalReserve)
	if err != nil {
		t.Fatal(err)
	}
	return cal
}

func slotDates(slots []Slot) []string {
	out := make([]string, len(slots))
	for i, s := range slots {
		out[i] = s.BusinessDate.String()
	}
	return out
}

func mustRule(t *testing.T, s string) Rule {
	t.Helper()
	r, err := ParseRule(s)
	if err != nil {
		t.Fatalf("ParseRule(%q): %v", s, err)
	}
	return r
}

func TestEveryBusinessDaySkipsWeekendsAndHolidays(t *testing.T) {
	cal := fedCalendar(t)
	// Week containing Thanksgiving 2025 (Thursday 27 November).
	slots, err := mustRule(t, "EVERY_BUSINESS_DAY").Slots(
		NewDate(2025, time.November, 24), NewDate(2025, time.November, 30), cal, AdjustSkip)
	if err != nil {
		t.Fatal(err)
	}
	want := []string{"2025-11-24", "2025-11-25", "2025-11-26", "2025-11-28"}
	got := slotDates(slots)
	if len(got) != len(want) {
		t.Fatalf("dates = %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("dates = %v, want %v", got, want)
		}
	}
}

// EVERY_BUSINESS_DAY with FOLLOWING would fold Saturday and Sunday onto Monday,
// which already has an occurrence of its own.
func TestEveryBusinessDayIgnoresTheAdjustment(t *testing.T) {
	cal := fedCalendar(t)
	skip, err := mustRule(t, "EVERY_BUSINESS_DAY").Slots(
		NewDate(2025, time.June, 6), NewDate(2025, time.June, 10), cal, AdjustSkip)
	if err != nil {
		t.Fatal(err)
	}
	following, err := mustRule(t, "EVERY_BUSINESS_DAY").Slots(
		NewDate(2025, time.June, 6), NewDate(2025, time.June, 10), cal, AdjustFollowing)
	if err != nil {
		t.Fatal(err)
	}
	if len(skip) != len(following) {
		t.Errorf("SKIP produced %v and FOLLOWING produced %v; a daily rule must not multiply "+
			"the weekend onto Monday", slotDates(skip), slotDates(following))
	}
}

func TestWeeklyRuleSelectsNamedDays(t *testing.T) {
	cal := fedCalendar(t)
	slots, err := mustRule(t, "WEEKLY:MON,THU").Slots(
		NewDate(2025, time.June, 1), NewDate(2025, time.June, 14), cal, AdjustSkip)
	if err != nil {
		t.Fatal(err)
	}
	want := []string{"2025-06-02", "2025-06-05", "2025-06-09", "2025-06-12"}
	if got := slotDates(slots); len(got) != len(want) {
		t.Fatalf("dates = %v, want %v", got, want)
	}
	for i, d := range slotDates(slots) {
		if d != want[i] {
			t.Fatalf("dates = %v, want %v", slotDates(slots), want)
		}
	}
}

// A weekly file due on a holiday Monday, with the common banking convention of
// moving to the preceding business day.
func TestWeeklyOnAnObservedHolidayMovesToThePrecedingBusinessDay(t *testing.T) {
	cal := fedCalendar(t)
	// Monday 2025-01-20 is Martin Luther King, Jr. Day. The window is stated in
	// business dates -- Slots returns occurrences whose business date lands
	// inside it -- so it spans the week containing both the nominal Monday and
	// the Friday the occurrence moves to.
	slots, err := mustRule(t, "WEEKLY:MON").Slots(
		NewDate(2025, time.January, 13), NewDate(2025, time.January, 24), cal, AdjustPreceding)
	if err != nil {
		t.Fatal(err)
	}
	if len(slots) != 2 {
		t.Fatalf("dates = %v, want the 13th and the moved 17th", slotDates(slots))
	}
	slots = slots[1:]
	if slots[0].BusinessDate.String() != "2025-01-17" {
		t.Errorf("business date = %s, want 2025-01-17 (the preceding Friday)", slots[0].BusinessDate)
	}
	if slots[0].NominalDate.String() != "2025-01-20" {
		t.Errorf("nominal date = %s, want 2025-01-20", slots[0].NominalDate)
	}
	if slots[0].Note == "" {
		t.Error("a moved occurrence must record why it moved")
	}
}

func TestMonthlyLastHandlesEveryMonthLength(t *testing.T) {
	cal, err := BaseCalendar(BaseAllDays)
	if err != nil {
		t.Fatal(err)
	}
	slots, err := mustRule(t, "MONTHLY:LAST").Slots(
		NewDate(2024, time.January, 1), NewDate(2024, time.April, 30), cal, AdjustSkip)
	if err != nil {
		t.Fatal(err)
	}
	want := []string{"2024-01-31", "2024-02-29", "2024-03-31", "2024-04-30"}
	got := slotDates(slots)
	if len(got) != len(want) {
		t.Fatalf("dates = %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("dates = %v, want %v (February 2024 has 29 days)", got, want)
		}
	}
}

// Month end falling on a weekend, and a year boundary, in one window.
func TestMonthAndYearBoundary(t *testing.T) {
	cal := fedCalendar(t)
	// 31 December 2025 is a Wednesday and a business day; 31 January 2026 is a
	// Saturday, so PRECEDING moves it to Friday the 30th.
	slots, err := mustRule(t, "MONTHLY:LAST").Slots(
		NewDate(2025, time.December, 1), NewDate(2026, time.February, 28), cal, AdjustPreceding)
	if err != nil {
		t.Fatal(err)
	}
	want := []string{"2025-12-31", "2026-01-30", "2026-02-27"}
	got := slotDates(slots)
	if len(got) != len(want) {
		t.Fatalf("dates = %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("dates = %v, want %v", got, want)
		}
	}
	// 28 February 2026 is a Saturday, so the February slot moved to Friday the
	// 27th, and the note says so.
	if slots[2].Note == "" {
		t.Error("the February occurrence moved and must say so")
	}
}

// Two nominal dates can adjust onto the same business day. The occurrence table
// permits one row per contract per date, so this must be resolved here rather
// than by a unique-constraint failure at insert time.
func TestScheduleCollisionIsMergedWithANote(t *testing.T) {
	base, err := BaseCalendar(BaseWeekdays)
	if err != nil {
		t.Fatal(err)
	}
	// Close Monday and Tuesday so both a Monday and a Tuesday delivery fall
	// back onto the preceding Friday.
	cal, err := NewOverlayCalendar("acme", base, []Override{
		{Date: NewDate(2025, time.June, 9), Open: false, Reason: "site closed"},
		{Date: NewDate(2025, time.June, 10), Open: false, Reason: "site closed"},
	})
	if err != nil {
		t.Fatal(err)
	}

	slots, err := mustRule(t, "WEEKLY:MON,TUE").Slots(
		NewDate(2025, time.June, 6), NewDate(2025, time.June, 6), cal, AdjustPreceding)
	if err != nil {
		t.Fatal(err)
	}
	if len(slots) != 1 {
		t.Fatalf("got %d slots for one business day, want 1 merged slot: %v", len(slots), slotDates(slots))
	}
	if slots[0].Note == "" {
		t.Error("a merged collision must be recorded; two contracted deliveries landed on one day")
	}
}

// A date just outside the requested window can adjust into it. Dropping it
// would leave a hole at every rolling-window boundary.
func TestAdjustmentCanCarryADateIntoTheWindow(t *testing.T) {
	cal := fedCalendar(t)
	// Nominal Monday 2025-01-20 is MLK Day and moves back to Friday the 17th.
	// A window covering only the 17th must still see it.
	slots, err := mustRule(t, "WEEKLY:MON").Slots(
		NewDate(2025, time.January, 17), NewDate(2025, time.January, 17), cal, AdjustPreceding)
	if err != nil {
		t.Fatal(err)
	}
	if len(slots) != 1 || slots[0].BusinessDate.String() != "2025-01-17" {
		t.Fatalf("dates = %v, want [2025-01-17]", slotDates(slots))
	}
}

func TestRuleParsing(t *testing.T) {
	for _, bad := range []string{
		"", "DAILY", "WEEKLY", "WEEKLY:MONDAY", "WEEKLY:MON,MON",
		"MONTHLY", "MONTHLY:0", "MONTHLY:32", "MONTHLY:LAST,LAST",
		"EVERY_BUSINESS_DAY:MON", "MONTHLY:abc",
	} {
		if _, err := ParseRule(bad); err == nil {
			t.Errorf("ParseRule(%q) must fail", bad)
		}
	}

	// 29, 30 and 31 are refused rather than clamped: a contract that says "the
	// 31st" has an unanswerable question in it for four months of the year.
	for _, day := range []string{"MONTHLY:29", "MONTHLY:30", "MONTHLY:31"} {
		if _, err := ParseRule(day); err == nil {
			t.Errorf("ParseRule(%q) must fail and point the operator at LAST", day)
		}
	}

	// The canonical form is stable regardless of input order or case.
	r, err := ParseRule("weekly:fri,mon")
	if err != nil {
		t.Fatal(err)
	}
	if r.String() != "WEEKLY:MON,FRI" {
		t.Errorf("canonical form = %q, want WEEKLY:MON,FRI", r.String())
	}
}

func TestSlotsRefuseAnInvertedWindow(t *testing.T) {
	cal := fedCalendar(t)
	if _, err := mustRule(t, "EVERY_BUSINESS_DAY").Slots(
		NewDate(2025, time.June, 10), NewDate(2025, time.June, 1), cal, AdjustSkip); err == nil {
		t.Error("a window that ends before it starts must be refused")
	}
}
