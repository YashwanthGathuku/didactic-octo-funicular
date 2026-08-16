# Remediation outcomes — Prompts 00 to 04 and gap closure

**Covers:** commits `cb09694` through `ddae861` on branch
`claude/sentinel-flow-engineering-contract-ilsqlf`
**State described:** commit `ddae861`

> This document is a snapshot at `ddae861` and is deliberately not rewritten as
> later prompts land. Prompt 05 (secret management and egress controls) is
> recorded separately in `SECRETS_AND_EGRESS.md`; where the two disagree about
> current state, the later document and the code are correct.

## 1. What this document is

This is a consolidated record of what Prompts 00 through 04, and the gap-closure
work that followed Prompt 04, actually changed and actually proved. It exists so
a reviewer joining now does not have to reconstruct the state of the codebase
from five separate documents. Every claim below is traceable to a file, a test
name, or a commit. Where a control exists but is not proven, that is stated in
the same sentence as the control.

Where this document and the code disagree, the code is correct and this file is
the defect.

---

## 2. Starting condition

The Prompt 00 baseline (`CURRENT_STATE.md`, commit `cb09694`) reproduced the
following at runtime against a clean source build, not against the committed
binary. These are the defects the rest of this document is measured against.

**A zero-byte file was RELEASED.** `ProcessFileBytes` initialised every
ingestion result to `Status: "RELEASED"` and downgraded only on a positive
finding. The Moov ACH parser exception was recorded at `WARNING` and was the one
finding branch that did not set `QUARANTINED`. An empty file skipped every
arithmetic branch, so the parser warning was the only finding produced and the
file was released. `IsBalanced` was computed as `batchDebits == batchCredits`,
so `0 == 0` also reported the empty file balanced.

```
POST /api/v1/files/ingest-raw   {"filename":"empty.ach","content":""}
 -> {"status":"RELEASED","isBalanced":true,"totalRecordsParsed":0, ...}
```

**Security and settlement state were returned from source constants.**
`/api/v1/hub/edge/sync` returned `"mTLSVerified": true` over plain HTTP with no
client certificate. `/api/v1/instant-payments/validate` returned
`"isCompliant": true` and a transaction with `"status": "SETTLED_INSTANT"`,
`amountUsd: 150000` and routing numbers `021000021` / `121000358`, all invented
before any parsing occurred. `/metrics` emitted `sentinel_worker_pool_active 8`
while no worker pool existed, and named its chain gauge
`sentinel_merkle_chain_height` for a structure that is a linear predecessor
chain.

**Webhook secrets were disclosed and were not random.** The secret was returned
in the create response, returned again by `GET /api/v1/webhooks` to every
caller, and readable through the arbitrary SQL console. It was generated as
`"whsec_" + fmt.Sprintf("%x", time.Now().UnixNano())` — time-derived, not
cryptographically random, and guessable given an approximate creation time.

**Unrestricted outbound requests.** `POST /api/v1/webhooks/test` fetched any
caller-supplied URL with no scheme, host, IP, private-range or redirect
restriction. The baseline reproduced a request to
`http://169.254.169.254/latest/meta-data/`; the `403` observed came from the
sandbox egress proxy, not from anything in the code path.

**There was no authentication without a token.** `requireAuth` wrapped
`/api/v1` but, when `SENTINEL_API_TOKEN` was empty, called `next.ServeHTTP` and
served every route publicly, logging one warning line at startup.
`GET /api/v1/ledger` returned `200` unauthenticated. Actor identity came from
request fields: the approval endpoint read `body.Actor` and defaulted it to the
literal `"TREASURY_SUPERVISOR_01"`. No business table had a `tenant_id` column,
so every query returned every tenant's rows.

Also recorded and relevant to what follows: the fabricated AI triage fallback
(`Confidence: 0.94` with invented Nacha citations when the Python tier was
unreachable), the fabricated eval fallback (`passRatePct: 100.0`, `5/5` passed),
the committed 20 MB binary at `gateway/sentinel-gateway`, `go.mod` declaring Go
1.26.4 against CI and containers pinned to 1.22, and a frontend that caught
every API error and returned `[]` while logging "using local mock state".

---

## 3. Per-prompt outcomes

### Prompt 00 — Baseline audit (`cb09694`)

**Objective.** Produce a read-only, evidence-backed picture of the repository
before anything is changed.

**What changed.** One file added: `docs/engineering/CURRENT_STATE.md` (633
lines). No production code was touched — `git show --stat cb09694` shows a
single-file commit.

**What was proved, and how.** Every P0 finding was reproduced at runtime against
a freshly built binary from source, with the request and response recorded. The
committed binary was explicitly excluded as evidence. All fourteen documented
verification commands were run and each marked PASS, FAIL or NOT RUN with the
reason and exact output; four were FAIL or NOT RUN and are named as such
(Python evals FAIL exit 2, `npm audit` FAIL 1 high, containers/secret
scanning/migration tests NOT RUN for absent tooling).

**Left undone, deliberately.** Nothing was fixed. The document ends with a P0/P1/P2
change list and a list of existing work that must be preserved. Five items were
labelled UNKNOWN rather than guessed, including which UI code paths use the
browser-side NACHA parser versus the server API.

