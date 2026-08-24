"""Authoritative Evidence Grounding Guardrail for SentinelFlow (SGACA P05).

Invariants:
1. Grounding Invariant: ReturnedEvidenceRefs ⊆ AuthorizedEvidenceSet
2. Monotonic expansion: The AuthorizedEvidenceSet can only be expanded via verified Tool Gateway outputs.
3. Strict fail-closed rejection: Fabricated or ungrounded evidence references cause immediate GROUNDING_VIOLATION.
"""

from __future__ import annotations

from dataclasses import dataclass
from enum import Enum
from typing import Any, Dict, List, Optional, Set
from contracts.diagnosis import DiagnosisOutput


class GroundingVerdict(str, Enum):
    VERIFIED = "VERIFIED"
    PARTIALLY_GROUNDED = "PARTIALLY_GROUNDED"
    UNGROUNDED_REJECTED = "UNGROUNDED_REJECTED"


class GroundingViolationError(Exception):
    """Raised when an AI diagnosis cites unauthorized or fabricated evidence references."""

    def __init__(self, message: str, unauthorized_citations: List[str]):
        super().__init__(
            f"[GROUNDING_VIOLATION] {message} (unauthorized: {unauthorized_citations})"
        )
        self.unauthorized_citations = unauthorized_citations


@dataclass
class GroundingVerificationResult:
    verdict: GroundingVerdict
    is_valid: bool
    claimed_citations: Set[str]
    authorized_set: Set[str]
    unauthorized_citations: Set[str]
    remediated_output: Optional[DiagnosisOutput] = None
    error_message: Optional[str] = None


class AuthorizedEvidenceSet:
    """Manages the monotonic set of authorized evidence references for an agent execution."""

    def __init__(self, initial_refs: Optional[Set[str]] = None):
        self._authorized: Set[str] = set(initial_refs or [])

    @classmethod
    def from_envelope(cls, envelope: Dict[str, Any]) -> "AuthorizedEvidenceSet":
        """Initializes the authorized evidence set from a validated AgentContextEnvelope."""
        evidence = set()

        # 1. Finding IDs and Finding Codes
        for f in envelope.get("findings", []):
            if isinstance(f, dict):
                fid = f.get("id")
                fcode = f.get("code")
            else:
                fid = getattr(f, "id", None)
                fcode = getattr(f, "code", None)

            if fid:
                evidence.add(str(fid).strip())
            if fcode:
                evidence.add(str(fcode).strip())

        # 2. Available Runbooks
        for rb in envelope.get("available_runbooks", []):
            rb_str = str(rb).strip()
            evidence.add(rb_str)
            if not rb_str.startswith("RUNBOOK-"):
                evidence.add(f"RUNBOOK-{rb_str}")

        # 3. Telemetry summary keys
        for k in envelope.get("telemetry_summary", {}).keys():
            evidence.add(f"METRIC-{str(k).strip()}")

        # 4. Explicit authorized evidence refs from envelope
        for ref in envelope.get("authorized_evidence_refs", []):
            evidence.add(str(ref).strip())

        return cls(evidence)

    def add(self, ref: str) -> None:
        """Adds a single authorized reference."""
        if ref and isinstance(ref, str):
            self._authorized.add(ref.strip())

    def add_reference(self, ref: str) -> None:
        """Alias for add."""
        self.add(ref)

    def update(self, refs: List[str] | Set[str]) -> None:
        """Adds multiple authorized references."""
        for r in refs:
            self.add(r)

    def expand_from_tool_result(self, tool_id: str, tool_output: Any) -> None:
        """Expands the evidence set monotonically from verified tool output."""
        if isinstance(tool_output, dict):
            if "finding_code" in tool_output:
                self.add(tool_output["finding_code"])
            if "rule_citation" in tool_output and tool_output["rule_citation"]:
                self.add(tool_output["rule_citation"])
            if "incident_id" in tool_output:
                self.add(f"INCIDENT-{tool_output['incident_id']}")
            if "artifact_id" in tool_output:
                self.add(f"ARTIFACT-{tool_output['artifact_id']}")
            if "minted_evidence_tokens" in tool_output:
                for token in tool_output["minted_evidence_tokens"]:
                    self.add(str(token))
        elif isinstance(tool_output, list):
            for item in tool_output:
                self.expand_from_tool_result(tool_id, item)

    def contains(self, ref: str) -> bool:
        """Checks if a reference is in the authorized set."""
        return str(ref).strip() in self._authorized

    @property
    def references(self) -> Set[str]:
        return set(self._authorized)


