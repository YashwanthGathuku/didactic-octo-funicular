"""Unit and conformance tests for ReturnRiskAgent (SGACA P12.5)."""

from __future__ import annotations

import json
import os
from pathlib import Path
from unittest.mock import patch

import pytest

from agents.return_risk import ReturnRiskAgent, ReturnRiskExecutionError
from armor.client import MockModelArmorProvider
from armor.config import GuardrailMode, ModelArmorConfig
from contracts.manifests import validate_agent_roster_membership
from contracts.return_risk import (
    RETURN_RISK_NON_AUTHORITY_STATEMENT,
    FeatureContributionItem,
    ReturnRiskAssessment,
    ReturnRiskContextEnvelope,
    ReturnRiskTier,
)
from guardrails.boundary import GuardedModelBoundary


REPO_ROOT = Path(__file__).resolve().parents[2]
SEMANTICS_FIXTURE = REPO_ROOT / "docs" / "fixtures" / "return_risk_semantics.json"
CAPABILITY_MATRIX = REPO_ROOT / "docs" / "CAPABILITY_MATRIX.yaml"


def make_envelope(return_code: str = "R11") -> ReturnRiskContextEnvelope:
    return ReturnRiskContextEnvelope(
        tenant_scope="t-corp-99",
        return_event_ref="RET-EVT-101",
        return_code=return_code,
        return_code_label="ENTRY_NOT_IN_ACCORDANCE_WITH_AUTHORIZATION",
        risk_score=72.5,
        risk_tier=ReturnRiskTier.HIGH,
        contributions=[
            FeatureContributionItem(
                feature_name="ReturnFrequency7d",
                raw_value=4,
                contribution_score=15.0,
                description="4 returns observed over past 7 days.",
            )
        ],
        authorized_evidence_refs=["RET-EVT-101", "INCIDENT-500"],
        authorized_memory_refs=["MEM-HIT-01"],
        partner_ref="PARTNER-ALPHA",
        historical_summary="Historical return pattern is advisory only.",
        workflow_id="wf-return-101",
        incident_id=501,
        correlation_id="corr-return-101",
    )


def test_return_risk_manifest_conformance():
    manifest = validate_agent_roster_membership("ReturnRiskAgent")
    assert manifest.name == "ReturnRiskAgent"
    assert manifest.autonomy_level == "A1"
    assert manifest.model == "gemini-3.5-flash"
    assert "RETURN_EVENT_OBSERVED" in manifest.triggers
    assert "RETURN_RISK_ANALYSIS" in manifest.triggers
    assert "returnrisk.result.get" in manifest.allowed_tools
    assert "artifact.release" in manifest.denied_capabilities
    assert "incident.approve" in manifest.denied_capabilities
    assert "remediation.candidate.create" in manifest.denied_capabilities
    assert "database.raw_sql" in manifest.denied_capabilities
    assert manifest.output_schema_name == "ReturnRiskAssessment"


def test_local_mode_allows_truthful_deterministic_fallback():
    agent = ReturnRiskAgent()
    with patch.dict(os.environ, {"SENTINEL_AI_MODE": "local"}, clear=False):
        res = agent.run(make_envelope())

    assert isinstance(res, ReturnRiskAssessment)
    assert res.return_code == "R11"
    assert res.risk_score == 72.5
    assert res.risk_tier == ReturnRiskTier.HIGH
    assert res.non_authority_statement == RETURN_RISK_NON_AUTHORITY_STATEMENT
    assert res.execution_source == "LOCAL_ADK_DETERMINISTIC"
    assert "MEM-HIT-01" in res.memory_refs
    assert all("1.5%" not in text for text in res.operational_recommendations)


def test_return_risk_input_minimization_helper():
    agent = ReturnRiskAgent()
    raw_text = "Account 123456789012 at Routing 021000021 had an R01 return."
    sanitized = agent._mask_sensitive_data(raw_text)
    assert "123456789012" not in sanitized
    assert "021000021" not in sanitized
    assert "[ACCOUNT_REDACTED]" in sanitized
    assert "[ROUTING_REDACTED]" in sanitized


