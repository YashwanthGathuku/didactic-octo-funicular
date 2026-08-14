# Sentinel Flow — Current State Audit

**Audit date:** 2026-08-14
**Method:** read-only. No production code was changed to produce this document. Every
claim below is either a `file:line` reference or the exact output of a command run in
this environment. Items that could not be verified are labelled `UNKNOWN`.

This document is Prompt 00 of `docs/engineering/SENTINELFLOW_PRODUCTION_READY_PROMPT_GUIDE.md`.
It supersedes nothing — it checks what `docs/engineering/SentinelFlow_Code_Audit_and_Recovery_Plan.md`
and `docs/AUDIT_FIXES.md` claim against what the checked-out source actually does today.

---

## 0. Headline finding: two audits, one repo, partial overlap

`docs/AUDIT_FIXES.md` (dated 2026-08-14, baseline "as-uploaded `Sentinalflow.zip`") documents
a P0/P1 remediation pass that **has been applied to this checkout** — verified below by
re-running its exact checks. `docs/engineering/SentinelFlow_Code_Audit_and_Recovery_Plan.md`
(reviewed artifact "`SentinelFlow-remediated_2.zip`") documents a **separate, non-overlapping**
set of P0 findings — mTLS fabrication, the mock Integration Hub, instant-payment settlement
fabrication, self-healing without real approval, forged-actor trust, tenant isolation — and
every one of those is still present in this checkout, unchanged. The two documents are not
sequential drafts of the same review; they cover disjoint file sets. Treat both as current.

---

## 1. Module and route inventory

### 1.1 Processes / deployables

| Module | Path | Language | Entry point |
|---|---|---|---|
| Gateway (API + workers, monolith) | `gateway/` | Go (module `sentinel-gateway`) | `gateway/main.go` |
| AI tier | `ai-tier/` | Python (FastAPI) | `ai-tier/main.py` |
| Edge agent | `edge-agent/` | Go (standalone binary, not part of `gateway` module) | `edge-agent/main.go` |
| Operations UI | `src/`, `index.html` | React 18 + TypeScript (Vite) | `src/main.tsx` |
| Foreign/dead frontend project | `src/main.js` + `src/{physics,audio,renderer,ui,machines,diagnostics}` | Vanilla JS | `src/main.js` (unreferenced — see §7) |

### 1.2 Gateway Go source files (`gateway/*.go`, excluding `_test.go`)

`agent_swarm.go`, `anomaly.go`, `bai2.go`, `benchmark.go`, `compliance.go`, `connector.go`,
`drift.go`, `failover.go`, `generator.go`, `healing.go`, `instant_payment.go`, `iso20022.go`,
`kstest.go`, `ledger.go`, `main.go`, `metrics.go`, `processor.go`, `robust_anomaly.go`,
`security.go`, `stream.go`, `swift.go`, `vault.go`, `watcher.go`, `webhook.go` (24 files).
A prebuilt 20,644,521-byte binary `gateway/sentinel-gateway` is committed alongside source.

### 1.3 HTTP route inventory (chi router, `gateway/main.go`)

All routes are mounted under `r.Route("/api/v1", …)` (`gateway/main.go:190`) except
`GET /metrics` (`gateway/main.go:187`), which is mounted outside that group and is therefore
**not** behind the auth middleware described in §5.

Routes registered directly in `main.go:187-808` (24):
`GET /metrics`, `GET /health`, `GET /sla-board`, `GET /partners`, `POST /partners`,
`GET /contracts`, `GET /incidents`, `GET /ledger`, `POST /files/ingest-raw`,
`POST /files/upload`, `POST /incidents/{id}/triage`, `POST /incidents/{id}/approve`,
`GET /generator/sample`, `GET /benchmark/run`, `GET /evals/run`, `GET /compliance/export`,
`POST /security/verify-key`, `POST /security/verify-signature`, `GET /webhooks`,
`POST /webhooks`, `POST /webhooks/test`, `POST /sql/query`, `GET /analytics/anomalies`,
`POST /chaos/trigger`.

Routes registered by sub-modules and mounted via `RegisterXRoutes(r, db)` calls at
`gateway/main.go:192-199` (17):