**Procedural finding worth keeping.** Running `go build ./...` inside `gateway/`
writes to `gateway/sentinel-gateway`, the path of the then-committed binary, and
clobbered it. That is why builds in this repository should use `-o /dev/null`.

### Prompt 01 — Truth reset and scope reduction (`07b38f6`, `b4e7324`, `bdbe136`, `d69047d`, `ae88bd3`)

**Objective.** Remove every simulated surface and unsupported claim from the
shipped application, and establish a scope document that fixes what the product
claims.

**What changed.**

- `07b38f6` (additive only): `SCOPE.md`, plus verbatim non-executable archives
  `REMOVED_CODE_ARCHIVE.md` (2,656 lines) and `REMOVED_CODE_ARCHIVE_UI.md`
  (initial backend/UI capture), each entry carrying original path, line count,
  source commit and removal reason.
- `b4e7324`: the fail-closed ingestion fix. `gateway/processor.go` +51/-3,
  `gateway/quarantine_test.go` added (135 lines) **first**, failing against the
  previous behaviour.
- `bdbe136`: backend removal — `agent_swarm.go`, `anomaly.go`, `connector.go`,
  `drift.go`, `failover.go`, `healing.go`, `instant_payment.go`, `vault.go`,
  `webhook.go` and their tests deleted; `main.go` reduced by 463 lines net;
  `metrics.go` and `compliance.go` corrected. `NewRouter` was extracted from
  `main()` so the route table is addressable by tests.
- `d69047d`: UI removal — ten modal components deleted (Integration Hub, agent
  swarm, chaos monkey, self-healing, SQL console, vault/instant payments,
  benchmark, executive deck, infrastructure config, chaos controls),
  `DemoDataBanner.tsx` added, `src/services/api.ts` rewritten to surface typed
  errors, README claims corrected.
- `ae88bd3`: unsupported claims still rendered by *retained* screens —
  `AuditLedgerModal` SEC 17a-4 / SOX 404 / "Merkle Linked" copy, `FileDiffModal`
  "SIMD-94" badge, `AiAnalystPanel` "SOX 404 requirement", and a fixed
  4.2 ms / 1.8 MB / 28,500,000 B/s `resourceMetrics` block attached to every
  ingested file.

Across `07b38f6..ae88bd3` the tree shows 43 files changed, 1,090 insertions and
7,298 deletions (that total includes README and archive churn).

**What was proved, and how.**

- `quarantine_test.go:TestEmptyFileIsQuarantined` proves a zero-byte file cannot
  reach RELEASED. `TestEmptyFileIsNotReportedBalanced` proves the balance
  assertion is withdrawn over zero records.
  `TestWhitespaceOnlyFileIsQuarantined` and `TestTruncatedFileIsQuarantined`
  cover the adjacent cases; `TestParserExceptionIsNotAdvisory` pins the specific
  defect that the parser exception was `WARNING`;
  `TestValidNachaReachesValidated` is the counterweight, proving failing closed
  did not become failing always.
- `removed_surface_test.go:TestRemovedRoutesReturn404` enumerates 23 removed
  routes with the reason each was removed and asserts 404 (or 405) for each.
- `removed_surface_test.go:TestNoFabricatedValuesInProductionResponses` fetches
  eight surviving routes including `/metrics` and fails if any response body
  contains `mTLSVerified`, `SETTLED_INSTANT`, `99.998`, `12500`, `1.42 ms`,
  `breachRiskPct`, `countdownMinutes`, `sentinel_worker_pool_active`,
  `sentinel_merkle_chain_height`, `SEC Rule 17a-4`, `SOX 404`, `FINRA`,
  `passRatePct`, `Eliza 2.0` or `whsec_`.
- `removed_surface_test.go:TestAiTierUnavailableIsNotSuccess` proves the eval
  route returns non-200 with `NOT_RUN` or `NOT_CONFIGURED` and never a score.
- `removed_surface_test.go:TestSurvivingRoutesStillServe` guards against removal
  by breakage.

**Left undone, deliberately.** Authentication was not touched in this prompt —
the commit message for `bdbe136` states that a large deletion and a security
control change must not land together. The ingestion fix is minimal fail-closed
behaviour, not a versioned policy engine; that remains Prompt 07.

**Residue found during this review, not yet closed.** `src/services/api.ts`
still defines `triggerChaos()` targeting `/chaos/trigger` and `runBenchmark()`
targeting `/benchmark/run`, both of which were removed server-side and now
return 404. Neither function has any caller in `src/`, so nothing invokes them,
but the Prompt 01 acceptance criterion "repository search finds no removed
routes" is not literally satisfied.

### Prompt 02 — Reproducible build and secure configuration (`9fd94e3`, `975dceb`)

**Objective.** Make a clean checkout reproducible; make misconfiguration a
startup failure rather than a runtime surprise.

**What changed.**

- `9fd94e3`: one version pinned per toolchain. `gateway/go.mod` now declares
  `go 1.25.8`, matching `.github/workflows/ci.yml` `GO_VERSION`. `.nvmrc` is
  `22.22.2`, matching CI `NODE_VERSION`. `ai-tier/pyproject.toml` declares
  `>=3.11,<3.12`, matching CI `PYTHON_VERSION: "3.11"`.
  `ai-tier/requirements.txt` and `requirements-dev.txt` created. The committed
  binary `gateway/sentinel-gateway` (20,644,521 bytes) and `Sentinalflow.zip`
  deleted. Eight foreign DaVinci source files under `src/` deleted.
  `vite.config.js` deleted so `vite.config.ts` — and with it
  `@vitejs/plugin-react` — actually loads. `gofmt` applied repo-wide.
