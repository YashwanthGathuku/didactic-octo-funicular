package ledger

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"math/rand/v2"
	"strings"
	"time"
)

// dialect selects the per-tenant serialisation mechanism.
type dialect int

const (
	dialectPostgres dialect = iota
	dialectSQLite
)

// Ledger appends to and verifies the evidence chain.
type Ledger struct {
	db      *sql.DB
	dialect dialect
	now     func() time.Time
}

// New builds a Ledger.
//
// The driver is named explicitly for the same reason it is in internal/jobs:
// the serialisation mechanism differs per database, and guessing produces
// something that appears to work and forks under concurrency.
func New(db *sql.DB, driverName string) (*Ledger, error) {
	if db == nil {
		return nil, errors.New("a ledger requires a database handle")
	}
	var d dialect
	switch {
	case strings.Contains(driverName, "pgx"), strings.Contains(driverName, "postgres"):
		d = dialectPostgres
	case strings.Contains(driverName, "sqlite"):
		d = dialectSQLite
	default:
		return nil, fmt.Errorf("unsupported driver %q for the evidence ledger", driverName)
	}
	return &Ledger{db: db, dialect: d, now: time.Now}, nil
}

// SetClock replaces the time source, for tests.
func (l *Ledger) SetClock(fn func() time.Time) { l.now = fn }

func (l *Ledger) rebind(query string) string {
	if l.dialect != dialectPostgres {
		return query
	}
	var b strings.Builder
	n := 0
	for _, r := range query {
		if r == '?' {
			n++
			fmt.Fprintf(&b, "$%d", n)
			continue
		}
		b.WriteRune(r)
	}
	return b.String()
}

// maxAppendRetries bounds contention retries.
//
// Appends to one tenant's chain are serialised by construction: each reads the
// current head, so only one can win at a time and the losers retry against the
// new head. The budget has to cover the whole queue draining, not one collision.
//
// It is sized from the observed behaviour rather than guessed. On SQLite a
// deferred transaction that reads and then writes must upgrade its lock, and
// SQLite refuses to wait on an upgrade -- it returns SQLITE_BUSY immediately,
// so busy_timeout does not apply and every collision costs a retry rather than
// a wait. With 24 concurrent writers a loser can lose many times in a row.
//
// The alternative, beginning every transaction in IMMEDIATE mode, fixes this
// and breaks the worker pool; see sqliteConnectionSettings in the gateway
// package for why it was rejected.
const maxAppendRetries = 40

// maxAppendBackoff caps the pause between contention retries.
const maxAppendBackoff = 40 * time.Millisecond

// Append writes one record, serialised within its tenant's stream.
//
// The whole operation is one transaction: read the head, compute, insert. The
// implementation this replaces read the last hash on one connection, computed
// outside any transaction, and inserted on another -- so two concurrent appends
// could both read the same predecessor. The unique constraints stopped that
// producing a fork, but the loser received a constraint error rather than
// retrying, which meant a dropped audit record under load.
func (l *Ledger) Append(ctx context.Context, req AppendRequest) (*Record, error) {
	if err := req.validate(); err != nil {
		return nil, err
	}

	canonical, err := canonicalPayload(req.Payload)
	if err != nil {
		return nil, err
	}
	if err := checkPayload(req.Payload, canonical); err != nil {
		return nil, err
	}

	occurred := normaliseTime(req.OccurredAt, l.now)

	var lastErr error
	for attempt := 0; attempt < maxAppendRetries; attempt++ {
		record, err := l.appendOnce(ctx, req, canonical, occurred)
		if err == nil {
			return record, nil
		}
		if !isContention(err) {
			return nil, err
		}
		lastErr = err

		// A brief, growing pause. Retrying instantly means every loser retries
		// at the same moment and loses again; the point is to spread them.
		//
		// This is contention, not failure: SQLite requires a writer to hold the
		// write lock for the whole read-compute-write, so concurrent appends
		// queue. See sqliteDSN in the gateway package for why the transaction
		// must begin in IMMEDIATE mode for that queueing to work at all.
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-time.After(appendBackoff(attempt)):
		}
	}
	return nil, fmt.Errorf("audit append lost %d times to concurrent writers: %w", maxAppendRetries, lastErr)
}

