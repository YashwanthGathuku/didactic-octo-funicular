package jobs

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"log"
	"sync"
	"sync/atomic"
	"time"
)

// Handler does the work of one job.
//
// It receives a transaction and must commit its business state through it, so
// the job's completion and the state it produced land together. The handler
// must not perform an external network call using this transaction's lifetime:
// holding database locks across a network is how a slow upstream becomes a
// database outage.
//
// A handler must be idempotent. At-least-once is what this system guarantees:
// a worker can crash between doing the work and recording it, and the job will
// be retried.
type Handler interface {
	Handle(ctx context.Context, tx *sql.Tx, job *Job) error
}

// HandlerFunc adapts a function.
type HandlerFunc func(ctx context.Context, tx *sql.Tx, job *Job) error

// Handle implements Handler.
func (f HandlerFunc) Handle(ctx context.Context, tx *sql.Tx, job *Job) error { return f(ctx, tx, job) }

// PoolConfig bounds the worker system.
//
// Every field is a bound. There is deliberately no "unlimited" setting: the
// contract requires bounded concurrency, and a configuration that can express
// unboundedness will eventually be set that way.
type PoolConfig struct {
	// Workers is the number of concurrent handlers. Bounded because each holds
	// a database connection and a job lease.
	Workers int

	// MaxPerTenant is the default per-tenant concurrency, overridable per
	// tenant in tenant_job_quotas. Without it one tenant's backlog occupies
	// every worker and every other tenant stops being served.
	MaxPerTenant int

	// PollInterval is how long an idle worker waits before looking again.
	PollInterval time.Duration

	// HeartbeatInterval must be comfortably shorter than the lease duration, or
	// a healthy worker's lease expires under it.
	HeartbeatInterval time.Duration

	// HandlerTimeout bounds one attempt. A handler with no timeout holds its
	// lease until the process dies.
	HandlerTimeout time.Duration

	// MaxQueueDepth is the backpressure threshold. Above it, Enqueue callers
	// receive ErrOverloaded rather than adding to a backlog nobody is draining.
	MaxQueueDepth int
}

// DefaultPoolConfig is a starting point sized for one process.
//
// The numbers are conservative rather than tuned: this repository has no
// measured throughput to tune against, and a plausible-looking figure chosen
// without measurement is the kind of claim this whole programme exists to
// remove.
func DefaultPoolConfig() PoolConfig {
	return PoolConfig{
		Workers:           4,
		MaxPerTenant:      2,
		PollInterval:      250 * time.Millisecond,
		HeartbeatInterval: 10 * time.Second,
		HandlerTimeout:    2 * time.Minute,
		MaxQueueDepth:     10_000,
	}
}

// Pool runs handlers against a queue.
type Pool struct {
	queue    *Queue
	handlers map[string]Handler
	cfg      PoolConfig

	// leasing is cleared on shutdown so workers stop claiming new work while
	// finishing what they hold. It is the difference between a graceful stop
	// and an abandoned job.
	leasing atomic.Bool

	wg sync.WaitGroup

	// Observed counters. They are reported as what happened, never as a rate.
	claimed   atomic.Int64
	succeeded atomic.Int64
	failed    atomic.Int64
	lost      atomic.Int64
	active    atomic.Int64
}

// NewPool builds a worker pool.
func NewPool(q *Queue, cfg PoolConfig) (*Pool, error) {
	if q == nil {
		return nil, errors.New("a worker pool requires a queue")
	}
	if cfg.Workers <= 0 {
		return nil, errors.New("worker count must be positive; an unbounded pool is not a pool")
	}
	if cfg.MaxPerTenant <= 0 {
		return nil, errors.New("per-tenant concurrency must be positive")
	}
	if cfg.HeartbeatInterval >= q.leaseDuration {
		return nil, fmt.Errorf(
			"heartbeat interval %s is not shorter than the lease duration %s; a healthy worker's lease would expire under it",
			cfg.HeartbeatInterval, q.leaseDuration)
	}
	p := &Pool{queue: q, handlers: map[string]Handler{}, cfg: cfg}
	p.leasing.Store(true)
	return p, nil
}

