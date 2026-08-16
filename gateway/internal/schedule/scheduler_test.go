package schedule

import (
	"context"
	"database/sql"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"sentinel-gateway/internal/domain"

	_ "github.com/jackc/pgx/v5/stdlib"
	_ "modernc.org/sqlite"
)

// Every scheduling property is checked against both databases.
//
// The upsert that makes materialization idempotent, the optimistic update that
// makes concurrent advancement safe, and the date and timestamp round trips are
// all places where the two genuinely differ -- PostgreSQL's TIMESTAMPTZ and
// DATE types are not SQLite's formatted strings. A suite that ran against one
// would be verifying half the code.
//
// The PostgreSQL half is skipped without SENTINEL_TEST_POSTGRES_DSN, and the
// skip says what went unverified rather than passing quietly.

type backend struct {
	name   string
	driver string
	open   func(t *testing.T) *sql.DB
}

func backends(t *testing.T) []backend {
	t.Helper()
	out := []backend{{name: "sqlite", driver: "sqlite", open: openSQLite}}
	if dsn := os.Getenv("SENTINEL_TEST_POSTGRES_DSN"); dsn != "" {
		out = append(out, backend{name: "postgres", driver: "pgx", open: openPostgres})
	} else {
		t.Log("SKIPPED for postgres: SENTINEL_TEST_POSTGRES_DSN is unset, so date and timestamp " +
			"round trips and the conflict-do-nothing upsert are NOT verified against PostgreSQL by this run")
	}
	return out
}

func openSQLite(t *testing.T) *sql.DB {
	t.Helper()
	// File-backed, not :memory:. In-memory SQLite gives each pooled connection
	// its own database, which would make the concurrency tests below pass for
	// the wrong reason.
	path := filepath.Join(t.TempDir(), "schedule.db")
	db, err := sql.Open("sqlite", path+"?_pragma=busy_timeout(10000)&_pragma=journal_mode(WAL)&_pragma=foreign_keys(1)")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })

	// The real migrations, so the CHECK constraints and unique keys under test
	// are the shipped ones.
	for _, name := range []string{
		"001_init_schema.sql", "002_tenancy_and_state.sql", "003_secret_store.sql",
		"004_artifact_storage.sql", "005_redacted_findings.sql", "006_jobs_and_outbox.sql",
		"007_ledger_integrity.sql", "008_scheduling.sql", "009_breach_escalation.sql",
	} {
		body, err := os.ReadFile(filepath.Join("..", "..", "migrations", name))
		if err != nil {
			t.Fatalf("read %s: %v", name, err)
		}
		if _, err := db.Exec(string(body)); err != nil {
			t.Fatalf("apply %s: %v", name, err)
		}
	}
	seedTenants(t, db)
	return db
}

// openPostgres builds the shape under test in a throwaway schema.
//
// migrations_postgres/ does not yet carry the scheduling tables -- the
// application still runs on SQLite -- so this creates them here rather than
// pretending a port that has not happened. The types are the PostgreSQL ones a
// port would use, which is the point: DATE and TIMESTAMPTZ behave differently
// from SQLite's text columns and that difference is what this half verifies.
func openPostgres(t *testing.T) *sql.DB {
	t.Helper()
	dsn := os.Getenv("SENTINEL_TEST_POSTGRES_DSN")
	db, err := sql.Open("pgx", dsn)
	if err != nil {
		t.Fatal(err)
	}
	if err := db.Ping(); err != nil {
		t.Fatalf("ping postgres: %v", err)
	}
	schema := fmt.Sprintf("sched_test_%d", time.Now().UnixNano())
	if _, err := db.Exec(`CREATE SCHEMA ` + schema); err != nil {
		db.Close()
		t.Fatalf("create schema: %v", err)
	}
	db.Close()

	sep := "?"
	if len(dsn) > 0 && (containsRune(dsn, '?')) {
		sep = "&"
	}
	db, err = sql.Open("pgx", dsn+sep+"search_path="+schema)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		_, _ = db.Exec(`DROP SCHEMA ` + schema + ` CASCADE`)
		db.Close()
	})
	if _, err := db.Exec(postgresScheduleSchema); err != nil {
		t.Fatalf("apply schedule schema: %v", err)
	}
	seedTenants(t, db)
	return db
}

func containsRune(s string, r rune) bool {
	for _, c := range s {
		if c == r {
			return true
		}
	}
	return false
}

