"""Canonical Verification & Critic Contracts for SentinelFlow Phase P08.

The VerifierAgent operates with Autonomy Level A1 (Read-Only Independent Critic).
Independent deterministic re-validation, parent immutability, cryptographic derivation binding,
and policy fresh-binding are strictly verified by the Go Control Plane.
"""

from enum import Enum
from typing import List, Literal, Optional
from pydantic import BaseModel, Field

CRITIC_NON_AUTHORITY_STATEMENT = (
    "The VerifierAgent operates in a read-only critic capacity and has made no system state changes. "
    "Deterministic verification authority is strictly computed by the Go Control Plane."
)


class VerificationOutcome(str, Enum):
    """Deterministic verification outcome states."""

    PASS = "PASS"
    FAIL = "FAIL"
    STALE = "STALE"
    CORRUPTION_DETECTED = "CORRUPTION_DETECTED"
    ERROR = "ERROR"


class CriticAssessmentVerdict(str, Enum):
    """Critic agent qualitative assessment verdict."""

    CONSISTENT = "CONSISTENT"
    CONCERN = "CONCERN"
    INSUFFICIENT_EVIDENCE = "INSUFFICIENT_EVIDENCE"


# Aliases for flexible imports
CriticAssessmentType = CriticAssessmentVerdict


class CriticRiskLevel(str, Enum):
    """Risk severity assessment from critic review."""

    LOW = "LOW"
    MEDIUM = "MEDIUM"
    HIGH = "HIGH"


class CriticRecommendation(str, Enum):
    """Governed routing recommendation from critic assessment."""

    PROCEED_TO_HUMAN_REVIEW = "PROCEED_TO_HUMAN_REVIEW"
    REJECT_CANDIDATE = "REJECT_CANDIDATE"
    REQUEST_REMEDIATION_RETRY = "REQUEST_REMEDIATION_RETRY"
    REQUEST_HUMAN_INVESTIGATION = "REQUEST_HUMAN_INVESTIGATION"
    HUMAN_INVESTIGATION_REQUIRED = "HUMAN_INVESTIGATION_REQUIRED"


class VerificationCheck(BaseModel):
    """Individual deterministic integrity or validation check outcome."""

    check_type: str = Field(..., description="Classification of the check performed")
    passed: bool = Field(..., description="Whether the check passed")
    message: str = Field(..., description="Descriptive result or reason for failure")
    expected_value: Optional[str] = Field(
        None, description="Expected cryptographic or structural value"
    )
    actual_value: Optional[str] = Field(
        None, description="Actual observed cryptographic or structural value"
    )


class CriticContradiction(BaseModel):
    """Specific contradiction or discrepancy detected between remediation claims and verification reality."""

    finding_ref: str = Field(..., description="Referenced finding or check identifier")
    remediation_claim: str = Field(default="", description="Claim made in remediation proposal")
    verification_reality: str = Field(default="", description="Actual observed verification state")
    explanation: Optional[str] = Field(default="", description="Detailed technical explanation")


class SuspiciousChange(BaseModel):
    """Unexpected or non-allowlisted mutation detected in candidate artifact."""

    field_ref: str = Field(..., description="Field or offset path modified")
    operation_type: str = Field(
        "UNAUTHORIZED_FIELD_MUTATION", description="Classification of suspicious mutation"
    )
    rationale: Optional[str] = Field(
        default="", description="Explanation of why modification is suspicious"
    )


