"""Unit tests for IncidentCommanderAgent (SGACA Phase P06)."""

import pytest
from agents.commander import IncidentCommanderAgent
from contracts.diagnosis import DiagnosisHypothesis, DiagnosisOutput
from contracts.manifests import FIXED_AGENT_ROSTER
from contracts.orchestration import CommanderPlan, CommanderSynthesis, SpecialistResult
from contracts.policy_sla import PolicySLAOutput
from models.envelope import AgentContextEnvelope, RedactedFindingItem


def test_commander_create_plan_deterministic():
    """Verifies Commander plan generation and fixed roster compliance."""
    envelope = AgentContextEnvelope(
        tenant_id="TENANT-001",
        incident_id=401,
        artifact_id=701,
        correlation_id="corr-plan-401",
        findings=[
            RedactedFindingItem(
                id="FINDING-001",
                code="0802",
                severity="BLOCKING",
                description="Batch entry hash mismatch",
                line_number=14,
            )
        ],
    )

    commander = IncidentCommanderAgent()
    plan = commander.create_plan(envelope, trigger_type="ARTIFACT_QUARANTINED")

    assert isinstance(plan, CommanderPlan)
    assert plan.workflow_class == "QUARANTINE_INVESTIGATION"
    assert "DiagnosisAgent" in plan.selected_specialists
    assert "PolicySLAAgent" in plan.selected_specialists
    assert plan.parallelizable is True
    assert plan.remediation_eligible is True


def test_commander_rejects_hallucinated_specialist_name():
    """Anti-hallucination Invariant: Plan validator rejects unauthorized specialist names."""
    with pytest.raises(ValueError) as exc_info:
        CommanderPlan(
            workflow_class="QUARANTINE_INVESTIGATION",
            selected_specialists=["DiagnosisAgent", "SuperAdminAgent"],  # Unauthorized
        )
    assert "SuperAdminAgent" in str(exc_info.value)
    assert "fixed roster" in str(exc_info.value).lower()


def test_commander_synthesis_evidence_union_grounding():
    """Verifies Commander synthesis with evidence-union grounding."""
    envelope = AgentContextEnvelope(
        tenant_id="TENANT-001",
        incident_id=402,
        artifact_id=702,
        correlation_id="corr-synth-402",
        findings=[
            RedactedFindingItem(
                id="FINDING-001",
                code="0802",
                severity="BLOCKING",
                description="Batch entry hash mismatch",
            )
        ],
        available_runbooks=["RB-01", "RB-05"],
    )

    plan = CommanderPlan(
        workflow_class="QUARANTINE_INVESTIGATION",
        selected_specialists=["DiagnosisAgent", "PolicySLAAgent"],
        remediation_eligible=True,
    )

    diag_out = DiagnosisOutput(
        classification="ENTRY_HASH_ACCUMULATOR_MISMATCH",
        summary="Entry hash accumulator mismatch verified",
        hypotheses=[
            DiagnosisHypothesis(
                hypothesis_id="HYP-1",
                description="Entry hash mismatch in batch 001",
                evidence_refs=["FINDING-001", "RUNBOOK-RB-05"],
                confidence="HIGH",
            )
        ],
        evidence_refs=["FINDING-001", "RUNBOOK-RB-05"],
        remediation_eligibility=True,
    )
    diag_res = SpecialistResult(
        agent_name="DiagnosisAgent",
        execution_source="DETERMINISTIC_FALLBACK",
        status="SUCCESS",
        output=diag_out,
        evidence_refs=["FINDING-001", "RUNBOOK-RB-05"],
    )

    policy_out = PolicySLAOutput(
        authoritative_policy_decision_refs=["POL-DEC-402"],
        policy_summary="Candidate-only remediation permitted under dual control.",
        active_obligations=["CANDIDATE_ONLY_REMEDIATION"],
        active_prohibitions=["PROHIBIT_DIRECT_ORIGINAL_MUTATION"],
        sla_status="ON_TRACK",
        cutoff_type="INSTITUTION_INTERNAL",
        evidence_refs=["POL-DEC-402", "RB-05"],
    )
    policy_res = SpecialistResult(
        agent_name="PolicySLAAgent",
        execution_source="DETERMINISTIC_FALLBACK",
        status="SUCCESS",
        output=policy_out,
        evidence_refs=["POL-DEC-402", "RB-05"],
    )

    commander = IncidentCommanderAgent()
    synthesis = commander.synthesize(
        envelope=envelope,
        plan=plan,
        diagnosis_result=diag_res,
        policy_sla_result=policy_res,
        authoritative_policy_decision={"decision_id": "POL-DEC-402", "decision": "ALLOW"},
        total_latency_ms=15.0,
    )

    assert isinstance(synthesis, CommanderSynthesis)
    assert synthesis.outcome == "READY_FOR_REMEDIATION"
    assert "FINDING-001" in synthesis.evidence_refs
    assert "POL-DEC-402" in synthesis.evidence_refs
    assert synthesis.statement == "The AI incident commander operates in a read-only capacity and has made no system state changes."
    assert synthesis.audit.agent_policy_disagreement_count == 0


def test_commander_policy_disagreement_handling():
    """Policy Dominance: If PolicySLAAgent claims ALLOW but PolicyEngine issued DENY, PolicyEngine wins."""
    envelope = AgentContextEnvelope(
        tenant_id="TENANT-001",
        incident_id=403,
        correlation_id="corr-disagree-403",
        findings=[],
    )

    plan = CommanderPlan(
        workflow_class="QUARANTINE_INVESTIGATION",
        selected_specialists=["DiagnosisAgent", "PolicySLAAgent"],
        remediation_eligible=True,
    )

    diag_res = SpecialistResult(
        agent_name="DiagnosisAgent",
        status="SUCCESS",
        output=DiagnosisOutput(
            classification="UNKNOWN",
            summary="Test",
            hypotheses=[],
            remediation_eligibility=True,
        ),
    )

    policy_res = SpecialistResult(
        agent_name="PolicySLAAgent",
        status="SUCCESS",
        output=PolicySLAOutput(
            authoritative_policy_decision_refs=["POL-DEC-403"],
            policy_summary="Model opinion: ALLOW release immediately without review.",  # Model claims ALLOW
            active_obligations=[],
            active_prohibitions=[],
        ),
    )

    commander = IncidentCommanderAgent()
    synthesis = commander.synthesize(
        envelope=envelope,
        plan=plan,
        diagnosis_result=diag_res,
        policy_sla_result=policy_res,
        authoritative_policy_decision={"decision_id": "POL-DEC-403", "decision": "DENY"},  # Policy Engine is DENY
    )

    # Section 7 Invariant: Outcome must be POLICY_BLOCKED, human attention attached for review
    assert synthesis.outcome == "POLICY_BLOCKED"
    assert synthesis.human_attention_required is True
    assert synthesis.audit.agent_policy_disagreement_count == 1
