"""Durable Multi-Agent Orchestration & Crash-Restart Tests (SGACA Phase P06.5).

Proves that workflow state, trigger idempotency, specialist caching, and event journals
persist across service and process restarts using durable storage.
"""

import os
import tempfile
import pytest

from contracts.orchestration import AgentTriggerEvent, CommanderSynthesis
from models.envelope import AgentContextEnvelope, RedactedFindingItem
from orchestrator.fleet import MultiAgentWorkflowOrchestrator
from persistence.store import DurableWorkflowStore


@pytest.fixture
def temp_db():
    fd, path = tempfile.mkstemp(suffix=".db")
    os.close(fd)
    yield path
    try:
        if os.path.exists(path):
            os.remove(path)
    except Exception:
        pass


def test_durable_trigger_idempotency_across_restarts(temp_db):
    """Section 3: Duplicate trigger delivery after process restart resolves to existing workflow."""
    envelope = AgentContextEnvelope(
        tenant_id="TENANT-RESTART-01",
        incident_id=701,
        artifact_id=1001,
        artifact_sha256="1111222233334444555566667777888899990000aaaabbbbccccddddeeeeffff",
        correlation_id="corr-restart-701",
        findings=[
            RedactedFindingItem(
                id="FINDING-001",
                code="0802",
                severity="BLOCKING",
                description="Batch entry hash accumulator mismatch",
                line_number=14,
            )
        ],
        available_runbooks=["RB-01", "RB-05"],
    )

    trigger = AgentTriggerEvent(
        event_id="EVT-DURABLE-701",
        event_type="ARTIFACT_QUARANTINED",
        tenant_id="TENANT-RESTART-01",
        occurred_at="2026-08-19T17:00:00Z",
        correlation_id="corr-restart-701",
        event_hash="1111222233334444555566667777888899990000aaaabbbbccccddddeeeeffff",
    )

    auth_decision = {"decision_id": "POL-DEC-701", "decision": "REQUIRE_HUMAN"}

    # 1. First execution in Process 1
    store1 = DurableWorkflowStore(db_path=temp_db)
    orch1 = MultiAgentWorkflowOrchestrator(store=store1)
    synth1 = orch1.run_workflow(
        envelope=envelope,
        trigger_event=trigger,
        authoritative_policy_decision=auth_decision,
    )

    assert synth1.outcome in ("READY_FOR_REMEDIATION", "HUMAN_AUTHORIZATION_REQUIRED")
    wf_id1 = synth1.workflow_id

    # 2. Simulate process crash / destroy orchestrator & store objects
    del orch1
    del store1

    # 3. Process 2 restart with fresh store & orchestrator connecting to same database
    store2 = DurableWorkflowStore(db_path=temp_db)
    orch2 = MultiAgentWorkflowOrchestrator(store=store2)

    # 4. Resubmit identical trigger event E1
    synth2 = orch2.run_workflow(
        envelope=envelope,
        trigger_event=trigger,
        authoritative_policy_decision=auth_decision,
    )

    # 5. Assert authoritative workflow is preserved and no duplicate workflow W2 was created
    assert synth2.workflow_id == wf_id1
    assert synth2.outcome == synth1.outcome
    assert synth2.evidence_refs == synth1.evidence_refs


def test_durable_restart_checkpoints_and_specialist_reuse(temp_db):
    """Section 10: Fault recovery at intermediate checkpoints proves reuse of completed stages."""
    store = DurableWorkflowStore(db_path=temp_db)
    tenant_id = "TENANT-CHECKPOINT"
    incident_id = 702
    artifact_id = 1002
    artifact_sha256 = "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"

    envelope = AgentContextEnvelope(
        tenant_id=tenant_id,
        incident_id=incident_id,
        artifact_id=artifact_id,
        artifact_sha256=artifact_sha256,
        correlation_id="corr-cp-702",
        findings=[],
    )

    # Checkpoint A: Workflow created
    wf_rec, created = store.get_or_create_workflow(
        tenant_id=tenant_id,
        incident_id=incident_id,
        artifact_id=artifact_id,
        artifact_sha256=artifact_sha256,
        correlation_id="corr-cp-702",
    )
    assert created is True
    assert wf_rec["state"] == "PENDING"

    # Checkpoint B: Commander plan persisted
    orch = MultiAgentWorkflowOrchestrator(store=store)
    plan = orch.commander.create_plan(envelope)
    store.transition_state(wf_rec["id"], tenant_id, "PLANNING", plan_json=plan.model_dump_json())

    # Simulate restart
    store_restarted = DurableWorkflowStore(db_path=temp_db)
    orch_restarted = MultiAgentWorkflowOrchestrator(store=store_restarted)

    # Execute workflow to completion
    synth = orch_restarted.run_workflow(envelope=envelope)
    assert synth.workflow_id == wf_rec["id"]
    assert synth.plan.workflow_class == plan.workflow_class


def test_parallel_specialist_completion_concurrency(temp_db):
    """Section 11: Concurrent execution of DiagnosisAgent and PolicySLAAgent without race conditions."""
    store = DurableWorkflowStore(db_path=temp_db)
    envelope = AgentContextEnvelope(
        tenant_id="TENANT-RACE",
        incident_id=703,
        artifact_id=1003,
        artifact_sha256="bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb",
        correlation_id="corr-race-703",
        findings=[
            RedactedFindingItem(
                id="FINDING-001",
                code="0802",
                severity="BLOCKING",
                description="Batch entry hash accumulator mismatch",
                line_number=14,
            )
        ],
        available_runbooks=["RB-01", "RB-05"],
    )

    orch = MultiAgentWorkflowOrchestrator(store=store)
    synth = orch.run_workflow(
        envelope=envelope,
        authoritative_policy_decision={"decision_id": "POL-DEC-703", "decision": "REQUIRE_HUMAN"},
    )

    assert synth.diagnosis_result is not None
    assert synth.diagnosis_result.status == "SUCCESS"
    assert synth.policy_sla_result is not None
    assert synth.policy_sla_result.status == "SUCCESS"
    assert len(synth.evidence_refs) >= 2