class VerificationAuthorizedEvidenceSet(AuthorizedEvidenceSet):
    """Monotonic authorized evidence set specifically for candidate artifact verification critic."""

    @classmethod
    def from_verification_context(
        cls,
        context: Dict[str, Any] | Any,
    ) -> "VerificationAuthorizedEvidenceSet":
        evidence: Set[str] = set()

        if isinstance(context, dict):
            ctx = context
        elif hasattr(context, "model_dump"):
            ctx = context.model_dump()
        else:
            ctx = vars(context)

        # 1. Base authorized evidence refs
        for ref in ctx.get("authorized_evidence_refs", []):
            if ref:
                evidence.add(str(ref).strip())

        # 2. Findings and finding codes
        for f in ctx.get("findings", []):
            if isinstance(f, dict):
                fid = f.get("id")
                fcode = f.get("code")
            else:
                fid = getattr(f, "id", None)
                fcode = getattr(f, "code", None)
            if fid:
                evidence.add(str(fid).strip())
            if fcode:
                evidence.add(str(fcode).strip())

        # 3. Candidate & Parent artifacts
        cand_ref = ctx.get("candidate_ref")
        if cand_ref:
            evidence.add(str(cand_ref).strip())
        cand_id = ctx.get("candidate_artifact_id")
        if cand_id:
            evidence.add(f"ARTIFACT-{cand_id}")
            evidence.add(f"CANDIDATE-{cand_id}")

        parent_id = ctx.get("artifact_id") or ctx.get("parent_artifact_id")
        if parent_id:
            evidence.add(f"ARTIFACT-{parent_id}")

        incident_id = ctx.get("incident_id")
        if incident_id:
            evidence.add(f"INCIDENT-{incident_id}")

        # 4. Verification checks
        for chk in ctx.get("verification_checks", []):
            if isinstance(chk, dict):
                ctype = chk.get("check_type")
                cid = chk.get("id")
            else:
                ctype = getattr(chk, "check_type", None)
                cid = getattr(chk, "id", None)
            if ctype:
                evidence.add(str(ctype).strip())
                evidence.add(f"CHECK-{ctype}")
            if cid:
                evidence.add(f"CHECK-{cid}")

        return cls(evidence)


