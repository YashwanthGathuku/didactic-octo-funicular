package connectors

import (
	"context"
	"fmt"
	"net/url"
	"os"
	"strings"
	"testing"
	"time"

	"sentinel-gateway/internal/secrets"

	_ "github.com/jackc/pgx/v5/stdlib"
)

// The PostgreSQL driver's conformance run.
//
// This is the gate, not a smoke test. It runs against a real server, creates a
// disposable schema, and exercises every property in the shared suite --
// including the ones that can only be demonstrated by something being refused:
// an untrusted certificate, a wrong password, a schema outside the allowlist, a
// query that has to be cancelled.
//
// Without SENTINEL_TEST_POSTGRES_DSN it skips, and the skip says that the
// connector is consequently NOT verified. That matters more here than anywhere
// else in this repository: a skipped run is what keeps the connector out of the
// AVAILABLE set, so the skip is the mechanism working rather than a gap.

func postgresDSN(t *testing.T) string {
	t.Helper()
	dsn := os.Getenv("SENTINEL_TEST_POSTGRES_DSN")
	if dsn == "" {
		t.Skip("SKIPPED: SENTINEL_TEST_POSTGRES_DSN is unset, so the PostgreSQL connector is " +
			"NOT verified by this run and cannot become AVAILABLE")
	}
	return dsn
}

// fixtureFromDSN builds a real fixture: a throwaway schema with a table to
// count, a wide table to truncate, and a second schema outside the allowlist.
func fixtureFromDSN(t *testing.T, dsn string) (Fixture, func()) {
	t.Helper()
	ctx := context.Background()

	u, err := url.Parse(dsn)
	if err != nil {
		t.Fatalf("parse DSN: %v", err)
	}
	password, _ := u.User.Password()
	username := u.User.Username()
	host := u.Hostname()
	port := u.Port()
	if port == "" {
		port = "5432"
	}
	database := strings.TrimPrefix(u.Path, "/")

	// sslmode from the DSN, so a local server without TLS can still exercise
	// every other property. The TLS check reports honestly either way.
	tlsMode := u.Query().Get("sslmode")
	if tlsMode == "" {
		tlsMode = "require"
	}
	if tlsMode == "disable" {
		// "disable" is not an option this connector offers. The fixture uses
		// the weakest one it does offer and the untrusted-certificate check
		// records what that leaves unverified.
		tlsMode = "require"
	}

	admin, err := openAdmin(dsn)
	if err != nil {
		t.Fatalf("open admin connection: %v", err)
	}

	suffix := fmt.Sprintf("%d", time.Now().UnixNano())
	approved := "conn_ok_" + suffix
	forbidden := "conn_no_" + suffix

	for _, stmt := range []string{
		`CREATE SCHEMA ` + approved,
		`CREATE SCHEMA ` + forbidden,
		`CREATE TABLE ` + approved + `.entries (id int, label text)`,
		`INSERT INTO ` + approved + `.entries SELECT g, 'row-' || g FROM generate_series(1, 500) g`,
		`CREATE TABLE ` + forbidden + `.secrets_table (id int)`,
		`INSERT INTO ` + forbidden + `.secrets_table VALUES (1)`,
	} {
		if _, err := admin.ExecContext(ctx, stmt); err != nil {
			admin.Close()
			t.Fatalf("build fixture: %v", err)
		}
	}

	var serverVersion string
	if err := admin.QueryRowContext(ctx, "SHOW server_version").Scan(&serverVersion); err != nil {
		serverVersion = "unknown"
	}

	cleanup := func() {
		for _, s := range []string{approved, forbidden} {
			_, _ = admin.ExecContext(context.Background(), `DROP SCHEMA IF EXISTS `+s+` CASCADE`)
		}
		admin.Close()
	}

	// NewExternal, not New: a customer's database password is not a credential
	// this application chose, so the MinSecretLength floor does not apply to
	// it. The fixture's password is a local development one and is short.
	mustSecret := func(v string) secrets.Value {
		s, weak, err := secrets.NewExternal(v)
		if err != nil {
			t.Fatalf("seal fixture secret: %v", err)
		}
		if weak {
			t.Logf("note: the fixture credential is shorter than %d characters; "+
				"a real deployment would surface this to the operator", secrets.MinSecretLength)
		}
		return s
	}

	cfg := Config{
		Type: "postgresql",
		Fields: map[string]string{
			"host": host, "port": port, "database": database,
			"username": username, "tls_mode": tlsMode,
		},
		ResourceAllowlist: []string{approved},
	}

	// A configuration pointing at a host that will not present a certificate
	// this gateway trusts. verify-full against a server with no TLS, or with a
	// self-signed certificate, must be refused.
	untrusted := cfg
	untrusted.Fields = map[string]string{}
	for k, v := range cfg.Fields {
		untrusted.Fields[k] = v
	}
	untrusted.Fields["tls_mode"] = "verify-full"

	other := cfg
	other.Fields = map[string]string{}
	for k, v := range cfg.Fields {
		other.Fields[k] = v
	}
	other.ResourceAllowlist = []string{forbidden}

	countTemplate, err := RegisterTemplate(Template{
		ID: "fixture_count", ConnectorType: "postgresql",
		SQL:         `SELECT count(*) AS n FROM {{scope}}.entries WHERE label = $1`,
		Params:      []string{"label"},
		Identifiers: []string{"scope"},
		Classifications: map[string]Classification{
			"n": ClassPublic,
		},
	})
	if err != nil {
		cleanup()
		t.Fatalf("register count template: %v", err)
	}

	slowTemplate, err := RegisterTemplate(Template{
		ID: "fixture_slow", ConnectorType: "postgresql",
		SQL:    `SELECT count(*) AS n FROM generate_series(1, 200000000) g WHERE g > $1`,
		Params: []string{"floor"},
		Classifications: map[string]Classification{
			"n": ClassPublic,
		},
	})
	if err != nil {
		cleanup()
		t.Fatalf("register slow template: %v", err)
	}

	wideTemplate, err := RegisterTemplate(Template{
		ID: "fixture_wide", ConnectorType: "postgresql",
		SQL:         `SELECT id, label FROM {{scope}}.entries WHERE id > $1 ORDER BY id`,
		Params:      []string{"floor"},
		Identifiers: []string{"scope"},
		Classifications: map[string]Classification{
			"id":    ClassPublic,
			"label": ClassPublic,
		},
	})
	if err != nil {
		cleanup()
		t.Fatalf("register wide template: %v", err)
	}

	return Fixture{
		Config:  cfg,
		Secrets: NewSecrets(map[string]secrets.Value{"password": mustSecret(password)}),
		BadSecrets: NewSecrets(map[string]secrets.Value{
			"password": mustSecret("this-password-is-not-the-real-one-" + suffix),
		}),
		AuthMode:           "password",
		UntrustedTLSConfig: &untrusted,
		AllowedResource:    approved,
		ForbiddenResource:  forbidden,

		CountTemplate: countTemplate,
		CountParams:   map[string]any{"label": "row-1"},
		CountIdents:   map[string]string{"scope": approved},

		SlowTemplate: slowTemplate,
		SlowParams:   map[string]any{"floor": 0},

		WideTemplate: wideTemplate,
		WideParams:   map[string]any{"floor": 0},
		WideIdents:   map[string]string{"scope": approved},

		OtherTenantConfig: &other,
		OtherTenantSecret: NewSecrets(map[string]secrets.Value{"password": mustSecret(password)}),

		ServerVersion: "PostgreSQL " + serverVersion,
		DriverVersion: PostgresDriverVersion,
		TestCommit:    testCommit(),
	}, cleanup
}

