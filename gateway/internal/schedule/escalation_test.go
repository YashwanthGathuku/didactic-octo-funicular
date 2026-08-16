package schedule

import (
	"context"
	"database/sql"
	"encoding/json"
	"strings"
	"sync"
	"testing"
	"time"
)

// A breach that opens no incident and tells nobody is a breach that was
// detected and then forgotten. These tests are the difference between the two.

// recordingEscalator captures what the application would be handed.
type recordingEscalator struct {
	mu       sync.Mutex
	breaches []Breach
	fail     error
}

func (r *recordingEscalator) Breached(ctx context.Context, tx *sql.Tx, b Breach) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.fail != nil {
		return r.fail
	}
	r.breaches = append(r.breaches, b)
	return nil
}

func (r *recordingEscalator) all() []Breach {
	r.mu.Lock()
	defer r.mu.Unlock()
	out := make([]Breach, len(r.breaches))
	copy(out, r.breaches)
	return out
}

// breachOne materializes a single occurrence and drives it to BREACHED.
func breachOne(t *testing.T, db *sql.DB, driver string, mutate func(*NewVersionInput)) (*Store, int64, int64) {
	t.Helper()
	ctx := context.Background()
	due := time.Date(2025, time.June, 10, 13, 0, 0, 0, time.UTC)

	s := newStore(t, db, driver, due.Add(-2*time.Hour))
	setupFed(t, s, tenantA)
	contract := newContract(t, db, driver, tenantA, "Acme")
	in := standardInput(tenantA, contract, NewDate(2025, time.June, 1))
	if mutate != nil {
		mutate(&in)
	}
	if _, err := s.CreateVersion(ctx, in); err != nil {
		t.Fatal(err)
	}
	if _, err := s.Materialize(ctx, 1); err != nil {
		t.Fatal(err)
	}

	s.SetClock(func() time.Time { return due.Add(6 * time.Hour) })
	if _, err := s.Advance(ctx); err != nil {
		t.Fatal(err)
	}
	occ := occurrenceFor(t, s, tenantA, contract, NewDate(2025, time.June, 10))
	if occ.Status != "BREACHED" {
		t.Fatalf("status = %s, want BREACHED", occ.Status)
	}
	return s, contract, occ.ID
}

func TestABreachOpensAnIncidentAddressedToTheContractOwner(t *testing.T) {
	eachBackend(t, func(t *testing.T, db *sql.DB, driver string) {
		ctx := context.Background()
		s, _, occID := breachOne(t, db, driver, nil)

		var (
			incidentID  int64
			status      string
			severity    string
			kind        string
			summary     string
			owner       string
			policy      string
			versionID   sql.NullInt64
			expectation sql.NullInt64
		)
		err := s.db.QueryRow(s.rebind(`
			SELECT id, status, severity, type, summary, owner_subject, escalation_policy_id,
			       contract_version_id, expectation_id
			FROM incidents WHERE tenant_id = ? AND expectation_id = ?`),
			tenantA, occID).Scan(&incidentID, &status, &severity, &kind, &summary,
			&owner, &policy, &versionID, &expectation)
		if err != nil {
			t.Fatalf("a breach must open an incident: %v", err)
		}

		if status != "OPEN" {
			t.Errorf("incident status = %s, want OPEN", status)
		}
		if kind != IncidentTypeMissingFile {
			t.Errorf("incident type = %s, want %s", kind, IncidentTypeMissingFile)
		}
		if owner != "ops@example.test" {
			t.Errorf("incident owner = %q, want the contract version's owner; an alert with no "+
				"recipient is the same as no alert", owner)
		}
		if policy != "policy/payments-oncall" {
			t.Errorf("escalation policy = %q, want the contract version's", policy)
		}
		if !versionID.Valid {
			t.Error("the incident must name the contract version whose terms it was raised under")
		}
		if !strings.Contains(summary, "ACME-PAYROLL") || !strings.Contains(summary, "2025-06-10") {
			t.Errorf("summary = %q; it must name the feed and the business date", summary)
		}

		open, err := s.OpenIncidents(ctx, tenantA)
		if err != nil {
			t.Fatal(err)
		}
		if open != 1 {
			t.Errorf("open incidents = %d, want 1", open)
		}
	})
}

