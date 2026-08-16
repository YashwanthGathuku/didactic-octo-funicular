package secrets

import (
	"context"
	"database/sql"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"sentinel-gateway/internal/auth"

	_ "modernc.org/sqlite"
)

// Every property below is checked against both adapters.
//
// A contract verified against one implementation is a description of that
// implementation. The in-memory store is what a developer runs; the SQL store is
// what production runs; a difference between them in any of these properties is
// a security difference that would only appear after deployment.

type factory struct {
	name  string
	build func(t *testing.T) Store
	// setClock lets a test drive rotation overlap without sleeping.
	setClock func(s Store, fn func() time.Time)
}

func adapters() []factory {
	return []factory{
		{
			name: "memory",
			build: func(t *testing.T) Store {
				t.Helper()
				s, err := NewMemoryStore()
				if err != nil {
					t.Fatal(err)
				}
				return s
			},
			setClock: func(s Store, fn func() time.Time) { s.(*MemoryStore).SetClock(fn) },
		},
		{
			name: "sql",
			build: func(t *testing.T) Store {
				t.Helper()
				return NewTestSQLStore(t)
			},
			setClock: func(s Store, fn func() time.Time) { s.(*SQLStore).SetClock(fn) },
		},
	}
}

// NewTestSQLStore applies the real migration rather than a hand-written schema.
//
// An inline copy of the schema in a test drifts from the shipped one, and the
// CHECK constraints and immutability trigger in migration 003 are part of what
// is under test here -- a test against a simplified table would prove nothing
// about them.
func NewTestSQLStore(t *testing.T) *SQLStore {
	t.Helper()
	db, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })

	path := filepath.Join("..", "..", "migrations", "003_secret_store.sql")
	body, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	if _, err := db.Exec(string(body)); err != nil {
		t.Fatalf("apply migration: %v", err)
	}

	key := make([]byte, 32)
	for i := range key {
		key[i] = byte(i + 1)
	}
	sealer, err := NewAESSealer(key)
	if err != nil {
		t.Fatal(err)
	}
	store, err := NewSQLStore(db, sealer)
	if err != nil {
		t.Fatal(err)
	}
	return store
}

func adminScope(t *testing.T, tenant string) Scope {
	t.Helper()
	p := &auth.Principal{
		Subject: "auth0|admin",
		Issuer:  "https://issuer.example",
		Memberships: []auth.Membership{
			{TenantID: tenant, Roles: []auth.Role{auth.RoleTenantAdmin}},
		},
	}
	s, err := NewScope(p, tenant)
	if err != nil {
		t.Fatalf("NewScope: %v", err)
	}
	return s
}

func eachAdapter(t *testing.T, fn func(t *testing.T, f factory)) {
	t.Helper()
	for _, f := range adapters() {
		t.Run(f.name, func(t *testing.T) { fn(t, f) })
	}
}

// Write-once display: Create returns the value, and nothing else ever can.
func TestCreateShowsTheValueExactlyOnce(t *testing.T) {
	eachAdapter(t, func(t *testing.T, f factory) {
		store := f.build(t)
		ctx := context.Background()
		s := adminScope(t, "TENANT-A")

		ref, value, err := store.Create(ctx, s, CreateRequest{Name: "api-token", Kind: KindVerify})
		if err != nil {
			t.Fatal(err)
		}
		created := value.Expose()
		if created == "" {
			t.Fatal("Create returned an empty value")
		}
		if ref.Fingerprint == "" {
			t.Error("the reference carries no fingerprint, so no audit record can identify this credential")
		}

		// Every subsequent read path returns metadata only. This is the
		// property the removed webhook subsystem violated: its list endpoint
		// returned every stored secret in full.
		got, err := store.Get(ctx, s, "api-token")
		if err != nil {
			t.Fatal(err)
		}
		assertReferenceCarriesNoSecret(t, got, created)

		list, err := store.List(ctx, s)
		if err != nil {
			t.Fatal(err)
		}
		if len(list) != 1 {
			t.Fatalf("List returned %d references, want 1", len(list))
		}
		assertReferenceCarriesNoSecret(t, list[0], created)
	})
}

