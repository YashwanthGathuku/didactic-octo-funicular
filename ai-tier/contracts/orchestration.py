"""Multi-Agent Orchestration Contracts for SentinelFlow Phase P06.5."""

from typing import Any, Dict, Generic, List, Literal, Optional, TypeVar
from pydantic import BaseModel, Field, model_validator

from .diagnosis import DiagnosisOutput
from .manifests import FIXED_AGENT_ROSTER, validate_agent_roster_membership
from .policy_sla import PolicySLAOutput

T = TypeVar("T")


class AgentTriggerEvent(BaseModel):
    """Normalized trigger event initiating an autonomous investigation workflow."""
    event_id: str = Field(..., min_length=1, description="Unique event identifier")
    event_type: Literal[
        "ARTIFACT_QUARANTINED",
        "EXPECTED_FILE_MISSING",
        "SLA_AT_RISK",
        "CONNECTOR_FAILURE",
        "RETURN_RECEIVED",
        "RETURN_RATE_THRESHOLD_APPROACHING",
    ] = Field(..., description="Normalized trigger classification")
    tenant_id: str = Field(..., min_length=1, description="Authenticated tenant ID (infrastructure-controlled)")
    subject_refs: List[str] = Field(default_factory=list, description="Referenced entity IDs")
    artifact_ref: Optional[str] = None
    incident_ref: Optional[str] = None
    occurred_at: str = Field(..., description="ISO 8601 timestamp")
    correlation_id: str = Field(..., min_length=1, description="Distributed correlation ID")
    event_hash: str = Field(..., min_length=64, max_length=64, description="SHA-256 digest of event payload")


class AgentHandoffEnvelope(BaseModel):
    """Structured handoff envelope passed between Commander and Specialist agents.
    
    Messages are structured DATA, not executable prompt instructions.
    Tenant identity is strictly infrastructure-injected.
    """
    schema_version: Literal["1.0"] = "1.0"
    workflow_id: str = Field(..., min_length=1)
    tenant_id: str = Field(..., min_length=1)
    source_agent: str = Field(..., description="Name of delegating agent (e.g. IncidentCommanderAgent)")
    target_agent: str = Field(..., description="Name of delegated specialist (e.g. DiagnosisAgent)")
    trigger_event_id: str = Field(default="")
    incident_id: int = Field(..., gt=0)
    artifact_id: int = Field(0, ge=0)
    artifact_sha256: str = Field(default="0000000000000000000000000000000000000000000000000000000000000000")
    policy_bundle_hash: str = Field(default="")
    authorized_evidence_refs: List[str] = Field(default_factory=list)
    allowed_tools: List[str] = Field(default_factory=list)
    execution_budget: Dict[str, Any] = Field(default_factory=dict)
    correlation_id: str = Field(..., min_length=1)
    trace_id: str = Field(default="")
    delegation_depth: int = Field(1, ge=1, le=2, description="Delegation depth limit (max 2)")

    @model_validator(mode="after")
    def validate_handoff_invariants(self) -> "AgentHandoffEnvelope":
        validate_agent_roster_membership(self.target_agent)
        if self.delegation_depth > 2:
            raise ValueError(f"Delegation depth {self.delegation_depth} exceeds maximum limit of 2")
        if self.source_agent == self.target_agent:
            raise ValueError(f"Recursive self-delegation detected: {self.source_agent} -> {self.target_agent}")
        return self


