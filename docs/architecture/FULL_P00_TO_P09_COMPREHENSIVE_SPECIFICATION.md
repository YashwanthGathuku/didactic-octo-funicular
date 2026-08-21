# SentinelFlow Governed Agentic Architecture (SGACA)
# Full Technical Architecture & Implementation Specification (Phases P00 – P09)

> **Platform**: Google Gemini (`gemini-3.5-flash`), Google Agent Development Kit (ADK), Google Cloud Model Armor  
> **Core Engine**: Go 1.24 High-Assurance Control Plane, Zero-Copy NACHA Engine, RFC 8785 Canonical Policy Engine  
> **Target Track**: Fortified Enterprise Fleet  
> **Repository**: `YashwanthGathuku/didactic-octo-funicular`  
> **Status**: Comprehensive Production-Grade Implementation & Mathematical Specification (100% Pass Rate Across 95 Adversarial Scenarios)

---

## 1. Executive Summary & Core Architectural Mission

**SentinelFlow** is an autonomous, high-assurance financial file reliability and pre-ledger ingress platform. It processes, validates, and routes mission-critical batch payment files (NACHA ACH, Fedwire, ISO 20022) with zero-tolerance for silent data corruption, unauthorized state mutations, or uncalibrated autonomous AI actions.

The **SentinelFlow Governed Agentic Control Architecture (SGACA)** deploys a fortified multi-agent fleet running in sandboxed execution environments, surrounded by pre- and post-invocation **Google Cloud Model Armor** content-safety screening, bounded by an immutable **Policy Engine**, and governed by an authoritative **Go Control Plane**.

```
═════════════════════════════════════════════════════════════════════════════════════════
                            HUMAN OPERATIONS & SUPERVISORY TIER
═════════════════════════════════════════════════════════════════════════════════════════
                                        │ Dual-Control Review Queue
                                        │ (Identity-Bound Approval & Cryptographic Integrity)
                                        ▼
┌───────────────────────────────────────────────────────────────────────────────────────┐
│                     GOVERNED MULTI-AGENT CONTROL PLANE (PYTHON / ADK)                 │
│                                                                                       │
│   ┌───────────────────────────────────────────────────────────────────────────────┐   │
│   │               Google Cloud Model Armor Inline Dual-Screening                  │   │
│   │       (:sanitizeUserPrompt ──> Gemini 3.5 Flash ──> :sanitizeModelResponse)   │   │
│   └───────────────────────────────────────┬───────────────────────────────────────┘   │
│                                           │ Sanitized Prompt / Response               │
│                                           ▼                                           │
│   ┌───────────────────────────────────────────────────────────────────────────────┐   │
│   │                 IncidentCommanderAgent (Autonomy Level A1)                    │   │
│   └───────┬───────────────────────────────┬───────────────────────────────┬───────┘   │
│           │                               │                               │           │
│           ▼ (Parallel ADK Execution)      ▼ (Parallel ADK Execution)      ▼           │
│   ┌─────────────────┐             ┌─────────────────┐             ┌───────────────┐   │
│   │ DiagnosisAgent  │             │ PolicySLAAgent  │             │ Remediation   │   │
│   │  (Level A1)     │             │  (Level A1)     │             │ Agent (A2)    │   │
│   └─────────────────┘             └─────────────────┘             └───────┬───────┘   │
│           ▲                               ▲                               │           │
│           └───────────────────────────────┼───────────────────────────────┘           │
│                                           ▼                                           │
│                                   ┌───────────────┐                                   │
│                                   │ VerifierAgent │ (Independent Critic)              │
│                                   │  (Level A1)   │                                   │
│                                   └───────┬───────┘                                   │
└───────────────────────────────────────────┼───────────────────────────────────────────┘
                                            │ Typed Intent Proposals & Critic Assessments
                                            │ (Zero Mutation Authority)
                                            ▼
┌───────────────────────────────────────────────────────────────────────────────────────┐
│                    DETERMINISTIC FINANCIAL ENGINE (AUTHORITATIVE GO)                  │
│                                                                                       │
│  ┌──────────────────────┐  ┌──────────────────────┐  ┌─────────────────────────────┐  │
│  │ Hardened ToolGateway │  │  Deterministic Policy│  │  Authoritative NACHA Engine │  │
│  │ • 5-Term Conjunction │  │  • RFC 8785 Canonical│  │  • Modulo-10 Routing Checks │  │
│  │ • Strict Manifests   │  │  • SF-SAFE Rulepack  │  │  • Mod 10^10 Hash Sums      │  │
│  │ • Shadow Sandboxing  │  │  • TOCTOU Binding    │  │  • Debit/Credit Accumulators│  │
│  └──────────────────────┘  └──────────────────────┘  └─────────────────────────────┘  │
│  ┌──────────────────────┐  ┌──────────────────────┐  ┌─────────────────────────────┐  │
│  │ Candidate Service    │  │ Verification Service │  │ Evidence & ObjectStore      │  │
│  │ • Parent Immutability│  │ • 12 Integrity Checks│  │ • Deterministic Object Keys │  │
│  │ • Deterministic Math │  │ • Dual-Run Validator │  │ • Append-Only Hash Ledger   │  │
│  │ • Windows A-F Reconc.│  │ • Deterministic Domin│  │ • Linear SHA-256 Outbox     │  │
│  └──────────────────────┘  └──────────────────────┘  └─────────────────────────────┘  │
└───────────────────────────────────────────────────────────────────────────────────────┘
```

---

## 2. Fundamental Mathematical Foundations & Invariant Proofs

### 2.1 The Autonomous Execution Conjunction
Autonomous action in SentinelFlow is defined as the strict mathematical intersection of principal identity, tool capabilities, manifest schemas, deterministic policy, active obligations, and resource freshness:

$$\text{AUTONOMY} = \text{Identity} \cap \text{Capability} \cap \text{ToolManifest} \cap \text{Policy} \cap \text{Obligations} \cap \text{ResourceFreshness}$$

If any term in this conjunction evaluates to false, autonomy drops to zero ($\text{AUTONOMY} = \emptyset$), forcing an immediate fail-closed termination.

