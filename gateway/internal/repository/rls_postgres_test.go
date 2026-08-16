package repository

import (
	"database/sql"
	"os"
	"strings"
	"testing"

	_ "github.com/jackc/pgx/v5/stdlib"
)

// Row-level security, exercised against a real PostgreSQL server.
//
// This is the defence the application layer cannot provide. repository.Scope
// makes it impossible to *write* a query without a tenant; RLS makes it
// impossible for such a query to *return* another tenant's rows even if one
// were written by hand, by a future maintainer, or through a SQL injection that
// reached the driver.
//
// Skipped when SENTINEL_TEST_POSTGRES_DSN is unset, and the skip says so rather
// than passing silently -- a security test that quietly does nothing is worse
// than no test.
//
// Locally:
//   createdb sentinel_test && psql -f migrations_postgres/001_schema_and_rls.sql
//   SENTINEL_TEST_POSTGRES_DSN='postgres://sentinel_app:...@127.0.0.1/sentinel_test' go test ./...

func postgresDSN(t *testing.T) string {
	t.Helper()
	dsn := os.Getenv("SENTINEL_TEST_POSTGRES_DSN")
	if dsn == "" {
		t.Skip("SKIPPED: SENTINEL_TEST_POSTGRES_DSN is unset, so PostgreSQL row-level security is NOT verified by this run")
	}
	return dsn
}

func openPG(t *testing.T) *sql.DB {
	t.Helper()
	db, err := sql.Open("pgx", postgresDSN(t))
	if err != nil {
		t.Fatalf("open postgres: %v", err)
	}
	if err := db.Ping(); err != nil {
		t.Fatalf("ping postgres: %v", err)
	}
	t.Cleanup(func() { db.Close() })
	return db
}

// seedTwoTenants makes the fixture idempotent so the suite can be re-run.
func seedTwoTenants(t *testing.T, db *sql.DB) {
	t.Helper()
	stmts := []string{
		`INSERT INTO tenants (id,name) VALUES ('TENANT-ALPHA','Alpha') ON CONFLICT DO NOTHING`,
		`INSERT INTO tenants (id,name) VALUES ('TENANT-BETA','Beta') ON CONFLICT DO NOTHING`,
	}
	for _, s := range stmts {
		if _, err := db.Exec(s); err != nil {
			// The app role cannot write tenants by design; tolerate the denial
			// when the rows already exist from the migration fixture.
			t.Logf("seed note: %v", err)
		}
	}
}

// withTenant runs fn inside a transaction with the RLS setting applied.
func withTenant(t *testing.T, db *sql.DB, tenant string, fn func(tx *sql.Tx)) {
	t.Helper()
	tx, err := db.Begin()
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = tx.Rollback() }()

	// SET LOCAL scopes the setting to this transaction, so a pooled connection
	// cannot carry one tenant's scope into another's query.
	if _, err := tx.Exec("SELECT set_config('sentinel.tenant_id', $1, true)", tenant); err != nil {
		t.Fatalf("set tenant: %v", err)
	}
	fn(tx)
	_ = tx.Commit()
}

// The headline property: a query with no tenant set returns nothing.
//
// This is the failure mode that matters. A forgotten scope must starve, not
// disclose.
func TestRlsUnsetTenantSeesNothing(t *testing.T) {
	db := openPG(t)
	seedTwoTenants(t, db)

	var n int
	if err := db.QueryRow(`SELECT count(*) FROM file_instances`).Scan(&n); err != nil {
		t.Fatalf("query: %v", err)
	}
	if n != 0 {
		t.Errorf("a query with no tenant setting returned %d rows; RLS must deny by default", n)
	}
}

func TestRlsScopedReadSeesOnlyItsOwnTenant(t *testing.T) {
	db := openPG(t)
	seedTwoTenants(t, db)

	// Give each tenant one artifact.
	withTenant(t, db, "TENANT-ALPHA", func(tx *sql.Tx) {
		_, _ = tx.Exec(`INSERT INTO file_instances (tenant_id,filename,storage_path,size_bytes,sha256_hash,status)
			VALUES ('TENANT-ALPHA','rls-alpha.ach','/p',10,'rls-ha','VALIDATED')`)
	})
	withTenant(t, db, "TENANT-BETA", func(tx *sql.Tx) {
		_, _ = tx.Exec(`INSERT INTO file_instances (tenant_id,filename,storage_path,size_bytes,sha256_hash,status)
			VALUES ('TENANT-BETA','rls-beta.ach','/p',20,'rls-hb','VALIDATED')`)
	})

	withTenant(t, db, "TENANT-ALPHA", func(tx *sql.Tx) {
		rows, err := tx.Query(`SELECT filename, tenant_id FROM file_instances`)
		if err != nil {
			t.Fatal(err)
		}
		defer rows.Close()
		for rows.Next() {
			var name, tenant string
			if err := rows.Scan(&name, &tenant); err != nil {
				t.Fatal(err)
			}
			if tenant != "TENANT-ALPHA" {
				t.Errorf("ALPHA's scoped query returned a row belonging to %s (%s)", tenant, name)
			}
			if strings.Contains(name, "beta") {
				t.Errorf("ALPHA saw BETA's artifact %q", name)
			}
		}
	})
}

