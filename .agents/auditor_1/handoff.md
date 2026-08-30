# Forensic Audit Report — SentinelFlow (R1, R2, R3, R4)

**Work Product**: SentinelFlow Implementation (gateway, ai-tier, docs, migrations_postgres, scripts)
**Integrity Mode**: Development Mode (per ORIGINAL_REQUEST.md)
**Verdict**: **CLEAN**

---

## 1. Observation

Direct forensic observations across all audited code, configurations, schemas, and runtime traces:

1. **Managed Agent Tools Gateway (`gateway/managed_agent_tools.go` & `gateway/internal/toolgateway/tools.go`)**:
   - `buildManagedToolGateway` directly instantiates authentic services: `candidateService := candidate.NewService(db, store, engine)` and binds them through `toolgateway.RegisterCandidateTool`.
   - `deriveManagedWorkflowContext` pulls authoritative context (`tenant_id`, `incident_id`, `artifact_sha256`, `row_version`, `policy_bundle_hash`) directly from durable database records (`agent_workflows`). Tenant spoofing is prevented fail-closed.
   - Preconditions (`ExpectedArtifactSHA256`, `ExpectedRowVersion`, `ExpectedWorkflowState`, `ExpectedPolicyBundle`) are verified against live database state, returning HTTP 412 Precondition Failed on stale state.
   - RBAC rules strictly enforce that only `RemediationAgent` can invoke `remediation.candidate.create`. All other 6 agents (`IncidentCommanderAgent`, `DiagnosisAgent`, `PolicySLAAgent`, `MemoryAgent`, `VerifierAgent`, `ReturnRiskAgent`) are blocked with HTTP 403 Forbidden.

2. **Candidate Creation Derivation & Immutability (`gateway/internal/candidate/service.go`)**:
   - `GenerateCandidate` validates attempt bounds (`MAX_ATTEMPTS = 5`), enforces idempotency using SHA256 hashes, checks policy engine obligations, fetches original parent artifact bytes from `ObjectStore`, and computes parent SHA256 before and after derivation to ensure zero in-place mutation.
   - Genuine candidate derivation executes through `GeneratePPDDerivedArtifact` (modifying only the failing entry) and re-validates the resulting NACHA file with `nacha.Validate`.
   - Records are durably written into `file_instances` and `artifact_derivations` within a single database transaction.

3. **Dual-Engine PostgreSQL Migrations (`gateway/migrations_postgres/001_schema_and_rls.sql` to `023_lens_lite.sql`)**:
   - Full 1:1 parity with SQLite migrations 001-023.
   - Genuine PostgreSQL dialect syntax: uses `TIMESTAMPTZ`, `BYTEA`, `BIGINT GENERATED ALWAYS AS IDENTITY`, `RAISE EXCEPTION`, dollar-quoted PL/pgSQL triggers (`lens_investigation_nodes_no_change`), and table-level RLS policies (`ENABLE ROW LEVEL SECURITY`, `FORCE ROW LEVEL SECURITY`, `CREATE POLICY tenant_isolation_*`).

4. **Agent Roster Manifests & Capability Matrix (`ai-tier/contracts/manifests.py`, `docs/registry/agent_registry_v1.json`, `docs/CAPABILITY_MATRIX.yaml`)**:
   - Manifest roster defines the canonical 7 agents.
   - `lens.query` is registered and allowed for `IncidentCommanderAgent` and `ReturnRiskAgent`.
   - `returnrisk.result.get` is registered and allowed for `ReturnRiskAgent`.
   - `docs/registry/agent_registry_v1.json` lists `lens.query` and `returnrisk.result.get` under `registeredCapabilities`.
   - `docs/CAPABILITY_MATRIX.yaml` sets `sentinelflow_lens` to status `TESTED`.