### 2.2 The Upstream Guardrail Non-Authority Invariant
Model Armor functions exclusively as a content-filtering boundary defense. It possesses zero authorization capability:

$$\text{ModelArmorPass} \neq \text{Authorization}$$

$$\text{Authorization} = \text{IdentityValid} \land \text{PolicyAllowed} \land \text{ManifestApproved} \land \text{DeterministicVerificationPass} \land \text{HumanAuthorized}$$

### 2.3 The Original Parent Immutability Invariant
Under all conditions (including multi-attempt candidate generation, crash recovery, and verifier audits), quarantined original files in ObjectStore are permanent and bitwise immutable:

$$\forall \text{ operations } \mathcal{O}, \quad \text{SHA256}(\text{parent}_{\text{after}}) \equiv \text{SHA256}(\text{parent}_{\text{before}})$$

Any violation of this equation immediately halts the engine with `ErrOriginalMutated` and triggers `OutcomeCorruptionDetected`.

### 2.4 The Maker-Checker & Deterministic Dominance Invariant
The agent proposing a candidate repair cannot verify or approve it. Furthermore, deterministic validation strictly dominates probabilistic model opinion:

$$\text{RemediationAgent} \neq \text{VerifierAgent} \land \text{CriticOpinion} \neq \text{VerificationAuthority}$$

$$\text{DeterministicValidation} = \text{FAIL} \implies \text{VerificationOutcome} = \text{FAIL} \quad (\forall \text{ Critic Assessments})$$

$$\text{DeterministicValidation} = \text{PASS} \land \text{CriticRisk} = \text{HIGH} \implies \text{WorkflowState} = \text{HUMAN\_INVESTIGATION\_REQUIRED}$$

### 2.5 The Authoritative Evidence Grounding Invariant
All evidence citations emitted by any specialist agent must be a strict mathematical subset of the cryptographic `AuthorizedEvidenceSet` injected by the Go Control Plane:

$$E_{\text{claimed}} \subseteq E_{\text{authorized}}$$

Any reference $r \notin E_{\text{authorized}}$ immediately triggers `GroundingViolationError` and fails closed with `UNGROUNDED_REJECTED`.

---

## 3. The 12 Non-Negotiable Architectural Laws of SGACA

1. **Deterministic Software Establishes Financial Truth**: No AI model output may overrule a deterministic checksum, Modulo-10 check digit, batch hash sum, or parser error.
2. **Original Artifacts are Bitwise Immutable**: Quarantined files are never mutated, patched in-place, or overwritten. Corrections exist exclusively as new **derived candidate artifacts** referencing the immutable parent.
3. **Bounded Explicit Agent Capabilities**: Every agent operates strictly within a declared, static, immutable capability manifest (`AgentManifest`).
4. **Typed Tools Only**: Free-form raw SQL queries, arbitrary shell commands, unconstrained filesystem traversal, and untyped network requests are prohibited by construction.
5. **Policy Pre-Evaluation**: Policy boundaries are evaluated deterministically before prompt construction and model inference.
6. **Infrastructure-Bound Tenancy**: Tenancy is established from cryptographic claims in the verified JWT context (`X-Sentinel-Tenant`), never from agent inference or user prompt text.
7. **Zero Direct Secret Access**: Agents never hold database credentials, API secrets, KMS private keys, or signing tokens.
8. **Independent Verification (Maker-Checker)**: The `RemediationAgent` that drafts a fix cannot verify or approve its own proposal. A separate `VerifierAgent` (Critic) and the Go Verification Service execute 12 deterministic checks under **Deterministic Dominance**.
9. **Advisory Memory Isolation**: Cross-session Memory Bank contents provide pattern context only; they can never waive a validation rule or grant an approval.
10. **Zero Retention of Private Chain-of-Thought**: Model scratchpads and ungrounded intermediate thoughts are discarded; only structured `AgentStep` execution records are retained.
11. **Zero Autonomous Release Authority**: Agents have zero capability to execute file release, bank transmission, or funds settlement.
12. **Identity-Bound Dual-Control Release**: Candidate release requires identity-bound dual-control approval from two distinct authorized human operators with cryptographic artifact and policy integrity binding.

---

## 4. Phase-by-Phase Technical Specifications & Algorithmic Details

---

### Phase P00: Baseline Ingress & Authoritative Financial Core

#### 1. Mission & Scope
Establishes the authoritative, zero-copy financial validation engine, NACHA ACH protocol parser, append-only SQLite/PostgreSQL transactional persistence layer, and distributed lease-based worker concurrency pool.

#### 2. Key Code Files & Artifacts
- `gateway/internal/nacha/parse.go`: Streaming fixed-width NACHA parser.
- `gateway/internal/nacha/validate.go`: Authoritative rule validator for 94-character records, batch headers, entry details, addenda, batch controls, and file controls.
- `gateway/internal/nacha/types.go`: Canonical NACHA domain models.
- `gateway/migrate.go` & `gateway/migrations/001-017`: Core transactional schemas.
- `gateway/worker.go` & `gateway/worker_test.go`: Distributed lease-based worker pool (40 artifacts settled in 1.135s under 24 parallel workers with zero lease loss).

#### 3. Core Algorithms Implemented
- **Zero-Copy Fixed-Width Record Parsing**: Parses 94-byte fixed-width records across File Header (`1`), Batch Header (`5`), Entry Detail (`6`), Addenda (`7`), Batch Control (`8`), and File Control (`9`). Validates character set constraints (ASCII 32–126).
- **Federal Reserve Modulo-10 Routing Checksum Algorithm**:
  For an 8-digit transit routing number $d_1 d_2 \dots d_8$, computes check digit $d_9$:
  $$\text{Checksum} = (3(d_1 + d_4 + d_7) + 7(d_2 + d_5 + d_8) + 1(d_3 + d_6)) \pmod{10}$$
  $$d_9 = (10 - \text{Checksum}) \pmod{10}$$
