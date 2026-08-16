package schedule

import (
	"errors"
	"fmt"
	"strings"
	"time"
)

// Direction is which way a feed moves.
type Direction string

const (
	// Inbound is a file a partner sends to this tenant.
	Inbound Direction = "INBOUND"
	// Outbound is a file this tenant sends to a partner.
	Outbound Direction = "OUTBOUND"
)

// BalancedMode records whether the contract authorises an unbalanced file.
//
// It lives on the contract rather than in the validator's defaults because it
// is a commercial term, not a technical one: an unbalanced file is a rule
// violation for most partners and a normal delivery for the few whose agreement
// says so. Prompt 07 applied one default to every tenant, which meant either
// failing the authorised partners or passing the unauthorised ones.
type BalancedMode string

const (
	// Balanced requires offsetting entries.
	Balanced BalancedMode = "BALANCED"
	// UnbalancedAuthorized permits their absence, by written agreement.
	UnbalancedAuthorized BalancedMode = "UNBALANCED_AUTHORIZED"
)

// ErrNoVersion is returned when no contract version was in effect on a date.
var ErrNoVersion = errors.New("no contract version was active on that date")

// Version is a fully parsed, validated contract version.
//
// Every field that has a parsed form carries the parsed form. A version that
// reaches this struct cannot contain an unparseable timezone or an unknown
// schedule rule, so the scheduler has no error path for bad configuration in
// its inner loop -- the error is raised when the version is written or loaded,
// where there is someone to tell.
type Version struct {
	ID         int64
	TenantID   string
	ContractID int64
	Version    int

	// Identity of the feed. FeedID is the tenant's own name for it and is what
	// appears in an alert; a partner is frequently the counterparty for
	// several feeds and "a file from Acme is late" is not actionable.
	PartnerID int64
	FeedID    string
	Direction Direction

	Pattern Pattern
	Format  string

	ExpectedLocal LocalTime
	Timezone      string
	Location      *time.Location
	GraceMinutes  int
	BreachMinutes int

	CalendarID string
	Rule       Rule
	Adjust     Adjust

	BalancedMode       BalancedMode
	OwnerSubject       string
	EscalationPolicyID string

	EffectiveFrom Date
	EffectiveTo   *Date // nil is an open interval
	CreatedAt     time.Time
}

// Timing returns the part of the version that derives deadlines.
func (v Version) Timing() Timing {
	return Timing{
		ExpectedLocal: v.ExpectedLocal,
		Location:      v.Location,
		GraceMinutes:  v.GraceMinutes,
		BreachMinutes: v.BreachMinutes,
	}
}

// ActiveOn reports whether this version governs a business date.
//
// The interval is half-open, [EffectiveFrom, EffectiveTo). A closed interval
// would make the last day of the old version and the first day of the new one
// the same day, and an occurrence on that day would resolve to whichever row
// the database returned first.
func (v Version) ActiveOn(d Date) bool {
	if d.Before(v.EffectiveFrom) {
		return false
	}
	if v.EffectiveTo != nil && !d.Before(*v.EffectiveTo) {
		return false
	}
	return true
}

// NewVersionInput is the unvalidated form, as supplied by configuration or an
// API request.
type NewVersionInput struct {
	TenantID   string
	ContractID int64
	PartnerID  int64
	FeedID     string
	Direction  string

	FilenamePattern string
	Format          string

	ExpectedLocal string
	Timezone      string
	GraceMinutes  int
	BreachMinutes int

	CalendarID       string
	ScheduleRule     string
	NonBusinessDay   string
	BalancedMode     string
	OwnerSubject     string
	EscalationPolicy string

	EffectiveFrom Date
	EffectiveTo   *Date
}

// productionFormat is the only format a contract may name for production use.
//
// ISO 20022, BAI2 and SWIFT parsers exist in this repository and are labelled
// experimental in docs/engineering/SCOPE.md. Allowing a contract to name one
// would put an experimental parser on the deterministic path for a real
// partner, which is precisely the claim Prompt 01 removed.
const productionFormat = "NACHA"