// AppendTx writes a record inside a caller's transaction.
//
// Use it when the record must commit with the state it describes. The caller
// owns the transaction, so it also owns retry: a constraint failure here rolls
// back the caller's work, which is correct -- an audit record that could not be
// written must not leave the business state it describes committed.
func (l *Ledger) AppendTx(ctx context.Context, tx *sql.Tx, req AppendRequest) (*Record, error) {
	if err := req.validate(); err != nil {
		return nil, err
	}
	canonical, err := canonicalPayload(req.Payload)
	if err != nil {
		return nil, err
	}
	if err := checkPayload(req.Payload, canonical); err != nil {
		return nil, err
	}
	return l.insert(ctx, tx, req, canonical, normaliseTime(req.OccurredAt, l.now))
}

// normaliseTime puts a timestamp in UTC at the precision every supported
// database can store, so the value hashed is the value that comes back.
// appendBackoff grows the pause and jitters it.
//
// Without jitter every loser retries at the same instant and loses again to the
// same winner; the pause exists to spread them, not merely to slow them down.
func appendBackoff(attempt int) time.Duration {
	base := time.Duration(attempt+1) * 2 * time.Millisecond
	if base > maxAppendBackoff {
		base = maxAppendBackoff
	}
	return time.Duration(rand.Int64N(int64(base)) + int64(time.Millisecond))
}

func normaliseTime(t time.Time, now func() time.Time) time.Time {
	if t.IsZero() {
		t = now()
	}
	return t.UTC().Truncate(TimestampPrecision)
}

func (l *Ledger) appendOnce(ctx context.Context, req AppendRequest, canonical string, occurred time.Time) (*Record, error) {
	tx, err := l.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, err
	}
	defer func() { _ = tx.Rollback() }()

	record, err := l.insert(ctx, tx, req, canonical, occurred)
	if err != nil {
		return nil, err
	}
	if err := tx.Commit(); err != nil {
		return nil, err
	}
	return record, nil
}

// insert does the serialised read-compute-write.
func (l *Ledger) insert(ctx context.Context, tx *sql.Tx, req AppendRequest, canonical string, occurred time.Time) (*Record, error) {
	// Serialise within the tenant's stream.
	//
	// PostgreSQL locks the tenant row: two appends for one tenant queue behind
	// each other while different tenants proceed in parallel, which is exactly
	// the granularity the chain needs -- one linear sequence per tenant, not one
	// globally.
	//
	// SQLite has no row lock and needs none: it serialises writers, and the
	// unique constraints below are the backstop either way.
	if l.dialect == dialectPostgres {
		if _, err := tx.ExecContext(ctx, l.rebind(
			`SELECT 1 FROM tenants WHERE id = ? FOR UPDATE`), req.TenantID); err != nil {
			return nil, err
		}
	}

	var prevHash string
	var prevSeq int64
	err := tx.QueryRowContext(ctx, l.rebind(`
		SELECT sequence_no, current_hash FROM audit_events
		WHERE tenant_id = ? ORDER BY sequence_no DESC LIMIT 1`),
		req.TenantID).Scan(&prevSeq, &prevHash)
	if errors.Is(err, sql.ErrNoRows) {
		prevSeq = 0
		prevHash = GenesisHash
	} else if err != nil {
		return nil, err
	}

	record := &Record{
		TenantID:         req.TenantID,
		SequenceNo:       prevSeq + 1,
		Action:           req.Action,
		Actor:            req.Actor,
		ObjectType:       req.ObjectType,
		ObjectID:         req.ObjectID,
		CorrelationID:    req.CorrelationID,
		OccurredAt:       occurred,
		Payload:          canonical,
		PayloadHash:      payloadHash(canonical),
		PreviousHash:     prevHash,
		CanonicalVersion: CanonicalVersion,
	}
	record.CurrentHash = recordHash(record)

	const insertSQL = `
		INSERT INTO audit_events
			(tenant_id, sequence_no, event_type, actor, object_type, object_id,
			 correlation_id, payload, payload_hash, previous_hash, current_hash,
			 canonical_version, created_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`

	if l.dialect == dialectPostgres {
		if err := tx.QueryRowContext(ctx, l.rebind(insertSQL+" RETURNING id"),
			record.TenantID, record.SequenceNo, record.Action, record.Actor,
			record.ObjectType, record.ObjectID, record.CorrelationID,
			record.Payload, record.PayloadHash, record.PreviousHash,
			record.CurrentHash, record.CanonicalVersion, record.OccurredAt).Scan(&record.ID); err != nil {
			return nil, err
		}
		return record, nil
	}

	res, err := tx.ExecContext(ctx, insertSQL,
		record.TenantID, record.SequenceNo, record.Action, record.Actor,
		record.ObjectType, record.ObjectID, record.CorrelationID,
		record.Payload, record.PayloadHash, record.PreviousHash,
		record.CurrentHash, record.CanonicalVersion, record.OccurredAt.Format(time.RFC3339Nano))
	if err != nil {
		return nil, err
	}
	record.ID, _ = res.LastInsertId()
	return record, nil
}

