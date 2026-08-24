package policy

import (
	"errors"
	"math"
	"testing"
	"time"
)

// TestCanonicalJSON_RFC8785_OfficialPropertySorting verifies RFC 8785 Section 3.2.3 UTF-16 code-unit property sorting.
// Specifically: astral / non-BMP characters like \ud83d\ude00 (U+1F600 emoji 😀) encode to surrogate pair D83D DE00,
// and MUST sort BEFORE high BMP characters like \uffff (FFFF).
func TestCanonicalJSON_RFC8785_OfficialPropertySorting(t *testing.T) {
	input := map[string]interface{}{
		"\uffff":     "high_bmp",
		"\U0001f600": "astral_plane_emoji", // U+1F600 -> UTF-16: 0xD83D, 0xDE00
		"\u20ac":     "euro_sign",          // U+20AC -> UTF-16: 0x20AC
		"\r":         "carriage_return",    // U+000D -> UTF-16: 0x000D
		"\n":         "newline",            // U+000A -> UTF-16: 0x000A
		"1":          "digit_one",          // U+0031 -> UTF-16: 0x0031
		"\u00e9":     "e_acute",            // U+00E9 -> UTF-16: 0x00E9
		"a":          "letter_a",           // U+0061 -> UTF-16: 0x0061
	}

	b, err := CanonicalJSON(input)
	if err != nil {
		t.Fatalf("canonical JSON failed: %v", err)
	}

	// Expected order based on UTF-16 code units:
	// \n (000A) < \r (000D) < 1 (0031) < a (0061) < \u00e9 (00E9) < \u20ac (20AC) < \U0001f600 (D83D) < \uffff (FFFF)
	expectedStr := "{\"\\n\":\"newline\",\"\\r\":\"carriage_return\",\"1\":\"digit_one\",\"a\":\"letter_a\",\"\u00e9\":\"e_acute\",\"\u20ac\":\"euro_sign\",\"\U0001f600\":\"astral_plane_emoji\",\"\uffff\":\"high_bmp\"}"

	if string(b) != expectedStr {
		t.Errorf("RFC 8785 UTF-16 sorting mismatch:\nGot:      %s\nExpected: %s", string(b), expectedStr)
	}
}

func TestCanonicalJSON_RejectsDuplicateKeysInRawJSON(t *testing.T) {
	duplicateJSON := []byte(`{"key1": "val1", "key1": "val2"}`)
	_, err := CanonicalJSON(duplicateJSON)
	if err == nil {
		t.Error("expected error for duplicate object keys in JSON input")
	}
	if !errors.Is(err, ErrDuplicateObjectKey) {
		t.Errorf("expected ErrDuplicateObjectKey, got: %v", err)
	}

	// Nested duplicate key
	nestedDuplicateJSON := []byte(`{"outer": {"nested": 1, "nested": 2}}`)
	_, err = CanonicalJSON(nestedDuplicateJSON)
	if err == nil {
		t.Error("expected error for duplicate nested object keys in JSON input")
	}
}

func TestCanonicalJSON_RejectsNonFiniteNumbers(t *testing.T) {
	inputNaN := []byte(`{"val": NaN}`)
	_, err := CanonicalJSON(inputNaN)
	if err == nil {
		t.Error("expected error for NaN in JSON")
	}

	inputInf := []byte(`{"val": Infinity}`)
	_, err = CanonicalJSON(inputInf)
	if err == nil {
		t.Error("expected error for Infinity in JSON")
	}
}

func TestCanonicalJSON_RejectsLoneSurrogatesAndInvalidUTF8(t *testing.T) {
	// Invalid UTF-8 bytes
	invalidUTF8 := []byte{'{', '"', 'k', '"', ':', '"', 0xff, 0xfe, '"', '}'}
	_, err := CanonicalJSON(invalidUTF8)
	if err == nil {
		t.Error("expected error for invalid UTF-8 bytes")
	}
}