func TestABreachWritesExactlyOneNotificationIntent(t *testing.T) {
	eachBackend(t, func(t *testing.T, db *sql.DB, driver string) {
		ctx := context.Background()
		s, _, occID := breachOne(t, db, driver, nil)

		pending, err := s.PendingNotifications(ctx, tenantA, 10)
		if err != nil {
			t.Fatal(err)
		}
		if len(pending) != 1 {
			t.Fatalf("pending notifications = %d, want 1", len(pending))
		}
		n := pending[0]
		if n.Kind != NotificationKindBreach {
			t.Errorf("kind = %s, want %s", n.Kind, NotificationKindBreach)
		}
		if n.Recipient != "ops@example.test" {
			t.Errorf("recipient = %q; a dispatcher must be able to route without parsing the payload", n.Recipient)
		}
		if n.SubjectID != occID {
			t.Errorf("subject = %d, want the occurrence %d", n.SubjectID, occID)
		}

		var payload map[string]any
		if err := json.Unmarshal([]byte(n.Payload), &payload); err != nil {
			t.Fatalf("payload is not JSON: %v", err)
		}
		for _, key := range []string{"incidentId", "expectationId", "feedId", "businessDate", "dueAt", "breachedAt"} {
			if _, ok := payload[key]; !ok {
				t.Errorf("payload has no %q; the recipient cannot act on an alert that does not "+
					"say which file, for which day, due when", key)
			}
		}
		// A notification is the most widely distributed artifact this system
		// produces, so nothing derived from file content may appear in it.
		for _, forbidden := range []string{"filenamePattern", "rawData", "record", "accountNumber"} {
			if _, ok := payload[forbidden]; ok {
				t.Errorf("payload carries %q, which must never leave this system in an alert", forbidden)
			}
		}
	})
}

func TestTheEscalatorReceivesTheTermsInForceAtTheBreach(t *testing.T) {
	eachBackend(t, func(t *testing.T, db *sql.DB, driver string) {
		esc := &recordingEscalator{}
		due := time.Date(2025, time.June, 10, 13, 0, 0, 0, time.UTC)

		ctx := context.Background()
		s := newStore(t, db, driver, due.Add(-2*time.Hour))
		s.SetEscalator(esc)
		setupFed(t, s, tenantA)
		contract := newContract(t, db, driver, tenantA, "Acme")
		if _, err := s.CreateVersion(ctx, standardInput(tenantA, contract, NewDate(2025, time.June, 1))); err != nil {
			t.Fatal(err)
		}
		if _, err := s.Materialize(ctx, 1); err != nil {
			t.Fatal(err)
		}
		s.SetClock(func() time.Time { return due.Add(6 * time.Hour) })
		if _, err := s.Advance(ctx); err != nil {
			t.Fatal(err)
		}

		got := esc.all()
		if len(got) == 0 {
			t.Fatal("the escalator was never called; a breach must reach the application")
		}
		b := got[0]
		if b.OwnerSubject != "ops@example.test" || b.EscalationPolicyID != "policy/payments-oncall" {
			t.Errorf("breach carried owner %q policy %q, want the contract version's",
				b.OwnerSubject, b.EscalationPolicyID)
		}
		if b.FeedID != "ACME-PAYROLL" {
			t.Errorf("feed = %q, want ACME-PAYROLL", b.FeedID)
		}
		if b.IncidentID == 0 {
			t.Error("the escalator must be told which incident was opened")
		}
		if b.DueLocal != "09:00:00" || b.Timezone != "America/New_York" {
			t.Errorf("breach carried local deadline %q %q; an alert stating only a UTC instant "+
				"cannot be checked against the agreement", b.DueLocal, b.Timezone)
		}
	})
}

