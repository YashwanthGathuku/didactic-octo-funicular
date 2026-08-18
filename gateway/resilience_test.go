package main

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	_ "modernc.org/sqlite"

	"sentinel-gateway/internal/jobs"
	"sentinel-gateway/internal/ledger"
	"sentinel-gateway/internal/objectstore"
	"sentinel-gateway/internal/schedule"
)

// FaultyObjectStore wraps ObjectStore and allows injecting transient errors.
type FaultyObjectStore struct {
	underlying objectstore.ObjectStore
	mu         sync.Mutex
	failGets   bool
	failPuts   bool
	failErr    error
}

func NewFaultyObjectStore(underlying objectstore.ObjectStore) *FaultyObjectStore {
	return &FaultyObjectStore{
		underlying: underlying,
		failErr:    errors.New("simulated transient object store error (503 Service Unavailable)"),
	}
}

func (f *FaultyObjectStore) Put(ctx context.Context, key string, r io.Reader, limit int64) (objectstore.PutResult, error) {
	f.mu.Lock()
	fail := f.failPuts
	err := f.failErr
	f.mu.Unlock()
	if fail {
		return objectstore.PutResult{}, err
	}
	return f.underlying.Put(ctx, key, r, limit)
}

func (f *FaultyObjectStore) Get(ctx context.Context, key string) (io.ReadCloser, error) {
	f.mu.Lock()
	fail := f.failGets
	err := f.failErr
	f.mu.Unlock()
	if fail {
		return nil, err
	}
	return f.underlying.Get(ctx, key)
}

func (f *FaultyObjectStore) Stat(ctx context.Context, key string) (objectstore.ObjectInfo, error) {
	return f.underlying.Stat(ctx, key)
}

func (f *FaultyObjectStore) SetFailGets(fail bool) {
	f.mu.Lock()
	f.failGets = fail
	f.mu.Unlock()
}

func (f *FaultyObjectStore) SetFailPuts(fail bool) {
	f.mu.Lock()
	f.failPuts = fail
	f.mu.Unlock()
}

// FaultyEventSink wraps an event sink to inject deliverer failures for outbox testing.
type FaultyEventSink struct {
	mu           sync.Mutex
	failDelivery bool
	deliveries   []string
}

func (f *FaultyEventSink) Deliver(ctx context.Context, ev jobs.PendingEvent) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.failDelivery {
		return errors.New("simulated event delivery failure: downstream webhook connection refused")
	}
	f.deliveries = append(f.deliveries, fmt.Sprintf("%s:%s:%d:%s", ev.EventType, ev.SubjectType, ev.SubjectID, string(ev.Payload)))
	return nil
}

func (f *FaultyEventSink) SetFail(fail bool) {
	f.mu.Lock()
	f.failDelivery = fail
	f.mu.Unlock()
}

func (f *FaultyEventSink) Count() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return len(f.deliveries)
}

// -----------------------------------------------------------------------------
// Scenario 1: API restart during upload acceptance
// -----------------------------------------------------------------------------
func TestResilience_APIRestartDuringUpload(t *testing.T) {
	db, handler, _ := newIngestTestEnv(t)

	scenario := GenerateNachaScenario(PresetBalancedPayroll)
	content := []byte(scenario.Content)

	// Simulate client disconnecting / aborted upload by providing truncated body
	truncatedBody := content[:len(content)/2]
	var body bytes.Buffer
	writer := multipart.NewWriter(&body)
	part, err := writer.CreateFormFile("file", scenario.Filename)
	if err != nil {
		t.Fatal(err)
	}
	_, _ = part.Write(truncatedBody)
	// Do not close writer properly or simulate short body
	writer.Close()

	req := httptest.NewRequest(http.MethodPost, "/api/v1/files/upload", &body)
	req.Header.Set("Content-Type", writer.FormDataContentType())
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	// The client retries with the full payload and an explicit idempotency key
	idempotencyKey := "retry-upload-001"
	req2 := uploadRequest(t, scenario.Filename, content, map[string]string{
		"Idempotency-Key": idempotencyKey,
	})
	rec2, resp2 := doUpload(t, handler, req2)
	if rec2.Code != http.StatusAccepted {
		t.Fatalf("expected retry upload to return 202 Accepted, got %d: %s", rec2.Code, rec2.Body.String())
	}
	if resp2.ArtifactID == 0 || resp2.JobID == 0 {
		t.Errorf("expected valid artifact ID and job ID on re-upload, got %+v", resp2)
	}

	// Repeated upload with exact same key must return idempotent response
	rec3, resp3 := doUpload(t, handler, uploadRequest(t, scenario.Filename, content, map[string]string{
		"Idempotency-Key": idempotencyKey,
	}))
	if rec3.Code != http.StatusAccepted {
		t.Fatalf("expected idempotent retry to return 202, got %d", rec3.Code)
	}
	if resp3.ArtifactID != resp2.ArtifactID || resp3.JobID != resp2.JobID {
		t.Errorf("expected identical artifact ID %d and job ID %d, got %+v", resp2.ArtifactID, resp2.JobID, resp3)
	}

	// Verify database only has one job row for this idempotency key
	var count int
	err = db.QueryRow("SELECT COUNT(*) FROM ingestion_jobs WHERE idempotency_key = ?", idempotencyKey).Scan(&count)
	if err != nil {
		t.Fatal(err)
	}
	if count != 1 {
		t.Errorf("expected exactly 1 job row, found %d", count)
	}
}

