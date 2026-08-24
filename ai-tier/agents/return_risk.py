"""ReturnRiskAgent — governed ACH return-risk specialist (SGACA P12.5).

Formal invariants:
- ReturnRiskAssessment != FinancialDecision.
- ReturnRiskAgent != ReturnAuthority.
- HistoricalReturnPattern != CurrentTransactionTruth.
- MemoryRecall != Evidence.
- RiskHigh != AutoRejectFinancialFile and RiskLow != AutoReleaseFinancialFile.
- All live model inference crosses the shared P09 GuardedModelBoundary.
"""

from __future__ import annotations

import hashlib
import logging
import os
import re
from typing import Any, Dict, List, Optional, Union

import google.adk.agents as adk_agents
import google.adk.runners as adk_runners

from contracts.manifests import FIXED_AGENT_ROSTER
from contracts.return_risk import (
    RETURN_RISK_NON_AUTHORITY_STATEMENT,
    ReturnRiskAssessment,
    ReturnRiskContextEnvelope,
    ReturnRiskTier,
)
from guardrails.boundary import GuardedModelBoundary
from guardrails.evidence import AuthorizedEvidenceSet
from models.envelope import AgentContextEnvelope

logger = logging.getLogger("sentinel.ai.return_risk")

ROUTING_REGEX = re.compile(r"\b\d{9}\b")
ACCOUNT_REGEX = re.compile(r"\b\d{10,17}\b")


class ReturnRiskExecutionError(RuntimeError):
    """Typed execution failure for a governed return-risk stage."""

    def __init__(self, error_code: str, message: str, execution_source: str):
        super().__init__(message)
        self.error_code = error_code
        self.execution_source = execution_source


