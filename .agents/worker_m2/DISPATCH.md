## 2026-08-27T21:11:47Z
You are worker_m2, a Worker implementing Milestone 2: R2 (Governed Remediation Candidate Creation on Managed Cloud Ingress).

Your working directory is C:\Users\Gathu\Projects\fintech\.agents\worker_m2.
Read ORIGINAL_REQUEST.md at C:\Users\Gathu\Projects\fintech\ORIGINAL_REQUEST.md first.

MANDATORY INTEGRITY WARNING:
DO NOT CHEAT. All implementations must be genuine. DO NOT hardcode test results, create dummy/facade implementations, or circumvent the intended task. A auditor will independently verify your work. Integrity violations WILL be detected and your work WILL be rejected.

File Ownership:
You exclusively own:
- gateway/managed_agent_tools.go
- gateway/managed_agent_tools_test.go
- gateway/agents.go
- gateway/main.go
- gateway/agent_orchestrator.go
- gateway/internal/toolgateway/tools.go

Requirements & Architecture for R2:
1. gateway/managed_agent_tools.go:
   - Wire CandidateService (gateway/internal/candidate/service.go) into POST /api/v1/internal/agent-tools (POST /internal/agent-tools) for emediation.candidate.create.
   - Remove the HTTP 403 block returning managed_candidate_creation_not_enabled.
   - Update uildManagedToolGateway(db *sql.DB, store objectstore.ObjectStore) to instantiate candService := candidate.NewService(db, store, engine) and register it via 	oolgateway.RegisterCandidateTool(reg, candService).
   - In deriveManagedWorkflowContext, ensure incident_id is queried and populated on the derived context.
   - Enforce server-side workflow state verification: ExpectedArtifactSHA256, ExpectedRowVersion, and ExpectedWorkflowState must match the durable workflow row.
   - Enforce that Tenant ID is derived exclusively from the server-side workflow row. If X-Sentinel-Tenant header is passed and mismatches, return HTTP 403 	enant_context_mismatch. Payload 	enant_id must be ignored/overridden.
   - Enforce mandatory idempotency key: reject requests without an idempotency key (HTTP 400), return cached candidate on identical replay, return HTTP 409 on conflict.
   - Enforce fail-closed quarantine: candidate ile_instances record has status = 'CANDIDATE'.
   - Enforce invariant DerivedArtifact != MutatedOriginal: parent artifact in ObjectStore must not be mutated, candidate SHA256 != parent SHA256.
2. gateway/internal/toolgateway/tools.go:
   - In RegisterCandidateTool, update handlerRemediationCreate so default fields are properly set from execCtx (ParentArtifactID, IncidentID, AttemptNumber, AgentName, AgentVersion, PolicyBundleHash, etc.).
3. gateway/agents.go & gateway/main.go:
   - Plumb store objectstore.ObjectStore into egisterAgentRoutes and egisterManagedAgentToolRoute.
4. gateway/agent_orchestrator.go:
   - Set PolicyBundleHash: currentPolicyBundleHash on execCtx at line ~622.
5. Unit & Integration Tests:
   - In gateway/managed_agent_tools_test.go, write comprehensive unit and integration tests testing:
     - Successful candidate creation with valid preconditions, verification of status = CANDIDATE, independent verification.
     - Tenant ID spoofing prevention (tenant derived strictly from workflow row, header mismatch returns 403).
     - Precondition failure tests (stale SHA, stale row version, wrong workflow state).
     - Idempotency replay (cached response) and conflict (409).
     - RBAC check (non-RemediationAgent denied with 403).
     - Immutability check (parent artifact bytes unchanged).
6. Run and verify:
   - go test -v -race ./... in gateway/
7. Write a complete report to C:\Users\Gathu\Projects\fintech\.agents\worker_m2\handoff.md with:
   - Observation: files modified, exact changes
   - Verification: commands run and verbatim stdout/exit codes
   - Conclusion: pass/fail status
8. Send a message to parent with the summary and verification results.
