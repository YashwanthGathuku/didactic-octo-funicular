package main

import (
	"bytes"
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"fmt"
	"io"

	"sentinel-gateway/internal/nacha"
	"sentinel-gateway/internal/objectstore"
	"sentinel-gateway/internal/review"
)

// VerificationResult encapsulates the independent deterministic re-validation outcome.
type VerificationResult struct {
	Passed          bool     `json:"passed"`
	ArtifactID      int64    `json:"artifactId"`
	OriginalDigest  string   `json:"originalDigest"`
	ComputedDigest  string   `json:"computedDigest"`
	MismatchReason  string   `json:"mismatchReason,omitempty"`
	RulePackVersion string   `json:"rulePackVersion"`
	FindingsCount   int      `json:"findingsCount"`
	BlockingRuleIDs []string `json:"blockingRuleIds"`
}

// VerifyBeforeApproval runs independent deterministic verification on an immutable artifact
// without relying on or sharing in-memory state with the primary processing worker.
func VerifyBeforeApproval(ctx context.Context, db *sql.DB, store objectstore.ObjectStore, tenantID string, artifactID int64) (*VerificationResult, error) {
	// 1. Fetch artifact metadata and expected validation run digest
	var (
		storagePath    string
		expectedSha256 string
		recordedDigest sql.NullString
	)

	err := db.QueryRowContext(ctx, `
		SELECT f.storage_path, f.sha256_hash, r.findings_digest
		FROM file_instances f
		LEFT JOIN validation_runs r ON r.file_instance_id = f.id AND r.tenant_id = f.tenant_id
		WHERE f.id = ? AND f.tenant_id = ?
		ORDER BY r.started_at DESC LIMIT 1
	`, artifactID, tenantID).Scan(&storagePath, &expectedSha256, &recordedDigest)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, fmt.Errorf("artifact %d not found for tenant %s", artifactID, tenantID)
		}
		return nil, fmt.Errorf("query artifact: %w", err)
	}

	// 2. Read raw artifact from immutable object store
	if store == nil {
		return nil, fmt.Errorf("object store not configured for verification")
	}

	objReader, err := store.Get(ctx, storagePath)
	if err != nil {
		return nil, fmt.Errorf("retrieve artifact from store %q: %w", storagePath, err)
	}
	defer objReader.Close()

	content, err := io.ReadAll(objReader)
	if err != nil {
		return nil, fmt.Errorf("read artifact content: %w", err)
	}

	// 3. Verify content SHA-256 matches expected hash (anti-tamper)
	h := sha256.Sum256(content)
	computedSha256 := hex.EncodeToString(h[:])
	if computedSha256 != expectedSha256 {
		return &VerificationResult{
			Passed:         false,
			ArtifactID:     artifactID,
			OriginalDigest: expectedSha256,
			ComputedDigest: computedSha256,
			MismatchReason: "artifact content SHA-256 does not match recorded database hash",
		}, nil
	}

	// 4. Run clean deterministic NACHA validation
	result, err := nacha.Validate(bytes.NewReader(content))
	if err != nil {
		return nil, fmt.Errorf("re-validate artifact: %w", err)
	}

	// 5. Convert findings to review.Finding to compute identical FindingsDigest
	reviewFindings := make([]review.Finding, len(result.Findings))
	blockingIDs := make([]string, 0)
	for i, f := range result.Findings {
		reviewFindings[i] = review.Finding{
			RuleID:   f.RuleID,
			Severity: string(f.Severity),
		}
		if f.Blocking() {
			blockingIDs = append(blockingIDs, f.RuleID)
		}
	}

	subject := review.Subject{
		Findings: reviewFindings,
	}
	findingsDigest := subject.FindingsDigest()

	res := &VerificationResult{
		ArtifactID:      artifactID,
		OriginalDigest:  recordedDigest.String,
		ComputedDigest:  findingsDigest,
		RulePackVersion: "nacha-2025/1",
		FindingsCount:   len(result.Findings),
		BlockingRuleIDs: blockingIDs,
	}

	if recordedDigest.Valid && recordedDigest.String != "" && recordedDigest.String != findingsDigest {
		res.Passed = false
		res.MismatchReason = fmt.Sprintf("findings digest mismatch: recorded=%s computed=%s", recordedDigest.String, findingsDigest)
		return res, nil
	}

	res.Passed = true
	return res, nil
}