const postgresScheduleSchema = `
CREATE TABLE tenants (id TEXT PRIMARY KEY, name TEXT NOT NULL);
CREATE TABLE partners (
    id BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    tenant_id TEXT NOT NULL REFERENCES tenants(id),
    name TEXT NOT NULL,
    routing_number TEXT NOT NULL,
    UNIQUE (tenant_id, routing_number)
);
CREATE TABLE file_contracts (
    id BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    tenant_id TEXT NOT NULL REFERENCES tenants(id),
    partner_id BIGINT NOT NULL REFERENCES partners(id),
    name TEXT NOT NULL,
    direction TEXT NOT NULL CHECK (direction IN ('INBOUND','OUTBOUND')),
    filename_pattern TEXT NOT NULL,
    expected_time TEXT NOT NULL,
    grace_period_minutes INTEGER NOT NULL,
    timezone TEXT NOT NULL
);
CREATE TABLE file_contract_versions (
    id BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    tenant_id TEXT NOT NULL REFERENCES tenants(id),
    contract_id BIGINT NOT NULL REFERENCES file_contracts(id),
    version INTEGER NOT NULL CHECK (version >= 1),
    feed_id TEXT NOT NULL DEFAULT '',
    direction TEXT NOT NULL DEFAULT 'INBOUND',
    filename_pattern TEXT NOT NULL,
    format TEXT NOT NULL DEFAULT 'NACHA',
    expected_local TEXT NOT NULL,
    timezone TEXT NOT NULL,
    grace_minutes INTEGER NOT NULL CHECK (grace_minutes >= 0),
    breach_after_minutes INTEGER NOT NULL DEFAULT 60,
    calendar_id TEXT,
    schedule_rule TEXT NOT NULL DEFAULT 'EVERY_BUSINESS_DAY',
    nonbusiness_action TEXT NOT NULL DEFAULT 'SKIP',
    balanced_mode TEXT NOT NULL DEFAULT 'BALANCED'
        CHECK (balanced_mode IN ('BALANCED','UNBALANCED_AUTHORIZED')),
    owner_subject TEXT NOT NULL DEFAULT '',
    escalation_policy_id TEXT NOT NULL DEFAULT '',
    effective_from DATE NOT NULL,
    effective_to DATE,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE (tenant_id, contract_id, version)
);
CREATE TABLE expectations (
    id BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    tenant_id TEXT NOT NULL REFERENCES tenants(id),
    contract_id BIGINT NOT NULL REFERENCES file_contracts(id),
    contract_version_id BIGINT REFERENCES file_contract_versions(id),
    business_date DATE,
    expected_delivery_start TIMESTAMPTZ NOT NULL,
    expected_delivery_end TIMESTAMPTZ NOT NULL,
    breach_at TIMESTAMPTZ,
    status TEXT NOT NULL CHECK (status IN ('PENDING','DUE','OVERDUE','BREACHED','ARRIVED','WAIVED')),
    matched_artifact_id BIGINT,
    matched_at TIMESTAMPTZ,
    schedule_note TEXT NOT NULL DEFAULT '',
    due_local TEXT NOT NULL DEFAULT '',
    timezone TEXT NOT NULL DEFAULT '',
    review_required INTEGER NOT NULL DEFAULT 0,
    waived_by TEXT,
    waived_reason TEXT,
    waived_at TIMESTAMPTZ,
    row_version BIGINT NOT NULL DEFAULT 0,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE (tenant_id, contract_id, business_date)
);
CREATE TABLE file_instances (
    id BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    tenant_id TEXT NOT NULL REFERENCES tenants(id),
    expectation_id BIGINT REFERENCES expectations(id),
    filename TEXT NOT NULL,
    storage_path TEXT NOT NULL DEFAULT '',
    size_bytes BIGINT NOT NULL DEFAULT 0,
    sha256_hash TEXT NOT NULL DEFAULT '',
    status TEXT NOT NULL DEFAULT 'RECEIVED',
    received_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE TABLE status_history (
    id BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    tenant_id TEXT NOT NULL REFERENCES tenants(id),
    object_type TEXT NOT NULL CHECK (object_type IN ('artifact','expectation','job','decision')),
    object_id BIGINT NOT NULL,
    from_state TEXT NOT NULL,
    to_state TEXT NOT NULL,
    actor_id TEXT NOT NULL,
    reason TEXT,
    occurred_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    CHECK (from_state <> to_state)
);
CREATE TABLE business_calendars (
    tenant_id TEXT NOT NULL REFERENCES tenants(id),
    calendar_id TEXT NOT NULL,
    name TEXT NOT NULL,
    base TEXT NOT NULL CHECK (base IN ('FEDERAL_RESERVE','WEEKDAYS','ALL_DAYS')),
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    PRIMARY KEY (tenant_id, calendar_id)
);
CREATE TABLE business_calendar_overrides (
    tenant_id TEXT NOT NULL REFERENCES tenants(id),
    calendar_id TEXT NOT NULL,
    calendar_date DATE NOT NULL,
    kind TEXT NOT NULL CHECK (kind IN ('HOLIDAY','BUSINESS_DAY')),
    reason TEXT NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    PRIMARY KEY (tenant_id, calendar_id, calendar_date)
);
CREATE TABLE incidents (
    id BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    tenant_id TEXT NOT NULL REFERENCES tenants(id),
    expectation_id BIGINT REFERENCES expectations(id),
    file_instance_id BIGINT,
    type TEXT NOT NULL,
    severity TEXT NOT NULL,
    status TEXT NOT NULL CHECK (status IN ('OPEN','INVESTIGATING','RESOLVED','CLOSED')),
    contract_version_id BIGINT REFERENCES file_contract_versions(id),
    summary TEXT NOT NULL DEFAULT '',
    owner_subject TEXT NOT NULL DEFAULT '',
    escalation_policy_id TEXT NOT NULL DEFAULT '',
    resolved_at TIMESTAMPTZ,
    resolved_by TEXT,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE UNIQUE INDEX idx_incidents_occurrence_type
    ON incidents(tenant_id, expectation_id, type) WHERE expectation_id IS NOT NULL;
CREATE TABLE notification_intents (
    id BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    tenant_id TEXT NOT NULL REFERENCES tenants(id),
    kind TEXT NOT NULL,
    subject_type TEXT NOT NULL,
    subject_id BIGINT NOT NULL,
    payload TEXT NOT NULL,
    dedupe_key TEXT NOT NULL DEFAULT '',
    recipient TEXT NOT NULL DEFAULT '',
    escalation_policy_id TEXT NOT NULL DEFAULT '',
    attempt_count INTEGER NOT NULL DEFAULT 0,
    last_error TEXT,
    delivered_at TIMESTAMPTZ,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE UNIQUE INDEX idx_notification_dedupe
    ON notification_intents(tenant_id, dedupe_key) WHERE dedupe_key <> '';
CREATE TABLE expectation_match_candidates (
    id BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    tenant_id TEXT NOT NULL REFERENCES tenants(id),
    expectation_id BIGINT NOT NULL REFERENCES expectations(id),
    file_instance_id BIGINT NOT NULL REFERENCES file_instances(id),
    filename TEXT NOT NULL,
    reason TEXT NOT NULL,
    resolution TEXT NOT NULL DEFAULT 'REVIEW_REQUIRED'
        CHECK (resolution IN ('REVIEW_REQUIRED','ACCEPTED','REJECTED')),
    resolved_by TEXT,
    resolved_at TIMESTAMPTZ,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE (tenant_id, expectation_id, file_instance_id)
);
`

func seedTenants(t *testing.T, db *sql.DB) {
	t.Helper()
	for _, id := range []string{"TENANT-A", "TENANT-B"} {
		if _, err := db.Exec(`INSERT INTO tenants (id, name) VALUES ($1, $2)`, id, id); err != nil {
			if _, err2 := db.Exec(`INSERT INTO tenants (id, name) VALUES (?, ?)`, id, id); err2 != nil {
				t.Fatalf("seed tenant %s: %v", id, err2)
			}
		}
	}
}

func eachBackend(t *testing.T, fn func(t *testing.T, db *sql.DB, driver string)) {
	t.Helper()
	for _, b := range backends(t) {
		t.Run(b.name, func(t *testing.T) { fn(t, b.open(t), b.driver) })
	}
}

// ---------------------------------------------------------------------------
// Fixtures
// ---------------------------------------------------------------------------

const tenantA = "TENANT-A"

func newStore(t *testing.T, db *sql.DB, driver string, now time.Time) *Store {
	t.Helper()
	s, err := NewStore(db, driver)
	if err != nil {
		t.Fatal(err)
	}
	s.SetClock(func() time.Time { return now })
	return s
}

// newContract creates a partner and a contract shell, returning the contract id.
func newContract(t *testing.T, db *sql.DB, driver, tenantID, name string) int64 {
	t.Helper()
	ctx := context.Background()

	rebind := func(q string) string {
		if driver != "pgx" {
			return q
		}
		out := ""
		n := 0
		for _, r := range q {
			if r == '?' {
				n++
				out += fmt.Sprintf("$%d", n)
				continue
			}
			out += string(r)
		}
		return out
	}

	var partnerID, contractID int64
	routing := fmt.Sprintf("%09d", time.Now().UnixNano()%1_000_000_000)

	if driver == "pgx" {
		if err := db.QueryRowContext(ctx, rebind(`
			INSERT INTO partners (tenant_id, name, routing_number) VALUES (?, ?, ?) RETURNING id`),
			tenantID, name+" Bank", routing).Scan(&partnerID); err != nil {
			t.Fatal(err)
		}
		if err := db.QueryRowContext(ctx, rebind(`
			INSERT INTO file_contracts
				(tenant_id, partner_id, name, direction, filename_pattern, expected_time, grace_period_minutes, timezone)
			VALUES (?, ?, ?, 'INBOUND', 'unused-legacy', '09:00', 30, 'America/New_York') RETURNING id`),
			tenantID, partnerID, name).Scan(&contractID); err != nil {
			t.Fatal(err)
		}
		return contractID
	}

	res, err := db.ExecContext(ctx,
		`INSERT INTO partners (tenant_id, name, routing_number) VALUES (?, ?, ?)`,
		tenantID, name+" Bank", routing)
	if err != nil {
		t.Fatal(err)
	}
	partnerID, _ = res.LastInsertId()
	res, err = db.ExecContext(ctx, `
		INSERT INTO file_contracts
			(tenant_id, partner_id, name, direction, filename_pattern, expected_time, grace_period_minutes, timezone)
		VALUES (?, ?, ?, 'INBOUND', 'unused-legacy', '09:00', 30, 'America/New_York')`,
		tenantID, partnerID, name)
	if err != nil {
		t.Fatal(err)
	}
	contractID, _ = res.LastInsertId()
	return contractID
}