// isContention reports whether an error is a lost race rather than a defect.
//
// It matches on the constraint the chain relies on. A broader match would
// retry genuine errors, and retrying a real failure ten times before reporting
// it is worse than reporting it once.
func isContention(err error) bool {
	if err == nil {
		return false
	}
	msg := strings.ToLower(err.Error())
	return strings.Contains(msg, "unique") ||
		strings.Contains(msg, "duplicate key") ||
		strings.Contains(msg, "constraint failed") ||
		strings.Contains(msg, "database is locked") ||
		strings.Contains(msg, "sqlite_busy")
}

// VerificationResult describes the state of one tenant's chain.
//
// It reports what was checked as well as what was found. "Verified" over zero
// records is a different statement from "verified" over ten thousand, and a
// result that does not say which is not usable as evidence.
type VerificationResult struct {
	TenantID       string    `json:"tenantId"`
	RecordsChecked int64     `json:"recordsChecked"`
	Intact         bool      `json:"intact"`
	FirstBreakAt   int64     `json:"firstBreakAt,omitempty"`
	Reason         string    `json:"reason,omitempty"`
	CheckedAt      time.Time `json:"checkedAt"`

	// HeadHash is the chain's current tip. Recording it lets a later
	// verification confirm the chain was only appended to, never rewritten
	// beneath -- which a per-record check alone cannot see.
	HeadHash string `json:"headHash,omitempty"`
}

