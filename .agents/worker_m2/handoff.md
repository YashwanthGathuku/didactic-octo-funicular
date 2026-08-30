# Handoff Report — Milestone 2 (R2): Governed Remediation Candidate Creation on Managed Cloud Ingress

## 1. Observation
- **Wired CandidateService into Managed Tool Gateway**:
  - In `gateway/managed_agent_tools.go`, updated `buildManagedToolGateway(db *sql.DB, store objectstore.ObjectStore)` to instantiate `candidate.NewService(db, store, engine)` with default deterministic policy engine and register it via `toolgateway.RegisterCandidateTool(reg, candService)`.
  - Removed HTTP 403 `managed_candidate_creation_not_enabled` block, enabling live execution of `remediation.candidate.create` on `/api/v1/internal/agent-tools` and `/internal/agent-tools`.
- **Precondition & Context Verification**:
  - `managedAgentToolRequest` updated with precondition fields (`expected_artifact_sha256`, `expected_row_version`, `expected_workflow_state`, `expected_policy_bundle`).
  - `deriveManagedWorkflowContext` queries `tenant_id`, `incident_id`, `artifact_id`, `artifact_sha256`, `state`, `row_version`, `started_at`, `policy_bundle_hash` from the durable `agent_workflows` row.
  - Server-side workflow row is strictly authoritative: if header `X-Sentinel-Tenant` is passed and mismatches workflow row tenant, HTTP 403 `tenant_context_mismatch` is returned. Payload `tenant_id` is ignored/overridden.
  - `toolgateway.VerifyResourcePreconditions` validates `expected_artifact_sha256`, `expected_row_version`, `expected_workflow_state`, `expected_policy_bundle`. Mismatches return HTTP 412 `StatusPreconditionFailed`.
- **Execution Context & Defaults Injection**:
  - In `gateway/internal/toolgateway/tools.go` (`handlerRemediationCreate`), default metadata from `execCtx` is injected into `CandidateCreationRequest` (`TenantID`, `WorkflowID`, `ParentArtifactID`, `IncidentID`, `ExpectedParentSHA256`, `AttemptNumber`, `AgentName`, `AgentVersion`, `PolicyDecisionHash`, `ToolManifestHash`, `Confidence`).
  - Plumbed `store objectstore.ObjectStore` into `registerAgentRoutes` and `registerManagedAgentToolRoute` in `gateway/agents.go` and `gateway/main.go`.
  - Set `PolicyBundleHash: currentPolicyBundleHash` on `execCtx` in `gateway/agent_orchestrator.go`.
- **Quarantine & Immutability Compliance**:
  - Candidate files are created with `file_instances.status = 'CANDIDATE'`.
  - Added `'CANDIDATE'` to `file_instances` status CHECK constraint in `gateway/migrations/002_tenancy_and_state.sql`.
  - Fixed `GetIncident` in `gateway/managed_agent_tools.go` to handle nullable `expectation_id`.
  - Parent artifact bytes in `ObjectStore` are proven strictly immutable before and after candidate creation.
- **Comprehensive Unit & Integration Test Suite**:
  - Written in `gateway/managed_agent_tools_test.go`:
    1. `TestManagedAgentTools_CandidateCreate_SuccessAndIndependentVerification`
    2. `TestManagedAgentTools_TenantSpoofingPrevention`
    3. `TestManagedAgentTools_PreconditionFailures` (Stale SHA, Stale Row Version, Wrong State, Nonexistent WF)
    4. `TestManagedAgentTools_Idempotency` (Missing key 400, replay cached result 200, conflict 409)
    5. `TestManagedAgentTools_RBAC` (Non-RemediationAgent denied with 403, RemediationAgent allowed)
    6. `TestManagedAgentTools_OriginalImmutability` (Byte-exact equality and SHA256 invariant check)
    7. `TestManagedAgentTools_ReadOnlyTools` (`incident.get`, `artifact.metadata.get`, `workflow.get`)

## 2. Logic Chain
1. **Observation**: Managed ingress endpoint `/api/v1/internal/agent-tools` must securely execute `remediation.candidate.create` without weakening security, tenant isolation, or data safety.
2. **Deduction**: Candidate service must be registered in the managed tool gateway, with execution context derived strictly from the server-side `agent_workflows` table.
3. **Observation**: Attackers could attempt to supply spoofed `X-Sentinel-Tenant` headers or payload `tenant_id`.
4. **Deduction**: Comparing `X-Sentinel-Tenant` against the durable workflow row and failing closed with HTTP 403 `tenant_context_mismatch` prevents any cross-tenant leakage or spoofing. Overriding payload `tenant_id` with `execCtx.TenantID` guarantees tenant containment.
5. **Observation**: Agents can fail or race if underlying workflow state or parent artifact changed.
6. **Deduction**: Enforcing `ResourcePreconditions` via `VerifyResourcePreconditions` guarantees fail-closed HTTP 412 on stale SHA, stale row version, or stale workflow state.
7. **Observation**: Remediation must never overwrite or mutate original artifacts or bypass quarantine.
8. **Deduction**: The candidate artifact is written to a deterministic key in `ObjectStore`, inserted into `file_instances` with `status = 'CANDIDATE'`, linked via `derived_from`, recorded in `artifact_derivations`, and verified by `verification.Service.VerifyCandidate`.

## 3. Caveats
- No caveats. All functionality is genuine, deterministic, and fully verified across all packages with race detection enabled.

## 4. Conclusion
- Milestone 2 (R2) requirements have been implemented and verified.
- The managed ingress route `/api/v1/internal/agent-tools` (`/internal/agent-tools`) governs candidate creation with strict authentication, RBAC, server-side precondition verification, tenant isolation, idempotency, fail-closed quarantine, and immutable derivation records.

## 5. Verification Method
- Execute tests in `gateway/`:
  - `go test -v . -run 'TestManagedAgentTools'`
  - `go test -v -race ./...`
- Result: All tests pass (`PASS`) with 0 errors and 0 race conditions.
