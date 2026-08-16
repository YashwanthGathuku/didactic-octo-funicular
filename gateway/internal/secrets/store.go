package secrets

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"time"

	"sentinel-gateway/internal/auth"
)

// Kind decides what the store keeps and therefore what it can ever give back.
//
// The distinction is the whole design. A credential this system only ever
// *checks* need not be recoverable, so it is stored as a salted digest and no
// code path -- not a bug, not a SQL console, not a database backup read by an
// attacker -- can produce the original. A credential this system must *present*
// to someone else has to be recoverable, so it is stored sealed and readable
// through exactly one audited method.
//
// Choosing KindVerify wherever it is possible is the point; KindRetrieve is the
// exception that must be justified per secret.
type Kind string

const (
	// KindVerify is for inbound credentials: an API token or a scrape token the
	// caller presents and this system compares. Stored as a digest. Never
	// recoverable, by anyone, including an operator.
	KindVerify Kind = "VERIFY"

	// KindRetrieve is for outbound credentials this system must present: an
	// OIDC client secret, an HMAC signing key. Stored sealed under a key that
	// lives outside the database, and readable only inside Use.
	KindRetrieve Kind = "RETRIEVE"
)

// PlatformScope is the reserved tenant for credentials that belong to the
// deployment rather than to a customer -- the metrics scrape token, for example.
// It is a real value rather than an empty string so a missing tenant can never
// be mistaken for a platform secret.
const PlatformScope = "PLATFORM"

var (
	// ErrSecretNotFound is returned for an unknown id and for an id belonging to
	// another tenant, with identical text. Distinguishing them would confirm the
	// existence of another tenant's credential.
	ErrSecretNotFound = errors.New("secret not found in this tenant")

	// ErrVerificationFailed is the single failure of Verify.
	//
	// Unknown name, wrong value, retired version and expired overlap all return
	// this. A caller must not be able to tell which, or the error becomes an
	// oracle for enumerating credential names.
	ErrVerificationFailed = errors.New("credential verification failed")

	// ErrNotRetrievable is returned by Use for a KindVerify secret. It is a
	// programming error, not a runtime condition.
	ErrNotRetrievable = errors.New("this secret is stored as a digest and cannot be retrieved")

	// ErrAlreadyExists prevents a silent overwrite of a live credential.
	ErrAlreadyExists = errors.New("a secret with this name already exists in this tenant")

	// ErrNoSealer is returned when a store that must seal values has no sealer.
	ErrNoSealer = errors.New("secret store has no sealer; retrievable secrets cannot be stored")
)

// Scope carries the verified tenant and the principal that authorized the call.
//
// It mirrors repository.Scope deliberately: the two boundaries have the same
// requirement, that a tenant can only be reached through a principal that was
// authorized for it, and there is no exported way to build one from a request
// field.
type Scope struct {
	tenantID  string
	principal *auth.Principal
}

// NewScope binds a principal to a tenant for secret administration.
//
// The permission is fixed rather than a parameter: there is exactly one
// authority over credentials and it is not something a caller chooses.
func NewScope(p *auth.Principal, tenantID string) (Scope, error) {
	if p == nil || p.Subject == "" {
		return Scope{}, auth.ErrNoPrincipal
	}
	if tenantID == "" {
		return Scope{}, ErrSecretNotFound
	}
	if err := p.Authorize(tenantID, auth.PermManageSecret); err != nil {
		return Scope{}, err
	}
	return Scope{tenantID: tenantID, principal: p}, nil
}

// TenantID returns the scope's tenant.
func (s Scope) TenantID() string { return s.tenantID }

// ActorID returns the verified subject recorded against every change.
func (s Scope) ActorID() string { return s.principal.ActorID() }

func (s Scope) valid() error {
	if s.tenantID == "" || s.principal == nil {
		return ErrSecretNotFound
	}
	return nil
}

// systemScope is for startup and background work that has no request and
// therefore no principal. It is unexported and constructed only by SystemScope,
// which names the subsystem doing the work so the audit record is attributable
// to something.
func systemScope(tenantID, subsystem string) Scope {
	return Scope{
		tenantID: tenantID,
		principal: &auth.Principal{
			Subject: "system:" + subsystem,
			Issuer:  "sentinel-gateway",
		},
	}
}

// SystemScope is the scope for process-owned credentials: those created at
// startup or rotated by a scheduled job, where no human is present.
//
// The subsystem name is mandatory and lands in the audit record, so "who
// rotated this" has an answer that is not "the system".
func SystemScope(tenantID, subsystem string) (Scope, error) {
	if tenantID == "" || subsystem == "" {
		return Scope{}, errors.New("a system scope requires a tenant and a named subsystem")
	}
	return systemScope(tenantID, subsystem), nil
}

// Reference is everything about a secret that is safe to return, log, export
// and persist in an audit record.
//
// It holds no credential material and no field from which one could be derived.
// Every method of Store except Create and Rotate returns this and nothing else.
type Reference struct {
	ID          string     `json:"id"`
	TenantID    string     `json:"tenantId"`
	Name        string     `json:"name"`
	Kind        Kind       `json:"kind"`
	Version     int        `json:"version"`
	Fingerprint string     `json:"fingerprint"`
	CreatedAt   time.Time  `json:"createdAt"`
	CreatedBy   string     `json:"createdBy"`
	RotatedAt   *time.Time `json:"rotatedAt,omitempty"`
	LastUsedAt  *time.Time `json:"lastUsedAt,omitempty"`

	// NotAfter is set on a superseded version during its rotation overlap. A
	// nil value on an active version means it does not expire on its own.
	NotAfter *time.Time `json:"notAfter,omitempty"`

	// Retired versions verify nothing. The field is reported so an operator can
	// see that a rotation completed rather than inferring it.
	Retired bool `json:"retired"`
}