// standardInput is a complete, valid contract version. Tests vary one field at
// a time from it so a failure names the field that caused it.
func standardInput(tenantID string, contractID int64, from Date) NewVersionInput {
	return NewVersionInput{
		TenantID:         tenantID,
		ContractID:       contractID,
		FeedID:           "ACME-PAYROLL",
		Direction:        "INBOUND",
		FilenamePattern:  "ACH_{YYYY}{MM}{DD}.txt",
		Format:           "NACHA",
		ExpectedLocal:    "09:00",
		Timezone:         "America/New_York",
		GraceMinutes:     30,
		BreachMinutes:    60,
		CalendarID:       "fed",
		ScheduleRule:     "EVERY_BUSINESS_DAY",
		NonBusinessDay:   "SKIP",
		BalancedMode:     "BALANCED",
		OwnerSubject:     "ops@example.test",
		EscalationPolicy: "policy/payments-oncall",
		EffectiveFrom:    from,
	}
}

func setupFed(t *testing.T, s *Store, tenantID string) {
	t.Helper()
	if err := s.CreateCalendar(context.Background(), tenantID, "fed", "Federal Reserve", BaseFederalReserve); err != nil {
		t.Fatal(err)
	}
}

type occRow struct {
	ID       int64
	Status   string
	Business string
	Due      time.Time
	Grace    time.Time
	Breach   sql.NullTime
	Note     string
	Review   int
	Matched  sql.NullInt64
}

func occurrences(t *testing.T, s *Store, tenantID string) []occRow {
	t.Helper()
	rows, err := s.db.QueryContext(context.Background(), s.rebind(`
		SELECT id, status, business_date, expected_delivery_start, expected_delivery_end,
		       breach_at, schedule_note, review_required, matched_artifact_id
		FROM expectations WHERE tenant_id = ? ORDER BY business_date, id`), tenantID)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()

	var out []occRow
	for rows.Next() {
		var o occRow
		var business sql.NullTime
		if err := rows.Scan(&o.ID, &o.Status, &business, &o.Due, &o.Grace,
			&o.Breach, &o.Note, &o.Review, &o.Matched); err != nil {
			t.Fatal(err)
		}
		if business.Valid {
			o.Business = DateOf(business.Time, time.UTC).String()
		}
		out = append(out, o)
	}
	if err := rows.Err(); err != nil {
		t.Fatal(err)
	}
	return out
}

// newArtifact inserts a stored artifact.
//
// Each gets a distinct content hash. Migration 004 makes (tenant, hash, size)
// unique -- two artifacts with identical bytes are one artifact, which is the
// deduplication ingest relies on -- so a fixture that reused an empty hash
// would be testing that constraint rather than the matcher.
func newArtifact(t *testing.T, s *Store, tenantID, filename string) int64 {
	t.Helper()
	ctx := context.Background()
	hash := fmt.Sprintf("%064x", time.Now().UnixNano())
	if s.dialect == dialectPostgres {
		var id int64
		if err := s.db.QueryRowContext(ctx, s.rebind(`
			INSERT INTO file_instances (tenant_id, filename, sha256_hash, status)
			VALUES (?, ?, ?, 'RECEIVED') RETURNING id`),
			tenantID, filename, hash).Scan(&id); err != nil {
			t.Fatal(err)
		}
		return id
	}
	res, err := s.db.ExecContext(ctx, `
		INSERT INTO file_instances (tenant_id, filename, storage_path, size_bytes, sha256_hash, status, received_at)
		VALUES (?, ?, '', 1, ?, 'RECEIVED', CURRENT_TIMESTAMP)`, tenantID, filename, hash)
	if err != nil {
		t.Fatal(err)
	}
	id, _ := res.LastInsertId()
	return id
}

func matchArrival(t *testing.T, s *Store, tenantID string, artifactID int64, filename string, at time.Time) MatchResult {
	t.Helper()
	ctx := context.Background()
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = tx.Rollback() }()

	res, err := s.MatchArrival(ctx, tx, tenantID, artifactID, filename, at)
	if err != nil {
		t.Fatal(err)
	}
	if err := tx.Commit(); err != nil {
		t.Fatal(err)
	}
	return res
}

// ---------------------------------------------------------------------------
// Materialization
// ---------------------------------------------------------------------------

// The core claim: the row exists before the file does, so a file that never
// arrives has something to make overdue.
func TestOccurrencesExistBeforeAnyArrival(t *testing.T) {
	eachBackend(t, func(t *testing.T, db *sql.DB, driver string) {
		// Monday 9 June 2025, 08:00 New York.
		now := time.Date(2025, time.June, 9, 12, 0, 0, 0, time.UTC)
		s := newStore(t, db, driver, now)
		setupFed(t, s, tenantA)
		contract := newContract(t, db, driver, tenantA, "Acme")

		if _, err := s.CreateVersion(context.Background(),
			standardInput(tenantA, contract, NewDate(2025, time.June, 1))); err != nil {
			t.Fatal(err)
		}

		res, err := s.Materialize(context.Background(), 7)
		if err != nil {
			t.Fatal(err)
		}
		if len(res.Problems) != 0 {
			t.Fatalf("problems: %v", res.Problems)
		}

		got := occurrences(t, s, tenantA)
		// 9-13 June are weekdays; 14-15 are the weekend; 16 June closes the
		// seven-day horizon.
		want := []string{"2025-06-09", "2025-06-10", "2025-06-11", "2025-06-12", "2025-06-13", "2025-06-16"}
		if len(got) != len(want) {
			t.Fatalf("materialized %d occurrences, want %d: %v", len(got), len(want), summarise(got))
		}
		for i := range want {
			if got[i].Business != want[i] {
				t.Errorf("occurrence %d business date = %s, want %s", i, got[i].Business, want[i])
			}
			if got[i].Status != "PENDING" {
				t.Errorf("%s status = %s, want PENDING", got[i].Business, got[i].Status)
			}
		}

		// The deadline really is 09:00 New York, which is 13:00 UTC in June.
		if h := got[0].Due.UTC().Hour(); h != 13 {
			t.Errorf("due hour = %d UTC, want 13 (09:00 EDT)", h)
		}
		if d := got[0].Grace.Sub(got[0].Due); d != 30*time.Minute {
			t.Errorf("grace = %s, want 30m", d)
		}
		if !got[0].Breach.Valid || got[0].Breach.Time.Sub(got[0].Grace) != 60*time.Minute {
			t.Errorf("breach threshold is not 60m after grace: %v", got[0].Breach)
		}
	})
}

func summarise(rows []occRow) []string {
	out := make([]string, len(rows))
	for i, r := range rows {
		out[i] = r.Business + "=" + r.Status
	}
	return out
}

// Restart, overlapping windows, and a second scheduler all converge on one row.
func TestMaterializationIsIdempotentAcrossRestart(t *testing.T) {
	eachBackend(t, func(t *testing.T, db *sql.DB, driver string) {
		now := time.Date(2025, time.June, 9, 12, 0, 0, 0, time.UTC)
		s := newStore(t, db, driver, now)
		setupFed(t, s, tenantA)
		contract := newContract(t, db, driver, tenantA, "Acme")
		if _, err := s.CreateVersion(context.Background(),
			standardInput(tenantA, contract, NewDate(2025, time.June, 1))); err != nil {
			t.Fatal(err)
		}

		first, err := s.Materialize(context.Background(), 7)
		if err != nil {
			t.Fatal(err)
		}
		if first.Created == 0 {
			t.Fatal("the first pass created nothing")
		}

		// A restart is simply another Store over the same database.
		s2 := newStore(t, db, driver, now)
		second, err := s2.Materialize(context.Background(), 7)
		if err != nil {
			t.Fatal(err)
		}
		if second.Created != 0 {
			t.Errorf("a second pass created %d occurrences; materialization must be idempotent", second.Created)
		}
		if second.Skipped != first.Created {
			t.Errorf("second pass skipped %d, want %d", second.Skipped, first.Created)
		}
		if n := len(occurrences(t, s, tenantA)); n != first.Created {
			t.Errorf("%d rows after two passes, want %d", n, first.Created)
		}
	})
}

