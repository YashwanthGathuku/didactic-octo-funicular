package main

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"log"
	"strconv"
	"time"

	"sentinel-gateway/internal/nacha"
	"sentinel-gateway/internal/schedule"
)

// The gateway's adapter onto internal/schedule.
//
// The package is database-driven and has no knowledge of this application's
// configuration or HTTP surface; this file is the only place the two meet.

// schedulerFor builds a Store on the application's database.
//
// Constructed per call rather than cached in a package variable. Prompt 09
// found the cached form holding whichever *sql.DB it saw first, which in a test
// binary is a database since closed.
func schedulerFor(db *sql.DB) (*schedule.Store, error) {
	return schedule.NewStore(db, "sqlite")
}

// startScheduler runs materialization and advancement until ctx is cancelled.
func startScheduler(ctx context.Context, db *sql.DB) error {
	store, err := schedulerFor(db)
	if err != nil {
		return err
	}
	cfg := schedule.DefaultRunConfig()
	go store.Run(ctx, cfg)
	log.Printf("Scheduler started: materializing %d days ahead, advancing every %s",
		cfg.HorizonDays, cfg.Interval)
	return nil
}

// matchArrivalToExpectation attributes a newly stored artifact to whatever it
// was expected to satisfy, inside the ingest transaction.
//
// It never fails an ingest. A file that arrived is stored and validated on its
// own merits whether or not anything was waiting for it, and refusing an upload
// because the scheduler could not decide which expectation it satisfied would
// turn a reporting question into an outage. The outcome is returned so the
// caller can record it; a failure is logged and treated as unmatched.
func matchArrivalToExpectation(
	ctx context.Context, tx *sql.Tx, store *schedule.Store,
	tenantID string, artifactID int64, filename string,
) schedule.MatchResult {
	res, err := store.MatchArrival(ctx, tx, tenantID, artifactID, filename, time.Now().UTC())
	if err != nil {
		log.Printf("ingest: could not match artifact %d for tenant %s against an expectation: %v",
			artifactID, tenantID, err)
		return schedule.MatchResult{
			Outcome: schedule.MatchUnexpected,
			Reason:  "expectation matching failed; the artifact is stored and will be validated",
		}
	}
	return res
}

// ---------------------------------------------------------------------------
// Contract resolution for validation
// ---------------------------------------------------------------------------

// contractForArtifact returns the feed contract terms that govern an artifact.
//
// Until this existed, every artifact was validated against nacha.DefaultContract
// -- one permissive default applied to every tenant and every partner, which
// meant either failing the counterparties authorised to send unbalanced files
// or passing the ones that are not. The terms now come from the contract version
// in force on the occurrence's business date.
//
// An artifact that matched no expectation still validates. It falls back to the
// default contract, and Decision.ContractID stays empty so "no contract was
// applied" remains visible in the decision rather than inferred from silence.
func contractForArtifact(ctx context.Context, tx *sql.Tx, tenantID string, artifactID int64) (nacha.FeedContract, error) {
	var expectationID sql.NullInt64
	err := tx.QueryRowContext(ctx, `
		SELECT expectation_id FROM file_instances WHERE tenant_id = ? AND id = ?`,
		tenantID, artifactID).Scan(&expectationID)
	if err != nil {
		return nacha.DefaultContract, err
	}
	if !expectationID.Valid {
		return nacha.DefaultContract, nil
	}

	var (
		versionID  int64
		feedID     string
		mode       string
		version    int
		contractID int64
	)
	err = tx.QueryRowContext(ctx, `
		SELECT v.id, v.version, v.contract_id, v.feed_id, v.balanced_mode
		FROM expectations e
		JOIN file_contract_versions v
		  ON v.id = e.contract_version_id AND v.tenant_id = e.tenant_id
		WHERE e.tenant_id = ? AND e.id = ?`,
		tenantID, expectationID.Int64).Scan(&versionID, &version, &contractID, &feedID, &mode)
	if errors.Is(err, sql.ErrNoRows) {
		// The occurrence exists but names no version. That is a pre-008 row;
		// it cannot supply terms, so the default applies and says so.
		return nacha.DefaultContract, nil
	}
	if err != nil {
		return nacha.DefaultContract, err
	}

	if feedID == "" {
		feedID = "contract-" + strconv.FormatInt(contractID, 10)
	}
	return nacha.FeedContract{
		ID:      feedID,
		Version: fmt.Sprintf("v%d", version),
		// The single term this contract carries into validation today. A
		// contract that authorises unbalanced files does not require balance;
		// every other contract does. See docs/engineering/NACHA_VALIDATION.md
		// for the terms that remain unmodelled.
		RequireBalanced: schedule.BalancedMode(mode) != schedule.UnbalancedAuthorized,
	}, nil
}
