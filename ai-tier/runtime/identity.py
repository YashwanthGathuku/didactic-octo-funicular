"""Agent Runtime identity context for SentinelFlow.

P11.5 deliberately separates *managed, system-attested* Google Agent Identity
from local test fixtures. SentinelFlow never fabricates a managed workload
principal and never sends a client-authored legacy principal header as proof of
identity.

Formal invariants:
- AgentIdentity != ToolAuthorization.
- Managed identity is supplied/attested by Google infrastructure, not generated
  from an agent name in application code.
- Internal specialist names are fixed-roster metadata, not cloud credentials.
- Model-supplied identity cannot override trusted runtime/ingress identity.
"""

from __future__ import annotations

import logging
import os
from typing import Dict, Literal, Optional

from pydantic import BaseModel, Field

from contracts.manifests import FIXED_AGENT_ROSTER, validate_agent_roster_membership

logger = logging.getLogger("sentinel.runtime.identity")

PlatformMode = Literal["local", "managed"]


class AgentIdentityContext(BaseModel):
    """Immutable identity context for a fixed-roster specialist invocation."""

    agent_name: str = Field(..., description="Canonical SentinelFlow specialist name")
    workload_principal: str = Field(
        ...,
        description="System-attested managed workload principal or explicit local test fixture",
    )
    project_id: str = Field(..., description="Google Cloud project ID")
    autonomy_level: str = Field(..., description="Autonomy tier (A1 / A2)")
    correlation_id: str = Field(default="", description="Workflow / trace correlation ID")
    identity_source: Literal["GOOGLE_AGENT_IDENTITY", "LOCAL_TEST_FIXTURE"]


class AgentIdentityProvider:
    """Builds trusted identity *context* without fabricating managed identities."""

    @staticmethod
    def platform_mode() -> PlatformMode:
        value = os.environ.get("SENTINEL_PLATFORM_MODE", "local").strip().lower()
        if value not in ("local", "managed"):
            raise RuntimeError(f"unsupported SENTINEL_PLATFORM_MODE={value!r}")
        return value  # type: ignore[return-value]

    @staticmethod
    def get_managed_workload_principal() -> str:
        """Returns the Google-provisioned Agent Identity principal from trusted config.

        The value is populated only after a real Agent Runtime deployment is
        created and its output-only principal is observed. The application does
        not derive or guess this value.
        """

        principal = os.environ.get("SENTINEL_AGENT_IDENTITY_PRINCIPAL", "").strip()
        if not principal:
            raise RuntimeError(
                "managed mode requires SENTINEL_AGENT_IDENTITY_PRINCIPAL from the "
                "deployed Google Agent Runtime resource"
            )
        if not (
            principal.startswith("principal://agents.") or principal.startswith("spiffe://agents.")
        ):
            raise RuntimeError("managed Agent Identity principal has an unexpected format")
        return principal

    @staticmethod
    def get_local_fixture_principal(agent_name: str) -> str:
        """Returns a visibly non-production principal for deterministic local tests."""

        validate_agent_roster_membership(agent_name)
        return f"test-agent:{agent_name}"

    @classmethod
    def get_runtime_principal(cls, agent_name: str) -> tuple[str, str]:
        validate_agent_roster_membership(agent_name)
        if cls.platform_mode() == "managed":
            return cls.get_managed_workload_principal(), "GOOGLE_AGENT_IDENTITY"
        return cls.get_local_fixture_principal(agent_name), "LOCAL_TEST_FIXTURE"

    @classmethod
    def create_identity_context(
        cls,
        agent_name: str,
        project_id: Optional[str] = None,
        correlation_id: str = "",
    ) -> AgentIdentityContext:
        validate_agent_roster_membership(agent_name)
        manifest = FIXED_AGENT_ROSTER[agent_name]
        project = project_id or os.environ.get("GOOGLE_CLOUD_PROJECT", "project-3687901b-8355-4073-ac3")
        principal, source = cls.get_runtime_principal(agent_name)

        return AgentIdentityContext(
            agent_name=agent_name,
            workload_principal=principal,
            project_id=project,
            autonomy_level=manifest.autonomy_level,
            correlation_id=correlation_id,
            identity_source=source,  # type: ignore[arg-type]
        )

    @classmethod
    def get_egress_headers(
        cls,
        agent_name: str,
        project_id: Optional[str] = None,
        workflow_id: str = "",
        tenant_id: str = "",
    ) -> Dict[str, str]:
        """Builds application metadata headers for the governed Go endpoint.

        These headers are NOT authentication. In managed mode Google Agent
        Gateway/IAP authenticates the workload out-of-band and the Go endpoint
        verifies that managed ingress before trusting this metadata.
        """

        validate_agent_roster_membership(agent_name)
        manifest = FIXED_AGENT_ROSTER[agent_name]
        project = project_id or os.environ.get("GOOGLE_CLOUD_PROJECT", "project-3687901b-8355-4073-ac3")

        headers: Dict[str, str] = {
            "X-Sentinel-Agent-Name": agent_name,
            "X-Sentinel-Agent-Version": manifest.version,
            "X-Sentinel-Agent-Project": project,
        }
        if workflow_id:
            headers["X-Workflow-ID"] = workflow_id
        if tenant_id:
            headers["X-Sentinel-Tenant"] = tenant_id

        # Local fixtures are explicit and can never be confused with a Google
        # attested identity. Managed mode intentionally sends no principal
        # header from application code.
        if cls.platform_mode() == "local":
            headers["X-Sentinel-Test-Principal"] = cls.get_local_fixture_principal(agent_name)

        return headers
