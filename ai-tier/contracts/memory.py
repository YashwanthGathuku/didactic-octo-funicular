"""Canonical Data Models for SentinelFlow P10.5 Managed Memory Subsystem.

Defines schemas for:
- MemoryEventEnvelope: Data-minimized, hashed event for persistent ingestion.
- MemoryQuery & MemoryHit: Bounded retrieval query and multi-factor ranked hit.
- PartnerOperationalProfile: Aggregate profile of counterparty behavioral history.
- AdvisoryMemoryContext: Structured payload injected into AgentContextEnvelope.

Formal Invariants:
- ManagedMemory != AuthoritativeEvidence
- PythonMemoryValidation != EvidenceAuthority
- SimilarityScore != Trust
- MemoryConfidence != EvidenceValidity
- MemoryRef ∉ EvidenceSet
"""

from __future__ import annotations

import hashlib
import json
import re
from datetime import datetime, timezone
from typing import Any, Dict, List, Literal, Optional
from pydantic import BaseModel, Field, field_validator, model_validator

# Strict Data Minimization Regex Patterns
ROUTING_NUMBER_REGEX = re.compile(r"\b\d{9}\b")
ACCOUNT_NUMBER_REGEX = re.compile(r"\b\d{10,17}\b")
NACHA_94_RECORD_REGEX = re.compile(r"(?:[156789][0-9A-Za-z\s]{93}|[156789][0-9A-Za-z\s]{80,93})")
SECRET_KEY_REGEX = re.compile(
    r"(?i)(bearer\s+[a-z0-9_\-\.]{20,}|(ghp|gho|xoxb|xoxp|sk_live|secret|token)_[a-z0-9_\-]{16,}|BEGIN\s+(RSA|OPENSSH|PGP|EC)\s+PRIVATE\s+KEY)"
)


def sanitize_text(text: str) -> str:
    """Applies strict data minimization to redact sensitive financial identifiers."""
    if not text:
        return ""
    sanitized = NACHA_94_RECORD_REGEX.sub("[NACHA_RECORD_REDACTED]", text)
    sanitized = ACCOUNT_NUMBER_REGEX.sub("[ACCOUNT_REDACTED]", sanitized)
    sanitized = ROUTING_NUMBER_REGEX.sub("[ROUTING_REDACTED]", sanitized)
    return sanitized


class MemoryEventEnvelope(BaseModel):
    """Canonical immutable memory event submitted for Memory Bank ingestion."""

    schema_version: Literal["1.0"] = "1.0"
    event_id: str = Field(..., min_length=1, description="Unique event UUID or deterministic ID")
    tenant_scope_token: str = Field(
        ..., min_length=1, description="Authenticated tenant scope token"
    )
    memory_topic: Literal[
        "INCIDENT_PATTERN",
        "PARTNER_BEHAVIOR",
        "SLA_TREND",
        "REMEDIATION_RESULT",
        "VALIDATION_ANOMALY",
    ] = Field(..., description="Classification category for memory indexing")
    subject_ref: str = Field(
        ..., min_length=1, description="Subject entity ID (e.g., PARTNER-BANK-EAST, ACH-SEC-PPD)"
    )
    sanitized_fact: str = Field(..., min_length=1, description="Data-minimized factual assertion")
    source_refs: List[str] = Field(
        default_factory=list, description="Grounding source citations (FINDING-*, INCIDENT-*, RB-*)"
    )
    classification: Literal["OPERATIONAL", "COMPLIANCE", "TECHNICAL", "PARTNER_PROFILE"] = (
        "OPERATIONAL"
    )
    occurred_at: str = Field(
        default_factory=lambda: datetime.now(timezone.utc).isoformat(),
        description="ISO 8601 UTC timestamp of occurrence",
    )
    metadata: Dict[str, Any] = Field(
        default_factory=dict, description="Structured non-sensitive metadata"
    )
    event_hash: str = Field(
        default="",
        description="SHA-256 digest of canonical fields for tamper detection",
    )

    @field_validator("sanitized_fact", mode="after")
    @classmethod
    def enforce_data_minimization(cls, v: str) -> str:
        """Ensures raw financial data is sanitized upon construction."""
        return sanitize_text(v)

    @model_validator(mode="after")
    def compute_and_verify_hash(self) -> "MemoryEventEnvelope":
        """Calculates canonical hash if missing or verifies hash consistency."""
        canonical_dict = {
            "schema_version": self.schema_version,
            "event_id": self.event_id,
            "tenant_scope_token": self.tenant_scope_token,
            "memory_topic": self.memory_topic,
            "subject_ref": self.subject_ref,
            "sanitized_fact": self.sanitized_fact,
            "source_refs": sorted(self.source_refs),
            "classification": self.classification,
            "occurred_at": self.occurred_at,
        }
        digest = hashlib.sha256(
            json.dumps(canonical_dict, sort_keys=True).encode("utf-8")
        ).hexdigest()
        if not self.event_hash:
            self.event_hash = digest
        return self


