package main

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"

	"sentinel-gateway/internal/auth"
)

// Server-Sent Events, read from the transactional outbox.
//
// What this replaces: an in-process broadcaster with a map of channels, no
// tenant scoping, no cursor and no persistence. Nothing ever published to it,
// so the only thing a subscriber received was a literal
// {"status":"CONNECTED","stream":"SENTINEL_REALTIME_BUS"} -- a live-looking
// connection carrying no events, which is the same class of defect as a health
// check that returns "healthy" without checking anything. Had anything
// published to it, every subscriber would have received every tenant's events,
// because the fan-out had no notion of who was listening.
//
// The outbox is the right backing store and it already exists. Its rows are
// written in the same transaction as the state they describe, its content is
// immutable by trigger, and its id is monotonic -- which is exactly a
// last-event cursor. So this endpoint is a *reader* of that log. It marks
// nothing delivered and competes with no dispatcher; two readers of an
// append-only log do not interfere.

const (
	// streamPollInterval is how often the reader looks for new rows.
	//
	// Polling rather than a notification channel because the store is SQLite
	// here and PostgreSQL elsewhere, and a LISTEN/NOTIFY path that only worked
	// on one of them would mean the UI behaved differently depending on the
	// deployment. A second of latency on an operations screen is not the
	// constraint worth adding a second mechanism for.
	streamPollInterval = 1 * time.Second

	// streamKeepAlive bounds the silence on the wire.
	//
	// Without it an idle connection is indistinguishable from a dead one to
	// every proxy between here and the browser, and they close it at intervals
	// nobody controls. The comment line also lets the client tell "connected
	// and quiet" from "connected and broken", which are different things to
	// show an operator.
	streamKeepAlive = 20 * time.Second

	// streamBatch caps how many events one poll emits, so a client that
	// reconnects after a long absence replays in bounded chunks instead of
	// serialising a tenant's whole history into one write.
	streamBatch = 200

	// streamMaxDuration forces a reconnect. A connection held open forever is
	// a resource whose lifetime nothing bounds; a client that reconnects with
	// its cursor loses nothing, which is the property the cursor exists for.
	streamMaxDuration = 30 * time.Minute
)

// streamEvent is one row of the outbox as the browser sees it.
type streamEvent struct {
	ID          int64           `json:"id"`
	EventType   string          `json:"eventType"`
	SubjectType string          `json:"subjectType"`
	SubjectID   int64           `json:"subjectId"`
	Payload     json.RawMessage `json:"payload"`
	CreatedAt   string          `json:"createdAt"`
}

// RegisterStreamRoutes wires the event stream.
func RegisterStreamRoutes(r chi.Router, db *sql.DB) {
	r.Get("/stream", streamEvents(db))
}

func streamEvents(db *sql.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		// Authorized like every other read. The previous endpoint sat inside
		// the authenticated router and then scoped nothing, so authentication
		// gated the connection and authorization gated none of its content.
		scope, serr := resolveScope(r, auth.PermReadTenant)
		if serr != nil {
			serr.write(w)
			return
		}
		flusher, ok := w.(http.Flusher)
		if !ok {
			writeJSON(w, http.StatusInternalServerError,
				map[string]any{"error": "streaming_unsupported"})
			return
		}

		cursor, cursorGiven, err := streamCursor(r)
		if err != nil {
			writeJSON(w, http.StatusBadRequest,
				map[string]any{"error": "bad_cursor", "detail": err.Error()})
			return
		}

		// Establish the starting point before any header is written, so a
		// failure here is still an HTTP error rather than an error delivered
		// inside a 200 stream.
		head, err := streamHead(r.Context(), db, scope.TenantID())
		if err != nil {
			log.Printf("stream: head read failed for tenant %s: %v", scope.TenantID(), err)
			writeJSON(w, http.StatusServiceUnavailable, map[string]any{"error": "stream_unavailable"})
			return
		}

		// A cursor from before the retained window cannot be honoured. Saying
		// so is the point: silently restarting at the head would hand the
		// client a stream with a hole in it that it had no way to detect, and
		// a UI built on that stream would show a state it never received the
		// transitions for.
		gap := false
		if cursorGiven {
			oldest, err := streamOldest(r.Context(), db, scope.TenantID())
			if err == nil && oldest > 0 && cursor < oldest-1 {
				gap = true
			}
		} else {
			// No cursor: start at the head rather than replaying history. A
			// first connection wants what happens next; a reconnecting one
			// says where it got to.
			cursor = head
		}

		w.Header().Set("Content-Type", "text/event-stream")
		w.Header().Set("Cache-Control", "no-store")
		w.Header().Set("Connection", "keep-alive")
		// Proxy buffering turns an event stream into a batch delivery that
		// arrives when the connection closes.
		w.Header().Set("X-Accel-Buffering", "no")
		w.WriteHeader(http.StatusOK)

		hello, _ := json.Marshal(map[string]any{
			"cursor":    cursor,
			"head":      head,
			"tenantId":  scope.TenantID(),
			"replay":    cursorGiven,
			"gap":       gap,
			"serverNow": time.Now().UTC().Format(time.RFC3339),
		})
		fmt.Fprintf(w, "event: hello\ndata: %s\n\n", hello)
		flusher.Flush()

		ticker := time.NewTicker(streamPollInterval)
		defer ticker.Stop()
		deadline := time.NewTimer(streamMaxDuration)
		defer deadline.Stop()
		lastWrite := time.Now()

		for {
			select {
			case <-r.Context().Done():
				return
			case <-deadline.C:
				// Told, not dropped. A client that is informed reconnects with
				// its cursor; one that is silently disconnected has to guess
				// whether it missed anything.
				fmt.Fprintf(w, "event: reconnect\ndata: %s\n\n",
					`{"reason":"connection lifetime reached; reconnect with your last event id"}`)
				flusher.Flush()
				return
			case <-ticker.C:
				events, err := streamSince(r.Context(), db, scope.TenantID(), cursor, streamBatch)
				if err != nil {
					// A transient read failure is reported on the stream and
					// the connection is kept. The cursor is not advanced, so
					// nothing is skipped by the failure.
					log.Printf("stream: read failed for tenant %s: %v", scope.TenantID(), err)
					fmt.Fprintf(w, "event: degraded\ndata: %s\n\n",
						`{"reason":"event log temporarily unreadable; no events were skipped"}`)
					flusher.Flush()
					lastWrite = time.Now()
					continue
				}
				for _, ev := range events {
					data, err := json.Marshal(ev)
					if err != nil {
						continue
					}
					// The SSE id is the outbox id. That is what the browser
					// sends back as Last-Event-ID, which is why the cursor and
					// the id must be the same number and not two encodings of
					// it.
					fmt.Fprintf(w, "id: %d\nevent: %s\ndata: %s\n\n",
						ev.ID, sseEventName(ev.EventType), data)
					// Advanced per event, after it is written. Advancing the
					// cursor for the whole batch before writing would skip the
					// tail of a batch whose write failed halfway.
					cursor = ev.ID
				}
				if len(events) > 0 {
					flusher.Flush()
					lastWrite = time.Now()
					continue
				}
				if time.Since(lastWrite) >= streamKeepAlive {
					// A comment line: valid SSE, dispatched to no handler.
					fmt.Fprintf(w, ": keepalive %d\n\n", time.Now().Unix())
					flusher.Flush()
					lastWrite = time.Now()
				}
			}
		}
	}
}

