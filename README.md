# Sentinel Flow: Financial File Reliability Gateway

[![CI/CD](https://img.shields.io/badge/CI%2FCD-GitHub%20Actions-2088FF?style=flat-square&logo=githubactions)](.github/workflows/ci.yml)

> Know which financial files are expected, record whether they arrived, validate NACHA files
> deterministically before downstream use, and quarantine unsafe input.

**Status: pre-release engineering project.** This is a production-*shaped* system, not a
production-*approved* one. Read [`docs/engineering/SCOPE.md`](docs/engineering/SCOPE.md) before
reading anything else — it is the authority on what this software actually does. If a claim
anywhere contradicts SCOPE.md, SCOPE.md wins and the claim is a defect.

---

## What this does today

> Sentinel Flow knows which financial files are expected, records finalized arrivals, validates
> NACHA files deterministically, quarantines unsafe inputs, and gives operators a traceable
> evidence path from source object to human release decision.

Working and tested:

- Deterministic NACHA structural validation — record width, ABA routing Mod10 check digit, entry
  hash, batch control arithmetic
- **Fail-closed ingestion.** Empty, whitespace-only, truncated and unparseable input is
  quarantined, never released
- SHA-256 content hashing of every received artifact
- An append-only application hash chain with tamper detection by recomputation
- PGP detached-signature verification and SSH public-key parsing that fail closed
- A two-sample Kolmogorov–Smirnov test and a median/MAD robust anomaly detector (both real, both
  currently unwired from production decisions)

Per-capability status, including what is Partial and why, is in
[`docs/engineering/SCOPE.md`](docs/engineering/SCOPE.md#2-capability-status).

## What this does NOT do

Stated plainly because earlier versions of this file claimed otherwise:

- **No payment initiation, settlement, or rail connectivity.** Settlement is not a state in this
  product.
- **No authentication worth the name.** The API runs fully open when `SENTINEL_API_TOKEN` is unset.
- **No tenant isolation.** No business table has a tenant column.
- **No production support for ISO 20022, BAI2, or SWIFT.** Those parsers are experimental.
- **No measured performance figures.** No throughput, latency, or SLA number appears anywhere in
  this repository, because none has been reproducibly measured.
- **No compliance status.** Not FIPS certified, not SOC 2, not SEC 17a-4 compliant, not "bank-grade".
  The audit ledger is a linear hash chain, not a Merkle tree.

Full non-goals list: [`docs/engineering/SCOPE.md`](docs/engineering/SCOPE.md#3-explicit-non-goals).

## Performance

No performance figures are published. Earlier versions of this file reported 296,000 rec/s,
148 MB/s, `RTO 42.5ms` and `RPO 0.00s`. None were produced by this codebase: the record counter was
structurally always zero because the corpus generator emitted invalid record widths, the RTO figure
measured a `time.Sleep(42ms)`, and the Prometheus throughput gauge was a hardcoded constant.

A reproducible benchmark harness — with committed hardware, dataset, concurrency and raw output —
is scheduled work. Until that artifact exists, no number goes here.

## Local quickstart

Pinned toolchain: **Go 1.25.8**, **Node 22.22.2** (`.nvmrc`), **Python 3.11**. These versions are
identical in `go.mod`, `package.json`, `pyproject.toml`, CI and every container.

### One command

```bash
cp .env.example .env      # then set POSTGRES_PASSWORD, MINIO_ROOT_USER, MINIO_ROOT_PASSWORD
docker compose up --build # or: podman-compose up --build
```

`compose.yaml` is the single authoritative stack. Ports are published to loopback only.
The UI is on `http://localhost:3000`, the gateway on `http://localhost:8080`.

### Without containers

```bash
cd gateway
go run . migrate            # apply versioned migrations
go run . migrate seed-demo  # optional; refused outside the local-demo profile
go run .                    # binds 127.0.0.1:8080 in the local-demo profile

npm ci && npm run dev       # operations UI, separate terminal
```

The Python AI tier is optional. When `AI_TIER_URL` is unset the gateway reports `NOT_CONFIGURED`
on AI endpoints; deterministic ingestion is unaffected.

```bash
cd ai-tier && pip install -r requirements.txt && uvicorn main:app --port 8000
```

### Profiles

| Profile | Behaviour |
|---|---|
| `local-demo` (default) | Binds loopback only and refuses any other address. May run unauthenticated, and says so on startup and on screen. |
| `production` | Refuses to start unless API token, database, object store, allowed origin and PGP keyring are all configured. Rejects well-known tokens and tokens under 32 characters. |

Health endpoints are separate on purpose: `/api/v1/health` is liveness and checks nothing else;
`/api/v1/ready` probes the database and reports each dependency's real state.

> **The operations board is demo data.** Partners, contracts and expectations come from a local
> synthetic corpus, not from the gateway, and every affected screen says so in a banner. File
> upload and validation are real and do call the server.

## Tests

```bash
cd gateway && go test ./...      # Go unit, integration and route tests
npx tsc --noEmit && npm run build # TypeScript typecheck and production build
cd ai-tier && python3 evals/runner.py
```

No test count is published here. CI is the authority on what passes; a number typed into a README
is not evidence.

Note that `evals/runner.py` **exits non-zero with `NOT_RUN`** unless a system under test is wired
up. That is intended: it refuses to emit a pass rate it did not measure.

## Engineering documentation

| Document | Purpose |
|---|---|
| [`SCOPE.md`](docs/engineering/SCOPE.md) | Authoritative capability status, non-goals, vocabulary rules |
| [`CURRENT_STATE.md`](docs/engineering/CURRENT_STATE.md) | Read-only baseline audit with runtime-reproduced findings |
| [`SentinelFlow_Code_Audit_and_Recovery_Plan.md`](docs/engineering/SentinelFlow_Code_Audit_and_Recovery_Plan.md) | External audit and recovery sequence |
| [`SENTINELFLOW_PRODUCTION_READY_PROMPT_GUIDE.md`](docs/engineering/SENTINELFLOW_PRODUCTION_READY_PROMPT_GUIDE.md) | Gated implementation plan |
| [`REMOVED_CODE_ARCHIVE.md`](docs/engineering/REMOVED_CODE_ARCHIVE.md) | Verbatim backend code removed in the truth reset, with reasons |
| [`REMOVED_CODE_ARCHIVE_UI.md`](docs/engineering/REMOVED_CODE_ARCHIVE_UI.md) | Verbatim UI code removed in the truth reset |

## License

No license file is present, so no license is granted. A `LICENSE` will be added after a dependency
and license review. The previous MIT badge linked to a file that did not exist.
