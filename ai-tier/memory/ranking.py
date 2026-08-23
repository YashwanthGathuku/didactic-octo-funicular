"""Deterministic Ranking Engine for SentinelFlow P10 Memory Retrieval."""

from __future__ import annotations

import math
from datetime import datetime, timezone
from typing import List, Optional
from contracts.memory import MemoryEventEnvelope, MemoryHit, MemoryQuery


class DeterministicMemoryRanker:
    """Deterministic, multi-factor ranking calculator for Memory Bank hits."""

    WEIGHT_SEMANTIC = 0.40
    WEIGHT_RECENCY = 0.20
    WEIGHT_SOURCE = 0.20
    WEIGHT_SUBJECT = 0.20
    HALF_LIFE_DAYS = 30.0

    @classmethod
    def compute_semantic_score(cls, query_text: Optional[str], fact_text: str) -> float:
        """Computes lexical/token-overlap relevance score [0.0, 1.0]."""
        if not query_text or not fact_text:
            return 0.5  # Neutral default when query_text is omitted
        q_tokens = set(query_text.lower().split())
        f_tokens = set(fact_text.lower().split())
        if not q_tokens:
            return 0.5
        intersection = q_tokens.intersection(f_tokens)
        return min(1.0, len(intersection) / len(q_tokens))

    @classmethod
    def compute_recency_score(cls, occurred_at_iso: str, now: Optional[datetime] = None) -> float:
        """Computes exponential half-life decay score [0.0, 1.0]."""
        current_time = now or datetime.now(timezone.utc)
        try:
            occurred_dt = datetime.fromisoformat(occurred_at_iso.replace("Z", "+00:00"))
        except Exception:
            return 0.1
        delta_seconds = max(0.0, (current_time - occurred_dt).total_seconds())
        delta_days = delta_seconds / 86400.0
        decay_constant = math.log(2) / cls.HALF_LIFE_DAYS
        return float(math.exp(-decay_constant * delta_days))

    @classmethod
    def compute_source_strength(cls, source_refs: List[str], metadata: dict) -> float:
        """Computes hierarchical source verification score [0.0, 1.0]."""
        classification = metadata.get("verification_level", "").upper()
        if "HUMAN_SIGNED" in classification or metadata.get("operator_confirmed"):
            return 1.00
        if any(r.startswith("VERIF-") or r.startswith("CRITIC-") for r in source_refs):
            return 0.85
        if any(r.startswith("DIAG-") or r.startswith("POL-") for r in source_refs):
            return 0.70
        if any(r.startswith("FINDING-") or r.startswith("RULE-") for r in source_refs):
            return 0.50
        return 0.30

    @classmethod
    def compute_subject_match(cls, target_subject: Optional[str], event_subject: str) -> float:
        """Computes subject exactness match score [0.0, 1.0]."""
        if not target_subject:
            return 0.5
        t_clean = target_subject.strip().upper()
        e_clean = event_subject.strip().upper()
        if t_clean == e_clean:
            return 1.00
        if t_clean in e_clean or e_clean in t_clean:
            return 0.60
        return 0.10

    @classmethod
    def rank_events(
        cls,
        events: List[MemoryEventEnvelope],
        query: MemoryQuery,
        now: Optional[datetime] = None,
    ) -> List[MemoryHit]:
        """Ranks a list of candidate memory events against a bounded query."""
        scored_hits: List[MemoryHit] = []

        for evt in events:
            # Enforce strict tenant isolation
            if evt.tenant_scope_token != query.tenant_scope_token:
                continue

            # Optional topic filter
            if query.memory_topic and evt.memory_topic != query.memory_topic:
                continue

            s_sem = cls.compute_semantic_score(query.query_text, evt.sanitized_fact)
            s_rec = cls.compute_recency_score(evt.occurred_at, now=now)
            s_src = cls.compute_source_strength(evt.source_refs, evt.metadata)
            s_sub = cls.compute_subject_match(query.subject_ref, evt.subject_ref)

            composite = (
                cls.WEIGHT_SEMANTIC * s_sem
                + cls.WEIGHT_RECENCY * s_rec
                + cls.WEIGHT_SOURCE * s_src
                + cls.WEIGHT_SUBJECT * s_sub
            )
            composite_rounded = round(composite, 4)

            if composite_rounded >= query.min_score_threshold:
                scored_hits.append(
                    MemoryHit(
                        memory_id=evt.event_id,
                        memory_topic=evt.memory_topic,
                        subject_ref=evt.subject_ref,
                        fact_summary=evt.sanitized_fact,
                        source_refs=evt.source_refs,
                        confidence_score=float(evt.metadata.get("confidence", 0.95)),
                        relevance_score=s_sem,
                        recency_score=s_rec,
                        source_strength_score=s_src,
                        subject_match_score=s_sub,
                        aggregate_ranking_score=composite_rounded,
                        occurred_at=evt.occurred_at,
                        ingested_at=evt.metadata.get("ingested_at", evt.occurred_at),
                        provenance_hash=evt.event_hash,
                    )
                )

        # Deterministic 3-tier sort
        # 1. aggregate_ranking_score descending
        # 2. occurred_at descending
        # 3. provenance_hash ascending
        scored_hits.sort(
            key=lambda h: (
                -h.aggregate_ranking_score,
                -datetime.fromisoformat(h.occurred_at.replace("Z", "+00:00")).timestamp(),
                h.provenance_hash,
            )
        )

        return scored_hits[: query.limit]