- `975dceb`: `gateway/config.go` (typed configuration with profile-aware
  startup validation), `gateway/migrate.go` (versioned, embedded, recorded
  migrations with a `migrate` / `migrate status` subcommand),
  `gateway/migrations/001_init_schema.sql` (renamed from `01_init.sql`),
  `compose.yaml` as the single stack replacing `gateway/docker-compose.yml` and
  `podman-compose.yml`, and a rewritten `.github/workflows/ci.yml`.

**What was proved, and how.**

- `config_test.go:TestProductionProfileRefusesIncompleteConfiguration` proves
  the production profile refuses to start without its dependencies;
  `TestProductionRejectsWeakOrDefaultToken` covers short and well-known values;
  `TestDemoProfileBindsLoopbackOnly` proves the demo profile refuses any
  non-loopback bind address; `TestUnknownProfileIsRejected`,
  `TestMalformedDependencyUrlFailsAtStartup` and
  `TestConfiguredAiTierUrlIsHonoured` cover the rest. The AI-tier test matters
  specifically because the previous code read `AI_TIER_URL` and then used a
  hardcoded `127.0.0.1`.
- `config_test.go:TestMigrateEmptyDatabase`, `TestMigrateIsIdempotent` and
  `TestMigrateUpgradesLegacySchemaFixture` cover the migration paths;
  `TestMigrationDoesNotSeedDemoData` and
  `TestDemoSeedIsRefusedOutsideDemoProfile` keep the demo corpus out of a real
  database.
- The `config` CI job additionally runs the built binary with
  `SENTINEL_PROFILE=production` and no configuration and **fails if it starts**.
  The `migrations` job runs `migrate`, `migrate status`, `migrate` again, and
  `migrate status` again against the real command path.
- CI gained gofmt, `go mod tidy` verification, `go vet`, race tests,
  `npm audit --audit-level=high`, `ruff`, gitleaks and container builds. Two
  gates failed when first enabled and were fixed rather than relaxed: the
  `nanoid` high-severity advisory and six unused imports in the AI tier.

**Left undone.** The application opens SQLite (`sql.Open("sqlite", …)` in
`main.go`), while `compose.yaml` provisions PostgreSQL and MinIO. Object storage
is configured and reported by `/ready` but no artifact is written to it —
`storage_path` remains the synthesised string `"/s3/incoming/" + filename` in
`processor.go`. Both are Prompt 06.

### Prompt 03 — Domain model and state machines (`54fc016`)

**Objective.** Make the release decision a property of a modelled domain rather
than a sequence of conditionals, and make impossible states unrepresentable.

**What changed.** `gateway/internal/domain/` added: `state.go` (four state
machines as maps of legal edges), `artifact.go` (entities and
`AuthorizeRelease`), `entities.go`. `gateway/migrations/002_tenancy_and_state.sql`
(394 lines) rebuilds seven tables. `processor.go` now routes its outcome through
`domain.Artifact.TransitionTo` instead of assigning a status string.
`docs/engineering/adr/0001-domain-model-and-state-machines.md` records the
decision.

The artifact machine, from `state.go`, contains no edge from `RECEIVED` or
`QUARANTINED` to `RELEASED`; `ArtifactReleased` is reachable only from
`ArtifactApproved`, which is reachable only from `ArtifactValidated`.

**What was proved, and how.**

- `domain/state_test.go:TestArtifactCannotReachReleasedExceptFromApproved` and
  `TestReleaseFromReceivedOrQuarantinedFails` prove the defect class is
  unrepresentable at the domain layer.
  `TestArtifactTransitionMatrixIsExhaustive` and
  `TestArtifactTransitionToRefusesIllegalEdges` enumerate illegal edges by
  difference from the table rather than by a hand-written list.
- `TestStaleApprovalCannotReleaseChangedContent` proves the three identity
  bindings in `AuthorizeRelease` — artifact SHA-256, artifact ID and validation
  run ID — reject an approval replayed against different bytes.
  `TestApprovalCannotBeSkippedWhenPolicyRequiresIt`,
  `TestDualControlRequiresTwoDistinctPeople`,
  `TestReleaseRequiresVersionedPolicyDecision` and
  `TestReleaseRefusedOnBlockingFinding` cover the remaining preconditions.
- `tenancy_test.go:TestEveryBusinessTableRequiresATenant` proves `tenant_id` is
  `NOT NULL` everywhere. `TestArtifactStatusIsConstrainedToModelledStates`
  proves the database `CHECK` refuses a status the model does not define.
  `TestSettlementIsNotAStateAnywhere` proves it across tables.
- `TestDuplicateIdempotencyKeyIsRejectedPerTenant`,
  `TestSamePersonCannotApproveTwice` and
  `TestConcurrentDecisionsCannotBothFinalize` prove the uniqueness constraints
  in migration 002 (`UNIQUE (tenant_id, idempotency_key)`,
  `UNIQUE (tenant_id, decision_id, actor_id)`,
  `UNIQUE (tenant_id, validation_run_id)`).
