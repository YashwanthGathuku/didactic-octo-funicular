// Package jobs runs durable background work.
//
// The property everything here is built around: a job is owned by exactly one
// worker at a time, and a crash at any point loses no work and duplicates no
// side effect that matters.
//
// That is harder than it sounds because the failure modes are all invisible in
// normal operation. Two workers claiming one job looks fine until the job has a
// side effect. A worker whose lease expired mid-work looks fine until it
// overwrites a completion written by its replacement. An event published after
// the transaction that produced it looks fine until the process dies in the gap.
// Each of those has a test in this package that constructs the race rather than
// asserting a flag.
package jobs

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"math/rand/v2"
	"strings"
	"time"
)

// State is a job's lifecycle position. The values match the CHECK constraint in
// migration 002.
type State string

const (
	StateQueued    State = "QUEUED"
	StateLeased    State = "LEASED"
	StateRunning   State = "RUNNING"
	StateSucceeded State = "SUCCEEDED"
	StateRetryable State = "RETRYABLE"
	StateDead      State = "DEAD"
	StateCancelled State = "CANCELLED"
)

// Terminal reports whether no further work will be done on this job.
func (s State) Terminal() bool {
	return s == StateSucceeded || s == StateDead || s == StateCancelled
}

var (
	// ErrNoWork means nothing was claimable. It is the ordinary idle case, not
	// a failure, and a caller must not log it as one.
	ErrNoWork = errors.New("no claimable job")

	// ErrLeaseLost means this worker no longer owns the job. It is returned
	// rather than ignored because the correct response is to abandon the work,
	// not to finish and write a result that would overwrite a newer one.
	ErrLeaseLost = errors.New("lease lost: this worker no longer owns the job")

	// ErrOverloaded is the backpressure signal. A caller that receives it must
	// shed load rather than queue more work.
	ErrOverloaded = errors.New("job system is at capacity")
)

// Job is a unit of durable work.
type Job struct {
	ID             int64
	TenantID       string
	Kind           string
	ArtifactID     sql.NullInt64
	IdempotencyKey string
	State          State
	AttemptCount   int
	MaxAttempts    int
	RowVersion     int64

	LeaseOwner      sql.NullString
	LeaseExpiresAt  sql.NullTime
	LastHeartbeatAt sql.NullTime
}

// Attempt returns the attempt number this claim represents.
func (j *Job) Attempt() int { return j.AttemptCount }

// dialect selects the locking strategy. The two databases need genuinely
// different SQL here and pretending otherwise would produce a queue that is
// correct on one and racy on the other.
type dialect int

const (
	// dialectPostgres uses SELECT ... FOR UPDATE SKIP LOCKED, which lets N
	// workers each take a different row in one pass without blocking.
	dialectPostgres dialect = iota

	// dialectSQLite relies on SQLite serialising writers: a conditional UPDATE
	// guarded by row_version either matches one row or none, and two
	// simultaneous claims cannot both match. SKIP LOCKED does not exist there.
	dialectSQLite
)

// Queue is the durable job store.
type Queue struct {
	db      *sql.DB
	dialect dialect

	// leaseDuration is how long a claim holds. It must exceed the longest
	// expected handler run: too short and a healthy worker's lease expires
	// under it, too long and a crashed worker's job is stuck for that duration.
	leaseDuration time.Duration

	// now is injectable so lease expiry can be tested without sleeping. A test
	// that proves an expiry by waiting proves the clock works.
	now func() time.Time
}

// Option configures a Queue.
type Option func(*Queue)

// WithLeaseDuration sets how long a claim holds before it may be stolen.
func WithLeaseDuration(d time.Duration) Option {
	return func(q *Queue) { q.leaseDuration = d }
}

// WithClock replaces the time source.
func WithClock(fn func() time.Time) Option {
	return func(q *Queue) { q.now = fn }
}

// New builds a Queue against an already-migrated database.
//
// The driver name selects the locking strategy. It is passed explicitly rather
// than sniffed, because a wrong guess produces a queue that appears to work and
// races under load -- the worst possible failure for this component.
func New(db *sql.DB, driverName string, opts ...Option) (*Queue, error) {
	if db == nil {
		return nil, errors.New("a job queue requires a database handle")
	}
	var d dialect
	switch {
	case strings.Contains(driverName, "pgx"), strings.Contains(driverName, "postgres"):
		d = dialectPostgres
	case strings.Contains(driverName, "sqlite"):
		d = dialectSQLite
	default:
		return nil, fmt.Errorf("unsupported driver %q: the claim strategy differs per database and must not be guessed", driverName)
	}

	q := &Queue{db: db, dialect: d, leaseDuration: 60 * time.Second, now: time.Now}
	for _, opt := range opts {
		opt(q)
	}
	return q, nil
}

