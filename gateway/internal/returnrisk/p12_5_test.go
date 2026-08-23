package returnrisk

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

type returnRiskSemanticsFixture struct {
	Thresholds struct {
		Unauthorized  float64 `json:"unauthorized"`
		Administrative float64 `json:"administrative"`
		Overall       float64 `json:"overall"`
	} `json:"thresholds"`
	UnauthorizedReturnRateCodes []string `json:"unauthorized_return_rate_codes"`
	ReturnCodes                 map[string]struct {
		Title              string `json:"title"`
		NormalizedCategory string `json:"normalized_category"`
		ReturnWindow       string `json:"return_window"`
		ThresholdCategory  string `json:"threshold_category"`
		ThresholdApplicable *bool  `json:"threshold_applicable"`
	} `json:"return_codes"`
}

func loadReturnRiskSemanticsFixture(t *testing.T) returnRiskSemanticsFixture {
	t.Helper()
	path := filepath.Join("..", "..", "..", "docs", "fixtures", "return_risk_semantics.json")
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read shared return-risk fixture: %v", err)
	}
	var fixture returnRiskSemanticsFixture
	if err := json.Unmarshal(raw, &fixture); err != nil {
		t.Fatalf("parse shared return-risk fixture: %v", err)
	}
	return fixture
}

func TestP125_PublicThresholdValuesAndSharedFixture(t *testing.T) {
	fixture := loadReturnRiskSemanticsFixture(t)
	if UnauthorizedReturnRateThreshold != 0.005 || fixture.Thresholds.Unauthorized != UnauthorizedReturnRateThreshold {
		t.Fatalf("unauthorized threshold mismatch: go=%v fixture=%v", UnauthorizedReturnRateThreshold, fixture.Thresholds.Unauthorized)
	}
	if AdministrativeReturnRateLevel != 0.030 || fixture.Thresholds.Administrative != AdministrativeReturnRateLevel {
		t.Fatalf("administrative threshold mismatch: go=%v fixture=%v", AdministrativeReturnRateLevel, fixture.Thresholds.Administrative)
	}
	if OverallReturnRateLevel != 0.150 || fixture.Thresholds.Overall != OverallReturnRateLevel {
		t.Fatalf("overall threshold mismatch: go=%v fixture=%v", OverallReturnRateLevel, fixture.Thresholds.Overall)
	}
}

func TestP125_R10AndR11CurrentSemantics(t *testing.T) {
	fixture := loadReturnRiskSemanticsFixture(t)

	r10, err := LookupReturnCode("R10")
	if err != nil {
		t.Fatalf("R10 lookup failed: %v", err)
	}
	if r10.Title != fixture.ReturnCodes["R10"].Title {
		t.Fatalf("R10 title drift: %q", r10.Title)
	}
	if r10.ThresholdCategory != ThresholdUnauthorized05Percent || r10.ReturnWindow != ReturnWindow60CalendarDays {
		t.Fatalf("R10 threshold/window mismatch: %s %s", r10.ThresholdCategory, r10.ReturnWindow)
	}

	r11, err := LookupReturnCode("R11")
	if err != nil {
		t.Fatalf("R11 lookup failed: %v", err)
	}
	if r11.Title != fixture.ReturnCodes["R11"].Title {
		t.Fatalf("R11 title drift: %q", r11.Title)
	}
	if r11.NormalizedCategory != CategoryAuthorizationTerms {
		t.Fatalf("R11 category=%s, want %s", r11.NormalizedCategory, CategoryAuthorizationTerms)
	}
	if r11.ReturnWindow != ReturnWindow60CalendarDays || r11.ThresholdCategory != ThresholdUnauthorized05Percent {
		t.Fatalf("R11 must use extended window and unauthorized return-rate handling")
	}

	foundR11 := false
	for _, code := range fixture.UnauthorizedReturnRateCodes {
		if code == "R11" {
			foundR11 = true
			break
		}
	}
	if !foundR11 {
		t.Fatal("shared fixture must include R11 in unauthorized return-rate family")
	}
}

func TestP125_R16HasNoInventedPercentageThreshold(t *testing.T) {
	engine, err := NewDeterministicRiskEngine(DefaultEngineConfig())
	if err != nil {
		t.Fatal(err)
	}
	code, err := LookupReturnCode("R16")
	if err != nil {
		t.Fatal(err)
	}
	vec := engine.extractFeatureVector(
		ReturnEvent{ReturnCode: "R16", VerificationStatus: VerificationStatusVerified},
		code,
		HistoricalReturnContext{PartnerTotalEntries30d: 100, PartnerTotalReturns30d: 100},
		SLAContext{},
	)
	if vec.PartnerReturnRateThresholdApplicable {
		t.Fatal("R16 regulatory-restricted category must not have a percentage threshold")
	}
	if vec.PartnerReturnRateThreshold != 0 || vec.PartnerReturnRate != 0 {
		t.Fatalf("R16 threshold contribution must be neutral/not-applicable, got threshold=%v score=%v", vec.PartnerReturnRateThreshold, vec.PartnerReturnRate)
	}
}

func TestP125_AssessmentHashRFC8785Vector(t *testing.T) {
	result := &ReturnRiskResult{
		TenantID:      "TENANT-1",
		WorkflowID:    "WF-1",
		ReturnEventID: "RET-1",
		ReturnCode:    "R11",
		RiskScore:     42.5,
		RiskTier:      RiskTierMedium,
		Contributions: []RiskContribution{},
		FeatureVector: RiskFeatureVector{},
		EngineVersion: EngineVersion,
	}
	got, err := computeAssessmentHash(result)
	if err != nil {
		t.Fatal(err)
	}
	const want = "22ab67eb76f150dc9ecc3d50393daef8356b136ba53ed73d954cd39e14de8c4d"
	if got != want {
		t.Fatalf("RFC8785 assessment vector mismatch: got %s want %s", got, want)
	}
}

func TestP125_TaxonomyGuidanceIsOperationalNotAuthority(t *testing.T) {
	allowed := map[GuidanceType]bool{
		GuidanceReviewRequired:               true,
		GuidanceComplianceReviewRequired:     true,
		GuidanceAuthorizationReviewRequired:  true,
		GuidanceDoNotAutomaticallyReinitiate: true,
		GuidanceCorrectionRequired:           true,
		GuidanceStandardExceptionReview:      true,
	}
	for code, def := range Catalog {
		if !allowed[def.OperationalGuidance] {
			t.Fatalf("%s has non-governed guidance type %q", code, def.OperationalGuidance)
		}
		lower := strings.ToLower(def.GuidanceSummary)
		for _, forbidden := range []string{"strictly prohibited by federal law", "reinitiation is illegal", "cease all transaction activity", "approve the file", "authorize release"} {
			if strings.Contains(lower, forbidden) {
				t.Fatalf("%s contains unsupported authority language %q", code, forbidden)
			}
		}
		if len(def.SourceProvenance) == 0 {
			t.Fatalf("%s is missing public source provenance", code)
		}
	}
}
