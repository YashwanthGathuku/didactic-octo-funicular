# Runbook RB-02: Database Outage & Connection Degradation

## 1. Failure Scenario
- **Failure Mode**: Primary database (PostgreSQL / SQLite) becomes unreachable, disk runs out of space, or connection pool is exhausted.
- **Affected Components**: All database-backed components (`gateway`, `internal/jobs`, `internal/ledger`, `internal/schedule`).

## 2. Expected State & System Behavior
- `/api/v1/ready` readiness probe immediately fails closed and returns **HTTP 503 Service Unavailable** with JSON payload:
  `{"ready": false, "checks": {"database": {"status": "UNAVAILABLE", "detail": "..."}}}`.
- Load balancers and ingress controllers stop routing new client traffic to degraded gateway instances.
- Upload acceptance routes return 500/503 without storing orphaned objects in the object store.
- Running workers fail their current transactions and transition jobs to backoff retry (`run_after = NOW() + backoff`).
- No fabricated successes or partial state transitions are committed.

## 3. Guarantees & Tolerances
- **Data Loss Tolerance**: 0 (zero loss). All state changes are ACID-guarded.
- **Duplicate Tolerance**: Full idempotency upon database reconnection.
- **Target Recovery Time**: Immediate readiness recovery upon database ping restoration.

## 4. Telemetry & Alerts
- **Prometheus Metric**: `sentinel_dependency_errors_total{dependency="database"}` increasing.
- **Prometheus Metric**: `sentinel_http_requests_total{route="/api/v1/ready",status="5xx"}` > 0.
- **Prometheus Metric**: `sentinel_dependency_request_duration_seconds{dependency="database"}` p95 > 2s.

## 5. Operator Action & Remediation
1. **Verify Database Connectivity**:
   ```bash
   # Check PostgreSQL cluster health
   pg_isready -h $POSTGRES_HOST -p $POSTGRES_PORT -U $POSTGRES_USER
   ```
2. **Check Connection Pool & Disk Space**:
   ```sql
   SELECT count(*), state FROM pg_stat_activity GROUP BY state;
   ```
3. **Inspect Database Locks**:
   Check for blocking queries or transactions held open longer than statement timeouts:
   ```sql
   SELECT pid, age(clock_timestamp(), query_start), usename, query
   FROM pg_stat_activity
   WHERE state != 'idle' AND query_start < now() - interval '30 seconds';
   ```
4. **Post-Recovery Health Verification**:
   Verify `/api/v1/ready` returns HTTP 200:
   ```bash
   curl -i http://localhost:8080/api/v1/ready
   ```