// streamCursor reads the resume position.
//
// Last-Event-ID is what a browser's EventSource resends by itself on
// reconnect, and it is preferred for exactly that reason: the resume is
// correct without the client having to remember anything. The query parameter
// exists because EventSource cannot set a header on the first connection, so a
// client resuming a session it stored itself has no other way to say where it
// got to.
func streamCursor(r *http.Request) (int64, bool, error) {
	raw := strings.TrimSpace(r.Header.Get("Last-Event-ID"))
	if raw == "" {
		raw = strings.TrimSpace(r.URL.Query().Get("cursor"))
	}
	if raw == "" {
		return 0, false, nil
	}
	if len(raw) > 20 {
		return 0, false, fmt.Errorf("cursor is not an event id")
	}
	n, err := strconv.ParseInt(raw, 10, 64)
	if err != nil || n < 0 {
		return 0, false, fmt.Errorf("cursor is not an event id")
	}
	return n, true, nil
}

func streamHead(ctx context.Context, db *sql.DB, tenantID string) (int64, error) {
	var head sql.NullInt64
	err := db.QueryRowContext(ctx,
		`SELECT MAX(id) FROM outbox_events WHERE tenant_id = ?`, tenantID).Scan(&head)
	if err != nil {
		return 0, err
	}
	return head.Int64, nil
}

func streamOldest(ctx context.Context, db *sql.DB, tenantID string) (int64, error) {
	var oldest sql.NullInt64
	err := db.QueryRowContext(ctx,
		`SELECT MIN(id) FROM outbox_events WHERE tenant_id = ?`, tenantID).Scan(&oldest)
	if err != nil {
		return 0, err
	}
	return oldest.Int64, nil
}

// streamSince reads events strictly after the cursor.
//
// Strictly after, and ordered by the same key: that is the whole duplicate
// suppression. There is no de-duplication set, no seen-ids cache and nothing
// to get out of step with the log, because the log's own ordering is the
// guarantee.
func streamSince(ctx context.Context, db *sql.DB, tenantID string, after int64, limit int) ([]streamEvent, error) {
	rows, err := db.QueryContext(ctx, `
		SELECT id, event_type, subject_type, subject_id, payload, created_at
		FROM outbox_events
		WHERE tenant_id = ? AND id > ?
		ORDER BY id ASC
		LIMIT ?`, tenantID, after, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []streamEvent
	for rows.Next() {
		var (
			ev      streamEvent
			payload string
			created time.Time
		)
		if err := rows.Scan(&ev.ID, &ev.EventType, &ev.SubjectType, &ev.SubjectID,
			&payload, &created); err != nil {
			return nil, err
		}
		// The payload is already redacted where it was produced -- findings
		// pass through internal/nacha before they reach the outbox -- so there
		// is nothing to strip here, and stripping here would be the wrong
		// place for it anyway.
		if json.Valid([]byte(payload)) {
			ev.Payload = json.RawMessage(payload)
		} else {
			ev.Payload = json.RawMessage(`null`)
		}
		ev.CreatedAt = created.UTC().Format(time.RFC3339)
		out = append(out, ev)
	}
	return out, rows.Err()
}

// sseEventName makes an event type safe to put in an SSE field.
//
// An event name containing a newline would end the field and let the rest of
// the value be read as another field -- header injection, in a protocol that
// is nothing but headers. Event types are written by this codebase, so this is
// defence against a future one rather than against today's, which is when it
// is cheap to add.
func sseEventName(t string) string {
	t = strings.Map(func(r rune) rune {
		if r == '\n' || r == '\r' || r < 0x20 {
			return -1
		}
		return r
	}, t)
	if t == "" {
		return "event"
	}
	if len(t) > 128 {
		return t[:128]
	}
	return t
}
