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

Requires Go and Node. Version pinning across `go.mod`, CI and the containers is inconsistent and is
being fixed; see [`CURRENT_STATE.md`](docs/engineering/CURRENT_STATE.md#3-toolchain-and-version-matrix).

```bash
# Gateway
cd gateway && go run .

# Operations UI (separate terminal)
npm ci && npm run dev
```

The UI opens on `http://localhost:3000` and expects the gateway on `http://localhost:8080`.

The optional Python AI tier is not required; the gateway reports `UNAVAILABLE` when it is absent
rather than substituting a generated answer.

```bash
cd ai-tier && uvicorn main:app --port 8000
```

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