// Validate parses and checks an input, returning the strongly typed version.
//
// Every check here is one that would otherwise become a silently wrong
// schedule. There is no partial acceptance: a version either governs an
// unambiguous set of deadlines or it is refused.
func (in NewVersionInput) Validate(overrides []Override, base Base) (Version, Calendar, error) {
	var v Version

	if strings.TrimSpace(in.TenantID) == "" {
		return v, nil, errors.New("a contract version needs a tenant")
	}
	if in.ContractID == 0 {
		return v, nil, errors.New("a contract version needs a contract")
	}
	if strings.TrimSpace(in.FeedID) == "" {
		return v, nil, errors.New("a contract version needs a feed id")
	}

	dir := Direction(strings.ToUpper(strings.TrimSpace(in.Direction)))
	if dir != Inbound && dir != Outbound {
		return v, nil, fmt.Errorf("direction must be INBOUND or OUTBOUND, got %q", in.Direction)
	}

	pattern, err := ParsePattern(in.FilenamePattern)
	if err != nil {
		return v, nil, err
	}

	format := strings.ToUpper(strings.TrimSpace(in.Format))
	if format == "" {
		format = productionFormat
	}
	if format != productionFormat {
		return v, nil, fmt.Errorf(
			"format %q cannot be contracted; only %s is supported for production feeds "+
				"(see docs/engineering/SCOPE.md for the experimental parsers)", format, productionFormat)
	}

	expected, err := ParseLocalTime(in.ExpectedLocal)
	if err != nil {
		return v, nil, err
	}
	loc, err := LoadLocation(in.Timezone)
	if err != nil {
		return v, nil, err
	}
	if in.GraceMinutes < 0 {
		return v, nil, fmt.Errorf("grace period cannot be negative, got %d", in.GraceMinutes)
	}
	// A breach delay of zero would make OVERDUE unreachable: the occurrence
	// would pass from DUE straight to BREACHED at one instant, and the state
	// that exists to be escalated on would never be observed by a scheduler
	// running at any finite interval.
	if in.BreachMinutes < 1 {
		return v, nil, fmt.Errorf(
			"breach delay must be at least one minute, got %d; "+
				"zero would make OVERDUE unreachable", in.BreachMinutes)
	}

	rule, err := ParseRule(in.ScheduleRule)
	if err != nil {
		return v, nil, err
	}
	adjust, err := ParseAdjust(in.NonBusinessDay)
	if err != nil {
		return v, nil, err
	}

	mode := BalancedMode(strings.ToUpper(strings.TrimSpace(in.BalancedMode)))
	if mode != Balanced && mode != UnbalancedAuthorized {
		return v, nil, fmt.Errorf("balanced mode must be BALANCED or UNBALANCED_AUTHORIZED, got %q", in.BalancedMode)
	}

	// The owner is required. An expectation with no owner produces an alert
	// with no recipient, which is the same as no alert -- and the absence is
	// only discovered on the day a file is missing.
	if strings.TrimSpace(in.OwnerSubject) == "" {
		return v, nil, errors.New("a contract version needs an owner to escalate to")
	}
	if strings.TrimSpace(in.EscalationPolicy) == "" {
		return v, nil, errors.New("a contract version needs an escalation policy reference")
	}

	if in.EffectiveFrom.Zero() {
		return v, nil, errors.New("a contract version needs an activation date")
	}
	if in.EffectiveTo != nil && !in.EffectiveFrom.Before(*in.EffectiveTo) {
		return v, nil, fmt.Errorf("contract version is effective from %s to %s, which is not an interval",
			in.EffectiveFrom, *in.EffectiveTo)
	}

	calendarID := strings.TrimSpace(in.CalendarID)
	if calendarID == "" {
		return v, nil, errors.New("a contract version needs a business calendar")
	}
	baseCal, err := BaseCalendar(base)
	if err != nil {
		return v, nil, err
	}
	cal, err := NewOverlayCalendar(calendarID, baseCal, overrides)
	if err != nil {
		return v, nil, err
	}

	v = Version{
		TenantID:           in.TenantID,
		ContractID:         in.ContractID,
		PartnerID:          in.PartnerID,
		FeedID:             strings.TrimSpace(in.FeedID),
		Direction:          dir,
		Pattern:            pattern,
		Format:             format,
		ExpectedLocal:      expected,
		Timezone:           in.Timezone,
		Location:           loc,
		GraceMinutes:       in.GraceMinutes,
		BreachMinutes:      in.BreachMinutes,
		CalendarID:         calendarID,
		Rule:               rule,
		Adjust:             adjust,
		BalancedMode:       mode,
		OwnerSubject:       strings.TrimSpace(in.OwnerSubject),
		EscalationPolicyID: strings.TrimSpace(in.EscalationPolicy),
		EffectiveFrom:      in.EffectiveFrom,
		EffectiveTo:        in.EffectiveTo,
	}
	return v, cal, nil
}