- **NACHA Batch Entry Hash Accumulator Summation**:
  Computes the 10-digit entry hash accumulator sum from the first 8 digits of Receiving DFI Identification across all entry detail records in the batch, truncated modulo $10^{10}$:
  $$\text{BatchEntryHash} = \left( \sum_{i=1}^{N_{\text{entries}}} \text{RoutingPrefix}_8(i) \right) \pmod{10^{10}}$$
  File Control Entry Hash equals the sum of all Batch Entry Hashes modulo $10^{10}$:
  $$\text{FileEntryHash} = \left( \sum_{b=1}^{N_{\text{batches}}} \text{BatchEntryHash}(b) \right) \pmod{10^{10}}$$
- **Debit vs Credit Transaction Classification**:
  Authoritative classification using `nacha.IsDebitTransaction(txCode)`:
  - **Debit Codes**: `27`, `28`, `29` (Checking Debit), `37`, `38`, `39` (Savings Debit), `47`, `48`, `49` (GL Debit), `55`, `56` (Loan Debit).
  - **Credit Codes**: `22`, `23`, `24` (Checking Credit), `32`, `33`, `34` (Savings Credit), `42`, `43`, `44` (GL Credit), `52`, `53`, `54` (Loan Credit).
- **Block Count Ceiling Calculation**:
  Computes total 10-record blocks including line padding:
  $$\text{BlockCount} = \left\lceil \frac{N_{\text{total\_records}}}{10} \right\rceil$$

---

### Phase P01: High-Assurance Evidence Ledger & Cryptographic Provenance

#### 1. Mission & Scope
Implements an append-only linear hash-chain audit ledger, cryptographic outbox pattern, state machine transitions, and dual-control approval schemas.

#### 2. Key Code Files & Artifacts
- `gateway/internal/ledger/`: Linear append-only SHA-256 hash chaining engine.
- `gateway/internal/domain/agent_workflow.go`: Workflow states and transitions.
- `gateway/migrations/010_evidence_ledger.sql` & `015_kms_checkpoints.sql`: Checkpoint and ledger schemas.
- `gateway/agent_workflow_service.go`: Transactional workflow state machine and outbox journaling.

#### 3. Core Algorithms Implemented
- **Linear Append-Only Hash Chain**:
  Every ledger entry $i$ cryptographically binds to entry $i-1$:
  $$H_0 = \text{GenesisHash}$$
  $$H_i = \text{SHA256}(H_{i-1} \parallel \text{TenantID} \parallel \text{EventType} \parallel \text{CanonicalPayload}_i \parallel \text{Timestamp}_i)$$
- **Transactional Outbox Event Journaling**:
  Guarantees zero dual-write inconsistencies by persisting domain state changes and outbox events in a single atomic database transaction with idempotency deduplication keys (`ik-<event>-<id>`).

---

### Phase P02: Connector Platform & Secure Transport

#### 1. Mission & Scope
Provides enterprise transport adapters (SFTP, HTTP/REST, Webhooks) with tenant directory sandboxing, HMAC-SHA256 signature verification, and dead-letter queues.

#### 2. Key Code Files & Artifacts
- `gateway/internal/sftp/`: SFTP server and client adapters with chroot isolation.
- `gateway/internal/egress/`: Egress dispatch engine and bank transmission simulators.
- `gateway/internal/connectors/`: Ingress connector abstraction.

#### 3. Core Algorithms Implemented
- **HMAC-SHA256 Webhook Verification**:
  $$\text{Signature} = \text{HMAC-SHA256}(K_{\text{secret}}, \text{Timestamp} \parallel "." \parallel \text{PayloadBody})$$
- **Exponential Backoff with Full Jitter**:
  $$t_{\text{retry}} = \min(t_{\text{max}}, t_{\text{base}} \cdot 2^{\text{attempt}}) \times \text{Uniform}(0.5, 1.5)$$

---

### Phase P03: Deterministic Policy Engine & Canonical Evaluation

#### 1. Mission & Scope
Enforces deterministic, pre-reasoning policy evaluations using RFC 8785 Canonical JSON hashing, SF-SAFE safety rulesets, and time-of-check-to-time-of-use (TOCTOU) invariant validation.

#### 2. Key Code Files & Artifacts
- `gateway/internal/policy/evaluator.go`: Authoritative policy evaluator.
- `gateway/internal/policy/canonical_json.go`: RFC 8785 canonical JSON serializer.
- `gateway/internal/policy/safety_rules.go`: SF-SAFE-001 through SF-SAFE-005 rulesets.
- `gateway/internal/policy/types.go` & `store.go`: Versioned policy bundle store.
- `ai-tier/models/policy.py`: Python mirror models.

#### 3. Core Algorithms Implemented
- **RFC 8785 Canonical JSON Serialization**:
  Recursively sorts all JSON object keys lexicographically by UTF-16 code units, formats floats with IEEE 754 precision without trailing zeroes, and encodes strings strictly in UTF-8 without unnecessary escapes.
- **Cryptographic Policy Bundle Integrity Binding**:
  Computes SHA-256 hash over canonical JSON representation of all active rules:
  $$\text{PolicyBundleHash} = \text{SHA256}(\text{CanonicalJSON}(\mathcal{R}_{\text{rules}}))$$
- **SF-SAFE Safety Rules Tree**:
  - `SF-SAFE-001`: Mandatory Quarantining of Checksum/Format Failures.
  - `SF-SAFE-002`: Strict Read-Only Agent Capability Ceiling (Autonomy $\le$ A2).
  - `SF-SAFE-003`: Ban on Autonomous Release of Financial Artifacts.
  - `SF-SAFE-004`: Remediation Limited to Allowlisted Control Total Recomputation.
  - `SF-SAFE-005`: Identity-Bound Dual-Control Approval Required for Release.
- **TOCTOU Policy Invariant Enforcement**:
  Asserts that `PolicyBundleHash` at candidate creation matches `PolicyBundleHash` at verification. Any divergence rejects candidate progression.

---

### Phase P04: Hardened Tool Gateway & Agent Sandboxing

#### 1. Mission & Scope
Establishes the exclusive Tool Gateway mediation boundary between AI agents and system tools, enforcing the 5-term capability conjunction, manifest validation, shadow execution modes, and data loss prevention.