// assertReferenceCarriesNoSecret checks every string field and the marshalled
// form, so a field added later is covered without editing this helper.
func assertReferenceCarriesNoSecret(t *testing.T, ref Reference, secret string) {
	t.Helper()
	blob := marshalToString(t, ref)
	if strings.Contains(blob, secret) {
		t.Errorf("a Reference disclosed the credential: %s", blob)
	}
	// A prefix would be enough to make an offline search feasible.
	if len(secret) >= 8 && strings.Contains(blob, secret[:8]) {
		t.Errorf("a Reference disclosed a prefix of the credential: %s", blob)
	}
}

func TestVerifyAcceptsTheCredentialAndNothingElse(t *testing.T) {
	eachAdapter(t, func(t *testing.T, f factory) {
		store := f.build(t)
		ctx := context.Background()
		s := adminScope(t, "TENANT-A")

		_, value, err := store.Create(ctx, s, CreateRequest{Name: "api-token", Kind: KindVerify})
		if err != nil {
			t.Fatal(err)
		}

		if _, err := store.Verify(ctx, "TENANT-A", "api-token", value.Expose()); err != nil {
			t.Fatalf("the correct credential was rejected: %v", err)
		}

		wrong := []struct{ label, presented string }{
			{"wrong value", "not-the-credential-not-the-credential"},
			{"empty", ""},
			{"one character short", value.Expose()[:len(value.Expose())-1]},
			{"one character longer", value.Expose() + "x"},
		}
		for _, w := range wrong {
			if _, err := store.Verify(ctx, "TENANT-A", "api-token", w.presented); !errors.Is(err, ErrVerificationFailed) {
				t.Errorf("%s: got %v, want ErrVerificationFailed", w.label, err)
			}
		}
	})
}

// A verification failure must not say which part was wrong. Distinguishing an
// unknown name from a wrong value turns the endpoint into an oracle for
// enumerating what credentials exist.
func TestVerifyFailuresAreIndistinguishable(t *testing.T) {
	eachAdapter(t, func(t *testing.T, f factory) {
		store := f.build(t)
		ctx := context.Background()
		s := adminScope(t, "TENANT-A")

		_, value, err := store.Create(ctx, s, CreateRequest{Name: "api-token", Kind: KindVerify})
		if err != nil {
			t.Fatal(err)
		}

		cases := map[string]error{}
		for label, args := range map[string][3]string{
			"unknown name":            {"TENANT-A", "no-such-secret", value.Expose()},
			"unknown tenant":          {"TENANT-NOPE", "api-token", value.Expose()},
			"right name, wrong value": {"TENANT-A", "api-token", "wrong-wrong-wrong-wrong-wrong-wr"},
			"another tenant's name":   {"TENANT-B", "api-token", value.Expose()},
		} {
			_, err := store.Verify(ctx, args[0], args[1], args[2])
			if err == nil {
				t.Fatalf("%s: verification succeeded when it must not", label)
			}
			cases[label] = err
		}

		var texts []string
		for label, err := range cases {
			if !errors.Is(err, ErrVerificationFailed) {
				t.Errorf("%s returned %v, not ErrVerificationFailed", label, err)
			}
			texts = append(texts, err.Error())
		}
		for _, text := range texts {
			if text != texts[0] {
				t.Errorf("verification failures are distinguishable by message: %q vs %q", text, texts[0])
			}
		}
	})
}

// Rotation must not lock out a caller that has not yet picked up the new
// credential. Without an overlap window, rotations get postponed forever.
func TestRotationOverlapKeepsThePreviousCredentialValid(t *testing.T) {
	eachAdapter(t, func(t *testing.T, f factory) {
		store := f.build(t)
		ctx := context.Background()
		s := adminScope(t, "TENANT-A")

		clock := time.Date(2026, 3, 1, 12, 0, 0, 0, time.UTC)
		f.setClock(store, func() time.Time { return clock })

		_, old, err := store.Create(ctx, s, CreateRequest{Name: "api-token", Kind: KindVerify})
		if err != nil {
			t.Fatal(err)
		}

		ref, fresh, err := store.Rotate(ctx, s, "api-token", time.Hour)
		if err != nil {
			t.Fatal(err)
		}
		if ref.Version != 2 {
			t.Errorf("rotation produced version %d, want 2", ref.Version)
		}
		if fresh.Expose() == old.Expose() {
			t.Fatal("rotation returned the same credential")
		}
		if ref.RotatedAt == nil {
			t.Error("the rotated reference has no rotation timestamp")
		}

		// Inside the window both work.
		if _, err := store.Verify(ctx, "TENANT-A", "api-token", fresh.Expose()); err != nil {
			t.Errorf("the new credential was rejected during overlap: %v", err)
		}
		if _, err := store.Verify(ctx, "TENANT-A", "api-token", old.Expose()); err != nil {
			t.Errorf("the previous credential was rejected during the overlap window: %v", err)
		}

		// After the window only the new one works.
		clock = clock.Add(2 * time.Hour)
		if _, err := store.Verify(ctx, "TENANT-A", "api-token", fresh.Expose()); err != nil {
			t.Errorf("the new credential was rejected after the overlap: %v", err)
		}
		if _, err := store.Verify(ctx, "TENANT-A", "api-token", old.Expose()); !errors.Is(err, ErrVerificationFailed) {
			t.Error("the previous credential still verifies after its overlap expired")
		}
	})
}

