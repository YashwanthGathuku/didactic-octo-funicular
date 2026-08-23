"""Google Agent Identity & Principal Context Propagation for SentinelFlow P11.

Formal Invariants:
- AgentIdentity != ToolAuthorization
- AuthenticatedAgentIdentity -> CallerIdentityInput -> Go Tool Gateway Authorization
- Model-supplied identity cannot override cryptographically bound principal
"""

from __future__ import annotations

import logging
import os
from typing import Dict, Optional
from pydantic import BaseModel, Field

from contracts.manifests import FIXED_AGENT_ROSTER, validate_agent_roster_membership

logger = logging.getLogger("sentinel.runtime.identity")


class AgentIdentityContext(BaseModel):
    """Immutable identity context bound to an executing agent workload."""

    agent_name: str = Field(..., description="Canonical agent name")
    principal: str = Field(..., description="SPIFFE or service account principal URI")
    project_id: str = Field(..., description="Google Cloud project ID")
    autonomy_level: str = Field(..., description="Autonomy tier (A1 / A2)")
    correlation_id: str = Field(default="", description="Workflow / trace correlation ID")


class AgentIdentityProvider:
    """Provides SPIFFE principal formatting and header propagation for Agent Identity."""

    @staticmethod
    def get_spiffe_principal(agent_name: str, project_id: Optional[str] = None) -> str:
        """Returns the canonical SPIFFE identity URI for an agent."""
        validate_agent_roster_membership(agent_name)
        proj = project_id or os.environ.get("GOOGLE_CLOUD_PROJECT", "telos-agent")
        # Format: spiffe://{project}.iam.gserviceaccount.com/agent/{agent_slug}
        agent_slug = agent_name.lower().replace("agent", "")
        return f"spiffe://{proj}.iam.gserviceaccount.com/agent/{agent_slug}"

    @staticmethod
    def get_service_account_email(agent_name: str, project_id: Optional[str] = None) -> str:
        """Returns the service account email identity for an agent."""
        validate_agent_roster_membership(agent_name)
        proj = project_id or os.environ.get("GOOGLE_CLOUD_PROJECT", "telos-agent")
        agent_slug = agent_name.lower().replace("agent", "")
        return f"sentinelflow-{agent_slug}@{proj}.iam.gserviceaccount.com"

    @classmethod
    def create_identity_context(
        cls,
        agent_name: str,
        project_id: Optional[str] = None,
        correlation_id: str = "",
    ) -> AgentIdentityContext:
        """Constructs an AgentIdentityContext for the canonical agent."""
        validate_agent_roster_membership(agent_name)
        manifest = FIXED_AGENT_ROSTER[agent_name]
        proj = project_id or os.environ.get("GOOGLE_CLOUD_PROJECT", "telos-agent")
        principal = cls.get_spiffe_principal(agent_name, proj)

        return AgentIdentityContext(
            agent_name=agent_name,
            principal=principal,
            project_id=proj,
            autonomy_level=manifest.autonomy_level,
            correlation_id=correlation_id,
        )

    @classmethod
    def get_egress_headers(
        cls,
        agent_name: str,
        project_id: Optional[str] = None,
        workflow_id: str = "",
        tenant_id: str = "",
    ) -> Dict[str, str]:
        """Generates managed egress headers with Agent Identity and correlation binding."""
        validate_agent_roster_membership(agent_name)
        proj = project_id or os.environ.get("GOOGLE_CLOUD_PROJECT", "telos-agent")
        principal = cls.get_spiffe_principal(agent_name, proj)

        headers = {
            "X-Agent-Identity-Principal": principal,
            "X-Agent-Identity-Name": agent_name,
            "X-Agent-Identity-Project": proj,
        }
        if workflow_id:
            headers["X-Workflow-ID"] = workflow_id
        if tenant_id:
            headers["X-Sentinel-Tenant"] = tenant_id

        return headers
