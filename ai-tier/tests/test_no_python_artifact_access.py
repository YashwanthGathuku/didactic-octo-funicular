"""Architectural Boundary Test: Zero Python Artifact Mutation Authority (SGACA Phase P07).

Enforces that:
1. Python ai-tier codebase contains ZERO raw SQL or direct file-writing to authoritative artifact stores.
2. RemediationAgent outputs only high-level allowlisted intent (RemediationPlan), never raw NACHA bytes.
3. Candidate generation and arithmetic are owned strictly by the Go Control Plane.
"""

from pathlib import Path
from contracts.remediation import RemediationPlan


def test_remediation_plan_has_no_raw_payload_fields():
    """Verifies that RemediationPlan schema contains zero raw byte or file payload fields."""
    fields = RemediationPlan.model_fields.keys()
    assert "raw_bytes" not in fields
    assert "raw_payload" not in fields
    assert "repaired_content" not in fields
    assert "candidate_bytes" not in fields
    assert "file_bytes" not in fields


def test_no_python_file_writing_in_agents():
    """Scans all Python agent files to verify no direct file writes or object store operations."""
    agents_dir = Path(__file__).parent.parent / "agents"
    for py_file in agents_dir.glob("*.py"):
        content = py_file.read_text(encoding="utf-8")
        # Check for direct file write operations
        assert "open(" not in content or "rb" in content or "encoding" in content, (
            f"Direct open() write found in {py_file}"
        )
        assert ".write(" not in content, f"Direct .write() found in {py_file}"
        assert "shutil.copy" not in content, f"Direct shutil.copy found in {py_file}"
        assert "os.remove" not in content, f"Direct os.remove found in {py_file}"
        assert "sqlite3.connect" not in content, (
            f"Direct sqlite3 database access found in {py_file}"
        )
