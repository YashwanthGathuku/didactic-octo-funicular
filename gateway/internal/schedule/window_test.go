package schedule

import (
	"testing"
	"time"

	"sentinel-gateway/internal/domain"
)

func mustLocation(t *testing.T, name string) *time.Location {
	t.Helper()
	loc, err := LoadLocation(name)
	if err != nil {
		t.Skipf("timezone database unavailable for %s: %v", name, err)
	}
	return loc
}

// Spring forward: 02:30 local does not exist on 9 March 2025 in New York.
//
// Go's time.Date answers 03:30 without saying it moved anything, which puts the
// deadline an hour later than the contract says with nothing in the record to
// explain it. The contracted moment must resolve to the instant the clock
// reaches the far side of the gap, and the resolution must be reported.
func TestSpringForwardGapIsResolvedExplicitly(t *testing.T) {
	ny := mustLocation(t, "America/New_York")
	d := NewDate(2025, time.March, 9)

	instant, res := ResolveLocal(d, LocalTime{Hour: 2, Minute: 30}, ny)
	if res != ResolutionGap {
		t.Fatalf("resolution = %s, want %s: 02:30 does not exist on 2025-03-09 in New York", res, ResolutionGap)
	}

	local := instant.In(ny)
	if got := local.Format("15:04:05"); got != "03:00:00" {
		t.Errorf("deadline resolved to %s local, want 03:00:00 -- the first instant at which "+
			"the contracted time can be said to have passed", got)
	}
	// Stated as an absolute instant: 03:00 EDT is 07:00 UTC.
	if want := time.Date(2025, time.March, 9, 7, 0, 0, 0, time.UTC); !instant.Equal(want) {
		t.Errorf("deadline = %s, want %s", instant.UTC(), want)
	}

	// Go's own answer, recorded here so the difference is deliberate and
	// visible rather than something a future reader has to rediscover.
	if naive := time.Date(2025, 3, 9, 2, 30, 0, 0, ny); naive.In(ny).Hour() != 3 || naive.In(ny).Minute() != 30 {
		t.Logf("note: time.Date normalised 02:30 to %s", naive.In(ny).Format("15:04:05"))
	}
}

// Fall back: 01:30 local occurs twice on 2 November 2025 in New York.
func TestFallBackAmbiguityTakesTheEarlierInstant(t *testing.T) {
	ny := mustLocation(t, "America/New_York")
	d := NewDate(2025, time.November, 2)

	instant, res := ResolveLocal(d, LocalTime{Hour: 1, Minute: 30}, ny)
	if res != ResolutionAmbiguous {
		t.Fatalf("resolution = %s, want %s: 01:30 occurs twice on 2025-11-02 in New York", res, ResolutionAmbiguous)
	}

	// The earlier instant is 01:30 EDT = 05:30 UTC; the later is 01:30 EST =
	// 06:30 UTC. The earlier is the stricter deadline.
	want := time.Date(2025, time.November, 2, 5, 30, 0, 0, time.UTC)
	if !instant.Equal(want) {
		t.Errorf("deadline = %s, want %s (the earlier of the two 01:30s)", instant.UTC(), want)
	}
	if _, offset := instant.In(ny).Zone(); offset != -4*3600 {
		t.Errorf("offset = %d, want -14400 (EDT); the later instant would be EST", offset)
	}
}

func TestOrdinaryDayResolvesExactly(t *testing.T) {
	ny := mustLocation(t, "America/New_York")
	instant, res := ResolveLocal(NewDate(2025, time.June, 10), LocalTime{Hour: 9}, ny)
	if res != ResolutionExact {
		t.Errorf("resolution = %s, want %s", res, ResolutionExact)
	}
	if want := time.Date(2025, time.June, 10, 13, 0, 0, 0, time.UTC); !instant.Equal(want) {
		t.Errorf("09:00 New York on 2025-06-10 = %s, want %s", instant.UTC(), want)
	}
}

