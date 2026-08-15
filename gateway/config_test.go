package main

import (
	"database/sql"
	"strings"
	"testing"
)

// withEnv sets environment variables for one test and restores them after.
func withEnv(t *testing.T, kv map[string]string) {
	t.Helper()
	for k, v := range kv {
		t.Setenv(k, v)
	}
}

// A production process must refuse to start rather than guess. Each case below
// omits exactly one required setting.
func TestProductionProfileRefusesIncompleteConfiguration(t *testing.T) {
	complete := map[string]string{
		"SENTINEL_PROFILE":        "production",
		"SENTINEL_API_TOKEN":      strings.Repeat("k", 48),
		"DATABASE_URL":            "/var/lib/sentinel/sentinel.db",
		"OBJECT_STORE_URL":        "http://minio:9000",
		"SENTINEL_ALLOWED_ORIGIN": "https://ops.example.com",
		"SENTINEL_PGP_KEYRING":    "/etc/sentinel/keyring.asc",
		"SENTINEL_OIDC_ISSUER":    "https://idp.example.com/",
		"SENTINEL_OIDC_AUDIENCE":  "sentinel-flow-api",
		"SENTINEL_OIDC_JWKS_URL":  "https://idp.example.com/.well-known/jwks.json",
	}

	// Sanity: the complete set must load.
	withEnv(t, complete)
	if _, err := Load(); err != nil {
		t.Fatalf("complete production config should load, got: %v", err)
	}

	for _, missing := range []string{
		"SENTINEL_API_TOKEN",
		"DATABASE_URL",
		"OBJECT_STORE_URL",
		"SENTINEL_ALLOWED_ORIGIN",
		"SENTINEL_PGP_KEYRING",
		// Identity is as mandatory as the database. Without a verified issuer
		// and audience there is no actor to attribute a decision to.
		"SENTINEL_OIDC_ISSUER",
		"SENTINEL_OIDC_AUDIENCE",
		"SENTINEL_OIDC_JWKS_URL",
	} {
		t.Run("missing_"+missing, func(t *testing.T) {
			withEnv(t, complete)
			t.Setenv(missing, "")

			cfg, err := Load()
			if err == nil {
				t.Fatalf("production profile started without %s (bind=%s)", missing, cfg.Addr())
			}
			if !strings.Contains(err.Error(), missing) {
				t.Errorf("error should name the missing setting %s, got: %v", missing, err)
			}
		})
	}
}

func TestProductionRejectsWeakOrDefaultToken(t *testing.T) {
	base := map[string]string{
		"SENTINEL_PROFILE":        "production",
		"DATABASE_URL":            "/var/lib/sentinel/sentinel.db",
		"OBJECT_STORE_URL":        "http://minio:9000",
		"SENTINEL_ALLOWED_ORIGIN": "https://ops.example.com",
		"SENTINEL_PGP_KEYRING":    "/etc/sentinel/keyring.asc",
		"SENTINEL_OIDC_ISSUER":    "https://idp.example.com/",
		"SENTINEL_OIDC_AUDIENCE":  "sentinel-flow-api",
		"SENTINEL_OIDC_JWKS_URL":  "https://idp.example.com/.well-known/jwks.json",
	}

	for _, token := range []string{"password", "minioadmin", "changeme", "admin", "short"} {
		t.Run(token, func(t *testing.T) {
			withEnv(t, base)
			t.Setenv("SENTINEL_API_TOKEN", token)
			if _, err := Load(); err == nil {
				t.Errorf("production accepted the weak or well-known token %q", token)
			}
		})
	}
}

// The demo profile is the only one that may run unauthenticated, so it must not
// be reachable from off-host.
func TestDemoProfileBindsLoopbackOnly(t *testing.T) {
	withEnv(t, map[string]string{"SENTINEL_PROFILE": "local-demo"})

	cfg, err := Load()
	if err != nil {
		t.Fatalf("demo profile should load with defaults: %v", err)
	}
	if cfg.BindAddress != "127.0.0.1" {
		t.Errorf("demo profile must default to loopback, got %q", cfg.BindAddress)
	}
	if !cfg.IsDemo() {
		t.Errorf("IsDemo() must be true in the demo profile")
	}

	// And it must refuse to be told otherwise.
	t.Setenv("SENTINEL_BIND_ADDRESS", "0.0.0.0")
	if _, err := Load(); err == nil {
		t.Errorf("demo profile accepted a non-loopback bind address")
	}
}

func TestUnknownProfileIsRejected(t *testing.T) {
	withEnv(t, map[string]string{"SENTINEL_PROFILE": "staging"})
	if _, err := Load(); err == nil {
		t.Errorf("an unrecognised profile must not fall through to permissive defaults")
	}
}

func TestMalformedDependencyUrlFailsAtStartup(t *testing.T) {
	withEnv(t, map[string]string{
		"SENTINEL_PROFILE": "local-demo",
		"AI_TIER_URL":      "not-a-url",
	})
	if _, err := Load(); err == nil {
		t.Errorf("a malformed AI_TIER_URL must fail at startup, not at first use")
	}
}

// AI_TIER_URL was previously set in compose and ignored by code that hardcoded
// 127.0.0.1. This pins that the configured value is the one carried forward.
func TestConfiguredAiTierUrlIsHonoured(t *testing.T) {
	withEnv(t, map[string]string{
		"SENTINEL_PROFILE": "local-demo",
		"AI_TIER_URL":      "http://ai-tier:8000",
	})
	cfg, err := Load()
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	if cfg.AITierURL != "http://ai-tier:8000" {
		t.Errorf("configured AI tier URL not carried into config, got %q", cfg.AITierURL)
	}
}

