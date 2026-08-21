"""DiagnosisAgent — Governed Read-Only AI Incident Analyst (SGACA P05/P09).

Powered by Google Gemini 3.5 Flash, Google ADK, and Google Cloud Model Armor.
Operates with Autonomy Level A1 (Investigate / Recommend Only).
Zero authority to release, approve, mutate, or settle payments.
"""

from __future__ import annotations

import hashlib
import json
import logging
import os
import time
from typing import Any, Dict, List, Optional
from pydantic import BaseModel, Field

import google.adk.agents as adk_agents
import google.adk.runners as adk_runners
from contracts.diagnosis import (
    AuditMetadata,
    DiagnosisHypothesis,
    DiagnosisOutput,
    DiagnosisRunResponse,
)
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

logger = logging.getLogger("sentinel.agents.diagnosis")

DEFAULT_MODEL = os.getenv("SENTINEL_GEMINI_MODEL", "gemini-3.5-flash")


class DiagnosisAgent:
    """Governed Read-Only Diagnosis Agent."""

    def __init__(
        self,
        model_name: str = DEFAULT_MODEL,
        max_turns: int = 5,
        max_tool_calls: int = 10,
        timeout_seconds: float = 15.0,
        gateway_base_url: str = "http://127.0.0.1:8080",
    ):
        self.model_name = model_name
        self.max_turns = max_turns
        self.max_tool_calls = max_tool_calls
        self.timeout_seconds = timeout_seconds
        self.gateway_base_url = gateway_base_url
        self.boundary = GuardedModelBoundary(default_model=self.model_name)

        # Real Google ADK Agent & Runner Runtime Objects
        self.adk_agent = adk_agents.Agent(
            name="DiagnosisAgent",
            model=self.model_name,
            instruction=(
                "You are the SentinelFlow Diagnosis Agent.\n"
                "You analyze pre-ledger NACHA validation findings and diagnose root causes.\n"
                "You operate in a strictly READ-ONLY capacity and have no authority to mutate or release files."
            ),
            output_key="diagnosis_result",
        )
        self.adk_runner = adk_runners.InMemoryRunner(agent=self.adk_agent)

    def run(self, envelope_data: Dict[str, Any] | AgentContextEnvelope) -> DiagnosisRunResponse:
        """Executes evidence-grounded diagnosis for an incident envelope."""
        start_time = time.time()
        
        if isinstance(envelope_data, dict):
            envelope = AgentContextEnvelope(**envelope_data)
        else:
            envelope = envelope_data

        tenant_id = envelope.tenant_id
        incident_id = envelope.incident_id
        workflow_id = envelope.workflow_id or f"wf-diag-{tenant_id}-{incident_id}"

        # Initialize AuthorizedEvidenceSet from Envelope
        evidence_set = AuthorizedEvidenceSet.from_envelope(envelope.model_dump())

        # Compile partitioned prompt for input hash
        prompt = PromptTrustPartitioner.compile(envelope)
        input_hash = hashlib.sha256(prompt.user_prompt.encode("utf-8")).hexdigest()

        # Check execution mode configuration
        ai_mode = os.getenv("SENTINEL_AI_MODE", "auto").lower()
        google_key = os.getenv("GOOGLE_API_KEY") or os.getenv("GEMINI_API_KEY")

        # Strict LIVE mode check: if live execution is explicitly requested without credentials, fail fast
        if ai_mode == "live" and not google_key:
            latency_ms = (time.time() - start_time) * 1000.0
            return DiagnosisRunResponse(
                workflow_id=workflow_id,
                incident_id=incident_id,
                tenant_id=tenant_id,
                status="PROVIDER_UNAVAILABLE",
                output=None,
                audit=AuditMetadata(
                    model=self.model_name,
                    provider="google",
                    latency_ms=latency_ms,
                    execution_source="LIVE_GEMINI",
                    input_hash=input_hash,
                    grounding_verdict="UNGROUNDED_REJECTED",
                ),
                error={
                    "code": "MISSING_CREDENTIALS",
                    "message": "GOOGLE_API_KEY is required for LIVE_GEMINI mode but was not found in environment",
                },
            )

        # Fallback closure
        def _fallback(env: AgentContextEnvelope, auth_set: AuthorizedEvidenceSet) -> DiagnosisOutput:
            return self._build_deterministic_diagnosis(env, auth_set)

        inv_result = self.boundary.invoke(
            envelope=envelope,
            response_schema=DiagnosisOutput,
            evidence_set=evidence_set,
            fallback_fn=_fallback,
            strict_grounding=True,
        )

        latency_ms = inv_result.audit.latency_ms

        if inv_result.success and inv_result.output:
            out = inv_result.output
            return DiagnosisRunResponse(
                workflow_id=workflow_id,
                incident_id=incident_id,
                tenant_id=tenant_id,
                status="SUCCESS",
                output=out,
                audit=AuditMetadata(
                    model=inv_result.audit.model_name,
                    provider=inv_result.audit.provider,
                    latency_ms=latency_ms,
                    token_usage={
                        "prompt_tokens": inv_result.audit.prompt_tokens,
                        "completion_tokens": inv_result.audit.completion_tokens,
                        "total_tokens": inv_result.audit.total_tokens,
                    },
                    estimated_cost_usd=inv_result.audit.estimated_cost_usd,
                    execution_source=inv_result.audit.execution_source,
                    adk_version="2.7.1",
                    genai_version="2.18.1",
                    input_hash=inv_result.audit.post_guardrail_input_hash,
                    output_hash=inv_result.audit.post_guardrail_output_hash,
                    grounding_verdict=inv_result.audit.grounding_verdict,
                ),
            )

        # Failed or blocked by guardrail / live execution error
        status_code = inv_result.error_code or "FAILED"
        if status_code in ("GROUNDING_VIOLATION", "PROMPT_SECURITY_BLOCKED", "GUARDRAIL_UNAVAILABLE"):
            return DiagnosisRunResponse(
                workflow_id=workflow_id,
                incident_id=incident_id,
                tenant_id=tenant_id,
                status=status_code,
                output=None,
                audit=AuditMetadata(
                    model=self.model_name,
                    provider="google",
                    latency_ms=latency_ms,
                    execution_source="LIVE_GEMINI",
                    input_hash=inv_result.audit.post_guardrail_input_hash or input_hash,
                    grounding_verdict="UNGROUNDED_REJECTED",
                ),
                error={"code": status_code, "message": inv_result.error_message or "Guardrail screening or model execution blocked."},
            )

        # Deterministic fallback response
        fallback_output = self._build_deterministic_diagnosis(envelope, evidence_set)
        output_json = fallback_output.model_dump_json()
        output_hash = hashlib.sha256(output_json.encode("utf-8")).hexdigest()

        return DiagnosisRunResponse(
            workflow_id=workflow_id,
            incident_id=incident_id,
            tenant_id=tenant_id,
            status="SUCCESS",
            output=fallback_output,
            audit=AuditMetadata(
                model="deterministic-baseline",
                provider="deterministic",
                latency_ms=latency_ms,
                token_usage={"prompt_tokens": 0, "completion_tokens": 0, "total_tokens": 0},
                estimated_cost_usd=0.0,
                execution_source="DETERMINISTIC_FALLBACK",
                input_hash=input_hash,
                output_hash=output_hash,
                grounding_verdict="VERIFIED",
            ),
        )

    def _build_deterministic_diagnosis(
        self,
        envelope: AgentContextEnvelope,
        evidence_set: AuthorizedEvidenceSet,
    ) -> DiagnosisOutput:
        """Deterministic rule-grounded engine baseline."""
        findings_text = " ".join([f.code + " " + f.description for f in envelope.findings]).upper()

        has_hash_mismatch = "HASH" in findings_text or "0802" in findings_text
        has_routing_invalid = "ROUTING" in findings_text or "ABA" in findings_text or "0602" in findings_text
        has_length_truncation = "LENGTH" in findings_text or "RECORDLENGTH" in findings_text or "TRUNCAT" in findings_text or "0001" in findings_text

        hypotheses: List[DiagnosisHypothesis] = []
        evidence_refs: List[str] = [f.id for f in envelope.findings]
        recommended_checks: List[str] = []
        unknowns: List[str] = []
        classification = "PRE_LEDGER_VALIDATION_FAILURE"

        if has_hash_mismatch:
            classification = "ENTRY_HASH_ACCUMULATOR_MISMATCH"
            evidence_refs.append("RUNBOOK-RB-05")
            hypotheses.append(
                DiagnosisHypothesis(
                    hypothesis_id="HYP-1",
                    description="Batch control entry hash accumulator sum does not match individual entry detail record calculations.",
                    evidence_refs=[f.id for f in envelope.findings] + ["RUNBOOK-RB-05"],
                    confidence="HIGH",
                    status="PROPOSED",
                )
            )
            recommended_checks.append("Verify counterparty batch compilation hash calculation logic.")
            recommended_checks.append("Perform dual-control derived artifact review if counterparty retransmission is unavailable.")
            unknowns.append("Has the originating partner updated their ACH batch generation software recently?")
            summary = f"Quarantined File #{envelope.artifact_id} due to deterministic Batch Entry Hash accumulator failure."

        elif has_routing_invalid:
            classification = "INVALID_RDFI_ROUTING_NUMBER"
            evidence_refs.append("RUNBOOK-RB-05")
            hypotheses.append(
                DiagnosisHypothesis(
                    hypothesis_id="HYP-1",
                    description="Receiving DFI transit routing number failed Federal Reserve Modulo-10 checksum validation.",
                    evidence_refs=[f.id for f in envelope.findings] + ["RUNBOOK-RB-05"],
                    confidence="HIGH",
                    status="PROPOSED",
                )
            )
            recommended_checks.append("Inspect entry detail record routing number check digit against FedACH directory.")
            recommended_checks.append("Require supervisor sign-off before proposing derived artifact correction.")
            unknowns.append("Is the destination routing number an active FedACH participant?")
            summary = f"Quarantined File #{envelope.artifact_id} due to invalid RDFI routing number checksum."

        elif has_length_truncation:
            classification = "FIXED_WIDTH_RECORD_LENGTH_VIOLATION"
            evidence_refs.append("RUNBOOK-RB-05")
            hypotheses.append(
                DiagnosisHypothesis(
                    hypothesis_id="HYP-1",
                    description="Record does not conform to the 94-character fixed-width NACHA standard or contains CRLF delimiter issues.",
                    evidence_refs=[f.id for f in envelope.findings] + ["RUNBOOK-RB-05"],
                    confidence="HIGH",
                    status="PROPOSED",
                )
            )
            recommended_checks.append("Examine file line delimiters and character padding.")
            unknowns.append("Check transmission channel encoding settings (UTF-8 vs ASCII).")
            summary = f"Quarantined File #{envelope.artifact_id} due to fixed-width record length truncation."

        else:
            classification = "UNCLASSIFIED_VALIDATION_FINDING"
            evidence_refs.append("RUNBOOK-RB-01")
            confidence_level = "LOW" if len(envelope.findings) == 0 else "MEDIUM"
            hypotheses.append(
                DiagnosisHypothesis(
                    hypothesis_id="HYP-1",
                    description="Ingest processing encountered non-zero validation findings requiring operator review.",
                    evidence_refs=[f.id for f in envelope.findings] + ["RUNBOOK-RB-01"],
                    confidence=confidence_level,
                    status="PROPOSED",
                )
            )
            recommended_checks.append("Review full redacted finding list in operator dashboard.")
            unknowns.append("Request Tier-2 operational investigation.")
            summary = f"Pre-ledger investigation completed with {len(envelope.findings)} findings."

        affected_records = [f"LINE-{f.line_number}" for f in envelope.findings if f.line_number]
        valid_evidence_refs = [r for r in list(dict.fromkeys(evidence_refs)) if evidence_set.contains(r)]

        return DiagnosisOutput(
            schema_version="1.0",
            classification=classification,
            summary=summary,
            hypotheses=hypotheses,
            affected_records=affected_records,
            evidence_refs=valid_evidence_refs,
            unknowns=unknowns,
            recommended_checks=recommended_checks,
            remediation_eligibility=has_hash_mismatch or has_length_truncation,
            escalation_required=has_routing_invalid,
            statement="The AI incident analyst operates in a read-only capacity and has made no system state changes.",
        )
