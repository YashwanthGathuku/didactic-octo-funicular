"""PolicySLAAgent — Governed, Read-Only Policy and SLA Interpretation Specialist (SGACA Phase P06.5/P09).

Formal Invariant: PolicySLAAgentOpinion != PolicyDecision.
The agent explains and interprets authoritative deterministic policy decisions and SLA contract context.
It cannot override PolicyEngine decisions (ALLOW, DENY, REQUIRE_HUMAN).
Uses actual Google ADK Agent and Runner runtime primitives and Google Cloud Model Armor guardrails.
"""

from __future__ import annotations

import hashlib
import json
import logging
import os
import time
from typing import Any, Dict, List, Optional, Union

import google.adk.agents as adk_agents
import google.adk.runners as adk_runners
from contracts.manifests import FIXED_AGENT_ROSTER
from contracts.orchestration import AgentHandoffEnvelope, SpecialistResult
from contracts.policy_sla import PolicySLAOutput
from guardrails.boundary import GuardedModelBoundary
from guardrails.evidence import (
    AuthorizedEvidenceSet,
    EvidenceGroundingVerifier,
    GroundingVerdict,
    GroundingViolationError,
)
from guardrails.prompt import PromptTrustPartitioner
from models.envelope import AgentContextEnvelope
from tools.gateway_client import ToolGatewayClient, ToolGatewayContext
from tools.tool_adapter import SentinelToolAdapter

logger = logging.getLogger("sentinel.ai.policy_sla")


