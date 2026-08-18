package main

import (
	"bufio"
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"
)

// The event stream's contract: a reconnecting client resumes exactly where it
// stopped, receives everything it missed, and receives nothing twice.
//
// The old endpoint could not be tested for any of this. It had no cursor, no
// persistence and nothing that published to it, so the only assertion available
// was that it emitted a hardcoded "CONNECTED" frame.

// sseFrame is one parsed event from the wire.
type sseFrame struct {
	ID    string
	Event string
	Data  string
}

// readSSE consumes frames until the reader is done or n frames have been seen.
//
// Written against the wire format rather than a client library on purpose: the
// id/event/data field discipline is the part a browser's EventSource depends
// on, so it is the part worth asserting.
func readSSE(t *testing.T, body *strings.Reader, want int) []sseFrame {
	t.Helper()
	var frames []sseFrame
	sc := bufio.NewScanner(body)
	cur := sseFrame{}
	for sc.Scan() {
		line := sc.Text()
		switch {
		case line == "":
			if cur.Event != "" || cur.Data != "" {
				frames = append(frames, cur)
				cur = sseFrame{}
			}
			if want > 0 && len(frames) >= want {
				return frames
			}
		case strings.HasPrefix(line, ":"):
			// keepalive comment
		case strings.HasPrefix(line, "id: "):
			cur.ID = strings.TrimPrefix(line, "id: ")
		case strings.HasPrefix(line, "event: "):
			cur.Event = strings.TrimPrefix(line, "event: ")
		case strings.HasPrefix(line, "data: "):
			cur.Data = strings.TrimPrefix(line, "data: ")
		}
	}
	return frames
}

// publishOutbox writes events directly, standing in for whatever produced them.
func publishOutbox(t *testing.T, db *sql.DB, tenantID string, n int, prefix string) {
	t.Helper()
	for i := range n {
		_, err := db.Exec(`
			INSERT INTO outbox_events
				(tenant_id, event_type, subject_type, subject_id, payload, dedupe_key, created_at)
			VALUES (?, ?, 'artifact', ?, ?, ?, ?)`,
			tenantID, "TEST_EVENT", int64(i+1),
			fmt.Sprintf(`{"n":%d}`, i+1), prefix+"-"+strconv.Itoa(i+1), time.Now().UTC())
		if err != nil {
			t.Fatal(err)
		}
	}
}

// syncRecorder is an http.ResponseWriter safe to read while the handler writes.
//
// httptest.ResponseRecorder is not. These tests read the accumulating body from
// the test goroutine to decide when enough frames have arrived, while the
// handler goroutine is still writing to it -- and the race detector is right to
// object: bytes.Buffer.String reads the slice header that Buffer.grow rewrites.
//
// A streaming handler cannot be tested with a recorder that only becomes
// readable when the request finishes, because the request finishing is the one
// thing that never happens on a stream. So the recorder is the thing that has
// to change.
type syncRecorder struct {
	mu     sync.Mutex
	buf    bytes.Buffer
	header http.Header
	code   int
}

func newSyncRecorder() *syncRecorder {
	return &syncRecorder{header: make(http.Header), code: http.StatusOK}
}

func (r *syncRecorder) Header() http.Header { return r.header }

func (r *syncRecorder) Write(p []byte) (int, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.buf.Write(p)
}

func (r *syncRecorder) WriteHeader(code int) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.code = code
}

// Flush satisfies http.Flusher. Without it the handler refuses the request
// outright, so its presence is part of what these tests exercise.
func (r *syncRecorder) Flush() {}

func (r *syncRecorder) body() string {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.buf.String()
}

func (r *syncRecorder) status() int {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.code
}

// collectStream runs one stream request until it has seen `want` data frames or
// the timeout elapses, then cancels.
func collectStream(t *testing.T, handler http.Handler, cursor string, want int) []sseFrame {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 6*time.Second)
	defer cancel()

	req := httptest.NewRequest(http.MethodGet, "/api/v1/stream", nil).WithContext(ctx)
	if cursor != "" {
		req.Header.Set("Last-Event-ID", cursor)
	}
	rec := newSyncRecorder()

	done := make(chan struct{})
	go func() {
		handler.ServeHTTP(rec, req)
		close(done)
	}()

	// The handler polls once a second; give it room to emit, then cancel so
	// ServeHTTP returns and the body is complete.
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		time.Sleep(250 * time.Millisecond)
		if countData(rec.body()) >= want {
			break
		}
	}
	cancel()
	<-done

	body := rec.body()
	if rec.status() != http.StatusOK {
		t.Fatalf("stream: %d %s", rec.status(), body)
	}
	return readSSE(t, strings.NewReader(body), 0)
}

func countData(body string) int {
	n := 0
	for _, line := range strings.Split(body, "\n") {
		if strings.HasPrefix(line, "event: TEST_EVENT") {
			n++
		}
	}
	return n
}

