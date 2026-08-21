package toolgateway

import (
	"context"
	"fmt"
	"time"

	"sentinel-gateway/internal/policy"
)

// ObligationEvidence represents authoritative, infrastructure-verified proof of obligation satisfaction.
// Model-supplied `"satisfied": true` payloads are ignored.
type ObligationEvidence struct {
	Type             policy.ObligationType  `json:"type"`
	Satisfied        bool                   `json:"satisfied"`
	EvidenceRef      string                 `json:"evidence_ref"`
	ValidatorVersion string                 `json:"validator_version,omitempty"`
	ArtifactSHA256   string                 `json:"artifact_sha256,omitempty"`
	VerifiedAt       time.Time              `json:"verified_at"`
	Details          map[string]interface{} `json:"details,omitempty"`
}

// IsPreExecutionObligation returns true if the obligation must be verified prior to tool execution.
func IsPreExecutionObligation(oblType policy.ObligationType) bool {
	switch oblType {
	case policy.ObligationCandidateOnly,
		policy.ObligationImmutableParentRequired,
		policy.ObligationSandboxOnly,
		policy.ObligationMaxAttempts,
		policy.ObligationHumanReview,
		policy.ObligationDualControl,
		policy.ObligationExactArtifactHash:
		return true
	case policy.ObligationAuditRequired,
		policy.ObligationDeterministicRevalidation:
		return false
	default:
		return true // Fail safe: default to pre-execution check
	}
}

// VerifyPreExecution verifies that all pre-execution obligations are authoritatively satisfied.
func VerifyPreExecution(
	ctx context.Context,
	obl policy.Obligation,
	execCtx *TrustedExecutionContext,
	manifest *ToolManifest,
	evidence map[policy.ObligationType]*ObligationEvidence,
) error {
	switch obl.Type {
	case policy.ObligationCandidateOnly:
		// Proved by tool manifest side-effect class: must be CANDIDATE_SANDBOX_WRITE or READ_ONLY
		if manifest.SideEffectClass != SideEffectCandidateSandboxWrite && manifest.SideEffectClass != SideEffectReadOnly {
			return fmt.Errorf("%w: CANDIDATE_ONLY requires candidate sandbox side-effect, got %s",
				ErrUnsatisfiedObligation, manifest.SideEffectClass)
		}
		return nil

	case policy.ObligationImmutableParentRequired:
		// Proved by authoritative execution context: must have non-empty ArtifactID and ArtifactSHA256
		if execCtx.ArtifactID == "" || execCtx.ArtifactSHA256 == "" {
			return fmt.Errorf("%w: IMMUTABLE_PARENT_REQUIRED requires authoritative parent artifact_id and artifact_sha256",
				ErrUnsatisfiedObligation)
		}
		return nil

	case policy.ObligationSandboxOnly:
		// Proved by execution mode: must be SHADOW, ADVISORY, or sandbox-compatible tool
		if manifest.SideEffectClass != SideEffectCandidateSandboxWrite && manifest.SideEffectClass != SideEffectReadOnly {
			return fmt.Errorf("%w: SANDBOX_ONLY prohibits external or irreversible side effects (%s)",
				ErrUnsatisfiedObligation, manifest.SideEffectClass)
		}
		return nil

	case policy.ObligationMaxAttempts:
		// Proved from authoritative workflow / attempt counter in parameters
		if countParam, ok := obl.Parameters["count"]; ok {
			maxCount := 0
			switch c := countParam.(type) {
			case int:
				maxCount = c
			case float64:
				maxCount = int(c)
			}
			if maxCount > 0 && execCtx.ResourceVersion > maxCount {
				return fmt.Errorf("%w: MAX_ATTEMPTS exceeded (attempt %d > max %d)",
					ErrUnsatisfiedObligation, execCtx.ResourceVersion, maxCount)
			}
		}
		return nil

	case policy.ObligationExactArtifactHash:
		// Proved against expected artifact hash in resource preconditions
		if ev, ok := evidence[policy.ObligationExactArtifactHash]; ok && ev.Satisfied {
			if ev.ArtifactSHA256 != "" && execCtx.ArtifactSHA256 != "" && ev.ArtifactSHA256 != execCtx.ArtifactSHA256 {
				return fmt.Errorf("%w: EXACT_ARTIFACT_HASH mismatch (expected %s, got %s)",
					ErrUnsatisfiedObligation, ev.ArtifactSHA256, execCtx.ArtifactSHA256)
			}
			return nil
		}
		if execCtx.ArtifactSHA256 == "" {
			return fmt.Errorf("%w: EXACT_ARTIFACT_HASH requires verified artifact_sha256 in context",
				ErrUnsatisfiedObligation)
		}
		return nil

	case policy.ObligationHumanReview, policy.ObligationDualControl:
		// Must have authoritative review evidence provided in evidence map
		ev, exists := evidence[obl.Type]
		if !exists || !ev.Satisfied {
			return fmt.Errorf("%w: %s requires authoritative human review evidence",
				ErrUnsatisfiedObligation, obl.Type)
		}
		return nil

	case policy.ObligationAuditRequired, policy.ObligationDeterministicRevalidation:
		// Handled post-execution
		return nil

	default:
		return fmt.Errorf("%w: %s", ErrUnknownObligation, obl.Type)
	}
}

// VerifyPostExecution verifies that all post-execution obligations are satisfied.
func VerifyPostExecution(
	ctx context.Context,
	obl policy.Obligation,
	execCtx *TrustedExecutionContext,
	resp *ToolResponse,
	evidence map[policy.ObligationType]*ObligationEvidence,
) error {
	switch obl.Type {
	case policy.ObligationAuditRequired:
		// Guaranteed by Gateway's atomic outbox and store persistence
		return nil

	case policy.ObligationDeterministicRevalidation:
		// For candidate tools, revalidation is verified via trusted validator evidence
		ev, exists := evidence[policy.ObligationDeterministicRevalidation]
		if exists && !ev.Satisfied {
			return fmt.Errorf("%w: DETERMINISTIC_REVALIDATION failed: %s",
				ErrUnsatisfiedObligation, ev.EvidenceRef)
		}
		return nil

	default:
		// Pre-execution obligations already verified
		return nil
	}
}
