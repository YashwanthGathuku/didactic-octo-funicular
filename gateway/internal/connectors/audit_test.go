package connectors

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"sentinel-gateway/internal/secrets"

	_ "modernc.org/sqlite"
)

// recordingAuditor captures lifecycle events.
type recordingAuditor struct {
	mu     sync.Mutex
	events []AuditEvent
	fail   error
}

func (a *recordingAuditor) RecordConnectorEvent(_ context.Context, ev AuditEvent) error {
	a.mu.Lock()
	defer a.mu.Unlock()
	if a.fail != nil {
		return a.fail
	}
	a.events = append(a.events, ev)
	return nil
}

func (a *recordingAuditor) actions() []Action {
	a.mu.Lock()
	defer a.mu.Unlock()
	out := make([]Action, len(a.events))
	for i, e := range a.events {
		out[i] = e.Action
	}
	return out
}

func (a *recordingAuditor) find(action Action) (AuditEvent, bool) {
	a.mu.Lock()
	defer a.mu.Unlock()
	for _, e := range a.events {
		if e.Action == action {
			return e, true
		}
	}
	return AuditEvent{}, false
}

// A lifecycle event must carry identifiers and classifications, and nothing a
// credential or a customer's internal topology could be read out of.
func TestLifecycleEventsCarryNoCredentialOrHost(t *testing.T) {
	// secret-scan-allow: a fixture credential for audit tests; it authenticates nothing and exists so payloads can be searched for it
	const credential = "audit-fixture-credential-77de-not-a-real-password"
	const host = "internal-db-07.corp.example.invalid"

	ev := AuditEvent{
		TenantID: "TENANT-A", Action: ActionCreated, Actor: "ops@example.test", ConnectionID: 7,
		Payload: map[string]any{
			"connectionId":      int64(7),
			"connectorType":     "postgresql",
			"displayName":       "Acme reporting",
			"authMode":          "password",
			"secretsConfigured": []string{"password"},
			"tlsMode":           "verify-full",
		},
	}

	body, err := json.Marshal(ev.Payload)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(body), credential) {
		t.Error("the audit payload carries a credential")
	}
	if strings.Contains(string(body), host) {
		t.Error("the audit payload carries a hostname; an event that travels into an export " +
			"should not describe a customer's internal topology")
	}
	// The field names must be there: "which credentials were configured" is
	// what an operator reconciles against the secret store.
	if !strings.Contains(string(body), "password") {
		t.Error("the payload does not name which credential fields were configured")
	}
}

func TestEveryLifecycleOperationIsAudited(t *testing.T) {
	auditor := &recordingAuditor{}
	store, scope, _ := newAuditStore(t, auditor)
	ctx := context.Background()

	created, err := store.Create(ctx, scope, auditConnectionRequest("Acme"))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.TestConnection(ctx, scope, created.ID); err == nil {
		t.Log("the fixture host is unreachable, which is the expected outcome here")
	}
	if err := store.ReplaceSecret(ctx, scope, created.ID, "password",
		strings.Repeat("replacement-credential-", 2)); err != nil {
		t.Fatal(err)
	}
	if err := store.Delete(ctx, scope, created.ID); err != nil {
		t.Fatal(err)
	}

	got := auditor.actions()
	for _, want := range []Action{ActionCreated, ActionTested, ActionSecretRotated, ActionDeleted} {
		found := false
		for _, a := range got {
			if a == want {
				found = true
			}
		}
		if !found {
			t.Errorf("%s was not audited; got %v", want, got)
		}
	}

	// A failed check is audited as a failure, not omitted. Recording only
	// successes makes silence ambiguous between "nobody tested it" and
	// "everybody who tested it failed".
	tested, _ := auditor.find(ActionTested)
	if state, _ := tested.Payload["state"].(string); state != string(HealthFailed) {
		t.Errorf("the tested event recorded state %q; the fixture host does not resolve", state)
	}
	if _, ok := tested.Payload["errorClass"]; !ok {
		t.Error("a failed check must record its error classification")
	}

	// The delete event names which credentials were retired -- the question
	// asked after one turns up somewhere it should not be.
	deleted, _ := auditor.find(ActionDeleted)
	if _, ok := deleted.Payload["secretsRetired"]; !ok {
		t.Error("the delete event does not say which credentials were retired")
	}
}

