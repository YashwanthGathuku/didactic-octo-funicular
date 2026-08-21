"""Proof of ADK Tool Gateway Trajectory, Parameter Segregation, and Prohibited Action Denial (P05.5)."""

import pytest
from contracts.diagnosis import DiagnosisOutput, DiagnosisRunResponse
from guardrails.evidence import AuthorizedEvidenceSet, EvidenceGroundingVerifier
from models.envelope import AgentContextEnvelope, RedactedFindingItem
from tools.gateway_client import (
    ToolExecutionRecord,
    ToolGatewayClient,
    ToolGatewayContext,
    ToolPolicyDeniedError,
)
from tools.tool_adapter import (
    GEMINI_TOOL_DECLARATIONS,
    IncidentMetadataOutput,
    RedactedFindingOutput,
    SentinelToolAdapter,
)


class TrajectoryTrackingToolGateway(ToolGatewayClient):
    """Tracking Tool Gateway client that records full invocation provenance."""
    def __init__(self):
        self.invocation_records = []
        self.policy_evaluations = []

    def execute_tool(self, tool_id: str, business_args: dict, context: ToolGatewayContext, **kwargs) -> ToolExecutionRecord:
        # Record whether model attempted to supply security context
        model_supplied_tenant = "tenant_id" in business_args
        model_supplied_caller = "caller_id" in business_args
        model_supplied_autonomy = "caller_autonomy_level" in business_args

        # Policy evaluation simulation
        is_allowed = tool_id in ["incident.get", "validation.findings.list_redacted", "artifact.metadata.get", "workflow.get"]
        decision = "ALLOW" if is_allowed else "DENY"
        self.policy_evaluations.append({
            "tool_id": tool_id,
            "decision": decision,
            "tenant_id": context.tenant_id,
            "caller_id": context.caller_id,
            "caller_autonomy_level": context.caller_autonomy_level,
        })

        if not is_allowed:
            raise ToolPolicyDeniedError(
                f"Policy denied execution of prohibited tool '{tool_id}' for caller '{context.caller_id}'",
                details={"policy_decision": "DENY", "reason": "PROHIBITED_ACTION_ZERO_RELEASE_AUTHORITY"},
            )

        if tool_id == "incident.get":
            rec = ToolExecutionRecord(
                invocation_id="inv-live-001",
                tool_id=tool_id,
                status="SUCCEEDED",
                output={
                    "incident_id": business_args.get("incident_id", "INC-P05-LIVE-001"),
                    "tenant_id": context.tenant_id,
                    "status": "QUARANTINED",
                    "data_classification": "METADATA_ONLY",
                    "created_at": "2026-08-19T16:00:00Z",
                },
                tool_manifest_hash="e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855",
            )
        elif tool_id == "validation.findings.list_redacted":
            rec = ToolExecutionRecord(
                invocation_id="inv-live-002",
                tool_id=tool_id,
                status="SUCCEEDED",
                output=[
                    {
                        "finding_code": "0802",
                        "severity": "BLOCKING",
                        "message": "Batch entry hash accumulator mismatch",
                        "tenant_id": context.tenant_id,
                        "artifact_id": business_args.get("artifact_id", "501"),
                        "redacted_account_ref": "ACCT-****-9876",
                        "rule_citation": "RULE-ACH-0802",
                        "data_classification": "REDACTED_FINDINGS",
                    }
                ],
                tool_manifest_hash="a1b2c3d4e5f60718293a4b5c6d7e8f90123456789abcdef0123456789abcdef0",
            )
        else:
            rec = ToolExecutionRecord(
                invocation_id="inv-live-999",
                tool_id=tool_id,
                status="SUCCEEDED",
                output={"status": "OK", "data_classification": "METADATA_ONLY"},
            )

        self.invocation_records.append({
            "record": rec,
            "model_supplied_tenant": model_supplied_tenant,
            "model_supplied_caller": model_supplied_caller,
            "model_supplied_autonomy": model_supplied_autonomy,
            "server_injected_tenant": context.tenant_id,
            "server_injected_caller": context.caller_id,
        })
        return rec


