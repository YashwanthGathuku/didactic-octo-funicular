"""Governed Multi-Agent AI Stage Execution & Orchestration Shell for SentinelFlow (SGACA Phase P06.6).

CRITICAL ARCHITECTURAL INVARIANT:
This module executes bounded AI reasoning stages using Google ADK Agent and ParallelAgent objects.
Authoritative workflow identity, state machine transitions, trigger idempotency, row_versioning,
event journals, and TOCTOU enforcement are owned strictly by the Go Control Plane.
"""

from __future__ import annotations

import concurrent.futures
import hashlib
import json
import logging
import time
from typing import Any, Dict, List, Optional, Tuple

import google.adk.agents as adk_agents
import google.adk.runners as adk_runners
from agents.commander import IncidentCommanderAgent
from agents.diagnosis import DiagnosisAgent
from agents.policy_sla import PolicySLAAgent
from agents.remediation import RemediationAgent
from agents.verifier import VerifierAgent
from contracts.diagnosis import DiagnosisOutput
from contracts.manifests import FIXED_AGENT_ROSTER, validate_agent_roster_membership
from contracts.orchestration import (
    AgentHandoffEnvelope,
    AgentStageRequest,
    AgentStageResponse,
    AgentTriggerEvent,
    CommanderPlan,
    CommanderSynthesis,
    SpecialistResult,
    WorkflowAuditMetadata,
)
from contracts.policy_sla import PolicySLAOutput
from contracts.remediation import RemediationPlan
from contracts.verification import CriticAssessment
from models.envelope import AgentContextEnvelope, RedactedFindingItem
from persistence.store import NonAuthoritativeSessionStore

logger = logging.getLogger("sentinel.ai.orchestrator")