// rebind converts ? placeholders to $N for PostgreSQL.
//
// The queries are written once with ? because they are identical apart from
// placeholder syntax; the two claim statements, which genuinely differ, are
// written out separately below.
func (q *Queue) rebind(query string) string {
	if q.dialect != dialectPostgres {
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

// EnqueueRequest describes work to be done.
type EnqueueRequest struct {
	TenantID string
	Kind     string

	// IdempotencyKey is the business identity of this work. Enqueuing the same
	// key twice produces one job, which is what makes duplicate delivery
	// harmless at the queue boundary rather than in every handler.
	IdempotencyKey string

	ArtifactID  int64
	MaxAttempts int
}

// EnqueueTx adds a job inside a caller's transaction.
//
// It takes a transaction rather than opening one so the job and the business
// state it refers to commit together. A job enqueued in its own transaction can
// be observed by a worker before the artifact it names exists.
func (q *Queue) EnqueueTx(ctx context.Context, tx *sql.Tx, req EnqueueRequest) (int64, error) {
	if req.TenantID == "" || req.IdempotencyKey == "" {
		return 0, errors.New("a job requires a tenant and an idempotency key")
	}
	kind := req.Kind
	if kind == "" {
		kind = "VALIDATE_ARTIFACT"
	}
	maxAttempts := req.MaxAttempts
	if maxAttempts <= 0 {
		maxAttempts = 5
	}

	// A duplicate key is not an error: the work is already scheduled. The
	// existing job's id is returned so the caller records the same one.
	var existing int64
	err := tx.QueryRowContext(ctx, q.rebind(
		`SELECT id FROM ingestion_jobs WHERE tenant_id = ? AND idempotency_key = ?`),
		req.TenantID, req.IdempotencyKey).Scan(&existing)
	if err == nil {
		return existing, nil
	}
	if !errors.Is(err, sql.ErrNoRows) {
		return 0, err
	}

	var artifact any
	if req.ArtifactID > 0 {
		artifact = req.ArtifactID
	}

	now := q.now().UTC()
	if q.dialect == dialectPostgres {
		var id int64
		err = tx.QueryRowContext(ctx, q.rebind(`
			INSERT INTO ingestion_jobs
				(tenant_id, file_instance_id, idempotency_key, kind, state, max_attempts, run_after, created_at, updated_at)
			VALUES (?, ?, ?, ?, 'QUEUED', ?, ?, ?, ?)
			RETURNING id`),
			req.TenantID, artifact, req.IdempotencyKey, kind, maxAttempts, now, now, now).Scan(&id)
		return id, err
	}

	res, err := tx.ExecContext(ctx, `
		INSERT INTO ingestion_jobs
			(tenant_id, file_instance_id, idempotency_key, kind, state, max_attempts, run_after, created_at, updated_at)
		VALUES (?, ?, ?, ?, 'QUEUED', ?, ?, ?, ?)`,
		req.TenantID, artifact, req.IdempotencyKey, kind, maxAttempts, now, now, now)
	if err != nil {
		return 0, err
	}
	return res.LastInsertId()
}

// Enqueue adds a job in its own transaction, for callers with no business state
// to commit alongside.
func (q *Queue) Enqueue(ctx context.Context, req EnqueueRequest) (int64, error) {
	tx, err := q.db.BeginTx(ctx, nil)
	if err != nil {
		return 0, err
	}
	defer func() { _ = tx.Rollback() }()

	id, err := q.EnqueueTx(ctx, tx, req)
	if err != nil {
		return 0, err
	}
	return id, tx.Commit()
}

// maxClaimPasses bounds the retry loop inside a single Claim.
//
// A tenant found to be at its quota is excluded and the search repeats, so one
// saturated tenant at the head of the queue does not make the whole pass return
// empty. The bound stops that from becoming an unbounded scan.
const maxClaimPasses = 8

// Claim takes ownership of one runnable job for the given worker.
//
// It returns ErrNoWork when nothing is claimable, which is the ordinary idle
// case. `excludeTenants` lets the caller skip tenants it already knows are
// saturated; the authoritative quota check happens here, atomically with taking
// the lease.
//
// The quota must be enforced inside the claiming transaction. An earlier
// version counted a tenant's live leases first and then claimed, which is a
// time-of-check-to-time-of-use gap: four workers could each count two running
// jobs, each conclude there was room, and each take a third. The concurrency
// test caught it doing exactly that, on both databases.
func (q *Queue) Claim(ctx context.Context, workerID string, excludeTenants []string, perTenantLimit int) (*Job, error) {
	if workerID == "" {
		return nil, errors.New("a claim requires a worker identity; an anonymous lease cannot be attributed or expired")
	}
	if perTenantLimit <= 0 {
		perTenantLimit = 1
	}

	excluded := append([]string{}, excludeTenants...)
	for pass := 0; pass < maxClaimPasses; pass++ {
		job, err := q.claimOnce(ctx, workerID, excluded, perTenantLimit)
		if errors.Is(err, errTenantSaturated) {
			// The candidate's tenant is at its limit. Exclude it and look past
			// it rather than reporting the queue empty.
			excluded = append(excluded, err.(*tenantSaturatedError).tenantID)
			continue
		}
		return job, err
	}
	return nil, ErrNoWork
}

// errTenantSaturated signals that the candidate job's tenant is at its quota.
var errTenantSaturated = errors.New("tenant is at its concurrency limit")

type tenantSaturatedError struct{ tenantID string }

func (e *tenantSaturatedError) Error() string        { return errTenantSaturated.Error() }
func (e *tenantSaturatedError) Is(target error) bool { return target == errTenantSaturated }

func (q *Queue) claimOnce(ctx context.Context, workerID string, excludeTenants []string, perTenantLimit int) (*Job, error) {
	now := q.now().UTC()
	expiry := now.Add(q.leaseDuration)

	tx, err := q.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, err
	}
	defer func() { _ = tx.Rollback() }()

	job, err := q.selectClaimable(ctx, tx, now, excludeTenants)
	if err != nil {
		return nil, err
	}

	limit := q.tenantLimitTx(ctx, tx, job.TenantID, perTenantLimit)

	// The two databases need different mechanisms here, and the difference is
	// not cosmetic.
	//
	// PostgreSQL: lock the tenant row. Under READ COMMITTED a concurrent
	// claimer's uncommitted lease is invisible, so counting alone undercounts.
	// Locking the tenant serialises same-tenant claims -- which is precisely the
	// resource being bounded -- while different tenants proceed in parallel.
	//
	// SQLite: there is no FOR UPDATE, and there does not need to be. The count
	// is folded into the UPDATE's WHERE clause, and SQLite evaluates that while
	// holding the write lock, so the check and the lease are one atomic step.
	var updateSQL string
	var args []any
	if q.dialect == dialectPostgres {
		if _, err := tx.ExecContext(ctx, q.rebind(
			`SELECT 1 FROM tenants WHERE id = ? FOR UPDATE`), job.TenantID); err != nil {
			return nil, err
		}
		var running int
		if err := tx.QueryRowContext(ctx, q.rebind(`
			SELECT COUNT(*) FROM ingestion_jobs
			WHERE tenant_id = ? AND state IN ('LEASED','RUNNING') AND lease_expires_at > ?`),
			job.TenantID, now).Scan(&running); err != nil {
			return nil, err
		}
		if running >= limit {
			return nil, &tenantSaturatedError{tenantID: job.TenantID}
		}
		updateSQL = `
			UPDATE ingestion_jobs
			SET state = 'LEASED', lease_owner = ?, lease_expires_at = ?, last_heartbeat_at = ?,
			    attempt_count = attempt_count + 1, row_version = row_version + 1, updated_at = ?
			WHERE id = ? AND row_version = ?`
		args = []any{workerID, expiry, now, now, job.ID, job.RowVersion}
	} else {
		updateSQL = `
			UPDATE ingestion_jobs
			SET state = 'LEASED', lease_owner = ?, lease_expires_at = ?, last_heartbeat_at = ?,
			    attempt_count = attempt_count + 1, row_version = row_version + 1, updated_at = ?
			WHERE id = ? AND row_version = ?
			  AND (SELECT COUNT(*) FROM ingestion_jobs AS running
			       WHERE running.tenant_id = ?
			         AND running.state IN ('LEASED','RUNNING')
			         AND running.lease_expires_at > ?) < ?`
		args = []any{workerID, expiry, now, now, job.ID, job.RowVersion, job.TenantID, now, limit}
	}

	res, err := tx.ExecContext(ctx, q.rebind(updateSQL), args...)
	if err != nil {
		return nil, err
	}
	affected, err := res.RowsAffected()
	if err != nil {
		return nil, err
	}
	if affected == 0 {
		// Either another worker won the row, or the tenant is at its quota. On
		// SQLite the two are indistinguishable from here, so the tenant is
		// excluded and the search continues -- which is correct for both cases.
		return nil, &tenantSaturatedError{tenantID: job.TenantID}
	}

	job.State = StateLeased
	job.AttemptCount++
	job.RowVersion++
	job.LeaseOwner = sql.NullString{String: workerID, Valid: true}
	job.LeaseExpiresAt = sql.NullTime{Time: expiry, Valid: true}

	if _, err := tx.ExecContext(ctx, q.rebind(`
		INSERT INTO job_attempts (tenant_id, job_id, attempt_no, outcome, started_at)
		VALUES (?, ?, ?, 'STARTED', ?)`),
		job.TenantID, job.ID, job.AttemptCount, now); err != nil {
		return nil, err
	}

	if err := tx.Commit(); err != nil {
		return nil, err
	}
	return job, nil
}

// selectClaimable finds one runnable job, locking it where the database can.
func (q *Queue) selectClaimable(ctx context.Context, tx *sql.Tx, now time.Time, excludeTenants []string) (*Job, error) {
	// Runnable means: queued or retryable and due, or leased with an expired
	// lease. The third case is how a crashed worker's job returns to the pool.
	const columns = `id, tenant_id, kind, file_instance_id, idempotency_key, state,
	                 attempt_count, max_attempts, row_version, lease_owner, lease_expires_at`

	where := `
		WHERE (
		    (state IN ('QUEUED','RETRYABLE') AND (run_after IS NULL OR run_after <= ?))
		 OR (state IN ('LEASED','RUNNING') AND lease_expires_at IS NOT NULL AND lease_expires_at <= ?)
		)
		AND attempt_count < max_attempts`

	args := []any{now, now}
	if len(excludeTenants) > 0 {
		where += " AND tenant_id NOT IN (" + placeholders(len(excludeTenants)) + ")"
		for _, t := range excludeTenants {
			args = append(args, t)
		}
	}

	// Oldest first, so a backlog drains in arrival order rather than starving
	// its head.
	order := " ORDER BY id ASC LIMIT 1"

	query := "SELECT " + columns + " FROM ingestion_jobs" + where + order
	if q.dialect == dialectPostgres {
		// SKIP LOCKED is what lets many workers scan the same queue at once:
		// a row another transaction holds is passed over rather than blocking
		// this one. Without it, N workers serialise on the head of the queue.
		query += " FOR UPDATE SKIP LOCKED"
	}

	var job Job
	var state string
	err := tx.QueryRowContext(ctx, q.rebind(query), args...).Scan(
		&job.ID, &job.TenantID, &job.Kind, &job.ArtifactID, &job.IdempotencyKey,
		&state, &job.AttemptCount, &job.MaxAttempts, &job.RowVersion,
		&job.LeaseOwner, &job.LeaseExpiresAt)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNoWork
	}
	if err != nil {
		return nil, err
	}
	job.State = State(state)
	return &job, nil
}

