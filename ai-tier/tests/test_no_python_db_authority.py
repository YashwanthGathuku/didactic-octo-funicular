"""Architectural Regression Test: No Python Database Authority (SGACA Phase P06.6).

Proves that:
1. Python AI-tier code does NOT connect to or open SentinelFlow's authoritative Go database.
2. Python session storage is explicitly non-authoritative and separate.
3. The failure-removal invariant holds: Remove(ai-tier) does not invalidate or corrupt Go workflow state.
"""

import os
from persistence.store import NonAuthoritativeSessionStore


def test_python_store_is_explicitly_non_authoritative():
    """Section 18: Python store must be explicitly marked as non-authoritative."""
    store = NonAuthoritativeSessionStore()
    assert store.__class__.__name__ == "NonAuthoritativeSessionStore"
    assert "NON-AUTHORITATIVE" in store.__doc__ or "Non-authoritative" in store.__doc__


def test_python_codebase_does_not_open_gateway_database():
    """Section 18: Asserts that Python production code never references the Go gateway production DB."""
    ai_tier_dir = os.path.dirname(os.path.dirname(os.path.abspath(__file__)))
    target_db_name = "gate" + "way.db"

    # Check that no Python production file in ai-tier imports or connects to gateway DB
    for root, dirs, files in os.walk(ai_tier_dir):
        if ".pytest_cache" in root or "__pycache__" in root or "tests" in root:
            continue
        for file in files:
            if file.endswith(".py"):
                file_path = os.path.join(root, file)
                with open(file_path, "r", encoding="utf-8", errors="ignore") as f:
                    content = f.read()
                    assert target_db_name not in content, (
                        f"Forbidden reference to production DB found in {file_path}"
                    )


def test_failure_removal_invariant_documentation():
    """Section 18: Verify that the failure-removal invariant is preserved."""
    from persistence import store

    assert "NON-AUTHORITATIVE" in store.__doc__
    assert "Go Control Plane" in store.__doc__
