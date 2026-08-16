package connectors

import (
	"context"
	"fmt"
	"sync"
	"time"
)

// Auditing the connection lifecycle, and bounding how often a connection is
// used.
//
// The secret store records its own events for the credential half of a
// connection. Until now the connection half had none: creating, testing,
// rotating and deleting one left no trail, which are exactly the events an
// operator reconstructs during an incident. "Who pointed this at a production
// replica, and when" had no answer.

// Action names a connection lifecycle event.
type Action string

const (
	ActionCreated       Action = "SOURCE_CONNECTION_CREATED"
	ActionTested        Action = "SOURCE_CONNECTION_TESTED"
	ActionSecretRotated Action = "SOURCE_CONNECTION_SECRET_ROTATED"
	ActionDeleted       Action = "SOURCE_CONNECTION_DELETED"
	ActionRateLimited   Action = "SOURCE_CONNECTION_RATE_LIMITED"
)

// AuditEvent is one lifecycle record.
//
// The payload is built here rather than by the caller, so there is one place
// that decides what a connection event may say. Every field is an identifier, a
// state or a classification -- never a credential, a host, or a driver message.
// The host is omitted deliberately: a customer's internal hostname is not a
// secret and is also not something that needs to be in an export shared with a
// third party.
type AuditEvent struct {
	TenantID     string
	Action       Action
	Actor        string
	ConnectionID int64
	Payload      map[string]any
}

// Auditor receives lifecycle events.
//
// A narrow interface rather than a dependency on internal/ledger, for the same
// reason internal/schedule keeps its Escalator narrow: what an event *means*
// beyond "record it" is the application's decision. It also keeps this package
// testable without a database.
type Auditor interface {
	RecordConnectorEvent(ctx context.Context, ev AuditEvent) error
}

// SetAuditor installs the audit sink. Nil disables recording, which is the
// state of a test that does not care -- but never of the running gateway, which
// wires one in.
func (s *Store) SetAuditor(a Auditor) { s.auditor = a }

// audit records an event, or reports why it could not.
//
// A failure to audit does not fail the operation it describes. That is a real
// trade-off and it is made deliberately: refusing to delete a connection
// because the ledger is unavailable would leave a live credential in place for
// the duration of an unrelated outage. The failure is logged loudly instead,
// and the operation's own record -- the row, or its absence -- remains true.
func (s *Store) audit(ctx context.Context, ev AuditEvent) {
	if s.auditor == nil {
		return
	}
	if err := s.auditor.RecordConnectorEvent(ctx, ev); err != nil {
		s.auditFailures.Add(1)
		if s.onAuditFailure != nil {
			s.onAuditFailure(ev, err)
		}
	}
}

// AuditFailures reports how many lifecycle events could not be recorded.
//
// Exposed so a health endpoint can surface it. An audit sink that has been
// silently failing is worse than one that was never configured, because the
// trail looks complete.
func (s *Store) AuditFailures() int64 { return s.auditFailures.Load() }

// ---------------------------------------------------------------------------
// Rate limiting
// ---------------------------------------------------------------------------

// ErrRateLimited is returned when a connection has been used too often.
var ErrRateLimited = fmt.Errorf("%w: this connection's per-minute limit has been reached", ErrLimitExceeded)

// rateLimiter bounds executions per connection per minute.
//
// Per-execution limits bound one query's rows, bytes and time. Without this, an
// unbounded number of bounded queries is still unbounded -- and the load lands
// on a customer's production database, caused by their reporting integration.
//
// A fixed window rather than a token bucket. The window's edge allows a burst
// of up to twice the limit across a boundary, which is a real imprecision and
// an acceptable one: the purpose is to stop a runaway loop, not to shape
// traffic, and a fixed window is something an operator reading the code can
// predict.
type rateLimiter struct {
	mu      sync.Mutex
	windows map[int64]*window
	now     func() time.Time
}

type window struct {
	start time.Time
	count int
}

func newRateLimiter(now func() time.Time) *rateLimiter {
	return &rateLimiter{windows: map[int64]*window{}, now: now}
}

// allow reports whether one more execution is permitted.
func (l *rateLimiter) allow(connectionID int64, limit int) bool {
	if limit <= 0 {
		return true
	}
	l.mu.Lock()
	defer l.mu.Unlock()

	now := l.now()
	w, ok := l.windows[connectionID]
	if !ok || now.Sub(w.start) >= time.Minute {
		l.windows[connectionID] = &window{start: now, count: 1}
		l.sweep(now)
		return true
	}
	if w.count >= limit {
		return false
	}
	w.count++
	return true
}

// sweep drops windows nobody is using.
//
// Without it the map grows by one entry per connection ever seen and never
// shrinks, which is a slow leak in a long-lived process -- the kind that is
// invisible until a tenant with many connections arrives.
func (l *rateLimiter) sweep(now time.Time) {
	if len(l.windows) < 256 {
		return
	}
	for id, w := range l.windows {
		if now.Sub(w.start) >= 2*time.Minute {
			delete(l.windows, id)
		}
	}
}

// checkRate enforces a connection's per-minute limit and records a refusal.
//
// The refusal is audited. A connection hitting its limit is either a
// misconfigured caller or a runaway loop, and both are things an operator finds
// out about from the trail rather than from the customer.
func (s *Store) checkRate(ctx context.Context, sc interface{ TenantID() string }, actor string, c *Connection) error {
	if s.limiter.allow(c.ID, c.MaxPerMinute) {
		return nil
	}
	s.audit(ctx, AuditEvent{
		TenantID:     c.TenantID,
		Action:       ActionRateLimited,
		Actor:        actor,
		ConnectionID: c.ID,
		Payload: map[string]any{
			"connectionId":  c.ID,
			"connectorType": c.ConnectorType,
			"maxPerMinute":  c.MaxPerMinute,
		},
	})
	return ErrRateLimited
}
