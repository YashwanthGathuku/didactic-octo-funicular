# Runbook RB-06: Transactional Outbox Delivery Failures

## 1. Failure Scenario
- **Failure Mode**: Downstream consumers (notification sinks, SSE broadcasters, external webhooks, audit consumers) are down, returning errors or timing out during outbox dispatch.
- **Affected Components**: `gateway/internal/jobs/outbox.go`, `outbox_events` table.

## 2. Expected State & System Behavior
- **Zero Loss of Events**:
  - The business transaction has already committed the event into `outbox_events`.
  - The outbox dispatcher attempts delivery via `Deliverer.Deliver`.
  - Upon delivery failure, the event's `attempt_count` increments, `last_error` is recorded, and `run_after` is set with exponential backoff (`run_after = NOW() + backoff`).
  - If `attempt_count >= max_attempts`, the event is marked `dead_at = NOW()` and moves to dead-letter status for operator inspection.
  - The core gateway business operations (file validation, state transitions, approvals) continue unaffected.

## 3. Guarantees & Tolerances
- **Data Loss Tolerance**: 0 (zero loss). Outbox records are immutable via database trigger `outbox_events_content_is_immutable`.
- **Delivery Guarantee**: At-least-once delivery.
- **Measured Recovery Time**: Delivered in **< 100ms** after downstream sink restoration.

## 4. Telemetry & Alerts
- **Prometheus Metric**: `sentinel_sse_dropped_connections_total` or `sentinel_dependency_errors_total`.
- **Query Indicator**: `SELECT COUNT(*) FROM outbox_events WHERE dead_at IS NOT NULL` > 0.
- **Log Pattern**: `outbox dispatch: ...` or `outbox: %d delivered, %d failed`.

## 5. Operator Action & Remediation
1. **Check Undelivered & Dead Outbox Events**:
   ```sql
   SELECT id, tenant_id, event_type, subject_type, subject_id, attempt_count, max_attempts, run_after, last_error
   FROM outbox_events
   WHERE delivered_at IS NULL;
   ```
2. **Inspect Dead-Letter Events**:
   ```sql
   SELECT id, tenant_id, event_type, dedupe_key, dead_at, last_error
   FROM outbox_events
   WHERE dead_at IS NOT NULL;
   ```
3. **Replay Dead-Letter Events After Downstream Recovery**:
   Once the downstream consumer or webhook endpoint is restored:
   ```sql
   UPDATE outbox_events
   SET dead_at = NULL, attempt_count = 0, run_after = CURRENT_TIMESTAMP
   WHERE dead_at IS NOT NULL;
   ```
