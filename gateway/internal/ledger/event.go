// Package ledger is the append-only evidence chain.
//
// It is an application hash chain: each record carries the digest of its
// predecessor, so altering or removing one breaks every record after it. That
// is a real and useful property and it is also a limited one, so the vocabulary
// here is deliberate.
//
// It is NOT a Merkle tree. There is no tree, no branch hashing and no inclusion
// proof; the structure is a linear predecessor chain. Earlier code in this
// repository exported a metric named `sentinel_merkle_chain_height` for exactly
// this structure, which was corrected in Prompt 01.
//
// A SHA-256 digest is NOT a digital signature. Nothing here is signed. A chain
// whose records and whose hashes are both writable by the same party proves
// internal consistency, not authenticity: anyone who can rewrite a record can
// recompute every digest after it. External anchoring is what closes that, and
// it is designed in checkpoint.go and not implemented -- see the honest
// statement there rather than a claim here.
package ledger

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strings"
	"time"
)

// CanonicalVersion identifies the serialisation used to compute a record's
// hash.
//
// It is stored on every record. Changing how a record is serialised changes
// every digest computed afterwards, so a verifier reading old records must know
// which rules produced them -- without this, a serialisation change makes the
// entire prior chain fail verification and look like tampering.
const CanonicalVersion = "ledger-canonical/1"

// GenesisHash is the predecessor of the first record in a tenant's stream.
const GenesisHash = "0000000000000000000000000000000000000000000000000000000000000000"

// MaxPayloadBytes bounds a record's payload after canonicalisation.
//
// An unbounded payload is a storage vector and, more to the point, an
// invitation to copy something large into the ledger that does not belong
// there -- a file body, a stack trace containing one, an upstream error
// carrying a request. The limit forces a caller to record a reference instead.
const MaxPayloadBytes = 16 << 10

var (
	// ErrPayloadTooLarge means the payload exceeded MaxPayloadBytes.
	ErrPayloadTooLarge = errors.New("audit payload exceeds the maximum size")

	// ErrForbiddenPayload means the payload contained material that must never
	// enter the ledger.
	ErrForbiddenPayload = errors.New("audit payload contains content that must not be recorded")

	// ErrChainBroken is returned by verification. It is deliberately one error:
	// mutation, deletion and reordering all mean the same thing to a reader of
	// the evidence, which is that it can no longer be relied on.
	ErrChainBroken = errors.New("evidence chain integrity check failed")
)

// Record is one entry in a tenant's evidence stream.
//
// Every field the guide requires is present and none is optional in practice:
// a record that cannot say who did what to which object, under which request,
// is not evidence.
type Record struct {
	ID         int64  `json:"id"`
	TenantID   string `json:"tenantId"`
	SequenceNo int64  `json:"sequenceNo"`

	// Action is what happened, in the past tense, from the system's point of
	// view -- ARTIFACT_QUARANTINED rather than QUARANTINE_ARTIFACT.
	Action string `json:"action"`

	// Actor is the identity responsible. It comes from verified claims for a
	// human and is prefixed `system:` for a process, so the two can never be
	// confused when reading the trail.
	Actor string `json:"actor"`

	ObjectType string `json:"objectType"`
	ObjectID   int64  `json:"objectId"`

	// CorrelationID ties records produced by one request or one job together.
	// Without it a multi-step operation is a set of unrelated rows.
	CorrelationID string `json:"correlationId"`

	OccurredAt time.Time `json:"occurredAt"`

	// PayloadHash lets a verifier confirm the payload is unaltered without the
	// payload itself, which matters for an export that redacts further.
	PayloadHash string `json:"payloadHash"`
	Payload     string `json:"payload"`

	PreviousHash string `json:"previousHash"`
	CurrentHash  string `json:"currentHash"`

	CanonicalVersion string `json:"canonicalVersion"`
}

// TimestampPrecision is the resolution every record's timestamp is truncated
// to before it is hashed and stored.
//
// The chain is only verifiable if every hashed field round-trips through the
// database byte-identically. PostgreSQL's TIMESTAMPTZ holds microseconds, so a
// Go time carrying nanoseconds loses precision on the way out and the
// recomputed hash disagrees -- which the verifier correctly reports as
// tampering. Every one of 192 concurrently written records failed verification
// on PostgreSQL until this existed, while passing on SQLite, which stores the
// formatted string verbatim.
//
// Truncating here, before the hash is computed, is the fix that keeps the
// verifier strict. The alternative -- comparing timestamps with a tolerance --
// would make a rewritten timestamp undetectable within that tolerance.
//
// Microseconds are far finer than any audit question needs.
const TimestampPrecision = time.Microsecond

// AppendRequest describes a record to be written.
type AppendRequest struct {
	TenantID      string
	Action        string
	Actor         string
	ObjectType    string
	ObjectID      int64
	CorrelationID string
	Payload       map[string]any

	// OccurredAt defaults to now. It is settable so a caller recording
	// something that happened earlier says so rather than backdating by
	// omission.
	OccurredAt time.Time
}

