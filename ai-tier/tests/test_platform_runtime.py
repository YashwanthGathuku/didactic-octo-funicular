"""Unit tests for Agent Runtime packaging, identity, egress policy and telemetry."""

import pytest

from contracts.manifests import FIXED_AGENT_ROSTER, validate_agent_roster_membership
from observability.telemetry import sanitize_span_attributes
from runtime.app import SentinelFlowAdkApp
from runtime.gateway_client import AgentGatewayClient, DEFAULT_MANAGED_TOOL_PATH
from runtime.identity import AgentIdentityProvider
from runtime.managed_adk import MANAGED_MODEL, MANAGED_ROOT_NAME, build_managed_fleet


def test_fixed_canonical_roster_membership():
    expected = [
        "IncidentCommanderAgent",
        "DiagnosisAgent",
        "PolicySLAAgent",
        "MemoryAgent",
        "RemediationAgent",
        "VerifierAgent",
        "ReturnRiskAgent",
    ]
    assert sorted(FIXED_AGENT_ROSTER) == sorted(expected)
    for agent_name in expected:
        manifest = validate_agent_roster_membership(agent_name)
        assert manifest.name == agent_name

    with pytest.raises(ValueError, match="is not in the fixed agent roster"):
        validate_agent_roster_membership("DynamicCloudAgent")


def test_local_runtime_adapter_does_not_fake_managed_execution(monkeypatch):
    monkeypatch.setenv("SENTINEL_PLATFORM_MODE", "local")
    app = SentinelFlowAdkApp(project_id="telos-agent")
    assert len(app.agents) == 7

    result = app.execute_agent_step(
        agent_name="DiagnosisAgent",
        input_payload={"test": "data"},
        workflow_id="wf-p11-test-01",
    )
    assert result["status"] == "NOT_EXECUTED"
    assert result["execution_source"] == "LOCAL_RUNTIME_ADAPTER"
    assert result["identity_source"] == "LOCAL_TEST_FIXTURE"
    assert result["workload_principal"] == "test-agent:DiagnosisAgent"
    assert "managed_adk" in result["managed_runtime_app"]


def test_real_managed_adk_topology_uses_fixed_seven_agent_roster():
    fleet = build_managed_fleet()
    assert MANAGED_ROOT_NAME == "IncidentCommanderAgent"
    assert MANAGED_MODEL == "gemini-3.5-flash"
    assert fleet.root_agent.name == "IncidentCommanderAgent"
    assert set(fleet.specialists) == {
        "DiagnosisAgent",
        "PolicySLAAgent",
        "MemoryAgent",
        "RemediationAgent",
        "VerifierAgent",
        "ReturnRiskAgent",
    }
    assert {agent.name for agent in fleet.specialists.values()} == set(fleet.specialists)


def test_identity_provider_local_fixture_is_visibly_non_production(monkeypatch):
    monkeypatch.setenv("SENTINEL_PLATFORM_MODE", "local")
    principal, source = AgentIdentityProvider.get_runtime_principal("VerifierAgent")
    assert principal == "test-agent:VerifierAgent"
    assert source == "LOCAL_TEST_FIXTURE"

    headers = AgentIdentityProvider.get_egress_headers(
        agent_name="PolicySLAAgent",
        project_id="telos-agent",
        workflow_id="wf-001",
        tenant_id="TENANT-A",
    )
    assert "X-Agent-Identity-Principal" not in headers
    assert headers["X-Sentinel-Agent-Name"] == "PolicySLAAgent"
    assert headers["X-Sentinel-Test-Principal"] == "test-agent:PolicySLAAgent"


