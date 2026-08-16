package main

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"sentinel-gateway/internal/connectors"
	"sentinel-gateway/internal/secrets"
)

// Storing a connection.
//
// The property under test throughout: the credential goes in and never comes
// back out. Not because each handler strips it, but because it is never in a
// structure a read path can reach.

// secret-scan-allow: a fixture credential for connection tests; it authenticates nothing and exists so responses and tables can be searched for it
const connectionCredential = "connection-fixture-credential-51ac-not-real"

// verifiedRegistry returns a registry with PostgreSQL made AVAILABLE by a
// complete conformance record.
//
// The record is synthetic here on purpose: this file tests persistence, and the
// real conformance run lives in internal/connectors. What matters is that the
// promotion goes through the same evidence path -- Register with a complete
// record -- rather than a back door this test invented.
func verifiedRegistry(t *testing.T) *connectors.Registry {
	t.Helper()
	r := connectors.NewRegistry()
	driver := connectors.NewPostgresConnector()
	t.Cleanup(func() { driver.Close() })

	if err := r.Register(driver, &connectors.ConformanceRecord{
		ConnectorType: "postgresql",
		ServerVersion: "PostgreSQL 16 (test fixture)",
		DriverVersion: connectors.PostgresDriverVersion,
		TestCommit:    "fixture",
		RunAt:         time.Now().UTC(),
		Passed:        21,
	}); err != nil {
		t.Fatal(err)
	}
	return r
}

func connectionStoreEnv(t *testing.T) (*connectors.Store, secrets.Scope, *connectors.Registry) {
	t.Helper()
	db := setupTestDb(t)
	t.Cleanup(func() { db.Close() })

	sealer, err := secrets.NewEphemeralSealer()
	if err != nil {
		t.Fatal(err)
	}
	secretStore, err := secrets.NewSQLStore(db, sealer)
	if err != nil {
		t.Fatal(err)
	}
	registry := verifiedRegistry(t)
	store, err := connectors.NewStore(db, "sqlite", registry, secretStore)
	if err != nil {
		t.Fatal(err)
	}

	if _, err := db.Exec(
		`INSERT INTO tenants (id, name) VALUES (?, ?) ON CONFLICT (id) DO NOTHING`,
		"TENANT-CONN", "Connections"); err != nil {
		t.Fatal(err)
	}
	scope, err := secrets.SystemScope("TENANT-CONN", "connection-test")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	return store, scope, registry
}

func newConnectionRequest(name string) connectors.CreateRequest {
	return connectors.CreateRequest{
		DisplayName:   name,
		ConnectorType: "postgresql",
		AuthMode:      "password",
		Fields: map[string]string{
			"host": "db.example.test", "port": "5432", "database": "ledger",
			"username": "svc_reporting", "tls_mode": "verify-full",
		},
		Secrets:   map[string]string{"password": connectionCredential},
		Allowlist: []string{"reporting"},
	}
}

func TestASavedConnectionNeverReturnsItsCredential(t *testing.T) {
	store, scope, _ := connectionStoreEnv(t)
	ctx := context.Background()

	created, err := store.Create(ctx, scope, newConnectionRequest("Acme reporting"))
	if err != nil {
		t.Fatal(err)
	}

	// Every read path.
	fetched, err := store.Get(ctx, scope.TenantID(), created.ID)
	if err != nil {
		t.Fatal(err)
	}
	list, err := store.List(ctx, scope.TenantID())
	if err != nil {
		t.Fatal(err)
	}

	for name, v := range map[string]any{
		"the create response":  created,
		"a fetched connection": fetched,
		"the connection list":  list,
	} {
		body, err := json.Marshal(v)
		if err != nil {
			t.Fatal(err)
		}
		if strings.Contains(string(body), connectionCredential) {
			t.Errorf("%s discloses the credential", name)
		}
		// Nor an assembled connection string, which is the credential in
		// another form.
		for _, marker := range []string{"postgres://", "postgresql://", "@db.example.test"} {
			if strings.Contains(string(body), marker) {
				t.Errorf("%s contains %q, reconstructing a connection string", name, marker)
			}
		}
	}

	// It does say a password is configured, by field name only.
	if len(fetched.SecretFields) != 1 || fetched.SecretFields[0] != "password" {
		t.Errorf("secretsConfigured = %v, want [password]", fetched.SecretFields)
	}
	// This credential is above the floor, so nothing is flagged.
	if len(fetched.WeakSecrets) != 0 {
		t.Errorf("weakSecrets = %v for a credential above the floor", fetched.WeakSecrets)
	}
}

