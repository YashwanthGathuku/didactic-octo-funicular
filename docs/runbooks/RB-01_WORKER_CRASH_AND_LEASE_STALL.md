# Runbook RB-01: Worker Crash & Lease Stall Recovery

## 1. Failure Scenario
- **Failure Mode**: Worker process dies unexpectedly (OOM, SIGKILL, host reboot) during artifact validation, or worker stalls on an I/O operation and stops sending heartbeats.
- **Affected Components**: `gateway/internal/jobs/worker.go`, `gateway/internal/jobs/queue.go`, `ingestion_jobs` table.

## 2. Expected State & System Behavior
- The claimed job remains in `LEASED` state until `lease_expires_at` timestamp lapses.
- The heartbeat column `last_heartbeat_at` stops updating.
- Once the lease expires, any healthy worker in the pool reclaims the job via atomic conditional UPDATE (`WHERE state = 'LEASED' AND lease_expires_at < NOW()`).
- The uncommitted partial state of the dead worker is rolled back by database transaction boundaries.
- The new worker attempts validation from the beginning, incrementing `attempt_count`.

## 3. Guarantees & Tolerances
- **Data Loss Tolerance**: 0 (zero loss). Original artifact in object store is immutable.
- **Duplicate Tolerance**: Full idempotency. Duplicate findings are cleared in the validation transaction before insertion (`DELETE FROM validation_findings WHERE tenant_id = ? AND file_instance_id = ?`).
- **Target Recovery Time**: Measured in test environment as **< 60ms** after lease expiry.

## 4. Telemetry & Alerts
- **Prometheus Metric**: `sentinel_jobs_state_count{state="leased"}` elevated for > lease timeout.
- **Prometheus Metric**: `sentinel_worker_saturation_ratio` drops or shows mismatch with active leases.
- **Log Pattern**: `jobs: worker-%s claim failed` or `jobs: heartbeat failed`.

## 5. Operator Action & Remediation
1. **Check Queue State**:
   ```sql
   SELECT id, tenant_id, file_instance_id, state, lease_owner, lease_expires_at, last_heartbeat_at, attempt_count
   FROM ingestion_jobs
   WHERE state = 'LEASED' AND lease_expires_at < CURRENT_TIMESTAMP;
   ```
2. **Inspect Worker Pool Logs**:
   Search for worker panics, OOM killer messages (`dmesg -T | grep -i oom`), or hung goroutines.
3. **Manual Lease Break (Emergency Only)**:
   If a dead worker's lease needs immediate reclamation before natural expiry:
   ```sql
   UPDATE ingestion_jobs
   SET state = 'QUEUED', lease_owner = NULL, lease_expires_at = NULL, run_after = CURRENT_TIMESTAMP
   WHERE id = <JOB_ID> AND state = 'LEASED';
   ```
4. **Verify Health**:
   Verify job transitions to `SUCCEEDED` or `DEAD` with a recorded `terminal_reason`.
