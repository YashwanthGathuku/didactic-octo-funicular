package repository

import (
	"context"
	"database/sql"
	"errors"
	"testing"

	"sentinel-gateway/internal/auth"

	_ "modernc.org/sqlite"
)

// The isolation claim this package exists to support: two tenants cannot read,
// infer, update or enumerate each other's records.

func newDB(t *testing.T) *sql.DB {
	t.Helper()
	db, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })

	// Minimal shape matching migration 002 for the tables under test.
	schema := `
	CREATE TABLE tenants (id TEXT PRIMARY KEY, name TEXT NOT NULL);
	CREATE TABLE file_instances (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		tenant_id TEXT NOT NULL REFERENCES tenants(id),
		filename TEXT NOT NULL, storage_path TEXT NOT NULL DEFAULT '/p',
		size_bytes INTEGER NOT NULL, sha256_hash TEXT NOT NULL,
		status TEXT NOT NULL CHECK (status IN ('RECEIVED','VALIDATING','VALIDATED','QUARANTINED','APPROVED','RELEASED','REJECTED')),
		row_version INTEGER NOT NULL DEFAULT 0,
		received_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
		updated_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP
	);
	CREATE TABLE incidents (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		tenant_id TEXT NOT NULL REFERENCES tenants(id),
		type TEXT NOT NULL, severity TEXT NOT NULL,
		status TEXT NOT NULL CHECK (status IN ('OPEN','INVESTIGATING','RESOLVED','CLOSED'))
	);
	CREATE TABLE status_history (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		tenant_id TEXT NOT NULL, object_type TEXT NOT NULL, object_id INTEGER NOT NULL,
		from_state TEXT NOT NULL, to_state TEXT NOT NULL, actor_id TEXT NOT NULL,
		reason TEXT, occurred_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
		CHECK (from_state <> to_state)
	);
	INSERT INTO tenants (id,name) VALUES ('TENANT-A','A'),('TENANT-B','B');
	INSERT INTO file_instances (tenant_id, filename, size_bytes, sha256_hash, status)
		VALUES ('TENANT-A','a1.ach',10,'hash-a1','VALIDATED'),
		       ('TENANT-A','a2.ach',20,'hash-a2','QUARANTINED'),
		       ('TENANT-B','b1.ach',30,'hash-b1','VALIDATED');
	INSERT INTO incidents (tenant_id,type,severity,status)
		VALUES ('TENANT-A','X','HIGH','OPEN'),('TENANT-B','Y','HIGH','OPEN');
	`
	if _, err := db.Exec(schema); err != nil {
		t.Fatal(err)
	}
	return db
}

func scopeFor(t *testing.T, tenant string, role auth.Role, perm auth.Permission) Scope {
	t.Helper()
	p := &auth.Principal{
		Subject:     "user-" + tenant,
		Memberships: []auth.Membership{{TenantID: tenant, Roles: []auth.Role{role}}},
	}
	s, err := NewScope(p, tenant, perm)
	if err != nil {
		t.Fatalf("NewScope(%s): %v", tenant, err)
	}
	return s
}

func TestListReturnsOnlyOwnTenant(t *testing.T) {
	r := New(newDB(t))
	ctx := context.Background()

	a, err := r.ListArtifacts(ctx, scopeFor(t, "TENANT-A", auth.RoleViewer, auth.PermReadTenant), 100)
	if err != nil {
		t.Fatal(err)
	}
	if len(a) != 2 {
		t.Fatalf("TENANT-A should see 2 artifacts, saw %d", len(a))
	}
	for _, row := range a {
		if row.TenantID != "TENANT-A" {
			t.Errorf("TENANT-A received a row belonging to %s", row.TenantID)
		}
		if row.Filename == "b1.ach" {
			t.Errorf("TENANT-A received TENANT-B's artifact")
		}
	}

	b, err := r.ListArtifacts(ctx, scopeFor(t, "TENANT-B", auth.RoleViewer, auth.PermReadTenant), 100)
	if err != nil {
		t.Fatal(err)
	}
	if len(b) != 1 {
		t.Fatalf("TENANT-B should see 1 artifact, saw %d", len(b))
	}
}