func TestTwoConcurrentSchedulersProduceOneOccurrence(t *testing.T) {
	eachBackend(t, func(t *testing.T, db *sql.DB, driver string) {
		now := time.Date(2025, time.June, 9, 12, 0, 0, 0, time.UTC)
		s := newStore(t, db, driver, now)
		setupFed(t, s, tenantA)
		contract := newContract(t, db, driver, tenantA, "Acme")
		if _, err := s.CreateVersion(context.Background(),
			standardInput(tenantA, contract, NewDate(2025, time.June, 1))); err != nil {
			t.Fatal(err)
		}
		db.SetMaxOpenConns(8)

		const schedulers = 6
		var wg sync.WaitGroup
		errs := make(chan error, schedulers)
		start := make(chan struct{})

		for range schedulers {
			wg.Add(1)
			go func() {
				defer wg.Done()
				peer := newStore(t, db, driver, now)
				<-start
				if _, err := peer.Materialize(context.Background(), 14); err != nil {
					errs <- err
				}
			}()
		}
		close(start)
		wg.Wait()
		close(errs)
		for err := range errs {
			t.Errorf("concurrent materialize: %v", err)
		}

		got := occurrences(t, s, tenantA)
		seen := map[string]int{}
		for _, o := range got {
			seen[o.Business]++
		}
		for date, n := range seen {
			if n != 1 {
				t.Errorf("business date %s has %d occurrences; the unique key must make "+
					"concurrent materialization converge on one", date, n)
			}
		}
		if len(got) == 0 {
			t.Fatal("six concurrent schedulers produced nothing")
		}
	})
}

// ---------------------------------------------------------------------------
// Arrival scenarios
// ---------------------------------------------------------------------------

// on-time, within grace, late, never arrives, arrives after breach.
func TestArrivalScenarios(t *testing.T) {
	eachBackend(t, func(t *testing.T, db *sql.DB, driver string) {
		business := NewDate(2025, time.June, 10)
		// Due 09:00 EDT = 13:00 UTC; grace to 13:30; breach at 14:30.
		due := time.Date(2025, time.June, 10, 13, 0, 0, 0, time.UTC)

		cases := []struct {
			name    string
			arrival *time.Duration // offset from due; nil means never
			want    domain.ExpectationState
			// state observed at the moment of arrival, before it is matched
			wantBefore domain.ExpectationState
		}{
			{"on time", dur(-5 * time.Minute), domain.ExpectationArrived, domain.ExpectationPending},
			{"exactly at the deadline", dur(0), domain.ExpectationArrived, domain.ExpectationDue},
			{"within grace", dur(20 * time.Minute), domain.ExpectationArrived, domain.ExpectationDue},
			{"late, past grace", dur(45 * time.Minute), domain.ExpectationArrived, domain.ExpectationOverdue},
			{"after breach", dur(4 * time.Hour), domain.ExpectationArrived, domain.ExpectationBreached},
			{"never arrives", nil, domain.ExpectationBreached, domain.ExpectationBreached},
		}

		for _, c := range cases {
			t.Run(c.name, func(t *testing.T) {
				// A fresh tenant-scoped contract per case.
				s := newStore(t, db, driver, due.Add(-2*time.Hour))
				setupFed(t, s, tenantA)
				if err := s.CreateCalendar(context.Background(), tenantA, "fed", "Fed", BaseFederalReserve); err != nil {
					t.Fatal(err)
				}
				contract := newContract(t, db, driver, tenantA, "Acme-"+c.name)
				in := standardInput(tenantA, contract, NewDate(2025, time.June, 1))
				if _, err := s.CreateVersion(context.Background(), in); err != nil {
					t.Fatal(err)
				}
				if _, err := s.Materialize(context.Background(), 1); err != nil {
					t.Fatal(err)
				}

				observe := due.Add(4 * time.Hour)
				if c.arrival != nil {
					observe = due.Add(*c.arrival)
				}

				// Advance the clock to the observation point.
				s.SetClock(func() time.Time { return observe })
				if _, err := s.Advance(context.Background()); err != nil {
					t.Fatal(err)
				}

				got := occurrenceFor(t, s, tenantA, contract, business)
				if domain.ExpectationState(got.Status) != c.wantBefore {
					t.Errorf("state at observation = %s, want %s", got.Status, c.wantBefore)
				}

				if c.arrival != nil {
					artifact := newArtifact(t, s, tenantA, "ACH_20250610.txt")
					res := matchArrival(t, s, tenantA, artifact, "ACH_20250610.txt", observe)
					if res.Outcome != MatchAttributed {
						t.Fatalf("outcome = %s (%s), want ATTRIBUTED", res.Outcome, res.Reason)
					}
				}

				got = occurrenceFor(t, s, tenantA, contract, business)
				if domain.ExpectationState(got.Status) != c.want {
					t.Errorf("final state = %s, want %s", got.Status, c.want)
				}
			})
		}
	})
}

func dur(d time.Duration) *time.Duration { return &d }

func occurrenceFor(t *testing.T, s *Store, tenantID string, contractID int64, d Date) occRow {
	t.Helper()
	for _, o := range occurrences(t, s, tenantID) {
		if o.Business == d.String() {
			var got int64
			if err := s.db.QueryRow(s.rebind(
				`SELECT contract_id FROM expectations WHERE id = ?`), o.ID).Scan(&got); err != nil {
				t.Fatal(err)
			}
			if got == contractID {
				return o
			}
		}
	}
	t.Fatalf("no occurrence for contract %d on %s", contractID, d)
	return occRow{}
}

// A late arrival does not erase the breach.
func TestLateArrivalAfterBreachIsRecordedAsSuch(t *testing.T) {
	eachBackend(t, func(t *testing.T, db *sql.DB, driver string) {
		due := time.Date(2025, time.June, 10, 13, 0, 0, 0, time.UTC)
		s := newStore(t, db, driver, due.Add(-time.Hour))
		setupFed(t, s, tenantA)
		contract := newContract(t, db, driver, tenantA, "Acme")
		if _, err := s.CreateVersion(context.Background(),
			standardInput(tenantA, contract, NewDate(2025, time.June, 1))); err != nil {
			t.Fatal(err)
		}
		if _, err := s.Materialize(context.Background(), 1); err != nil {
			t.Fatal(err)
		}

		s.SetClock(func() time.Time { return due.Add(6 * time.Hour) })
		if _, err := s.Advance(context.Background()); err != nil {
			t.Fatal(err)
		}
		occ := occurrenceFor(t, s, tenantA, contract, NewDate(2025, time.June, 10))
		if occ.Status != "BREACHED" {
			t.Fatalf("status = %s, want BREACHED", occ.Status)
		}

		artifact := newArtifact(t, s, tenantA, "ACH_20250610.txt")
		matchArrival(t, s, tenantA, artifact, "ACH_20250610.txt", due.Add(6*time.Hour))

		// The history must still contain the breach and must name the late
		// arrival, or a report of final states would show a clean day.
		var breachRows, lateRows int
		if err := s.db.QueryRow(s.rebind(`
			SELECT COUNT(*) FROM status_history
			WHERE tenant_id = ? AND object_type = 'expectation' AND object_id = ? AND to_state = 'BREACHED'`),
			tenantA, occ.ID).Scan(&breachRows); err != nil {
			t.Fatal(err)
		}
		if breachRows != 1 {
			t.Errorf("breach history rows = %d, want 1", breachRows)
		}
		if err := s.db.QueryRow(s.rebind(`
			SELECT COUNT(*) FROM status_history
			WHERE tenant_id = ? AND object_id = ? AND from_state = 'BREACHED' AND to_state = 'ARRIVED'`),
			tenantA, occ.ID).Scan(&lateRows); err != nil {
			t.Fatal(err)
		}
		if lateRows != 1 {
			t.Errorf("late-arrival history rows = %d, want 1", lateRows)
		}
	})
}

