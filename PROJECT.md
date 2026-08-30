# Project: SentinelFlow Hardening & Parity

## Architecture
- **Go Gateway** (`gateway/`): Chi HTTP router, SQLite/PostgreSQL dual-engine persistence, deterministic financial processing, policy engine, tool gateway, agent orchestration, ledger, tenant isolation, RBAC, Lens semantic compiler, remediation candidate service.
- **Python AI Tier** (`ai-tier/`): 7-agent Google ADK fleet (IncidentCommander, Diagnosis, PolicySLA, Remediation, Verifier, Memory, ReturnRisk), guarded boundaries, manifests.
- **React UI** (`src/`): Governed operations control room, Lens workspace.
- **Database Schema**: Dual-engine parity across SQLite (`gateway/migrations/`) and PostgreSQL (`gateway/migrations_postgres/`) with tenant-scoped RLS policies.

## Feature Inventory
| # | Feature | Description | Milestone | Source |
|---|---------|-------------|-----------|--------|
| 1 | R1: Lens Lite Verification & Promotion | Verify 7-stage gate `scripts/verify_lens_lite.sh`, promote `sentinelflow_lens` in `docs/CAPABILITY_MATRIX.yaml` | M1 | ORIGINAL_REQUEST §R1 |
| 2 | R3: Manifest & Registry Synchronization | Synchronize `ai-tier/contracts/manifests.py`, `docs/registry/agent_registry_v1.json`, pass pytest & doc generator checks | M1 | ORIGINAL_REQUEST §R3 |
| 3 | R2: Governed Remediation Candidate Creation | Wire `CandidateService` into `POST /internal/agent-tools` (`gateway/managed_agent_tools.go`), server-side workflow verification, tenant derivation from workflow row, idempotency, quarantine, invariant enforcement | M2 | ORIGINAL_REQUEST §R2 |
| 4 | R4: PostgreSQL Schema Parity | Port SQLite migrations 002–023 to PostgreSQL dialect in `gateway/migrations_postgres/` with tenant-scoped RLS policies, repository interface parity, valid PostgreSQL syntax | M3 | ORIGINAL_REQUEST §R4 |
| 5 | Comprehensive Quality & Audit Gate | Run end-to-end verification, `go test -race ./...`, full pytest suite, `scripts/verify_lens_lite.sh`, adversarial challenge, forensic integrity audit | M4 | ORIGINAL_REQUEST Acceptance Criteria |

## Milestones
| # | Name | Scope | Dependencies | Status |
|---|------|-------|-------------|--------|
| M0 | Survey & Exploration | 3 parallel explorers to investigate R1/R3, R2, and R4 | none | DONE |
| M1 | R1 & R3 Sync & Promotion | Lens lite verification stage checks, manifests sync, doc checks, capability matrix | M0 | DONE |
| M2 | R2 Governed Remediation Ingress | Implement & test CandidateService wiring in managed ingress with state checks & tests | M0 | IN_PROGRESS |
| M3 | R4 PostgreSQL Migrations & Parity | Implement & verify PostgreSQL migrations 002-023 with RLS and syntax checks | M0 | DONE |
| M4 | Final Verification & Audit Gate | Full test suites, Reviewers, Challengers, and Forensic Auditor | M1, M2, M3 | PLANNED |

## Code Layout
- `gateway/`
  - `managed_agent_tools.go`: Managed cloud ingress handler (`POST /internal/agent-tools`)
  - `agent_orchestrator.go`: Reference orchestrator pattern
  - `internal/candidate/`: Candidate service, quarantine, derivation, invariant checks
  - `internal/repository/`: DB repository interfaces
  - `migrations/`: SQLite migrations (001-023)
  - `migrations_postgres/`: PostgreSQL migrations (001-023 with RLS)
- `ai-tier/`
  - `contracts/manifests.py`: Agent fleet roster and allowed tools
  - `tests/`: Introspection and platform runtime tests
- `docs/`
  - `CAPABILITY_MATRIX.yaml`: Feature promotion status
  - `registry/agent_registry_v1.json`: GCP agent capability registry
