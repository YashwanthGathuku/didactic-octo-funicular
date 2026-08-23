package returnrisk

import (
	"errors"
	"time"

	"sentinel-gateway/internal/memory"
)

const EngineVersion = "1.0.1-p12.5"

// Public Nacha return-rate monitoring values used by the deterministic engine.
// These are operational risk-monitoring inputs, not legal or compliance decisions.
const (
	UnauthorizedReturnRateThreshold = 0.005
	AdministrativeReturnRateLevel   = 0.030
	OverallReturnRateLevel          = 0.150
)

var (
	ErrNilScope             = errors.New("returnrisk: repository scope is required")
	ErrNilEvent             = errors.New("returnrisk: return event is nil")
	ErrMissingEventID       = errors.New("returnrisk: return_event_id is required")
	ErrMissingTenantID      = errors.New("returnrisk: tenant_id is required")
	ErrTenantMismatch       = errors.New("returnrisk: event tenant_id does not match caller scope")
	ErrMissingReturnCode    = errors.New("returnrisk: return_code is required")
	ErrInvalidReturnCode    = errors.New("returnrisk: unrecognized NACHA return code")
	ErrInvalidAmount        = errors.New("returnrisk: amount_cents cannot be negative")
	ErrInvalidWeights       = errors.New("returnrisk: engine feature weights must sum to 1.0")
	ErrNilMemoryStore       = errors.New("returnrisk: operational memory store is required")
	ErrAssessmentNotFound   = errors.New("returnrisk: risk assessment record not found")
	ErrInvalidFeatureVector = errors.New("returnrisk: feature vector values out of legal range [0, 100]")
)

type RiskTier string

const (
	RiskTierLow    RiskTier = "LOW"
	RiskTierMedium RiskTier = "MEDIUM"
	RiskTierHigh   RiskTier = "HIGH"
	RiskTierSevere RiskTier = "SEVERE"
)

type ReturnCategory string

const (
	CategoryInsufficientFunds   ReturnCategory = "INSUFFICIENT_FUNDS"
	CategoryAccountStatus       ReturnCategory = "ACCOUNT_STATUS"
	CategoryAccountData         ReturnCategory = "ACCOUNT_DATA"
	CategoryUnauthorized        ReturnCategory = "UNAUTHORIZED"
	CategoryAuthorizationTerms  ReturnCategory = "AUTHORIZATION_TERMS"
	CategoryAdministrative      ReturnCategory = "ADMINISTRATIVE"
	CategoryOFACRestricted      ReturnCategory = "OFAC_RESTRICTED"
)

type OperationalSeverity string

const (
	SeverityLow      OperationalSeverity = "LOW"
	SeverityMedium   OperationalSeverity = "MEDIUM"
	SeverityHigh     OperationalSeverity = "HIGH"
	SeverityCritical OperationalSeverity = "CRITICAL"
)

type RetryCharacteristic string

const (
	RetryableOnce           RetryCharacteristic = "RETRYABLE_ONCE"
	RetryableWithCorrection RetryCharacteristic = "RETRYABLE_WITH_CORRECTION"
	NonRetryable            RetryCharacteristic = "NON_RETRYABLE"
	Prohibited              RetryCharacteristic = "PROHIBITED"
)

type ReturnWindowType string

const (
	ReturnWindow2BankingDays   ReturnWindowType = "STANDARD_2_BANKING_DAYS"
	ReturnWindow60CalendarDays ReturnWindowType = "EXTENDED_60_CALENDAR_DAYS"
)

type ThresholdType string

const (
	ThresholdUnauthorized05Percent  ThresholdType = "UNAUTHORIZED_0_5_PERCENT"
	ThresholdAdministrative3Percent ThresholdType = "ADMINISTRATIVE_3_0_PERCENT"
	ThresholdOverall15Percent       ThresholdType = "OVERALL_15_0_PERCENT"
	ThresholdRegulatoryRestricted   ThresholdType = "REGULATORY_RESTRICTED"
)

type GuidanceType string

const (
	GuidanceReviewRequired               GuidanceType = "REVIEW_REQUIRED"
	GuidanceComplianceReviewRequired     GuidanceType = "COMPLIANCE_REVIEW_REQUIRED"
	GuidanceAuthorizationReviewRequired  GuidanceType = "AUTHORIZATION_REVIEW_REQUIRED"
	GuidanceDoNotAutomaticallyReinitiate GuidanceType = "DO_NOT_AUTOMATICALLY_REINITIATE"
	GuidanceCorrectionRequired           GuidanceType = "CORRECTION_REQUIRED"
	GuidanceStandardExceptionReview      GuidanceType = "STANDARD_EXCEPTION_REVIEW"
)

type PublicSourceProvenance struct {
	SourceID          string `json:"source_id"`
	SourceName        string `json:"source_name"`
	Reference         string `json:"reference"`
	RetrievedDate     string `json:"retrieved_date"`
	SemanticsVerified bool   `json:"semantics_verified"`
}

type VerificationStatus string

const (
	VerificationStatusUnverified VerificationStatus = "UNVERIFIED"
	VerificationStatusVerified   VerificationStatus = "VERIFIED"
	VerificationStatusDisputed   VerificationStatus = "DISPUTED"
	VerificationStatusRejected   VerificationStatus = "REJECTED"
)

