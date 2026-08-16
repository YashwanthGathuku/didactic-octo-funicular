package secrets

import (
	"bytes"
	"io"
	"regexp"
	"sync"
)

// Redaction is the second line, not the first.
//
// The first line is the Value type: a credential held as a Value cannot reach a
// log or a response in the first place. This file exists for the material that
// is not a Value at the point it is written -- an Authorization header copied
// into an error, a connection string in a startup message, a provider's error
// body echoed into ours. Those are strings by the time anything sees them, and
// the only remaining defence is to scrub them.
//
// Treating this as the primary control would be a mistake. A scrubber only
// removes what it recognises, and the credential it does not recognise is the
// one that leaks. Every pattern here is a backstop for a specific way material
// has escaped a type boundary.

var (
	// Authorization headers, in both header and Go-struct rendering. The
	// scheme is kept so a diagnostic still says which kind of credential was
	// presented.
	reAuthHeader = regexp.MustCompile(`(?i)(authorization\s*[:=]\s*"?)(bearer|basic|token|digest)(\s+)(\S+)`)

	// Credentials embedded in a URL's userinfo. Connection strings reach logs
	// constantly: this codebase logs DATABASE_URL by design on a failed open.
	reURLUserinfo = regexp.MustCompile(`([a-zA-Z][a-zA-Z0-9+.-]*://)([^/\s:@]+):([^/\s@]+)@`)

	// Sensitive query and form parameters. Naming them explicitly rather than
	// redacting every parameter keeps the rest of a URL diagnosable.
	reSensitiveParam = regexp.MustCompile(`(?i)\b(access_token|refresh_token|id_token|client_secret|api_key|apikey|token|secret|password|passwd|signature|sig|code_verifier)(=|":\s*"|"\s*:\s*")([^&\s"'` + "`" + `]+)`)

	// A compact JWS/JWT. Every one in this system is a credential.
	reJWT = regexp.MustCompile(`\beyJ[A-Za-z0-9_-]{4,}\.[A-Za-z0-9_-]{4,}\.[A-Za-z0-9_-]+`)

	// PEM private key blocks, which have appeared in logs via a mis-set path
	// where the file body was read and reported as the error's detail.
	//
	// secret-scan-allow: this is the detection pattern for private keys, not a key; the hygiene scan cannot tell a detector from its target
	rePEMPrivateKey = regexp.MustCompile(`(?s)-----BEGIN [A-Z ]*PRIVATE KEY-----.*?-----END [A-Z ]*PRIVATE KEY-----`)
)

// Redact scrubs recognised credential shapes from arbitrary text.
//
// It is deliberately conservative about what it replaces: the goal is that a
// log line remains useful for diagnosis after redaction, because a scrubber
// that destroys context gets switched off.
func Redact(s string) string {
	if s == "" {
		return s
	}
	s = rePEMPrivateKey.ReplaceAllString(s, Placeholder)
	s = reAuthHeader.ReplaceAllString(s, "${1}${2}${3}"+Placeholder)
	s = reURLUserinfo.ReplaceAllString(s, "${1}${2}:"+Placeholder+"@")
	s = reSensitiveParam.ReplaceAllString(s, "${1}${2}"+Placeholder)
	s = reJWT.ReplaceAllString(s, Placeholder)
	return s
}

// RedactError scrubs an error's message.
//
// Errors are the most common leak path because they carry the input that
// failed, and the input that failed is often the credential. The returned error
// keeps the original for errors.Is and errors.As -- a scrubber that breaks
// error matching would be removed the first time it broke a control flow.
func RedactError(err error) error {
	if err == nil {
		return nil
	}
	msg := err.Error()
	scrubbed := Redact(msg)
	if scrubbed == msg {
		return err
	}
	return &redactedError{wrapped: err, message: scrubbed}
}

type redactedError struct {
	wrapped error
	message string
}

func (e *redactedError) Error() string { return e.message }

// Unwrap keeps errors.Is and errors.As working through the redaction. The
// wrapped error's own Error() still contains the original text, so callers must
// not print it -- which is why nothing in this package returns it.
func (e *redactedError) Unwrap() error { return e.wrapped }

// Scrubber redacts the specific credential values this process holds, in
// addition to the shape-based patterns.
//
// Registration is what catches a credential in a form no pattern anticipates:
// a token concatenated into a URL path, a secret used as a map key, a value
// interpolated into a message by code written after this file. The registered
// values are held as sealed Values, so the Scrubber is not itself a plaintext
// credential store.
type Scrubber struct {
	mu     sync.RWMutex
	values []Value
}

// NewScrubber returns an empty Scrubber.
func NewScrubber() *Scrubber { return &Scrubber{} }

// Register adds a credential to be scrubbed from all output.
//
// Registering a short or common value would redact ordinary text, so anything
// below MinSecretLength is refused by New before it can get here. Zero values
// are ignored rather than erroring: an unset optional credential is a normal
// condition at startup.
func (s *Scrubber) Register(v Value) {
	if v.IsZero() {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, existing := range s.values {
		if existing.Fingerprint() == v.Fingerprint() {
			return
		}
	}
	s.values = append(s.values, v)
}

// Count reports how many credentials are registered. It is for diagnostics and
// for the startup line that says redaction is active; it discloses nothing.
func (s *Scrubber) Count() int {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return len(s.values)
}

// Scrub applies the registered values and the shape patterns.
func (s *Scrubber) Scrub(text string) string {
	if s != nil {
		s.mu.RLock()
		for _, v := range s.values {
			raw := v.Expose()
			if raw != "" && len(raw) >= MinSecretLength {
				// Replaced with the fingerprint rather than a bare placeholder,
				// so a redacted log line still says *which* credential was
				// involved. That is the difference between a diagnosable
				// incident and a mystery.
				text = replaceAll(text, raw, "["+v.Fingerprint()+"]")
			}
		}
		s.mu.RUnlock()
	}
	return Redact(text)
}

// replaceAll avoids strings.ReplaceAll only to keep the dependency surface of
// this file to what it needs; the behaviour is identical.
func replaceAll(haystack, needle, with string) string {
	if needle == "" {
		return haystack
	}
	return string(bytes.ReplaceAll([]byte(haystack), []byte(needle), []byte(with)))
}

// LogWriter wraps a log destination so every line written through it is
// scrubbed.
//
// This is installed on the standard logger at startup. It is a backstop for the
// log calls that already exist and for every one written later by someone who
// has not read this package -- which, over the life of a codebase, is most of
// them.
type LogWriter struct {
	out      io.Writer
	scrubber *Scrubber
}

// NewLogWriter wraps out. A nil scrubber still applies the shape patterns.
func NewLogWriter(out io.Writer, s *Scrubber) *LogWriter {
	return &LogWriter{out: out, scrubber: s}
}

// Write scrubs the buffer before it reaches the underlying writer.
//
// The standard logger writes one complete record per call, so a credential
// cannot be split across two Write calls and slip past the patterns. A writer
// fed arbitrary streamed fragments would need buffering; this one is not.
func (w *LogWriter) Write(p []byte) (int, error) {
	scrubbed := w.scrubber.Scrub(string(p))
	if _, err := w.out.Write([]byte(scrubbed)); err != nil {
		return 0, err
	}
	// Report the caller's length rather than the scrubbed length: a short write
	// would otherwise be reported to a caller that has nothing to do about it.
	return len(p), nil
}
