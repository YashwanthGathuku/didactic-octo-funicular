package main

import (
	"bytes"
	"context"
	"database/sql"
	"fmt"
	"net/http"
	"strings"
	"sync"
	"testing"
	"time"

	"sentinel-gateway/internal/jobs"
	"sentinel-gateway/internal/objectstore"
)

// End-to-end: an upload is validated by a worker, not by the request handler.
//
// Before Prompt 08 the queue had no consumer, so this whole path did not exist:
// ingest wrote a job row and the artifact stayed RECEIVED forever.

func newWorkerTestEnv(t *testing.T) (*sql.DB, http.Handler, *jobs.Queue, objectstore.ObjectStore) {
	t.Helper()
	db, handler, store := newIngestTestEnv(t)

	queue, err := jobs.New(db, "sqlite", jobs.WithLeaseDuration(30*time.Second))
	if err != nil {
		t.Fatal(err)
	}
	return db, handler, queue, store
}

func runPoolUntil(t *testing.T, queue *jobs.Queue, store objectstore.ObjectStore, done func() bool) *jobs.Pool {
	t.Helper()

	cfg := jobs.DefaultPoolConfig()
	cfg.Workers = 2
	cfg.PollInterval = 5 * time.Millisecond
	cfg.HeartbeatInterval = time.Second

	pool, err := jobs.NewPool(queue, cfg)
	if err != nil {
		t.Fatal(err)
	}
	pool.Register(KindValidateArtifact, &validateArtifactHandler{store: store, queue: queue})

	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(func() {
		cancel()
		_ = pool.Stop(5 * time.Second)
	})
	pool.Start(ctx)

	deadline := time.After(15 * time.Second)
	for !done() {
		select {
		case <-deadline:
			t.Fatalf("the work did not complete. pool stats: %+v", pool.Stats())
		case <-time.After(10 * time.Millisecond):
		}
	}
	return pool
}

func artifactStatus(t *testing.T, db *sql.DB, id int64) string {
	t.Helper()
	var status string
	if err := db.QueryRow(`SELECT status FROM file_instances WHERE id = ?`, id).Scan(&status); err != nil {
		t.Fatal(err)
	}
	return status
}

// A valid upload reaches VALIDATED through the queue.
func TestUploadedArtifactIsValidatedByAWorker(t *testing.T) {
	db, handler, queue, store := newWorkerTestEnv(t)

	scenario := GenerateNachaScenario(PresetBalancedPayroll)
	rec, resp := doUpload(t, handler, uploadRequest(t, scenario.Filename, []byte(scenario.Content), nil))
	if rec.Code != http.StatusAccepted {
		t.Fatalf("upload: %d %s", rec.Code, rec.Body.String())
	}
	if got := artifactStatus(t, db, resp.ArtifactID); got != "RECEIVED" {
		t.Fatalf("the artifact is %s before any worker ran; ingest must not validate", got)
	}

	runPoolUntil(t, queue, store, func() bool {
		return artifactStatus(t, db, resp.ArtifactID) != "RECEIVED"
	})

	if got := artifactStatus(t, db, resp.ArtifactID); got != "VALIDATED" {
		t.Errorf("artifact status = %s, want VALIDATED", got)
	}

	job, err := queue.Get(context.Background(), resp.JobID)
	if err != nil {
		t.Fatal(err)
	}
	if job.State != jobs.StateSucceeded {
		t.Errorf("job state = %s, want SUCCEEDED", job.State)
	}

	// And the outbox carries the event, written in the handler's transaction.
	var events int
	if err := db.QueryRow(`SELECT COUNT(*) FROM outbox_events WHERE subject_id = ?`, resp.ArtifactID).Scan(&events); err != nil {
		t.Fatal(err)
	}
	if events != 1 {
		t.Errorf("%d outbox events for one validation, want 1", events)
	}
}

// An invalid upload reaches QUARANTINED with findings, and never RELEASED.
func TestInvalidUploadIsQuarantinedByAWorker(t *testing.T) {
	db, handler, queue, store := newWorkerTestEnv(t)

	scenario := GenerateNachaScenario(PresetCorruptedEntryHash)
	_, resp := doUpload(t, handler, uploadRequest(t, scenario.Filename, []byte(scenario.Content), nil))

	runPoolUntil(t, queue, store, func() bool {
		return artifactStatus(t, db, resp.ArtifactID) != "RECEIVED"
	})

	status := artifactStatus(t, db, resp.ArtifactID)
	if status != "QUARANTINED" {
		t.Errorf("artifact status = %s, want QUARANTINED", status)
	}

	var findings int
	if err := db.QueryRow(`
		SELECT COUNT(*) FROM validation_findings
		WHERE file_instance_id = ? AND severity = 'BLOCKING'`, resp.ArtifactID).Scan(&findings); err != nil {
		t.Fatal(err)
	}
	if findings == 0 {
		t.Error("quarantined with no blocking finding recorded")
	}

	// The history row explains it without raw content.
	var reason string
	if err := db.QueryRow(`
		SELECT reason FROM status_history
		WHERE object_type = 'artifact' AND object_id = ? ORDER BY id DESC LIMIT 1`,
		resp.ArtifactID).Scan(&reason); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(reason, "QUARANTINED") {
		t.Errorf("the history reason does not state the outcome: %q", reason)
	}
	for _, record := range strings.Split(strings.TrimRight(scenario.Content, "\n"), "\n") {
		if len(record) >= 20 && strings.Contains(reason, record) {
			t.Error("the history reason contains a raw record")
		}
	}
}

