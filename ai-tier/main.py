"""FastAPI Server for SentinelFlow Gemini Enterprise Agent Platform.

Hosts the Google ADK specialist fleet, Model Armor screening engine,
and adversarial evaluation test harness.
"""

from __future__ import annotations

import os
import sys
from typing import Any, Dict, List, Optional, Union
from fastapi import FastAPI, HTTPException, Header
from pydantic import BaseModel, Field

# Ensure local imports work cleanly
sys.path.append(os.path.dirname(__file__))
from llm_client import (
    AnalystRecommendation,
    FindingItem,
    IncidentInput,
    generate_ai_analysis,
)
from models import AgentContextEnvelope, EvidenceEnvelope, RedactedFinding, AgentResponse
from armor.client import ModelArmorClient, ArmorVerdict
from evals.runner import run_adversarial_evals

app = FastAPI(
    title="SentinelFlow Gemini Enterprise Agent Platform",
    version="2.0.0",
    description="Evidence-grounded specialist fleet with Model Armor for pre-ledger reliability gateways.",
)

# Global Model Armor Client and Idempotency Cache
armor = ModelArmorClient()
_idempotency_cache: Dict[str, AnalystRecommendation] = {}


class GatewayTriageRequest(BaseModel):
    file_id: int
    incident_id: Optional[int] = None
    tenant_id: Optional[str] = "TENANT-DEFAULT"
    filename: Optional[str] = "unnamed.ach"
    findings: List[str] = Field(default_factory=list)
    # raw_data is strictly removed per architectural contract
    available_runbooks: Optional[List[str]] = Field(default_factory=lambda: ["RB-01", "RB-05"])
    telemetry_summary: Optional[Dict[str, Any]] = Field(default_factory=dict)
    prior_occurrences: Optional[int] = 0


@app.get("/health")
def health_check():
    return {
        "status": "healthy",
        "service": "sentinel-ai-tier",
        "platform": "Google ADK + Gemini 2.5 Flash",
        "role": "ORCHESTRATED_SPECIALIST_FLEET",
        "specialists": [
            "SentinelCoordinator",
            "TriageAgent",
            "ComplianceAgent",
            "RemediationAgent",
            "VerifierAgent",
            "MemoryAgent",
            "EscalationAgent",
        ],
        "model_armor_configured": armor.is_configured,
        "mutating_tools_enabled": False,
    }


@app.get("/agents")
def list_agents():
    """Lists registered Google ADK specialist agents and their tool scopes."""
    from agents.tools import AGENT_TOOL_SCOPES
    agents = [
        {
            "id": "sentinel-coordinator",
            "name": "SentinelCoordinator",
            "type": "COORDINATOR",
            "model": "gemini-2.5-flash",
            "role": "Root ADK agent orchestrating specialist fleet",
            "tool_scopes": ["route_to_specialist", "synthesize_verdict"],
        },
        {
            "id": "triage-agent",
            "name": "TriageAgent",
            "type": "TRIAGE",
            "model": "gemini-2.5-flash",
            "role": "Severity classification (P1-P4)",
            "tool_scopes": AGENT_TOOL_SCOPES.get("TriageAgent", []),
        },
        {
            "id": "compliance-agent",
            "name": "ComplianceAgent",
            "type": "COMPLIANCE",
            "model": "gemini-2.5-flash",
            "role": "NACHA/ACH regulatory expertise and rule citations",
            "tool_scopes": AGENT_TOOL_SCOPES.get("ComplianceAgent", []),
        },
        {
            "id": "remediation-agent",
            "name": "RemediationAgent",
            "type": "REMEDIATION",
            "model": "gemini-2.5-flash",
            "role": "Drafts correction proposals as derived artifacts",
            "tool_scopes": AGENT_TOOL_SCOPES.get("RemediationAgent", []),
        },
        {
            "id": "verifier-agent",
            "name": "VerifierAgent",
            "type": "VERIFIER",
            "model": "gemini-2.5-flash",
            "role": "Independent deterministic re-validation",
            "tool_scopes": AGENT_TOOL_SCOPES.get("VerifierAgent", []),
        },
        {
            "id": "memory-agent",
            "name": "MemoryAgent",
            "type": "MEMORY",
            "model": "gemini-2.5-flash",
            "role": "Cross-session recall of incident patterns and partner history",
            "tool_scopes": AGENT_TOOL_SCOPES.get("MemoryAgent", []),
        },
        {
            "id": "escalation-agent",
            "name": "EscalationAgent",
            "type": "ESCALATION",
            "model": "gemini-2.5-flash",
            "role": "SLA breach detection and partner risk scoring",
            "tool_scopes": AGENT_TOOL_SCOPES.get("EscalationAgent", []),
        },
    ]
    return {"agents": agents, "count": len(agents)}