// Retire cuts an overlap short. This is the response to a suspected compromise,
// so it must take effect immediately rather than at the end of the window.
func TestRetireEndsAVersionImmediately(t *testing.T) {
	eachAdapter(t, func(t *testing.T, f factory) {
		store := f.build(t)
		ctx := context.Background()
		s := adminScope(t, "TENANT-A")

		_, old, err := store.Create(ctx, s, CreateRequest{Name: "api-token", Kind: KindVerify})
		if err != nil {
			t.Fatal(err)
		}
		if _, _, err := store.Rotate(ctx, s, "api-token", 24*time.Hour); err != nil {
			t.Fatal(err)
		}
		if _, err := store.Verify(ctx, "TENANT-A", "api-token", old.Expose()); err != nil {
			t.Fatalf("precondition: the old credential should still verify: %v", err)
		}

		if err := store.Retire(ctx, s, "api-token", 1); err != nil {
			t.Fatal(err)
		}
		if _, err := store.Verify(ctx, "TENANT-A", "api-token", old.Expose()); !errors.Is(err, ErrVerificationFailed) {
			t.Error("a retired credential still verifies")
		}
		// Retiring twice is not an error; a compromise response must be safe to
		// run again without an operator wondering whether it worked.
		if err := store.Retire(ctx, s, "api-token", 1); err != nil {
			t.Errorf("Retire is not idempotent: %v", err)
		}
	})
}

// Rotating with a zero overlap is the immediate-cutover case: the old value
// stops working at once.
func TestRotationWithNoOverlapRetiresThePreviousVersion(t *testing.T) {
	eachAdapter(t, func(t *testing.T, f factory) {
		store := f.build(t)
		ctx := context.Background()
		s := adminScope(t, "TENANT-A")

		_, old, err := store.Create(ctx, s, CreateRequest{Name: "api-token", Kind: KindVerify})
		if err != nil {
			t.Fatal(err)
		}
		if _, _, err := store.Rotate(ctx, s, "api-token", 0); err != nil {
			t.Fatal(err)
		}
		if _, err := store.Verify(ctx, "TENANT-A", "api-token", old.Expose()); !errors.Is(err, ErrVerificationFailed) {
			t.Error("a zero-overlap rotation left the previous credential valid")
		}
	})
}

// Rotation evidence: an auditor must be able to answer "when did this change,
// and who changed it" without anyone handling the credential.
func TestRotationLeavesAuditEvidence(t *testing.T) {
	eachAdapter(t, func(t *testing.T, f factory) {
		store := f.build(t)
		ctx := context.Background()
		s := adminScope(t, "TENANT-A")

		_, value, err := store.Create(ctx, s, CreateRequest{Name: "api-token", Kind: KindVerify})
		if err != nil {
			t.Fatal(err)
		}
		if _, _, err := store.Rotate(ctx, s, "api-token", time.Hour); err != nil {
			t.Fatal(err)
		}
		if err := store.Retire(ctx, s, "api-token", 1); err != nil {
			t.Fatal(err)
		}

		events, err := store.Events(ctx, s, "api-token")
		if err != nil {
			t.Fatal(err)
		}

		seen := map[string]bool{}
		for _, e := range events {
			seen[e.Action] = true
			if e.Actor == "" {
				t.Errorf("event %s has no actor; unattributable evidence is not evidence", e.Action)
			}
		}
		for _, want := range []string{ActionCreated, ActionRotated, ActionRetired} {
			if !seen[want] {
				t.Errorf("no %s event was recorded", want)
			}
		}

		blob := marshalToString(t, events)
		if strings.Contains(blob, value.Expose()) {
			t.Errorf("the audit trail contains the credential: %s", blob)
		}
	})
}

