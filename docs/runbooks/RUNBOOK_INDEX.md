# SentinelFlow Operational Runbooks Index

## Overview
This directory contains production-ready operational runbooks for SentinelFlow failure modes.
Each runbook details the symptoms, telemetry indicators, recovery mechanisms, data-loss tolerances, and operator actions for incident remediation.

## Core Operational Guarantees
1. **Zero Financial Data Loss**: All financial state transitions, validation findings, and audit records are transactionally committed with zero data loss tolerance.
2. **Idempotent Ingestion & Delivery**: Re-transmissions, burst arrivals, and worker retries are automatically deduplicated via `idempotency_key` and `dedupe_key`.
3. **Deterministic Core Isolation**: Ingestion, validation, and dual-control approval never depend on AI models or non-essential external integrations.

---

## Runbook Catalog

| ID | Runbook | Severity | Failure Mode |
|---|---|---|---|
| [RB-01](file:///docs/runbooks/RB-01_WORKER_CRASH_AND_LEASE_STALL.md) | **Worker Crash & Lease Stall Recovery** | High | Worker termination mid-execution, hung leases, stuck `RUNNING` jobs |
| [RB-02](file:///docs/runbooks/RB-02_DATABASE_OUTAGE_AND_DEGRADATION.md) | **Database Outage & Connection Degradation** | Critical | PostgreSQL/SQLite connection saturation, failover, lock timeouts |
| [RB-03](file:///docs/runbooks/RB-03_OBJECT_STORE_OUTAGE.md) | **Object Storage Outage & Transient Read Failures** | High | S3/MinIO/Filesystem read/write errors, storage backpressure |
| [RB-04](file:///docs/runbooks/RB-04_DUPLICATE_BURST_AND_BACKPRESSURE.md) | **Duplicate Ingestion Burst & Tenant Backpressure** | Medium | Client retry storms, duplicate files, tenant quota exhaustion |
| [RB-05](file:///docs/runbooks/RB-05_POISON_FILE_AND_QUARANTINE.md) | **Malformed / Poison File Handling** | Medium | Truncated files, corrupt records, character encoding errors |
| [RB-06](file:///docs/runbooks/RB-06_OUTBOX_DELIVERY_FAILURE.md) | **Transactional Outbox Delivery Failures** | High | Downstream webhook outage, dead-letter event backlog |
| [RB-07](file:///docs/runbooks/RB-07_AI_PROVIDER_OUTAGE.md) | **AI Incident Analyst Provider Outage** | Low | LLM API rate limits, provider downtime, token budget exhaustion |

---

## Escalation Paths
- **Critical (P1)**: Production database down, audit ledger chain breach, corrupted state. Contact Platform & Data Reliability Team immediately.
- **High (P2)**: Object store degraded, worker pool saturation > 90%, outbox dead letters accumulating. Contact On-call Payments Engineer.
- **Medium (P3)**: Individual tenant quarantined files, single-partner arrival window missing. Handled by Payment Operations team during business hours.
- **Low (P4)**: AI incident analysis service unavailable; deterministic gateway operating normally.