- `integrity_test.go:TestAuditEventsAreAppendOnly` and
  `TestStatusHistoryIsAppendOnly` prove the SQLite triggers hold. The ledger
  tamper tests (`TestLedgerDetectsContentTampering`,
  `TestLedgerDetectsActorTampering`) now drop those triggers first, deliberately
  modelling an attacker with direct database access, so the property being
  exercised remains hash-chain verification by recomputation.

**Behavioural change accepted rather than worked around.** Ingestion now
terminates at `VALIDATED` or `QUARANTINED`, never `RELEASED`. Two tests were
updated to match, and the UI status mapping changed.

**Left undone, stated in the ADR.** Tenancy at this point was a storage
precondition, not isolation: `DefaultTenantID` was still used by every write
because the request path had no identity. Money remained `float64` in the API
response struct (`processor.go` `IngestionResult.TotalDebitsUsd` /
`TotalCreditsUsd`) while the database columns and domain types are integer minor
units. `validation_runs`, `policy_decisions`, `approvals`, `ingestion_jobs`,
`job_attempts` and `notification_intents` exist as schema and domain types but
are not written by the ingestion path.

**Boundary found during this review.** `domain.AuthorizeRelease` has no caller
outside its tests — `grep -rn AuthorizeRelease` over non-test Go files matches
only its own definition. It is a proven release rule that no HTTP path invokes.
The `/api/v1/incidents/{id}/approve` handler resolves an *incident* to
`RESOLVED` and appends an audit event; it does not move an artifact to
`APPROVED` or `RELEASED`. Wiring release through `AuthorizeRelease` is Prompt 11.

### Prompt 04 — Authentication, authorization, tenant isolation (`e26d846`, `e6dcd5a`)

**Objective.** Mandatory provider-neutral OIDC, an authorization matrix, actor
identity from verified claims only, and tenant scoping enforced at the
repository boundary rather than the route.

**What changed.**

- `e26d846`: `internal/auth/{principal,verifier,middleware}.go` and
  `internal/repository/repository.go` added. `config.go` gained the OIDC
  requirements for the production profile.
- `e6dcd5a`: the verifier wired into `NewRouter`, the shared-secret middleware
  removed, `internal/auth/jwks.go` added, `router_auth_test.go` added, and
  `ROUTE_PERMISSION_MATRIX.md` written.

**What was proved, and how.**

- `router_auth_test.go:TestProductionRouterFailsClosedWithoutVerifier` proves a
  production router built with no verifier returns `503
  authentication_not_configured` on business routes rather than serving them.
  `TestProductionRouterRejectsAnonymousRequests` covers the anonymous case.
  This is the direct replacement for the open-when-unset branch.
- `verifier_test.go:TestRejectsAlgNone` and `TestRejectsAlgorithmConfusion`
  construct the attacks rather than asserting a flag; `verifier.go` passes
  `jwt.WithValidMethods([]string{"RS256"})`. `TestRejectsExpiredToken`,
  `TestRejectsTokenWithNoExpiry`, `TestRejectsWrongIssuer`,
  `TestRejectsWrongAudience`, `TestRejectsUntrustedSigner` and
  `TestRejectsUnknownKeyID` cover the rest of the validation surface.
- `verifier_test.go:TestRejectsUnknownRoleInClaim` proves an unknown role is a
  hard failure, not a silent drop — a mis-mapped provider configuration must not
  present as "user with no permissions".
  `TestPlatformAdminCannotBeSmuggledThroughMembership` proves `platform_admin`
  inside a membership list is refused.
- `verifier_test.go:TestAuthorizeMatrix` enforces the permission table in
  `principal.go` `rolePermissions`, including the two deliberate gaps:
  `TestTenantAdminCannotReachPlatformScope` and
  `TestPlatformAdminIsNotAUniversalReader`.
- `verifier_test.go:TestActorIdentityComesOnlyFromTheToken` and
  `router_auth_test.go:TestApprovalRecordsVerifiedActorNotRequestBody` prove the
  forged-actor defect is closed. The router test asserts against the audit
  ledger, not only the response body.
  `TestApprovalRequiresJustification` covers the missing-reason case.
- `router_auth_test.go:TestCsrfRequiredForCookieAuthenticatedMutations` proves
  CSRF enforcement; `middleware.go` `RequireCSRFToken` applies only when a
  session cookie is present, on the reasoning that a request authenticated by an
  `Authorization` header is not forgeable cross-origin by a browser.
- `repository_test.go:TestZeroScopeIsRefusedByEveryMethod` is what makes the
  scoping structural: a `Scope` can only be produced by `NewScope`, which
  requires an already-authorized `Principal`, and every method calls `s.valid()`
  first. `TestCrossTenantIdLookupIsIndistinguishableFromMissing`,
  `TestCountIsTenantScoped`, `TestCrossTenantUpdateAffectsNothing`,
  `TestConcurrentUpdateOnlyOneWins`, `TestStatusHistoryRecordsVerifiedActor`,
  `TestScopeRequiresPermission` and `TestListIsBounded` cover the rest.

