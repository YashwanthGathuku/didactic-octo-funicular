package secrets

import (
	"context"
	"crypto/subtle"
	"database/sql"
	"errors"
	"fmt"
	"sort"
	"time"
)

// SQLStore is the durable adapter.
//
// It is written against database/sql with positional parameters so the same
// code serves SQLite today and PostgreSQL after the port. Every statement is
// parameterised; nothing in this file concatenates a caller-supplied string
// into SQL.
//
// The production contract it must satisfy beyond the interface:
//   - its Sealer's key comes from the environment, never from this database,
//     so a database compromise alone does not yield credentials
//   - RequireDurableSealer has been called, so a process-scoped key cannot be
//     used for storage that outlives the process
type SQLStore struct {
	db     *sql.DB
	sealer Sealer
	now    func() time.Time
}

// NewSQLStore builds the durable adapter.
//
// The sealer is mandatory even for a store that will only ever hold KindVerify
// secrets: making it optional would mean a later KindRetrieve write fails at
// runtime instead of at construction.
func NewSQLStore(db *sql.DB, sealer Sealer) (*SQLStore, error) {
	if db == nil {
		return nil, errors.New("secret store requires a database handle")
	}
	if sealer == nil {
		return nil, ErrNoSealer
	}
	return &SQLStore{db: db, sealer: sealer, now: time.Now}, nil
}

// SetClock replaces the time source so rotation overlap can be tested without
// sleeping.
func (s *SQLStore) SetClock(fn func() time.Time) { s.now = fn }

// row is the stored shape of one version.
type row struct {
	secretID    string
	tenantID    string
	name        string
	kind        Kind
	version     int
	fingerprint string
	salt        []byte
	digest      []byte
	sealed      []byte
	createdAt   time.Time
	createdBy   string
	rotatedAt   *time.Time
	lastUsedAt  *time.Time
	notAfter    *time.Time
	retiredAt   *time.Time
}

func (r row) reference() Reference {
	return Reference{
		ID: r.secretID, TenantID: r.tenantID, Name: r.name, Kind: r.kind,
		Version: r.version, Fingerprint: r.fingerprint,
		CreatedAt: r.createdAt, CreatedBy: r.createdBy,
		RotatedAt: r.rotatedAt, LastUsedAt: r.lastUsedAt,
		NotAfter: r.notAfter, Retired: r.retiredAt != nil,
	}
}

const selectColumns = `secret_id, tenant_id, name, kind, version, fingerprint,
	salt, digest, sealed, created_at, created_by, rotated_at, last_used_at,
	not_after, retired_at`

func scanRow(sc interface{ Scan(...any) error }) (row, error) {
	var r row
	var kind string
	err := sc.Scan(&r.secretID, &r.tenantID, &r.name, &kind, &r.version, &r.fingerprint,
		&r.salt, &r.digest, &r.sealed, &r.createdAt, &r.createdBy, &r.rotatedAt,
		&r.lastUsedAt, &r.notAfter, &r.retiredAt)
	r.kind = Kind(kind)
	return r, err
}

