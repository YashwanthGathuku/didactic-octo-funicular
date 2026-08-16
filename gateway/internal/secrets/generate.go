package secrets

import (
	"crypto/rand"
	"encoding/base64"
	"fmt"
)

// GeneratedBytes is the entropy of every secret this application creates.
//
// 32 bytes is 256 bits. The point of stating it as a constant is that the
// generator cannot be called with a smaller size by a caller in a hurry: there
// is no size parameter.
const GeneratedBytes = 32

// Generate creates a new credential from the operating system's CSPRNG.
//
// The removed webhook subsystem generated its signing secrets from
// time.Now().UnixNano() -- see the archived source in
// docs/engineering/REMOVED_CODE_ARCHIVE.md. That value is not merely
// predictable in principle: an attacker who observes any timestamp from the
// same host narrows it to a range small enough to enumerate, because a
// nanosecond clock has far less real resolution than its units suggest and the
// creation time is usually visible in the very response that returns the
// secret.
//
// crypto/rand is the only acceptable source. math/rand, time, process IDs, and
// hashes of any of them are not, in any combination.
func Generate() (Value, error) {
	buf := make([]byte, GeneratedBytes)
	if _, err := rand.Read(buf); err != nil {
		// Fail closed. A credential the system could not generate securely must
		// not be replaced by one it could generate insecurely.
		return Value{}, fmt.Errorf("generate secret: no entropy available: %w", err)
	}
	// URL-safe and unpadded so the value can be carried in a header, a query
	// string or an environment variable without re-encoding.
	return New(base64.RawURLEncoding.EncodeToString(buf))
}

// MustGenerate is for tests and one-shot administrative commands where a
// failure to produce a credential has no meaningful recovery.
func MustGenerate() Value {
	v, err := Generate()
	if err != nil {
		panic(err)
	}
	return v
}