**Left undone at the end of Prompt 04, recorded in the matrix at the time.**
Five gaps: `/metrics` unauthenticated; the browser PKCE flow not built; JWKS
fetching not exercised over the network; handlers still writing to
`DefaultTenantID` despite the mechanism existing; and PostgreSQL RLS unavailable
while the application runs on SQLite.

### Gap closure (`6fdb5e1`, `ddae861`)

**Objective.** Close the five gaps, or state precisely what remains.

**`6fdb5e1` — tenant from verified claims.** `gateway/tenant.go` adds
`resolveScope`, which derives the tenant from the principal's verified
memberships: an explicit `X-Sentinel-Tenant` header validated against
memberships, or the sole membership, or a refusal. `ProcessFileBytes`,
`ProcessFile`, `AppendAuditEvent`, `GetLedger` and `GenerateCompliancePackage`
are parameterised by tenant and each refuses an empty one, so a handler that
forgets to authorize still cannot write into a foreign tenant.

`tenant_isolation_test.go` proves the acceptance criterion over HTTP with real
signed tokens rather than at the repository layer alone:
`TestTwoTenantsCannotReadEachOthersArtifacts`,
`TestLedgerAndEvidenceExportAreTenantScoped`,
`TestTenantHeaderCannotSelectAnotherTenant`,
`TestUnknownAndForeignTenantsAreIndistinguishable` (a nonexistent tenant returns
a byte-identical response to an existing one the caller does not belong to, so
the header cannot enumerate tenants), `TestViewerCannotUploadOrApproveOverHttp`,
`TestTenantAdminCannotApproveOverHttp` and
`TestMultiTenantPrincipalMustSelectATenant`.

**`ddae861` — the remaining four.**

1. **`/metrics`** is guarded by `SENTINEL_METRICS_TOKEN` (minimum 32 characters,
   required in production, compared with `subtle.ConstantTimeCompare` in
   `main.go`). `router_auth_test.go:TestMetricsRequiresCredentialInProduction`
   covers anonymous, wrong and correct credentials;
   `TestMetricsIsOpenOnlyInTheDemoProfile` pins the demo exception, where the
   process binds loopback only. `config_test.go:TestProductionRejectsWeakMetricsToken`
   covers the length floor.
2. **PKCE** — `internal/auth/pkce.go` implements S256-only challenges, separate
   `state` and `nonce`, a 10-minute flow TTL, open-redirect sanitisation, an
   HttpOnly `SameSite` session cookie and a readable CSRF cookie for
   double-submit. `pkce_test.go:TestFullPkceLoginAgainstStubProvider` runs
   against a stub authorization server that actually verifies the verifier, and
   `TestStolenCodeCannotBeRedeemedWithAnotherVerifier` proves the case the
   mechanism exists for. `TestStateAndNonceMismatchAreRejected`,
   `TestExpiredFlowCannotBeRedeemed`,
   `TestRedirectAfterLoginCannotLeaveTheSite`,
   `TestExchangeRejectsProviderErrorsWithoutLeakingDetail`,
   `TestExchangeRejectsResponseWithoutIdToken`,
   `TestSessionCookieIsHttpOnlyAndSameSite` and
   `TestCsrfCookieIsReadableButScoped` cover the failure modes.
   **The handlers are not mounted.** `/auth/login`, `/auth/callback` and
   `/auth/logout` are not registered on any router; a repository-wide search for
   those paths matches only a test fixture URL. The flow logic and its failure
   modes are complete and tested; the HTTP wiring is not done.
3. **JWKS over the network** — `jwks_test.go` exercises `FetchJWKS` against a
   real `httptest` server: `TestFetchJWKSAndVerifyATokenEndToEnd`,
   `TestFetchJWKSHandlesMultipleKeysForRotation`,
   `TestFetchJWKSRejectsErrorResponses`, `TestFetchJWKSRejectsMalformedBody`,
   `TestFetchJWKSRejectsWeakKeys` (1024-bit refused; `jwks.go` enforces a
   2048-bit modulus floor), `TestFetchJWKSSkipsKeysWithoutAnId`,
   `TestFetchJWKSBoundsTheResponseBody` (`io.LimitReader(resp.Body, 1<<20)`),
   `TestFetchJWKSRespectsContextCancellation` and
   `TestFetchJWKSFailsOnUnreachableHost`. What remains unverified is named in a
   comment at the foot of `jwks_test.go`: TLS to a real host (httptest serves
   plain HTTP), provider-specific quirks such as `x5c` chains, and live rotation
   timing.
4. **Inbox watcher tenancy** — `StartInboxWatcher` takes the tenant as a
   required parameter and refuses to start on an empty one. `main.go` verifies
   the configured tenant exists in the `tenants` table before starting it, and
   logs that the watcher is disabled when `SENTINEL_WATCHER_TENANT` is unset.
   `config_test.go:TestWatcherIsDisabledWithoutAnExplicitTenant` and
   `TestWatcherRefusesEmptyTenant` cover both. Contract-based tenant resolution
   remains Prompt 10.
