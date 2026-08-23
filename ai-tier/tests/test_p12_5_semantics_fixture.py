"""Cross-language semantic pins for the P12.5 return-risk fixture."""

import json
from pathlib import Path


REPO_ROOT = Path(__file__).resolve().parents[2]
FIXTURE = REPO_ROOT / "docs" / "fixtures" / "return_risk_semantics.json"


def test_r11_category_and_unauthorized_rate_family_are_distinct_concepts():
    fixture = json.loads(FIXTURE.read_text(encoding="utf-8"))
    r11 = fixture["return_codes"]["R11"]

    # R11 represents an authorization-terms defect operationally, while public
    # Nacha materials include R11 in Unauthorized Entry Return Rate handling.
    assert r11["normalized_category"] == "AUTHORIZATION_TERMS"
    assert r11["threshold_category"] == "UNAUTHORIZED_0_5_PERCENT"
    assert "R11" in fixture["unauthorized_return_rate_codes"]
