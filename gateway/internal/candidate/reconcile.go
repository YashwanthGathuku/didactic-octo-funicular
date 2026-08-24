package candidate

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"errors"
	"fmt"
	"io"

	"sentinel-gateway/internal/domain"
	"sentinel-gateway/internal/objectstore"
	"sentinel-gateway/internal/repository"
)

type ReconciliationOutcome string

const (
	Reconciled             ReconciliationOutcome = "RECONCILED"
	RetrySafe              ReconciliationOutcome = "RETRY_SAFE"
	ReconciliationRequired ReconciliationOutcome = "RECONCILIATION_REQUIRED"
	CorruptionDetected     ReconciliationOutcome = "CORRUPTION_DETECTED"
)

type ReconciliationReport struct {
	Outcome             ReconciliationOutcome `json:"outcome"`
	TenantID            string                `json:"tenant_id"`
	WorkflowID          string                `json:"workflow_id"`
	AttemptNumber       int                   `json:"attempt_number"`
	DerivationID        string                `json:"derivation_id,omitempty"`
	CandidateArtifactID int64                 `json:"candidate_artifact_id,omitempty"`
	CandidateStorageKey string                `json:"candidate_storage_key,omitempty"`
	CandidateSHA256     string                `json:"candidate_sha256,omitempty"`
	ValidationOutcome   string                `json:"validation_outcome,omitempty"`
	Detail              string                `json:"detail"`
}

// ReconcileCandidate audits and reconciles cross-resource state between DB and ObjectStore.
func (s *Service) ReconcileCandidate(
	ctx context.Context,
	scope repository.Scope,
	tenantID, workflowID string,
	attemptNumber int,
	expectedPlanHash string,
	expectedParentSHA string,
) (*ReconciliationReport, error) {
	// 1. Check DB state: Does derivation exist?
	var deriv domain.ArtifactDerivationRecord
	err := s.db.QueryRowContext(ctx, `
		SELECT id, candidate_artifact_id, candidate_sha256, remediation_plan_hash
		FROM artifact_derivations
		WHERE tenant_id = ? AND workflow_id = ? AND attempt_number = ?`,
		tenantID, workflowID, attemptNumber,
	).Scan(&deriv.ID, &deriv.CandidateArtifactID, &deriv.CandidateSHA256, &deriv.RemediationPlanHash)

	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return nil, fmt.Errorf("query derivation: %w", err)
	}

	logicalPayload := fmt.Sprintf("%s:%s:%d:%s:%s", tenantID, workflowID, attemptNumber, expectedParentSHA, expectedPlanHash)
	hLogical := sha256.Sum256([]byte(logicalPayload))
	candidateLogicalID := hex.EncodeToString(hLogical[:])
	candidateKey, errKey := objectstore.DeterministicKey(tenantID, candidateLogicalID)
	if errKey != nil {
		return nil, fmt.Errorf("deterministic key: %w", errKey)
	}

	// 2. Check ObjectStore state
	rc, errStore := s.store.Get(ctx, candidateKey)
	var storeSHA string
	var storeExists bool
	if errStore == nil {
		storeExists = true
		defer rc.Close()
		bytes, errRead := io.ReadAll(rc)
		if errRead == nil {
			h := sha256.Sum256(bytes)
			storeSHA = hex.EncodeToString(h[:])
		}
	} else if !errors.Is(errStore, objectstore.ErrObjectNotFound) && !errors.Is(errStore, objectstore.ErrObjectExists) { // some objectstores return not found
		return nil, fmt.Errorf("store get: %w", errStore)
	}

	report := &ReconciliationReport{
		TenantID:            tenantID,
		WorkflowID:          workflowID,
		AttemptNumber:       attemptNumber,
		CandidateStorageKey: candidateKey,
	}

	// Matrix analysis
	if errors.Is(err, sql.ErrNoRows) {
		// DB missing
		if !storeExists {
			report.Outcome = RetrySafe
			report.Detail = "Neither DB nor ObjectStore contains candidate. Safe to retry."
			return report, nil
		}
		// DB missing, ObjectStore has it -> Window B or C crash
		report.Outcome = ReconciliationRequired
		report.Detail = "ObjectStore contains candidate but DB derivation missing. Requires cleanup or safe retry (object will be overwritten/verified safely)."
		report.CandidateSHA256 = storeSHA
		return report, nil
	}

	// DB exists
	report.DerivationID = deriv.ID
	report.CandidateArtifactID = deriv.CandidateArtifactID
	report.CandidateSHA256 = deriv.CandidateSHA256

	if deriv.RemediationPlanHash != expectedPlanHash {
		report.Outcome = CorruptionDetected
		report.Detail = "DB derivation exists but plan hash mismatch."
		return report, nil
	}

	if !storeExists {
		// DB exists, ObjectStore missing! Corruption.
		report.Outcome = CorruptionDetected
		report.Detail = "DB derivation exists but ObjectStore is missing the file."
		return report, nil
	}

	// Both exist, verify hashes
	if deriv.CandidateSHA256 != storeSHA {
		report.Outcome = CorruptionDetected
		report.Detail = fmt.Sprintf("Hash mismatch: DB %s, Store %s", deriv.CandidateSHA256, storeSHA)
		return report, nil
	}

	report.Outcome = Reconciled
	report.Detail = "DB and ObjectStore are fully synchronized."
	return report, nil
}
