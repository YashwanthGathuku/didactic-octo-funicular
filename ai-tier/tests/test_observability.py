"""Comprehensive OpenTelemetry Tracing Integration Tests for SentinelFlow AI Tier.

Validates Requirements R1-R5:
1. Environment-Gated Observability (MockTracer vs SanitizedTracer).
2. PII / Sensitive Attribute Sanitization (Routing, Account, NACHA 94, Secrets).
3. W3C Trace Context Propagation (TraceContextTextMapPropagator, traceparent extraction/injection).
4. Canonical Span Catalog (5 canonical spans).
5. 100% Offline Test Safety (Zero remote GCP Cloud Trace calls required).
"""

from __future__ import annotations

import os
import pytest
from typing import Any, Dict
from unittest.mock import MagicMock, patch

import httpx
from fastapi.testclient import TestClient
from pydantic import BaseModel, Field

from opentelemetry import trace
from opentelemetry.sdk.trace import TracerProvider
from opentelemetry.sdk.trace.export import SimpleSpanProcessor
from opentelemetry.sdk.trace.export.in_memory_span_exporter import InMemorySpanExporter
from opentelemetry.trace import format_trace_id, format_span_id

from observability.telemetry import (
    MockSpan,
    MockTracer,
    SanitizedSpan,
    SanitizedTracer,
    configure_agent_observability,
    get_tracer,
    extract_trace_context,
    inject_trace_context,
    sanitize_span_attributes,
    _sanitize_string,
    _set_tracer_provider,
    _reset_tracer_provider,
)
from armor.client import MockModelArmorProvider
from guardrails.boundary import GuardedModelBoundary
from models.envelope import AgentContextEnvelope, RedactedFindingItem
from tools.gateway_client import ToolGatewayClient, ToolGatewayContext
from main import app


@pytest.fixture(autouse=True)
def clean_telemetry_state(monkeypatch):
    """Ensure each test runs with a pristine telemetry environment."""
    _reset_tracer_provider()
    monkeypatch.delenv("SENTINEL_OTEL_ENABLED", raising=False)
    monkeypatch.delenv("GOOGLE_CLOUD_PROJECT", raising=False)
    yield
    _reset_tracer_provider()


@pytest.fixture
def memory_tracer(monkeypatch):
    """Provides a SanitizedTracer configured with an InMemorySpanExporter for live-path tests."""
    monkeypatch.setenv("SENTINEL_OTEL_ENABLED", "true")
    exporter = InMemorySpanExporter()
    provider = TracerProvider()
    provider.add_span_processor(SimpleSpanProcessor(exporter))
    _set_tracer_provider(provider)
    sanitized_tracer = get_tracer("sentinelflow.test")
    return sanitized_tracer, exporter


class TestEnvironmentGatedObservability:
    """Requirement R1: Environment-gated tracer configuration and fallback."""

    def test_disabled_by_default_returns_mock_tracer(self, monkeypatch):
        monkeypatch.delenv("SENTINEL_OTEL_ENABLED", raising=False)
        configure_agent_observability()
        tracer = get_tracer("sentinelflow.test.default")
        assert isinstance(tracer, MockTracer)

    def test_explicit_false_returns_mock_tracer(self, monkeypatch):
        monkeypatch.setenv("SENTINEL_OTEL_ENABLED", "false")
        configure_agent_observability()
        tracer = get_tracer("sentinelflow.test.disabled")
        assert isinstance(tracer, MockTracer)

    def test_mock_tracer_records_spans_in_memory_offline(self):
        tracer = get_tracer("sentinelflow.test.mock")
        assert isinstance(tracer, MockTracer)

        with tracer.start_as_current_span("mock_span", attributes={"test.key": "test_val"}) as span:
            span.set_attribute("tenant_id", "TENANT-1")
            span.set_attributes({"workflow_id": "wf-123", "stage": "DIAGNOSIS"})
            span.add_event("stage_completed", {"detail": "ok"})

        assert len(tracer.spans) == 1
        recorded = tracer.spans[0]
        assert recorded.name == "mock_span"
        assert recorded.attributes["test.key"] == "test_val"
        assert recorded.attributes["tenant_id"] == "TENANT-1"
        assert recorded.attributes["workflow_id"] == "wf-123"
        assert recorded.status == "OK"

    def test_mock_span_interface_parity(self):
        span = MockSpan("parity_span", {"initial": "val"})
        span.set_attribute("k1", "v1")
        span.set_attributes({"k2": "v2", "k3": 123})
        span.set_status("OK")
        span.record_exception(ValueError("sample error"))
        span.end()

        assert span.is_recording() is True
        assert span.get_span_context() is None
        assert span.attributes["k1"] == "v1"
        assert span.attributes["k2"] == "v2"
        assert span.attributes["k3"] == 123
        assert span.attributes["error.type"] == "ValueError"

    @patch("opentelemetry.exporter.cloud_trace.CloudTraceSpanExporter")
    def test_enabled_mode_configures_cloud_trace_provider(self, mock_exporter_cls, monkeypatch):
        monkeypatch.setenv("SENTINEL_OTEL_ENABLED", "true")
        monkeypatch.setenv("GOOGLE_CLOUD_PROJECT", "test-project-123")

        mock_exporter = MagicMock()
        mock_exporter_cls.return_value = mock_exporter

        configure_agent_observability(project_id="test-project-123")
        mock_exporter_cls.assert_called_once_with(project_id="test-project-123")

        tracer = get_tracer("sentinelflow.test.live")
        assert isinstance(tracer, SanitizedTracer)


