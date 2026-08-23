package memory

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"sentinel-gateway/internal/jobs"
	"sentinel-gateway/internal/policy"
	"sentinel-gateway/internal/repository"
)

// Store provides persistence and query APIs for authoritative operational memory.
type Store struct {
	db       *sql.DB
	gate     *EligibilityGate
	resolver *SourceResolver
}

// NewStore instantiates a new operational memory Store.
func NewStore(db *sql.DB) *Store {
	return &Store{
		db:       db,
		gate:     NewEligibilityGate(db),
		resolver: NewSourceResolver(db),
	}
}

// ResolveMemorySources executes authoritative Go-owned source resolution and evidence minting.
func (s *Store) ResolveMemorySources(ctx context.Context, scope repository.Scope, req *ResolveMemorySourcesRequest) (*ResolvedMemorySources, error) {
	return s.resolver.ResolveMemorySources(ctx, scope, req)
}

// SetFreshnessPolicy configures a custom Go memory freshness policy on the resolver.
func (s *Store) SetFreshnessPolicy(factType FactType, freshnessPolicy MemoryFreshnessPolicy) {
	s.resolver.SetPolicy(factType, freshnessPolicy)
}

// PersistOperationalFact persists an operational memory record inside a new transaction.
func (s *Store) PersistOperationalFact(ctx context.Context, scope repository.Scope, record *OperationalMemoryRecord) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin tx: %w", err)
	}
	defer tx.Rollback()

	if err := s.PersistOperationalFactTx(ctx, tx, scope, record); err != nil {
		return err
	}

	return tx.Commit()
}

// PersistOperationalFactTx atomically stores a verified M1 fact, provenance sources, and initial revision.
func (s *Store) PersistOperationalFactTx(ctx context.Context, tx *sql.Tx, scope repository.Scope, record *OperationalMemoryRecord) error {
	if err := s.gate.EvaluateWithQueryable(ctx, tx, scope, record); err != nil {
		return fmt.Errorf("eligibility gate rejected record: %w", err)
	}

	// 1. Insert Master Memory Record
	queryMem := `
		INSERT INTO operational_memories (
			id, tenant_id, memory_type, subject_type, subject_ref, fact_type,
			structured_value, confidence_source, classification, status,
			valid_from, expires_at, superseded_by, created_at, created_by, memory_hash
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`

	structJSON, err := policy.CanonicalJSON(record.StructuredValue)
	if err != nil {
		return fmt.Errorf("canonicalize structured value: %w", err)
	}

	_, err = tx.ExecContext(ctx, queryMem,
		record.MemoryID, record.TenantID, string(record.MemoryType), string(record.SubjectType),
		record.SubjectRef, string(record.FactType), string(structJSON), string(record.ConfidenceSource),
		string(record.Classification), string(record.Status), record.ValidFrom, record.ExpiresAt,
		record.SupersededBy, record.CreatedAt, record.CreatedBy, record.MemoryHash,
	)
	if err != nil {
		return fmt.Errorf("insert operational_memories: %w", err)
	}

	// 2. Insert Provenance Sources
	querySource := `
		INSERT INTO memory_sources (
			memory_id, tenant_id, source_ref, source_hash, source_verification_ref, created_at
		) VALUES (?, ?, ?, ?, ?, ?)`

	for i, srcRef := range record.SourceRefs {
		var srcHash string
		if i < len(record.SourceHashes) {
			srcHash = record.SourceHashes[i]
		}
		var verifRef *string
		if i < len(record.SourceVerificationRefs) {
			verifRef = &record.SourceVerificationRefs[i]
		}

		_, err = tx.ExecContext(ctx, querySource,
			record.MemoryID, record.TenantID, srcRef, srcHash, verifRef, record.CreatedAt,
		)
		if err != nil {
			return fmt.Errorf("insert memory_source: %w", err)
		}
	}

	// 3. Insert Initial Revision Record
	queryRev := `
		INSERT INTO memory_revisions (
			id, memory_id, tenant_id, revision_number, previous_hash,
			new_hash, transition_type, reason, actor_id, created_at
		) VALUES (?, ?, ?, ?, NULL, ?, 'CREATED', 'Initial verified fact ingest', ?, ?)`

	revID := fmt.Sprintf("rev-%s-1", record.MemoryID)
	_, err = tx.ExecContext(ctx, queryRev,
		revID, record.MemoryID, record.TenantID, 1,
		record.MemoryHash, record.CreatedBy, record.CreatedAt,
	)
	if err != nil {
		return fmt.Errorf("insert memory_revisions: %w", err)
	}

	return nil
}

