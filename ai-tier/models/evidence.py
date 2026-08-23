"""Typed evidence envelope models for agent fleet communication.

These Pydantic models define the contract between the Go gateway and the
Python ADK agent fleet. They enforce:
- No raw financial data ever reaches an agent
- All findings are pre-redacted
- Explicit redaction attestation is always present
- Tenant context is always included
"""

from __future__ import annotations

from typing import Literal, Optional
from pydantic import BaseModel, Field, model_validator


class RedactedFinding(BaseModel):
    """A single validation finding with raw data stripped."""
    id: str
    code: str
    severity: Literal["INFO", "WARNING", "BLOCKING"]
    description: str
    rule_version: str
    provenance: str = ""
    line_number: Optional[int] = None
    byte_offset: Optional[int] = None
    field_start: Optional[int] = None
    field_end: Optional[int] = None
    evidence_redacted: Optional[str] = None
    expected_value: Optional[str] = None
    actual_value: Optional[str] = None
    # Explicitly excluded: raw_data, raw_content, full_line, account_number


class EvidenceEnvelope(BaseModel):
    """Typed envelope wrapping all evidence passed to the agent fleet.

    This is the ONLY data structure agents receive. It guarantees that
    raw financial content never reaches an AI model provider.
    """
    envelope_version: Literal["1.0"] = "1.0"
    tenant_id: str
    incident_id: int
    file_id: int
    artifact_sha256: str = ""
    filename: str = "unnamed.ach"
    partner_name: str = ""
    contract_id: Optional[str] = None
    sla_status: str = ""
    findings: list[RedactedFinding] = Field(default_factory=list)
    validation_run_id: Optional[str] = None
    available_runbooks: list[str] = Field(default_factory=list)
    telemetry_summary: dict = Field(default_factory=dict)
    prior_occurrences: int = 0

    # Explicit redaction attestation
    redaction_applied: bool = True
    raw_data_present: bool = False

    @model_validator(mode="after")
    def enforce_no_raw_data(self) -> "EvidenceEnvelope":
        """Hard invariant: raw_data_present must always be False."""
        if self.raw_data_present:
            raise ValueError(
                "EvidenceEnvelope.raw_data_present must be False. "
                "Raw financial data must never reach the agent fleet."
            )
        return self


class AgentResponse(BaseModel):
    """Unified response from the agent fleet coordinator."""
    incident_id: int
    tenant_id: str
    file_id: int

    # Triage output
    severity: str = ""  # P1/P2/P3/P4
    severity_rationale: str = ""

    # Compliance output
    summary: str = ""
    hypotheses: list[dict] = Field(default_factory=list)
    regulatory_citations: list[str] = Field(default_factory=list)
    runbook_passage_ids: list[str] = Field(default_factory=list)

    # Remediation output
    correctable: Optional[bool] = None
    correction_spec: Optional[dict] = None
    derivation_reason: Optional[str] = None

    # Verification output
    verification_status: str = ""  # CONFIRMED / DISPUTED / PARTIAL
    confirmed_findings: list[str] = Field(default_factory=list)
    disputed_findings: list[str] = Field(default_factory=list)

    # Memory output
    similar_incidents: list[dict] = Field(default_factory=list)
    partner_reliability_score: Optional[float] = None
    pattern_detected: bool = False

    # Escalation output
    escalation_level: str = "MONITOR"  # MONITOR / ALERT / ESCALATE / BREACH
    time_to_breach_minutes: Optional[int] = None

    # Common
    recommended_actions: list[str] = Field(default_factory=list)
    missing_evidence: list[str] = Field(default_factory=list)
    statement: str = "The AI incident analyst operates in a read-only capacity and has made no system state changes."

    # Audit
    agents_invoked: list[str] = Field(default_factory=list)
    model: str = "gemini-3.5-flash"
    provider: str = "Google Gemini"
    total_latency_ms: float = 0.0
    total_tokens: int = 0
    model_armor_input_verdict: str = "NOT_SCREENED"
    model_armor_output_verdict: str = "NOT_SCREENED"
