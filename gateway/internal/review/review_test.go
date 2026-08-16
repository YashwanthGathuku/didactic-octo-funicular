package review

import (
	"context"
	"database/sql"
	"errors"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"sentinel-gateway/internal/domain"

	_ "github.com/jackc/pgx/v5/stdlib"
	_ "modernc.org/sqlite"
)

// Human review and dual-control release.
//
// The properties here are the ones that decide whether money moves. Each test
// constructs the failure rather than asserting a flag: a second vote by the
// same person is actually cast, a changed artifact is actually written, two
// reviewers actually race.

// ---------------------------------------------------------------------------
// Fixtures
// ---------------------------------------------------------------------------

type backend struct {
	name   string
	driver string
	open   func(t *testing.T) *sql.DB
}

func backends(t *testing.T) []backend {
	t.Helper()
	out := []backend{{name: "sqlite", driver: "sqlite", open: openSQLite}}
	if os.Getenv("SENTINEL_TEST_POSTGRES_DSN") != "" {
		out = append(out, backend{name: "postgres", driver: "pgx", open: openPostgres})
	} else {
		t.Log("SKIPPED for postgres: SENTINEL_TEST_POSTGRES_DSN is unset, so the optimistic " +
			"concurrency that decides simultaneous approve/reject is NOT verified against PostgreSQL")
	}
	return out
}

func openSQLite(t *testing.T) *sql.DB {
	t.Helper()
	path := filepath.Join(t.TempDir(), "review.db")
	db, err := sql.Open("sqlite", path+"?_pragma=busy_timeout(10000)&_pragma=journal_mode(WAL)&_pragma=foreign_keys(1)")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })

	for _, name := range []string{
		"001_init_schema.sql", "002_tenancy_and_state.sql", "003_secret_store.sql",
		"004_artifact_storage.sql", "005_redacted_findings.sql", "006_jobs_and_outbox.sql",
		"007_ledger_integrity.sql", "008_scheduling.sql", "009_breach_escalation.sql",
		"010_source_connections.sql", "011_dual_control_release.sql",
	} {
		body, err := os.ReadFile(filepath.Join("..", "..", "migrations", name))
		if err != nil {
			t.Fatalf("read %s: %v", name, err)
		}
		if _, err := db.Exec(string(body)); err != nil {
			t.Fatalf("apply %s: %v", name, err)
		}
	}
	seed(t, db)
	return db
}

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
	schema := "review_test_" + time.Now().Format("20060102150405.000000000")
	schema = sanitize(schema)
	if _, err := db.Exec(`CREATE SCHEMA ` + schema); err != nil {
		db.Close()
		t.Fatalf("create schema: %v", err)
	}
	db.Close()

	sep := "?"
	for _, r := range dsn {
		if r == '?' {
			sep = "&"
			break
		}
	}
	db, err = sql.Open("pgx", dsn+sep+"search_path="+schema)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		_, _ = db.Exec(`DROP SCHEMA ` + schema + ` CASCADE`)
		db.Close()
	})
	if _, err := db.Exec(postgresReviewSchema); err != nil {
		t.Fatalf("apply review schema: %v", err)
	}
	seed(t, db)
	return db
}

func sanitize(s string) string {
	out := make([]rune, 0, len(s))
	for _, r := range s {
		if r == '.' || r == '-' {
			out = append(out, '_')
			continue
		}
		out = append(out, r)
	}
	return string(out)
}

