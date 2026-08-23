package memory

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"regexp"
	"strings"
	"time"

	"sentinel-gateway/internal/repository"
)

var (
	// Redaction and forbidden content detection patterns
	reRawNACHARecord = regexp.MustCompile(`^[156789][0-9A-Za-z\s]{80,100}$`)
	// secret-scan-allow: detection pattern for secrets in operational memory eligibility gate
	reSecretKeyPattern = regexp.MustCompile(`(?i)(bearer\s+[a-z0-9_\-\.]{20,}|(ghp|gho|xoxb|xoxp|sk_live|secret|token)_[a-z0-9_\-]{16,}|BEGIN\s+(RSA|OPENSSH|PGP|EC)\s+PRIVATE\s+KEY)`)
	reAccountNumber    = regexp.MustCompile(`\b\d{10,17}\b`)
	reRoutingNumber    = regexp.MustCompile(`\b[0123678]\d{7}\d\b`)
	reHex64            = regexp.MustCompile(`^[a-f0-9]{64}$`)
)

// Queryable represents a database interface capable of querying single rows.
type Queryable interface {
	QueryRowContext(ctx context.Context, query string, args ...any) *sql.Row
}

// EligibilityGate evaluates write admissibility of an operational memory candidate.
type EligibilityGate struct {
	db *sql.DB
}

// NewEligibilityGate instantiates an EligibilityGate.
func NewEligibilityGate(db *sql.DB) *EligibilityGate {
	return &EligibilityGate{db: db}
}

// Evaluate performs the complete validation pipeline for M1 persistence using default db.
func (g *EligibilityGate) Evaluate(ctx context.Context, scope repository.Scope, record *OperationalMemoryRecord) error {
	var q Queryable
	if g.db != nil {
		q = g.db
	}
	return g.EvaluateWithQueryable(ctx, q, scope, record)
}

// EvaluateWithQueryable performs validation using the provided Queryable (db or tx).
func (g *EligibilityGate) EvaluateWithQueryable(ctx context.Context, q Queryable, scope repository.Scope, record *OperationalMemoryRecord) error {
	if record == nil {
		return ErrNilRecord
	}

	// 1. Tenant Scope Verification
	if record.TenantID == "" {
		if scope.TenantID() != "" {
			record.TenantID = scope.TenantID()
		} else {
			return ErrMissingTenantID
		}
	} else if scope.TenantID() != "" && record.TenantID != scope.TenantID() {
		return repository.ErrNotFound
	}

	// 2. Memory Tier Invariant
	if record.MemoryType != MemoryTypeOperationalFact {
		return fmt.Errorf("%w: only M1_OPERATIONAL_FACT can be stored authoritatively, got %s", ErrInvalidMemoryType, record.MemoryType)
	}

	// 3. Subject and Fact Type Validation
	if record.SubjectRef == "" {
		return ErrMissingSubjectRef
	}
	if !isValidSubjectType(record.SubjectType) {
		return fmt.Errorf("%w: %s", ErrInvalidSubjectType, record.SubjectType)
	}
	if !isValidFactType(record.FactType) {
		return fmt.Errorf("%w: %s", ErrInvalidFactType, record.FactType)
	}

	// 4. Confidence Source Authorization
	switch record.ConfidenceSource {
	case ConfidenceSourceDeterministicDerived, ConfidenceSourceHumanConfirmed, ConfidenceSourceVerifiedWorkflow:
		// Authorized authoritative confidence sources
	case ConfidenceSourceManagedMemorySuggest:
		return fmt.Errorf("%w: advisory memory suggestions (M2/M3) cannot directly write M1 facts without independent verification", ErrInvalidConfidenceSource)
	default:
		return fmt.Errorf("%w: unknown confidence source %s", ErrInvalidConfidenceSource, record.ConfidenceSource)
	}

	// 5. Provenance Invariants
	if len(record.SourceRefs) == 0 {
		return errors.New("provenance error: source_refs must contain at least 1 reference")
	}
	if len(record.SourceHashes) == 0 {
		return errors.New("provenance error: source_hashes must contain at least 1 hash")
	}
	for _, h := range record.SourceHashes {
		if !reHex64.MatchString(strings.ToLower(h)) {
			return fmt.Errorf("provenance error: invalid source hash format %q (must be 64-char hex SHA-256)", h)
		}
	}
	if len(record.SourceVerificationRefs) == 0 {
		return errors.New("provenance error: source_verification_refs cannot be empty for M1 operational fact")
	}

	// 6. Verification Status Check in Database (if Queryable available)
	if q != nil {
		for _, verifRef := range record.SourceVerificationRefs {
			var outcome string
			err := q.QueryRowContext(ctx, `
				SELECT deterministic_outcome FROM candidate_verifications
				WHERE tenant_id = ? AND (id = ? OR workflow_id = ?)
				LIMIT 1`, record.TenantID, verifRef, verifRef).Scan(&outcome)
			if err == nil {
				if outcome != "PASS" {
					return fmt.Errorf("%w: associated candidate verification %s has outcome %q (requires PASS)", ErrIneligibleMemory, verifRef, outcome)
				}
			}
		}
	}

	// 7. Safety, Redaction & Disallowed Content Scanning (Reject rather than silently modify)
	if len(record.StructuredValue) == 0 {
		return ErrEmptyStructuredValue
	}
	if err := scanForDisallowedContent(record.StructuredValue); err != nil {
		return err
	}

	// 8. Timestamps and Validity Window
	if record.ValidFrom.IsZero() {
		record.ValidFrom = time.Now().UTC()
	}
	if record.ExpiresAt != nil && record.ExpiresAt.Before(record.ValidFrom) {
		return errors.New("validity error: expires_at cannot be prior to valid_from")
	}
	if record.CreatedAt.IsZero() {
		record.CreatedAt = time.Now().UTC()
	}

	// 9. Status and Classification Defaults
	if record.Status == "" {
		record.Status = StatusActive
	}
	if record.Classification == "" {
		record.Classification = ClassificationInternal
	}

	// 10. Cryptographic Canonical Hash Verification
	expectedHash, err := ComputeMemoryHash(record)
	if err != nil {
		return fmt.Errorf("compute canonical memory hash: %w", err)
	}
	if record.MemoryHash == "" {
		record.MemoryHash = expectedHash
	} else if record.MemoryHash != expectedHash {
		return fmt.Errorf("%w: provided %s, expected %s", ErrHashMismatch, record.MemoryHash, expectedHash)
	}

	return nil
}