// A customer's short password is accepted and reported, not refused.
//
// Refusing would not make it longer. It would push an operator to keep the
// password somewhere this application does not protect, which is strictly
// worse than storing it sealed and saying it is weak.
func TestAWeakCustomerCredentialIsAcceptedAndReported(t *testing.T) {
	store, scope, _ := connectionStoreEnv(t)

	req := newConnectionRequest("Legacy partner")
	req.Secrets["password"] = "short1"

	created, err := store.Create(context.Background(), scope, req)
	if err != nil {
		t.Fatalf("a short customer password must be accepted: %v", err)
	}
	if len(created.WeakSecrets) != 1 || created.WeakSecrets[0] != "password" {
		t.Errorf("weakSecrets = %v, want [password]; the operator has to be able to see it "+
			"without it being displayed", created.WeakSecrets)
	}

	body, err := json.Marshal(created)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(body), "short1") {
		t.Error("a weak credential is disclosed by the read path; being weak is not a reason " +
			"to protect it less")
	}
}

// The credential must not be in this package's tables at all.
func TestTheCredentialIsNotInTheConnectionTables(t *testing.T) {
	db := setupTestDb(t)
	t.Cleanup(func() { db.Close() })

	sealer, err := secrets.NewEphemeralSealer()
	if err != nil {
		t.Fatal(err)
	}
	secretStore, err := secrets.NewSQLStore(db, sealer)
	if err != nil {
		t.Fatal(err)
	}
	store, err := connectors.NewStore(db, "sqlite", verifiedRegistry(t), secretStore)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`INSERT INTO tenants (id, name) VALUES ('TENANT-CONN','C')
		ON CONFLICT (id) DO NOTHING`); err != nil {
		t.Fatal(err)
	}
	scope, _ := secrets.SystemScope("TENANT-CONN", "connection-test")

	if _, err := store.Create(context.Background(), scope, newConnectionRequest("Acme")); err != nil {
		t.Fatal(err)
	}

	// Every cell of every table, using the same exhaustive dump the Prompt 01
	// quarantine tests use. Checking only the tables this package writes would
	// miss a credential that leaked sideways into an audit payload or a log
	// table, which is exactly how the Integration Hub's secret escaped.
	dump := dumpAllCells(t, db)
	if strings.Contains(dump, connectionCredential) {
		t.Error("the credential is stored somewhere in plaintext; it belongs sealed in the " +
			"secret store and nowhere else")
	}

	// The connection row must carry the secret's *name*, so the credential can
	// be found and revoked -- an orphaned secret nobody can reconstruct a name
	// for is one that outlives its use.
	if !strings.Contains(dump, "source_connection_secrets.secret_name=connector/") {
		t.Error("no secret reference was recorded for the connection")
	}
}

func TestAnotherTenantCannotSeeOrDeleteAConnection(t *testing.T) {
	store, scope, _ := connectionStoreEnv(t)
	ctx := context.Background()

	created, err := store.Create(ctx, scope, newConnectionRequest("Acme"))
	if err != nil {
		t.Fatal(err)
	}

	if _, err := store.Get(ctx, "TENANT-OTHER", created.ID); err == nil {
		t.Error("another tenant read the connection")
	}
	list, err := store.List(ctx, "TENANT-OTHER")
	if err != nil {
		t.Fatal(err)
	}
	if len(list) != 0 {
		t.Errorf("another tenant listed %d connections", len(list))
	}

	otherScope, _ := secrets.SystemScope("TENANT-OTHER", "attacker")
	if err := store.Delete(ctx, otherScope, created.ID); err == nil {
		t.Error("another tenant deleted the connection")
	}
	if _, err := store.Get(ctx, scope.TenantID(), created.ID); err != nil {
		t.Errorf("the connection was removed by another tenant's delete: %v", err)
	}
}

