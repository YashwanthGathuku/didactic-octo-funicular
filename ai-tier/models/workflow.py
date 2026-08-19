"""Agent Workflow domain models for Google ADK and Python services.

Strict separation of concerns:
AgentWorkflowState is strictly decoupled from ArtifactState.
Private model chain-of-thought is never persisted.
"""

from datetime import datetime
from enum import Enum
from typing import Any, Dict, List, Optional
from pydantic import BaseModel, Field


class AgentWorkflowState(str, Enum):
    PENDING = "PENDING"
    CONTEXT_BUILDING = "CONTEXT_BUILDING"
    INVESTIGATING = "INVESTIGATING"
    PLANNING = "PLANNING"
    REMEDIATING = "REMEDIATING"
    VALIDATING_CANDIDATE = "VALIDATING_CANDIDATE"
    RETRYING = "RETRYING"
    VERIFIED = "VERIFIED"
    UNRESOLVED = "UNRESOLVED"
    HUMAN_REVIEW = "HUMAN_REVIEW"
    COMPLETED = "COMPLETED"
    AGENT_UNAVAILABLE = "AGENT_UNAVAILABLE"
    POLICY_DENIED = "POLICY_DENIED"
    BUDGET_EXHAUSTED = "BUDGET_EXHAUSTED"
    CANCELLED = "CANCELLED"
    FAILED = "FAILED"


# State machine allowed transitions
LEGAL_TRANSITIONS: Dict[AgentWorkflowState, List[AgentWorkflowState]] = {
    AgentWorkflowState.PENDING: [
        AgentWorkflowState.CONTEXT_BUILDING,
        AgentWorkflowState.AGENT_UNAVAILABLE,
        AgentWorkflowState.POLICY_DENIED,
        AgentWorkflowState.CANCELLED,
    ],
    AgentWorkflowState.CONTEXT_BUILDING: [
        AgentWorkflowState.INVESTIGATING,
        AgentWorkflowState.AGENT_UNAVAILABLE,
        AgentWorkflowState.POLICY_DENIED,
        AgentWorkflowState.BUDGET_EXHAUSTED,
        AgentWorkflowState.FAILED,
        AgentWorkflowState.CANCELLED,
    ],
    AgentWorkflowState.INVESTIGATING: [
        AgentWorkflowState.PLANNING,
        AgentWorkflowState.UNRESOLVED,
        AgentWorkflowState.HUMAN_REVIEW,
        AgentWorkflowState.AGENT_UNAVAILABLE,
        AgentWorkflowState.BUDGET_EXHAUSTED,
        AgentWorkflowState.FAILED,
        AgentWorkflowState.CANCELLED,
    ],
    AgentWorkflowState.PLANNING: [
        AgentWorkflowState.REMEDIATING,
        AgentWorkflowState.HUMAN_REVIEW,
        AgentWorkflowState.UNRESOLVED,
        AgentWorkflowState.POLICY_DENIED,
        AgentWorkflowState.BUDGET_EXHAUSTED,
        AgentWorkflowState.FAILED,
        AgentWorkflowState.CANCELLED,
    ],
    AgentWorkflowState.REMEDIATING: [
        AgentWorkflowState.VALIDATING_CANDIDATE,
        AgentWorkflowState.RETRYING,
        AgentWorkflowState.HUMAN_REVIEW,
        AgentWorkflowState.FAILED,
        AgentWorkflowState.CANCELLED,
        AgentWorkflowState.BUDGET_EXHAUSTED,
    ],
    AgentWorkflowState.VALIDATING_CANDIDATE: [
        AgentWorkflowState.VERIFIED,
        AgentWorkflowState.RETRYING,
        AgentWorkflowState.HUMAN_REVIEW,
        AgentWorkflowState.UNRESOLVED,
        AgentWorkflowState.FAILED,
        AgentWorkflowState.CANCELLED,
    ],
    AgentWorkflowState.RETRYING: [
        AgentWorkflowState.CONTEXT_BUILDING,
        AgentWorkflowState.INVESTIGATING,
        AgentWorkflowState.REMEDIATING,
        AgentWorkflowState.BUDGET_EXHAUSTED,
        AgentWorkflowState.FAILED,
        AgentWorkflowState.CANCELLED,
    ],
    AgentWorkflowState.VERIFIED: [
        AgentWorkflowState.HUMAN_REVIEW,
        AgentWorkflowState.COMPLETED,
    ],
    AgentWorkflowState.UNRESOLVED: [
        AgentWorkflowState.HUMAN_REVIEW,
        AgentWorkflowState.COMPLETED,
        AgentWorkflowState.CANCELLED,
    ],
    AgentWorkflowState.HUMAN_REVIEW: [
        AgentWorkflowState.COMPLETED,
        AgentWorkflowState.REMEDIATING,
        AgentWorkflowState.CANCELLED,
        AgentWorkflowState.POLICY_DENIED,
    ],
    AgentWorkflowState.COMPLETED: [],
    AgentWorkflowState.AGENT_UNAVAILABLE: [],
    AgentWorkflowState.POLICY_DENIED: [],
    AgentWorkflowState.BUDGET_EXHAUSTED: [],
    AgentWorkflowState.CANCELLED: [],
    AgentWorkflowState.FAILED: [],
}