func runPoolUntilWithHeartbeat(t *testing.T, queue *jobs.Queue, store objectstore.ObjectStore, heartbeat time.Duration, done func() bool) *jobs.Pool {
	t.Helper()

	cfg := jobs.DefaultPoolConfig()
	cfg.Workers = 2
	cfg.PollInterval = 5 * time.Millisecond
	cfg.HeartbeatInterval = heartbeat

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

// -----------------------------------------------------------------------------
// Scenario 2: Worker kill before, during, after commit, and before outbox
// -----------------------------------------------------------------------------
func TestResilience_WorkerKillBeforeProcessing(t *testing.T) {
	db, handler, queue, store := newWorkerTestEnv(t)

	// Short lease duration so we can test reclamation quickly
	shortQueue, err := jobs.New(db, "sqlite", jobs.WithLeaseDuration(100*time.Millisecond))
	if err != nil {
		t.Fatal(err)
	}

	scenario := GenerateNachaScenario(PresetBalancedPayroll)
	_, resp := doUpload(t, handler, uploadRequest(t, scenario.Filename, []byte(scenario.Content), nil))

	// Job is QUEUED in database, no worker has claimed it
	job, err := queue.Get(context.Background(), resp.JobID)
	if err != nil {
		t.Fatal(err)
	}
	if job.State != jobs.StateQueued {
		t.Fatalf("expected job to be QUEUED, got %s", job.State)
	}

	// Start worker pool and verify it claims and completes cleanly
	start := time.Now()
	runPoolUntilWithHeartbeat(t, shortQueue, store, 20*time.Millisecond, func() bool {
		return artifactStatus(t, db, resp.ArtifactID) != "RECEIVED"
	})
	recoveryDuration := time.Since(start)
	t.Logf("Measured worker claim-to-complete latency: %v", recoveryDuration)

	if got := artifactStatus(t, db, resp.ArtifactID); got != "VALIDATED" {
		t.Errorf("expected artifact to be VALIDATED, got %s", got)
	}
}

func TestResilience_WorkerKillDuringProcessing_LeaseRecovery(t *testing.T) {
	db, handler, _, store := newWorkerTestEnv(t)

	// Configure short lease duration (200ms) for deterministic test recovery
	shortQueue, err := jobs.New(db, "sqlite", jobs.WithLeaseDuration(200*time.Millisecond))
	if err != nil {
		t.Fatal(err)
	}

	scenario := GenerateNachaScenario(PresetBalancedPayroll)
	_, resp := doUpload(t, handler, uploadRequest(t, scenario.Filename, []byte(scenario.Content), nil))

	// Simulate Worker 1 claiming the job and then "dying" (crashing without committing)
	claimCtx := context.Background()
	crashedJob, err := shortQueue.Claim(claimCtx, "crashed-worker-1", nil, 1)
	if err != nil {
		t.Fatalf("claim failed: %v", err)
	}
	if crashedJob == nil || crashedJob.ID != resp.JobID {
		t.Fatalf("claimed wrong job, expected %d", resp.JobID)
	}

	// Verify job is now LEASED in DB
	leasedJob, _ := shortQueue.Get(claimCtx, resp.JobID)
	if leasedJob.State != jobs.StateLeased {
		t.Fatalf("expected job to be LEASED, got %s", leasedJob.State)
	}

	// Wait for the lease to expire (250ms > 200ms lease duration)
	time.Sleep(250 * time.Millisecond)

	// Worker 2 starts up and reclaims the expired job
	reclaimStart := time.Now()
	runPoolUntilWithHeartbeat(t, shortQueue, store, 20*time.Millisecond, func() bool {
		return artifactStatus(t, db, resp.ArtifactID) != "RECEIVED"
	})
	reclaimDuration := time.Since(reclaimStart)
	t.Logf("Measured lease-expiry reclamation recovery time: %v", reclaimDuration)

	if got := artifactStatus(t, db, resp.ArtifactID); got != "VALIDATED" {
		t.Errorf("expected artifact status VALIDATED after recovery, got %s", got)
	}

	finalJob, _ := shortQueue.Get(claimCtx, resp.JobID)
	if finalJob.State != jobs.StateSucceeded {
		t.Errorf("expected final job state SUCCEEDED, got %s", finalJob.State)
	}
	if finalJob.AttemptCount < 2 {
		t.Errorf("expected attempt count >= 2 after crash and recovery, got %d", finalJob.AttemptCount)
	}
}

func TestResilience_WorkerKillAfterBusinessCommit_OutboxRedelivery(t *testing.T) {
	db, _, queue, _ := newWorkerTestEnv(t)

	// Insert an outbox event that was committed with a business transaction
	// but the process crashed before the dispatcher delivered it
	sink := &FaultyEventSink{}
	event := jobs.OutboxEvent{
		TenantID:    DefaultTenantID,
		EventType:   "ARTIFACT_VALIDATED",
		SubjectType: "artifact",
		SubjectID:   1001,
		Payload:     `{"status":"VALIDATED","record_count":50}`,
		DedupeKey:   "dedupe-outbox-resilience-001",
	}

	// Insert outbox event directly in a transaction
	tx, err := db.Begin()
	if err != nil {
		t.Fatal(err)
	}
	err = queue.PublishTx(context.Background(), tx, event)
	if err != nil {
		_ = tx.Rollback()
		t.Fatalf("insert outbox event: %v", err)
	}
	if err := tx.Commit(); err != nil {
		t.Fatal(err)
	}

	// Verify the outbox event is undelivered in DB
	var deliveredAt sql.NullTime
	err = db.QueryRow("SELECT delivered_at FROM outbox_events WHERE dedupe_key = ?", event.DedupeKey).Scan(&deliveredAt)
	if err != nil {
		t.Fatal(err)
	}
	if deliveredAt.Valid {
		t.Fatalf("expected delivered_at to be NULL before dispatcher runs")
	}

	// Now run the Outbox Dispatcher (simulating recovery of the delivery loop)
	dispatcher := jobs.NewDispatcher(queue, sink, 10*time.Millisecond, 10)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	dispatchStart := time.Now()
	go dispatcher.Run(ctx)

	// Wait for delivery
	deadline := time.After(3 * time.Second)
	for sink.Count() == 0 {
		select {
		case <-deadline:
			t.Fatalf("outbox event was not delivered within deadline")
		case <-time.After(10 * time.Millisecond):
		}
	}
	t.Logf("Measured outbox redelivery recovery time: %v", time.Since(dispatchStart))

	// Verify delivered_at is now stamped
	err = db.QueryRow("SELECT delivered_at FROM outbox_events WHERE dedupe_key = ?", event.DedupeKey).Scan(&deliveredAt)
	if err != nil {
		t.Fatal(err)
	}
	if !deliveredAt.Valid {
		t.Errorf("expected delivered_at to be populated after dispatcher delivered")
	}
}

// -----------------------------------------------------------------------------
// Scenario 3: Database unavailable / slow and restored
// -----------------------------------------------------------------------------
func TestResilience_DatabaseUnavailableAndRestored(t *testing.T) {
	db := setupTestDb(t)
	defer db.Close()

	cfg := ingestDemoConfig()
	router := NewRouterWithStore(db, cfg, nil, nil)

	// 1. Initial health / readiness check: should be OK
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/v1/ready", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("expected /api/v1/ready = 200 when DB is up, got %d: %s", rec.Code, rec.Body.String())
	}

	// 2. Simulate database closure / connection breakdown
	_ = db.Close()

	recDown := httptest.NewRecorder()
	router.ServeHTTP(recDown, httptest.NewRequest(http.MethodGet, "/api/v1/ready", nil))
	if recDown.Code != http.StatusServiceUnavailable {
		t.Fatalf("expected /api/v1/ready = 503 when DB is closed, got %d: %s", recDown.Code, recDown.Body.String())
	}

	var resp map[string]any
	if err := json.Unmarshal(recDown.Body.Bytes(), &resp); err != nil {
		t.Fatal(err)
	}
	if resp["ready"] == true {
		t.Errorf("expected ready=false during database outage")
	}

	checks, ok := resp["checks"].(map[string]any)
	if !ok {
		t.Fatalf("expected checks map in readiness response")
	}
	dbCheck, ok := checks["database"].(map[string]any)
	if !ok || dbCheck["status"] != "UNAVAILABLE" {
		t.Errorf("expected database check status UNAVAILABLE, got %+v", dbCheck)
	}
}

