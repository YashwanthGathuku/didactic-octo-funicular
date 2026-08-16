package schedule

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"
)

// dialect selects the database-specific statements.
type dialect int

const (
	dialectPostgres dialect = iota
	dialectSQLite
)

// Store is the persistence boundary for contracts, calendars and occurrences.
type Store struct {
	db      *sql.DB
	dialect dialect
	now     func() time.Time

	// escalator is what a breach means beyond opening an incident. Optional:
	// the incident and the notification intent are written either way, because
	// those are domain records rather than an integration.
	escalator Escalator
}

// NewStore builds a Store.
//
// The driver is named explicitly, as it is for internal/jobs and
// internal/ledger. Insert-returning-id and upsert syntax differ, and a wrong
// guess produces a scheduler that appears to work.
func NewStore(db *sql.DB, driverName string) (*Store, error) {
	if db == nil {
		return nil, errors.New("the scheduler requires a database handle")
	}
	var d dialect
	switch {
	case strings.Contains(driverName, "pgx"), strings.Contains(driverName, "postgres"):
		d = dialectPostgres
	case strings.Contains(driverName, "sqlite"):
		d = dialectSQLite
	default:
		return nil, fmt.Errorf("unsupported driver %q for the scheduler", driverName)
	}
	return &Store{db: db, dialect: d, now: time.Now}, nil
}

// SetClock replaces the time source, for tests.
func (s *Store) SetClock(fn func() time.Time) { s.now = fn }

// Now returns the store's clock reading in UTC.
func (s *Store) Now() time.Time { return s.now().UTC() }

func (s *Store) rebind(query string) string {
	if s.dialect != dialectPostgres {
		return query
	}
	var b strings.Builder
	n := 0
	for _, r := range query {
		if r == '?' {
			n++
			fmt.Fprintf(&b, "$%d", n)
			continue
		}
		b.WriteRune(r)
	}
	return b.String()
}

// ---------------------------------------------------------------------------
// Calendars
// ---------------------------------------------------------------------------

// CreateCalendar records a tenant calendar built on a published base.
func (s *Store) CreateCalendar(ctx context.Context, tenantID, calendarID, name string, base Base) error {
	if strings.TrimSpace(tenantID) == "" {
		return errors.New("a calendar needs a tenant")
	}
	if _, err := BaseCalendar(base); err != nil {
		return err
	}
	_, err := s.db.ExecContext(ctx, s.rebind(`
		INSERT INTO business_calendars (tenant_id, calendar_id, name, base)
		VALUES (?, ?, ?, ?)
		ON CONFLICT (tenant_id, calendar_id) DO NOTHING`),
		tenantID, calendarID, name, string(base))
	return err
}

// SetOverride records or replaces a tenant-specific calendar correction.
func (s *Store) SetOverride(ctx context.Context, tenantID, calendarID string, o Override) error {
	if strings.TrimSpace(o.Reason) == "" {
		return fmt.Errorf("the override on %s needs a reason", o.Date)
	}
	kind := "HOLIDAY"
	if o.Open {
		kind = "BUSINESS_DAY"
	}
	_, err := s.db.ExecContext(ctx, s.rebind(`
		INSERT INTO business_calendar_overrides (tenant_id, calendar_id, calendar_date, kind, reason)
		VALUES (?, ?, ?, ?, ?)
		ON CONFLICT (tenant_id, calendar_id, calendar_date)
		DO UPDATE SET kind = EXCLUDED.kind, reason = EXCLUDED.reason`),
		tenantID, calendarID, o.Date.utc(), kind, o.Reason)
	return err
}