// Register attaches a handler to a job kind.
func (p *Pool) Register(kind string, h Handler) { p.handlers[kind] = h }

// EnqueueTx adds work, refusing when the queue is already too deep.
//
// Backpressure returns a typed overload error rather than accepting the work
// and spawning something to do it. An accepted job nobody will run is worse
// than a refused one: the caller believes it was scheduled.
func (p *Pool) EnqueueTx(ctx context.Context, tx *sql.Tx, req EnqueueRequest) (int64, error) {
	depth, err := p.queue.Depth(ctx)
	if err != nil {
		return 0, err
	}
	if depth >= p.cfg.MaxQueueDepth {
		return 0, fmt.Errorf("%w: %d jobs queued, limit %d", ErrOverloaded, depth, p.cfg.MaxQueueDepth)
	}
	return p.queue.EnqueueTx(ctx, tx, req)
}

// Start launches the workers. It returns immediately.
func (p *Pool) Start(ctx context.Context) {
	for i := 0; i < p.cfg.Workers; i++ {
		workerID := fmt.Sprintf("worker-%d-%d", time.Now().UnixNano()%1e6, i)
		p.wg.Add(1)
		go func() {
			defer p.wg.Done()
			p.run(ctx, workerID)
		}()
	}
}

// Stop stops leasing and waits for in-flight work to finish.
//
// The two halves are separate deliberately. Clearing `leasing` stops new claims
// immediately, so the pool drains rather than picking up work it will then
// abandon. Waiting lets held jobs complete, so their leases are released
// properly rather than left to expire -- which would delay their retry by the
// whole lease duration on the next process.
func (p *Pool) Stop(timeout time.Duration) error {
	p.leasing.Store(false)

	done := make(chan struct{})
	go func() {
		p.wg.Wait()
		close(done)
	}()

	select {
	case <-done:
		return nil
	case <-time.After(timeout):
		// The jobs still held will have their leases expire and be retried by
		// another process. Saying so is more useful than reporting a clean
		// shutdown that did not happen.
		return fmt.Errorf("shutdown timed out after %s; jobs still held will be retried when their leases expire", timeout)
	}
}

// Stats reports what this pool has observed. Counts, not rates.
type Stats struct {
	Claimed    int64
	Succeeded  int64
	Failed     int64
	LeasesLost int64
}

// Stats returns the observed counters.
func (p *Pool) Stats() Stats {
	return Stats{
		Claimed:    p.claimed.Load(),
		Succeeded:  p.succeeded.Load(),
		Failed:     p.failed.Load(),
		LeasesLost: p.lost.Load(),
	}
}

func (p *Pool) run(ctx context.Context, workerID string) {
	for {
		if ctx.Err() != nil || !p.leasing.Load() {
			return
		}

		job, err := p.claimRespectingQuotas(ctx, workerID)
		if errors.Is(err, ErrNoWork) {
			select {
			case <-ctx.Done():
				return
			case <-time.After(p.cfg.PollInterval):
			}
			continue
		}
		if err != nil {
			if !errors.Is(err, context.Canceled) {
				log.Printf("jobs: %s claim failed: %v", workerID, err)
			}
			select {
			case <-ctx.Done():
				return
			case <-time.After(p.cfg.PollInterval):
			}
			continue
		}

		p.claimed.Add(1)
		p.execute(ctx, workerID, job)
	}
}

