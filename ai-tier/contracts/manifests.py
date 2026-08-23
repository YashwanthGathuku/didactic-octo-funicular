"""Fixed Agent Roster and Canonical Manifests for SentinelFlow Phase P10."""

import hashlib
import json
from typing import Dict, List, Literal, Optional
from pydantic import BaseModel, Field


class AgentManifest(BaseModel):
    """Immutable, versioned capability declaration for an agent in the fixed roster."""
    name: str = Field(..., description="Canonical agent name")
    version: str = Field(..., description="Semantic version string")
    autonomy_level: Literal["A1", "A2", "A3", "A4", "A5"] = Field(
        "A1", description="Autonomy tier (All agents are A1-A2: Investigate, Plan, or Generate Candidate)"
    )
    provider: str = Field("google", description="Model provider ('google' or 'deterministic')")
    model: str = Field("gemini-3.5-flash", description="Configured target model")
    triggers: List[str] = Field(default_factory=list, description="Supported trigger event types")
    allowed_tools: List[str] = Field(default_factory=list, description="Allowlist of tool identifiers")
    denied_capabilities: List[str] = Field(
        default_factory=lambda: [
            "artifact.release",
            "incident.approve",
            "ledger.mutate",
            "database.raw_sql",
            "system.shell",
            "agent.create_dynamic",
        ],
        description="Explicitly prohibited capabilities",
    )
    max_turns: int = Field(5, le=5, description="Maximum conversation turns")
    max_tool_calls: int = Field(10, le=10, description="Maximum tool invocations per turn")
    timeout_seconds: float = Field(15.0, le=30.0, description="Execution timeout ceiling")
    memory_read: bool = Field(False, description="Read access to persistent memory")
    memory_write: bool = Field(False, description="Write access to persistent memory (False across all agents)")
    output_schema_name: str = Field(..., description="Name of the canonical Pydantic output schema")
    data_classifications: List[str] = Field(
        default_factory=lambda: ["METADATA_ONLY", "REDACTED_FINDINGS"],
        description="Allowed data classifications in output",
    )
    guardrail_required: bool = Field(True, description="Whether Model Armor guardrail screening is required")
    guardrail_provider: str = Field("google_model_armor", description="Configured guardrail provider")
    guardrail_template_ref: str = Field(
        "projects/telos-agent/locations/us-central1/templates/sentinelflow-guardrail-template",
        description="Model Armor template resource path",
    )

    @property
    def manifest_hash(self) -> str:
        """Deterministic SHA-256 hash of the canonical manifest JSON."""
        serialized = self.model_dump_json()
        return hashlib.sha256(serialized.encode("utf-8")).hexdigest()


# ---------------------------------------------------------------------------
# FIXED AGENT ROSTER (Immutable registry of permitted P10 agents)
# ---------------------------------------------------------------------------

