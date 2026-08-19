# Sentinel Flow — Current State Baseline (Prompt 00)

**Audit date:** 14 August 2026
**Auditor:** Claude Code (read-only baseline pass)
**Commit audited:** `d447d66` on branch `claude/sentinel-flow-engineering-contract-ilsqlf`
**Working tree at audit time:** clean (`git status --short` empty)

This document is the read-only baseline required by Prompt 00 of
`SENTINELFLOW_PRODUCTION_READY_PROMPT_GUIDE.md`. **No production code was changed in this
task.** Every claim below carries a `file:line` reference or a reproduced runtime request.
Items that could not be determined are labelled **UNKNOWN**.

---

## 1. Method and limits

What was done:

- Static inspection of all 115 tracked files.
- A clean **source build** of the Go gateway (`go build ./...`) and execution of that
  freshly built binary against a throwaway SQLite database in a scratch directory. The
  committed binary `gateway/sentinel-gateway` was **not** used as evidence of source
  behaviour.
- Execution of every verification command the repository documents.

Limits:

- No PostgreSQL, MinIO/S3, SFTPGo, OIDC provider, secret manager, or payment rail was
  available. Claims that depend on them are assessed from code only.
- Container images were not built (`podman` absent; `docker` present but the Containerfiles
  were not built — see §2, marked NOT RUN with reason).
- No load or memory profiling was performed; this pass makes no performance claim.

**One procedural note for the record:** running `go build ./...` inside `gateway/` writes
its output to `gateway/sentinel-gateway`, which is the path of the committed binary, so the
committed artifact was overwritten as a side effect. It was restored with
`git checkout -- gateway/sentinel-gateway` and the tree verified clean. This is itself a
finding: a committed build artifact sits at the exact path the build system clobbers.

---

## 2. Verification command results

Commands are the ones documented in `README.md`, `.github/workflows/ci.yml`, and
`docs/AUDIT_FIXES.md`.

| # | Check | Command | Result | Evidence |
|---|---|---|---|---|
| 1 | Go build | `cd gateway && go build ./...` | **PASS** (see caveat) | exit 0, no output |
| 2 | Go vet | `cd gateway && go vet ./...` | **PASS** | exit 0, no output |
| 3 | Go tests | `cd gateway && go test ./...` | **PASS** | `ok sentinel-gateway 0.154s` |
| 4 | Go race tests | `go test -race ./...` | **NOT RUN** | Not documented anywhere in the repo or CI; no pinned command exists to run. |
| 5 | TypeScript typecheck | `npx tsc --noEmit` | **PASS** | exit 0, no output |
| 6 | Frontend production build | `npm ci && npm run build` | **PASS** | `✓ 1830 modules transformed`, `dist/assets/index-C5_6t_RK.js 404.38 kB` |
| 7 | Python adversarial evals | `cd ai-tier && python3 evals/runner.py` | **FAIL (exit 2)** | see below |
| 8 | Python lint / typecheck | — | **NOT RUN** | No linter, formatter, or type checker is configured for `ai-tier/`. |
| 9 | Migration up/down/upgrade | — | **NOT RUN** | No migration tool or command exists. `gateway/migrations/01_init.sql` is applied opportunistically at startup (`gateway/main.go:114-122`) with no version table and no down path. |
| 10 | Container build | `podman-compose build` | **NOT RUN** | `podman` and `podman-compose` are not installed. `docker` is available but `Containerfile.ai` cannot succeed — see §7 defect B2. |
| 11 | Clean-stack smoke test | `./start.sh` / `podman-compose up` | **NOT RUN** | Requires container runtime above; no stack definition includes the database the code opens. |
| 12 | Secret scanning | — | **NOT RUN** | No secret scanner configured; `gitleaks`/`trufflehog` not installed. |
| 13 | Dependency scan (npm) | `npm audit` | **FAIL (1 high)** | `nanoid <3.3.18` — GHSA-2v37-7h3g-55p8 |
| 14 | Dependency scan (Go/container) | `govulncheck` / `trivy` | **NOT RUN** | Neither tool installed; no pinned command in repo. |

### Caveat on check #1 (Go build)

The build passes **only because the sandbox has network egress and `GOTOOLCHAIN` defaults
to `auto`.** The local Go is `go1.24.7`; `gateway/go.mod:3` declares `go 1.26.4`, so the
toolchain silently downloaded go1.26.4 mid-build:

```
go: downloading go1.26.4 (linux/amd64)
```

With `GOTOOLCHAIN=local`, an air-gapped builder, or the pinned CI/container Go (both 1.22),
this build **fails**. The build is therefore not reproducible as pinned. See §4.

### Detail on check #7 (Python evals)

```
$ cd ai-tier && python3 evals/runner.py
{
  "status": "NOT_RUN",
  "error": "No system under test is wired up. Refusing to emit a pass rate. Pass a callable, or make swarm.execute_multi_agent_swarm importable."
}
exit 2
```

The refusal itself is **correct behaviour** and matches `docs/AUDIT_FIXES.md` §12 — the
eval fails closed instead of fabricating a score. But two things follow:

- `README.md:111` documents *"Expected Output: 5/5 adversarial attacks contained (100% pass
  rate)"* — an output the code now deliberately refuses to produce.
- `.github/workflows/ci.yml:52` runs this exact command as a required CI job. **The
  `ai-evals` job fails on every push to `main`.** Whether it is currently red on the remote
  is **UNKNOWN** (CI run history not inspected in this pass).

