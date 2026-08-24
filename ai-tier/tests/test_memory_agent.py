"""Unit Tests for SentinelFlow P10 MemoryAgent."""

from agents.memory_agent import MemoryAgent
from contracts.manifests import FIXED_AGENT_ROSTER
from contracts.memory import (
    AdvisoryMemoryContext,
    MemoryEventEnvelope,
    PartnerOperationalProfile,
)
from memory.mock_provider import MockManagedMemoryProvider


def test_memory_agent_manifest_conformance():
    """Verify MemoryAgent adheres strictly to fixed manifest parameters."""
    manifest = FIXED_AGENT_ROSTER.get("MemoryAgent")
    assert manifest is not None
    assert manifest.name == "MemoryAgent"
    assert manifest.autonomy_level == "A1"
    assert manifest.memory_read is True
    assert manifest.memory_write is False
    assert "artifact.release" in manifest.denied_capabilities
    assert "incident.approve" in manifest.denied_capabilities
    assert "memory.write_direct" in manifest.denied_capabilities


def test_memory_agent_assemble_advisory_context():
    """Verify MemoryAgent bounded assembly and citation generation."""
    provider = MockManagedMemoryProvider()

    # Seed 6 memories (should be bounded to max 5)
    for i in range(1, 7):
        evt = MemoryEventEnvelope(
            event_id=f"EVT-MEM-{i:03d}",
            tenant_scope_token="TENANT-P10",
            memory_topic="INCIDENT_PATTERN",
            subject_ref="PARTNER-RECURRENT",
            sanitized_fact=f"Historical incident pattern {i} observation.",
            source_refs=[f"INCIDENT-{i:03d}"],
        )
        provider.ingest_event(evt)

    profile = PartnerOperationalProfile(
        partner_ref="PARTNER-RECURRENT",
        tenant_scope_token="TENANT-P10",
        recurring_verified_issue_codes=["R01"],
    )
    provider.set_profile(profile)

    agent = MemoryAgent(memory_provider=provider)
    ctx = agent.assemble_advisory_context(
        tenant_scope_token="TENANT-P10",
        subject_ref="PARTNER-RECURRENT",
        partner_ref="PARTNER-RECURRENT",
    )

    assert isinstance(ctx, AdvisoryMemoryContext)
    # Bounded retrieval limit of 5
    assert len(ctx.retrieved_hits) == 5
    assert ctx.partner_profile is not None
    assert ctx.partner_profile.partner_ref == "PARTNER-RECURRENT"
    # Citations generated
    assert len(ctx.memory_evidence_refs) > 0
    assert any(r.startswith("MEM-HIT-") for r in ctx.memory_evidence_refs)
    assert any(r.startswith("MEM-PROFILE-") for r in ctx.memory_evidence_refs)
    # Advisory disclaimer present
    assert "ADVISORY ONLY" in ctx.advisory_disclaimer