#### 2. Key Code Files & Artifacts
- `gateway/internal/toolgateway/gateway.go`: Core Tool Gateway mediation engine.
- `gateway/internal/toolgateway/tools.go`: Static tool definitions and registry.
- `gateway/internal/toolgateway/types.go`: Request, Response, and Manifest models.
- `gateway/migrations/018_tool_gateway.sql`: Durable tool manifest and invocation audit tables.
- `ai-tier/tools/gateway_client.py`: Python Tool Gateway HTTP adapter.

#### 3. Core Algorithms Implemented
- **5-Term Capability Conjunction Access Control**:
  Evaluates:
  $$\text{AllowTool} \iff \text{AgentRegistered} \land \text{ToolInAllowedList} \land \text{ManifestHashMatched} \land \text{PolicyVerdict} \in \{\text{ALLOW}, \text{ALLOW\_WITH\_OBLIGATIONS}\} \land \text{ObligationsSatisfied}$$
- **Tool Manifest Canonical Hash Verification**:
  Asserts caller's `manifest_hash` matches authoritative database entry:
  $$\text{ManifestHash} = \text{SHA256}(\text{CanonicalJSON}(\text{ToolManifest}))$$
- **Shadow Execution Sandboxing**:
  When `ShadowMode = true`, mutative tools execute internal logic but suppress disk/database writes, returning simulation receipts.
- **Output Redaction & DLP Filtering**:
  Scans all tool outputs for unredacted 10–17 digit account numbers, 9-digit routing numbers, private keys, and credential strings before returning data to the AI tier.

---

### Phase P05: Governed Read-Only AI Incident Analyst (Single-Agent Diagnosis)

#### 1. Mission & Scope
Integrates Google Gemini 3.5 Flash and Google ADK into a governed, read-only incident analyst (`DiagnosisAgent`, Autonomy Level A1) with 4-domain prompt trust partitioning and strict evidence grounding.

#### 2. Key Code Files & Artifacts
- `ai-tier/agents/diagnosis.py`: `DiagnosisAgent` implementation.
- `ai-tier/contracts/diagnosis.py`: Structured `DiagnosisOutput` and `DiagnosisRunResponse` Pydantic models.
- `ai-tier/guardrails/prompt.py`: `PromptTrustPartitioner` compiler.
- `ai-tier/guardrails/evidence.py`: `EvidenceGroundingVerifier` and `AuthorizedEvidenceSet`.
- `ai-tier/evals/runner.py`: Single-agent adversarial evaluation suite (14 scenarios).

#### 3. Core Algorithms Implemented
- **4-Domain Prompt Trust Partitioning**:
  Isolates context into 4 disjoint security domains:
  - `Domain 1: SYSTEM_POLICY`: Authoritative system instructions and safety invariants.
  - `Domain 2: TRUSTED_CONTEXT`: Authenticated workflow, incident, and tenant metadata.
  - `Domain 3: UNTRUSTED_FINANCIAL_CONTENT`: Minimised validation findings fenced inside `<untrusted_content>` tags.
  - `Domain 4: TOOL_OUTPUT`: Read-only deterministic tool execution receipts.
- **Strict Evidence Grounding Verification Algorithm**:
  Extracts all citations $\{c_1, c_2, \dots, c_k\}$ from `output.evidence_refs` and hypothesis references. Asserts:
  $$\forall c \in C, \quad c \in E_{\text{authorized}}$$
  If $C \setminus E_{\text{authorized}} \neq \emptyset \implies$ fails closed with `GroundingViolationError`.
- **Deterministic Rule-Grounded Fallback Engine**:
  Provides a bitwise deterministic diagnosis baseline when credentials are absent or upstream APIs fail.

---

### Phase P06: Governed Multi-Agent Orchestration & Structured Delegation

#### 1. Mission & Scope
Implements a multi-agent orchestration shell featuring `IncidentCommanderAgent` (A1), parallel specialist execution (`DiagnosisAgent` + `PolicySLAAgent`) via Google ADK `ParallelAgent`, bounded depth-2 delegation, and partial specialist failure isolation.

#### 2. Key Code Files & Artifacts
- `ai-tier/agents/commander.py`: `IncidentCommanderAgent` planning and synthesis.
- `ai-tier/agents/policy_sla.py`: `PolicySLAAgent` policy interpreter and deterministic SLA calculator.
- `ai-tier/orchestrator/fleet.py`: `MultiAgentWorkflowOrchestrator` shell.
- `ai-tier/contracts/orchestration.py`: `CommanderPlan`, `CommanderSynthesis`, and `SpecialistResult`.
- `ai-tier/contracts/manifests.py`: `FIXED_AGENT_ROSTER` immutable definitions.
- `ai-tier/evals/multi_agent_runner.py` & `adversarial_multi_agent.json`: 16 multi-agent adversarial scenarios.

#### 3. Core Algorithms Implemented
- **Fixed Agent Roster Membership Validation**:
  Asserts that delegation targets belong strictly to `FIXED_AGENT_ROSTER` (`['IncidentCommanderAgent', 'DiagnosisAgent', 'PolicySLAAgent', 'RemediationAgent', 'VerifierAgent']`). Rejects unknown or invented agent names (`SuperAdminAgent`).
- **Bounded Delegation & Loop Prevention**:
  Enforces maximum delegation depth = 2. Recursion or circular delegation attempts fail closed.
- **Parallel Specialist Execution with Isolated State Namespaces**:
  Executes `DiagnosisAgent` and `PolicySLAAgent` concurrently using Google ADK `ParallelAgent`. Captures independent execution results in distinct namespace keys (`diagnosis_result`, `policy_sla_result`).
- **Deterministic SLA Cutoff Computation**:
  Computes delivery time remaining directly from authoritative Unix timestamps:
  $$t_{\text{remaining}} = \max(0, t_{\text{cutoff}} - t_{\text{evaluation}})$$
- **Partial Specialist Failure Isolation**:
  If one specialist times out or fails grounding, the orchestrator isolates the failure (`PARTIAL_SPECIALIST_FAILURE`) without corrupting overall workflow state or crashing the remaining specialist.

