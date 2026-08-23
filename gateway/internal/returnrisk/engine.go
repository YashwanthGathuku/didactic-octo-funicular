package returnrisk

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"math"
	"sort"
	"time"

	"sentinel-gateway/internal/memory"
	"sentinel-gateway/internal/repository"
)

// EngineConfig provides configurable weights with strict normalization validation.
type EngineConfig struct {
	WeightCodeSeverity      float64
	WeightFreq7d            float64
	WeightFreq30d           float64
	WeightPartnerReturnRate float64
	WeightTrend             float64
	WeightExposure          float64
	WeightSLA               float64
}

// DefaultEngineConfig returns standard normalized SentinelFlow P12 weights.
func DefaultEngineConfig() EngineConfig {
	return EngineConfig{
		WeightCodeSeverity:      0.30,
		WeightFreq7d:            0.15,
		WeightFreq30d:           0.10,
		WeightPartnerReturnRate: 0.15,
		WeightTrend:             0.10,
		WeightExposure:          0.10,
		WeightSLA:               0.10,
	}
}

// Validate ensures all weights sum to exactly 1.00 within floating point epsilon.
func (c EngineConfig) Validate() error {
	sum := c.WeightCodeSeverity + c.WeightFreq7d + c.WeightFreq30d +
		c.WeightPartnerReturnRate + c.WeightTrend + c.WeightExposure + c.WeightSLA
	if math.Abs(sum-1.0) > 1e-6 {
		return fmt.Errorf("%w: weights sum to %f, expected 1.0", ErrInvalidWeights, sum)
	}
	return nil
}

// RiskEngine defines the interface for deterministic ACH return risk assessment.
type RiskEngine interface {
	CalculateRisk(
		ctx context.Context,
		scope repository.Scope,
		event ReturnEvent,
		history HistoricalReturnContext,
		sla SLAContext,
	) (*ReturnRiskResult, error)

	EmitVerifiedReturnFact(
		ctx context.Context,
		scope repository.Scope,
		store *memory.Store,
		result *ReturnRiskResult,
		resolutionAction string,
		verifierRef string,
	) (*memory.OperationalMemoryRecord, error)
}

// DeterministicRiskEngine implements RiskEngine with immutable cryptographic audit guarantees.
type DeterministicRiskEngine struct {
	config EngineConfig
}

// NewDeterministicRiskEngine instantiates an engine instance with validated config.
func NewDeterministicRiskEngine(cfg EngineConfig) (*DeterministicRiskEngine, error) {
	if err := cfg.Validate(); err != nil {
		return nil, err
	}
	return &DeterministicRiskEngine{config: cfg}, nil
}

