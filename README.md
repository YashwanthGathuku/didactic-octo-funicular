# SentinelFlow — Governed Autonomous Financial Operations

<p align="center">
  <strong>Autonomous investigation with deterministic financial control.</strong>
</p>

<p align="center">
  <img src="https://img.shields.io/badge/Google%20Cloud-Platform-4285F4?style=for-the-badge&logo=googlecloud&logoColor=white" alt="Google Cloud" />
  <img src="https://img.shields.io/badge/Google%20ADK-7--Agent%20Fleet-FBBC05?style=for-the-badge&logo=google&logoColor=black" alt="Google ADK" />
  <img src="https://img.shields.io/badge/Gemini-3.5%20Flash-EA4335?style=for-the-badge&logo=google&logoColor=white" alt="Gemini 3.5 Flash" />
  <img src="https://img.shields.io/badge/Cloud%20Run-Deployed-4285F4?style=for-the-badge&logo=googlecloud&logoColor=white" alt="Cloud Run" />
  <img src="https://img.shields.io/badge/Model%20Armor-Guardrails-6D4BC3?style=for-the-badge&logo=googlecloud&logoColor=white" alt="Model Armor" />
  <img src="https://img.shields.io/badge/Go-1.25.13-00ADD8?style=for-the-badge&logo=go&logoColor=white" alt="Go 1.25.13" />
  <img src="https://img.shields.io/badge/React-18.3.1-61DAFB?style=for-the-badge&logo=react&logoColor=black" alt="React 18.3.1" />
</p>

> **Core principle:** **AI proposes → deterministic core decides → humans authorize.**
>
> SentinelFlow allows Gemini agents to investigate, reason, correlate context, and propose remediation without granting the model unilateral authority to mutate source payment artifacts, approve incidents, or release funds.

---

## Demo & Cloud Proof