---

## 3. Toolchain and version matrix

| Component | Declared where | Version | Conflict |
|---|---|---|---|
| Go | `gateway/go.mod:3` | `1.26.4` | — |
| Go | `Containerfile.gateway:2` | `golang:1.22-alpine` | **Conflicts with go.mod** |
| Go | `.github/workflows/ci.yml:24` | `1.22` | **Conflicts with go.mod** |
| Go | local sandbox | `go1.24.7` | Conflicts; masked by toolchain auto-download |
| Python | `ai-tier/pyproject.toml:6` | `>=3.12` | — |
| Python | `ai-tier/.python-version` | `3.12` | — |
| Python | `Containerfile.ai:2` | `python:3.11-slim` | **Conflicts with pyproject** |
| Python | `.github/workflows/ci.yml:44` | `3.11` | **Conflicts with pyproject** |
| Python | local sandbox | `3.11.15` | Conflicts with pyproject |
| Node | `.github/workflows/ci.yml:71` | `20` | — |
| Node | local sandbox | `22.22.2` | Unpinned locally; no `.nvmrc` or `engines` field |

There is **no single pinned version of any runtime** that all four surfaces agree on.

**Python dependencies are not locked and not installable as declared.**
`ai-tier/pyproject.toml:8-14` requires `fastapi`, `langchain>=1.3.15`, `openai>=3.0.0`,
`pydantic`, `uvicorn`. CI installs only `fastapi uvicorn pydantic requests`
(`.github/workflows/ci.yml:49-51`) — `langchain` and `openai` are never installed, and
`requests` is installed but not declared. There is no `requirements.txt` and no lock file.

---

## 4. Module inventory

### Go gateway — `gateway/` (single `package main`, 24 non-test files)

| File | Lines | Responsibility |
|---|---:|---|
| `main.go` | 862 | Router, auth middleware, CORS, 20 inline route handlers |
| `connector.go` | 398 | "Integration Hub" — in-memory catalog, edge sync |
| `vault.go` | 380 | Tokenisation, AES-GCM at rest, detokenize authz |
| `processor.go` | 304 | File ingestion, NACHA validation, persistence |
| `security.go` | 227 | SSH key parsing, PGP detached-signature verification |
| `kstest.go` | 220 | Two-sample Kolmogorov–Smirnov test, Benjamini–Hochberg |
| `agent_swarm.go` | 219 | Scripted 4-agent transcript |
| `healing.go` | 196 | "Self-healing" proposal + apply |
| `robust_anomaly.go` | 195 | Median/MAD modified z-score detector |
| `iso20022.go` | 165 | ISO 20022 XML projection parser |
| `benchmark.go` | 165 | In-memory fixed-width scan benchmark |
| `ledger.go` | 142 | Audit hash chain append + verification |
| `generator.go` | 140 | Synthetic NACHA scenario generator |
| `metrics.go` | 95 | Prometheus text exposition |
| `instant_payment.go` | 115 | FedNow/RTP simulation |
| `compliance.go` | — | SEC 17a-4 / SOX 404 export package |
| `webhook.go` | — | HMAC signing + outbound dispatch |
| `watcher.go` | — | Polling inbox daemon |
| `failover.go` | — | Scripted DR failover |
| `drift.go` | — | Drift report envelope |
| `anomaly.go` | — | 3σ z-score on constant baseline |
| `swift.go`, `bai2.go` | — | SWIFT MT / BAI2 parsers |
| `stream.go` | — | Server-Sent Events endpoint |

### Python AI tier — `ai-tier/`

`main.py` (7 FastAPI routes), `swarm.py`, `llm_client.py`, `agent_hub_tools.py`,
`evals/runner.py`, `evals/adversarial_dataset.json`. **No `requirements.txt`.**

### Edge agent — `edge-agent/main.go`

Single file, **no `go.mod`** — it is not part of any Go module and is not built by CI or by
any Containerfile. It is unbuildable as committed.

### React UI — `src/`

All 17 modal components are imported by `src/App.tsx:2-19` and therefore ship in the
production bundle. There is no route-level or build-level gating of any screen.

**Dead/foreign code still tracked:** `src/physics/PhysicsWorld.js`, `src/ui/CodexUI.js`,
`src/machines/MachinePresets.js`, `src/audio/SoundEngine.js`,
`src/diagnostics/FailureAnalyzer.js`, `src/renderer/CanvasRenderer.js`, `src/main.js`,
`src/style.css` — a separate "DaVinci Codex Sandbox" project (~2,100 lines) unreachable
from `src/main.tsx`. `docs/AUDIT_FIXES.md:183-187` claims these were "Moved to
`.removed-davinci/`". **They are still tracked at their original paths.**

---

## 5. Route inventory with authentication and tenant-isolation matrix

`requireAuth` (`gateway/main.go:171-184`) wraps the whole `/api/v1` subtree
(`gateway/main.go:191`). Its behaviour: **if `SENTINEL_API_TOKEN` is empty, it calls
`next.ServeHTTP` and every route is public** (`gateway/main.go:173-176`), logging one
warning line at startup (`gateway/main.go:169`).

Tenant column count in the schema: **zero**. `gateway/migrations/01_init.sql` defines
`partners`, `file_contracts`, `expectations`, `file_instances`, `incidents`,
`validation_findings`, `audit_events` — none has a `tenant_id`. Therefore **every route
below is tenant-isolation NONE**, and the column is marked accordingly without repetition.

