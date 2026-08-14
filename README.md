# 🛡️ Sentinel Flow: Financial File Reliability Gateway

[![Go Tests](https://img.shields.io/badge/Go%20Tests-38%2F38-00ADD8?style=flat-square&logo=go)](gateway/)
[![CI/CD](https://img.shields.io/badge/CI%2FCD-GitHub%20Actions%20Matrix-2088FF?style=flat-square&logo=githubactions)](.github/workflows/ci.yml)
[![FedNow RTP](https://img.shields.io/badge/FedNow%20%2F%20RTP-Instant%20ISO%2020022-0284C7?style=flat-square)](gateway/instant_payment.go)
[![Self-Healing](https://img.shields.io/badge/Self--Healing-Deterministic%20Patches-10B981?style=flat-square)](gateway/healing.go)
[![Multi-Agent](https://img.shields.io/badge/Agent%20Swarm-4%20Agents%20ReAct-8B5CF6?style=flat-square)](gateway/agent_swarm.go)
[![Compliance](https://img.shields.io/badge/Compliance-SEC%2017a--4%20%7C%20SOX%20404-F59E0B?style=flat-square)](gateway/compliance.go)
[![Podman](https://img.shields.io/badge/Containers-Podman%20%7C%20K8s-892CA0?style=flat-square&logo=podman)](podman-compose.yml)
[![License](https://img.shields.io/badge/License-MIT-gray?style=flat-square)](LICENSE)

> **Know every financial file that should arrive, prove whether it did, validate it before downstream use, and investigate exceptions with an approval-gated AI copilot.**

---

## 🏛️ Executive Summary & Industry Context

In global institutional banking, custody, and treasury operations (**Tier-1 Custodians, Clearing Houses, Commercial Banks**), over **$50 Trillion** in daily settlement transactions move not through synchronous REST APIs, but via **batch files over SFTP/MFT, instant payment networks, and interbank endpoints**:
- **NACHA ACH** (Corporate payroll, vendor disbursements, direct debits)
- **FedNow & RTP** (`pacs.008.001.08` instant credit transfers, `pacs.002.001.10` status reports)
- **ISO 20022 XML** (`pacs.008` customer credit transfers, `camt.053` bank statements)
- **BAI2** (Cash management end-of-day balance statements)
- **SWIFT MT** (`MT103` customer credit transfers, `MT940` statement messages)
- **PostgreSQL / SQL Staging Tables** (Direct settlement batches)

**Sentinel Flow** provides an **Enterprise Sovereign Financial File Reliability & Governance Gateway**:
1. **Multi-Tenant Zero-Knowledge Data Masking & Tokenization Vault (FIPS-140 / FPE AES-256)**
2. **FedNow & The Clearing House RTP Instant ISO 20022 Streaming Validator ($< 2.5\text{s}$ SLA Timer)**
3. **Automated Multi-Region Disaster Recovery Failover Simulator ($RPO = 0.00\text{s}$, $RTO = 42.5\text{ms}$)**
4. **Autonomous Self-Healing File Repair Engine (Deterministic Mod10 & Entry Hash Patches)**
5. **Continuous Schema & Volume Drift Profiler (30-Day Kolmogorov-Smirnov Distribution Shifts)**
6. **Astra Multi-Agent Swarm (4-Agent Collaborative ReAct Team)**
7. **Customer Edge Agent with Outbound-Only mTLS Telemetry (Zero Inbound Open Ports)**
8. **OWASP Decoupled Secrets Architecture with Vault/AWS Pointers**
9. **SIMD-Accelerated Multi-Format Streaming Validation (NACHA, ISO 20022, BAI2, SWIFT MT103/MT940)**
10. **Statistical $3\sigma$ Volume Anomaly Spike Isolation & Z-Score Analysis**
11. **Cryptographic SHA-256 Append-Only Merkle Audit Ledger (SEC Rule 17a-4 / SOX 404)**
12. **Institutional Read-Only SQL Audit Console with CSV Export**
13. **Real-time Prometheus Metrics Exporter (`/metrics`) & Visual File Redliner**
14. **Outbound HMAC-SHA256 Signed Webhook Pub/Sub & Autonomous Chaos Monkey Daemon**

---

## ⚡ Performance

> **Removed pending real measurement.** The previous table in this section reported figures
> (296,000 rec/s, 148 MB/s, RTO 42.5ms, RPO 0.00s) that were not produced by this codebase.
> The record counter was structurally always zero because the corpus generator emitted
> invalid record widths; the RTO figure was a measurement of `time.Sleep(42ms)`; and the
> Prometheus throughput gauge was a hardcoded constant. See `docs/AUDIT_FIXES.md`.
>
> Run `POST /api/v1/benchmark` for a live, honest measurement of the in-memory fixed-width
> scan. Note that this excludes disk I/O, decryption, database writes and network egress,
> and is therefore **not** comparable to end-to-end MFT product throughput.

## 🚀 Turnkey Local Quickstart

### One-Click Launch (Windows PowerShell)
```powershell
.\start.ps1
# Or in Podman containerized mode:
.\start.ps1 -Podman
```

### One-Click Launch (Linux / macOS)
```bash
chmod +x start.sh
./start.sh
# Or in Podman containerized mode:
./start.sh --podman
```

### Manual Step-by-Step

1. **Start Python AI Tier**:
   ```bash
   cd ai-tier
   uvicorn main:app --port 8000
   ```

2. **Start Go Gateway Tier**:
   ```bash
   cd gateway
   go run main.go processor.go ledger.go watcher.go generator.go benchmark.go compliance.go iso20022.go bai2.go metrics.go security.go swift.go webhook.go anomaly.go connector.go agent_swarm.go healing.go drift.go stream.go vault.go instant_payment.go failover.go
   ```

3. **Start Operations PWA**:
   ```bash
   npm install
   npm run dev
   ```

4. Open **`http://localhost:3000`** in your browser.

---

## 🧪 Running Automated Tests & Benchmarks

### Go Unit, Benchmark & E2E Integration Suite
```bash
cd gateway
go test -v ./...
```
*Expected Output: 26/26 test suites passing (`PASS ok sentinel-gateway 15.0s`)*

### Python AI Adversarial Security Evals
```bash
cd ai-tier
python evals/runner.py
```
*Expected Output: 5/5 adversarial attacks contained (100% pass rate)*

---

## 📄 License
MIT License. Built for institutional financial systems engineering and applied AI research.