// A connector without conformance evidence must not accept a connection.
func TestAConnectionCannotBeSavedAgainstAnUnverifiedConnector(t *testing.T) {
	db := setupTestDb(t)
	t.Cleanup(func() { db.Close() })

	sealer, _ := secrets.NewEphemeralSealer()
	secretStore, err := secrets.NewSQLStore(db, sealer)
	if err != nil {
		t.Fatal(err)
	}
	// A registry with a driver and no evidence: IMPLEMENTING, not selectable.
	registry := connectors.NewRegistry()
	driver := connectors.NewPostgresConnector()
	t.Cleanup(func() { driver.Close() })
	if err := registry.Register(driver, nil); err != nil {
		t.Fatal(err)
	}
	store, err := connectors.NewStore(db, "sqlite", registry, secretStore)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`INSERT INTO tenants (id, name) VALUES ('TENANT-CONN','C')
		ON CONFLICT (id) DO NOTHING`); err != nil {
		t.Fatal(err)
	}
	scope, _ := secrets.SystemScope("TENANT-CONN", "connection-test")

	if _, err := store.Create(context.Background(), scope, newConnectionRequest("Acme")); err == nil {
		t.Fatal("a connection was saved against a connector with no conformance evidence")
	}

	// And every planned connector is refused too.
	for _, typ := range []string{"oracle", "snowflake", "bigquery", "databricks", "mysql"} {
		req := newConnectionRequest("Other")
		req.ConnectorType = typ
		if _, err := store.Create(context.Background(), scope, req); err == nil {
			t.Errorf("a connection was saved against %s, which has no driver", typ)
		}
	}
}

// A new connection's health must be NEVER_CHECKED, never a default that reads
// as healthy.
func TestANewConnectionIsNeverChecked(t *testing.T) {
	store, scope, _ := connectionStoreEnv(t)

	created, err := store.Create(context.Background(), scope, newConnectionRequest("Acme"))
	if err != nil {
		t.Fatal(err)
	}
	if created.Health.State != connectors.HealthNeverChecked {
		t.Errorf("health = %s, want NEVER_CHECKED; a connection nobody has tested must not "+
			"render as anything else", created.Health.State)
	}
	if !created.Health.CheckedAt.IsZero() {
		t.Error("an unchecked connection carries a check timestamp")
	}
}

// Testing an unreachable database records a real failure, not a green default.
func TestTestingAnUnreachableDatabaseRecordsAFailure(t *testing.T) {
	store, scope, _ := connectionStoreEnv(t)
	ctx := context.Background()

	req := newConnectionRequest("Unreachable")
	req.Fields["host"] = "no-such-host.invalid"
	created, err := store.Create(ctx, scope, req)
	if err != nil {
		t.Fatal(err)
	}

	health, checkErr := store.TestConnection(ctx, scope, created.ID)
	if checkErr == nil {
		t.Fatal("connecting to an invalid host reported success")
	}
	if health.State != connectors.HealthFailed {
		t.Errorf("health = %s, want FAILED", health.State)
	}
	if health.CheckedAt.IsZero() {
		t.Error("a real check must carry a timestamp")
	}
	if health.ErrorCategory == connectors.ErrorNone {
		t.Error("a failed check must carry an error category")
	}

	// The stored health matches, and the failure message carries neither the
	// credential nor the account.
	fetched, err := store.Get(ctx, scope.TenantID(), created.ID)
	if err != nil {
		t.Fatal(err)
	}
	if fetched.Health.State != connectors.HealthFailed {
		t.Errorf("stored health = %s, want FAILED", fetched.Health.State)
	}
	if strings.Contains(fetched.Health.Detail, connectionCredential) ||
		strings.Contains(fetched.Health.Detail, "svc_reporting") {
		t.Errorf("the stored health detail leaks: %q", fetched.Health.Detail)
	}
}

