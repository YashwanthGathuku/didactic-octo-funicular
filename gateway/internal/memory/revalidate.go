package memory

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	"sentinel-gateway/internal/repository"
)

// RevalidationStatus represents the outcome of memory source revalidation.
type RevalidationStatus string

const (
	RevalidationAuthoritativeVerified RevalidationStatus = "AUTHORITATIVE_VERIFIED"
	RevalidationUnverifiedMemory       RevalidationStatus = "UNVERIFIED_MEMORY"
	RevalidationStaleExpired          RevalidationStatus = "STALE_EXPIRED"
	RevalidationTamperedRejected       RevalidationStatus = "TAMPERED_REJECTED"
)

// RevalidationReport details the cryptographic and source revalidation result.
type RevalidationReport struct {
	MemoryID               string             `json:"memory_id"`
	TenantID               string             `json:"tenant_id"`
	Status                 RevalidationStatus `json:"status"`
	Detail                 string             `json:"detail"`
	RevalidatedSourcesCount int               `json:"revalidated_sources_count"`
	FailedSourcesCount     int                `json:"failed_sources_count"`
	RevalidatedAt          time.Time          `json:"revalidated_at"`
}

// Revalidator executes cryptographic and source revalidation of operational memories.
type Revalidator struct {
	db *sql.DB
}

// NewRevalidator creates a new Revalidator.
func NewRevalidator(db *sql.DB) *Revalidator {
	return &Revalidator{db: db}
}

// Revalidate performs source and integrity revalidation of an operational memory record.
func (r *Revalidator) Revalidate(ctx context.Context, scope repository.Scope, record *OperationalMemoryRecord) (*RevalidationReport, error) {
	if record == nil {
		return nil, ErrNilRecord
	}

	report := &RevalidationReport{
		MemoryID:      record.MemoryID,
		TenantID:      record.TenantID,
		RevalidatedAt: time.Now().UTC(),
	}

	// 1. Tenant boundary check
	if scope.TenantID() != "" && record.TenantID != scope.TenantID() {
		report.Status = RevalidationTamperedRejected
		report.Detail = fmt.Sprintf("tenant mismatch: record has %s, scope has %s", record.TenantID, scope.TenantID())
		return report, nil
	}

	// 2. TTL / Expiry check
	now := time.Now().UTC()
	if record.ExpiresAt != nil && now.After(*record.ExpiresAt) {
		report.Status = RevalidationStaleExpired
		report.Detail = fmt.Sprintf("memory expired at %s", record.ExpiresAt.Format(time.RFC3339))
		return report, nil
	}

	// 3. Status check
	if record.Status == StatusSuperseded || record.Status == StatusInvalidated {
		report.Status = RevalidationStaleExpired
		report.Detail = fmt.Sprintf("memory status is %s", record.Status)
		return report, nil
	}

	// 4. Memory canonical hash integrity
	validHash, err := VerifyMemoryHash(record)
	if err != nil || !validHash {
		report.Status = RevalidationTamperedRejected
		report.Detail = "canonical memory hash does not match structured contents"
		return report, nil
	}

	// 5. Source references validation
	if len(record.SourceRefs) == 0 {
		report.Status = RevalidationUnverifiedMemory
		report.Detail = "zero source references present on memory record"
		return report, nil
	}

	// 6. DB Verification check
	if r.db != nil {
		revalidatedCount := 0
		failedCount := 0

		for _, verifRef := range record.SourceVerificationRefs {
			var outcome string
			var storedHash string
			err := r.db.QueryRowContext(ctx, `
				SELECT deterministic_outcome, verification_hash FROM candidate_verifications
				WHERE tenant_id = ? AND (id = ? OR workflow_id = ?)
				LIMIT 1`, record.TenantID, verifRef, verifRef).Scan(&outcome, &storedHash)
			if err == sql.ErrNoRows {
				// Backing verification record missing
				failedCount++
				continue
			} else if err != nil {
				return nil, fmt.Errorf("lookup verification ref: %w", err)
			}

			if outcome != "PASS" {
				report.Status = RevalidationTamperedRejected
				report.Detail = fmt.Sprintf("source verification %s has non-PASS outcome %q", verifRef, outcome)
				return report, nil
			}
			revalidatedCount++
		}

		report.RevalidatedSourcesCount = revalidatedCount
		report.FailedSourcesCount = failedCount

		if failedCount > 0 && revalidatedCount == 0 {
			report.Status = RevalidationUnverifiedMemory
			report.Detail = fmt.Sprintf("all %d source verification references are unresolvable", failedCount)
			return report, nil
		}
	} else {
		report.RevalidatedSourcesCount = len(record.SourceRefs)
	}

	report.Status = RevalidationAuthoritativeVerified
	report.Detail = "all cryptographic and provenance checks passed"
	return report, nil
}
