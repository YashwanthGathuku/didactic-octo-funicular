// Package secrets holds credentials as write-only references.
//
// The defect this package exists to make unrepresentable was live in this
// repository: webhook secrets were generated from time.Now().UnixNano(),
// returned by the create response, listed by the index endpoint, and readable
// through a SQL console. Each of those is a separate mistake, but they share a
// cause -- a credential was an ordinary string, so every mechanism that handles
// strings handled it.
//
// Value fixes that at the type level. A Value renders as a placeholder under
// fmt, encoding/json, log and log/slog; it refuses to be decoded from a request
// body; and the only way to read it is a method named Expose, which is
// deliberately conspicuous in a diff. Redaction is therefore the default and
// disclosure is the exception a reviewer can grep for.
package secrets

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/hex"
	"errors"
	"fmt"
	"log/slog"
)

const (
	// Placeholder is what a set secret renders as everywhere.
	Placeholder = "[REDACTED]"

	// PlaceholderUnset distinguishes "configured but hidden" from "not
	// configured". Collapsing the two would make a missing credential look like
	// a present one in every diagnostic an operator has.
	PlaceholderUnset = "[UNSET]"

	// MinSecretLength is the shortest value this package will hold.
	//
	// It is not a password policy. It is what makes the truncated fingerprint
	// safe to publish: a 32-character random value has enough entropy that a
	// 16-hex-character digest prefix cannot be matched back to it offline. A
	// short or guessable value would make the fingerprint a disclosure channel.
	MinSecretLength = 32

	// fingerprintDomain separates this digest from any other SHA-256 computed
	// over the same bytes elsewhere in the system, so a fingerprint published in
	// an audit record cannot be compared against, say, an artifact hash.
	fingerprintDomain = "sentinel/secret-fingerprint/v1\x00"
)

// ErrTooShort is returned by New for a value below MinSecretLength.
var ErrTooShort = errors.New("secret is shorter than the minimum length")

// ErrNotDecodable is returned when something tries to deserialize a Value.
var ErrNotDecodable = errors.New("a secret cannot be decoded from input; construct it through the secret store")

// procAEAD encrypts every in-memory Value under a key generated once per
// process and never stored in any Value.
//
// The reason is narrow and worth stating exactly, because it would be easy to
// overclaim. fmt handles %T and %p before consulting fmt.Formatter, and its
// bad-verb path then dumps the argument's fields by reflection -- including
// unexported ones. A plain string field is therefore disclosed by
// fmt.Sprintf("%p", v), and by any third-party struct dumper that walks fields
// reflectively. Holding ciphertext means reflection finds ciphertext.
//
// What this does NOT do: protect against a core dump, a debugger, /proc/pid/mem,
// or a Go heap profile. The key is in the same address space as the ciphertext.
// This is a defence against accidental disclosure through the language's own
// output machinery, not against an attacker who already has the process memory.
var procAEAD = func() cipher.AEAD {
	key := make([]byte, 32)
	if _, err := rand.Read(key); err != nil {
		// Without a working CSPRNG this process cannot safely handle credentials
		// at all, and there is no degraded mode worth offering.
		panic("secrets: no entropy available for the process key: " + err.Error())
	}
	block, err := aes.NewCipher(key)
	if err != nil {
		panic("secrets: " + err.Error())
	}
	aead, err := cipher.NewGCM(block)
	if err != nil {
		panic("secrets: " + err.Error())
	}
	return aead
}()

// Value is a credential that redacts itself on every output path.
//
// The fields are unexported and hold no plaintext: `sealed` is ciphertext under
// the process key, and `fp` is the published fingerprint, which is already safe
// to disclose. A reflective dump of a Value therefore yields nothing useful.
// Copying is safe -- the contents are immutable.
type Value struct {
	sealed []byte
	fp     string
}

