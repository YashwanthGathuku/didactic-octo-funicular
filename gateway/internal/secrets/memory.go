package secrets

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"fmt"
	"sort"
	"sync"
	"time"
)

// MemoryStore is the local-development adapter.
//
// It is explicitly non-durable: everything it holds dies with the process, and
// its seal key is process-scoped, so there is no way to mistake it for
// something that could serve production. It exists so a developer can run the
// gateway without provisioning a key management system, and so the conformance
// suite has a second implementation to check the contract against -- a contract
// verified against one implementation is a description of that implementation.
type MemoryStore struct {
	mu      sync.RWMutex
	sealer  Sealer
	now     func() time.Time
	secrets map[string]*memSecret // key: tenantID + "\x00" + name
	events  []Event
}

type memSecret struct {
	id       string
	tenantID string
	name     string
	kind     Kind
	versions []*memVersion
}

type memVersion struct {
	version     int
	fingerprint string
	salt        []byte
	digest      []byte
	sealed      []byte
	keyID       string
	createdAt   time.Time
	createdBy   string
	rotatedAt   *time.Time
	lastUsedAt  *time.Time
	notAfter    *time.Time
	retired     bool
}

// NewMemoryStore builds the development adapter with a process-scoped key.
func NewMemoryStore() (*MemoryStore, error) {
	sealer, err := NewEphemeralSealer()
	if err != nil {
		return nil, err
	}
	return &MemoryStore{
		sealer:  sealer,
		now:     time.Now,
		secrets: map[string]*memSecret{},
	}, nil
}

// SetClock replaces the time source. Rotation overlap is a time-dependent
// security property, and a test that proves it by sleeping proves very little.
func (m *MemoryStore) SetClock(fn func() time.Time) { m.now = fn }

func memKey(tenantID, name string) string { return tenantID + "\x00" + name }

// digestOf is the stored form of a KindVerify secret.
//
// Plain salted SHA-256 rather than a password KDF, deliberately. These values
// are 256-bit random strings produced by Generate, not human-chosen passwords,
// so there is no dictionary to slow down; a KDF would add latency to every
// inbound request and buy nothing. The salt is still present so two tenants
// holding the same credential do not produce the same stored digest, which
// would let a database reader detect the reuse.
//
// This reasoning depends on MinSecretLength being enforced on import. If that
// were relaxed to admit operator-chosen values, this would need to become
// Argon2id.
func digestOf(salt []byte, raw string) []byte {
	h := sha256.New()
	h.Write([]byte("sentinel/secret-digest/v1\x00"))
	h.Write(salt)
	h.Write([]byte(raw))
	return h.Sum(nil)
}

func newSalt() ([]byte, error) {
	salt := make([]byte, 32)
	if _, err := rand.Read(salt); err != nil {
		return nil, fmt.Errorf("generate salt: %w", err)
	}
	return salt, nil
}

// buildVersion turns a plaintext credential into the stored representation for
// its kind. Neither branch retains the plaintext.
func buildVersion(sealer Sealer, kind Kind, v Value) (salt, digest, sealed []byte, keyID string, err error) {
	switch kind {
	case KindVerify:
		salt, err = newSalt()
		if err != nil {
			return nil, nil, nil, "", err
		}
		return salt, digestOf(salt, v.Expose()), nil, "", nil
	case KindRetrieve:
		if sealer == nil {
			return nil, nil, nil, "", ErrNoSealer
		}
		sealed, err = sealer.Seal([]byte(v.Expose()))
		if err != nil {
			return nil, nil, nil, "", err
		}
		return nil, nil, sealed, sealer.KeyID(), nil
	default:
		return nil, nil, nil, "", fmt.Errorf("unknown secret kind %q", kind)
	}
}

func (v *memVersion) reference(s *memSecret) Reference {
	return Reference{
		ID:          s.id,
		TenantID:    s.tenantID,
		Name:        s.name,
		Kind:        s.kind,
		Version:     v.version,
		Fingerprint: v.fingerprint,
		CreatedAt:   v.createdAt,
		CreatedBy:   v.createdBy,
		RotatedAt:   v.rotatedAt,
		LastUsedAt:  v.lastUsedAt,
		NotAfter:    v.notAfter,
		Retired:     v.retired,
	}
}

// active returns the highest non-retired version.
func (s *memSecret) active() *memVersion {
	for i := len(s.versions) - 1; i >= 0; i-- {
		if !s.versions[i].retired {
			return s.versions[i]
		}
	}
	return nil
}

func (m *MemoryStore) record(ev Event) { m.events = append(m.events, ev) }