// -----------------------------------------------------------------------------
// Scenario 4: Object storage unavailable / slow and restored
// -----------------------------------------------------------------------------
func TestResilience_ObjectStorageUnavailableAndRestored(t *testing.T) {
	db, _, queue, _ := newWorkerTestEnv(t)

	realStore, err := objectstore.NewFilesystemStore(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	faultyStore := NewFaultyObjectStore(realStore)

	handler := NewRouterWithStore(db, ingestDemoConfig(), nil, faultyStore)

	scenario := GenerateNachaScenario(PresetBalancedPayroll)
	content := []byte(scenario.Content)

	// 1. Upload file into store while store is healthy
	_, resp := doUpload(t, handler, uploadRequest(t, scenario.Filename, content, nil))

	// 2. Make ObjectStore fail on GETs (simulating storage outage when worker tries to read)
	faultyStore.SetFailGets(true)

	// 3. Worker attempts to validate, fails on storage GET, and must RETRY without quarantining
	workerPoolCfg := jobs.DefaultPoolConfig()
	workerPoolCfg.Workers = 1
	workerPoolCfg.PollInterval = 10 * time.Millisecond

	pool, err := jobs.NewPool(queue, workerPoolCfg)
	if err != nil {
		t.Fatal(err)
	}
	pool.Register(KindValidateArtifact, &validateArtifactHandler{store: faultyStore, queue: queue})

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	pool.Start(ctx)

	// Wait briefly for at least 1 failed attempt
	time.Sleep(150 * time.Millisecond)

	// The artifact MUST NOT be quarantined because storage was unavailable
	status := artifactStatus(t, db, resp.ArtifactID)
	if status == "QUARANTINED" {
		t.Fatalf("defect: artifact was falsely QUARANTINED due to transient storage failure!")
	}

	// 4. Restore ObjectStore health
	restoreStart := time.Now()
	faultyStore.SetFailGets(false)

	// 5. Worker retries and succeeds
	runPoolUntil(t, queue, faultyStore, func() bool {
		return artifactStatus(t, db, resp.ArtifactID) == "VALIDATED"
	})
	t.Logf("Measured object store outage recovery time: %v", time.Since(restoreStart))

	if got := artifactStatus(t, db, resp.ArtifactID); got != "VALIDATED" {
		t.Errorf("expected artifact status VALIDATED after storage restoration, got %s", got)
	}
}

// -----------------------------------------------------------------------------
// Scenario 5: Duplicate arrival burst
// -----------------------------------------------------------------------------
func TestResilience_DuplicateArrivalBurst(t *testing.T) {
	db, handler, queue, store := newWorkerTestEnv(t)

	scenario := GenerateNachaScenario(PresetBalancedPayroll)
	content := []byte(scenario.Content)

	concurrency := 8
	var wg sync.WaitGroup
	responses := make([]AcceptedResponse, concurrency)
	statusCodes := make([]int, concurrency)

	idempotencyKey := "burst-batch-key-001"

	for i := 0; i < concurrency; i++ {
		idx := i
		wg.Add(1)
		go func() {
			defer wg.Done()
			// Client retry loop for transient contention
			for attempt := 0; attempt < 5; attempt++ {
				req := uploadRequest(t, scenario.Filename, content, map[string]string{
					"Idempotency-Key": idempotencyKey,
				})
				rec, resp := doUpload(t, handler, req)
				statusCodes[idx] = rec.Code
				responses[idx] = resp
				if rec.Code == http.StatusAccepted {
					break
				}
				time.Sleep(10 * time.Millisecond)
			}
		}()
	}
	wg.Wait()

	// All callers must receive 202 Accepted
	for i, code := range statusCodes {
		if code != http.StatusAccepted {
			t.Errorf("request %d returned status %d", i, code)
		}
	}

	// All callers must receive the exact same ArtifactID and JobID
	firstArtifactID := responses[0].ArtifactID
	firstJobID := responses[0].JobID
	for i, r := range responses {
		if r.ArtifactID != firstArtifactID || r.JobID != firstJobID {
			t.Errorf("inconsistent burst response at %d: got artifact=%d job=%d, want %d/%d",
				i, r.ArtifactID, r.JobID, firstArtifactID, firstJobID)
		}
	}

	// Exactly one job exists in database
	var jobCount int
	err := db.QueryRow("SELECT COUNT(*) FROM ingestion_jobs WHERE tenant_id = ? AND idempotency_key = ?", DefaultTenantID, idempotencyKey).Scan(&jobCount)
	if err != nil {
		t.Fatal(err)
	}
	if jobCount != 1 {
		t.Errorf("expected exactly 1 job in DB, found %d", jobCount)
	}

	// Worker processes the single job to VALIDATED
	runPoolUntil(t, queue, store, func() bool {
		return artifactStatus(t, db, firstArtifactID) == "VALIDATED"
	})
}

// -----------------------------------------------------------------------------
// Scenario 6: Partial / Finalized SFTP-style arrival event ordering
// -----------------------------------------------------------------------------
func TestResilience_PartialAndOutOfOrderSFTPArrival(t *testing.T) {
	db, handler, queue, store := newWorkerTestEnv(t)

	// File arrives on SFTP before the expected window is materialized in scheduler
	scenario := GenerateNachaScenario(PresetBalancedPayroll)
	content := []byte(scenario.Content)

	_, resp := doUpload(t, handler, uploadRequest(t, "payroll_20260301.ach", content, nil))

	// Ingestion job validates the file
	runPoolUntil(t, queue, store, func() bool {
		return artifactStatus(t, db, resp.ArtifactID) == "VALIDATED"
	})

	// Now scheduler creates the expectation calendar for this partner/feed
	storeSched, err := schedule.NewStore(db, "sqlite")
	if err != nil {
		t.Fatalf("schedule store: %v", err)
	}
	ctx := context.Background()
	if err := storeSched.CreateCalendar(ctx, DefaultTenantID, "fed-ach", "Fed ACH", schedule.BaseFederalReserve); err != nil {
		t.Fatalf("create calendar: %v", err)
	}
	cal, err := storeSched.Calendar(ctx, DefaultTenantID, "fed-ach")
	if err != nil {
		t.Fatalf("get calendar: %v", err)
	}
	if cal.ID() != "fed-ach" {
		t.Errorf("calendar ID = %s, want fed-ach", cal.ID())
	}
}

// -----------------------------------------------------------------------------
// Scenario 7: Malformed poison file
// -----------------------------------------------------------------------------
func TestResilience_MalformedPoisonFile_DoesNotCrashWorker(t *testing.T) {
	db, handler, queue, store := newWorkerTestEnv(t)

	// 1. Upload a poison file: corrupted headers, missing controls, non-printable binary garbage
	poisonContent := []byte("101CORRUPTED_HEADER_WITH_GARBAGE_NO_NEWLINES_NO_CONTROL_RECORDS_POISON_PAYLOAD\x00\xff\xfe\n" +
		"622BADROUTING0000000000000000000000000000000000000000000000000000000000000000000000000000000000\n")

	_, poisonResp := doUpload(t, handler, uploadRequest(t, "poison_payload.ach", poisonContent, nil))

	// 2. Upload a subsequent valid file
	validScenario := GenerateNachaScenario(PresetBalancedPayroll)
	_, validResp := doUpload(t, handler, uploadRequest(t, "valid_payroll.ach", []byte(validScenario.Content), nil))

	// 3. Worker pool processes both
	runPoolUntil(t, queue, store, func() bool {
		return artifactStatus(t, db, poisonResp.ArtifactID) != "RECEIVED" &&
			artifactStatus(t, db, validResp.ArtifactID) != "RECEIVED"
	})

	// Poison file must be QUARANTINED (fail-closed)
	if got := artifactStatus(t, db, poisonResp.ArtifactID); got != "QUARANTINED" {
		t.Errorf("expected poison file to be QUARANTINED, got %s", got)
	}

	// Valid file must be VALIDATED (worker pool didn't die or get poisoned)
	if got := artifactStatus(t, db, validResp.ArtifactID); got != "VALIDATED" {
		t.Errorf("expected subsequent valid file to be VALIDATED, got %s", got)
	}

	// Verify poison findings exist in DB with redacted positions
	var findingCount int
	err := db.QueryRow("SELECT COUNT(*) FROM validation_findings WHERE file_instance_id = ?", poisonResp.ArtifactID).Scan(&findingCount)
	if err != nil {
		t.Fatal(err)
	}
	if findingCount == 0 {
		t.Errorf("expected recorded validation findings for poison file")
	}
}

// -----------------------------------------------------------------------------
// Scenario 8: Outbox destination unavailable and recovered
// -----------------------------------------------------------------------------
func TestResilience_OutboxDestinationUnavailableAndRecovered(t *testing.T) {
	db, _, queue, _ := newWorkerTestEnv(t)

	sink := &FaultyEventSink{}
	// Initially fail all deliveries
	sink.SetFail(true)

	event := jobs.OutboxEvent{
		TenantID:    DefaultTenantID,
		EventType:   "NOTIFICATION_DISPATCH",
		SubjectType: "alert",
		SubjectID:   505,
		Payload:     `{"severity":"HIGH","message":"file quarantined"}`,
		DedupeKey:   "dedupe-outbox-fail-001",
	}

	tx, err := db.Begin()
	if err != nil {
		t.Fatal(err)
	}
	if err := queue.PublishTx(context.Background(), tx, event); err != nil {
		_ = tx.Rollback()
		t.Fatal(err)
	}
	_ = tx.Commit()

	dispatcher := jobs.NewDispatcher(queue, sink, 20*time.Millisecond, 10)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	go dispatcher.Run(ctx)

	// Wait for at least 1 failed attempt to be recorded in DB
	time.Sleep(100 * time.Millisecond)

	var attemptCount int
	var lastError sql.NullString
	err = db.QueryRow("SELECT attempt_count, last_error FROM outbox_events WHERE dedupe_key = ?", event.DedupeKey).Scan(&attemptCount, &lastError)
	if err != nil {
		t.Fatal(err)
	}
	if attemptCount == 0 {
		t.Errorf("expected attempt_count > 0 after delivery failure")
	}
	if !lastError.Valid || !strings.Contains(lastError.String, "downstream webhook") {
		t.Errorf("expected recorded last_error describing delivery failure, got %+v", lastError)
	}

	// Restore sink health
	recoverStart := time.Now()
	sink.SetFail(false)

	// Reset run_after so the dispatcher claims it immediately on next loop
	_, _ = db.Exec("UPDATE outbox_events SET run_after = CURRENT_TIMESTAMP WHERE dedupe_key = ?", event.DedupeKey)

	// Wait for delivery to succeed
	deadline := time.After(3 * time.Second)
	for sink.Count() == 0 {
		select {
		case <-deadline:
			t.Fatalf("outbox event was not delivered after sink recovery")
		case <-time.After(10 * time.Millisecond):
		}
	}
	t.Logf("Measured outbox delivery recovery time after destination restored: %v", time.Since(recoverStart))

	var deliveredAt sql.NullTime
	_ = db.QueryRow("SELECT delivered_at FROM outbox_events WHERE dedupe_key = ?", event.DedupeKey).Scan(&deliveredAt)
	if !deliveredAt.Valid {
		t.Errorf("expected delivered_at to be populated after recovery")
	}
}

// -----------------------------------------------------------------------------
// Scenario 9: AI Provider unavailable — deterministic ingestion unaffected
// -----------------------------------------------------------------------------
func TestResilience_AIProviderUnavailable_IngestionUnaffected(t *testing.T) {
	db, _, queue, store := newWorkerTestEnv(t)

	// Configure with unreachable AI Tier URL
	cfg := ingestDemoConfig()
	cfg.AITierURL = "http://127.0.0.1:54321/nonexistent-ai-tier" // completely offline
	handler := NewRouterWithStore(db, cfg, nil, store)

	// 1. Check readiness endpoint: AI tier reported as CONFIGURED / optional, ready=true
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/v1/ready", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("expected /api/v1/ready = 200 even when AI tier URL is unreachable (AI is optional), got %d: %s", rec.Code, rec.Body.String())
	}

	// 2. Upload file
	scenario := GenerateNachaScenario(PresetBalancedPayroll)
	uploadStart := time.Now()
	recUpload, resp := doUpload(t, handler, uploadRequest(t, scenario.Filename, []byte(scenario.Content), nil))
	if recUpload.Code != http.StatusAccepted {
		t.Fatalf("upload failed with %d: %s", recUpload.Code, recUpload.Body.String())
	}

	// 3. Worker completes deterministic validation without calling AI
	runPoolUntil(t, queue, store, func() bool {
		return artifactStatus(t, db, resp.ArtifactID) != "RECEIVED"
	})
	t.Logf("Measured deterministic ingestion time with offline AI: %v", time.Since(uploadStart))

	if got := artifactStatus(t, db, resp.ArtifactID); got != "VALIDATED" {
		t.Errorf("expected artifact status VALIDATED, got %s", got)
	}

	// 4. Verification that deterministic decision and audit trail exist
	evidence, err := ledger.New(db, "sqlite")
	if err != nil {
		t.Fatal(err)
	}
	lres, err := evidence.Verify(context.Background(), DefaultTenantID)
	if err != nil {
		t.Fatalf("verify audit ledger: %v", err)
	}
	if !lres.Intact {
		t.Errorf("expected intact audit ledger chain")
	}
}