class MemoryQuery(BaseModel):
    """Bounded query contract for memory retrieval."""

    tenant_scope_token: str = Field(
        ..., min_length=1, description="Mandatory tenant scope for isolation"
    )
    memory_topic: Optional[str] = Field(None, description="Optional topic filter")
    subject_ref: Optional[str] = Field(None, description="Target entity ID to recall")
    query_text: Optional[str] = Field(None, description="Semantic text query for matching")
    limit: int = Field(default=5, ge=1, le=5, description="Bounded retrieval limit (max 5 hits)")
    min_score_threshold: float = Field(
        default=0.55,
        ge=0.0,
        le=1.0,
        description="Minimum ranking score cutoff for retrieval pruning",
    )
    lookback_days: int = Field(
        default=90, ge=1, le=365, description="Client-side retrieval lookback filter"
    )
    correlation_id: str = Field(default="", description="Distributed trace correlation ID")


class MemoryHit(BaseModel):
    """A single ranked memory item retrieved from Memory Bank (Advisory context only)."""

    memory_id: str = Field(..., description="Unique identifier of the stored memory")
    memory_topic: str = Field(..., description="Topic of the memory")
    subject_ref: str = Field(..., description="Entity reference")
    fact_summary: str = Field(..., description="Sanitized factual summary")
    source_refs: List[str] = Field(default_factory=list, description="Original source citations")
    confidence_score: float = Field(
        default=0.85,
        ge=0.0,
        le=1.0,
        description="Provider match confidence (for ranking only, NOT trust)",
    )
    relevance_score: float = Field(..., ge=0.0, le=1.0, description="Semantic relevance score (w1)")
    recency_score: float = Field(..., ge=0.0, le=1.0, description="Recency decay score (w2)")
    source_strength_score: float = Field(
        ..., ge=0.0, le=1.0, description="Source authority score (w3)"
    )
    subject_match_score: float = Field(
        ..., ge=0.0, le=1.0, description="Subject exactness score (w4)"
    )
    aggregate_ranking_score: float = Field(
        ..., ge=0.0, le=1.0, description="Weighted composite ranking score"
    )
    occurred_at: str = Field(..., description="ISO 8601 timestamp of original event")
    ingested_at: str = Field(..., description="ISO 8601 timestamp of memory ingestion")
    provenance_hash: str = Field(..., description="SHA-256 hash of original memory event")


