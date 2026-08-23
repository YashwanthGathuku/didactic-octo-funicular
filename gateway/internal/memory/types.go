package memory

import (
	"encoding/json"
	"errors"
	"time"
)

// Common domain errors for operational memory.
var (
	ErrNilRecord               = errors.New("memory record is nil")
	ErrMissingTenantID         = errors.New("tenant_id is required")
	ErrMissingMemoryID         = errors.New("memory_id is required")
	ErrMissingSubjectRef       = errors.New("subject_ref is required")
	ErrEmptyStructuredValue    = errors.New("structured_value cannot be empty")
	ErrInvalidMemoryType       = errors.New("invalid memory_type")
	ErrInvalidSubjectType      = errors.New("invalid subject_type")
	ErrInvalidFactType         = errors.New("invalid fact_type")
	ErrInvalidConfidenceSource = errors.New("invalid confidence_source")
	ErrInvalidClassification   = errors.New("invalid data classification")
	ErrInvalidStatus           = errors.New("invalid memory status")
	ErrHashMismatch            = errors.New("computed memory_hash does not match record memory_hash")
	ErrIneligibleMemory        = errors.New("memory record failed deterministic eligibility gate")
	ErrPIIDetected             = errors.New("unredacted PII or raw financial identifier detected in memory payload")
	ErrSecretDetected          = errors.New("secret or credential pattern detected in memory payload")
	ErrMemoryNotFound          = errors.New("operational memory record not found")
	ErrMemoryExpired           = errors.New("operational memory record has expired")
	ErrMemorySuperseded        = errors.New("operational memory record is superseded")
	ErrMemoryInvalidated       = errors.New("operational memory record is invalidated")
	ErrSourceTampered          = errors.New("memory source hash does not match underlying record")
	ErrSourceNotFound          = errors.New("memory source reference not found")
)

// MemoryType represents the 4-tier memory hierarchy.
type MemoryType string

const (
	MemoryTypeSession           MemoryType = "M0_SESSION"
	MemoryTypeOperationalFact   MemoryType = "M1_OPERATIONAL_FACT"
	MemoryTypeManagedSemantic   MemoryType = "M2_MANAGED_SEMANTIC"
	MemoryTypeStructuredProfile MemoryType = "M3_STRUCTURED_PROFILE"
)

// SubjectType defines the entity class that this fact pertains to.
type SubjectType string

const (
	SubjectTypeIncident        SubjectType = "INCIDENT"
	SubjectTypePartner         SubjectType = "PARTNER"
	SubjectTypeRemediationPlan SubjectType = "REMEDIATION_PLAN"
	SubjectTypeValidationRule  SubjectType = "VALIDATION_RULE"
	SubjectTypeTenantPolicy    SubjectType = "TENANT_POLICY"
	SubjectTypeFileFormat      SubjectType = "FILE_FORMAT"
	SubjectTypeArtifact        SubjectType = "ARTIFACT"
)

// FactType defines the semantic category of the operational fact.
type FactType string

const (
	FactTypeVerifiedRemediationSuccess FactType = "VERIFIED_REMEDIATION_SUCCESS"
	FactTypeVerifiedFailurePattern     FactType = "VERIFIED_FAILURE_PATTERN"
	FactTypePartnerFormatTolerance     FactType = "PARTNER_FILE_FORMAT_TOLERANCE"
	FactTypeOperationalSLABreach       FactType = "OPERATIONAL_SLA_BREACH"
	FactTypeCanonicalRuleAmendment     FactType = "CANONICAL_RULE_AMENDMENT"
	FactTypeHumanInvestigationOutcome  FactType = "HUMAN_INVESTIGATION_OUTCOME"
	FactTypeDualControlReleaseOutcome  FactType = "DUAL_CONTROL_RELEASE_OUTCOME"
)

// ConfidenceSource identifies how this operational fact was verified.
type ConfidenceSource string

const (
	ConfidenceSourceDeterministicDerived ConfidenceSource = "DETERMINISTIC_DERIVED"
	ConfidenceSourceHumanConfirmed       ConfidenceSource = "HUMAN_CONFIRMED"
	ConfidenceSourceVerifiedWorkflow     ConfidenceSource = "VERIFIED_WORKFLOW"
	ConfidenceSourceManagedMemorySuggest ConfidenceSource = "MANAGED_MEMORY_SUGGESTION"
)

// Classification defines the data classification tier.
type Classification string

const (
	ClassificationPublic       Classification = "PUBLIC"
	ClassificationInternal     Classification = "INTERNAL"
	ClassificationConfidential Classification = "CONFIDENTIAL"
	ClassificationRestricted   Classification = "RESTRICTED"
)

// MemoryStatus represents the operational lifecycle state of the memory record.
type MemoryStatus string

const (
	StatusActive      MemoryStatus = "ACTIVE"
	StatusExpired     MemoryStatus = "EXPIRED"
	StatusSuperseded  MemoryStatus = "SUPERSEDED"
	StatusInvalidated MemoryStatus = "INVALIDATED"
)

// OperationalMemoryRecord represents an authoritative M1 operational memory record in Go.
type OperationalMemoryRecord struct {
	MemoryID               string           `json:"memory_id"`
	TenantID               string           `json:"tenant_id"`
	MemoryType             MemoryType       `json:"memory_type"`
	SubjectType            SubjectType      `json:"subject_type"`
	SubjectRef             string           `json:"subject_ref"`
	FactType               FactType         `json:"fact_type"`
	StructuredValue        json.RawMessage  `json:"structured_value"`
	SourceRefs             []string         `json:"source_refs"`
	SourceHashes           []string         `json:"source_hashes"`
	SourceVerificationRefs []string         `json:"source_verification_refs"`
	ConfidenceSource       ConfidenceSource `json:"confidence_source"`
	Classification         Classification   `json:"classification"`
	Status                 MemoryStatus     `json:"status"`
	ValidFrom              time.Time        `json:"valid_from"`
	ExpiresAt              *time.Time       `json:"expires_at,omitempty"`
	SupersededBy           *string          `json:"superseded_by,omitempty"`
	CreatedAt              time.Time        `json:"created_at"`
	CreatedBy              string           `json:"created_by"`
	MemoryHash             string           `json:"memory_hash"`
}

// MemorySourceRecord represents a normalized provenance source link in memory_sources.
type MemorySourceRecord struct {
	ID                    int64     `json:"id"`
	MemoryID              string    `json:"memory_id"`
	TenantID              string    `json:"tenant_id"`
	SourceRef             string    `json:"source_ref"`
	SourceHash            string    `json:"source_hash"`
	SourceVerificationRef *string   `json:"source_verification_ref,omitempty"`
	CreatedAt             time.Time `json:"created_at"`
}

// MemoryRevisionRecord represents an audit ledger entry in memory_revisions.
type MemoryRevisionRecord struct {
	ID             string    `json:"id"`
	MemoryID       string    `json:"memory_id"`
	TenantID       string    `json:"tenant_id"`
	RevisionNumber int       `json:"revision_number"`
	PreviousHash   *string   `json:"previous_hash,omitempty"`
	NewHash        string    `json:"new_hash"`
	TransitionType string    `json:"transition_type"`
	Reason         string    `json:"reason"`
	ActorID        string    `json:"actor_id"`
	CreatedAt      time.Time `json:"created_at"`
}
