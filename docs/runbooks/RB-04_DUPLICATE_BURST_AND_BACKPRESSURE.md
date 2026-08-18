# Runbook RB-04: Duplicate Ingestion Burst & Tenant Backpressure

## 1. Failure Scenario
- **Failure Mode**: Upstream counterparties or retry scripts transmit bursts of identical files with the same idempotency key, or a single tenant floods the gateway with high file volume.
- **Affected Components**: `gateway/ingest.go`, `tenant_job_quotas` table, `gateway/internal/jobs/queue.go`.

## 2. Expected State & System Behavior
- **Duplicate Ingestion**:
  - The first arriving request creates the file instance and ingestion job.
  - Concurrent duplicate arrivals with identical `Idempotency-Key` or file SHA-256 hash detect the existing record and return **HTTP 202 Accepted** with the original `artifact_id` and `job_id`.
  - Exactly one execution job is enqueued in `ingestion_jobs`.
- **Tenant Concurrency Quotas**:
  - The worker pool claims at most `max_concurrent` jobs per tenant from `tenant_job_quotas` (default 5).
  - Excess jobs remain in `QUEUED` state, preventing a single noisy tenant from starving other counterparties.

## 3. Guarantees & Tolerances
- **Data Loss Tolerance**: 0 (zero loss).
- **Duplicate Tolerance**: Full deduplication; zero duplicate jobs executed.
- **Measured Response Latency**: Replay responses served in **< 10ms**.

## 4. Telemetry & Alerts
- **Prometheus Metric**: `sentinel_backpressure_rejections_total` > 0.
- **Prometheus Metric**: `sentinel_worker_saturation_ratio` > 0.85 sustained.
- **Prometheus Metric**: `sentinel_jobs_state_count{state="queued"}` elevated for specific tenant.

## 5. Operator Action & Remediation
1. **Identify Saturated Tenant**:
   ```sql
   SELECT tenant_id, COUNT(*) as queued_jobs
   FROM ingestion_jobs
   WHERE state = 'QUEUED'
   GROUP BY tenant_id
   ORDER BY queued_jobs DESC;
   ```
2. **Check Quota Configuration**:
   ```sql
   SELECT tenant_id, max_concurrent, updated_at FROM tenant_job_quotas WHERE tenant_id = '<TENANT_ID>';
   ```
3. **Adjust Concurrency Quota (If Approved by Capacity Planning)**:
   ```sql
   INSERT INTO tenant_job_quotas (tenant_id, max_concurrent)
   VALUES ('<TENANT_ID>', 10)
   ON CONFLICT (tenant_id) DO UPDATE SET max_concurrent = EXCLUDED.max_concurrent;
   ```