// GetMemory retrieves an operational memory record by ID within tenant scope.
func (s *Store) GetMemory(ctx context.Context, scope repository.Scope, memoryID string) (*OperationalMemoryRecord, error) {
	tenantID := scope.TenantID()

	query := `
		SELECT id, tenant_id, memory_type, subject_type, subject_ref, fact_type,
		       structured_value, confidence_source, classification, status,
		       valid_from, expires_at, superseded_by, created_at, created_by, memory_hash
		FROM operational_memories
		WHERE id = ?`

	var args []interface{}
	args = append(args, memoryID)
	if tenantID != "" {
		query += ` AND tenant_id = ?`
		args = append(args, tenantID)
	}

	rec := &OperationalMemoryRecord{}
	var structStr string
	var expiresAt sql.NullTime
	var supersededBy sql.NullString

	err := s.db.QueryRowContext(ctx, query, args...).Scan(
		&rec.MemoryID, &rec.TenantID, &rec.MemoryType, &rec.SubjectType, &rec.SubjectRef, &rec.FactType,
		&structStr, &rec.ConfidenceSource, &rec.Classification, &rec.Status,
		&rec.ValidFrom, &expiresAt, &supersededBy, &rec.CreatedAt, &rec.CreatedBy, &rec.MemoryHash,
	)
	if err == sql.ErrNoRows {
		return nil, ErrMemoryNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("query operational_memories: %w", err)
	}

	rec.StructuredValue = json.RawMessage(structStr)
	if expiresAt.Valid {
		rec.ExpiresAt = &expiresAt.Time
	}
	if supersededBy.Valid {
		rec.SupersededBy = &supersededBy.String
	}

	// Load sources
	srcQuery := `SELECT source_ref, source_hash, source_verification_ref FROM memory_sources WHERE memory_id = ? AND tenant_id = ? ORDER BY id ASC`
	rows, err := s.db.QueryContext(ctx, srcQuery, rec.MemoryID, rec.TenantID)
	if err == nil {
		defer rows.Close()
		for rows.Next() {
			var ref, hash string
			var verRef sql.NullString
			if err := rows.Scan(&ref, &hash, &verRef); err == nil {
				rec.SourceRefs = append(rec.SourceRefs, ref)
				rec.SourceHashes = append(rec.SourceHashes, hash)
				if verRef.Valid {
					rec.SourceVerificationRefs = append(rec.SourceVerificationRefs, verRef.String)
				}
			}
		}
	}

	return rec, nil
}

// ListMemoriesForSubject returns all operational memory records for a given subject.
func (s *Store) ListMemoriesForSubject(ctx context.Context, scope repository.Scope, subjectType SubjectType, subjectRef string) ([]*OperationalMemoryRecord, error) {
	tenantID := scope.TenantID()

	query := `
		SELECT id, tenant_id, memory_type, subject_type, subject_ref, fact_type,
		       structured_value, confidence_source, classification, status,
		       valid_from, expires_at, superseded_by, created_at, created_by, memory_hash
		FROM operational_memories
		WHERE subject_type = ? AND subject_ref = ?`

	var args []interface{}
	args = append(args, string(subjectType), subjectRef)
	if tenantID != "" {
		query += ` AND tenant_id = ?`
		args = append(args, tenantID)
	}
	query += ` ORDER BY created_at DESC`

	rows, err := s.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("list operational_memories: %w", err)
	}
	defer rows.Close()

	var records []*OperationalMemoryRecord
	for rows.Next() {
		rec := &OperationalMemoryRecord{}
		var structStr string
		var expiresAt sql.NullTime
		var supersededBy sql.NullString

		if err := rows.Scan(
			&rec.MemoryID, &rec.TenantID, &rec.MemoryType, &rec.SubjectType, &rec.SubjectRef, &rec.FactType,
			&structStr, &rec.ConfidenceSource, &rec.Classification, &rec.Status,
			&rec.ValidFrom, &expiresAt, &supersededBy, &rec.CreatedAt, &rec.CreatedBy, &rec.MemoryHash,
		); err != nil {
			return nil, fmt.Errorf("scan operational memory: %w", err)
		}

		rec.StructuredValue = json.RawMessage(structStr)
		if expiresAt.Valid {
			rec.ExpiresAt = &expiresAt.Time
		}
		if supersededBy.Valid {
			rec.SupersededBy = &supersededBy.String
		}
		records = append(records, rec)
	}

	return records, nil
}

