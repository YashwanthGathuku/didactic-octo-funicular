"""Deterministic Policy Engine models and decision contracts for Python AI Tier.

Enforces SGACA Invariant:
AgentRecommendation != Permission
Permission comes strictly from DeterministicPolicyEngine.
"""

import hashlib
import json
from datetime import datetime, timezone
from enum import Enum
from typing import Any, Dict, List, Optional
from pydantic import BaseModel, Field


class Decision(str, Enum):
    ALLOW = "ALLOW"
    DENY = "DENY"
    ALLOW_WITH_OBLIGATIONS = "ALLOW_WITH_OBLIGATIONS"
    REQUIRE_HUMAN = "REQUIRE_HUMAN"


class PolicyDomain(str, Enum):
    AGENT = "AGENT"
    ARTIFACT = "ARTIFACT"
    REMEDIATION = "REMEDIATION"
    TOOL = "TOOL"
    RELEASE = "RELEASE"
    ENTERPRISE_ACTION = "ENTERPRISE_ACTION"


class PolicyLayer(str, Enum):
    NETWORK_EXTERNAL = "NETWORK_EXTERNAL"
    SENTINEL_SAFETY = "SENTINEL_SAFETY"
    ENTERPRISE = "ENTERPRISE"
    TENANT = "TENANT"
    PARTNER = "PARTNER"


class PolicyStatus(str, Enum):
    DRAFT = "DRAFT"
    ACTIVE = "ACTIVE"
    RETIRED = "RETIRED"


class ObligationType(str, Enum):
    DETERMINISTIC_REVALIDATION = "DETERMINISTIC_REVALIDATION"
    DUAL_CONTROL = "DUAL_CONTROL"
    CANDIDATE_ONLY = "CANDIDATE_ONLY"
    IMMUTABLE_PARENT_REQUIRED = "IMMUTABLE_PARENT_REQUIRED"
    MAX_ATTEMPTS = "MAX_ATTEMPTS"
    HUMAN_REVIEW = "HUMAN_REVIEW"
    EXACT_ARTIFACT_HASH = "EXACT_ARTIFACT_HASH"
    AUDIT_REQUIRED = "AUDIT_REQUIRED"
    SANDBOX_ONLY = "SANDBOX_ONLY"


class Obligation(BaseModel):
    type: ObligationType
    parameters: Optional[Dict[str, Any]] = None


class ProhibitionType(str, Enum):
    MUTATE_ORIGINAL = "MUTATE_ORIGINAL"
    RELEASE = "RELEASE"
    APPROVE = "APPROVE"
    EXECUTE_SQL = "EXECUTE_SQL"
    ACCESS_SECRET = "ACCESS_SECRET"
    CROSS_TENANT_ACCESS = "CROSS_TENANT_ACCESS"
    IRREVERSIBLE_FINANCIAL_AUTHORITY = "IRREVERSIBLE_FINANCIAL_AUTHORITY"


class Prohibition(BaseModel):
    type: ProhibitionType
    description: Optional[str] = None


class PolicyManifestEntry(BaseModel):
    policy_id: str
    version: int
    content_hash: str


class PolicyBundleManifest(BaseModel):
    bundle_id: str
    version: str
    bundle_hash: str
    manifest: List[PolicyManifestEntry] = Field(default_factory=list)


class PolicySubject(BaseModel):
    type: str = "AGENT"
    id: str
    roles: List[str] = Field(default_factory=list)
    autonomy_level: int = 1
    tenant_id: str


class PolicyResource(BaseModel):
    type: str = "ARTIFACT"
    id: str
    sha256: Optional[str] = None
    state: Optional[str] = None
    classification: Optional[str] = None
    tenant_id: str


class PolicyWorkflowContext(BaseModel):
    workflow_id: Optional[str] = None
    state: Optional[str] = None
    attempt: int = 1


class PolicyEnvironment(BaseModel):
    evaluation_time: datetime = Field(default_factory=lambda: datetime.now(timezone.utc))
    fleet_mode: str = "SHADOW"


