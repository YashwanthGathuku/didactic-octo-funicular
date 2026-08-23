"""Unit Tests for SentinelFlow P10.5 Memory Context Filtering."""

from datetime import datetime, timezone, timedelta
import pytest

from contracts.memory import (
    AdvisoryMemoryContext,
    MemoryHit,
    PartnerOperationalProfile,
)
from memory.revalidation import MemoryRevalidator


def test_revalidator_clean_pass():
    """Verify clean memory context passes with ADVISORY_CONTEXT_READY status."""
    now = datetime.now(timezone.utc)
    hit = MemoryHit(
        memory_id="MEM-001",
        memory_topic="INCIDENT_PATTERN",
        subject_ref="PARTNER-01",
        fact_summary="Origin identification discrepancy remediated successfully.",
        source_refs=["FINDING-101"],
        confidence_score=0.90,
        relevance_score=0.85,
        recency_score=0.95,
        source_strength_score=0.80,
        subject_match_score=1.00,
        aggregate_ranking_score=0.88,
        occurred_at=now.isoformat(),
        ingested_at=now.isoformat(),
        provenance_hash="0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef",
    )
    profile = PartnerOperationalProfile(
        partner_ref="PARTNER-01",
        tenant_scope_token="TENANT-A",
        recurring_verified_issue_codes=["R01"],
    )
    ctx = AdvisoryMemoryContext(
        retrieved_hits=[hit],
        partner_profile=profile,
    )

    report = MemoryRevalidator.revalidate(
        ctx,
        tenant_scope_token="TENANT-A",
        authorized_evidence_set={"FINDING-101"},
        now=now,
    )
    assert report.overall_status == "ADVISORY_CONTEXT_READY"
    assert len(report.revalidated_hits) == 1
    assert report.partner_profile_verified is True


def test_revalidator_rejects_stale_memory():
    """Verify memories older than retrieval window are filtered as STALE_EXPIRED."""
    now = datetime.now(timezone.utc)
    old_time = (now - timedelta(days=95)).isoformat()

    stale_hit = MemoryHit(
        memory_id="MEM-STALE",
        memory_topic="INCIDENT_PATTERN",
        subject_ref="PARTNER-01",
        fact_summary="Stale observation.",
        source_refs=["FINDING-01"],
        confidence_score=0.80,
        relevance_score=0.80,
        recency_score=0.10,
        source_strength_score=0.70,
        subject_match_score=1.00,
        aggregate_ranking_score=0.60,
        occurred_at=old_time,
        ingested_at=old_time,
        provenance_hash="0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef",
    )
    ctx = AdvisoryMemoryContext(retrieved_hits=[stale_hit])

    report = MemoryRevalidator.revalidate(
        ctx,
        tenant_scope_token="TENANT-A",
        now=now,
    )
    assert report.overall_status == "STALE_EXPIRED"
    assert len(report.revalidated_hits) == 0
    assert len(report.rejected_hits) == 1
    assert report.rejected_hits[0]["reason"] == "STALE_EXPIRED"


def test_revalidator_rejects_ungrounded_source():
    """Verify memories referencing unauthorized sources are flagged as UNGROUNDED_SOURCE."""
    now = datetime.now(timezone.utc)
    hit = MemoryHit(
        memory_id="MEM-UNGROUNDED",
        memory_topic="INCIDENT_PATTERN",
        subject_ref="PARTNER-01",
        fact_summary="Ungrounded claim.",
        source_refs=["FABRICATED-CITATION-999"],
        confidence_score=0.90,
        relevance_score=0.80,
        recency_score=0.90,
        source_strength_score=0.70,
        subject_match_score=1.00,
        aggregate_ranking_score=0.80,
        occurred_at=now.isoformat(),
        ingested_at=now.isoformat(),
        provenance_hash="0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef",
    )
    ctx = AdvisoryMemoryContext(retrieved_hits=[hit])

    report = MemoryRevalidator.revalidate(
        ctx,
        tenant_scope_token="TENANT-A",
        authorized_evidence_set={"FINDING-101"},
        now=now,
    )
    assert report.overall_status == "TAMPERED_REJECTED"
    assert len(report.revalidated_hits) == 0
    assert report.rejected_hits[0]["reason"] == "UNGROUNDED_SOURCE"


def test_revalidator_filters_low_relevance():
    """Verify memories with relevance < 0.20 are filtered out for context budgeting."""
    now = datetime.now(timezone.utc)
    hit = MemoryHit(
        memory_id="MEM-LOW-REL",
        memory_topic="INCIDENT_PATTERN",
        subject_ref="PARTNER-01",
        fact_summary="Low relevance note.",
        source_refs=["FINDING-01"],
        confidence_score=0.90,
        relevance_score=0.10,
        recency_score=0.90,
        source_strength_score=0.70,
        subject_match_score=1.00,
        aggregate_ranking_score=0.50,
        occurred_at=now.isoformat(),
        ingested_at=now.isoformat(),
        provenance_hash="0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef",
    )
    ctx = AdvisoryMemoryContext(retrieved_hits=[hit])

    report = MemoryRevalidator.revalidate(
        ctx,
        tenant_scope_token="TENANT-A",
        now=now,
    )
    assert len(report.revalidated_hits) == 0
    assert report.rejected_hits[0]["reason"] == "LOW_RELEVANCE"