def test_model_armor_required_block_means_zero_gemini_calls():
    provider = MockModelArmorProvider(mode=GuardrailMode.REQUIRED)
    provider.inject_fault("EXPLICIT_BLOCK")
    boundary = GuardedModelBoundary(
        provider=provider,
        config=ModelArmorConfig(mode=GuardrailMode.REQUIRED),
        default_model="gemini-3.5-flash",
    )
    agent = ReturnRiskAgent(boundary=boundary)

    with (
        patch.dict(
            os.environ,
            {"SENTINEL_AI_MODE": "live", "GOOGLE_API_KEY": "test-key-not-a-secret"},
            clear=False,
        ),
        patch("google.genai.Client") as gemini_client,
    ):
        with pytest.raises(ReturnRiskExecutionError) as exc:
            agent.run(make_envelope())

    assert exc.value.execution_source == "GUARDRAIL_BLOCKED"
    assert exc.value.error_code == "PROMPT_SECURITY_BLOCKED"
    assert gemini_client.call_count == 0


def test_live_provider_failure_does_not_silently_fallback():
    provider = MockModelArmorProvider(mode=GuardrailMode.REQUIRED)
    boundary = GuardedModelBoundary(
        provider=provider,
        config=ModelArmorConfig(mode=GuardrailMode.REQUIRED),
        default_model="gemini-3.5-flash",
    )
    agent = ReturnRiskAgent(boundary=boundary)

    with (
        patch.dict(
            os.environ,
            {"SENTINEL_AI_MODE": "live", "GOOGLE_API_KEY": "test-key-not-a-secret"},
            clear=False,
        ),
        patch("google.genai.Client", side_effect=RuntimeError("simulated provider outage")),
    ):
        with pytest.raises(ReturnRiskExecutionError) as exc:
            agent.run(make_envelope())

    assert exc.value.execution_source == "PROVIDER_UNAVAILABLE"
    assert exc.value.error_code == "LIVE_EXECUTION_FAILED"


def test_return_risk_agent_uses_guarded_boundary_in_auto_mode():
    provider = MockModelArmorProvider(mode=GuardrailMode.REQUIRED)
    boundary = GuardedModelBoundary(
        provider=provider,
        config=ModelArmorConfig(mode=GuardrailMode.REQUIRED),
        default_model="gemini-3.5-flash",
    )
    agent = ReturnRiskAgent(boundary=boundary)

    with patch.dict(os.environ, {"SENTINEL_AI_MODE": "auto", "GOOGLE_API_KEY": ""}, clear=False):
        with patch.object(boundary, "invoke", wraps=boundary.invoke) as invoke:
            res = agent.run(make_envelope())

    assert invoke.call_count == 1
    assert res.execution_source == "LOCAL_ADK_DETERMINISTIC"


def test_return_risk_disjoint_grounding_local():
    agent = ReturnRiskAgent()
    env = make_envelope("R10")
    with patch.dict(os.environ, {"SENTINEL_AI_MODE": "local"}, clear=False):
        res = agent.run(env)

    ev_set = set(res.evidence_refs)
    mem_set = set(res.memory_refs)
    assert ev_set.isdisjoint(mem_set)
    assert "MEM-HIT-01" not in ev_set
    assert "RET-EVT-101" not in mem_set


def test_shared_threshold_and_r10_r11_semantics_fixture():
    fixture = json.loads(SEMANTICS_FIXTURE.read_text(encoding="utf-8"))
    assert fixture["thresholds"] == {
        "unauthorized": 0.005,
        "administrative": 0.03,
        "overall": 0.15,
    }
    assert fixture["return_codes"]["R10"]["threshold_category"] == "UNAUTHORIZED_0_5_PERCENT"
    assert fixture["return_codes"]["R11"]["threshold_category"] == "UNAUTHORIZED_0_5_PERCENT"
    assert fixture["return_codes"]["R11"]["return_window"] == "EXTENDED_60_CALENDAR_DAYS"
    assert "R11" in fixture["unauthorized_return_rate_codes"]
    assert fixture["return_codes"]["R16"]["threshold_applicable"] is False


def test_gemini_35_capability_matrix_truth():
    matrix = CAPABILITY_MATRIX.read_text(encoding="utf-8")
    assert "gemini_3_5_provider_path:" in matrix
    assert "live_gemini_3_5:" in matrix
    assert "Google Gemini 2.5 Flash grounded incident hypothesis generation" not in matrix