// Last-used answers "is this credential still in service?", which is what makes
// retiring an old one safe. It must be recorded without exposing the value.
func TestVerificationStampsLastUsed(t *testing.T) {
	eachAdapter(t, func(t *testing.T, f factory) {
		store := f.build(t)
		ctx := context.Background()
		s := adminScope(t, "TENANT-A")

		clock := time.Date(2026, 3, 1, 9, 0, 0, 0, time.UTC)
		f.setClock(store, func() time.Time { return clock })

		_, value, err := store.Create(ctx, s, CreateRequest{Name: "api-token", Kind: KindVerify})
		if err != nil {
			t.Fatal(err)
		}
		before, err := store.Get(ctx, s, "api-token")
		if err != nil {
			t.Fatal(err)
		}
		if before.LastUsedAt != nil {
			t.Error("a credential that has never been presented reports a last-used time")
		}

		clock = clock.Add(90 * time.Minute)
		if _, err := store.Verify(ctx, "TENANT-A", "api-token", value.Expose()); err != nil {
			t.Fatal(err)
		}

		after, err := store.Get(ctx, s, "api-token")
		if err != nil {
			t.Fatal(err)
		}
		if after.LastUsedAt == nil {
			t.Fatal("last-used was not recorded after a successful verification")
		}
		if !after.LastUsedAt.Equal(clock) {
			t.Errorf("last-used is %v, want %v", after.LastUsedAt, clock)
		}
	})
}

// A KindVerify secret is stored one-way. Use must refuse it rather than
// returning something that looks like the credential.
func TestUseRefusesADigestOnlySecret(t *testing.T) {
	eachAdapter(t, func(t *testing.T, f factory) {
		store := f.build(t)
		ctx := context.Background()
		s := adminScope(t, "TENANT-A")

		if _, _, err := store.Create(ctx, s, CreateRequest{Name: "api-token", Kind: KindVerify}); err != nil {
			t.Fatal(err)
		}
		err := store.Use(ctx, s, "api-token", func(Value) error {
			t.Error("Use handed out a credential that is stored only as a digest")
			return nil
		})
		if !errors.Is(err, ErrNotRetrievable) {
			t.Errorf("got %v, want ErrNotRetrievable", err)
		}
	})
}

// A KindRetrieve secret round-trips through the sealer and is usable.
func TestUseReturnsARetrievableSecret(t *testing.T) {
	eachAdapter(t, func(t *testing.T, f factory) {
		store := f.build(t)
		ctx := context.Background()
		s := adminScope(t, "TENANT-A")

		imported, err := New("Zx7Z3lqB-provider-issued-material-9f2")
		if err != nil {
			t.Fatal(err)
		}
		if _, _, err := store.Create(ctx, s, CreateRequest{
			Name: "oidc-client-secret", Kind: KindRetrieve, Import: imported,
		}); err != nil {
			t.Fatal(err)
		}

		var got string
		if err := store.Use(ctx, s, "oidc-client-secret", func(v Value) error {
			got = v.Expose()
			return nil
		}); err != nil {
			t.Fatal(err)
		}
		if got != imported.Expose() {
			t.Error("Use returned a different value than was stored")
		}

		ref, err := store.Get(ctx, s, "oidc-client-secret")
		if err != nil {
			t.Fatal(err)
		}
		if ref.LastUsedAt == nil {
			t.Error("Use did not stamp last-used")
		}
		assertReferenceCarriesNoSecret(t, ref, imported.Expose())
	})
}

