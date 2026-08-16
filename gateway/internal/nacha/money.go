package nacha

import (
	"errors"
	"fmt"
	"math"
	"strconv"
	"strings"
)

// Amount is a signed count of minor units -- cents, for USD.
//
// Never a float. The previous implementation computed totals as
// `float64(cents) / 100.0` and compared debit and credit sums for equality,
// which is a correctness defect rather than a style preference: 0.1 + 0.2 is not
// 0.3 in binary floating point, and a file with enough entries can report itself
// unbalanced when it balances, or balanced when it does not.
//
// An ACH amount field is already in minor units -- ten characters, right
// justified, zero filled, with two implied decimals -- so the conversion this
// type replaces was introducing a representation the format never used.
type Amount int64

// ErrOverflow means an accumulation exceeded the representable range.
//
// It is a blocking condition rather than a wrap. A wrapped total can equal a
// declared total and silently balance a file that does not balance, which is
// exactly the class of defect this package exists to prevent.
var ErrOverflow = errors.New("amount accumulation overflowed")

// ErrNotNumeric means a field defined as numeric contained something else.
var ErrNotNumeric = errors.New("field is not numeric")

// ParseAmount reads a fixed-width, zero-filled minor-unit field.
//
// It deliberately takes no width parameter. Amount fields in this format come in
// three widths -- 10 characters on an entry detail record, 12 on a batch control
// and 12 on a file control -- and an earlier version of this function capped the
// input at the entry width, which rejected every valid control record's totals
// as an overflow. A width check here would be re-deriving information the caller
// already has from the field positions it passed; the real bound is what an
// int64 of minor units can hold, and that is what ParseInt enforces.
//
// Spaces are accepted as padding, because some originators emit space-padded
// rather than zero-filled fields and rejecting those would be stricter than the
// format requires. An empty or all-space field is zero.
func ParseAmount(field string) (Amount, error) {
	trimmed := strings.TrimSpace(field)
	if trimmed == "" {
		return 0, nil
	}
	for _, r := range trimmed {
		if r < '0' || r > '9' {
			return 0, ErrNotNumeric
		}
	}
	n, err := strconv.ParseInt(trimmed, 10, 64)
	if err != nil {
		// The only remaining failure is a value beyond int64, which for minor
		// units is an amount no legitimate file carries.
		return 0, fmt.Errorf("%w: %v", ErrOverflow, err)
	}
	return Amount(n), nil
}

// Add returns the sum, refusing to wrap.
//
// Go's integer addition wraps silently on overflow. For a running total of
// payment amounts that produces a number that looks plausible and is wrong, so
// this checks before committing.
func (a Amount) Add(b Amount) (Amount, error) {
	if b > 0 && a > math.MaxInt64-b {
		return 0, ErrOverflow
	}
	if b < 0 && a < math.MinInt64-b {
		return 0, ErrOverflow
	}
	return a + b, nil
}

// String renders the amount as currency with two decimals.
//
// It formats from the integer directly rather than dividing, so the rendering
// cannot introduce the imprecision the type exists to avoid.
func (a Amount) String() string {
	neg := a < 0
	v := int64(a)
	if neg {
		v = -v
	}
	s := fmt.Sprintf("%d.%02d", v/100, v%100)
	if neg {
		return "-" + s
	}
	return s
}

// Minor returns the raw minor-unit count, for storage and comparison.
func (a Amount) Minor() int64 { return int64(a) }

// Count is a non-negative record or batch count read from a control record.
//
// It is a distinct type from Amount so a count and a total cannot be compared
// or added by accident -- both are integers in this format and confusing them
// produces a validator that passes.
type Count int64

// ParseCount reads a fixed-width, zero-filled count field.
func ParseCount(field string) (Count, error) {
	trimmed := strings.TrimSpace(field)
	if trimmed == "" {
		return 0, nil
	}
	for _, r := range trimmed {
		if r < '0' || r > '9' {
			return 0, ErrNotNumeric
		}
	}
	n, err := strconv.ParseInt(trimmed, 10, 64)
	if err != nil {
		return 0, fmt.Errorf("%w: %v", ErrOverflow, err)
	}
	return Count(n), nil
}

// Add returns the sum, refusing to wrap.
func (c Count) Add(b Count) (Count, error) {
	if b > 0 && c > math.MaxInt64-b {
		return 0, ErrOverflow
	}
	return c + b, nil
}

// entryHashModulus truncates the entry hash to its low-order ten digits, which
// is what the format specifies when the sum exceeds the field width.
const entryHashModulus = 10_000_000_000

// EntryHash accumulates the sum of routing-number prefixes.
//
// The truncation is part of the specified calculation, not an overflow: the
// field is ten characters, and a file with enough entries legitimately carries
// a sum larger than that. Accumulating in an int64 and reducing at the end
// keeps the intermediate exact.
type EntryHash int64

// AddRouting adds the first eight digits of a routing number.
func (h EntryHash) AddRouting(routing string) (EntryHash, error) {
	trimmed := strings.TrimSpace(routing)
	if len(trimmed) < 8 {
		return h, fmt.Errorf("%w: routing number is %d characters", ErrNotNumeric, len(trimmed))
	}
	prefix := trimmed[:8]
	for _, r := range prefix {
		if r < '0' || r > '9' {
			return h, ErrNotNumeric
		}
	}
	n, err := strconv.ParseInt(prefix, 10, 64)
	if err != nil {
		return h, ErrNotNumeric
	}
	if h > math.MaxInt64-EntryHash(n) {
		return 0, ErrOverflow
	}
	return h + EntryHash(n), nil
}

// Truncated reduces the accumulated sum to the ten-digit field value.
func (h EntryHash) Truncated() int64 { return int64(h) % entryHashModulus }

// ValidateRoutingCheckDigit applies the ABA check-digit calculation.
//
// This is arithmetic on the number itself, not a rule-set requirement, which is
// why it can be a blocking rule in a repository with no licensed rule source: a
// routing number that fails it is malformed under its own definition.
func ValidateRoutingCheckDigit(routing string) error {
	trimmed := strings.TrimSpace(routing)
	if len(trimmed) != 9 {
		return fmt.Errorf("routing number must be 9 digits, got %d", len(trimmed))
	}
	weights := [9]int{3, 7, 1, 3, 7, 1, 3, 7, 1}
	sum := 0
	for i, r := range trimmed {
		if r < '0' || r > '9' {
			return ErrNotNumeric
		}
		sum += int(r-'0') * weights[i]
	}
	if sum%10 != 0 {
		return errors.New("routing number check digit is invalid")
	}
	return nil
}