// An escalator that fails must not leave the breach recorded without it.
func TestAFailingEscalatorRollsBackTheTransition(t *testing.T) {
	eachBackend(t, func(t *testing.T, db *sql.DB, driver string) {
		ctx := context.Background()
		esc := &recordingEscalator{fail: errNotDelivered}
		due := time.Date(2025, time.June, 10, 13, 0, 0, 0, time.UTC)

		s := newStore(t, db, driver, due.Add(-2*time.Hour))
		s.SetEscalator(esc)
		setupFed(t, s, tenantA)
		contract := newContract(t, db, driver, tenantA, "Acme")
		if _, err := s.CreateVersion(ctx, standardInput(tenantA, contract, NewDate(2025, time.June, 1))); err != nil {
			t.Fatal(err)
		}
		if _, err := s.Materialize(ctx, 1); err != nil {
			t.Fatal(err)
		}

		s.SetClock(func() time.Time { return due.Add(6 * time.Hour) })
		if _, err := s.Advance(ctx); err == nil {
			t.Fatal("an escalation failure must surface, not be swallowed")
		}

		occ := occurrenceFor(t, s, tenantA, contract, NewDate(2025, time.June, 10))
		if occ.Status == "BREACHED" {
			t.Error("the occurrence is BREACHED although escalation failed; the state and the " +
				"obligation to tell someone must commit together or not at all")
		}
		var incidents int
		if err := s.db.QueryRow(s.rebind(
			`SELECT COUNT(*) FROM incidents WHERE tenant_id = ?`), tenantA).Scan(&incidents); err != nil {
			t.Fatal(err)
		}
		if incidents != 0 {
			t.Errorf("incidents = %d, want 0: the transaction rolled back", incidents)
		}
	})
}

var errNotDelivered = &escalationError{}

type escalationError struct{}

func (*escalationError) Error() string { return "the notification channel is unavailable" }

// Advancing twice must not page anyone twice.
func TestRepeatedAdvancementDoesNotDuplicateTheAlert(t *testing.T) {
	eachBackend(t, func(t *testing.T, db *sql.DB, driver string) {
		ctx := context.Background()
		s, _, occID := breachOne(t, db, driver, nil)

		// Force the occurrence back to OVERDUE and let it breach again, which
		// is what a retry after a partial failure looks like.
		if _, err := s.db.Exec(s.rebind(
			`UPDATE expectations SET status = 'OVERDUE' WHERE tenant_id = ? AND id = ?`),
			tenantA, occID); err != nil {
			t.Fatal(err)
		}
		if _, err := s.Advance(ctx); err != nil {
			t.Fatal(err)
		}

		var incidents, notifications int
		if err := s.db.QueryRow(s.rebind(
			`SELECT COUNT(*) FROM incidents WHERE tenant_id = ? AND expectation_id = ?`),
			tenantA, occID).Scan(&incidents); err != nil {
			t.Fatal(err)
		}
		if incidents != 1 {
			t.Errorf("incidents = %d, want 1; a duplicate alert is how an operator's queue stops "+
				"being read", incidents)
		}
		if err := s.db.QueryRow(s.rebind(
			`SELECT COUNT(*) FROM notification_intents WHERE tenant_id = ? AND subject_id = ?`),
			tenantA, occID).Scan(&notifications); err != nil {
			t.Fatal(err)
		}
		if notifications != 1 {
			t.Errorf("notification intents = %d, want 1", notifications)
		}
	})
}

