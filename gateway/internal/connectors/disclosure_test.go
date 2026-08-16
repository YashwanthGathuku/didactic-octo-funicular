package connectors

import (
	"encoding/json"
	"fmt"
	"reflect"
	"strings"
	"testing"

	"sentinel-gateway/internal/secrets"
)

// The disclosure tests.
//
// The Integration Hub this platform replaces returned webhook secrets from its
// list endpoint, its create response and its SQL console. Nobody intended that;
// it happened because the secret lived in the same struct as everything else
// and three different code paths marshalled the struct.
//
// These tests are the structural answer: every rendering path is checked
// against a known credential, so a future field added to the wrong struct fails
// here rather than in production.

const (
	// A credential the tests search every output for. Long and distinctive so a
	// partial match is still a hit.
	//
	// secret-scan-allow: a fixture credential for disclosure tests; it authenticates nothing and exists so the tests can search output for it
	fixtureCredential = "correct-horse-battery-staple-fixture-credential-9f2b"
	// secret-scan-allow: a second fixture credential, used to check that one secret's rendering does not disclose another
	fixtureWallet = "wallet-passphrase-fixture-4a71-never-a-real-secret"
)

func fixtureSecrets(t *testing.T) Secrets {
	t.Helper()
	pw, _, err := secrets.NewExternal(fixtureCredential)
	if err != nil {
		t.Fatal(err)
	}
	wallet, _, err := secrets.NewExternal(fixtureWallet)
	if err != nil {
		t.Fatal(err)
	}
	return NewSecrets(map[string]secrets.Value{
		"password":   pw,
		"wallet_ref": wallet,
	})
}

// mustNotContainCredential fails if any fixture credential appears in text.
func mustNotContainCredential(t *testing.T, where, text string) {
	t.Helper()
	for _, cred := range []string{fixtureCredential, fixtureWallet} {
		if strings.Contains(text, cred) {
			t.Errorf("%s discloses a credential", where)
		}
		// A prefix long enough to be recognisable is also a disclosure.
		if len(cred) > 16 && strings.Contains(text, cred[:16]) {
			t.Errorf("%s discloses the first 16 characters of a credential", where)
		}
	}
}

func TestSecretsAreNotDisclosedByAnyFormattingVerb(t *testing.T) {
	sec := fixtureSecrets(t)

	// Every verb fmt offers, including the ones that bypass Stringer. %#v and
	// %p go through paths that dump unexported fields by reflection, which is
	// how the Prompt 05 secret type was originally caught disclosing.
	for _, verb := range []string{"%v", "%s", "%q", "%d", "%x", "%X", "%#v", "%+v", "%T", "%p", "%!"} {
		out := fmt.Sprintf(verb, sec)
		mustNotContainCredential(t, "fmt "+verb, out)
	}
	// And on the containing Config-plus-Secrets pair, which is what a handler
	// would print while debugging.
	cfg := Config{Type: "postgresql", Fields: map[string]string{"host": "db.example.test"}}
	pair := struct {
		Config  Config
		Secrets Secrets
	}{cfg, sec}
	for _, verb := range []string{"%v", "%#v", "%+v"} {
		mustNotContainCredential(t, "the config/secret pair with "+verb, fmt.Sprintf(verb, pair))
	}
}

func TestSecretsRefuseToMarshal(t *testing.T) {
	sec := fixtureSecrets(t)

	// Marshalling must fail rather than emit an empty object. An empty object
	// would let a secret set be dropped silently into an API response that a
	// reader would then believe carried the connection's full state.
	if _, err := json.Marshal(sec); err == nil {
		t.Error("connector secrets marshalled successfully; they must refuse")
	}

	wrapper := struct {
		Name    string  `json:"name"`
		Secrets Secrets `json:"secrets"`
	}{"acme", sec}
	out, err := json.Marshal(wrapper)
	if err == nil {
		mustNotContainCredential(t, "a struct containing Secrets", string(out))
		t.Error("a struct containing Secrets marshalled; the refusal must propagate")
	}
}