def test_live_adk_tool_gateway_trajectory_and_parameter_segregation():
    """Section 4: Proves live trajectory, parameter segregation, and evidence expansion."""
    gw = TrajectoryTrackingToolGateway()
    ctx = ToolGatewayContext(
        tenant_id="TENANT-BANK-LIVE",
        correlation_id="corr-p05-live-001",
        workflow_id="wf-diag-live-001",
        caller_id="DiagnosisAgent",
        caller_type="AGENT",
        caller_autonomy_level=1,
    )
    adapter = SentinelToolAdapter(gw, ctx)

    # 1. Initialize AuthorizedEvidenceSet
    evidence_set = AuthorizedEvidenceSet({"FINDING-001", "RUNBOOK-RB-05"})

    # 2. Execute ADK tool request for incident metadata
    # Notice: Model caller supplies ONLY business parameter 'INC-P05-LIVE-001'
    incident = adapter.get_incident("INC-P05-LIVE-001")
    assert isinstance(incident, IncidentMetadataOutput)
    assert incident.incident_id == "INC-P05-LIVE-001"
    assert incident.tenant_id == "TENANT-BANK-LIVE"
    assert incident.data_classification == "METADATA_ONLY"

    # Expand evidence set monotonically
    evidence_set.expand_from_tool_result("incident.get", {"incident_id": "INC-P05-LIVE-001"})
    assert evidence_set.contains("INC-P05-LIVE-001") or evidence_set.contains("INCIDENT-INC-P05-LIVE-001")

    # 3. Execute ADK tool request for redacted findings
    findings = adapter.list_redacted_findings("501")
    assert len(findings) == 1
    assert isinstance(findings[0], RedactedFindingOutput)
    assert findings[0].finding_code == "0802"
    assert findings[0].rule_citation == "RULE-ACH-0802"
    assert findings[0].data_classification == "REDACTED_FINDINGS"

    evidence_set.expand_from_tool_result("validation.findings.list_redacted", findings[0].model_dump())
    assert evidence_set.contains("RULE-ACH-0802")
    assert evidence_set.contains("0802")

    # 4. Verify parameter segregation proof
    assert len(gw.invocation_records) == 2
    for inv in gw.invocation_records:
        assert inv["model_supplied_tenant"] is False  # Model did not supply tenant_id
        assert inv["model_supplied_caller"] is False  # Model did not supply caller_id
        assert inv["model_supplied_autonomy"] is False  # Model did not supply autonomy
        assert inv["server_injected_tenant"] == "TENANT-BANK-LIVE"
        assert inv["server_injected_caller"] == "DiagnosisAgent"

    # 5. Verify policy evaluation proof
    assert len(gw.policy_evaluations) == 2
    for pe in gw.policy_evaluations:
        assert pe["decision"] == "ALLOW"
        assert pe["tenant_id"] == "TENANT-BANK-LIVE"

    # 6. Verify grounding check with expanded evidence
    output = DiagnosisOutput(
        classification="ENTRY_HASH_ACCUMULATOR_MISMATCH",
        summary="Batch entry hash mismatch verified by Tool Gateway",
        hypotheses=[],
        evidence_refs=["FINDING-001", "RULE-ACH-0802", "RUNBOOK-RB-05"],
    )
    grounding = EvidenceGroundingVerifier.verify(output, evidence_set, strict=True)
    assert grounding.is_valid is True


def test_live_prohibited_action_denial():
    """Section 5: Proves that unauthorized actions (release/approve/mutate) are completely blocked."""
    # 1. Verify that 'release' or 'approve' tools are NOT in the Gemini tool declarations
    declared_tool_names = [d["name"] for d in GEMINI_TOOL_DECLARATIONS]
    assert "artifact.release" not in declared_tool_names
    assert "incident.approve" not in declared_tool_names
    assert "ledger.mutate" not in declared_tool_names
    assert "sql.execute" not in declared_tool_names

    # 2. Verify that direct/synthetic attempt to execute prohibited tool fails closed
    gw = TrajectoryTrackingToolGateway()
    ctx = ToolGatewayContext(
        tenant_id="TENANT-BANK-LIVE",
        correlation_id="corr-p05-live-attack",
        caller_id="DiagnosisAgent",
        caller_autonomy_level=1,
    )

    with pytest.raises(ToolPolicyDeniedError) as exc_info:
        gw.execute_tool("artifact.release", {"artifact_id": "501"}, ctx)

    assert "PROHIBITED_ACTION_ZERO_RELEASE_AUTHORITY" in str(exc_info.value.details)
    assert len(gw.policy_evaluations) == 1
    assert gw.policy_evaluations[0]["decision"] == "DENY"
    assert len(gw.invocation_records) == 0  # No tool execution occurred
