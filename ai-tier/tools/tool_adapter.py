"""High-Level Tool Adapter for Google ADK and Gemini API (SGACA P05).

Provides strongly typed tools for:
- incident.get
- validation.findings.list_redacted
- artifact.metadata.get
- workflow.get

Invariants:
1. Only business parameters (incident_id, artifact_id, workflow_id) are exposed to models.
2. TenantID, CallerID, Autonomy, Roles, and Policy Decisions CANNOT be supplied by models.
3. Outputs are typed with explicit DataClassification annotations.
"""

from __future__ import annotations

from typing import Any, Dict, List, Optional
from pydantic import BaseModel, Field
from .gateway_client import ToolGatewayClient, ToolGatewayContext, ToolGatewayError


# ============================================================================
# Strongly Typed Output Models (Reflecting Go Tool Gateway Contracts)
# ============================================================================

class IncidentMetadataOutput(BaseModel):
    incident_id: str
    tenant_id: str
    status: str
    data_classification: str = "METADATA_ONLY"
    created_at: str


class RedactedFindingOutput(BaseModel):
    finding_code: str
    severity: str
    message: str
    tenant_id: str
    artifact_id: str
    batch_number: Optional[int] = None
    entry_detail_sequence: Optional[int] = None
    redacted_account_ref: Optional[str] = None
    rule_citation: Optional[str] = None
    data_classification: str = "REDACTED_FINDINGS"


class ArtifactMetadataOutput(BaseModel):
    artifact_id: str
    tenant_id: str
    state: str
    data_classification: str = "METADATA_ONLY"
    artifact_sha256: str
    byte_count: Optional[int] = None
    created_at: str


class WorkflowStatusOutput(BaseModel):
    workflow_id: str
    tenant_id: str
    state: str
    attempt_count: int = 1
    data_classification: str = "METADATA_ONLY"
    updated_at: str


# ============================================================================
# High-Level Tool Adapter
# ============================================================================

class SentinelToolAdapter:
    """Provides strongly-typed, security-governed tool methods for Agent execution."""

    def __init__(self, client: ToolGatewayClient, context: ToolGatewayContext):
        self.client = client
        self.context = context

    def get_incident(self, incident_id: str) -> IncidentMetadataOutput:
        """Retrieves metadata and lifecycle status for a quarantined file incident."""
        clean_args = {"incident_id": str(incident_id).strip()}
        rec = self.client.execute_tool(
            tool_id="incident.get",
            business_args=clean_args,
            context=self.context,
        )
        if rec.status != "SUCCEEDED" or not rec.output:
            raise ToolGatewayError(f"incident.get failed with status {rec.status}", "EXECUTION_FAILED")
        return IncidentMetadataOutput(**rec.output)

    def list_redacted_findings(self, artifact_id: str) -> List[RedactedFindingOutput]:
        """Lists deterministic validation findings with sensitive payload fields redacted."""
        clean_args = {"artifact_id": str(artifact_id).strip()}
        rec = self.client.execute_tool(
            tool_id="validation.findings.list_redacted",
            business_args=clean_args,
            context=self.context,
        )
        if rec.status != "SUCCEEDED" or rec.output is None:
            raise ToolGatewayError(f"validation.findings.list_redacted failed with status {rec.status}", "EXECUTION_FAILED")

        items = rec.output if isinstance(rec.output, list) else [rec.output]
        return [RedactedFindingOutput(**item) for item in items]

    def get_artifact_metadata(self, artifact_id: str) -> ArtifactMetadataOutput:
        """Retrieves immutable artifact metadata (SHA-256, classification, quarantine status)."""
        clean_args = {"artifact_id": str(artifact_id).strip()}
        rec = self.client.execute_tool(
            tool_id="artifact.metadata.get",
            business_args=clean_args,
            context=self.context,
        )
        if rec.status != "SUCCEEDED" or not rec.output:
            raise ToolGatewayError(f"artifact.metadata.get failed with status {rec.status}", "EXECUTION_FAILED")
        return ArtifactMetadataOutput(**rec.output)

    def get_workflow(self, workflow_id: str) -> WorkflowStatusOutput:
        """Retrieves agent workflow execution status and step timeline."""
        clean_args = {"workflow_id": str(workflow_id).strip()}
        rec = self.client.execute_tool(
            tool_id="workflow.get",
            business_args=clean_args,
            context=self.context,
        )
        if rec.status != "SUCCEEDED" or not rec.output:
            raise ToolGatewayError(f"workflow.get failed with status {rec.status}", "EXECUTION_FAILED")
        return WorkflowStatusOutput(**rec.output)


# ============================================================================
# Google ADK / Gemini API Function Declarations
# ============================================================================

GEMINI_TOOL_DECLARATIONS = [
    {
        "name": "incident_get",
        "description": "Retrieves metadata and status for a quarantined file incident within tenant boundaries.",
        "parameters": {
            "type": "OBJECT",
            "properties": {
                "incident_id": {
                    "type": "STRING",
                    "description": "The incident ID to inspect (e.g. '1001' or 'inc-1001').",
                },
            },
            "required": ["incident_id"],
        },
    },
    {
        "name": "validation_findings_list_redacted",
        "description": "Lists deterministic validation findings with sensitive financial payload fields redacted.",
        "parameters": {
            "type": "OBJECT",
            "properties": {
                "artifact_id": {
                    "type": "STRING",
                    "description": "The artifact/file instance ID whose validation findings should be retrieved.",
                },
            },
            "required": ["artifact_id"],
        },
    },
    {
        "name": "artifact_metadata_get",
        "description": "Retrieves immutable artifact metadata (SHA-256, classification, quarantined state).",
        "parameters": {
            "type": "OBJECT",
            "properties": {
                "artifact_id": {
                    "type": "STRING",
                    "description": "The artifact ID to retrieve metadata for.",
                },
            },
            "required": ["artifact_id"],
        },
    },
    {
        "name": "workflow_get",
        "description": "Retrieves agent workflow execution status and step timeline.",
        "parameters": {
            "type": "OBJECT",
            "properties": {
                "workflow_id": {
                    "type": "STRING",
                    "description": "The agent workflow ID to retrieve status for.",
                },
            },
            "required": ["workflow_id"],
        },
    },
]
