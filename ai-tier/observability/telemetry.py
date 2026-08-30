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
from typing import Any, Dict, List, Optional, Union

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
        self.events: List[Dict[str, Any]] = []
        self.status = "OK"

    def set_attribute(self, key: str, value: Any) -> None:
        self.attributes.update(sanitize_span_attributes({key: value}))

    def set_attributes(self, attributes: Dict[str, Any]) -> None:
        self.attributes.update(sanitize_span_attributes(attributes))

    def set_status(self, status: Any, description: Optional[str] = None) -> None:
        self.status = str(status)

    def record_exception(
        self,
        exception: Exception,
        attributes: Optional[Dict[str, Any]] = None,
        **kwargs: Any,
    ) -> None:
        self.set_attribute("error.type", type(exception).__name__)
        self.set_attribute("error.message", str(exception))
        if attributes:
            self.set_attributes(attributes)

    def add_event(
        self,
        name: str,
        attributes: Optional[Dict[str, Any]] = None,
        **kwargs: Any,
    ) -> None:
        self.events.append({
            "name": name,
            "attributes": sanitize_span_attributes(attributes or {}),
        })

    def end(self, **kwargs: Any) -> None:
        pass

    def is_recording(self) -> bool:
        return True

    def get_span_context(self) -> Any:
        return None

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
        self.spans: List[MockSpan] = []

    @contextmanager
    def start_as_current_span(
        self,
        name: str,
        attributes: Optional[Dict[str, Any]] = None,
        context: Optional[Any] = None,
        **kwargs: Any,
    ):
        span = MockSpan(name, attributes)
        self.spans.append(span)
        try:
            yield span
        finally:
            pass

    def start_span(
        self,
        name: str,
        attributes: Optional[Dict[str, Any]] = None,
        context: Optional[Any] = None,
        **kwargs: Any,
    ) -> MockSpan:
        span = MockSpan(name, attributes)
        self.spans.append(span)
        return span


class SanitizedSpan:
    """Wrapper around OpenTelemetry SDK Span to enforce PII/secret attribute sanitization."""

    def __init__(self, span: Any):
        self._span = span

    @property
    def raw_span(self) -> Any:
        return self._span

    def set_attribute(self, key: str, value: Any) -> None:
        sanitized = sanitize_span_attributes({key: value})
        for k, v in sanitized.items():
            self._span.set_attribute(k, v)

    def set_attributes(self, attributes: Dict[str, Any]) -> None:
        sanitized = sanitize_span_attributes(attributes)
        self._span.set_attributes(sanitized)

    def set_status(self, status: Any, description: Optional[str] = None) -> None:
        self._span.set_status(status, description)

    def record_exception(
        self,
        exception: Exception,
        attributes: Optional[Dict[str, Any]] = None,
        timestamp: Optional[int] = None,
        escaped: bool = False,
    ) -> None:
        sanitized_attrs = sanitize_span_attributes(attributes or {}) if attributes else None
        self._span.record_exception(
            exception,
            attributes=sanitized_attrs,
            timestamp=timestamp,
            escaped=escaped,
        )

    def add_event(
        self,
        name: str,
        attributes: Optional[Dict[str, Any]] = None,
        timestamp: Optional[int] = None,
    ) -> None:
        sanitized_attrs = sanitize_span_attributes(attributes or {}) if attributes else None
        self._span.add_event(name, attributes=sanitized_attrs, timestamp=timestamp)

    def end(self, end_time: Optional[int] = None) -> None:
        self._span.end(end_time=end_time)

    def is_recording(self) -> bool:
        return self._span.is_recording()

    def get_span_context(self) -> Any:
        return self._span.get_span_context()

    def __enter__(self) -> "SanitizedSpan":
        self._span.__enter__()
        return self

    def __exit__(self, exc_type, exc_val, exc_tb) -> Any:
        return self._span.__exit__(exc_type, exc_val, exc_tb)

    def __getattr__(self, item: str) -> Any:
        return getattr(self._span, item)