class PolicyEvaluationRequest(BaseModel):
    request_id: str
    tenant_id: str
    subject: PolicySubject
    action: str
    resource: PolicyResource
    workflow: PolicyWorkflowContext = Field(default_factory=PolicyWorkflowContext)
    environment: PolicyEnvironment = Field(default_factory=PolicyEnvironment)
    authoritative_attributes: Dict[str, Any] = Field(default_factory=dict)


class PolicyDecision(BaseModel):
    decision_id: str
    request_id: str
    decision: Decision
    action: str
    reason_codes: List[str] = Field(default_factory=list)
    obligations: List[Obligation] = Field(default_factory=list)
    prohibitions: List[Prohibition] = Field(default_factory=list)
    matched_policy_refs: List[str] = Field(default_factory=list)
    policy_bundle_id: str = "bundle-sentinel-default"
    policy_bundle_version: str = "1.0.0"
    policy_bundle_hash: str
    manifest: List[PolicyManifestEntry] = Field(default_factory=list)
    evaluated_context_hash: str
    evaluated_at: datetime
    evaluator_version: str = "1.0.0"
    decision_hash: str


class PolicyDefinition(BaseModel):
    policy_id: str
    version: int
    domain: PolicyDomain
    layer: PolicyLayer
    priority: int = 100
    status: PolicyStatus = PolicyStatus.ACTIVE
    effective_from: datetime
    effective_to: Optional[datetime] = None
    tenant_id: Optional[str] = None
    partner_id: Optional[str] = None
    action: str
    subject_constraints: Dict[str, Any] = Field(default_factory=dict)
    resource_constraints: Dict[str, Any] = Field(default_factory=dict)
    conditions: Dict[str, str] = Field(default_factory=dict)
    effect: Decision
    obligations: List[Obligation] = Field(default_factory=list)
    prohibitions: List[Prohibition] = Field(default_factory=list)
    reason_code: str
    source_reference: Optional[str] = None
    created_at: datetime = Field(default_factory=lambda: datetime.now(timezone.utc))
    content_hash: str


def canonical_json_bytes(obj: Any) -> bytes:
    """Format any object according to RFC 8785 JSON Canonicalization Scheme (JCS)."""
    return json.dumps(
        obj,
        separators=(',', ':'),
        sort_keys=True,
        ensure_ascii=False
    ).encode('utf-8')


def compute_policy_content_hash(p: PolicyDefinition) -> str:
    eff_to_str = p.effective_to.strftime('%Y-%m-%dT%H:%M:%SZ') if p.effective_to else None
    eff_from_str = p.effective_from.strftime('%Y-%m-%dT%H:%M:%SZ')

    obls = sorted([o.model_dump(exclude_none=True) for o in p.obligations], key=lambda x: x['type'])
    prohs = sorted([pr.model_dump(exclude_none=True) for pr in p.prohibitions], key=lambda x: x['type'])

    payload = {
        "schema_version": "1.0",
        "policy_id": p.policy_id,
        "version": p.version,
        "domain": str(p.domain.value),
        "layer": str(p.layer.value),
        "priority": p.priority,
        "status": str(p.status.value),
        "effective_from": eff_from_str,
        "effective_to": eff_to_str,
        "tenant_id": p.tenant_id,
        "partner_id": p.partner_id,
        "action": p.action,
        "effect": str(p.effect.value),
        "reason_code": p.reason_code,
        "obligations": obls,
        "prohibitions": prohs,
        "subject_constraints": {
            "type": p.subject_constraints.get("type", "*"),
            "id": p.subject_constraints.get("id", "*"),
            "roles": sorted(p.subject_constraints.get("roles", [])),
            "min_autonomy": p.subject_constraints.get("min_autonomy", 0),
            "max_autonomy": p.subject_constraints.get("max_autonomy", 0),
        },
        "resource_constraints": {
            "type": p.resource_constraints.get("type", "ARTIFACT"),
            "id": p.resource_constraints.get("id", "*"),
            "states": sorted(p.resource_constraints.get("states", [])),
            "classification": p.resource_constraints.get("classification", ""),
        },
        "conditions": p.conditions,
        "source_reference": p.source_reference,
    }
    return hashlib.sha256(canonical_json_bytes(payload)).hexdigest()