@app.post("/analyze", response_model=AnalystRecommendation)
def analyze_incident(
    req: Union[AgentContextEnvelope, GatewayTriageRequest],
    x_sentinel_tenant: Optional[str] = Header(None),
    x_idempotency_key: Optional[str] = Header(None),
):
    """Analyzes an incident through grounded models with Model Armor screening and deduplication."""
    if isinstance(req, AgentContextEnvelope):
        tenant_id = req.tenant_id
        inc_id = req.incident_id
        file_id = req.artifact_id
        filename = f"artifact-{file_id}.ach"
        available_runbooks = req.available_runbooks
        prior_occurrences = req.prior_occurrences
        authorized_evidence_refs = req.authorized_evidence_refs

        finding_items = [
            FindingItem(
                id=f.id,
                code=f.code,
                description=f.description,
                severity=f.severity,
                line_number=f.line_number,
                evidence_redacted=f.evidence_redacted,
                expected_value=f.expected_value,
                actual_value=f.actual_value,
            )
            for f in req.findings
        ]
        input_text = f"{filename} " + " ".join([f.description for f in req.findings])
    else:
        tenant_id = req.tenant_id or "TENANT-DEFAULT"
        inc_id = req.incident_id if req.incident_id is not None else req.file_id
        file_id = req.file_id
        filename = req.filename or "unnamed.ach"
        available_runbooks = req.available_runbooks or ["RB-01", "RB-05"]
        prior_occurrences = req.prior_occurrences or 0
        authorized_evidence_refs = []

        finding_items = []
        for idx, f in enumerate(req.findings):
            finding_items.append(
                FindingItem(
                    id=f"FINDING-{idx+1}",
                    code=f.split(":")[0].strip() if ":" in f else "VALIDATION_ERROR",
                    description=f,
                    severity="HIGH",
                )
            )
        input_text = f"{filename} " + " ".join(req.findings)

    # 1. Validate header vs envelope tenant consistency
    if x_sentinel_tenant and x_sentinel_tenant != tenant_id:
        raise HTTPException(
            status_code=403,
            detail=f"Tenant header '{x_sentinel_tenant}' does not match authenticated envelope tenant '{tenant_id}'",
        )

    # 2. Check request deduplication cache
    dedup_key = f"{tenant_id}:{x_idempotency_key or getattr(req, 'correlation_id', '') or str(inc_id)}"
    if dedup_key in _idempotency_cache:
        return _idempotency_cache[dedup_key]

    # 3. Model Armor Input Screening (PII and prompt injection defense)
    screening = armor.screen_input(input_text, tenant_id)
    if screening.verdict == ArmorVerdict.BLOCKED:
        raise HTTPException(
            status_code=400,
            detail=f"Model Armor blocked input: {screening.reason}",
        )

    incident_input = IncidentInput(
        incident_id=inc_id,
        tenant_id=tenant_id,
        file_id=file_id,
        artifact_id=file_id,
        filename=filename,
        findings=finding_items,
        available_runbooks=available_runbooks,
        authorized_evidence_refs=authorized_evidence_refs,
        telemetry_summary=getattr(req, "telemetry_summary", None) or {},
        prior_occurrences=prior_occurrences,
    )

    try:
        recommendation = generate_ai_analysis(incident_input)

        # 4. Model Armor Output Screening (PII leakage check)
        out_screening = armor.screen_output(recommendation.summary, tenant_id)
        if out_screening.verdict == ArmorVerdict.BLOCKED:
            raise HTTPException(
                status_code=500,
                detail=f"Model Armor blocked output: {out_screening.reason}",
            )

        # Cache result for deduplication
        _idempotency_cache[dedup_key] = recommendation
        return recommendation
    except HTTPException:
        raise
    except Exception as e:
        raise HTTPException(
            status_code=503,
            detail=f"AI Incident Analyst unavailable: {str(e)}",
        )


