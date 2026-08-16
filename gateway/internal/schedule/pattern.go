package schedule

import (
	"fmt"
	"strings"
	"unicode"
)

// Pattern matches an arriving filename or object name against a contract.
//
// It is deliberately not a regular expression. Three reasons, in order of how
// much they cost when ignored:
//
//  1. A pattern is tenant-supplied configuration. Go's regexp is RE2 and does
//     not backtrack, so it is not a catastrophic-backtracking risk, but it is
//     still a full language handed to a config field, and a pattern like `.*`
//     silently matches every feed the tenant has -- turning the ambiguity path
//     below into the normal path.
//  2. The people who write these are operations staff reading a partner's
//     spec sheet. `ACH_*.txt` is a thing they can check by eye; the equivalent
//     regex is a thing they can get subtly wrong, and a pattern that matches
//     one character too few produces a permanently missing file.
//  3. The date tokens below are the point. Most partner filenames carry the
//     business date, and matching it is what makes an arrival attributable to
//     a specific occurrence rather than to whichever one is open.
//
// The accepted syntax, where "star" is the character * :
//
//	star      any run of characters, excluding a path separator
//	?         exactly one character, excluding a path separator
//	{YYYY}    four-digit year of the occurrence's business date
//	{YY}      two-digit year
//	{MM}      two-digit month
//	{DD}      two-digit day
//	{JJJ}     three-digit day of year
//
// Anything else matches itself, case-insensitively.
type Pattern struct {
	raw   string
	parts []patternPart
}

type patternPartKind int

const (
	partLiteral patternPartKind = iota
	partAny                     // *
	partOne                     // ?
	partToken                   // {YYYY} and friends
)

type patternPart struct {
	kind    patternPartKind
	literal string
	token   string
}

// Limits on a pattern, bounding both parse and match cost. The matcher below is
// O(len(name) * len(parts)) in the worst case, so both ends are capped.
const (
	maxPatternLength  = 256
	maxPatternWildcds = 8
)