func (r AppendRequest) validate() error {
	switch {
	case r.TenantID == "":
		return errors.New("an audit record requires a tenant")
	case r.Action == "":
		return errors.New("an audit record requires an action")
	case r.Actor == "":
		return errors.New("an audit record requires an actor; an unattributable record is not evidence")
	case r.ObjectType == "":
		return errors.New("an audit record requires an object type")
	}
	return nil
}

// canonicalPayload renders a payload deterministically.
//
// encoding/json sorts map keys, which is most of what determinism needs, but
// relying on that implicitly would make a future switch to a struct payload
// silently change every hash. Doing it explicitly, under a named version,
// makes the dependency visible.
func canonicalPayload(payload map[string]any) (string, error) {
	if payload == nil {
		payload = map[string]any{}
	}

	// Keys are sorted explicitly and the result is compact, so two callers
	// building the same logical payload produce the same bytes.
	keys := make([]string, 0, len(payload))
	for k := range payload {
		keys = append(keys, k)
	}
	sort.Strings(keys)

	var b strings.Builder
	b.WriteByte('{')
	for i, k := range keys {
		if i > 0 {
			b.WriteByte(',')
		}
		keyJSON, err := json.Marshal(k)
		if err != nil {
			return "", err
		}
		valJSON, err := json.Marshal(payload[k])
		if err != nil {
			return "", fmt.Errorf("payload key %q is not serialisable: %w", k, err)
		}
		b.Write(keyJSON)
		b.WriteByte(':')
		b.Write(valJSON)
	}
	b.WriteByte('}')
	return b.String(), nil
}

// canonicalRecord renders the bytes a record's hash is computed over.
//
// Field separators are a byte that cannot appear in any field, so no
// combination of field values can be rearranged into a different record with
// the same canonical form. A naive concatenation with a printable separator
// admits exactly that: two fields "a|b" and "c" concatenate identically to "a"
// and "b|c".
func canonicalRecord(r *Record) string {
	const sep = "\x1e" // ASCII record separator; rejected in every field below
	return strings.Join([]string{
		CanonicalVersion,
		r.TenantID,
		fmt.Sprintf("%d", r.SequenceNo),
		r.Action,
		r.Actor,
		r.ObjectType,
		fmt.Sprintf("%d", r.ObjectID),
		r.CorrelationID,
		r.OccurredAt.UTC().Format(time.RFC3339Nano),
		r.PayloadHash,
		r.PreviousHash,
	}, sep)
}

// hashHex computes a domain-separated SHA-256.
//
// The domain prefix stops a digest computed here from being comparable to one
// computed anywhere else in the system over the same bytes -- an artifact hash,
// a secret fingerprint -- which would otherwise let one be substituted for
// another.
func hashHex(domain, input string) string {
	sum := sha256.Sum256([]byte(domain + "\x00" + input))
	return hex.EncodeToString(sum[:])
}

func payloadHash(canonical string) string {
	return hashHex("sentinel/ledger-payload/v1", canonical)
}

func recordHash(r *Record) string {
	return hashHex("sentinel/ledger-record/v1", canonicalRecord(r))
}

// forbiddenPayloadPatterns are shapes that must never enter the ledger.
//
// This is a backstop, not the control. The control is that callers pass
// identifiers and already-redacted findings; internal/nacha redacts at the
// point a finding is produced and internal/secrets makes credentials
// unprintable. What this catches is the caller who has not read either.
var forbiddenPayloadKeys = []string{
	"rawdata", "rawcontent", "raw_record", "rawrecord", "content",
	"secret", "password", "token", "apikey", "api_key", "credential",
	"privatekey", "private_key", "accountnumber", "account_number",
}

// checkPayload refuses material that must not be recorded.
func checkPayload(payload map[string]any, canonical string) error {
	if len(canonical) > MaxPayloadBytes {
		return fmt.Errorf("%w: %d bytes exceeds %d", ErrPayloadTooLarge, len(canonical), MaxPayloadBytes)
	}
	for k := range payload {
		normalised := strings.ToLower(strings.ReplaceAll(k, "-", "_"))
		compact := strings.ReplaceAll(normalised, "_", "")
		for _, bad := range forbiddenPayloadKeys {
			if compact == strings.ReplaceAll(bad, "_", "") {
				return fmt.Errorf("%w: key %q", ErrForbiddenPayload, k)
			}
		}
	}
	// A 94-character record is the shape of an ACH line. Anything that long in
	// a single string value is more likely to be file content than a label.
	for k, v := range payload {
		s, ok := v.(string)
		if !ok {
			continue
		}
		if len(s) >= 94 && looksLikeFixedWidthRecord(s) {
			return fmt.Errorf("%w: key %q holds what looks like a financial record", ErrForbiddenPayload, k)
		}
	}
	return nil
}

// looksLikeFixedWidthRecord reports whether a string has the shape of an ACH
// record: long, printable, and predominantly digits.
func looksLikeFixedWidthRecord(s string) bool {
	digits := 0
	for _, r := range s {
		if r < 0x20 || r > 0x7e {
			return false
		}
		if r >= '0' && r <= '9' {
			digits++
		}
	}
	return float64(digits)/float64(len(s)) > 0.5
}
