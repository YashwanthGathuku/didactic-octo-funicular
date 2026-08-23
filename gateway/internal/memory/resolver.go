package memory

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strings"
	"time"

	"sentinel-gateway/internal/repository"
)

// SourceResolver executes authoritative source validation and evidence promotion in Go.
type SourceResolver struct {
	db       *sql.DB
	policies map[FactType]MemoryFreshnessPolicy
}

// NewSourceResolver instantiates a SourceResolver with default Go-owned freshness policies.
func NewSourceResolver(db *sql.DB) *SourceResolver {
	return &SourceResolver{
		db:       db,
		policies: DefaultMemoryFreshnessPolicies(),
	}
}

// SetPolicy overrides or adds a memory freshness policy for a specific fact type.
func (r *SourceResolver) SetPolicy(factType FactType, policy MemoryFreshnessPolicy) {
	r.policies[factType] = policy
}

// ResolveMemorySources executes the authoritative resolution of advisory source references.
// Only the Go Control Plane may mint and promote historical source references into AuthorizedEvidenceRefs.
func (r *SourceResolver) ResolveMemorySources(
	ctx context.Context,
	scope repository.Scope,
	req *ResolveMemorySourcesRequest,
) (*ResolvedMemorySources, error) {
	if req == nil {
		return nil, errors.New("resolve memory sources request is nil")
	}
	if req.MemoryRef == "" {
		return nil, errors.New("memory_ref is required")
	}

	// 1. Tenant boundary enforcement
	if req.TenantID == "" {
		if scope.TenantID() != "" {
			req.TenantID = scope.TenantID()
		} else {
			return nil, ErrMissingTenantID
		}
	} else if scope.TenantID() != "" && req.TenantID != scope.TenantID() {
		return nil, repository.ErrNotFound
	}

	res := &ResolvedMemorySources{
		TenantID:           req.TenantID,
		MemoryRef:          req.MemoryRef,
		ValidSourceRefs:    make([]string, 0),
		InvalidSourceRefs:  make([]string, 0),
		StaleSourceRefs:    make([]string, 0),
		EvidenceRefsMinted: make([]string, 0),
		ResolvedAt:         time.Now().UTC(),
	}

	now := res.ResolvedAt

	// 2. Iterate and authoritatively resolve each source reference
	for _, srcRef := range req.SourceRefs {
		trimmed := strings.TrimSpace(srcRef)
		if trimmed == "" {
			continue
		}

		// Look up operational memory records
		if strings.HasPrefix(trimmed, "MEM-") || strings.HasPrefix(trimmed, "OPMEM-") {
			memoryID := trimmed
			var status, classification, factTypeStr, memoryHash string
			var validFrom time.Time
			var expiresAt *time.Time
			var supersededBy *string

			err := r.db.QueryRowContext(ctx, `
				SELECT status, classification, fact_type, memory_hash, valid_from, expires_at, superseded_by
				FROM operational_memories
				WHERE tenant_id = ? AND id = ?
				LIMIT 1`, req.TenantID, memoryID).Scan(&status, &classification, &factTypeStr, &memoryHash, &validFrom, &expiresAt, &supersededBy)

			if err != nil {
				if errors.Is(err, sql.ErrNoRows) {
					res.InvalidSourceRefs = append(res.InvalidSourceRefs, trimmed)
				} else {
					return nil, fmt.Errorf("query operational memory %s: %w", memoryID, err)
				}
				continue
			}

			// Check status
			if status == string(StatusInvalidated) {
				res.InvalidSourceRefs = append(res.InvalidSourceRefs, trimmed)
				continue
			}
			if status == string(StatusSuperseded) || supersededBy != nil {
				res.StaleSourceRefs = append(res.StaleSourceRefs, trimmed)
				continue
			}

			// Check classification
			if classification == string(ClassificationRestricted) {
				res.InvalidSourceRefs = append(res.InvalidSourceRefs, trimmed)
				continue
			}

			// Check freshness policy
			ft := FactType(factTypeStr)
			policy, hasPolicy := r.policies[ft]
			if hasPolicy {
				maxAge := policy.DefaultTTL
				if expiresAt != nil {
					if now.After(*expiresAt) {
						res.StaleSourceRefs = append(res.StaleSourceRefs, trimmed)
						continue
					}
				} else if now.Sub(validFrom) > maxAge {
					res.StaleSourceRefs = append(res.StaleSourceRefs, trimmed)
					continue
				}
			}

			// Valid operational fact source -> promote to authorized evidence ref
			res.ValidSourceRefs = append(res.ValidSourceRefs, trimmed)
			evidenceRef := fmt.Sprintf("EVID-MEM-%s", memoryID)
			res.EvidenceRefsMinted = append(res.EvidenceRefsMinted, evidenceRef)
			continue
		}

		// Look up candidate verification sources
		if strings.HasPrefix(trimmed, "VER-") || strings.HasPrefix(trimmed, "VERIF-") {
			var outcome string
			err := r.db.QueryRowContext(ctx, `
				SELECT deterministic_outcome FROM candidate_verifications
				WHERE tenant_id = ? AND id = ?
				LIMIT 1`, req.TenantID, trimmed).Scan(&outcome)

			if err != nil {
				if errors.Is(err, sql.ErrNoRows) {
					res.InvalidSourceRefs = append(res.InvalidSourceRefs, trimmed)
				} else {
					return nil, fmt.Errorf("query verification %s: %w", trimmed, err)
				}
				continue
			}

			if outcome != "PASS" {
				res.InvalidSourceRefs = append(res.InvalidSourceRefs, trimmed)
				continue
			}

			res.ValidSourceRefs = append(res.ValidSourceRefs, trimmed)
			res.EvidenceRefsMinted = append(res.EvidenceRefsMinted, trimmed)
			continue
		}

		// Look up incident sources
		if strings.HasPrefix(trimmed, "INC-") || strings.HasPrefix(trimmed, "INCIDENT-") {
			var incID int64
			err := r.db.QueryRowContext(ctx, `
				SELECT id FROM incidents
				WHERE tenant_id = ? AND (id = ? OR CAST(id AS TEXT) = ?)
				LIMIT 1`, req.TenantID, trimmed, strings.TrimPrefix(trimmed, "INC-")).Scan(&incID)

			if err != nil {
				if errors.Is(err, sql.ErrNoRows) {
					res.InvalidSourceRefs = append(res.InvalidSourceRefs, trimmed)
				} else {
					return nil, fmt.Errorf("query incident %s: %w", trimmed, err)
				}
				continue
			}

			res.ValidSourceRefs = append(res.ValidSourceRefs, trimmed)
			res.EvidenceRefsMinted = append(res.EvidenceRefsMinted, trimmed)
			continue
		}

		// Look up finding or runbook sources (FINDING-, RB-, POL-)
		// Invariant: MemorySourceRelation != EvidenceAuthority.
		// A finding/runbook citation must be bound to an active, verified memory record or incident.
		if strings.HasPrefix(trimmed, "FINDING-") || strings.HasPrefix(trimmed, "RB-") || strings.HasPrefix(trimmed, "POL-") {
			var activeCount int
			err := r.db.QueryRowContext(ctx, `
				SELECT COUNT(*) 
				FROM memory_sources ms
				JOIN operational_memories om ON ms.memory_id = om.id AND ms.tenant_id = om.tenant_id
				WHERE ms.tenant_id = ? AND ms.source_ref = ? AND om.status = 'ACTIVE'
				LIMIT 1`, req.TenantID, trimmed).Scan(&activeCount)

			if err == nil && activeCount > 0 {
				res.ValidSourceRefs = append(res.ValidSourceRefs, trimmed)
				res.EvidenceRefsMinted = append(res.EvidenceRefsMinted, trimmed)
			} else {
				res.InvalidSourceRefs = append(res.InvalidSourceRefs, trimmed)
			}
			continue
		}

		// Any other unmatched source is invalid
		res.InvalidSourceRefs = append(res.InvalidSourceRefs, trimmed)
	}

	// 3. Sort for deterministic canonical hashing
	sort.Strings(res.ValidSourceRefs)
	sort.Strings(res.InvalidSourceRefs)
	sort.Strings(res.StaleSourceRefs)
	sort.Strings(res.EvidenceRefsMinted)

	// 4. Compute RFC 8785 canonical resolution hash
	resolutionPayload := map[string]interface{}{
		"tenant_id":            res.TenantID,
		"memory_ref":           res.MemoryRef,
		"valid_source_refs":    res.ValidSourceRefs,
		"invalid_source_refs":  res.InvalidSourceRefs,
		"stale_source_refs":    res.StaleSourceRefs,
		"evidence_refs_minted": res.EvidenceRefsMinted,
	}

	b, err := json.Marshal(resolutionPayload)
	if err != nil {
		return nil, fmt.Errorf("marshal resolution payload: %w", err)
	}

	h := sha256.Sum256(b)
	res.ResolutionHash = hex.EncodeToString(h[:])

	return res, nil
}
