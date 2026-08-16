package main

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"sentinel-gateway/internal/nacha"
	"sentinel-gateway/internal/schedule"
)

// The scheduling core is verified in internal/schedule against both databases.
// These tests verify the wiring: that an upload arriving through the real HTTP
// handler is attributed to an expectation, and that the contract governing that
// expectation is the one validation applies.
//
// Wiring is worth testing separately because it fails silently. A scheduler
// that materializes perfectly and is never consulted by ingest produces exactly
// the behaviour it exists to prevent -- a delivered file that no expectation
// records as delivered.

func newScheduledContract(t *testing.T, db *sql.DB, tenantID string, in func(*schedule.NewVersionInput)) int64 {
	t.Helper()
	ctx := context.Background()

	store, err := schedulerFor(db)
	if err != nil {
		t.Fatal(err)
	}
	if err := store.CreateCalendar(ctx, tenantID, "fed", "Federal Reserve", schedule.BaseFederalReserve); err != nil {
		t.Fatal(err)
	}

	if _, err := db.Exec(
		`INSERT INTO tenants (id, name) VALUES (?, ?) ON CONFLICT (id) DO NOTHING`,
		tenantID, tenantID); err != nil {
		t.Fatal(err)
	}
	res, err := db.Exec(
		`INSERT INTO partners (tenant_id, name, routing_number) VALUES (?, 'Acme Bank', '021000021')`,
		tenantID)
	if err != nil {
		t.Fatal(err)
	}
	partnerID, _ := res.LastInsertId()

	res, err = db.Exec(`
		INSERT INTO file_contracts
			(tenant_id, partner_id, name, direction, filename_pattern, expected_time, grace_period_minutes, timezone)
		VALUES (?, ?, 'Acme payroll', 'INBOUND', 'legacy', '09:00', 30, 'America/New_York')`,
		tenantID, partnerID)
	if err != nil {
		t.Fatal(err)
	}
	contractID, _ := res.LastInsertId()

	// Effective well before any test date, so every business date resolves.
	input := schedule.NewVersionInput{
		TenantID:         tenantID,
		ContractID:       contractID,
		PartnerID:        partnerID,
		FeedID:           "ACME-PAYROLL",
		Direction:        "INBOUND",
		FilenamePattern:  "payroll-{YYYY}{MM}{DD}.ach",
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
		EffectiveFrom:    schedule.NewDate(2020, time.January, 1),
	}
	if in != nil {
		in(&input)
	}
	if _, err := store.CreateVersion(ctx, input); err != nil {
		t.Fatal(err)
	}
	if _, err := store.Materialize(ctx, 2); err != nil {
		t.Fatal(err)
	}
	return contractID
}

// nextExpectedFile returns the filename that satisfies the earliest open
// occurrence, and that occurrence's id.
//
// It is derived from the database rather than from time.Now() because the
// contract's calendar decides which day is next: run on a Sunday, or the day
// before Thanksgiving, and "today" has no occurrence at all. A fixture that
// assumed otherwise would fail on some days of the year and pass on others,
// which is the worst kind of test.
func nextExpectedFile(t *testing.T, db *sql.DB, tenantID string) (string, int64) {
	t.Helper()
	var id int64
	var business time.Time
	err := db.QueryRow(`
		SELECT id, business_date FROM expectations
		WHERE tenant_id = ? AND status = 'PENDING' AND business_date IS NOT NULL
		ORDER BY business_date LIMIT 1`, tenantID).Scan(&id, &business)
	if err != nil {
		t.Fatalf("no materialized occurrence to match against: %v", err)
	}
	d := schedule.DateOf(business, time.UTC)
	return fmt.Sprintf("payroll-%04d%02d%02d.ach", d.Year, int(d.Month), d.Day), id
}

func getSlaBoard(t *testing.T, handler http.Handler) []SlaExpectationResponse {
	t.Helper()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/sla-board", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("sla-board: %d %s", rec.Code, rec.Body.String())
	}
	var out []SlaExpectationResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &out); err != nil {
		t.Fatalf("decode sla-board: %v (body %s)", err, rec.Body.String())
	}
	return out
}

