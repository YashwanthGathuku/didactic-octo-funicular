# Runbook RB-05: Malformed / Poison File Handling

## 1. Failure Scenario
- **Failure Mode**: Client uploads a corrupted, malformed, non-ASCII, or adversarial NACHA file (e.g. truncated control records, invalid routing numbers, mismatched batch totals).
- **Affected Components**: `gateway/internal/nacha/validate.go`, `gateway/internal/nacha/policy.go`, `validation_findings` table.

## 2. Expected State & System Behavior
- **Fail-Closed Isolation**:
  - The streaming parser scans the file and constructs deterministic typed findings.
  - Policy decision engine (`nacha.Decide`) categorizes all blocking violations into `OutcomeQuarantined`.
  - The file instance status transitions to `QUARANTINED`.
  - The immutable raw payload remains stored in the object store for audit and repair workflows.
- **Worker Immunity**:
  - The worker records findings, writes an append-only audit event, commits the transaction, and marks the job `SUCCEEDED` (since validation evaluation succeeded in reaching a decision).
  - The worker is never poisoned or halted; it immediately proceeds to validate the next file in the queue.

## 3. Guarantees & Tolerances
- **Data Loss Tolerance**: 0 (zero loss). Original raw bytes preserved exactly as received.
- **Privacy Guarantee**: All routing and account numbers in `validation_findings` are redacted before persistence.
- **Safety Invariant**: Quarantined files can NEVER be released without explicit dual-control review or derived repair.

## 4. Telemetry & Alerts
- **Prometheus Metric**: `sentinel_pipeline_files_total{status="quarantined"}` increments.
- **Prometheus Metric**: `sentinel_active_incidents` increments.
- **Log Pattern**: `DECISION: QUARANTINED under release-policy/1.0.0`.

## 5. Operator Action & Remediation
1. **Retrieve Redacted Findings**:
   ```bash
   curl -H "Authorization: Bearer $TOKEN" http://localhost:8080/api/v1/artifacts/<ARTIFACT_ID>/findings
   ```
2. **Review Quarantine Reasons in Database**:
   ```sql
   SELECT code, description, severity, line_number, evidence_redacted, expected_value, actual_value
   FROM validation_findings
   WHERE file_instance_id = <ARTIFACT_ID>;
   ```
3. **Notify Originating Counterparty**:
   Notify counterparty with rule code (e.g., `NACHA.MATH.BATCH_ENTRY_HASH`) and expected vs actual values.
4. **Repair / Derivation Workflow (If Authorized)**:
   If an authorized operator repairs the file, a new derived artifact with a distinct SHA-256 is created and submitted through the gateway. The original remains immutable.
