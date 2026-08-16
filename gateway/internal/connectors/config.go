package connectors

import (
	"fmt"
	"sort"
	"strings"

	"sentinel-gateway/internal/secrets"
)

// Config is the non-secret half of a connection.
//
// The split is the whole point. Everything in Config may be displayed, logged,
// exported and stored in plain columns; everything in Secrets may not, and the
// type system rather than a convention keeps them apart. The Integration Hub
// this replaces returned the webhook secret from its list endpoint because
// there was one struct and one JSON tag away from disclosure.
type Config struct {
	// Type is the catalog identifier.
	Type string

	// Fields are the descriptor's non-secret values, keyed by field id.
	Fields map[string]string

	// SecretRefs names the secret-store entries this connection uses, keyed by
	// field id. The reference is not the secret: it is the name under which
	// internal/secrets holds it, and it is safe to display.
	SecretRefs map[string]string

	// ResourceAllowlist is the set of schemas, datasets or catalogs this
	// connection may see. An empty allowlist means nothing is discoverable,
	// not everything -- a connection whose allowlist was never configured must
	// not read the whole database.
	ResourceAllowlist []string

	// Limits may lower the platform defaults, never raise them.
	Limits Limits
}

// Get returns a field, trimmed.
func (c Config) Get(id string) string { return strings.TrimSpace(c.Fields[id]) }

// Secrets carries the sensitive half, and never leaves this package's callers.
//
// Values are `secrets.Value`, which holds AES-GCM ciphertext rather than
// plaintext precisely so a `%v`, a `%#v`, a JSON marshal or a reflective dump
// cannot disclose one. Reading a secret requires `Use`, which is deliberately
// conspicuous at the call site.
type Secrets struct {
	values map[string]secrets.Value
}

// NewSecrets builds a secret set.
func NewSecrets(values map[string]secrets.Value) Secrets {
	m := make(map[string]secrets.Value, len(values))
	for k, v := range values {
		m[k] = v
	}
	return Secrets{values: m}
}

// Has reports whether a secret was supplied, without reading it.
func (s Secrets) Has(id string) bool {
	_, ok := s.values[id]
	return ok
}

// IDs returns the supplied secret field ids, sorted. The ids are field names,
// never values, and are safe to display.
func (s Secrets) IDs() []string {
	out := make([]string, 0, len(s.values))
	for k := range s.values {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}

// Use runs fn with the plaintext of one secret and nothing else.
//
// It wraps `secrets.Value.Expose`, which is named to be conspicuous in review.
// The wrapper adds the scope: a driver that needs a password has to say so at
// the point of use, inside a closure, and cannot accidentally carry the
// plaintext out into a struct that something later marshals. The plaintext
// exists only for the duration of fn.
func (s Secrets) Use(id string, fn func(string) error) error {
	v, ok := s.values[id]
	if !ok {
		return fmt.Errorf("no secret was supplied for %q", id)
	}
	if v.IsZero() {
		return fmt.Errorf("the secret for %q is empty", id)
	}
	return fn(v.Expose())
}

// Fingerprint returns the non-reversible fingerprint of a secret.
//
// It is what a UI may show to distinguish two saved credentials, and what an
// audit record may carry to prove which one was used. It is not the secret and
// cannot be turned back into one.
func (s Secrets) Fingerprint(id string) string {
	v, ok := s.values[id]
	if !ok {
		return ""
	}
	return v.Fingerprint()
}

// String, GoString and MarshalJSON refuse to render the set.
//
// Secrets holds `secrets.Value`s that are individually non-disclosing, so this
// is belt and braces -- but the container is what appears in a log line when
// someone prints the whole config, and it should say nothing rather than
// listing which credentials exist.
func (s Secrets) String() string   { return "connectors.Secrets{redacted}" }
func (s Secrets) GoString() string { return "connectors.Secrets{redacted}" }

// MarshalJSON refuses rather than emitting an empty object.
//
// Emitting `{}` would let a secret set be silently dropped into an API response
// that a reader would then believe carried the connection's full state.
func (s Secrets) MarshalJSON() ([]byte, error) {
	return nil, fmt.Errorf("connector secrets cannot be serialised")
}

// Classification decides how a column's values may be shown.
//
// It is required rather than optional. A column with no classification is
// treated as the most restrictive, because the alternative -- defaulting to
// public -- means every column anyone forgets to classify is disclosed.
type Classification string

const (
	// ClassPublic is safe to display: counts, dates, non-identifying metadata.
	ClassPublic Classification = "PUBLIC"
	// ClassInternal is shown to authorised operators only.
	ClassInternal Classification = "INTERNAL"
	// ClassSensitive is masked in every rendering. Account and routing numbers
	// live here.
	ClassSensitive Classification = "SENSITIVE"
	// ClassUnclassified is the default and is treated as SENSITIVE.
	ClassUnclassified Classification = "UNCLASSIFIED"
)

// Masked reports whether values of this class must be masked before leaving the
// gateway. Unclassified masks, deliberately.
func (c Classification) Masked() bool {
	return c != ClassPublic && c != ClassInternal
}