// CalculateRisk executes the deterministic risk scoring algorithm.
func (e *DeterministicRiskEngine) CalculateRisk(
	ctx context.Context,
	scope repository.Scope,
	event ReturnEvent,
	history HistoricalReturnContext,
	sla SLAContext,
) (*ReturnRiskResult, error) {
	// 1. Zero-Trust Tenant Scoping Invariant
	if scope.TenantID() == "" {
		return nil, ErrNilScope
	}
	if event.TenantID == "" {
		event.TenantID = scope.TenantID()
	} else if event.TenantID != scope.TenantID() {
		return nil, fmt.Errorf("%w: event tenant %s != scope tenant %s", ErrTenantMismatch, event.TenantID, scope.TenantID())
	}
	if event.ReturnEventID == "" {
		return nil, ErrMissingEventID
	}
	if event.AmountCents < 0 {
		return nil, ErrInvalidAmount
	}

	// 2. Resolve NACHA Return Code Taxonomy
	codeDef, err := LookupReturnCode(event.ReturnCode)
	if err != nil {
		return nil, err
	}

	// 3. Extract & Normalize Feature Vector
	vec := e.extractFeatureVector(event, codeDef, history, sla)

	// 4. Compute Weighted Contributions
	contributions := []RiskContribution{
		{
			FeatureName:       "ReturnCodeSeverity",
			RawValue:          codeDef.BaseSeverity,
			NormalizedValue:   vec.ReturnCodeSeverity,
			Weight:            e.config.WeightCodeSeverity,
			ContributionScore: math.Round(vec.ReturnCodeSeverity*e.config.WeightCodeSeverity*100) / 100,
		},
		{
			FeatureName:       "ReturnFrequency7d",
			RawValue:          float64(history.TotalReturns7d),
			NormalizedValue:   vec.ReturnFrequency7d,
			Weight:            e.config.WeightFreq7d,
			ContributionScore: math.Round(vec.ReturnFrequency7d*e.config.WeightFreq7d*100) / 100,
		},
		{
			FeatureName:       "ReturnFrequency30d",
			RawValue:          float64(history.TotalReturns30d),
			NormalizedValue:   vec.ReturnFrequency30d,
			Weight:            e.config.WeightFreq30d,
			ContributionScore: math.Round(vec.ReturnFrequency30d*e.config.WeightFreq30d*100) / 100,
		},
		{
			FeatureName:       "PartnerReturnRate",
			RawValue:          float64(history.PartnerTotalReturns30d) / math.Max(1.0, float64(history.PartnerTotalEntries30d)),
			NormalizedValue:   vec.PartnerReturnRate,
			Weight:            e.config.WeightPartnerReturnRate,
			ContributionScore: math.Round(vec.PartnerReturnRate*e.config.WeightPartnerReturnRate*100) / 100,
		},
		{
			FeatureName:       "RecentTrend",
			RawValue:          history.RecentTrendVelocity,
			NormalizedValue:   vec.RecentTrend,
			Weight:            e.config.WeightTrend,
			ContributionScore: math.Round(vec.RecentTrend*e.config.WeightTrend*100) / 100,
		},
		{
			FeatureName:       "AmountExposureBucket",
			RawValue:          float64(event.AmountCents),
			NormalizedValue:   vec.AmountExposureBucket,
			Weight:            e.config.WeightExposure,
			ContributionScore: math.Round(vec.AmountExposureBucket*e.config.WeightExposure*100) / 100,
		},
		{
			FeatureName:       "SLAProximity",
			RawValue:          sla.RemainingDuration.Hours(),
			NormalizedValue:   vec.SLAProximity,
			Weight:            e.config.WeightSLA,
			ContributionScore: math.Round(vec.SLAProximity*e.config.WeightSLA*100) / 100,
		},
	}

	// 5. Compute Raw & Clamped Composite Risk Score
	var rawScore float64
	for _, c := range contributions {
		rawScore += c.NormalizedValue * c.Weight
	}
	clampedScore := math.Max(0.0, math.Min(100.0, rawScore))
	finalRiskScore := math.Round(clampedScore*100) / 100

	// 6. Assign Risk Tier
	tier := RiskTierLow
	switch {
	case finalRiskScore >= 80.0:
		tier = RiskTierSevere
	case finalRiskScore >= 60.0:
		tier = RiskTierHigh
	case finalRiskScore >= 30.0:
		tier = RiskTierMedium
	default:
		tier = RiskTierLow
	}

	// 7. Extract Primary Drivers (Top 3 Contributions)
	sortedContribs := make([]RiskContribution, len(contributions))
	copy(sortedContribs, contributions)
	sort.Slice(sortedContribs, func(i, j int) bool {
		return sortedContribs[i].ContributionScore > sortedContribs[j].ContributionScore
	})

	primaryDrivers := make([]string, 0, 3)
	for i := 0; i < len(sortedContribs) && i < 3; i++ {
		if sortedContribs[i].ContributionScore > 0 {
			primaryDrivers = append(primaryDrivers, fmt.Sprintf("%s (score=%.2f, weight=%.2f)",
				sortedContribs[i].FeatureName, sortedContribs[i].ContributionScore, sortedContribs[i].Weight))
		}
	}

	// 8. Mint Assessment ID and Canonical Cryptographic Hash
	now := time.Now().UTC()
	assessmentID := fmt.Sprintf("rr-asm-%s-%d", event.ReturnEventID, now.UnixNano())

	result := &ReturnRiskResult{
		AssessmentID:   assessmentID,
		TenantID:       event.TenantID,
		WorkflowID:     event.WorkflowID,
		ReturnEventID:  event.ReturnEventID,
		ReturnCode:     event.ReturnCode,
		RiskScore:      finalRiskScore,
		RiskTier:       tier,
		Contributions:  contributions,
		PrimaryDrivers: primaryDrivers,
		FeatureVector:  vec,
		SLAContext:     sla,
		ComputedAt:     now,
		EngineVersion:  EngineVersion,
	}

	// Compute immutable assessment digest
	resultHash, err := computeAssessmentHash(result)
	if err != nil {
		return nil, fmt.Errorf("failed to compute assessment hash: %w", err)
	}
	result.AssessmentHash = resultHash

	return result, nil
}

