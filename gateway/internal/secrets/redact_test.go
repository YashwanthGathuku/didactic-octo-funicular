package secrets

import (
	"bytes"
	"errors"
	"fmt"
	"log"
	"strings"
	"testing"
)

// Each case is a shape a credential has actually escaped in through: a header
// copied into an error, a connection string in a startup log, a token in a
// query string, a JWT in a trace.
func TestRedactScrubsKnownCredentialShapes(t *testing.T) {
	cases := []struct {
		name        string
		in          string
		mustNotHave string
		mustHave    string
	}{
		{
			name:        "bearer token in a header",
			in:          `Authorization: Bearer eyJhbGciOiJSUzI1NiJ9.eyJzdWIiOiJhIn0.sIgNaTuRe`,
			mustNotHave: "sIgNaTuRe",
			mustHave:    "Bearer",
		},
		{
			name:        "basic credentials in a header",
			in:          `authorization: Basic dXNlcjpzdXBlcnNlY3JldHBhc3N3b3Jk`,
			mustNotHave: "dXNlcjpzdXBlcnNlY3JldHBhc3N3b3Jk",
			mustHave:    "Basic",
		},
		{
			name:        "password in a connection string",
			in:          `Unable to connect to database "postgres://sentinel_app:hunter2-real-password@db:5432/sentinel"`,
			mustNotHave: "hunter2-real-password",
			mustHave:    "sentinel_app", // the username stays; it is not a credential
		},
		{
			name:        "client secret in a form body",
			in:          `POST /token failed: grant_type=authorization_code&client_secret=s3cr3t-value-from-provider&code=abc`,
			mustNotHave: "s3cr3t-value-from-provider",
			mustHave:    "grant_type=authorization_code",
		},
		{
			name:        "access token in a query string",
			in:          `GET /callback?state=xyz&access_token=ya29.a0AfH6SMBnotarealtoken HTTP/1.1`,
			mustNotHave: "ya29.a0AfH6SMBnotarealtoken",
			mustHave:    "state=xyz",
		},
		{
			name:        "PKCE verifier",
			in:          `exchange failed: code_verifier=dBjftJeZ4CVPmB92K27uhbUJU1p1r-wW1gFWFOEjXk`,
			mustNotHave: "dBjftJeZ4CVPmB92K27uhbUJU1p1r-wW1gFWFOEjXk",
			mustHave:    "exchange failed",
		},
		{
			name:        "bare JWT in a message",
			in:          `token rejected: eyJraWQiOiJrMSJ9.eyJpc3MiOiJodHRwczovL2kifQ.QUJDREVGRw`,
			mustNotHave: "QUJDREVGRw",
			mustHave:    "token rejected",
		},
		{
			name:        "a JSON field",
			in:          `{"client_secret":"provider-issued-value-9f2a","scope":"openid"}`,
			mustNotHave: "provider-issued-value-9f2a",
			mustHave:    "openid",
		},
		{
			name:        "a PEM private key",
			in:          "signing failed with -----BEGIN RSA PRIVATE KEY-----\nMIIEowIBAAKCAQEA\n-----END RSA PRIVATE KEY----- loaded",
			mustNotHave: "MIIEowIBAAKCAQEA",
			mustHave:    "signing failed",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := Redact(tc.in)
			if strings.Contains(got, tc.mustNotHave) {
				t.Errorf("credential material survived redaction:\n  in:  %s\n  out: %s", tc.in, got)
			}
			if tc.mustHave != "" && !strings.Contains(got, tc.mustHave) {
				t.Errorf("redaction destroyed the diagnostic context %q:\n  out: %s", tc.mustHave, got)
			}
		})
	}
}

// Ordinary text must survive untouched, or the scrubber gets switched off.
func TestRedactLeavesOrdinaryTextAlone(t *testing.T) {
	for _, s := range []string{
		"Applied 3 migration(s): 001_init_schema, 002_tenancy_and_state, 003_secret_store",
		"Inbox watcher enabled for tenant TENANT-A at ./inbox",
		"artifact 42 moved RECEIVED -> VALIDATING",
		"",
	} {
		if got := Redact(s); got != s {
			t.Errorf("redaction altered ordinary text:\n  in:  %q\n  out: %q", s, got)
		}
	}
}