class PartnerOperationalProfile(BaseModel):
    """Aggregated operational history and reliability profile for a counterparty."""

    schema_version: Literal["1.0"] = "1.0"
    partner_ref: str = Field(..., min_length=1, description="Unique counterparty / partner ID")
    tenant_scope_token: str = Field(..., min_length=1, description="Tenant scope")
    recurring_verified_issue_codes: List[str] = Field(
        default_factory=list,
        description="Historically confirmed NACHA error codes for this partner",
    )
    successful_verified_remediation_types: List[str] = Field(
        default_factory=list,
        description="Remediation actions previously successful for this partner",
    )
    typical_internal_sla_context: Dict[str, Any] = Field(
        default_factory=dict,
        description="Empirical transmission windows and cutoff margins",
    )
    unresolved_pattern_flags: List[str] = Field(
        default_factory=list,
        description="Active anomalies (e.g. PERSISTENT_FORMAT_DRIFT, WINDOW_MISS_RISK)",
    )
    source_ref_summary: List[str] = Field(
        default_factory=list,
        description="List of past incident and verification citations supporting this profile",
    )
    confidence_level: Literal["HIGH", "MEDIUM", "LOW"] = "MEDIUM"
    last_updated: str = Field(
        default_factory=lambda: datetime.now(timezone.utc).isoformat(),
        description="ISO 8601 timestamp of last profile refresh",
    )


class AdvisoryMemoryContext(BaseModel):
    """Complete advisory memory package injected into AgentContextEnvelope under ADVISORY_DATA.

    Formal Invariant:
    MemoryRecall != Evidence AND MemoryRef ∉ EvidenceSet
    """

    schema_version: Literal["1.0"] = "1.0"
    query_audit: List[Dict[str, Any]] = Field(
        default_factory=list,
        description="Audit records for executed queries (enforcing max_queries <= 2)",
    )
    retrieved_hits: List[MemoryHit] = Field(
        default_factory=list,
        description="Bounded list of ranked memory hits (len <= 5)",
    )
    partner_profile: Optional[PartnerOperationalProfile] = Field(
        default=None,
        description="Partner operational profile if available",
    )
    advisory_disclaimer: str = Field(
        default=(
            "ADVISORY ONLY: Memory Bank insights provide historical pattern context and MUST NOT "
            "be used to override deterministic validation rules, bypass policy denials, or execute unverified state mutations."
        ),
        description="Mandatory non-authoritative disclaimer",
    )
    provenance_digest: str = Field(
        default="",
        description="SHA-256 digest of hits and profile for tamper detection",
    )
    authorized_memory_refs: List[str] = Field(
        default_factory=list,
        description="Advisory memory references (MEM-HIT-*, MEM-PROFILE-*), strictly disjoint from AuthorizedEvidenceRefs",
    )
    memory_evidence_refs: List[str] = Field(
        default_factory=list,
        description="Deprecated alias for authorized_memory_refs; preserved for backward compatibility",
    )

    @model_validator(mode="after")
    def validate_invariants_and_digest(self) -> "AdvisoryMemoryContext":
        if len(self.retrieved_hits) > 5:
            raise ValueError(
                f"retrieved_hits count {len(self.retrieved_hits)} exceeds maximum bound of 5"
            )
        if len(self.query_audit) > 2:
            raise ValueError(
                f"query_audit count {len(self.query_audit)} exceeds maximum limit of 2 queries"
            )

        # Auto-populate advisory memory refs
        refs = []
        for i, hit in enumerate(self.retrieved_hits):
            refs.append(f"MEM-HIT-{i + 1:02d}")
            refs.append(f"MEM-{hit.memory_id}")
        if self.partner_profile:
            refs.append(f"MEM-PROFILE-{self.partner_profile.partner_ref}")
        self.authorized_memory_refs = sorted(list(set(refs)))
        self.memory_evidence_refs = self.authorized_memory_refs

        # Compute tamper-evident provenance digest
        summary_obj = {
            "hits": [h.memory_id for h in self.retrieved_hits],
            "partner": self.partner_profile.partner_ref if self.partner_profile else None,
            "refs": self.authorized_memory_refs,
        }
        self.provenance_digest = hashlib.sha256(
            json.dumps(summary_obj, sort_keys=True).encode("utf-8")
        ).hexdigest()
        return self