// A scheduler that was down over a weekend must walk every state, not jump.
func TestAdvancementWalksEveryIntermediateState(t *testing.T) {
	eachBackend(t, func(t *testing.T, db *sql.DB, driver string) {
		due := time.Date(2025, time.June, 10, 13, 0, 0, 0, time.UTC)
		s := newStore(t, db, driver, due.Add(-time.Hour))
		setupFed(t, s, tenantA)
		contract := newContract(t, db, driver, tenantA, "Acme")
		if _, err := s.CreateVersion(context.Background(),
			standardInput(tenantA, contract, NewDate(2025, time.June, 1))); err != nil {
			t.Fatal(err)
		}
		if _, err := s.Materialize(context.Background(), 1); err != nil {
			t.Fatal(err)
		}

		// The scheduler wakes up three days later, having missed every step.
		s.SetClock(func() time.Time { return due.Add(72 * time.Hour) })
		res, err := s.Advance(context.Background())
		if err != nil {
			t.Fatal(err)
		}
		// A one-day horizon covers today and tomorrow, so more than one
		// occurrence may exist. Every one of them must have walked the full
		// path, so the three counts must be equal and non-zero.
		if res.Due == 0 || res.Due != res.Overdue || res.Overdue != res.Breached {
			t.Errorf("transitions: due=%d overdue=%d breached=%d; every occurrence must walk "+
				"all three edges", res.Due, res.Overdue, res.Breached)
		}

		occ := occurrenceFor(t, s, tenantA, contract, NewDate(2025, time.June, 10))
		var states []string
		rows, err := s.db.Query(s.rebind(`
			SELECT from_state || '->' || to_state FROM status_history
			WHERE tenant_id = ? AND object_type = 'expectation' AND object_id = ?
			ORDER BY id`), tenantA, occ.ID)
		if err != nil {
			t.Fatal(err)
		}
		defer rows.Close()
		for rows.Next() {
			var s string
			if err := rows.Scan(&s); err != nil {
				t.Fatal(err)
			}
			states = append(states, s)
		}
		want := []string{"PENDING->DUE", "DUE->OVERDUE", "OVERDUE->BREACHED"}
		if len(states) != len(want) {
			t.Fatalf("history = %v, want %v: writing BREACHED straight onto PENDING would be an "+
				"illegal edge and would lose the intermediate record", states, want)
		}
		for i := range want {
			if states[i] != want[i] {
				t.Fatalf("history = %v, want %v", states, want)
			}
		}
	})
}

func TestTwoConcurrentSchedulersProduceOneSetOfTransitions(t *testing.T) {
	eachBackend(t, func(t *testing.T, db *sql.DB, driver string) {
		due := time.Date(2025, time.June, 10, 13, 0, 0, 0, time.UTC)
		s := newStore(t, db, driver, due.Add(-time.Hour))
		setupFed(t, s, tenantA)
		contract := newContract(t, db, driver, tenantA, "Acme")
		if _, err := s.CreateVersion(context.Background(),
			standardInput(tenantA, contract, NewDate(2025, time.June, 1))); err != nil {
			t.Fatal(err)
		}
		if _, err := s.Materialize(context.Background(), 5); err != nil {
			t.Fatal(err)
		}
		db.SetMaxOpenConns(8)

		observe := due.Add(72 * time.Hour)
		var wg sync.WaitGroup
		for range 5 {
			wg.Add(1)
			go func() {
				defer wg.Done()
				peer := newStore(t, db, driver, observe)
				if _, err := peer.Advance(context.Background()); err != nil {
					t.Errorf("concurrent advance: %v", err)
				}
			}()
		}
		wg.Wait()

		// Exactly one history row per transition, regardless of how many
		// schedulers tried.
		rows, err := s.db.Query(s.rebind(`
			SELECT object_id, from_state, to_state, COUNT(*)
			FROM status_history
			WHERE tenant_id = ? AND object_type = 'expectation'
			GROUP BY object_id, from_state, to_state
			HAVING COUNT(*) > 1`), tenantA)
		if err != nil {
			t.Fatal(err)
		}
		defer rows.Close()
		for rows.Next() {
			var id int64
			var from, to string
			var n int
			if err := rows.Scan(&id, &from, &to, &n); err != nil {
				t.Fatal(err)
			}
			t.Errorf("occurrence %d recorded %s->%s %d times; the optimistic update must let "+
				"exactly one scheduler win", id, from, to, n)
		}
	})
}

// ---------------------------------------------------------------------------
// Ambiguity
// ---------------------------------------------------------------------------

func TestAmbiguousFilenameMatchRequiresReviewAndAttributesNothing(t *testing.T) {
	eachBackend(t, func(t *testing.T, db *sql.DB, driver string) {
		now := time.Date(2025, time.June, 12, 20, 0, 0, 0, time.UTC)
		s := newStore(t, db, driver, now)
		setupFed(t, s, tenantA)

		// A pattern with no date token: two days' occurrences are both
		// satisfiable by the same filename, which is exactly the situation
		// that must not be resolved by guessing.
		contract := newContract(t, db, driver, tenantA, "Acme")
		in := standardInput(tenantA, contract, NewDate(2025, time.June, 1))
		in.FilenamePattern = "ACH_daily.txt"
		if _, err := s.CreateVersion(context.Background(), in); err != nil {
			t.Fatal(err)
		}
		// Materialize from an earlier clock so several days are open at once.
		s.SetClock(func() time.Time { return time.Date(2025, time.June, 10, 12, 0, 0, 0, time.UTC) })
		if _, err := s.Materialize(context.Background(), 2); err != nil {
			t.Fatal(err)
		}
		s.SetClock(func() time.Time { return now })

		before := occurrences(t, s, tenantA)
		if len(before) < 2 {
			t.Fatalf("need at least two open occurrences, got %v", summarise(before))
		}

		artifact := newArtifact(t, s, tenantA, "ACH_daily.txt")
		res := matchArrival(t, s, tenantA, artifact, "ACH_daily.txt", now)

		if res.Outcome != MatchAmbiguous {
			t.Fatalf("outcome = %s (%s), want AMBIGUOUS", res.Outcome, res.Reason)
		}
		if len(res.Candidates) < 2 {
			t.Errorf("candidates = %v, want every occurrence the file could have satisfied", res.Candidates)
		}

		after := occurrences(t, s, tenantA)
		for _, o := range after {
			if o.Status == "ARRIVED" {
				t.Errorf("occurrence %s was marked ARRIVED; an ambiguous arrival must attribute "+
					"nothing, or a genuinely missing file is recorded as delivered", o.Business)
			}
			if o.Matched.Valid {
				t.Errorf("occurrence %s has a matched artifact despite the ambiguity", o.Business)
			}
			if o.Review != 1 {
				t.Errorf("occurrence %s is not flagged for review", o.Business)
			}
		}

		open, err := s.OpenReviews(context.Background(), tenantA)
		if err != nil {
			t.Fatal(err)
		}
		if open < 2 {
			t.Errorf("open reviews = %d, want one per candidate", open)
		}
	})
}