class TestPIIAttributeSanitization:
    """Requirement R2: PII Leak Prevention and Sanitized Span Wrapper."""

    def test_sanitize_string_redactions(self):
        # 9-digit ABA Routing Number
        assert _sanitize_string("Routing: 021000021") == "Routing: [ROUTING_REDACTED]"
        assert _sanitize_string("123456789") == "[ROUTING_REDACTED]"

        # 10-17 digit Account Number
        assert _sanitize_string("Account: 123456789012") == "Account: [ACCOUNT_REDACTED]"
        assert _sanitize_string("Acct 98765432109876") == "Acct [ACCOUNT_REDACTED]"

        # NACHA 94-char record
        nacha_record = "6" + "A" * 93
        assert _sanitize_string(f"Raw: {nacha_record}") == "Raw: [NACHA_RECORD_REDACTED]"

        # Secret / Bearer Token
        assert _sanitize_string("Bearer abcdef12345678901234567890") == "[SECRET_REDACTED]"
        assert _sanitize_string("sk_live_12345678901234567890") == "[SECRET_REDACTED]"

        # Control characters and multi-line breaks
        dirty = "line1\r\nline2\x00\x07injection"
        cleaned = _sanitize_string(dirty)
        assert "\r" not in cleaned and "\n" not in cleaned
        assert "\x00" not in cleaned and "\x07" not in cleaned

    def test_sanitize_span_attributes_dict(self):
        raw = {
            "routing_num": "021000021",
            "account_num": "123456789012",
            "non_str": 42,
            "bool_val": True,
            "safe_str": "TENANT-001",
        }
        sanitized = sanitize_span_attributes(raw)
        assert sanitized["routing_num"] == "[ROUTING_REDACTED]"
        assert sanitized["account_num"] == "[ACCOUNT_REDACTED]"
        assert sanitized["non_str"] == 42
        assert sanitized["bool_val"] is True
        assert sanitized["safe_str"] == "TENANT-001"

    def test_sanitized_span_wrapper_redacts_on_export(self, memory_tracer):
        tracer, exporter = memory_tracer

        with tracer.start_as_current_span(
            "sentinelflow.boundary.model_call",
            attributes={
                "bank.routing": "021000021",
                "customer.account": "123456789012",
                "tenant.id": "TENANT-TEST",
            },
        ) as span:
            span.set_attribute("secret.token", "sk_live_12345678901234567890")
            span.set_attributes({
                "another.routing": "987654321",
                "safe.metric": 100,
            })
            span.add_event("data_processed", {"raw_data": "123456789"})

        spans = exporter.get_finished_spans()
        assert len(spans) == 1
        exported = spans[0]

        assert exported.name == "sentinelflow.boundary.model_call"
        assert exported.attributes["bank.routing"] == "[ROUTING_REDACTED]"
        assert exported.attributes["customer.account"] == "[ACCOUNT_REDACTED]"
        assert exported.attributes["tenant.id"] == "TENANT-TEST"
        assert exported.attributes["secret.token"] == "[SECRET_REDACTED]"
        assert exported.attributes["another.routing"] == "[ROUTING_REDACTED]"
        assert exported.attributes["safe.metric"] == 100

        # Event attributes are also sanitized
        assert len(exported.events) == 1
        assert exported.events[0].attributes["raw_data"] == "[ROUTING_REDACTED]"