class PolicySLAAgent:
    """Governed Read-Only Policy & SLA Interpretation Specialist."""

    def __init__(self, model_name: str = "gemini-3.5-flash", gateway_base_url: str = "http://localhost:8080"):
        self.manifest = FIXED_AGENT_ROSTER["PolicySLAAgent"]
        self.model_name = model_name
        self.gateway_base_url = gateway_base_url
        self.boundary = GuardedModelBoundary(default_model=self.model_name)

        # Real Google ADK Agent & Runner Runtime Objects
        self.adk_agent = adk_agents.Agent(
            name=self.manifest.name,
            model=self.model_name,
            instruction=(
                "You are the SentinelFlow Policy and SLA Agent.\n"
                "You explain governing deterministic policy decisions, active obligations, "
                "prohibitions, and deterministic SLA delivery cutoffs for operators.\n"
                "You operate in a strictly READ-ONLY capacity and have no authority to mutate system state."
            ),
            output_key="policy_sla_result",
        )
        self.adk_runner = adk_runners.InMemoryRunner(agent=self.adk_agent)

    def run(
        self,
        envelope_or_handoff: Union[AgentContextEnvelope, AgentHandoffEnvelope, Dict[str, Any]],
        authoritative_policy_decision: Optional[Dict[str, Any]] = None,
        sla_context: Optional[Dict[str, Any]] = None,
    ) -> SpecialistResult[PolicySLAOutput]:
        """Executes PolicySLAAgent with evidence grounding, deterministic SLA computation, and protected bindings."""
        start_time = time.time()

        if isinstance(envelope_or_handoff, AgentHandoffEnvelope):
            envelope = AgentContextEnvelope(
                workflow_id=envelope_or_handoff.workflow_id,
                tenant_id=envelope_or_handoff.tenant_id,
                incident_id=envelope_or_handoff.incident_id,
                artifact_id=envelope_or_handoff.artifact_id,
                artifact_sha256=envelope_or_handoff.artifact_sha256,
                policy_version=envelope_or_handoff.policy_bundle_hash or "default/1",
                correlation_id=envelope_or_handoff.correlation_id,
                trace_id=envelope_or_handoff.trace_id,
                authorized_evidence_refs=envelope_or_handoff.authorized_evidence_refs,
                allowed_tools=envelope_or_handoff.allowed_tools,
            )
        elif isinstance(envelope_or_handoff, dict):
            envelope = AgentContextEnvelope(**envelope_or_handoff)
        else:
            envelope = envelope_or_handoff

        tenant_id = envelope.tenant_id
        incident_id = envelope.incident_id
        workflow_id = envelope.workflow_id or f"wf-policy-{tenant_id}-{incident_id}"

        # Initialize AuthorizedEvidenceSet
        evidence_set = AuthorizedEvidenceSet.from_envelope(envelope.model_dump())
        if authoritative_policy_decision and "decision_id" in authoritative_policy_decision:
            evidence_set.add_reference(authoritative_policy_decision["decision_id"])
            evidence_set.add_reference(f"POLICY-DECISION-{authoritative_policy_decision['decision_id']}")

        # Extract authoritative policy & SLA context (server-injected)
        decision_id = (authoritative_policy_decision or {}).get("decision_id", f"POL-DEC-{incident_id}")
        decision_verdict = (authoritative_policy_decision or {}).get("decision", "REQUIRE_HUMAN")
        active_obligations = (authoritative_policy_decision or {}).get(
            "obligations", ["CANDIDATE_ONLY_REMEDIATION", "DUAL_CONTROL_APPROVAL_REQUIRED"]
        )
        active_prohibitions = (authoritative_policy_decision or {}).get(
            "prohibitions", ["PROHIBIT_DIRECT_ORIGINAL_MUTATION", "PROHIBIT_AUTONOMOUS_RELEASE"]
        )

        cutoff_type = (sla_context or {}).get("cutoff_type", "INSTITUTION_INTERNAL")
        
        # Section 8: Deterministic SLA computation from authoritative timestamps
        if sla_context and "cutoff_timestamp" in sla_context and "evaluation_timestamp" in sla_context:
            time_remaining_seconds = max(0, int(sla_context["cutoff_timestamp"] - sla_context["evaluation_timestamp"]))
        elif sla_context and "time_remaining_seconds" in sla_context:
            time_remaining_seconds = int(sla_context["time_remaining_seconds"])
        else:
            time_remaining_seconds = 3600

        sla_status = (sla_context or {}).get("sla_status", "ON_TRACK" if time_remaining_seconds > 1800 else "AT_RISK")

        # Compile Prompt
        prompt = PromptTrustPartitioner.compile(envelope)
        input_hash = hashlib.sha256(prompt.user_prompt.encode("utf-8")).hexdigest()
        evidence_set_hash = hashlib.sha256(json.dumps(sorted(list(evidence_set.references))).encode("utf-8")).hexdigest()
        policy_bundle_hash = envelope.policy_version or "default/1"

        ai_mode = os.getenv("SENTINEL_AI_MODE", "auto").lower()
        google_key = os.getenv("GOOGLE_API_KEY") or os.getenv("GEMINI_API_KEY")

        # Strict Live Mode Check
        if ai_mode == "live" and not google_key:
            latency_ms = (time.time() - start_time) * 1000.0
            return SpecialistResult(
                agent_name=self.manifest.name,
                agent_version=self.manifest.version,
                manifest_hash=self.manifest.manifest_hash,
                input_context_hash=input_hash,
                artifact_sha256=envelope.artifact_sha256,
                policy_bundle_hash=policy_bundle_hash,
                authorized_evidence_set_hash=evidence_set_hash,
                execution_source="LIVE_GEMINI",
                status="FAILED",
                output=None,
                latency_ms=latency_ms,
                error={
                    "code": "MISSING_CREDENTIALS",
                    "message": "GOOGLE_API_KEY is required for live PolicySLAAgent execution",
                },
            )

        policy_system_instruction = (
            f"{prompt.system_instruction}\n\n"
            "ROLE SPECIFIC INSTRUCTION (PolicySLAAgent):\n"
            "You explain and interpret governing deterministic policy decisions, active obligations, "
            "prohibitions, and SLA cutoffs for operators.\n"
            f"AUTHORITATIVE POLICY ENGINE VERDICT: {decision_verdict}\n"
            f"ACTIVE OBLIGATIONS: {json.dumps(active_obligations)}\n"
            f"ACTIVE PROHIBITIONS: {json.dumps(active_prohibitions)}\n"
            "INVARIANT: You CANNOT override or dispute the deterministic policy engine verdict.\n"
        )

        def _fallback(env: AgentContextEnvelope, auth_set: AuthorizedEvidenceSet) -> PolicySLAOutput:
            return self._build_deterministic_policy_sla(
                incident_id=incident_id,
                decision_id=decision_id,
                decision_verdict=decision_verdict,
                active_obligations=active_obligations,
                active_prohibitions=active_prohibitions,
                sla_status=sla_status,
                cutoff_type=cutoff_type,
                time_remaining_seconds=time_remaining_seconds,
                sla_context=sla_context,
                envelope=envelope,
            )

        res = self.boundary.invoke(
            envelope=envelope,
            response_schema=PolicySLAOutput,
            role_system_prompt=policy_system_instruction,
            evidence_set=evidence_set,
            fallback_fn=_fallback,
            strict_grounding=True,
        )

        latency_ms = res.audit.latency_ms

        if res.success and res.output:
            pol_out = res.output
            if decision_id not in pol_out.authoritative_policy_decision_refs:
                pol_out.authoritative_policy_decision_refs.append(decision_id)

            return SpecialistResult(
                agent_name=self.manifest.name,
                agent_version=self.manifest.version,
                manifest_hash=self.manifest.manifest_hash,
                input_context_hash=res.audit.post_guardrail_input_hash or input_hash,
                artifact_sha256=envelope.artifact_sha256,
                policy_bundle_hash=policy_bundle_hash,
                authorized_evidence_set_hash=evidence_set_hash,
                execution_source=res.audit.execution_source,
                status="SUCCESS",
                output=pol_out,
                evidence_refs=pol_out.evidence_refs,
                model_provenance={
                    "model": res.audit.model_name,
                    "provider": res.audit.provider,
                },
                input_hash=res.audit.post_guardrail_input_hash or input_hash,
                output_hash=res.audit.post_guardrail_output_hash,
                latency_ms=latency_ms,
            )

        # Fallback
        fallback_output = self._build_deterministic_policy_sla(
            incident_id=incident_id,
            decision_id=decision_id,
            decision_verdict=decision_verdict,
            active_obligations=active_obligations,
            active_prohibitions=active_prohibitions,
            sla_status=sla_status,
            cutoff_type=cutoff_type,
            time_remaining_seconds=time_remaining_seconds,
            sla_context=sla_context,
            envelope=envelope,
        )

        output_json = fallback_output.model_dump_json()
        output_hash = hashlib.sha256(output_json.encode("utf-8")).hexdigest()

        return SpecialistResult(
            agent_name=self.manifest.name,
            agent_version=self.manifest.version,
            manifest_hash=self.manifest.manifest_hash,
            input_context_hash=input_hash,
            artifact_sha256=envelope.artifact_sha256,
            policy_bundle_hash=policy_bundle_hash,
            authorized_evidence_set_hash=evidence_set_hash,
            execution_source="DETERMINISTIC_FALLBACK",
            status="SUCCESS",
            output=fallback_output,
            evidence_refs=fallback_output.evidence_refs,
            model_provenance={
                "model": "deterministic-baseline",
                "provider": "deterministic",
            },
            input_hash=input_hash,
            output_hash=output_hash,
            latency_ms=latency_ms,
        )

    def _build_deterministic_policy_sla(
        self,
        incident_id: int,
        decision_id: str,
        decision_verdict: str,
        active_obligations: List[str],
        active_prohibitions: List[str],
        sla_status: str,
        cutoff_type: str,
        time_remaining_seconds: int,
        sla_context: Optional[Dict[str, Any]],
        envelope: AgentContextEnvelope,
    ) -> PolicySLAOutput:
        """Deterministic policy and SLA summary baseline."""
        policy_summary = (
            f"Authoritative Policy Engine evaluated incident #{incident_id} with verdict '{decision_verdict}'. "
            "Remediation is governed by candidate-only immutability. Dual-control approval is required before release."
        )

        applicable_contracts = (sla_context or {}).get("contract_refs", ["SLA-PARTNER-CORE-01"])
        risk_factors = []
        if sla_status == "AT_RISK":
            risk_factors.append("Processing window expiring within 30 minutes; risk of missed daily bank cut-off.")
        if decision_verdict == "DENY":
            risk_factors.append("Deterministic policy engine issued DENY on current state.")

        return PolicySLAOutput(
            schema_version="1.0",
            authoritative_policy_decision_refs=[decision_id],
            policy_summary=policy_summary,
            active_obligations=active_obligations,
            active_prohibitions=active_prohibitions,
            sla_status=sla_status,
            cutoff_type=cutoff_type,
            time_remaining_seconds=time_remaining_seconds,
            applicable_contract_refs=applicable_contracts,
            risk_factors=risk_factors,
            unknowns=[],
            escalation_required=sla_status == "AT_RISK" or decision_verdict in ("REQUIRE_HUMAN", "DENY"),
            evidence_refs=[decision_id] + envelope.available_runbooks,
            statement="The AI Policy/SLA analyst operates in a read-only capacity and has made no system state changes.",
        )