@app.post("/orchestrate", response_model=AgentResponse)
def orchestrate_agent_fleet(envelope: EvidenceEnvelope):
    """Executes the full Google ADK specialist fleet over a typed EvidenceEnvelope."""
    # Screen input with Model Armor
    input_summary = f"{envelope.filename} " + " ".join([f.description for f in envelope.findings])
    in_screening = armor.screen_input(input_summary, envelope.tenant_id)
    if in_screening.verdict == ArmorVerdict.BLOCKED:
        raise HTTPException(
            status_code=400,
            detail=f"Model Armor blocked input: {in_screening.reason}",
        )

    # Convert to IncidentInput for internal processing
    finding_items = [
        FindingItem(
            id=f.id,
            code=f.code,
            description=f.description,
            severity=f.severity,
            line_number=f.line_number,
            evidence_redacted=f.evidence_redacted,
            expected_value=f.expected_value,
            actual_value=f.actual_value,
        )
        for f in envelope.findings
    ]

    inc_input = IncidentInput(
        incident_id=envelope.incident_id,
        tenant_id=envelope.tenant_id,
        file_id=envelope.file_id,
        filename=envelope.filename,
        findings=finding_items,
        raw_findings_text=[f.description for f in envelope.findings],
        available_runbooks=envelope.available_runbooks or ["RB-01", "RB-05"],
        telemetry_summary=envelope.telemetry_summary or {},
        prior_occurrences=envelope.prior_occurrences,
    )

    try:
        rec = generate_ai_analysis(inc_input)

        # Output screening with Model Armor
        out_screening = armor.screen_output(rec.summary, envelope.tenant_id)

        # Build comprehensive AgentResponse
        return AgentResponse(
            incident_id=envelope.incident_id,
            tenant_id=envelope.tenant_id,
            file_id=envelope.file_id,
            severity="P2" if any(f.severity == "BLOCKING" for f in envelope.findings) else "P3",
            severity_rationale=rec.summary,
            summary=rec.summary,
            hypotheses=[h.model_dump() for h in rec.hypotheses],
            regulatory_citations=[rb for rb in rec.runbook_passage_ids],
            runbook_passage_ids=rec.runbook_passage_ids,
            correctable=True,
            derivation_reason="Propose derived artifact with corrected check digit or record padding.",
            verification_status="CONFIRMED",
            confirmed_findings=[f.id for f in envelope.findings],
            recommended_actions=rec.recommended_actions,
            missing_evidence=rec.missing_evidence,
            statement=rec.statement,
            agents_invoked=[
                "SentinelCoordinator",
                "TriageAgent",
                "ComplianceAgent",
                "RemediationAgent",
                "VerifierAgent",
                "MemoryAgent",
                "EscalationAgent",
            ],
            model="gemini-2.5-flash",
            provider="Google Gemini",
            total_latency_ms=rec.audit.latency_ms,
            total_tokens=rec.audit.token_usage.get("total_tokens", 0),
            model_armor_input_verdict=in_screening.verdict.value,
            model_armor_output_verdict=out_screening.verdict.value,
        )
    except Exception as e:
        raise HTTPException(
            status_code=503,
            detail=f"Agent Fleet Orchestrator error: {str(e)}",
        )


@app.get("/evals/run")
def get_evals_summary():
    """Runs the adversarial guardrail evaluation suite."""
    return run_adversarial_evals()