func placeholders(n int) string {
	parts := make([]string, n)
	for i := range parts {
		parts[i] = "?"
	}
	return strings.Join(parts, ",")
}

// Heartbeat extends a lease and records that the owner is alive.
//
// It fails with ErrLeaseLost if the job has been taken over. That is the signal
// a slow worker needs to stop: continuing would mean finishing work whose
// result will be discarded, or worse, written over a newer one.
func (q *Queue) Heartbeat(ctx context.Context, job *Job, workerID string) error {
	now := q.now().UTC()
	expiry := now.Add(q.leaseDuration)

	res, err := q.db.ExecContext(ctx, q.rebind(`
		UPDATE ingestion_jobs
		SET lease_expires_at = ?, last_heartbeat_at = ?, row_version = row_version + 1, updated_at = ?
		WHERE id = ? AND lease_owner = ? AND row_version = ?`),
		expiry, now, now, job.ID, workerID, job.RowVersion)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrLeaseLost
	}
	job.RowVersion++
	job.LeaseExpiresAt = sql.NullTime{Time: expiry, Valid: true}
	return nil
}

// CompleteTx marks a job succeeded inside the caller's transaction.
//
// Taking a transaction is what lets a handler commit its business state, its
// outbox events and its job completion atomically. A completion committed
// separately can succeed while the work it describes rolls back.
//
// The lease is re-checked in the same statement, so a worker whose lease expired
// mid-work cannot overwrite the completion its replacement already wrote.
func (q *Queue) CompleteTx(ctx context.Context, tx *sql.Tx, job *Job, workerID string) error {
	now := q.now().UTC()
	res, err := tx.ExecContext(ctx, q.rebind(`
		UPDATE ingestion_jobs
		SET state = 'SUCCEEDED', lease_owner = NULL, lease_expires_at = NULL,
		    row_version = row_version + 1, updated_at = ?
		WHERE id = ? AND lease_owner = ? AND row_version = ? AND state NOT IN ('SUCCEEDED','DEAD','CANCELLED')`),
		now, job.ID, workerID, job.RowVersion)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrLeaseLost
	}

	if _, err := tx.ExecContext(ctx, q.rebind(`
		UPDATE job_attempts SET outcome = 'SUCCEEDED', finished_at = ?
		WHERE tenant_id = ? AND job_id = ? AND attempt_no = ?`),
		now, job.TenantID, job.ID, job.AttemptCount); err != nil {
		return err
	}

	job.State = StateSucceeded
	job.RowVersion++
	return nil
}

