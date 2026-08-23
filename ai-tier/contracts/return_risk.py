"""Canonical Return Risk Contracts for SentinelFlow Phase P12.

The ReturnRiskAgent operates with Autonomy Level A1 (Advisory Specialist).
Formal Invariants:
1. Autonomy Level A1: Advisory Only. Zero authority to approve, reject, release, or mutate financial artifacts.
2. Deterministic Dominance: Risk score (0-100) and risk tier are computed deterministically by the Go engine.
   Model prose cannot alter or override the authoritative score or tier.
3. Disjoint Grounding: Claimed evidence_refs MUST be a subset of AuthorizedEvidenceRefs.
   Claimed memory_refs MUST be a subset of AuthorizedMemoryRefs.
   Evidence and memory references are strictly disjoint.
4. Input Minimization: Financial routing and account numbers are sanitized before prompt assembly.
"""

from __future__ import annotations

from enum import Enum
import hashlib
import json
from typing import Any, Dict, List, Literal, Optional, Union
from pydantic import BaseModel, Field, field_validator, model_validator

RETURN_RISK_NON_AUTHORITY_STATEMENT = (
    "This return risk assessment is advisory only and has no authority to approve, "
    "reject, release, or mutate financial artifacts."
)


class ReturnRiskTier(str, Enum):
    """Authoritative deterministic return risk tier."""
    LOW = "LOW"
    MEDIUM = "MEDIUM"
    HIGH = "HIGH"
    SEVERE = "SEVERE"


class FeatureContributionItem(BaseModel):
    """Feature contribution attribution from deterministic risk scoring engine."""
    feature_name: str = Field(..., description="Name of the model or heuristic feature")
    raw_value: Any = Field(..., description="Observed raw or transformed value for this feature")
    contribution_score: float = Field(
        ..., description="Directional impact score or contribution (-100.0 to 100.0)"
    )
    description: str = Field(..., description="Human-readable explanation of this feature's contribution")


class ReturnRiskContextEnvelope(BaseModel):
    """Structured context envelope passed from Go Gateway to ReturnRiskAgent."""
    schema_version: Literal["1.0"] = "1.0"
    tenant_scope: str = Field(..., min_length=1, description="Authenticated tenant scope identifier")
    return_event_ref: str = Field(..., min_length=1, description="Unique return event identifier (e.g. RET-EVT-2026-001)")
    return_code: str = Field(..., min_length=3, max_length=4, description="Standard NACHA return reason code (e.g. R01, R08, R10)")
    return_code_label: str = Field(..., min_length=1, description="Standard NACHA return code label (e.g. INSUFFICIENT_FUNDS)")
    risk_score: float = Field(..., ge=0.0, le=100.0, description="Authoritative deterministic risk score (0-100)")
    risk_tier: ReturnRiskTier = Field(..., description="Authoritative deterministic risk tier")
    contributions: List[FeatureContributionItem] = Field(
        default_factory=list,
        description="Feature contributions and principal risk drivers from deterministic scoring model",
    )
    authorized_evidence_refs: List[str] = Field(
        default_factory=list,
        description="Monotonic list of authorized evidence citations (RET-EVT-*, INCIDENT-*, EVID-*)",
    )
    authorized_memory_refs: List[str] = Field(
        default_factory=list,
        description="Monotonic list of authorized advisory memory citations (MEM-HIT-*, MEM-PROFILE-*)",
    )
    partner_ref: Optional[str] = Field(None, description="Counterparty / Partner identifier")
    historical_summary: Optional[str] = Field(None, description="Summary of past return patterns and rate benchmarks")
    sla_cutoff_context: Optional[Dict[str, Any]] = Field(default_factory=dict, description="SLA window and cutoff timing metadata")
    workflow_id: Optional[str] = Field(default=None, description="Associated workflow identifier")
    incident_id: Optional[int] = Field(default=None, description="Associated incident ID if triggered from an incident")
    correlation_id: Optional[str] = Field(default=None, description="Distributed correlation trace ID")


class ReturnRiskAssessment(BaseModel):
    """Structured evidence-grounded return risk assessment produced by ReturnRiskAgent."""
    schema_version: Literal["1.0"] = "1.0"
    return_event_ref: str = Field(..., description="Referenced return event ID")
    return_code: str = Field(..., description="NACHA return reason code")
    risk_score: float = Field(..., ge=0.0, le=100.0, description="Authoritative deterministic risk score (0-100)")
    risk_tier: ReturnRiskTier = Field(..., description="Authoritative deterministic risk tier")
    summary: str = Field(..., description="Human-readable synthesis of return risk posture")
    principal_drivers: List[str] = Field(
        default_factory=list, description="Ranked list of principal feature drivers contributing to risk"
    )
    historical_patterns: List[str] = Field(
        default_factory=list, description="Historical behavioral correlations and pattern insights"
    )
    operational_recommendations: List[str] = Field(
        default_factory=list, description="Actionable advisory recommendations for human operators"
    )
    evidence_refs: List[str] = Field(
        default_factory=list, description="Authorized evidence citations (RET-EVT-*, INCIDENT-*, EVID-*)"
    )
    memory_refs: List[str] = Field(
        default_factory=list, description="Authorized advisory memory citations (MEM-HIT-*, MEM-PROFILE-*)"
    )
    uncertainties: List[str] = Field(
        default_factory=list, description="Identified unknowns or missing telemetry"
    )
    escalation_recommended: bool = Field(
        False, description="Whether immediate human supervisor review or hold is recommended"
    )
    non_authority_statement: str = Field(
        RETURN_RISK_NON_AUTHORITY_STATEMENT,
        description="Mandatory non-authority declaration statement",
    )
    statement: str = Field(
        RETURN_RISK_NON_AUTHORITY_STATEMENT,
        description="Mandatory non-authority declaration statement",
    )
    input_context_hash: str = Field(default="", description="SHA-256 hash of input context prompt")
    output_hash: str = Field(default="", description="SHA-256 hash of serialized assessment output")
    manifest_hash: str = Field(default="", description="SHA-256 hash of ReturnRiskAgent manifest")
    execution_source: Literal[
        "LIVE_GEMINI", "DETERMINISTIC_FALLBACK", "LOCAL_ADK_DETERMINISTIC", "LOCAL_ADK"
    ] = Field("LOCAL_ADK_DETERMINISTIC", description="Execution provenance source")

    @field_validator("non_authority_statement", "statement", mode="after")
    @classmethod
    def enforce_non_authority_statement(cls, v: str) -> str:
        return RETURN_RISK_NON_AUTHORITY_STATEMENT