// versions returns every version of one secret, oldest first.
func (s *SQLStore) versions(ctx context.Context, tenantID, name string) ([]row, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT `+selectColumns+`
		FROM secret_versions
		WHERE tenant_id = ? AND name = ?
		ORDER BY version ASC`, tenantID, name)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []row
	for rows.Next() {
		r, err := scanRow(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, r)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	if len(out) == 0 {
		return nil, ErrSecretNotFound
	}
	return out, nil
}

// activeOf returns the highest non-retired version.
func activeOf(rs []row) (row, bool) {
	for i := len(rs) - 1; i >= 0; i-- {
		if rs[i].retiredAt == nil {
			return rs[i], true
		}
	}
	return row{}, false
}

func (s *SQLStore) recordEvent(ctx context.Context, tx *sql.Tx, e Event) error {
	exec := func(q string, args ...any) error {
		var err error
		if tx != nil {
			_, err = tx.ExecContext(ctx, q, args...)
		} else {
			_, err = s.db.ExecContext(ctx, q, args...)
		}
		return err
	}
	return exec(`
		INSERT INTO secret_events (tenant_id, secret_id, name, version, action, actor, fingerprint, created_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?)`,
		e.TenantID, e.SecretID, e.Name, e.Version, e.Action, e.Actor, e.Fingerprint, e.At)
}

// Create mints or imports a credential and returns it exactly once.
func (s *SQLStore) Create(ctx context.Context, sc Scope, req CreateRequest) (Reference, Value, error) {
	if err := sc.valid(); err != nil {
		return Reference{}, Value{}, err
	}
	if err := validateName(req.Name); err != nil {
		return Reference{}, Value{}, err
	}
	if req.Kind != KindVerify && req.Kind != KindRetrieve {
		return Reference{}, Value{}, fmt.Errorf("unknown secret kind %q", req.Kind)
	}

	if _, err := s.versions(ctx, sc.tenantID, req.Name); err == nil {
		return Reference{}, Value{}, ErrAlreadyExists
	} else if !errors.Is(err, ErrSecretNotFound) {
		return Reference{}, Value{}, err
	}

	value, err := resolveValue(req.Import)
	if err != nil {
		return Reference{}, Value{}, err
	}
	salt, digest, sealed, keyID, err := buildVersion(s.sealer, req.Kind, value)
	if err != nil {
		return Reference{}, Value{}, err
	}
	id, err := newSecretID()
	if err != nil {
		return Reference{}, Value{}, err
	}

	now := s.now().UTC()
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return Reference{}, Value{}, err
	}
	defer func() { _ = tx.Rollback() }()

	if _, err := tx.ExecContext(ctx, `
		INSERT INTO secret_versions
			(secret_id, tenant_id, name, kind, version, fingerprint, salt, digest, sealed, key_id, created_at, created_by)
		VALUES (?, ?, ?, ?, 1, ?, ?, ?, ?, ?, ?, ?)`,
		id, sc.tenantID, req.Name, string(req.Kind), value.Fingerprint(),
		salt, digest, sealed, nullIfEmpty(keyID), now, sc.ActorID()); err != nil {
		return Reference{}, Value{}, err
	}
	if err := s.recordEvent(ctx, tx, Event{
		At: now, TenantID: sc.tenantID, SecretID: id, Name: req.Name, Version: 1,
		Action: ActionCreated, Actor: sc.ActorID(), Fingerprint: value.Fingerprint(),
	}); err != nil {
		return Reference{}, Value{}, err
	}
	if err := tx.Commit(); err != nil {
		return Reference{}, Value{}, err
	}

	return Reference{
		ID: id, TenantID: sc.tenantID, Name: req.Name, Kind: req.Kind, Version: 1,
		Fingerprint: value.Fingerprint(), CreatedAt: now, CreatedBy: sc.ActorID(),
	}, value, nil
}

// nullIfEmpty keeps the schema's kind CHECK satisfiable: key_id must be NULL,
// not the empty string, for a KindVerify row.
func nullIfEmpty(s string) any {
	if s == "" {
		return nil
	}
	return s
}

// Get returns metadata for the active version. It cannot return a value.
func (s *SQLStore) Get(ctx context.Context, sc Scope, name string) (Reference, error) {
	if err := sc.valid(); err != nil {
		return Reference{}, err
	}
	rs, err := s.versions(ctx, sc.tenantID, name)
	if err != nil {
		return Reference{}, err
	}
	active, ok := activeOf(rs)
	if !ok {
		return Reference{}, ErrSecretNotFound
	}
	return active.reference(), nil
}

// List returns the active version of every secret in the tenant.
func (s *SQLStore) List(ctx context.Context, sc Scope) ([]Reference, error) {
	if err := sc.valid(); err != nil {
		return nil, err
	}
	rows, err := s.db.QueryContext(ctx, `
		SELECT `+selectColumns+`
		FROM secret_versions
		WHERE tenant_id = ? AND retired_at IS NULL
		ORDER BY name ASC, version ASC`, sc.tenantID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	// Keep only the highest live version per name.
	latest := map[string]row{}
	for rows.Next() {
		r, err := scanRow(rows)
		if err != nil {
			return nil, err
		}
		if prev, ok := latest[r.name]; !ok || r.version > prev.version {
			latest[r.name] = r
		}
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}

	out := make([]Reference, 0, len(latest))
	for _, r := range latest {
		out = append(out, r.reference())
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out, nil
}

// Rotate appends a new active version and leaves the previous one verifiable
// for the overlap window.
func (s *SQLStore) Rotate(ctx context.Context, sc Scope, name string, overlap time.Duration) (Reference, Value, error) {
	if err := sc.valid(); err != nil {
		return Reference{}, Value{}, err
	}
	if overlap < 0 {
		return Reference{}, Value{}, errors.New("rotation overlap cannot be negative")
	}

	rs, err := s.versions(ctx, sc.tenantID, name)
	if err != nil {
		return Reference{}, Value{}, err
	}
	previous, ok := activeOf(rs)
	if !ok {
		return Reference{}, Value{}, ErrSecretNotFound
	}

	value, err := Generate()
	if err != nil {
		return Reference{}, Value{}, err
	}
	salt, digest, sealed, keyID, err := buildVersion(s.sealer, previous.kind, value)
	if err != nil {
		return Reference{}, Value{}, err
	}

	now := s.now().UTC()
	cutoff := now.Add(overlap)

	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return Reference{}, Value{}, err
	}
	defer func() { _ = tx.Rollback() }()

	// Only lifecycle columns are touched on the previous row; the immutability
	// trigger refuses anything else.
	var retiredAt any
	if overlap == 0 {
		retiredAt = now
	}
	if _, err := tx.ExecContext(ctx, `
		UPDATE secret_versions SET not_after = ?, retired_at = ?
		WHERE secret_id = ? AND version = ?`,
		cutoff, retiredAt, previous.secretID, previous.version); err != nil {
		return Reference{}, Value{}, err
	}
	if _, err := tx.ExecContext(ctx, `
		INSERT INTO secret_versions
			(secret_id, tenant_id, name, kind, version, fingerprint, salt, digest, sealed, key_id, created_at, created_by, rotated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		previous.secretID, sc.tenantID, name, string(previous.kind), previous.version+1,
		value.Fingerprint(), salt, digest, sealed, nullIfEmpty(keyID), now, sc.ActorID(), now); err != nil {
		return Reference{}, Value{}, err
	}
	if err := s.recordEvent(ctx, tx, Event{
		At: now, TenantID: sc.tenantID, SecretID: previous.secretID, Name: name,
		Version: previous.version + 1, Action: ActionRotated, Actor: sc.ActorID(),
		Fingerprint: value.Fingerprint(),
	}); err != nil {
		return Reference{}, Value{}, err
	}
	if err := tx.Commit(); err != nil {
		return Reference{}, Value{}, err
	}

	return Reference{
		ID: previous.secretID, TenantID: sc.tenantID, Name: name, Kind: previous.kind,
		Version: previous.version + 1, Fingerprint: value.Fingerprint(),
		CreatedAt: now, CreatedBy: sc.ActorID(), RotatedAt: &now,
	}, value, nil
}

