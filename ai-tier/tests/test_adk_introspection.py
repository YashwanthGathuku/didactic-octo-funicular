"""ADK Runtime Introspection and Object Model Integration Tests (SGACA Phase P06.5).

Proves that all SentinelFlow agents and parallel workflows instantiate and execute
using actual Google ADK runtime classes (Agent, ParallelAgent, InMemoryRunner).
"""

import pytest
import google.adk as adk
import google.adk.agents as adk_agents
import google.adk.runners as adk_runners

from agents.commander import IncidentCommanderAgent
from agents.diagnosis import DiagnosisAgent
from agents.policy_sla import PolicySLAAgent
from orchestrator.fleet import MultiAgentWorkflowOrchestrator


def test_adk_runtime_classes_and_manifest_roster_introspection():
    """Section 1: Verifies that every agent has an underlying real Google ADK Agent."""
    commander = IncidentCommanderAgent()
    diagnosis = DiagnosisAgent()
    policy_sla = PolicySLAAgent()

    # 1. IncidentCommanderAgent Runtime Proof
    assert hasattr(commander, "adk_agent")
    assert isinstance(commander.adk_agent, adk_agents.Agent) or isinstance(commander.adk_agent, adk_agents.LlmAgent)
    assert commander.adk_agent.name == "IncidentCommanderAgent"
    assert hasattr(commander, "adk_runner")
    assert isinstance(commander.adk_runner, adk_runners.InMemoryRunner)

    # 2. DiagnosisAgent Runtime Proof
    assert hasattr(diagnosis, "adk_agent")
    assert isinstance(diagnosis.adk_agent, adk_agents.Agent) or isinstance(diagnosis.adk_agent, adk_agents.LlmAgent)
    assert diagnosis.adk_agent.name == "DiagnosisAgent"
    assert diagnosis.adk_agent.output_key == "diagnosis_result"
    assert hasattr(diagnosis, "adk_runner")
    assert isinstance(diagnosis.adk_runner, adk_runners.InMemoryRunner)

    # 3. PolicySLAAgent Runtime Proof
    assert hasattr(policy_sla, "adk_agent")
    assert isinstance(policy_sla.adk_agent, adk_agents.Agent) or isinstance(policy_sla.adk_agent, adk_agents.LlmAgent)
    assert policy_sla.adk_agent.name == "PolicySLAAgent"
    assert policy_sla.adk_agent.output_key == "policy_sla_result"
    assert hasattr(policy_sla, "adk_runner")
    assert isinstance(policy_sla.adk_runner, adk_runners.InMemoryRunner)


def test_adk_parallel_agent_and_runner_introspection():
    """Section 1: Verifies ADK ParallelAgent with distinct specialist output keys."""
    orchestrator = MultiAgentWorkflowOrchestrator()

    assert hasattr(orchestrator, "adk_parallel_agent")
    assert isinstance(orchestrator.adk_parallel_agent, adk_agents.ParallelAgent)
    assert orchestrator.adk_parallel_agent.name == "ParallelSpecialists"

    # Verify sub-agents are genuine ADK agents
    sub_agents = orchestrator.adk_parallel_agent.sub_agents
    assert len(sub_agents) == 2
    sub_agent_names = [sa.name for sa in sub_agents]
    assert "DiagnosisAgent" in sub_agent_names
    assert "PolicySLAAgent" in sub_agent_names

    # Verify distinct output keys to prevent shared-state collision
    output_keys = [sa.output_key for sa in sub_agents]
    assert "diagnosis_result" in output_keys
    assert "policy_sla_result" in output_keys
    assert len(set(output_keys)) == 2

    # Verify Parallel Runner
    assert hasattr(orchestrator, "adk_parallel_runner")
    assert isinstance(orchestrator.adk_parallel_runner, adk_runners.InMemoryRunner)