// Horizontal escalation by direct object reference: guessing another tenant's
// primary key must be indistinguishable from guessing a nonexistent one.
func TestCrossTenantIdLookupIsIndistinguishableFromMissing(t *testing.T) {
	r := New(newDB(t))
	ctx := context.Background()
	scopeA := scopeFor(t, "TENANT-A", auth.RoleViewer, auth.PermReadTenant)

	// id 3 is TENANT-B's artifact and exists.
	_, errOther := r.GetArtifact(ctx, scopeA, 3)
	// id 9999 does not exist at all.
	_, errMissing := r.GetArtifact(ctx, scopeA, 9999)

	if !errors.Is(errOther, ErrNotFound) {
		t.Errorf("another tenant's ID must return ErrNotFound, got %v", errOther)
	}
	if !errors.Is(errMissing, ErrNotFound) {
		t.Errorf("a missing ID must return ErrNotFound, got %v", errMissing)
	}
	if errOther.Error() != errMissing.Error() {
		t.Errorf("error text differs between 'another tenant' and 'does not exist', "+
			"which confirms existence:\n  other:   %v\n  missing: %v", errOther, errMissing)
	}
}

// Enumeration: a count must not leak other tenants' volume.
func TestCountIsTenantScoped(t *testing.T) {
	r := New(newDB(t))
	ctx := context.Background()

	nA, err := r.CountArtifacts(ctx, scopeFor(t, "TENANT-A", auth.RoleViewer, auth.PermReadTenant))
	if err != nil {
		t.Fatal(err)
	}
	nB, err := r.CountArtifacts(ctx, scopeFor(t, "TENANT-B", auth.RoleViewer, auth.PermReadTenant))
	if err != nil {
		t.Fatal(err)
	}
	if nA != 2 || nB != 1 {
		t.Errorf("counts leaked across tenants: A=%d B=%d (want 2 and 1)", nA, nB)
	}
}

func TestIncidentsAreTenantScoped(t *testing.T) {
	r := New(newDB(t))
	ctx := context.Background()
	ids, err := r.ListIncidents(ctx, scopeFor(t, "TENANT-A", auth.RoleViewer, auth.PermReadTenant), 100)
	if err != nil {
		t.Fatal(err)
	}
	if len(ids) != 1 {
		t.Errorf("TENANT-A should see exactly its own incident, saw %d", len(ids))
	}
}

// Writes are scoped too: updating another tenant's row must affect nothing.
func TestCrossTenantUpdateAffectsNothing(t *testing.T) {
	db := newDB(t)
	r := New(db)
	ctx := context.Background()
	scopeA := scopeFor(t, "TENANT-A", auth.RoleOperator, auth.PermUploadArtifact)

	// Artifact 3 belongs to TENANT-B.
	err := r.UpdateArtifactStatus(ctx, scopeA, 3, "VALIDATED", "APPROVED", 0, "attempted")
	if !errors.Is(err, ErrNotFound) {
		t.Errorf("cross-tenant update must fail as not-found, got %v", err)
	}

	var status string
	if err := db.QueryRow(`SELECT status FROM file_instances WHERE id = 3`).Scan(&status); err != nil {
		t.Fatal(err)
	}
	if status != "VALIDATED" {
		t.Errorf("TENANT-B's artifact was modified by TENANT-A: status is now %s", status)
	}
}

// Optimistic concurrency: two writers holding the same version, one wins.
func TestConcurrentUpdateOnlyOneWins(t *testing.T) {
	db := newDB(t)
	r := New(db)
	ctx := context.Background()
	s := scopeFor(t, "TENANT-A", auth.RoleOperator, auth.PermUploadArtifact)

	if err := r.UpdateArtifactStatus(ctx, s, 1, "VALIDATED", "APPROVED", 0, "first"); err != nil {
		t.Fatalf("first update should succeed: %v", err)
	}
	// The second writer still believes it is at version 0.
	err := r.UpdateArtifactStatus(ctx, s, 1, "VALIDATED", "REJECTED", 0, "second")
	if err == nil {
		t.Errorf("a stale-version update must not succeed")
	}

	var status string
	if err := db.QueryRow(`SELECT status FROM file_instances WHERE id = 1`).Scan(&status); err != nil {
		t.Fatal(err)
	}
	if status != "APPROVED" {
		t.Errorf("expected the first writer to win, status is %s", status)
	}
}