// InvalidateMemory marks an operational memory record as INVALIDATED and records a revision.
func (s *Store) InvalidateMemory(ctx context.Context, scope repository.Scope, memoryID, actorID, reason string) error {
	rec, err := s.GetMemory(ctx, scope, memoryID)
	if err != nil {
		return err
	}

	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()

	_, err = tx.ExecContext(ctx, `UPDATE operational_memories SET status = 'INVALIDATED' WHERE id = ? AND tenant_id = ?`, memoryID, rec.TenantID)
	if err != nil {
		return fmt.Errorf("update status to INVALIDATED: %w", err)
	}

	revID := fmt.Sprintf("rev-%s-%d", memoryID, time.Now().UnixNano())
	now := time.Now().UTC()
	_, err = tx.ExecContext(ctx, `
		INSERT INTO memory_revisions (id, memory_id, tenant_id, revision_number, previous_hash, new_hash, transition_type, reason, actor_id, created_at)
		VALUES (?, ?, ?, (SELECT COUNT(*) + 1 FROM memory_revisions WHERE memory_id = ?), ?, ?, 'INVALIDATED', ?, ?, ?)`,
		revID, memoryID, rec.TenantID, memoryID, rec.MemoryHash, rec.MemoryHash, reason, actorID, now,
	)
	if err != nil {
		return fmt.Errorf("insert invalidation revision: %w", err)
	}

	return tx.Commit()
}

// SupersedeMemory marks an existing memory record as SUPERSEDED by a new record and records revisions.
func (s *Store) SupersedeMemory(ctx context.Context, scope repository.Scope, oldMemoryID string, newRecord *OperationalMemoryRecord, actorID, reason string) error {
	oldRec, err := s.GetMemory(ctx, scope, oldMemoryID)
	if err != nil {
		return err
	}

	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()

	// Persist the new record
	if err := s.PersistOperationalFactTx(ctx, tx, scope, newRecord); err != nil {
		return fmt.Errorf("persist superseding record: %w", err)
	}

	// Update old record
	_, err = tx.ExecContext(ctx, `UPDATE operational_memories SET status = 'SUPERSEDED', superseded_by = ? WHERE id = ? AND tenant_id = ?`,
		newRecord.MemoryID, oldMemoryID, oldRec.TenantID,
	)
	if err != nil {
		return fmt.Errorf("update status to SUPERSEDED: %w", err)
	}

	// Record revision on old record
	revID := fmt.Sprintf("rev-%s-%d", oldMemoryID, time.Now().UnixNano())
	now := time.Now().UTC()
	_, err = tx.ExecContext(ctx, `
		INSERT INTO memory_revisions (id, memory_id, tenant_id, revision_number, previous_hash, new_hash, transition_type, reason, actor_id, created_at)
		VALUES (?, ?, ?, (SELECT COUNT(*) + 1 FROM memory_revisions WHERE memory_id = ?), ?, ?, 'SUPERSEDED', ?, ?, ?)`,
		revID, oldMemoryID, oldRec.TenantID, oldMemoryID, oldRec.MemoryHash, newRecord.MemoryHash, reason, actorID, now,
	)
	if err != nil {
		return fmt.Errorf("insert supersession revision: %w", err)
	}

	return tx.Commit()
}

// IngestionBridgeDeliverer processes MEMORY_EVENT_ELIGIBLE outbox events into M1 operational facts.
type IngestionBridgeDeliverer struct {
	store *Store
}

// NewIngestionBridgeDeliverer creates an IngestionBridgeDeliverer.
func NewIngestionBridgeDeliverer(store *Store) *IngestionBridgeDeliverer {
	return &IngestionBridgeDeliverer{store: store}
}

