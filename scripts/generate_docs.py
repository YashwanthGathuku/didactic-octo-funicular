#!/usr/bin/env python3
"""Generate documentation, capability matrices, and Devpost submission from CAPABILITY_MATRIX.yaml.

Usage:
    python scripts/generate_docs.py          # Regenerates all derived docs
    python scripts/generate_docs.py --check  # Verifies docs are up to date (for CI)
"""

from __future__ import annotations

import argparse
import os
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
**SentinelFlow** is a next-generation autonomous AI Agent Control Plane built for **Gemini models** and targeting the **Google Agent Development Kit (ADK)** for high-assurance enterprise financial file reliability, incident triaging, and pre-ledger compliance.

Instead of a generic chatbot, SentinelFlow deploys an **orchestrated fleet of specialist agents** running asynchronously to handle the heavy lifting of batch payments validation, anomaly classification, regulatory compliance auditing, and derived artifact remediation while preserving authoritative deterministic controls.

---

## The Specialist Agent Fleet
1. **SentinelCoordinator (Root Agent)**: Orchestrates the specialist fleet, routes incident findings, and enforces Model Armor guardrails.
2. **TriageAgent**: Classifies incident severity (P1–P4) from deterministic findings and SLA commitments.
3. **ComplianceAgent**: Deep NACHA/ACH regulatory expertise with rule citations.
4. **RemediationAgent**: Proposes non-destructive fixes as **derived artifacts** (preserving immutable originals).
5. **VerifierAgent**: Independent deterministic re-validation of findings before dual-control human release.
6. **MemoryAgent (Memory Bank)**: Persistent cross-session recall of incident patterns, counterparty reliability, and SLA trends.
7. **EscalationAgent**: Proactive SLA breach detection and risk scoring.

---

## Google Cloud & Gemini Technology Stack
- **Gemini 2.5 Flash (`google-genai` SDK)**: Grounded reasoning with calibrated uncertainty.
- **Google Agent Development Kit (ADK)**: Multi-agent hierarchical delegation with least-privilege tool scopes (local fleet implemented, managed ADK runtime PLANNED).
- **Google Cloud Model Armor**: Input/output screening against prompt injection, jailbreaks, and PII leakage (local filter implemented, Cloud API PLANNED).
- **Google Cloud KMS**: Periodic asymmetric signatures on linear hash chain ledger checkpoints (schema implemented, Cloud KMS API PLANNED).
- **Google Cloud SQL (PostgreSQL 16)**: System of record with row-level security and transactional outbox.
- **Google Cloud Run**: Containerized, auto-scaling backend gateway and AI Tier.

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
- **{len(tested)} Tested Capabilities** backed by automated regression tests in CI.
- **{len(implemented)} Implemented Components** with schema/code present.
- **{len(planned)} Planned Google Integrations** scheduled for runtime deployment.
- **100% Deterministic Grounding**: AI operates in a read-only advisory capacity; all releases require verified dual-control human authorization.
"""
    return doc


def main():
    parser = argparse.ArgumentParser(description="Generate docs from capability matrix")
    parser.add_argument("--check", action="store_true", help="Check that generated docs match committed files")
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
        print("[OK] All documentation matches CAPABILITY_MATRIX.yaml")
        return

    devpost_path.write_text(devpost_content, encoding="utf-8")
    print(f"[OK] Generated {devpost_path.relative_to(REPO_ROOT)}")


if __name__ == "__main__":
    main()
