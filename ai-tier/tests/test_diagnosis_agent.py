"""Unit tests for DiagnosisAgent (SGACA P05.5)."""

import os
import pytest
from agents.diagnosis import DiagnosisAgent
from contracts.diagnosis import DiagnosisOutput, DiagnosisRunResponse
from models.envelope import AgentBudget, AgentContextEnvelope, RedactedFindingItem


def test_diagnosis_agent_batch_hash_mismatch():
    """Verifies DiagnosisAgent analysis on batch entry hash mismatch."""
    envelope = AgentContextEnvelope(
        tenant_id="TENANT-ACME",
        incident_id=201,
        artifact_id=501,
        artifact_sha256="e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855",
        correlation_id="corr-hash-201",
        findings=[
            RedactedFindingItem(
                id="FINDING-001",
                code="0802",
                severity="BLOCKING",
                description="Batch entry hash mismatch in batch 001",
                line_number=14,
                expected_value="0012345678",
                actual_value="0012345679",
            )
        ],
        available_runbooks=["RB-01", "RB-05"],
    )

    agent = DiagnosisAgent()
    resp = agent.run(envelope)

    assert isinstance(resp, DiagnosisRunResponse)
    assert resp.status == "SUCCESS"
    assert resp.output is not None
    assert resp.output.classification == "ENTRY_HASH_ACCUMULATOR_MISMATCH"
    assert len(resp.output.hypotheses) > 0
    assert resp.output.hypotheses[0].confidence == "HIGH"
    assert "FINDING-001" in resp.output.evidence_refs
    assert "RUNBOOK-RB-05" in resp.output.evidence_refs
    assert resp.output.remediation_eligibility is True
    assert resp.output.statement == "The AI incident analyst operates in a read-only capacity and has made no system state changes."
    assert resp.audit.execution_source in ["DETERMINISTIC_FALLBACK", "LIVE_GEMINI"]
    assert resp.audit.provider in ["deterministic", "google"]


def test_diagnosis_agent_invalid_routing():
    """Verifies DiagnosisAgent analysis on invalid RDFI routing number."""
    envelope = AgentContextEnvelope(
        tenant_id="TENANT-ACME",
        incident_id=202,
        artifact_id=502,
        correlation_id="corr-route-202",
        findings=[
            RedactedFindingItem(
                id="FINDING-002",
                code="0602",
                severity="BLOCKING",
                description="Receiving DFI routing number failed Mod-10 checksum",
                line_number=28,
            )
        ],
        available_runbooks=["RB-05"],
    )

    agent = DiagnosisAgent()
    resp = agent.run(envelope)

    assert resp.status == "SUCCESS"
    assert resp.output is not None
    assert resp.output.classification == "INVALID_RDFI_ROUTING_NUMBER"
    assert resp.output.escalation_required is True
    assert "FINDING-002" in resp.output.evidence_refs


def test_diagnosis_agent_empty_findings_calibrated_uncertainty():
    """Verifies that 0 findings yields LOW confidence and populates unknowns."""
    envelope = AgentContextEnvelope(
        tenant_id="TENANT-ACME",
        incident_id=203,
        artifact_id=503,
        correlation_id="corr-empty-203",
        findings=[],
        available_runbooks=["RB-01"],
    )

    agent = DiagnosisAgent()
    resp = agent.run(envelope)

    assert resp.status == "SUCCESS"
    assert resp.output is not None
    assert len(resp.output.hypotheses) > 0
    assert resp.output.hypotheses[0].confidence == "LOW"
    assert len(resp.output.unknowns) > 0
    assert resp.output.statement == "The AI incident analyst operates in a read-only capacity and has made no system state changes."


def test_diagnosis_agent_mandatory_read_only_invariant():
    """Safety Invariant: The statement must always be present and immutable."""
    envelope = AgentContextEnvelope(
        tenant_id="TENANT-ACME",
        incident_id=204,
        artifact_id=504,
        correlation_id="corr-safety-204",
        findings=[
            RedactedFindingItem(
                id="FINDING-003",
                code="0001",
                severity="WARNING",
                description="Fixed-width record length truncation detected",
                line_number=40,
            )
        ],
    )

    agent = DiagnosisAgent()
    resp = agent.run(envelope)

    assert resp.status == "SUCCESS"
    assert resp.output is not None
    assert resp.output.statement == "The AI incident analyst operates in a read-only capacity and has made no system state changes."


def test_diagnosis_agent_live_mode_without_credentials_fails_fast(monkeypatch):
    """Section 2: Live mode without credentials must fail fast with PROVIDER_UNAVAILABLE rather than silently falling back."""
    monkeypatch.setenv("SENTINEL_AI_MODE", "live")
    monkeypatch.delenv("GOOGLE_API_KEY", raising=False)
    monkeypatch.delenv("GEMINI_API_KEY", raising=False)

    envelope = AgentContextEnvelope(
        tenant_id="TENANT-ACME",
        incident_id=205,
        artifact_id=505,
        correlation_id="corr-live-fail-205",
        findings=[],
    )

    agent = DiagnosisAgent()
    resp = agent.run(envelope)

    assert resp.status == "PROVIDER_UNAVAILABLE"
    assert resp.output is None
    assert resp.audit.execution_source == "LIVE_GEMINI"
    assert resp.audit.provider == "google"
    assert resp.error is not None
    assert resp.error["code"] == "MISSING_CREDENTIALS"