// An occurrence under review keeps ageing. Freezing it would let a wrong guess
// stop the clock on a file that never came.
func TestAmbiguousOccurrenceStillBreaches(t *testing.T) {
	eachBackend(t, func(t *testing.T, db *sql.DB, driver string) {
		s := newStore(t, db, driver, time.Date(2025, time.June, 10, 12, 0, 0, 0, time.UTC))
		setupFed(t, s, tenantA)
		contract := newContract(t, db, driver, tenantA, "Acme")
		in := standardInput(tenantA, contract, NewDate(2025, time.June, 1))
		in.FilenamePattern = "ACH_daily.txt"
		if _, err := s.CreateVersion(context.Background(), in); err != nil {
			t.Fatal(err)
		}
		if _, err := s.Materialize(context.Background(), 2); err != nil {
			t.Fatal(err)
		}

		at := time.Date(2025, time.June, 10, 14, 0, 0, 0, time.UTC)
		s.SetClock(func() time.Time { return at })
		artifact := newArtifact(t, s, tenantA, "ACH_daily.txt")
		if res := matchArrival(t, s, tenantA, artifact, "ACH_daily.txt", at); res.Outcome != MatchAmbiguous {
			t.Fatalf("outcome = %s, want AMBIGUOUS", res.Outcome)
		}

		s.SetClock(func() time.Time { return at.Add(72 * time.Hour) })
		if _, err := s.Advance(context.Background()); err != nil {
			t.Fatal(err)
		}
		for _, o := range occurrences(t, s, tenantA) {
			if o.Status != "BREACHED" {
				t.Errorf("occurrence %s is %s; an occurrence whose arrival is in doubt has not "+
					"been shown to have arrived and must keep ageing", o.Business, o.Status)
			}
		}
	})
}

func TestASecondFileForASatisfiedOccurrenceIsAReviewNotAMatch(t *testing.T) {
	eachBackend(t, func(t *testing.T, db *sql.DB, driver string) {
		at := time.Date(2025, time.June, 10, 12, 0, 0, 0, time.UTC)
		s := newStore(t, db, driver, at)
		setupFed(t, s, tenantA)
		contract := newContract(t, db, driver, tenantA, "Acme")
		if _, err := s.CreateVersion(context.Background(),
			standardInput(tenantA, contract, NewDate(2025, time.June, 1))); err != nil {
			t.Fatal(err)
		}
		if _, err := s.Materialize(context.Background(), 1); err != nil {
			t.Fatal(err)
		}

		first := newArtifact(t, s, tenantA, "ACH_20250610.txt")
		if res := matchArrival(t, s, tenantA, first, "ACH_20250610.txt", at); res.Outcome != MatchAttributed {
			t.Fatalf("first arrival: %s (%s)", res.Outcome, res.Reason)
		}

		second := newArtifact(t, s, tenantA, "ACH_20250610.txt")
		res := matchArrival(t, s, tenantA, second, "ACH_20250610.txt", at.Add(time.Hour))
		if res.Outcome != MatchDuplicate {
			t.Errorf("second arrival outcome = %s, want DUPLICATE", res.Outcome)
		}

		occ := occurrenceFor(t, s, tenantA, contract, NewDate(2025, time.June, 10))
		if !occ.Matched.Valid || occ.Matched.Int64 != first {
			t.Errorf("matched artifact = %v, want the first arrival %d; a second file must not "+
				"silently replace the attribution", occ.Matched, first)
		}
	})
}

func TestAnUnexpectedFileIsNotAnError(t *testing.T) {
	eachBackend(t, func(t *testing.T, db *sql.DB, driver string) {
		at := time.Date(2025, time.June, 10, 12, 0, 0, 0, time.UTC)
		s := newStore(t, db, driver, at)
		setupFed(t, s, tenantA)
		contract := newContract(t, db, driver, tenantA, "Acme")
		if _, err := s.CreateVersion(context.Background(),
			standardInput(tenantA, contract, NewDate(2025, time.June, 1))); err != nil {
			t.Fatal(err)
		}
		if _, err := s.Materialize(context.Background(), 1); err != nil {
			t.Fatal(err)
		}

		artifact := newArtifact(t, s, tenantA, "SOMETHING_ELSE.csv")
		res := matchArrival(t, s, tenantA, artifact, "SOMETHING_ELSE.csv", at)
		if res.Outcome != MatchUnexpected {
			t.Errorf("outcome = %s, want UNEXPECTED", res.Outcome)
		}
		occ := occurrenceFor(t, s, tenantA, contract, NewDate(2025, time.June, 10))
		if occ.Status != "PENDING" {
			t.Errorf("the expectation was disturbed by an unrelated file: status = %s", occ.Status)
		}
	})
}

// ---------------------------------------------------------------------------
// Contract versioning
// ---------------------------------------------------------------------------

func TestEditingTermsCreatesANewVersionAndHistoryResolvesToTheOldOne(t *testing.T) {
	eachBackend(t, func(t *testing.T, db *sql.DB, driver string) {
		ctx := context.Background()
		s := newStore(t, db, driver, time.Date(2025, time.June, 9, 12, 0, 0, 0, time.UTC))
		setupFed(t, s, tenantA)
		contract := newContract(t, db, driver, tenantA, "Acme")

		v1, err := s.CreateVersion(ctx, standardInput(tenantA, contract, NewDate(2025, time.January, 1)))
		if err != nil {
			t.Fatal(err)
		}

		// The partner moves the delivery an hour earlier from 1 July.
		second := standardInput(tenantA, contract, NewDate(2025, time.July, 1))
		second.ExpectedLocal = "08:00"
		v2, err := s.CreateVersion(ctx, second)
		if err != nil {
			t.Fatal(err)
		}
		if v2.Version != 2 {
			t.Errorf("version = %d, want 2", v2.Version)
		}
		if v2.ID == v1.ID {
			t.Error("editing terms must create a row, not update one")
		}

		// A date under the old terms resolves to version 1, forever.
		got, err := s.VersionOn(ctx, tenantA, contract, NewDate(2025, time.June, 10))
		if err != nil {
			t.Fatal(err)
		}
		if got.ID != v1.ID {
			t.Errorf("2025-06-10 resolved to version %d, want %d", got.Version, v1.Version)
		}
		if got.ExpectedLocal.String() != "09:00:00" {
			t.Errorf("June resolved to expected time %s, want 09:00:00; changing today's terms "+
				"must not alter whether last month's file was late", got.ExpectedLocal)
		}

		// A date under the new terms resolves to version 2.
		got, err = s.VersionOn(ctx, tenantA, contract, NewDate(2025, time.July, 10))
		if err != nil {
			t.Fatal(err)
		}
		if got.ID != v2.ID {
			t.Errorf("2025-07-10 resolved to version %d, want %d", got.Version, v2.Version)
		}

		// The boundary is half-open: 1 July belongs to version 2, 30 June to
		// version 1.
		if g, _ := s.VersionOn(ctx, tenantA, contract, NewDate(2025, time.July, 1)); g.ID != v2.ID {
			t.Error("2025-07-01 must belong to the new version")
		}
		if g, _ := s.VersionOn(ctx, tenantA, contract, NewDate(2025, time.June, 30)); g.ID != v1.ID {
			t.Error("2025-06-30 must belong to the old version")
		}
	})
}

func TestAVersionCannotBeBackdatedOverAnExistingOne(t *testing.T) {
	eachBackend(t, func(t *testing.T, db *sql.DB, driver string) {
		ctx := context.Background()
		s := newStore(t, db, driver, time.Date(2025, time.June, 9, 12, 0, 0, 0, time.UTC))
		setupFed(t, s, tenantA)
		contract := newContract(t, db, driver, tenantA, "Acme")
		if _, err := s.CreateVersion(ctx, standardInput(tenantA, contract, NewDate(2025, time.June, 1))); err != nil {
			t.Fatal(err)
		}
		if _, err := s.CreateVersion(ctx, standardInput(tenantA, contract, NewDate(2025, time.May, 1))); err == nil {
			t.Error("a backdated version must be refused: it would rewrite which terms governed " +
				"dates that have already been scheduled and possibly breached")
		}
	})
}