---

### Phase P07: Governed Sandbox Candidate Remediation

#### 1. Mission & Scope
Establishes the governed sandbox remediation engine where `RemediationAgent` (A2) proposes allowlisted mathematical repair intents, while the Go Control Plane executes authoritative byte generation, original parent byte re-read verification, deterministic object key hashing, crash consistency across Windows A-F, and orphan reconciliation.

#### 2. Key Code Files & Artifacts
- `ai-tier/agents/remediation.py`: `RemediationAgent` proposal generator.
- `ai-tier/contracts/remediation.py`: `RemediationPlan` and `RemediationOperation`.
- `gateway/internal/candidate/service.go`: Authoritative candidate generation service.
- `gateway/internal/candidate/reconcile.go`: Cross-resource orphan reconciliation engine.
- `gateway/internal/objectstore/objectstore.go`: `DeterministicKey` generator.
- `gateway/migrations/020_remediation_candidate_derivations.sql`: Immutable derivation schema.
- `ai-tier/evals/remediation_runner.py` & `adversarial_remediation.json`: 20 remediation adversarial scenarios.

#### 3. Core Algorithms Implemented
- **Formal Invariant**:
  $$\text{AgentProposal} \neq \text{CandidateMutationAuthority}$$
- **Allowlisted Operation Validation**:
  Only two structured operations are permitted:
  1. `RECOMPUTE_BATCH_CONTROL_TOTAL`: Recomputes batch debit, credit, entry count, and entry hash sum from entry records.
  2. `RECOMPUTE_FILE_CONTROL_TOTAL`: Recomputes file-level batch count, block count, entry count, debits, credits, and entry hash sum.
  Arbitrary byte patches or raw payload replacements are rejected by the Go parser.
- **Original Parent Byte Re-Read Verification**:
  Before candidate generation, reads original bytes from ObjectStore $\implies H_{\text{before}} = \text{SHA256}(\text{parent})$. After writing candidate bytes, performs a fresh read from ObjectStore $\implies H_{\text{after}} = \text{SHA256}(\text{parent})$. Asserts $H_{\text{before}} \equiv H_{\text{after}}$.
- **Deterministic Logical ID & Object Key Derivation**:
  $$\text{LogicalPayload} = \text{TenantID} \parallel ":" \parallel \text{WorkflowID} \parallel ":" \parallel \text{AttemptNumber} \parallel ":" \parallel \text{ParentSHA256} \parallel ":" \parallel \text{PlanHash}$$
  $$\text{CandidateLogicalID} = \text{SHA256}(\text{LogicalPayload})$$
  $$\text{StorageKey} = \text{path.Join}("tenant", \text{TenantID}, "candidates", \text{CandidateLogicalID})$$
- **PutAndVerify Storage Protocol**:
  Writes candidate bytes to ObjectStore. If object exists, verifies existing bytes match candidate SHA-256. Re-reads candidate bytes from ObjectStore and validates cryptographic integrity.
- **Crash Consistency State Machine (Windows A – F)**:
  - *Window A (Crash before object write)*: Retry safely generates object.
  - *Window B (Object written, crash before DB commit)*: Retry reuses deterministic storage key and completes DB commit.
  - *Window C (DB committed, crash before outbox)*: Reconciliation confirms consistency.
  - *Window D (Candidate persisted, crash before validation)*: Validation safely completes.
  - *Window E (Validation finished, crash before workflow transition)*: Replays persisted derivation result.
  - *Window F (Workflow committed, duplicate retry arrives)*: Returns identical idempotent `CandidateResult`.
- **Attempt Bounds**:
  Enforces $1 \le \text{AttemptNumber} \le 3$. Attempt 4 is rejected with `ErrMaxAttemptsExceeded`.

---

### Phase P08: Independent Deterministic Verification & Verifier Critic

#### 1. Mission & Scope
Implements independent maker-checker verification combining an authoritative Go Verification Service executing 12 deterministic checks and an advisory Python `VerifierAgent` (CriticAgent, Autonomy Level A1).

#### 2. Key Code Files & Artifacts
- `gateway/internal/verification/service.go`: Authoritative verification service.
- `gateway/internal/verification/types.go`: 12 check types and verification outcomes.
- `gateway/internal/verification/hash.go`: RFC 8785 canonical verification hash calculator.
- `gateway/migrations/021_candidate_verifications.sql`: `candidate_verifications`, `verification_checks`, and `critic_assessments` tables.
- `ai-tier/agents/verifier.py`: `VerifierAgent` (Critic) implementation.
- `ai-tier/contracts/verification.py`: `CriticAssessment` schema.
- `ai-tier/evals/verification_runner.py` & `adversarial_verification.json`: 20 verification adversarial scenarios.

#### 3. Core Algorithms Implemented
- **The 12 Deterministic Verification Checks**:
  1. `CheckParentHashMatch`: Verifies original bytes in ObjectStore match derivation parent SHA-256.
  2. `CheckCandidateHashMatch`: Verifies candidate bytes in ObjectStore match derivation candidate SHA-256.
  3. `CheckDerivationHashMatch`: Recomputes canonical derivation hash and compares with recorded hash.
  4. `CheckParentBindingMatch`: Verifies parent artifact ID matches workflow origin artifact.
  5. `CheckWorkflowBindingMatch`: Verifies workflow ID binding consistency.
  6. `CheckAttemptBindingMatch`: Verifies attempt number matches derivation attempt.
  7. `CheckPlanHashMatch`: Verifies remediation plan hash matches recorded plan.
  8. `CheckValidatorPass`: Re-runs `nacha.Validate(candidateBytes)` independently (producing P08 run).
  9. `CheckValidationResultMatch`: Compares P08 re-validation result with P07 recorded validation outcome.
  10. `CheckPolicyContextFresh`: Asserts current policy bundle hash matches derivation policy bundle hash.
  11. `CheckEvidenceContextValid`: Asserts evidence references belong to workflow authorized evidence set.
  12. `CheckSemanticDiffValid`: Validates semantic diff consistency between parent and candidate.