class CommanderPlan(BaseModel):
    """Structured investigation and delegation plan produced by IncidentCommanderAgent."""
    schema_version: Literal["1.0"] = "1.0"
    workflow_class: Literal[
        "QUARANTINE_INVESTIGATION",
        "SLA_RISK_REMEDIATION",
        "FORMAT_ERROR",
        "UNCLASSIFIED",
    ] = Field("QUARANTINE_INVESTIGATION")
    selected_specialists: List[str] = Field(
        default_factory=lambda: ["DiagnosisAgent", "PolicySLAAgent"],
        description="List of specialist agent names to invoke (validated against fixed roster)",
    )
    reason_codes: List[str] = Field(default_factory=list)
    evidence_requirements: List[str] = Field(default_factory=list)
    parallelizable: bool = Field(True, description="Whether specialists may run in parallel")
    remediation_eligible: bool = Field(False, description="Initial assessment of remediation feasibility")
    human_attention_required: bool = Field(False, description="Whether immediate human escalation is necessary")
    policy_bundle_hash: str = Field(default="", description="Binding hash of governing policy bundle when plan was formed")
    artifact_sha256: str = Field(default="", description="Binding hash of quarantined artifact when plan was formed")
    next_stage: Literal[
        "READY_FOR_REMEDIATION",
        "HUMAN_AUTHORIZATION_REQUIRED",
        "POLICY_BLOCKED",
        "RETRY_INVESTIGATION",
        "CLOSE",
    ] = Field("READY_FOR_REMEDIATION")

    @model_validator(mode="after")
    def validate_roster_and_anti_hallucination(self) -> "CommanderPlan":
        for spec in self.selected_specialists:
            if spec not in FIXED_AGENT_ROSTER:
                raise ValueError(
                    f"Commander plan requested unauthorized or hallucinated specialist '{spec}'. "
                    f"Must belong to fixed roster: {list(FIXED_AGENT_ROSTER.keys())}"
                )
        return self


class SpecialistResult(BaseModel, Generic[T]):
    """Generic wrapper capturing execution provenance, timing, output, and protected binding hashes."""
    agent_name: str
    agent_version: str = "1.0.0"
    manifest_hash: str = Field(default="", description="SHA-256 of canonical AgentManifest")
    input_context_hash: str = Field(default="", description="SHA-256 of serialized input prompt/context")
    artifact_sha256: str = Field(default="", description="SHA-256 of target artifact")
    policy_bundle_hash: str = Field(default="", description="SHA-256 of governing policy bundle")
    authorized_evidence_set_hash: str = Field(default="", description="SHA-256 of authorized evidence set")
    tool_manifest_hash: str = Field(default="", description="SHA-256 of allowed tools schema")
    execution_source: Literal["LIVE_GEMINI", "DETERMINISTIC_FALLBACK", "NOT_RUN"] = "DETERMINISTIC_FALLBACK"
    status: Literal["SUCCESS", "FAILED", "TIMEOUT", "POLICY_DENIED", "GROUNDING_VIOLATION", "STALE"]
    output: Optional[T] = None
    evidence_refs: List[str] = Field(default_factory=list)
    tool_invocation_refs: List[str] = Field(default_factory=list)
    model_provenance: Dict[str, Any] = Field(default_factory=dict)
    input_hash: Optional[str] = None
    output_hash: Optional[str] = None
    latency_ms: float = 0.0
    error: Optional[Dict[str, Any]] = None


class WorkflowAuditMetadata(BaseModel):
    """Aggregate workflow-level execution audit and billing telemetry."""
    workflow_id: str
    execution_source: Literal["LIVE_GEMINI", "DETERMINISTIC_FALLBACK", "MIXED_NOT_LIVE", "NOT_RUN"] = "DETERMINISTIC_FALLBACK"
    total_latency_ms: float = 0.0
    total_model_calls: int = 0
    total_tool_calls: int = 0
    total_tokens: int = 0
    estimated_cost_usd: float = 0.0
    agent_policy_disagreement_count: int = 0
    trace_id: str = ""


