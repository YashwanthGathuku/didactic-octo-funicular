"""Unit tests for MultiAgentWorkflowOrchestrator (SGACA Phase P06)."""

import pytest
from contracts.orchestration import AgentTriggerEvent, CommanderSynthesis
from models.envelope import AgentContextEnvelope, RedactedFindingItem
from orchestrator.fleet import MultiAgentWorkflowOrchestrator


def test_multi_agent_workflow_full_trajectory_ready_for_remediation():
    """Section 30: Full multi-agent investigation fixture for control-total / hash mismatch."""
    envelope = AgentContextEnvelope(
        tenant_id="TENANT-BANK-01",
        incident_id=501,
        artifact_id=801,
        artifact_sha256="e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855",
        correlation_id="corr-wf-501",
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

    trigger = AgentTriggerEvent(
        event_id="EVT-TRIG-501",
        event_type="ARTIFACT_QUARANTINED",
        tenant_id="TENANT-BANK-01",
        subject_refs=["INC-501", "ART-801"],
        occurred_at="2026-08-19T16:00:00Z",
        correlation_id="corr-wf-501",
        event_hash="e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855",
    )

    auth_decision = {
        "decision_id": "POL-DEC-501",
        "decision": "ALLOW",
        "obligations": ["CANDIDATE_ONLY_REMEDIATION"],
    }

    orchestrator = MultiAgentWorkflowOrchestrator()
    synthesis = orchestrator.run_workflow(
        envelope=envelope,
        trigger_event=trigger,
        authoritative_policy_decision=auth_decision,
    )

    assert isinstance(synthesis, CommanderSynthesis)
    assert synthesis.outcome == "READY_FOR_REMEDIATION"
    assert synthesis.diagnosis_result is not None
    assert synthesis.diagnosis_result.status == "SUCCESS"
    assert synthesis.policy_sla_result is not None
    assert synthesis.policy_sla_result.status == "SUCCESS"
    assert "FINDING-001" in synthesis.evidence_refs
    assert "POL-DEC-501" in synthesis.evidence_refs
    assert synthesis.statement == "The AI incident commander operates in a read-only capacity and has made no system state changes."


def test_multi_agent_workflow_event_idempotency():
    """Section 19: Duplicate event trigger returns identical synthesis without duplicate runs."""
    envelope = AgentContextEnvelope(
        tenant_id="TENANT-BANK-01",
        incident_id=502,
        correlation_id="corr-idem-502",
        findings=[],
    )

    trigger = AgentTriggerEvent(
        event_id="EVT-TRIG-502",
        event_type="ARTIFACT_QUARANTINED",
        tenant_id="TENANT-BANK-01",
        occurred_at="2026-08-19T16:00:00Z",
        correlation_id="corr-idem-502",
        event_hash="a1b2c3d4e5f60718293a4b5c6d7e8f90123456789abcdef0123456789abcdef0",
    )

    orchestrator = MultiAgentWorkflowOrchestrator()

    synth_1 = orchestrator.run_workflow(envelope=envelope, trigger_event=trigger)
    synth_2 = orchestrator.run_workflow(envelope=envelope, trigger_event=trigger)

    assert synth_1.workflow_id == synth_2.workflow_id
    assert synth_1.outcome == synth_2.outcome
    assert synth_1 == synth_2


def test_multi_agent_workflow_partial_specialist_failure_handling():
    """Section 21: Partial specialist failure does not pretend complete analysis; sets PARTIAL_SPECIALIST_FAILURE."""
    class FailingPolicyAgent:
        def run(self, *args, **kwargs):
            raise TimeoutError("PolicySLAAgent simulated timeout")

    envelope = AgentContextEnvelope(
        tenant_id="TENANT-BANK-01",
        incident_id=503,
        correlation_id="corr-partial-503",
        findings=[],
    )

    orchestrator = MultiAgentWorkflowOrchestrator(policy_sla_agent=FailingPolicyAgent())  # type: ignore
    synthesis = orchestrator.run_workflow(envelope=envelope)

    assert synthesis.outcome == "PARTIAL_SPECIALIST_FAILURE"
    assert synthesis.policy_sla_result is not None
    assert synthesis.policy_sla_result.status in ["FAILED", "TIMEOUT"]
    assert "Partial specialist failure" in synthesis.synthesis_summary
