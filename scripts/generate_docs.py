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

    return matrix


def generate_devpost_submission(matrix: dict) -> str:
    capabilities = matrix.get("capabilities", {})
    tested = [k for k, v in capabilities.items() if v.get("status") == "TESTED"]
    implemented = [k for k, v in capabilities.items() if v.get("status") == "IMPLEMENTED"]
    planned = [k for k, v in capabilities.items() if v.get("status") == "PLANNED"]

    doc = f"""# SentinelFlow — All Things Agentic Hackathon Submission

## Project Overview
**SentinelFlow** is a governed AI Agent Control Plane built for **Gemini 3.5** and the **Google Agent Development Kit (ADK)** for high-assurance enterprise financial file reliability, incident triage, and pre-ledger operational intelligence.

Instead of a generic chatbot, SentinelFlow deploys an orchestrated fleet of specialist agents to perform bounded analysis and remediation planning while deterministic Go controls retain authority over evidence, policy, state transitions, verification, and financial release.

---

## Governed Specialist Agent Fleet
1. **IncidentCommanderAgent**: Plans and synthesizes bounded incident investigations.
2. **DiagnosisAgent**: Produces evidence-grounded diagnostic hypotheses.
3. **PolicySLAAgent**: Explains deterministic policy and SLA context without overriding it.
4. **MemoryAgent**: Retrieves advisory historical context; memory is never promoted to evidence by the model.
5. **RemediationAgent**: Proposes non-destructive derived-artifact repairs only.
6. **VerifierAgent**: Critiques verification evidence while deterministic Go verification remains authoritative.
7. **ReturnRiskAgent**: Explains deterministic ACH return-risk results in an A1 read-only role.

---

## Google Cloud & Gemini Technology Stack
- **Gemini 3.5 Flash (`gemini-3.5-flash`, `google-genai` SDK)**: Governed provider path for structured reasoning. A live external invocation is claimed only when separately evidenced in the capability matrix.
- **Google Agent Development Kit (ADK)**: Fixed specialist-agent runtime objects and bounded orchestration.
- **Google Cloud Model Armor**: Shared GuardedModelBoundary performs configured pre/post screening before/after live model invocation.
- **Google Cloud KMS**: Ledger checkpoint signing integration remains separately statused in the capability matrix.
- **PostgreSQL / SQLite**: Deterministic system-of-record and test/storage paths according to capability status.

---

## Verified Capabilities Matrix (Single Source of Truth)

| Capability | Status | Evidence & Test Suite | Description |
|---|---|---|---|
"""
    for name, cap in capabilities.items():
        status = cap.get("status", "UNKNOWN")
        evidence = cap.get("evidence", "—")
        desc = cap.get("description", "")
        doc += f"| `{name}` | **{status}** | `{evidence}` | {desc} |\n"

    doc += f"""
---

## Summary of Capabilities
- **{len(tested)} Tested Capabilities** backed by automated regression evidence.
- **{len(implemented)} Implemented Components** with code/schema present but not necessarily live-provider verified.
- **{len(planned)} Planned Integrations** that are not represented as implemented runtime behavior.
- **Authority invariant**: AI output is advisory; financial release remains behind deterministic controls and human authorization.
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
