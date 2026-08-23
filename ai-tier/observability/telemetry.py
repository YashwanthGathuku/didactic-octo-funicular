"""OpenTelemetry & Cloud Trace Privacy-Preserving Observability for SentinelFlow P11.

Formal Invariant:
TraceMetadataAllowed != PromptPayloadLoggingAllowed

Rules:
- Traces contain sanitized metadata, latency, agent name, stage, tool ID, and hashes.
- Spans NEVER contain raw NACHA lines, 10-17 digit account numbers, routing numbers, or secrets.
"""

from __future__ import annotations

import logging
import os
import re
from typing import Any, Dict, Optional
from contextlib import contextmanager

logger = logging.getLogger("sentinel.observability.telemetry")

# Strict Data Minimization Regex Patterns
NACHA_94_REGEX = re.compile(r"[156789][0-9A-Za-z\s]{90,}")
ACCOUNT_REGEX = re.compile(r"\b\d{10,17}\b")
ROUTING_REGEX = re.compile(r"\b\d{9}\b")
SECRET_REGEX = re.compile(r"(?i)(bearer\s+[a-z0-9_\-\.]{20,}|(ghp|gho|xoxb|xoxp|sk_live|secret|token)_[a-z0-9_\-]{16,}|BEGIN\s+(RSA|OPENSSH|PGP|EC)\s+PRIVATE\s+KEY)")


def sanitize_span_attributes(attributes: Dict[str, Any]) -> Dict[str, Any]:
    """Sanitizes span attribute values ensuring zero financial PII or raw payloads."""
    sanitized = {}
    for k, v in attributes.items():
        if isinstance(v, str):
            val = NACHA_94_REGEX.sub("[NACHA_RECORD_REDACTED]", v)
            val = ACCOUNT_REGEX.sub("[ACCOUNT_REDACTED]", val)
            val = ROUTING_REGEX.sub("[ROUTING_REDACTED]", val)
            val = SECRET_REGEX.sub("[SECRET_REDACTED]", val)
            sanitized[k] = val
        else:
            sanitized[k] = v
    return sanitized


class MockSpan:
    """Lightweight in-memory span used for offline test execution and tracing."""

    def __init__(self, name: str, attributes: Optional[Dict[str, Any]] = None):
        self.name = name
        self.attributes = sanitize_span_attributes(attributes or {})
        self.events = []
        self.status = "OK"

    def set_attribute(self, key: str, value: Any) -> None:
        clean = sanitize_span_attributes({key: value})
        self.attributes.update(clean)

    def set_status(self, status: str) -> None:
        self.status = status

    def __enter__(self) -> "MockSpan":
        return self

    def __exit__(self, exc_type, exc_val, exc_tb) -> None:
        if exc_type:
            self.status = "ERROR"
            self.set_attribute("error.type", exc_type.__name__)


class MockTracer:
    """Mock tracer for local execution without remote GCP overhead."""

    def __init__(self, instrument_name: str):
        self.instrument_name = instrument_name
        self.spans: list[MockSpan] = []

    @contextmanager
    def start_as_current_span(self, name: str, attributes: Optional[Dict[str, Any]] = None):
        span = MockSpan(name, attributes)
        self.spans.append(span)
        try:
            yield span
        finally:
            pass


_GLOBAL_TRACERS: Dict[str, MockTracer] = {}


def get_tracer(instrument_name: str = "sentinelflow.default") -> MockTracer:
    """Retrieves or creates a tracer instance."""
    if instrument_name not in _GLOBAL_TRACERS:
        _GLOBAL_TRACERS[instrument_name] = MockTracer(instrument_name)
    return _GLOBAL_TRACERS[instrument_name]


def configure_agent_observability() -> None:
    """Sets standard environment variables enforcing privacy in Google Agent Observability."""
    os.environ["OTEL_INSTRUMENTATION_GENAI_CAPTURE_MESSAGE_CONTENT"] = "NO_CONTENT"
    os.environ["ADK_CAPTURE_MESSAGE_CONTENT_IN_SPANS"] = "false"
    os.environ["OTEL_SEMCONV_STABILITY_OPT_IN"] = "gen_ai_latest_experimental"
    logger.info("Configured privacy-preserving Agent Observability (NO_CONTENT in spans)")