class SanitizedTracer:
    """Wrapper around OpenTelemetry SDK Tracer that sanitizes attributes and yields SanitizedSpan."""

    def __init__(self, tracer: Any):
        self._tracer = tracer

    @property
    def raw_tracer(self) -> Any:
        return self._tracer

    @contextmanager
    def start_as_current_span(
        self,
        name: str,
        context: Optional[Any] = None,
        kind: Any = None,
        attributes: Optional[Dict[str, Any]] = None,
        links: Optional[Any] = None,
        start_time: Optional[int] = None,
        record_exception: bool = True,
        set_status_on_exception: bool = True,
        end_on_exit: bool = True,
        **kwargs: Any,
    ):
        sanitized_attrs = sanitize_span_attributes(attributes or {}) if attributes else None
        call_kwargs: Dict[str, Any] = {
            "record_exception": record_exception,
            "set_status_on_exception": set_status_on_exception,
            "end_on_exit": end_on_exit,
        }
        if context is not None:
            call_kwargs["context"] = context
        if kind is not None:
            call_kwargs["kind"] = kind
        if sanitized_attrs is not None:
            call_kwargs["attributes"] = sanitized_attrs
        if links is not None:
            call_kwargs["links"] = links
        if start_time is not None:
            call_kwargs["start_time"] = start_time
        call_kwargs.update(kwargs)

        with self._tracer.start_as_current_span(name, **call_kwargs) as span:
            yield SanitizedSpan(span)

    def start_span(
        self,
        name: str,
        context: Optional[Any] = None,
        kind: Any = None,
        attributes: Optional[Dict[str, Any]] = None,
        links: Optional[Any] = None,
        start_time: Optional[int] = None,
        record_exception: bool = True,
        set_status_on_exception: bool = True,
        **kwargs: Any,
    ) -> SanitizedSpan:
        sanitized_attrs = sanitize_span_attributes(attributes or {}) if attributes else None
        call_kwargs: Dict[str, Any] = {
            "record_exception": record_exception,
            "set_status_on_exception": set_status_on_exception,
        }
        if context is not None:
            call_kwargs["context"] = context
        if kind is not None:
            call_kwargs["kind"] = kind
        if sanitized_attrs is not None:
            call_kwargs["attributes"] = sanitized_attrs
        if links is not None:
            call_kwargs["links"] = links
        if start_time is not None:
            call_kwargs["start_time"] = start_time
        call_kwargs.update(kwargs)

        span = self._tracer.start_span(name, **call_kwargs)
        return SanitizedSpan(span)


def extract_trace_context(
    headers: Optional[Dict[str, str]] = None,
    traceparent: Optional[str] = None,
    trace_id: Optional[str] = None,
) -> Optional[Any]:
    """Extract OpenTelemetry context from W3C traceparent header or carrier dictionary."""
    try:
        from opentelemetry.trace.propagation.tracecontext import TraceContextTextMapPropagator
    except ImportError:
        logger.debug("TraceContextTextMapPropagator not available")
        return None

    carrier: Dict[str, str] = {}
    if headers:
        carrier.update({str(k).lower(): str(v) for k, v in headers.items()})
    if traceparent:
        carrier["traceparent"] = traceparent
    elif "traceparent" not in carrier and trace_id:
        clean_tid = trace_id.replace("-", "").lower()
        if len(clean_tid) < 32:
            clean_tid = clean_tid.zfill(32)
        carrier["traceparent"] = f"00-{clean_tid[:32]}-0000000000000001-01"

    if "traceparent" in carrier:
        try:
            return TraceContextTextMapPropagator().extract(carrier=carrier)
        except Exception as e:
            logger.debug("Failed to extract traceparent: %s", e)
            return None
    return None


def inject_trace_context(
    headers: Dict[str, str],
    context: Optional[Any] = None,
) -> Dict[str, str]:
    """Inject W3C traceparent (and tracestate if present) into headers dictionary."""
    try:
        from opentelemetry.trace.propagation.tracecontext import TraceContextTextMapPropagator
        TraceContextTextMapPropagator().inject(carrier=headers, context=context)
    except Exception as e:
        logger.debug("Failed to inject trace context: %s", e)
    return headers


_REAL_TRACER_PROVIDER: Optional[Any] = None
_GLOBAL_TRACERS: Dict[str, Any] = {}


