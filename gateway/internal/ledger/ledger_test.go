package ledger

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	_ "github.com/jackc/pgx/v5/stdlib"
	_ "modernc.org/sqlite"
)

// The chain is verified against both databases. The per-tenant serialisation
// mechanism differs -- PostgreSQL locks the tenant row, SQLite relies on its
// single writer -- so a suite that ran against one would verify half the code.

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
		t.Log("SKIPPED for postgres: SENTINEL_TEST_POSTGRES_DSN is unset, so per-tenant append serialisation is NOT verified against PostgreSQL by this run")
	}
	return out
}

func openSQLite(t *testing.T) *sql.DB {
	t.Helper()
	// File-backed: an in-memory SQLite database belongs to a single connection,
	// so a concurrency test on a pool would see per-goroutine empty chains.
	path := filepath.Join(t.TempDir(), "ledger.db")
	db, err := sql.Open("sqlite", path+"?_pragma=busy_timeout(10000)&_pragma=journal_mode(WAL)&_pragma=foreign_keys(1)")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })

	for _, name := range []string{
		"001_init_schema.sql", "002_tenancy_and_state.sql", "003_secret_store.sql",
		"004_artifact_storage.sql", "005_redacted_findings.sql",
		"006_jobs_and_outbox.sql", "007_ledger_integrity.sql",
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

func openPostgres(t *testing.T) *sql.DB {
	t.Helper()
	dsn := os.Getenv("SENTINEL_TEST_POSTGRES_DSN")
	admin, err := sql.Open("pgx", dsn)
	if err != nil {
		t.Fatal(err)
	}
	if err := admin.Ping(); err != nil {
		t.Fatalf("ping postgres: %v", err)
	}
	schema := fmt.Sprintf("ledger_test_%d", time.Now().UnixNano())
	if _, err := admin.Exec(`CREATE SCHEMA ` + schema); err != nil {
		t.Fatalf("create schema: %v", err)
	}
	admin.Close()

	db, err := sql.Open("pgx", dsn+"&search_path="+schema)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		db.Close()
		if cleanup, err := sql.Open("pgx", dsn); err == nil {
			_, _ = cleanup.Exec(`DROP SCHEMA ` + schema + ` CASCADE`)
			cleanup.Close()
		}
	})

	if _, err := db.Exec(postgresLedgerSchema); err != nil {
		t.Fatalf("apply ledger schema: %v", err)
	}
	seedTenants(t, db)
	return db
}

const postgresLedgerSchema = `
CREATE TABLE tenants (id TEXT PRIMARY KEY, name TEXT NOT NULL);
CREATE TABLE audit_events (
    id                BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    tenant_id         TEXT NOT NULL REFERENCES tenants(id),
    sequence_no       BIGINT NOT NULL,
    event_type        TEXT NOT NULL,
    actor             TEXT NOT NULL,
    object_type       TEXT,
    object_id         BIGINT,
    correlation_id    TEXT,
    payload           TEXT NOT NULL,
    payload_hash      TEXT NOT NULL DEFAULT '',
    previous_hash     TEXT NOT NULL,
    current_hash      TEXT NOT NULL,
    canonical_version TEXT NOT NULL DEFAULT '',
    created_at        TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE (tenant_id, sequence_no),
    UNIQUE (tenant_id, previous_hash)
);
`