// The handler is idempotent: running it twice produces one set of findings and
// one outcome. At-least-once delivery makes this a requirement, not a nicety.
func TestValidationHandlerIsIdempotent(t *testing.T) {
	db, handler, queue, store := newWorkerTestEnv(t)

	scenario := GenerateNachaScenario(PresetCorruptedEntryHash)
	_, resp := doUpload(t, handler, uploadRequest(t, scenario.Filename, []byte(scenario.Content), nil))

	h := &validateArtifactHandler{store: store, queue: queue}
	job := &jobs.Job{
		ID: resp.JobID, TenantID: DefaultTenantID, Kind: KindValidateArtifact,
		ArtifactID: sql.NullInt64{Int64: resp.ArtifactID, Valid: true},
	}

	countFindings := func() int {
		var n int
		if err := db.QueryRow(`SELECT COUNT(*) FROM validation_findings WHERE file_instance_id = ?`,
			resp.ArtifactID).Scan(&n); err != nil {
			t.Fatal(err)
		}
		return n
	}

	// First run.
	tx, _ := db.Begin()
	if err := h.Handle(context.Background(), tx, job); err != nil {
		t.Fatal(err)
	}
	if err := tx.Commit(); err != nil {
		t.Fatal(err)
	}
	first := countFindings()
	if first == 0 {
		t.Fatal("no findings after the first run; this test would pass vacuously")
	}

	// Second run, as a retry would. The artifact is already QUARANTINED, so the
	// handler short-circuits rather than restating everything.
	tx, _ = db.Begin()
	if err := h.Handle(context.Background(), tx, job); err != nil {
		t.Fatal(err)
	}
	if err := tx.Commit(); err != nil {
		t.Fatal(err)
	}

	if second := countFindings(); second != first {
		t.Errorf("a second run changed the finding count from %d to %d", first, second)
	}

	var events int
	if err := db.QueryRow(`SELECT COUNT(*) FROM outbox_events WHERE subject_id = ?`, resp.ArtifactID).Scan(&events); err != nil {
		t.Fatal(err)
	}
	if events != 1 {
		t.Errorf("%d outbox events after two runs, want 1", events)
	}
}

// The dispatcher writes each delivered event into the append-only audit ledger,
// exactly once.
func TestOutboxEventsReachTheAuditLedgerOnce(t *testing.T) {
	db, handler, queue, store := newWorkerTestEnv(t)

	scenario := GenerateNachaScenario(PresetBalancedPayroll)
	_, resp := doUpload(t, handler, uploadRequest(t, scenario.Filename, []byte(scenario.Content), nil))

	runPoolUntil(t, queue, store, func() bool {
		return artifactStatus(t, db, resp.ArtifactID) != "RECEIVED"
	})

	dispatcher := jobs.NewDispatcher(queue, &auditLogDeliverer{db: db}, time.Millisecond, 50)
	ctx := context.Background()

	delivered, failed, err := dispatcher.DispatchOnce(ctx)
	if err != nil || failed != 0 {
		t.Fatalf("dispatch: delivered=%d failed=%d err=%v", delivered, failed, err)
	}
	if delivered != 1 {
		t.Fatalf("delivered %d events, want 1", delivered)
	}

	countAudit := func() int {
		var n int
		if err := db.QueryRow(`
			SELECT COUNT(*) FROM audit_events WHERE event_type LIKE 'ARTIFACT_%'`).Scan(&n); err != nil {
			t.Fatal(err)
		}
		return n
	}
	after := countAudit()

	// A second pass delivers nothing, so the ledger does not grow.
	if n, _, err := dispatcher.DispatchOnce(ctx); err != nil || n != 0 {
		t.Errorf("a second dispatch delivered %d events (err=%v)", n, err)
	}
	if countAudit() != after {
		t.Error("a second dispatch pass added another audit entry")
	}
}

