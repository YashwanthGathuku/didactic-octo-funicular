package secrets

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"log/slog"
	"reflect"
	"strings"
	"testing"
)

// The sentinel used throughout this file. Every test asserts it does not appear
// in some output channel. It is a literal in a test, which is the one place a
// secret-shaped string is legitimate, and the source scan in
// hygiene_test.go excludes _test.go files for exactly this reason.
const canary = "canary-4f3a9c1e8b7d6520canary-4f3a9c1e8b7d6520"

func mustNew(t *testing.T, raw string) Value {
	t.Helper()
	v, err := New(raw)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	return v
}

// A secret handed to fmt must never print itself, under any verb.
//
// This is the leak that actually happens in practice: someone writes
// log.Printf("config: %+v", cfg) during an incident and the credential lands in
// a log aggregator that is retained for a year.
func TestValueNeverPrintsUnderAnyVerb(t *testing.T) {
	v := mustNew(t, canary)

	verbs := []string{"%v", "%+v", "%#v", "%s", "%q", "%x", "%X", "%d", "%T", "%p", "%10s", "%-20v"}
	for _, verb := range verbs {
		got := fmt.Sprintf(verb, v)
		if strings.Contains(got, canary) {
			t.Errorf("fmt.Sprintf(%q, Value) disclosed the secret: %s", verb, got)
		}
		if strings.Contains(got, "canary") {
			t.Errorf("fmt.Sprintf(%q, Value) disclosed part of the secret: %s", verb, got)
		}
	}

	// The same through a pointer, which is a different fmt path.
	for _, verb := range []string{"%v", "%+v", "%#v", "%s"} {
		if got := fmt.Sprintf(verb, &v); strings.Contains(got, "canary") {
			t.Errorf("fmt.Sprintf(%q, *Value) disclosed the secret: %s", verb, got)
		}
	}
}

// Regression: %p and %T are handled by fmt before fmt.Formatter is consulted,
// and the resulting bad-verb path dumps the argument's fields by reflection --
// unexported fields included. An earlier draft of Value held the credential in a
// plain string field and fmt.Sprintf("%p", v) printed it in full.
//
// The same reflective walk is what any third-party struct dumper does, so this
// test stands in for that whole class.
func TestValueSurvivesReflectiveInspection(t *testing.T) {
	v := mustNew(t, canary)

	if got := fmt.Sprintf("%p", v); strings.Contains(got, "canary") {
		t.Errorf("%%p disclosed the secret through fmt's bad-verb reflection path: %s", got)
	}

	rv := reflect.ValueOf(v)
	for i := 0; i < rv.NumField(); i++ {
		f := rv.Field(i)
		var rendered string
		switch f.Kind() {
		case reflect.String:
			rendered = f.String()
		case reflect.Slice:
			rendered = fmt.Sprintf("%s", f.Bytes())
		default:
			rendered = fmt.Sprint(f)
		}
		if strings.Contains(rendered, "canary") {
			t.Errorf("field %q holds the plaintext secret; a reflective dump would disclose it",
				rv.Type().Field(i).Name)
		}
	}
}

// A secret nested inside a struct is the common shape: it is a config field, not
// a standalone variable.
func TestValueNestedInStructDoesNotPrint(t *testing.T) {
	type creds struct {
		Name  string
		Token Value
	}
	c := creds{Name: "api", Token: mustNew(t, canary)}

	for _, verb := range []string{"%v", "%+v", "%#v"} {
		if got := fmt.Sprintf(verb, c); strings.Contains(got, "canary") {
			t.Errorf("a struct containing a Value disclosed it under %s: %s", verb, got)
		}
	}

	// And inside a map and a slice.
	if got := fmt.Sprintf("%v", map[string]Value{"api": mustNew(t, canary)}); strings.Contains(got, "canary") {
		t.Errorf("a map of Values disclosed one: %s", got)
	}
	if got := fmt.Sprintf("%v", []Value{mustNew(t, canary)}); strings.Contains(got, "canary") {
		t.Errorf("a slice of Values disclosed one: %s", got)
	}
}

// JSON is how a secret reaches an API response. Marshalling must redact rather
// than requiring every handler to remember to omit the field.
func TestValueMarshalsRedacted(t *testing.T) {
	type response struct {
		ID    string `json:"id"`
		Token Value  `json:"token"`
	}
	b, err := json.Marshal(response{ID: "sec-1", Token: mustNew(t, canary)})
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	if bytes.Contains(b, []byte("canary")) {
		t.Errorf("json.Marshal disclosed the secret: %s", b)
	}
	if !bytes.Contains(b, []byte(Placeholder)) {
		t.Errorf("json.Marshal should emit %q so the field is visibly redacted, got %s", Placeholder, b)
	}

	// Indented marshalling takes a different code path.
	b, _ = json.MarshalIndent(response{ID: "sec-1", Token: mustNew(t, canary)}, "", "  ")
	if bytes.Contains(b, []byte("canary")) {
		t.Errorf("json.MarshalIndent disclosed the secret: %s", b)
	}
}