// A write records the verified subject, never a caller-supplied name.
func TestStatusHistoryRecordsVerifiedActor(t *testing.T) {
	db := newDB(t)
	r := New(db)
	ctx := context.Background()
	s := scopeFor(t, "TENANT-A", auth.RoleOperator, auth.PermUploadArtifact)

	if err := r.UpdateArtifactStatus(ctx, s, 1, "VALIDATED", "APPROVED", 0, "ok"); err != nil {
		t.Fatal(err)
	}
	var actor string
	if err := db.QueryRow(`SELECT actor_id FROM status_history WHERE object_id = 1`).Scan(&actor); err != nil {
		t.Fatal(err)
	}
	if actor != "user-TENANT-A" {
		t.Errorf("history recorded actor %q; must be the verified token subject", actor)
	}
}

// A zero Scope must be unusable. This is the structural guarantee: even if a
// handler forgets authorization, the repository refuses.
func TestZeroScopeIsRefusedByEveryMethod(t *testing.T) {
	r := New(newDB(t))
	ctx := context.Background()
	var zero Scope

	if _, err := r.ListArtifacts(ctx, zero, 10); !errors.Is(err, ErrNoScope) {
		t.Errorf("ListArtifacts accepted a zero scope: %v", err)
	}
	if _, err := r.GetArtifact(ctx, zero, 1); !errors.Is(err, ErrNoScope) {
		t.Errorf("GetArtifact accepted a zero scope: %v", err)
	}
	if _, err := r.CountArtifacts(ctx, zero); !errors.Is(err, ErrNoScope) {
		t.Errorf("CountArtifacts accepted a zero scope: %v", err)
	}
	if _, err := r.ListIncidents(ctx, zero, 10); !errors.Is(err, ErrNoScope) {
		t.Errorf("ListIncidents accepted a zero scope: %v", err)
	}
	if err := r.UpdateArtifactStatus(ctx, zero, 1, "VALIDATED", "APPROVED", 0, "x"); !errors.Is(err, ErrNoScope) {
		t.Errorf("UpdateArtifactStatus accepted a zero scope: %v", err)
	}
}

// A scope cannot be built without the permission it claims.
func TestScopeRequiresPermission(t *testing.T) {
	viewer := &auth.Principal{
		Subject:     "v",
		Memberships: []auth.Membership{{TenantID: "TENANT-A", Roles: []auth.Role{auth.RoleViewer}}},
	}
	if _, err := NewScope(viewer, "TENANT-A", auth.PermReadTenant); err != nil {
		t.Errorf("viewer should get a read scope: %v", err)
	}
	if _, err := NewScope(viewer, "TENANT-A", auth.PermApproveRelease); err == nil {
		t.Errorf("viewer obtained an approve scope")
	}
	if _, err := NewScope(viewer, "TENANT-B", auth.PermReadTenant); err == nil {
		t.Errorf("viewer obtained a scope in a tenant it does not belong to")
	}
	if _, err := NewScope(nil, "TENANT-A", auth.PermReadTenant); err == nil {
		t.Errorf("a nil principal produced a usable scope")
	}
}

// Result sets are bounded regardless of what the caller asks for.
func TestListIsBounded(t *testing.T) {
	r := New(newDB(t))
	ctx := context.Background()
	s := scopeFor(t, "TENANT-A", auth.RoleViewer, auth.PermReadTenant)

	for _, limit := range []int{0, -1, 1000000} {
		rows, err := r.ListArtifacts(ctx, s, limit)
		if err != nil {
			t.Fatalf("limit %d: %v", limit, err)
		}
		if len(rows) > 500 {
			t.Errorf("limit %d returned %d rows; result sets must be bounded", limit, len(rows))
		}
	}
}