// Verify walks a tenant's chain and reports whether it is intact.
//
// It detects mutation, deletion and reordering, which are three names for one
// underlying check: every record's stored hash must equal the hash recomputed
// from its own fields, and its previous_hash must equal its predecessor's
// current_hash, with sequence numbers dense and ascending.
//
// A deleted record breaks the sequence and the link. A reordered pair breaks
// both. A mutated field changes the recomputed hash. None of the three can be
// repaired without recomputing every subsequent digest, which is the property
// the chain provides -- and, equally, the reason external anchoring matters:
// a party who can rewrite the rows can also recompute the digests.
func (l *Ledger) Verify(ctx context.Context, tenantID string) (*VerificationResult, error) {
	result := &VerificationResult{
		TenantID:  tenantID,
		Intact:    true,
		CheckedAt: l.now().UTC(),
	}

	rows, err := l.db.QueryContext(ctx, l.rebind(`
		SELECT id, tenant_id, sequence_no, event_type, actor, object_type, object_id,
		       correlation_id, payload, payload_hash, previous_hash, current_hash,
		       canonical_version, created_at
		FROM audit_events WHERE tenant_id = ? ORDER BY sequence_no ASC`), tenantID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	expectedPrev := GenesisHash
	var expectedSeq int64 = 1

	for rows.Next() {
		var r Record
		var objectType sql.NullString
		var objectID sql.NullInt64
		var correlation sql.NullString
		var createdAt any

		if err := rows.Scan(&r.ID, &r.TenantID, &r.SequenceNo, &r.Action, &r.Actor,
			&objectType, &objectID, &correlation, &r.Payload, &r.PayloadHash,
			&r.PreviousHash, &r.CurrentHash, &r.CanonicalVersion, &createdAt); err != nil {
			return nil, err
		}
		r.ObjectType = objectType.String
		r.ObjectID = objectID.Int64
		r.CorrelationID = correlation.String

		occurred, err := parseTime(createdAt)
		if err != nil {
			return breakAt(result, r.SequenceNo, fmt.Sprintf("unparseable timestamp: %v", err)), nil
		}
		r.OccurredAt = occurred

		// Records written under a different canonical version cannot be
		// verified by these rules. Saying so is the honest outcome; silently
		// skipping them would let an attacker mark a record as another version
		// to exempt it from checking.
		if r.CanonicalVersion != CanonicalVersion {
			return breakAt(result, r.SequenceNo,
				fmt.Sprintf("record uses canonical version %q; this verifier implements %q",
					r.CanonicalVersion, CanonicalVersion)), nil
		}

		// Deletion or reordering shows up here: the sequence must be dense and
		// ascending from 1.
		if r.SequenceNo != expectedSeq {
			return breakAt(result, r.SequenceNo,
				fmt.Sprintf("sequence gap or reorder: expected %d, found %d", expectedSeq, r.SequenceNo)), nil
		}
		// A broken predecessor link.
		if r.PreviousHash != expectedPrev {
			return breakAt(result, r.SequenceNo, "previous hash does not match the preceding record"), nil
		}
		// Payload mutation.
		if payloadHash(r.Payload) != r.PayloadHash {
			return breakAt(result, r.SequenceNo, "payload does not match its recorded hash"), nil
		}
		// Any other field mutation.
		if recomputed := recordHash(&r); recomputed != r.CurrentHash {
			return breakAt(result, r.SequenceNo, "record hash does not match its contents"), nil
		}

		expectedPrev = r.CurrentHash
		expectedSeq++
		result.RecordsChecked++
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}

	if result.RecordsChecked > 0 {
		result.HeadHash = expectedPrev
	}
	return result, nil
}

func breakAt(result *VerificationResult, seq int64, reason string) *VerificationResult {
	result.Intact = false
	result.FirstBreakAt = seq
	result.Reason = reason
	return result
}

// parseTime accepts what either driver returns for a timestamp column.
func parseTime(v any) (time.Time, error) {
	switch t := v.(type) {
	case time.Time:
		return t.UTC(), nil
	case string:
		for _, layout := range []string{time.RFC3339Nano, time.RFC3339, "2006-01-02 15:04:05.999999999-07:00", "2006-01-02 15:04:05"} {
			if parsed, err := time.Parse(layout, t); err == nil {
				return parsed.UTC(), nil
			}
		}
		return time.Time{}, fmt.Errorf("unrecognised timestamp %q", t)
	case []byte:
		return parseTime(string(t))
	default:
		return time.Time{}, fmt.Errorf("unrecognised timestamp type %T", v)
	}
}

// Head returns a tenant's current chain tip, or the genesis hash.
func (l *Ledger) Head(ctx context.Context, tenantID string) (int64, string, error) {
	var seq int64
	var hash string
	err := l.db.QueryRowContext(ctx, l.rebind(`
		SELECT sequence_no, current_hash FROM audit_events
		WHERE tenant_id = ? ORDER BY sequence_no DESC LIMIT 1`), tenantID).Scan(&seq, &hash)
	if errors.Is(err, sql.ErrNoRows) {
		return 0, GenesisHash, nil
	}
	return seq, hash, err
}

// Tenants lists every tenant with at least one record, so a verification pass
// can cover all of them without being told which exist.
func (l *Ledger) Tenants(ctx context.Context) ([]string, error) {
	rows, err := l.db.QueryContext(ctx, `SELECT DISTINCT tenant_id FROM audit_events ORDER BY tenant_id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []string
	for rows.Next() {
		var t string
		if err := rows.Scan(&t); err != nil {
			return nil, err
		}
		out = append(out, t)
	}
	return out, rows.Err()
}