// claimRespectingQuotas hands the pool's default limit to the queue.
//
// The quota is enforced inside the claiming transaction, not here. An earlier
// version computed the saturated set first and then claimed, which left a
// time-of-check-to-time-of-use gap wide enough for every worker to conclude
// there was room at the same moment. The pre-filter is retained only as a hint
// that saves a pass; correctness does not depend on it.
func (p *Pool) claimRespectingQuotas(ctx context.Context, workerID string) (*Job, error) {
	running, err := p.queue.RunningPerTenant(ctx)
	if err != nil {
		return nil, err
	}

	var saturated []string
	for tenant, n := range running {
		if n >= p.queue.TenantConcurrencyLimit(ctx, tenant, p.cfg.MaxPerTenant) {
			saturated = append(saturated, tenant)
		}
	}
	return p.queue.Claim(ctx, workerID, saturated, p.cfg.MaxPerTenant)
}

// ActiveWorkers returns the number of currently executing workers.
func (p *Pool) ActiveWorkers() int64 {
	return p.active.Load()
}

// Capacity returns the configured max worker count.
func (p *Pool) Capacity() int {
	return p.cfg.Workers
}

// SaturationRatio returns active workers divided by capacity (0.0 - 1.0).
func (p *Pool) SaturationRatio() float64 {
	if p.cfg.Workers == 0 {
		return 0.0
	}
	return float64(p.active.Load()) / float64(p.cfg.Workers)
}

// execute runs one attempt with a heartbeat and a timeout.
func (p *Pool) execute(ctx context.Context, workerID string, job *Job) {
	p.active.Add(1)
	defer p.active.Add(-1)

	handler, ok := p.handlers[job.Kind]
	if !ok {
		// An unregistered kind is a deployment error, not a transient one.
		// Failing it moves it toward DEAD rather than leaving it to be claimed
		// and abandoned by every worker in turn, which would block the queue.
		_ = p.queue.Fail(ctx, job, workerID, fmt.Errorf("no handler registered for job kind %q", job.Kind))
		p.failed.Add(1)
		return
	}

	runCtx, cancel := context.WithTimeout(ctx, p.cfg.HandlerTimeout)
	defer cancel()

	// The heartbeat runs alongside the handler and stops it when the lease is
	// lost, so a worker that was superseded does not finish and write a result
	// over its replacement's.
	heartbeatDone := make(chan struct{})
	go func() {
		defer close(heartbeatDone)
		ticker := time.NewTicker(p.cfg.HeartbeatInterval)
		defer ticker.Stop()
		for {
			select {
			case <-runCtx.Done():
				return
			case <-ticker.C:
				if err := p.queue.Heartbeat(runCtx, job, workerID); err != nil {
					if errors.Is(err, ErrLeaseLost) {
						p.lost.Add(1)
						cancel()
					}
					return
				}
			}
		}
	}()

	err := p.runHandler(runCtx, handler, job, workerID)
	cancel()
	<-heartbeatDone

	if err != nil {
		if errors.Is(err, ErrLeaseLost) {
			// Nothing to record: the job belongs to someone else now, and
			// writing a failure would overwrite their progress.
			p.lost.Add(1)
			return
		}
		p.failed.Add(1)
		// A failure is recorded with the parent context, not the timed-out one:
		// a handler that exceeded its timeout still needs its attempt recorded.
		if ferr := p.queue.Fail(context.WithoutCancel(ctx), job, workerID, err); ferr != nil && !errors.Is(ferr, ErrLeaseLost) {
			log.Printf("jobs: recording failure for job %d: %v", job.ID, ferr)
		}
		return
	}
	p.succeeded.Add(1)
}

// runHandler executes the handler and commits its work with the completion.
func (p *Pool) runHandler(ctx context.Context, handler Handler, job *Job, workerID string) error {
	tx, err := p.queue.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()

	if err := handler.Handle(ctx, tx, job); err != nil {
		return err
	}

	// The completion is part of the handler's transaction. If the lease was
	// lost while the handler ran, this fails and the whole transaction rolls
	// back -- so the work is discarded rather than committed under a lease this
	// worker no longer holds.
	if err := p.queue.CompleteTx(ctx, tx, job, workerID); err != nil {
		return err
	}
	return tx.Commit()
}