// migrations_postgres/ does not carry these tables -- the application still
// runs on SQLite -- so the shape under test is built here rather than
// pretending a port that has not happened.
const postgresReviewSchema = `
CREATE TABLE tenants (id TEXT PRIMARY KEY, name TEXT NOT NULL);
CREATE TABLE file_instances (
    id BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    tenant_id TEXT NOT NULL REFERENCES tenants(id),
    filename TEXT NOT NULL,
    storage_path TEXT NOT NULL DEFAULT '',
    size_bytes BIGINT NOT NULL DEFAULT 0,
    sha256_hash TEXT NOT NULL DEFAULT '',
    status TEXT NOT NULL DEFAULT 'RECEIVED',
    row_version BIGINT NOT NULL DEFAULT 0,
    received_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
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
CREATE TABLE validation_runs (
    id BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    tenant_id TEXT NOT NULL REFERENCES tenants(id),
    file_instance_id BIGINT NOT NULL REFERENCES file_instances(id),
    parser_name TEXT NOT NULL,
    parser_version TEXT NOT NULL,
    rule_pack_version TEXT NOT NULL,
    parser_ok INTEGER NOT NULL CHECK (parser_ok IN (0,1)),
    records_parsed INTEGER NOT NULL DEFAULT 0,
    total_debits_minor BIGINT NOT NULL DEFAULT 0,
    total_credits_minor BIGINT NOT NULL DEFAULT 0,
    policy_version TEXT NOT NULL DEFAULT '',
    contract_id TEXT NOT NULL DEFAULT '',
    contract_version TEXT NOT NULL DEFAULT '',
    outcome TEXT NOT NULL DEFAULT '',
    findings_digest TEXT NOT NULL DEFAULT '',
    blocking_rule_ids TEXT NOT NULL DEFAULT '',
    started_at TIMESTAMPTZ NOT NULL,
    completed_at TIMESTAMPTZ
);
CREATE TABLE release_policies (
    tenant_id TEXT PRIMARY KEY REFERENCES tenants(id),
    min_approvals INTEGER NOT NULL DEFAULT 2 CHECK (min_approvals >= 1),
    separation_of_duties INTEGER NOT NULL DEFAULT 1,
    override_allowed INTEGER NOT NULL DEFAULT 1,
    updated_by TEXT NOT NULL DEFAULT '',
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE TABLE policy_decisions (
    id BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    tenant_id TEXT NOT NULL REFERENCES tenants(id),
    file_instance_id BIGINT NOT NULL REFERENCES file_instances(id),
    validation_run_id BIGINT NOT NULL REFERENCES validation_runs(id),
    policy_version TEXT NOT NULL,
    state TEXT NOT NULL CHECK (state IN ('PROPOSED','APPROVED','REJECTED','EXPIRED')),
    outcome TEXT NOT NULL CHECK (outcome IN ('VALIDATED','QUARANTINED')),
    artifact_sha256 TEXT NOT NULL,
    reason TEXT,
    row_version BIGINT NOT NULL DEFAULT 0,
    integrity_digest TEXT NOT NULL DEFAULT '',
    findings_digest TEXT NOT NULL DEFAULT '',
    proposed_by TEXT NOT NULL DEFAULT '',
    proposed_at TIMESTAMPTZ,
    required_approvals INTEGER NOT NULL DEFAULT 2,
    separation_of_duties INTEGER NOT NULL DEFAULT 1,
    contract_version_id BIGINT,
    expired_reason TEXT,
    released_at TIMESTAMPTZ,
    released_by TEXT,
    decided_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE (tenant_id, validation_run_id)
);
CREATE TABLE approvals (
    id BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    tenant_id TEXT NOT NULL REFERENCES tenants(id),
    decision_id BIGINT NOT NULL REFERENCES policy_decisions(id),
    actor_id TEXT NOT NULL CHECK (length(actor_id) > 0),
    role TEXT NOT NULL,
    reason TEXT NOT NULL CHECK (length(reason) > 0),
    vote TEXT NOT NULL DEFAULT 'APPROVE' CHECK (vote IN ('APPROVE','REJECT')),
    integrity_digest TEXT NOT NULL DEFAULT '',
    approved_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE (tenant_id, decision_id, actor_id)
);
CREATE TABLE release_overrides (
    id BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    tenant_id TEXT NOT NULL REFERENCES tenants(id),
    decision_id BIGINT NOT NULL REFERENCES policy_decisions(id),
    file_instance_id BIGINT NOT NULL REFERENCES file_instances(id),
    actor_id TEXT NOT NULL,
    role TEXT NOT NULL,
    reason TEXT NOT NULL,
    bypassed TEXT NOT NULL,
    approvals_held INTEGER NOT NULL DEFAULT 0,
    approvals_required INTEGER NOT NULL,
    blocking_rule_ids TEXT NOT NULL DEFAULT '',
    integrity_digest TEXT NOT NULL DEFAULT '',
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE (tenant_id, decision_id)
);
`