func TestConcurrentSchedulersRaiseOneIncident(t *testing.T) {
	eachBackend(t, func(t *testing.T, db *sql.DB, driver string) {
		ctx := context.Background()
		due := time.Date(2025, time.June, 10, 13, 0, 0, 0, time.UTC)

		s := newStore(t, db, driver, due.Add(-2*time.Hour))
		setupFed(t, s, tenantA)
		contract := newContract(t, db, driver, tenantA, "Acme")
		if _, err := s.CreateVersion(ctx, standardInput(tenantA, contract, NewDate(2025, time.June, 1))); err != nil {
			t.Fatal(err)
		}
		if _, err := s.Materialize(ctx, 3); err != nil {
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
				if _, err := peer.Advance(ctx); err != nil {
					t.Errorf("concurrent advance: %v", err)
				}
			}()
		}
		wg.Wait()

		rows, err := s.db.Query(s.rebind(`
			SELECT expectation_id, COUNT(*) FROM incidents
			WHERE tenant_id = ? GROUP BY expectation_id HAVING COUNT(*) > 1`), tenantA)
		if err != nil {
			t.Fatal(err)
		}
		defer rows.Close()
		for rows.Next() {
			var id int64
			var n int
			if err := rows.Scan(&id, &n); err != nil {
				t.Fatal(err)
			}
			t.Errorf("occurrence %d raised %d incidents; the unique index must collapse them", id, n)
		}
	})
}

// ---------------------------------------------------------------------------
// Review resolution
// ---------------------------------------------------------------------------

// setupAmbiguity produces one artifact matching two open occurrences.
func setupAmbiguity(t *testing.T, db *sql.DB, driver string) (*Store, int64, []int64) {
	t.Helper()
	ctx := context.Background()
	at := time.Date(2025, time.June, 10, 14, 0, 0, 0, time.UTC)

	s := newStore(t, db, driver, time.Date(2025, time.June, 10, 12, 0, 0, 0, time.UTC))
	setupFed(t, s, tenantA)
	contract := newContract(t, db, driver, tenantA, "Acme")
	in := standardInput(tenantA, contract, NewDate(2025, time.June, 1))
	in.FilenamePattern = "ACH_daily.txt"
	if _, err := s.CreateVersion(ctx, in); err != nil {
		t.Fatal(err)
	}
	if _, err := s.Materialize(ctx, 2); err != nil {
		t.Fatal(err)
	}

	s.SetClock(func() time.Time { return at })
	artifact := newArtifact(t, s, tenantA, "ACH_daily.txt")
	res := matchArrival(t, s, tenantA, artifact, "ACH_daily.txt", at)
	if res.Outcome != MatchAmbiguous {
		t.Fatalf("setup: outcome = %s, want AMBIGUOUS", res.Outcome)
	}

	rows, err := s.db.Query(s.rebind(`
		SELECT id FROM expectation_match_candidates
		WHERE tenant_id = ? AND resolution = 'REVIEW_REQUIRED' ORDER BY id`), tenantA)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	var candidates []int64
	for rows.Next() {
		var id int64
		if err := rows.Scan(&id); err != nil {
			t.Fatal(err)
		}
		candidates = append(candidates, id)
	}
	if len(candidates) < 2 {
		t.Fatalf("setup: %d candidates, want at least 2", len(candidates))
	}
	return s, artifact, candidates
}

func TestAcceptingACandidateAttributesTheArtifactAndClosesTheOthers(t *testing.T) {
	eachBackend(t, func(t *testing.T, db *sql.DB, driver string) {
		ctx := context.Background()
		s, artifact, candidates := setupAmbiguity(t, db, driver)

		if err := s.ResolveCandidate(ctx, tenantA, candidates[0], true,
			"analyst@example.test", "partner confirmed this is Tuesday's file"); err != nil {
			t.Fatal(err)
		}

		// Exactly one occurrence arrived, and it is the accepted one.
		arrived := 0
		for _, o := range occurrences(t, s, tenantA) {
			if o.Status == "ARRIVED" {
				arrived++
				if !o.Matched.Valid || o.Matched.Int64 != artifact {
					t.Errorf("the arrived occurrence points at artifact %v, want %d", o.Matched, artifact)
				}
			}
			if o.Review != 0 {
				t.Errorf("occurrence %s is still flagged for review after the decision", o.Business)
			}
		}
		if arrived != 1 {
			t.Errorf("arrived occurrences = %d, want exactly 1; one file satisfies one expectation", arrived)
		}

		// The same artifact cannot satisfy the others, so those questions are
		// answered by the same decision.
		open, err := s.OpenReviews(ctx, tenantA)
		if err != nil {
			t.Fatal(err)
		}
		if open != 0 {
			t.Errorf("open reviews = %d, want 0; a resolved decision must not return to the queue", open)
		}
	})
}