func TestPostgresConnectorPassesTheSharedConformanceSuite(t *testing.T) {
	dsn := postgresDSN(t)
	fixture, cleanup := fixtureFromDSN(t, dsn)
	defer cleanup()

	c := NewPostgresConnector()
	defer c.Close()

	report := RunConformance(context.Background(), c, fixture)
	t.Log("\n" + report.Summary())

	for _, check := range report.Checks {
		switch {
		case check.Skipped:
			t.Errorf("check %s was SKIPPED (%s); a skipped check is not a passed one, and the "+
				"connector must not become AVAILABLE on a partial run", check.Name, check.Reason)
		case !check.Passed:
			t.Errorf("check %s FAILED: %s", check.Name, check.Reason)
		}
	}

	if !report.Passed() {
		t.Fatal("the PostgreSQL connector did not pass conformance and must remain unselectable")
	}
	if !report.Record.Complete() {
		t.Fatalf("the conformance record is incomplete: %+v", report.Record)
	}
}

// The registry must refuse to make a driver selectable without evidence.
func TestOnlyAConformanceRecordMakesAConnectorAvailable(t *testing.T) {
	c := NewPostgresConnector()
	defer c.Close()

	r := NewRegistry()

	// Before anything is registered, every entry is PLANNED and refuses.
	for _, d := range r.Catalog() {
		if d.Status != StatusPlanned {
			t.Errorf("%s starts as %s, want PLANNED", d.Type, d.Status)
		}
		if _, _, err := r.Driver(d.Type); err == nil {
			t.Errorf("%s returned a driver before any was registered", d.Type)
		}
	}

	// A driver with no record is visible and not selectable.
	if err := r.Register(c, nil); err != nil {
		t.Fatal(err)
	}
	d, err := r.Descriptor("postgresql")
	if err != nil {
		t.Fatal(err)
	}
	if d.Status != StatusImplementing {
		t.Errorf("status = %s, want IMPLEMENTING for a driver with no conformance record", d.Status)
	}
	if _, _, err := r.Driver("postgresql"); err == nil {
		t.Error("a driver with no conformance record must not be reachable")
	}

	// A record with a failure does not qualify either.
	if err := r.Register(c, &ConformanceRecord{
		ConnectorType: "postgresql", ServerVersion: "PostgreSQL 16",
		TestCommit: "abc", RunAt: time.Now(), Passed: 20, Failed: 1,
	}); err != nil {
		t.Fatal(err)
	}
	if d, _ := r.Descriptor("postgresql"); d.Status == StatusAvailable {
		t.Error("a conformance record with a failure must not make a connector available")
	}

	// Nor does a record with a skipped check, which is recorded as a failure.
	if err := r.Register(c, &ConformanceRecord{
		ConnectorType: "postgresql", ServerVersion: "PostgreSQL 16",
		TestCommit: "abc", RunAt: time.Now(), Passed: 20, Failed: 1,
		Skipped: []string{"untrusted_certificate_is_rejected"},
	}); err != nil {
		t.Fatal(err)
	}
	if d, _ := r.Descriptor("postgresql"); d.Status == StatusAvailable {
		t.Error("a record with a skipped check must not make a connector available")
	}

	// A complete record does.
	if err := r.Register(c, &ConformanceRecord{
		ConnectorType: "postgresql", ServerVersion: "PostgreSQL 16",
		DriverVersion: PostgresDriverVersion,
		TestCommit:    "abc", RunAt: time.Now(), Passed: 21, Failed: 0,
	}); err != nil {
		t.Fatal(err)
	}
	d, _ = r.Descriptor("postgresql")
	if d.Status != StatusAvailable {
		t.Fatalf("status = %s, want AVAILABLE with a complete record", d.Status)
	}
	if _, _, err := r.Driver("postgresql"); err != nil {
		t.Errorf("an available connector must be reachable: %v", err)
	}

	// And every other entry is still refused.
	for _, other := range r.Catalog() {
		if other.Type == "postgresql" {
			continue
		}
		if _, _, err := r.Driver(other.Type); err == nil {
			t.Errorf("%s is selectable without a driver", other.Type)
		}
	}
	if avail := r.Available(); len(avail) != 1 || avail[0] != "postgresql" {
		t.Errorf("available connectors = %v, want only postgresql", avail)
	}
}