// New wraps an existing credential -- one supplied by an operator through
// configuration, or read from an external secret manager.
//
// Generate is the constructor to use when the application is the one creating
// the credential; this one is for values that already exist.
func New(raw string) (Value, error) {
	if len(raw) < MinSecretLength {
		return Value{}, fmt.Errorf("%w: got %d characters, require at least %d", ErrTooShort, len(raw), MinSecretLength)
	}
	nonce := make([]byte, procAEAD.NonceSize())
	if _, err := rand.Read(nonce); err != nil {
		return Value{}, fmt.Errorf("seal secret: %w", err)
	}
	return Value{
		sealed: procAEAD.Seal(nonce, nonce, []byte(raw), nil),
		fp:     fingerprint(raw),
	}, nil
}

func fingerprint(raw string) string {
	sum := sha256.Sum256(append([]byte(fingerprintDomain), raw...))
	return "sfp_" + hex.EncodeToString(sum[:8])
}

// open recovers the plaintext. Any failure here is a programming error or
// memory corruption, not a condition a caller can handle, so it yields the
// empty string -- which never authenticates anything, because Equal refuses it.
func (v Value) open() string {
	if len(v.sealed) <= procAEAD.NonceSize() {
		return ""
	}
	nonce := v.sealed[:procAEAD.NonceSize()]
	raw, err := procAEAD.Open(nil, nonce, v.sealed[procAEAD.NonceSize():], nil)
	if err != nil {
		return ""
	}
	return string(raw)
}

// IsZero reports whether this Value holds nothing.
func (v Value) IsZero() bool { return len(v.sealed) == 0 }

// Expose returns the underlying credential.
//
// The name is the point. Every disclosure of a secret in this codebase is a
// call to Expose, so `grep -rn '\.Expose()'` is a complete inventory of them and
// a reviewer can check each one against its purpose.
func (v Value) Expose() string { return v.open() }

// Equal compares in constant time.
//
// A byte-by-byte comparison of a bearer token against an attacker-supplied
// string leaks the correct prefix through response timing. The zero Value never
// matches, so an unconfigured credential cannot authenticate an empty
// presentation.
func (v Value) Equal(presented string) bool {
	if v.IsZero() {
		return false
	}
	return subtle.ConstantTimeCompare([]byte(v.open()), []byte(presented)) == 1
}

// Fingerprint is a short, stable, non-reversible handle for this secret.
//
// It exists so an audit record can say which credential was used without
// containing it, and so two log lines from different services can be correlated
// to the same credential. It is a domain-separated SHA-256 truncated to 64 bits,
// which is safe only because MinSecretLength bounds the input entropy from
// below -- see that constant.
func (v Value) Fingerprint() string { return v.fp }

// --- Output paths. Each of these is a channel a credential has escaped through
// in some real system; all of them return the placeholder. ---

// Format implements fmt.Formatter, which fmt consults before Stringer,
// GoStringer or reflection. Handling every verb here is what makes %#v, %q and
// %x safe, not only %v and %s.
func (v Value) Format(f fmt.State, verb rune) {
	_, _ = f.Write([]byte(v.String()))
}

// String implements fmt.Stringer for callers that are not fmt.
func (v Value) String() string {
	if v.IsZero() {
		return PlaceholderUnset
	}
	return Placeholder
}

// GoString implements fmt.GoStringer, covering %#v for any caller that bypasses
// Format.
func (v Value) GoString() string { return v.String() }

// MarshalJSON keeps a secret out of every API response, including ones written
// later by someone who does not know this field is sensitive.
func (v Value) MarshalJSON() ([]byte, error) {
	return []byte(`"` + v.String() + `"`), nil
}

// MarshalText covers encoders that prefer TextMarshaler, and map keys.
func (v Value) MarshalText() ([]byte, error) { return []byte(v.String()), nil }

// UnmarshalJSON refuses. A secret must never be populated from a request body:
// that is how a caller-supplied value ends up echoed back by a handler that
// believes it is only marshalling its own state.
func (v *Value) UnmarshalJSON([]byte) error { return ErrNotDecodable }

// UnmarshalText refuses, for the same reason.
func (v *Value) UnmarshalText([]byte) error { return ErrNotDecodable }

// LogValue implements slog.LogValuer so structured logging redacts too.
func (v Value) LogValue() slog.Value { return slog.StringValue(v.String()) }
