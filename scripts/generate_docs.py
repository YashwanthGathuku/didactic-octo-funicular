#!/usr/bin/env python3
"""Generate and verify the Devpost submission from CAPABILITY_MATRIX.yaml.

Usage:
    python scripts/generate_docs.py          # Regenerates docs/DEVPOST_SUBMISSION.md
    python scripts/generate_docs.py --check  # Verifies that generated submission is current
"""

from __future__ import annotations

import argparse
import sys
from pathlib import Path

import yaml

REPO_ROOT = Path(__file__).resolve().parent.parent
MATRIX_FILE = REPO_ROOT / "docs" / "CAPABILITY_MATRIX.yaml"
LEGAL_STATUSES = {"IMPLEMENTED", "TESTED", "DEMO_ONLY", "EXPERIMENTAL", "PLANNED"}
SUBMISSION_CAPABILITIES = [
    "gemini_3_5_provider_path",
    "live_gemini_3_5",
    "guarded_model_boundary",
    "deterministic_ach_return_intelligence",
    "return_risk_agent",
    "return_risk_public_semantics_fixture",
    "dual_control_release",
    "independent_verification",
]


def load_and_validate_matrix() -> dict:
    if not MATRIX_FILE.exists():
        print(f"Error: {MATRIX_FILE} does not exist.", file=sys.stderr)
        sys.exit(1)
    with open(MATRIX_FILE, "r", encoding="utf-8") as f:
        matrix = yaml.safe_load(f)

    capabilities = matrix.get("capabilities", {})
    invalid_statuses = []
    for name, cap in capabilities.items():
        status = cap.get("status")
        if status not in LEGAL_STATUSES:
            invalid_statuses.append((name, status))
    if invalid_statuses:
        print(f"Error: Invalid capability status vocabulary detected in {MATRIX_FILE}:", file=sys.stderr)
        for name, status in invalid_statuses:
            print(f"  - {name}: '{status}' (legal statuses: {', '.join(sorted(LEGAL_STATUSES))})", file=sys.stderr)
        sys.exit(1)

    missing = [name for name in SUBMISSION_CAPABILITIES if name not in capabilities]
    if missing:
        print(f"Error: submission capabilities missing from matrix: {missing}", file=sys.stderr)
        sys.exit(1)
    return matrix


def generate_devpost_submission(matrix: dict) -> str:
    capabilities = matrix["capabilities"]
    doc = """# SentinelFlow — All Things Agentic Hackathon Submission

## Project Overview
**SentinelFlow** is a governed AI Agent Control Plane built for **Gemini 3.5** and the **Google Agent Development Kit (ADK)** for high-assurance financial-file reliability, incident triage, and pre-ledger operational intelligence.

AI is intentionally separated from financial authority: deterministic Go services own validation, evidence, policy, risk scoring, verification, and state transitions; human dual control owns release decisions.

## Governed Specialist Fleet
1. **IncidentCommanderAgent** — bounded planning and synthesis.
2. **DiagnosisAgent** — evidence-grounded diagnostic hypotheses.
3. **PolicySLAAgent** — advisory explanation of deterministic policy/SLA context.
4. **MemoryAgent** — advisory historical retrieval; memory is not evidence.
5. **RemediationAgent** — proposal-only derived-artifact remediation.
6. **VerifierAgent** — verification critic; deterministic Go verification remains authoritative.
7. **ReturnRiskAgent** — A1 ACH return-intelligence specialist.

## Gemini & Guardrail Path
- **Gemini 3.5 Flash (`gemini-3.5-flash`)** is the executable provider target for the governed path.
- **GuardedModelBoundary** performs data minimization, trust partitioning, configured Model Armor pre/post screening, Pydantic validation, and evidence grounding around live inference.
- **LIVE** provider failure is surfaced as failure/unavailable; it is not silently relabeled as successful deterministic AI.
- **LOCAL/DETERMINISTIC** mode permits clearly labeled rule-grounded fallback.
- **AUTO** follows the common SentinelFlow boundary semantics.

## P12.5 Return Intelligence Truth Gate
- Public return-rate monitoring values represented by SentinelFlow: **0.5% unauthorized, 3.0% administrative, 15.0% overall**.
- R10 and R11 semantics are pinned by a shared public-semantics fixture; R11 participates in unauthorized-return-rate handling.
- R16 has **no invented percentage threshold**; threshold applicability is explicit and false for the regulatory-restricted category.
- The representative MVP taxonomy is **not a complete ACH return-code catalog**.
- Operational taxonomy guidance is not a legal decision, and a return-risk score is not a compliance decision.
- Assessment hashes reuse SentinelFlow's RFC 8785 canonical JSON implementation over deterministic protected fields; volatile record identity/time are excluded.

## Submission Capability Truth

| Capability | Status | Evidence | Description |
|---|---|---|---|
"""
    for name in SUBMISSION_CAPABILITIES:
        cap = capabilities[name]
        doc += (
            f"| `{name}` | **{cap.get('status', 'UNKNOWN')}** | "
            f"`{cap.get('evidence', '—')}` | {cap.get('description', '')} |\n"
        )

    doc += """

## Authority Invariants

`ReturnRiskAssessment != FinancialDecision`

`MemoryRecall != Evidence`

`ReturnRiskScore != ComplianceDecision`

`RiskHigh != AutoRejectFinancialFile` and `RiskLow != AutoReleaseFinancialFile`
"""
    return doc


def main():
    parser = argparse.ArgumentParser(description="Generate Devpost submission from capability matrix")
    parser.add_argument("--check", action="store_true", help="Check that generated Devpost submission is current")
    args = parser.parse_args()

    matrix = load_and_validate_matrix()
    devpost_content = generate_devpost_submission(matrix)
    devpost_path = REPO_ROOT / "docs" / "DEVPOST_SUBMISSION.md"

    if args.check:
        if not devpost_path.exists():
            print(f"Error: {devpost_path} missing. Run python scripts/generate_docs.py", file=sys.stderr)
            sys.exit(1)
        existing = devpost_path.read_text(encoding="utf-8")
        if existing != devpost_content:
            print("Error: docs/DEVPOST_SUBMISSION.md is out of date with CAPABILITY_MATRIX.yaml!", file=sys.stderr)
            sys.exit(1)
        print("[OK] Generated Devpost submission matches CAPABILITY_MATRIX.yaml")
        return

    devpost_path.write_text(devpost_content, encoding="utf-8")
    print(f"[OK] Generated {devpost_path.relative_to(REPO_ROOT)}")


if __name__ == "__main__":
    main()