// Retire ends a version's validity immediately.
func (s *SQLStore) Retire(ctx context.Context, sc Scope, name string, version int) error {
	if err := sc.valid(); err != nil {
		return err
	}
	rs, err := s.versions(ctx, sc.tenantID, name)
	if err != nil {
		return err
	}
	for _, r := range rs {
		if r.version != version {
			continue
		}
		if r.retiredAt != nil {
			return nil // idempotent
		}
		now := s.now().UTC()
		if _, err := s.db.ExecContext(ctx, `
			UPDATE secret_versions SET retired_at = ?
			WHERE secret_id = ? AND version = ?`, now, r.secretID, version); err != nil {
			return err
		}
		return s.recordEvent(ctx, nil, Event{
			At: now, TenantID: sc.tenantID, SecretID: r.secretID, Name: name,
			Version: version, Action: ActionRetired, Actor: sc.ActorID(),
			Fingerprint: r.fingerprint,
		})
	}
	return ErrSecretNotFound
}

// Verify checks a presented credential against every currently valid version.
func (s *SQLStore) Verify(ctx context.Context, tenantID, name, presented string) (Reference, error) {
	rs, err := s.versions(ctx, tenantID, name)
	if err != nil {
		return Reference{}, ErrVerificationFailed
	}

	now := s.now().UTC()
	var matched *row
	for i := range rs {
		r := rs[i]
		if r.kind != KindVerify || r.retiredAt != nil || r.digest == nil {
			continue
		}
		if r.notAfter != nil && now.After(*r.notAfter) {
			continue
		}
		if subtle.ConstantTimeCompare(r.digest, digestOf(r.salt, presented)) == 1 {
			matched = &rs[i]
		}
	}
	if matched == nil {
		_ = s.recordEvent(ctx, nil, Event{
			At: now, TenantID: tenantID, SecretID: rs[0].secretID, Name: name,
			Action: ActionRejected, Actor: "anonymous",
		})
		return Reference{}, ErrVerificationFailed
	}

	if _, err := s.db.ExecContext(ctx, `
		UPDATE secret_versions SET last_used_at = ?
		WHERE secret_id = ? AND version = ?`, now, matched.secretID, matched.version); err != nil {
		return Reference{}, err
	}
	if err := s.recordEvent(ctx, nil, Event{
		At: now, TenantID: tenantID, SecretID: matched.secretID, Name: name,
		Version: matched.version, Action: ActionVerified, Actor: "credential-holder",
		Fingerprint: matched.fingerprint,
	}); err != nil {
		return Reference{}, err
	}
	matched.lastUsedAt = &now
	return matched.reference(), nil
}