// ---------------------------------------------------------------------------
// Migrations
// ---------------------------------------------------------------------------

func memDB(t *testing.T) *sql.DB {
	t.Helper()
	db, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	t.Cleanup(func() { db.Close() })
	return db
}

func TestMigrateEmptyDatabase(t *testing.T) {
	db := memDB(t)

	applied, err := Migrate(db)
	if err != nil {
		t.Fatalf("migrate empty database: %v", err)
	}
	if len(applied) == 0 {
		t.Fatalf("expected at least one migration to apply to an empty database")
	}

	for _, table := range []string{
		"partners", "file_contracts", "expectations", "file_instances",
		"incidents", "validation_findings", "audit_events", "schema_migrations",
	} {
		var name string
		err := db.QueryRow("SELECT name FROM sqlite_master WHERE type='table' AND name=?", table).Scan(&name)
		if err != nil {
			t.Errorf("table %q missing after migration: %v", table, err)
		}
	}
}

func TestMigrateIsIdempotent(t *testing.T) {
	db := memDB(t)

	first, err := Migrate(db)
	if err != nil {
		t.Fatalf("first migrate: %v", err)
	}
	second, err := Migrate(db)
	if err != nil {
		t.Fatalf("second migrate must not error: %v", err)
	}
	if len(second) != 0 {
		t.Errorf("second migrate applied %v; expected nothing", second)
	}

	st, err := Status(db)
	if err != nil {
		t.Fatalf("status: %v", err)
	}
	if len(st.Pending) != 0 {
		t.Errorf("expected no pending migrations, got %v", st.Pending)
	}
	if len(st.Applied) != len(first) {
		t.Errorf("applied count drifted: %v vs %v", st.Applied, first)
	}
}

// Upgrade path: a database created by the previous opportunistic startup code
// has the tables but no schema_migrations row. Migrating it must succeed rather
// than fail on "table already exists".
func TestMigrateUpgradesLegacySchemaFixture(t *testing.T) {
	db := memDB(t)

	legacy := `
	CREATE TABLE partners (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		name VARCHAR(255) NOT NULL,
		routing_number VARCHAR(9) UNIQUE NOT NULL,
		created_at DATETIME DEFAULT CURRENT_TIMESTAMP
	);
	CREATE TABLE audit_events (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		event_type VARCHAR(100) NOT NULL,
		actor VARCHAR(100) NOT NULL,
		payload TEXT NOT NULL,
		previous_hash VARCHAR(64) NOT NULL,
		current_hash VARCHAR(64) NOT NULL,
		created_at DATETIME DEFAULT CURRENT_TIMESTAMP
	);`
	if _, err := db.Exec(legacy); err != nil {
		t.Fatalf("build legacy fixture: %v", err)
	}
	if _, err := db.Exec(`INSERT INTO partners (name, routing_number) VALUES ('Pre-existing Partner','021000021')`); err != nil {
		t.Fatalf("seed legacy row: %v", err)
	}

	if _, err := Migrate(db); err != nil {
		t.Fatalf("migrating a legacy database must succeed, got: %v", err)
	}

	// Pre-existing data must survive the upgrade.
	var name string
	if err := db.QueryRow(`SELECT name FROM partners WHERE routing_number='021000021'`).Scan(&name); err != nil {
		t.Fatalf("pre-existing row lost during upgrade: %v", err)
	}
	if name != "Pre-existing Partner" {
		t.Errorf("row altered during upgrade: %q", name)
	}

	// And tables the legacy schema lacked must now exist.
	var t2 string
	if err := db.QueryRow("SELECT name FROM sqlite_master WHERE type='table' AND name='validation_findings'").Scan(&t2); err != nil {
		t.Errorf("upgrade did not create missing table validation_findings: %v", err)
	}
}

// The schema migration must not carry demo data. Every deployment used to get
// three fictional partners on first start.
func TestMigrationDoesNotSeedDemoData(t *testing.T) {
	db := memDB(t)
	if _, err := Migrate(db); err != nil {
		t.Fatalf("migrate: %v", err)
	}

	var partners int
	if err := db.QueryRow("SELECT COUNT(*) FROM partners").Scan(&partners); err != nil {
		t.Fatalf("count partners: %v", err)
	}
	if partners != 0 {
		t.Errorf("a freshly migrated database contains %d partner rows; schema migrations must not seed data", partners)
	}
}

func TestDemoSeedIsRefusedOutsideDemoProfile(t *testing.T) {
	db := memDB(t)
	if _, err := Migrate(db); err != nil {
		t.Fatalf("migrate: %v", err)
	}

	err := SeedDemo(db, &Config{Profile: ProfileProduction})
	if err == nil {
		t.Fatalf("demo seed was applied in the production profile")
	}

	var partners int
	_ = db.QueryRow("SELECT COUNT(*) FROM partners").Scan(&partners)
	if partners != 0 {
		t.Errorf("refused seed still wrote %d rows", partners)
	}

	// And it must work in the demo profile.
	if err := SeedDemo(db, &Config{Profile: ProfileLocalDemo}); err != nil {
		t.Fatalf("demo seed should apply in the demo profile: %v", err)
	}
	_ = db.QueryRow("SELECT COUNT(*) FROM partners").Scan(&partners)
	if partners == 0 {
		t.Errorf("demo seed applied but wrote no rows")
	}
}