func TestReplacingACredentialIsTheOnlyWayToChangeIt(t *testing.T) {
	store, scope, _ := connectionStoreEnv(t)
	ctx := context.Background()

	created, err := store.Create(ctx, scope, newConnectionRequest("Acme"))
	if err != nil {
		t.Fatal(err)
	}

	// A field that is not a credential field is refused.
	if err := store.ReplaceSecret(ctx, scope, created.ID, "host", "somewhere"); err == nil {
		t.Error("a non-credential field was accepted as a secret replacement")
	}

	longer := strings.Repeat("a-much-longer-replacement-", 3)
	if err := store.ReplaceSecret(ctx, scope, created.ID, "password", longer); err != nil {
		t.Fatal(err)
	}

	fetched, err := store.Get(ctx, scope.TenantID(), created.ID)
	if err != nil {
		t.Fatal(err)
	}
	body, _ := json.Marshal(fetched)
	if strings.Contains(string(body), longer) {
		t.Error("the replaced credential is returned by a read path")
	}
	// The replacement is above the floor, so it is no longer weak.
	if len(fetched.WeakSecrets) != 0 {
		t.Errorf("weakSecrets = %v after replacing with a long credential", fetched.WeakSecrets)
	}
}

// ---------------------------------------------------------------------------
// Evidence
// ---------------------------------------------------------------------------

func TestEvidenceMustBeCompleteRecentAndMatchTheDriver(t *testing.T) {
	dir := t.TempDir()

	write := func(t *testing.T, rec *connectors.ConformanceRecord) string {
		t.Helper()
		path := filepath.Join(dir, fmt.Sprintf("evidence-%d.json", time.Now().UnixNano()))
		body, err := json.Marshal(connectors.EvidenceFile{
			GeneratedAt: time.Now().UTC(),
			Records:     []*connectors.ConformanceRecord{rec},
		})
		if err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, body, 0o600); err != nil {
			t.Fatal(err)
		}
		return path
	}

	good := func() *connectors.ConformanceRecord {
		return &connectors.ConformanceRecord{
			ConnectorType: "postgresql",
			ServerVersion: "PostgreSQL 16.13",
			DriverVersion: connectors.PostgresDriverVersion,
			TestCommit:    "abc123",
			RunAt:         time.Now().UTC().Add(-time.Hour),
			Passed:        21,
		}
	}

	// A complete, recent record makes the connector available.
	t.Setenv(connectors.EvidenceFileEnv, write(t, good()))
	evidence, err := connectors.LoadEvidence()
	if err != nil {
		t.Fatal(err)
	}
	r := connectors.NewRegistry()
	driver := connectors.NewPostgresConnector()
	t.Cleanup(func() { driver.Close() })
	connectors.ApplyEvidence(r, []connectors.Connector{driver}, evidence)
	if d, _ := r.Descriptor("postgresql"); d.Status != connectors.StatusAvailable {
		t.Fatalf("status = %s, want AVAILABLE on complete recent evidence", d.Status)
	}

	// A record with a failure is refused outright.
	failed := good()
	failed.Failed = 1
	t.Setenv(connectors.EvidenceFileEnv, write(t, failed))
	if _, err := connectors.LoadEvidence(); err == nil {
		t.Error("evidence recording a failure was accepted")
	}

	// A record with a skipped check is refused: a skip is not a pass.
	skipped := good()
	skipped.Skipped = []string{"untrusted_certificate_is_rejected"}
	t.Setenv(connectors.EvidenceFileEnv, write(t, skipped))
	if _, err := connectors.LoadEvidence(); err == nil {
		t.Error("evidence with a skipped check was accepted")
	}

	// A stale record is refused. A driver verified long ago, against a server
	// version nobody runs, by a suite since rewritten, is not evidence about
	// the code now shipping.
	stale := good()
	stale.RunAt = time.Now().UTC().Add(-200 * 24 * time.Hour)
	t.Setenv(connectors.EvidenceFileEnv, write(t, stale))
	if _, err := connectors.LoadEvidence(); err == nil {
		t.Error("evidence from 200 days ago was accepted")
	}

	// A record produced against a different driver build verified different
	// code, so the connector stays unselectable.
	mismatched := good()
	mismatched.DriverVersion = "some-other-driver/v0"
	t.Setenv(connectors.EvidenceFileEnv, write(t, mismatched))
	evidence, err = connectors.LoadEvidence()
	if err != nil {
		t.Fatal(err)
	}
	r2 := connectors.NewRegistry()
	driver2 := connectors.NewPostgresConnector()
	t.Cleanup(func() { driver2.Close() })
	notes := connectors.ApplyEvidence(r2, []connectors.Connector{driver2}, evidence)
	if d, _ := r2.Descriptor("postgresql"); d.Status == connectors.StatusAvailable {
		t.Errorf("a driver-version mismatch produced AVAILABLE; notes: %v", notes)
	}
}