// Use runs fn with the credential and stamps last-used.
func (s *SQLStore) Use(ctx context.Context, sc Scope, name string, fn func(Value) error) error {
	if err := sc.valid(); err != nil {
		return err
	}
	rs, err := s.versions(ctx, sc.tenantID, name)
	if err != nil {
		return err
	}
	active, ok := activeOf(rs)
	if !ok {
		return ErrSecretNotFound
	}
	if active.kind != KindRetrieve || active.sealed == nil {
		return ErrNotRetrievable
	}

	plaintext, err := s.sealer.Open(active.sealed)
	if err != nil {
		return err
	}
	value, err := New(string(plaintext))
	if err != nil {
		return err
	}

	now := s.now().UTC()
	if _, err := s.db.ExecContext(ctx, `
		UPDATE secret_versions SET last_used_at = ?
		WHERE secret_id = ? AND version = ?`, now, active.secretID, active.version); err != nil {
		return err
	}
	if err := s.recordEvent(ctx, nil, Event{
		At: now, TenantID: sc.tenantID, SecretID: active.secretID, Name: name,
		Version: active.version, Action: ActionUsed, Actor: sc.ActorID(),
		Fingerprint: active.fingerprint,
	}); err != nil {
		return err
	}
	return fn(value)
}

// Events returns the audit trail for one secret, newest first.
func (s *SQLStore) Events(ctx context.Context, sc Scope, name string) ([]Event, error) {
	if err := sc.valid(); err != nil {
		return nil, err
	}
	if _, err := s.versions(ctx, sc.tenantID, name); err != nil {
		return nil, err
	}
	rows, err := s.db.QueryContext(ctx, `
		SELECT created_at, tenant_id, secret_id, name, version, action, actor, fingerprint
		FROM secret_events
		WHERE tenant_id = ? AND name = ?
		ORDER BY id DESC`, sc.tenantID, name)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []Event
	for rows.Next() {
		var e Event
		if err := rows.Scan(&e.At, &e.TenantID, &e.SecretID, &e.Name, &e.Version,
			&e.Action, &e.Actor, &e.Fingerprint); err != nil {
			return nil, err
		}
		out = append(out, e)
	}
	return out, rows.Err()
}