// extractFeatureVector maps domain event and context into [0, 100] normalized scores.
func (e *DeterministicRiskEngine) extractFeatureVector(
	event ReturnEvent,
	code ACHReturnCode,
	history HistoricalReturnContext,
	sla SLAContext,
) RiskFeatureVector {
	// 1. Code Severity
	codeSeverity := math.Max(0.0, math.Min(100.0, code.BaseSeverity))

	// 2. Frequency 7d (sublinear saturation)
	freq7 := math.Min(100.0, 100.0*(1.0-math.Exp(-0.08*float64(history.TotalReturns7d))))

	// 3. Frequency 30d (sublinear saturation)
	freq30 := math.Min(100.0, 100.0*(1.0-math.Exp(-0.025*float64(history.TotalReturns30d))))

	// 4. Partner Return Rate vs Regulatory Threshold
	actualRate := float64(history.PartnerTotalReturns30d) / math.Max(1.0, float64(history.PartnerTotalEntries30d))
	threshold := 0.075
	if code.ThresholdCategory == ThresholdUnauthorized05Percent {
		threshold = 0.005
	} else if code.ThresholdCategory == ThresholdAdministrative3Percent {
		threshold = 0.030
	} else if code.ThresholdCategory == ThresholdOverall15Percent {
		threshold = 0.150
	}
	partnerRate := math.Min(100.0, (actualRate/threshold)*50.0)

	// 5. Same Code Recurrence
	sameCodeRecurrence := math.Min(100.0, float64(history.SameCodeCount30d)*10.0)

	// 6. Recent Trend Velocity
	v7 := float64(history.TotalReturns7d) / 7.0
	v30 := float64(history.TotalReturns30d) / 30.0
	velRatio := v7 / math.Max(0.1, v30)
	trend := math.Max(0.0, math.Min(100.0, 50.0+25.0*(velRatio-1.0)))

	// 7. Amount Exposure Bucket
	var exposure float64
	switch {
	case event.AmountCents >= 25000000: // >= $250,000
		exposure = 100.0
	case event.AmountCents >= 5000000: // >= $50,000
		exposure = 80.0
	case event.AmountCents >= 1000000: // >= $10,000
		exposure = 60.0
	case event.AmountCents >= 100000: // >= $1,000
		exposure = 30.0
	default:
		exposure = 10.0
	}

	// 8. SLA Proximity
	var slaScore float64
	if sla.IsBreached || sla.RemainingDuration <= 0 {
		slaScore = 100.0
	} else if sla.RemainingDuration <= 2*time.Hour {
		slaScore = 90.0
	} else if sla.RemainingDuration <= 6*time.Hour {
		slaScore = 70.0
	} else if sla.RemainingDuration <= 24*time.Hour {
		slaScore = 45.0
	} else {
		slaScore = 15.0
	}

	// 9. Source Strength
	sourceStrength := 50.0
	if event.VerificationStatus == VerificationStatusVerified {
		sourceStrength = 100.0
	} else if event.VerificationStatus == VerificationStatusDisputed {
		sourceStrength = 20.0
	}

	// 10. Verified Prior Occurrences
	verifiedPrior := math.Min(100.0, float64(history.VerifiedPriorCount)*20.0)

	return RiskFeatureVector{
		ReturnCodeSeverity:       math.Round(codeSeverity*100) / 100,
		ReturnFrequency7d:        math.Round(freq7*100) / 100,
		ReturnFrequency30d:       math.Round(freq30*100) / 100,
		PartnerReturnRate:        math.Round(partnerRate*100) / 100,
		SameCodeRecurrence:       math.Round(sameCodeRecurrence*100) / 100,
		RecentTrend:              math.Round(trend*100) / 100,
		VerifiedPriorOccurrences: math.Round(verifiedPrior*100) / 100,
		SLAProximity:             math.Round(slaScore*100) / 100,
		AmountExposureBucket:     exposure,
		SourceStrength:           sourceStrength,
	}
}

// computeAssessmentHash builds a deterministic canonical SHA-256 hash of calculation inputs/outputs.
func computeAssessmentHash(r *ReturnRiskResult) (string, error) {
	canonicalMap := map[string]interface{}{
		"assessment_id":   r.AssessmentID,
		"tenant_id":       r.TenantID,
		"workflow_id":     r.WorkflowID,
		"return_event_id": r.ReturnEventID,
		"return_code":     r.ReturnCode,
		"risk_score":      r.RiskScore,
		"risk_tier":       string(r.RiskTier),
		"feature_vector":  r.FeatureVector,
		"engine_version":  r.EngineVersion,
	}
	rawJSON, err := json.Marshal(canonicalMap)
	if err != nil {
		return "", err
	}
	h := sha256.Sum256(rawJSON)
	return hex.EncodeToString(h[:]), nil
}
