"""Unit tests for Evidence Grounding Guardrail (SGACA P05)."""

import pytest
from contracts.diagnosis import DiagnosisHypothesis, DiagnosisOutput
from guardrails.evidence import (
    AuthorizedEvidenceSet,
    EvidenceGroundingVerifier,
    GroundingVerdict,
    GroundingViolationError,
)


def test_authorized_evidence_set_initialization():
    """Verifies that AuthorizedEvidenceSet is correctly populated from envelope."""
    envelope = {
        "tenant_id": "TENANT-001",
        "incident_id": 101,
        "findings": [
            {"id": "FINDING-001", "code": "NACHA_ENTRY_HASH_MISMATCH"},
            {"id": "FINDING-002", "code": "RDFI_ROUTING_INVALID"},
        ],
        "available_runbooks": ["RB-01", "RB-05"],
        "telemetry_summary": {"parse_rate": 100000},
        "authorized_evidence_refs": ["EVID-CUSTOM-01"],
    }
    evidence_set = AuthorizedEvidenceSet.from_envelope(envelope)
    refs = evidence_set.references

    assert "FINDING-001" in refs
    assert "NACHA_ENTRY_HASH_MISMATCH" in refs
    assert "FINDING-002" in refs
    assert "RDFI_ROUTING_INVALID" in refs
    assert "RB-01" in refs
    assert "RUNBOOK-RB-01" in refs
    assert "RB-05" in refs
    assert "RUNBOOK-RB-05" in refs
    assert "METRIC-parse_rate" in refs
    assert "EVID-CUSTOM-01" in refs
    assert "FINDING-999999" not in refs


def test_authorized_evidence_set_tool_expansion():
    """Verifies that evidence set expands monotonically only with tool results."""
    evidence_set = AuthorizedEvidenceSet({"FINDING-001"})

    evidence_set.expand_from_tool_result("incident.get", {"incident_id": "101", "artifact_id": "501"})
    assert evidence_set.contains("INCIDENT-101")
    assert evidence_set.contains("ARTIFACT-501")

    evidence_set.expand_from_tool_result(
        "validation.findings.list_redacted",
        {"finding_code": "0802", "rule_citation": "RULE-ACH-0802"},
    )
    assert evidence_set.contains("0802")
    assert evidence_set.contains("RULE-ACH-0802")


def test_evidence_grounding_verifier_valid_citations():
    """Verifies that a 100% grounded diagnosis passes verification."""
    evidence_set = AuthorizedEvidenceSet({"FINDING-001", "RUNBOOK-RB-05"})

    output = DiagnosisOutput(
        classification="ENTRY_HASH_MISMATCH",
        summary="Entry hash accumulator mismatch",
        hypotheses=[
            DiagnosisHypothesis(
                hypothesis_id="HYP-1",
                description="Entry hash mismatch in batch control",
                evidence_refs=["FINDING-001", "RUNBOOK-RB-05"],
                confidence="HIGH",
            )
        ],
        evidence_refs=["FINDING-001", "RUNBOOK-RB-05"],
    )

    result = EvidenceGroundingVerifier.verify(output, evidence_set, strict=True)
    assert result.is_valid is True
    assert result.verdict == GroundingVerdict.VERIFIED
    assert len(result.unauthorized_citations) == 0


def test_evidence_grounding_verifier_strict_rejection_of_fake_citations():
    """Verifies fail-closed rejection on fabricated evidence like FINDING-999999."""
    evidence_set = AuthorizedEvidenceSet({"FINDING-001", "RUNBOOK-RB-05"})

    output = DiagnosisOutput(
        classification="ENTRY_HASH_MISMATCH",
        summary="Entry hash accumulator mismatch",
        hypotheses=[
            DiagnosisHypothesis(
                hypothesis_id="HYP-1",
                description="Entry hash mismatch in batch control",
                evidence_refs=["FINDING-001", "FINDING-999999"],  # FABRICATED
                confidence="HIGH",
            )
        ],
        evidence_refs=["FINDING-001", "EVID-FAKE-01"],  # FABRICATED
    )

    result = EvidenceGroundingVerifier.verify(output, evidence_set, strict=True)
    assert result.is_valid is False
    assert result.verdict == GroundingVerdict.UNGROUNDED_REJECTED
    assert "FINDING-999999" in result.unauthorized_citations
    assert "EVID-FAKE-01" in result.unauthorized_citations


def test_evidence_grounding_verifier_non_strict_pruning():
    """Verifies that non-strict mode prunes invalid citations and demotes confidence to LOW."""
    evidence_set = AuthorizedEvidenceSet({"FINDING-001"})

    output = DiagnosisOutput(
        classification="TEST",
        summary="Testing pruning",
        hypotheses=[
            DiagnosisHypothesis(
                hypothesis_id="HYP-1",
                description="Test hypothesis",
                evidence_refs=["FINDING-001", "FINDING-999999"],
                confidence="HIGH",
            )
        ],
        evidence_refs=["FINDING-001", "FINDING-999999"],
    )

    result = EvidenceGroundingVerifier.verify(output, evidence_set, strict=False)
    assert result.is_valid is True
    assert result.verdict == GroundingVerdict.PARTIALLY_GROUNDED
    assert result.remediated_output is not None
    assert result.remediated_output.hypotheses[0].confidence == "LOW"
    assert result.remediated_output.hypotheses[0].evidence_refs == ["FINDING-001"]
    assert result.remediated_output.evidence_refs == ["FINDING-001"]
    assert any("PRUNED_UNAUTHORIZED_REFERENCE" in u for u in result.remediated_output.unknowns)
