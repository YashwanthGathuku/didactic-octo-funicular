package returnrisk

import "testing"

func TestP125_R11OperationalCategoryMatchesSharedFixture(t *testing.T) {
	fixture := loadReturnRiskSemanticsFixture(t)
	r11, err := LookupReturnCode("R11")
	if err != nil {
		t.Fatalf("R11 lookup failed: %v", err)
	}

	want := fixture.ReturnCodes["R11"].NormalizedCategory
	if string(r11.NormalizedCategory) != want {
		t.Fatalf("R11 normalized category drift: go=%q fixture=%q", r11.NormalizedCategory, want)
	}
	if r11.NormalizedCategory != CategoryAuthorizationTerms {
		t.Fatalf("R11 operational category=%q, want %q", r11.NormalizedCategory, CategoryAuthorizationTerms)
	}
	if r11.ThresholdCategory != ThresholdUnauthorized05Percent {
		t.Fatalf("R11 must remain in unauthorized return-rate monitoring family")
	}
}