class CommanderSynthesis(BaseModel):
    """Authoritative synthesized investigation outcome produced by IncidentCommanderAgent.
    
    Section 7 Distinct Outcomes:
    - READY_FOR_REMEDIATION: Policy engine allows + diagnosis is eligible + obligations satisfiable
    - HUMAN_AUTHORIZATION_REQUIRED: Policy decision requires human review/waiver
    - POLICY_BLOCKED: Policy decision is DENY; cannot be overridden by human click or model prose
    - PARTIAL_SPECIALIST_FAILURE: One or more parallel specialists failed/timed out
    - UNRESOLVED: Investigation unable to conclude (e.g. TOCTOU stale context or all specialists failed)
    """
    schema_version: Literal["1.0"] = "1.0"
    workflow_id: str
    incident_id: int
    tenant_id: str
    plan: CommanderPlan
    diagnosis_result: Optional[SpecialistResult[DiagnosisOutput]] = None
    policy_sla_result: Optional[SpecialistResult[PolicySLAOutput]] = None
    synthesis_summary: str
    outcome: Literal[
        "READY_FOR_REMEDIATION",
        "HUMAN_AUTHORIZATION_REQUIRED",
        "POLICY_BLOCKED",
        "PARTIAL_SPECIALIST_FAILURE",
        "UNRESOLVED",
    ]
    human_attention_required: bool = Field(
        default=False,
        description="Flag indicating operational review needed (attached for investigation, NOT to override DENY)",
    )
    evidence_refs: List[str] = Field(
        default_factory=list,
        description="Unified evidence references (grounded in union of initial + specialist evidence)",
    )
    statement: str = Field(
        default="The AI incident commander operates in a read-only capacity and has made no system state changes.",
        description="Mandatory READ_ONLY_EXECUTION_DISCLOSURE for operator UX transparency",
    )
    audit: WorkflowAuditMetadata


class AgentStageRequest(BaseModel):
    """Typed stage execution request from Go Control Plane to Python ADK Tier."""
    stage_type: Literal[
        "COMMANDER_PLAN",
        "PARALLEL_SPECIALISTS",
        "COMMANDER_SYNTHESIS",
        "REMEDIATION_PLAN",
        "VERIFIER_CRITIC",
        "STAGE_VERIFIER_CRITIC",
    ]
    workflow_id: str
    tenant_id: str
    incident_id: int
    artifact_id: int = 0
    artifact_sha256: str = "0000000000000000000000000000000000000000000000000000000000000000"
    candidate_artifact_id: Optional[int] = 0
    candidate_ref: Optional[str] = None
    candidate_sha256: Optional[str] = None
    derivation_hash: Optional[str] = None
    remediation_plan_hash: Optional[str] = None
    policy_bundle_hash: str = "default/1"
    authorized_evidence_refs: List[str] = Field(default_factory=list)
    findings: List[Any] = Field(default_factory=list)
    unresolved_findings: List[str] = Field(default_factory=list)
    verification_checks: List[Dict[str, Any]] = Field(default_factory=list)
    semantic_diff: Dict[str, Any] = Field(default_factory=dict)
    remediation_plan: Optional[Dict[str, Any]] = None
    attempt_number: Optional[int] = 1
    available_runbooks: List[str] = Field(default_factory=list)
    correlation_id: str = ""
    trace_id: Optional[str] = None
    plan: Optional[Dict[str, Any]] = None
    diagnosis_result: Optional[Dict[str, Any]] = None
    policy_sla_result: Optional[Dict[str, Any]] = None
    authoritative_policy_decision: Optional[Dict[str, Any]] = None
    sla_context: Optional[Dict[str, Any]] = None
    max_elapsed_seconds: float = 30.0


class AgentStageResponse(BaseModel):
    """Structured response returned by Python ADK Tier to Go Control Plane."""
    stage_type: Literal[
        "COMMANDER_PLAN",
        "PARALLEL_SPECIALISTS",
        "COMMANDER_SYNTHESIS",
        "REMEDIATION_PLAN",
        "VERIFIER_CRITIC",
        "STAGE_VERIFIER_CRITIC",
    ]
    status: Literal["SUCCESS", "FAILED"] = "SUCCESS"
    workflow_id: str
    plan: Optional[Dict[str, Any]] = None
    diagnosis_result: Optional[Dict[str, Any]] = None
    policy_sla_result: Optional[Dict[str, Any]] = None
    remediation_plan: Optional[Dict[str, Any]] = None
    critic_assessment: Optional[Dict[str, Any]] = None
    synthesis: Optional[Dict[str, Any]] = None
    outcome: Optional[str] = None
    evidence_refs: List[str] = Field(default_factory=list)
    latency_ms: float = 0.0
    input_tokens: int = 0
    output_tokens: int = 0
    execution_source: str = "LOCAL_ADK_DETERMINISTIC"
    error_detail: Optional[str] = None
