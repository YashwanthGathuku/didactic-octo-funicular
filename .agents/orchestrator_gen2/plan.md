# Plan: SentinelFlow Hardening & Parity

## Phase 0: Survey & Codebase Exploration
- [ ] Dispatch Explorer 1: Investigate R1 & R3 (Lens Lite verification script, test states, manifests, registry, docs)
- [ ] Dispatch Explorer 2: Investigate R2 (CandidateService, managed_agent_tools.go, workflow state, idempotency, quarantine, unit/integration tests)
- [ ] Dispatch Explorer 3: Investigate R4 (SQLite migrations 001-023 vs Postgres migrations 001, table schemas, RLS policy patterns, repository interfaces)
- [ ] Synthesize explorer findings and finalize implementation plan

## Phase 1: Implementation & Verification
- [ ] Milestone 1 (R1 & R3): Finalize any missing pieces in manifests/registry/docs, verify via test runners
- [ ] Milestone 2 (R2): Worker implements governed remediation candidate creation in `gateway/managed_agent_tools.go` with full server-side validation and comprehensive test suite
- [ ] Milestone 3 (R4): Worker ports all SQLite migrations to `gateway/migrations_postgres/` with tenant-scoped RLS policies and PostgreSQL syntax verification
- [ ] Milestone 4 (Quality & Audit Gate): Run full test suites, Reviewers, Challengers, and Forensic Auditor

## Phase 2: Final Verification & Reporting
- [ ] `bash scripts/verify_lens_lite.sh` passing all 7 stages
- [ ] `go test -race ./...` passing in `gateway/`
- [ ] `pytest ai-tier/tests/...` passing
- [ ] `python scripts/generate_docs.py --check` passing
- [ ] Forensic integrity audit passed
- [ ] Completion report to Sentinel