def test_identity_provider_managed_mode_never_fabricates_principal(monkeypatch):
    monkeypatch.setenv("SENTINEL_PLATFORM_MODE", "managed")
    monkeypatch.delenv("SENTINEL_AGENT_IDENTITY_PRINCIPAL", raising=False)
    with pytest.raises(RuntimeError, match="requires SENTINEL_AGENT_IDENTITY_PRINCIPAL"):
        AgentIdentityProvider.get_runtime_principal("DiagnosisAgent")

    real_shape = (
        "principal://agents.global.org-123.system.id.goog/resources/aiplatform/"
        "projects/456/locations/us-central1/reasoningEngines/789"
    )
    monkeypatch.setenv("SENTINEL_AGENT_IDENTITY_PRINCIPAL", real_shape)
    principal, source = AgentIdentityProvider.get_runtime_principal("DiagnosisAgent")
    assert principal == real_shape
    assert source == "GOOGLE_AGENT_IDENTITY"


def test_gateway_client_default_deny_is_explicitly_local_policy(monkeypatch):
    monkeypatch.setenv("SENTINEL_PLATFORM_MODE", "local")
    gw = AgentGatewayClient(project_id="telos-agent", mode="ENFORCE")

    allowed = gw.evaluate_egress("IncidentCommanderAgent", DEFAULT_MANAGED_TOOL_PATH, {})
    assert allowed.decision == "ALLOW"
    assert allowed.is_registered is True
    assert allowed.decision_source == "LOCAL_POLICY"

    blocked = gw.evaluate_egress("IncidentCommanderAgent", "https://arbitrary-api.example/leak", {})
    assert blocked.decision == "DENY"
    assert blocked.is_registered is False
    assert blocked.status_code == 403
    assert blocked.decision_source == "LOCAL_POLICY"

    gw.set_mode("DRY_RUN")
    dry_result = gw.evaluate_egress(
        "IncidentCommanderAgent", "https://arbitrary-api.example/leak", {}
    )
    assert dry_result.decision == "WOULD_DENY"
    assert dry_result.status_code == 200


def test_gateway_managed_mode_requires_exact_registered_url(monkeypatch):
    monkeypatch.setenv("SENTINEL_PLATFORM_MODE", "managed")
    monkeypatch.setenv(
        "SENTINEL_AGENT_IDENTITY_PRINCIPAL",
        "principal://agents.global.org-123.system.id.goog/resources/aiplatform/"
        "projects/456/locations/us-central1/reasoningEngines/789",
    )
    registered = "https://gateway.example.internal/api/v1/internal/agent-tools"
    gw = AgentGatewayClient(
        project_id="telos-agent",
        mode="ENFORCE",
        registered_endpoint_url=registered,
    )
    allowed_absolute = gw.evaluate_egress("DiagnosisAgent", registered, {})
    assert allowed_absolute.decision == "ALLOW"

    allowed_relative = gw.evaluate_egress("DiagnosisAgent", DEFAULT_MANAGED_TOOL_PATH, {})
    assert allowed_relative.decision == "ALLOW"

    suffix_attack = gw.evaluate_egress(
        "DiagnosisAgent",
        "https://evil.example/api/v1/internal/agent-tools",
        {},
    )
    assert suffix_attack.decision == "DENY"

    legacy_path = gw.evaluate_egress("DiagnosisAgent", "/internal/agent-tools", {})
    assert legacy_path.decision == "DENY"


def test_telemetry_financial_privacy_sanitization():
    raw_attrs = {
        "agent.name": "IncidentCommanderAgent",
        "nacha.raw": "6221234567890123456789012345678901234567890123456789012345678901234567890123456789012345678901234",
        "bank.account": "12345678901234",
        "bank.routing": "121000358",
        "secret.key": "Bearer ghp_abcdef1234567890abcdef1234567890",
        "safe.metric": 42,
    }

    clean = sanitize_span_attributes(raw_attrs)
    assert clean["agent.name"] == "IncidentCommanderAgent"
    assert clean["nacha.raw"] == "[NACHA_RECORD_REDACTED]"
    assert clean["bank.account"] == "[ACCOUNT_REDACTED]"
    assert clean["bank.routing"] == "[ROUTING_REDACTED]"
    assert "[SECRET_REDACTED]" in clean["secret.key"]
    assert clean["safe.metric"] == 42