func scanForDisallowedContent(raw json.RawMessage) error {
	rawStr := string(raw)

	if reSecretKeyPattern.MatchString(rawStr) {
		return ErrSecretDetected
	}

	// Ensure structured JSON is well-formed
	var val interface{}
	if err := json.Unmarshal(raw, &val); err != nil {
		return fmt.Errorf("structured_value must be a valid JSON object: %w", err)
	}

	// Recursively check all strings in the payload for raw NACHA records and PII
	if err := scanValueForSensitiveData(val); err != nil {
		return err
	}

	return nil
}

func scanValueForSensitiveData(val interface{}) error {
	switch v := val.(type) {
	case string:
		trimmed := strings.TrimSpace(v)
		// Check raw NACHA line
		if len(trimmed) >= 80 && len(trimmed) <= 100 && reRawNACHARecord.MatchString(trimmed) {
			return fmt.Errorf("%w: raw NACHA line detected in memory value", ErrPIIDetected)
		}
		// Check unmasked account numbers (10-17 digits)
		if reAccountNumber.MatchString(trimmed) {
			return fmt.Errorf("%w: unmasked account number detected in memory value", ErrPIIDetected)
		}
	case []interface{}:
		for _, item := range v {
			if err := scanValueForSensitiveData(item); err != nil {
				return err
			}
		}
	case map[string]interface{}:
		for key, item := range v {
			// Also inspect key names for forbidden sensitive fields
			keyLower := strings.ToLower(key)
			if keyLower == "raw_nacha" || keyLower == "account_number" || keyLower == "routing_number" {
				if strVal, ok := item.(string); ok && strVal != "" && !strings.Contains(strVal, "REDACTED") {
					return fmt.Errorf("%w: raw financial identifier field %q detected", ErrPIIDetected, key)
				}
			}
			if err := scanValueForSensitiveData(item); err != nil {
				return err
			}
		}
	}
	return nil
}

func isValidSubjectType(s SubjectType) bool {
	switch s {
	case SubjectTypeIncident, SubjectTypePartner, SubjectTypeRemediationPlan,
		SubjectTypeValidationRule, SubjectTypeTenantPolicy, SubjectTypeFileFormat, SubjectTypeArtifact:
		return true
	default:
		return false
	}
}

func isValidFactType(f FactType) bool {
	switch f {
	case FactTypeVerifiedRemediationSuccess, FactTypeVerifiedFailurePattern,
		FactTypePartnerFormatTolerance, FactTypeOperationalSLABreach,
		FactTypeCanonicalRuleAmendment, FactTypeHumanInvestigationOutcome,
		FactTypeDualControlReleaseOutcome:
		return true
	default:
		return false
	}
}