class ReturnRiskAgent:
    """Autonomy A1 return-risk specialist with no financial decision authority."""

    def __init__(
        self,
        model_name: str = "gemini-3.5-flash",
        gateway_base_url: str = "http://localhost:8080",
        boundary: Optional[GuardedModelBoundary] = None,
    ):
        self.manifest = FIXED_AGENT_ROSTER["ReturnRiskAgent"]
        self.model_name = model_name
        self.gateway_base_url = gateway_base_url
        self.boundary = boundary or GuardedModelBoundary(default_model=self.model_name)

        # ADK runtime objects remain the registered local agent representation.
        self.adk_agent = adk_agents.Agent(
            name=self.manifest.name,
            model=self.model_name,
            instruction=(
                "You are the SentinelFlow Return Risk Specialist Agent (A1 Autonomy).\n"
                "Analyze trusted deterministic return-risk results and produce advisory operational guidance.\n"
                "You have no authority to approve, reject, release, waive, or mutate financial state.\n"
                "The Go-provided return event, return code, risk score, and risk tier are immutable.\n"
                "Evidence references and memory references are disjoint, and memory is advisory only.\n"
                f"Always preserve this statement: {RETURN_RISK_NON_AUTHORITY_STATEMENT}"
            ),
            output_key="return_risk_assessment",
        )
        self.adk_runner = adk_runners.InMemoryRunner(agent=self.adk_agent)

    def _extract_context(
        self, context: Union[Dict[str, Any], ReturnRiskContextEnvelope, Any]
    ) -> ReturnRiskContextEnvelope:
        if isinstance(context, ReturnRiskContextEnvelope):
            return context
        if isinstance(context, dict):
            return ReturnRiskContextEnvelope.model_validate(context)
        if hasattr(context, "model_dump"):
            return ReturnRiskContextEnvelope.model_validate(context.model_dump())
        return ReturnRiskContextEnvelope.model_validate(vars(context))

    def _mask_sensitive_data(self, text: str) -> str:
        """Backward-compatible helper; the shared boundary remains authoritative for minimization."""
        if not text:
            return ""
        text = ACCOUNT_REGEX.sub("[ACCOUNT_REDACTED]", text)
        text = ROUTING_REGEX.sub("[ROUTING_REDACTED]", text)
        return text

    def _stable_incident_id(self, env: ReturnRiskContextEnvelope) -> int:
        if env.incident_id and env.incident_id > 0:
            return env.incident_id
        digest = hashlib.sha256(env.return_event_ref.encode("utf-8")).digest()
        return (int.from_bytes(digest[:4], "big") % 2_000_000_000) + 1

    def _to_boundary_envelope(self, env: ReturnRiskContextEnvelope) -> AgentContextEnvelope:
        """Build the canonical P09 envelope without introducing raw financial payloads."""
        return AgentContextEnvelope(
            tenant_id=env.tenant_scope,
            workflow_id=env.workflow_id or f"return-risk-{env.return_event_ref}",
            incident_id=self._stable_incident_id(env),
            artifact_id=0,
            correlation_id=env.correlation_id or env.return_event_ref,
            agent_name=self.manifest.name,
            agent_version=self.manifest.version,
            authorized_evidence_refs=list(env.authorized_evidence_refs),
            allowed_tools=list(self.manifest.allowed_tools),
            findings=[],
            available_runbooks=[],
            filename="return-event-context",
        )

    def _trusted_tool_output(self, env: ReturnRiskContextEnvelope) -> List[Dict[str, Any]]:
        """Represent Go-owned return-risk results as governed tool output (Domain 4)."""
        sanitized_history = self.boundary.sanitize_financial_content(env.historical_summary or "")
        return [
            {
                "tool_id": "returnrisk.result.get",
                "status": "SUCCESS",
                "output": {
                    "return_event_ref": env.return_event_ref,
                    "return_code": env.return_code,
                    "return_code_label": env.return_code_label,
                    "risk_score": env.risk_score,
                    "risk_tier": env.risk_tier.value,
                    "partner_ref": env.partner_ref,
                    "contributions": [c.model_dump() for c in env.contributions],
                    "sla_cutoff_context": env.sla_cutoff_context,
                    "authorized_memory_refs": list(env.authorized_memory_refs),
                    "advisory_historical_summary": sanitized_history,
                    "historical_summary_authority": "ADVISORY_ONLY_NOT_EVIDENCE",
                },
            }
        ]

    def _deterministic_fallback(self, env: ReturnRiskContextEnvelope) -> ReturnRiskAssessment:
        """Rule-grounded advisory output allowed only under explicit local/fallback semantics."""
        principal_drivers = [
            f"{c.feature_name} (impact: {c.contribution_score:+.2f}): {c.description}"
            for c in env.contributions
        ]

        historical_patterns: List[str] = []
        if env.historical_summary:
            historical_patterns.append(
                self.boundary.sanitize_financial_content(env.historical_summary)
            )

        code = env.return_code.upper()
        recommendations: List[str] = []
        uncertainties: List[str] = []

        if code in ("R01", "R09"):
            recommendations.append(
                "Review the return event and institution-controlled reinitiation eligibility before any subsequent processing decision."
            )
        elif code in ("R05", "R07", "R10", "R11", "R29"):
            recommendations.append(
                "Route the event for authorization review; do not automatically reinitiate or release based on risk output."
            )
            recommendations.append(
                "Use the trusted Go return-risk context for the applicable public Nacha return-rate monitoring category."
            )
        elif code in ("R02", "R03", "R04"):
            recommendations.append(
                "Review and correct account information before considering a subsequent entry."
            )
        elif code == "R08":
            recommendations.append(
                "Review the stop-payment context before any subsequent processing decision."
            )
        elif code == "R16":
            recommendations.append(
                "Route for compliance review; no percentage threshold or sanctions/legal determination is produced by this agent."
            )
        else:
            recommendations.append(
                f"Perform standard operator exception review for return code {code} ({env.return_code_label})."
            )

        escalation_recommended = (
            env.risk_tier in (ReturnRiskTier.HIGH, ReturnRiskTier.SEVERE)
            or env.risk_score >= 60.0
            or code in ("R05", "R07", "R10", "R11", "R16", "R29")
        )
        if escalation_recommended:
            recommendations.insert(
                0,
                f"Supervisor review recommended for {env.risk_tier.value} risk return event.",
            )

        if not env.partner_ref:
            uncertainties.append(
                "Partner profile reference is not linked to the return event context."
            )
        if not env.historical_summary:
            uncertainties.append(
                "No historical return-pattern context is available for this assessment."
            )

        canonical_input = {
            "tenant_scope": env.tenant_scope,
            "return_event_ref": env.return_event_ref,
            "return_code": env.return_code,
            "risk_score": env.risk_score,
            "risk_tier": env.risk_tier.value,
            "evidence_refs": sorted(set(env.authorized_evidence_refs)),
            "memory_refs": sorted(set(env.authorized_memory_refs)),
        }
        input_hash = hashlib.sha256(
            str(sorted(canonical_input.items())).encode("utf-8")
        ).hexdigest()

        out = ReturnRiskAssessment(
            schema_version="1.0",
            return_event_ref=env.return_event_ref,
            return_code=env.return_code,
            risk_score=env.risk_score,
            risk_tier=env.risk_tier,
            summary=(
                f"Return Event {env.return_event_ref} evaluated with deterministic risk score "
                f"{env.risk_score:.2f}/100 ({env.risk_tier.value})."
            ),
            principal_drivers=principal_drivers,
            historical_patterns=historical_patterns,
            operational_recommendations=recommendations,
            evidence_refs=list(env.authorized_evidence_refs),
            memory_refs=list(env.authorized_memory_refs),
            uncertainties=uncertainties,
            escalation_recommended=escalation_recommended,
            non_authority_statement=RETURN_RISK_NON_AUTHORITY_STATEMENT,
            statement=RETURN_RISK_NON_AUTHORITY_STATEMENT,
            input_context_hash=input_hash,
            output_hash="",
            manifest_hash=self.manifest.manifest_hash,
            execution_source="LOCAL_ADK_DETERMINISTIC",
        )
        out.output_hash = hashlib.sha256(out.model_dump_json().encode("utf-8")).hexdigest()
        return out

    def _raise_boundary_failure(self, error_code: Optional[str], message: Optional[str]) -> None:
        code = error_code or "PROVIDER_UNAVAILABLE"
        if code in {
            "PROMPT_SECURITY_BLOCKED",
            "MODEL_RESPONSE_BLOCKED",
            "GUARDRAIL_UNAVAILABLE",
            "GROUNDING_VIOLATION",
        }:
            source = "GUARDRAIL_BLOCKED"
        else:
            source = "PROVIDER_UNAVAILABLE"
        raise ReturnRiskExecutionError(code, message or code, source)

    def _enforce_deterministic_dominance(
        self,
        env: ReturnRiskContextEnvelope,
        output: ReturnRiskAssessment,
        input_hash: str,
    ) -> ReturnRiskAssessment:
        """Go-owned event/code/score/tier always dominate model-provided fields."""
        auth_memory = set(env.authorized_memory_refs)
        if any(ref not in auth_memory for ref in output.memory_refs):
            raise ReturnRiskExecutionError(
                "GROUNDING_VIOLATION",
                "ReturnRiskAssessment cited an unauthorized memory reference",
                "GUARDRAIL_BLOCKED",
            )

        output.return_event_ref = env.return_event_ref
        output.return_code = env.return_code
        output.risk_score = env.risk_score
        output.risk_tier = env.risk_tier
        output.memory_refs = [ref for ref in output.memory_refs if ref in auth_memory]
        output.input_context_hash = input_hash
        output.manifest_hash = self.manifest.manifest_hash
        output.non_authority_statement = RETURN_RISK_NON_AUTHORITY_STATEMENT
        output.statement = RETURN_RISK_NON_AUTHORITY_STATEMENT
        output.execution_source = "LIVE_GEMINI"
        output.output_hash = ""
        output.output_hash = hashlib.sha256(output.model_dump_json().encode("utf-8")).hexdigest()
        return output

    def run(
        self,
        envelope_data: Union[Dict[str, Any], ReturnRiskContextEnvelope, Any],
    ) -> ReturnRiskAssessment:
        """Execute return-risk advisory analysis with explicit provider truth semantics."""
        env = self._extract_context(envelope_data)
        ai_mode = os.getenv("SENTINEL_AI_MODE", "auto").strip().lower()

        if ai_mode in {"local", "deterministic"}:
            return self._deterministic_fallback(env)
        if ai_mode not in {"live", "auto"}:
            raise ReturnRiskExecutionError(
                "INVALID_AI_MODE",
                f"Unsupported SENTINEL_AI_MODE={ai_mode!r}",
                "PROVIDER_UNAVAILABLE",
            )

        boundary_envelope = self._to_boundary_envelope(env)
        evidence_set = AuthorizedEvidenceSet(initial_refs=set(env.authorized_evidence_refs))
        tool_outputs = self._trusted_tool_output(env)
        memory_index = sorted(set(env.authorized_memory_refs))

        role_instruction = (
            "ROLE SPECIFIC INSTRUCTION (ReturnRiskAgent):\n"
            "Return JSON conforming to ReturnRiskAssessment. The trusted tool output contains "
            "the authoritative Go return event, return code, risk score, and risk tier; never alter them.\n"
            "ReturnTaxonomyGuidance is operational intelligence, not a legal decision. "
            "ReturnRiskScore is not a compliance decision.\n"
            f"Authorized memory refs (advisory only, never evidence): {memory_index}.\n"
            f"Mandatory statement: {RETURN_RISK_NON_AUTHORITY_STATEMENT}"
        )

        fallback_fn = None
        if ai_mode == "auto":

            def fallback_fn(_boundary_env, _auth_set):
                return self._deterministic_fallback(env)

        result = self.boundary.invoke(
            envelope=boundary_envelope,
            response_schema=ReturnRiskAssessment,
            role_system_prompt=role_instruction,
            tool_outputs=tool_outputs,
            evidence_set=evidence_set,
            fallback_fn=fallback_fn,
            strict_grounding=True,
            max_tokens=2048,
            temperature=0.1,
        )

        if not result.success or result.output is None:
            self._raise_boundary_failure(result.error_code, result.error_message)

        if result.audit.execution_source == "DETERMINISTIC_FALLBACK":
            # AUTO fallback is truthful deterministic output, never labeled live AI.
            output = result.output
            output.execution_source = "LOCAL_ADK_DETERMINISTIC"
            output.input_context_hash = result.audit.post_guardrail_input_hash
            output.output_hash = ""
            output.output_hash = hashlib.sha256(
                output.model_dump_json().encode("utf-8")
            ).hexdigest()
            return output

        if result.audit.execution_source != "LIVE_GEMINI":
            raise ReturnRiskExecutionError(
                "PROVIDER_UNAVAILABLE",
                f"Unexpected boundary execution source: {result.audit.execution_source}",
                "PROVIDER_UNAVAILABLE",
            )

        return self._enforce_deterministic_dominance(
            env,
            result.output,
            result.audit.post_guardrail_input_hash,
        )