// Create mints or imports a credential and returns it exactly once.
func (m *MemoryStore) Create(ctx context.Context, s Scope, req CreateRequest) (Reference, Value, error) {
	if err := s.valid(); err != nil {
		return Reference{}, Value{}, err
	}
	if err := validateName(req.Name); err != nil {
		return Reference{}, Value{}, err
	}

	m.mu.Lock()
	defer m.mu.Unlock()

	if _, exists := m.secrets[memKey(s.tenantID, req.Name)]; exists {
		// Refuse rather than overwrite. Silently replacing a live credential
		// would break every caller holding the old one with no audit trail
		// saying it happened.
		return Reference{}, Value{}, ErrAlreadyExists
	}

	value, err := resolveValue(req.Import)
	if err != nil {
		return Reference{}, Value{}, err
	}
	salt, digest, sealed, keyID, err := buildVersion(m.sealer, req.Kind, value)
	if err != nil {
		return Reference{}, Value{}, err
	}
	id, err := newSecretID()
	if err != nil {
		return Reference{}, Value{}, err
	}

	now := m.now().UTC()
	sec := &memSecret{id: id, tenantID: s.tenantID, name: req.Name, kind: req.Kind}
	ver := &memVersion{
		version: 1, fingerprint: value.Fingerprint(),
		salt: salt, digest: digest, sealed: sealed, keyID: keyID,
		createdAt: now, createdBy: s.ActorID(),
	}
	sec.versions = append(sec.versions, ver)
	m.secrets[memKey(s.tenantID, req.Name)] = sec

	m.record(Event{
		At: now, TenantID: s.tenantID, SecretID: id, Name: req.Name,
		Version: 1, Action: ActionCreated, Actor: s.ActorID(),
		Fingerprint: value.Fingerprint(),
	})
	return ver.reference(sec), value, nil
}

func (m *MemoryStore) lookup(tenantID, name string) (*memSecret, error) {
	sec, ok := m.secrets[memKey(tenantID, name)]
	if !ok {
		return nil, ErrSecretNotFound
	}
	return sec, nil
}

// Get returns metadata only. There is no variant of this that returns a value.
func (m *MemoryStore) Get(ctx context.Context, s Scope, name string) (Reference, error) {
	if err := s.valid(); err != nil {
		return Reference{}, err
	}
	m.mu.RLock()
	defer m.mu.RUnlock()

	sec, err := m.lookup(s.tenantID, name)
	if err != nil {
		return Reference{}, err
	}
	active := sec.active()
	if active == nil {
		return Reference{}, ErrSecretNotFound
	}
	return active.reference(sec), nil
}

