package verification

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"time"

	"sentinel-gateway/internal/policy"
)

const (
	DefaultValidatorVersion = "1.0.0"
	DefaultRulepackID       = "nacha-2025/1"
)

// ComputeRulepackHash returns a deterministic SHA-256 digest for the NACHA validator ruleset version.
func ComputeRulepackHash() string {
	h := sha256.Sum256([]byte(DefaultRulepackID))
	return hex.EncodeToString(h[:])
}

// ComputeDerivationHash computes the canonical derivation hash for an artifact derivation record.
func ComputeDerivationHash(workflowID string, attemptNumber int, parentSHA256 string, candidateArtifactID int64, candidateSHA256 string, planHash string) string {
	payload := fmt.Sprintf("%s:%d:%s:%d:%s:%s", workflowID, attemptNumber, parentSHA256, candidateArtifactID, candidateSHA256, planHash)
	h := sha256.Sum256([]byte(payload))
	return hex.EncodeToString(h[:])
}

// ComputeVerificationHash calculates the RFC 8785 canonical JSON hash of the verification result.
func ComputeVerificationHash(res *VerificationResult) (string, error) {
	if res == nil {
		return "", errors.New("verification result is nil")
	}

	createdAtStr := ""
	if !res.CreatedAt.IsZero() {
		createdAtStr = res.CreatedAt.UTC().Format(time.RFC3339Nano)
	}

	checksPayload := make([]map[string]interface{}, 0, len(res.Checks))
	for _, c := range res.Checks {
		checksPayload = append(checksPayload, map[string]interface{}{
			"type":     string(c.Type),
			"passed":   c.Passed,
			"message":  c.Message,
			"expected": c.Expected,
			"actual":   c.Actual,
		})
	}

	payload := map[string]interface{}{
		"verification_id":       res.VerificationID,
		"tenant_id":             res.TenantID,
		"workflow_id":           res.WorkflowID,
		"candidate_artifact_id": res.CandidateArtifactID,
		"candidate_sha256":      res.CandidateSHA256,
		"parent_artifact_id":    res.ParentArtifactID,
		"parent_sha256":         res.ParentSHA256,
		"derivation_id":         res.DerivationID,
		"derivation_hash":       res.DerivationHash,
		"remediation_plan_hash": res.RemediationPlanHash,
		"p07_validation_run_id": res.P07ValidationRunID,
		"p08_validation_run_id": res.P08ValidationRunID,
		"validator_version":     res.ValidatorVersion,
		"rulepack_hash":         res.RulepackHash,
		"policy_bundle_hash":    res.PolicyBundleHash,
		"checks":                checksPayload,
		"deterministic_outcome": string(res.DeterministicOutcome),
		"created_at":            createdAtStr,
	}

	canonicalBytes, err := policy.CanonicalJSON(payload)
	if err != nil {
		return "", fmt.Errorf("canonicalize verification result: %w", err)
	}

	h := sha256.Sum256(canonicalBytes)
	return hex.EncodeToString(h[:]), nil
}
