"""IncidentCommanderAgent — Governed Lead Investigation & Delegation Commander (SGACA Phase P06.5).

Autonomy Level A1 (Investigate / Plan Only).
Zero authority to release files, approve waivers, or mutate system state.
Uses actual Google ADK Agent and Runner runtime primitives.
"""

from __future__ import annotations

import logging
from typing import Any, Dict, Optional

import google.adk.agents as adk_agents
import google.adk.runners as adk_runners
from contracts.diagnosis import DiagnosisOutput
from contracts.manifests import FIXED_AGENT_ROSTER, validate_agent_roster_membership
from contracts.orchestration import (
    CommanderPlan,
    CommanderSynthesis,
    SpecialistResult,
    WorkflowAuditMetadata,
)
from contracts.policy_sla import PolicySLAOutput
from guardrails.boundary import GuardedModelBoundary
from guardrails.evidence import AuthorizedEvidenceSet
from models.envelope import AgentContextEnvelope

logger = logging.getLogger("sentinel.ai.commander")


class IncidentCommanderAgent:
    """Governed A1 Incident Commander coordinating the P06.5 specialist fleet."""

    def __init__(self, model_name: str = "gemini-3.5-flash"):
        self.manifest = FIXED_AGENT_ROSTER["IncidentCommanderAgent"]
        self.model_name = model_name

        self.boundary = GuardedModelBoundary(default_model=self.model_name)
        # Real Google ADK Agent & Runner Runtime Objects
        self.adk_agent = adk_agents.Agent(
            name=self.manifest.name,
            model=self.model_name,
            instruction=(
                "You are the SentinelFlow Incident Commander Agent.\n"
                "You classify operational incidents, create structured investigation plans, "
                "and synthesize specialist findings.\n"
                "You operate in a strictly READ-ONLY capacity and have no authority to mutate or release files."
            ),
            output_key="commander_plan",
        )
        self.adk_runner = adk_runners.InMemoryRunner(agent=self.adk_agent)

    def create_plan(
        self,
        envelope: AgentContextEnvelope,
        trigger_type: str = "ARTIFACT_QUARANTINED",
    ) -> CommanderPlan:
        """Generates a bounded, structured delegation plan for the operational incident."""
        has_blocking = any(f.severity == "BLOCKING" for f in envelope.findings)
        is_return_risk = (
            trigger_type
            in (
                "RETURN_RECEIVED",
                "RETURN_EVENT_OBSERVED",
                "RETURN_RISK_ANALYSIS",
                "RETURN_SURGE_DETECTED",
            )
            or "return_event_ref" in getattr(envelope, "metadata", {})
            or getattr(envelope, "return_event_ref", None) is not None
        )

        if is_return_risk:
            workflow_class = "RETURN_RISK_ANALYSIS"
            default_specialists = ["DiagnosisAgent", "PolicySLAAgent", "ReturnRiskAgent"]
        elif has_blocking:
            workflow_class = "QUARANTINE_INVESTIGATION"
            default_specialists = ["DiagnosisAgent", "PolicySLAAgent"]
        else:
            workflow_class = "FORMAT_ERROR"
            default_specialists = ["DiagnosisAgent", "PolicySLAAgent"]

        def _fallback(env: AgentContextEnvelope, auth_set: Any) -> CommanderPlan:
            return CommanderPlan(
                schema_version="1.0",
                workflow_class=workflow_class,
                selected_specialists=default_specialists,
                reason_codes=["EVENT_TRIGGERED_INVESTIGATION", "PRE_LEDGER_FINDINGS_DETECTED"],
                evidence_requirements=["FINDING_IDS", "RUNBOOK_CITATIONS", "POLICY_DECISION_ID"],
                parallelizable=True,
                remediation_eligible=has_blocking and not is_return_risk,
                human_attention_required=False,
                policy_bundle_hash=envelope.policy_version or "default/1",
                artifact_sha256=envelope.artifact_sha256,
                next_stage="READY_FOR_REMEDIATION"
                if (has_blocking and not is_return_risk)
                else "HUMAN_AUTHORIZATION_REQUIRED",
            )

        commander_instruction = (
            "ROLE SPECIFIC INSTRUCTION (IncidentCommanderAgent):\n"
            "You are the root Incident Commander. Classify this event and select specialist agents.\n"
            f"FIXED ROSTER ALLOWLIST: {list(FIXED_AGENT_ROSTER.keys())}\n"
            "INVARIANT: You can ONLY delegate to registered specialists ('DiagnosisAgent', 'PolicySLAAgent', 'ReturnRiskAgent'). Never invent agent names."
        )

        res = self.boundary.invoke(
            envelope=envelope,
            response_schema=CommanderPlan,
            role_system_prompt=commander_instruction,
            fallback_fn=_fallback,
            strict_grounding=False,
        )

        if res.success and res.output:
            plan = res.output
            plan.policy_bundle_hash = envelope.policy_version or "default/1"
            plan.artifact_sha256 = envelope.artifact_sha256
            # Enforce roster membership validation
            valid_specialists = [
                s for s in plan.selected_specialists if validate_agent_roster_membership(s)
            ]
            if not valid_specialists:
                valid_specialists = default_specialists
            plan.selected_specialists = valid_specialists
            return plan

        return _fallback(envelope, None)

        # 2. Deterministic Fallback Planning Baseline
        has_blocking = any(f.severity == "BLOCKING" for f in envelope.findings)
        workflow_class = "QUARANTINE_INVESTIGATION" if has_blocking else "FORMAT_ERROR"

        return CommanderPlan(
            schema_version="1.0",
            workflow_class=workflow_class,
            selected_specialists=["DiagnosisAgent", "PolicySLAAgent"],
            reason_codes=["EVENT_TRIGGERED_INVESTIGATION", "PRE_LEDGER_FINDINGS_DETECTED"],
            evidence_requirements=["FINDING_IDS", "RUNBOOK_CITATIONS", "POLICY_DECISION_ID"],
            parallelizable=True,
            remediation_eligible=has_blocking,
            human_attention_required=False,
            policy_bundle_hash=envelope.policy_version or "default/1",
            artifact_sha256=envelope.artifact_sha256,
            next_stage="READY_FOR_REMEDIATION" if has_blocking else "HUMAN_AUTHORIZATION_REQUIRED",
        )

    def synthesize(
        self,
        envelope: AgentContextEnvelope,
        plan: CommanderPlan,
        diagnosis_result: Optional[SpecialistResult[DiagnosisOutput]],
        policy_sla_result: Optional[SpecialistResult[PolicySLAOutput]],
        authoritative_policy_decision: Optional[Dict[str, Any]] = None,
        total_latency_ms: float = 0.0,
    ) -> CommanderSynthesis:
        """Synthesizes structured outputs from parallel specialists into an authoritative outcome.

        Section 7 Invariants:
        - PolicyDecision == DENY -> outcome = POLICY_BLOCKED (human click cannot override DENY)
        - PolicyDecision == REQUIRE_HUMAN -> outcome = HUMAN_AUTHORIZATION_REQUIRED
        - PolicyDecision == ALLOW / ALLOW_WITH_OBLIGATIONS -> outcome = READY_FOR_REMEDIATION (if eligible)
        - Evidence-Union Grounding Invariant: Commander evidence must belong strictly to
          Union(WorkflowAuthorizedEvidence, VerifiedSpecialistEvidence).
        """
        workflow_id = (
            envelope.workflow_id or f"wf-synth-{envelope.tenant_id}-{envelope.incident_id}"
        )

        # 1. Build Authorized Evidence Union
        evidence_set = AuthorizedEvidenceSet.from_envelope(envelope.model_dump())
        if diagnosis_result and diagnosis_result.status == "SUCCESS" and diagnosis_result.output:
            for ref in diagnosis_result.output.evidence_refs:
                evidence_set.add_reference(ref)
        if policy_sla_result and policy_sla_result.status == "SUCCESS" and policy_sla_result.output:
            for ref in policy_sla_result.output.evidence_refs:
                evidence_set.add_reference(ref)

        # 2. Check Disagreement between PolicySLAAgent and Authoritative Policy Engine
        disagreement_count = 0
        policy_verdict = (
            (authoritative_policy_decision or {}).get("decision", "REQUIRE_HUMAN").upper()
        )

        if policy_sla_result and policy_sla_result.output:
            if "ALLOW" in policy_sla_result.output.policy_summary and policy_verdict == "DENY":
                disagreement_count += 1
                logger.warning(
                    "Policy disagreement detected: PolicySLAAgent reported ALLOW while PolicyEngine issued DENY. "
                    "Deterministic Policy Engine strictly wins."
                )

        # 3. Determine Synthesized Outcome & Summary
        has_diag_success = bool(
            diagnosis_result and diagnosis_result.status == "SUCCESS" and diagnosis_result.output
        )
        has_policy_success = bool(
            policy_sla_result and policy_sla_result.status == "SUCCESS" and policy_sla_result.output
        )

        human_attention = False

        if not has_diag_success and not has_policy_success:
            outcome = "UNRESOLVED"
            human_attention = True
            synthesis_summary = "Both specialist investigations failed or timed out. Human supervisor escalation required."
        elif not has_diag_success or not has_policy_success:
            outcome = "PARTIAL_SPECIALIST_FAILURE"
            human_attention = True
            failed_name = "DiagnosisAgent" if not has_diag_success else "PolicySLAAgent"
            synthesis_summary = (
                f"Partial specialist failure: {failed_name} encountered an error or timeout. "
                "Investigation cannot conclude autonomously."
            )
        else:
            diag_out = diagnosis_result.output  # type: ignore

            if policy_verdict == "DENY":
                outcome = "POLICY_BLOCKED"
                human_attention = True  # Attached for review only; cannot relax DENY
                synthesis_summary = (
                    f"Incident #{envelope.incident_id} is POLICY_BLOCKED by authoritative deterministic policy engine. "
                    "No remediation or candidate generation is permitted. Operator investigation required."
                )
            elif policy_verdict == "REQUIRE_HUMAN":
                outcome = "HUMAN_AUTHORIZATION_REQUIRED"
                human_attention = True
                synthesis_summary = (
                    f"Incident #{envelope.incident_id} requires HUMAN_AUTHORIZATION_REQUIRED under governing policy rules. "
                    "Dual-control human approval is required before proceeding."
                )
            elif diag_out.remediation_eligibility and policy_verdict in (
                "ALLOW",
                "ALLOW_WITH_OBLIGATIONS",
            ):
                outcome = "READY_FOR_REMEDIATION"
                synthesis_summary = (
                    f"Investigation complete for Incident #{envelope.incident_id}. Root cause identified as "
                    f"'{diag_out.classification}'. Remediation eligible as derived artifact under governing policy."
                )
            else:
                outcome = "HUMAN_AUTHORIZATION_REQUIRED"
                human_attention = True
                synthesis_summary = (
                    f"Investigation complete for Incident #{envelope.incident_id}. Policy constraints or non-remediable "
                    f"findings require dual-control operator review before any corrective action."
                )

        # 4. Aggregate evidence references
        unified_evidence = list(evidence_set.references)

        # 5. Determine Overall Execution Source
        diag_source = diagnosis_result.execution_source if diagnosis_result else "NOT_RUN"
        policy_source = policy_sla_result.execution_source if policy_sla_result else "NOT_RUN"

        if diag_source == "LIVE_GEMINI" and policy_source == "LIVE_GEMINI":
            exec_source = "LIVE_GEMINI"
        elif diag_source == "DETERMINISTIC_FALLBACK" and policy_source == "DETERMINISTIC_FALLBACK":
            exec_source = "DETERMINISTIC_FALLBACK"
        elif "LIVE_GEMINI" in [diag_source, policy_source]:
            exec_source = "MIXED_NOT_LIVE"
        else:
            exec_source = "DETERMINISTIC_FALLBACK"

        audit = WorkflowAuditMetadata(
            workflow_id=workflow_id,
            execution_source=exec_source,
            total_latency_ms=total_latency_ms,
            total_model_calls=(1 if diag_source == "LIVE_GEMINI" else 0)
            + (1 if policy_source == "LIVE_GEMINI" else 0),
            total_tool_calls=len(diagnosis_result.tool_invocation_refs if diagnosis_result else [])
            + len(policy_sla_result.tool_invocation_refs if policy_sla_result else []),
            agent_policy_disagreement_count=disagreement_count,
            trace_id=envelope.trace_id,
        )

        return CommanderSynthesis(
            schema_version="1.0",
            workflow_id=workflow_id,
            incident_id=envelope.incident_id,
            tenant_id=envelope.tenant_id,
            plan=plan,
            diagnosis_result=diagnosis_result,
            policy_sla_result=policy_sla_result,
            synthesis_summary=synthesis_summary,
            outcome=outcome,
            human_attention_required=human_attention,
            evidence_refs=unified_evidence,
            statement="The AI incident commander operates in a read-only capacity and has made no system state changes.",
            audit=audit,
        )
