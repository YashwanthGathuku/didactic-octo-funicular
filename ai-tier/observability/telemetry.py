"""Privacy-preserving OpenTelemetry helpers for SentinelFlow.

Formal invariant:
    TraceMetadataAllowed != PromptPayloadLoggingAllowed

Span attributes may contain bounded operational metadata only. Raw NACHA,
account/routing numbers, credentials, and control characters that can forge
multi-line log records are removed before telemetry leaves the process.
"""

from __future__ import annotations

from contextlib import contextmanager
import logging
import os
import re
from typing import Any, Dict, Optional

logger = logging.getLogger("sentinel.observability.telemetry")

NACHA_94_REGEX = re.compile(r"[156789][0-9A-Za-z\s]{90,}")
ACCOUNT_REGEX = re.compile(r"\b\d{10,17}\b")
ROUTING_REGEX = re.compile(r"\b\d{9}\b")
# secret-scan-allow: detector regex intentionally names credential shapes so telemetry can redact them
SECRET_REGEX = re.compile(
    r"(?i)(bearer\s+[a-z0-9_\-\.]{20,}|"
    r"(ghp|gho|xoxb|xoxp|sk_live|secret|token)_[a-z0-9_\-]{16,}|"
    r"BEGIN\s+(RSA|OPENSSH|PGP|EC)\s+PRIVATE\s+KEY)"
)
CONTROL_CHARS_REGEX = re.compile(r"[\x00-\x08\x0b\x0c\x0e-\x1f\x7f]")
LINE_BREAK_REGEX = re.compile(r"[\r\n]+")


def _sanitize_string(value: str) -> str:
    """Returns a single-line, minimized telemetry-safe string."""
    value = NACHA_94_REGEX.sub("[NACHA_RECORD_REDACTED]", value)
    value = ACCOUNT_REGEX.sub("[ACCOUNT_REDACTED]", value)
    value = ROUTING_REGEX.sub("[ROUTING_REDACTED]", value)
    value = SECRET_REGEX.sub("[SECRET_REDACTED]", value)
    value = CONTROL_CHARS_REGEX.sub("", value)
    # Preserve the text as one attribute value instead of allowing an attacker
    # to visually forge additional structured log/span fields.
    value = LINE_BREAK_REGEX.sub(" ", value)
    return value.strip()


def sanitize_span_attributes(attributes: Dict[str, Any]) -> Dict[str, Any]:
    """Sanitize span values while preserving the original attribute keys."""
    sanitized: Dict[str, Any] = {}
    for key, value in attributes.items():
        if isinstance(value, str):
            sanitized[key] = _sanitize_string(value)
        else:
            sanitized[key] = value
    return sanitized


class MockSpan:
    """Lightweight in-memory span for offline tests."""

    def __init__(self, name: str, attributes: Optional[Dict[str, Any]] = None):
        self.name = name
        self.attributes = sanitize_span_attributes(attributes or {})
        self.events = []
        self.status = "OK"

    def set_attribute(self, key: str, value: Any) -> None:
        self.attributes.update(sanitize_span_attributes({key: value}))

    def set_status(self, status: str) -> None:
        self.status = status

    def __enter__(self) -> "MockSpan":
        return self

    def __exit__(self, exc_type, exc_val, exc_tb) -> None:
        if exc_type:
            self.status = "ERROR"
            self.set_attribute("error.type", exc_type.__name__)


class MockTracer:
    """Mock tracer for local execution without remote GCP calls."""

    def __init__(self, instrument_name: str):
        self.instrument_name = instrument_name
        self.spans: list[MockSpan] = []

    @contextmanager
    def start_as_current_span(
        self,
        name: str,
        attributes: Optional[Dict[str, Any]] = None,
    ):
        span = MockSpan(name, attributes)
        self.spans.append(span)
        try:
            yield span
        finally:
            pass


_GLOBAL_TRACERS: Dict[str, MockTracer] = {}


def get_tracer(instrument_name: str = "sentinelflow.default") -> MockTracer:
    """Return a process-local tracer for deterministic/offline execution."""
    if instrument_name not in _GLOBAL_TRACERS:
        _GLOBAL_TRACERS[instrument_name] = MockTracer(instrument_name)
    return _GLOBAL_TRACERS[instrument_name]


def configure_agent_observability() -> None:
    """Disable prompt/model-content capture in managed tracing integrations."""
    os.environ["OTEL_INSTRUMENTATION_GENAI_CAPTURE_MESSAGE_CONTENT"] = "NO_CONTENT"
    os.environ["ADK_CAPTURE_MESSAGE_CONTENT_IN_SPANS"] = "false"
    os.environ["OTEL_SEMCONV_STABILITY_OPT_IN"] = "gen_ai_latest_experimental"
    logger.info("Configured privacy-preserving Agent Observability (NO_CONTENT in spans)")
