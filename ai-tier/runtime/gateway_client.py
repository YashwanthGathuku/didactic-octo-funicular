"""Google Agent Gateway v1 Egress Client with Default-Deny Governance.

Formal Invariants:
- AgentGatewayAllow != PolicyAllow
- NetworkReachable != ToolExecutable
- Agent Gateway ALLOW + SentinelFlow Tool Gateway DENY => DENY
"""

from __future__ import annotations

import logging
import os
import ssl
from typing import Any, Dict, List, Literal, Optional
from pydantic import BaseModel, Field

from runtime.identity import AgentIdentityProvider

logger = logging.getLogger("sentinel.runtime.gateway")

GatewayMode = Literal["DRY_RUN", "ENFORCE"]


class GatewayEgressReport(BaseModel):
    """Audit report for an outbound request evaluated by Agent Gateway."""

    mode: GatewayMode
    decision: Literal["ALLOW", "DENY", "WOULD_DENY"]
    target_endpoint: str
    is_registered: bool
    agent_principal: str
    details: str
    status_code: int = 200


class AgentGatewayClient:
    """Egress gateway client enforcing default-deny routing and IAP identity propagation."""

    APPROVED_DESTINATIONS: List[str] = [
        "/internal/agent-tools",
        "internal/agent-tools",
        "/internal/agent-tools/execute",
        "internal/agent-tools/execute",
    ]

    def __init__(
        self,
        gateway_endpoint: Optional[str] = None,
        project_id: Optional[str] = None,
        region: str = "us-central1",
        mode: GatewayMode = "ENFORCE",
    ):
        self.gateway_endpoint = gateway_endpoint or os.environ.get("AGENT_GATEWAY_ENDPOINT", "https://sentinelflow-gw-dev.internal")
        self.project_id = project_id or os.environ.get("GOOGLE_CLOUD_PROJECT", "telos-agent")
        self.region = region
        self.mode: GatewayMode = mode

    def set_mode(self, mode: GatewayMode) -> None:
        """Configures Gateway enforcement mode (DRY_RUN vs ENFORCE)."""
        self.mode = mode

    def evaluate_egress(
        self,
        agent_name: str,
        target_endpoint: str,
        payload: Dict[str, Any],
        workflow_id: str = "",
        tenant_id: str = "",
    ) -> GatewayEgressReport:
        """Evaluates outbound egress destination against the registered default-deny topology."""
        principal = AgentIdentityProvider.get_spiffe_principal(agent_name, self.project_id)
        endpoint_clean = target_endpoint.strip()

        # Check against approved destination list
        is_approved = any(endpoint_clean == app or endpoint_clean.endswith(app) for app in self.APPROVED_DESTINATIONS)

        if not is_approved:
            if self.mode == "DRY_RUN":
                return GatewayEgressReport(
                    mode="DRY_RUN",
                    decision="WOULD_DENY",
                    target_endpoint=endpoint_clean,
                    is_registered=False,
                    agent_principal=principal,
                    details=f"DRY_RUN: Unregistered destination {endpoint_clean} logged; WOULD_DENY in ENFORCE mode",
                    status_code=200,
                )
            else:
                return GatewayEgressReport(
                    mode="ENFORCE",
                    decision="DENY",
                    target_endpoint=endpoint_clean,
                    is_registered=False,
                    agent_principal=principal,
                    details=f"ENFORCE: Unregistered egress destination {endpoint_clean} blocked by Agent Gateway default-deny posture",
                    status_code=403,
                )

        return GatewayEgressReport(
            mode=self.mode,
            decision="ALLOW",
            target_endpoint=endpoint_clean,
            is_registered=True,
            agent_principal=principal,
            details="Registered destination authorized through Agent Gateway",
            status_code=200,
        )

    def dispatch_tool_call(
        self,
        agent_name: str,
        tool_name: str,
        tool_args: Dict[str, Any],
        target_endpoint: str = "/internal/agent-tools",
        workflow_id: str = "",
        tenant_id: str = "",
    ) -> Dict[str, Any]:
        """Dispatches tool call through Agent Gateway with identity and correlation headers."""
        egress = self.evaluate_egress(
            agent_name=agent_name,
            target_endpoint=target_endpoint,
            payload={"tool_name": tool_name, "tool_args": tool_args},
            workflow_id=workflow_id,
            tenant_id=tenant_id,
        )

        if egress.decision == "DENY":
            return {
                "success": False,
                "error_code": "GATEWAY_EGRESS_BLOCKED",
                "error_message": egress.details,
                "gateway_report": egress.model_dump(),
            }

        headers = AgentIdentityProvider.get_egress_headers(
            agent_name=agent_name,
            project_id=self.project_id,
            workflow_id=workflow_id,
            tenant_id=tenant_id,
        )

        # In offline/mock test mode: returns mock dispatch envelope for ToolGateway
        return {
            "success": True,
            "status": "DISPATCHED_TO_GO_TOOL_GATEWAY",
            "agent_name": agent_name,
            "tool_name": tool_name,
            "tool_args": tool_args,
            "egress_headers": headers,
            "gateway_report": egress.model_dump(),
        }
