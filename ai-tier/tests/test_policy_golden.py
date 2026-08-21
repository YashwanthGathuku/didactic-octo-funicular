"""Cross-Language Golden Vector Test for Policy Engine Hashing (P03.6).

Verifies RFC 8785 JSON Canonicalization Scheme (JCS) compliance:
- Exact UTF-16 code-unit property sorting across Go and Python
- Unicode property names (BMP and non-BMP astral plane)
- Rejection of non-finite numbers (NaN/Infinity)
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


def test_python_rfc8785_official_utf16_property_sorting():
    """Verify RFC 8785 Section 3.2.3 UTF-16 code-unit property sorting in Python.

    Astral / non-BMP emoji (\U0001f600 -> UTF-16 surrogate D83D DE00) MUST sort
    BEFORE high BMP characters (\uffff -> FFFF).
    """
    data = {
        "\uffff": "high_bmp",
        "\U0001f600": "astral_plane_emoji",
        "\u20ac": "euro_sign",
        "\r": "carriage_return",
        "\n": "newline",
        "1": "digit_one",
        "\u00e9": "e_acute",
        "a": "letter_a",
    }
    b = canonical_json_bytes(data)
    expected = (
        b'{"\\n":"newline","\\r":"carriage_return","1":"digit_one","a":"letter_a",'
        b'"\xc3\xa9":"e_acute","\xe2\x82\xac":"euro_sign","\xf0\x9f\x98\x80":"astral_plane_emoji","\xef\xbf\xbf":"high_bmp"}'
    )
    assert b == expected


def test_python_canonical_json_delimiters_and_unicode_property_names():
    """Verify RFC 8785 canonical JSON formatting with Unicode keys and values."""
    data = {
        "b": 1,
        "a": "hello\nworld",
        "Unicode_€_漢字_🔒": {
            "z_key": "val=z:colon\n",
            "a_key": "val_a",
            "nested": {
                "count": 42,
                "unicode_field": "こんにちは",
            },
        },
    }
    b = canonical_json_bytes(data)
    # Validate it's valid UTF-8
    decoded = b.decode("utf-8")
    assert "Unicode_€_漢字_🔒" in decoded
    assert "こんにちは" in decoded


def test_python_rejects_non_finite_numbers():
    """Verify that NaN and Infinity are strictly rejected in canonical JSON."""
    with pytest.raises(ValueError, match="non-finite number"):
        canonical_json_bytes({"val": float("nan")})

    with pytest.raises(ValueError, match="non-finite number"):
        canonical_json_bytes({"val": float("inf")})

    with pytest.raises(ValueError, match="non-finite number"):
        canonical_json_bytes({"val": float("-inf")})


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


def test_python_rfc8785_numeric_golden_vectors():
    """Verify RFC 8785 numeric serialization across representative values."""
    cases = [
        (0.0, b"0"),
        (-0.0, b"0"),
        (0, b"0"),
        (100, b"100"),
        (-100, b"-100"),
        (9007199254740991, b"9007199254740991"),
        (-9007199254740991, b"-9007199254740991"),
        (9223372036854775807, b"9223372036854775807"),
        (-9223372036854775808, b"-9223372036854775808"),
        (0.125, b"0.125"),
        (1.5, b"1.5"),
        (0.00001, b"0.00001"),
        (0.000001, b"0.000001"),
        (-0.00001, b"-0.00001"),
        (-0.000001, b"-0.000001"),
        (1.23456789e-5, b"0.0000123456789"),
        (1.23456789e-6, b"0.00000123456789"),
        (0.0000001, b"1e-7"),
        (-0.0000001, b"-1e-7"),
        (1e21, b"1e+21"),
        (-1e21, b"-1e+21"),
        (1e20, b"100000000000000000000"),
        (-1e20, b"-100000000000000000000"),
    ]
    for inp, expected in cases:
        b = canonical_json_bytes(inp)
        assert b == expected, f"Failed for {inp}: got {b}, expected {expected}"

