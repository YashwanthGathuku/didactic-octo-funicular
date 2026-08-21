"""Canonical immutable AgentContextEnvelope models for Google ADK Agent Control Plane.

Invariants:
- TenantID is injected by the authenticated gateway server and cannot be overridden by client prompt.
- IncidentID, ArtifactID, and ValidationRunID are strictly distinguished.
- Raw financial data is excluded by construction.
"""

from __future__ import annotations

from typing import Any, Dict, List, Literal, Optional
from pydantic import BaseModel, Field, model_validator


class AgentBudget(BaseModel):
    """Resource constraints for agent execution."""
    max_tokens: int = 4096
    max_seconds: int = 15
    max_cost_usd: float = 0.05


class RedactedFindingItem(BaseModel):
    """Pre-redacted validation finding excerpt."""
    id: str
    code: str
    severity: Literal["INFO", "WARNING", "BLOCKING"]
    description: str
    rule_version: str = "1.0"
    provenance: str = ""
    line_number: Optional[int] = None
    byte_offset: Optional[int] = None
    field_start: Optional[int] = None
    field_end: Optional[int] = None
    evidence_redacted: Optional[str] = None
    expected_value: Optional[str] = None
    actual_value: Optional[str] = None
    # Hard exclusion: raw_data, raw_content, full_record, account_number


class AgentContextEnvelope(BaseModel):
    """Canonical immutable context envelope passed from Go gateway to AI tier."""
    schema_version: Literal["1.0"] = "1.0"
    workflow_id: str = ""
    tenant_id: str = Field(..., min_length=1)
    trigger_event_id: str = ""
    incident_id: int = Field(..., gt=0)
    artifact_id: int = Field(0, ge=0)
    artifact_sha256: str = Field(default="0000000000000000000000000000000000000000000000000000000000000000")
    validation_run_id: str = ""
    policy_version: str = "default/1"
    correlation_id: str = Field(..., min_length=1)
    trace_id: str = ""
    agent_name: str = "SentinelCoordinator"
    agent_version: str = "1.0.0"
    authorized_evidence_refs: List[str] = Field(default_factory=list)
    allowed_tools: List[str] = Field(default_factory=list)
    budget: AgentBudget = Field(default_factory=AgentBudget)
    findings: List[RedactedFindingItem] = Field(default_factory=list)
    available_runbooks: List[str] = Field(default_factory=lambda: ["RB-01", "RB-05"])
    telemetry_summary: Dict[str, Any] = Field(default_factory=dict)
    filename: str = "unnamed.ach"
    prior_occurrences: int = 0

    @model_validator(mode="after")
    def validate_security_invariants(self) -> "AgentContextEnvelope":
        if not self.tenant_id:
            raise ValueError("tenant_id must be supplied by authenticated server context")
        if self.incident_id <= 0:
            raise ValueError("incident_id must be positive non-zero integer")
        if self.artifact_id < 0:
            raise ValueError("artifact_id must be non-negative integer")
        return self