// An audit failure must be surfaced and must not fail the operation.
func TestAnAuditFailureIsLoudAndDoesNotBlockTheOperation(t *testing.T) {
	auditor := &recordingAuditor{fail: fmt.Errorf("the ledger is unavailable")}
	store, scope, _ := newAuditStore(t, auditor)

	var failures []AuditEvent
	var mu sync.Mutex
	store.SetAuditFailureHandler(func(ev AuditEvent, _ error) {
		mu.Lock()
		defer mu.Unlock()
		failures = append(failures, ev)
	})

	created, err := store.Create(context.Background(), scope, auditConnectionRequest("Acme"))
	if err != nil {
		t.Fatalf("an unavailable ledger must not block the operation: %v", err)
	}
	if created == nil {
		t.Fatal("the connection was not created")
	}

	mu.Lock()
	n := len(failures)
	mu.Unlock()
	if n == 0 {
		t.Error("the audit failure was swallowed; a trail that is silently failing is worse " +
			"than one that was never configured, because it looks complete")
	}
	if store.AuditFailures() == 0 {
		t.Error("the failure counter did not move, so a health endpoint could not surface it")
	}

	// Deleting must also survive, for the reason the trade-off exists at all:
	// refusing would leave a live credential in place during an unrelated
	// outage.
	if err := store.Delete(context.Background(), scope, created.ID); err != nil {
		t.Errorf("an unavailable ledger blocked a delete, leaving a live credential: %v", err)
	}
}

// ---------------------------------------------------------------------------
// Rate limiting
// ---------------------------------------------------------------------------

func TestTheRateLimiterBoundsAFixedWindow(t *testing.T) {
	now := time.Date(2025, time.June, 10, 12, 0, 0, 0, time.UTC)
	l := newRateLimiter(func() time.Time { return now })

	for i := range 3 {
		if !l.allow(1, 3) {
			t.Fatalf("request %d was refused inside the limit", i+1)
		}
	}
	if l.allow(1, 3) {
		t.Error("the fourth request inside one minute must be refused")
	}

	// A different connection has its own window.
	if !l.allow(2, 3) {
		t.Error("one connection's limit must not bound another's")
	}

	// The window rolls.
	now = now.Add(61 * time.Second)
	if !l.allow(1, 3) {
		t.Error("the window did not roll after a minute")
	}
}

func TestAZeroLimitIsUnbounded(t *testing.T) {
	l := newRateLimiter(time.Now)
	for range 100 {
		if !l.allow(1, 0) {
			t.Fatal("a zero limit must mean no limit, not no requests")
		}
	}
}

// The window map must not grow without bound in a long-lived process.
func TestTheRateLimiterSweepsStaleWindows(t *testing.T) {
	now := time.Date(2025, time.June, 10, 12, 0, 0, 0, time.UTC)
	l := newRateLimiter(func() time.Time { return now })

	for id := range int64(400) {
		l.allow(id, 10)
	}
	before := len(l.windows)

	// Three minutes later, a new connection triggers the sweep.
	now = now.Add(3 * time.Minute)
	l.allow(9999, 10)

	if len(l.windows) >= before {
		t.Errorf("windows went from %d to %d; stale entries must be swept or the map is a "+
			"slow leak", before, len(l.windows))
	}
}