FIXED_AGENT_ROSTER: Dict[str, AgentManifest] = {
    "IncidentCommanderAgent": AgentManifest(
        name="IncidentCommanderAgent",
        version="1.0.0",
        autonomy_level="A1",
        model="gemini-3.5-flash",
        triggers=[
            "ARTIFACT_QUARANTINED",
            "EXPECTED_FILE_MISSING",
            "SLA_AT_RISK",
            "CONNECTOR_FAILURE",
            "RETURN_RECEIVED",
        ],
        allowed_tools=[
            "incident.get",
            "workflow.get",
            "artifact.metadata.get",
            "memory.retrieve",
            "memory.profile.get",
        ],
        memory_read=True,
        max_turns=5,
        max_tool_calls=5,
        timeout_seconds=15.0,
        output_schema_name="CommanderPlan",
    ),
    "DiagnosisAgent": AgentManifest(
        name="DiagnosisAgent",
        version="1.0.0",
        autonomy_level="A1",
        model="gemini-3.5-flash",
        triggers=["ARTIFACT_QUARANTINED", "VALIDATION_FAILURE"],
        allowed_tools=[
            "incident.get",
            "validation.findings.list_redacted",
            "artifact.metadata.get",
            "workflow.get",
            "memory.retrieve",
        ],
        memory_read=True,
        max_turns=5,
        max_tool_calls=10,
        timeout_seconds=15.0,
        output_schema_name="DiagnosisOutput",
    ),
    "PolicySLAAgent": AgentManifest(
        name="PolicySLAAgent",
        version="1.0.0",
        autonomy_level="A1",
        model="gemini-3.5-flash",
        triggers=["ARTIFACT_QUARANTINED", "SLA_AT_RISK"],
        allowed_tools=[
            "incident.get",
            "workflow.get",
            "artifact.metadata.get",
            "memory.profile.get",
        ],
        memory_read=True,
        max_turns=5,
        max_tool_calls=5,
        timeout_seconds=15.0,
        output_schema_name="PolicySLAOutput",
    ),
    "RemediationAgent": AgentManifest(
        name="RemediationAgent",
        version="1.0.0",
        autonomy_level="A2",
        model="gemini-3.5-flash",
        triggers=["ARTIFACT_QUARANTINED", "READY_FOR_REMEDIATION"],
        allowed_tools=[
            "incident.get",
            "validation.findings.list_redacted",
            "artifact.metadata.get",
            "workflow.get",
            "memory.retrieve",
        ],
        memory_read=True,
        denied_capabilities=[
            "artifact.release",
            "incident.approve",
            "ledger.mutate",
            "database.raw_sql",
            "system.shell",
            "agent.create_dynamic",
            "artifact.write_direct",
        ],
        max_turns=5,
        max_tool_calls=10,
        timeout_seconds=15.0,
        output_schema_name="RemediationPlan",
    ),
    "VerifierAgent": AgentManifest(
        name="VerifierAgent",
        version="1.0.0",
        autonomy_level="A1",
        model="gemini-3.5-flash",
        triggers=[
            "CANDIDATE_PREPARED",
            "AWAITING_VERIFICATION",
            "VERIFY_CANDIDATE",
        ],
        allowed_tools=[
            "incident.get",
            "validation.findings.list_redacted",
            "artifact.metadata.get",
            "workflow.get",
            "verification.result.get",
        ],
        memory_read=False,
        denied_capabilities=[
            "artifact.release",
            "incident.approve",
            "ledger.mutate",
            "database.raw_sql",
            "system.shell",
            "agent.create_dynamic",
            "artifact.write_direct",
            "remediation.candidate.create",
        ],
        max_turns=5,
        max_tool_calls=10,
        timeout_seconds=15.0,
        output_schema_name="CriticAssessment",
    ),
    "MemoryAgent": AgentManifest(
        name="MemoryAgent",
        version="1.0.0",
        autonomy_level="A1",
        model="gemini-3.5-flash",
        triggers=[
            "MEMORY_QUERY_REQUESTED",
            "MEMORY_PROFILE_REQUESTED",
            "INCIDENT_ANALYSIS",
        ],
        allowed_tools=[
            "incident.get",
            "workflow.get",
            "artifact.metadata.get",
            "memory.retrieve",
            "memory.profile.get",
        ],
        memory_read=True,
        memory_write=False,
        denied_capabilities=[
            "artifact.release",
            "incident.approve",
            "ledger.mutate",
            "database.raw_sql",
            "system.shell",
            "agent.create_dynamic",
            "artifact.write_direct",
            "remediation.candidate.create",
            "memory.write_direct",
            "evidence.mint_authoritative",
            "source.validate_authoritative",
            "policy.override",
            "candidate.verify",
        ],
        max_turns=5,
        max_tool_calls=10,
        timeout_seconds=15.0,
        output_schema_name="AdvisoryMemoryContext",
    ),
    "ReturnRiskAgent": AgentManifest(
        name="ReturnRiskAgent",
        version="1.0.0",
        autonomy_level="A1",
        model="gemini-3.5-flash",
        triggers=[
            "RETURN_EVENT_OBSERVED",
            "RETURN_RISK_ANALYSIS",
            "RETURN_SURGE_DETECTED",
        ],
        allowed_tools=[
            "incident.get",
            "workflow.get",
            "memory.retrieve",
            "returnrisk.result.get",
        ],
        denied_capabilities=[
            "artifact.release",
            "incident.approve",
            "ledger.mutate",
            "database.raw_sql",
            "system.shell",
            "agent.create_dynamic",
            "artifact.write_direct",
            "remediation.candidate.create",
            "memory.write_direct",
            "evidence.mint_authoritative",
            "source.validate_authoritative",
            "policy.override",
            "candidate.verify",
        ],
        memory_read=True,
        memory_write=False,
        max_turns=5,
        max_tool_calls=10,
        timeout_seconds=15.0,
        output_schema_name="ReturnRiskAssessment",
    ),
}


def validate_agent_roster_membership(agent_name: str) -> AgentManifest:
    """Validates that an agent name belongs strictly to the fixed immutable roster."""
    if agent_name not in FIXED_AGENT_ROSTER:
        raise ValueError(
            f"Agent '{agent_name}' is not in the fixed agent roster: {list(FIXED_AGENT_ROSTER.keys())}"
        )
    return FIXED_AGENT_ROSTER[agent_name]