5. **PostgreSQL RLS** — `migrations_postgres/001_schema_and_rls.sql` enables and
   `FORCE`s row-level security on `partners`, `file_instances`, `incidents` and
   `audit_events`, with both `USING` and `WITH CHECK` on every policy.
   `rls_postgres_test.go` verifies this against a real PostgreSQL 16 server as a
   `NOSUPERUSER` role owning none of the tables:
   `TestRlsUnsetTenantSeesNothing` (a forgotten scope starves rather than
   discloses), `TestRlsScopedReadSeesOnlyItsOwnTenant`,
   `TestRlsRefusesCrossTenantInsert`, `TestRlsCrossTenantUpdateAffectsNoRows`,
   `TestRlsCountsDoNotLeakAcrossTenants`,
   `TestApplicationRoleIsNotSuperuserOrTableOwner` and
   `TestRlsIsEnabledAndForcedOnEveryProtectedTable`. The `postgres-rls` CI job
   runs these against a `postgres:16.6-alpine` service container and has a
   second step that fails the build if the output contains `SKIP`.
   **RLS is not in the request path.** The running application still opens
   SQLite. The PostgreSQL schema and its policies are verified in a test
   database, not currently protecting production traffic.

---

## 4. Contract invariants and where each is enforced

Invariants are numbered as in `docs/engineering/CLAUDE.md`.

| # | Invariant | Enforced at | Proven by |
|---|---|---|---|
| 1 | Every financial input begins untrusted and unreleased | `processor.go` `ProcessFileBytes` initialises `Status: "RECEIVED"`; `domain/state.go` has no `RECEIVED → RELEASED` edge | `state_test.go:TestArtifactCannotReachReleasedExceptFromApproved` |
| 2 | Empty/partial/unparseable input fails closed into typed quarantine | `processor.go` terminal decision (`!parserSucceeded \|\| TotalRecordsParsed == 0 \|\| hasBlockingFinding`) | `quarantine_test.go:TestEmptyFileIsQuarantined`, `TestWhitespaceOnlyFileIsQuarantined`, `TestTruncatedFileIsQuarantined`, `TestParserExceptionIsNotAdvisory` |
| — | *Duplicate-conflicting* input, same invariant | **Not yet enforced in the ingestion path.** `migrations/002` has `UNIQUE (tenant_id, idempotency_key)` on jobs, but redelivery through `/files/ingest-raw` creates a second file instance. Prompt 08 | constraint proven by `tenancy_test.go:TestDuplicateIdempotencyKeyIsRejectedPerTenant`; the ingestion path is not |
| 3 | Original artifact immutable; repair creates a derived artifact | Schema only: `file_instances.derived_from_id` in `migrations/002` and `migrations_postgres/001`; `domain/artifact.go` `Artifact.DerivedFromID` | **No repair path exists to test.** The self-healing endpoint was deleted in `bdbe136`; Prompt 07/11 |
| 4 | Deterministic parsing and release decisions do not depend on AI | `processor.go` calls no AI; `/incidents/{id}/triage` is a separate read-only route returning 503 when unconfigured | `removed_surface_test.go:TestAiTierUnavailableIsNotSuccess`; `e2e_test.go:TestE2E_ValidNachaPipeline` runs with no AI tier |
| 5 | AI is read-only and cannot release, repair, pay, notify, execute SQL or use a shell | The routes that offered those capabilities are deleted: `/sql/query`, `/healing/apply`, `/webhooks/test`, `/swarm/*` | `removed_surface_test.go:TestRemovedRoutesReturn404` (23 routes) |
| 6 | Authentication mandatory outside a named local demo profile | `auth.Middleware.Authenticate` (nil verifier ⇒ 503, no token ⇒ 401); `config.go` requires OIDC issuer/audience/JWKS in production; `DemoPrincipal` is set only when `cfg.IsDemo()` | `router_auth_test.go:TestProductionRouterFailsClosedWithoutVerifier`, `TestProductionRouterRejectsAnonymousRequests`; `config_test.go:TestProductionProfileRefusesIncompleteConfiguration` |
| 7 | Actor identity comes from authenticated claims, never request fields | `auth.Principal.ActorID()` returns the token subject; the approve handler ignores any `actor` field; `repository.Scope.ActorID()` is what `status_history` records | `router_auth_test.go:TestApprovalRecordsVerifiedActorNotRequestBody`; `verifier_test.go:TestActorIdentityComesOnlyFromTheToken`; `repository_test.go:TestStatusHistoryRecordsVerifiedActor` |
| 8 | Every business record tenant-scoped; repository queries enforce it | `tenant_id NOT NULL` on every business table (`migrations/002`); `repository.Scope` cannot be built without an authorized principal and every method refuses a zero scope; `tenant.go` `resolveScope` | `tenancy_test.go:TestEveryBusinessTableRequiresATenant`; `repository_test.go:TestZeroScopeIsRefusedByEveryMethod`, `TestListReturnsOnlyOwnTenant`; `tenant_isolation_test.go` (whole file, over HTTP) |
| 9 | Secrets are write-only references, never returned or logged | **Partially enforced by deletion, not by a mechanism.** The webhook subsystem that returned plaintext secrets and the SQL console that read them are gone; `auth/middleware.go` logs a denial reason but never the token. There is no `SecretStore` abstraction — that is Prompt 05 | `removed_surface_test.go:TestNoFabricatedValuesInProductionResponses` bans `whsec_` from eight route bodies; no test proves a general redaction property |
| 10 | Security state derived from verified runtime state, never constants | The routes returning `mTLSVerified`, `isCompliant`, `SETTLED_INSTANT` are deleted; `/ready` derives every field from an actual probe | `removed_surface_test.go:TestNoFabricatedValuesInProductionResponses`; `removed_surface_test.go:TestRemovedRoutesReturn404` |
| 11 | Operational metrics measured; synthetic values isolated and labelled | `metrics.go` counters are `atomic` counts; the parse-rate gauge returns `-1` until `RecordMeasuredParseRate` is called; `sentinel_worker_pool_active` removed; UI `DemoDataBanner.tsx` labels the synthetic board | `security_test.go:TestPrometheusMetricsExposition`; `TestNoFabricatedValuesInProductionResponses` bans the removed gauge names |
| 12 | A missing dependency produces UNAVAILABLE or DEGRADED, never fabricated success | `/ready` reports `UNAVAILABLE` / `DEGRADED` / `NOT_CONFIGURED` per dependency; `/evals/run` returns 503 `NOT_RUN` or `NOT_CONFIGURED`; the triage route has no offline branch | `removed_surface_test.go:TestAiTierUnavailableIsNotSuccess` |
| 13 | State transitions explicit, validated, persisted, auditable | `domain.Artifact.TransitionTo`; database `CHECK` on every status column; `status_history` written by `repository.UpdateArtifactStatus` and protected by append-only triggers | `state_test.go:TestArtifactTransitionToRefusesIllegalEdges`; `tenancy_test.go:TestArtifactStatusIsConstrainedToModelledStates`; `integrity_test.go:TestStatusHistoryIsAppendOnly` |
| 14 | Duplicate delivery and restart are idempotent | **Not yet enforced.** The schema supports it (`UNIQUE (tenant_id, idempotency_key)`, `UNIQUE (tenant_id, contract_id, business_date)`); the ingestion path does not use it. Prompt 08 | schema constraints proven by `tenancy_test.go`; the delivery path is not |
| 15 | Bounded concurrency and backpressure; no unbounded goroutines, queues, bodies, result sets, retries or model calls | Partially: `repository` bounds every result set at 500 and defaults to 100; `stream.go` uses buffered channels with non-blocking send and unsubscribe on disconnect; `jwks.go` and `pkce.go` bound response bodies at 1 MiB; `/files/upload` parses at most 20 MB of multipart form | `repository_test.go:TestListIsBounded`; `jwks_test.go:TestFetchJWKSBoundsTheResponseBody`. **Not enforced:** `/files/ingest-raw` decodes its JSON body with no `http.MaxBytesReader`, and `ProcessFileBytes` holds the whole file in memory. Prompt 06 |