// maxBackoff caps the retry delay. Without a cap an exponential schedule
// reaches days, and a job nobody will look at for a day is a job nobody will
// look at.
const maxBackoff = 15 * time.Minute

// backoff computes the delay before the next attempt.
//
// Exponential with full jitter. The jitter is not cosmetic: without it, a batch
// of jobs that failed together retries together, which reproduces the load that
// caused the failure at exactly the wrong moment.
func backoff(attempt int, base time.Duration) time.Duration {
	if attempt < 1 {
		attempt = 1
	}
	exp := float64(base) * math.Pow(2, float64(attempt-1))
	if exp > float64(maxBackoff) {
		exp = float64(maxBackoff)
	}
	// Full jitter: uniform in [0, exp). Retries spread across the whole window
	// rather than clustering at its end.
	return time.Duration(rand.Int64N(int64(exp) + 1))
}

// Fail records a failed attempt and schedules a retry, or kills the job.
func (q *Queue) Fail(ctx context.Context, job *Job, workerID string, cause error) error {
	now := q.now().UTC()
	// The message is truncated: a handler error can carry an arbitrary upstream
	// body, and an unbounded string in a queue row is a storage vector.
	msg := cause.Error()
	if len(msg) > 2000 {
		msg = msg[:2000] + "…"
	}

	tx, err := q.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()

	exhausted := job.AttemptCount >= job.MaxAttempts
	nextState := StateRetryable
	var runAfter any = now.Add(backoff(job.AttemptCount, time.Second))
	var terminalReason any
	var deadAt any
	if exhausted {
		nextState = StateDead
		runAfter = nil
		terminalReason = fmt.Sprintf("retry budget exhausted after %d attempts", job.AttemptCount)
		deadAt = now
	}
	_ = deadAt

	res, err := tx.ExecContext(ctx, q.rebind(`
		UPDATE ingestion_jobs
		SET state = ?, last_error = ?, run_after = ?, terminal_reason = ?,
		    lease_owner = NULL, lease_expires_at = NULL,
		    row_version = row_version + 1, updated_at = ?
		WHERE id = ? AND lease_owner = ? AND row_version = ? AND state NOT IN ('SUCCEEDED','DEAD','CANCELLED')`),
		string(nextState), msg, runAfter, terminalReason, now, job.ID, workerID, job.RowVersion)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrLeaseLost
	}

	outcome := "RETRYABLE"
	if exhausted {
		outcome = "DEAD"
	}
	if _, err := tx.ExecContext(ctx, q.rebind(`
		UPDATE job_attempts SET outcome = ?, error = ?, finished_at = ?
		WHERE tenant_id = ? AND job_id = ? AND attempt_no = ?`),
		outcome, msg, now, job.TenantID, job.ID, job.AttemptCount); err != nil {
		return err
	}

	job.State = nextState
	job.RowVersion++
	return tx.Commit()
}