// ParsePattern validates and compiles a filename matcher.
func ParsePattern(s string) (Pattern, error) {
	if strings.TrimSpace(s) == "" {
		return Pattern{}, fmt.Errorf("a contract version needs a filename pattern")
	}
	if len(s) > maxPatternLength {
		return Pattern{}, fmt.Errorf("filename pattern is %d characters, the limit is %d", len(s), maxPatternLength)
	}
	// A pattern that reaches across directories is refused. Object keys are
	// assigned by this system (internal/objectstore), not by the partner, so a
	// pattern has no legitimate reason to contain a separator -- and one that
	// does is either a mistake or an attempt to match on a path the partner
	// controls.
	if strings.ContainsAny(s, `/\`) {
		return Pattern{}, fmt.Errorf("filename pattern %q contains a path separator", s)
	}
	for _, r := range s {
		if unicode.IsControl(r) {
			return Pattern{}, fmt.Errorf("filename pattern contains a control character")
		}
	}

	p := Pattern{raw: s}
	wildcards := 0
	var lit strings.Builder
	flush := func() {
		if lit.Len() > 0 {
			p.parts = append(p.parts, patternPart{kind: partLiteral, literal: strings.ToLower(lit.String())})
			lit.Reset()
		}
	}

	for i := 0; i < len(s); {
		switch c := s[i]; c {
		case '*':
			flush()
			// Consecutive stars are collapsed. `**` matches exactly what `*`
			// matches, and leaving both in multiplies match cost for nothing.
			if n := len(p.parts); n == 0 || p.parts[n-1].kind != partAny {
				p.parts = append(p.parts, patternPart{kind: partAny})
				wildcards++
			}
			i++
		case '?':
			flush()
			p.parts = append(p.parts, patternPart{kind: partOne})
			wildcards++
			i++
		case '{':
			end := strings.IndexByte(s[i:], '}')
			if end < 0 {
				return Pattern{}, fmt.Errorf("filename pattern has an unclosed '{' at position %d", i)
			}
			token := s[i+1 : i+end]
			if _, ok := tokenWidths[token]; !ok {
				return Pattern{}, fmt.Errorf(
					"filename pattern has unknown token {%s}; known tokens are {YYYY} {YY} {MM} {DD} {JJJ}", token)
			}
			flush()
			p.parts = append(p.parts, patternPart{kind: partToken, token: token})
			i += end + 1
		default:
			lit.WriteByte(c)
			i++
		}
	}
	flush()

	if wildcards > maxPatternWildcds {
		return Pattern{}, fmt.Errorf("filename pattern has %d wildcards, the limit is %d", wildcards, maxPatternWildcds)
	}
	// A pattern that is nothing but wildcards matches every file the tenant
	// ever receives. It is refused rather than allowed, because the resulting
	// behaviour -- every arrival ambiguous against every open occurrence -- is
	// indistinguishable from the system being broken.
	if !hasLiteralOrToken(p.parts) {
		return Pattern{}, fmt.Errorf(
			"filename pattern %q matches every filename; it needs at least one literal or date token", s)
	}
	return p, nil
}

func hasLiteralOrToken(parts []patternPart) bool {
	for _, p := range parts {
		if p.kind == partLiteral || p.kind == partToken {
			return true
		}
	}
	return false
}

var tokenWidths = map[string]int{"YYYY": 4, "YY": 2, "MM": 2, "DD": 2, "JJJ": 3}

// String returns the pattern as written.
func (p Pattern) String() string { return p.raw }

// UsesDate reports whether the pattern pins a business date. A pattern that
// does not is the reason an arrival can be ambiguous across days.
func (p Pattern) UsesDate() bool {
	for _, part := range p.parts {
		if part.kind == partToken {
			return true
		}
	}
	return false
}

// Match reports whether name is the file this pattern expects for date d.
//
// Matching is case-insensitive. Partners change the case of their filenames
// without telling anyone, and a case-sensitive miss presents as a missing file
// -- an alert about the wrong thing that also hides the arrival.
func (p Pattern) Match(name string, d Date) bool {
	name = strings.ToLower(strings.TrimSpace(name))
	return matchParts(p.expand(d), name)
}

// expand resolves date tokens into literals for a specific business date.
func (p Pattern) expand(d Date) []patternPart {
	out := make([]patternPart, 0, len(p.parts))
	for _, part := range p.parts {
		if part.kind != partToken {
			out = append(out, part)
			continue
		}
		out = append(out, patternPart{kind: partLiteral, literal: strings.ToLower(expandToken(part.token, d))})
	}
	return out
}

func expandToken(token string, d Date) string {
	switch token {
	case "YYYY":
		return fmt.Sprintf("%04d", d.Year)
	case "YY":
		return fmt.Sprintf("%02d", d.Year%100)
	case "MM":
		return fmt.Sprintf("%02d", int(d.Month))
	case "DD":
		return fmt.Sprintf("%02d", d.Day)
	case "JJJ":
		jan1 := NewDate(d.Year, 1, 1)
		return fmt.Sprintf("%03d", jan1.DaysUntil(d)+1)
	}
	return ""
}

// matchParts is an iterative glob match with a single backtrack point.
//
// The recursive formulation is the usual one and it is exponential on inputs
// like `a*a*a*a*b` against a run of a's. This form keeps one saved position per
// star and is linear in practice and O(n*m) in the worst case, with no stack
// growth from a config value.
func matchParts(parts []patternPart, s string) bool {
	pi, si := 0, 0
	starPart, starStr := -1, 0

	for si < len(s) {
		if pi < len(parts) {
			switch parts[pi].kind {
			case partAny:
				starPart, starStr = pi, si
				pi++
				continue
			case partOne:
				if !isSeparator(s[si]) {
					pi++
					si++
					continue
				}
			case partLiteral:
				if strings.HasPrefix(s[si:], parts[pi].literal) {
					si += len(parts[pi].literal)
					pi++
					continue
				}
			}
		}
		if starPart >= 0 {
			// Give the last star one more character and retry from there.
			starStr++
			if starStr > len(s) || isSeparator(s[starStr-1]) {
				return false
			}
			pi, si = starPart+1, starStr
			continue
		}
		return false
	}

	for pi < len(parts) && parts[pi].kind == partAny {
		pi++
	}
	return pi == len(parts)
}

func isSeparator(c byte) bool { return c == '/' || c == '\\' }