| Sub-router | Mount prefix | File | Routes |
|---|---|---|---|
| Integration Hub | `/hub` | `connector.go:299-300` | `GET /connections`, `GET /assets`, `GET /assets/{id}/sample`, `GET /lineage`, `POST /edge/sync` |
| Agent swarm | `/swarm` | `agent_swarm.go:167-168` | `POST /deliberate`, `GET /sessions`, `GET /sessions/{id}` |
| Self-healing | `/healing` | `healing.go:144-145` | `POST /propose`, `POST /apply` |
| Drift | `/analytics/drift` | `drift.go:102-103` | `GET /` |
| SSE stream | `/stream` | `stream.go:64-65` | `GET /stream` |
| Vault | `/vault` | `vault.go:281-282` | `POST /tokenize`, `GET /policies`, `POST /detokenize` |
| Instant payments | `/instant-payments` | `instant_payment.go:81-82` | `POST /validate`, `GET /metrics` |
| Failover | `/chaos/failover` | `failover.go:64-65` | `POST /simulate` |

Total: 42 HTTP endpoints (41 under `/api/v1`, 1 outside it).

### 1.4 UI screens/modals wired into the shipped app (`src/App.tsx:2-19` imports, `65-79` state)

`Header`, `ChaosControls`, `SlaBoard`, `FileInspectorModal`, `AiAnalystPanel`,
`AuditLedgerModal`, `UploadModal`, `BenchmarkModal`, `ExecutiveDeckModal`,
`ContractConfigModal`, `FileDiffModal`, `ChaosMonkeyModal`, `SqlConsoleModal`,
`IntegrationHubModal`, `AgentSwarmModal`, `SelfHealingModal`,
`VaultAndInstantPaymentsModal`, `InfrastructureConfigModal` — all 17 are imported, all have
a `show*Modal` state flag, and all are opened from `Header` callbacks
(`src/App.tsx:504-510` and following). None of this is dead code — it is the production
build's actual surface area.

---

## 2. Hardcoded operational/security/compliance/performance results

This section separates what `docs/AUDIT_FIXES.md` already fixed (verified still fixed) from
what is still hardcoded today.

### 2.1 Verified fixed (re-checked against current source)

- `security.go` PGP/SSH validation, `vault.go` key handling, `main.go` auth middleware,
  SQL console read-only enforcement, `ledger.go` tamper *detection* (not concurrency —
  see §2.2), `benchmark.go` corpus width, `metrics.go` parse-rate gauge, `failover.go`
  RTO/RPO labels, `drift.go` KS test, `anomaly.go` robust detector, `ai-tier/evals/runner.py`
  eval logic — all match the "Fixed" descriptions in `docs/AUDIT_FIXES.md` items 1–12 on
  inspection of the current file contents.

### 2.2 Still hardcoded / fabricated today

| Location | What it does | Evidence |
|---|---|---|
| `gateway/connector.go:394` | `"mTLSVerified": true` returned unconditionally, no certificate ever inspected | grep-confirmed literal `true` |
| `edge-agent/main.go:22` | Default control-plane URL is `http://localhost:8080` (plaintext) | flag default |
| `edge-agent/main.go:42,79` | Plain `http.Client{}`, no TLS config, yet logs `"Outbound mTLS metadata sync successful"` | `edge-agent/main.go:42`, `:79` |
| `edge-agent/main.go:50-63` | `discoveredResources` (SFTP dir with `filesCount: 4`, Postgres schema with fixed table names) is a struct literal, not a real scan | `edge-agent/main.go:50-63` |
| `gateway/instant_payment.go:53-65` | `ValidateInstantPaymentXml` ignores the input body's actual amount/routing/IDs; hardcodes `AmountUSD: 150000.00`, `DebtorAgentRouting: "021000021"`, `CreditorAgentRouting: "121000358"`, and defaults `Status: "SETTLED_INSTANT"` regardless of content | `gateway/instant_payment.go:59-65` |
| `gateway/compliance.go:46-56` | `TenantIdentifier: "SENTINEL-TENANT-MERIDIAN-CUSTODY-001"` and `AuditEngineVersion: "Sentinel-Merkle-Chain-v1.0"` are struct literals, not derived from a request/tenant | `gateway/compliance.go:46,48` |
| `gateway/webhook.go` via `main.go:660` | New webhook secret generated as `"whsec_" + fmt.Sprintf("%x", time.Now().UnixNano())` — not cryptographically random | `gateway/main.go:660` |
| `gateway/main.go:513` | `POST /incidents/{id}/approve` defaults `Actor` to hardcoded `"TREASURY_SUPERVISOR_01"` when the caller omits it, then writes that into the audit ledger as if authenticated | `gateway/main.go:509-513` |
| `gateway/healing.go:184-188` | `POST /healing/apply` echoes back caller-supplied `supervisorId` and `proposalId` with no verification either was real | `gateway/healing.go:165-188` |

