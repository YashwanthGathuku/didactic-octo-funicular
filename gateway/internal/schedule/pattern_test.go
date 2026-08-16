package schedule

import (
	"strings"
	"testing"
	"time"
)

func TestPatternMatchesLiteralsAndWildcards(t *testing.T) {
	d := NewDate(2025, time.June, 10)
	cases := []struct {
		pattern string
		name    string
		want    bool
	}{
		{"ACH_*.txt", "ACH_morning.txt", true},
		{"ACH_*.txt", "ACH_.txt", true},
		{"ACH_*.txt", "ach_MORNING.TXT", true}, // case-insensitive
		{"ACH_*.txt", "PAYROLL_morning.txt", false},
		{"ACH_*.txt", "ACH_morning.csv", false},
		{"ACH_??.txt", "ACH_01.txt", true},
		{"ACH_??.txt", "ACH_1.txt", false},
		{"ACH_??.txt", "ACH_001.txt", false},
		{"payroll.ach", "payroll.ach", true},
		{"payroll.ach", "payroll.ach.bak", false},
		{"*payroll*", "acme-payroll-final", true},
	}
	for _, c := range cases {
		p, err := ParsePattern(c.pattern)
		if err != nil {
			t.Fatalf("ParsePattern(%q): %v", c.pattern, err)
		}
		if got := p.Match(c.name, d); got != c.want {
			t.Errorf("%q.Match(%q) = %v, want %v", c.pattern, c.name, got, c.want)
		}
	}
}

func TestPatternDateTokensPinTheBusinessDate(t *testing.T) {
	p, err := ParsePattern("ACH_{YYYY}{MM}{DD}.txt")
	if err != nil {
		t.Fatal(err)
	}
	if !p.UsesDate() {
		t.Error("a pattern with date tokens must report that it pins a date")
	}

	tenth := NewDate(2025, time.June, 10)
	eleventh := NewDate(2025, time.June, 11)

	if !p.Match("ACH_20250610.txt", tenth) {
		t.Error("the file for the 10th must match the 10th's occurrence")
	}
	// The whole value of the token: the same filename must not satisfy a
	// different day's occurrence.
	if p.Match("ACH_20250610.txt", eleventh) {
		t.Error("the file for the 10th must not satisfy the 11th's occurrence")
	}
}

func TestPatternDayOfYearToken(t *testing.T) {
	p, err := ParsePattern("FEED{JJJ}.dat")
	if err != nil {
		t.Fatal(err)
	}
	// 1 March 2024 is day 61 in a leap year, day 60 otherwise.
	if !p.Match("FEED061.dat", NewDate(2024, time.March, 1)) {
		t.Error("2024 is a leap year: 1 March is day 61")
	}
	if !p.Match("FEED060.dat", NewDate(2025, time.March, 1)) {
		t.Error("2025 is not a leap year: 1 March is day 60")
	}
	if !p.Match("FEED001.dat", NewDate(2025, time.January, 1)) {
		t.Error("1 January is day 001")
	}
	if !p.Match("FEED365.dat", NewDate(2025, time.December, 31)) {
		t.Error("31 December 2025 is day 365")
	}
}

func TestPatternTwoDigitYear(t *testing.T) {
	p, err := ParsePattern("x{YY}{MM}{DD}")
	if err != nil {
		t.Fatal(err)
	}
	if !p.Match("x250610", NewDate(2025, time.June, 10)) {
		t.Error("two-digit year should render as 25")
	}
	if !p.Match("x060102", NewDate(2006, time.January, 2)) {
		t.Error("2006 should render as 06, not 6")
	}
}

// A pattern is tenant configuration; the failure modes below are worth
// refusing at write time rather than discovering at match time.
func TestPatternRefusesDangerousAndUselessForms(t *testing.T) {
	for _, bad := range []struct {
		pattern string
		why     string
	}{
		{"", "empty"},
		{"   ", "blank"},
		{"*", "matches every filename the tenant ever receives"},
		{"**", "same, written differently"},
		{"?*?", "wildcards only"},
		{"../etc/passwd", "path separator"},
		{`sub\dir`, "windows path separator"},
		{"ACH_{YYYY", "unclosed token"},
		{"ACH_{MONTH}.txt", "unknown token"},
		{"a*b*c*d*e*f*g*h*i*j*k", "too many wildcards"},
		{strings.Repeat("a", maxPatternLength+1), "too long"},
		{"ACH\x00.txt", "control character"},
	} {
		if _, err := ParsePattern(bad.pattern); err == nil {
			t.Errorf("ParsePattern(%q) must fail (%s)", bad.pattern, bad.why)
		}
	}
}

// The recursive glob formulation is exponential on this input. The iterative
// one is not, and a pattern is a config value so the difference is reachable.
func TestPatternMatchDoesNotBlowUpOnAdversarialInput(t *testing.T) {
	p, err := ParsePattern("a*a*a*a*a*a*a*b")
	if err != nil {
		t.Fatal(err)
	}
	name := strings.Repeat("a", 4000)

	done := make(chan bool, 1)
	go func() { done <- p.Match(name, NewDate(2025, time.June, 10)) }()

	select {
	case got := <-done:
		if got {
			t.Error("the pattern ends in b and the name does not; match must be false")
		}
	case <-time.After(5 * time.Second):
		t.Fatal("matching 4000 characters against eight wildcards did not finish in five seconds")
	}
}

func TestPatternWildcardDoesNotCrossSeparators(t *testing.T) {
	// Separators are refused in patterns, but a filename could still contain
	// one if normalisation upstream ever changed. A star must not span it.
	p, err := ParsePattern("ACH_*.txt")
	if err != nil {
		t.Fatal(err)
	}
	if p.Match("ACH_../../etc/passwd.txt", NewDate(2025, time.June, 10)) {
		t.Error("a wildcard must not match across a path separator")
	}
}