- **RFC 8785 Canonical Verification Hashing**:
  Computes SHA-256 digest over canonical JSON representation of all 12 checks, candidate hash, parent hash, derivation hash, validator run ID, and policy bundle hash:
  $$\text{VerificationHash} = \text{SHA256}(\text{CanonicalJSON}(\text{VerificationResult}))$$
- **Deterministic Dominance & Conflict Resolution**:
  - Deterministic FAIL + Critic CONSISTENT $\implies$ Candidate REJECTED (`OutcomeFail`).
  - Deterministic PASS + Critic HIGH-RISK CONCERN $\implies$ Workflow routed to `HUMAN_INVESTIGATION_REQUIRED` (`domain.WorkflowHumanReview`), blocking automated release.
  - Deterministic PASS + Critic CONSISTENT $\implies$ Workflow transitions to `WorkflowVerified`.

---

### Phase P09: Model Armor & AI Boundary Guardrails

#### 1. Mission & Scope
Integrates Google Cloud Model Armor regional REST API screening (:sanitizeUserPrompt, :sanitizeModelResponse) with ADC token management, data minimization, fail-closed availability semantics, polymorphic evidence grounding, and unified red-team evaluations.

#### 2. Key Code Files & Artifacts
- `ai-tier/armor/config.py`: `GuardrailMode`, `GuardrailDecision`, and `ModelArmorConfig`.
- `ai-tier/armor/provider.py`: `GuardrailProvider` interface and `GuardrailResult` model.
- `ai-tier/armor/client.py`: `GoogleModelArmorProvider` (regional REST) and `MockModelArmorProvider` (fault injection).
- `ai-tier/guardrails/boundary.py`: `GuardedModelBoundary` 8-step lifecycle wrapper.
- `ai-tier/evals/model_armor_runner.py` & `adversarial_model_armor.json`: 25 Model Armor adversarial scenarios.
- `ai-tier/evals/runner.py`: Unified 95-scenario fleet adversarial evaluation harness.
- `docs/architecture/AI_GUARDRAILS_MODEL_ARMOR.md`: Complete P09 architecture specification.

#### 3. Core Algorithms Implemented
- **Google Cloud Model Armor Regional REST Client**:
  Targets regional execution endpoint `https://modelarmor.us-central1.rep.googleapis.com/v1` with Application Default Credentials (ADC) token management.
- **8-Step Guarded Boundary Execution Lifecycle**:
  1. *Pre-Invocation Data Minimization*: Strips raw account numbers, routing numbers, and NACHA lines.
  2. *4-Domain Prompt Trust Partitioning*: Isolates system policy from untrusted financial content.
  3. *Input Content Hashing*: Computes SHA-256 digests over pre/post sanitized prompts.
  4. *Pre-Invocation Model Armor Screening*: Dispatches to `:sanitizeUserPrompt`. If blocked $\implies$ halts execution with `PROMPT_SECURITY_BLOCKED` (**0 Gemini calls**). If unavailable and mode is `REQUIRED` $\implies$ returns `GUARDRAIL_UNAVAILABLE`.
  5. *Governed Model Invocation*: Invokes `gemini-3.5-flash` with strict response schema.
  6. *Post-Invocation Model Armor Screening*: Dispatches to `:sanitizeModelResponse`. Blocks secret and PII leakage (`MODEL_RESPONSE_BLOCKED`).
  7. *Pydantic Schema Validation & Polymorphic Evidence Grounding*: Rejects malformed JSON and enforces $E_{\text{claimed}} \subseteq E_{\text{authorized}}$ across all schema types (`DiagnosisOutput`, `PolicySLAOutput`, `RemediationPlan`, `CommanderPlan`, `CriticAssessment`).
  8. *Deterministic Fallback & Audit Journaling*: Emits `BoundaryAuditRecord` with cryptographic provenance.
- **Pre-Invocation Data Minimization Regex Engine**:
  - Account Numbers: `\b\d{10,17}\b` $\implies$ `[ACCOUNT_REDACTED]`.
  - Routing Numbers: `\b\d{9}\b` $\implies$ `[ROUTING_REDACTED]`.
  - NACHA Records: `^[0-9A-Z\s]{94}$` $\implies$ `[NACHA_RECORD_REDACTED]`.
- **Fail-Closed Availability Isolation**:
  When Model Armor is `REQUIRED` and unavailable, AI assistants fail closed while the Go deterministic financial engine continues operating normally.

---

## 5. Comprehensive Fleet Evaluation Matrix (95 Adversarial Scenarios)

The unified evaluation runner (`python ai-tier/evals/runner.py`) executes all 95 adversarial scenarios across 5 distinct phases with a **100.0% pass rate (312/312 checks)**:

| Phase | Runner / Dataset | Scenario Count | Checks Passed | Pass Rate | Core Invariants Tested |
| :--- | :--- | :--- | :--- | :--- | :--- |
| **P05** | `single_agent_evals` | 14 Scenarios | 70 / 70 | **100.0%** | Direct injection, fake runbooks, raw SQL attempts, credential theft, read-only disclaimers |
| **P06** | `multi_agent_runner` | 16 Scenarios | 48 / 48 | **100.0%** | Specialist roster spoofing, recursive loops, policy disagreement, partial specialist timeouts |
| **P07** | `remediation_runner`| 20 Scenarios | 60 / 60 | **100.0%** | Parent byte tampering, arbitrary patches, attempt bound violations, crash recovery consistency |
| **P08** | `verification_runner` | 20 Scenarios | 59 / 59 | **100.0%** | Candidate corruption, derivation tampering, deterministic dominance, policy freshness |
| **P09** | `model_armor_runner` | 25 Scenarios | 75 / 75 | **100.0%** | Jailbreaks, metadata SSRF, homoglyphs, Model Armor timeouts, 503 outages, secret leakage |
| **Total** | **Unified Fleet Runner** | **95 Scenarios** | **312 / 312** | **100.0%** | **Comprehensive Autonomous Defense-in-Depth Across All Phases** |

---

## 6. Complete Repository Structure & File Taxonomy

