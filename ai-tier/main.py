"""
FastAPI Server for SentinelFlow Read-Only AI Incident Analyst.
"""

from __future__ import annotations

import os
import sys
from typing import Any, Dict, List, Optional
from fastapi import FastAPI, HTTPException
from pydantic import BaseModel, Field

# Ensure local imports work cleanly
sys.path.append(os.path.dirname(__file__))
from llm_client import (
    AnalystRecommendation,
    FindingItem,
    IncidentInput,
    generate_ai_analysis,
)
from evals.runner import run_adversarial_evals

app = FastAPI(
    title="Sentinel Flow Read-Only AI Incident Analyst",
    version="1.0.0",
    description="Evidence-grounded, read-only AI incident analyst for pre-ledger reliability gateways.",
)


class GatewayTriageRequest(BaseModel):
    file_id: int
    incident_id: Optional[int] = None
    tenant_id: Optional[str] = "TENANT-DEFAULT"
    filename: Optional[str] = "unnamed.ach"
    findings: List[str] = Field(default_factory=list)
    raw_data: Optional[str] = ""
    available_runbooks: Optional[List[str]] = Field(default_factory=lambda: ["RB-01", "RB-05"])
    telemetry_summary: Optional[Dict[str, Any]] = Field(default_factory=dict)
    prior_occurrences: Optional[int] = 0


@app.get("/health")
def health_check():
    return {
        "status": "healthy",
        "service": "sentinel-ai-tier",
        "role": "READ_ONLY_INCIDENT_ANALYST",
        "mutating_tools_enabled": False,
    }


@app.post("/analyze", response_model=AnalystRecommendation)
def analyze_incident(req: GatewayTriageRequest):
    """Analyzes an incident in a strictly read-only, evidence-grounded manner."""
    inc_id = req.incident_id if req.incident_id is not None else req.file_id

    # Parse raw findings into typed finding items if strings were passed
    finding_items: List[FindingItem] = []
    for idx, f in enumerate(req.findings):
        finding_items.append(
            FindingItem(
                id=f"FINDING-{idx+1}",
                code=f.split(":")[0].strip() if ":" in f else "VALIDATION_ERROR",
                description=f,
                severity="HIGH",
            )
        )

    incident_input = IncidentInput(
        incident_id=inc_id,
        tenant_id=req.tenant_id or "TENANT-DEFAULT",
        file_id=req.file_id,
        filename=req.filename,
        findings=finding_items,
        raw_findings_text=req.findings,
        available_runbooks=req.available_runbooks or ["RB-01", "RB-05"],
        telemetry_summary=req.telemetry_summary or {},
        prior_occurrences=req.prior_occurrences or 0,
    )

    try:
        recommendation = generate_ai_analysis(incident_input)
        return recommendation
    except Exception as e:
        raise HTTPException(
            status_code=503,
            detail=f"AI Incident Analyst unavailable: {str(e)}",
        )


@app.get("/evals/run")
def get_evals_summary():
    """Runs the adversarial guardrail evaluation suite."""
    return run_adversarial_evals()