| Method | Path | Handler | Auth when token set | Auth when token unset | Tenant scope |
|---|---|---|---|---|---|
| GET | `/metrics` | `main.go:187` | **NONE — registered outside `/api/v1`** | NONE | n/a |
| GET | `/api/v1/health` | `main.go:201` | Bearer | **public** | none |
| GET | `/api/v1/sla-board` | `main.go:207` | Bearer | **public** | none |
| GET | `/api/v1/partners` | `main.go:244` | Bearer | **public** | none |
| POST | `/api/v1/partners` | `main.go:267` | Bearer | **public** | none |
| GET | `/api/v1/contracts` | `main.go:287` | Bearer | **public** | none |
| GET | `/api/v1/incidents` | `main.go:318` | Bearer | **public** | none |
| GET | `/api/v1/ledger` | `main.go:373` | Bearer | **public** | none |
| POST | `/api/v1/files/ingest-raw` | `main.go:384` | Bearer | **public** | none |
| POST | `/api/v1/files/upload` | `main.go:405` | Bearer | **public** | none |
| POST | `/api/v1/incidents/{id}/triage` | `main.go:430` | Bearer | **public** | none |
| POST | `/api/v1/incidents/{id}/approve` | `main.go:504` | Bearer | **public** | none |
| GET | `/api/v1/generator/sample` | `main.go:529` | Bearer | **public** | none |
| GET | `/api/v1/benchmark/run` | `main.go:540` | Bearer | **public** | none |
| GET | `/api/v1/evals/run` | `main.go:554` | Bearer | **public** | none |
| GET | `/api/v1/compliance/export` | `main.go:577` | Bearer | **public** | none |
| POST | `/api/v1/security/verify-key` | `main.go:589` | Bearer | **public** | none |
| POST | `/api/v1/security/verify-signature` | `main.go:607` | Bearer | **public** | none |
| GET | `/api/v1/webhooks` | `main.go:626` | Bearer | **public** | none |
| POST | `/api/v1/webhooks` | `main.go:652` | Bearer | **public** | none |
| POST | `/api/v1/webhooks/test` | `main.go:685` | Bearer | **public** | none |
| POST | `/api/v1/sql/query` | `main.go:721` | Bearer | **public** | none |
| GET | `/api/v1/analytics/anomalies` | `main.go:798` | Bearer | **public** | none |
| POST | `/api/v1/chaos/trigger` | `main.go:808` | Bearer | **public** | none |
| GET | `/api/v1/hub/connections` | `connector.go:302` | Bearer | **public** | none |
| GET | `/api/v1/hub/assets` | `connector.go:310` | Bearer | **public** | none |
| GET | `/api/v1/hub/assets/{id}/sample` | `connector.go:318` | Bearer | **public** | none |
| GET | `/api/v1/hub/lineage` | `connector.go:366` | Bearer | **public** | none |
| POST | `/api/v1/hub/edge/sync` | `connector.go:378` | Bearer | **public** | none |
| POST | `/api/v1/swarm/deliberate` | `agent_swarm.go:170` | Bearer | **public** | none |
| GET | `/api/v1/swarm/sessions` | `agent_swarm.go:190` | Bearer | **public** | none |
| GET | `/api/v1/swarm/sessions/{id}` | `agent_swarm.go:204` | Bearer | **public** | none |
| POST | `/api/v1/healing/propose` | `healing.go:147` | Bearer | **public** | none |
| POST | `/api/v1/healing/apply` | `healing.go:165` | Bearer | **public** | none |
| POST | `/api/v1/instant-payments/validate` | `instant_payment.go:84` | Bearer | **public** | none |
| GET | `/api/v1/instant-payments/metrics` | `instant_payment.go:105` | Bearer | **public** | none |
| POST | `/api/v1/chaos/failover/simulate` | `failover.go:67` | Bearer | **public** | none |
| GET | `/api/v1/analytics/drift` | `drift.go:105` | Bearer | **public** | none |
| GET | `/api/v1/stream` | `stream.go:65` | Bearer | **public** | none |
| POST | `/api/v1/vault/tokenize` | `vault.go:283` | Bearer | **public** | request-supplied `tenantId` |
| GET | `/api/v1/vault/policies` | `vault.go:302` | Bearer | **public** | none |
| POST | `/api/v1/vault/detokenize` | `vault.go:309` | Bearer **+ supervisor token** | **supervisor token still enforced** | request-supplied |

Notes:

- `/api/v1/vault/detokenize` is the **only** route with a second authorisation factor
  (`vault.go:268-280`), and it is the only one that fails closed when unconfigured. This is
  a genuine improvement and should be preserved.
- The only tenant identifier anywhere in a business path is `body.TenantID` read from the
  **request body** at `vault.go:285-293` — i.e. the caller declares its own tenant.
- Actor identity is likewise caller-supplied: `main.go:509-515` reads `body.Actor` and
  defaults it to the literal `"TREASURY_SUPERVISOR_01"`; `healing.go:169` reads
  `body.SupervisorID`. The UI hardcodes an actor string client-side at
  `src/App.tsx:365` (`'SUPERVISOR_OPERATOR_GATHU'`). This violates invariant 7.

### Runtime confirmation — authentication

With no `SENTINEL_API_TOKEN` configured, against a clean source build:

```
GET /api/v1/ledger -> HTTP 200
```

---

## 6. Hardcoded operational, security, compliance and performance results

Each row is a value returned to a caller that is a source constant rather than a
measurement or a verified runtime fact.