// A registered credential is scrubbed even in a shape no pattern anticipates.
func TestScrubberRemovesRegisteredValuesInAnyShape(t *testing.T) {
	v := MustGenerate()
	s := NewScrubber()
	s.Register(v)

	shapes := []string{
		"upstream said: " + v.Expose(),
		"GET /internal/" + v.Expose() + "/status",
		fmt.Sprintf("map[%s:enabled]", v.Expose()),
		"first=" + v.Expose() + " second=" + v.Expose(),
	}
	for _, in := range shapes {
		got := s.Scrub(in)
		if strings.Contains(got, v.Expose()) {
			t.Errorf("a registered credential survived scrubbing:\n  in:  %s\n  out: %s", in, got)
		}
		// The fingerprint replaces it, so a redacted line still identifies
		// which credential was involved.
		if !strings.Contains(got, v.Fingerprint()) {
			t.Errorf("the replacement does not identify the credential: %s", got)
		}
	}
}

func TestScrubberIgnoresZeroValuesAndDeduplicates(t *testing.T) {
	s := NewScrubber()
	s.Register(Value{})
	if s.Count() != 0 {
		t.Error("a zero Value was registered")
	}

	v := MustGenerate()
	s.Register(v)
	s.Register(v)
	if s.Count() != 1 {
		t.Errorf("registering the same credential twice produced %d entries", s.Count())
	}
}

// The standard logger is what this codebase uses. Wrapping its output is what
// makes redaction automatic for log calls written later by someone who has not
// read this package.
func TestLogWriterScrubsEveryRecord(t *testing.T) {
	v := MustGenerate()
	scrubber := NewScrubber()
	scrubber.Register(v)

	var sink bytes.Buffer
	logger := log.New(NewLogWriter(&sink, scrubber), "", 0)

	logger.Printf("connecting with token %s", v.Expose())
	logger.Printf("Unable to connect to database %q", "postgres://app:a-real-password@db/sentinel")
	logger.Printf("Authorization: Bearer eyJhbGciOiJSUzI1NiJ9.eyJzdWIiOiJhIn0.SiGnAtUrE")

	out := sink.String()
	for _, forbidden := range []string{v.Expose(), "a-real-password", "SiGnAtUrE"} {
		if strings.Contains(out, forbidden) {
			t.Errorf("credential material reached the log:\n%s", out)
		}
	}
	if !strings.Contains(out, "Unable to connect to database") {
		t.Errorf("the log lost its diagnostic content:\n%s", out)
	}
}

// A wrapped Write must report the caller's byte count, or the standard logger
// treats a successful scrubbed write as a short write.
func TestLogWriterReportsTheCallerLength(t *testing.T) {
	v := MustGenerate()
	scrubber := NewScrubber()
	scrubber.Register(v)

	var sink bytes.Buffer
	w := NewLogWriter(&sink, scrubber)
	payload := []byte("token " + v.Expose() + "\n")
	n, err := w.Write(payload)
	if err != nil {
		t.Fatal(err)
	}
	if n != len(payload) {
		t.Errorf("Write reported %d of %d bytes; the caller would see a short write", n, len(payload))
	}
}

// Errors carry the input that failed, and the input that failed is often the
// credential. Redaction must not break errors.Is, or it will be removed the
// first time it breaks control flow.
func TestRedactErrorScrubsWhilePreservingMatching(t *testing.T) {
	sentinel := errors.New("token exchange rejected")
	wrapped := fmt.Errorf("%w: client_secret=provider-issued-value-9f2a", sentinel)

	redacted := RedactError(wrapped)
	if strings.Contains(redacted.Error(), "provider-issued-value-9f2a") {
		t.Errorf("RedactError left the credential in the message: %v", redacted)
	}
	if !errors.Is(redacted, sentinel) {
		t.Error("RedactError broke errors.Is; callers could no longer branch on the cause")
	}
	if !strings.Contains(redacted.Error(), "token exchange rejected") {
		t.Errorf("RedactError destroyed the diagnostic: %v", redacted)
	}

	if RedactError(nil) != nil {
		t.Error("RedactError(nil) must be nil")
	}
	clean := errors.New("nothing sensitive here")
	if RedactError(clean) != clean {
		t.Error("RedactError allocated a wrapper for an error needing no redaction")
	}
}

// A nil Scrubber must still apply the shape patterns rather than panicking:
// code paths that have no scrubber available are exactly the ones least likely
// to be reviewed.
func TestNilScrubberStillRedacts(t *testing.T) {
	var s *Scrubber
	got := s.Scrub("Authorization: Bearer abcdefghijklmnop")
	if strings.Contains(got, "abcdefghijklmnop") {
		t.Errorf("a nil Scrubber skipped redaction entirely: %s", got)
	}
}