Two further contract rules, tracked here because they are frequently assumed
closed:

- **"Use integer minor units or a decimal library for money; never float."**
  The database columns and `domain` types are integer minor units, and NACHA
  cent values are parsed as `int64` in `processor.go`. The API response struct
  still carries `TotalDebitsUsd float64` / `TotalCreditsUsd float64`, and the
  experimental ISO 20022 path handles money as `float64`. Prompt 07.
- **"Never log or return unredacted account/routing values."**
  `domain/artifact.go` defines `Finding.EvidenceRedacted` as a bounded excerpt,
  but the production path in `processor.go` still writes `RawData: line` — the
  complete 94-character record — into `validation_findings`, and `GET
  /api/v1/incidents` selects and returns `raw_data`. `IngestionResult` still
  carries `RawContent` with the whole file body. The `raw_data` field is no
  longer sent to the AI tier (the triage request populates only `FileID` and
  `Findings`), but the finding *descriptions* that are sent include routing
  numbers. Baseline item P0-11 is therefore **not closed**.

---

## 5. Verification ledger

Status as of commit `ddae861`.

| Check | Command | Result |
|---|---|---|
| Go formatting | `gofmt -l .` | PASS |
| Go static analysis | `go vet ./...` | PASS |
| Go tests | `go test ./...` | PASS |
| Go race tests | `go test -race ./...` | PASS |
| TypeScript typecheck | `npx tsc --noEmit` | PASS |
| Frontend production build | `npm run build` | PASS |
| PostgreSQL row-level security | RLS suite against a real PostgreSQL 16 server | PASS |

Test totals: **218 tests — 126 gateway, 45 auth, 30 domain, 17 repository.**

Not run in this environment, with the reason:

| Check | Reason |
|---|---|
| CI `containers` job (three `docker build` invocations) | The Docker daemon is unavailable in this environment. This is an environment limitation, not a failure: the job is defined in `.github/workflows/ci.yml` and runs on GitHub Actions. |
| JWKS fetch against a live identity provider over TLS | No identity provider is reachable from this environment. `jwks_test.go` covers the network path against a plain-HTTP `httptest` server; TLS behaviour, provider-specific document quirks such as `x5c`, and live rotation timing remain unverified, and the test file says so. |
| End-to-end PKCE against a real provider | Same reason. The flow is tested against a stub authorization server that verifies the PKCE verifier. |
| RLS against the running application | The application uses SQLite. The policies are proven against PostgreSQL in a test database only. |

Toolchain notes: the local Go toolchain is 1.24.7; `go.mod` declares 1.25.8 and
it is fetched via `GOTOOLCHAIN=auto`. PostgreSQL 16 is installed and startable
locally, which is how the RLS suite ran.

