"""TOCTOU Invariants & Deterministic Policy Authority Tests (SGACA Phase P06.5).

Proves that:
1. Policy bundle TOCTOU fails closed (PolicyBundleHash(plan) != PolicyBundleHash(current) => OldPlanNotActionable).
2. Artifact hash TOCTOU fails closed (ArtifactHash(plan) != ArtifactHash(current) => OldPlanNotActionable).
3. Authoritative PolicyEngine decisions dominate model prose (DENY => POLICY_BLOCKED; REQUIRE_HUMAN => HUMAN_AUTHORIZATION_REQUIRED).
4. Protected binding hash changes invalidate cached specialist results.
"""

import os
import tempfile
import pytest

from contracts.orchestration import SpecialistResult
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


def test_policy_bundle_toctou_fails_closed(temp_db):
    """Section 5: Policy bundle hash mismatch between plan and synthesis fails closed."""
    store = DurableWorkflowStore(db_path=temp_db)
    orch = MultiAgentWorkflowOrchestrator(store=store)

    envelope = AgentContextEnvelope(
        tenant_id="TENANT-TOCTOU",
        incident_id=801,
        artifact_id=1101,
        artifact_sha256="ffff0000ffff0000ffff0000ffff0000ffff0000ffff0000ffff0000ffff0000",
        policy_version="policy/bundle/v1",
        correlation_id="corr-toctou-801",
        findings=[
            RedactedFindingItem(
                id="FINDING-001",
                code="0802",
                severity="BLOCKING",
                description="Entry hash mismatch",
            )
        ],
    )

    # Execute workflow with simulated current policy bundle changed to v2
    synth = orch.run_workflow(
        envelope=envelope,
        current_policy_bundle_hash="policy/bundle/v2",  # TOCTOU change
    )

    # Invariant: Old plan cannot authorize action; outcome must be UNRESOLVED
    assert synth.outcome == "UNRESOLVED"
    assert synth.human_attention_required is True
    assert "TOCTOU Policy Invalidation" in synth.synthesis_summary


def test_artifact_hash_toctou_fails_closed(temp_db):
    """Section 6: Artifact hash mismatch between plan and synthesis fails closed."""
    store = DurableWorkflowStore(db_path=temp_db)
    orch = MultiAgentWorkflowOrchestrator(store=store)

    envelope = AgentContextEnvelope(
        tenant_id="TENANT-TOCTOU",
        incident_id=802,
        artifact_id=1102,
        artifact_sha256="original_sha256_aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
        policy_version="default/1",
        correlation_id="corr-toctou-802",
        findings=[],
    )

    # Execute workflow with simulated artifact mutation
    synth = orch.run_workflow(
        envelope=envelope,
        current_artifact_sha256="mutated_sha256_bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb",  # Mutation
    )

    # Invariant: Mutated artifact invalidates plan; outcome must be UNRESOLVED
    assert synth.outcome == "UNRESOLVED"
    assert synth.human_attention_required is True
    assert "TOCTOU Artifact Mutation" in synth.synthesis_summary


def test_policy_engine_deny_vs_require_human_semantics(temp_db):
    """Section 7: Authoritative PolicyDecision == DENY produces POLICY_BLOCKED."""
    store = DurableWorkflowStore(db_path=temp_db)
    orch = MultiAgentWorkflowOrchestrator(store=store)

    envelope = AgentContextEnvelope(
        tenant_id="TENANT-AUTH",
        incident_id=803,
        correlation_id="corr-auth-803",
        findings=[],
    )

    # Case A: Policy Decision is DENY
    synth_deny = orch.run_workflow(
        envelope=envelope,
        authoritative_policy_decision={"decision_id": "POL-DEC-803", "decision": "DENY"},
    )
    assert synth_deny.outcome == "POLICY_BLOCKED"
    assert synth_deny.human_attention_required is True

    # Case B: Policy Decision is REQUIRE_HUMAN
    envelope.incident_id = 804
    synth_human = orch.run_workflow(
        envelope=envelope,
        authoritative_policy_decision={"decision_id": "POL-DEC-804", "decision": "REQUIRE_HUMAN"},
    )
    assert synth_human.outcome == "HUMAN_AUTHORIZATION_REQUIRED"
    assert synth_human.human_attention_required is True


def test_protected_state_bindings_invalidation(temp_db):
    """Section 4: Changing manifest hash or evidence set hash invalidates cached specialist results."""
    store = DurableWorkflowStore(db_path=temp_db)

    # Save a valid specialist result
    store.save_specialist_result(
        workflow_id="wf-test-binding",
        tenant_id="TENANT-BIND",
        agent_name="DiagnosisAgent",
        agent_version="1.0.0",
        manifest_hash="manifest_hash_original",
        input_context_hash="input_hash_1",
        artifact_sha256="artifact_sha_1",
        policy_bundle_hash="policy_v1",
        authorized_evidence_set_hash="evidence_hash_1",
        tool_manifest_hash="tools_hash_1",
        status="SUCCESS",
        output_json="{}",
        evidence_refs=["FINDING-001"],
        latency_ms=10.0,
    )

    # Query with matching bindings -> Valid hit
    valid = store.get_specialist_result_if_valid(
        workflow_id="wf-test-binding",
        agent_name="DiagnosisAgent",
        manifest_hash="manifest_hash_original",
        input_context_hash="input_hash_1",
        artifact_sha256="artifact_sha_1",
        policy_bundle_hash="policy_v1",
        authorized_evidence_set_hash="evidence_hash_1",
        tool_manifest_hash="tools_hash_1",
    )
    assert valid is not None

    # Query with changed manifest_hash -> STALE (returns None)
    stale_manifest = store.get_specialist_result_if_valid(
        workflow_id="wf-test-binding",
        agent_name="DiagnosisAgent",
        manifest_hash="manifest_hash_MODIFIED",
        input_context_hash="input_hash_1",
        artifact_sha256="artifact_sha_1",
        policy_bundle_hash="policy_v1",
        authorized_evidence_set_hash="evidence_hash_1",
        tool_manifest_hash="tools_hash_1",
    )
    assert stale_manifest is None

    # Query with changed evidence set hash -> STALE (returns None)
    stale_evidence = store.get_specialist_result_if_valid(
        workflow_id="wf-test-binding",
        agent_name="DiagnosisAgent",
        manifest_hash="manifest_hash_original",
        input_context_hash="input_hash_1",
        artifact_sha256="artifact_sha_1",
        policy_bundle_hash="policy_v1",
        authorized_evidence_set_hash="evidence_hash_MODIFIED",
        tool_manifest_hash="tools_hash_1",
    )
    assert stale_evidence is None
