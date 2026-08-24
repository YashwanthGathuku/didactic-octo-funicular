"""Canonical DiagnosisAgent Structured Contracts for SentinelFlow (SGACA P05).

Invariants:
1. Autonomous state mutations are strictly prohibited; statement is mandatory.
2. Returned evidence references must resolve strictly to authorized evidence items.
3. Calibrated qualitative confidence: LOW, MEDIUM, or HIGH.
"""

from __future__ import annotations

from typing import Any, Dict, List, Literal, Optional
from pydantic import BaseModel, Field


class DiagnosisHypothesis(BaseModel):
    """Structured root cause hypothesis with explicit evidence grounding."""

    hypothesis_id: str = Field(description="Unique hypothesis identifier, e.g. HYP-1")
    description: str = Field(description="Clear explanation of the probable root cause")
    evidence_refs: List[str] = Field(
        default_factory=list,
        description="Explicit list of authorized evidence IDs (FINDING-*, RUNBOOK-*, METRIC-*, EVID-*)",
    )
    confidence: Literal["LOW", "MEDIUM", "HIGH"] = Field(
        description="Calibrated qualitative confidence based strictly on available evidence"
    )
    status: str = Field(
        default="PROPOSED",
        description="Lifecycle status of hypothesis (e.g. PROPOSED, CONFIRMED, DISPUTED)",
    )


class DiagnosisOutput(BaseModel):
    """Canonical structured diagnosis output schema emitted by DiagnosisAgent."""

    schema_version: Literal["1.0"] = "1.0"
    classification: str = Field(description="High-level incident failure taxonomy classification")
    summary: str = Field(description="Concise summary of observed anomaly and diagnostic reasoning")
    hypotheses: List[DiagnosisHypothesis] = Field(
        default_factory=list,
        description="Ranked root cause hypotheses supported by authorized evidence",
    )
    affected_records: List[str] = Field(
        default_factory=list,
        description="Identifiers or line references of affected records (sanitized/redacted)",
    )
    evidence_refs: List[str] = Field(
        default_factory=list,
        description="All authorized evidence identifiers referenced across the diagnosis",
    )
    unknowns: List[str] = Field(
        default_factory=list,
        description="Missing information or questions required to increase diagnostic certainty",
    )
    recommended_checks: List[str] = Field(
        default_factory=list,
        description="Concrete, read-only operational verification steps for human operators",
    )
    remediation_eligibility: bool = Field(
        default=False,
        description="Whether this incident is eligible for derived artifact correction",
    )
    escalation_required: bool = Field(
        default=False,
        description="Whether this incident requires Tier-2/supervisor escalation",
    )
    statement: str = Field(
        default="The AI incident analyst operates in a read-only capacity and has made no system state changes.",
        description="Mandatory READ_ONLY_EXECUTION_DISCLOSURE for operator UX transparency (security proven by Tool Gateway and Policy Engine)",
    )


class AuditMetadata(BaseModel):
    """Audit provenance and execution metadata for model invocations."""

    model: str = Field(
        ..., description="Model identifier, e.g. gemini-3.5-flash or deterministic-baseline"
    )
    provider: str = Field(
        ...,
        description="Provider name: 'google' for live model or 'deterministic' for in-tree rules",
    )
    prompt_version: str = "1.0.0"
    schema_version: str = "1.0.0"
    latency_ms: float = 0.0
    token_usage: Dict[str, int] = Field(
        default_factory=lambda: {"prompt_tokens": 0, "completion_tokens": 0, "total_tokens": 0}
    )
    estimated_cost_usd: float = 0.0
    execution_source: Literal["LIVE_GEMINI", "DETERMINISTIC_FALLBACK", "NOT_RUN"] = (
        "DETERMINISTIC_FALLBACK"
    )
    adk_version: Optional[str] = None
    genai_version: Optional[str] = None
    request_id: Optional[str] = None
    input_hash: Optional[str] = None
    output_hash: Optional[str] = None
    grounding_verdict: str = "VERIFIED"


class DiagnosisRunRequest(BaseModel):
    """Execution request containing server-injected AgentContextEnvelope."""

    envelope: Dict[str, Any] = Field(..., description="Validated AgentContextEnvelope dictionary")


class DiagnosisRunResponse(BaseModel):
    """Top-level response returned by AI Tier to Go Gateway bridge."""

    workflow_id: str
    incident_id: int
    tenant_id: str
    status: Literal[
        "SUCCESS",
        "UNAVAILABLE",
        "PROVIDER_UNAVAILABLE",
        "POLICY_DENIED",
        "GROUNDING_VIOLATION",
        "FAILED",
    ]
    output: Optional[DiagnosisOutput] = None
    audit: AuditMetadata
    error: Optional[Dict[str, Any]] = None
