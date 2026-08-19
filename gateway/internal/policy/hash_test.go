package policy

import (
	"encoding/json"
	"testing"
	"time"
)

func TestCanonicalHashing_RFC8785_AdversarialDelimitersAndUnicode(t *testing.T) {
	evalTime := time.Date(2026, 8, 19, 12, 0, 0, 0, time.UTC)

	// Object with adversarial delimiters and unicode
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

	// Verify that key order variations inside AuthoritativeAttributes produce identical canonical JSON bytes
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

	// Reorder obligations and inner parameters
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

func TestCanonicalJSON_ConformsToJCS(t *testing.T) {
	input := map[string]interface{}{
		"b": 1,
		"a": "hello\nworld",
		"c": []interface{}{3, 2, 1},
		"d": map[string]interface{}{
			"z": true,
			"y": false,
			"x": nil,
		},
	}

	b, err := CanonicalJSON(input)
	if err != nil {
		t.Fatal(err)
	}

	// Verify strict JSON validity
	var decoded map[string]interface{}
	if err := json.Unmarshal(b, &decoded); err != nil {
		t.Fatalf("canonical bytes must be valid JSON: %v", err)
	}

	expectedStr := `{"a":"hello\nworld","b":1,"c":[3,2,1],"d":{"x":null,"y":false,"z":true}}`
	if string(b) != expectedStr {
		t.Errorf("canonical JSON mismatch:\nGot:      %s\nExpected: %s", string(b), expectedStr)
	}
}
