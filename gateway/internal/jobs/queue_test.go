package jobs

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

// Every property is checked against both databases.
//
// PostgreSQL is the stated target and is where SKIP LOCKED lives; SQLite is what
// runs today. The claim strategies genuinely differ, so a suite that ran against
// only one would be verifying half the code. The PostgreSQL half is skipped
// without SENTINEL_TEST_POSTGRES_DSN, and the skip says so rather than passing
// quietly.

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
		t.Log("SKIPPED for postgres: SENTINEL_TEST_POSTGRES_DSN is unset, so SKIP LOCKED claiming is NOT verified by this run")
	}
	return out
}

// openSQLite applies the real migrations rather than a hand-written schema, so
// the CHECK constraints and triggers under test are the shipped ones.
func openSQLite(t *testing.T) *sql.DB {
	t.Helper()
	// A file-backed database, not :memory:. In-memory SQLite gives each
	// connection its own database unless shared, and a pool of workers on
	// separate connections would each see an empty queue -- which would make
	// every concurrency test pass for the wrong reason.
	path := filepath.Join(t.TempDir(), "jobs.db")
	db, err := sql.Open("sqlite", path+"?_pragma=busy_timeout(5000)&_pragma=journal_mode(WAL)")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })

	for _, name := range []string{
		"001_init_schema.sql", "002_tenancy_and_state.sql", "003_secret_store.sql",
		"004_artifact_storage.sql", "005_redacted_findings.sql", "006_jobs_and_outbox.sql",
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

// openPostgres uses a schema built for these tests.
//
// The PostgreSQL production schema does not yet carry the job tables -- the
// application still runs on SQLite -- so this creates the shape under test
// rather than pretending a port that has not happened.
func openPostgres(t *testing.T) *sql.DB {
	t.Helper()
	db, err := sql.Open("pgx", os.Getenv("SENTINEL_TEST_POSTGRES_DSN"))
	if err != nil {
		t.Fatal(err)
	}
	if err := db.Ping(); err != nil {
		t.Fatalf("ping postgres: %v", err)
	}
	t.Cleanup(func() { db.Close() })

	// Each test gets its own schema, so parallel runs and repeated runs do not
	// see each other's jobs.
	schema := fmt.Sprintf("jobs_test_%d", time.Now().UnixNano())
	if _, err := db.Exec(`CREATE SCHEMA ` + schema); err != nil {
		t.Fatalf("create schema: %v", err)
	}
	t.Cleanup(func() { _, _ = db.Exec(`DROP SCHEMA ` + schema + ` CASCADE`) })
	if _, err := db.Exec(`SET search_path TO ` + schema); err != nil {
		t.Fatal(err)
	}
	// The pool must use the schema on every connection, not only the one that
	// created it.
	db.SetMaxIdleConns(0)
	db.Close()

	db, err = sql.Open("pgx", os.Getenv("SENTINEL_TEST_POSTGRES_DSN")+"&search_path="+schema)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })

	if _, err := db.Exec(postgresJobSchema); err != nil {
		t.Fatalf("apply job schema: %v", err)
	}
	seedTenants(t, db)
	return db
}

