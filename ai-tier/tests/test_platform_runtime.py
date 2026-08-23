"""Unit & Integration Tests for Google Agent Platform Runtime, Identity, Gateway & Observability."""

import pytest
from contracts.manifests import FIXED_AGENT_ROSTER, validate_agent_roster_membership
from runtime.app import SentinelFlowAdkApp, create_app
from runtime.identity import AgentIdentityProvider
from runtime.gateway_client import AgentGatewayClient
from observability.telemetry import sanitize_span_attributes, get_tracer


def test_fixed_canonical_roster_membership():
    """Verify all 6 fixed agents are approved and dynamic agents are rejected."""
    expected = [
        "IncidentCommanderAgent",
        "DiagnosisAgent",
        "PolicySLAAgent",
        "MemoryAgent",
        "RemediationAgent",
        "VerifierAgent",
    ]
    for agent_name in expected:
        manifest = validate_agent_roster_membership(agent_name)
        assert manifest.name == agent_name

    with pytest.raises(ValueError, match="is not in the fixed agent roster"):
        validate_agent_roster_membership("DynamicCloudAgent")


def test_adk_app_fleet_initialization():
    """Verify SentinelFlowAdkApp initializes all 6 agents."""
    app = SentinelFlowAdkApp(project_id="telos-agent")
    assert len(app.agents) == 6
    assert "IncidentCommanderAgent" in app.agents
    assert "VerifierAgent" in app.agents

    # Execution step returns proper envelope
    step_res = app.execute_agent_step(
        agent_name="DiagnosisAgent",
        input_payload={"test": "data"},
        workflow_id="wf-p11-test-01",
    )
    assert step_res["status"] == "COMPLETED"
    assert step_res["agent_name"] == "DiagnosisAgent"
    assert step_res["principal"] == "spiffe://telos-agent.iam.gserviceaccount.com/agent/diagnosis"


def test_agent_identity_provider_spiffe_principals():
    """Verify SPIFFE principal generation and header formatting."""
    principal = AgentIdentityProvider.get_spiffe_principal("VerifierAgent", project_id="telos-agent")
    assert principal == "spiffe://telos-agent.iam.gserviceaccount.com/agent/verifier"

    sa_email = AgentIdentityProvider.get_service_account_email("RemediationAgent", project_id="telos-agent")
    assert sa_email == "sentinelflow-remediation@telos-agent.iam.gserviceaccount.com"

    headers = AgentIdentityProvider.get_egress_headers(
        agent_name="PolicySLAAgent",
        project_id="telos-agent",
        workflow_id="wf-001",
        tenant_id="TENANT-A",
    )
    assert headers["X-Agent-Identity-Principal"] == "spiffe://telos-agent.iam.gserviceaccount.com/agent/policysla"
    assert headers["X-Workflow-ID"] == "wf-001"
    assert headers["X-Sentinel-Tenant"] == "TENANT-A"


def test_gateway_client_default_deny_enforcement():
    """Verify Agent Gateway default-deny routing."""
    gw = AgentGatewayClient(project_id="telos-agent", mode="ENFORCE")

    # 1. Registered endpoint ALLOW
    allowed = gw.evaluate_egress("IncidentCommanderAgent", "/internal/agent-tools", {})
    assert allowed.decision == "ALLOW"
    assert allowed.is_registered is True
    assert allowed.status_code == 200

    # 2. Unregistered endpoint DENY
    blocked = gw.evaluate_egress("IncidentCommanderAgent", "https://arbitrary-api.com/leak", {})
    assert blocked.decision == "DENY"
    assert blocked.is_registered is False
    assert blocked.status_code == 403

    # 3. Dry-run mode WOULD_DENY
    gw.set_mode("DRY_RUN")
    dry_res = gw.evaluate_egress("IncidentCommanderAgent", "https://arbitrary-api.com/leak", {})
    assert dry_res.decision == "WOULD_DENY"
    assert dry_res.status_code == 200


def test_telemetry_financial_privacy_sanitization():
    """Verify financial records and account numbers are strictly scrubbed from span attributes."""
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