// A future PENDING occurrence follows a version change; a judged one does not.
func TestVersionChangeRepinsFutureOccurrencesOnly(t *testing.T) {
	eachBackend(t, func(t *testing.T, db *sql.DB, driver string) {
		ctx := context.Background()
		now := time.Date(2025, time.June, 9, 12, 0, 0, 0, time.UTC)
		s := newStore(t, db, driver, now)
		setupFed(t, s, tenantA)
		contract := newContract(t, db, driver, tenantA, "Acme")
		if _, err := s.CreateVersion(ctx, standardInput(tenantA, contract, NewDate(2025, time.June, 1))); err != nil {
			t.Fatal(err)
		}
		if _, err := s.Materialize(ctx, 10); err != nil {
			t.Fatal(err)
		}

		// Mark one occurrence as already judged, by matching it. The arrival
		// timestamp is the 12th, not the scheduler's clock: an arrival is
		// matched against occurrences near the day it arrived, so a file
		// presented three days before its business date is out of range by
		// design.
		future := occurrenceFor(t, s, tenantA, contract, NewDate(2025, time.June, 12))
		artifact := newArtifact(t, s, tenantA, "ACH_20250612.txt")
		arrivedAt := time.Date(2025, time.June, 12, 12, 0, 0, 0, time.UTC)
		if res := matchArrival(t, s, tenantA, artifact, "ACH_20250612.txt", arrivedAt); res.Outcome != MatchAttributed {
			t.Fatalf("setup arrival: %s (%s)", res.Outcome, res.Reason)
		}

		// New terms from 11 June.
		second := standardInput(tenantA, contract, NewDate(2025, time.June, 11))
		second.ExpectedLocal = "07:00"
		v2, err := s.CreateVersion(ctx, second)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := s.Materialize(ctx, 10); err != nil {
			t.Fatal(err)
		}

		// 13 June was PENDING and in the future: it now points at version 2.
		repinned := occurrenceFor(t, s, tenantA, contract, NewDate(2025, time.June, 13))
		var versionID int64
		if err := s.db.QueryRow(s.rebind(
			`SELECT contract_version_id FROM expectations WHERE id = ?`), repinned.ID).Scan(&versionID); err != nil {
			t.Fatal(err)
		}
		if versionID != v2.ID {
			t.Errorf("2025-06-13 still points at version %d, want %d", versionID, v2.ID)
		}
		if h := repinned.Due.UTC().Hour(); h != 11 {
			t.Errorf("re-pinned due hour = %d UTC, want 11 (07:00 EDT)", h)
		}

		// 12 June was already matched: it must keep the terms it was judged
		// under.
		if err := s.db.QueryRow(s.rebind(
			`SELECT contract_version_id FROM expectations WHERE id = ?`), future.ID).Scan(&versionID); err != nil {
			t.Fatal(err)
		}
		if versionID == v2.ID {
			t.Error("a matched occurrence was re-pinned to new terms; the arrival was judged " +
				"against the old deadline and the record must say so")
		}
	})
}

// ---------------------------------------------------------------------------
// Calendars, DST and boundaries, end to end through the database
// ---------------------------------------------------------------------------

func TestObservedHolidayProducesNoOccurrence(t *testing.T) {
	eachBackend(t, func(t *testing.T, db *sql.DB, driver string) {
		ctx := context.Background()
		// The week of Thanksgiving 2025: Thursday 27 November is a holiday.
		s := newStore(t, db, driver, time.Date(2025, time.November, 24, 12, 0, 0, 0, time.UTC))
		setupFed(t, s, tenantA)
		contract := newContract(t, db, driver, tenantA, "Acme")
		if _, err := s.CreateVersion(ctx, standardInput(tenantA, contract, NewDate(2025, time.November, 1))); err != nil {
			t.Fatal(err)
		}
		if _, err := s.Materialize(ctx, 5); err != nil {
			t.Fatal(err)
		}

		for _, o := range occurrences(t, s, tenantA) {
			if o.Business == "2025-11-27" {
				t.Error("an occurrence was created on Thanksgiving Day; the partner would be " +
					"reported late for a file nobody expected")
			}
			if o.Business == "2025-11-29" || o.Business == "2025-11-30" {
				t.Errorf("an occurrence was created on the weekend (%s)", o.Business)
			}
		}
		// The Friday after Thanksgiving is a normal business day for the
		// Reserve Banks, so it must be present.
		found := false
		for _, o := range occurrences(t, s, tenantA) {
			if o.Business == "2025-11-28" {
				found = true
			}
		}
		if !found {
			t.Error("no occurrence on 2025-11-28; the Reserve Banks are open the day after Thanksgiving")
		}
	})
}

func TestTenantOverrideChangesTheSchedule(t *testing.T) {
	eachBackend(t, func(t *testing.T, db *sql.DB, driver string) {
		ctx := context.Background()
		s := newStore(t, db, driver, time.Date(2025, time.November, 24, 12, 0, 0, 0, time.UTC))
		setupFed(t, s, tenantA)
		// This tenant's partner delivers on Thanksgiving by agreement, and the
		// tenant's own office is shut the following Monday.
		if err := s.SetOverride(ctx, tenantA, "fed", Override{
			Date: NewDate(2025, time.November, 27), Open: true,
			Reason: "partner delivers on Thanksgiving under the 2024 addendum"}); err != nil {
			t.Fatal(err)
		}
		if err := s.SetOverride(ctx, tenantA, "fed", Override{
			Date: NewDate(2025, time.December, 1), Open: false,
			Reason: "annual systems maintenance window"}); err != nil {
			t.Fatal(err)
		}

		contract := newContract(t, db, driver, tenantA, "Acme")
		if _, err := s.CreateVersion(ctx, standardInput(tenantA, contract, NewDate(2025, time.November, 1))); err != nil {
			t.Fatal(err)
		}
		if _, err := s.Materialize(ctx, 8); err != nil {
			t.Fatal(err)
		}

		dates := map[string]bool{}
		for _, o := range occurrences(t, s, tenantA) {
			dates[o.Business] = true
		}
		if !dates["2025-11-27"] {
			t.Error("the tenant opened Thanksgiving and must have an occurrence for it")
		}
		if dates["2025-12-01"] {
			t.Error("the tenant closed 1 December and must not have an occurrence for it")
		}
	})
}

func TestDstTransitionIsPersistedWithItsNote(t *testing.T) {
	eachBackend(t, func(t *testing.T, db *sql.DB, driver string) {
		ctx := context.Background()
		// 9 March 2025 is a Sunday, so a daily business feed has no occurrence
		// that day. Use an ALL_DAYS calendar and a 02:30 deadline to reach the
		// gap through the database.
		s := newStore(t, db, driver, time.Date(2025, time.March, 7, 12, 0, 0, 0, time.UTC))
		if err := s.CreateCalendar(ctx, tenantA, "all", "Every day", BaseAllDays); err != nil {
			t.Fatal(err)
		}
		contract := newContract(t, db, driver, tenantA, "Acme")
		in := standardInput(tenantA, contract, NewDate(2025, time.March, 1))
		in.CalendarID = "all"
		in.ExpectedLocal = "02:30"
		if _, err := s.CreateVersion(ctx, in); err != nil {
			t.Fatal(err)
		}
		if _, err := s.Materialize(ctx, 4); err != nil {
			t.Fatal(err)
		}

		var found bool
		for _, o := range occurrences(t, s, tenantA) {
			if o.Business != "2025-03-09" {
				continue
			}
			found = true
			if o.Note == "" {
				t.Error("the occurrence spanning the DST gap must record how the deadline was resolved")
			}
			// The gap runs 02:00 EST -> 03:00 EDT, and both are the same
			// instant: 07:00 UTC. That instant is the deadline.
			if want := time.Date(2025, time.March, 9, 7, 0, 0, 0, time.UTC); !o.Due.UTC().Equal(want) {
				t.Errorf("due = %s, want %s (the first instant past the gap)", o.Due.UTC(), want)
			}
		}
		if !found {
			t.Fatal("no occurrence for 2025-03-09")
		}
	})
}