// WITH CHECK is what stops a write into another tenant. USING alone would
// filter reads while still permitting the insert.
func TestRlsRefusesCrossTenantInsert(t *testing.T) {
	db := openPG(t)
	seedTwoTenants(t, db)

	tx, err := db.Begin()
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = tx.Rollback() }()

	if _, err := tx.Exec("SELECT set_config('sentinel.tenant_id', 'TENANT-ALPHA', true)"); err != nil {
		t.Fatal(err)
	}
	_, err = tx.Exec(`INSERT INTO file_instances (tenant_id,filename,storage_path,size_bytes,sha256_hash,status)
		VALUES ('TENANT-BETA','rls-planted.ach','/p',1,'rls-hx','VALIDATED')`)
	if err == nil {
		t.Fatalf("ALPHA inserted a row into BETA; WITH CHECK is not enforcing")
	}
	if !strings.Contains(strings.ToLower(err.Error()), "row-level security") {
		t.Errorf("insert failed for the wrong reason: %v", err)
	}
}

// A cross-tenant UPDATE must match zero rows rather than erroring or
// succeeding: the row is simply not visible to this scope.
func TestRlsCrossTenantUpdateAffectsNoRows(t *testing.T) {
	db := openPG(t)
	seedTwoTenants(t, db)

	withTenant(t, db, "TENANT-BETA", func(tx *sql.Tx) {
		_, _ = tx.Exec(`INSERT INTO file_instances (tenant_id,filename,storage_path,size_bytes,sha256_hash,status)
			VALUES ('TENANT-BETA','rls-target.ach','/p',20,'rls-target','VALIDATED')`)
	})

	withTenant(t, db, "TENANT-ALPHA", func(tx *sql.Tx) {
		res, err := tx.Exec(`UPDATE file_instances SET status='REJECTED' WHERE filename='rls-target.ach'`)
		if err != nil {
			t.Fatalf("update: %v", err)
		}
		n, _ := res.RowsAffected()
		if n != 0 {
			t.Errorf("ALPHA updated %d of BETA's rows", n)
		}
	})

	// And the row is untouched.
	withTenant(t, db, "TENANT-BETA", func(tx *sql.Tx) {
		var status string
		if err := tx.QueryRow(`SELECT status FROM file_instances WHERE filename='rls-target.ach'`).Scan(&status); err != nil {
			t.Fatal(err)
		}
		if status != "VALIDATED" {
			t.Errorf("BETA's row was modified across the tenant boundary: status=%s", status)
		}
	})
}

// Counting is a disclosure channel too.
func TestRlsCountsDoNotLeakAcrossTenants(t *testing.T) {
	db := openPG(t)
	seedTwoTenants(t, db)

	var alpha, beta int
	withTenant(t, db, "TENANT-ALPHA", func(tx *sql.Tx) {
		if err := tx.QueryRow(`SELECT count(*) FROM file_instances`).Scan(&alpha); err != nil {
			t.Fatal(err)
		}
	})
	withTenant(t, db, "TENANT-BETA", func(tx *sql.Tx) {
		if err := tx.QueryRow(`SELECT count(*) FROM file_instances`).Scan(&beta); err != nil {
			t.Fatal(err)
		}
	})

	var total int
	if err := db.QueryRow(`SELECT count(*) FROM file_instances`).Scan(&total); err != nil {
		t.Fatal(err)
	}
	if total != 0 {
		t.Errorf("unscoped count returned %d; RLS must deny by default", total)
	}
	// Each tenant sees a nonzero count of its own and cannot infer the other's.
	if alpha == 0 || beta == 0 {
		t.Errorf("scoped counts should be nonzero: alpha=%d beta=%d", alpha, beta)
	}
}

// The application role must not be able to bypass its own policies.
func TestApplicationRoleIsNotSuperuserOrTableOwner(t *testing.T) {
	db := openPG(t)

	var isSuper bool
	if err := db.QueryRow(`SELECT rolsuper FROM pg_roles WHERE rolname = current_user`).Scan(&isSuper); err != nil {
		t.Fatal(err)
	}
	if isSuper {
		t.Errorf("the application connects as a superuser, which bypasses every RLS policy")
	}

	var owns int
	err := db.QueryRow(`
		SELECT count(*) FROM pg_tables
		WHERE schemaname = 'public'
		  AND tablename IN ('partners','file_instances','incidents','audit_events','secret_versions','secret_events',
			                  'tenant_quotas','ingest_idempotency','artifact_access_log')
		  AND tableowner = current_user`).Scan(&owns)
	if err != nil {
		t.Fatal(err)
	}
	if owns > 0 {
		t.Errorf("the application role owns %d of the RLS-protected tables; owners bypass RLS unless FORCE is set", owns)
	}
}

