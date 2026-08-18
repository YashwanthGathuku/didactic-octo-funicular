package main

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"log"
	"time"

	"sentinel-gateway/internal/jobs"
	"sentinel-gateway/internal/ledger"
	"sentinel-gateway/internal/nacha"
	"sentinel-gateway/internal/objectstore"
	"sentinel-gateway/internal/review"
	"sentinel-gateway/internal/telemetry"
)

// The validation worker.
//
// Until this existed, ingest enqueued a job and nothing leased it: every
// uploaded artifact stayed RECEIVED forever. `ingestion_jobs` had been in the
// schema since Prompt 03 with no consumer.

// KindValidateArtifact is the job kind for validating a stored artifact.
const KindValidateArtifact = "VALIDATE_ARTIFACT"

// ledgerVerifyInterval is how often the evidence chain is walked.
//
// Hourly is a starting point, not a measured one. A full walk costs one pass
// over a tenant's records, so the right interval depends on chain length and
// this repository has no production chain to measure. It is stated as a
// default rather than a recommendation.
const ledgerVerifyInterval = time.Hour

// validateArtifactHandler reads an artifact from immutable storage, validates
// it, and records the outcome.
//
// It is idempotent, which at-least-once delivery requires: running it twice on
// the same artifact produces the same state and one set of findings. The
// findings delete-then-insert is what makes that true -- a retry after a
// partial write must not leave two copies of the same finding.
type validateArtifactHandler struct {
	store objectstore.ObjectStore
	queue *jobs.Queue
	// review proposes the release decision the validation rests on. Optional
	// only so a test can exercise validation alone; the running gateway always
	// wires one in.
	review *review.Store
}

