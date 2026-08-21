package verification

import (
	"encoding/json"
	"testing"
	"time"
)

func FuzzVerificationResultDecoding(f *testing.F) {
	sample := VerificationResult{
		VerificationID:       "verif-wf-1-1",
		TenantID:             "tenant-1",
		WorkflowID:           "wf-1",
		CandidateArtifactID:  100,
		CandidateSHA256:      "cand-sha-256",
		ParentArtifactID:     10,
		ParentSHA256:         "parent-sha-256",
		DerivationID:         "deriv-1",
		DerivationHash:       "deriv-hash-1",
		RemediationPlanHash:  "plan-hash-1",
		P07ValidationRunID:   "vrun-p07-1",
		P08ValidationRunID:   "vrun-p08-1",
		ValidatorVersion:     "1.0.0",
		RulepackHash:         "rulepack-hash-1",
		PolicyBundleHash:     "policy-bundle-hash-1",
		DeterministicOutcome: OutcomePass,
		VerificationHash:     "verif-hash-1",
		CreatedAt:            time.Now().UTC(),
		Checks: []VerificationCheck{
			{
				Type:     CheckParentHashMatch,
				Passed:   true,
				Message:  "parent matches",
				Expected: "parent-sha-256",
				Actual:   "parent-sha-256",
			},
			{
				Type:     CheckValidatorPass,
				Passed:   true,
				Message:  "validator passed",
				Expected: "VALIDATION_PASSED",
				Actual:   "VALIDATION_PASSED",
			},
		},
	}
	raw, _ := json.Marshal(sample)
	f.Add(raw)
	f.Add([]byte(`{"verification_id": "v1", "deterministic_outcome": "CORRUPTION_DETECTED"}`))
	f.Add([]byte(`{"invalid_json": `))
	f.Add([]byte(`{}`))
	f.Add([]byte(``))

	f.Fuzz(func(t *testing.T, data []byte) {
		var res VerificationResult
		_ = json.Unmarshal(data, &res)

		var check VerificationCheck
		_ = json.Unmarshal(data, &check)

		var req VerificationRequest
		_ = json.Unmarshal(data, &req)
	})
}

func FuzzComputeVerificationHash(f *testing.F) {
	f.Add("v-1", "tenant-1", "wf-1", int64(10), "cand-sha", int64(1), "parent-sha", "deriv-1", "d-hash", "p-hash", "PASS")
	f.Add("", "", "", int64(0), "", int64(0), "", "", "", "", "CORRUPTION_DETECTED")
	f.Add("long-id-1234567890", "tenant-alpha", "workflow-beta", int64(999999), "aaaaaaaaaaaaaaaa", int64(12345), "bbbbbbbbbbbbbbbb", "deriv-xyz", "h1", "h2", "STALE")

	f.Fuzz(func(t *testing.T, verifID, tenantID, wfID string, candID int64, candSHA string, parentID int64, parentSHA string, derivID, derivHash, planHash, outcome string) {
		res := &VerificationResult{
			VerificationID:       verifID,
			TenantID:             tenantID,
			WorkflowID:           wfID,
			CandidateArtifactID:  candID,
			CandidateSHA256:      candSHA,
			ParentArtifactID:     parentID,
			ParentSHA256:         parentSHA,
			DerivationID:         derivID,
			DerivationHash:       derivHash,
			RemediationPlanHash:  planHash,
			P07ValidationRunID:   "vrun-p07",
			P08ValidationRunID:   "vrun-p08",
			ValidatorVersion:     DefaultValidatorVersion,
			RulepackHash:         ComputeRulepackHash(),
			PolicyBundleHash:     "policy-bundle-digest",
			DeterministicOutcome: VerificationOutcome(outcome),
			CreatedAt:            time.Unix(1700000000, 0).UTC(),
			Checks: []VerificationCheck{
				{
					Type:     CheckParentHashMatch,
					Passed:   true,
					Message:  "parent hash valid",
					Expected: parentSHA,
					Actual:   parentSHA,
				},
				{
					Type:     CheckCandidateHashMatch,
					Passed:   true,
					Message:  "cand hash valid",
					Expected: candSHA,
					Actual:   candSHA,
				},
			},
		}

		h1, err1 := ComputeVerificationHash(res)
		if err1 != nil {
			t.Fatalf("ComputeVerificationHash failed: %v", err1)
		}
		if h1 == "" {
			t.Fatalf("ComputeVerificationHash returned empty hash")
		}

		// Verify determinism
		h2, err2 := ComputeVerificationHash(res)
		if err2 != nil || h1 != h2 {
			t.Fatalf("non-deterministic verification hash: %s != %s", h1, h2)
		}
	})
}

func FuzzComputeDerivationHash(f *testing.F) {
	f.Add("wf-1", 1, "parent-sha", int64(10), "cand-sha", "plan-hash")
	f.Add("", 0, "", int64(0), "", "")
	f.Add("wf-complex-999", 3, "0123456789abcdef", int64(987654321), "fedcba9876543210", "plan-xyz")

	f.Fuzz(func(t *testing.T, wfID string, attempt int, parentSHA string, candID int64, candSHA, planHash string) {
		h1 := ComputeDerivationHash(wfID, attempt, parentSHA, candID, candSHA, planHash)
		h2 := ComputeDerivationHash(wfID, attempt, parentSHA, candID, candSHA, planHash)
		if h1 == "" || h1 != h2 {
			t.Fatalf("ComputeDerivationHash must be non-empty and deterministic: got %s vs %s", h1, h2)
		}
	})
}