| Value returned | Location | Nature |
|---|---|---|
| `"mTLSVerified": true` | `connector.go:394` | **Security state from a constant.** No TLS state inspected. |
| `Status: "SETTLED_INSTANT"` | `instant_payment.go:60` | **Settlement fabricated** before any parsing. |
| `AmountUSD: 150000.00`, routings `021000021`/`121000358` | `instant_payment.go:57-59` | Payment values invented from nothing. |
| `"averageValidationLatency": "1.42 ms"` | `instant_payment.go:109` | Constant presented as a metric. |
| `"slaComplianceRate": "99.998%"` | `instant_payment.go:110` | Constant presented as compliance. |
| `"maxThroughputTps": 12500` | `instant_payment.go:111` | Constant presented as throughput. |
| `sentinel_worker_pool_active 8` | `metrics.go:94` | **Prometheus gauge is a literal.** No worker pool exists. |
| `BreachRiskPct = 98.4` / `12.5`, `CountdownMinutes = -15` / `23` | `main.go:229-235` | Predictive risk assigned by an `if` on status. |
| `passRatePct: 100.0`, `passedTests: 5`, `averageLatencyMs: 14.2` | `main.go:565-573` | **Fabricated eval success** when the Python tier is unreachable. |
| `Confidence: 0.94` + invented Nacha citations | `main.go:468-480` | **Fabricated AI analysis** when the Python tier is unreachable. |
| `ConfidenceScore: 0.995` | `healing.go:135` | Repair confidence is a literal. |
| `ConfidenceScore: 0.978`, `ReportID: "DRIFT-REP-20260814"`, `PartnerName: "Meridian Custody Bank"` | `drift.go:90-97` | Report envelope frozen even though `kstest.go` computes real statistics. |
| `Confidence: 0.96 … 1.00`, `"Consensus finalized (Confidence: 98.4%)"` | `agent_swarm.go:71-155` | Entire multi-agent transcript is a script. |
| `RpoSecondsTarget: 0.00`, `RtoMillisecondsTarget: 42.5` | `failover.go:54-55` | Now honestly labelled `*Target` with `IsScriptedDemo: true` (`failover.go:47`) and `StandbyHealthStatus: "NOT_PROVISIONED"` (`failover.go:58`). **Improved — retain the labelling pattern.** |
| `RegulatoryStandard: "SEC Rule 17a-4 / SOX 404 / FINRA Rule 4511 Tamper-Evident Recordkeeping"` | `compliance.go:45` | Regulatory assurance label with no external evidence. |
| `AuditEngineVersion: "Sentinel-Merkle-Chain-v1.0"` | `compliance.go:48` | **"Merkle" is inaccurate** — it is a linear hash chain (`ledger.go:34-75`). |
| `sentinel_merkle_chain_height` | `metrics.go:81-83` | Same misnomer in the metric name. |
| `RuleReference: "Nacha Operating Rules 2025, …"` on every finding | `processor.go:142,160,202,214,226`; `main.go:357` | Rule citations are string literals with no licensed rule source behind them. |
| Anomaly evaluation against `DefaultBaseline` with literals `15200, 1428800` | `main.go:799` | Production route evaluates a constant, not live volume. |

**Correctly de-fabricated (do not regress):** `metrics.go:26,88-90` — the streaming parse
rate now reports `-1` until genuinely measured. Runtime-confirmed:
`sentinel_streaming_parse_rate_records_per_sec -1`.

### Runtime confirmation — fabricated security and settlement

```
POST /api/v1/hub/edge/sync            (plain HTTP, no client certificate)
 -> {"edgeAgentId":"X","mTLSVerified":true,"status":"ACKNOWLEDGED", ...}

POST /api/v1/instant-payments/validate   {"payloadXml":""}
 -> {"isCompliant":true,"instantSlaMet":true,
     "transaction":{"status":"SETTLED_INSTANT","amountUsd":150000,
                    "debtorAgentRouting":"021000021", ...}}

GET /metrics
 -> sentinel_worker_pool_active 8
```

---

## 7. Fail-open ingestion — the primary release blocker

`processor.go:68-74` initialises every ingestion as **`Status: "RELEASED"`** and downgrades
only on a positive finding. The Moov ACH parser exception is recorded at
`processor.go:236-241` with `Severity: "WARNING"` and, critically, **does not set
`result.Status = "QUARANTINED"`** — unlike every other finding branch
(`processor.go:144, 162, 204, 216, 228`).

For an empty file the arithmetic branches are all skipped (`declaredBatchDebits > 0` is
false, `expectedEntryHash` is `""`), so the only finding produced is the WARNING, and the
file is released. `IsBalanced` is computed as `batchDebits == batchCredits`
(`processor.go:193`) — `0 == 0` — so an empty file is also reported balanced.

### Runtime confirmation

```
POST /api/v1/files/ingest-raw   {"filename":"empty.ach","content":""}
 -> {"fileId":1,"sizeBytes":0,"status":"RELEASED","isBalanced":true,
     "totalRecordsParsed":0,
     "findings":[{"code":"ACH_ERR_0099_PARSER_EXCEPTION",
                  "description":"Moov ACH Parser reported: none or more than one file headers exists…",
                  "severity":"WARNING"}]}
```

**A zero-byte file is RELEASED and reported balanced.** This violates invariants 1, 2 and
12 and is the single highest-priority defect in the repository.