// A grace period is an amount of real time, not an amount of clock face.
func TestGraceIsAbsoluteTimeAcrossADstTransition(t *testing.T) {
	ny := mustLocation(t, "America/New_York")

	// Due at 00:30 on fall-back day with a 120-minute grace. In wall-clock
	// terms the grace appears to end at 01:30 because an hour repeats; in real
	// time it ends two hours after it began.
	w, err := WindowFor(NewDate(2025, time.November, 2), Timing{
		ExpectedLocal: LocalTime{Hour: 0, Minute: 30},
		Location:      ny,
		GraceMinutes:  120,
		BreachMinutes: 60,
	})
	if err != nil {
		t.Fatal(err)
	}
	if got := w.GraceEndsAt.Sub(w.DueAt); got != 120*time.Minute {
		t.Errorf("grace lasted %s of real time, want 2h; a wall-clock addition would "+
			"have given the partner an extra hour", got)
	}
	if got := w.BreachesAt.Sub(w.GraceEndsAt); got != 60*time.Minute {
		t.Errorf("breach delay = %s, want 1h", got)
	}
	// The grace ends at the *second* 01:30 of the day, in EST. A wall-clock
	// addition of two hours to 00:30 would have landed on the first 01:30, in
	// EDT, one real hour earlier -- so the offset is what distinguishes the
	// correct answer from the plausible wrong one.
	if got := w.GraceEndsAt.In(ny).Format("15:04 MST"); got != "01:30 EST" {
		t.Errorf("grace ends at %s local, want 01:30 EST (the second 01:30); "+
			"01:30 EDT would mean the addition was done on the clock face", got)
	}
}

func TestWindowRecordsTheDstNote(t *testing.T) {
	ny := mustLocation(t, "America/New_York")
	w, err := WindowFor(NewDate(2025, time.March, 9), Timing{
		ExpectedLocal: LocalTime{Hour: 2, Minute: 30},
		Location:      ny,
		GraceMinutes:  30,
		BreachMinutes: 30,
	})
	if err != nil {
		t.Fatal(err)
	}
	if w.Note == "" {
		t.Error("a window resolved across a DST gap must carry a note; the alternative is a " +
			"deadline that moved by an hour with no record of why")
	}
	if w.Resolution != ResolutionGap {
		t.Errorf("resolution = %s, want %s", w.Resolution, ResolutionGap)
	}
}

// Zones that shift at midnight break a naive search that bounds itself at local
// midnight, because midnight is itself the non-existent time.
func TestMidnightShiftingZoneResolves(t *testing.T) {
	beirut := mustLocation(t, "Asia/Beirut")
	// Beirut moves the clock forward at midnight on the last Sunday of March.
	d := NewDate(2025, time.March, 30)
	instant, res := ResolveLocal(d, LocalTime{Hour: 0, Minute: 30}, beirut)
	if res != ResolutionGap {
		t.Skipf("2025-03-30 is not a midnight transition in this tzdata build (resolution %s)", res)
	}
	if got := instant.In(beirut).Format("15:04:05"); got != "01:00:00" {
		t.Errorf("deadline resolved to %s, want 01:00:00 (the far side of the midnight gap)", got)
	}
}

func TestTimezoneAbbreviationsAreRefused(t *testing.T) {
	for _, name := range []string{"EST", "PST", "UTC-5", "", "  "} {
		if _, err := LoadLocation(name); err == nil {
			t.Errorf("timezone %q must be refused: an abbreviation names a fixed offset and "+
				"does not observe daylight saving", name)
		}
	}
	for _, name := range []string{"America/New_York", "Europe/London", "UTC"} {
		if _, err := LoadLocation(name); err != nil {
			t.Errorf("timezone %q should load: %v", name, err)
		}
	}
}

func TestLocalTimeParsing(t *testing.T) {
	for _, bad := range []string{"9:00", "09", "09:00:00:00", "24:00:00", "09:60", "aa:bb", ""} {
		if _, err := ParseLocalTime(bad); err == nil {
			t.Errorf("ParseLocalTime(%q) must fail", bad)
		}
	}
	got, err := ParseLocalTime("09:30")
	if err != nil {
		t.Fatal(err)
	}
	if got.String() != "09:30:00" {
		t.Errorf("09:30 parsed to %s", got)
	}
}