func TestStreamReplaysFromTheCursorWithoutDuplicates(t *testing.T) {
	db, handler, _ := newIngestTestEnv(t)

	// The tenant the demo principal resolves to.
	_, probe := doUpload(t, handler, uploadRequest(t, "probe.ach", []byte("stream-probe"), nil))
	var tenantID string
	if err := db.QueryRow(`SELECT tenant_id FROM file_instances WHERE id = ?`,
		probe.ArtifactID).Scan(&tenantID); err != nil {
		t.Fatal(err)
	}

	// Five events exist before anybody connects.
	publishOutbox(t, db, tenantID, 5, "first")

	// A client that has already seen event 2 asks to resume from there.
	frames := collectStream(t, handler, "2", 3)

	var got []int64
	var sawHello bool
	for _, f := range frames {
		if f.Event == "hello" {
			sawHello = true
			continue
		}
		if f.Event != "TEST_EVENT" {
			continue
		}
		id, err := strconv.ParseInt(f.ID, 10, 64)
		if err != nil {
			t.Fatalf("event carried no usable id: %+v", f)
		}
		got = append(got, id)
	}
	if !sawHello {
		t.Fatal("no hello frame; a client cannot tell it is connected")
	}
	if len(got) < 3 {
		t.Fatalf("replayed %d events from cursor 2, want at least 3: %+v", len(got), frames)
	}

	// Nothing at or before the cursor, nothing twice, ids ascending.
	seen := map[int64]bool{}
	var last int64
	for _, id := range got {
		if id <= 2 {
			t.Errorf("event %d was replayed although the client said it had seen 2", id)
		}
		if seen[id] {
			t.Errorf("event %d was delivered twice", id)
		}
		if id <= last {
			t.Errorf("event ids went backwards: %d after %d", id, last)
		}
		seen[id] = true
		last = id
	}
	t.Logf("resumed from 2 and received %v exactly once each", got)
}

func TestStreamWithoutACursorStartsAtTheHead(t *testing.T) {
	db, handler, _ := newIngestTestEnv(t)
	_, probe := doUpload(t, handler, uploadRequest(t, "probe.ach", []byte("head-probe"), nil))
	var tenantID string
	if err := db.QueryRow(`SELECT tenant_id FROM file_instances WHERE id = ?`,
		probe.ArtifactID).Scan(&tenantID); err != nil {
		t.Fatal(err)
	}
	publishOutbox(t, db, tenantID, 3, "history")

	// No Last-Event-ID: a first connection wants what happens next, not a
	// replay of everything the tenant has ever done.
	frames := collectStream(t, handler, "", 0)
	for _, f := range frames {
		if f.Event == "TEST_EVENT" {
			t.Errorf("a first connection replayed history: %+v", f)
		}
	}

	var hello struct {
		Cursor int64 `json:"cursor"`
		Head   int64 `json:"head"`
		Replay bool  `json:"replay"`
	}
	for _, f := range frames {
		if f.Event == "hello" {
			if err := json.Unmarshal([]byte(f.Data), &hello); err != nil {
				t.Fatal(err)
			}
		}
	}
	if hello.Replay {
		t.Error("hello claimed a replay when no cursor was sent")
	}
	if hello.Cursor != hello.Head || hello.Head == 0 {
		t.Errorf("cursor %d, head %d: a first connection must start at the head",
			hello.Cursor, hello.Head)
	}
}

func TestStreamRefusesAnUnauthenticatedCaller(t *testing.T) {
	db := setupTestDb(t)
	t.Cleanup(func() { db.Close() })
	handler := NewRouter(db, &Config{Profile: ProfileProduction}, nil)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/stream", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code == http.StatusOK {
		t.Fatalf("the event stream served an unauthenticated caller: %d", rec.Code)
	}
	if strings.Contains(rec.Header().Get("Content-Type"), "text/event-stream") {
		t.Error("the refusal was delivered as a stream; a client would read it as connected")
	}
}

func TestStreamRefusesAMalformedCursor(t *testing.T) {
	_, handler, _ := newIngestTestEnv(t)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/stream", nil)
	req.Header.Set("Last-Event-ID", "'; DROP TABLE outbox_events; --")
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("malformed cursor: %d %s (want 400)", rec.Code, rec.Body.String())
	}
}

// The property the in-memory broadcaster could not have had: an event belongs
// to one tenant and reaches only that tenant's readers.
//
// The old fan-out held a set of channels and wrote every event to all of them.
// Nothing published to it, so the disclosure never happened -- but the first
// publisher to be wired up would have caused it, silently, on a code path that
// looked like it was working.
func TestStreamReadsAreTenantScoped(t *testing.T) {
	db, handler, _ := newIngestTestEnv(t)
	_, probe := doUpload(t, handler, uploadRequest(t, "probe.ach", []byte("scope-probe"), nil))
	var mine string
	if err := db.QueryRow(`SELECT tenant_id FROM file_instances WHERE id = ?`,
		probe.ArtifactID).Scan(&mine); err != nil {
		t.Fatal(err)
	}

	other := "TENANT-OTHER-STREAM"
	if _, err := db.Exec(
		`INSERT INTO tenants (id, name) VALUES (?, ?)`, other, "Other"); err != nil {
		t.Fatal(err)
	}
	publishOutbox(t, db, mine, 2, "mine")
	publishOutbox(t, db, other, 2, "theirs")

	events, err := streamSince(context.Background(), db, mine, 0, 100)
	if err != nil {
		t.Fatal(err)
	}
	if len(events) != 2 {
		t.Fatalf("read %d events for tenant %s, want its own 2", len(events), mine)
	}
	for _, ev := range events {
		var tenantID string
		if err := db.QueryRow(`SELECT tenant_id FROM outbox_events WHERE id = ?`,
			ev.ID).Scan(&tenantID); err != nil {
			t.Fatal(err)
		}
		if tenantID != mine {
			t.Errorf("event %d belongs to %s and was read for %s", ev.ID, tenantID, mine)
		}
	}
}