func TestRejectingACandidateLeavesTheOccurrenceAgeing(t *testing.T) {
	eachBackend(t, func(t *testing.T, db *sql.DB, driver string) {
		ctx := context.Background()
		s, _, candidates := setupAmbiguity(t, db, driver)

		for _, id := range candidates {
			if err := s.ResolveCandidate(ctx, tenantA, id, false,
				"analyst@example.test", "this file belongs to a different feed"); err != nil {
				t.Fatal(err)
			}
		}

		for _, o := range occurrences(t, s, tenantA) {
			if o.Status == "ARRIVED" {
				t.Errorf("occurrence %s was marked arrived although every candidate was rejected", o.Business)
			}
			if o.Review != 0 {
				t.Errorf("occurrence %s is still under review after every candidate was decided", o.Business)
			}
		}

		// And it still breaches, because no file arrived for it.
		s.SetClock(func() time.Time { return time.Date(2025, time.June, 14, 12, 0, 0, 0, time.UTC) })
		if _, err := s.Advance(ctx); err != nil {
			t.Fatal(err)
		}
		for _, o := range occurrences(t, s, tenantA) {
			if o.Status != "BREACHED" {
				t.Errorf("occurrence %s is %s; rejecting every candidate means nothing arrived",
					o.Business, o.Status)
			}
		}
	})
}

func TestResolvingRequiresAnActorAndAReasonAndHappensOnce(t *testing.T) {
	eachBackend(t, func(t *testing.T, db *sql.DB, driver string) {
		ctx := context.Background()
		s, _, candidates := setupAmbiguity(t, db, driver)

		if err := s.ResolveCandidate(ctx, tenantA, candidates[0], true, "", "a reason"); err == nil {
			t.Error("resolving with no actor must be refused; this is the one place a person " +
				"overrides what the system could not determine")
		}
		if err := s.ResolveCandidate(ctx, tenantA, candidates[0], true, "analyst", ""); err == nil {
			t.Error("resolving with no reason must be refused")
		}

		if err := s.ResolveCandidate(ctx, tenantA, candidates[0], false, "analyst", "not ours"); err != nil {
			t.Fatal(err)
		}
		if err := s.ResolveCandidate(ctx, tenantA, candidates[0], true, "analyst", "changed my mind"); err == nil {
			t.Error("a candidate must not be resolved twice")
		}
	})
}

func TestACandidateOfAnotherTenantIsNotResolvable(t *testing.T) {
	eachBackend(t, func(t *testing.T, db *sql.DB, driver string) {
		ctx := context.Background()
		s, _, candidates := setupAmbiguity(t, db, driver)

		if err := s.ResolveCandidate(ctx, "TENANT-B", candidates[0], true,
			"attacker@example.test", "trying another tenant's queue"); err == nil {
			t.Error("a candidate must not be resolvable by another tenant")
		}
	})
}

// ---------------------------------------------------------------------------
// Waivers
// ---------------------------------------------------------------------------