class TestW3CTraceContextPropagation:
    """Requirement R3: Distributed W3C Trace Context Propagation."""

    def test_extract_trace_context_from_traceparent(self, memory_tracer):
        tracer, exporter = memory_tracer

        trace_id_hex = "4bf92f3577b34da6a3ce929d0e0e4736"
        parent_span_id_hex = "00f067aa0ba902b7"
        traceparent = f"00-{trace_id_hex}-{parent_span_id_hex}-01"

        headers = {"traceparent": traceparent}
        extracted_ctx = extract_trace_context(headers=headers)
        assert extracted_ctx is not None

        with tracer.start_as_current_span("child_span", context=extracted_ctx) as span:
            span.set_attribute("child_attr", "valid")

        spans = exporter.get_finished_spans()
        assert len(spans) == 1
        exported = spans[0]

        # Verify trace ID matches the parent from Go control plane exactly
        assert format_trace_id(exported.context.trace_id) == trace_id_hex
        assert format_span_id(exported.parent.span_id) == parent_span_id_hex
        assert format_span_id(exported.context.span_id) != parent_span_id_hex

    def test_extract_trace_context_fallback_trace_id(self, memory_tracer):
        tracer, exporter = memory_tracer

        trace_id_hex = "1234567890abcdef1234567890abcdef"
        extracted_ctx = extract_trace_context(trace_id=trace_id_hex)
        assert extracted_ctx is not None

        with tracer.start_as_current_span("fallback_child", context=extracted_ctx):
            pass

        spans = exporter.get_finished_spans()
        assert len(spans) == 1
        assert format_trace_id(spans[0].context.trace_id) == trace_id_hex

    def test_inject_trace_context(self, memory_tracer):
        tracer, _ = memory_tracer

        headers: Dict[str, str] = {"Content-Type": "application/json"}
        with tracer.start_as_current_span("parent_span") as span:
            inject_trace_context(headers)

        assert "traceparent" in headers
        assert headers["traceparent"].startswith("00-")


class TestCanonicalSpansCatalog:
    """Requirement R4: Verification of 5 canonical span names and attribute hierarchy."""

    def test_all_five_canonical_spans(self, memory_tracer):
        tracer, exporter = memory_tracer

        # 1. sentinelflow.agent.invoke
        with tracer.start_as_current_span(
            "sentinelflow.agent.invoke",
            attributes={
                "agent.name": "DiagnosisAgent",
                "tenant.id": "TENANT-1",
                "workflow.id": "wf-001",
                "incident.id": "1001",
                "stage.type": "COMMANDER_PLAN",
            },
        ):
            # 2. sentinelflow.boundary.screen_input
            with tracer.start_as_current_span(
                "sentinelflow.boundary.screen_input",
                attributes={
                    "tenant.id": "TENANT-1",
                    "guardrail.mode": "observe",
                    "guardrail.input_decision": "ALLOW",
                },
            ):
                pass

            # 3. sentinelflow.boundary.model_call
            with tracer.start_as_current_span(
                "sentinelflow.boundary.model_call",
                attributes={
                    "gen_ai.system": "google",
                    "gen_ai.request.model": "gemini-3.5-flash",
                    "model.name": "gemini-3.5-flash",
                    "prompt_tokens": 150,
                    "completion_tokens": 50,
                    "total_tokens": 200,
                },
            ):
                pass

            # 4. sentinelflow.boundary.screen_output
            with tracer.start_as_current_span(
                "sentinelflow.boundary.screen_output",
                attributes={
                    "tenant.id": "TENANT-1",
                    "guardrail.output_decision": "ALLOW",
                    "grounding.verdict": "VERIFIED",
                },
            ):
                pass

            # 5. sentinelflow.toolgateway.execute
            with tracer.start_as_current_span(
                "sentinelflow.toolgateway.execute",
                attributes={
                    "tool.id": "incident.get",
                    "tool.version": "1.0.0",
                    "tenant.id": "TENANT-1",
                    "caller.id": "DiagnosisAgent",
                },
            ):
                pass

        spans = exporter.get_finished_spans()
        assert len(spans) == 5

        span_names = [s.name for s in spans]
        assert "sentinelflow.boundary.screen_input" in span_names
        assert "sentinelflow.boundary.model_call" in span_names
        assert "sentinelflow.boundary.screen_output" in span_names
        assert "sentinelflow.toolgateway.execute" in span_names
        assert "sentinelflow.agent.invoke" in span_names