```
fintech/
├── ai-tier/                                    # Python / Google ADK AI Control Plane
│   ├── agents/                                 # Specialist Agent Implementations
│   │   ├── commander.py                        # IncidentCommanderAgent (Autonomy A1)
│   │   ├── diagnosis.py                        # DiagnosisAgent (Autonomy A1)
│   │   ├── policy_sla.py                       # PolicySLAAgent (Autonomy A1)
│   │   ├── remediation.py                      # RemediationAgent (Autonomy A2)
│   │   └── verifier.py                         # VerifierAgent / CriticAgent (Autonomy A1)
│   ├── armor/                                  # Google Cloud Model Armor Subsystem
│   │   ├── config.py                           # GuardrailMode, GuardrailDecision, ModelArmorConfig
│   │   ├── provider.py                         # GuardrailProvider interface & GuardrailResult
│   │   └── client.py                           # GoogleModelArmorProvider & MockModelArmorProvider
│   ├── contracts/                              # Pydantic Schemas & Agent Manifests
│   │   ├── diagnosis.py                        # DiagnosisOutput & DiagnosisRunResponse
│   │   ├── manifests.py                        # FIXED_AGENT_ROSTER & AgentManifest
│   │   ├── orchestration.py                    # CommanderPlan & CommanderSynthesis
│   │   ├── policy_sla.py                       # PolicySLAOutput & SLA Context
│   │   ├── remediation.py                      # RemediationPlan & RemediationOperation
│   │   └── verification.py                     # CriticAssessment & Contradiction schemas
│   ├── evals/                                  # Adversarial Red-Team Evaluation Suites
│   │   ├── adversarial_model_armor.json        # 25 Model Armor scenarios
│   │   ├── adversarial_multi_agent.json        # 16 Multi-agent scenarios
│   │   ├── adversarial_remediation.json        # 20 Remediation scenarios
│   │   ├── adversarial_verification.json       # 20 Verification scenarios
│   │   ├── model_armor_runner.py               # Model Armor evaluation runner
│   │   ├── multi_agent_runner.py               # Multi-agent evaluation runner
│   │   ├── remediation_runner.py               # Remediation evaluation runner
│   │   ├── verification_runner.py              # Verification evaluation runner
│   │   └── runner.py                           # Unified 95-scenario fleet runner
│   ├── guardrails/                             # Guardrails & Model Boundary
│   │   ├── boundary.py                         # GuardedModelBoundary 8-step lifecycle wrapper
│   │   ├── evidence.py                         # Polymorphic EvidenceGroundingVerifier
│   │   └── prompt.py                           # 4-Domain PromptTrustPartitioner
│   ├── orchestrator/                           # Orchestration Runtime Shell
│   │   └── fleet.py                            # MultiAgentWorkflowOrchestrator with Google ADK
│   ├── persistence/                            # Non-authoritative session storage
│   │   └── store.py                            # SQLite-backed session store
│   ├── tests/                                  # Python Test Suite (78 Tests, 100% Pass)
│   │   ├── test_adk_introspection.py           # Google ADK agent & runner introspection
│   │   ├── test_adversarial_remediation.py     # Remediation security invariants
│   │   ├── test_commander.py                   # Commander planning and delegation
│   │   ├── test_diagnosis_agent.py             # Single-agent diagnosis
│   │   ├── test_durable_orchestration.py       # Checkpoints, restart recovery & concurrency
│   │   ├── test_evidence_grounding.py          # Evidence grounding verification
│   │   ├── test_live_tool_trajectory.py        # Tool calling trajectories
│   │   ├── test_model_armor_boundary.py        # 17 Model Armor boundary unit tests
│   │   ├── test_multi_agent_orchestrator.py    # Multi-agent stage execution
│   │   ├── test_no_python_artifact_access.py   # Python artifact access isolation
│   │   ├── test_no_python_db_authority.py      # Python database authority isolation
│   │   ├── test_policy_golden.py               # Golden policy decision tests
│   │   ├── test_policy_sla_agent.py            # Policy and SLA interpretation
│   │   ├── test_prompt_partition.py            # 4-Domain prompt partitioning
│   │   ├── test_remediation_agent.py           # Remediation proposal generation
│   │   ├── test_toctou_and_policy_authority.py # TOCTOU and policy authority tests
│   │   ├── test_tool_adapter.py                # Tool Gateway Python adapter
│   │   └── test_verifier_agent.py              # Verifier critic tests
│   └── tools/                                  # Tool Gateway Client & Adapters
│       ├── gateway_client.py                   # HTTP Tool Gateway client
│       └── tool_adapter.py                     # SentinelToolAdapter
├── docs/                                       # Architecture & Documentation
│   ├── architecture/                           # Deep Architecture Specifications
│   │   ├── AGENT_RUNTIME_FOUNDATION.md         # Google ADK foundation specification
│   │   ├── AI_GUARDRAILS_MODEL_ARMOR.md        # Model Armor architecture (P09)
│   │   ├── FULL_P00_TO_P09_COMPREHENSIVE_SPECIFICATION.md # Master specification (This Document)
│   │   ├── GOVERNED_REMEDIATION.md             # Governed remediation architecture (P07)
│   │   ├── INDEPENDENT_VERIFICATION.md         # Independent verification architecture (P08)
│   │   ├── MULTI_AGENT_ORCHESTRATION.md        # Multi-agent orchestration (P06)
│   │   ├── POLICY_ENGINE.md                    # Deterministic Policy Engine (P03)
│   │   ├── SENTINELFLOW_LENS.md                # SentinelFlow Lens blueprint
│   │   ├── SGACA.md                            # High-level SGACA baseline
│   │   └── TOOL_GATEWAY.md                     # Hardened Tool Gateway (P04)
│   ├── CAPABILITY_MATRIX.yaml                  # Machine-readable capability & test matrix
│   ├── DECISIONS.md                            # Architectural Decision Records (ADRs)
│   └── DEVPOST_SUBMISSION.md                   # Hackathon submission summary
├── gateway/                                    # Authoritative Go 1.24 Control Plane
│   ├── internal/                               # Internal Subsystems
│   │   ├── auth/                               # JWT / OIDC Authentication
│   │   ├── candidate/                          # Authoritative Candidate Service (P07)
│   │   │   ├── nacha_vectors_test.go           # Known NACHA vector tests
│   │   │   ├── property_test.go                # Candidate property tests
│   │   │   ├── fuzz_test.go                    # Candidate fuzz tests
│   │   │   ├── reconcile.go                    # Orphan reconciliation engine
│   │   │   ├── service.go                      # Candidate generation service
│   │   │   └── service_crash_test.go           # Crash fault injection tests (Windows A-F)
│   │   ├── connectors/                         # Ingress connectors
│   │   ├── domain/                             # Domain models & state machines
│   │   ├── egress/                             # Egress dispatch & settlement
│   │   ├── jobs/                               # Distributed background jobs
│   │   ├── ledger/                             # Append-only hash ledger
│   │   ├── nacha/                              # NACHA validator, parser & Mod-10 checks
│   │   ├── objectstore/                        # Object storage & DeterministicKey generator
│   │   ├── policy/                             # RFC 8785 Canonical Policy Engine
│   │   │   ├── canonical_json.go               # RFC 8785 canonical JSON serializer
│   │   │   ├── evaluator.go                    # Safety rule evaluator
│   │   │   ├── safety_rules.go                 # SF-SAFE-001 through SF-SAFE-005
│   │   │   ├── store.go                        # Policy store
│   │   │   └── types.go                        # Policy domain types
│   │   ├── repository/                         # Tenant-scoped database repositories
│   │   ├── review/                             # Review queue & dual-control gating
│   │   ├── schedule/                           # Scheduled trigger timers
│   │   ├── secrets/                            # Secret manager integration
│   │   ├── sftp/                               # SFTP server & client adapters
│   │   ├── telemetry/                          # Prometheus metrics & OpenTelemetry
│   │   ├── toolgateway/                        # Hardened Tool Gateway (P04)
│   │   │   ├── gateway.go                      # 5-Term conjunction execution engine
│   │   │   ├── tools.go                        # Static tool definitions
│   │   │   └── types.go                        # Tool request/response models
│   │   └── verification/                       # Independent Verification Service (P08)
│   │       ├── fuzz_test.go                    # Verification fuzz tests
│   │       ├── hash.go                         # RFC 8785 verification hash derivation
│   │       ├── property_test.go                # Verification property tests
│   │       ├── service.go                      # 12 deterministic check executor
│   │       ├── service_test.go                 # Verification unit tests
│   │       └── types.go                        # Check types & verification outcomes
│   ├── migrations/                             # Database Migrations (001 – 021)
│   │   ├── 018_tool_gateway.sql                # Tool manifests & invocation logs
│   │   ├── 019_agent_workflow_trigger_idempotency.sql # Trigger idempotency
│   │   ├── 020_remediation_candidate_derivations.sql # Candidate derivations
│   │   └── 021_candidate_verifications.sql     # Candidate verifications & checks
│   ├── agent_orchestrator.go                   # Go orchestrator & stage dispatcher
│   ├── agent_orchestrator_gateway_test.go      # Tool Gateway orchestrator tests
│   ├── agent_orchestrator_test.go              # Workflow state machine tests
│   ├── agent_remediation_test.go               # Governed remediation tests
│   ├── agent_verification_test.go              # Independent verification tests
│   ├── agent_workflow_service.go               # Workflow persistence service
│   ├── migrate.go                              # Database migration runner
│   └── worker.go                               # Lease-based worker concurrency pool
└── scripts/                                    # Automation & Build Utilities
    └── generate_docs.py                        # Automated documentation sync checker
```

