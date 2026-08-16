package secrets

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"fmt"
)

// Sealer encrypts retrievable credentials before they reach durable storage.
//
// The requirement it exists to enforce: the key must not live where the
// ciphertext lives. A database backup, a replica, a support export, or a
// compromised read-only reporting user must yield ciphertext and nothing else.
// An implementation that derives its key from the database, or from a constant
// in this source tree, satisfies the interface and defeats the purpose.
type Sealer interface {
	// Seal returns ciphertext including whatever nonce it needs.
	Seal(plaintext []byte) ([]byte, error)
	// Open recovers plaintext, or fails. It must fail on any modification.
	Open(ciphertext []byte) ([]byte, error)
	// KeyID names the key so a stored row records which key sealed it, making
	// key rotation identifiable. It is not secret.
	KeyID() string
}

// ErrSealKeyLength is returned for a key that is not 32 bytes.
var ErrSealKeyLength = errors.New("seal key must be exactly 32 bytes")

// aesSealer is AES-256-GCM. Authenticated encryption is required rather than
// merely preferred: with an unauthenticated mode an attacker with write access
// to the database could flip bits in a stored credential and observe how the
// system behaves with the modified value.
type aesSealer struct {
	aead  cipher.AEAD
	keyID string
}

// NewAESSealer builds a sealer from a 32-byte key.
//
// The key id is derived from the key rather than supplied, so two deployments
// cannot label different keys identically, and a row sealed under a rotated key
// is recognisable. The id is a truncated digest of the key: it identifies
// without disclosing, exactly as a secret fingerprint does.
func NewAESSealer(key []byte) (Sealer, error) {
	if len(key) != 32 {
		return nil, fmt.Errorf("%w: got %d", ErrSealKeyLength, len(key))
	}
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, err
	}
	aead, err := cipher.NewGCM(block)
	if err != nil {
		return nil, err
	}
	sum := sha256.Sum256(append([]byte("sentinel/seal-key-id/v1\x00"), key...))
	return &aesSealer{aead: aead, keyID: "key_" + hex.EncodeToString(sum[:6])}, nil
}

// SealerFromBase64 parses a key supplied through configuration.
//
// Standard and URL-safe alphabets are both accepted because operators paste
// whichever their key management system emits, and rejecting one produces a
// startup failure that looks like a wrong key rather than a wrong encoding.
func SealerFromBase64(encoded string) (Sealer, error) {
	for _, dec := range []*base64.Encoding{
		base64.StdEncoding, base64.RawStdEncoding,
		base64.URLEncoding, base64.RawURLEncoding,
	} {
		if key, err := dec.DecodeString(encoded); err == nil && len(key) == 32 {
			return NewAESSealer(key)
		}
	}
	return nil, fmt.Errorf("%w: the value is not base64 for 32 bytes", ErrSealKeyLength)
}

// NewEphemeralSealer generates a key that exists only for this process.
//
// It is correct for the in-memory store, whose contents do not outlive the
// process either, and for tests. It is wrong for anything durable: restarting
// the process would make every stored credential permanently unreadable. The
// production profile refuses it -- see RequireDurableSealer.
func NewEphemeralSealer() (Sealer, error) {
	key := make([]byte, 32)
	if _, err := rand.Read(key); err != nil {
		return nil, fmt.Errorf("generate ephemeral seal key: %w", err)
	}
	s, err := NewAESSealer(key)
	if err != nil {
		return nil, err
	}
	return &ephemeralSealer{Sealer: s}, nil
}

// ephemeralSealer marks a key as process-scoped so it can be refused where
// durability matters. The marker is a type rather than a flag on the config, so
// it cannot be lost by passing the sealer through a function.
type ephemeralSealer struct{ Sealer }

func (e *ephemeralSealer) isEphemeral() bool { return true }

// RequireDurableSealer refuses a process-scoped key.
//
// Called during production startup. Without it, a deployment that forgot to set
// a seal key would appear to work perfectly until its first restart, at which
// point every retrievable credential would be unrecoverable ciphertext.
func RequireDurableSealer(s Sealer) error {
	if s == nil {
		return ErrNoSealer
	}
	type ephemeral interface{ isEphemeral() bool }
	if e, ok := s.(ephemeral); ok && e.isEphemeral() {
		return errors.New("the configured seal key is process-scoped: stored credentials would be unrecoverable after a restart. Set SENTINEL_SECRET_SEAL_KEY")
	}
	return nil
}

func (a *aesSealer) KeyID() string { return a.keyID }

func (a *aesSealer) Seal(plaintext []byte) ([]byte, error) {
	nonce := make([]byte, a.aead.NonceSize())
	if _, err := rand.Read(nonce); err != nil {
		return nil, fmt.Errorf("seal: %w", err)
	}
	return a.aead.Seal(nonce, nonce, plaintext, nil), nil
}

func (a *aesSealer) Open(ciphertext []byte) ([]byte, error) {
	n := a.aead.NonceSize()
	if len(ciphertext) <= n {
		return nil, errors.New("open: ciphertext is too short to contain a nonce")
	}
	out, err := a.aead.Open(nil, ciphertext[:n], ciphertext[n:], nil)
	if err != nil {
		// The underlying error is deliberately not wrapped: distinguishing
		// "wrong key" from "modified ciphertext" tells an attacker which of the
		// two they achieved.
		return nil, errors.New("open: sealed credential failed authentication")
	}
	return out, nil
}
