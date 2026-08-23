package memory

import "time"

// RemediationSuccessFact payload for FactTypeVerifiedRemediationSuccess.
type RemediationSuccessFact struct {
	WorkflowID           string   `json:"workflow_id"`
	IncidentID           int64    `json:"incident_id"`
	ParentArtifactSHA    string   `json:"parent_artifact_sha"`
	CandidateArtifactSHA string   `json:"candidate_artifact_sha"`
	DerivationHash       string   `json:"derivation_hash"`
	VerificationHash     string   `json:"verification_hash"`
	AppliedOperations    []string `json:"applied_operations"`
	ResolvedRuleCodes    []string `json:"resolved_rule_codes"`
	AttemptNumber        int      `json:"attempt_number"`
	ValidatorVersion     string   `json:"validator_version"`
}

// VerifiedFailurePatternFact payload for FactTypeVerifiedFailurePattern.
type VerifiedFailurePatternFact struct {
	WorkflowID        string   `json:"workflow_id"`
	IncidentID        int64    `json:"incident_id"`
	RuleCode          string   `json:"rule_code"`
	FailedCheckType   string   `json:"failed_check_type"`
	ObservedPattern   string   `json:"observed_pattern"`
	AttemptCount      int      `json:"attempt_count"`
	FinalVerdict      string   `json:"final_verdict"`
	VerificationRef   string   `json:"verification_ref"`
}

// PartnerFormatToleranceFact payload for FactTypePartnerFormatTolerance.
type PartnerFormatToleranceFact struct {
	PartnerID           string   `json:"partner_id"`
	FileStandard        string   `json:"file_standard"`
	PermittedVariance   string   `json:"permitted_variance"`
	MaxBatchCount       int      `json:"max_batch_count"`
	SettlementCutoffUTC string   `json:"settlement_cutoff_utc"`
	VerificationRef     string   `json:"verification_ref"`
}

// OperationalSLABreachFact payload for FactTypeOperationalSLABreach.
type OperationalSLABreachFact struct {
	PartnerID           string    `json:"partner_id"`
	ExpectedDeliveryUTC time.Time `json:"expected_delivery_utc"`
	ActualDeliveryUTC   time.Time `json:"actual_delivery_utc"`
	DelayMinutes        int64     `json:"delay_minutes"`
	ImpactSeverity      string    `json:"impact_severity"`
	IncidentRef         string    `json:"incident_ref"`
}

// HumanInvestigationOutcomeFact payload for FactTypeHumanInvestigationOutcome.
type HumanInvestigationOutcomeFact struct {
	WorkflowID         string   `json:"workflow_id"`
	IncidentID         int64    `json:"incident_id"`
	ReviewerID         string   `json:"reviewer_id"`
	ResolutionAction   string   `json:"resolution_action"`
	NotesSummary       string   `json:"notes_summary"`
	VerifiedArtifactID *int64   `json:"verified_artifact_id,omitempty"`
	ApprovedAt         time.Time `json:"approved_at"`
}

// DualControlReleaseOutcomeFact payload for FactTypeDualControlReleaseOutcome.
type DualControlReleaseOutcomeFact struct {
	ReleaseID       string    `json:"release_id"`
	WorkflowID      string    `json:"workflow_id"`
	ArtifactSHA256  string    `json:"artifact_sha256"`
	InitiatorID     string    `json:"initiator_id"`
	ApproverID      string    `json:"approver_id"`
	ReleaseTarget   string    `json:"release_target"`
	SettledAt       time.Time `json:"settled_at"`
}