### 2.3 "Merkle" claim — contradicts its own fix log

`docs/AUDIT_FIXES.md:97` states *"'Merkle' removed from claims until a history tree exists."*
This is false against the current checkout. `Merkle` still appears as a live claim in:
`gateway/compliance.go:48` (`AuditEngineVersion`), `gateway/metrics.go:83`
(Prometheus metric literally named `sentinel_merkle_chain_height`), `gateway/connector.go:283`,
`gateway/agent_swarm.go:132,138`, and in eight UI components including
`src/components/AuditLedgerModal.tsx:172`, `src/components/ExecutiveDeckModal.tsx:65,84,180,191`,
`src/components/SqlConsoleModal.tsx:29,159`, `src/components/SelfHealingModal.tsx:351`,
`src/components/VaultAndInstantPaymentsModal.tsx:470,637`, `src/components/AgentSwarmModal.tsx:81,256`,
and `README.md:37`. `docs/RESEARCH_FOUNDATIONS.md:279-307` itself explains why the claim is
wrong (no membership/consistency proofs exist — it's a hash chain). This is a genuine
documentation-vs-code contradiction: the remediation log claims a fix that was not applied
repo-wide.

---

## 3. Production routes/screens backed by synthetic data, or that silently fall back to it

- `src/services/api.ts:87-88` — `getSlaBoard()` catches a backend failure and
  `console.warn('Backend SLA Board unavailable, using local mock state', e)`, then presumably
  renders local mock state (caller-side fallback, not an explicit `UNAVAILABLE` UI state).
- `src/services/api.ts:98-99` — same pattern for `getIncidents()`
  (`"Backend incidents unavailable, using local mock state"`).
- `src/services/api.ts:77,109` — two more bare `catch {}` blocks with no re-throw, same
  silent-degrade shape (need per-call confirmation of what each falls back to; flagged here
  as a pattern, not individually traced).
- `gateway/connector.go` (`RegisterIntegrationHubRoutes`) — `/hub/connections`,
  `/hub/assets`, `/hub/lineage` serve hardcoded catalog/asset/lineage data, not a real
  connector (matches Recovery Plan §"Integration Hub is a mock").
- `src/mockData/syntheticCorpus.ts`, `src/mockData/generator.ts` — used by
  `FileDiffModal.tsx:14`, `UploadModal.tsx:14`, `App.tsx:37` to generate sample/preset NACHA
  content for demo buttons. This is labelled sample data at the point of use (not a silent
  production fallback) — lower severity than the api.ts pattern above, but still worth an
  explicit `DEMO_DATA` banner per the engineering contract once Prompt 01 runs.
- Every one of the 17 modals in §1.4 (`BenchmarkModal`, `ExecutiveDeckModal`, `ChaosMonkeyModal`,
  `SqlConsoleModal`, `IntegrationHubModal`, `AgentSwarmModal`, `SelfHealingModal`,
  `VaultAndInstantPaymentsModal`, …) presents fabricated-or-partially-fabricated backend
  behavior (§2) as if it were live infrastructure state, with no visible "synthetic/demo"
  labelling in the UI chrome. `UNKNOWN`: whether any of these components render a disclaimer
  string client-side — not traced component-by-component in this pass.

---

## 4. Where raw financial data, credentials, tokens, or secrets can be stored, returned, logged, queried, or sent externally

| Vector | Evidence | Status |
|---|---|---|
| `GET /api/v1/webhooks` returns `secret` in plaintext JSON | `gateway/main.go:626-648`, `SELECT id, url, secret, …` then `json.NewEncoder(w).Encode(webhooks)` with `wh.Secret` populated | **Open** |
| `POST /api/v1/webhooks/test` calls any caller-supplied `URL` with no allowlist/private-range check | `gateway/main.go:685-711` | **Open (SSRF)** |
| `POST /api/v1/sql/query` — read-only handle + 10s timeout enforced (`docs/AUDIT_FIXES.md #5`, confirmed structurally: separate `roDB` opened `?mode=ro&_pragma=query_only(1)` at `gateway/main.go:101`) but can still `SELECT url, secret FROM webhook_subscriptions` and read any other table, including `file_instances.storage_path` / `validation_findings.raw_data` | **Partially open** — write-path fixed, read-path (including secrets and raw file evidence) still fully queryable |
| `POST /api/v1/vault/detokenize` | Guarded by `authorizeDetokenize` (`gateway/vault.go:269-280`) per `docs/AUDIT_FIXES.md #3` — not independently re-verified line-by-line in this pass beyond confirming the function exists and is called | `UNKNOWN` (mostly fixed per fix log; not re-audited in depth) |
| `gateway/processor.go` retains full raw file content in memory and (per Recovery Plan, not re-verified this pass) in findings | Not re-read this pass — carried over from `docs/engineering/SentinelFlow_Code_Audit_and_Recovery_Plan.md` §"Raw financial data is overexposed" | `UNKNOWN` — flagged for Prompt 07/Phase C re-check |
| AI tier LLM client | `ai-tier/llm_client.py` — not read this pass; Recovery Plan states it can send raw input to a model | `UNKNOWN` |
| `.env.example` | Contains only variable names and descriptions, no live credentials | **Clean** |

---

## 5. Route-by-route authentication and tenant-isolation matrix

**Authentication.** `gateway/main.go:167-191`: if `SENTINEL_API_TOKEN` is unset, `requireAuth`
is a no-op (`gateway/main.go:172-175`) and every route runs open, with only a log warning
(`gateway/main.go:169`). If set, `requireAuth` does a constant-time Bearer-token compare
(`gateway/main.go:177-179`) and is applied via `r.Use(requireAuth)` at `gateway/main.go:191`
to the entire `/api/v1` subrouter — i.e. **one shared token for every route, every caller**,
not per-user/per-tenant identity. `GET /metrics` (`main.go:187`) is mounted before this and
is never authenticated, by any configuration.

**Tenant isolation.** `gateway/migrations/01_init.sql` — the only migration — defines
`partners`, `file_contracts`, `expectations`, `file_instances`, `incidents`,
`validation_findings`, `audit_events`. **None of the seven tables has a `tenant_id` or
`workspace_id` column.** Every "tenant" value seen in the code (`compliance.go:47`,
`vault.go:91,98`, `main.go:702`, `webhook.go:28`) is either a hardcoded string literal or a
caller-supplied field trusted without verification — there is no enforcement point anywhere
in the repository (grep of `gateway/*.go` for `tenant` outside `_test.go` files, §2.2/§4).

| Route group | Auth | Tenant scoping |
|---|---|---|
| `GET /metrics` | None, unconditionally | N/A |
| Everything under `/api/v1/*` (41 routes, §1.3) | Single shared bearer token if `SENTINEL_API_TOKEN` is set; **fully open if unset** | **None** — no schema column, no query-level filter, no per-tenant identity derivation anywhere |

**Frontend cannot use auth even when configured.** `src/services/api.ts` makes 11 `fetch()`
calls (lines 74, 84, 95, 106, 115, 128, 139, 151, 163, 169, 175) and none of them sets an
`Authorization` header (grep-confirmed: zero matches for `Authorization` in the file). If an
operator sets `SENTINEL_API_TOKEN` in production, the shipped UI breaks — every request
returns 401. The UI is only functional in the fully-unauthenticated configuration.
`API_BASE_URL` is also a compile-time literal, `'http://localhost:8080/api/v1'`
(`src/services/api.ts:6`), not runtime configuration.

**Actor identity.** `POST /incidents/{id}/approve` (`gateway/main.go:504-525`) and
`POST /healing/apply` (`gateway/healing.go:165-188`) both take actor/supervisor identity from
the request body, not from any authenticated principal — there is no authenticated-principal
concept anywhere in this codebase beyond the single shared bearer token.

---

## 6. Build/runtime dependency graph and exact versions

| Component | Declared version | Container version | Match? |
|---|---|---|---|
| Go (`gateway/go.mod:3`) | `go 1.26.4` | `Containerfile.gateway:2` — `golang:1.22-alpine` | **No.** Local build succeeded only because Go's `GOTOOLCHAIN=auto` transparently downloaded 1.26.4 over the network (`go version` printed `go1.26.4` after `go build` in this sandbox, `go: downloading go1.26.4` on stdout). Inside `golang:1.22-alpine` this same auto-download would need network egress during the container build — undocumented, fragile, and contradicts Prompt 02's "pin one supported Go version across go.mod, CI, containers, and docs." |
| Go (CI) | `.github/workflows/ci.yml:20` sets up Go **1.22** explicitly | — | Same mismatch as above, third inconsistent value alongside go.mod's 1.26.4 |
| Node (`package.json` has no engines field) | — | `Containerfile.ui:2` — `node:20-alpine`; CI (`ci.yml` ui-build job) sets up Node **20** | Consistent with each other; local sandbox has Node 22.22.2, untested against this repo's pin |
| Python (`ai-tier/.python-version`) | `3.12` | `Containerfile.ai:2` — `python:3.11-slim`; CI sets up Python **3.11** | **No** — `.python-version` says 3.12, container and CI use 3.11 |
| Go module deps | `gateway/go.mod:5-29` — 24 indirect requires (moov-io/ach v1.63.3, ProtonMail/go-crypto v1.4.1, jackc/pgx/v5 v5.10.0 present despite no Postgres runtime use, mattn/go-sqlite3 + modernc.org/sqlite both present) | — | Two SQLite drivers declared (`mattn/go-sqlite3` cgo-based, `modernc.org/sqlite` pure-Go); `Containerfile.gateway` builds `CGO_ENABLED=0`, so only the modernc driver can actually be linked into the shipped binary — the mattn dependency is dead weight or a latent build trap if code ever imports it directly. `pgx` (Postgres driver) is declared but `gateway/main.go:86` opens `sql.Open("sqlite", …)` — Postgres driver is unused. |
| Node deps | `package.json:11-15` — react 18.3.1, vite 6.0.0 (installed: 6.4.3), typescript 5.5.3 | — | `npm audit` reports 1 high-severity transitive vulnerability (`nanoid <3.3.18`, GHSA-2v37-7h3g-55p8, via a dependency of an installed package) |
| Python deps | `ai-tier/pyproject.toml:6-11` — fastapi, langchain, openai, pydantic, uvicorn, `requires-python = ">=3.12"` | No lock file. **`ai-tier/requirements.txt` does not exist** (`ls` confirmed) | `Containerfile.ai:14` runs `COPY ai-tier/requirements.txt ./` — **this container build will fail** at that COPY step. This is the exact defect flagged in the original Recovery Plan and in Prompt 02 ("create the missing Python dependency manifest") and it is still unfixed. |
| License | README badge claims MIT (`README.md:9`) | No `LICENSE` file anywhere in the repo root | Still unfixed, matches original audit finding |
| Root compose (`podman-compose.yml`) | Defines `ai-tier`, `gateway`, `ui` only | — | No PostgreSQL, no MinIO/S3, no SFTPGo — despite README's architecture claims and the guide's recommended stack |
| Secondary compose (`gateway/docker-compose.yml`) | Defines `postgres:15-alpine` and `minio/minio` | — | Neither is wired to the gateway container network in `podman-compose.yml`, and the gateway itself opens SQLite (`gateway/main.go:86`) — this compose file describes infrastructure the running application does not use |

---

## 7. Documentation-vs-code contradictions found

1. **`docs/AUDIT_FIXES.md:97`** claims "Merkle" was removed from all claims. False — see §2.3,
   still present in 5 Go files and 6 UI components plus the README.
2. **`docs/AUDIT_FIXES.md`'s P2 section** ("Foreign project in `src/`... Moved to
   `.removed-davinci/`") is false against this checkout. `find` confirms no
   `.removed-davinci/` directory exists anywhere in the repo. `src/main.js`,
   `src/physics/PhysicsWorld.js`, `src/audio/SoundEngine.js`,
   `src/renderer/CanvasRenderer.js`, `src/ui/CodexUI.js`, `src/machines/MachinePresets.js`,
   `src/diagnostics/FailureAnalyzer.js`, and `src/style.css` are all still present in `src/`.
   Separately verified as genuinely dead: `index.html:16` loads `/src/main.tsx` (the real
   React entry), not `/src/main.js` — so this is unreachable code sitting in the tree, not a
   live parallel app, but the fix log's claim that it was moved out is incorrect.
3. **README performance section** (`README.md`, "⚡ Performance") has already been
   self-corrected in-place to say figures were removed pending real measurement and points to
   `docs/AUDIT_FIXES.md` — this one is accurate and should be treated as a model for how the
   rest of the README should read after Prompt 01.
4. **README badges** (`README.md:1-9`) still advertise "Self-Healing," "Multi-Agent" swarm,
   and "Compliance-SEC 17a-4 | SOX 404" as headline features with links straight to
   `gateway/healing.go`, `gateway/agent_swarm.go`, `gateway/compliance.go` — the same files
   the Recovery Plan requires removed or clearly relabeled from version one. The README does
   not currently distinguish implemented/experimental/planned per Prompt 01 §"vocabulary
   rules."

---

## 8. Tests mapped to production behavior

Full suite: **38/38 pass** (§9.3) — this number matches both the README badge and
`docs/AUDIT_FIXES.md`'s "Final state" table, so that specific claim is currently accurate.

Coverage gaps found while cross-referencing tests against the P0 items in §2:

- **`gateway/vault_test.go:35` (`TestInstantPaymentFedNowValidation`)** feeds
  `ValidateInstantPaymentXml` a well-formed, realistic sample XML
  (`vault_test.go:36-50`) whose `IntrBkSttlmAmt` happens to be `150000.00` — the same value
  the function hardcodes regardless of input (§2.2). The test only asserts `tx.Network` and
  that latency is under the SLA threshold (`vault_test.go:54-60`); it never asserts the
  amount, routing numbers, or IDs in the response actually came from the parsed XML, and it
  never tests an empty or malformed payload. This is the same "test encodes the bug" pattern
  `docs/AUDIT_FIXES.md` itself flagged and fixed in `security_test.go` and `integrity_test.go`
  (items 1 and 6) — it was not applied here.
- No test in `gateway/connector_test.go` (`TestIntegrationHubSanitizedConnections`,
  `TestCatalogAssetsAndMaskedPreview`, `TestDataLineageGraph`) asserts on `mTLSVerified`, so
  the hardcoded `true` at `connector.go:394` has no regression coverage either way.
- No test exercises `POST /incidents/{id}/approve` or `POST /healing/apply` with a forged or
  missing actor/supervisor field to confirm current (absent) rejection behavior.
- No test exercises cross-tenant access, because there is no tenant concept to test (§5).

---

## 9. Verification commands run (exact results)

Environment: Go toolchain auto-resolved to `go1.26.4` (base image had `go1.24.7`; go.mod's
`go 1.26.4` directive triggered `GOTOOLCHAIN=auto` download — network egress was available in
this environment). Node `v22.22.2` / npm `10.9.7`. Python `3.11.15`.

### 9.1 Go

| Command | Result |
|---|---|
| `cd gateway && go build ./...` | **PASS** (exit 0, after toolchain/module auto-download) |
| `go vet ./...` | **PASS** (exit 0, no output) |
| `go test ./...` | **PASS** — `ok sentinel-gateway 0.141s`, 38/38 tests pass (full list captured; representative names: `TestMultiAgentSwarmDeliberation`, `TestHashChainTamperDetection`, `TestE2E_ValidNachaPipeline`, `TestRobustDetectorResistsMasking`, `TestInstantPaymentFedNowValidation`, `TestWebhookHmacComputationAndDispatch`) |
| `go test ./... -race` | **PASS** — `ok sentinel-gateway 1.438s` |

### 9.2 TypeScript / frontend

| Command | Result |
|---|---|
| `npm ci` | **PASS** — 74 packages installed; **1 high-severity vulnerability** reported (`nanoid <3.3.18`, GHSA-2v37-7h3g-55p8) |
| `npx tsc --noEmit` | **PASS** (exit 0, no output) |
| `npx vite build` | **PASS** — `✓ 1830 modules transformed`, `dist/assets/index-*.js 404.38 kB │ gzip: 99.45 kB`, built in 1.71s. Two benign warnings about `lucide-react`'s `"use client"` directive being ignored (Vite bundling a package written for a different bundler; no functional impact observed) |

### 9.3 Python / AI tier

| Command | Result |
|---|---|
| `python3 -c "import fastapi"` | **FAIL** — `ModuleNotFoundError: No module named 'fastapi'` (no dependencies installed; no lock/requirements file to install from) |
| `python evals/runner.py` | **NOT RUN** — blocked by the missing `fastapi` install above, itself blocked by the missing `ai-tier/requirements.txt` (§6). `docs/AUDIT_FIXES.md`'s claim that a deliberately vulnerable stub agent scores 0.0% was not independently re-verified in this pass. |

### 9.4 Containers

| Command | Result |
|---|---|
| `podman-compose build` / `docker compose build` | **NOT RUN** — not attempted this pass. Given `Containerfile.ai:14` copies a file that does not exist in the repo (§6), this build is expected to fail at that step; not empirically confirmed here. |

### 9.5 Migrations

| Command | Result |
|---|---|
| Migrate empty DB, upgrade fixture, restart-persistence test | **NOT RUN** — no migration tool/command is defined in the repo beyond the single static `gateway/migrations/01_init.sql`; no versioned up/down migration framework exists to exercise. |

### 9.6 Security/dependency scanning

| Command | Result |
|---|---|
| `npm audit` | **PASS (ran)** — 1 high-severity finding, see §9.2 |
| Go dependency/vuln scan (`govulncheck` or similar) | **NOT RUN** — tool not installed, not attempted |
| Secret scanning (gitleaks/trufflehog or similar) | **NOT RUN** — tool not installed, not attempted |
| Container image scanning | **NOT RUN** — no image was built (§9.4) |

---

## 10. Proposed deletion/change list

### P0 — before anything else ships

1. `gateway/connector.go:394` — remove the hardcoded `mTLSVerified: true`; derive from real
   verified transport state or remove the field/claim entirely.
2. `edge-agent/main.go` — remove the "mTLS" language and default-`http://` client, or
   implement real mutual TLS; do not log success language ("Outbound mTLS metadata sync
   successful") for a plaintext HTTP call.
3. `gateway/instant_payment.go:36-77` (`ValidateInstantPaymentXml`) — either remove the
   module from the shipped build or stop hardcoding amount/routing/status and stop defaulting
   to `SETTLED_INSTANT`.
4. `gateway/main.go:509-513` and `gateway/healing.go:165-188` — stop trusting
   caller-supplied `Actor`/`SupervisorID`; there is no authenticated-principal concept to
   source it from yet, so this needs Prompt 04 (auth) before it can be fixed correctly, not a
   local patch.
5. `gateway/main.go:626-648` — stop returning `secret` from `GET /webhooks`.
6. `gateway/main.go:685-711` — remove or allowlist-restrict `POST /webhooks/test`'s
   arbitrary destination URL (SSRF).
7. `gateway/main.go:660` — replace `time.Now().UnixNano()` webhook-secret generation with a
   CSPRNG.
8. Tenant isolation (§5) — no `tenant_id` column exists anywhere; this is a schema and
   query-layer gap, not a one-line fix, and blocks the "Every business record is
   tenant-scoped" invariant in the new `docs/engineering/claude.md` contract outright.
9. Auth (§5) — `SENTINEL_API_TOKEN` unset still runs fully open with only a log warning
   (`gateway/main.go:167-169`); should refuse to start outside an explicitly named local
   demo profile, per the new contract's invariant 6. Frontend sends no `Authorization`
   header at all (`src/services/api.ts`), so fixing the backend alone will break the UI —
   both need to move together.
10. `ai-tier/requirements.txt` is missing; `Containerfile.ai` cannot build without it.

### P1

11. `gateway/ledger.go:34-73` (`AppendAuditEvent`) — select-then-insert with no transaction
    or lock; concurrent callers can fork the chain. Tamper *detection* was fixed
    (`docs/AUDIT_FIXES.md #6`); concurrent-append safety was not (Recovery Plan finding, still
    accurate).
12. `gateway/go.mod:3` vs `Containerfile.gateway:2` vs `.github/workflows/ci.yml:20` — three
    different Go versions (1.26.4 / 1.22 / 1.22). Pin one.
13. `ai-tier/.python-version` (3.12) vs `Containerfile.ai:2`/CI (3.11) — pin one.
14. `gateway/docker-compose.yml` (Postgres + MinIO) is disconnected from what the app
    actually opens (`sql.Open("sqlite", …)` at `gateway/main.go:86`) and from
    `podman-compose.yml`, which doesn't include either service. Provide one authoritative
    stack or remove the unused compose file.
15. `nanoid` high-severity transitive vulnerability (`npm audit`, §9.2) — run `npm audit fix`
    or pin the upstream package that pulls it in.
16. `src/services/api.ts:6` — `API_BASE_URL` is a compile-time literal; needs runtime
    configuration before any non-localhost deployment is possible.
17. Silent frontend mock fallback on backend failure (`src/services/api.ts:87-88,98-99` and
    two more bare `catch {}` blocks) — replace with an explicit unavailable/degraded UI
    state per the engineering contract.
18. No `LICENSE` file despite the MIT badge in `README.md:9`.
19. No CI job runs `go vet`, `-race`, `npm audit`, or any secret/dependency scanner —
    `.github/workflows/ci.yml` only runs `go test`, the Python eval, and the frontend build.

### P2

20. README badges (§7.4) overstate several features as headline capabilities; needs the
    implemented/experimental/planned split from Prompt 01.
21. Dead foreign frontend project still physically present in `src/` (§7.2) — genuinely
    unreachable (confirmed via `index.html`) but contradicts `docs/AUDIT_FIXES.md`'s claim
    that it was moved; either actually remove it or correct the fix log.
22. `gateway/go.mod` declares both `mattn/go-sqlite3` (cgo) and `modernc.org/sqlite`
    (pure-Go) as dependencies while `Containerfile.gateway` builds `CGO_ENABLED=0` — the cgo
    driver can't be linked into the shipped binary; likely dead weight.
23. `pgx/v5` (Postgres driver) is a declared dependency with no corresponding
    `sql.Open("postgres", …)` call anywhere found in this pass — `UNKNOWN` whether it's used
    indirectly; worth confirming before Prompt 02's Postgres migration.
24. `GET /api/v1/health` is mounted inside the auth-required `/api/v1` group
    (`gateway/main.go:190-201`) — an orchestrator health check would need the bearer token,
    which is atypical and worth a deliberate decision rather than an accident.

### Existing user work to preserve

- `docs/AUDIT_FIXES.md`'s P0/P1 fixes (§2.1) are real, tested, and should not be reverted.
- The KS-test (`gateway/kstest.go`) and robust-anomaly (`gateway/robust_anomaly.go`)
  implementations are genuine, tested statistical code, explicitly called out as worth
  keeping in the Recovery Plan.
- `.env.example`, `.gitignore`, and the auth-middleware skeleton in `main.go:156-191` are a
  reasonable foundation for Prompt 04, not something to throw away.
- The README's already-corrected Performance section (§7.3) is a good template for the rest
  of the document.

---

## 11. Explicitly out of scope for this pass

Per the guide's Prompt 00 instructions, no code was changed. Not independently re-verified
this pass (carried over from the Recovery Plan and flagged `UNKNOWN` where stated):
`gateway/processor.go`'s full-file-read/raw-content-retention behavior, `ai-tier/llm_client.py`
and `ai-tier/agent_hub_tools.py`, the NACHA/ISO 20022/BAI2/SWIFT parser internals beyond what
the existing test names indicate, and the SQL-console blocklist's current exact keyword list.
