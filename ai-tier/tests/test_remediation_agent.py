"""Unit and Integration Tests for RemediationAgent (SGACA Phase P07)."""

import pytest
from agents.remediation import RemediationAgent
from contracts.manifests import FIXED_AGENT_ROSTER
from contracts.remediation import (
    RemediationOperation,
    RemediationOperationType,
    RemediationPlan,
)
from models.envelope import AgentContextEnvelope, RedactedFindingItem


def test_remediation_agent_manifest_conformance():
    """Verifies that RemediationAgent is registered with Autonomy Level A2 and exact metadata."""
    assert "RemediationAgent" in FIXED_AGENT_ROSTER
    manifest = FIXED_AGENT_ROSTER["RemediationAgent"]
    assert manifest.name == "RemediationAgent"
    assert manifest.autonomy_level == "A2"
    assert manifest.model == "gemini-3.5-flash"
    assert manifest.output_schema_name == "RemediationPlan"
    assert "artifact.release" in manifest.denied_capabilities
    assert "incident.approve" in manifest.denied_capabilities
    assert "artifact.write_direct" in manifest.denied_capabilities


def test_remediation_agent_deterministic_fallback():
    """Verifies that RemediationAgent produces valid structured RemediationPlan in deterministic fallback."""
    agent = RemediationAgent()
    envelope = AgentContextEnvelope(
        tenant_id="TENANT-001",
        workflow_id="wf-test-rem-001",
        incident_id=101,
        artifact_id=202,
        artifact_sha256="abc123parentsha",
        correlation_id="corr-test-rem-001",
        authorized_evidence_refs=["FINDING-001", "FINDING-002"],
        findings=[
            RedactedFindingItem(
                id="FINDING-001",
                code="0802",
                severity="BLOCKING",
                description="Batch debit total mismatch",
            ),
            RedactedFindingItem(
                id="FINDING-002",
                code="0902",
                severity="BLOCKING",
                description="File debit total mismatch",
            ),
        ],
    )

    plan = agent.run(envelope, attempt_number=1)
    assert isinstance(plan, RemediationPlan)
    assert plan.schema_version == "1.0"
    assert plan.workflow_id == "wf-test-rem-001"
    assert plan.tenant_id == "TENANT-001"
    assert plan.incident_id == 101
    assert plan.artifact_id == 202
    assert plan.expected_parent_sha256 == "abc123parentsha"
    assert plan.attempt_number == 1
    assert len(plan.operations) >= 1
    assert plan.confidence in ["HIGH", "MEDIUM", "LOW"]
    assert plan.statement.startswith("The RemediationAgent proposes typed remediation intent")


def test_remediation_operation_enum_validation():
    """Verifies that only allowlisted operations can be created."""
    op1 = RemediationOperation(
        operation_type=RemediationOperationType.RECOMPUTE_BATCH_CONTROL_TOTAL,
        target_ref="BATCH-1",
        finding_refs=["FINDING-001"],
        rationale="Recompute arithmetic totals from entry detail records",
        evidence_refs=["FINDING-001"],
    )
    assert op1.operation_type == "RECOMPUTE_BATCH_CONTROL_TOTAL"

    op2 = RemediationOperation(
        operation_type=RemediationOperationType.RECOMPUTE_FILE_CONTROL_TOTAL,
        target_ref="FILE_CONTROL",
        finding_refs=["FINDING-002"],
        rationale="Recompute file block count and debit sum",
        evidence_refs=["FINDING-002"],
    )
    assert op2.operation_type == "RECOMPUTE_FILE_CONTROL_TOTAL"

    with pytest.raises(ValueError):
        RemediationOperation(
            operation_type="ARBITRARY_BYTE_PATCH",  # Disallowed
            target_ref="RECORD-3",
            finding_refs=[],
            rationale="Disallowed operation",
        )