// Calendar loads a tenant calendar with its overrides applied.
func (s *Store) Calendar(ctx context.Context, tenantID, calendarID string) (Calendar, error) {
	var baseName string
	err := s.db.QueryRowContext(ctx, s.rebind(`
		SELECT base FROM business_calendars WHERE tenant_id = ? AND calendar_id = ?`),
		tenantID, calendarID).Scan(&baseName)
	if errors.Is(err, sql.ErrNoRows) {
		// Refused rather than defaulted. Falling back to a weekday calendar
		// would put a partner's Christmas Day expectation into the queue, and
		// the operator who mistyped the calendar id would see a plausible
		// schedule rather than an error.
		return nil, fmt.Errorf("tenant %s has no calendar %q", tenantID, calendarID)
	}
	if err != nil {
		return nil, err
	}
	base, err := ParseBase(baseName)
	if err != nil {
		return nil, err
	}
	baseCal, err := BaseCalendar(base)
	if err != nil {
		return nil, err
	}

	rows, err := s.db.QueryContext(ctx, s.rebind(`
		SELECT calendar_date, kind, reason
		FROM business_calendar_overrides
		WHERE tenant_id = ? AND calendar_id = ?
		ORDER BY calendar_date`), tenantID, calendarID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var overrides []Override
	for rows.Next() {
		var when time.Time
		var kind, reason string
		if err := rows.Scan(&when, &kind, &reason); err != nil {
			return nil, err
		}
		overrides = append(overrides, Override{
			Date:   DateOf(when, time.UTC),
			Open:   kind == "BUSINESS_DAY",
			Reason: reason,
		})
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return NewOverlayCalendar(calendarID, baseCal, overrides)
}

// ---------------------------------------------------------------------------
// Contract versions
// ---------------------------------------------------------------------------

// CreateVersion writes a new immutable contract version.
//
// Editing terms means calling this, never updating a row. The previous open
// version is closed at the new one's activation date, so the two form a
// half-open interval and every business date resolves to exactly one version.
//
// A version cannot be inserted at or before the currently open one's activation
// date. Doing so would silently rewrite which terms governed dates that have
// already been scheduled and possibly already breached -- history would change
// underneath an occurrence whose evidence has been exported.
func (s *Store) CreateVersion(ctx context.Context, in NewVersionInput) (Version, error) {
	// The calendar must exist before a contract can reference it. Creating the
	// version first and discovering the calendar is missing at materialize
	// time would leave a contract that silently produces no occurrences.
	base, err := s.calendarBase(ctx, in.TenantID, in.CalendarID)
	if err != nil {
		return Version{}, err
	}
	// Overrides are not needed to validate the terms; the calendar is loaded
	// with them at materialize time, where the dates being classified are
	// known.
	v, _, err := in.Validate(nil, base)
	if err != nil {
		return Version{}, err
	}

	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return Version{}, err
	}
	defer func() { _ = tx.Rollback() }()

	var maxVersion sql.NullInt64
	var openFrom sql.NullTime
	if err := tx.QueryRowContext(ctx, s.rebind(`
		SELECT MAX(version) FROM file_contract_versions
		WHERE tenant_id = ? AND contract_id = ?`),
		in.TenantID, in.ContractID).Scan(&maxVersion); err != nil {
		return Version{}, err
	}
	err = tx.QueryRowContext(ctx, s.rebind(`
		SELECT effective_from FROM file_contract_versions
		WHERE tenant_id = ? AND contract_id = ? AND effective_to IS NULL
		ORDER BY version DESC`),
		in.TenantID, in.ContractID).Scan(&openFrom)
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return Version{}, err
	}
	if openFrom.Valid {
		prior := DateOf(openFrom.Time, time.UTC)
		if !prior.Before(v.EffectiveFrom) {
			return Version{}, fmt.Errorf(
				"contract %d already has a version effective from %s; a new version must start later than %s",
				in.ContractID, prior, prior)
		}
		if _, err := tx.ExecContext(ctx, s.rebind(`
			UPDATE file_contract_versions SET effective_to = ?
			WHERE tenant_id = ? AND contract_id = ? AND effective_to IS NULL`),
			v.EffectiveFrom.utc(), in.TenantID, in.ContractID); err != nil {
			return Version{}, err
		}
	}

	v.Version = int(maxVersion.Int64) + 1
	var effectiveTo any
	if v.EffectiveTo != nil {
		effectiveTo = v.EffectiveTo.utc()
	}

	const insert = `
		INSERT INTO file_contract_versions
			(tenant_id, contract_id, version, feed_id, direction, filename_pattern, format,
			 expected_local, timezone, grace_minutes, breach_after_minutes, calendar_id,
			 schedule_rule, nonbusiness_action, balanced_mode, owner_subject,
			 escalation_policy_id, effective_from, effective_to)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`
	args := []any{
		v.TenantID, v.ContractID, v.Version, v.FeedID, string(v.Direction), v.Pattern.String(), v.Format,
		v.ExpectedLocal.String(), v.Timezone, v.GraceMinutes, v.BreachMinutes, v.CalendarID,
		v.Rule.String(), string(v.Adjust), string(v.BalancedMode), v.OwnerSubject,
		v.EscalationPolicyID, v.EffectiveFrom.utc(), effectiveTo,
	}

	if s.dialect == dialectPostgres {
		if err := tx.QueryRowContext(ctx, s.rebind(insert+" RETURNING id"), args...).Scan(&v.ID); err != nil {
			return Version{}, err
		}
	} else {
		res, err := tx.ExecContext(ctx, insert, args...)
		if err != nil {
			return Version{}, err
		}
		if v.ID, err = res.LastInsertId(); err != nil {
			return Version{}, err
		}
	}

	if err := tx.Commit(); err != nil {
		return Version{}, err
	}
	return v, nil
}

func (s *Store) calendarBase(ctx context.Context, tenantID, calendarID string) (Base, error) {
	var name string
	err := s.db.QueryRowContext(ctx, s.rebind(
		`SELECT base FROM business_calendars WHERE tenant_id = ? AND calendar_id = ?`),
		tenantID, calendarID).Scan(&name)
	if errors.Is(err, sql.ErrNoRows) {
		return "", fmt.Errorf("tenant %s has no calendar %q", tenantID, calendarID)
	}
	if err != nil {
		return "", err
	}
	return ParseBase(name)
}

