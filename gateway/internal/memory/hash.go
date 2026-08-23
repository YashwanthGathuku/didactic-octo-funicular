package memory

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"sort"
	"time"

	"sentinel-gateway/internal/policy"
)

// ComputeMemoryHash generates an RFC 8785 compliant SHA-256 canonical hash of the operational memory record.
func ComputeMemoryHash(m *OperationalMemoryRecord) (string, error) {
	if m == nil {
		return "", ErrNilRecord
	}

	// 1. Sort provenance arrays for deterministic ordering
	sortedSourceRefs := make([]string, len(m.SourceRefs))
	copy(sortedSourceRefs, m.SourceRefs)
	sort.Strings(sortedSourceRefs)

	sortedSourceHashes := make([]string, len(m.SourceHashes))
	copy(sortedSourceHashes, m.SourceHashes)
	sort.Strings(sortedSourceHashes)

	sortedVerifRefs := make([]string, len(m.SourceVerificationRefs))
	copy(sortedVerifRefs, m.SourceVerificationRefs)
	sort.Strings(sortedVerifRefs)

	// 2. Parse structured_value payload for canonicalization
	var structValObj interface{}
	if len(m.StructuredValue) > 0 {
		if err := json.Unmarshal(m.StructuredValue, &structValObj); err != nil {
			return "", fmt.Errorf("unmarshal structured_value for canonicalization: %w", err)
		}
	} else {
		structValObj = map[string]interface{}{}
	}

	// 3. Format timestamps canonically (RFC 3339 UTC Nano)
	validFromStr := ""
	if !m.ValidFrom.IsZero() {
		validFromStr = m.ValidFrom.UTC().Format(time.RFC3339Nano)
	}

	createdAtStr := ""
	if !m.CreatedAt.IsZero() {
		createdAtStr = m.CreatedAt.UTC().Format(time.RFC3339Nano)
	}

	// 4. Construct canonical hashing payload
	payload := map[string]interface{}{
		"memory_id":                m.MemoryID,
		"tenant_id":                m.TenantID,
		"memory_type":              string(m.MemoryType),
		"subject_type":             string(m.SubjectType),
		"subject_ref":              m.SubjectRef,
		"fact_type":                string(m.FactType),
		"structured_value":         structValObj,
		"source_refs":              sortedSourceRefs,
		"source_hashes":            sortedSourceHashes,
		"source_verification_refs": sortedVerifRefs,
		"confidence_source":        string(m.ConfidenceSource),
		"classification":           string(m.Classification),
		"valid_from":               validFromStr,
		"created_at":               createdAtStr,
		"created_by":               m.CreatedBy,
	}

	canonicalBytes, err := policy.CanonicalJSON(payload)
	if err != nil {
		return "", fmt.Errorf("canonicalize operational memory payload: %w", err)
	}

	digest := sha256.Sum256(canonicalBytes)
	return hex.EncodeToString(digest[:]), nil
}

// VerifyMemoryHash validates whether the record's MemoryHash matches its canonical content.
func VerifyMemoryHash(m *OperationalMemoryRecord) (bool, error) {
	if m == nil {
		return false, ErrNilRecord
	}
	expected, err := ComputeMemoryHash(m)
	if err != nil {
		return false, err
	}
	return m.MemoryHash == expected, nil
}