Related money-type defect: amounts are carried as `float64`
(`processor.go:34-35, 191-192`; `instant_payment.go:28`), violating the CLAUDE.md rule
against float money. Cent values are parsed as `int64` (`processor.go:171`) and only
divided into floats for presentation — the integer path is worth preserving.

---

## 8. Production surfaces backed by synthetic data

| Surface | Location | What is synthetic |
|---|---|---|
| `GET /api/v1/hub/connections` | `connector.go:120-207` | All 4 connections are a Go struct literal: hostnames, fingerprints, vault keys, latencies, `Status: "HEALTHY"`. |
| `GET /api/v1/hub/assets` | `connector.go:208-273` | 3 assets with invented row counts, byte sizes, `ValidationStatus: "COMPLIANT"`. |
| `GET /api/v1/hub/assets/{id}/sample` | `connector.go:337-359` | Preview rows are literals; nothing is read from any source. |
| `GET /api/v1/hub/lineage` | `connector.go:274-295` | Lineage edges are literals; `Transformation` text claims "Merkle Ledger Hash Commitment". |
| `POST /api/v1/swarm/deliberate` | `agent_swarm.go:60-160` | Full scripted transcript. |
| `POST /api/v1/chaos/failover/simulate` | `failover.go:40-60` | Scripted; **honestly labelled** `IsScriptedDemo: true`. |
| `GET /api/v1/analytics/anomalies` | `main.go:799` | Evaluates hardcoded volume against a constant baseline. |
| `GET /api/v1/analytics/drift` | `drift.go:90-97` | Real KS maths inside a frozen report envelope. |
| `POST /api/v1/instant-payments/validate` | `instant_payment.go:52-63` | Entire transaction invented. |
| UI — all 17 modals | `src/App.tsx:2-19` | Every modal ships in the production bundle with no demo gating. |
| UI — file diff / upload presets | `src/components/FileDiffModal.tsx:14`, `src/components/UploadModal.tsx:14` | Import from `src/mockData/`. |

**There is no `DEMO_DATA` banner or demo profile anywhere in the frontend.** A
case-insensitive search for `demo_data|DEMO DATA|demoMode` across `src/` returns zero
matches. Synthetic and real state are visually indistinguishable.

### Client-side validation authority

`src/App.tsx:39-42` imports `parseAndValidateNacha` (`src/parsers/nachaParser.ts`, 552
lines), `evaluateMissingFileIncident`, `runExceptionAnalyst`, and
`TamperEvidentEventStore`. The browser therefore contains a **second, independent
implementation** of NACHA validation, deadline evaluation, AI analysis, and the audit hash
chain. Any verdict rendered from these paths has no server evidence behind it. Which paths
the UI takes under which conditions was not traced exhaustively — **UNKNOWN**, and it
should be resolved before Prompt 12.

### Silent degradation

`src/services/api.ts:83-99` — `getSlaBoard()` and `getIncidents()` catch all errors, log
`"Backend … unavailable, using local mock state"`, and return `[]`. A backend outage is
indistinguishable from "no incidents". `checkHealth()` and `getLedger()`
(`src/services/api.ts:70-78, 101-108`) return `null` on failure with no typed error state.

---

## 9. Sensitive-data exposure map

| Path | Location | Exposure |
|---|---|---|
| Webhook secret returned on create | `main.go:676-681` | Secret in the POST response body. |
| Webhook secret returned on list | `main.go:627, 638` | `SELECT … secret …` returned to every caller of `GET /webhooks`. |
| Webhook secret readable via SQL console | `main.go:721-795` | Read-only handle prevents *writes*; it does not restrict *columns*. |
| Webhook secret generation | `main.go:663` | `"whsec_" + fmt.Sprintf("%x", time.Now().UnixNano())` — **time-derived, not cryptographically random, and guessable given an approximate creation time.** |
| Arbitrary outbound POST | `main.go:685-718` → `webhook.go:51-70` | Any caller-supplied URL is fetched. No scheme/host/IP allowlist, no private-range or metadata-IP block, no redirect limit. 5s timeout is the only bound. |
| Raw file content persisted in API response | `processor.go:40, 72` | `RawContent` carries the entire file. |
| Complete NACHA lines in findings | `processor.go:139, 159` | `RawData: line` — the full 94-char record, including routing and account fields, is stored in `validation_findings` and returned by `GET /incidents`. |
| **Raw financial data sent to a third-party model** | `ai-tier/llm_client.py:29` | `prompt = f"… Raw Sample:\n{raw_data}"` posted to OpenAI when `OPENAI_API_KEY` is set. |
| Raw data forwarded gateway → AI tier | `main.go:456` | `RawData` field populated on every triage call. |
| Vault plaintext disclosure | `vault.go:309+` | Gated by a supervisor token — the strongest control present. |

### Runtime confirmation — secret disclosure

```
POST /api/v1/webhooks  {"url":"https://example.invalid/hook"}
 -> {"id":1,"secret":"whsec_18cbc6c60c4908b5","status":"ACTIVE", ...}

GET /api/v1/webhooks
 -> [{"id":1,"url":"…","secret":"whsec_18cbc6c60c4908b5", …}]

POST /api/v1/sql/query  {"query":"SELECT url, secret FROM webhook_subscriptions"}
 -> {"columns":["url","secret"],"rowCount":1,
     "rows":[["https://example.invalid/hook","whsec_18cbc6c60c4908b5"]]}
```

### Runtime confirmation — SSRF