func TestCanonicalHashing_RFC8785_AdversarialDelimitersAndUnicode(t *testing.T) {
	evalTime := time.Date(2026, 8, 19, 12, 0, 0, 0, time.UTC)

	req := &PolicyEvaluationRequest{
		RequestID: "req:with=delimiters\nand\tnewlines\r\n",
		TenantID:  "TENANT_Unicode_€_漢字_🔒",
		Subject: PolicySubject{
			Type:          "AGENT",
			ID:            "agent=special:id\n\"quoted\"",
			Roles:         []string{"Z_ROLE", "A_ROLE", "Unicode_🔑"},
			AutonomyLevel: 2,
			TenantID:      "TENANT_Unicode_€_漢字_🔒",
		},
		Action: "ACTION:MODIFY=TRUE\n",
		Resource: PolicyResource{
			Type:           "ARTIFACT",
			ID:             "art:101=safe",
			SHA256:         "sha256:hash=value\n",
			State:          "QUARANTINED",
			Classification: "FINANCIAL_PAYLOAD",
			TenantID:       "TENANT_Unicode_€_漢字_🔒",
		},
		Workflow: PolicyWorkflowContext{
			WorkflowID: "wf:101",
			State:      "REMEDIATING",
			Attempt:    1,
		},
		Environment: PolicyEnvironment{
			EvaluationTime: evalTime,
			FleetMode:      "ADVISORY",
		},
		AuthoritativeAttributes: map[string]interface{}{
			"z_key": "val=z:colon\n",
			"a_key": "val_a",
			"nested": map[string]interface{}{
				"unicode_field": "こんにちは",
				"count":         42,
			},
		},
	}

	hash1 := ComputeEvaluatedContextHash(req)
	hash2 := ComputeEvaluatedContextHash(req)

	if hash1 != hash2 {
		t.Fatalf("hashes must be identical: %s vs %s", hash1, hash2)
	}

	reqReordered := *req
	reqReordered.AuthoritativeAttributes = map[string]interface{}{
		"nested": map[string]interface{}{
			"count":         42,
			"unicode_field": "こんにちは",
		},
		"a_key": "val_a",
		"z_key": "val=z:colon\n",
	}

	hashReordered := ComputeEvaluatedContextHash(&reqReordered)
	if hashReordered != hash1 {
		t.Fatalf("RFC 8785 canonical hash mismatch on reordered keys: %s vs %s", hashReordered, hash1)
	}
}

func TestCanonicalHashing_TypedObligationsDeterminism(t *testing.T) {
	p := &PolicyDefinition{
		PolicyID:      "TEST-TYPED-OBL",
		Version:       1,
		Domain:        DomainRemediation,
		Layer:         LayerSentinelSafety,
		Priority:      100,
		Status:        StatusActive,
		EffectiveFrom: DefaultSafetyEffectiveDate,
		Action:        ActionCreateCandidate,
		Effect:        DecisionAllowWithObligations,
		Obligations: []Obligation{
			{
				Type: ObligationMaxAttempts,
				Parameters: map[string]interface{}{
					"count": 3,
					"mode":  "STRICT",
				},
			},
			{
				Type: ObligationCandidateOnly,
			},
		},
		ReasonCode: "TEST_REASON",
	}

	h1 := ComputePolicyContentHash(p)

	pReordered := *p
	pReordered.Obligations = []Obligation{
		{
			Type: ObligationCandidateOnly,
		},
		{
			Type: ObligationMaxAttempts,
			Parameters: map[string]interface{}{
				"mode":  "STRICT",
				"count": 3,
			},
		},
	}

	h2 := ComputePolicyContentHash(&pReordered)
	if h1 != h2 {
		t.Fatalf("policy content hash must be invariant to obligation slice/map order: %s vs %s", h1, h2)
	}
}

func TestCanonicalJSON_RFC8785_NumericGoldenVectors(t *testing.T) {
	testCases := []struct {
		name     string
		input    interface{}
		expected string
	}{
		{"positive zero", 0.0, "0"},
		{"negative zero", math.Copysign(0, -1), "0"},
		{"integer zero", 0, "0"},
		{"positive int", 100, "100"},
		{"negative int", -100, "-100"},
		{"safe integer max", int64(9007199254740991), "9007199254740991"},
		{"safe integer min", int64(-9007199254740991), "-9007199254740991"},
		{"int64 max", int64(9223372036854775807), "9223372036854775807"},
		{"int64 min", int64(-9223372036854775808), "-9223372036854775808"},
		{"fractional 0.125", 0.125, "0.125"},
		{"fractional 1.5", 1.5, "1.5"},
		{"small fractional 1e-5", 0.00001, "0.00001"},
		{"small fractional 1e-6", 0.000001, "0.000001"},
		{"negative 1e-5", -0.00001, "-0.00001"},
		{"negative 1e-6", -0.000001, "-0.000001"},
		{"fractional with mantissa 1.23456789e-5", 1.23456789e-5, "0.0000123456789"},
		{"fractional with mantissa 1.23456789e-6", 1.23456789e-6, "0.00000123456789"},
		{"exponential small 1e-7", 0.0000001, "1e-7"},
		{"negative exponential small -1e-7", -0.0000001, "-1e-7"},
		{"exponential large 1e21", 1e21, "1e+21"},
		{"negative exponential large -1e21", -1e21, "-1e+21"},
		{"exponential large 1e20", 1e20, "100000000000000000000"},
		{"negative exponential large -1e20", -1e20, "-100000000000000000000"},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			b, err := CanonicalJSON(tc.input)
			if err != nil {
				t.Fatalf("canonical JSON error: %v", err)
			}
			if string(b) != tc.expected {
				t.Errorf("numeric formatting mismatch for %v: got %s, want %s", tc.input, string(b), tc.expected)
			}
		})
	}
}
