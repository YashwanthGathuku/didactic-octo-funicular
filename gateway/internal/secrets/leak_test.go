package secrets

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"strings"
	"testing"
	"time"

	_ "modernc.org/sqlite"
)

func marshalToString(t *testing.T, v any) string {
	t.Helper()
	b, err := json.Marshal(v)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	return string(b)
}

// The strongest available statement about durable storage: create a credential,
// then read every byte of every column of every table and assert the credential
// is not among them.
//
// This is the test that would have caught the removed webhook subsystem, which
// stored signing secrets as plain text in a column a SQL console endpoint could
// select. It does not depend on knowing which column might hold it -- a new
// column added later is covered automatically.
func TestNoCredentialMaterialReachesTheDatabase(t *testing.T) {
	store := NewTestSQLStore(t)
	ctx := context.Background()
	s := adminScope(t, "TENANT-A")

	_, verified, err := store.Create(ctx, s, CreateRequest{Name: "api-token", Kind: KindVerify})
	if err != nil {
		t.Fatal(err)
	}
	imported, err := New("Qv4T8mnR-partner-signing-material-3a7")
	if err != nil {
		t.Fatal(err)
	}
	if _, _, err := store.Create(ctx, s, CreateRequest{
		Name: "signing-key", Kind: KindRetrieve, Import: imported,
	}); err != nil {
		t.Fatal(err)
	}

	// Exercise the paths that write: rotation, verification and use.
	if _, _, err := store.Rotate(ctx, s, "api-token", time.Hour); err != nil {
		t.Fatal(err)
	}
	if _, err := store.Verify(ctx, "TENANT-A", "api-token", verified.Expose()); err != nil {
		t.Fatal(err)
	}
	if err := store.Use(ctx, s, "signing-key", func(Value) error { return nil }); err != nil {
		t.Fatal(err)
	}

	dump := dumpEveryCell(t, store.db)
	for _, secret := range []string{verified.Expose(), imported.Expose()} {
		if strings.Contains(dump, secret) {
			t.Errorf("a credential is stored in plain text in the database")
		}
		// Any substantial run of the credential would be enough to recover it.
		if len(secret) >= 12 && strings.Contains(dump, secret[:12]) {
			t.Errorf("a 12-character prefix of a credential appears in the database")
		}
	}
}

// dumpEveryCell renders every value in every user table as text. Values that do
// not decode as UTF-8 are included as raw bytes, so ciphertext that happens to
// contain the plaintext would still be caught.
func dumpEveryCell(t *testing.T, db *sql.DB) string {
	t.Helper()

	tables, err := db.Query(`SELECT name FROM sqlite_master WHERE type='table' AND name NOT LIKE 'sqlite_%'`)
	if err != nil {
		t.Fatal(err)
	}
	var names []string
	for tables.Next() {
		var n string
		if err := tables.Scan(&n); err != nil {
			t.Fatal(err)
		}
		names = append(names, n)
	}
	tables.Close()
	if len(names) == 0 {
		t.Fatal("no tables found; the dump would trivially pass")
	}

	var out bytes.Buffer
	for _, name := range names {
		rows, err := db.Query(`SELECT * FROM "` + name + `"`)
		if err != nil {
			t.Fatalf("%s: %v", name, err)
		}
		cols, err := rows.Columns()
		if err != nil {
			t.Fatal(err)
		}
		for rows.Next() {
			cells := make([]any, len(cols))
			ptrs := make([]any, len(cols))
			for i := range cells {
				ptrs[i] = &cells[i]
			}
			if err := rows.Scan(ptrs...); err != nil {
				t.Fatal(err)
			}
			for i, c := range cells {
				switch v := c.(type) {
				case []byte:
					fmt.Fprintf(&out, "%s.%s=%s\n", name, cols[i], v)
				default:
					fmt.Fprintf(&out, "%s.%s=%v\n", name, cols[i], v)
				}
			}
		}
		rows.Close()
	}
	return out.String()
}

// A reporting or support export is built by selecting columns, often with
// SELECT *. Nothing selectable may be a credential, and the columns that exist
// must be the safe ones.
func TestSecretTablesExposeOnlySafeColumns(t *testing.T) {
	store := NewTestSQLStore(t)
	ctx := context.Background()
	s := adminScope(t, "TENANT-A")

	if _, _, err := store.Create(ctx, s, CreateRequest{Name: "api-token", Kind: KindVerify}); err != nil {
		t.Fatal(err)
	}

	rows, err := store.db.Query(`SELECT name FROM pragma_table_info('secret_versions')`)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()

	got := map[string]bool{}
	for rows.Next() {
		var n string
		if err := rows.Scan(&n); err != nil {
			t.Fatal(err)
		}
		got[n] = true
	}

	// A column whose name suggests it holds a value is a review flag. The point
	// is not the names themselves but that adding one fails this test and
	// forces the decision to be made deliberately.
	for _, forbidden := range []string{"value", "secret", "token", "plaintext", "password", "raw"} {
		if got[forbidden] {
			t.Errorf("secret_versions has a column named %q; credential material must not be stored in a readable column", forbidden)
		}
	}
	for _, required := range []string{"fingerprint", "last_used_at", "rotated_at", "retired_at", "not_after"} {
		if !got[required] {
			t.Errorf("secret_versions is missing the %q column", required)
		}
	}
}