func TestWaivingStopsTheClockAndResolvesTheIncident(t *testing.T) {
	eachBackend(t, func(t *testing.T, db *sql.DB, driver string) {
		ctx := context.Background()
		s, contract, occID := breachOne(t, db, driver, nil)

		if err := s.Waive(ctx, tenantA, occID, "manager@example.test",
			"partner confirmed no file is produced on this date"); err != nil {
			t.Fatal(err)
		}

		occ := occurrenceFor(t, s, tenantA, contract, NewDate(2025, time.June, 10))
		if occ.Status != "WAIVED" {
			t.Fatalf("status = %s, want WAIVED", occ.Status)
		}

		var by, reason sql.NullString
		if err := s.db.QueryRow(s.rebind(
			`SELECT waived_by, waived_reason FROM expectations WHERE id = ?`), occID).Scan(&by, &reason); err != nil {
			t.Fatal(err)
		}
		if by.String != "manager@example.test" || reason.String == "" {
			t.Error("a waiver must record who decided and why; it is the one operation that makes " +
				"a missing-file alert go away without a file arriving")
		}

		open, err := s.OpenIncidents(ctx, tenantA)
		if err != nil {
			t.Fatal(err)
		}
		if open != 0 {
			t.Errorf("open incidents = %d, want 0; a waived occurrence's incident closes with it", open)
		}

		// It must not age any further.
		s.SetClock(func() time.Time { return time.Date(2025, time.July, 1, 12, 0, 0, 0, time.UTC) })
		if _, err := s.Advance(ctx); err != nil {
			t.Fatal(err)
		}
		occ = occurrenceFor(t, s, tenantA, contract, NewDate(2025, time.June, 10))
		if occ.Status != "WAIVED" {
			t.Errorf("status = %s after a later pass, want WAIVED", occ.Status)
		}
	})
}

func TestWaivingRequiresAnActorAndAReason(t *testing.T) {
	eachBackend(t, func(t *testing.T, db *sql.DB, driver string) {
		ctx := context.Background()
		s, _, occID := breachOne(t, db, driver, nil)

		if err := s.Waive(ctx, tenantA, occID, "", "a reason"); err == nil {
			t.Error("waiving with no actor must be refused")
		}
		if err := s.Waive(ctx, tenantA, occID, "manager", ""); err == nil {
			t.Error("waiving with no reason must be refused")
		}
	})
}

func TestAnArrivedOccurrenceCannotBeWaived(t *testing.T) {
	eachBackend(t, func(t *testing.T, db *sql.DB, driver string) {
		ctx := context.Background()
		at := time.Date(2025, time.June, 10, 12, 0, 0, 0, time.UTC)
		s := newStore(t, db, driver, at)
		setupFed(t, s, tenantA)
		contract := newContract(t, db, driver, tenantA, "Acme")
		if _, err := s.CreateVersion(ctx, standardInput(tenantA, contract, NewDate(2025, time.June, 1))); err != nil {
			t.Fatal(err)
		}
		if _, err := s.Materialize(ctx, 1); err != nil {
			t.Fatal(err)
		}
		artifact := newArtifact(t, s, tenantA, "ACH_20250610.txt")
		if res := matchArrival(t, s, tenantA, artifact, "ACH_20250610.txt", at); res.Outcome != MatchAttributed {
			t.Fatalf("setup: %s", res.Outcome)
		}
		occ := occurrenceFor(t, s, tenantA, contract, NewDate(2025, time.June, 10))

		if err := s.Waive(ctx, tenantA, occ.ID, "manager", "trying to waive a satisfied expectation"); err == nil {
			t.Error("an occurrence that has already arrived must not be waivable; the file exists")
		}
	})
}

func TestWaivingAnotherTenantsOccurrenceIsRefused(t *testing.T) {
	eachBackend(t, func(t *testing.T, db *sql.DB, driver string) {
		ctx := context.Background()
		s, _, occID := breachOne(t, db, driver, nil)

		if err := s.Waive(ctx, "TENANT-B", occID, "attacker@example.test",
			"suppressing another tenant's alert"); err == nil {
			t.Error("waiving across tenants must be refused")
		}
	})
}