```
POST /api/v1/webhooks/test  {"url":"http://169.254.169.254/latest/meta-data/"}
 -> {"eventType":"GATEWAY_PING_TEST","responseCode":403,"status":"FAILED","durationMs":4}
```

The application **issued the request to the cloud metadata address** and reported the
upstream status. The 403 came from this sandbox's egress proxy, **not** from any Sentinel
Flow policy. In an environment without such a proxy there is nothing in the code path to
stop it.

---

## 10. Build and runtime dependency graph

```
                    ┌────────────────────────────────┐
  browser ────────► │ React SPA (vite)               │
                    │ hardcoded http://localhost:8080│ api.ts:6
                    │ no Authorization header sent   │
                    └───────────────┬────────────────┘
                                    │ HTTP (CORS: SENTINEL_ALLOWED_ORIGIN
                                    │        default http://localhost:3000)
                    ┌───────────────▼────────────────┐
                    │ Go gateway (:8080)             │
                    │ chi router, single package main│
                    └──┬─────────────┬────────────┬──┘
                       │             │            │
              ┌────────▼──────┐  ┌───▼────────┐  ┌▼─────────────────┐
              │ SQLite file   │  │ AI tier    │  │ arbitrary URL    │
              │ ./sentinel.db │  │ hardcoded  │  │ (webhook/test)   │
              │ modernc/sqlite│  │ 127.0.0.1  │  │ no allowlist     │
              └───────────────┘  │ :8000      │  └──────────────────┘
                                 └─────┬──────┘
                                       │ optional
                                 ┌─────▼──────┐
                                 │ OpenAI API │ llm_client.py:27
                                 └────────────┘

  DECLARED-BUT-UNREACHABLE:
    gateway/docker-compose.yml → postgres:15 + minio   (no code path opens either)
    podman-compose.yml         → AI_TIER_URL=http://ai-tier:8000 (never read)
    edge-agent/main.go         → no go.mod; not built by anything
```

Key disconnects:

- **`AI_TIER_URL` is configured but never read.** `podman-compose.yml:38` sets
  `AI_TIER_URL=http://ai-tier:8000`; the Go code hardcodes `http://127.0.0.1:8000/analyze`
  (`main.go:460`) and `http://127.0.0.1:8000/evals/run` (`main.go:555`). In containers the
  gateway cannot reach the AI tier, so **both fallbacks fire — meaning the fabricated
  triage and the fabricated 100% eval are the *default* container behaviour**, not an edge
  case.
- **PostgreSQL and MinIO are started by `gateway/docker-compose.yml` but nothing connects
  to them.** The app opens SQLite unconditionally (`main.go:81-101`).
- **Two compose files, neither authoritative.** `podman-compose.yml` (root) has
  ai-tier/gateway/ui and no database; `gateway/docker-compose.yml` has postgres/minio and
  no application.
- **State is lost on container replacement.** `Containerfile.gateway:38` declares
  `VOLUME ["/app/inbox", "/app/data"]`, but `DATABASE_URL` is unset in
  `podman-compose.yml`, so `main.go:83` defaults to `./sentinel.db` = `/app/sentinel.db` —
  outside the mounted volume.
- **Default credentials in a committed file:** `gateway/docker-compose.yml`
  `POSTGRES_PASSWORD: password`, `MINIO_ROOT_USER/PASSWORD: minioadmin`.

---

## 11. Tests mapped to production behaviour

38 test functions + 1 benchmark, all passing in 0.154s. Mapping:

| Test group | File | What it actually covers | Production value |
|---|---|---|---|
| KS test, BH-FDR, robust anomaly (12 tests) | `integrity_test.go` | Genuine statistical properties incl. monotonicity, reference values, masking resistance, thin-history refusal | **High — genuine, keep** |
| Ledger content/actor tampering (2) | `integrity_test.go` | Recomputes hashes; detects payload and actor edits | **High — keep** |
| PGP + SSH key verification (2) | `security_test.go` | Real key generation, sign, verify, then flip a byte and require failure | **High — keep** |
| SWIFT MT103/MT940 (3) | `swift_test.go` | Parser behaviour on valid + missing-tag inputs | Medium — format is out of v1 scope |
| Mod10 routing (1) | `benchmark_test.go` | ABA check digit | **High — keep** |
| E2E pipeline (5) | `e2e_test.go` | Valid file, corrupted hash, invalid ABA, ISO 20022, ledger+compliance | Medium — **no empty/truncated/zero-byte case; the release bug in §7 is not covered by any test** |
| Webhook HMAC (1) | `webhook_test.go` | Signature computation and dispatch | Medium — no SSRF/allowlist test |
| SQL console guardrails (1) | `anomaly_test.go` | Keyword/prefix rejection | Medium — **does not test column-level secret disclosure, which succeeds** |
| Prometheus exposition (1) | `security_test.go` | Format only | Low — asserts nothing about gauge truth |
| Integration Hub (3) | `connector_test.go` | Asserts the **hardcoded catalog** is returned and "sanitized" | **Negative value — encodes the mock as expected behaviour** |
| Swarm deliberation (1) | `agent_swarm_test.go` | Asserts the scripted transcript | **Negative value — encodes the script** |
| Self-healing + drift (2) | `healing_test.go` | Asserts proposal shape / drift envelope | Low |
| Vault, instant payment, DR failover (3) | `vault_test.go` | `TestInstantPaymentFedNowValidation` and `TestDisasterRecoveryFailoverSimulation` assert simulated outputs | **Negative value for the first two** |

