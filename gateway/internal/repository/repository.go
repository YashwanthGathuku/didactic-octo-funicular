// Package repository is the only place business records are read or written.
//
// Its purpose is structural: every method takes a Scope and every query filters
// by tenant. A handler cannot forget the filter, because there is no method
// that accepts a raw query. Middleware that checks permissions is necessary but
// not sufficient -- a route added without the middleware would otherwise be an
// unauthenticated read of every tenant's data.
package repository

import (
	"context"
	"database/sql"
	"errors"
	"fmt"

	"sentinel-gateway/internal/auth"
)

var (
	// ErrNoScope means a caller tried to query without a tenant. It is always
	// a programming error and is never recoverable by widening the query.
	ErrNoScope = errors.New("repository call requires a tenant scope")
	// ErrNotFound distinguishes "absent" from "belongs to another tenant".
	// Both return this, deliberately: telling a caller that an ID exists but
	// is not theirs confirms the existence of another tenant's record.
	ErrNotFound = errors.New("record not found in this tenant")
)

// Scope carries the verified tenant and the principal that authorized it.
//
// It can only be built by NewScope, which requires a Principal that has already
// been authorized for the tenant. There is no way to construct one from a
// request field.
type Scope struct {
	tenantID  string
	principal *auth.Principal
}

// NewScope binds a principal to a tenant after checking membership.
func NewScope(p *auth.Principal, tenantID string, perm auth.Permission) (Scope, error) {
	if p == nil || p.Subject == "" {
		return Scope{}, auth.ErrNoPrincipal
	}
	if tenantID == "" {
		return Scope{}, ErrNoScope
	}
	if err := p.Authorize(tenantID, perm); err != nil {
		return Scope{}, err
	}
	return Scope{tenantID: tenantID, principal: p}, nil
}

// TenantID returns the scope's tenant.
func (s Scope) TenantID() string { return s.tenantID }

// ActorID returns the verified subject to record against writes.
func (s Scope) ActorID() string { return s.principal.ActorID() }

// valid reports whether this scope may be used. A zero Scope cannot.
func (s Scope) valid() error {
	if s.tenantID == "" || s.principal == nil {
		return ErrNoScope
	}
	return nil
}

// Repository holds tenant-scoped data access.
type Repository struct {
	db *sql.DB
}

// New builds a Repository.
func New(db *sql.DB) *Repository { return &Repository{db: db} }

// ArtifactRow is a projection of a stored artifact.
type ArtifactRow struct {
	ID         int64
	TenantID   string
	Filename   string
	SHA256     string
	SizeBytes  int64
	Status     string
	ReceivedAt string
}

// ListArtifacts returns artifacts in the scope's tenant only.
func (r *Repository) ListArtifacts(ctx context.Context, s Scope, limit int) ([]ArtifactRow, error) {
	if err := s.valid(); err != nil {
		return nil, err
	}
	if limit <= 0 || limit > 500 {
		limit = 100 // bounded result set; never SELECT * unbounded
	}

	rows, err := r.db.QueryContext(ctx, `
		SELECT id, tenant_id, filename, sha256_hash, size_bytes, status, received_at
		FROM file_instances
		WHERE tenant_id = ?
		ORDER BY id DESC
		LIMIT ?`, s.tenantID, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	out := make([]ArtifactRow, 0, limit)
	for rows.Next() {
		var a ArtifactRow
		if err := rows.Scan(&a.ID, &a.TenantID, &a.Filename, &a.SHA256, &a.SizeBytes, &a.Status, &a.ReceivedAt); err != nil {
			return nil, err
		}
		out = append(out, a)
	}
	return out, rows.Err()
}

// GetArtifact fetches one artifact by ID within the scope's tenant.
//
// An ID belonging to another tenant returns ErrNotFound, identical to an ID
// that does not exist. This is the direct-object-reference defence: the caller
// learns nothing about records outside its tenant.
func (r *Repository) GetArtifact(ctx context.Context, s Scope, id int64) (*ArtifactRow, error) {
	if err := s.valid(); err != nil {
		return nil, err
	}

	var a ArtifactRow
	err := r.db.QueryRowContext(ctx, `
		SELECT id, tenant_id, filename, sha256_hash, size_bytes, status, received_at
		FROM file_instances
		WHERE id = ? AND tenant_id = ?`, id, s.tenantID).
		Scan(&a.ID, &a.TenantID, &a.Filename, &a.SHA256, &a.SizeBytes, &a.Status, &a.ReceivedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	return &a, nil
}

// CountArtifacts is separated from listing because an enumeration defence that
// leaks a count is not a defence.
func (r *Repository) CountArtifacts(ctx context.Context, s Scope) (int, error) {
	if err := s.valid(); err != nil {
		return 0, err
	}
	var n int
	err := r.db.QueryRowContext(ctx,
		`SELECT COUNT(*) FROM file_instances WHERE tenant_id = ?`, s.tenantID).Scan(&n)
	return n, err
}

// UpdateArtifactStatus applies a state change with optimistic concurrency.
//
// The WHERE clause carries the tenant, the expected current state and the
// expected row version, so a stale or cross-tenant update matches zero rows
// rather than silently succeeding.
func (r *Repository) UpdateArtifactStatus(
	ctx context.Context, s Scope, id int64, fromState, toState string, expectedVersion int, reason string,
) error {
	if err := s.valid(); err != nil {
		return err
	}

	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()

	res, err := tx.ExecContext(ctx, `
		UPDATE file_instances
		SET status = ?, row_version = row_version + 1, updated_at = CURRENT_TIMESTAMP
		WHERE id = ? AND tenant_id = ? AND status = ? AND row_version = ?`,
		toState, id, s.tenantID, fromState, expectedVersion)
	if err != nil {
		return err
	}
	n, err := res.RowsAffected()
	if err != nil {
		return err
	}
	if n == 0 {
		// Either the row is not ours, not in the expected state, or another
		// writer moved it first. All three are the same answer to this caller.
		return fmt.Errorf("%w: artifact %d not updatable from %s at version %d",
			ErrNotFound, id, fromState, expectedVersion)
	}

	// Status history is append-only and records the verified actor, never a
	// caller-supplied name.
	if _, err := tx.ExecContext(ctx, `
		INSERT INTO status_history (tenant_id, object_type, object_id, from_state, to_state, actor_id, reason)
		VALUES (?, 'artifact', ?, ?, ?, ?, ?)`,
		s.tenantID, id, fromState, toState, s.ActorID(), reason); err != nil {
		return err
	}

	return tx.Commit()
}

// ListIncidents returns incidents in the scope's tenant only.
func (r *Repository) ListIncidents(ctx context.Context, s Scope, limit int) ([]int64, error) {
	if err := s.valid(); err != nil {
		return nil, err
	}
	if limit <= 0 || limit > 500 {
		limit = 100
	}
	rows, err := r.db.QueryContext(ctx,
		`SELECT id FROM incidents WHERE tenant_id = ? ORDER BY id DESC LIMIT ?`, s.tenantID, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []int64
	for rows.Next() {
		var id int64
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		out = append(out, id)
	}
	return out, rows.Err()
}