// Rotation evidence must survive an attempt to rewrite it. The trigger in
// migration 003 is what enforces this; without it a "rotation" could be
// implemented as an UPDATE that destroys the previous version's record.
func TestStoredCredentialMaterialCannotBeRewrittenInPlace(t *testing.T) {
	store := NewTestSQLStore(t)
	ctx := context.Background()
	s := adminScope(t, "TENANT-A")

	if _, _, err := store.Create(ctx, s, CreateRequest{Name: "api-token", Kind: KindVerify}); err != nil {
		t.Fatal(err)
	}

	_, err := store.db.Exec(`UPDATE secret_versions SET digest = X'00' WHERE version = 1`)
	if err == nil {
		t.Error("credential material was rewritten in place; rotation history can be destroyed")
	}
	_, err = store.db.Exec(`UPDATE secret_versions SET fingerprint = 'sfp_0000000000000000' WHERE version = 1`)
	if err == nil {
		t.Error("a fingerprint was rewritten in place, breaking the link between audit records and the credential")
	}

	// The audit trail is append-only.
	if _, err := store.db.Exec(`DELETE FROM secret_events`); err == nil {
		t.Error("secret_events rows were deleted; the rotation trail is not append-only")
	}
	if _, err := store.db.Exec(`UPDATE secret_events SET actor = 'someone-else'`); err == nil {
		t.Error("a secret_events row was modified; the rotation trail is not append-only")
	}
}

// The schema must refuse a half-populated row: a version with no digest and no
// ciphertext would appear active while verifying nothing.
func TestSchemaRefusesAVersionWithNoStoredMaterial(t *testing.T) {
	store := NewTestSQLStore(t)

	_, err := store.db.Exec(`
		INSERT INTO secret_versions (secret_id, tenant_id, name, kind, version, fingerprint, created_by)
		VALUES ('sec_x','TENANT-A','empty','VERIFY',1,'sfp_x','tester')`)
	if err == nil {
		t.Error("a VERIFY version with no salt or digest was accepted")
	}

	_, err = store.db.Exec(`
		INSERT INTO secret_versions (secret_id, tenant_id, name, kind, version, fingerprint, created_by, salt, digest, sealed, key_id)
		VALUES ('sec_y','TENANT-A','both','VERIFY',1,'sfp_y','tester',X'00',X'01',X'02','key_1')`)
	if err == nil {
		t.Error("a version carrying both a digest and ciphertext was accepted")
	}
}

// Sealing must be authenticated: an attacker with write access to the database
// must not be able to modify a stored credential and observe the result.
func TestSealedCredentialsFailAuthenticationWhenModified(t *testing.T) {
	key := make([]byte, 32)
	for i := range key {
		key[i] = byte(i)
	}
	sealer, err := NewAESSealer(key)
	if err != nil {
		t.Fatal(err)
	}

	sealed, err := sealer.Seal([]byte("a-credential-worth-protecting-01"))
	if err != nil {
		t.Fatal(err)
	}
	if bytes.Contains(sealed, []byte("credential")) {
		t.Error("the sealed form contains the plaintext")
	}

	modified := append([]byte(nil), sealed...)
	modified[len(modified)-1] ^= 0x01
	if _, err := sealer.Open(modified); err == nil {
		t.Error("modified ciphertext was accepted; the mode is not authenticated")
	}

	// A different key must not open it, and must not say why.
	other := make([]byte, 32)
	for i := range other {
		other[i] = byte(i + 100)
	}
	otherSealer, _ := NewAESSealer(other)
	if _, err := otherSealer.Open(sealed); err == nil {
		t.Error("a different key opened the sealed credential")
	}
}

// A process-scoped seal key must be refused wherever storage outlives the
// process, or a restart would render every stored credential unrecoverable.
func TestEphemeralSealKeyIsRefusedForDurableStorage(t *testing.T) {
	ephemeral, err := NewEphemeralSealer()
	if err != nil {
		t.Fatal(err)
	}
	if err := RequireDurableSealer(ephemeral); err == nil {
		t.Error("a process-scoped seal key was accepted for durable storage")
	}
	if err := RequireDurableSealer(nil); err == nil {
		t.Error("a nil sealer was accepted")
	}

	key := make([]byte, 32)
	durable, err := NewAESSealer(key)
	if err != nil {
		t.Fatal(err)
	}
	if err := RequireDurableSealer(durable); err != nil {
		t.Errorf("a configured key was refused: %v", err)
	}
}

func TestSealerRefusesAWrongLengthKey(t *testing.T) {
	for _, n := range []int{0, 16, 24, 31, 33, 64} {
		if _, err := NewAESSealer(make([]byte, n)); err == nil {
			t.Errorf("a %d-byte seal key was accepted; AES-256 requires exactly 32", n)
		}
	}
	if _, err := SealerFromBase64("not-base64-at-all!!"); err == nil {
		t.Error("a malformed base64 seal key was accepted")
	}
	if _, err := SealerFromBase64("c2hvcnQ="); err == nil {
		t.Error("a base64 key of the wrong length was accepted")
	}
}