---

## 7. Verification Proof & Quality Flywheel Results

```
================================================================================
FINAL VERIFICATION SUMMARY (SENTINELFLOW PHASES P00 – P09)
================================================================================

1. Python AI-Tier Unit & Regression Suite:
   $ pytest ai-tier/tests/
   ================= 78 passed, 15 warnings in 74.12s ==================
   Pass Rate: 100.0% (78 / 78 Tests)

2. Unified Fleet Adversarial Security Evaluation Suite:
   $ python ai-tier/evals/runner.py
   Pass Rate: 100.0% (312 / 312 Checks Across 95 Scenarios)
   - Single-Agent Diagnosis (P05): 14/14 Scenarios PASS (70/70 Checks)
   - Multi-Agent Orchestration (P06): 16/16 Scenarios PASS (48/48 Checks)
   - Governed Remediation (P07): 20/20 Scenarios PASS (60/60 Checks)
   - Independent Verification (P08): 20/20 Scenarios PASS (59/59 Checks)
   - Model Armor Boundary (P09): 25/25 Scenarios PASS (75/75 Checks)

3. Go Control Plane & Internal Subsystem Tests:
   $ go test -v ./internal/candidate/... ./internal/verification/... ./internal/toolgateway/...
   - internal/candidate: PASS (Unit, Crash Windows A-F, Vectors, Property, Fuzz)
   - internal/verification: PASS (Unit, 12 Checks, Property, Fuzz)
   - internal/toolgateway: PASS (Conjunction, Manifest, Shadow, DLP, Fuzz)
   - All 18 internal Go packages: PASS

4. Go Control Plane & Verification Orchestrator Tests:
   $ go test -v -run TestAgentOrchestrator_ .
   - TestAgentOrchestrator_ExclusiveToolGateway_Success: PASS
   - TestAgentOrchestrator_ToolGateway_MissingManifest_FailsClosed: PASS
   - TestAgentOrchestrator_ToolGateway_ShadowMode_BlocksCandidateWrite: PASS
   - TestAgentOrchestrator_ToolGateway_PolicyDeny_FailsClosed: PASS
   - TestAgentOrchestrator_IndependentVerification_Success: PASS
   - TestAgentOrchestrator_IndependentVerification_CriticHighRiskConcern_RoutesHumanReview: PASS
   - TestAgentOrchestrator_HumanInvestigation_CannotReachRelease: PASS

5. Documentation Synchronization & Quality Flywheel Check:
   $ python scripts/generate_docs.py --check
   [OK] All documentation matches CAPABILITY_MATRIX.yaml

================================================================================
```