// FORCE ROW LEVEL SECURITY must be on, so ownership is not an accidental bypass.
func TestRlsIsEnabledAndForcedOnEveryProtectedTable(t *testing.T) {
	db := openPG(t)

	for _, table := range []string{"partners", "file_instances", "incidents", "audit_events", "secret_versions", "secret_events",
		"tenant_quotas", "ingest_idempotency", "artifact_access_log"} {
		var enabled, forced bool
		err := db.QueryRow(
			`SELECT relrowsecurity, relforcerowsecurity FROM pg_class WHERE relname = $1`, table,
		).Scan(&enabled, &forced)
		if err != nil {
			t.Fatalf("%s: %v", table, err)
		}
		if !enabled {
			t.Errorf("%s does not have row-level security enabled", table)
		}
		if !forced {
			t.Errorf("%s does not FORCE row-level security; the owner would bypass it", table)
		}
	}
}

// Credential metadata is tenant-scoped like every other business record.
//
// It matters more here than elsewhere: the metadata says which credentials a
// tenant holds, when each was last used and when it was last rotated. That is
// an operational map of another organisation's security posture even though it
// contains no credential.
func TestRlsProtectsCredentialMetadata(t *testing.T) {
	db := openPG(t)
	seedTwoTenants(t, db)

	insert := func(tx *sql.Tx, tenant, name string) (sql.Result, error) {
		return tx.Exec(`
			INSERT INTO secret_versions
				(secret_id, tenant_id, name, kind, version, fingerprint, salt, digest, created_by)
			VALUES ($1, $2, $3, 'VERIFY', 1, 'sfp_rls', '\x00', '\x01', 'rls-test')`,
			"sec_"+tenant+"_"+name, tenant, name)
	}

	withTenant(t, db, "TENANT-ALPHA", func(tx *sql.Tx) {
		if _, err := insert(tx, "TENANT-ALPHA", "rls-alpha-secret"); err != nil {
			t.Logf("seed note: %v", err)
		}
	})

	// A query with no tenant set returns nothing.
	var unscoped int
	if err := db.QueryRow(`SELECT count(*) FROM secret_versions`).Scan(&unscoped); err != nil {
		t.Fatal(err)
	}
	if unscoped != 0 {
		t.Errorf("an unscoped query returned %d credential records; RLS must deny by default", unscoped)
	}

	// BETA cannot see ALPHA's credential metadata.
	withTenant(t, db, "TENANT-BETA", func(tx *sql.Tx) {
		var n int
		if err := tx.QueryRow(`SELECT count(*) FROM secret_versions WHERE name = 'rls-alpha-secret'`).Scan(&n); err != nil {
			t.Fatal(err)
		}
		if n != 0 {
			t.Errorf("BETA can see %d of ALPHA's credential records", n)
		}
	})

	// And cannot plant one in ALPHA's tenant.
	withTenant(t, db, "TENANT-BETA", func(tx *sql.Tx) {
		if _, err := insert(tx, "TENANT-ALPHA", "planted"); err == nil {
			t.Error("BETA inserted a credential record into ALPHA")
		} else if !strings.Contains(strings.ToLower(err.Error()), "row-level security") {
			t.Errorf("the insert failed for the wrong reason: %v", err)
		}
	})
}

// Credential material is immutable and the rotation trail is append-only. Both
// are enforced by triggers in the PostgreSQL schema, so a database-level actor
// cannot rewrite the record of what a credential was or when it changed.
func TestSecretMaterialAndAuditTrailCannotBeRewritten(t *testing.T) {
	db := openPG(t)
	seedTwoTenants(t, db)

	withTenant(t, db, "TENANT-ALPHA", func(tx *sql.Tx) {
		_, _ = tx.Exec(`
			INSERT INTO secret_versions
				(secret_id, tenant_id, name, kind, version, fingerprint, salt, digest, created_by)
			VALUES ('sec_immutable','TENANT-ALPHA','rls-immutable','VERIFY',1,'sfp_i','\x00','\x01','rls-test')`)
		_, _ = tx.Exec(`
			INSERT INTO secret_events (tenant_id, secret_id, name, version, action, actor)
			VALUES ('TENANT-ALPHA','sec_immutable','rls-immutable',1,'SECRET_CREATED','rls-test')`)
	})

	withTenant(t, db, "TENANT-ALPHA", func(tx *sql.Tx) {
		if _, err := tx.Exec(`UPDATE secret_versions SET digest = '\x99' WHERE secret_id = 'sec_immutable'`); err == nil {
			t.Error("credential material was rewritten in place")
		}
	})

	// A lifecycle column may still change; that is what rotation and last-used
	// stamping need.
	withTenant(t, db, "TENANT-ALPHA", func(tx *sql.Tx) {
		if _, err := tx.Exec(`UPDATE secret_versions SET last_used_at = now() WHERE secret_id = 'sec_immutable'`); err != nil {
			t.Errorf("a lifecycle update was refused, which would break last-used tracking: %v", err)
		}
	})

	// The audit trail refuses modification. The application role holds no
	// DELETE grant either, so this is defended twice.
	withTenant(t, db, "TENANT-ALPHA", func(tx *sql.Tx) {
		if _, err := tx.Exec(`UPDATE secret_events SET actor = 'someone-else' WHERE secret_id = 'sec_immutable'`); err == nil {
			t.Error("the rotation trail was modified; it is not append-only")
		}
	})
}