// A database integration stress test.
//
// It reports what it observed. There is no assertion on duration or throughput:
// this machine's numbers are not a claim about any other machine, and a
// hardcoded threshold would either be meaningless or flaky.
func TestConcurrentValidationStress(t *testing.T) {
	if testing.Short() {
		t.Skip("stress test")
	}
	db, handler, queue, store := newWorkerTestEnv(t)
	// SQLite is a single writer; more connections than this produce lock
	// contention rather than throughput, which would be measuring the wrong
	// thing.
	db.SetMaxOpenConns(8)

	const artifacts = 40
	uploaded := make([]int64, 0, artifacts)

	// Distinct content per artifact, so the content-hash uniqueness index does
	// not collapse them into one.
	for i := 0; i < artifacts; i++ {
		scenario := GenerateNachaScenario(PresetBalancedPayroll)
		content := scenario.Content + strings.Repeat(" ", 0) +
			fmt.Sprintf("%s\n", strings.Repeat("9", 94-len(fmt.Sprintf("%d", i))-1)+fmt.Sprintf("%d", i))
		// The trailing record makes each file distinct and is itself invalid,
		// so every artifact quarantines -- which exercises the finding-write
		// path under load rather than the trivial one.
		rec, resp := doUpload(t, handler, uploadRequest(t, fmt.Sprintf("stress-%d.ach", i), []byte(content), nil))
		if rec.Code != http.StatusAccepted {
			t.Fatalf("upload %d: %d %s", i, rec.Code, rec.Body.String())
		}
		uploaded = append(uploaded, resp.ArtifactID)
	}

	settled := func() bool {
		var pending int
		if err := db.QueryRow(`
			SELECT COUNT(*) FROM file_instances WHERE status = 'RECEIVED'`).Scan(&pending); err != nil {
			t.Fatal(err)
		}
		return pending == 0
	}

	start := time.Now()
	pool := runPoolUntil(t, queue, store, settled)
	elapsed := time.Since(start)

	// Every artifact settled, none left RECEIVED, none released.
	var released int
	if err := db.QueryRow(`SELECT COUNT(*) FROM file_instances WHERE status = 'RELEASED'`).Scan(&released); err != nil {
		t.Fatal(err)
	}
	if released != 0 {
		t.Errorf("%d artifacts reached RELEASED; a worker has no authority to release", released)
	}

	// Exactly one job per artifact, and each succeeded once.
	var succeeded, deadJobs int
	db.QueryRow(`SELECT COUNT(*) FROM ingestion_jobs WHERE state = 'SUCCEEDED'`).Scan(&succeeded)
	db.QueryRow(`SELECT COUNT(*) FROM ingestion_jobs WHERE state = 'DEAD'`).Scan(&deadJobs)
	if succeeded != artifacts {
		t.Errorf("%d jobs succeeded, want %d (dead: %d)", succeeded, artifacts, deadJobs)
	}

	// One outbox event per artifact: no duplicates under concurrency.
	var events int
	db.QueryRow(`SELECT COUNT(*) FROM outbox_events`).Scan(&events)
	if events != artifacts {
		t.Errorf("%d outbox events for %d artifacts", events, artifacts)
	}

	// No artifact has duplicated findings from a retry.
	rows, err := db.Query(`
		SELECT file_instance_id, COUNT(*) FROM validation_findings
		GROUP BY file_instance_id, code, line_number HAVING COUNT(*) > 1`)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	for rows.Next() {
		var id int64
		var n int
		rows.Scan(&id, &n)
		t.Errorf("artifact %d has %d copies of one finding", id, n)
	}

	stats := pool.Stats()
	t.Logf("observed: %d artifacts settled in %s; jobs claimed=%d succeeded=%d failed=%d leases-lost=%d",
		artifacts, elapsed.Round(time.Millisecond),
		stats.Claimed, stats.Succeeded, stats.Failed, stats.LeasesLost)
	t.Log("these numbers describe this run on this machine and are not a throughput claim")
}

// Concurrent enqueues of the same idempotency key must produce one job even
// when they race.
func TestConcurrentEnqueueOfOneKeyProducesOneJob(t *testing.T) {
	db, _, queue, _ := newWorkerTestEnv(t)
	db.SetMaxOpenConns(8)

	const racers = 16
	var wg sync.WaitGroup
	ids := make([]int64, racers)
	start := make(chan struct{})

	for i := 0; i < racers; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			<-start
			id, err := queue.Enqueue(context.Background(), jobs.EnqueueRequest{
				TenantID: DefaultTenantID, IdempotencyKey: "concurrent-key",
			})
			if err == nil {
				ids[i] = id
			}
		}(i)
	}
	close(start)
	wg.Wait()

	var n int
	if err := db.QueryRow(`SELECT COUNT(*) FROM ingestion_jobs WHERE idempotency_key = 'concurrent-key'`).Scan(&n); err != nil {
		t.Fatal(err)
	}
	if n != 1 {
		t.Errorf("%d jobs created for one idempotency key under a race", n)
	}
}

var _ = bytes.NewReader
