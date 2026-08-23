"""Submission-eligibility truth checks for executable Gemini model references."""

from __future__ import annotations

import re
from pathlib import Path


REPO_ROOT = Path(__file__).resolve().parents[2]
LEGACY_MODEL = re.compile(r"gemini-(?:1\.5|2\.0|2\.5)", re.IGNORECASE)


def _audited_paths() -> list[Path]:
    paths: list[Path] = []
    for root in (
        REPO_ROOT / "ai-tier" / "agents",
        REPO_ROOT / "ai-tier" / "runtime",
    ):
        paths.extend(sorted(root.rglob("*.py")))
    paths.extend(
        [
            REPO_ROOT / "ai-tier" / "contracts" / "manifests.py",
            REPO_ROOT / "ai-tier" / "llm_client.py",
            REPO_ROOT / "scripts" / "generate_docs.py",
            REPO_ROOT / "docs" / "CAPABILITY_MATRIX.yaml",
            REPO_ROOT / "docs" / "DEVPOST_SUBMISSION.md",
        ]
    )
    return paths


def test_executable_and_submission_sources_have_no_legacy_gemini_models():
    violations: list[str] = []
    combined = ""
    for path in _audited_paths():
        text = path.read_text(encoding="utf-8")
        combined += text
        for match in LEGACY_MODEL.finditer(text):
            violations.append(f"{path.relative_to(REPO_ROOT)}:{match.group(0)}")

    assert not violations, f"legacy executable/submission Gemini references: {violations}"
    assert "gemini-3.5-flash" in combined


def test_live_gemini_capability_is_not_overclaimed():
    matrix = (REPO_ROOT / "docs" / "CAPABILITY_MATRIX.yaml").read_text(encoding="utf-8")
    assert "gemini_3_5_provider_path:\n    status: TESTED" in matrix
    assert "live_gemini_3_5:\n    status: IMPLEMENTED" in matrix