def configure_agent_observability(project_id: Optional[str] = None) -> None:
    """Configure privacy-preserving Agent Observability and OpenTelemetry Cloud Trace export."""
    global _REAL_TRACER_PROVIDER, _GLOBAL_TRACERS

    os.environ["OTEL_INSTRUMENTATION_GENAI_CAPTURE_MESSAGE_CONTENT"] = "NO_CONTENT"
    os.environ["ADK_CAPTURE_MESSAGE_CONTENT_IN_SPANS"] = "false"
    os.environ["OTEL_SEMCONV_STABILITY_OPT_IN"] = "gen_ai_latest_experimental"

    otel_enabled = os.environ.get("SENTINEL_OTEL_ENABLED", "false").lower() == "true"
    if not otel_enabled:
        _REAL_TRACER_PROVIDER = None
        _GLOBAL_TRACERS.clear()
        logger.info("Configured privacy-preserving Agent Observability (NO_CONTENT in spans, offline/mock mode)")
        return

    # Real OpenTelemetry configuration when SENTINEL_OTEL_ENABLED is "true"
    try:
        from opentelemetry import trace
        from opentelemetry.sdk.trace import TracerProvider
        from opentelemetry.sdk.trace.export import BatchSpanProcessor
        from opentelemetry.exporter.cloud_trace import CloudTraceSpanExporter
    except ImportError as e:
        logger.warning("OpenTelemetry SDK or CloudTraceSpanExporter not available: %s", e)
        return

    # Resolve project ID via parameter, GOOGLE_CLOUD_PROJECT env var, or ADC
    resolved_project_id = project_id or os.environ.get("GOOGLE_CLOUD_PROJECT")
    if not resolved_project_id:
        try:
            import google.auth
            _, default_project = google.auth.default()
            resolved_project_id = default_project
        except Exception as e:
            logger.debug("Failed to resolve Google Cloud Project via ADC: %s", e)

    try:
        exporter = CloudTraceSpanExporter(project_id=resolved_project_id)
        provider = TracerProvider()
        provider.add_span_processor(BatchSpanProcessor(exporter))
        trace.set_tracer_provider(provider)
        _REAL_TRACER_PROVIDER = provider
        _GLOBAL_TRACERS.clear()
        logger.info(
            "Configured real OpenTelemetry TracerProvider with CloudTraceSpanExporter (project: %s)",
            resolved_project_id,
        )
    except Exception as e:
        logger.error("Failed to initialize CloudTraceSpanExporter: %s", e)


def get_tracer(instrument_name: str = "sentinelflow.default") -> Union[MockTracer, SanitizedTracer]:
    """Return an OpenTelemetry tracer: SanitizedTracer when enabled, MockTracer when disabled."""
    otel_enabled = os.environ.get("SENTINEL_OTEL_ENABLED", "false").lower() == "true"

    if not otel_enabled and _REAL_TRACER_PROVIDER is None:
        if instrument_name not in _GLOBAL_TRACERS or not isinstance(_GLOBAL_TRACERS[instrument_name], MockTracer):
            _GLOBAL_TRACERS[instrument_name] = MockTracer(instrument_name)
        return _GLOBAL_TRACERS[instrument_name]

    # Enabled mode
    if instrument_name not in _GLOBAL_TRACERS or isinstance(_GLOBAL_TRACERS[instrument_name], MockTracer):
        try:
            if _REAL_TRACER_PROVIDER is not None:
                raw_tracer = _REAL_TRACER_PROVIDER.get_tracer(instrument_name)
            else:
                from opentelemetry import trace
                raw_tracer = trace.get_tracer(instrument_name)
            _GLOBAL_TRACERS[instrument_name] = SanitizedTracer(raw_tracer)
        except Exception as e:
            logger.warning("Failed to get OpenTelemetry tracer, falling back to MockTracer: %s", e)
            _GLOBAL_TRACERS[instrument_name] = MockTracer(instrument_name)

    return _GLOBAL_TRACERS[instrument_name]


def _set_tracer_provider(provider: Any) -> None:
    """Test helper to register a custom TracerProvider (e.g. with InMemorySpanExporter)."""
    global _REAL_TRACER_PROVIDER, _GLOBAL_TRACERS
    from opentelemetry import trace
    trace._TRACER_PROVIDER = provider
    _REAL_TRACER_PROVIDER = provider
    _GLOBAL_TRACERS.clear()


def _reset_tracer_provider() -> None:
    """Test helper to reset tracer provider state to disabled/mock baseline."""
    global _REAL_TRACER_PROVIDER, _GLOBAL_TRACERS
    from opentelemetry import trace
    trace._TRACER_PROVIDER = None
    _REAL_TRACER_PROVIDER = None
    _GLOBAL_TRACERS.clear()