func TestReflectionOverTheSecretSetYieldsNothing(t *testing.T) {
	sec := fixtureSecrets(t)

	// Walk every reachable field, as a debug dumper or a structured logger
	// would, reading raw bytes rather than calling Interface().
	//
	// Interface() panics on a value obtained from an unexported field, which is
	// itself a small protection -- but a determined dumper reads the bytes
	// directly, so that is what this does. The values are sealed ciphertext, so
	// nothing recoverable is present even at this depth.
	var walk func(v reflect.Value, depth int) string
	walk = func(v reflect.Value, depth int) string {
		if depth > 8 || !v.IsValid() {
			return ""
		}
		var b strings.Builder
		switch v.Kind() {
		case reflect.Map:
			for _, k := range v.MapKeys() {
				b.WriteString(walk(k, depth+1))
				b.WriteString(walk(v.MapIndex(k), depth+1))
			}
		case reflect.Struct:
			for i := range v.NumField() {
				b.WriteString(walk(v.Field(i), depth+1))
			}
		case reflect.Slice, reflect.Array:
			// A byte slice is where sealed ciphertext lives, so its raw
			// contents are read as text and searched.
			if v.Type().Elem().Kind() == reflect.Uint8 {
				raw := make([]byte, v.Len())
				for i := range v.Len() {
					raw[i] = byte(v.Index(i).Uint())
				}
				b.Write(raw)
				break
			}
			for i := range v.Len() {
				b.WriteString(walk(v.Index(i), depth+1))
			}
		case reflect.Pointer, reflect.Interface:
			if !v.IsNil() {
				b.WriteString(walk(v.Elem(), depth+1))
			}
		case reflect.String:
			b.WriteString(v.String())
		}
		return b.String()
	}

	mustNotContainCredential(t, "a reflective walk of Secrets", walk(reflect.ValueOf(sec), 0))
}

// The summary is what every read path returns. It must carry no secret value
// and no assembled connection string.
func TestTheSavedSummaryNeverCarriesASecretOrAConnectionString(t *testing.T) {
	r := NewRegistry()
	d, err := r.Descriptor("postgresql")
	if err != nil {
		t.Fatal(err)
	}

	cfg := Config{
		Type: "postgresql",
		Fields: map[string]string{
			"host": "db.example.test", "port": "5432", "database": "ledger",
			"username": "svc_reporting", "tls_mode": "verify-full",
			// A secret value that has wrongly been placed in a non-secret
			// field. The summary must not carry it either, because the field
			// is declared KindSecret in the descriptor.
			"password": fixtureCredential,
		},
		ResourceAllowlist: []string{"reporting"},
	}
	summary := d.Summarize(cfg, fixtureSecrets(t), "password", Health{State: HealthNeverChecked})

	out, err := json.Marshal(summary)
	if err != nil {
		t.Fatal(err)
	}
	body := string(out)
	mustNotContainCredential(t, "the saved connection summary", body)

	// The summary lists which secrets exist, by field name only.
	if !strings.Contains(body, "password") {
		t.Error("the summary should say a password is configured, by field name")
	}
	// And it must not contain an assembled connection string, which would be
	// the credential in another form.
	for _, marker := range []string{"postgres://", "postgresql://", "@db.example.test"} {
		if strings.Contains(body, marker) {
			t.Errorf("the summary contains %q, which reconstructs a connection string", marker)
		}
	}
	// The declared health state must be the never-checked one, not a default
	// that reads as healthy.
	if summary.Health.State != HealthNeverChecked {
		t.Errorf("health = %s; an untested connection must not render as anything else", summary.Health.State)
	}
}

// The descriptor is public. It must show the template with placeholders, and
// nothing from any saved connection.
func TestDescriptorsCarryPlaceholderTemplatesOnly(t *testing.T) {
	r := NewRegistry()
	for _, d := range r.Catalog() {
		if d.Template == "" {
			continue
		}
		if !strings.Contains(d.Template, "<secret>") && !strings.Contains(d.Template, "<user>") {
			t.Errorf("%s: the template %q has no placeholders, so it may be a real string",
				d.Type, d.Template)
		}
		body, err := json.Marshal(d)
		if err != nil {
			t.Fatal(err)
		}
		mustNotContainCredential(t, d.Type+"'s descriptor", string(body))
	}
}

