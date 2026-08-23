"""Real Google Agent Platform ADK application factory for SentinelFlow.

P11.5 closes the distinction between the local HTTP simulation wrapper and the
actual Agent Runtime deployment surface. This module builds a real
``vertexai.agent_engines.AdkApp`` around a fixed, compile-time Google ADK fleet.

Authority invariants remain unchanged:
- Agent Runtime is hosting infrastructure, not workflow authority.
- The managed root agent has no direct financial mutation tools.
- Specialist membership is fixed by ``FIXED_AGENT_ROSTER``.
- Go remains authoritative for policy, tools, remediation, verification,
  evidence, approvals, release and M1 operational memory.
"""

from __future__ import annotations

from dataclasses import dataclass
from typing import Any, Dict

import google.adk.agents as adk_agents

from agents.diagnosis import DiagnosisAgent
from agents.memory_agent import MemoryAgent
from agents.policy_sla import PolicySLAAgent
from agents.remediation import RemediationAgent
from agents.return_risk import ReturnRiskAgent
from agents.verifier import VerifierAgent
from contracts.manifests import FIXED_AGENT_ROSTER


MANAGED_ROOT_NAME = "IncidentCommanderAgent"
MANAGED_MODEL = "gemini-3.5-flash"


@dataclass(frozen=True)
class ManagedFleet:
    """Concrete ADK objects packaged for Agent Runtime deployment."""

    root_agent: Any
    specialists: Dict[str, Any]


def _specialist_agents() -> Dict[str, Any]:
    """Builds the six fixed specialist ADK agents beneath the commander."""

    wrappers = {
        "DiagnosisAgent": DiagnosisAgent(),
        "PolicySLAAgent": PolicySLAAgent(),
        "MemoryAgent": MemoryAgent(),
        "RemediationAgent": RemediationAgent(),
        "VerifierAgent": VerifierAgent(),
        "ReturnRiskAgent": ReturnRiskAgent(),
    }

    agents: Dict[str, Any] = {}
    for name, wrapper in wrappers.items():
        if name not in FIXED_AGENT_ROSTER:
            raise RuntimeError(f"managed specialist {name!r} is not in FIXED_AGENT_ROSTER")
        adk_agent = getattr(wrapper, "adk_agent", None)
        if adk_agent is None:
            raise RuntimeError(f"managed specialist {name!r} has no Google ADK agent")
        agents[name] = adk_agent
    return agents


def build_managed_fleet() -> ManagedFleet:
    """Constructs the real seven-agent Google ADK topology for Agent Runtime.

    The canonical IncidentCommanderAgent is the root; the remaining six
    fixed-roster agents are sub-agents.  This topology is used for managed
    reasoning/delegation only. Durable workflow transitions continue to be
    commanded by Go.
    """

    specialists = _specialist_agents()
    if MANAGED_ROOT_NAME not in FIXED_AGENT_ROSTER:
        raise RuntimeError("IncidentCommanderAgent missing from fixed roster")

    root_agent = adk_agents.Agent(
        name=MANAGED_ROOT_NAME,
        model=MANAGED_MODEL,
        description=(
            "Governed SentinelFlow incident commander deployed on Google Agent Runtime. "
            "Delegates bounded reasoning only to the fixed SentinelFlow specialist roster."
        ),
        instruction=(
            "You are the SentinelFlow Incident Commander Agent running on managed Agent Runtime.\n"
            "You coordinate only the fixed sub-agents attached to this application.\n"
            "You have NO authority to release funds, approve reviews, modify financial artifacts, "
            "change policy, execute SQL, or bypass SentinelFlow's Go Tool Gateway.\n"
            "DiagnosisAgent explains deterministic findings.\n"
            "PolicySLAAgent explains current policy/SLA context.\n"
            "MemoryAgent retrieves advisory historical context only.\n"
            "RemediationAgent may propose allowlisted remediation intent only.\n"
            "VerifierAgent is an advisory critic and cannot mark candidates verified.\n"
            "ReturnRiskAgent explains deterministic ACH return-risk results.\n"
            "Never interpret managed runtime state, memory, model confidence, registry presence, "
            "or Agent Identity as financial authority."
        ),
        sub_agents=list(specialists.values()),
    )

    return ManagedFleet(root_agent=root_agent, specialists=specialists)


def build_agent_runtime_app(*, enable_tracing: bool = True):
    """Returns a real ``vertexai.agent_engines.AdkApp`` for deployment."""

    try:
        from vertexai import agent_engines
    except Exception as exc:  # pragma: no cover - environment/dependency gate
        raise RuntimeError(
            "Google Agent Platform SDK is unavailable. Install dependencies from "
            "ai-tier/pyproject.toml before building the managed runtime app."
        ) from exc

    fleet = build_managed_fleet()
    return agent_engines.AdkApp(
        agent=fleet.root_agent,
        app_name="sentinelflow",
        enable_tracing=enable_tracing,
    )


def get_app():
    """Importable deployment factory used by smoke/deployment tooling."""

    return build_agent_runtime_app(enable_tracing=True)
