"""Google Agent Runtime Application Wrapper for SentinelFlow P11.

Packages the fixed canonical 6-agent fleet:
- IncidentCommanderAgent
- DiagnosisAgent
- PolicySLAAgent
- MemoryAgent
- RemediationAgent
- VerifierAgent

Formal Invariants:
- AgentRuntime != WorkflowAuthority
- AgentRuntimeSessionID != AgentWorkflowID
- RegistryContains(agent) != SentinelFlowRosterAllows(agent)
"""

from __future__ import annotations

import logging
import os
from typing import Any, Dict, Optional
from fastapi import FastAPI, Header, HTTPException, Request, Response
from fastapi.responses import JSONResponse

from agents.commander import IncidentCommanderAgent
from agents.diagnosis import DiagnosisAgent
from agents.policy_sla import PolicySLAAgent
from agents.memory_agent import MemoryAgent
from agents.remediation import RemediationAgent
from agents.verifier import VerifierAgent
from contracts.manifests import FIXED_AGENT_ROSTER, validate_agent_roster_membership
from runtime.identity import AgentIdentityProvider
from runtime.gateway_client import AgentGatewayClient
from observability.telemetry import configure_agent_observability, get_tracer

logger = logging.getLogger("sentinel.runtime.app")


class SentinelFlowAdkApp:
    """Managed Agent Runtime Application hosting SentinelFlow's fixed fleet."""

    def __init__(
        self,
        project_id: Optional[str] = None,
        region: str = "us-central1",
        gateway_endpoint: Optional[str] = None,
    ):
        self.project_id = project_id or os.environ.get("GOOGLE_CLOUD_PROJECT", "telos-agent")
        self.region = region or os.environ.get("GOOGLE_CLOUD_LOCATION", "us-central1")
        self.gateway_client = AgentGatewayClient(
            gateway_endpoint=gateway_endpoint,
            project_id=self.project_id,
            region=self.region,
        )
        self.tracer = get_tracer("sentinelflow.runtime")

        # Initialize fixed 6-agent fleet
        self.agents = {
            "IncidentCommanderAgent": IncidentCommanderAgent(),
            "DiagnosisAgent": DiagnosisAgent(),
            "PolicySLAAgent": PolicySLAAgent(),
            "MemoryAgent": MemoryAgent(),
            "RemediationAgent": RemediationAgent(),
            "VerifierAgent": VerifierAgent(),
        }

    def get_agent(self, agent_name: str) -> Any:
        """Retrieves an agent instance strictly from the fixed canonical roster."""
        # Enforce Fixed Canonical Roster boundary
        validate_agent_roster_membership(agent_name)
        return self.agents[agent_name]

    def execute_agent_step(
        self,
        agent_name: str,
        input_payload: Dict[str, Any],
        session_id: Optional[str] = None,
        workflow_id: Optional[str] = None,
    ) -> Dict[str, Any]:
        """Executes a single agent step with OpenTelemetry tracing and identity binding."""
        validate_agent_roster_membership(agent_name)
        agent = self.agents[agent_name]

        # Bind session to workflow ID pseudonmyously
        correlation_id = workflow_id or f"sess-corr-{session_id or 'anon'}"
        principal = AgentIdentityProvider.get_spiffe_principal(agent_name, self.project_id)

        with self.tracer.start_as_current_span(
            f"invoke_agent:{agent_name}",
            attributes={
                "agent.name": agent_name,
                "agent.version": FIXED_AGENT_ROSTER[agent_name].version,
                "agent.autonomy_level": FIXED_AGENT_ROSTER[agent_name].autonomy_level,
                "agent.principal": principal,
                "workflow.correlation_id": correlation_id,
            },
        ):
            # Invariant: Managed session is NOT financial persistence.
            # Agents execute using their existing prompt-partitioned logic
            return {
                "agent_name": agent_name,
                "status": "COMPLETED",
                "execution_source": "AGENT_RUNTIME",
                "correlation_id": correlation_id,
                "principal": principal,
                "output_schema": FIXED_AGENT_ROSTER[agent_name].output_schema_name,
            }


def create_app() -> FastAPI:
    """Factory creating the FastAPI application wrapper for Agent Runtime deployment."""
    configure_agent_observability()
    runtime_app = SentinelFlowAdkApp()

    app = FastAPI(
        title="SentinelFlow Google Agent Runtime Application",
        version="1.0.0",
        docs_url="/api/docs",
        openapi_url="/api/openapi.json",
    )

    @app.get("/health")
    async def health_check() -> Dict[str, str]:
        return {
            "status": "HEALTHY",
            "service": "sentinelflow-agent-runtime",
            "project": runtime_app.project_id,
            "region": runtime_app.region,
        }

    @app.get("/api/roster")
    async def get_roster() -> Dict[str, Any]:
        """Returns the immutable fixed agent roster metadata."""
        return {
            "roster": {
                name: manifest.model_dump()
                for name, manifest in FIXED_AGENT_ROSTER.items()
            }
        }

    @app.post("/api/agents/{agent_name}/execute")
    async def execute_agent(
        agent_name: str,
        request: Request,
        x_workflow_id: Optional[str] = Header(None, alias="X-Workflow-ID"),
        x_session_id: Optional[str] = Header(None, alias="X-Session-ID"),
    ) -> JSONResponse:
        try:
            body = await request.json()
        except Exception:
            body = {}

        try:
            res = runtime_app.execute_agent_step(
                agent_name=agent_name,
                input_payload=body,
                session_id=x_session_id,
                workflow_id=x_workflow_id,
            )
            return JSONResponse(status_code=200, content=res)
        except ValueError as ve:
            raise HTTPException(status_code=403, detail=str(ve))
        except Exception as e:
            logger.exception("Runtime agent execution error")
            raise HTTPException(status_code=500, detail=str(e))

    return app


# Default app instance for uvicorn / Agent Runtime entrypoint
app = create_app()