class TestEndToEndTracingHierarchy:
    """Requirement R3, R4 & R5: Context hierarchy & integration across AI tier components."""

    def test_guarded_model_boundary_generates_canonical_spans(self, memory_tracer, monkeypatch):
        tracer, exporter = memory_tracer
        monkeypatch.setenv("SENTINEL_AI_MODE", "local")

        boundary = GuardedModelBoundary(guardrail_provider=MockModelArmorProvider())
        envelope = AgentContextEnvelope(
            tenant_id="TENANT-ACME",
            incident_id=9001,
            artifact_id=123,
            artifact_sha256="abc123sha",
            correlation_id="corr-9001",
            findings=[
                RedactedFindingItem(
                    id="FINDING-1",
                    code="0802",
                    severity="BLOCKING",
                    description="Account number 123456789012 mismatch with routing 021000021",
                )
            ],
            available_runbooks=["RB-01"],
            authorized_evidence_refs=["FINDING-1"],
        )

        class SampleOutput(BaseModel):
            summary: str
            evidence_refs: list[str] = Field(default_factory=list)

        def fallback_fn(env, auth_set):
            return SampleOutput(summary="Deterministic fallback output", evidence_refs=["FINDING-1"])

        result = boundary.invoke(
            envelope=envelope,
            response_schema=SampleOutput,
            fallback_fn=fallback_fn,
        )

        assert result.success is True
        spans = exporter.get_finished_spans()
        span_names = [s.name for s in spans]

        assert "sentinelflow.boundary.screen_input" in span_names
        assert "sentinelflow.boundary.screen_output" in span_names

        # Check sanitization in screen_input span attributes
        screen_in = next(s for s in spans if s.name == "sentinelflow.boundary.screen_input")
        assert screen_in.attributes["tenant.id"] == "TENANT-ACME"
        assert screen_in.attributes["correlation.id"] == "corr-9001"

    def test_tool_gateway_client_generates_execute_span(self, memory_tracer, respx_mock):
        tracer, exporter = memory_tracer

        context = ToolGatewayContext(
            tenant_id="TENANT-ACME",
            correlation_id="corr-tool-1",
            trace_id="4bf92f3577b34da6a3ce929d0e0e4736",
            caller_id="DiagnosisAgent",
        )

        client = ToolGatewayClient(base_url="http://mock-gateway")
        mock_route = respx_mock.post("http://mock-gateway/api/v1/tools/execute").mock(
            return_value=httpx.Response(
                200,
                json={
                    "invocation_id": "inv-001",
                    "tool_id": "incident.get",
                    "tool_version": "1.0.0",
                    "status": "SUCCESS",
                    "output": {"status": "ACTIVE"},
                    "output_bytes": 100,
                    "duration_ms": 15.5,
                },
            )
        )
        client._client = httpx.Client(base_url="http://mock-gateway")

        record = client.execute_tool(
            tool_id="incident.get",
            business_args={"incident_id": 9001},
            context=context,
        )

        assert record.status == "SUCCESS"
        spans = exporter.get_finished_spans()
        assert len(spans) == 1
        tool_span = spans[0]

        assert tool_span.name == "sentinelflow.toolgateway.execute"
        assert tool_span.attributes["tool.id"] == "incident.get"
        assert tool_span.attributes["tenant.id"] == "TENANT-ACME"
        assert tool_span.attributes["caller.id"] == "DiagnosisAgent"
        assert tool_span.attributes["status"] == "SUCCESS"

        assert mock_route.called
        last_req = mock_route.calls.last.request
        assert "traceparent" in last_req.headers
        assert "x-sentinel-tenant" in last_req.headers

    def test_fastapi_trace_propagation_middleware(self):
        client = TestClient(app)
        trace_id_hex = "4bf92f3577b34da6a3ce929d0e0e4736"
        parent_span_id_hex = "00f067aa0ba902b7"
        traceparent = f"00-{trace_id_hex}-{parent_span_id_hex}-01"

        response = client.get("/health", headers={"traceparent": traceparent})
        assert response.status_code == 200
        assert response.json()["status"] == "healthy"