func TestUploadIsAttributedToAnExpectation(t *testing.T) {
	db, handler, _ := newIngestTestEnv(t)

	// The tenant an upload through the demo profile lands in.
	rec, resp := doUpload(t, handler, uploadRequest(t, "probe.ach", []byte("probe"), nil))
	if rec.Code != http.StatusAccepted {
		t.Fatalf("probe upload: %d %s", rec.Code, rec.Body.String())
	}
	var tenantID string
	if err := db.QueryRow(`SELECT tenant_id FROM file_instances WHERE id = ?`, resp.ArtifactID).Scan(&tenantID); err != nil {
		t.Fatal(err)
	}
	if resp.ExpectationMatch != string(schedule.MatchUnexpected) {
		t.Errorf("with no contracts configured the probe should be UNEXPECTED, got %q", resp.ExpectationMatch)
	}

	newScheduledContract(t, db, tenantID, nil)
	filename, expected := nextExpectedFile(t, db, tenantID)

	rec, resp = doUpload(t, handler, uploadRequest(t, filename, []byte("some content"), nil))
	if rec.Code != http.StatusAccepted {
		t.Fatalf("upload: %d %s", rec.Code, rec.Body.String())
	}
	if resp.ExpectationMatch != string(schedule.MatchAttributed) {
		t.Fatalf("expectationMatch = %q (%s), want ATTRIBUTED -- the scheduler is materializing "+
			"occurrences that ingest never consults", resp.ExpectationMatch, resp.ExpectationDetail)
	}
	if resp.ExpectationID != expected {
		t.Errorf("attributed to occurrence %d, want %d -- the file names its business date and "+
			"must satisfy that day's expectation, not whichever one is open",
			resp.ExpectationID, expected)
	}

	// The occurrence is ARRIVED and the artifact points back at it.
	var status string
	var matched sql.NullInt64
	if err := db.QueryRow(
		`SELECT status, matched_artifact_id FROM expectations WHERE id = ?`,
		resp.ExpectationID).Scan(&status, &matched); err != nil {
		t.Fatal(err)
	}
	if status != "ARRIVED" {
		t.Errorf("occurrence status = %s, want ARRIVED", status)
	}
	if !matched.Valid || matched.Int64 != resp.ArtifactID {
		t.Errorf("occurrence matched artifact = %v, want %d", matched, resp.ArtifactID)
	}

	var linked sql.NullInt64
	if err := db.QueryRow(
		`SELECT expectation_id FROM file_instances WHERE id = ?`, resp.ArtifactID).Scan(&linked); err != nil {
		t.Fatal(err)
	}
	if !linked.Valid || linked.Int64 != resp.ExpectationID {
		t.Errorf("artifact expectation_id = %v, want %d; the link must be navigable from both ends",
			linked, resp.ExpectationID)
	}
}

// The contract's balanced mode, not one global default, decides whether an
// unbalanced file is a violation.
func TestValidationAppliesTheContractOfTheMatchedExpectation(t *testing.T) {
	for _, tc := range []struct {
		name            string
		mode            string
		wantRequire     bool
		wantContractSet bool
	}{
		{"a balanced contract requires balance", "BALANCED", true, true},
		{"an authorized contract does not", "UNBALANCED_AUTHORIZED", false, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			db, handler, _ := newIngestTestEnv(t)

			_, probe := doUpload(t, handler, uploadRequest(t, "probe.ach", []byte("probe"), nil))
			var tenantID string
			if err := db.QueryRow(`SELECT tenant_id FROM file_instances WHERE id = ?`, probe.ArtifactID).Scan(&tenantID); err != nil {
				t.Fatal(err)
			}
			newScheduledContract(t, db, tenantID, func(in *schedule.NewVersionInput) {
				in.BalancedMode = tc.mode
			})

			filename, _ := nextExpectedFile(t, db, tenantID)
			_, resp := doUpload(t, handler, uploadRequest(t, filename, []byte("content"), nil))
			if resp.ExpectationMatch != string(schedule.MatchAttributed) {
				t.Fatalf("setup: expectationMatch = %q (%s)", resp.ExpectationMatch, resp.ExpectationDetail)
			}

			tx, err := db.Begin()
			if err != nil {
				t.Fatal(err)
			}
			defer func() { _ = tx.Rollback() }()

			contract, err := contractForArtifact(context.Background(), tx, tenantID, resp.ArtifactID)
			if err != nil {
				t.Fatal(err)
			}
			if contract.RequireBalanced != tc.wantRequire {
				t.Errorf("RequireBalanced = %v, want %v", contract.RequireBalanced, tc.wantRequire)
			}
			if tc.wantContractSet && contract.ID == "" {
				t.Error("a matched artifact must carry the contract that governed it, or a decision " +
					"cannot say which terms it applied")
			}
			if contract.Version == "" {
				t.Error("the contract version must be recorded on the decision")
			}
		})
	}
}

