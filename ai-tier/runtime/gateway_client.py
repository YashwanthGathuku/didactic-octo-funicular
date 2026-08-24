"""Governed egress client for SentinelFlow's Agent Runtime workload.

The local destination check is defense-in-depth and testable policy; it is never
reported as live Google Agent Gateway evidence. In managed mode the client sends
a bounded HTTP request to one *exact* registered Go endpoint. Google Agent
Gateway/IAP governs the network interaction outside this process; SentinelFlow's
Go Tool Gateway then governs the business capability inside that endpoint.

Formal invariants:
- AgentGatewayAllow != PolicyAllow.
- NetworkReachable != ToolExecutable.
- Agent Gateway ALLOW + SentinelFlow Tool Gateway DENY => DENY.
"""

from __future__ import annotations

import logging
import os
from typing import Any, Dict, Literal, Optional
from urllib.parse import urlparse

import httpx
from pydantic import BaseModel

from contracts.manifests import validate_agent_roster_membership
from runtime.identity import AgentIdentityProvider

logger = logging.getLogger("sentinel.runtime.gateway")

GatewayMode = Literal["DRY_RUN", "ENFORCE"]
DEFAULT_MANAGED_TOOL_PATH = "/api/v1/internal/agent-tools"


class GatewayEgressReport(BaseModel):
    mode: GatewayMode
    decision: Literal["ALLOW", "DENY", "WOULD_DENY"]
    target_endpoint: str
    is_registered: bool
    decision_source: Literal["LOCAL_POLICY", "MANAGED_GATEWAY_REQUEST"]
    details: str
    status_code: int = 200


