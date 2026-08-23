package returnrisk

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"sentinel-gateway/internal/memory"
	"sentinel-gateway/internal/repository"
)

// EmitVerifiedReturnFact writes a confirmed return risk resolution to M1 operational memory.
func (e *DeterministicRiskEngine) EmitVerifiedReturnFact(
	ctx context.Context,
	scope repository.Scope,
	store *memory.Store,
	result *ReturnRiskResult,
	resolutionAction string,
	verifierRef string,
) (*memory.OperationalMemoryRecord, error) {
	if store == nil {
		return nil, ErrNilMemoryStore
	}
	if result == nil {
		return nil, ErrAssessmentNotFound
	}
	if scope.TenantID() == "" {
		return nil, ErrNilScope
	}

	now := time.Now().UTC()
	memoryID := fmt.Sprintf("mem-ret-%s-%d", result.ReturnEventID, now.UnixNano())

	// 1. Structure Canonical Fact Payload
	payload := ReturnRiskFactPayload{
		AssessmentID:     result.AssessmentID,
		WorkflowID:       result.WorkflowID,
		ReturnEventID:    result.ReturnEventID,
		PartnerRef:       result.TenantID,
		ReturnCode:       result.ReturnCode,
		RiskScore:        result.RiskScore,
		RiskTier:         result.RiskTier,
		PrimaryDrivers:   result.PrimaryDrivers,
		ResolutionAction: resolutionAction,
		VerifierRef:      verifierRef,
		ResolvedAt:       now,
		AssessmentHash:   result.AssessmentHash,
	}

	payloadJSON, err := json.Marshal(payload)
	if err != nil {
		return nil, fmt.Errorf("marshal return fact payload: %w", err)
	}

	// 2. Set TTL based on Standard SLA and Severity
	expiresAt := now.Add(90 * 24 * time.Hour)
	if result.RiskTier == RiskTierHigh || result.RiskTier == RiskTierSevere {
		expiresAt = now.Add(180 * 24 * time.Hour)
	}

	// 3. Assemble Authoritative Operational Memory Record
	record := &memory.OperationalMemoryRecord{
		MemoryID:         memoryID,
		TenantID:         result.TenantID,
		MemoryType:       memory.MemoryTypeOperationalFact,
		SubjectType:      memory.SubjectTypePartner,
		SubjectRef:       result.ReturnCode,
		FactType:         memory.FactTypeVerifiedFailurePattern,
		StructuredValue:  json.RawMessage(payloadJSON),
		ConfidenceSource: memory.ConfidenceSourceDeterministicDerived,
		Classification:   memory.ClassificationInternal,
		Status:           memory.StatusActive,
		ValidFrom:        now,
		ExpiresAt:        &expiresAt,
		CreatedAt:        now,
		CreatedBy:        fmt.Sprintf("engine:%s", EngineVersion),
		SourceRefs: []string{
			fmt.Sprintf("event:%s", result.ReturnEventID),
			fmt.Sprintf("assessment:%s", result.AssessmentID),
		},
		SourceHashes: []string{
			result.AssessmentHash,
		},
		SourceVerificationRefs: []string{
			verifierRef,
			result.WorkflowID,
		},
	}

	// 4. Compute Canonical Memory Hash
	expectedHash, err := memory.ComputeMemoryHash(record)
	if err != nil {
		return nil, fmt.Errorf("failed to compute memory hash: %w", err)
	}
	record.MemoryHash = expectedHash

	// 5. Persist through Memory Store (Evaluates EligibilityGate + DB Transaction)
	if err := store.PersistOperationalFact(ctx, scope, record); err != nil {
		return nil, fmt.Errorf("failed to persist M1 return fact: %w", err)
	}

	return record, nil
}
