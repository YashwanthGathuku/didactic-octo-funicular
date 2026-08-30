# SentinelFlow Demo Day & GCP Deployment Playbook

## 1. Executive Pre-Flight Status

All test suites, container builds, and security invariant gates are **100% green**:
- **Gateway Control Plane**: \go test ./...\ -> 100% PASS (24 migrations, zero TOCTOU, immutable ledgers).
- **AI Tier Specialist Fleet**: \pytest ai-tier/tests/ -v\ -> 142/142 tests PASS.
- **Adversarial Evaluation Matrix**: \python ai-tier/evals/runner.py\ -> 169 scenarios, 405/405 checks PASS (100% containment).
- **Frontend Cockpit**: pm test -- --run\ & pm run build\ -> 14/14 tests PASS, Vite production build successful.
- **Submission Hardening Gate**: \scripts/verify_submission_freeze.sh\ -> 12/12 stages PASS.

---

## 2. Google Cloud Deployment Guide

### Option A: Automated One-Command Cloud Run Deployment
Run the automated deployment script:
\\ash
# 1. Provision foundational GCP infrastructure (Artifact Registry, KMS, Cloud SQL, Storage)
./deploy/setup-gcp.sh <YOUR_PROJECT_ID> us-central1

# 2. Build and deploy all 3 tiers (AI Tier, Gateway, UI) to Cloud Run
./deploy/deploy-cloudrun.sh <YOUR_PROJECT_ID> us-central1
\
### Option B: Local Demo Mode (Zero-Cloud Fallback)
If recording locally before pushing to Cloud Run:
\\ash
# Start Docker / Podman stack
docker compose up --build

# Open Web Cockpit at http://localhost:3000
# Gateway API at http://localhost:8080
# AI Tier at http://localhost:8000
\
---

## 3. Demo Datasets Prepared & Ready

All demo fixtures are generated in \demo/\:

| Dataset / File | Source / Type | Purpose in Demo |
| :--- | :--- | :--- |
| \demo/demo_clean_payroll.ach\ | NACHA 94-char PPD batch | Shows automated clean ingestion & immediate green settlement |
| \demo/demo_corrupted_hash.ach\ | NACHA batch (Rule 0802 hash mismatch) | Triggers automated quarantine, multi-agent remediation, and dual-control release |
| \demo/demo_invalid_routing.ach\ | NACHA batch (mod-10 routing failure) | Demonstrates hard validation gate & fail-closed security |
| \demo/lens_return_events.csv\ | Synthetic ACH return history (74 events) | Demonstrates Lens Lite return-risk intelligence & R11 payroll anomaly detection |
| \demo/public/ibm-aml/\ | Kaggle / IBM AML transaction dataset | Demonstrates real privacy-safe external dataset analytics |
| \demo/public/moov-ach/\ | Moov-io genuine NACHA fixtures | Demonstrates open-source real ACH format conformance |

### Using Your Own HuggingFace / Kaggle Dataset:
If you download a larger IBM AML transactions CSV (\HI-Small_Trans.csv\):
\\ash
python scripts/prepare_ibm_aml_demo.py /path/to/HI-Small_Trans.csv --output-dir demo/public/ibm-aml --max-rows 100000
python scripts/verify_public_demo_data.py demo/public/ibm-aml
\
---

## 4. Recommended Demo Video Recording Script (3–5 Minutes)

### Act 1: The Problem & The Architecture (0:00 – 1:00)
- **Narrative**: High-throughput financial ACH clearing (\ annually) faces catastrophic risk if ungrounded AI is given unconstrained authority. SentinelFlow enforces the **Principle of Non-Authoritative AI**: deterministic Go control plane holds financial authority, while a Google ADK specialist fleet provides operational intelligence.
- **Visual**: Show the Architecture diagram & the Operations Cockpit (\http://localhost:3000\ or Cloud Run URL).

### Act 2: Clean Processing & Feed Monitoring (1:00 – 1:45)
- **Action**: Upload \demo/demo_clean_payroll.ach\.
- **Observation**: File validates in <5ms. SLA status is green. Immutably recorded to the append-only SHA-256 audit ledger.

### Act 3: Incident Quarantine & Multi-Agent Fleet Triage (1:45 – 3:15)
- **Action**: Upload \demo/demo_corrupted_hash.ach\ (Rule 0802 entry hash mismatch).
- **Observation**:
  1. **Deterministic Gate**: File is immediately quarantined with \BLOCKING\ severity.
  2. **Multi-Agent Specialist Fleet**:
     - \IncidentCommanderAgent\: Orchestrates diagnostic dispatch and synthesizes evidence.
     - \DiagnosisAgent\: Isolates the corrupted hash without seeing raw unredacted PII (Model Armor screening).
     - \RemediationAgent\: Proposes a candidate binary patch.
     - \VerifierAgent\ (Critic): Independently validates the candidate patch against ground truth and invariants.
     - \PolicySLAAgent\: Explains regulatory cutoff requirements and cutoff risk.
     - \ReturnRiskAgent\: Evaluates partner return history risk (R10/R11).
  3. **Dual-Control Supervisor Release**: Human supervisor reviews the verifier proof and authorizes candidate release.

### Act 4: Lens Lite & Return Risk Intelligence (3:15 – 4:15)
- **Action**: Switch to the **Lens Workspace** tab.
- **Observation**: Query return event patterns. Show the R11 concentration detection on partner \TEST-PAYROLL-17\, grounded evidence graph, and zero raw SQL exposure.

### Act 5: Data Sovereignty & Model Armor Invariants (4:15 – 5:00)
- **Narrative**: Highlight \SF-SAFE-007\ (Layer 20 boot invariant). If an EU tenant's data is routed to a non-EU model or memory bank, SentinelFlow fails closed with a typed failure before a single byte leaves the region.
- **Conclusion**: Explain that SentinelFlow bridges Gemini Enterprise with mission-critical banking reliability.