class CriticAssessment(BaseModel):
    """Structured independent critic assessment produced by VerifierAgent."""

    schema_version: Literal["1.0"] = "1.0"
    id: Optional[str] = Field(default=None, description="Unique critic assessment identifier")
    verification_id: Optional[str] = Field(
        default=None, description="Bound candidate verification identifier"
    )
    tenant_id: Optional[str] = Field(default=None, description="Authoritative tenant identifier")
    workflow_id: Optional[str] = Field(
        default=None, description="Authoritative workflow identifier"
    )
    candidate_artifact_id: Optional[int] = Field(
        default=None, description="Evaluated candidate artifact ID"
    )
    candidate_ref: Optional[str] = Field(default=None, description="Candidate reference tag")
    agent_name: str = Field("VerifierAgent", description="Canonical agent name")
    agent_version: str = Field("1.0.0", description="Agent version string")
    assessment: CriticAssessmentType = Field(..., description="Qualitative consistency assessment")
    risk_level: CriticRiskLevel = Field(
        ..., description="Evaluated risk level of the proposed candidate"
    )
    recommendation: CriticRecommendation = Field(..., description="Proposed routing recommendation")
    contradictions: List[CriticContradiction] = Field(
        default_factory=list,
        description="Contradictions or discrepancies detected between intent and result",
    )
    suspicious_changes: List[SuspiciousChange] = Field(
        default_factory=list,
        description="Unexpected or non-allowlisted mutations detected in candidate",
    )
    unresolved_findings: List[str] = Field(
        default_factory=list, description="Findings that remain unresolved"
    )
    evidence_refs: List[str] = Field(
        default_factory=list,
        description="Grounded authorized evidence citations supporting the assessment",
    )
    non_authority_statement: str = Field(
        CRITIC_NON_AUTHORITY_STATEMENT, description="Mandatory non-authority declaration statement"
    )
    statement: str = Field(
        CRITIC_NON_AUTHORITY_STATEMENT, description="Mandatory non-authority declaration statement"
    )
    input_context_hash: str = Field(default="", description="SHA-256 hash of critic input context")
    output_hash: str = Field(default="", description="SHA-256 hash of serialized assessment output")
    manifest_hash: str = Field(default="", description="SHA-256 hash of VerifierAgent manifest")
    execution_source: Literal[
        "LIVE_GEMINI", "LOCAL_ADK", "LOCAL_ADK_DETERMINISTIC", "AGENT_UNAVAILABLE"
    ] = Field("LOCAL_ADK_DETERMINISTIC", description="Execution provenance source")


class CandidateVerificationRecord(BaseModel):
    """Immutable verification ledger entry binding candidate, parent, derivation, and critic evidence."""

    schema_version: Literal["1.0"] = "1.0"
    id: str = Field(..., description="Unique verification identifier")
    tenant_id: str = Field(..., description="Authoritative tenant identifier")
    workflow_id: str = Field(..., description="Authoritative workflow identifier")
    candidate_artifact_id: int = Field(..., description="Evaluated candidate artifact ID")
    candidate_sha256: str = Field(..., description="Authoritative SHA-256 hash of candidate bytes")
    parent_artifact_id: int = Field(..., description="Authoritative parent artifact ID")
    parent_sha256: str = Field(..., description="Authoritative SHA-256 hash of parent bytes")
    derivation_id: str = Field(..., description="Artifact derivation ledger record ID")
    derivation_hash: str = Field(
        ..., description="Cryptographic hash binding parent, plan, and candidate"
    )
    remediation_plan_hash: str = Field(
        ..., description="Cryptographic hash of approved remediation plan"
    )
    p07_validation_run_id: str = Field(..., description="Initial P07 candidate revalidation run ID")
    p08_validation_run_id: str = Field(..., description="Independent P08 clean revalidation run ID")
    validator_version: str = Field("1.0.0", description="Validator engine version")
    rulepack_hash: str = Field("nacha-2026-ruleset", description="NACHA rulepack hash")
    policy_bundle_hash: str = Field(..., description="Governing policy bundle hash")
    deterministic_outcome: VerificationOutcome = Field(
        ..., description="Authoritative deterministic verification outcome"
    )
    verification_hash: str = Field(
        ..., description="Authoritative composite SHA-256 verification digest"
    )
    checks: List[VerificationCheck] = Field(
        default_factory=list, description="Detailed individual verification check outcomes"
    )
    critic_assessment: Optional[CriticAssessment] = Field(
        None, description="Independent critic evaluation from VerifierAgent"
    )
    final_routing: Literal[
        "PROCEED_TO_HUMAN_REVIEW",
        "REJECT_CANDIDATE",
        "REQUEST_REMEDIATION_RETRY",
        "HUMAN_INVESTIGATION_REQUIRED",
    ] = Field(
        "PROCEED_TO_HUMAN_REVIEW",
        description="Final governed routing decision after evaluating deterministic dominance and critic risk",
    )
