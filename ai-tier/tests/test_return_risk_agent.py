"""Unit & Conformance Tests for ReturnRiskAgent (SGACA Phase P12)."""

import pytest
from contracts.manifests import FIXED_AGENT_ROSTER, validate_agent_roster_membership
from contracts.return_risk import (
    RETURN_RISK_NON_AUTHORITY_STATEMENT,
    FeatureContributionItem,
    ReturnRiskAssessment,
    ReturnRiskContextEnvelope,
    ReturnRiskTier,
)
from agents.return_risk import ReturnRiskAgent


def test_return_risk_manifest_conformance():
    """Verify ReturnRiskAgent strictly adheres to SGACA Manifest requirements."""
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


def test_return_risk_agent_deterministic_fallback():
    """Verify deterministic fallback engine produces grounded output with non-authority statement."""
    agent = ReturnRiskAgent()
    envelope = ReturnRiskContextEnvelope(
        tenant_scope="t-corp-99",
        return_event_ref="RET-EVT-101",
        return_code="R01",
        return_code_label="INSUFFICIENT_FUNDS",
        risk_score=45.5,
        risk_tier=ReturnRiskTier.MEDIUM,
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
        historical_summary="Historical return rate is within 0.1% baseline.",
    )

    res = agent.run(envelope)
    assert isinstance(res, ReturnRiskAssessment)
    assert res.return_event_ref == "RET-EVT-101"
    assert res.return_code == "R01"
    assert res.risk_score == 45.5
    assert res.risk_tier == ReturnRiskTier.MEDIUM
    assert res.non_authority_statement == RETURN_RISK_NON_AUTHORITY_STATEMENT
    assert "RET-EVT-101" in res.evidence_refs
    assert "MEM-HIT-01" in res.memory_refs
    assert res.execution_source in ("LOCAL_ADK_DETERMINISTIC", "LIVE_GEMINI")


def test_return_risk_input_minimization():
    """Verify account and routing numbers are stripped before prompt formulation."""
    agent = ReturnRiskAgent()
    raw_text = "Account 123456789012 at Routing 021000021 had an R01 return."
    sanitized = agent._mask_sensitive_data(raw_text)
    assert "123456789012" not in sanitized
    assert "021000021" not in sanitized
    assert "[ACCOUNT_REDACTED]" in sanitized
    assert "[ROUTING_REDACTED]" in sanitized


def test_return_risk_disjoint_grounding():
    """Verify evidence_refs and memory_refs remain strictly disjoint."""
    agent = ReturnRiskAgent()
    envelope = ReturnRiskContextEnvelope(
        tenant_scope="t-corp-99",
        return_event_ref="RET-EVT-102",
        return_code="R10",
        return_code_label="CUSTOMER_ADVISES_UNAUTHORIZED",
        risk_score=92.0,
        risk_tier=ReturnRiskTier.SEVERE,
        authorized_evidence_refs=["RET-EVT-102"],
        authorized_memory_refs=["MEM-HIT-02"],
    )

    res = agent.run(envelope)
    ev_set = set(res.evidence_refs)
    mem_set = set(res.memory_refs)
    assert ev_set.isdisjoint(mem_set)
    assert "MEM-HIT-02" not in ev_set
    assert "RET-EVT-102" not in mem_set