class MultiAgentWorkflowOrchestrator:
    """Governed Multi-Agent Stage Reasoning Shell with Google ADK Primitives."""

    def __init__(
        self,
        commander: Optional[IncidentCommanderAgent] = None,
        diagnosis_agent: Optional[DiagnosisAgent] = None,
        policy_sla_agent: Optional[PolicySLAAgent] = None,
        remediation_agent: Optional[RemediationAgent] = None,
        verifier_agent: Optional[VerifierAgent] = None,
        store: Optional[NonAuthoritativeSessionStore] = None,
    ):
        self.commander = commander or IncidentCommanderAgent()
        self.diagnosis_agent = diagnosis_agent or DiagnosisAgent()
        self.policy_sla_agent = policy_sla_agent or PolicySLAAgent()
        self.remediation_agent = remediation_agent or RemediationAgent()
        self.verifier_agent = verifier_agent or VerifierAgent()
        self.store = store or NonAuthoritativeSessionStore()

        # Real Google ADK ParallelAgent with distinct output keys
        diag_adk = getattr(self.diagnosis_agent, "adk_agent", None)
        policy_adk = getattr(self.policy_sla_agent, "adk_agent", None)
        sub_agents = [a for a in [diag_adk, policy_adk] if a is not None]
        if sub_agents:
            self.adk_parallel_agent = adk_agents.ParallelAgent(
                name="ParallelSpecialists",
                description="Parallel execution container for DiagnosisAgent and PolicySLAAgent",
                sub_agents=sub_agents,
            )
            self.adk_parallel_runner = adk_runners.InMemoryRunner(agent=self.adk_parallel_agent)
        else:
            self.adk_parallel_agent = None
            self.adk_parallel_runner = None

    def execute_stage(self, req: AgentStageRequest) -> AgentStageResponse:
        """Executes a bounded AI stage on behalf of the authoritative Go Control Plane."""
        start_time = time.time()
        workflow_id = req.workflow_id
        tenant_id = req.tenant_id
        incident_id = req.incident_id
        artifact_id = req.artifact_id
        artifact_sha256 = req.artifact_sha256
        policy_bundle_hash = req.policy_bundle_hash or "default/1"
        correlation_id = req.correlation_id or f"corr-{workflow_id}"
        trace_id = req.trace_id or f"trace-{workflow_id}"

        # Convert findings to RedactedFindingItems
        finding_items: List[RedactedFindingItem] = []
        for idx, f in enumerate(req.findings):
            if isinstance(f, dict):
                finding_items.append(RedactedFindingItem(
                    id=f.get("id", f"FINDING-{idx+1}"),
                    code=f.get("code", "0802"),
                    severity=f.get("severity", "BLOCKING"),
                    description=f.get("description", "Validation finding"),
                    line_number=f.get("line_number"),
                ))
            elif isinstance(f, str):
                finding_items.append(RedactedFindingItem(
                    id=f"FINDING-{idx+1}",
                    code="0802",
                    severity="BLOCKING",
                    description=f,
                ))

        envelope = AgentContextEnvelope(
            tenant_id=tenant_id,
            incident_id=incident_id,
            artifact_id=artifact_id,
            artifact_sha256=artifact_sha256,
            policy_version=policy_bundle_hash,
            correlation_id=correlation_id,
            trace_id=trace_id,
            findings=finding_items,
            available_runbooks=req.available_runbooks or ["RB-01", "RB-05"],
            authorized_evidence_refs=req.authorized_evidence_refs,
            workflow_id=workflow_id,
        )

        if req.stage_type == "COMMANDER_PLAN":
            plan = self.commander.create_plan(envelope, trigger_type="ARTIFACT_QUARANTINED")
            plan.policy_bundle_hash = policy_bundle_hash
            plan.artifact_sha256 = artifact_sha256
            latency_ms = (time.time() - start_time) * 1000.0
            return AgentStageResponse(
                stage_type="COMMANDER_PLAN",
                status="SUCCESS",
                workflow_id=workflow_id,
                plan=plan.model_dump(),
                latency_ms=latency_ms,
                execution_source="LOCAL_ADK_DETERMINISTIC",
            )

        elif req.stage_type == "PARALLEL_SPECIALISTS":
            diag_handoff = AgentHandoffEnvelope(
                workflow_id=workflow_id,
                tenant_id=tenant_id,
                source_agent="IncidentCommanderAgent",
                target_agent="DiagnosisAgent",
                incident_id=incident_id,
                artifact_id=artifact_id,
                artifact_sha256=artifact_sha256,
                policy_bundle_hash=policy_bundle_hash,
                authorized_evidence_refs=envelope.authorized_evidence_refs,
                allowed_tools=FIXED_AGENT_ROSTER["DiagnosisAgent"].allowed_tools,
                correlation_id=correlation_id,
                trace_id=trace_id,
                delegation_depth=1,
            )

            policy_handoff = AgentHandoffEnvelope(
                workflow_id=workflow_id,
                tenant_id=tenant_id,
                source_agent="IncidentCommanderAgent",
                target_agent="PolicySLAAgent",
                incident_id=incident_id,
                artifact_id=artifact_id,
                artifact_sha256=artifact_sha256,
                policy_bundle_hash=policy_bundle_hash,
                authorized_evidence_refs=envelope.authorized_evidence_refs,
                allowed_tools=FIXED_AGENT_ROSTER["PolicySLAAgent"].allowed_tools,
                correlation_id=correlation_id,
                trace_id=trace_id,
                delegation_depth=1,
            )

            diag_res: Optional[SpecialistResult[DiagnosisOutput]] = None
            policy_res: Optional[SpecialistResult[PolicySLAOutput]] = None

            def run_diag():
                resp = self.diagnosis_agent.run(envelope)
                return SpecialistResult(
                    agent_name="DiagnosisAgent",
                    agent_version="1.0.0",
                    status="SUCCESS" if resp.status == "SUCCESS" else "FAILED",
                    output=resp.output,
                    evidence_refs=resp.output.evidence_refs if resp.output else [],
                    latency_ms=resp.audit.latency_ms,
                    error=resp.error,
                )

            def run_policy():
                return self.policy_sla_agent.run(
                    policy_handoff,
                    authoritative_policy_decision=req.authoritative_policy_decision,
                    sla_context=req.sla_context,
                )

            with concurrent.futures.ThreadPoolExecutor(max_workers=2) as executor:
                f_diag = executor.submit(run_diag)
                f_policy = executor.submit(run_policy)
                try:
                    diag_res = f_diag.result(timeout=15.0)
                except Exception as e:
                    diag_res = SpecialistResult(
                        agent_name="DiagnosisAgent",
                        status="FAILED",
                        output=None,
                        error={"message": str(e)},
                    )
                try:
                    policy_res = f_policy.result(timeout=15.0)
                except Exception as e:
                    policy_res = SpecialistResult(
                        agent_name="PolicySLAAgent",
                        status="FAILED",
                        output=None,
                        error={"message": str(e)},
                    )

            latency_ms = (time.time() - start_time) * 1000.0
            return AgentStageResponse(
                stage_type="PARALLEL_SPECIALISTS",
                status="SUCCESS",
                workflow_id=workflow_id,
                diagnosis_result=diag_res.model_dump() if diag_res else None,
                policy_sla_result=policy_res.model_dump() if policy_res else None,
                latency_ms=latency_ms,
                execution_source="LOCAL_ADK_DETERMINISTIC",
            )

        elif req.stage_type == "COMMANDER_SYNTHESIS":
            plan = CommanderPlan.model_validate(req.plan) if req.plan else self.commander.create_plan(envelope)
            
            diag_specialist = None
            if req.diagnosis_result:
                try:
                    diag_specialist = SpecialistResult[DiagnosisOutput].model_validate(req.diagnosis_result)
                except Exception:
                    diag_specialist = None

            policy_specialist = None
            if req.policy_sla_result:
                try:
                    policy_specialist = SpecialistResult[PolicySLAOutput].model_validate(req.policy_sla_result)
                except Exception:
                    policy_specialist = None

            total_lat = (time.time() - start_time) * 1000.0
            synthesis = self.commander.synthesize(
                envelope=envelope,
                plan=plan,
                diagnosis_result=diag_specialist,
                policy_sla_result=policy_specialist,
                authoritative_policy_decision=req.authoritative_policy_decision,
                total_latency_ms=total_lat,
            )

            return AgentStageResponse(
                stage_type="COMMANDER_SYNTHESIS",
                status="SUCCESS",
                workflow_id=workflow_id,
                synthesis=synthesis.model_dump(),
                outcome=synthesis.outcome,
                evidence_refs=synthesis.evidence_refs,
                latency_ms=total_lat,
                execution_source="LOCAL_ADK_DETERMINISTIC",
            )

        elif req.stage_type == "REMEDIATION_PLAN":
            plan = self.remediation_agent.run(envelope, attempt_number=req.attempt_number or 1)
            total_lat = (time.time() - start_time) * 1000.0
            return AgentStageResponse(
                stage_type="REMEDIATION_PLAN",
                status="SUCCESS",
                workflow_id=workflow_id,
                remediation_plan=plan.model_dump(),
                evidence_refs=plan.evidence_refs,
                latency_ms=total_lat,
                execution_source="LOCAL_ADK_DETERMINISTIC",
            )

        elif req.stage_type in ("VERIFIER_CRITIC", "STAGE_VERIFIER_CRITIC"):
            assessment = self.verifier_agent.run(req)
            total_lat = (time.time() - start_time) * 1000.0
            return AgentStageResponse(
                stage_type=req.stage_type,
                status="SUCCESS",
                workflow_id=workflow_id,
                critic_assessment=assessment.model_dump(),
                evidence_refs=assessment.evidence_refs,
                latency_ms=total_lat,
                execution_source="LOCAL_ADK_DETERMINISTIC",
            )

        return AgentStageResponse(
            stage_type=req.stage_type,
            status="FAILED",
            workflow_id=workflow_id,
            error_detail=f"Unsupported stage type: {req.stage_type}",
        )

    def run_workflow(
        self,
        envelope: AgentContextEnvelope,
        trigger_event: Optional[AgentTriggerEvent] = None,
        authoritative_policy_decision: Optional[Dict[str, Any]] = None,
        sla_context: Optional[Dict[str, Any]] = None,
        current_policy_bundle_hash: Optional[str] = None,
        current_artifact_sha256: Optional[str] = None,
        max_elapsed_seconds: float = 30.0,
    ) -> CommanderSynthesis:
        """Executes the workflow using non-authoritative local session store for testing."""
        start_time = time.time()
        tenant_id = envelope.tenant_id
        incident_id = envelope.incident_id
        artifact_id = envelope.artifact_id
        artifact_sha256 = envelope.artifact_sha256
        policy_bundle_hash = envelope.policy_version or "default/1"
        correlation_id = envelope.correlation_id
        trace_id = envelope.trace_id

        workflow_type = trigger_event.event_type if trigger_event else "ARTIFACT_QUARANTINED"
        trigger_id = trigger_event.event_id if trigger_event else f"trig-{incident_id}"

        # Non-authoritative local session
        wf_record, created = self.store.get_or_create_workflow(
            tenant_id=tenant_id,
            incident_id=incident_id,
            artifact_id=artifact_id,
            artifact_sha256=artifact_sha256,
            correlation_id=correlation_id,
            workflow_type=workflow_type,
            policy_bundle_hash=policy_bundle_hash,
            trace_id=trace_id,
        )
        workflow_id = wf_record["id"]
        envelope.workflow_id = workflow_id

        # Return cached synthesis if valid
        if wf_record.get("synthesis_json") and not created:
            try:
                cached_synth_dict = json.loads(wf_record["synthesis_json"])
                cached_synth = CommanderSynthesis.model_validate(cached_synth_dict)
                policy_match = (not current_policy_bundle_hash) or (cached_synth.plan.policy_bundle_hash == current_policy_bundle_hash)
                artifact_match = (not current_artifact_sha256) or (cached_synth.plan.artifact_sha256 == current_artifact_sha256)
                
                current_verdict = (authoritative_policy_decision or {}).get("decision", "").upper()
                verdict_match = True
                if current_verdict:
                    if current_verdict == "DENY" and cached_synth.outcome != "POLICY_BLOCKED":
                        verdict_match = False
                    elif current_verdict == "ALLOW" and cached_synth.outcome not in ("READY_FOR_REMEDIATION", "COMPLETED"):
                        verdict_match = False

                if policy_match and artifact_match and verdict_match:
                    return cached_synth
            except Exception:
                pass

        self.store.transition_state(workflow_id, tenant_id, "INVESTIGATING")
        
        # Plan
        plan = self.commander.create_plan(envelope, trigger_type=workflow_type)
        plan.policy_bundle_hash = policy_bundle_hash
        plan.artifact_sha256 = artifact_sha256
        self.store.transition_state(workflow_id, tenant_id, "PLANNING", plan_json=plan.model_dump_json())

        for specialist_name in plan.selected_specialists:
            validate_agent_roster_membership(specialist_name)

        # Stage request for parallel specialists
        stage_req = AgentStageRequest(
            stage_type="PARALLEL_SPECIALISTS",
            workflow_id=workflow_id,
            tenant_id=tenant_id,
            incident_id=incident_id,
            artifact_id=artifact_id,
            artifact_sha256=artifact_sha256,
            policy_bundle_hash=policy_bundle_hash,
            authorized_evidence_refs=envelope.authorized_evidence_refs,
            findings=[f.model_dump() for f in envelope.findings],
            available_runbooks=envelope.available_runbooks,
            authoritative_policy_decision=authoritative_policy_decision,
            sla_context=sla_context,
        )

        stage_resp = self.execute_stage(stage_req)

        diag_res = None
        if stage_resp.diagnosis_result:
            diag_res = SpecialistResult[DiagnosisOutput].model_validate(stage_resp.diagnosis_result)

        policy_res = None
        if stage_resp.policy_sla_result:
            policy_res = SpecialistResult[PolicySLAOutput].model_validate(stage_resp.policy_sla_result)

        total_latency_ms = (time.time() - start_time) * 1000.0

        # TOCTOU checks
        if current_policy_bundle_hash and plan.policy_bundle_hash != current_policy_bundle_hash:
            self.store.transition_state(workflow_id, tenant_id, "UNRESOLVED")
            return CommanderSynthesis(
                schema_version="1.0",
                workflow_id=workflow_id,
                incident_id=incident_id,
                tenant_id=tenant_id,
                plan=plan,
                diagnosis_result=diag_res,
                policy_sla_result=policy_res,
                synthesis_summary="TOCTOU Policy Invalidation: Policy bundle changed during execution. Synthesis aborted.",
                outcome="UNRESOLVED",
                human_attention_required=True,
                evidence_refs=[],
                statement="The AI incident commander operates in a read-only capacity and has made no system state changes.",
                audit=WorkflowAuditMetadata(
                    workflow_id=workflow_id,
                    execution_source="DETERMINISTIC_FALLBACK",
                    total_latency_ms=total_latency_ms,
                    agent_policy_disagreement_count=1,
                    trace_id=trace_id,
                ),
            )

        if current_artifact_sha256 and plan.artifact_sha256 != current_artifact_sha256:
            self.store.transition_state(workflow_id, tenant_id, "UNRESOLVED")
            return CommanderSynthesis(
                schema_version="1.0",
                workflow_id=workflow_id,
                incident_id=incident_id,
                tenant_id=tenant_id,
                plan=plan,
                diagnosis_result=diag_res,
                policy_sla_result=policy_res,
                synthesis_summary="TOCTOU Artifact Mutation: Quarantined artifact modified during execution. Synthesis aborted.",
                outcome="UNRESOLVED",
                human_attention_required=True,
                evidence_refs=[],
                statement="The AI incident commander operates in a read-only capacity and has made no system state changes.",
                audit=WorkflowAuditMetadata(
                    workflow_id=workflow_id,
                    execution_source="DETERMINISTIC_FALLBACK",
                    total_latency_ms=total_latency_ms,
                    agent_policy_disagreement_count=0,
                    trace_id=trace_id,
                ),
            )

        synthesis = self.commander.synthesize(
            envelope=envelope,
            plan=plan,
            diagnosis_result=diag_res,
            policy_sla_result=policy_res,
            authoritative_policy_decision=authoritative_policy_decision,
            total_latency_ms=total_latency_ms,
        )

        state_map = {
            "READY_FOR_REMEDIATION": "COMPLETED",
            "HUMAN_AUTHORIZATION_REQUIRED": "HUMAN_REVIEW",
            "POLICY_BLOCKED": "POLICY_DENIED",
            "PARTIAL_SPECIALIST_FAILURE": "UNRESOLVED",
            "UNRESOLVED": "UNRESOLVED",
        }
        final_state = state_map.get(synthesis.outcome, "UNRESOLVED")

        self.store.transition_state(
            workflow_id=workflow_id,
            tenant_id=tenant_id,
            new_state=final_state,
            synthesis_json=synthesis.model_dump_json(),
        )

        return synthesis