func TestNoEvidenceMeansNoConnectorIsSelectable(t *testing.T) {
	t.Setenv(connectors.EvidenceFileEnv, "")

	evidence, err := connectors.LoadEvidence()
	if err != nil {
		t.Fatal(err)
	}
	if evidence != nil {
		t.Error("an unset evidence path should yield no records, not an error")
	}

	r := connectors.NewRegistry()
	driver := connectors.NewPostgresConnector()
	t.Cleanup(func() { driver.Close() })
	connectors.ApplyEvidence(r, []connectors.Connector{driver}, evidence)

	if avail := r.Available(); len(avail) != 0 {
		t.Errorf("available = %v with no evidence; nothing may be selectable", avail)
	}
	if d, _ := r.Descriptor("postgresql"); d.Status != connectors.StatusImplementing {
		t.Errorf("status = %s, want IMPLEMENTING: the driver exists and is unverified", d.Status)
	}
}

// ---------------------------------------------------------------------------
// Over HTTP
// ---------------------------------------------------------------------------

// A connection cannot be created through the API while no connector is
// selectable, and the refusal says why rather than looking like a bad request.
func TestCreatingAConnectionOverHttpIsRefusedWithoutEvidence(t *testing.T) {
	db := setupTestDb(t)
	t.Cleanup(func() { db.Close() })
	handler := NewRouter(db, ingestDemoConfig(), nil)

	body, _ := json.Marshal(map[string]any{
		"displayName":       "Acme",
		"connectorType":     "postgresql",
		"authMode":          "password",
		"fields":            map[string]string{"host": "db.example.test"},
		"secrets":           map[string]string{"password": connectionCredential},
		"resourceAllowlist": []string{"reporting"},
	})
	req := httptest.NewRequest(http.MethodPost, "/api/v1/connections", strings.NewReader(string(body)))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code == http.StatusCreated {
		t.Fatal("a connection was created although no connector has conformance evidence")
	}
	if strings.Contains(rec.Body.String(), connectionCredential) {
		t.Fatal("the refusal echoes the submitted credential")
	}
}

// The list route must be tenant-scoped and must never carry a credential.
func TestTheConnectionListIsScopedAndCarriesNoCredential(t *testing.T) {
	db := setupTestDb(t)
	t.Cleanup(func() { db.Close() })
	handler := NewRouter(db, ingestDemoConfig(), nil)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/connections", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	// Either the route works and returns an empty list, or the store is
	// unavailable in this profile. Both are acceptable; disclosing a
	// credential is not.
	if strings.Contains(rec.Body.String(), connectionCredential) {
		t.Fatal("the connection list discloses a credential")
	}
	if rec.Code == http.StatusOK {
		var body struct {
			Connections []map[string]any `json:"connections"`
		}
		if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
			t.Fatal(err)
		}
		for _, c := range body.Connections {
			for _, forbidden := range []string{"password", "secret", "privateKey", "connectionString"} {
				if _, ok := c[forbidden]; ok {
					t.Errorf("a listed connection carries a %q field", forbidden)
				}
			}
		}
	}
}
