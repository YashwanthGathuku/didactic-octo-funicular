package verification

import (
	"errors"
	"time"
)

var (
	ErrCandidateNotFound  = errors.New("candidate artifact not found")
	ErrDerivationNotFound = errors.New("artifact derivation record not found")
	ErrParentNotFound     = errors.New("parent artifact not found")
	ErrNilRequest         = errors.New("verification request is nil")
	ErrMissingTenantID    = errors.New("tenant_id is required")
	ErrMissingWorkflowID  = errors.New("workflow_id is required")
	ErrInvalidCandidateID = errors.New("candidate_artifact_id must be greater than 0")
	ErrMissingExpectedSHA = errors.New("expected candidate/parent sha256 hashes are required")
	ErrCorruptionDetected = errors.New("artifact or derivation corruption detected")
	ErrPolicyStale        = errors.New("policy bundle context is stale")
	ErrVerificationFailed = errors.New("candidate deterministic verification failed")
)

// VerificationOutcome defines the authoritative verdict for a candidate verification run.
type VerificationOutcome string

const (
	OutcomePass               VerificationOutcome = "PASS"
	OutcomeFail               VerificationOutcome = "FAIL"
	OutcomeStale              VerificationOutcome = "STALE"
	OutcomeCorruptionDetected VerificationOutcome = "CORRUPTION_DETECTED"
	OutcomeError              VerificationOutcome = "ERROR"
)

// VerificationCheckType identifies each distinct verification check performed.
type VerificationCheckType string

const (
	CheckParentHashMatch       VerificationCheckType = "PARENT_HASH_MATCH"
	CheckCandidateHashMatch    VerificationCheckType = "CANDIDATE_HASH_MATCH"
	CheckDerivationHashMatch   VerificationCheckType = "DERIVATION_HASH_MATCH"
	CheckParentBindingMatch    VerificationCheckType = "PARENT_BINDING_MATCH"
	CheckWorkflowBindingMatch  VerificationCheckType = "WORKFLOW_BINDING_MATCH"
	CheckAttemptBindingMatch   VerificationCheckType = "ATTEMPT_BINDING_MATCH"
	CheckPlanHashMatch         VerificationCheckType = "PLAN_HASH_MATCH"
	CheckValidatorPass         VerificationCheckType = "VALIDATOR_PASS"
	CheckValidationResultMatch VerificationCheckType = "VALIDATION_RESULT_MATCH"
	CheckPolicyContextFresh    VerificationCheckType = "POLICY_CONTEXT_FRESH"
	CheckEvidenceContextValid  VerificationCheckType = "EVIDENCE_CONTEXT_VALID"
	CheckSemanticDiffValid     VerificationCheckType = "SEMANTIC_DIFF_VALID"
)

// VerificationCheck records the diagnostic output and outcome of a specific check.
type VerificationCheck struct {
	Type     VerificationCheckType `json:"type"`
	Passed   bool                  `json:"passed"`
	Message  string                `json:"message"`
	Expected string                `json:"expected"`
	Actual   string                `json:"actual"`
}

// VerificationRequest specifies the parameters for verifying a candidate artifact derivation.
type VerificationRequest struct {
	TenantID                string `json:"tenant_id"`
	WorkflowID              string `json:"workflow_id"`
	CandidateArtifactID     int64  `json:"candidate_artifact_id"`
	ExpectedCandidateSHA256 string `json:"expected_candidate_sha256"`
	ExpectedParentSHA256    string `json:"expected_parent_sha256"`
	DerivationID            string `json:"derivation_id"`
	PolicyBundleHash        string `json:"policy_bundle_hash"`
	CallerID                string `json:"caller_id"`
}

// VerificationResult holds the immutable, cryptographically verifiable result of deterministic candidate verification.
type VerificationResult struct {
	VerificationID       string              `json:"verification_id"`
	TenantID             string              `json:"tenant_id"`
	WorkflowID           string              `json:"workflow_id"`
	CandidateArtifactID  int64               `json:"candidate_artifact_id"`
	CandidateSHA256      string              `json:"candidate_sha256"`
	ParentArtifactID     int64               `json:"parent_artifact_id"`
	ParentSHA256         string              `json:"parent_sha256"`
	DerivationID         string              `json:"derivation_id"`
	DerivationHash       string              `json:"derivation_hash"`
	RemediationPlanHash  string              `json:"remediation_plan_hash"`
	P07ValidationRunID   string              `json:"p07_validation_run_id"`
	P08ValidationRunID   string              `json:"p08_validation_run_id"`
	ValidatorVersion     string              `json:"validator_version"`
	RulepackHash         string              `json:"rulepack_hash"`
	PolicyBundleHash     string              `json:"policy_bundle_hash"`
	Checks               []VerificationCheck `json:"checks"`
	DeterministicOutcome VerificationOutcome `json:"deterministic_outcome"`
	VerificationHash     string              `json:"verification_hash"`
	CreatedAt            time.Time           `json:"created_at"`
}
