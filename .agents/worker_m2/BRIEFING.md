# BRIEFING — 2026-08-27T21:12:00Z

## Mission
Implement Milestone 2 (R2): Governed Remediation Candidate Creation on Managed Cloud Ingress.

## 🔒 My Identity
- Archetype: implementer, qa, specialist
- Roles: implementer, qa, specialist
- Working directory: C:\Users\Gathu\Projects\fintech\.agents\worker_m2
- Original parent: 7cccc4a6-5e19-449a-8f34-32aeb24aa187
- Milestone: M2 (R2)

## 🔒 Key Constraints
- License hygiene: Do not use AGPL-3.0-licensed dependencies.
- Exclusively owned files:
  - gateway/managed_agent_tools.go
  - gateway/managed_agent_tools_test.go
  - gateway/agents.go
  - gateway/main.go
  - gateway/agent_orchestrator.go
  - gateway/internal/toolgateway/tools.go
- Do not hardcode test results or create dummy/facade implementations.
- Fail-closed quarantine: candidate file_instances record has status = 'CANDIDATE'.
- Invariant: DerivedArtifact != MutatedOriginal (parent artifact in ObjectStore not mutated, candidate SHA256 != parent SHA256).
- Tenant ID derived exclusively from server-side workflow row; if X-Sentinel-Tenant header is passed and mismatches, return HTTP 403 tenant_context_mismatch.
- Mandatory idempotency key enforcement.

## Current Parent
- Conversation ID: 7cccc4a6-5e19-449a-8f34-32aeb24aa187
- Updated: not yet

## Task Summary
- **What to build**: Wire CandidateService into POST /internal/agent-tools (/api/v1/internal/agent-tools) for emediation.candidate.create on managed cloud ingress with strict server-side precondition verification, fail-closed quarantine, idempotency, RBAC, immutability, and tenant isolation.
- **Success criteria**: All preconditions verified, 403 on tenant mismatch, idempotency handling, CANDIDATE status quarantined, full unit & integration tests pass with go test -v -race ./....
- **Interface contracts**: ORIGINAL_REQUEST.md & DISPATCH.md
- **Code layout**: gateway/

## Key Decisions Made
- Initializing workspace and investigating existing codebase.

## Artifact Index
- DISPATCH.md — Assignment instructions
- BRIEFING.md — Persistent situational awareness
- progress.md — Liveness & step progress

## Change Tracker
- **Files modified**: None yet
- **Build status**: Pending
- **Pending issues**: None

## Quality Status
- **Build/test result**: Pending
- **Lint status**: Clean
- **Tests added/modified**: Pending

## Loaded Skills
- None
