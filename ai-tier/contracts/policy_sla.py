"""Contract definitions for PolicySLAAgent (SGACA Phase P06)."""

from typing import List, Literal, Optional
from pydantic import BaseModel, Field


class PolicySLAOutput(BaseModel):
    """Structured, evidence-grounded interpretation of policy and SLA context.

    Formal Invariant: PolicySLAAgentOpinion != PolicyDecision.
    The agent explains and interprets authoritative policy and SLA rules;
    it cannot override deterministic PolicyEngine decisions (ALLOW, DENY, REQUIRE_HUMAN).
    """

    schema_version: Literal["1.0"] = "1.0"
    authoritative_policy_decision_refs: List[str] = Field(
        default_factory=list,
        description="References to verified deterministic PolicyDecision IDs",
    )
    policy_summary: str = Field(
        ...,
        description="Human-readable explanation of the governing policy constraints",
    )
    active_obligations: List[str] = Field(
        default_factory=list,
        description="Active policy obligations (e.g. CANDIDATE_ONLY_REMEDIATION, REVALIDATION_REQUIRED)",
    )
    active_prohibitions: List[str] = Field(
        default_factory=list,
        description="Active policy prohibitions (e.g. PROHIBIT_DIRECT_ORIGINAL_MUTATION)",
    )
    sla_status: Literal["ON_TRACK", "AT_RISK", "BREACHED", "UNKNOWN"] = Field(
        "ON_TRACK",
        description="Current SLA delivery status based on expectation timetable",
    )
    cutoff_type: Literal["INSTITUTION_INTERNAL", "PARTNER_CONTRACT", "NETWORK_RULE", "UNKNOWN"] = (
        Field(
            "INSTITUTION_INTERNAL",
            description="Provenance of the governing cutoff window (must not conflate internal with network)",
        )
    )
    time_remaining_seconds: Optional[int] = Field(
        None,
        description="Seconds remaining until cutoff deadline, if deterministically known",
    )
    applicable_contract_refs: List[str] = Field(
        default_factory=list,
        description="References to partner contracts or SLA schedule IDs",
    )
    risk_factors: List[str] = Field(
        default_factory=list,
        description="Identified operational or regulatory risk factors",
    )
    unknowns: List[str] = Field(
        default_factory=list,
        description="Missing contractual or timetable data required for higher certainty",
    )
    escalation_required: bool = Field(
        False,
        description="Whether policy or SLA constraints dictate immediate operator escalation",
    )
    evidence_refs: List[str] = Field(
        default_factory=list,
        description="Authorized evidence IDs cited (must belong to AuthorizedEvidenceSet)",
    )
    statement: str = Field(
        default="The AI Policy/SLA analyst operates in a read-only capacity and has made no system state changes.",
        description="Mandatory READ_ONLY_EXECUTION_DISCLOSURE for operator UX transparency",
    )