// Tenant isolation, at this boundary as at every other: a foreign secret is
// indistinguishable from one that does not exist.
func TestSecretsAreTenantScoped(t *testing.T) {
	eachAdapter(t, func(t *testing.T, f factory) {
		store := f.build(t)
		ctx := context.Background()
		a := adminScope(t, "TENANT-A")
		b := adminScope(t, "TENANT-B")

		if _, _, err := store.Create(ctx, a, CreateRequest{Name: "api-token", Kind: KindVerify}); err != nil {
			t.Fatal(err)
		}

		_, foreignErr := store.Get(ctx, b, "api-token")
		_, absentErr := store.Get(ctx, b, "no-such-secret")
		if !errors.Is(foreignErr, ErrSecretNotFound) {
			t.Errorf("B read A's secret metadata: %v", foreignErr)
		}
		if foreignErr.Error() != absentErr.Error() {
			t.Errorf("a foreign secret (%v) is distinguishable from an absent one (%v); the error confirms another tenant holds it",
				foreignErr, absentErr)
		}

		list, err := store.List(ctx, b)
		if err != nil {
			t.Fatal(err)
		}
		if len(list) != 0 {
			t.Errorf("B listed %d of A's secrets", len(list))
		}

		if err := store.Retire(ctx, b, "api-token", 1); !errors.Is(err, ErrSecretNotFound) {
			t.Errorf("B retired A's credential: %v", err)
		}
		if _, _, err := store.Rotate(ctx, b, "api-token", time.Hour); !errors.Is(err, ErrSecretNotFound) {
			t.Errorf("B rotated A's credential: %v", err)
		}
	})
}

// A zero Scope is what a handler that forgot to authorize looks like.
func TestZeroScopeCannotReachAnySecret(t *testing.T) {
	eachAdapter(t, func(t *testing.T, f factory) {
		store := f.build(t)
		ctx := context.Background()
		var zero Scope

		if _, _, err := store.Create(ctx, zero, CreateRequest{Name: "x", Kind: KindVerify}); err == nil {
			t.Error("a zero Scope created a secret")
		}
		if _, err := store.Get(ctx, zero, "x"); err == nil {
			t.Error("a zero Scope read a secret")
		}
		if _, err := store.List(ctx, zero); err == nil {
			t.Error("a zero Scope listed secrets")
		}
		if _, _, err := store.Rotate(ctx, zero, "x", time.Hour); err == nil {
			t.Error("a zero Scope rotated a secret")
		}
		if err := store.Use(ctx, zero, "x", func(Value) error { return nil }); err == nil {
			t.Error("a zero Scope used a secret")
		}
	})
}

// Creating over a live credential would break every holder of the old one with
// no record that it happened. Rotate is the supported path.
func TestCreateRefusesToOverwriteALiveCredential(t *testing.T) {
	eachAdapter(t, func(t *testing.T, f factory) {
		store := f.build(t)
		ctx := context.Background()
		s := adminScope(t, "TENANT-A")

		if _, _, err := store.Create(ctx, s, CreateRequest{Name: "api-token", Kind: KindVerify}); err != nil {
			t.Fatal(err)
		}
		if _, _, err := store.Create(ctx, s, CreateRequest{Name: "api-token", Kind: KindVerify}); !errors.Is(err, ErrAlreadyExists) {
			t.Errorf("a second Create returned %v, want ErrAlreadyExists", err)
		}
	})
}

// Only a role holding secret:manage may administer credentials. A reviewer can
// approve the movement of money and still must not be able to rotate the key
// that authenticates the system.
func TestScopeRequiresTheSecretManagementPermission(t *testing.T) {
	for _, role := range []auth.Role{auth.RoleViewer, auth.RoleOperator, auth.RoleReviewer} {
		p := &auth.Principal{
			Subject:     "auth0|someone",
			Memberships: []auth.Membership{{TenantID: "TENANT-A", Roles: []auth.Role{role}}},
		}
		if _, err := NewScope(p, "TENANT-A"); err == nil {
			t.Errorf("%s was granted a secret-management scope", role)
		}
	}

	admin := &auth.Principal{
		Subject:     "auth0|admin",
		Memberships: []auth.Membership{{TenantID: "TENANT-A", Roles: []auth.Role{auth.RoleTenantAdmin}}},
	}
	if _, err := NewScope(admin, "TENANT-A"); err != nil {
		t.Errorf("tenant_admin was refused a secret-management scope: %v", err)
	}
	if _, err := NewScope(admin, "TENANT-B"); err == nil {
		t.Error("a tenant_admin of A was granted a secret-management scope in B")
	}
	if _, err := NewScope(nil, "TENANT-A"); err == nil {
		t.Error("a nil principal was granted a scope")
	}
}