// Event is one audited change to a credential.
//
// Rotation evidence is a requirement, not a convenience: an auditor asking
// "when was this key last changed and by whom" must be answerable without
// anyone handling the key.
type Event struct {
	At       time.Time `json:"at"`
	TenantID string    `json:"tenantId"`
	SecretID string    `json:"secretId"`
	Name     string    `json:"name"`
	Version  int       `json:"version"`
	Action   string    `json:"action"`
	Actor    string    `json:"actor"`
	// Fingerprint identifies which credential the event concerns without
	// containing it.
	Fingerprint string `json:"fingerprint"`
}

// Event actions.
const (
	ActionCreated  = "SECRET_CREATED"
	ActionRotated  = "SECRET_ROTATED"
	ActionRetired  = "SECRET_RETIRED"
	ActionUsed     = "SECRET_USED"
	ActionVerified = "SECRET_VERIFIED"
	ActionRejected = "SECRET_VERIFY_REJECTED"
)

// CreateRequest describes a new credential.
type CreateRequest struct {
	Name string
	Kind Kind

	// Import supplies an existing credential instead of generating one. It is
	// for values minted elsewhere -- an OIDC client secret issued by the
	// identity provider. Leave it zero and the store generates from crypto/rand,
	// which is the path to prefer.
	Import Value
}

// Store is the credential boundary.
//
// The shape enforces write-once display. Create and Rotate return a Value
// because that is the only moment it can be shown; no other method can return
// one. Reading a stored credential for retrievable kinds happens inside Use,
// which never hands the caller a Value it can retain past the callback.
//
// Adapter contracts, which every implementation must satisfy and the shared
// conformance suite in store_conformance_test.go checks:
//
//   - Create returns the Value exactly once. A subsequent Get, List or any
//     other call must be incapable of returning it.
//   - No implementation may write plaintext credential material to durable
//     storage. KindVerify stores a salted digest; KindRetrieve stores
//     ciphertext produced by a Sealer whose key lives outside the store.
//   - Verify returns ErrVerificationFailed for every failure without
//     distinguishing them, and compares in constant time.
//   - Rotate leaves the previous version verifiable for the overlap window, so
//     a caller that has not yet picked up the new credential is not locked out
//     mid-deployment.
//   - Every mutating call and every successful verification records an Event.
//   - Every method is tenant-scoped; a foreign id is ErrSecretNotFound, with
//     text identical to an id that does not exist.
type Store interface {
	// Create mints or imports a credential and returns it once.
	Create(ctx context.Context, s Scope, req CreateRequest) (Reference, Value, error)

	// Get returns metadata for the active version. It cannot return a value.
	Get(ctx context.Context, s Scope, name string) (Reference, error)

	// List returns metadata for every secret in the tenant, active versions
	// only. It cannot return a value.
	List(ctx context.Context, s Scope) ([]Reference, error)

	// Rotate mints a new active version and keeps the previous one verifiable
	// for the overlap window. It returns the new value once.
	Rotate(ctx context.Context, s Scope, name string, overlap time.Duration) (Reference, Value, error)

	// Retire ends a version's validity immediately, cutting an overlap short.
	Retire(ctx context.Context, s Scope, name string, version int) error

	// Verify checks a presented credential.
	//
	// It takes no Scope because it runs before there is a principal: verifying
	// an inbound credential is how a caller becomes authenticated. The tenant
	// and name are a lookup key supplied by the caller, not an authorization
	// claim, and a wrong pair is indistinguishable from a wrong value.
	Verify(ctx context.Context, tenantID, name, presented string) (Reference, error)

	// Use runs fn with the credential for a KindRetrieve secret and stamps its
	// last-used time. The Value is valid only for the duration of the call; the
	// caller must not retain it.
	Use(ctx context.Context, s Scope, name string, fn func(Value) error) error

	// Events returns the audit trail for one secret, newest first.
	Events(ctx context.Context, s Scope, name string) ([]Event, error)
}

// newSecretID returns an opaque identifier. It carries no timestamp and no
// counter, so it discloses neither creation order nor volume.
func newSecretID() (string, error) {
	buf := make([]byte, 12)
	if _, err := rand.Read(buf); err != nil {
		return "", fmt.Errorf("generate secret id: %w", err)
	}
	return "sec_" + hex.EncodeToString(buf), nil
}

// resolveValue produces the credential for a create or rotate: the imported one
// if supplied, otherwise a freshly generated one.
func resolveValue(imported Value) (Value, error) {
	if !imported.IsZero() {
		return imported, nil
	}
	return Generate()
}

// validateName keeps names printable and bounded. Names appear in logs and
// audit records, so a name carrying control characters or arbitrary length is a
// log-injection vector.
func validateName(name string) error {
	if name == "" {
		return errors.New("a secret requires a name")
	}
	if len(name) > 128 {
		return errors.New("secret name exceeds 128 characters")
	}
	for _, r := range name {
		if r < 0x20 || r == 0x7f {
			return errors.New("secret name contains a control character")
		}
	}
	return nil
}