func testCommit() string {
	if c := os.Getenv("GITHUB_SHA"); c != "" {
		return c
	}
	return "local-build"
}

// The conformance run publishes the evidence artefact a deployment carries.
//
// The file is written by the same code that reads it, so the format cannot
// drift between producer and consumer -- and drift there would present as a
// deployment that silently has no evidence, which looks identical to one that
// never ran the suite.
func TestConformanceRunPublishesEvidence(t *testing.T) {
	dsn := postgresDSN(t)
	fixture, cleanup := fixtureFromDSN(t, dsn)
	defer cleanup()

	c := NewPostgresConnector()
	defer c.Close()

	report := RunConformance(context.Background(), c, fixture)
	if !report.Passed() {
		t.Skipf("the run did not pass, so there is no evidence to publish:\n%s", report.Summary())
	}

	path := os.Getenv("SENTINEL_CONNECTOR_EVIDENCE_OUT")
	if path == "" {
		path = t.TempDir() + "/evidence.json"
	}
	if err := WriteEvidence(path, report); err != nil {
		t.Fatalf("write evidence: %v", err)
	}

	// Read it back through the loader the binary uses, and confirm it promotes
	// the connector. A file that cannot do that is not evidence.
	t.Setenv(EvidenceFileEnv, path)
	evidence, err := LoadEvidence()
	if err != nil {
		t.Fatalf("the published evidence was rejected by the loader: %v", err)
	}

	r := NewRegistry()
	driver := NewPostgresConnector()
	defer driver.Close()
	notes := ApplyEvidence(r, []Connector{driver}, evidence)

	d, err := r.Descriptor("postgresql")
	if err != nil {
		t.Fatal(err)
	}
	if d.Status != StatusAvailable {
		t.Fatalf("status = %s after applying this run's own evidence; notes: %v", d.Status, notes)
	}
	t.Logf("evidence written to %s and accepted: %v", path, notes)

	// A run that failed must refuse to publish. Evidence is the whole gate, so
	// the one thing WriteEvidence must never do is record a pass that did not
	// happen.
	broken := report
	broken.Checks = append([]CheckResult{{Name: "synthetic", Passed: false}}, broken.Checks...)
	if err := WriteEvidence(t.TempDir()+"/bad.json", broken); err == nil {
		t.Error("evidence was written for a run that did not pass")
	}
}
