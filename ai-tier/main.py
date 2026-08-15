from fastapi import FastAPI
from pydantic import BaseModel
from typing import List, Optional
import time
import os
import sys

# Ensure modules can be imported
sys.path.append(os.path.dirname(__file__))
from evals.runner import run_adversarial_evals
from llm_client import generate_ai_analysis
import agent_hub_tools as hub_tools
from swarm import execute_multi_agent_swarm, SwarmDeliberationResult

app = FastAPI(title="Sentinel Flow AI Analyst Tier", version="1.0.0")

class AnalyzeRequest(BaseModel):
    file_id: int
    findings: List[str]
    raw_data: str

class SwarmRequest(BaseModel):
    incident_id: int
    file_id: int
    findings: List[str]
    raw_data: str

class ActionProposal(BaseModel):
    type: str
    description: str

class AnalystResponse(BaseModel):
    summary: str
    citations: List[str]
    proposed_actions: List[ActionProposal]
    confidence: float
    provider: Optional[str] = "Astra 2.0 Engine"

@app.get("/health")
def health_check():
    return {"status": "healthy", "service": "sentinel-ai-tier", "engine": "Astra 2.0 RRR Standard"}

@app.get("/evals/run")
def get_evals_summary():
    return run_adversarial_evals()

@app.post("/analyze", response_model=AnalystResponse)
def analyze_exception(request: AnalyzeRequest):
    time.sleep(0.3) # Micro latency for UI progress demonstration
    res = generate_ai_analysis(request.file_id, request.findings, request.raw_data)
    return AnalystResponse(**res)

@app.post("/swarm/deliberate", response_model=SwarmDeliberationResult)
def run_swarm_deliberation(request: SwarmRequest):
    return execute_multi_agent_swarm(request.incident_id, request.file_id, request.findings, request.raw_data)

@app.get("/tools/assets")
def get_agent_assets():
    return hub_tools.list_assets()

@app.get("/tools/schema/{asset_id}")
def get_agent_schema(asset_id: str):
    return hub_tools.get_schema_snapshot(asset_id)

@app.get("/tools/sample/{asset_id}")
def get_agent_sample(asset_id: str):
    return hub_tools.get_masked_sample(asset_id, ["approved"])