// ACHReturnCode is one entry in SentinelFlow's representative MVP catalog, not a complete ACH taxonomy.
type ACHReturnCode struct {
	Code                string                   `json:"code"`
	ShortLabel          string                   `json:"short_label"`
	Title               string                   `json:"title"`
	Description         string                   `json:"description"`
	NormalizedCategory  ReturnCategory           `json:"normalized_category"`
	OperationalSeverity OperationalSeverity      `json:"operational_severity"`
	RetryCharacteristic RetryCharacteristic      `json:"retry_characteristic"`
	AccountDataIssue    bool                     `json:"account_data_issue"`
	AuthorizationIssue  bool                     `json:"authorization_issue"`
	AdministrativeIssue bool                     `json:"administrative_issue"`
	ReturnWindow        ReturnWindowType         `json:"return_window"`
	ThresholdCategory   ThresholdType            `json:"threshold_category"`
	BaseSeverity        float64                  `json:"base_severity"`
	TaxonomyVersion     string                   `json:"taxonomy_version"`
	OperationalGuidance GuidanceType             `json:"operational_guidance"`
	GuidanceSummary     string                   `json:"guidance_summary"`
	SourceProvenance    []PublicSourceProvenance `json:"source_provenance"`
}

type ReturnEvent struct {
	ReturnEventID      string                `json:"return_event_id"`
	TenantID           string                `json:"tenant_id"`
	WorkflowID         string                `json:"workflow_id"`
	ArtifactID         int64                 `json:"artifact_id"`
	PartnerRef         string                `json:"partner_ref"`
	ReturnCode         string                `json:"return_code"`
	AmountCents        int64                 `json:"amount_cents"`
	Timestamp          time.Time             `json:"timestamp"`
	SourceRef          string                `json:"source_ref"`
	SourceHash         string                `json:"source_hash"`
	VerificationStatus VerificationStatus    `json:"verification_status"`
	Classification     memory.Classification `json:"classification"`
}

type SLAContext struct {
	DeadlineUTC       time.Time     `json:"deadline_utc"`
	RemainingDuration time.Duration `json:"remaining_duration"`
	IsBreached        bool          `json:"is_breached"`
	BreachProbability float64       `json:"breach_probability"`
	TargetCutoffUTC   string        `json:"target_cutoff_utc"`
}

type HistoricalReturnContext struct {
	TotalReturns7d         int     `json:"total_returns_7d"`
	TotalReturns30d        int     `json:"total_returns_30d"`
	TotalVolume7dCents     int64   `json:"total_volume_7d_cents"`
	TotalVolume30dCents    int64   `json:"total_volume_30d_cents"`
	PartnerTotalEntries30d int     `json:"partner_total_entries_30d"`
	PartnerTotalReturns30d int     `json:"partner_total_returns_30d"`
	SameCodeCount30d       int     `json:"same_code_count_30d"`
	VerifiedPriorCount     int     `json:"verified_prior_count"`
	RecentTrendVelocity    float64 `json:"recent_trend_velocity"`
}

// RiskFeatureVector records seven weighted scoring features plus contextual/diagnostic features.
type RiskFeatureVector struct {
	ReturnCodeSeverity                   float64 `json:"return_code_severity"`
	ReturnFrequency7d                    float64 `json:"return_frequency_7d"`
	ReturnFrequency30d                   float64 `json:"return_frequency_30d"`
	PartnerReturnRate                    float64 `json:"partner_return_rate"`
	PartnerReturnRateThreshold           float64 `json:"partner_return_rate_threshold"`
	PartnerReturnRateThresholdApplicable bool    `json:"partner_return_rate_threshold_applicable"`
	SameCodeRecurrence                   float64 `json:"same_code_recurrence"`
	RecentTrend                          float64 `json:"recent_trend"`
	VerifiedPriorOccurrences             float64 `json:"verified_prior_occurrences"`
	SLAProximity                         float64 `json:"sla_proximity"`
	AmountExposureBucket                 float64 `json:"amount_exposure_bucket"`
	SourceStrength                       float64 `json:"source_strength"`
}

type RiskContribution struct {
	FeatureName       string  `json:"feature_name"`
	RawValue          float64 `json:"raw_value"`
	NormalizedValue   float64 `json:"normalized_value"`
	Weight            float64 `json:"weight"`
	ContributionScore float64 `json:"contribution_score"`
}

// ReturnRiskResult is operational risk intelligence, not a compliance or financial decision.
type ReturnRiskResult struct {
	AssessmentID   string             `json:"assessment_id"`
	TenantID       string             `json:"tenant_id"`
	WorkflowID     string             `json:"workflow_id"`
	ReturnEventID  string             `json:"return_event_id"`
	ReturnCode     string             `json:"return_code"`
	RiskScore      float64            `json:"risk_score"`
	RiskTier       RiskTier           `json:"risk_tier"`
	Contributions  []RiskContribution `json:"contributions"`
	PrimaryDrivers []string           `json:"primary_drivers"`
	FeatureVector  RiskFeatureVector  `json:"feature_vector"`
	SLAContext     SLAContext         `json:"sla_context"`
	AssessmentHash string             `json:"assessment_hash"`
	ComputedAt     time.Time          `json:"computed_at"`
	EngineVersion  string             `json:"engine_version"`
}

type ReturnRiskFactPayload struct {
	AssessmentID     string    `json:"assessment_id"`
	WorkflowID       string    `json:"workflow_id"`
	ReturnEventID    string    `json:"return_event_id"`
	PartnerRef       string    `json:"partner_ref"`
	ReturnCode       string    `json:"return_code"`
	AmountCents      int64     `json:"amount_cents"`
	RiskScore        float64   `json:"risk_score"`
	RiskTier         RiskTier  `json:"risk_tier"`
	PrimaryDrivers   []string  `json:"primary_drivers"`
	ResolutionAction string    `json:"resolution_action"`
	VerifierRef      string    `json:"verifier_ref"`
	ResolvedAt       time.Time `json:"resolved_at"`
	AssessmentHash   string    `json:"assessment_hash"`
}