// Handle validates one artifact inside the job's transaction.
//
// Everything it writes -- the artifact's new state, its findings, its history
// row and its outbox event -- commits with the job's completion or not at all.
// The object read happens before the transaction's writes and is the only I/O
// outside the database; there is no external network call inside it.
func (h *validateArtifactHandler) Handle(ctx context.Context, tx *sql.Tx, job *jobs.Job) error {
	start := time.Now()
	defer func() {
		metricJobProcessingDuration.Observe(time.Since(start).Seconds(), telemetry.Label{Key: "kind", Value: KindValidateArtifact})
	}()

	if !job.ArtifactID.Valid {
		// A job with no artifact cannot be retried into correctness. Returning
		// an error lets it exhaust its budget and reach DEAD, which is right:
		// something enqueued work that does not describe anything.
		return fmt.Errorf("job %d names no artifact", job.ID)
	}
	artifactID := job.ArtifactID.Int64

	var objectKey sql.NullString
	var state string
	err := tx.QueryRowContext(ctx, `
		SELECT object_key, status FROM file_instances
		WHERE tenant_id = ? AND id = ?`, job.TenantID, artifactID).Scan(&objectKey, &state)
	if err != nil {
		return fmt.Errorf("read artifact %d: %w", artifactID, err)
	}

	// Already validated by an earlier attempt whose completion did not land.
	// Succeeding is correct: the work is done, and redoing it would produce a
	// second set of findings for one artifact.
	if state == "VALIDATED" || state == "QUARANTINED" {
		return nil
	}
	if !objectKey.Valid || objectKey.String == "" {
		return fmt.Errorf("artifact %d has no stored object", artifactID)
	}

	getStart := time.Now()
	body, err := h.store.Get(ctx, objectKey.String)
	RecordDependencyOperation("object_store", "get", time.Since(getStart).Seconds(), err)
	if err != nil {
		// A storage failure is transient in a way a malformed file is not, so
		// this retries rather than quarantining. Quarantining an artifact
		// because the object store was briefly unavailable would blame the file
		// for an outage.
		return fmt.Errorf("open artifact %d: %w", artifactID, err)
	}
	defer body.Close()

	result, err := nacha.Validate(body)
	if err != nil {
		return fmt.Errorf("read artifact %d during validation: %w", artifactID, err)
	}

	// The terms come from the contract version in force on the business date of
	// the expectation this artifact satisfied. Prompt 07 applied one permissive
	// default to every tenant and partner, which meant either failing the
	// counterparties authorised to send unbalanced files or passing the ones
	// that are not. An artifact matching no expectation still validates, under
	// the default, and the decision records that no contract was applied.
	contract, err := contractForArtifact(ctx, tx, job.TenantID, artifactID)
	if err != nil {
		return fmt.Errorf("resolve the contract governing artifact %d: %w", artifactID, err)
	}
	decision := nacha.Decide(result, contract)

	status := "VALIDATED"
	if decision.Quarantined() {
		status = "QUARANTINED"
	}

	// Findings are replaced rather than appended, so a retry does not duplicate
	// them. The append-only trigger from migration 005 forbids UPDATE, not
	// DELETE, precisely so a re-validation can restate its findings.
	if _, err := tx.ExecContext(ctx, `
		DELETE FROM validation_findings WHERE tenant_id = ? AND file_instance_id = ?`,
		job.TenantID, artifactID); err != nil {
		return err
	}

	now := time.Now().UTC().Format(time.RFC3339)
	for _, f := range result.Findings {
		if _, err := tx.ExecContext(ctx, `
			INSERT INTO validation_findings
				(tenant_id, file_instance_id, code, rule_version, provenance, description,
				 severity, line_number, byte_offset, field_start, field_end,
				 evidence_redacted, expected_value, actual_value, created_at)
			VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
			job.TenantID, artifactID, f.RuleID, f.RuleVersion, string(f.Provenance), f.Description,
			string(f.Severity), f.RecordNumber, f.ByteOffset, f.FieldStart, f.FieldEnd,
			f.Evidence, f.Expected, f.Actual, now); err != nil {
			return err
		}
	}

	if _, err := tx.ExecContext(ctx, `
		UPDATE file_instances SET status = ?, updated_at = ?, row_version = row_version + 1
		WHERE tenant_id = ? AND id = ?`, status, now, job.TenantID, artifactID); err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, `
		INSERT INTO status_history (tenant_id, object_type, object_id, from_state, to_state, actor_id, reason)
		VALUES (?, 'artifact', ?, ?, ?, ?, ?)`,
		job.TenantID, artifactID, state, status,
		"system:validation-worker", decision.Summary()); err != nil {
		return err
	}

	// The release decision is proposed in this same transaction. An artifact
	// that reached VALIDATED with no decision would be one nobody could
	// release without a second mechanism inventing the decision after the
	// fact -- which is how a release ends up resting on a validation run
	// nobody can reconstruct.
	if h.review != nil {
		var sha string
		if err := tx.QueryRowContext(ctx,
			`SELECT sha256_hash FROM file_instances WHERE tenant_id = ? AND id = ?`,
			job.TenantID, artifactID).Scan(&sha); err != nil {
			return err
		}
		policy, err := h.review.Policy(ctx, job.TenantID)
		if err != nil {
			return err
		}
		if err := proposeRelease(ctx, tx, h.review, job.TenantID, artifactID, sha,
			result, decision, policy); err != nil {
			return fmt.Errorf("propose a release decision for artifact %d: %w", artifactID, err)
		}
	}

	var sizeBytes int64
	_ = tx.QueryRowContext(ctx, `SELECT size_bytes FROM file_instances WHERE tenant_id = ? AND id = ?`, job.TenantID, artifactID).Scan(&sizeBytes)
	RecordPipelineCompletion(status, sizeBytes, result.RecordsParsed, time.Since(start).Seconds())

	// The event goes to the outbox in this same transaction. Publishing it
	// after the commit would lose it on exactly the crash it needs to survive.
	//
	// The payload carries the decision and counts, never a record: findings are
	// already redacted by internal/nacha, and none is included here anyway.
	return h.queue.PublishTx(ctx, tx, jobs.OutboxEvent{
		TenantID:    job.TenantID,
		EventType:   "ARTIFACT_" + status,
		SubjectType: "artifact",
		SubjectID:   artifactID,
		// Keyed by artifact and outcome, so a retry that reaches this point
		// again records one event rather than two.
		DedupeKey: fmt.Sprintf("artifact-%d-%s", artifactID, status),
		Payload: map[string]any{
			"artifactId":        artifactID,
			"status":            status,
			"policyVersion":     decision.PolicyVersion,
			"contractId":        decision.ContractID,
			"recordsParsed":     result.RecordsParsed,
			"entriesParsed":     result.EntriesParsed,
			"findingCount":      len(result.Findings),
			"blockingRuleIds":   decision.BlockingRuleIDs,
			"notCheckedRuleIds": decision.NotCheckedRuleIDs,
		},
	})
}

// auditLogDeliverer writes outbox events into the append-only audit ledger.
//
// It is the default consumer, and it is deliberately local: the first
// deliverer must not be a network call, or the dispatcher's failure modes would
// be untestable without a remote service. SSE broadcast and external
// notification attach as additional deliverers when those surfaces exist.
type auditLogDeliverer struct {
	db *sql.DB
}

func (d *auditLogDeliverer) Deliver(ctx context.Context, ev jobs.PendingEvent) error {
	var payload map[string]any
	if err := ev.Payload.Decode(&payload); err != nil {
		return fmt.Errorf("decode event %d: %w", ev.ID, err)
	}
	payload["outboxEventId"] = ev.ID
	payload["dedupeKey"] = ev.DedupeKey

	_, err := AppendAuditEvent(d.db, ev.TenantID, ev.EventType, "system:outbox-dispatcher", payload)
	return err
}

// startWorkers builds and starts the job system.
//
// It returns a stop function so the caller controls shutdown ordering: the
// HTTP server stops accepting first, then the pool drains, so no request is
// answered by a process that can no longer do the work it promises.
func startWorkers(ctx context.Context, db *sql.DB, cfg *Config) (stop func(), err error) {
	// The driver is named explicitly. The claim strategy differs per database
	// and a wrong guess produces a queue that appears to work and races.
	queue, err := jobs.New(db, "sqlite", jobs.WithLeaseDuration(60*time.Second))
	if err != nil {
		return nil, err
	}

	poolCfg := jobs.DefaultPoolConfig()
	pool, err := jobs.NewPool(queue, poolCfg)
	if err != nil {
		return nil, err
	}

	if cfg.ObjectStore == nil {
		// Without artifact storage there is nothing to validate. Refusing to
		// start the pool is better than running one whose every job fails.
		//
		// The scheduler still runs. Detecting a file that never arrived needs
		// no object store -- the whole point is that no object exists -- and a
		// deployment that cannot accept files is exactly the one where a
		// partner's delivery is most likely to go missing.
		log.Println("Job workers not started: artifact storage is not configured, so there is nothing to validate.")
		schedCtx, cancel := context.WithCancel(ctx)
		if err := startScheduler(schedCtx, db, queue); err != nil {
			cancel()
			return nil, err
		}
		return cancel, nil
	}

	reviews, err := reviewStoreFor(db)
	if err != nil {
		return nil, err
	}
	reviews.SetEventSink(&ledgerReleaseSink{db: db, queue: queue})

	pool.Register(KindValidateArtifact, &validateArtifactHandler{
		store: cfg.ObjectStore, queue: queue, review: reviews,
	})

	dispatcher := jobs.NewDispatcher(queue, &auditLogDeliverer{db: db}, time.Second, 50)

	// The evidence chain is verified periodically and the result is recorded in
	// the chain itself. A chain nobody checks is a chain that is broken for
	// however long it takes someone to notice.
	evidence, err := ledger.New(db, "sqlite")
	if err != nil {
		return nil, err
	}

	runCtx, cancel := context.WithCancel(ctx)
	pool.Start(runCtx)
	go dispatcher.Run(runCtx)
	go evidence.RunVerifier(runCtx, ledgerVerifyInterval)

	// Periodically refresh worker saturation and database job state metrics.
	go func() {
		ticker := time.NewTicker(2 * time.Second)
		defer ticker.Stop()
		for {
			select {
			case <-runCtx.Done():
				return
			case <-ticker.C:
				metricWorkerSaturation.Set(pool.SaturationRatio(), telemetry.Label{Key: "pool", Value: "default"})
				RefreshDatabaseJobGauges(db)
			}
		}
	}()

	// The scheduler materializes future expectations and ages them. It runs in
	// the same process as the workers because both are background work with the
	// same lifecycle, and because a missing-file alert is worthless if the
	// component that produces it can be deployed separately and forgotten.
	if err := startScheduler(runCtx, db, queue); err != nil {
		cancel()
		return nil, err
	}

	log.Printf("Job workers started: %d workers, %d concurrent per tenant, %s leases",
		poolCfg.Workers, poolCfg.MaxPerTenant, 60*time.Second)
	log.Printf("Evidence chain verification every %s. %s", ledgerVerifyInterval, ledger.AnchorGapStatement)

	return func() {
		// Stop leasing and drain first, then cancel: cancelling first would
		// abort in-flight handlers and leave their leases to expire.
		if err := pool.Stop(30 * time.Second); err != nil {
			log.Printf("Worker shutdown: %v", err)
		}
		// One last dispatch pass so events produced by the final jobs are
		// delivered rather than waiting for the next process.
		if delivered, failed, derr := dispatcher.DispatchOnce(context.WithoutCancel(ctx)); derr != nil {
			log.Printf("Final outbox dispatch: %v", derr)
		} else if delivered > 0 || failed > 0 {
			log.Printf("Final outbox dispatch: %d delivered, %d failed", delivered, failed)
		}
		cancel()
	}, nil
}

// ensure encoding/json stays referenced by this file's payload construction.
var _ = json.Marshal
