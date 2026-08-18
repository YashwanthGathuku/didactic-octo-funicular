# Runbook RB-03: Object Storage Outage & Transient Read Failures

## 1. Failure Scenario
- **Failure Mode**: S3/MinIO/Filesystem storage endpoint is unreachable, returns HTTP 5xx, or experiences extreme latency during artifact writes or worker reads.
- **Affected Components**: `gateway/internal/objectstore`, `validateArtifactHandler` in `gateway/worker.go`.

## 2. Expected State & System Behavior
- **Upload Failures**: If object storage `Put` fails during file ingress, the transaction is rolled back, zero rows are written to `file_instances` or `ingestion_jobs`, and client receives HTTP 500/503.
- **Worker Read Failures**: If worker `Get` fails when retrieving an uploaded artifact:
  - **The artifact is NOT quarantined**. Quarantining a valid file due to a transient infrastructure failure would falsely blame the customer.
  - The job attempt records the error and updates `run_after` with exponential backoff (`run_after = NOW() + backoff`).
  - The job remains in `RETRYABLE` state until storage recovers or `max_attempts` is exhausted.

## 3. Guarantees & Tolerances
- **Data Loss Tolerance**: 0 (zero loss). Stored objects are immutable.
- **Duplicate Tolerance**: Full idempotency.
- **Measured Recovery Time**: **< 900ms** after storage endpoint becomes available again.

## 4. Telemetry & Alerts
- **Prometheus Metric**: `sentinel_dependency_errors_total{dependency="object_store"}` spiking.
- **Prometheus Metric**: `sentinel_dependency_request_duration_seconds{dependency="object_store"}` p95 > 1s.
- **Prometheus Metric**: `sentinel_jobs_state_count{state="retryable"}` increasing.

## 5. Operator Action & Remediation
1. **Check S3 / Storage Service Status**:
   ```bash
   aws --endpoint-url=$S3_ENDPOINT s3 ls s3://$S3_BUCKET/
   ```
2. **Inspect Failed Storage Requests in Gateway Logs**:
   Look for `open artifact %d: ...` or `object store put failed`.
3. **Verify Retry Backlog**:
   ```sql
   SELECT id, tenant_id, file_instance_id, attempt_count, max_attempts, run_after, last_error
   FROM ingestion_jobs
   WHERE state = 'RETRYABLE' AND last_error LIKE '%object store%';
   ```
4. **Drain Retries upon Storage Restoration**:
   Once storage is restored, reset `run_after` to process queued jobs immediately:
   ```sql
   UPDATE ingestion_jobs
   SET run_after = CURRENT_TIMESTAMP
   WHERE state = 'RETRYABLE' AND last_error LIKE '%object store%';
   ```
