# Sentinel Flow — Code Audit & Remediation Log

**Date:** 14 August 2026 · **Auditor:** Claude · **Baseline:** as-uploaded `Sentinalflow.zip`

Verified by execution, not inspection: Go toolchain installed, full suite run, benchmarks
executed, frontend built.

---

## Baseline state (before changes)

| Check | Result |
|---|---|
| `go build ./...` | clean |
| `go vet ./...` | clean |
| `go test ./...` | **26/26 PASS** — the README badge was accurate |
| `npx tsc --noEmit` | clean |
| `npx vite build` | succeeded |

The build health was real. The problem was underneath it.

---

## P0 — Security

### 1. `security.go` — PGP verification always returned authentic
`VerifyDetachedPgpSignature` checked only that the input contained
`-----BEGIN PGP SIGNATURE-----`, then returned `IsAuthentic: true` with
`"Signature validated against trusted counterparty PGP Keyring."` No keyring existed. No
signature was parsed. Any party could forge a non-repudiation record over arbitrary bytes.

`TestPgpDetachedSignatureValidation` **asserted this behaviour** — it passed a truncated,
meaningless armor block and required the result be authentic. The test encoded the bug.

**Fixed:** real detached-signature verification via `ProtonMail/go-crypto/openpgp` against
a keyring at `SENTINEL_PGP_KEYRING`. Fails closed on missing keyring, unknown signer, or
mismatch. Test rewritten to generate a key, sign real bytes, verify, then flip one payload
byte and require failure.

### 2. `security.go` — SSH key validation was cosmetic
Returned `IsValid: true` for anything that base64-decoded. Never checked that the algorithm
name inside the RFC 4253 wire blob matched the declared prefix. Reported a hardcoded
`4096` bits for every RSA key — so a 1024-bit key entered the audit trail as compliant.

**Fixed:** real wire-format parsing, algorithm-match enforcement, true modulus length,
NIST SP 800-131A Rev.2 minimum of 2048 bits enforced.

### 3. `vault.go` — four independent full breaks
- **Hardcoded key.** HMAC secret was the literal `"SENTINEL_FIPS140_HMAC_SECRET"` in
  source. Deterministic tokenisation over a low-entropy domain (ABA routing = ~10⁵
  assigned values) is fully invertible by anyone with the repo.
- **Plaintext store.** `Tokens map[string]string` held raw values unencrypted.
- **No authentication on detokenize.** Only checks were `SupervisorID != ""` and
  `len(AuditReason) >= 10`. `AuthCertDigest` was parsed and discarded. `RequireApproval`
  was never read.
- **False audit claim.** Returned `"auditLogged": true` without ever writing to the ledger.
- Plus: `fieldType[0:3]` panicked on any fieldType shorter than 3 characters.

**Fixed:** keys from `SENTINEL_VAULT_HMAC_KEY` / `SENTINEL_VAULT_AES_KEY` with no fallback;
AES-256-GCM at rest; constant-time supervisor credential check; retention deadline
enforced; **disclosure refused if the audit write fails**; denied attempts also logged.
`FPE_AES256` label removed (see RESEARCH_FOUNDATIONS §8 — it was never FPE, and FPE is the
wrong primitive for these domains anyway). FIPS 140 claim removed — that is a CMVP
certificate, not a string constant.

### 4. `main.go` — no authentication anywhere + wildcard CORS
Every endpoint was open, including `/vault/detokenize` (plaintext PII) and `/sql/query`
(arbitrary database reads), with `Access-Control-Allow-Origin: *`. Any web page the
operator visited could read the database cross-origin.

**Fixed:** Bearer-token middleware on `/api/v1` (`SENTINEL_API_TOKEN`, warns loudly if
unset); CORS restricted to `SENTINEL_ALLOWED_ORIGIN`.

### 5. SQL console guardrail was a keyword blocklist
Blocklists on SQL both over-block (`SELECT * FROM t WHERE note='DROP'`) and under-block —
`VACUUM`, `REINDEX`, `load_extension()`, and `PRAGMA` was **explicitly allowed as a
prefix** (`PRAGMA writable_schema=1`). No timeout, so an unbounded cross join was a
trivial DoS.

**Fixed:** separate read-only handle (`?mode=ro&_pragma=query_only(1)`) — a structural
guarantee rather than string matching — plus a 10s context timeout. Blocklist kept as
defence in depth and extended.

### 6. `ledger.go` — tamper detection could not detect tampering
`GetLedger` only checked `row[n].previous_hash == row[n-1].current_hash`. It **never
recomputed** the hash. Editing `payload`, `actor`, `event_type` or `created_at` on any row
left every link intact and `IsChainValid` returned `true`.

`TestHashChainTamperDetection` mutated `current_hash` — the one vector the link check
happens to catch — and passed, giving false assurance about all the others.

**Fixed:** verification recomputes each row's hash from its stored fields. Added
`IntegrityStatus` per event (`VERIFIED` / `BROKEN_LINK` / `CONTENT_TAMPERED`) and
`FirstBreachEvent`. New tests `TestLedgerDetectsContentTampering` and
`TestLedgerDetectsActorTampering` cover the previously untested vectors.

Note: this is a **hash chain**, not a Merkle tree. "Merkle" removed from claims until a
history tree with membership and consistency proofs exists.

---

## P1 — Correctness and honesty of measurement

### 7. `benchmark.go` — the headline number counted zero records
`GenerateLargeNachaCorpus` emitted records of **95, 96, 103, 98 and 97** characters. NACHA
is strictly 94. The parse loop filters on `len(line) == 94`, so it matched **nothing**:

```
n=10000   recs=0  rec/s=0
n=25000   recs=0  rec/s=0
n=100000  recs=0  rec/s=0
```

`TotalRecordsParsed` and `RecordsPerSecond` were structurally always zero. The corpus was
also invalid NACHA that `moov-io/ach` (already a dependency) would reject.

**Fixed:** every record padded/truncated to exactly 94; single-pass scan without the two
extra full copies the old `strings.Split(strings.ReplaceAll(...))` created; routing
validation results are now used rather than discarded into `_`; `ValidationPassRate`
measured rather than hardcoded to `100.0`. Engine name corrected from
`Sentinel-Go-SIMD-Streaming` — there is no SIMD and it was not streaming.

### 8. `metrics.go` — a fabricated Prometheus gauge
Line 72 emitted `sentinel_streaming_parse_rate_records_per_sec 296000` as a **hardcoded
constant** labelled "measured". Any Grafana panel (there is a dashboard in `deploy/`)
would show a flat invented rate forever. A monitoring product emitting a fake metric about
itself.

**Fixed:** gauge reports the last genuinely measured rate, `-1` until one occurs.

### 9. `failover.go` — `RTO = 42.5ms` was `time.Sleep(42ms)`
The function slept 42ms then measured how long it slept. No second region, no replica, no
promotion. `RpoSeconds: 0.00` and `ReplicatedBlocksCount: 4829` were struct literals. The
comment claimed "Synchronous Merkle proof mirroring guarantees RPO = 0" — there is no
Merkle tree and no mirroring.

**Fixed:** fields renamed to `*Target`, `IsScriptedDemo: true` and an explicit
`Disclaimer` added, `StandbyHealthStatus` now `NOT_PROVISIONED`.

### 10. `drift.go` — "Kolmogorov-Smirnov" performed no test
`CalculateDriftMetrics()` returned a hardcoded struct literal. `ConfidenceScore: 0.978`
typed in. `ReportID` frozen to `"DRIFT-REP-20260814"`.

**Fixed:** `kstest.go` implements a genuine two-sample KS test — exact `D` via merge pass
with correct tie handling, Stephens (1970) finite-sample correction, and a numerically
stable p-value. Plus `BenjaminiHochberg()` for FDR control across thousands of feeds.

> A bug I introduced and my own test caught: the alternating Kolmogorov series converges
> arbitrarily slowly as t→0, producing non-monotone output.
> `TestKSQIsMonotoneAndBounded` flagged it. Fixed with the Jacobi theta-transformed
> series for t<1; both forms cross-validated to 1e-17.

### 11. `anomaly.go` — 3σ on a hardcoded baseline
Real z-score arithmetic, but against `DefaultBaseline` constants, and mean/σ have
breakdown point 0 — one outlier in the window masks the next anomaly.

**Fixed (added alongside, original left intact):** `robust_anomaly.go` implements
median/MAD modified z-scores (Iglewicz & Hoaglin), 50% breakdown, with the MAD=0 edge case
handled and a refusal to assert anomalies on thin history.
`TestRobustDetectorResistsMasking` shows one contaminant driving the classic z-score of a
genuine 2× spike to **−0.04**.

### 12. `ai-tier/evals/runner.py` — the eval could not fail
```python
is_contained = True      # never reassigned
if is_contained: contained_attacks += 1
"status": "PASSED",      # literal
```
No model invoked, no agent called, `expected_containment` never read. `pass_rate` was
mathematically guaranteed to be 100.0% for any dataset. `latency_ms` added a fabricated
`+12.4` to a loop doing no work. The badge "Adversarial AI Evals — 100% PASS" was the
output of `if True: passed += 1`.

**Fixed:** invokes the real system under test, derives pass/fail from observable response
properties (autonomous release, approval requirement, secret exfiltration, side effects,
fabricated citations), and **raises rather than reporting a score** when nothing is wired
up. Verified: a deliberately vulnerable stub agent now scores **0.0%**.

---

## P2 — Hygiene

- **Foreign project in `src/`.** `PhysicsWorld.js`, `CodexUI.js`, `MachinePresets.js`,
  `SoundEngine.js`, `FailureAnalyzer.js`, `CanvasRenderer.js`, `main.js`, `style.css` are
  DaVinci's Codex Sandbox, importing `planck-js` which is not in `package.json`.
  Unreachable from the React app. Moved to `.removed-davinci/`.
- **`vite.config.js` shadowed `vite.config.ts`.** Vite resolves `.js` first, so
  `@vitejs/plugin-react` was never loading and dev-mode Fast Refresh was silently broken.
  Removed the `.js`.
- Deleted committed `gateway/sentinel.db` and `__pycache__/`. Added `.gitignore`,
  `.env.example`.

---

## Final state

| Check | Before | After |
|---|---|---|
| Go tests | 26 pass | **38 pass** |
| `go vet` | clean | clean |
| Frontend build | ok | ok (react plugin now active) |
| Tests that cannot fail | ≥3 | 0 |
| Security controls returning unconditional success | 2 | 0 |

## Not fixed — deliberately left for you

1. **Merkle history tree** (Crosby & Wallach) — membership and consistency proofs. Design
   decision, not a patch.
2. **Conformal `P(breach)`** — the actual product. See RESEARCH_FOUNDATIONS §1.
3. `generator.go` silently truncates over-length lines with `lines[i][:94]`, which can cut
   meaningful data in the timestamp-concatenated presets. Should error, not truncate.
4. `agent_swarm.go` / `swarm.py` transcripts are scripted, not model-generated. Fine as a
   demo; must be labelled as one.
5. `SENTINEL_API_TOKEN` unset still runs open with only a log warning. Should refuse to
   start in any non-local mode.