func seedTenants(t *testing.T, db *sql.DB) {
	t.Helper()
	for _, id := range []string{"TENANT-A", "TENANT-B"} {
		if _, err := db.Exec(`INSERT INTO tenants (id, name) VALUES ($1, $2)`, id, id); err != nil {
			if _, err2 := db.Exec(`INSERT INTO tenants (id, name) VALUES (?, ?)`, id, id); err2 != nil {
				t.Logf("seed %s: %v", id, err2)
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

func newLedger(t *testing.T, db *sql.DB, driver string) *Ledger {
	t.Helper()
	l, err := New(db, driver)
	if err != nil {
		t.Fatal(err)
	}
	return l
}

// tamperDirectly drops the append-only triggers so a test can simulate an
// attacker with direct database write access.
//
// The triggers are doing their job -- every tamper test below failed with
// "audit_events is append-only" until this existed. Weakening the trigger to
// make the tests pass would have removed a production control to satisfy a
// test; dropping it inside the test models the threat the hash chain exists to
// detect, which is someone who already has database access. That the triggers
// hold is asserted separately, in TestAuditEventsAreAppendOnlyToTheApplication.
func tamperDirectly(t *testing.T, db *sql.DB, statements ...string) {
	t.Helper()
	for _, trigger := range []string{"audit_events_no_update", "audit_events_no_delete"} {
		if _, err := db.Exec(`DROP TRIGGER IF EXISTS ` + trigger); err != nil {
			t.Logf("drop %s: %v", trigger, err)
		}
	}
	for _, stmt := range statements {
		if _, err := db.Exec(stmt); err != nil {
			t.Fatalf("tamper %q: %v", stmt, err)
		}
	}
}

func appendN(t *testing.T, l *Ledger, tenant string, n int) {
	t.Helper()
	for i := 0; i < n; i++ {
		if _, err := l.Append(context.Background(), AppendRequest{
			TenantID: tenant, Action: "ARTIFACT_RECEIVED", Actor: "system:test",
			ObjectType: "artifact", ObjectID: int64(i + 1),
			CorrelationID: fmt.Sprintf("corr-%d", i),
			Payload:       map[string]any{"index": i},
		}); err != nil {
			t.Fatalf("append %d: %v", i, err)
		}
	}
}

// --- Concurrency ---

// The headline property: concurrent appends produce one linear sequence.
//
// The implementation this replaces read the last hash outside any transaction
// and inserted on a different connection, so two appends could read the same
// predecessor. The unique constraints stopped a fork, but the loser got a
// constraint error and its record was dropped.
func TestConcurrentAppendsProduceOneLinearSequence(t *testing.T) {
	eachBackend(t, func(t *testing.T, db *sql.DB, driver string) {
		db.SetMaxOpenConns(16)
		l := newLedger(t, db, driver)

		const writers = 24
		const perWriter = 8
		var failures atomic.Int64
		var wg sync.WaitGroup
		start := make(chan struct{})

		for w := 0; w < writers; w++ {
			wg.Add(1)
			go func(w int) {
				defer wg.Done()
				<-start
				for i := 0; i < perWriter; i++ {
					if _, err := l.Append(context.Background(), AppendRequest{
						TenantID: "TENANT-A", Action: "ARTIFACT_RECEIVED",
						Actor: fmt.Sprintf("system:writer-%d", w), ObjectType: "artifact",
						ObjectID: int64(w*perWriter + i), CorrelationID: fmt.Sprintf("w%d-i%d", w, i),
						Payload: map[string]any{"writer": w, "index": i},
					}); err != nil {
						failures.Add(1)
						t.Errorf("writer %d append %d: %v", w, i, err)
					}
				}
			}(w)
		}
		close(start)
		wg.Wait()

		if failures.Load() > 0 {
			t.Fatalf("%d appends were dropped under contention", failures.Load())
		}

		// Every record landed, the sequence is dense, and the chain verifies.
		var count int64
		if err := db.QueryRow(`SELECT COUNT(*) FROM audit_events WHERE tenant_id = 'TENANT-A'`).Scan(&count); err != nil {
			t.Fatal(err)
		}
		if want := int64(writers * perWriter); count != want {
			t.Errorf("%d records written, want %d", count, want)
		}

		result, err := l.Verify(context.Background(), "TENANT-A")
		if err != nil {
			t.Fatal(err)
		}
		if !result.Intact {
			t.Errorf("the chain is broken at sequence %d after concurrent appends: %s",
				result.FirstBreakAt, result.Reason)
		}
		if result.RecordsChecked != count {
			t.Errorf("verification covered %d records, %d exist", result.RecordsChecked, count)
		}
		t.Logf("observed: %d concurrent writers produced one linear chain of %d records",
			writers, result.RecordsChecked)
	})
}

// --- Tamper detection ---

func TestVerifyDetectsMutation(t *testing.T) {
	eachBackend(t, func(t *testing.T, db *sql.DB, driver string) {
		l := newLedger(t, db, driver)
		appendN(t, l, "TENANT-A", 10)

		if r, _ := l.Verify(context.Background(), "TENANT-A"); !r.Intact {
			t.Fatalf("precondition: the chain is already broken: %s", r.Reason)
		}

		// An attacker with database access alters a payload. The chain hash is
		// left alone, which is the realistic case: recomputing every subsequent
		// digest is the work the chain forces.
		tamperDirectly(t, db,
			`UPDATE audit_events SET payload = '{"index":999}' WHERE tenant_id = 'TENANT-A' AND sequence_no = 5`)

		result, err := l.Verify(context.Background(), "TENANT-A")
		if err != nil {
			t.Fatal(err)
		}
		if result.Intact {
			t.Fatal("a mutated payload was not detected")
		}
		if result.FirstBreakAt != 5 {
			t.Errorf("break reported at sequence %d, want 5", result.FirstBreakAt)
		}
		if !strings.Contains(result.Reason, "payload") {
			t.Errorf("the reason does not name the payload: %s", result.Reason)
		}
	})
}

// Mutating a non-payload field must also be caught, or a record's actor could
// be rewritten while its payload hash still matched.
func TestVerifyDetectsActorMutation(t *testing.T) {
	eachBackend(t, func(t *testing.T, db *sql.DB, driver string) {
		l := newLedger(t, db, driver)
		appendN(t, l, "TENANT-A", 5)

		tamperDirectly(t, db,
			`UPDATE audit_events SET actor = 'someone-else' WHERE tenant_id = 'TENANT-A' AND sequence_no = 3`)

		result, _ := l.Verify(context.Background(), "TENANT-A")
		if result.Intact {
			t.Fatal("a rewritten actor was not detected")
		}
		if result.FirstBreakAt != 3 {
			t.Errorf("break at %d, want 3", result.FirstBreakAt)
		}
	})
}

func TestVerifyDetectsDeletion(t *testing.T) {
	eachBackend(t, func(t *testing.T, db *sql.DB, driver string) {
		l := newLedger(t, db, driver)
		appendN(t, l, "TENANT-A", 10)

		tamperDirectly(t, db, `DELETE FROM audit_events WHERE tenant_id = 'TENANT-A' AND sequence_no = 6`)

		result, _ := l.Verify(context.Background(), "TENANT-A")
		if result.Intact {
			t.Fatal("a deleted record was not detected")
		}
		if !strings.Contains(result.Reason, "gap") && !strings.Contains(result.Reason, "reorder") {
			t.Errorf("the reason does not describe a gap: %s", result.Reason)
		}
	})
}

func TestVerifyDetectsReordering(t *testing.T) {
	eachBackend(t, func(t *testing.T, db *sql.DB, driver string) {
		l := newLedger(t, db, driver)
		appendN(t, l, "TENANT-A", 6)

		// Swap two records' sequence numbers, which is what a reorder looks
		// like to a reader ordering by sequence.
		tamperDirectly(t, db,
			`UPDATE audit_events SET sequence_no = 99 WHERE tenant_id = 'TENANT-A' AND sequence_no = 3`,
			`UPDATE audit_events SET sequence_no = 3 WHERE tenant_id = 'TENANT-A' AND sequence_no = 4`,
			`UPDATE audit_events SET sequence_no = 4 WHERE tenant_id = 'TENANT-A' AND sequence_no = 99`)

		result, _ := l.Verify(context.Background(), "TENANT-A")
		if result.Intact {
			t.Fatal("a reordered pair was not detected")
		}
	})
}

// A broken predecessor link, with everything else left consistent.
func TestVerifyDetectsBrokenPredecessorLink(t *testing.T) {
	eachBackend(t, func(t *testing.T, db *sql.DB, driver string) {
		l := newLedger(t, db, driver)
		appendN(t, l, "TENANT-A", 5)

		// A hash no record claims. Setting it to the genesis hash instead is
		// refused by the UNIQUE (tenant_id, previous_hash) constraint, because
		// record 1 already claims that predecessor -- which is the constraint
		// preventing a fork, working.
		const unclaimed = "deadbeefdeadbeefdeadbeefdeadbeefdeadbeefdeadbeefdeadbeefdeadbeef"
		tamperDirectly(t, db, `UPDATE audit_events SET previous_hash = '`+unclaimed+
			`' WHERE tenant_id = 'TENANT-A' AND sequence_no = 4`)

		result, _ := l.Verify(context.Background(), "TENANT-A")
		if result.Intact {
			t.Fatal("a broken predecessor link was not detected")
		}
		if result.FirstBreakAt != 4 {
			t.Errorf("break at %d, want 4", result.FirstBreakAt)
		}
	})
}

// A record claiming an unknown canonical version cannot be verified, and that
// is reported rather than skipped -- skipping would let an attacker exempt a
// record from checking by relabelling it.
func TestUnknownCanonicalVersionIsReportedNotSkipped(t *testing.T) {
	eachBackend(t, func(t *testing.T, db *sql.DB, driver string) {
		l := newLedger(t, db, driver)
		appendN(t, l, "TENANT-A", 4)

		tamperDirectly(t, db,
			`UPDATE audit_events SET canonical_version = 'ledger-canonical/99' WHERE tenant_id = 'TENANT-A' AND sequence_no = 2`)

		result, _ := l.Verify(context.Background(), "TENANT-A")
		if result.Intact {
			t.Fatal("a record with an unknown canonical version was treated as verified")
		}
		if !strings.Contains(result.Reason, "canonical version") {
			t.Errorf("the reason does not name the version mismatch: %s", result.Reason)
		}
	})
}

// --- Tenant isolation ---

// Tenant streams are independent: each starts at sequence 1 from genesis, and
// neither's records appear in the other's verification.
func TestTenantStreamsCannotReferenceEachOther(t *testing.T) {
	eachBackend(t, func(t *testing.T, db *sql.DB, driver string) {
		l := newLedger(t, db, driver)
		appendN(t, l, "TENANT-A", 5)
		appendN(t, l, "TENANT-B", 3)

		for tenant, want := range map[string]int64{"TENANT-A": 5, "TENANT-B": 3} {
			result, err := l.Verify(context.Background(), tenant)
			if err != nil {
				t.Fatal(err)
			}
			if !result.Intact {
				t.Errorf("%s: chain broken: %s", tenant, result.Reason)
			}
			if result.RecordsChecked != want {
				t.Errorf("%s: verified %d records, want %d", tenant, result.RecordsChecked, want)
			}
		}

		// Each stream begins at genesis independently.
		var aFirst, bFirst string
		db.QueryRow(`SELECT previous_hash FROM audit_events WHERE tenant_id = 'TENANT-A' AND sequence_no = 1`).Scan(&aFirst)
		db.QueryRow(`SELECT previous_hash FROM audit_events WHERE tenant_id = 'TENANT-B' AND sequence_no = 1`).Scan(&bFirst)
		if aFirst != GenesisHash || bFirst != GenesisHash {
			t.Errorf("a tenant stream does not begin at genesis: A=%s B=%s", aFirst, bFirst)
		}

		// And breaking one does not break the other.
		tamperDirectly(t, db, `DELETE FROM audit_events WHERE tenant_id = 'TENANT-A' AND sequence_no = 3`)
		aResult, _ := l.Verify(context.Background(), "TENANT-A")
		bResult, _ := l.Verify(context.Background(), "TENANT-B")
		if aResult.Intact {
			t.Error("TENANT-A's chain should be broken")
		}
		if !bResult.Intact {
			t.Error("breaking TENANT-A's chain also broke TENANT-B's; the streams are not independent")
		}
	})
}

// --- Payload rules ---

func TestPayloadSizeIsBounded(t *testing.T) {
	eachBackend(t, func(t *testing.T, db *sql.DB, driver string) {
		l := newLedger(t, db, driver)

		_, err := l.Append(context.Background(), AppendRequest{
			TenantID: "TENANT-A", Action: "X", Actor: "system:test", ObjectType: "artifact",
			Payload: map[string]any{"blob": strings.Repeat("a", MaxPayloadBytes+1)},
		})
		if !errors.Is(err, ErrPayloadTooLarge) {
			t.Errorf("got %v, want ErrPayloadTooLarge", err)
		}

		var n int
		db.QueryRow(`SELECT COUNT(*) FROM audit_events`).Scan(&n)
		if n != 0 {
			t.Errorf("an oversized payload wrote %d records", n)
		}
	})
}

// Sensitive content must never enter the ledger. This is a backstop -- callers
// pass identifiers and already-redacted findings -- but the backstop must work.
func TestForbiddenPayloadContentIsRefused(t *testing.T) {
	eachBackend(t, func(t *testing.T, db *sql.DB, driver string) {
		l := newLedger(t, db, driver)

		forbidden := []map[string]any{
			{"rawData": "6220210000211234567890      0000250000EMP-00101"},
			{"raw_content": "anything"},
			{"secret": "a-value"},
			{"apiKey": "a-value"},
			{"password": "a-value"},
			{"accountNumber": "1234567890"},
			{"private_key": "a-value"},
			// A record-shaped value under an innocent key is caught by shape.
			// A genuine 94-character ACH entry detail record.
			{"note": "622021000021123456789012345670000250000EMP-00101      GATHU JOHN              0021000010000001"},
		}

		for _, payload := range forbidden {
			_, err := l.Append(context.Background(), AppendRequest{
				TenantID: "TENANT-A", Action: "X", Actor: "system:test",
				ObjectType: "artifact", Payload: payload,
			})
			if !errors.Is(err, ErrForbiddenPayload) {
				t.Errorf("payload %v returned %v, want ErrForbiddenPayload", payload, err)
			}
		}

		var n int
		db.QueryRow(`SELECT COUNT(*) FROM audit_events`).Scan(&n)
		if n != 0 {
			t.Errorf("%d forbidden payloads were recorded", n)
		}

		// And an ordinary payload is still accepted.
		if _, err := l.Append(context.Background(), AppendRequest{
			TenantID: "TENANT-A", Action: "ARTIFACT_QUARANTINED", Actor: "system:test",
			ObjectType: "artifact", ObjectID: 1,
			Payload: map[string]any{"artifactId": 1, "findingCount": 3, "policyVersion": "release-policy/1.0.0"},
		}); err != nil {
			t.Errorf("an ordinary payload was refused: %v", err)
		}
	})
}

// --- Required fields ---

func TestAppendRequiresAttribution(t *testing.T) {
	eachBackend(t, func(t *testing.T, db *sql.DB, driver string) {
		l := newLedger(t, db, driver)
		base := AppendRequest{
			TenantID: "TENANT-A", Action: "X", Actor: "system:test", ObjectType: "artifact",
		}

		missing := map[string]func(AppendRequest) AppendRequest{
			"tenant":     func(r AppendRequest) AppendRequest { r.TenantID = ""; return r },
			"action":     func(r AppendRequest) AppendRequest { r.Action = ""; return r },
			"actor":      func(r AppendRequest) AppendRequest { r.Actor = ""; return r },
			"objectType": func(r AppendRequest) AppendRequest { r.ObjectType = ""; return r },
		}
		for name, mutate := range missing {
			if _, err := l.Append(context.Background(), mutate(base)); err == nil {
				t.Errorf("a record with no %s was accepted", name)
			}
		}
	})
}

// Every stored record carries the fields the guide requires.
func TestStoredRecordsCarryEveryRequiredField(t *testing.T) {
	eachBackend(t, func(t *testing.T, db *sql.DB, driver string) {
		l := newLedger(t, db, driver)

		rec, err := l.Append(context.Background(), AppendRequest{
			TenantID: "TENANT-A", Action: "ARTIFACT_VALIDATED", Actor: "auth0|reviewer",
			ObjectType: "artifact", ObjectID: 42, CorrelationID: "req-abc-123",
			Payload: map[string]any{"artifactId": 42},
		})
		if err != nil {
			t.Fatal(err)
		}

		for name, value := range map[string]string{
			"tenant":           rec.TenantID,
			"action":           rec.Action,
			"actor":            rec.Actor,
			"objectType":       rec.ObjectType,
			"correlationId":    rec.CorrelationID,
			"payloadHash":      rec.PayloadHash,
			"previousHash":     rec.PreviousHash,
			"currentHash":      rec.CurrentHash,
			"canonicalVersion": rec.CanonicalVersion,
		} {
			if value == "" {
				t.Errorf("the stored record has no %s", name)
			}
		}
		if rec.ObjectID == 0 || rec.SequenceNo == 0 || rec.OccurredAt.IsZero() {
			t.Errorf("the stored record is missing an identifier or timestamp: %+v", rec)
		}
	})
}

// --- Canonical serialisation ---

// The same logical payload must hash identically regardless of map ordering,
// or two records describing the same thing would not be comparable.
func TestCanonicalSerialisationIsOrderIndependent(t *testing.T) {
	a, err := canonicalPayload(map[string]any{"z": 1, "a": 2, "m": 3})
	if err != nil {
		t.Fatal(err)
	}
	b, err := canonicalPayload(map[string]any{"a": 2, "m": 3, "z": 1})
	if err != nil {
		t.Fatal(err)
	}
	if a != b {
		t.Errorf("canonicalisation is order-dependent:\n  %s\n  %s", a, b)
	}
	if payloadHash(a) != payloadHash(b) {
		t.Error("two orderings of one payload produced different hashes")
	}
}

// Field boundaries must be unambiguous: no two different records may
// canonicalise to the same bytes.
func TestCanonicalRecordSeparatorsAreUnambiguous(t *testing.T) {
	base := func() *Record {
		return &Record{
			TenantID: "T", SequenceNo: 1, Action: "A", Actor: "B", ObjectType: "C",
			ObjectID: 1, CorrelationID: "D", OccurredAt: time.Unix(0, 0).UTC(),
			PayloadHash: "h", PreviousHash: GenesisHash,
		}
	}
	// Two records whose adjacent fields concatenate the same way under a naive
	// separator must still differ.
	x := base()
	x.Action, x.Actor = "AB", "C"
	y := base()
	y.Action, y.Actor = "A", "BC"

	if canonicalRecord(x) == canonicalRecord(y) {
		t.Error("two different records canonicalise identically; the field separator is ambiguous")
	}
	if recordHash(x) == recordHash(y) {
		t.Error("two different records hash identically")
	}
}

// --- Verification job and the anchoring gap ---

func TestVerifyAllRecordsItsResultInTheChain(t *testing.T) {
	eachBackend(t, func(t *testing.T, db *sql.DB, driver string) {
		l := newLedger(t, db, driver)
		appendN(t, l, "TENANT-A", 4)
		appendN(t, l, "TENANT-B", 2)

		results, err := l.VerifyAll(context.Background())
		if err != nil {
			t.Fatal(err)
		}
		if len(results) != 2 {
			t.Fatalf("verified %d tenants, want 2", len(results))
		}
		for _, r := range results {
			if !r.Intact {
				t.Errorf("%s: %s", r.TenantID, r.Reason)
			}
		}

		// The verification is itself a record, and the chain including it still
		// verifies.
		last, err := l.LastVerification(context.Background(), "TENANT-A")
		if err != nil {
			t.Fatal(err)
		}
		if last.RecordsChecked != 4 {
			t.Errorf("the recorded verification says %d records, want 4", last.RecordsChecked)
		}
		if !last.Intact {
			t.Error("the recorded verification reports a break")
		}

		after, _ := l.Verify(context.Background(), "TENANT-A")
		if !after.Intact {
			t.Errorf("appending the verification record broke the chain: %s", after.Reason)
		}
		if after.RecordsChecked != 5 {
			t.Errorf("the chain has %d records after verification, want 5", after.RecordsChecked)
		}
	})
}

// A failed verification must be recorded, not only logged. Recording only
// successes makes the trail's silence ambiguous between "not checked" and
// "checked and broken".
func TestFailedVerificationIsRecorded(t *testing.T) {
	eachBackend(t, func(t *testing.T, db *sql.DB, driver string) {
		l := newLedger(t, db, driver)
		appendN(t, l, "TENANT-A", 5)

		tamperDirectly(t, db,
			`UPDATE audit_events SET payload = '{"tampered":true}' WHERE tenant_id = 'TENANT-A' AND sequence_no = 2`)

		if _, err := l.VerifyAll(context.Background()); err != nil {
			t.Fatal(err)
		}

		last, err := l.LastVerification(context.Background(), "TENANT-A")
		if err != nil {
			t.Fatal(err)
		}
		if last.Intact {
			t.Error("a broken chain was recorded as intact")
		}
		if last.FirstBreakAt != 2 {
			t.Errorf("the record says the break is at %d, want 2", last.FirstBreakAt)
		}
	})
}

// The anchoring gap must be stated, never claimed away.
func TestCheckpointsAreNeverClaimedAsAnchored(t *testing.T) {
	eachBackend(t, func(t *testing.T, db *sql.DB, driver string) {
		l := newLedger(t, db, driver)
		appendN(t, l, "TENANT-A", 3)

		cp, err := l.TakeCheckpoint(context.Background(), "TENANT-A")
		if err != nil {
			t.Fatal(err)
		}
		if cp.Anchored {
			t.Error("a checkpoint claims to be externally anchored; no anchoring exists")
		}
		if cp.AnchorGap == "" {
			t.Error("a checkpoint carries no statement of what is missing")
		}
		if cp.SequenceNo != 3 || cp.HeadHash == "" {
			t.Errorf("the checkpoint does not describe the head: %+v", cp)
		}

		// The recorded verification carries the same qualification, so an
		// export cannot show a checkpoint without it.
		var payload string
		if err := db.QueryRow(`
			SELECT payload FROM audit_events
			WHERE tenant_id = 'TENANT-A' AND event_type = 'LEDGER_VERIFIED'
			ORDER BY sequence_no DESC LIMIT 1`).Scan(&payload); err == nil {
			if !strings.Contains(payload, "anchorGap") {
				t.Error("a recorded verification omits the anchoring qualification")
			}
		}
	})
}

// The vocabulary rule, asserted rather than trusted to review.
func TestNoMerkleOrSignatureClaimsInThisPackage(t *testing.T) {
	entries, err := os.ReadDir(".")
	if err != nil {
		t.Fatal(err)
	}
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".go") {
			continue
		}
		// This file is excluded because it contains the forbidden list itself.
		// A textual scan that includes its own patterns flags its own
		// documentation -- the same self-match the secret hygiene scan hit in
		// Prompt 05. The rule is about the implementation's vocabulary.
		if strings.HasSuffix(entry.Name(), "_test.go") {
			continue
		}
		body, err := os.ReadFile(entry.Name())
		if err != nil {
			t.Fatal(err)
		}
		text := string(body)

		// "Merkle" and "signature" appear only in comments explaining what this
		// is not. What must never appear is an identifier claiming otherwise.
		for _, forbidden := range []string{
			"MerkleRoot", "merkleRoot", "MerkleTree", "merkleTree",
			"Signature =", "signature =", "DigitalSignature", "SignedCheckpoint",
		} {
			if strings.Contains(text, forbidden) {
				t.Errorf("%s contains %q; this is an application hash chain and nothing here is signed",
					entry.Name(), forbidden)
			}
		}
	}
}

func TestUnknownDriverIsRefused(t *testing.T) {
	db, _ := sql.Open("sqlite", ":memory:")
	defer db.Close()
	if _, err := New(db, "mysql"); err == nil {
		t.Error("a ledger was built for a driver whose serialisation semantics are unknown")
	}
	if _, err := New(nil, "sqlite"); err == nil {
		t.Error("a ledger was built with no database")
	}
}

// The application must not be able to rewrite the ledger.
//
// The hash chain detects tampering; the append-only triggers prevent the
// ordinary case of it. Both matter, and this asserts the second -- the tamper
// tests above deliberately drop these triggers to model an attacker who already
// has database access, so without this test that removal would go unnoticed.
func TestAuditEventsAreAppendOnlyToTheApplication(t *testing.T) {
	// SQLite only: the triggers under test are defined in the SQLite
	// migrations. The PostgreSQL schema's equivalent is not yet written, which
	// is recorded in docs/engineering/EVIDENCE_LEDGER.md rather than asserted
	// here as if it existed.
	db := openSQLite(t)
	l := newLedger(t, db, "sqlite")
	appendN(t, l, "TENANT-A", 3)

	if _, err := db.Exec(`UPDATE audit_events SET actor = 'someone-else' WHERE sequence_no = 1`); err == nil {
		t.Error("an audit record was updated; the ledger is not append-only")
	}
	if _, err := db.Exec(`DELETE FROM audit_events WHERE sequence_no = 1`); err == nil {
		t.Error("an audit record was deleted; the ledger is not append-only")
	}

	// And appending still works, so the trigger is not simply refusing
	// everything.
	if _, err := l.Append(context.Background(), AppendRequest{
		TenantID: "TENANT-A", Action: "X", Actor: "system:test", ObjectType: "artifact",
	}); err != nil {
		t.Errorf("the append-only trigger also blocks appends: %v", err)
	}
}