const postgresJobSchema = `
CREATE TABLE tenants (id TEXT PRIMARY KEY, name TEXT NOT NULL);
CREATE TABLE ingestion_jobs (
    id               BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    tenant_id        TEXT NOT NULL REFERENCES tenants(id),
    file_instance_id BIGINT,
    idempotency_key  TEXT NOT NULL,
    kind             TEXT NOT NULL DEFAULT 'VALIDATE_ARTIFACT',
    state            TEXT NOT NULL CHECK (state IN ('QUEUED','LEASED','RUNNING','SUCCEEDED','RETRYABLE','DEAD','CANCELLED')),
    attempt_count    INTEGER NOT NULL DEFAULT 0 CHECK (attempt_count >= 0),
    max_attempts     INTEGER NOT NULL DEFAULT 5 CHECK (max_attempts >= 1),
    lease_owner      TEXT,
    lease_expires_at TIMESTAMPTZ,
    last_heartbeat_at TIMESTAMPTZ,
    run_after        TIMESTAMPTZ,
    last_error       TEXT,
    terminal_reason  TEXT,
    row_version      BIGINT NOT NULL DEFAULT 0,
    created_at       TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at       TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE (tenant_id, idempotency_key)
);
CREATE INDEX idx_jobs_runnable ON ingestion_jobs(state, run_after, tenant_id);
CREATE TABLE job_attempts (
    id          BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    tenant_id   TEXT NOT NULL,
    job_id      BIGINT NOT NULL,
    attempt_no  INTEGER NOT NULL CHECK (attempt_no >= 1),
    outcome     TEXT NOT NULL,
    error       TEXT,
    started_at  TIMESTAMPTZ NOT NULL,
    finished_at TIMESTAMPTZ,
    UNIQUE (tenant_id, job_id, attempt_no)
);
CREATE TABLE tenant_job_quotas (
    tenant_id      TEXT PRIMARY KEY REFERENCES tenants(id),
    max_concurrent INTEGER NOT NULL CHECK (max_concurrent > 0),
    updated_at     TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE TABLE outbox_events (
    id            BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    tenant_id     TEXT NOT NULL REFERENCES tenants(id),
    event_type    TEXT NOT NULL,
    subject_type  TEXT NOT NULL,
    subject_id    BIGINT NOT NULL,
    payload       TEXT NOT NULL,
    dedupe_key    TEXT NOT NULL,
    attempt_count INTEGER NOT NULL DEFAULT 0,
    max_attempts  INTEGER NOT NULL DEFAULT 10,
    run_after     TIMESTAMPTZ,
    last_error    TEXT,
    delivered_at  TIMESTAMPTZ,
    dead_at       TIMESTAMPTZ,
    created_at    TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE (tenant_id, dedupe_key)
);
`