func TestMonthEndAcrossAYearBoundaryThroughTheDatabase(t *testing.T) {
	eachBackend(t, func(t *testing.T, db *sql.DB, driver string) {
		ctx := context.Background()
		s := newStore(t, db, driver, time.Date(2025, time.December, 26, 12, 0, 0, 0, time.UTC))
		setupFed(t, s, tenantA)
		contract := newContract(t, db, driver, tenantA, "Acme")
		in := standardInput(tenantA, contract, NewDate(2025, time.January, 1))
		in.ScheduleRule = "MONTHLY:LAST"
		in.NonBusinessDay = "PRECEDING"
		if _, err := s.CreateVersion(ctx, in); err != nil {
			t.Fatal(err)
		}
		// A horizon crossing into the new year.
		if _, err := s.Materialize(ctx, 40); err != nil {
			t.Fatal(err)
		}

		dates := []string{}
		for _, o := range occurrences(t, s, tenantA) {
			dates = append(dates, o.Business)
		}
		// 31 December 2025 is a Wednesday and open. 31 January 2026 is a
		// Saturday, so PRECEDING gives Friday the 30th.
		want := map[string]bool{"2025-12-31": false, "2026-01-30": false}
		for _, d := range dates {
			if _, ok := want[d]; ok {
				want[d] = true
			}
		}
		for d, seen := range want {
			if !seen {
				t.Errorf("no occurrence on %s; got %v", d, dates)
			}
		}
	})
}

// ---------------------------------------------------------------------------
// Tenancy
// ---------------------------------------------------------------------------

func TestArrivalsNeverMatchAnotherTenantsExpectation(t *testing.T) {
	eachBackend(t, func(t *testing.T, db *sql.DB, driver string) {
		ctx := context.Background()
		at := time.Date(2025, time.June, 10, 12, 0, 0, 0, time.UTC)
		s := newStore(t, db, driver, at)

		for _, tenant := range []string{"TENANT-A", "TENANT-B"} {
			if err := s.CreateCalendar(ctx, tenant, "fed", "Fed", BaseFederalReserve); err != nil {
				t.Fatal(err)
			}
			contract := newContract(t, db, driver, tenant, "Acme")
			if _, err := s.CreateVersion(ctx, standardInput(tenant, contract, NewDate(2025, time.June, 1))); err != nil {
				t.Fatal(err)
			}
		}
		if _, err := s.Materialize(ctx, 1); err != nil {
			t.Fatal(err)
		}

		// Tenant A's file must not satisfy tenant B's identical expectation.
		artifact := newArtifact(t, s, "TENANT-A", "ACH_20250610.txt")
		if res := matchArrival(t, s, "TENANT-A", artifact, "ACH_20250610.txt", at); res.Outcome != MatchAttributed {
			t.Fatalf("tenant A arrival: %s", res.Outcome)
		}
		for _, o := range occurrences(t, s, "TENANT-B") {
			if o.Status != "PENDING" || o.Matched.Valid {
				t.Errorf("tenant B occurrence %s was affected by tenant A's arrival: status=%s matched=%v",
					o.Business, o.Status, o.Matched)
			}
		}
	})
}

// ---------------------------------------------------------------------------
// Determinism
// ---------------------------------------------------------------------------

// With no arrival history and every optional subsystem absent, the schedule is
// fully determined. This is the "zero history, AI fully offline" case: nothing
// in this package imports the AI tier, reads past arrivals, or consults a model.
func TestScheduleIsDeterministicWithZeroHistory(t *testing.T) {
	eachBackend(t, func(t *testing.T, db *sql.DB, driver string) {
		ctx := context.Background()
		now := time.Date(2025, time.June, 9, 12, 0, 0, 0, time.UTC)

		run := func() []string {
			s := newStore(t, db, driver, now)
			_ = s.CreateCalendar(ctx, tenantA, "fed", "Fed", BaseFederalReserve)
			contract := newContract(t, db, driver, tenantA, fmt.Sprintf("Acme-%d", time.Now().UnixNano()))
			if _, err := s.CreateVersion(ctx, standardInput(tenantA, contract, NewDate(2025, time.June, 1))); err != nil {
				t.Fatal(err)
			}
			if _, err := s.Materialize(ctx, 10); err != nil {
				t.Fatal(err)
			}
			var out []string
			for _, o := range occurrences(t, s, tenantA) {
				var cid int64
				if err := s.db.QueryRow(s.rebind(
					`SELECT contract_id FROM expectations WHERE id = ?`), o.ID).Scan(&cid); err != nil {
					t.Fatal(err)
				}
				if cid == contract {
					out = append(out, o.Business+"@"+o.Due.UTC().Format(time.RFC3339))
				}
			}
			return out
		}

		first, second := run(), run()
		if len(first) == 0 {
			t.Fatal("no occurrences produced")
		}
		if len(first) != len(second) {
			t.Fatalf("two identical contracts produced %d and %d occurrences", len(first), len(second))
		}
		for i := range first {
			if first[i] != second[i] {
				t.Errorf("occurrence %d differs between two identical runs: %s vs %s", i, first[i], second[i])
			}
		}
	})
}

func TestContractValidationRefusesUnusableTerms(t *testing.T) {
	eachBackend(t, func(t *testing.T, db *sql.DB, driver string) {
		ctx := context.Background()
		s := newStore(t, db, driver, time.Date(2025, time.June, 9, 12, 0, 0, 0, time.UTC))
		setupFed(t, s, tenantA)
		contract := newContract(t, db, driver, tenantA, "Acme")

		mutate := map[string]func(*NewVersionInput){
			"no owner to escalate to":       func(in *NewVersionInput) { in.OwnerSubject = "" },
			"no escalation policy":          func(in *NewVersionInput) { in.EscalationPolicy = "" },
			"an experimental format":        func(in *NewVersionInput) { in.Format = "ISO20022" },
			"a timezone abbreviation":       func(in *NewVersionInput) { in.Timezone = "EST" },
			"a zero breach delay":           func(in *NewVersionInput) { in.BreachMinutes = 0 },
			"a pattern matching everything": func(in *NewVersionInput) { in.FilenamePattern = "*" },
			"an unknown schedule rule":      func(in *NewVersionInput) { in.ScheduleRule = "FORTNIGHTLY" },
			"a missing calendar":            func(in *NewVersionInput) { in.CalendarID = "does-not-exist" },
			"a negative grace period":       func(in *NewVersionInput) { in.GraceMinutes = -1 },
			"no feed id":                    func(in *NewVersionInput) { in.FeedID = "" },
		}
		for why, apply := range mutate {
			in := standardInput(tenantA, contract, NewDate(2025, time.June, 1))
			apply(&in)
			if _, err := s.CreateVersion(ctx, in); err == nil {
				t.Errorf("a contract version with %s must be refused", why)
			}
		}
	})
}