| Resource | Link / Identifier | Purpose |
| --- | --- | --- |
| **Operations Cockpit** | [sentinelflow-ui-axrdwvptka-uc.a.run.app](https://sentinelflow-ui-axrdwvptka-uc.a.run.app) | React 18 + Vite operations UI on Cloud Run |
| **Go Control Plane** | [sentinelflow-gateway-axrdwvptka-uc.a.run.app](https://sentinelflow-gateway-axrdwvptka-uc.a.run.app) | Deterministic validation, policy, Tool Gateway, workflow state |
| **Vertex AI Agent Runtime** | `projects/70712885585/locations/us-central1/reasoningEngines/3989878657815412736` | Managed Google ADK runtime |
| **Google Model Armor** | `us-central1 / sentinelflow-guardrail-template` | Model input/output screening |
| **Demo data** | `gs://sentinelflow-demo-data-project-3687901b-8355-4073-ac3` | Staged synthetic and test datasets |
| **Devpost material** | [`docs/DEVPOST_SUBMISSION.md`](docs/DEVPOST_SUBMISSION.md) | Submission narrative and evidence |

---

## Why SentinelFlow Exists

High-consequence payment operations are a poor place to give a probabilistic model direct authority.

A financial-file incident may involve malformed fixed-width records, invalid routing checksums, mismatched control totals, duplicate submissions, return spikes, stale policy state, or adversarial text embedded inside otherwise valid business data. An LLM can be valuable for investigation and explanation, but arithmetic truth, policy truth, mutation authority, and final release need stronger guarantees.

SentinelFlow separates those responsibilities:

- **Deterministic financial truth** — Go validators and policy engines own parsing, arithmetic, invariants, hashes, and executable decisions.
- **Bounded AI reasoning** — a fixed Google ADK fleet uses Gemini 3.5 Flash for diagnosis, context gathering, synthesis, and structured remediation proposals.
- **Governed execution** — the Tool Gateway enforces identity, manifest membership, capability allowlists, autonomy bounds, policy, idempotency, preconditions, schemas, timeouts, and audit persistence.
- **Independent verification** — remediation produces a new candidate artifact that is independently re-read and validated before approval.
- **Human authority** — `VERIFIED`, `APPROVED`, and `RELEASED` are distinct states.

```text
Financial event
    ↓
Deterministic validation
    ↓
Quarantine / incident
    ↓
Google ADK + Gemini investigation
    ↓
Structured remediation proposal
    ↓
Governed deterministic candidate creation
    ↓
Independent verification
    ↓
Human authorization / dual control
```

---

## System Architecture

<p align="center">
  <a href="./Architecture.png">
    <img src="./Architecture.png" alt="SentinelFlow architecture diagram" width="100%" />
  </a>
</p>

The architecture intentionally places a hard authority boundary between model reasoning and financial execution.

### Core governance invariants

```text
ReturnRiskAssessment != FinancialDecision
MemoryRecall         != Evidence
VERIFIED             != APPROVED
APPROVED             != RELEASED
AI proposal          != Financial authority
```

---

## What SentinelFlow Does

### 1. Deterministic ingest and quarantine

The Go control plane parses NACHA ACH records and evaluates structural and financial invariants before model reasoning is allowed to influence the workflow.

Examples include:

- 94-character record structure
- file/batch record hierarchy
- entry and control totals
- batch/file control reconciliation
- ABA routing checksum validation using the weighted Mod-10 rule
- artifact and policy hash bindings
- quarantine on blocking findings

ABA routing validation uses the standard weighted condition:

```text
3(d1 + d4 + d7) + 7(d2 + d5 + d8) + (d3 + d6 + d9) ≡ 0 (mod 10)
```

### 2. Autonomous multi-agent investigation

A quarantined incident can trigger a fixed seven-agent Google ADK roster running Gemini 3.5 Flash.

| Agent | Tier | Role | Important boundary |
| --- | --- | --- | --- |
| **IncidentCommanderAgent** | A1 | Plans and coordinates the investigation | Cannot approve or release |
| **DiagnosisAgent** | A1 | Explains deterministic findings and likely root cause | Read-only investigation |
| **PolicySLAAgent** | A1 | Interprets operational policy/SLA context | Cannot override Go policy |
| **RemediationAgent** | A2 | Produces a structured remediation plan | Cannot directly rewrite the source artifact |
| **VerifierAgent** | A1 | Critiques evidence and candidate state | Cannot create remediation candidates |
| **MemoryAgent** | A1 | Retrieves bounded historical context | Memory cannot mint authoritative evidence |
| **ReturnRiskAgent** | A1 | Analyzes ACH return-risk context | Risk score is advisory, not a financial decision |

Every canonical agent manifest declares its model, triggers, allowed tools, denied capabilities, turn/tool-call ceilings, timeout, memory permissions, output schema, data classifications, and guardrail requirement. A SHA-256 manifest hash binds execution to that declared capability set.

### 3. Governed Tool Gateway

The Tool Gateway is the enforcement boundary between agents and system capabilities. Before execution it evaluates:

1. identity and tenant scope
2. fixed-roster membership and manifest integrity
3. tool/capability allowlists
4. autonomy bounds
5. shadow-mode restrictions
6. idempotency and recovery semantics
7. resource preconditions / TOCTOU bindings
8. input schema and size
9. authoritative policy
10. required obligations
11. bounded execution and timeout handling
12. output validation and durable audit persistence

An agent therefore cannot turn a persuasive model response into an unauthorized financial action.

### 4. Immutable remediation candidates

The model proposes a typed remediation plan. The deterministic control plane performs allowlisted transformations against the current parent hash and creates a **new candidate artifact** rather than modifying the received source in place.

```text
Original artifact  --immutable-->  preserved
        |
        | governed remediation plan
        v
Candidate artifact --new SHA-256--> independent verification
```

### 5. Independent verification

The verifier independently reloads parent and candidate bytes, recomputes hashes, checks workflow and derivation bindings, and reruns deterministic validation.

A successful verification changes only the verification state:

> **VERIFIED ≠ APPROVED ≠ RELEASED**

### 6. Lens — governed financial operations intelligence

SentinelFlow Lens supports investigation over allowlisted analytical datasets without converting natural language into arbitrary SQL.

```text
Question
  ↓
Typed semantic QueryIntent
  ↓
Go Lens compiler
  ↓
Tool Gateway: ANALYTICS_QUERY
  ↓
Tenant-scoped parameterized execution
  ↓
Deterministic result
  ↓
Query hash + result hash + provenance
```

Lens currently covers operational views such as return intelligence, incident trends, validation findings, file operations, and agent operations. Synthetic demo observations are explicitly marked as synthetic and **cannot satisfy authoritative financial evidence requirements**.

### 7. Security and failure containment

- Google Model Armor screens model-bound inputs and outputs.
- Prompt content is partitioned by trust domain and minimized before inference.
- Sensitive financial data is redacted/minimized at the model boundary.
- Data-sovereignty violations fail closed instead of silently rerouting.
- Side-effectful tool executions are not blindly replayed after an uncertain failure.
- If the AI tier is unavailable, deterministic validation and quarantine remain authoritative.

**AI failure cannot become financial failure.**

---

## Google Cloud / Hackathon Stack

SentinelFlow is submitted to the **All Things Agentic Hackathon — Fortified Enterprise Fleet** category.

| Layer | Technology |
| --- | --- |
| Model | **Gemini 3.5 Flash** |
| Agent framework | **Google Agent Development Kit (ADK)** |
| Managed agent runtime | **Vertex AI Agent Runtime / Reasoning Engine** |
| Safety | **Google Model Armor** |
| Application hosting | **Google Cloud Run** |
| Demo object storage | **Google Cloud Storage** |
| Backend | **Go 1.25.13** |
| AI tier | **Python + FastAPI + google-adk + google-genai** |
| Frontend | **React 18.3.1 + TypeScript + Vite** |
| Local durable state | **SQLite** |
| Local supporting services | **PostgreSQL 16 + MinIO** via Compose; repository migration remains intentionally scoped/documented |
| Telemetry | **OpenTelemetry / Google Cloud Trace-compatible instrumentation** |

---

## Reproducible Local Setup

### Prerequisites

- Go **1.25.13**
- Node.js **22.x** and npm **10+**
- Python **3.11+**
- Docker Compose or Podman Compose

### Start the local stack

```bash
git clone https://github.com/YashwanthGathuku/didactic-octo-funicular.git
cd didactic-octo-funicular
cp .env.example .env
```

Set non-default local values in `.env` for at least:

```text
POSTGRES_PASSWORD=...
MINIO_ROOT_USER=...
MINIO_ROOT_PASSWORD=...
```

Then start the stack:

```bash
docker compose up --build
```

Open:

- UI: `http://localhost:3000`
- Gateway readiness: `http://localhost:8080/api/v1/ready`

The Compose stack intentionally does **not** fabricate an AI response when Gemini credentials are absent. The AI tier reports unavailable while the deterministic control plane remains operational and fail-closed.

### Useful development commands

```bash
make test        # Go backend test suite
make test-race   # Go race-enabled tests
make lint        # Go formatting check + frontend typecheck
make build       # Gateway + frontend production build
make demo        # End-to-end local demo script
npm test         # Frontend Vitest suite
```

Lens/submission verification:

```bash
bash scripts/verify_lens_lite.sh
bash scripts/verify_submission_freeze.sh
```

`verify_lens_lite.sh` is explicitly local-only and does not deploy to or mutate Google Cloud resources.

---

## Data & Demo Provenance

SentinelFlow uses reproducible test and synthetic data rather than presenting synthetic rows as real financial evidence.

- **Moov ACH** fixtures provide standards-oriented NACHA test artifacts.
- **Moov ACH Test Harness** supports returns, corrections, reconciliation, delays, and trace/amount matching scenarios.
- **IBM AML synthetic data** provides historical financial behavior for controlled analytics experiments.
- **SentinelFlow-generated scenarios** create deterministic malformed/control-mismatch, routing, duplicate, and adversarial-content cases.
- **Lens synthetic history** is explicitly tagged non-authoritative; it is useful for visualization and testing, not for satisfying a financial control.

---

## Repository Map

```text
.
├── ai-tier/             # Google ADK / Gemini bounded reasoning tier
├── gateway/             # Go deterministic control plane
├── src/                 # React operations cockpit
├── demo/                # Deterministic demo fixtures / Lens data
├── docs/                # Architecture, security, deployment, submission evidence
├── scripts/             # Verification, data generation, deployment helpers
├── compose.yaml         # Authoritative local stack
├── Architecture.png     # Judge-facing architecture diagram
└── README.md
```

---

## Design Lessons

- In regulated automation, **better boundaries matter more than asking the model to be more careful**.
- Agent memory is useful context, but **memory is not evidence**.
- A probabilistic risk score can prioritize investigation, but **risk is not policy**.
- Remediation should create immutable candidates with provenance, not rewrite evidence in place.
- Failure recovery and idempotency matter as much as the happy path for long-running agents.
- Enterprise agents become more useful when their tool authority is explicit, narrow, auditable, and independently enforceable.

---

## Acknowledgements

SentinelFlow builds on or uses public/open tooling and datasets including:

- [Google Agent Development Kit (ADK)](https://google.github.io/adk-docs/)
- [Moov ACH](https://github.com/moov-io/ach)
- [Moov ACH Test Harness](https://github.com/moov-io/ach-test-harness)
- [IBM AML synthetic dataset](https://github.com/IBM/AML-Data)

---

<p align="center">
  <strong>SentinelFlow</strong><br/>
  <em>Financial truth stays outside model authority.</em>
</p>