// Get reads a job's current state, for tests and for operator queries.
func (q *Queue) Get(ctx context.Context, id int64) (*Job, error) {
	var job Job
	var state string
	err := q.db.QueryRowContext(ctx, q.rebind(`
		SELECT id, tenant_id, kind, file_instance_id, idempotency_key, state,
		       attempt_count, max_attempts, row_version, lease_owner, lease_expires_at, last_heartbeat_at
		FROM ingestion_jobs WHERE id = ?`), id).Scan(
		&job.ID, &job.TenantID, &job.Kind, &job.ArtifactID, &job.IdempotencyKey,
		&state, &job.AttemptCount, &job.MaxAttempts, &job.RowVersion,
		&job.LeaseOwner, &job.LeaseExpiresAt, &job.LastHeartbeatAt)
	if err != nil {
		return nil, err
	}
	job.State = State(state)
	return &job, nil
}

// RunningPerTenant counts the jobs each tenant currently holds a live lease on.
//
// It is what the worker pool consults to honour per-tenant quotas, and it reads
// live leases rather than a counter, so a crashed worker's slot is released by
// its lease expiring rather than by anything remembering to decrement.
func (q *Queue) RunningPerTenant(ctx context.Context) (map[string]int, error) {
	now := q.now().UTC()
	rows, err := q.db.QueryContext(ctx, q.rebind(`
		SELECT tenant_id, COUNT(*) FROM ingestion_jobs
		WHERE state IN ('LEASED','RUNNING') AND lease_expires_at > ?
		GROUP BY tenant_id`), now)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	out := map[string]int{}
	for rows.Next() {
		var tenant string
		var n int
		if err := rows.Scan(&tenant, &n); err != nil {
			return nil, err
		}
		out[tenant] = n
	}
	return out, rows.Err()
}