// Deliver handles the asynchronous ingest of verified outbox events into M1 with idempotency conflict checks.
func (b *IngestionBridgeDeliverer) Deliver(ctx context.Context, ev jobs.PendingEvent) error {
	if ev.EventType != "MEMORY_EVENT_ELIGIBLE" {
		return nil
	}

	var payload struct {
		TenantID               string           `json:"tenant_id"`
		WorkflowID             string           `json:"workflow_id"`
		IncidentID             int64            `json:"incident_id"`
		SubjectType            SubjectType      `json:"subject_type"`
		SubjectRef             string           `json:"subject_ref"`
		FactType               FactType         `json:"fact_type"`
		ConfidenceSource       ConfidenceSource `json:"confidence_source"`
		SourceRefs             []string         `json:"source_refs"`
		SourceHashes           []string         `json:"source_hashes"`
		SourceVerificationRefs []string         `json:"source_verification_refs"`
		StructuredValue        json.RawMessage  `json:"structured_value"`
		CreatedBy              string           `json:"created_by"`
	}

	if err := ev.Payload.Decode(&payload); err != nil {
		return fmt.Errorf("decode memory outbox payload: %w", err)
	}

	dedupeShort := ev.DedupeKey
	if len(dedupeShort) > 8 {
		dedupeShort = dedupeShort[:8]
	}
	memoryID := fmt.Sprintf("mem-%s-%s", strings.ToLower(string(payload.SubjectType)), dedupeShort)

	memRecord := &OperationalMemoryRecord{
		MemoryID:               memoryID,
		TenantID:               payload.TenantID,
		MemoryType:             MemoryTypeOperationalFact,
		SubjectType:            payload.SubjectType,
		SubjectRef:             payload.SubjectRef,
		FactType:               payload.FactType,
		StructuredValue:        payload.StructuredValue,
		SourceRefs:             payload.SourceRefs,
		SourceHashes:           payload.SourceHashes,
		SourceVerificationRefs: payload.SourceVerificationRefs,
		ConfidenceSource:       payload.ConfidenceSource,
		Classification:         ClassificationInternal,
		Status:                 StatusActive,
		ValidFrom:              time.Unix(0, 0).UTC(),
		CreatedAt:              time.Unix(0, 0).UTC(),
		CreatedBy:              payload.CreatedBy,
	}

	expectedHash, err := ComputeMemoryHash(memRecord)
	if err != nil {
		return fmt.Errorf("compute event memory hash: %w", err)
	}
	memRecord.MemoryHash = expectedHash

	scope := repository.Scope{}

	// Idempotency check: look up existing record
	existing, err := b.store.GetMemory(ctx, scope, memoryID)
	if err == nil {
		// Existing record found: verify hash
		if existing.MemoryHash == expectedHash {
			// Idempotent replay: already persisted identically
			return nil
		}
		// Hash conflict: same event ID with different payload hash
		return fmt.Errorf("%w: memory %s exists with hash %s, event payload produced %s", ErrIdempotencyConflict, memoryID, existing.MemoryHash, expectedHash)
	}

	return b.store.PersistOperationalFact(ctx, scope, memRecord)
}

// ExportToEnvelope validates and exports an authoritative M1 record to an M2/M3 MemoryEventEnvelope.
func ExportToEnvelope(record *OperationalMemoryRecord, exportPolicy ManagedMemoryExportPolicy) (map[string]interface{}, error) {
	if record == nil {
		return nil, ErrNilRecord
	}

	// 1. Tenant match
	if exportPolicy.TenantID != "" && record.TenantID != exportPolicy.TenantID {
		return nil, fmt.Errorf("export error: tenant %s does not match policy %s", record.TenantID, exportPolicy.TenantID)
	}

	// 2. Fact type check
	if len(exportPolicy.AllowedFactTypes) > 0 {
		allowed := false
		for _, ft := range exportPolicy.AllowedFactTypes {
			if record.FactType == ft {
				allowed = true
				break
			}
		}
		if !allowed {
			return nil, fmt.Errorf("export error: fact_type %s is not permitted by export policy", record.FactType)
		}
	}

	// 3. Classification ceiling check
	if record.Classification == ClassificationRestricted {
		return nil, fmt.Errorf("%w: RESTRICTED memory cannot be exported to managed memory bank", ErrClassificationForbidden)
	}

	// 4. Compute export digest
	exportPayload := map[string]interface{}{
		"event_id":          fmt.Sprintf("evt-exp-%s", record.MemoryID),
		"tenant_scope":      record.TenantID,
		"memory_type":       string(record.MemoryType),
		"fact_type":         string(record.FactType),
		"subject_ref":       record.SubjectRef,
		"sanitized_fact":    string(record.StructuredValue),
		"source_refs":       record.SourceRefs,
		"provenance_hashes": record.SourceHashes,
		"occurred_at":       record.CreatedAt.Format(time.RFC3339),
	}

	b, err := json.Marshal(exportPayload)
	if err != nil {
		return nil, fmt.Errorf("marshal export envelope: %w", err)
	}
	h := sha256.Sum256(b)
	exportPayload["provenance_digest"] = hex.EncodeToString(h[:])

	return exportPayload, nil
}