func seedTenants(t *testing.T, db *sql.DB) {
	t.Helper()
	for _, id := range []string{"TENANT-A", "TENANT-B", "TENANT-DEFAULT"} {
		if _, err := db.Exec(`INSERT INTO tenants (id, name) VALUES ($1, $2)`, id, id); err != nil {
			if _, err2 := db.Exec(`INSERT INTO tenants (id, name) VALUES (?, ?)`, id, id); err2 != nil {
				// Already present from a migration fixture is fine.
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

func newQueue(t *testing.T, db *sql.DB, driver string, opts ...Option) *Queue {
	t.Helper()
	q, err := New(db, driver, opts...)
	if err != nil {
		t.Fatal(err)
	}
	return q
}

func enqueue(t *testing.T, q *Queue, tenant, key string) int64 {
	t.Helper()
	id, err := q.Enqueue(context.Background(), EnqueueRequest{
		TenantID: tenant, IdempotencyKey: key, MaxAttempts: 3,
	})
	if err != nil {
		t.Fatal(err)
	}
	return id
}

// --- The six required concurrency properties ---

// 50 workers race for one job; exactly one owns a valid lease.
//
// This is the property everything else rests on. A job claimed twice is a job
// whose side effect happens twice.
func TestFiftyWorkersRaceForOneJobAndExactlyOneWins(t *testing.T) {
	eachBackend(t, func(t *testing.T, db *sql.DB, driver string) {
		db.SetMaxOpenConns(60)
		q := newQueue(t, db, driver)
		jobID := enqueue(t, q, "TENANT-A", "race-key")

		const racers = 50
		var wins atomic.Int64
		var winner atomic.Value
		start := make(chan struct{})
		var wg sync.WaitGroup

		for i := 0; i < racers; i++ {
			wg.Add(1)
			go func(i int) {
				defer wg.Done()
				<-start // release them together
				workerID := fmt.Sprintf("racer-%d", i)
				job, err := q.Claim(context.Background(), workerID, nil, 100)
				if err == nil && job != nil {
					wins.Add(1)
					winner.Store(workerID)
				}
			}(i)
		}
		close(start)
		wg.Wait()

		if got := wins.Load(); got != 1 {
			t.Fatalf("%d workers claimed the same job; exactly one must", got)
		}

		// And the database agrees on who owns it.
		job, err := q.Get(context.Background(), jobID)
		if err != nil {
			t.Fatal(err)
		}
		if job.State != StateLeased {
			t.Errorf("job state = %s, want LEASED", job.State)
		}
		if !job.LeaseOwner.Valid || job.LeaseOwner.String != winner.Load().(string) {
			t.Errorf("lease owner is %v, want %v", job.LeaseOwner, winner.Load())
		}
		if job.AttemptCount != 1 {
			t.Errorf("attempt count = %d after one claim; a double claim would show here", job.AttemptCount)
		}
	})
}

// A process that commits business state and dies before publishing must not
// lose the event. The outbox is written in the same transaction, so the
// dispatcher delivers it on the next pass -- once.
func TestOutboxSurvivesACrashBetweenCommitAndPublish(t *testing.T) {
	eachBackend(t, func(t *testing.T, db *sql.DB, driver string) {
		q := newQueue(t, db, driver)
		ctx := context.Background()

		// The "process" commits its business state and its event together.
		tx, err := db.Begin()
		if err != nil {
			t.Fatal(err)
		}
		if err := q.PublishTx(ctx, tx, OutboxEvent{
			TenantID: "TENANT-A", EventType: "ARTIFACT_VALIDATED",
			SubjectType: "artifact", SubjectID: 42,
			DedupeKey: "artifact-42-validated",
			Payload:   map[string]any{"artifactId": 42},
		}); err != nil {
			t.Fatal(err)
		}
		if err := tx.Commit(); err != nil {
			t.Fatal(err)
		}
		// ...and then dies. Nothing published.

		var delivered []string
		dispatcher := NewDispatcher(q, DelivererFunc(func(_ context.Context, ev PendingEvent) error {
			delivered = append(delivered, ev.DedupeKey)
			return nil
		}), time.Millisecond, 10)

		n, failed, err := dispatcher.DispatchOnce(ctx)
		if err != nil || failed != 0 {
			t.Fatalf("dispatch: n=%d failed=%d err=%v", n, failed, err)
		}
		if n != 1 || len(delivered) != 1 {
			t.Fatalf("the event was not delivered after the crash: n=%d delivered=%v", n, delivered)
		}

		// A second pass must not deliver it again.
		n2, _, err := dispatcher.DispatchOnce(ctx)
		if err != nil {
			t.Fatal(err)
		}
		if n2 != 0 || len(delivered) != 1 {
			t.Errorf("the event was delivered twice: %v", delivered)
		}
	})
}

// A worker whose lease expired mid-work must not overwrite the completion its
// replacement wrote.
func TestStaleWorkerCannotOverwriteNewerCompletion(t *testing.T) {
	eachBackend(t, func(t *testing.T, db *sql.DB, driver string) {
		clock := time.Now().UTC()
		q := newQueue(t, db, driver,
			WithLeaseDuration(30*time.Second),
			WithClock(func() time.Time { return clock }))
		ctx := context.Background()
		jobID := enqueue(t, q, "TENANT-A", "stale-key")

		// The first worker claims it and then stalls.
		stale, err := q.Claim(ctx, "worker-stale", nil, 100)
		if err != nil {
			t.Fatal(err)
		}

		// Its lease expires.
		clock = clock.Add(31 * time.Second)

		// A second worker takes over.
		fresh, err := q.Claim(ctx, "worker-fresh", nil, 100)
		if err != nil {
			t.Fatalf("the expired lease was not reclaimable: %v", err)
		}
		if fresh.ID != jobID {
			t.Fatalf("claimed job %d, want %d", fresh.ID, jobID)
		}

		// The replacement finishes.
		tx, _ := db.Begin()
		if err := q.CompleteTx(ctx, tx, fresh, "worker-fresh"); err != nil {
			t.Fatal(err)
		}
		if err := tx.Commit(); err != nil {
			t.Fatal(err)
		}

		// Now the stale worker wakes and tries to finish. It must be refused.
		staleTx, _ := db.Begin()
		err = q.CompleteTx(ctx, staleTx, stale, "worker-stale")
		_ = staleTx.Rollback()
		if !errors.Is(err, ErrLeaseLost) {
			t.Fatalf("the stale worker's completion returned %v, want ErrLeaseLost", err)
		}

		// A stale failure must be refused too, or a succeeded job would be
		// dragged back to RETRYABLE and run a second time.
		if err := q.Fail(ctx, stale, "worker-stale", errors.New("stale failure")); !errors.Is(err, ErrLeaseLost) {
			t.Errorf("the stale worker's failure returned %v, want ErrLeaseLost", err)
		}

		job, err := q.Get(ctx, jobID)
		if err != nil {
			t.Fatal(err)
		}
		if job.State != StateSucceeded {
			t.Errorf("job state = %s after a stale worker interfered, want SUCCEEDED", job.State)
		}
	})
}

// A heartbeat from a superseded worker must fail, so the worker learns to stop.
func TestHeartbeatFailsOnceTheLeaseIsLost(t *testing.T) {
	eachBackend(t, func(t *testing.T, db *sql.DB, driver string) {
		clock := time.Now().UTC()
		q := newQueue(t, db, driver,
			WithLeaseDuration(30*time.Second),
			WithClock(func() time.Time { return clock }))
		ctx := context.Background()
		enqueue(t, q, "TENANT-A", "hb-key")

		stale, err := q.Claim(ctx, "worker-stale", nil, 100)
		if err != nil {
			t.Fatal(err)
		}
		// A heartbeat while still owned must succeed and extend the lease.
		if err := q.Heartbeat(ctx, stale, "worker-stale"); err != nil {
			t.Fatalf("a live worker's heartbeat failed: %v", err)
		}

		clock = clock.Add(31 * time.Second)
		if _, err := q.Claim(ctx, "worker-fresh", nil, 100); err != nil {
			t.Fatal(err)
		}

		if err := q.Heartbeat(ctx, stale, "worker-stale"); !errors.Is(err, ErrLeaseLost) {
			t.Errorf("a superseded worker's heartbeat returned %v, want ErrLeaseLost", err)
		}
	})
}

// Duplicate arrival is a normal condition: a partner retries, a watcher sees the
// same file twice. It must produce one job, not two.
func TestDuplicateArrivalProducesOneJob(t *testing.T) {
	eachBackend(t, func(t *testing.T, db *sql.DB, driver string) {
		q := newQueue(t, db, driver)
		ctx := context.Background()

		first := enqueue(t, q, "TENANT-A", "same-key")
		second := enqueue(t, q, "TENANT-A", "same-key")
		if first != second {
			t.Errorf("a duplicate enqueue produced jobs %d and %d", first, second)
		}

		var n int
		if err := db.QueryRow(`SELECT COUNT(*) FROM ingestion_jobs`).Scan(&n); err != nil {
			t.Fatal(err)
		}
		if n != 1 {
			t.Errorf("%d jobs exist after a duplicate enqueue", n)
		}

		// The same key in a different tenant is different work.
		other, err := q.Enqueue(ctx, EnqueueRequest{TenantID: "TENANT-B", IdempotencyKey: "same-key"})
		if err != nil {
			t.Fatal(err)
		}
		if other == first {
			t.Error("two tenants' jobs collapsed into one")
		}
	})
}

// A duplicate outbox write collapses to one row, so a handler that runs twice
// produces one event.
func TestDuplicateOutboxWriteIsHarmless(t *testing.T) {
	eachBackend(t, func(t *testing.T, db *sql.DB, driver string) {
		q := newQueue(t, db, driver)
		ctx := context.Background()

		for i := 0; i < 3; i++ {
			tx, _ := db.Begin()
			if err := q.PublishTx(ctx, tx, OutboxEvent{
				TenantID: "TENANT-A", EventType: "ARTIFACT_VALIDATED",
				SubjectType: "artifact", SubjectID: 7, DedupeKey: "artifact-7-validated",
				Payload: map[string]any{"attempt": i},
			}); err != nil {
				t.Fatal(err)
			}
			if err := tx.Commit(); err != nil {
				t.Fatal(err)
			}
		}

		var n int
		if err := db.QueryRow(`SELECT COUNT(*) FROM outbox_events`).Scan(&n); err != nil {
			t.Fatal(err)
		}
		if n != 1 {
			t.Errorf("%d outbox rows after three identical publishes, want 1", n)
		}
	})
}

// A poison job must reach DEAD without blocking the queue behind it.
func TestPoisonJobReachesDeadWithoutBlockingTheQueue(t *testing.T) {
	eachBackend(t, func(t *testing.T, db *sql.DB, driver string) {
		clock := time.Now().UTC()
		q := newQueue(t, db, driver, WithClock(func() time.Time { return clock }))
		ctx := context.Background()

		poison := enqueue(t, q, "TENANT-A", "poison")
		healthy := enqueue(t, q, "TENANT-A", "healthy")

		// Fail the poison job until its budget is exhausted. MaxAttempts is 3.
		for attempt := 1; attempt <= 3; attempt++ {
			job, err := q.Claim(ctx, "worker-1", nil, 100)
			if err != nil {
				t.Fatalf("attempt %d: %v", attempt, err)
			}
			if job.ID != poison {
				t.Fatalf("attempt %d claimed job %d, want the poison job %d", attempt, job.ID, poison)
			}
			if err := q.Fail(ctx, job, "worker-1", fmt.Errorf("poison attempt %d", attempt)); err != nil {
				t.Fatal(err)
			}
			// Move past the backoff so the next claim sees it.
			clock = clock.Add(maxBackoff + time.Second)
		}

		dead, err := q.Get(ctx, poison)
		if err != nil {
			t.Fatal(err)
		}
		if dead.State != StateDead {
			t.Errorf("the poison job is %s after exhausting its budget, want DEAD", dead.State)
		}

		// The healthy job behind it is still claimable.
		next, err := q.Claim(ctx, "worker-2", nil, 100)
		if err != nil {
			t.Fatalf("the queue is blocked behind the poison job: %v", err)
		}
		if next.ID != healthy {
			t.Errorf("claimed job %d, want the healthy job %d", next.ID, healthy)
		}

		// And every attempt was recorded.
		var attempts int
		if err := db.QueryRow(`SELECT COUNT(*) FROM job_attempts WHERE job_id = $1`, poison).Scan(&attempts); err != nil {
			if err2 := db.QueryRow(`SELECT COUNT(*) FROM job_attempts WHERE job_id = ?`, poison).Scan(&attempts); err2 != nil {
				t.Fatal(err2)
			}
		}
		if attempts != 3 {
			t.Errorf("%d attempt records for a job tried 3 times", attempts)
		}
	})
}

// One tenant must not consume every worker.
func TestOneTenantCannotConsumeAllWorkers(t *testing.T) {
	eachBackend(t, func(t *testing.T, db *sql.DB, driver string) {
		db.SetMaxOpenConns(20)
		q := newQueue(t, db, driver, WithLeaseDuration(30*time.Second))
		ctx := context.Background()

		// A backlog from one tenant, and one job from another behind it.
		for i := 0; i < 20; i++ {
			enqueue(t, q, "TENANT-A", fmt.Sprintf("noisy-%d", i))
		}
		quiet := enqueue(t, q, "TENANT-B", "quiet-1")

		cfg := DefaultPoolConfig()
		cfg.Workers = 4
		cfg.MaxPerTenant = 2
		cfg.PollInterval = 5 * time.Millisecond
		cfg.HeartbeatInterval = time.Second

		pool, err := NewPool(q, cfg)
		if err != nil {
			t.Fatal(err)
		}

		// Handlers block until released, so the quota is observable while jobs
		// are held rather than inferred after they complete.
		release := make(chan struct{})
		var concurrentA atomic.Int64
		var maxA atomic.Int64
		quietRan := make(chan struct{})
		var quietOnce sync.Once

		pool.Register("VALIDATE_ARTIFACT", HandlerFunc(func(ctx context.Context, tx *sql.Tx, job *Job) error {
			if job.TenantID == "TENANT-A" {
				n := concurrentA.Add(1)
				for {
					old := maxA.Load()
					if n <= old || maxA.CompareAndSwap(old, n) {
						break
					}
				}
				defer concurrentA.Add(-1)
			}
			if job.ID == quiet {
				quietOnce.Do(func() { close(quietRan) })
				return nil
			}
			select {
			case <-release:
			case <-ctx.Done():
			}
			return nil
		}))

		runCtx, cancel := context.WithCancel(ctx)
		defer cancel()
		pool.Start(runCtx)

		// The quiet tenant's single job must run despite the backlog. Without a
		// per-tenant quota, all four workers would be inside TENANT-A handlers
		// and this would time out.
		select {
		case <-quietRan:
		case <-time.After(10 * time.Second):
			t.Errorf("the quiet tenant's job never ran; one tenant consumed every worker (max concurrent for TENANT-A: %d)", maxA.Load())
		}

		close(release)
		cancel()
		_ = pool.Stop(5 * time.Second)

		if got := maxA.Load(); got > int64(cfg.MaxPerTenant) {
			t.Errorf("TENANT-A held %d workers concurrently, limit is %d", got, cfg.MaxPerTenant)
		}
		t.Logf("observed: TENANT-A held at most %d workers concurrently (limit %d, pool %d)",
			maxA.Load(), cfg.MaxPerTenant, cfg.Workers)
	})
}

// --- Supporting properties ---

// A handler's business state and its job completion commit together, so a
// rollback discards both.
func TestHandlerWorkAndCompletionCommitTogether(t *testing.T) {
	eachBackend(t, func(t *testing.T, db *sql.DB, driver string) {
		q := newQueue(t, db, driver)
		ctx := context.Background()
		jobID := enqueue(t, q, "TENANT-A", "atomic-key")

		cfg := DefaultPoolConfig()
		cfg.Workers = 1
		cfg.PollInterval = 5 * time.Millisecond
		cfg.HeartbeatInterval = time.Second
		pool, err := NewPool(q, cfg)
		if err != nil {
			t.Fatal(err)
		}

		// The handler writes an outbox event and then fails. Neither the event
		// nor the completion may survive.
		pool.Register("VALIDATE_ARTIFACT", HandlerFunc(func(ctx context.Context, tx *sql.Tx, job *Job) error {
			if err := q.PublishTx(ctx, tx, OutboxEvent{
				TenantID: job.TenantID, EventType: "SHOULD_NOT_SURVIVE",
				SubjectType: "artifact", SubjectID: 1, DedupeKey: "rolled-back",
				Payload: map[string]any{},
			}); err != nil {
				return err
			}
			return errors.New("handler failed after writing its event")
		}))

		runCtx, cancel := context.WithCancel(ctx)
		pool.Start(runCtx)

		deadline := time.After(10 * time.Second)
		for {
			job, err := q.Get(ctx, jobID)
			if err != nil {
				t.Fatal(err)
			}
			if job.State == StateRetryable || job.State == StateDead {
				break
			}
			select {
			case <-deadline:
				t.Fatalf("the job never left %s", job.State)
			case <-time.After(10 * time.Millisecond):
			}
		}
		cancel()
		_ = pool.Stop(5 * time.Second)

		var events int
		if err := db.QueryRow(`SELECT COUNT(*) FROM outbox_events WHERE dedupe_key = 'rolled-back'`).Scan(&events); err != nil {
			t.Fatal(err)
		}
		if events != 0 {
			t.Errorf("%d outbox events survived a rolled-back handler", events)
		}
	})
}

// Backpressure returns a typed overload rather than accepting work nobody will
// run.
func TestBackpressureRefusesRatherThanQueuing(t *testing.T) {
	eachBackend(t, func(t *testing.T, db *sql.DB, driver string) {
		q := newQueue(t, db, driver)
		ctx := context.Background()

		cfg := DefaultPoolConfig()
		cfg.MaxQueueDepth = 3
		cfg.HeartbeatInterval = time.Second
		pool, err := NewPool(q, cfg)
		if err != nil {
			t.Fatal(err)
		}

		for i := 0; i < 3; i++ {
			tx, _ := db.Begin()
			if _, err := pool.EnqueueTx(ctx, tx, EnqueueRequest{
				TenantID: "TENANT-A", IdempotencyKey: fmt.Sprintf("bp-%d", i),
			}); err != nil {
				t.Fatalf("enqueue %d: %v", i, err)
			}
			if err := tx.Commit(); err != nil {
				t.Fatal(err)
			}
		}

		tx, _ := db.Begin()
		_, err = pool.EnqueueTx(ctx, tx, EnqueueRequest{TenantID: "TENANT-A", IdempotencyKey: "bp-overflow"})
		_ = tx.Rollback()
		if !errors.Is(err, ErrOverloaded) {
			t.Fatalf("got %v, want ErrOverloaded", err)
		}
		if !strings.Contains(err.Error(), "limit 3") {
			t.Errorf("the overload error does not say what the limit was: %v", err)
		}
	})
}

// A graceful stop finishes held work rather than abandoning it.
func TestGracefulShutdownFinishesHeldWork(t *testing.T) {
	eachBackend(t, func(t *testing.T, db *sql.DB, driver string) {
		q := newQueue(t, db, driver, WithLeaseDuration(30*time.Second))
		ctx := context.Background()
		jobID := enqueue(t, q, "TENANT-A", "graceful")

		cfg := DefaultPoolConfig()
		cfg.Workers = 2
		cfg.PollInterval = 5 * time.Millisecond
		cfg.HeartbeatInterval = time.Second

		pool, err := NewPool(q, cfg)
		if err != nil {
			t.Fatal(err)
		}

		started := make(chan struct{})
		var once sync.Once
		pool.Register("VALIDATE_ARTIFACT", HandlerFunc(func(ctx context.Context, tx *sql.Tx, job *Job) error {
			once.Do(func() { close(started) })
			// Long enough that Stop must actually wait for it.
			time.Sleep(300 * time.Millisecond)
			return nil
		}))

		pool.Start(ctx)
		select {
		case <-started:
		case <-time.After(10 * time.Second):
			t.Fatal("no handler started")
		}

		if err := pool.Stop(10 * time.Second); err != nil {
			t.Fatalf("graceful stop: %v", err)
		}

		job, err := q.Get(ctx, jobID)
		if err != nil {
			t.Fatal(err)
		}
		if job.State != StateSucceeded {
			t.Errorf("job state = %s after a graceful stop, want SUCCEEDED", job.State)
		}
		if job.LeaseOwner.Valid {
			t.Error("the lease was not released")
		}
	})
}

// A failed delivery is retried and eventually parked, never dropped.
func TestUndeliverableEventIsParkedNotDropped(t *testing.T) {
	eachBackend(t, func(t *testing.T, db *sql.DB, driver string) {
		clock := time.Now().UTC()
		q := newQueue(t, db, driver, WithClock(func() time.Time { return clock }))
		ctx := context.Background()

		tx, _ := db.Begin()
		if err := q.PublishTx(ctx, tx, OutboxEvent{
			TenantID: "TENANT-A", EventType: "UNDELIVERABLE",
			SubjectType: "artifact", SubjectID: 1, DedupeKey: "undeliverable-1",
			Payload: map[string]any{},
		}); err != nil {
			t.Fatal(err)
		}
		if err := tx.Commit(); err != nil {
			t.Fatal(err)
		}

		// max_attempts defaults to 10.
		if _, err := db.Exec(`UPDATE outbox_events SET max_attempts = 2 WHERE dedupe_key = 'undeliverable-1'`); err != nil {
			t.Fatal(err)
		}

		dispatcher := NewDispatcher(q, DelivererFunc(func(context.Context, PendingEvent) error {
			return errors.New("consumer is down")
		}), time.Millisecond, 10)

		for i := 0; i < 2; i++ {
			if _, failed, err := dispatcher.DispatchOnce(ctx); err != nil || failed != 1 {
				t.Fatalf("pass %d: failed=%d err=%v", i, failed, err)
			}
			clock = clock.Add(maxBackoff + time.Second)
		}

		dead, err := dispatcher.DeadCount(ctx)
		if err != nil {
			t.Fatal(err)
		}
		if dead != 1 {
			t.Errorf("%d dead events, want 1", dead)
		}

		// The row still exists: a record of what happened is not deleted
		// because it could not be delivered.
		var n int
		if err := db.QueryRow(`SELECT COUNT(*) FROM outbox_events WHERE dedupe_key = 'undeliverable-1'`).Scan(&n); err != nil {
			t.Fatal(err)
		}
		if n != 1 {
			t.Error("an undeliverable event was deleted rather than parked")
		}
	})
}

// Retry backoff must be jittered, or a batch that failed together retries
// together and reproduces the load that caused the failure.
func TestBackoffIsJitteredAndBounded(t *testing.T) {
	seen := map[time.Duration]bool{}
	for i := 0; i < 200; i++ {
		d := backoff(3, time.Second)
		if d < 0 || d > 4*time.Second {
			t.Fatalf("attempt 3 backoff %s is outside [0, 4s]", d)
		}
		seen[d] = true
	}
	if len(seen) < 50 {
		t.Errorf("200 samples produced only %d distinct delays; the backoff is not jittered", len(seen))
	}

	// And it is capped rather than growing without bound.
	for attempt := 1; attempt <= 40; attempt++ {
		if d := backoff(attempt, time.Second); d > maxBackoff {
			t.Errorf("attempt %d produced %s, above the %s cap", attempt, d, maxBackoff)
		}
	}
}

// A queue built for an unknown driver must be refused. Guessing the claim
// strategy produces a queue that appears to work and races under load.
func TestUnknownDriverIsRefused(t *testing.T) {
	db, _ := sql.Open("sqlite", ":memory:")
	defer db.Close()

	if _, err := New(db, "mysql"); err == nil {
		t.Error("a queue was built for a driver whose locking semantics are unknown")
	}
	if _, err := New(nil, "sqlite"); err == nil {
		t.Error("a queue was built with no database")
	}
}

// A pool whose heartbeat is slower than its lease would lose leases while
// healthy. That is a configuration error and must be refused at construction.
func TestPoolRefusesAHeartbeatSlowerThanItsLease(t *testing.T) {
	db, _ := sql.Open("sqlite", ":memory:")
	defer db.Close()
	q, err := New(db, "sqlite", WithLeaseDuration(5*time.Second))
	if err != nil {
		t.Fatal(err)
	}

	cfg := DefaultPoolConfig()
	cfg.HeartbeatInterval = 10 * time.Second
	if _, err := NewPool(q, cfg); err == nil {
		t.Error("a pool was built whose heartbeat is slower than its lease")
	}

	for _, bad := range []PoolConfig{
		{Workers: 0, MaxPerTenant: 1, HeartbeatInterval: time.Second},
		{Workers: 1, MaxPerTenant: 0, HeartbeatInterval: time.Second},
	} {
		if _, err := NewPool(q, bad); err == nil {
			t.Errorf("an unbounded pool config was accepted: %+v", bad)
		}
	}
}
