"""Unit tests for PolicySLAAgent (SGACA Phase P06)."""

import pytest
from agents.policy_sla import PolicySLAAgent
from contracts.policy_sla import PolicySLAOutput
from models.envelope import AgentContextEnvelope, RedactedFindingItem


def test_policy_sla_agent_deterministic_baseline():
    """Verifies PolicySLAAgent deterministic interpretation of policy and SLA context."""
    envelope = AgentContextEnvelope(
        tenant_id="TENANT-BANK-01",
        incident_id=301,
        artifact_id=601,
        correlation_id="corr-pol-301",
        findings=[
            RedactedFindingItem(
                id="FINDING-001",
                code="0802",
                severity="BLOCKING",
                description="Batch entry hash mismatch in batch 001",
                line_number=14,
            )
        ],
        available_runbooks=["RB-01", "RB-05"],
    )

    auth_decision = {
        "decision_id": "POL-DEC-301",
        "decision": "REQUIRE_HUMAN",
        "obligations": ["CANDIDATE_ONLY_REMEDIATION", "DUAL_CONTROL_APPROVAL_REQUIRED"],
        "prohibitions": ["PROHIBIT_DIRECT_ORIGINAL_MUTATION", "PROHIBIT_AUTONOMOUS_RELEASE"],
    }

    sla_ctx = {
        "cutoff_type": "INSTITUTION_INTERNAL",
        "time_remaining_seconds": 2400,
        "sla_status": "ON_TRACK",
        "contract_refs": ["SLA-CORE-PAYROLL-01"],
    }

    agent = PolicySLAAgent()
    result = agent.run(envelope, authoritative_policy_decision=auth_decision, sla_context=sla_ctx)

    assert result.status == "SUCCESS"
    assert result.output is not None
    assert isinstance(result.output, PolicySLAOutput)
    assert result.output.authoritative_policy_decision_refs == ["POL-DEC-301"]
    assert "CANDIDATE_ONLY_REMEDIATION" in result.output.active_obligations
    assert "PROHIBIT_DIRECT_ORIGINAL_MUTATION" in result.output.active_prohibitions
    assert result.output.cutoff_type == "INSTITUTION_INTERNAL"
    assert result.output.sla_status == "ON_TRACK"
    assert result.output.time_remaining_seconds == 2400
    assert result.output.statement == "The AI Policy/SLA analyst operates in a read-only capacity and has made no system state changes."
    assert result.execution_source in ["DETERMINISTIC_FALLBACK", "LIVE_GEMINI"]


def test_policy_sla_agent_sla_truth_and_cutoff_provenance():
    """SLA Truth: Confirms that internal institution cutoffs are not mislabeled as network rules."""
    envelope = AgentContextEnvelope(
        tenant_id="TENANT-BANK-02",
        incident_id=302,
        correlation_id="corr-sla-302",
        findings=[],
    )

    sla_ctx = {
        "cutoff_type": "PARTNER_CONTRACT",
        "time_remaining_seconds": 600,
        "sla_status": "AT_RISK",
        "contract_refs": ["SLA-ACME-DISBURSEMENT-02"],
    }

    agent = PolicySLAAgent()
    result = agent.run(envelope, sla_context=sla_ctx)

    assert result.status == "SUCCESS"
    assert result.output is not None
    assert result.output.cutoff_type == "PARTNER_CONTRACT"
    assert result.output.sla_status == "AT_RISK"
    assert result.output.escalation_required is True
    assert any("expiring within 30 minutes" in rf for rf in result.output.risk_factors)