**Coverage gaps that matter most:** no test asserts that an empty, truncated, or
unparseable file is quarantined; no test asserts authentication fails closed; no
cross-tenant test exists (no tenant concept to test); no concurrency or race test; no
idempotency or duplicate-delivery test; no restart-persistence test.

Roughly 7 of 38 tests assert that simulated behaviour is present, and would need to be
deleted alongside the features they cover.

---

## 12. Documentation contradicting running code

Per CLAUDE.md, these are reported rather than silently resolved.

| # | Claim | Location | Reality |
|---|---|---|---|
| D1 | "Moved to `.removed-davinci/`" (8 foreign files) | `AUDIT_FIXES.md:183-187` | All 8 still tracked at original paths (`src/physics/…` etc.). |
| D2 | "`vite.config.js` … Removed the `.js`" | `AUDIT_FIXES.md:188-190` | `vite.config.js` still exists and **still shadows** `vite.config.ts`; `@vitejs/plugin-react` is still not loading. Ports differ (3000 vs 5173). |
| D3 | "Tests that cannot fail: 0" | `AUDIT_FIXES.md:202` | `main.go:565-573` still returns a hardcoded `passRatePct: 100.0` whenever the Python tier is unreachable — which is the container default (§10). |
| D4 | "Security controls returning unconditional success: 0" | `AUDIT_FIXES.md:203` | `connector.go:394` still returns `mTLSVerified: true` unconditionally. Runtime-confirmed. |
| D5 | "Expected Output: 26/26 test suites passing" | `README.md:104` | 38 tests exist and pass; badge says 38/38. The prose is stale and internally inconsistent with the badge. |
| D6 | "Expected Output: 5/5 adversarial attacks contained (100% pass rate)" | `README.md:111` | Command exits **2** with `status: NOT_RUN`. |
| D7 | "Run `POST /api/v1/benchmark`" | `README.md:52` | Registered route is `GET /api/v1/benchmark/run` (`main.go:540`). |
| D8 | Performance figures "Removed pending real measurement" | `README.md:46-54` | The same file still advertises `RPO = 0.00s, RTO = 42.5ms` at `README.md:29`, plus FIPS-140, Merkle, SIMD and zero-inbound-port claims at `README.md:27-40`. |
| D9 | MIT License badge | `README.md:10` | **No `LICENSE` file exists.** |
| D10 | `docs/RESEARCH_FOUNDATIONS.md`, `docs/bny_ai_agents_report.md` | — | Not re-audited in this pass — **UNKNOWN**; the recovery plan flags unsupported external claims in the BNY material. |
| D11 | Recovery plan says "No CI workflow despite a CI badge" | `SentinelFlow_Code_Audit_and_Recovery_Plan.md:219` | **Stale in our favour** — `.github/workflows/ci.yml` now exists. It is, however, missing lint, race, migration, secret-scanning, container-build and SBOM gates, and its `ai-evals` job fails (§2). |
| D12 | Recovery plan says AI container copies a missing `requirements.txt` | plan line 213 | **Still true.** `Containerfile.ai:22` runs `COPY ai-tier/requirements.txt ./`; the file does not exist. `COPY` fails before the `|| pip install …` fallback on line 23 can help — that fallback only guards the `RUN`, not the `COPY`. |
| D13 | CLAUDE.md placement | — | The guide (`§2 step 2`) says the standing instruction belongs at the **repository root** as `CLAUDE.md`. It currently exists only at `docs/engineering/CLAUDE.md`, so Claude Code will not auto-load it. Flagged, not moved. |

---

## 13. Proposed change list

No changes were made. This is the proposal for review.

### P0 — release blockers

| ID | Change | Evidence |
|---|---|---|
| P0-1 | Ingestion must start `RECEIVED`/`VALIDATING`, never `RELEASED`; parser exception must quarantine | `processor.go:70, 236-241` |
| P0-2 | Delete `POST /api/v1/instant-payments/*` — fabricates settlement | `instant_payment.go:52-113` |
| P0-3 | Delete `mTLSVerified` from the edge-sync response and the Edge Sync API | `connector.go:394` |
| P0-4 | Delete the fabricated AI-triage fallback and the fabricated eval fallback; return `UNAVAILABLE` | `main.go:466-481, 563-573` |
| P0-5 | Stop returning webhook secrets from create/list; generate with `crypto/rand` | `main.go:627, 663, 676-681` |
| P0-6 | Delete `POST /api/v1/sql/query` | `main.go:721-795` |
| P0-7 | Delete `POST /api/v1/webhooks/test` (unrestricted SSRF) | `main.go:685-718`, `webhook.go:51-70` |
| P0-8 | Delete `POST /api/v1/healing/apply` — caller-supplied supervisor + unbound content | `healing.go:165-193` |
| P0-9 | Auth must fail closed; remove the open-when-unset branch | `main.go:173-176` |
| P0-10 | Stop sending raw financial content to the model | `ai-tier/llm_client.py:29`, `main.go:456` |
| P0-11 | Stop persisting full 94-char records and whole file bodies | `processor.go:40, 72, 139, 159` |
| P0-12 | Remove `sentinel_worker_pool_active 8` | `metrics.go:94` |

### P1 — correctness, honesty, reproducibility