func TestExceedingTheLimitIsRefusedAndAudited(t *testing.T) {
	auditor := &recordingAuditor{}
	store, scope, _ := newAuditStore(t, auditor)
	ctx := context.Background()

	req := auditConnectionRequest("Busy")
	req.MaxPerMinute = 2
	created, err := store.Create(ctx, scope, req)
	if err != nil {
		t.Fatal(err)
	}

	// Two checks are permitted; the third is not. Each is a real connection
	// attempt to a customer database, so the limit is what stops a loop.
	for i := range 2 {
		if _, err := store.TestConnection(ctx, scope, created.ID); err == ErrRateLimited {
			t.Fatalf("check %d was rate limited inside the limit", i+1)
		}
	}
	if _, err := store.TestConnection(ctx, scope, created.ID); err != ErrRateLimited {
		t.Fatalf("the third check was not rate limited: %v", err)
	}

	ev, ok := auditor.find(ActionRateLimited)
	if !ok {
		t.Fatal("a rate-limited connection must be audited; it is a misconfigured caller or a " +
			"runaway loop, and an operator finds out from the trail")
	}
	if ev.ConnectionID != created.ID {
		t.Errorf("the rate-limit event names connection %d, want %d", ev.ConnectionID, created.ID)
	}
}

// ---------------------------------------------------------------------------
// Fixtures
// ---------------------------------------------------------------------------

// newAuditStore builds a connection store on a real SQLite database with the
// shipped migrations, a real secret store, and a registry whose PostgreSQL
// entry has been promoted by a complete conformance record.
//
// The promotion goes through Register with a record rather than a back door,
// so these tests exercise the same gate everything else does.
func newAuditStore(t *testing.T, auditor Auditor) (*Store, secrets.Scope, *Registry) {
	t.Helper()

	path := filepath.Join(t.TempDir(), "connections.db")
	db, err := sql.Open("sqlite", path+"?_pragma=busy_timeout(10000)&_pragma=journal_mode(WAL)&_pragma=foreign_keys(1)")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })

	for _, name := range []string{
		"001_init_schema.sql", "002_tenancy_and_state.sql", "003_secret_store.sql",
		"004_artifact_storage.sql", "005_redacted_findings.sql", "006_jobs_and_outbox.sql",
		"007_ledger_integrity.sql", "008_scheduling.sql", "009_breach_escalation.sql",
		"010_source_connections.sql",
	} {
		body, err := os.ReadFile(filepath.Join("..", "..", "migrations", name))
		if err != nil {
			t.Fatalf("read %s: %v", name, err)
		}
		if _, err := db.Exec(string(body)); err != nil {
			t.Fatalf("apply %s: %v", name, err)
		}
	}
	if _, err := db.Exec(
		`INSERT INTO tenants (id, name) VALUES ('TENANT-AUDIT','Audit') ON CONFLICT (id) DO NOTHING`); err != nil {
		t.Fatal(err)
	}

	sealer, err := secrets.NewEphemeralSealer()
	if err != nil {
		t.Fatal(err)
	}
	secretStore, err := secrets.NewSQLStore(db, sealer)
	if err != nil {
		t.Fatal(err)
	}

	registry := NewRegistry()
	driver := NewPostgresConnector()
	t.Cleanup(func() { driver.Close() })
	if err := registry.Register(driver, &ConformanceRecord{
		ConnectorType: "postgresql",
		ServerVersion: "PostgreSQL 16 (test fixture)",
		DriverVersion: PostgresDriverVersion,
		TestCommit:    "fixture",
		RunAt:         time.Now().UTC(),
		Passed:        21,
	}); err != nil {
		t.Fatal(err)
	}

	store, err := NewStore(db, "sqlite", registry, secretStore)
	if err != nil {
		t.Fatal(err)
	}
	store.SetAuditor(auditor)

	scope, err := secrets.SystemScope("TENANT-AUDIT", "audit-test")
	if err != nil {
		t.Fatal(err)
	}
	return store, scope, registry
}

// auditConnectionRequest names a host that does not resolve, so a check fails
// quickly and deterministically without reaching anything real.
func auditConnectionRequest(name string) CreateRequest {
	return CreateRequest{
		DisplayName:   name,
		ConnectorType: "postgresql",
		AuthMode:      "password",
		Fields: map[string]string{
			"host": "no-such-host.invalid", "port": "5432", "database": "ledger",
			"username": "svc_reporting", "tls_mode": "verify-full",
		},
		Secrets:   map[string]string{"password": "audit-fixture-credential-77de-not-a-real-password"},
		Allowlist: []string{"reporting"},
	}
}
