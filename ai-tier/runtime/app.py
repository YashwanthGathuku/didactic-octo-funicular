"""Local HTTP adapter for SentinelFlow's Agent Platform package.

IMPORTANT P11.5 TRUTH BOUNDARY
-----------------------------
This FastAPI module is a local/development adapter and health surface.  It is
*not* itself Google's ``vertexai.agent_engines.AdkApp`` and it does not claim a
managed Agent Runtime execution succeeded merely because an HTTP route was hit.

The real deployable Agent Runtime object is built in ``runtime.managed_adk``.

Formal invariants:
- AgentRuntime != WorkflowAuthority
- AgentRuntimeSessionID != AgentWorkflowID
- RegistryContains(agent) != SentinelFlowRosterAllows(agent)
- Local adapter metadata != managed execution proof
"""

from __future__ import annotations

import logging
import os
from typing import Any, Dict, Optional

from fastapi import FastAPI, HTTPException
from fastapi.responses import JSONResponse

from agents.commander import IncidentCommanderAgent
from agents.diagnosis import DiagnosisAgent
from agents.memory_agent import MemoryAgent
from agents.policy_sla import PolicySLAAgent
from agents.remediation import RemediationAgent
from agents.return_risk import ReturnRiskAgent
from agents.verifier import VerifierAgent
from contracts.manifests import FIXED_AGENT_ROSTER, validate_agent_roster_membership
from observability.telemetry import configure_agent_observability, get_tracer
from runtime.identity import AgentIdentityProvider

logger = logging.getLogger("sentinel.runtime.app")


class SentinelFlowLocalRuntimeAdapter:
    """Development adapter exposing fixed-roster metadata without fake execution."""

    def __init__(self, project_id: Optional[str] = None, region: str = "us-central1"):
        self.project_id = project_id or os.environ.get("GOOGLE_CLOUD_PROJECT", "telos-agent")
        self.region = region or os.environ.get("GOOGLE_CLOUD_LOCATION", "us-central1")
        self.tracer = get_tracer("sentinelflow.runtime.local-adapter")
        self.agents = {
            "IncidentCommanderAgent": IncidentCommanderAgent(),
            "DiagnosisAgent": DiagnosisAgent(),
            "PolicySLAAgent": PolicySLAAgent(),
            "MemoryAgent": MemoryAgent(),
            "RemediationAgent": RemediationAgent(),
            "VerifierAgent": VerifierAgent(),
            "ReturnRiskAgent": ReturnRiskAgent(),
        }

    def get_agent(self, agent_name: str) -> Any:
        validate_agent_roster_membership(agent_name)
        return self.agents[agent_name]

    def describe_agent_step(
        self,
        agent_name: str,
        session_id: Optional[str] = None,
        workflow_id: Optional[str] = None,
    ) -> Dict[str, Any]:
        """Returns local adapter metadata; it intentionally executes no model call."""

        validate_agent_roster_membership(agent_name)
        correlation_id = workflow_id or f"sess-corr-{session_id or 'anon'}"
        identity = AgentIdentityProvider.create_identity_context(
            agent_name=agent_name,
            project_id=self.project_id,
            correlation_id=correlation_id,
        )
        return {
            "agent_name": agent_name,
            "status": "NOT_EXECUTED",
            "execution_source": "LOCAL_RUNTIME_ADAPTER",
            "correlation_id": correlation_id,
            "identity_source": identity.identity_source,
            "workload_principal": identity.workload_principal,
            "output_schema": FIXED_AGENT_ROSTER[agent_name].output_schema_name,
            "managed_runtime_app": "runtime.managed_adk:build_agent_runtime_app",
        }

    # Backward-compatible method name.  Previous P11 code returned COMPLETED
    # without running the agent; P11.5 makes the semantics explicit.
    def execute_agent_step(
        self,
        agent_name: str,
        input_payload: Dict[str, Any],
        session_id: Optional[str] = None,
        workflow_id: Optional[str] = None,
    ) -> Dict[str, Any]:
        del input_payload
        return self.describe_agent_step(agent_name, session_id=session_id, workflow_id=workflow_id)


# Compatibility alias for existing imports.  Do not confuse this local adapter
# with vertexai.agent_engines.AdkApp.
SentinelFlowAdkApp = SentinelFlowLocalRuntimeAdapter


def create_app() -> FastAPI:
    configure_agent_observability()
    runtime_adapter = SentinelFlowLocalRuntimeAdapter()

    app = FastAPI(
        title="SentinelFlow Agent Platform Local Adapter",
        version="1.1.0-p11.5",
        docs_url="/api/docs",
        openapi_url="/api/openapi.json",
    )

    @app.get("/health")
    async def health_check() -> Dict[str, str]:
        return {
            "status": "HEALTHY",
            "service": "sentinelflow-agent-runtime-local-adapter",
            "project": runtime_adapter.project_id,
            "region": runtime_adapter.region,
            "managed_execution": "NOT_PROVEN_BY_THIS_PROCESS",
        }

    @app.get("/api/roster")
    async def get_roster() -> Dict[str, Any]:
        return {"roster": {name: manifest.model_dump() for name, manifest in FIXED_AGENT_ROSTER.items()}}

    @app.post("/api/agents/{agent_name}/describe")
    async def describe_agent(agent_name: str) -> JSONResponse:
        try:
            result = runtime_adapter.describe_agent_step(agent_name)
            return JSONResponse(status_code=200, content=result)
        except ValueError as exc:
            raise HTTPException(status_code=403, detail=str(exc)) from exc

    return app


app = create_app()