| ID | Change | Evidence |
|---|---|---|
| P1-1 | Pin **one** Go version across `go.mod`, CI, container; same for Python and Node | §3 |
| P1-2 | Create `ai-tier/requirements.txt` (or switch the Containerfile to `pyproject.toml`) and lock it | D12 |
| P1-3 | Remove the committed binary `gateway/sentinel-gateway` (116k lines) and `Sentinalflow.zip` | §1, `git ls-files` |
| P1-4 | Delete `vite.config.js` so `vite.config.ts` loads | D2 |
| P1-5 | Delete the 8 foreign DaVinci files | D1 |
| P1-6 | One authoritative compose stack; honour `AI_TIER_URL`; move DB into the mounted volume | §10 |
| P1-7 | Delete Integration Hub, agent swarm, chaos, failover, executive deck, vault UI, benchmark panel — and the ~7 tests that assert them | §8, §11 |
| P1-8 | Replace silent frontend mock fallback with typed error/degraded state | `src/services/api.ts:83-99` |
| P1-9 | Send `Authorization` from the UI; make the API base URL runtime config | `src/services/api.ts:6` |
| P1-10 | Remove `BreachRiskPct` constants; SLA state must be deterministic | `main.go:229-235` |
| P1-11 | Rename "Merkle" → "application hash chain" everywhere | `compliance.go:48`, `metrics.go:81` |
| P1-12 | Serialise ledger append in a transaction with a unique predecessor constraint | `ledger.go:34-63` |
| P1-13 | Move money off `float64` to integer minor units end to end | `processor.go:34-35, 191-192` |
| P1-14 | Correct README: remove D5–D9 claims; add `LICENSE` or drop the badge | §12 |
| P1-15 | Fix or remove the failing CI `ai-evals` job; add lint/race/migration/secret gates | §2 |
| P1-16 | Resolve `npm audit` high finding (`nanoid`) | §2 #13 |

### P2 — hygiene

| ID | Change |
|---|---|
| P2-1 | Move `CLAUDE.md` to the repository root (D13) |
| P2-2 | Give `edge-agent/` a `go.mod` or delete it (§4) |
| P2-3 | Split `main.go` (862 lines, 20 inline handlers) into modules |
| P2-4 | Remove default credentials from `gateway/docker-compose.yml` |
| P2-5 | Add `.nvmrc` / `engines` to pin Node |

---

## 14. Existing work that must be preserved

Deletion is broad; these parts are real and tested, and Prompt 01 must not remove them:

1. **`kstest.go` + 12 statistical tests** (`integrity_test.go`) — a genuine two-sample KS
   implementation with Stephens correction, cross-validated against reference values, plus
   Benjamini–Hochberg FDR. Keep, clearly separated from production decisions.
2. **`robust_anomaly.go`** — median/MAD modified z-scores with masking resistance and an
   explicit refusal on thin history. Keep.
3. **Ledger verification by recomputation** (`ledger.go:82-133`) and its two tampering
   tests. Keep the verification; fix the concurrent append.
4. **PGP + SSH verification** (`security.go`) — fails closed on missing keyring, enforces a
   2048-bit floor, tests generate real keys and require failure on a flipped byte. Keep.
5. **`ValidateRoutingMod10`** (`processor.go:45-64`) and its test. Keep.
6. **Vault detokenize authorisation** (`vault.go:268-280`) — constant-time comparison,
   fails closed when unconfigured, logs denied attempts before returning. This is the
   pattern every other route should adopt.
7. **The honest-labelling pattern in `failover.go`** — `IsScriptedDemo: true`,
   `*Target` suffixes, `StandbyHealthStatus: "NOT_PROVISIONED"`. Reuse this vocabulary.
8. **`metrics.go` measured-or-`-1` pattern** (`metrics.go:26, 88-90`). Reuse.
9. **The Python eval's refusal to score** (`evals/runner.py`) — exit 2 with `NOT_RUN`.
   Keep; fix the README and CI around it, not the behaviour.
10. **`.env.example`** — names and safe descriptions only, no credentials. Already correct.
11. **`docs/AUDIT_FIXES.md`** — retain as history, but correct the four claims in §12 that
    the code no longer supports.

---

## 15. Unknowns

- Whether CI is currently red on the remote (run history not inspected).
- Which UI code paths use the browser-side NACHA parser versus the server API (§8).
- Accuracy of `docs/RESEARCH_FOUNDATIONS.md` and `docs/bny_ai_agents_report.md` (not
  re-audited).
- Behaviour of the committed binary `gateway/sentinel-gateway` — deliberately not used as
  evidence; all runtime findings above come from a clean source build.
- Whether `Sentinalflow.zip` (252 KB, tracked) contains anything not otherwise in the tree.
- Real container behaviour (no container build was performed).

---

## 16. Recommended next task

**Prompt 01 — Truth reset and scope reduction**, scoped to the P0 deletions plus P1-7, and
explicitly *excluding* any security-boundary rewrite (auth, secrets, tenancy) so that a
removal change and a control change never land in the same task, per guide §2 rule 7.

One deviation from the guide's ordering is worth deciding now: **P0-1 (the empty-file
release bug) is a one-line-class defect with a live exploit and no test coverage.** The
guide places it in Prompt 07. Doing it inside Prompt 01 — preceded by a failing regression
test — would close the most dangerous gap roughly six prompts earlier. I recommend it;
confirm before I proceed.

## Recent Updates
- **UI/UX Transformation:** The frontend has been fully updated from a dark mode to a clean, light institutional fintech theme. All components (Operations Console, Review Queue, Connector Wizard, etc.) have been restyled to reflect a 'cleaner, less AI-wrapper' feel.