const versionColumns = `
	v.id, v.tenant_id, v.contract_id, v.version, v.feed_id, v.direction,
	v.filename_pattern, v.format, v.expected_local, v.timezone, v.grace_minutes,
	v.breach_after_minutes, v.calendar_id, v.schedule_rule, v.nonbusiness_action,
	v.balanced_mode, v.owner_subject, v.escalation_policy_id,
	v.effective_from, v.effective_to, c.partner_id`

func scanVersion(sc interface{ Scan(...any) error }) (Version, error) {
	var (
		v          Version
		direction  string
		pattern    string
		expected   string
		rule       string
		adjust     string
		mode       string
		from       time.Time
		to         sql.NullTime
		partnerID  sql.NullInt64
		feedID     string
		ownerSub   string
		escalation string
	)
	err := sc.Scan(
		&v.ID, &v.TenantID, &v.ContractID, &v.Version, &feedID, &direction,
		&pattern, &v.Format, &expected, &v.Timezone, &v.GraceMinutes,
		&v.BreachMinutes, &v.CalendarID, &rule, &adjust,
		&mode, &ownerSub, &escalation, &from, &to, &partnerID)
	if err != nil {
		return Version{}, err
	}

	// Every parse failure here names the version, because the row was written
	// by an older build or by hand and the operator needs to know which one.
	fail := func(err error) (Version, error) {
		return Version{}, fmt.Errorf("contract %d version %d is not usable: %w", v.ContractID, v.Version, err)
	}
	if v.Pattern, err = ParsePattern(pattern); err != nil {
		return fail(err)
	}
	if v.ExpectedLocal, err = ParseLocalTime(expected); err != nil {
		return fail(err)
	}
	if v.Location, err = LoadLocation(v.Timezone); err != nil {
		return fail(err)
	}
	if v.Rule, err = ParseRule(rule); err != nil {
		return fail(err)
	}
	if v.Adjust, err = ParseAdjust(adjust); err != nil {
		return fail(err)
	}
	v.Direction = Direction(direction)
	v.BalancedMode = BalancedMode(mode)
	v.FeedID = feedID
	v.OwnerSubject = ownerSub
	v.EscalationPolicyID = escalation
	v.PartnerID = partnerID.Int64
	v.EffectiveFrom = DateOf(from, time.UTC)
	if to.Valid {
		d := DateOf(to.Time, time.UTC)
		v.EffectiveTo = &d
	}
	return v, nil
}

// VersionOn resolves the contract version that governed a business date.
//
// This is what makes history stable. An occurrence stores the version id it was
// materialized under, and a report that re-derives it asks this question with
// the occurrence's own business date -- so editing today's terms cannot change
// whether last month's file was late.
func (s *Store) VersionOn(ctx context.Context, tenantID string, contractID int64, d Date) (Version, error) {
	row := s.db.QueryRowContext(ctx, s.rebind(`
		SELECT `+versionColumns+`
		FROM file_contract_versions v
		JOIN file_contracts c ON c.id = v.contract_id AND c.tenant_id = v.tenant_id
		WHERE v.tenant_id = ? AND v.contract_id = ?
		  AND v.effective_from <= ?
		  AND (v.effective_to IS NULL OR v.effective_to > ?)
		ORDER BY v.effective_from DESC, v.version DESC`),
		tenantID, contractID, d.utc(), d.utc())
	v, err := scanVersion(row)
	if errors.Is(err, sql.ErrNoRows) {
		return Version{}, fmt.Errorf("contract %d on %s: %w", contractID, d, ErrNoVersion)
	}
	return v, err
}

// VersionByID loads one version by primary key, for resolving an occurrence
// back to the terms it was created under.
func (s *Store) VersionByID(ctx context.Context, tenantID string, id int64) (Version, error) {
	row := s.db.QueryRowContext(ctx, s.rebind(`
		SELECT `+versionColumns+`
		FROM file_contract_versions v
		JOIN file_contracts c ON c.id = v.contract_id AND c.tenant_id = v.tenant_id
		WHERE v.tenant_id = ? AND v.id = ?`), tenantID, id)
	return scanVersion(row)
}

// allVersions returns every contract version, for materialization.
func (s *Store) allVersions(ctx context.Context) ([]Version, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT `+versionColumns+`
		FROM file_contract_versions v
		JOIN file_contracts c ON c.id = v.contract_id AND c.tenant_id = v.tenant_id
		ORDER BY v.tenant_id, v.contract_id, v.effective_from, v.version`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []Version
	for rows.Next() {
		v, err := scanVersion(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, v)
	}
	return out, rows.Err()
}
