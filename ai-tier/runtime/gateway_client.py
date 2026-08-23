"""Governed egress client for SentinelFlow's Agent Runtime workload.

This module contains two deliberately separate concerns:

1. A deterministic local allowlist used as defense-in-depth and for offline
   tests.  It is *not* represented as Google Agent Gateway proof.
2. A managed HTTP dispatch path.  When running on Agent Runtime, Google Agent
   Gateway/IAP governs the actual network egress outside this process.  The
   application does not manufacture Agent Identity credentials or claim a local
   allowlist decision is a managed-gateway decision.

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


class GatewayEgressReport(BaseModel):
    """Application-side egress policy report.

    ``decision_source`` is intentionally explicit so local policy simulations
    cannot be confused with live Google Agent Gateway evidence.
    """

    mode: GatewayMode
    decision: Literal["ALLOW", "DENY", "WOULD_DENY"]
    target_endpoint: str
    is_registered: bool
    decision_source: Literal["LOCAL_POLICY", "MANAGED_GATEWAY_REQUEST"]
    details: str
    status_code: int = 200


class AgentGatewayClient:
    """Bounded dispatcher for the registered SentinelFlow Go agent endpoint."""

    ALLOWED_PATHS = {
        "/internal/agent-tools",
        "/internal/agent-tools/execute",
    }

    def __init__(
        self,
        gateway_endpoint: Optional[str] = None,
        project_id: Optional[str] = None,
        region: str = "us-central1",
        mode: GatewayMode = "ENFORCE",
        registered_endpoint_base_url: Optional[str] = None,
        timeout_seconds: float = 10.0,
    ):
        # ``gateway_endpoint`` is retained for configuration/provenance only.
        # Managed Agent Gateway is an infrastructure egress layer; application
        # traffic is sent to the registered destination URL and intercepted by
        # the platform rather than authenticated by a hand-authored header.
        self.gateway_endpoint = gateway_endpoint or os.environ.get("AGENT_GATEWAY_ENDPOINT", "")
        self.project_id = project_id or os.environ.get("GOOGLE_CLOUD_PROJECT", "telos-agent")
        self.region = region or os.environ.get("GOOGLE_CLOUD_LOCATION", "us-central1")
        self.mode: GatewayMode = mode
        self.platform_mode = AgentIdentityProvider.platform_mode()
        self.registered_endpoint_base_url = (
            registered_endpoint_base_url
            or os.environ.get("SENTINEL_GO_AGENT_ENDPOINT", "")
        ).rstrip("/")
        self.timeout_seconds = max(0.1, min(float(timeout_seconds), 30.0))

    def set_mode(self, mode: GatewayMode) -> None:
        if mode not in ("DRY_RUN", "ENFORCE"):
            raise ValueError(f"unsupported gateway mode {mode!r}")
        self.mode = mode

    def _normalize_target(self, target_endpoint: str) -> tuple[str, str]:
        target = (target_endpoint or "").strip()
        if not target:
            return "", ""

        parsed = urlparse(target)
        if parsed.scheme and parsed.netloc:
            return f"{parsed.scheme}://{parsed.netloc}", parsed.path or "/"

        path = target if target.startswith("/") else f"/{target}"
        return self.registered_endpoint_base_url, path

    def _is_registered_destination(self, target_endpoint: str) -> bool:
        base, path = self._normalize_target(target_endpoint)
        if path not in self.ALLOWED_PATHS:
            return False

        # In managed mode an exact registered destination base URL is required;
        # this prevents suffix/path tricks from being treated as registered.
        if self.platform_mode == "managed":
            if not self.registered_endpoint_base_url or not base:
                return False
            return base == self.registered_endpoint_base_url

        # Local tests may use a relative path but still exercise default-deny
        # semantics.  This remains LOCAL_POLICY evidence only.
        return True

    def evaluate_egress(
        self,
        agent_name: str,
        target_endpoint: str,
        payload: Dict[str, Any],
        workflow_id: str = "",
        tenant_id: str = "",
    ) -> GatewayEgressReport:
        del payload, workflow_id, tenant_id  # policy decision uses destination + fixed roster only
        validate_agent_roster_membership(agent_name)
        target = (target_endpoint or "").strip()
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
                        "Local P11 defense-in-depth policy would deny this destination. "
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
                "Destination is in SentinelFlow's local registered-endpoint allowlist; "
                "managed mode still requires Google Agent Gateway/IAP authorization."
            ),
            status_code=200,
        )

    def _dispatch_url(self, target_endpoint: str) -> str:
        base, path = self._normalize_target(target_endpoint)
        if not base:
            raise RuntimeError("SENTINEL_GO_AGENT_ENDPOINT is required for managed dispatch")
        return f"{base}{path}"

    def dispatch_tool_call(
        self,
        agent_name: str,
        tool_name: str,
        tool_args: Dict[str, Any],
        target_endpoint: str = "/internal/agent-tools",
        workflow_id: str = "",
        tenant_id: str = "",
        idempotency_key: str = "",
    ) -> Dict[str, Any]:
        """Dispatches a bounded tool request.

        In local mode this returns an explicit simulation envelope.  In managed
        mode it performs a real HTTP request to the registered Go destination;
        Google Agent Gateway/IAP is expected to authenticate/authorize that
        egress at the infrastructure layer.
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
            # Never return arbitrarily large backend bodies into model context.
            result["response"] = {"text": response.text[:4096]}
        return result