// A Value must not be constructible from a request body. If it were, an API
// could accept a caller-supplied secret and echo it back through a struct that
// is otherwise safe to marshal.
func TestValueRefusesToUnmarshalFromJSON(t *testing.T) {
	var target struct {
		Token Value `json:"token"`
	}
	err := json.Unmarshal([]byte(`{"token":"`+canary+`"}`), &target)
	if err == nil {
		t.Fatal("a Value was populated from a JSON request body; secrets must enter only through the store")
	}
	if target.Token.Expose() == canary {
		t.Error("the secret was assigned despite the error")
	}
}

// slog is the structured-logging path. A Value must redact itself there too,
// including when it is an attribute value inside a group.
func TestValueRedactsUnderSlog(t *testing.T) {
	var buf bytes.Buffer
	logger := slog.New(slog.NewJSONHandler(&buf, nil))
	logger.Info("startup",
		slog.String("profile", "production"),
		slog.Any("api_token", mustNew(t, canary)),
		slog.Group("oidc", slog.Any("client_secret", mustNew(t, canary))),
	)
	if strings.Contains(buf.String(), "canary") {
		t.Errorf("slog disclosed the secret: %s", buf.String())
	}

	buf.Reset()
	text := slog.New(slog.NewTextHandler(&buf, nil))
	text.Info("startup", slog.Any("api_token", mustNew(t, canary)))
	if strings.Contains(buf.String(), "canary") {
		t.Errorf("slog text handler disclosed the secret: %s", buf.String())
	}
}

// The standard logger is what this codebase actually uses today.
func TestValueRedactsUnderStandardLog(t *testing.T) {
	var buf bytes.Buffer
	l := log.New(&buf, "", 0)
	l.Printf("token=%v", mustNew(t, canary))
	l.Printf("token=%s", mustNew(t, canary))
	if strings.Contains(buf.String(), "canary") {
		t.Errorf("log disclosed the secret: %s", buf.String())
	}
}

// An error message is a response body and a log line at the same time. Wrapping
// a Value into an error must not smuggle it out.
func TestValueDoesNotEscapeThroughErrors(t *testing.T) {
	v := mustNew(t, canary)
	err := fmt.Errorf("authenticating with %v failed: %w", v, errors.New("upstream refused"))
	if strings.Contains(err.Error(), "canary") {
		t.Errorf("an error disclosed the secret: %v", err)
	}
}

// Exposure has to be possible -- the value is used for something -- but it must
// require calling a method whose name shows up in review.
func TestExposeReturnsTheValue(t *testing.T) {
	v := mustNew(t, canary)
	if v.Expose() != canary {
		t.Error("Expose must return the underlying value; a store that cannot use its secret is not a store")
	}
}

// Comparison must not leak timing. This asserts the API exists and is correct;
// it cannot assert constant time from Go alone.
func TestValueEqualIsCorrect(t *testing.T) {
	v := mustNew(t, canary)
	if !v.Equal(canary) {
		t.Error("Equal returned false for the correct value")
	}
	if v.Equal(canary + "x") {
		t.Error("Equal returned true for a longer value")
	}
	if v.Equal("") {
		t.Error("Equal returned true for the empty string")
	}
	var zero Value
	if zero.Equal("") {
		t.Error("the zero Value must not match the empty string; an unset secret must never authenticate")
	}
}

// A zero Value is what an unset configuration field looks like. It must be
// distinguishable and must never authenticate anything.
func TestZeroValueIsUnset(t *testing.T) {
	var zero Value
	if !zero.IsZero() {
		t.Error("the zero Value should report IsZero")
	}
	if zero.Expose() != "" {
		t.Error("the zero Value should expose nothing")
	}
	if got := fmt.Sprintf("%v", zero); got != PlaceholderUnset {
		t.Errorf("the zero Value should render as %q so an unset secret is distinguishable from a set one, got %q", PlaceholderUnset, got)
	}
}

// Fingerprints let an operator correlate "which credential was used" across
// logs and audit records without the credential appearing in either.
func TestFingerprintIdentifiesWithoutDisclosing(t *testing.T) {
	a := mustNew(t, canary)
	b := mustNew(t, canary)
	c := mustNew(t, canary+"different")

	if a.Fingerprint() != b.Fingerprint() {
		t.Error("the same secret must produce the same fingerprint, or correlation is impossible")
	}
	if a.Fingerprint() == c.Fingerprint() {
		t.Error("different secrets produced the same fingerprint")
	}
	if strings.Contains(a.Fingerprint(), "canary") {
		t.Errorf("the fingerprint contains the secret: %s", a.Fingerprint())
	}
	if len(a.Fingerprint()) > 24 {
		t.Errorf("the fingerprint is %d characters; a truncated handle is enough and a full digest invites offline matching", len(a.Fingerprint()))
	}
}

// New must refuse a value too short to be a credential. The minimum is what
// makes the truncated fingerprint safe to publish.
func TestNewRefusesUndersizedSecrets(t *testing.T) {
	if _, err := New("short"); err == nil {
		t.Error("New accepted a 5-character secret")
	}
	if _, err := New(""); err == nil {
		t.Error("New accepted an empty secret")
	}
	if _, err := New(strings.Repeat("a", MinSecretLength)); err != nil {
		t.Errorf("New rejected a secret at the minimum length: %v", err)
	}
}
