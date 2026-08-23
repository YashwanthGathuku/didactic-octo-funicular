"""Unit and Conformance Tests for SentinelFlow P10 Memory Providers and Ranking."""

from datetime import datetime, timezone, timedelta
import pytest

from contracts.memory import (
    MemoryEventEnvelope,
    MemoryQuery,
    PartnerOperationalProfile,
    sanitize_text,
)
from memory.mock_provider import MockManagedMemoryProvider
from memory.ranking import DeterministicMemoryRanker


def test_data_minimization_sanitizer():
    """Verify raw NACHA records, account numbers, and routing numbers are redacted."""
    raw_text = (
        "Partner transmitted file with routing number 121000358 and account 123456789012. "
        "NACHA line: 6221210003581234567890          00000500000918273645JOHN DOE              0121000350000001"
    )
    sanitized = sanitize_text(raw_text)
    assert "121000358" not in sanitized
    assert "123456789012" not in sanitized
    assert "[ROUTING_REDACTED]" in sanitized
    assert "[ACCOUNT_REDACTED]" in sanitized
    assert "[NACHA_RECORD_REDACTED]" in sanitized


def test_memory_event_envelope_hashing():
    """Verify deterministic RFC-style canonical hashing on MemoryEventEnvelope."""
    evt = MemoryEventEnvelope(
        event_id="EVT-001",
        tenant_scope_token="TENANT-A",
        memory_topic="INCIDENT_PATTERN",
        subject_ref="PARTNER-01",
        sanitized_fact="Account field trailing space anomaly remediated by padding strip.",
        source_refs=["INCIDENT-101", "FINDING-202"],
    )
    assert evt.event_hash != ""
    assert len(evt.event_hash) == 64


def test_mock_provider_ingest_and_retrieve():
    """Verify ingestion, tenant isolation, and bounded retrieval in MockManagedMemoryProvider."""
    provider = MockManagedMemoryProvider()

    # Ingest 3 events for Tenant A
    for i in range(1, 4):
        evt = MemoryEventEnvelope(
            event_id=f"EVT-A-00{i}",
            tenant_scope_token="TENANT-A",
            memory_topic="REMEDIATION_RESULT",
            subject_ref="PARTNER-EAST",
            sanitized_fact=f"Remediation pattern {i} succeeded for batch header.",
            source_refs=[f"VERIF-00{i}"],
        )
        res = provider.ingest_event(evt)
        assert res.success is True

    # Ingest 1 event for Tenant B
    evt_b = MemoryEventEnvelope(
        event_id="EVT-B-001",
        tenant_scope_token="TENANT-B",
        memory_topic="REMEDIATION_RESULT",
        subject_ref="PARTNER-WEST",
        sanitized_fact="Tenant B private remediation result.",
        source_refs=["VERIF-B-001"],
    )
    provider.ingest_event(evt_b)

    # Query Tenant A
    q_a = MemoryQuery(
        tenant_scope_token="TENANT-A",
        subject_ref="PARTNER-EAST",
        limit=5,
    )
    hits_a = provider.retrieve_memories(q_a)
    assert len(hits_a) == 3
    assert all(h.memory_id.startswith("EVT-A-") for h in hits_a)

    # Query Tenant B
    q_b = MemoryQuery(
        tenant_scope_token="TENANT-B",
        subject_ref="PARTNER-WEST",
        limit=5,
    )
    hits_b = provider.retrieve_memories(q_b)
    assert len(hits_b) == 1
    assert hits_b[0].memory_id == "EVT-B-001"


def test_ranking_determinism_and_weights():
    """Verify deterministic ranking composite score and tie-breaking."""
    now = datetime.now(timezone.utc)
    old_time = (now - timedelta(days=60)).isoformat()
    new_time = now.isoformat()

    evt_old = MemoryEventEnvelope(
        event_id="EVT-OLD",
        tenant_scope_token="TENANT-A",
        memory_topic="INCIDENT_PATTERN",
        subject_ref="PARTNER-01",
        sanitized_fact="Immediate origin mismatch resolution.",
        source_refs=["FINDING-01"],
        occurred_at=old_time,
        metadata={"verification_level": "OPERATOR_CONFIRMED", "confidence": 0.9},
    )

    evt_new = MemoryEventEnvelope(
        event_id="EVT-NEW",
        tenant_scope_token="TENANT-A",
        memory_topic="INCIDENT_PATTERN",
        subject_ref="PARTNER-01",
        sanitized_fact="Immediate origin mismatch resolution.",
        source_refs=["FINDING-01"],
        occurred_at=new_time,
        metadata={"verification_level": "OPERATOR_CONFIRMED", "confidence": 0.9},
    )

    q = MemoryQuery(
        tenant_scope_token="TENANT-A",
        subject_ref="PARTNER-01",
        query_text="immediate origin mismatch",
        limit=5,
    )

    hits = DeterministicMemoryRanker.rank_events([evt_old, evt_new], q, now=now)
    assert len(hits) == 2
    # Newer event must score higher due to recency decay
    assert hits[0].memory_id == "EVT-NEW"
    assert hits[0].aggregate_ranking_score >= hits[1].aggregate_ranking_score


def test_mock_provider_fault_injection():
    """Verify all fault injection modes in MockManagedMemoryProvider."""
    provider = MockManagedMemoryProvider()

    # 1. TIMEOUT
    provider.set_fault("TIMEOUT")
    evt = MemoryEventEnvelope(
        event_id="EVT-TO",
        tenant_scope_token="TENANT-A",
        memory_topic="INCIDENT_PATTERN",
        subject_ref="PARTNER-01",
        sanitized_fact="Fact text",
    )
    res = provider.ingest_event(evt)
    assert res.success is False
    assert res.error_code == "TIMEOUT"

    # 2. UNAVAILABLE
    provider.set_fault("UNAVAILABLE")
    res_un = provider.ingest_event(evt)
    assert res_un.success is False
    assert res_un.error_code == "SERVICE_UNAVAILABLE"
    health = provider.health_check()
    assert health.status == "UNHEALTHY"

    # 3. POISONED_MEMORIES
    provider.set_fault("POISONED_MEMORIES")
    q = MemoryQuery(tenant_scope_token="TENANT-A", limit=5)
    hits = provider.retrieve_memories(q)
    assert len(hits) == 1
    assert "Ignore previous rules" in hits[0].fact_summary

    # 4. CROSS_TENANT
    provider.set_fault("CROSS_TENANT")
    hits_cross = provider.retrieve_memories(q)
    # The ranker's tenant check must discard foreign tenant event
    assert len(hits_cross) == 0


def test_partner_profile_retrieval():
    """Verify partner operational profile storage and retrieval."""
    provider = MockManagedMemoryProvider()
    profile = PartnerOperationalProfile(
        partner_ref="PARTNER-EAST",
        tenant_scope_token="TENANT-A",
        recurring_verified_issue_codes=["R01", "R03"],
        successful_verified_remediation_types=["RECALCULATE_BATCH_CONTROL"],
        typical_internal_sla_context={"cutoff_hour": 17},
        unresolved_pattern_flags=["WINDOW_MARGIN_NARROW"],
        source_ref_summary=["INCIDENT-101", "INCIDENT-102"],
        confidence_level="HIGH",
    )
    provider.set_profile(profile)

    loaded = provider.get_profile("PARTNER-EAST", "TENANT-A")
    assert loaded is not None
    assert loaded.partner_ref == "PARTNER-EAST"
    assert "R01" in loaded.recurring_verified_issue_codes

    # Wrong tenant returns None
    assert provider.get_profile("PARTNER-EAST", "TENANT-B") is None