class AgentGatewayClient:
    """Bounded dispatcher for SentinelFlow's single managed Go agent endpoint."""

    ALLOWED_LOCAL_PATHS = {
        DEFAULT_MANAGED_TOOL_PATH,
        "/internal/agent-tools",  # legacy/local compatibility only
    }

    def __init__(
        self,
        gateway_endpoint: Optional[str] = None,
        project_id: Optional[str] = None,
        region: str = "us-central1",
        mode: GatewayMode = "ENFORCE",
        registered_endpoint_url: Optional[str] = None,
        # Backward-compatible alias used by older local tests. If it has no path,
        # DEFAULT_MANAGED_TOOL_PATH is appended.
        registered_endpoint_base_url: Optional[str] = None,
        timeout_seconds: float = 10.0,
    ):
        self.gateway_endpoint = gateway_endpoint or os.environ.get("AGENT_GATEWAY_ENDPOINT", "")
        self.project_id = project_id or os.environ.get("GOOGLE_CLOUD_PROJECT", "telos-agent")
        self.region = region or os.environ.get("GOOGLE_CLOUD_LOCATION", "us-central1")
        self.mode: GatewayMode = mode
        self.platform_mode = AgentIdentityProvider.platform_mode()

        configured = (
            registered_endpoint_url
            or os.environ.get("SENTINEL_GO_AGENT_ENDPOINT", "")
            or registered_endpoint_base_url
            or ""
        ).strip()
        self.registered_endpoint_url = self._canonical_registered_url(configured)
        self.timeout_seconds = max(0.1, min(float(timeout_seconds), 30.0))

    @staticmethod
    def _canonical_registered_url(value: str) -> str:
        value = value.rstrip("/")
        if not value:
            return ""
        parsed = urlparse(value)
        if not parsed.scheme or not parsed.netloc:
            return value
        path = parsed.path.rstrip("/")
        if path in ("", "/"):
            path = DEFAULT_MANAGED_TOOL_PATH
        return f"{parsed.scheme}://{parsed.netloc}{path}"

    def set_mode(self, mode: GatewayMode) -> None:
        if mode not in ("DRY_RUN", "ENFORCE"):
            raise ValueError(f"unsupported gateway mode {mode!r}")
        self.mode = mode

    @staticmethod
    def _canonical_target(target_endpoint: str) -> str:
        target = (target_endpoint or "").strip()
        if not target:
            return ""
        parsed = urlparse(target)
        if parsed.scheme and parsed.netloc:
            return f"{parsed.scheme}://{parsed.netloc}{parsed.path.rstrip('/') or '/'}"
        return target if target.startswith("/") else f"/{target}"

    def _is_registered_destination(self, target_endpoint: str) -> bool:
        target = self._canonical_target(target_endpoint)
        if self.platform_mode == "managed":
            if not self.registered_endpoint_url:
                return False
            if target.startswith("http://") or target.startswith("https://"):
                return target == self.registered_endpoint_url
            # A relative request is legal only for the exact managed tool path;
            # dispatch resolves it to the one configured registered URL.
            return target == DEFAULT_MANAGED_TOOL_PATH
        return target in self.ALLOWED_LOCAL_PATHS

    def evaluate_egress(
        self,
        agent_name: str,
        target_endpoint: str,
        payload: Dict[str, Any],
        workflow_id: str = "",
        tenant_id: str = "",
    ) -> GatewayEgressReport:
        del payload, workflow_id, tenant_id
        validate_agent_roster_membership(agent_name)
        target = self._canonical_target(target_endpoint)
        is_registered = self._is_registered_destination(target)

        if not is_registered:
            if self.mode == "DRY_RUN":
                return GatewayEgressReport(
                    mode="DRY_RUN",
                    decision="WOULD_DENY",
                    target_endpoint=target,
                    is_registered=False,
                    decision_source="LOCAL_POLICY",
                    details=(
                        "WOULD_DENY: SentinelFlow local egress policy would deny this destination. "
                        "This is not evidence of a live Google Agent Gateway decision."
                    ),
                    status_code=200,
                )
            return GatewayEgressReport(
                mode="ENFORCE",
                decision="DENY",
                target_endpoint=target,
                is_registered=False,
                decision_source="LOCAL_POLICY",
                details="Unregistered egress destination blocked by SentinelFlow local policy.",
                status_code=403,
            )

        return GatewayEgressReport(
            mode=self.mode,
            decision="ALLOW",
            target_endpoint=target,
            is_registered=True,
            decision_source="LOCAL_POLICY",
            details=(
                "Destination matches SentinelFlow's exact local registered-endpoint policy; "
                "managed mode still requires Google Agent Gateway/IAP authorization."
            ),
            status_code=200,
        )

    def _dispatch_url(self, target_endpoint: str) -> str:
        if not self.registered_endpoint_url:
            raise RuntimeError("SENTINEL_GO_AGENT_ENDPOINT is required for managed dispatch")
        target = self._canonical_target(target_endpoint)
        if target.startswith("http://") or target.startswith("https://"):
            if target != self.registered_endpoint_url:
                raise RuntimeError("target does not match registered managed endpoint")
        elif target != DEFAULT_MANAGED_TOOL_PATH:
            raise RuntimeError("relative managed target must be the canonical agent-tools path")
        return self.registered_endpoint_url

    def dispatch_tool_call(
        self,
        agent_name: str,
        tool_name: str,
        tool_args: Dict[str, Any],
        target_endpoint: str = DEFAULT_MANAGED_TOOL_PATH,
        workflow_id: str = "",
        tenant_id: str = "",
        idempotency_key: str = "",
    ) -> Dict[str, Any]:
        """Dispatches a bounded tool request.

        Local mode returns an explicitly labeled simulation. Managed mode makes
        a real HTTP request. The caller-supplied tenant remains non-authoritative:
        the Go endpoint resolves the tenant from the durable workflow and merely
        rejects a mismatch in this metadata.
        """

        validate_agent_roster_membership(agent_name)
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
                "execution_source": "LOCAL_POLICY",
                "gateway_report": egress.model_dump(),
            }

        headers = AgentIdentityProvider.get_egress_headers(
            agent_name=agent_name,
            project_id=self.project_id,
            workflow_id=workflow_id,
            tenant_id=tenant_id,
        )
        if idempotency_key:
            headers["Idempotency-Key"] = idempotency_key

        request_body = {
            "agent_name": agent_name,
            "tool_name": tool_name,
            "tool_args": tool_args,
            "idempotency_key": idempotency_key,
        }

        if self.platform_mode != "managed":
            return {
                "success": True,
                "status": "LOCAL_GATEWAY_POLICY_SIMULATION",
                "execution_source": "LOCAL_SIMULATION",
                "agent_name": agent_name,
                "tool_name": tool_name,
                "tool_args": tool_args,
                "egress_headers": headers,
                "gateway_report": egress.model_dump(),
            }

        url = self._dispatch_url(target_endpoint)
        try:
            with httpx.Client(timeout=self.timeout_seconds, follow_redirects=False) as client:
                response = client.post(url, json=request_body, headers=headers)
        except httpx.TimeoutException as exc:
            return {
                "success": False,
                "error_code": "MANAGED_EGRESS_TIMEOUT",
                "error_message": str(exc),
                "execution_source": "MANAGED_NETWORK",
                "gateway_report": egress.model_dump(),
            }
        except httpx.HTTPError as exc:
            return {
                "success": False,
                "error_code": "MANAGED_EGRESS_ERROR",
                "error_message": str(exc),
                "execution_source": "MANAGED_NETWORK",
                "gateway_report": egress.model_dump(),
            }

        result: Dict[str, Any] = {
            "success": 200 <= response.status_code < 300,
            "status_code": response.status_code,
            "execution_source": "MANAGED_NETWORK",
            "gateway_report": {
                **egress.model_dump(),
                "decision_source": "MANAGED_GATEWAY_REQUEST",
                "status_code": response.status_code,
            },
        }
        try:
            result["response"] = response.json()
        except ValueError:
            result["response"] = {"text": response.text[:4096]}
        return result
