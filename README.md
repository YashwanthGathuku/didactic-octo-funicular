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
> SentinelFlow lets Gemini agents investigate, reason, correlate context, and propose remediation without granting the model unilateral authority to mutate source payment artifacts, approve incidents, or release funds.

---

## Project Demo & Cloud Proof

| Resource | Link / Identifier | Purpose |
| --- | --- | --- |
| **Operations Cockpit** | [sentinelflow-ui-axrdwvptka-uc.a.run.app](https://sentinelflow-ui-axrdwvptka-uc.a.run.app) | React 18 + Vite operations UI on Cloud Run |
| **Go Control Plane** | [sentinelflow-gateway-axrdwvptka-uc.a.run.app](https://sentinelflow-gateway-axrdwvptka-uc.a.run.app) | Deterministic validation, policy, Tool Gateway, workflow state |
| **Vertex AI Agent Runtime** | `projects/70712885585/locations/us-central1/reasoningEngines/3989878657815412736` | Managed Google ADK runtime |
| **Google Model Armor** | `us-central1 / sentinelflow-guardrail-template` | Model input/output screening |
| **Demo data** | `gs://sentinelflow-demo-data-project-3687901b-8355-4073-ac3` | Staged synthetic and test datasets |
| **Devpost material** | [`docs/DEVPOST_SUBMISSION.md`](docs/DEVPOST_SUBMISSION.md) | Submission narrative and evidence |

---

## About the Project

SentinelFlow is a **governed autonomous financial-operations control plane** for payment-file reliability, incident investigation, remediation, and pre-ledger decision support.

The system is designed around a simple but important observation: in regulated financial operations, an AI model can be extremely useful for investigation and reasoning while still being the wrong component to own arithmetic truth, policy truth, mutation authority, or final release.

SentinelFlow therefore splits the problem into three layers:

1. **Deterministic financial truth** — Go services own parsing, arithmetic, control totals, hashes, state transitions, and executable policy.
2. **Bounded autonomous reasoning** — Google ADK agents running Gemini 3.5 Flash diagnose incidents, retrieve governed context, synthesize explanations, and propose structured remediation.
3. **Human financial authority** — irreversible release remains behind explicit approval and dual-control rules.

The result is not a chatbot placed next to a payment system. It is an autonomous operations workflow whose AI can do useful work without becoming the authority that moves money.

---

## Inspiration & Real-World Context

The project was inspired by the operational reality behind batch payment processing: when a payment file fails close to a settlement cutoff, the expensive part is rarely noticing that *something* is wrong. The expensive part is determining **what failed, what changed, whether the issue has happened before, whether a safe correction exists, and whether that correction can be trusted enough to release**.

Three recurring classes of problems shaped SentinelFlow:

### 1. Small inconsistencies can block high-value payment artifacts
A single malformed record, routing checksum error, or mismatched batch control total can force operators into manual inspection even when the underlying file represents a large payroll or treasury run.

### 2. The same incident spans multiple kinds of reasoning
Operators may need to reconcile structural findings, policy and SLA constraints, historical partner behavior, prior incidents, return patterns, and remediation options at the same time. This is exactly where a specialist agent fleet can remove human toil.

### 3. Autonomous AI creates a new authority problem
Giving an agent enough permissions to be useful can also give it enough permissions to be dangerous. A prompt injection, stale memory, hallucinated calculation, replayed side effect, or compromised tool path must never become a financial release decision.

That led to SentinelFlow's defining architecture:

```text
AI can investigate.
AI can recommend.
AI can propose a remediation.

AI cannot unilaterally approve, mutate the original artifact, or release funds.
```

---

## The Problem SentinelFlow Solves

A financial-file incident may involve malformed fixed-width records, invalid routing checksums, mismatched control totals, duplicate submissions, return spikes, stale policy state, or adversarial text embedded inside otherwise valid business data.

Traditional manual workflows force operations engineers to move between validators, logs, historical data, runbooks, policy documents, tickets, and approval systems. A naive LLM integration merely replaces that tool-switching with a conversational interface while introducing new risks.

SentinelFlow instead automates the full investigation lifecycle while keeping financial authority outside the model boundary.

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

Lens currently covers return intelligence, incident trends, validation findings, file operations, and agent operations. Synthetic demo observations are explicitly marked as synthetic and **cannot satisfy authoritative financial evidence requirements**.

### 7. Security and failure containment

- Google Model Armor screens model-bound inputs and outputs.
- Prompt content is partitioned by trust domain and minimized before inference.
- Sensitive financial data is redacted/minimized at the model boundary.
- Data-sovereignty violations fail closed instead of silently rerouting.
- Side-effectful tool executions are not blindly replayed after an uncertain failure.
- If the AI tier is unavailable, deterministic validation and quarantine remain authoritative.

**AI failure cannot become financial failure.**

---

## How We Built It

SentinelFlow was built in layers so that every new AI capability inherited an existing deterministic control boundary rather than creating a parallel path around it.

### Step 1 — Domain research and failure modeling

We started with the operational mechanics of NACHA files, ACH return semantics, routing validation, batch/file control totals, artifact lineage, dual control, and payment-operations failure modes. The goal was to define what must remain deterministic before introducing an LLM.

This led to explicit distinctions such as:

```text
risk score        != financial decision
memory recall     != evidence
verification      != approval
approval          != release
```

### Step 2 — Deterministic Go control plane

The first executable layer was the Go control plane: parser, validator, policy engine, workflow state machine, audit chain, candidate generation, and independent verification.

Important design requirements included fail-closed behavior, idempotency, optimistic concurrency, state binding, and preserving the original artifact during remediation.

### Step 3 — Google ADK specialist fleet

Instead of one general-purpose agent, we created a fixed roster of specialist agents with explicit roles and permissions. Each agent has a canonical manifest that constrains its tool access, autonomy tier, memory access, turn budget, timeout, output schema, and denied capabilities.

The Incident Commander coordinates investigation while specialist agents perform diagnosis, policy/SLA interpretation, memory retrieval, remediation planning, independent critique, and return-risk analysis.

### Step 4 — Governed model boundary and Model Armor

Gemini inference was placed behind a shared guarded-model boundary that performs data minimization, trust partitioning, Model Armor screening, schema validation, evidence grounding, and failure classification.

The important design choice is that a safe model output is still only an **AI output**. It does not become an executable financial decision until the deterministic control plane accepts the relevant operation.

### Step 5 — Lens operational intelligence

Lens was built to answer operational questions without exposing raw SQL authority to either the browser or the model. Natural-language intent is compiled into a typed semantic query, executed through the Tool Gateway, and persisted with query/result hashes and provenance.

### Step 6 — Google Cloud deployment and proof

The UI and Go gateway were containerized for Cloud Run, the ADK agent topology was deployed to Vertex AI Agent Runtime / Reasoning Engine, Model Armor was configured in `us-central1`, and reproducible demo/test data was staged in Cloud Storage.

---

## Research & Demo Data

SentinelFlow deliberately separates realistic test data from authoritative evidence.

### Moov ACH
Used for standards-oriented NACHA fixtures and operational test vectors across common ACH record structures.

### Moov ACH Test Harness
Used as a reference for return, correction, reconciliation, trace-matching, and delayed-response scenarios.

### IBM synthetic AML data
Used for controlled historical-behavior experiments and analytics. Synthetic labels are treated as evaluation ground truth, not production evidence.

### SentinelFlow deterministic scenarios
Generated locally for specific failure modes such as malformed records, routing failures, control mismatches, duplicate-like operational scenarios, and adversarial text embedded inside valid business records.

### Lens synthetic history
Synthetic analytical rows are tagged non-authoritative by design. They can demonstrate trend discovery but cannot satisfy a financial control or mint trusted evidence.

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

## Challenges We Ran Into

### Keeping probabilistic reasoning away from deterministic financial truth
The easiest architecture would have been to ask Gemini to inspect a file and decide what to do. That would also have been the least defensible architecture. We instead had to define exactly where model reasoning ends and deterministic authority begins.

### Long-running workflow correctness
Autonomous systems must survive retries, duplicate triggers, stale state, partial failures, and uncertain side effects. Idempotency and resource preconditions therefore became first-class parts of the workflow rather than afterthoughts.

### Memory versus evidence
Historical context is useful for diagnosis, but remembered information can be stale or wrong. SentinelFlow therefore allows memory to inform investigation while preventing memory from directly becoming authoritative evidence.

### Secure remediation
The system must be useful enough to propose and prepare repairs without allowing an agent to rewrite the artifact under investigation. The solution is immutable candidate derivation with explicit parent hashes and independent re-verification.

### Prompt injection inside business data
Financial files can contain untrusted text fields. SentinelFlow treats that content as untrusted input, screens model-bound content, and keeps financial decisions outside the LLM boundary even if the content reaches inference.

### Honest demo provenance
Synthetic data is useful for showing behavior but dangerous if presented as real-world evidence. The project explicitly tags synthetic observations and prevents them from satisfying financial evidence requirements.

---

## Accomplishments We're Proud Of

- **A real multi-agent control architecture rather than a chat wrapper.** The seven-agent fleet operates inside an independently enforced capability model.
- **Deterministic dominance over AI output.** Financial validation, policy, state transitions, verification, and release authority remain outside Gemini.
- **Immutable remediation lineage.** The original artifact is preserved while proposed repairs become separately hashed candidates.
- **Independent verification.** Candidate validation is re-run by deterministic Go code instead of trusting the agent that proposed the remediation.
- **Enterprise-style safety controls.** Manifest allowlists, denied capabilities, Model Armor, data minimization, sovereignty checks, tenant scope, timeouts, and audit persistence are part of the execution path.
- **Governed analytics.** Lens converts questions into typed semantic intent rather than arbitrary SQL and records reproducible query/result provenance.
- **Live Google Cloud deployment evidence.** The project includes Cloud Run endpoints and a managed Vertex AI Agent Runtime resource for judging and demonstration.

---

## What We Learned

- In regulated automation, **better authority boundaries matter more than simply asking the model to be careful**.
- Agent memory is useful context, but **memory is not evidence**.
- A probabilistic risk score can prioritize investigation, but **risk is not policy**.
- Remediation should create immutable candidates with provenance, not rewrite evidence in place.
- Failure recovery and idempotency matter as much as the happy path for long-running agents.
- Multi-agent systems become more trustworthy when each specialist has narrow, inspectable permissions rather than one shared super-agent identity.
- Guardrails improve safety, but the strongest protection is architectural: the model should never possess authority it does not need.

---

## What's Next

1. **BigQuery / BigLake analytics** — move Lens from local analytical stores to governed large-scale historical analysis.
2. **Real-time payment rails** — extend deterministic parsing and control concepts beyond batch ACH into streaming and ISO 20022-style payment messages.
3. **Broader enterprise connectors** — expose governed capabilities through standardized enterprise integration layers while preserving the same Tool Gateway authority model.
4. **Multi-region data-sovereignty deployment** — bind agent execution, memory, evidence, and model routing to regional policy constraints.
5. **Additional deterministic remediation operations** — expand the safe operation allowlist while keeping original artifacts immutable.
6. **Deeper operational observability** — richer end-to-end traces for agent dispatch, tool calls, policy decisions, retries, and verification state.

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