class EvidenceGroundingVerifier:
    """Verifies that an agent output conforms strictly to the AuthorizedEvidenceSet."""

    @classmethod
    def verify(
        cls,
        output: Any,
        evidence_set: AuthorizedEvidenceSet,
        strict: bool = True,
    ) -> GroundingVerificationResult:
        """Validates that all evidence references in output are authorized.

        In strict mode (default): Any unauthorized reference triggers UNGROUNDED_REJECTED.
        """
        claimed: Set[str] = set()

        # Collect top-level evidence refs
        if hasattr(output, "evidence_refs") and output.evidence_refs:
            for ref in output.evidence_refs:
                claimed.add(str(ref).strip())

        # Collect hypothesis evidence refs (DiagnosisOutput)
        if hasattr(output, "hypotheses") and output.hypotheses:
            for hyp in output.hypotheses:
                if hasattr(hyp, "evidence_refs") and hyp.evidence_refs:
                    for ref in hyp.evidence_refs:
                        claimed.add(str(ref).strip())

        # Collect operation evidence refs (RemediationPlan)
        if hasattr(output, "operations") and output.operations:
            for op in output.operations:
                if hasattr(op, "evidence_refs") and op.evidence_refs:
                    for ref in op.evidence_refs:
                        claimed.add(str(ref).strip())
                if hasattr(op, "finding_refs") and op.finding_refs:
                    for ref in op.finding_refs:
                        claimed.add(str(ref).strip())

        auth_set = evidence_set.references
        unauthorized = claimed - auth_set

        if not unauthorized:
            return GroundingVerificationResult(
                verdict=GroundingVerdict.VERIFIED,
                is_valid=True,
                claimed_citations=claimed,
                authorized_set=auth_set,
                unauthorized_citations=set(),
                remediated_output=output,
                error_message=None,
            )

        if strict:
            return GroundingVerificationResult(
                verdict=GroundingVerdict.UNGROUNDED_REJECTED,
                is_valid=False,
                claimed_citations=claimed,
                authorized_set=auth_set,
                unauthorized_citations=unauthorized,
                remediated_output=None,
                error_message=(
                    f"Grounding violation: {len(unauthorized)} unauthorized citation(s) "
                    f"detected: {sorted(list(unauthorized))}"
                ),
            )

        # Non-strict / Pruning mode: filter out unauthorized citations and demote confidence
        updates: Dict[str, Any] = {}
        if hasattr(output, "evidence_refs"):
            updates["evidence_refs"] = [
                r for r in output.evidence_refs if str(r).strip() in auth_set
            ]

        if hasattr(output, "hypotheses") and output.hypotheses:
            pruned_hypotheses = []
            for hyp in output.hypotheses:
                valid_refs = [r for r in hyp.evidence_refs if str(r).strip() in auth_set]
                conf = hyp.confidence
                if len(valid_refs) < len(hyp.evidence_refs):
                    conf = "LOW"
                pruned_hyp = hyp.model_copy(
                    update={"evidence_refs": valid_refs, "confidence": conf}
                )
                pruned_hypotheses.append(pruned_hyp)
            updates["hypotheses"] = pruned_hypotheses

        if hasattr(output, "unknowns") and output.unknowns is not None:
            updates["unknowns"] = list(output.unknowns) + [
                f"PRUNED_UNAUTHORIZED_REFERENCE: {u}" for u in sorted(list(unauthorized))
            ]

        remediated = output.model_copy(update=updates) if hasattr(output, "model_copy") else output

        return GroundingVerificationResult(
            verdict=GroundingVerdict.PARTIALLY_GROUNDED,
            is_valid=True,
            claimed_citations=claimed,
            authorized_set=auth_set,
            unauthorized_citations=unauthorized,
            remediated_output=remediated,
            error_message=f"Pruned {len(unauthorized)} unauthorized citations.",
        )

    @classmethod
    def verify_references(
        cls,
        claimed_refs: List[str] | Set[str],
        evidence_set: AuthorizedEvidenceSet,
        strict: bool = True,
    ) -> GroundingVerificationResult:
        """Validates an arbitrary collection of evidence references."""
        claimed = {str(r).strip() for r in claimed_refs if r}
        auth_set = evidence_set.references
        unauthorized = claimed - auth_set

        if not unauthorized:
            return GroundingVerificationResult(
                verdict=GroundingVerdict.VERIFIED,
                is_valid=True,
                claimed_citations=claimed,
                authorized_set=auth_set,
                unauthorized_citations=set(),
                error_message=None,
            )

        return GroundingVerificationResult(
            verdict=GroundingVerdict.UNGROUNDED_REJECTED
            if strict
            else GroundingVerdict.PARTIALLY_GROUNDED,
            is_valid=not strict,
            claimed_citations=claimed,
            authorized_set=auth_set,
            unauthorized_citations=unauthorized,
            error_message=(
                f"Grounding violation: {len(unauthorized)} unauthorized citation(s) "
                f"detected: {sorted(list(unauthorized))}"
            ),
        )