// An artifact matching nothing still validates, under the default, and the
// decision says no contract was applied rather than implying one.
func TestUnmatchedArtifactValidatesUnderTheDefaultContract(t *testing.T) {
	db, handler, _ := newIngestTestEnv(t)

	_, resp := doUpload(t, handler, uploadRequest(t, "nobody-expected-this.ach", []byte("x"), nil))
	if resp.ExpectationMatch != string(schedule.MatchUnexpected) {
		t.Fatalf("expectationMatch = %q, want UNEXPECTED", resp.ExpectationMatch)
	}

	var tenantID string
	if err := db.QueryRow(`SELECT tenant_id FROM file_instances WHERE id = ?`, resp.ArtifactID).Scan(&tenantID); err != nil {
		t.Fatal(err)
	}

	tx, err := db.Begin()
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = tx.Rollback() }()

	contract, err := contractForArtifact(context.Background(), tx, tenantID, resp.ArtifactID)
	if err != nil {
		t.Fatal(err)
	}
	if contract.ID != nacha.DefaultContract.ID {
		t.Errorf("contract ID = %q, want the default's empty id so the decision records that no "+
			"contract was applied", contract.ID)
	}
}

// The board must serve what the scheduler wrote, including the local reading of
// the deadline. A board showing only UTC instants cannot be checked by a
// partner against their own agreement.
func TestSlaBoardServesTheScheduledFields(t *testing.T) {
	db, handler, _ := newIngestTestEnv(t)

	_, probe := doUpload(t, handler, uploadRequest(t, "probe.ach", []byte("probe"), nil))
	var tenantID string
	if err := db.QueryRow(`SELECT tenant_id FROM file_instances WHERE id = ?`, probe.ArtifactID).Scan(&tenantID); err != nil {
		t.Fatal(err)
	}
	newScheduledContract(t, db, tenantID, nil)

	rows := getSlaBoard(t, handler)
	if len(rows) == 0 {
		t.Fatal("the board is empty although occurrences were materialized")
	}
	for _, r := range rows {
		if r.BusinessDate == "" {
			t.Error("a board row must name its business date")
		}
		if r.DueLocal == "" || r.Timezone == "" {
			t.Errorf("row for %s has no local deadline (%q %q)", r.BusinessDate, r.DueLocal, r.Timezone)
		}
		if r.BreachesAt == "" {
			t.Errorf("row for %s has no breach threshold", r.BusinessDate)
		}
		if r.FeedID != "ACME-PAYROLL" {
			t.Errorf("row for %s reports feed %q; an alert naming only the partner does not say "+
				"which file is late", r.BusinessDate, r.FeedID)
		}
		if r.Status != "PENDING" && r.Status != "DUE" && r.Status != "OVERDUE" &&
			r.Status != "BREACHED" && r.Status != "ARRIVED" && r.Status != "WAIVED" {
			t.Errorf("row for %s has status %q, which is not a modelled expectation state",
				r.BusinessDate, r.Status)
		}
	}
}

// The demo seed and the scheduler must agree.
//
// They previously did not: the seed wrote regex-style filename patterns that
// nothing in this repository can match, and inserted an expectation row by hand
// with deadlines computed in UTC rather than in the contract's timezone. A
// developer would have seen a populated board produced by a mechanism that is
// not the scheduler, which is the most misleading possible demo of a scheduler.
func TestDemoSeedProducesAMaterializableSchedule(t *testing.T) {
	db := setupTestDb(t)
	t.Cleanup(func() { db.Close() })

	if err := SeedDemo(db, ingestDemoConfig()); err != nil {
		t.Fatal(err)
	}

	// No occurrence is seeded; they come from the scheduler alone.
	var seeded int
	if err := db.QueryRow(`SELECT COUNT(*) FROM expectations`).Scan(&seeded); err != nil {
		t.Fatal(err)
	}
	if seeded != 0 {
		t.Errorf("the seed wrote %d expectations by hand; occurrences must come from the "+
			"scheduler or the demo shows a schedule the scheduler did not produce", seeded)
	}

	store, err := schedulerFor(db)
	if err != nil {
		t.Fatal(err)
	}
	res, err := store.Materialize(context.Background(), 7)
	if err != nil {
		t.Fatal(err)
	}
	if len(res.Problems) != 0 {
		t.Fatalf("the seeded contracts cannot be scheduled: %v", res.Problems)
	}
	if res.Created == 0 {
		t.Fatal("the seeded contracts produced no occurrences over a week")
	}

	// Every seeded pattern must parse, or the feed can never be matched.
	rows, err := db.Query(`SELECT feed_id, filename_pattern FROM file_contract_versions`)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	seen := 0
	for rows.Next() {
		var feed, pattern string
		if err := rows.Scan(&feed, &pattern); err != nil {
			t.Fatal(err)
		}
		seen++
		if _, err := schedule.ParsePattern(pattern); err != nil {
			t.Errorf("seeded feed %s has an unusable pattern %q: %v", feed, pattern, err)
		}
	}
	if seen == 0 {
		t.Error("the seed created no contract versions, so no feed is scheduled at all")
	}
}
