package jobs

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"log"
	"time"
)

// OutboxEvent is something that happened, recorded for delivery.
type OutboxEvent struct {
	ID          int64
	TenantID    string
	EventType   string
	SubjectType string
	SubjectID   int64
	Payload     any

	// DedupeKey is the event's stable identity. Two attempts to record the same
	// occurrence collapse to one row, which is what makes a handler that runs
	// twice produce one event.
	DedupeKey string

	AttemptCount int
	MaxAttempts  int
}

// RawPayload is the stored payload of a fetched event, still encoded.
type RawPayload []byte

// Decode unmarshals into v.
func (r RawPayload) Decode(v any) error { return json.Unmarshal(r, v) }

// PendingEvent is an event ready for delivery.
type PendingEvent struct {
	OutboxEvent
	Payload RawPayload
}

// Deliverer sends an event somewhere. It is an interface so delivery can be a
// broadcast to SSE subscribers, an audit sink, or a notification service without
// the dispatcher knowing which.
//
// A Deliverer must be idempotent from the consumer's point of view, because at
// least one delivery is what this design guarantees and exactly one is what no
// design guarantees across a process boundary. The dedupe key is provided so a
// consumer can recognise a repeat.
type Deliverer interface {
	Deliver(ctx context.Context, ev PendingEvent) error
}

// DelivererFunc adapts a function.
type DelivererFunc func(ctx context.Context, ev PendingEvent) error

// Deliver implements Deliverer.
func (f DelivererFunc) Deliver(ctx context.Context, ev PendingEvent) error { return f(ctx, ev) }

// Dispatcher delivers outbox events.
//
// It is separate from the workers on purpose. A worker that published its own
// events would have to do so either inside its transaction -- making an external
// call part of a database transaction, which holds locks across a network -- or
// after it, in a gap where a crash loses the event. Neither is acceptable, so
// delivery is a distinct pass over rows that are already durable.
type Dispatcher struct {
	queue     *Queue
	deliverer Deliverer
	batchSize int
	interval  time.Duration
}

// NewDispatcher builds a dispatcher.
func NewDispatcher(q *Queue, d Deliverer, interval time.Duration, batchSize int) *Dispatcher {
	if batchSize <= 0 {
		batchSize = 50
	}
	if interval <= 0 {
		interval = time.Second
	}
	return &Dispatcher{queue: q, deliverer: d, batchSize: batchSize, interval: interval}
}

// DispatchOnce delivers one batch and reports how many succeeded and failed.
//
// The ordering is the whole correctness argument: fetch, deliver, then mark.
// Marking before delivering loses every event whose delivery then fails;
// marking inside the delivery call is not possible because delivery is not
// transactional. So delivery may repeat, and consumers are told to expect that.
func (d *Dispatcher) DispatchOnce(ctx context.Context) (delivered int, failed int, err error) {
	events, err := d.pending(ctx)
	if err != nil {
		return 0, 0, err
	}

	for _, ev := range events {
		if err := ctx.Err(); err != nil {
			return delivered, failed, err
		}
		if derr := d.deliverer.Deliver(ctx, ev); derr != nil {
			failed++
			if merr := d.markFailed(ctx, ev, derr); merr != nil {
				return delivered, failed, merr
			}
			continue
		}
		if merr := d.markDelivered(ctx, ev); merr != nil {
			// The delivery happened and the mark did not. The event will be
			// delivered again, which is why consumers must tolerate a repeat.
			return delivered, failed, merr
		}
		delivered++
	}
	return delivered, failed, nil
}

func (d *Dispatcher) pending(ctx context.Context) ([]PendingEvent, error) {
	now := d.queue.now().UTC()
	rows, err := d.queue.db.QueryContext(ctx, d.queue.rebind(`
		SELECT id, tenant_id, event_type, subject_type, subject_id, payload, dedupe_key,
		       attempt_count, max_attempts
		FROM outbox_events
		WHERE delivered_at IS NULL AND dead_at IS NULL
		  AND (run_after IS NULL OR run_after <= ?)
		  AND attempt_count < max_attempts
		ORDER BY id ASC
		LIMIT ?`), now, d.batchSize)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []PendingEvent
	for rows.Next() {
		var ev PendingEvent
		var payload string
		if err := rows.Scan(&ev.ID, &ev.TenantID, &ev.EventType, &ev.SubjectType,
			&ev.SubjectID, &payload, &ev.DedupeKey, &ev.AttemptCount, &ev.MaxAttempts); err != nil {
			return nil, err
		}
		ev.Payload = RawPayload(payload)
		out = append(out, ev)
	}
	return out, rows.Err()
}

func (d *Dispatcher) markDelivered(ctx context.Context, ev PendingEvent) error {
	now := d.queue.now().UTC()
	_, err := d.queue.db.ExecContext(ctx, d.queue.rebind(`
		UPDATE outbox_events
		SET delivered_at = ?, attempt_count = attempt_count + 1
		WHERE id = ? AND delivered_at IS NULL`), now, ev.ID)
	return err
}

func (d *Dispatcher) markFailed(ctx context.Context, ev PendingEvent, cause error) error {
	now := d.queue.now().UTC()
	msg := cause.Error()
	if len(msg) > 2000 {
		msg = msg[:2000] + "…"
	}

	attempt := ev.AttemptCount + 1
	var deadAt any
	var runAfter any = now.Add(backoff(attempt, time.Second))
	if attempt >= ev.MaxAttempts {
		// An event that cannot be delivered is parked, not dropped. Dropping it
		// would lose the record that it happened; parking it keeps the row for
		// an operator and stops it consuming every dispatch pass.
		deadAt = now
		runAfter = nil
	}

	_, err := d.queue.db.ExecContext(ctx, d.queue.rebind(`
		UPDATE outbox_events
		SET attempt_count = ?, last_error = ?, run_after = ?, dead_at = ?
		WHERE id = ? AND delivered_at IS NULL`),
		attempt, msg, runAfter, deadAt, ev.ID)
	return err
}

// Run dispatches until the context is cancelled.
//
// Cancellation stops the loop between batches rather than mid-delivery, so a
// shutdown does not abandon an in-flight send whose outcome is then unknown.
func (d *Dispatcher) Run(ctx context.Context) {
	ticker := time.NewTicker(d.interval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			delivered, failed, err := d.DispatchOnce(ctx)
			if err != nil && !errors.Is(err, context.Canceled) {
				log.Printf("outbox dispatch: %v", err)
			}
			if failed > 0 {
				log.Printf("outbox: %d delivered, %d failed", delivered, failed)
			}
		}
	}
}

// UndeliveredCount reports how many events are waiting, for metrics and for
// the readiness check. A growing number means the dispatcher is behind or a
// consumer is down, and both are worth seeing.
func (d *Dispatcher) UndeliveredCount(ctx context.Context) (int, error) {
	var n int
	err := d.queue.db.QueryRowContext(ctx, d.queue.rebind(`
		SELECT COUNT(*) FROM outbox_events WHERE delivered_at IS NULL AND dead_at IS NULL`)).Scan(&n)
	return n, err
}

// DeadCount reports events that exhausted their delivery budget.
func (d *Dispatcher) DeadCount(ctx context.Context) (int, error) {
	var n int
	err := d.queue.db.QueryRowContext(ctx, d.queue.rebind(`
		SELECT COUNT(*) FROM outbox_events WHERE dead_at IS NOT NULL`)).Scan(&n)
	return n, err
}

// ensure the sql import is used by the file's own types.
var _ = sql.ErrNoRows
