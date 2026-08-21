"""Unit tests for Tool Gateway Client and Tool Adapter (SGACA P05)."""

import pytest
from tools.gateway_client import (
    ToolExecutionRecord,
    ToolGatewayClient,
    ToolGatewayContext,
    ToolGatewayError,
    ToolPolicyDeniedError,
)
from tools.tool_adapter import (
    ArtifactMetadataOutput,
    IncidentMetadataOutput,
    RedactedFindingOutput,
    SentinelToolAdapter,
    WorkflowStatusOutput,
    GEMINI_TOOL_DECLARATIONS,
)


class MockToolGatewayClient(ToolGatewayClient):
    """Mock client returning deterministic responses for testing."""
    def __init__(self):
        # Do not initialize real HTTP connection
        self.invocations = []

    def execute_tool(self, tool_id: str, business_args: dict, context: ToolGatewayContext, **kwargs) -> ToolExecutionRecord:
        self.invocations.append({"tool_id": tool_id, "args": business_args, "context": context})

        if tool_id == "incident.get":
            return ToolExecutionRecord(
                invocation_id="inv-001",
                tool_id=tool_id,
                status="SUCCEEDED",
                output={
                    "incident_id": business_args["incident_id"],
                    "tenant_id": context.tenant_id,
                    "status": "QUARANTINED",
                    "data_classification": "METADATA_ONLY",
                    "created_at": "2026-08-19T12:00:00Z",
                },
            )
        elif tool_id == "validation.findings.list_redacted":
            return ToolExecutionRecord(
                invocation_id="inv-002",
                tool_id=tool_id,
                status="SUCCEEDED",
                output=[
                    {
                        "finding_code": "0802",
                        "severity": "BLOCKING",
                        "message": "Batch entry hash accumulator mismatch",
                        "tenant_id": context.tenant_id,
                        "artifact_id": business_args["artifact_id"],
                        "redacted_account_ref": "ACCT-****-1234",
                        "rule_citation": "RULE-ACH-0802",
                        "data_classification": "REDACTED_FINDINGS",
                    }
                ],
            )
        elif tool_id == "artifact.metadata.get":
            return ToolExecutionRecord(
                invocation_id="inv-003",
                tool_id=tool_id,
                status="SUCCEEDED",
                output={
                    "artifact_id": business_args["artifact_id"],
                    "tenant_id": context.tenant_id,
                    "state": "QUARANTINED",
                    "data_classification": "METADATA_ONLY",
                    "artifact_sha256": "e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855",
                    "created_at": "2026-08-19T12:00:00Z",
                },
            )
        elif tool_id == "workflow.get":
            return ToolExecutionRecord(
                invocation_id="inv-004",
                tool_id=tool_id,
                status="SUCCEEDED",
                output={
                    "workflow_id": business_args["workflow_id"],
                    "tenant_id": context.tenant_id,
                    "state": "INVESTIGATING",
                    "attempt_count": 1,
                    "data_classification": "METADATA_ONLY",
                    "updated_at": "2026-08-19T12:00:00Z",
                },
            )
        raise ToolGatewayError(f"Unknown tool {tool_id}", "NOT_FOUND")


def test_tool_adapter_business_args_only():
    """Verifies that model calls supply ONLY business parameters and context is injected."""
    mock_client = MockToolGatewayClient()
    ctx = ToolGatewayContext(
        tenant_id="TENANT-BANK-01",
        correlation_id="corr-test-100",
        caller_id="DiagnosisAgent",
        caller_autonomy_level=1,
    )
    adapter = SentinelToolAdapter(mock_client, ctx)

    # 1. incident.get
    inc = adapter.get_incident("1001")
    assert isinstance(inc, IncidentMetadataOutput)
    assert inc.incident_id == "1001"
    assert inc.tenant_id == "TENANT-BANK-01"
    assert inc.data_classification == "METADATA_ONLY"

    # 2. validation.findings.list_redacted
    findings = adapter.list_redacted_findings("501")
    assert len(findings) == 1
    assert isinstance(findings[0], RedactedFindingOutput)
    assert findings[0].finding_code == "0802"
    assert findings[0].redacted_account_ref == "ACCT-****-1234"
    assert findings[0].data_classification == "REDACTED_FINDINGS"

    # 3. artifact.metadata.get
    art = adapter.get_artifact_metadata("501")
    assert isinstance(art, ArtifactMetadataOutput)
    assert art.artifact_sha256.startswith("e3b0")

    # 4. workflow.get
    wf = adapter.get_workflow("wf-101")
    assert isinstance(wf, WorkflowStatusOutput)
    assert wf.state == "INVESTIGATING"


def test_gemini_tool_declarations_schema_purity():
    """Verifies that function declarations expose ONLY business semantic arguments."""
    for decl in GEMINI_TOOL_DECLARATIONS:
        params = decl["parameters"]["properties"]
        # Model must NEVER be offered tenant_id, caller_id, autonomy, roles, or decision parameters
        assert "tenant_id" not in params
        assert "caller_id" not in params
        assert "autonomy_level" not in params
        assert "caller_roles" not in params
        assert "policy_decision" not in params
