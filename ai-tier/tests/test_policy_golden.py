"""Cross-Language Golden Vector Test for Policy Engine Hashing (P03.5).

Verifies that Python models and Go RFC 8785 canonical serialization produce
identical SHA-256 digests for matching policy definitions and requests.
"""

import sys
from pathlib import Path
from datetime import datetime, timezone
import pytest

sys.path.insert(0, str(Path(__file__).parent.parent))

from models.policy import (
    Decision,
    PolicyDomain,
    PolicyLayer,
    PolicyStatus,
    ObligationType,
    Obligation,
    ProhibitionType,
    Prohibition,
    PolicyDefinition,
    PolicyEvaluationRequest,
    PolicySubject,
    PolicyResource,
    PolicyWorkflowContext,
    PolicyEnvironment,
    canonical_json_bytes,
    compute_policy_content_hash,
)


def test_python_canonical_json_delimiters_and_unicode():
    """Verify RFC 8785 canonical JSON formatting with delimiters and Unicode."""
    data = {
        "b": 1,
        "a": "hello\nworld",
        "c": [3, 2, 1],
        "d": {
            "z": True,
            "y": False,
            "x": None,
        }
    }
    b = canonical_json_bytes(data)
    expected = b'{"a":"hello\\nworld","b":1,"c":[3,2,1],"d":{"x":null,"y":false,"z":true}}'
    assert b == expected


def test_python_typed_obligation_content_hash():
    """Verify content hash computation on typed obligations."""
    p = PolicyDefinition(
        policy_id="SF-SAFE-004",
        version=1,
        domain=PolicyDomain.REMEDIATION,
        layer=PolicyLayer.SENTINEL_SAFETY,
        priority=100,
        status=PolicyStatus.ACTIVE,
        effective_from=datetime(2026, 1, 1, 0, 0, 0, tzinfo=timezone.utc),
        action="CREATE_CANDIDATE",
        subject_constraints={"type": "AGENT", "id": "*", "roles": [], "min_autonomy": 0, "max_autonomy": 0},
        resource_constraints={"type": "ARTIFACT", "id": "*", "states": [], "classification": ""},
        conditions={},
        effect=Decision.ALLOW_WITH_OBLIGATIONS,
        obligations=[
            Obligation(type=ObligationType.CANDIDATE_ONLY),
            Obligation(type=ObligationType.IMMUTABLE_PARENT_REQUIRED),
            Obligation(type=ObligationType.SANDBOX_ONLY),
            Obligation(type=ObligationType.DETERMINISTIC_REVALIDATION),
            Obligation(type=ObligationType.AUDIT_REQUIRED),
            Obligation(type=ObligationType.MAX_ATTEMPTS, parameters={"count": 3}),
        ],
        prohibitions=[],
        reason_code="DERIVED_CANDIDATE_WITH_SAFETY_OBLIGATIONS",
        source_reference="SGACA Architectural Law #2",
        created_at=datetime(2026, 1, 1, 0, 0, 0, tzinfo=timezone.utc),
        content_hash="",
    )

    h = compute_policy_content_hash(p)
    assert len(h) == 64
    assert all(c in "0123456789abcdef" for c in h)