5. **License Hygiene Audit**:
   - `gateway/go.mod` dependencies: `go-chi/chi/v5` (MIT), `golang-jwt/jwt/v5` (MIT), `jackc/pgx/v5` (MIT), `minio/minio-go/v7` (Apache-2.0), `modernc.org/sqlite` (BSD-3-Clause), `johannesboyne/gofakes3` (Apache-2.0), `ProtonMail/go-crypto` (BSD-3-Clause).
   - Zero AGPL-3.0 licensed dependencies found.

6. **Empirical Runtime Validation Results**:
   - `scripts/verify_lens_lite.sh`: **PASSED (7/7 Stages Complete)**
     - Stage 1: Synthetic demo data determinism & provenance (5/5 tests passed).
     - Stage 2: Go Lens semantic compiler & tenant tests (PASSED).
     - Stage 3: Go Lens HTTP compile gate (PASSED).
     - Stage 4: Raw-SQL authority guard (0 violations).
     - Stage 5: Frontend Lens Vitest (14/14 tests passed) + Vite production build (PASSED).
     - Stage 6: Documentation synchronization (`generate_docs.py --check` PASSED).
     - Stage 7: Submission hardening freeze (12/12 stages PASSED).
   - `pytest ai-tier/tests/ -v`: **PASSED (111/111 tests passed)**.
   - `ai-tier/evals/return_runner.py`: **PASSED (20/20 scenarios, 23/23 checks passed, 100%)**.
   - `ai-tier/evals/runner.py`: **PASSED (20/20 scenarios passed, 100%)**.
   - `go test -v -run TestManagedAgentTools ./...`: **PASSED**.
   - `go test -v -run TestPostgres ./...`: **PASSED**.

---

## 2. Logic Chain

1. **Authenticity Analysis**:
   - Scanned all source files in scope for hardcoded outputs, fake responses, and facade stubs. All tools and services (`CandidateService`, `ToolGateway`, `LensService`, `ReturnRiskAgent`) execute genuine computations, hash validations, database operations, and object storage transfers.
   - Tested candidate creation under hostile inputs (tampered parent SHA, wrong workflow state, missing idempotency key, unauthorized caller agent) and confirmed all error paths fail closed with expected HTTP status codes (403, 400, 412, 409).

2. **Schema & Migration Parity**:
   - Automated parser in `migrations_postgres/migrations_test.go` verified all 23 PostgreSQL migration files for correct PostgreSQL syntax, PL/pgSQL triggers, and mandatory Row Level Security policies. All tests passed cleanly.

3. **Autonomy and Roster Compliance**:
   - Verified that agents adhere to Autonomy Level A1 (read-only advisory) except for `RemediationAgent` which only submits candidate proposals to the Gateway for independent policy re-validation and dual-control approval.

---

## 3. Caveats

- In the local testing environment, live Google Cloud Model Armor endpoints return HTTP 403 (`telos-agent` demo project permissions). The codebase correctly detects this and exercises the truthful fail-closed fallback or local deterministic provider.
- Live Google Cloud managed deployment gates (`PASS_LIVE`) are intended for Google Cloud infrastructure with active billing and service credentials; local validation tests the full end-to-end deterministic logic and contract compliance.

---

## 4. Conclusion

**Verdict: CLEAN**
The work products for SentinelFlow (R1, R2, R3, R4) satisfy all forensic integrity criteria. There are no facade implementations, no hardcoded bypasses, no fabricated outputs, and no licensing violations. All dual-engine PostgreSQL migrations and managed tool integrations are genuine and fully verified.

---

## 5. Verification Method

To independently reproduce the forensic verification:
1. `go test -v -run TestManagedAgentTools ./...` in `gateway/`
2. `go test -v -run TestPostgres ./migrations_postgres`
3. `& 'C:\Program Files\Git\bin\bash.exe' -c 'export SENTINEL_FORCE_DETERMINISTIC_MODEL=1; bash scripts/verify_lens_lite.sh'`
4. `python scripts/generate_docs.py --check`
5. `python ai-tier/evals/return_runner.py`
6. `python ai-tier/evals/runner.py`