// Every secret field must be declared write-only, so no read path can return it.
func TestEverySecretFieldIsWriteOnly(t *testing.T) {
	r := NewRegistry()
	for _, d := range r.Catalog() {
		for _, f := range d.Fields {
			looksSecret := strings.Contains(strings.ToLower(f.Label), "password") ||
				strings.Contains(strings.ToLower(f.Label), "private key") ||
				strings.Contains(strings.ToLower(f.Label), "token") ||
				strings.Contains(strings.ToLower(f.Label), "wallet") ||
				strings.Contains(strings.ToLower(f.Label), "service account")

			if looksSecret && f.Kind != KindSecret && f.Kind != KindSecretRef {
				t.Errorf("%s: field %q looks like a credential but is %s, so it would be "+
					"returned by every read path", d.Type, f.Label, f.Kind)
			}
		}
	}
}

// A pasted connection string is split and discarded; nothing echoes it.
func TestPastedConnectionStringsAreSplitAndDiscarded(t *testing.T) {
	r := NewRegistry()
	d, err := r.Descriptor("postgresql")
	if err != nil {
		t.Fatal(err)
	}

	raw := "postgresql://svc_reporting:" + fixtureCredential + "@db.example.test:5432/ledger?sslmode=verify-full"
	parsed, err := ParseConnectionURI(d, raw)
	if err != nil {
		t.Fatal(err)
	}

	if !parsed.HasSecret {
		t.Fatal("the credential in the connection string was not extracted")
	}
	body, err := json.Marshal(parsed.Fields)
	if err != nil {
		t.Fatal(err)
	}
	mustNotContainCredential(t, "the parsed fields", string(body))

	if parsed.Fields["host"] != "db.example.test" ||
		parsed.Fields["database"] != "ledger" ||
		parsed.Fields["username"] != "svc_reporting" ||
		parsed.Fields["tls_mode"] != "verify-full" {
		t.Errorf("the connection string was not split correctly: %v", parsed.Fields)
	}

	// The sealed credential renders as nothing.
	mustNotContainCredential(t, "the extracted secret", fmt.Sprintf("%v %#v", parsed.Secret, parsed.Secret))
}

func TestAWeakTlsModeInAPastedStringIsWarnedAbout(t *testing.T) {
	r := NewRegistry()
	d, _ := r.Descriptor("postgresql")

	parsed, err := ParseConnectionURI(d,
		"postgresql://u:"+fixtureCredential+"@db.example.test:5432/ledger?sslmode=require")
	if err != nil {
		t.Fatal(err)
	}
	if len(parsed.Warnings) == 0 {
		t.Error("a pasted string selecting an unverified TLS mode must warn; the person pasting " +
			"it has usually never looked at that parameter")
	}
}

func TestUriPasteIsRefusedWhereThereIsNoUri(t *testing.T) {
	r := NewRegistry()
	for _, typ := range []string{"bigquery", "snowflake", "databricks"} {
		d, err := r.Descriptor(typ)
		if err != nil {
			t.Fatal(err)
		}
		if d.SupportsURIPaste {
			t.Errorf("%s offers URI paste; it has no password URI, so the box would invite an "+
				"operator to paste a service-account JSON into a non-secret field", typ)
		}
		if _, err := ParseConnectionURI(d, "anything"); err == nil {
			t.Errorf("%s parsed a connection string it does not have", typ)
		}
	}
}

// A malformed connection string must not be echoed: it is usually a working
// credential with a typo.
func TestAMalformedConnectionStringIsNotEchoed(t *testing.T) {
	r := NewRegistry()
	d, _ := r.Descriptor("postgresql")

	// A scheme typo, with a real-looking credential behind it.
	_, err := ParseConnectionURI(d, "postgres:/"+"/u:"+fixtureCredential+"@ ho st:5432/db")
	if err == nil {
		t.Skip("this input parsed; the echo check needs a rejection")
	}
	mustNotContainCredential(t, "the parse error", err.Error())
}