// ---------------------------------------------------------------------------
// State evaluation
// ---------------------------------------------------------------------------

func TestEvaluateBoundariesAreHalfOpen(t *testing.T) {
	base := time.Date(2025, time.June, 10, 13, 0, 0, 0, time.UTC)
	w := Window{
		DueAt:       base,
		GraceEndsAt: base.Add(30 * time.Minute),
		BreachesAt:  base.Add(90 * time.Minute),
	}

	cases := []struct {
		at   time.Time
		want domain.ExpectationState
		why  string
	}{
		{base.Add(-time.Nanosecond), domain.ExpectationPending, "an instant before the deadline"},
		{base, domain.ExpectationDue, "exactly at the deadline: due, not late"},
		{base.Add(29 * time.Minute), domain.ExpectationDue, "inside grace"},
		{base.Add(30 * time.Minute), domain.ExpectationOverdue, "grace has ended"},
		{base.Add(89 * time.Minute), domain.ExpectationOverdue, "before the breach threshold"},
		{base.Add(90 * time.Minute), domain.ExpectationBreached, "at the breach threshold"},
		{base.Add(365 * 24 * time.Hour), domain.ExpectationBreached, "long after"},
	}
	for _, c := range cases {
		if got := Evaluate(w, c.at); got != c.want {
			t.Errorf("%s: state = %s, want %s", c.why, got, c.want)
		}
	}
}

// A file that never arrives reaches BREACHED with no input of any kind.
func TestAFileThatNeverArrivesBreachesFromTheClockAlone(t *testing.T) {
	ny := mustLocation(t, "America/New_York")
	w, err := WindowFor(NewDate(2025, time.June, 10), Timing{
		ExpectedLocal: LocalTime{Hour: 9},
		Location:      ny,
		GraceMinutes:  30,
		BreachMinutes: 60,
	})
	if err != nil {
		t.Fatal(err)
	}
	// No arrival event, no history, nothing consulted.
	if got := Evaluate(w, w.BreachesAt.Add(time.Second)); got != domain.ExpectationBreached {
		t.Errorf("state = %s, want BREACHED", got)
	}
}

func TestPathWalksEveryIntermediateState(t *testing.T) {
	path, err := PathTo(domain.ExpectationPending, domain.ExpectationBreached)
	if err != nil {
		t.Fatal(err)
	}
	want := []domain.ExpectationState{
		domain.ExpectationDue, domain.ExpectationOverdue, domain.ExpectationBreached,
	}
	if len(path) != len(want) {
		t.Fatalf("path = %v, want %v: a scheduler that was down over a weekend must not write "+
			"BREACHED straight onto PENDING", path, want)
	}
	for i := range want {
		if path[i] != want[i] {
			t.Fatalf("path = %v, want %v", path, want)
		}
	}

	// Every step is a legal edge of the Prompt 03 state machine.
	prev := domain.ExpectationPending
	for _, next := range path {
		if !domain.CanTransitionExpectation(prev, next) {
			t.Fatalf("illegal edge %s -> %s", prev, next)
		}
		prev = next
	}
}

func TestTerminalStatesDoNotAge(t *testing.T) {
	for _, s := range []domain.ExpectationState{domain.ExpectationArrived, domain.ExpectationWaived} {
		if Ageing(s) {
			t.Errorf("%s must not be moved by the clock", s)
		}
		path, err := PathTo(s, domain.ExpectationBreached)
		if err != nil {
			t.Fatal(err)
		}
		if len(path) != 0 {
			t.Errorf("%s produced path %v; a satisfied occurrence must not breach", s, path)
		}
	}
}

func TestBackwardsPathIsEmpty(t *testing.T) {
	path, err := PathTo(domain.ExpectationBreached, domain.ExpectationDue)
	if err != nil {
		t.Fatal(err)
	}
	if len(path) != 0 {
		t.Errorf("path = %v; occurrences never move backwards", path)
	}
}