// List returns the active version of every secret in the tenant.
func (m *MemoryStore) List(ctx context.Context, s Scope) ([]Reference, error) {
	if err := s.valid(); err != nil {
		return nil, err
	}
	m.mu.RLock()
	defer m.mu.RUnlock()

	var out []Reference
	for _, sec := range m.secrets {
		if sec.tenantID != s.tenantID {
			continue
		}
		if active := sec.active(); active != nil {
			out = append(out, active.reference(sec))
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out, nil
}

// Rotate mints a new active version and leaves the previous one verifiable for
// the overlap window.
//
// The overlap is the difference between a rotation and an outage. Without it,
// the instant a new credential is minted every holder of the old one is
// rejected, which in practice means rotations get postponed indefinitely.
func (m *MemoryStore) Rotate(ctx context.Context, s Scope, name string, overlap time.Duration) (Reference, Value, error) {
	if err := s.valid(); err != nil {
		return Reference{}, Value{}, err
	}
	if overlap < 0 {
		return Reference{}, Value{}, fmt.Errorf("rotation overlap cannot be negative")
	}

	m.mu.Lock()
	defer m.mu.Unlock()

	sec, err := m.lookup(s.tenantID, name)
	if err != nil {
		return Reference{}, Value{}, err
	}
	previous := sec.active()
	if previous == nil {
		return Reference{}, Value{}, ErrSecretNotFound
	}

	value, err := resolveValue(Value{})
	if err != nil {
		return Reference{}, Value{}, err
	}
	salt, digest, sealed, keyID, err := buildVersion(m.sealer, sec.kind, value)
	if err != nil {
		return Reference{}, Value{}, err
	}

	now := m.now().UTC()
	cutoff := now.Add(overlap)
	previous.notAfter = &cutoff
	if overlap == 0 {
		previous.retired = true
	}

	ver := &memVersion{
		version: previous.version + 1, fingerprint: value.Fingerprint(),
		salt: salt, digest: digest, sealed: sealed, keyID: keyID,
		createdAt: now, createdBy: s.ActorID(), rotatedAt: &now,
	}
	sec.versions = append(sec.versions, ver)

	m.record(Event{
		At: now, TenantID: s.tenantID, SecretID: sec.id, Name: name,
		Version: ver.version, Action: ActionRotated, Actor: s.ActorID(),
		Fingerprint: value.Fingerprint(),
	})
	return ver.reference(sec), value, nil
}

// Retire ends a version's validity immediately.
func (m *MemoryStore) Retire(ctx context.Context, s Scope, name string, version int) error {
	if err := s.valid(); err != nil {
		return err
	}
	m.mu.Lock()
	defer m.mu.Unlock()

	sec, err := m.lookup(s.tenantID, name)
	if err != nil {
		return err
	}
	for _, v := range sec.versions {
		if v.version != version {
			continue
		}
		if v.retired {
			return nil // idempotent: retiring a retired version is not an error
		}
		v.retired = true
		now := m.now().UTC()
		m.record(Event{
			At: now, TenantID: s.tenantID, SecretID: sec.id, Name: name,
			Version: version, Action: ActionRetired, Actor: s.ActorID(),
			Fingerprint: v.fingerprint,
		})
		return nil
	}
	return ErrSecretNotFound
}

// Verify checks a presented credential against every currently valid version.
//
// Every failure returns ErrVerificationFailed with no detail. The comparison
// runs against all candidate versions rather than returning on first match, so
// the time taken does not reveal which version matched or how many exist.
func (m *MemoryStore) Verify(ctx context.Context, tenantID, name, presented string) (Reference, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	sec, err := m.lookup(tenantID, name)
	if err != nil {
		return Reference{}, ErrVerificationFailed
	}
	if sec.kind != KindVerify {
		return Reference{}, ErrVerificationFailed
	}

	now := m.now().UTC()
	var matched *memVersion
	for _, v := range sec.versions {
		if v.retired || v.digest == nil {
			continue
		}
		if v.notAfter != nil && now.After(*v.notAfter) {
			continue
		}
		if subtle.ConstantTimeCompare(v.digest, digestOf(v.salt, presented)) == 1 {
			matched = v
		}
	}
	if matched == nil {
		m.record(Event{
			At: now, TenantID: tenantID, SecretID: sec.id, Name: name,
			Action: ActionRejected, Actor: "anonymous",
		})
		return Reference{}, ErrVerificationFailed
	}

	matched.lastUsedAt = &now
	m.record(Event{
		At: now, TenantID: tenantID, SecretID: sec.id, Name: name,
		Version: matched.version, Action: ActionVerified, Actor: "credential-holder",
		Fingerprint: matched.fingerprint,
	})
	return matched.reference(sec), nil
}

// Use runs fn with the credential and stamps last-used.
func (m *MemoryStore) Use(ctx context.Context, s Scope, name string, fn func(Value) error) error {
	if err := s.valid(); err != nil {
		return err
	}
	m.mu.Lock()
	sec, err := m.lookup(s.tenantID, name)
	if err != nil {
		m.mu.Unlock()
		return err
	}
	if sec.kind != KindRetrieve {
		m.mu.Unlock()
		return ErrNotRetrievable
	}
	active := sec.active()
	if active == nil || active.sealed == nil {
		m.mu.Unlock()
		return ErrSecretNotFound
	}
	plaintext, err := m.sealer.Open(active.sealed)
	if err != nil {
		m.mu.Unlock()
		return err
	}
	now := m.now().UTC()
	active.lastUsedAt = &now
	m.record(Event{
		At: now, TenantID: s.tenantID, SecretID: sec.id, Name: name,
		Version: active.version, Action: ActionUsed, Actor: s.ActorID(),
		Fingerprint: active.fingerprint,
	})
	m.mu.Unlock()

	value, err := New(string(plaintext))
	if err != nil {
		return err
	}
	// fn runs outside the lock so a slow caller cannot block the store, and the
	// Value is not retained here afterwards.
	return fn(value)
}

// Events returns the audit trail for one secret, newest first.
func (m *MemoryStore) Events(ctx context.Context, s Scope, name string) ([]Event, error) {
	if err := s.valid(); err != nil {
		return nil, err
	}
	m.mu.RLock()
	defer m.mu.RUnlock()

	if _, err := m.lookup(s.tenantID, name); err != nil {
		return nil, err
	}
	var out []Event
	for _, e := range m.events {
		if e.TenantID == s.tenantID && e.Name == name {
			out = append(out, e)
		}
	}
	sort.SliceStable(out, func(i, j int) bool { return out[i].At.After(out[j].At) })
	return out, nil
}