func seed(t *testing.T, db *sql.DB) {
	t.Helper()
	for _, id := range []string{"TENANT-A", "TENANT-B"} {
		if _, err := db.Exec(`INSERT INTO tenants (id, name) VALUES ($1, $2)`, id, id); err != nil {
			if _, err2 := db.Exec(`INSERT INTO tenants (id, name) VALUES (?, ?)`, id, id); err2 != nil {
				t.Fatalf("seed tenant: %v", err2)
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

const tenantA = "TENANT-A"

func newStore(t *testing.T, db *sql.DB, driver string) *Store {
	t.Helper()
	s, err := NewStore(db, driver)
	if err != nil {
		t.Fatal(err)
	}
	return s
}

// newArtifact inserts a VALIDATED artifact ready to be released.
func newArtifact(t *testing.T, s *Store, tenantID, sha, status string) int64 {
	t.Helper()
	const insert = `INSERT INTO file_instances
	                (tenant_id, filename, storage_path, size_bytes, sha256_hash, status, received_at, updated_at)
	                VALUES (?, 'payroll.ach', '', 94, ?, ?, ?, ?)`
	now := time.Now().UTC()
	if s.dialect == dialectPostgres {
		var id int64
		if err := s.db.QueryRow(s.rebind(insert+" RETURNING id"),
			tenantID, sha, status, now, now).Scan(&id); err != nil {
			t.Fatal(err)
		}
		return id
	}
	res, err := s.db.Exec(insert, tenantID, sha, status, now, now)
	if err != nil {
		t.Fatal(err)
	}
	id, _ := res.LastInsertId()
	return id
}

func subjectFor(artifactID int64, sha string, findings ...Finding) Subject {
	return Subject{
		ArtifactID:      artifactID,
		ArtifactSHA256:  sha,
		PolicyVersion:   "release-policy/1.0.0",
		ContractID:      "ACME-PAYROLL",
		ContractVersion: "v1",
		Outcome:         "VALIDATED",
		Findings:        findings,
		ParserName:      "internal/nacha",
		ParserVersion:   "1",
		RulePackVersion: "nacha-rules/1.0.0",
		RecordsParsed:   4,
	}
}

// propose creates a decision the way the worker does.
func propose(t *testing.T, s *Store, tenantID string, subject Subject, policy Policy) *Decision {
	t.Helper()
	ctx := context.Background()

	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		t.Fatal(err)
	}
	id, err := s.ProposeTx(ctx, tx, tenantID, "system:validation-worker", subject, policy)
	if err != nil {
		_ = tx.Rollback()
		t.Fatal(err)
	}
	if err := tx.Commit(); err != nil {
		t.Fatal(err)
	}

	d, err := s.Get(ctx, tenantID, id)
	if err != nil {
		t.Fatal(err)
	}
	return d
}

// currentSubject rebuilds the subject from the decision, as a caller would.
func currentSubject(d *Decision, findings ...Finding) Subject {
	s := subjectFor(d.ArtifactID, d.ArtifactSHA256, findings...)
	s.ValidationRunID = d.ValidationRunID
	return s
}

// ---------------------------------------------------------------------------
// Dual control
// ---------------------------------------------------------------------------

func TestOnePersonCannotSatisfyTwoPersonControl(t *testing.T) {
	eachBackend(t, func(t *testing.T, db *sql.DB, driver string) {
		ctx := context.Background()
		s := newStore(t, db, driver)

		artifact := newArtifact(t, s, tenantA, "sha-one", "VALIDATED")
		d := propose(t, s, tenantA, subjectFor(artifact, "sha-one"), DefaultPolicy())
		current := currentSubject(d)

		if _, err := s.Vote(ctx, tenantA, "reviewer-a", "REVIEWER", current,
			VoteRequest{DecisionID: d.ID, Approve: true, Reason: "checked the totals"}); err != nil {
			t.Fatal(err)
		}

		// The same person votes again. The friendly check refuses it, and so
		// does the storage constraint underneath.
		if _, err := s.Vote(ctx, tenantA, "reviewer-a", "REVIEWER", current,
			VoteRequest{DecisionID: d.ID, Approve: true, Reason: "checking again"}); !errors.Is(err, ErrAlreadyVoted) {
			t.Fatalf("a second vote by the same person must be refused, got %v", err)
		}

		after, err := s.Get(ctx, tenantA, d.ID)
		if err != nil {
			t.Fatal(err)
		}
		if after.ApprovalsHeld() != 1 {
			t.Errorf("approvals held = %d after one person voted twice, want 1", after.ApprovalsHeld())
		}
		if after.State == domain.DecisionApproved {
			t.Error("one person clicking twice satisfied dual control")
		}
		if _, err := s.Release(ctx, tenantA, "reviewer-a", d.ID, current); !errors.Is(err, ErrNotApproved) {
			t.Errorf("release must be refused below the threshold, got %v", err)
		}

		// A second, distinct person completes it.
		if _, err := s.Vote(ctx, tenantA, "reviewer-b", "REVIEWER", current,
			VoteRequest{DecisionID: d.ID, Approve: true, Reason: "independently checked"}); err != nil {
			t.Fatal(err)
		}
		after, _ = s.Get(ctx, tenantA, d.ID)
		if after.State != domain.DecisionApproved {
			t.Fatalf("state = %s after two distinct approvals, want APPROVED", after.State)
		}
		if _, err := s.Release(ctx, tenantA, "releaser", d.ID, current); err != nil {
			t.Fatalf("release: %v", err)
		}
	})
}

func TestTheProposerCannotBeTheSecondApprover(t *testing.T) {
	eachBackend(t, func(t *testing.T, db *sql.DB, driver string) {
		ctx := context.Background()
		s := newStore(t, db, driver)

		artifact := newArtifact(t, s, tenantA, "sha-prop", "VALIDATED")

		// A person proposes this one, rather than the worker.
		tx, err := s.db.BeginTx(ctx, nil)
		if err != nil {
			t.Fatal(err)
		}
		id, err := s.ProposeTx(ctx, tx, tenantA, "analyst-a",
			subjectFor(artifact, "sha-prop"), DefaultPolicy())
		if err != nil {
			t.Fatal(err)
		}
		if err := tx.Commit(); err != nil {
			t.Fatal(err)
		}
		d, err := s.Get(ctx, tenantA, id)
		if err != nil {
			t.Fatal(err)
		}
		current := currentSubject(d)

		if _, err := s.Vote(ctx, tenantA, "analyst-a", "REVIEWER", current,
			VoteRequest{DecisionID: d.ID, Approve: true, Reason: "I proposed it and I approve it"}); !errors.Is(err, ErrSelfApproval) {
			t.Fatalf("the proposer must not be able to approve their own proposal, got %v", err)
		}
	})
}

func TestSeparationOfDutiesCanBeConfiguredOff(t *testing.T) {
	eachBackend(t, func(t *testing.T, db *sql.DB, driver string) {
		ctx := context.Background()
		s := newStore(t, db, driver)

		relaxed := Policy{TenantID: tenantA, MinApprovals: 1, SeparationOfDuties: false, OverrideAllowed: true}
		if err := s.SetPolicy(ctx, "admin", relaxed); err != nil {
			t.Fatal(err)
		}
		loaded, err := s.Policy(ctx, tenantA)
		if err != nil {
			t.Fatal(err)
		}
		if loaded.MinApprovals != 1 || loaded.SeparationOfDuties {
			t.Fatalf("policy did not round trip: %+v", loaded)
		}

		artifact := newArtifact(t, s, tenantA, "sha-relaxed", "VALIDATED")
		tx, _ := s.db.BeginTx(ctx, nil)
		id, err := s.ProposeTx(ctx, tx, tenantA, "analyst-a", subjectFor(artifact, "sha-relaxed"), loaded)
		if err != nil {
			t.Fatal(err)
		}
		_ = tx.Commit()

		d, _ := s.Get(ctx, tenantA, id)
		if _, err := s.Vote(ctx, tenantA, "analyst-a", "REVIEWER", currentSubject(d),
			VoteRequest{DecisionID: d.ID, Approve: true, Reason: "single control is configured"}); err != nil {
			t.Fatalf("with separation of duties off the proposer may approve: %v", err)
		}
	})
}

// A tenant with no configuration gets the strict policy, not the weak one.
func TestAnUnconfiguredTenantGetsTheStrictPolicy(t *testing.T) {
	eachBackend(t, func(t *testing.T, db *sql.DB, driver string) {
		s := newStore(t, db, driver)
		p, err := s.Policy(context.Background(), "TENANT-NEVER-CONFIGURED")
		if err != nil {
			t.Fatal(err)
		}
		if p.MinApprovals < 2 || !p.SeparationOfDuties {
			t.Errorf("an unconfigured tenant got %+v; the absence of configuration must not be "+
				"the weakest setting", p)
		}
	})
}

// ---------------------------------------------------------------------------
// Staleness
// ---------------------------------------------------------------------------

func TestAStaleApprovalCannotReleaseChangedContent(t *testing.T) {
	eachBackend(t, func(t *testing.T, db *sql.DB, driver string) {
		ctx := context.Background()
		s := newStore(t, db, driver)

		artifact := newArtifact(t, s, tenantA, "sha-original", "VALIDATED")
		d := propose(t, s, tenantA, subjectFor(artifact, "sha-original"), DefaultPolicy())
		current := currentSubject(d)

		for _, actor := range []string{"reviewer-a", "reviewer-b"} {
			if _, err := s.Vote(ctx, tenantA, actor, "REVIEWER", current,
				VoteRequest{DecisionID: d.ID, Approve: true, Reason: "checked"}); err != nil {
				t.Fatal(err)
			}
		}
		approved, _ := s.Get(ctx, tenantA, d.ID)
		if approved.State != domain.DecisionApproved {
			t.Fatalf("state = %s, want APPROVED", approved.State)
		}

		// The content changes underneath the approval.
		changed := currentSubject(d)
		changed.ArtifactSHA256 = "sha-different"

		_, err := s.Release(ctx, tenantA, "releaser", d.ID, changed)
		if !errors.Is(err, ErrStale) {
			t.Fatalf("a stale approval released changed content: %v", err)
		}

		// And it is expired, not merely refused -- otherwise the next caller
		// tries again and again against a still-approved decision.
		after, _ := s.Get(ctx, tenantA, d.ID)
		if after.State != domain.DecisionExpired {
			t.Errorf("state = %s after a stale release attempt, want EXPIRED", after.State)
		}
		if after.ExpiredReason == "" {
			t.Error("the expiry gives no reason; 'the approval expired' is not actionable")
		}
	})
}

// Each of the four things that can change must expire an approval, and the
// error must name which one.
func TestEveryKindOfChangeExpiresAnApproval(t *testing.T) {
	base := subjectFor(7, "sha-a", Finding{RuleID: "R1", Severity: "WARNING"})
	base.ValidationRunID = 11

	d := Decision{
		ArtifactID: 7, ArtifactSHA256: "sha-a", ValidationRunID: 11,
		PolicyVersion: base.PolicyVersion, RulePackVersion: base.RulePackVersion,
		IntegrityDigest: base.IntegrityDigest(),
		FindingsDigest:  base.FindingsDigest(),
	}

	if err := d.CheckFresh(base); err != nil {
		t.Fatalf("an unchanged subject must be fresh: %v", err)
	}

	for name, mutate := range map[string]func(Subject) Subject{
		"the artifact's content changed": func(s Subject) Subject {
			s.ArtifactSHA256 = "sha-b"
			return s
		},
		"the validation findings changed": func(s Subject) Subject {
			s.Findings = append(s.Findings, Finding{RuleID: "R2", Severity: "BLOCKING"})
			return s
		},
		"the release policy version changed": func(s Subject) Subject {
			s.PolicyVersion = "release-policy/2.0.0"
			return s
		},
		"the artifact was revalidated": func(s Subject) Subject {
			s.ValidationRunID = 12
			return s
		},
		"the governing contract or outcome changed": func(s Subject) Subject {
			s.ContractVersion = "v2"
			return s
		},
		"the validation rule pack changed": func(s Subject) Subject {
			s.RulePackVersion = "nacha-rules/2.0.0"
			return s
		},
	} {
		err := d.CheckFresh(mutate(base))
		if err == nil {
			t.Errorf("%s did not expire the approval", name)
			continue
		}
		if !errors.Is(err, ErrStale) {
			t.Errorf("%s produced %v, want a staleness error", name, err)
		}
		if got := err.Error(); !contains(got, name) {
			t.Errorf("the error for %q reads %q; it must name what changed, because 'the "+
				"approval expired' is not actionable", name, got)
		}
	}
}

func contains(haystack, needle string) bool {
	return len(haystack) >= len(needle) && (haystack == needle ||
		len(needle) == 0 || indexOf(haystack, needle) >= 0)
}

func indexOf(h, n string) int {
	for i := 0; i+len(n) <= len(h); i++ {
		if h[i:i+len(n)] == n {
			return i
		}
	}
	return -1
}

// A vote cast before the findings changed must not count towards a release.
func TestAVoteAgainstAnOlderStateDoesNotCount(t *testing.T) {
	older := subjectFor(3, "sha", Finding{RuleID: "R1", Severity: "WARNING"})
	newer := subjectFor(3, "sha", Finding{RuleID: "R1", Severity: "BLOCKING"})

	d := Decision{
		ProposedBy: "system:validation-worker", RequiredApprovals: 2,
		SeparationOfDuties: true, IntegrityDigest: newer.IntegrityDigest(),
		State: domain.DecisionApproved,
		Votes: []Vote{
			{ActorID: "a", Choice: "APPROVE", Digest: older.IntegrityDigest()},
			{ActorID: "b", Choice: "APPROVE", Digest: newer.IntegrityDigest()},
		},
	}
	if got := d.ApprovalsHeld(); got != 1 {
		t.Errorf("approvals held = %d, want 1; a reviewer approved a state of the world that "+
			"no longer holds", got)
	}
	if d.Releasable() {
		t.Error("a decision holding one live approval of two was releasable")
	}
}

// ---------------------------------------------------------------------------
// Concurrency
// ---------------------------------------------------------------------------

func TestSimultaneousApproveAndRejectProducesOneLegalOutcome(t *testing.T) {
	eachBackend(t, func(t *testing.T, db *sql.DB, driver string) {
		ctx := context.Background()
		s := newStore(t, db, driver)
		db.SetMaxOpenConns(8)

		artifact := newArtifact(t, s, tenantA, "sha-race", "VALIDATED")
		policy := Policy{MinApprovals: 1, SeparationOfDuties: true, OverrideAllowed: true}
		d := propose(t, s, tenantA, subjectFor(artifact, "sha-race"), policy)
		current := currentSubject(d)

		var wg sync.WaitGroup
		results := make(chan string, 2)
		start := make(chan struct{})

		for _, v := range []struct {
			actor   string
			approve bool
		}{{"reviewer-a", true}, {"reviewer-b", false}} {
			wg.Add(1)
			go func() {
				defer wg.Done()
				<-start
				_, err := s.Vote(ctx, tenantA, v.actor, "REVIEWER", current,
					VoteRequest{DecisionID: d.ID, Approve: v.approve, Reason: "simultaneous"})
				if err != nil {
					results <- "error: " + err.Error()
					return
				}
				results <- v.actor
			}()
		}
		close(start)
		wg.Wait()
		close(results)
		for r := range results {
			t.Logf("vote outcome: %s", r)
		}

		after, err := s.Get(ctx, tenantA, d.ID)
		if err != nil {
			t.Fatal(err)
		}

		// Exactly one legal state, and it is consistent with the votes.
		switch after.State {
		case domain.DecisionRejected:
			if !after.Rejected() {
				t.Error("the decision is REJECTED with no live rejection")
			}
		case domain.DecisionApproved:
			if after.Rejected() {
				t.Error("the decision is APPROVED while a reviewer rejected it")
			}
		case domain.DecisionProposed:
			// Legal: one vote landed and the other lost the race.
		default:
			t.Errorf("state = %s, which is not a legal outcome of one approve and one reject",
				after.State)
		}

		// A rejected decision must never release, whatever the approval count.
		if after.Rejected() {
			if _, err := s.Release(ctx, tenantA, "releaser", d.ID, current); err == nil {
				t.Error("a rejected decision was released")
			}
		}
	})
}

func TestConcurrentReleasesProduceOneRelease(t *testing.T) {
	eachBackend(t, func(t *testing.T, db *sql.DB, driver string) {
		ctx := context.Background()
		s := newStore(t, db, driver)
		db.SetMaxOpenConns(8)

		artifact := newArtifact(t, s, tenantA, "sha-once", "VALIDATED")
		d := propose(t, s, tenantA, subjectFor(artifact, "sha-once"), DefaultPolicy())
		current := currentSubject(d)
		for _, actor := range []string{"reviewer-a", "reviewer-b"} {
			if _, err := s.Vote(ctx, tenantA, actor, "REVIEWER", current,
				VoteRequest{DecisionID: d.ID, Approve: true, Reason: "checked"}); err != nil {
				t.Fatal(err)
			}
		}

		var wg sync.WaitGroup
		var mu sync.Mutex
		succeeded := 0
		for range 6 {
			wg.Add(1)
			go func() {
				defer wg.Done()
				if _, err := s.Release(ctx, tenantA, "releaser", d.ID, current); err == nil {
					mu.Lock()
					succeeded++
					mu.Unlock()
				}
			}()
		}
		wg.Wait()

		if succeeded != 1 {
			t.Errorf("%d concurrent releases succeeded, want exactly 1", succeeded)
		}

		var transitions int
		if err := s.db.QueryRow(s.rebind(`
			SELECT COUNT(*) FROM status_history
			WHERE tenant_id = ? AND object_type = 'artifact' AND object_id = ? AND to_state = 'RELEASED'`),
			tenantA, artifact).Scan(&transitions); err != nil {
			t.Fatal(err)
		}
		if transitions != 1 {
			t.Errorf("%d RELEASED transitions recorded, want 1", transitions)
		}
	})
}

// ---------------------------------------------------------------------------
// Authorization and tenancy
// ---------------------------------------------------------------------------

func TestAnEmptyActorIsRefusedEverywhere(t *testing.T) {
	eachBackend(t, func(t *testing.T, db *sql.DB, driver string) {
		ctx := context.Background()
		s := newStore(t, db, driver)

		artifact := newArtifact(t, s, tenantA, "sha-actor", "VALIDATED")
		d := propose(t, s, tenantA, subjectFor(artifact, "sha-actor"), DefaultPolicy())
		current := currentSubject(d)

		// The actor is a parameter this package takes from the caller, and the
		// caller takes it from a verified principal. An empty one is a
		// programming error and is refused rather than recorded as an
		// anonymous decision.
		if _, err := s.Vote(ctx, tenantA, "", "REVIEWER", current,
			VoteRequest{DecisionID: d.ID, Approve: true, Reason: "anonymous"}); err == nil {
			t.Error("a vote with no actor was accepted")
		}
		if _, err := s.Release(ctx, tenantA, "", d.ID, current); err == nil {
			t.Error("a release with no actor was accepted")
		}
		if _, err := s.Override(ctx, tenantA, "", current, OverrideRequest{
			DecisionID: d.ID, Reason: "a sufficiently long override justification here"}); err == nil {
			t.Error("an override with no actor was accepted")
		}
	})
}

func TestAnotherTenantCannotSeeOrDecide(t *testing.T) {
	eachBackend(t, func(t *testing.T, db *sql.DB, driver string) {
		ctx := context.Background()
		s := newStore(t, db, driver)

		artifact := newArtifact(t, s, tenantA, "sha-tenant", "VALIDATED")
		d := propose(t, s, tenantA, subjectFor(artifact, "sha-tenant"), DefaultPolicy())
		current := currentSubject(d)

		if _, err := s.Get(ctx, "TENANT-B", d.ID); !errors.Is(err, ErrNotFound) {
			t.Errorf("another tenant read the decision: %v", err)
		}
		if _, err := s.Vote(ctx, "TENANT-B", "attacker", "REVIEWER", current,
			VoteRequest{DecisionID: d.ID, Approve: true, Reason: "cross tenant"}); !errors.Is(err, ErrNotFound) {
			t.Errorf("another tenant voted on the decision: %v", err)
		}
		if _, err := s.Release(ctx, "TENANT-B", "attacker", d.ID, current); !errors.Is(err, ErrNotFound) {
			t.Errorf("another tenant released the artifact: %v", err)
		}

		queue, err := s.Queue(ctx, "TENANT-B", 10)
		if err != nil {
			t.Fatal(err)
		}
		if len(queue) != 0 {
			t.Errorf("another tenant's review queue contains %d decisions", len(queue))
		}
	})
}

// ---------------------------------------------------------------------------
// Override
// ---------------------------------------------------------------------------

func TestAnOverrideIsRecordedSeparatelyAndNeverRewritesFindings(t *testing.T) {
	eachBackend(t, func(t *testing.T, db *sql.DB, driver string) {
		ctx := context.Background()
		s := newStore(t, db, driver)

		artifact := newArtifact(t, s, tenantA, "sha-override", "VALIDATED")
		findings := []Finding{{RuleID: "NACHA-BATCH-CONTROL", Severity: "BLOCKING"}}
		d := propose(t, s, tenantA, subjectFor(artifact, "sha-override", findings...), DefaultPolicy())
		current := currentSubject(d, findings...)

		// No approvals at all: the override is bypassing dual control.
		res, err := s.Override(ctx, tenantA, "supervisor", current, OverrideRequest{
			DecisionID: d.ID, Role: "APPROVER",
			Reason: "partner confirmed the control total by telephone and will resend tomorrow",
		})
		if err != nil {
			t.Fatalf("override: %v", err)
		}
		if !res.Overridden {
			t.Error("the result does not report itself as an override")
		}

		// Separately reportable, without anyone needing to know which flag to
		// look at.
		records, err := s.Overrides(ctx, tenantA, 10)
		if err != nil {
			t.Fatal(err)
		}
		if len(records) != 1 {
			t.Fatalf("overrides reported = %d, want 1", len(records))
		}
		r := records[0]
		if r.ActorID != "supervisor" {
			t.Errorf("override actor = %q", r.ActorID)
		}
		if r.ApprovalsHeld != 0 || r.ApprovalsRequired != 2 {
			t.Errorf("override recorded %d of %d approvals", r.ApprovalsHeld, r.ApprovalsRequired)
		}
		if len(r.BlockingRuleIDs) != 1 || r.BlockingRuleIDs[0] != "NACHA-BATCH-CONTROL" {
			t.Errorf("the override does not record what was blocking: %v", r.BlockingRuleIDs)
		}
		if !contains(r.Bypassed, "dual control") || !contains(r.Bypassed, "blocking findings") {
			t.Errorf("bypassed = %q; it must state what was stepped around", r.Bypassed)
		}

		// The decision's findings digest is untouched: an override records that
		// a human released anyway, it does not resolve the findings.
		after, _ := s.Get(ctx, tenantA, d.ID)
		if after.FindingsDigest != d.FindingsDigest {
			t.Error("the override changed the findings digest; it must never rewrite the " +
				"validation result")
		}
	})
}

func TestAnOverrideNeedsARealReason(t *testing.T) {
	eachBackend(t, func(t *testing.T, db *sql.DB, driver string) {
		ctx := context.Background()
		s := newStore(t, db, driver)

		artifact := newArtifact(t, s, tenantA, "sha-reason", "VALIDATED")
		d := propose(t, s, tenantA, subjectFor(artifact, "sha-reason"), DefaultPolicy())
		current := currentSubject(d)

		for _, reason := range []string{"", "ok", "urgent", "approved by phone"} {
			if _, err := s.Override(ctx, tenantA, "supervisor", current, OverrideRequest{
				DecisionID: d.ID, Reason: reason}); err == nil {
				t.Errorf("override accepted the reason %q", reason)
			}
		}
	})
}

func TestATenantCanForbidOverridesEntirely(t *testing.T) {
	eachBackend(t, func(t *testing.T, db *sql.DB, driver string) {
		ctx := context.Background()
		s := newStore(t, db, driver)

		if err := s.SetPolicy(ctx, "admin", Policy{
			TenantID: tenantA, MinApprovals: 2, SeparationOfDuties: true, OverrideAllowed: false,
		}); err != nil {
			t.Fatal(err)
		}

		artifact := newArtifact(t, s, tenantA, "sha-noverride", "VALIDATED")
		d := propose(t, s, tenantA, subjectFor(artifact, "sha-noverride"), DefaultPolicy())
		current := currentSubject(d)

		if _, err := s.Override(ctx, tenantA, "supervisor", current, OverrideRequest{
			DecisionID: d.ID,
			Reason:     "there is genuinely no time to find a second approver right now",
		}); !errors.Is(err, ErrOverrideNotAllowed) {
			t.Fatalf("a tenant that forbids overrides was overridden: %v", err)
		}
	})
}

// An override may bypass an absent approval. It may not bypass a person who
// looked at the file and said no.
func TestAnOverrideCannotBypassARejection(t *testing.T) {
	eachBackend(t, func(t *testing.T, db *sql.DB, driver string) {
		ctx := context.Background()
		s := newStore(t, db, driver)

		artifact := newArtifact(t, s, tenantA, "sha-rejected", "VALIDATED")
		d := propose(t, s, tenantA, subjectFor(artifact, "sha-rejected"), DefaultPolicy())
		current := currentSubject(d)

		if _, err := s.Vote(ctx, tenantA, "reviewer-a", "REVIEWER", current,
			VoteRequest{DecisionID: d.ID, Approve: false, Reason: "the totals do not reconcile"}); err != nil {
			t.Fatal(err)
		}

		if _, err := s.Override(ctx, tenantA, "supervisor", current, OverrideRequest{
			DecisionID: d.ID,
			Reason:     "overriding the reviewer because the deadline is in fifteen minutes",
		}); err == nil {
			t.Error("an override bypassed a reviewer's rejection; that is not an unmet control, " +
				"it is a person saying no")
		}
	})
}

// Quarantine is not overridable.
func TestAQuarantinedArtifactCannotBeReleasedAtAll(t *testing.T) {
	eachBackend(t, func(t *testing.T, db *sql.DB, driver string) {
		ctx := context.Background()
		s := newStore(t, db, driver)

		artifact := newArtifact(t, s, tenantA, "sha-quarantined", "QUARANTINED")
		subject := subjectFor(artifact, "sha-quarantined",
			Finding{RuleID: "NACHA-RECORD-LENGTH", Severity: "BLOCKING"})
		subject.Outcome = "QUARANTINED"
		d := propose(t, s, tenantA, subject, DefaultPolicy())
		current := currentSubject(d, Finding{RuleID: "NACHA-RECORD-LENGTH", Severity: "BLOCKING"})
		current.Outcome = "QUARANTINED"

		if _, err := s.Release(ctx, tenantA, "releaser", d.ID, current); err == nil {
			t.Error("a quarantined artifact was released")
		}
		if _, err := s.Override(ctx, tenantA, "supervisor", current, OverrideRequest{
			DecisionID: d.ID,
			Reason:     "the partner insists the file is correct despite the record length finding",
		}); err == nil {
			t.Error("a quarantined artifact was released by override; quarantine would be advisory")
		}

		var state string
		if err := s.db.QueryRow(s.rebind(
			`SELECT status FROM file_instances WHERE id = ?`), artifact).Scan(&state); err != nil {
			t.Fatal(err)
		}
		if state != "QUARANTINED" {
			t.Errorf("artifact state = %s after two release attempts, want QUARANTINED", state)
		}
	})
}

// ---------------------------------------------------------------------------
// The queue
// ---------------------------------------------------------------------------

func TestTheQueueCarriesNoEvidence(t *testing.T) {
	eachBackend(t, func(t *testing.T, db *sql.DB, driver string) {
		ctx := context.Background()
		s := newStore(t, db, driver)

		artifact := newArtifact(t, s, tenantA, "sha-queue", "VALIDATED")
		propose(t, s, tenantA, subjectFor(artifact, "sha-queue",
			Finding{RuleID: "NACHA-ENTRY-HASH", Severity: "BLOCKING"}), DefaultPolicy())

		queue, err := s.Queue(ctx, tenantA, 10)
		if err != nil {
			t.Fatal(err)
		}
		if len(queue) != 1 {
			t.Fatalf("queue holds %d decisions, want 1", len(queue))
		}
		d := queue[0]
		if d.ArtifactID != artifact {
			t.Errorf("the queued decision names artifact %d, want %d", d.ArtifactID, artifact)
		}
		// The subject is referenced by identifier and digest. Nothing derived
		// from file content appears, and the findings themselves were already
		// redacted by internal/nacha where they were written.
		if d.ArtifactSHA256 == "" || d.IntegrityDigest == "" {
			t.Error("a queued decision must name what it is about")
		}
		if d.RequiredApprovals < 1 {
			t.Error("a queued decision must state how many approvals it needs")
		}
	})
}