def can_transition_workflow(
    from_state: AgentWorkflowState, to_state: AgentWorkflowState
) -> bool:
    if from_state == to_state:
        return True
    return to_state in LEGAL_TRANSITIONS.get(from_state, [])


class AgentWorkflow(BaseModel):
    id: str
    tenant_id: str
    incident_id: int
    artifact_id: int
    artifact_sha256: str
    state: AgentWorkflowState = AgentWorkflowState.PENDING
    agent_name: str = "SentinelCoordinator"
    agent_version: str = "1.0.0"
    workflow_type: str = "TRIAGE_AND_REMEDIATION"
    correlation_id: str
    trace_id: Optional[str] = None
    row_version: int = 1
    error_detail: Optional[str] = None
    created_at: datetime = Field(default_factory=datetime.utcnow)
    updated_at: datetime = Field(default_factory=datetime.utcnow)
    started_at: Optional[datetime] = None
    completed_at: Optional[datetime] = None


class AgentWorkflowEvent(BaseModel):
    id: str
    workflow_id: str
    tenant_id: str
    idempotency_key: str
    event_type: str
    state_from: AgentWorkflowState
    state_to: AgentWorkflowState
    row_version: int
    payload: str
    created_at: datetime = Field(default_factory=datetime.utcnow)


class AgentRun(BaseModel):
    id: str
    workflow_id: str
    tenant_id: str
    agent_name: str
    agent_version: str
    provider: Optional[str] = None
    model_name: Optional[str] = None
    model_version: Optional[str] = None
    status: str = "RUNNING"
    input_tokens: int = 0
    output_tokens: int = 0
    latency_ms: int = 0
    estimated_cost_microusd: int = 0
    pricing_version: Optional[str] = None
    error_message: Optional[str] = None
    started_at: datetime = Field(default_factory=datetime.utcnow)
    completed_at: Optional[datetime] = None


class AgentStep(BaseModel):
    id: str
    run_id: str
    workflow_id: str
    tenant_id: str
    step_number: int
    step_type: str = "CONTEXT_BUILD"  # CONTEXT_BUILD, MODEL_INVOCATION, DECISION, TOOL_REQUEST, TOOL_RESULT, HANDOFF, POLICY_CHECK, VALIDATION, VERIFICATION, HUMAN_REVIEW
    state_from: AgentWorkflowState
    state_to: AgentWorkflowState
    decision_payload: Optional[str] = None
    authorized_evidence_refs: List[str] = Field(default_factory=list)
    step_status: str = "COMPLETED"
    step_hash: Optional[str] = None
    latency_ms: int = 0
    created_at: datetime = Field(default_factory=datetime.utcnow)


class AgentToolCall(BaseModel):
    id: str
    step_id: str
    workflow_id: str
    tenant_id: str
    tool_name: str
    tool_scope: str = "READ"  # READ, WRITE
    input_redacted: str
    output_redacted: str
    is_error: bool = False
    latency_ms: int = 0
    executed_at: datetime = Field(default_factory=datetime.utcnow)


class VerificationAttestation(BaseModel):
    id: str
    workflow_id: str
    tenant_id: str
    verifier_agent: str = "VerifierAgent"
    candidate_artifact_id: Optional[int] = None
    candidate_sha256: str
    findings_count: int = 0
    blocking_findings_count: int = 0
    status: str = "CONFIRMED"  # CONFIRMED, DISPUTED, PARTIAL
    attestation_digest: str
    created_at: datetime = Field(default_factory=datetime.utcnow)