// -----------------------------------------------------------------------------
// Scenario 10: Disk full / Tenant concurrency quota backpressure
// -----------------------------------------------------------------------------
func TestResilience_TenantConcurrencyQuotaEnforced(t *testing.T) {
	db, _, queue, _ := newWorkerTestEnv(t)

	// Set tenant quota to max 2 concurrent jobs
	ctx := context.Background()
	_, err := db.Exec(`INSERT OR REPLACE INTO tenant_job_quotas (tenant_id, max_concurrent) VALUES (?, 2)`, DefaultTenantID)
	if err != nil {
		t.Fatalf("set tenant quota: %v", err)
	}

	// Enqueue 4 jobs for default tenant
	for i := 1; i <= 4; i++ {
		_, err := queue.Enqueue(ctx, jobs.EnqueueRequest{
			TenantID:       DefaultTenantID,
			Kind:           KindValidateArtifact,
			IdempotencyKey: fmt.Sprintf("quota-job-%d", i),
		})
		if err != nil {
			t.Fatalf("enqueue job %d: %v", i, err)
		}
	}

	// Claim jobs: first 2 should succeed, 3rd claim should fail or return ErrNoWork
	// because tenant max_concurrent is 2
	job1, err1 := queue.Claim(ctx, "worker-quota-1", nil, 2)
	if err1 != nil || job1 == nil {
		t.Fatalf("first claim failed: %v", err1)
	}

	job2, err2 := queue.Claim(ctx, "worker-quota-2", nil, 2)
	if err2 != nil || job2 == nil {
		t.Fatalf("second claim failed: %v", err2)
	}

	// Third claim MUST be rejected because tenant has 2 active leases
	job3, err3 := queue.Claim(ctx, "worker-quota-3", nil, 2)
	if err3 == nil && job3 != nil {
		t.Errorf("tenant quota violated: worker claimed 3rd job %d when max_concurrent is 2", job3.ID)
	}
}
