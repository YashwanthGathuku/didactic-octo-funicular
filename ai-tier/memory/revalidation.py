"""Source Revalidation Engine for SentinelFlow P10 Memory Context."""

from __future__ import annotations

import logging
from datetime import datetime, timezone
from typing import Dict, List, Literal, Optional, Set
from pydantic import BaseModel, Field

from contracts.memory import AdvisoryMemoryContext, MemoryHit, PartnerOperationalProfile

logger = logging.getLogger("sentinel.memory.revalidation")

RevalidationStatus = Literal[
    "AUTHORITATIVE_VERIFIED",
    "UNVERIFIED_MEMORY",
    "STALE_EXPIRED",
    "TAMPERED_REJECTED",
    "CROSS_TENANT_REJECTED",
]


class MemoryRevalidationReport(BaseModel):
    """Detailed audit report of memory context revalidation."""

    overall_status: RevalidationStatus
    tenant_scope_token: str
    revalidated_hits: List[MemoryHit] = Field(default_factory=list)
    rejected_hits: List[Dict[str, str]] = Field(default_factory=list)
    partner_profile_verified: bool = False
    rejection_reasons: List[str] = Field(default_factory=list)
    audit_timestamp: str = Field(default_factory=lambda: datetime.now(timezone.utc).isoformat())


class MemoryRevalidator:
    """Validates memory integrity, provenance, and freshness before prompt injection."""

    MAX_MEMORY_AGE_DAYS = 90

    @classmethod
    def revalidate(
        cls,
        context: AdvisoryMemoryContext,
        tenant_scope_token: str,
        authorized_evidence_set: Optional[Set[str]] = None,
        now: Optional[datetime] = None,
    ) -> MemoryRevalidationReport:
        """Revalidates an AdvisoryMemoryContext against grounding and freshness invariants."""
        current_time = now or datetime.now(timezone.utc)
        revalidated: List[MemoryHit] = []
        rejected: List[Dict[str, str]] = []
        reasons: List[str] = []

        for hit in context.retrieved_hits:
            # 1. Freshness Check
            try:
                dt = datetime.fromisoformat(hit.occurred_at.replace("Z", "+00:00"))
                age_days = (current_time - dt).total_seconds() / 86400.0
                if age_days > cls.MAX_MEMORY_AGE_DAYS:
                    rejected.append({"memory_id": hit.memory_id, "reason": "STALE_EXPIRED"})
                    reasons.append(f"Memory {hit.memory_id} is {age_days:.1f} days old (exceeds {cls.MAX_MEMORY_AGE_DAYS}d limit)")
                    continue
            except Exception:
                rejected.append({"memory_id": hit.memory_id, "reason": "INVALID_TIMESTAMP"})
                reasons.append(f"Memory {hit.memory_id} has invalid timestamp format")
                continue

            # 2. Source Grounding Check (if authorized_evidence_set is provided)
            if authorized_evidence_set:
                missing_sources = [s for s in hit.source_refs if s not in authorized_evidence_set and not s.startswith("INCIDENT-") and not s.startswith("RB-")]
                if missing_sources:
                    rejected.append({"memory_id": hit.memory_id, "reason": "UNGROUNDED_SOURCE"})
                    reasons.append(f"Memory {hit.memory_id} references ungrounded source refs: {missing_sources}")
                    continue

            # 3. Minimum Confidence Cutoff
            if hit.confidence_score < 0.50:
                rejected.append({"memory_id": hit.memory_id, "reason": "LOW_CONFIDENCE"})
                reasons.append(f"Memory {hit.memory_id} confidence score {hit.confidence_score} below 0.50 threshold")
                continue

            revalidated.append(hit)

        # 4. Profile Revalidation
        profile_verified = False
        if context.partner_profile:
            if context.partner_profile.tenant_scope_token != tenant_scope_token:
                reasons.append(f"Partner profile tenant {context.partner_profile.tenant_scope_token} does not match scope {tenant_scope_token}")
            else:
                profile_verified = True

        # Determine overall status
        if rejected and not revalidated:
            status = "TAMPERED_REJECTED" if any(r["reason"] == "UNGROUNDED_SOURCE" for r in rejected) else "STALE_EXPIRED"
        elif rejected and revalidated:
            status = "UNVERIFIED_MEMORY"
        else:
            status = "AUTHORITATIVE_VERIFIED"

        return MemoryRevalidationReport(
            overall_status=status,
            tenant_scope_token=tenant_scope_token,
            revalidated_hits=revalidated,
            rejected_hits=rejected,
            partner_profile_verified=profile_verified,
            rejection_reasons=reasons,
        )
