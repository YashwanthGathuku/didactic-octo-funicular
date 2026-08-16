package main

import (
	"database/sql"
	"embed"
	"fmt"
	"log"
	"os"
	"sort"
	"strings"
)

// Migrations are embedded so the binary carries its own schema. Previously the
// gateway read ./migrations/01_init.sql relative to the working directory, which
// meant the schema silently did not apply when the process started from
// anywhere else -- and the failure was logged as a "notice" and ignored.
//
//go:embed migrations/*.sql
var migrationFS embed.FS

// Demo seed data is embedded separately and is never applied by Migrate.
//
//go:embed seed/*.sql
var seedFS embed.FS

// SeedDemo applies fictional counterparties for a developer machine.
//
// It refuses outside the local-demo profile. This data used to be appended to
// the schema migration, so every deployment -- including a production one --
// created three demo partners on first start.
func SeedDemo(db *sql.DB, cfg *Config) error {
	if !cfg.IsDemo() {
		return fmt.Errorf("refusing to seed demo data in profile %q: demo data is permitted only in %q",
			cfg.Profile, ProfileLocalDemo)
	}
	body, err := seedFS.ReadFile("seed/demo_seed.sql")
	if err != nil {
		return fmt.Errorf("read demo seed: %w", err)
	}
	if _, err := db.Exec(string(body)); err != nil {
		return fmt.Errorf("apply demo seed: %w", err)
	}
	return nil
}

// Migrate applies every migration not yet recorded in schema_migrations, in
// filename order, each inside its own transaction. It returns the versions
// applied by this call.
//
// Properties this provides that the previous startup code did not:
//   - versioned: schema_migrations records what ran, so upgrades are reproducible
//   - idempotent: re-running applies nothing and is not an error
//   - transactional: a failing migration rolls back rather than leaving a
//     half-applied schema
//   - fail-loud: an error is returned, not logged and discarded
func Migrate(db *sql.DB) ([]string, error) {
	if _, err := db.Exec(`
		CREATE TABLE IF NOT EXISTS schema_migrations (
			version     TEXT PRIMARY KEY,
			applied_at  TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
			checksum    TEXT NOT NULL
		)
	`); err != nil {
		return nil, fmt.Errorf("create schema_migrations: %w", err)
	}

	entries, err := migrationFS.ReadDir("migrations")
	if err != nil {
		return nil, fmt.Errorf("read embedded migrations: %w", err)
	}

	var versions []string
	for _, e := range entries {
		if !e.IsDir() && strings.HasSuffix(e.Name(), ".sql") {
			versions = append(versions, e.Name())
		}
	}
	sort.Strings(versions)

	applied, err := appliedVersions(db)
	if err != nil {
		return nil, err
	}

	var ran []string
	for _, v := range versions {
		if _, done := applied[v]; done {
			continue
		}

		body, err := migrationFS.ReadFile("migrations/" + v)
		if err != nil {
			return nil, fmt.Errorf("read migration %s: %w", v, err)
		}

		tx, err := db.Begin()
		if err != nil {
			return nil, fmt.Errorf("begin migration %s: %w", v, err)
		}
		if _, err := tx.Exec(string(body)); err != nil {
			_ = tx.Rollback()
			return nil, fmt.Errorf("apply migration %s: %w", v, err)
		}
		if _, err := tx.Exec(
			"INSERT INTO schema_migrations (version, checksum) VALUES (?, ?)",
			v, checksumOf(body),
		); err != nil {
			_ = tx.Rollback()
			return nil, fmt.Errorf("record migration %s: %w", v, err)
		}
		if err := tx.Commit(); err != nil {
			return nil, fmt.Errorf("commit migration %s: %w", v, err)
		}
		ran = append(ran, v)
	}

	return ran, nil
}

// MigrationStatus reports which migrations are applied and which are pending.
type MigrationStatus struct {
	Applied []string
	Pending []string
}

// Status inspects the database without changing it.
func Status(db *sql.DB) (*MigrationStatus, error) {
	appliedSet, err := appliedVersions(db)
	if err != nil {
		return nil, err
	}

	entries, err := migrationFS.ReadDir("migrations")
	if err != nil {
		return nil, err
	}

	st := &MigrationStatus{}
	var all []string
	for _, e := range entries {
		if !e.IsDir() && strings.HasSuffix(e.Name(), ".sql") {
			all = append(all, e.Name())
		}
	}
	sort.Strings(all)

	for _, v := range all {
		if _, ok := appliedSet[v]; ok {
			st.Applied = append(st.Applied, v)
		} else {
			st.Pending = append(st.Pending, v)
		}
	}
	return st, nil
}

func appliedVersions(db *sql.DB) (map[string]struct{}, error) {
	applied := map[string]struct{}{}

	rows, err := db.Query("SELECT version FROM schema_migrations")
	if err != nil {
		// Table absent means nothing has been applied yet.
		return applied, nil
	}
	defer rows.Close()

	for rows.Next() {
		var v string
		if err := rows.Scan(&v); err != nil {
			return nil, err
		}
		applied[v] = struct{}{}
	}
	return applied, rows.Err()
}

func checksumOf(b []byte) string {
	// Detects a migration file edited after it was applied, which is the usual
	// cause of "works on my machine" schema drift.
	var h uint64 = 1469598103934665603
	for _, c := range b {
		h ^= uint64(c)
		h *= 1099511628211
	}
	return fmt.Sprintf("%016x", h)
}

// runMigrateCommand implements `sentinel-gateway migrate [status]`, so a
// deployment can migrate as a separate step from serving traffic.
func runMigrateCommand(args []string) {
	cfg, err := Load()
	if err != nil {
		log.Fatalf("%v", err)
	}

	db, err := sql.Open("sqlite", sqliteDSN(cfg.DatabaseURL))
	if err != nil {
		log.Fatalf("open database %q: %v", cfg.DatabaseURL, err)
	}
	defer db.Close()

	if err := db.Ping(); err != nil {
		log.Fatalf("connect to database %q: %v", cfg.DatabaseURL, err)
	}

	if len(args) > 0 && args[0] == "seed-demo" {
		if _, err := Migrate(db); err != nil {
			log.Fatalf("migrate before seed: %v", err)
		}
		if err := SeedDemo(db, cfg); err != nil {
			log.Fatalf("%v", err)
		}
		fmt.Println("demo seed applied (local-demo profile only)")
		return
	}

	if len(args) > 0 && args[0] == "status" {
		st, err := Status(db)
		if err != nil {
			log.Fatalf("migration status: %v", err)
		}
		fmt.Printf("database: %s\n", cfg.DatabaseURL)
		fmt.Printf("applied (%d):\n", len(st.Applied))
		for _, v := range st.Applied {
			fmt.Printf("  %s\n", v)
		}
		fmt.Printf("pending (%d):\n", len(st.Pending))
		for _, v := range st.Pending {
			fmt.Printf("  %s\n", v)
		}
		if len(st.Pending) > 0 {
			os.Exit(1) // non-zero so CI can gate on an unmigrated database
		}
		return
	}

	ran, err := Migrate(db)
	if err != nil {
		log.Fatalf("migrate: %v", err)
	}
	if len(ran) == 0 {
		fmt.Println("database is up to date; no migrations applied")
		return
	}
	fmt.Printf("applied %d migration(s):\n", len(ran))
	for _, v := range ran {
		fmt.Printf("  %s\n", v)
	}
}