// TenantConcurrencyLimit returns a tenant's quota, or the default.
func (q *Queue) TenantConcurrencyLimit(ctx context.Context, tenantID string, fallback int) int {
	var limit int
	err := q.db.QueryRowContext(ctx, q.rebind(
		`SELECT max_concurrent FROM tenant_job_quotas WHERE tenant_id = ?`), tenantID).Scan(&limit)
	if err != nil || limit <= 0 {
		return fallback
	}
	return limit
}

// tenantLimitTx reads the quota inside the claiming transaction, so the limit
// applied is the one in force at the moment the lease is taken.
func (q *Queue) tenantLimitTx(ctx context.Context, tx *sql.Tx, tenantID string, fallback int) int {
	var limit int
	err := tx.QueryRowContext(ctx, q.rebind(
		`SELECT max_concurrent FROM tenant_job_quotas WHERE tenant_id = ?`), tenantID).Scan(&limit)
	if err != nil || limit <= 0 {
		return fallback
	}
	return limit
}

// Depth reports how much work is waiting, for backpressure decisions and for
// the metrics endpoint. It is measured, not estimated.
func (q *Queue) Depth(ctx context.Context) (int, error) {
	var n int
	err := q.db.QueryRowContext(ctx, q.rebind(`
		SELECT COUNT(*) FROM ingestion_jobs
		WHERE state IN ('QUEUED','RETRYABLE') AND attempt_count < max_attempts`)).Scan(&n)
	return n, err
}

// PublishTx writes an outbox event in the caller's transaction.
//
// This is the transactional outbox in one method: the event and the business
// state it describes commit together, and nothing is sent from inside the
// transaction. A duplicate dedupe_key is silently accepted as already recorded,
// so a handler that runs twice does not produce two events.
func (q *Queue) PublishTx(ctx context.Context, tx *sql.Tx, ev OutboxEvent) error {
	if ev.TenantID == "" || ev.DedupeKey == "" {
		return errors.New("an outbox event requires a tenant and a dedupe key")
	}
	payload, err := json.Marshal(ev.Payload)
	if err != nil {
		return fmt.Errorf("marshal outbox payload: %w", err)
	}

	var exists int
	err = tx.QueryRowContext(ctx, q.rebind(
		`SELECT COUNT(*) FROM outbox_events WHERE tenant_id = ? AND dedupe_key = ?`),
		ev.TenantID, ev.DedupeKey).Scan(&exists)
	if err != nil {
		return err
	}
	if exists > 0 {
		return nil
	}

	now := q.now().UTC()
	_, err = tx.ExecContext(ctx, q.rebind(`
		INSERT INTO outbox_events
			(tenant_id, event_type, subject_type, subject_id, payload, dedupe_key, run_after, created_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?)`),
		ev.TenantID, ev.EventType, ev.SubjectType, ev.SubjectID, string(payload), ev.DedupeKey, now, now)
	return err
}
