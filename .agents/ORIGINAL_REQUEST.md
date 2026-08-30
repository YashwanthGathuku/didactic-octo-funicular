# Original User Request

## 2026-08-27T20:04:33Z

Close the 4 identified gaps in **SentinelFlow**, a governed financial-file reliability gateway. SentinelFlow is a production-shaped Go/React/Python fintech platform with deterministic validation, 7-agent ADK orchestration, tenant isolation, and an append-only audit chain. This is targeted engineering hardening across an existing ~40-file codebase, not a greenfield build.

Working directory: C:\Users\Gathu\Projects\fintech
Integrity mode: development

**License hygiene rule**: Do not use AGPL-3.0-licensed dependencies. If useful open-source core logic is found, replicate the approach rather than copying code verbatim. Verify the license of any external dependency before integrating it.

## Existing Codebase Context

The repository is on Git branch `lens-lite-2026-08-25`. Key structure:

- **Go gateway** (`gateway/`) — Chi HTTP router, SQLite/PostgreSQL persistence, deterministic financial processing, policy engine, tool gateway, agent orchestration, ledger, tenant isolation, RBAC
- **Python AI tier** (`ai-tier/`) — 7-agent Google ADK fleet (IncidentCommander, Diagnosis, PolicySLA, Remediation, Verifier, Memory, ReturnRisk) with guarded model boundaries, evals, and operational memory
- **React UI** (`src/`) — governed operations control room with 3-pane Lens investigation workspace
- **SQLite migrations** (`gateway/migrations/001–023`) — full schema through Lens Lite
- **PostgreSQL migrations** (`gateway/migrations_postgres/001`) — baseline schema only, lagging by 22 migrations
- **Verification scripts** (`scripts/verify_lens_lite.sh`, `scripts/verify_submission_freeze.sh`, `scripts/verify_p12_5.sh`)

The project has passed P12.5 (ACH Return Intelligence) and recently added Lens Lite (governed analytics). Four specific gaps remain.

## Requirements

### R1. Lens Lite Verification Gate & Capability Promotion

The Lens Lite subsystem (`gateway/internal/lens/`, `gateway/lens.go`, `src/components/ops/LensWorkspace.tsx`) is implemented but has not passed its 7-stage verification script (`scripts/verify_lens_lite.sh`). The verification gate covers:

1. Synthetic demo provenance and determinism (`python -m unittest tests.test_lens_demo_data -v`)
2. Go Lens semantic compiler + tenant/provenance tests (`go test -race ./internal/lens/...`)
3. Go Lens HTTP/migration compile gate (`go test ./... -run 'TestLens|TestNonExistentSentinelLensCompileGate'`)
4. Raw-SQL authority guard (grep-based check)
5. Frontend Lens tests + production build (`npm test && npm run build`)
6. Documentation synchronization (`python scripts/generate_docs.py --check`)
7. 12-stage submission freeze regression (`bash scripts/verify_submission_freeze.sh`)

Fix any issues that prevent the gate from passing. Upon passing, update `docs/CAPABILITY_MATRIX.yaml` to promote `sentinelflow_lens` from `IMPLEMENTED` to `TESTED`.

### R2. Governed Remediation Candidate Creation on Managed Cloud Ingress

The managed agent ingress endpoint (`POST /internal/agent-tools` in `gateway/managed_agent_tools.go`) explicitly returns HTTP 403 for `remediation.candidate.create` (see lines ~396–403 with the comment `managed_candidate_creation_not_enabled`). The `CandidateService` (in `gateway/internal/candidate/`) must be safely wired into the managed ingress path.

Required preconditions for managed candidate creation:
- Server-side workflow state verification: match `ExpectedArtifactSHA256`, `ExpectedRowVersion`, and `ExpectedWorkflowState` against the durable workflow row
- Tenant ID derived exclusively from the server-side workflow row, never from request headers
- Mandatory idempotency key enforcement preventing duplicate candidate derivations
- Fail-closed quarantine until independent verification passes
- The invariant `DerivedArtifact ≠ MutatedOriginal` must hold — candidates are new artifacts linked to quarantined originals

Reference the existing local orchestrator implementation in `gateway/agent_orchestrator.go` for the pattern.

### R3. Multi-Agent Fleet Manifest & Registry Synchronization

Three authoritative agent roster/capability sources have drifted after Lens Lite was added:

1. **Go roster** (`gateway/internal/auth/agent_identity.go` `FixedCanonicalRoster`) — grants `lens.query` to `IncidentCommanderAgent` and `ReturnRiskAgent` ✓
2. **Python roster** (`ai-tier/contracts/manifests.py` `FIXED_AGENT_ROSTER`) — does **not** include `lens.query` for those agents ✗
3. **GCP registry** (`docs/registry/agent_registry_v1.json`) — does **not** list `lens.query` or `returnrisk.result.get` in `registeredCapabilities` ✗

Synchronize all three sources so every agent's allowed tools are identical across Go, Python, and the registry JSON. The Go roster is the authority.

### R4. Dual-Engine PostgreSQL Schema Parity

SQLite migrations span `001_init_schema.sql` through `023_lens_lite.sql`, covering the complete feature set. PostgreSQL migrations in `gateway/migrations_postgres/` contain only `001_schema_and_rls.sql`. Port migrations for the features added in 011–023 (agent workflows, policy engine, tool gateway, derivations, verifications, operational memory, KMS checkpoints, lens lite) to PostgreSQL dialect with tenant-scoped Row-Level Security (RLS) policies. Ensure the repository interfaces in `gateway/internal/repository/` can function interchangeably across both engines.

## Acceptance Criteria

### Lens Lite Verification
- [ ] `bash scripts/verify_lens_lite.sh` passes all 7 stages
- [ ] `docs/CAPABILITY_MATRIX.yaml` has `sentinelflow_lens` status set to `TESTED`
- [ ] No raw-SQL authority patterns in `gateway/internal/lens/` or `gateway/lens.go`

### Managed Remediation Ingress
- [ ] `POST /internal/agent-tools` with `tool_name: remediation.candidate.create` returns a valid candidate response (not 403) when preconditions are satisfied
- [ ] Tenant ID for candidate creation is derived server-side from the workflow row
- [ ] Duplicate requests with the same idempotency key return the existing candidate
- [ ] A candidate created via managed ingress passes independent verification
- [ ] `go test ./... -race` in `gateway/` passes

### Fleet Manifest Synchronization
- [ ] `lens.query` appears in `allowed_tools` for `IncidentCommanderAgent` and `ReturnRiskAgent` in `ai-tier/contracts/manifests.py`
- [ ] `lens.query` and `returnrisk.result.get` appear in `registeredCapabilities` in `docs/registry/agent_registry_v1.json`
- [ ] `pytest ai-tier/tests/test_adk_introspection.py -v` passes
- [ ] `pytest ai-tier/tests/test_platform_runtime.py -v` passes
- [ ] `python scripts/generate_docs.py --check` passes

### PostgreSQL Schema Parity
- [ ] `gateway/migrations_postgres/` contains migration files covering features through migration 023
- [ ] Each PostgreSQL migration includes tenant-scoped RLS policies
- [ ] PostgreSQL migration files parse as valid PostgreSQL syntax
- [ ] Lens-related tables (`lens_return_events`, `lens_investigations`, `lens_investigation_nodes`) have the same constraints and triggers as their SQLite counterparts