Standing procedural note: do not run `go build` inside `gateway/` without
`-o /dev/null`. Prompt 00 recorded that it writes to `gateway/sentinel-gateway`.
The committed binary at that path was deleted in `9fd94e3`, so the hazard is
reduced, but the default output path is unchanged.

---

## 6. Open boundaries carried into Prompt 05 and beyond

Stated plainly, in rough order of how much they limit what may be claimed today.

1. **PKCE route handlers are not mounted.** `/auth/login`, `/auth/callback` and
   `/auth/logout` are not registered on any router. The flow logic and its
   failure modes are complete and tested; the HTTP wiring is not done. Nothing
   in this service issues the session cookie that the CSRF middleware protects.
2. **RLS is not in the request path.** The running application uses SQLite. The
   PostgreSQL schema and policies are verified in a test database, not currently
   protecting production traffic. Tenant isolation in the running system rests
   on `resolveScope` and the `tenant_id` predicate in each handler's SQL, proven
   over HTTP by `tenant_isolation_test.go`, not on the database (see item 6).
3. **The UI cannot authenticate.** `src/services/api.ts` sends no
   `Authorization` header (a comment in the file says Prompt 04 supplies the
   credentials it will send). The frontend can therefore only talk to a
   `local-demo` gateway. It also still defines `triggerChaos()` and
   `runBenchmark()` against routes that were removed and now 404, though nothing
   calls them.
4. **There is no secret management subsystem.** Invariant 9 holds by deletion,
   not by mechanism, at the commit this document describes. Closed by Prompt 05
   — see `SECRETS_AND_EGRESS.md`, which also records what that work left
   undone.
5. **`AuthorizeRelease` is unwired.** The release rule is proven by ten domain
   tests and invoked by no handler. `/incidents/{id}/approve` resolves an
   incident; it does not move an artifact to `APPROVED` or `RELEASED`, so no
   artifact in the running system can reach `RELEASED` at all today. The
   consequence is visible as a dead branch: `watcher.go` still tests
   `result.Status == "RELEASED"`, which ingestion can no longer produce.
   Prompt 11.
6. **The repository package is not yet on the request path.** `repository.New`
   has no caller outside its tests, and `UpdateArtifactStatus` has none at all.
   Handlers in `package main` still issue SQL directly, passing
   `scope.TenantID()` from `resolveScope` into each `WHERE` clause. The
   structural argument in `ROUTE_PERMISSION_MATRIX.md` — that a route added
   without the middleware still cannot read another tenant's data, because there
   is no way to obtain a usable `Scope` — describes the repository package
   accurately, but the running handlers do not go through it. Tenant isolation
   in the running system is enforced by `resolveScope` plus hand-written
   `tenant_id` predicates, and that is what `tenant_isolation_test.go` exercises
   over HTTP.
7. **Raw financial data is still stored and returned.** Full 94-character NACHA
   records in `validation_findings.raw_data`, returned by `GET /incidents`; the
   whole file body in `IngestionResult.RawContent`. Baseline P0-11 is open.
8. **Ingress is unbounded and non-idempotent.** `/files/ingest-raw` has no body
   size limit; the whole file is held in memory; redelivery creates a second
   file instance; artifacts are never written to object storage
   (`storage_path` is synthesised). Prompts 06 and 08.
9. **The ledger append is not serialised.** `AppendAuditEvent` reads the last
   hash and then inserts. The per-tenant `UNIQUE (tenant_id, sequence_no)` and
   `UNIQUE (tenant_id, previous_hash)` constraints mean a concurrent append
   loses on insert rather than forking the chain, but the application does not
   retry and there is no test of concurrent appends. Prompt 09.
10. **Money is `float64` at the wire boundary**, and the experimental ISO 20022
    path uses `float64` throughout. Prompt 07.
11. **The inbox watcher polls once per second and reads files that may still be
    uploading.** It now requires an explicit tenant, but the finalisation
    problem is untouched. Prompt 17.
12. **`/api/v1/stream` emits only a connect heartbeat.** `Broadcast` has no
    caller outside `stream.go`. Retained deliberately as honest infrastructure.
    Prompt 12.
13. **Validation rule citations are string literals.** Every finding carries a
    `RuleReference` naming "Nacha Operating Rules 2025" with no licensed rule
    source behind it, and the coverage is record width, entry hash and batch
    control arithmetic — not the Nacha rule set. Prompt 07.
14. **Documentation drift.** `SCOPE.md` §2 still describes API authentication as
    "Partial — unsafe … the API runs fully open when `SENTINEL_API_TOKEN` is
    unset", authorization as "Planned", tenant isolation as "Not implemented",
    and `/health` as returning a static string. All four were superseded by
    Prompts 02 and 04 and by the gap-closure commits. `SCOPE.md` has not been
    updated since `07b38f6`.
15. **Two housekeeping items from the baseline remain open.** `CLAUDE.md` exists
    only at `docs/engineering/CLAUDE.md`, not at the repository root where the
    guide places it (baseline item D13, P2-1). No `LICENSE` file exists
    (baseline D9); the README badge was removed rather than the file added.
16. **`SENTINEL_API_TOKEN` is vestigial.** `config.go` still requires it in the
    production profile with a 32-character floor, but no code path authenticates
    with it since the shared-secret middleware was removed in `e6dcd5a`. It is
    a required setting that does nothing.
