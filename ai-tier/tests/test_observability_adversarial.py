"""Adversarial and Stress Test Suite for SentinelFlow Telemetry (ai-tier/observability/telemetry.py).

Empirical Challenger Suite:
1. Complex and nested attribute types (sequences, dicts, tuples, sets).
2. PII edge cases: SSNs, formatted accounts, punctuation in NACHA 94, multi-line PEM keys, control/unicode chars.
3. Exception recording PII leak verification on SanitizedSpan vs MockSpan.
4. Multithreaded concurrency stress testing on span attribute mutations.
5. Interface parity between MockSpan, SanitizedSpan, and OpenTelemetry SDK Span.
"""

from __future__ import annotations

import concurrent.futures
import os
import re
import threading
import time
from typing import Any, Dict, List, Optional
import pytest

from opentelemetry import trace
from opentelemetry.sdk.trace import TracerProvider
from opentelemetry.sdk.trace.export import SimpleSpanProcessor
from opentelemetry.sdk.trace.export.in_memory_span_exporter import InMemorySpanExporter
from opentelemetry.trace import StatusCode, format_trace_id, format_span_id

from observability.telemetry import (
    MockSpan,
    MockTracer,
    SanitizedSpan,
    SanitizedTracer,
    configure_agent_observability,
    get_tracer,
    sanitize_span_attributes,
    _sanitize_string,
    _set_tracer_provider,
    _reset_tracer_provider,
)


@pytest.fixture(autouse=True)
def clean_telemetry_state(monkeypatch):
    """Ensure clean state before and after each test."""
    _reset_tracer_provider()
    monkeypatch.delenv("SENTINEL_OTEL_ENABLED", raising=False)
    monkeypatch.delenv("GOOGLE_CLOUD_PROJECT", raising=False)
    yield
    _reset_tracer_provider()


@pytest.fixture
def memory_tracer(monkeypatch):
    """Configures a live SanitizedTracer with InMemorySpanExporter."""
    monkeypatch.setenv("SENTINEL_OTEL_ENABLED", "true")
    exporter = InMemorySpanExporter()
    provider = TracerProvider()
    provider.add_span_processor(SimpleSpanProcessor(exporter))
    _set_tracer_provider(provider)
    sanitized_tracer = get_tracer("sentinelflow.adversarial")
    return sanitized_tracer, exporter


# ==============================================================================
# Suite 1: Complex & Nested Types in Span Attributes
# ==============================================================================
class TestComplexTypesSanitization:
    """Stress-test how sanitize_span_attributes and SanitizedSpan handle non-string and nested types."""

    def test_list_of_strings_containing_pii_bypass(self, memory_tracer):
        """Empirically verify: Sequence of strings (e.g. List[str]) containing PII bypasses sanitization."""
        tracer, exporter = memory_tracer

        with tracer.start_as_current_span("batch_process") as span:
            span.set_attribute("accounts", ["123456789012", "987654321098"])
            span.set_attribute("routings", ["021000021", "123456789"])
            span.set_attribute("tokens", ["sk_live_12345678901234567890"])

        spans = exporter.get_finished_spans()
        assert len(spans) == 1
        attrs = spans[0].attributes

        # OpenTelemetry SDK accepts Sequence[str].
        # In current implementation, list elements are NOT sanitized.
        assert attrs["accounts"] == ("123456789012", "987654321098")
        assert attrs["routings"] == ("021000021", "123456789")
        assert attrs["tokens"] == ("sk_live_12345678901234567890",)

    def test_nested_dict_and_list_of_dicts_behavior(self, memory_tracer):
        """Test behavior when passing nested dicts or lists of dicts."""
        tracer, exporter = memory_tracer

        with tracer.start_as_current_span("nested_data") as span:
            # Note: OTel SDK drops non-primitive attributes with a warning
            span.set_attribute("user_profile", {"account": "123456789012", "routing": "021000021"})
            span.set_attribute("records", [{"nacha": "6" + "A" * 93}])

        spans = exporter.get_finished_spans()
        assert len(spans) == 1
        attrs = spans[0].attributes
        # Dict and list of dicts are non-primitive, OTel SDK rejects them from span attributes
        assert "user_profile" not in attrs or attrs["user_profile"] == {"account": "123456789012", "routing": "021000021"}

    def test_tuple_and_set_attribute_values(self, memory_tracer):
        """Test tuple and set values containing sensitive strings."""
        tracer, exporter = memory_tracer

        with tracer.start_as_current_span("collection_data") as span:
            span.set_attribute("tuple_data", ("021000021", "123456789012"))

        spans = exporter.get_finished_spans()
        assert len(spans) == 1
        attrs = spans[0].attributes
        # Tuple of strings is converted to tuple in OTel SDK and NOT sanitized
        assert attrs["tuple_data"] == ("021000021", "123456789012")

    def test_pii_in_attribute_keys(self, memory_tracer):
        """Test whether attribute keys containing secrets/PII are preserved without key sanitization."""
        tracer, exporter = memory_tracer

        with tracer.start_as_current_span("key_leak") as span:
            span.set_attribute("sk_live_12345678901234567890", "value_for_secret_key")
            span.set_attribute("account_123456789012", "active")
            span.set_attribute("021000021", "routing_as_key")

        spans = exporter.get_finished_spans()
        assert len(spans) == 1
        attrs = spans[0].attributes
        # Keys are preserved as-is
        assert "sk_live_12345678901234567890" in attrs
        assert "account_123456789012" in attrs
        assert "021000021" in attrs


# ==============================================================================
# Suite 2: PII Edge Cases & Boundary Stress Testing
# ==============================================================================
class TestPIIEdgeCases:
    """Stress-test regex boundaries against adversarial variations."""

    def test_ssn_variations(self):
        """Test SSN formatting (dashes, spaces, dots, raw)."""
        raw_ssn = "123456789"
        dash_ssn = "123-45-6789"
        space_ssn = "123 45 6789"
        dot_ssn = "123.45.6789"
        underscore_ssn = "123_45_6789"

        r_raw = _sanitize_string(f"SSN: {raw_ssn}")
        r_dash = _sanitize_string(f"SSN: {dash_ssn}")
        r_space = _sanitize_string(f"SSN: {space_ssn}")
        r_dot = _sanitize_string(f"SSN: {dot_ssn}")
        r_underscore = _sanitize_string(f"SSN: {underscore_ssn}")

        assert r_raw == "SSN: [ROUTING_REDACTED]"
        # Delimited SSNs are NOT redacted by current regexes
        assert r_dash == "SSN: 123-45-6789"
        assert r_space == "SSN: 123 45 6789"
        assert r_dot == "SSN: 123.45.6789"
        assert r_underscore == "SSN: 123_45_6789"

    def test_account_numbers_in_complex_contexts(self):
        """Test 10-17 digit account numbers embedded in varied contexts."""
        assert _sanitize_string("1234567890") == "[ACCOUNT_REDACTED]"
        assert _sanitize_string("12345678901234567") == "[ACCOUNT_REDACTED]"
        assert _sanitize_string("123456789") == "[ROUTING_REDACTED]"
        # 18 digits exceeds 17-digit limit
        assert _sanitize_string("123456789012345678") == "123456789012345678"
        # Prefixes with word boundaries
        assert _sanitize_string("ACCT#123456789012") == "ACCT#[ACCOUNT_REDACTED]"
        assert _sanitize_string("acc:123456789012") == "acc:[ACCOUNT_REDACTED]"
        # Alpha prefix without word boundary is not matched by \b
        assert _sanitize_string("ACC123456789012") == "ACC123456789012"
        # Formatted account numbers with spaces/dashes are not matched by \b\d{10,17}\b
        assert _sanitize_string("1234 5678 9012") == "1234 5678 9012"
        assert _sanitize_string("1234-5678-9012") == "1234-5678-9012"
        # JSON and SQL contexts
        assert _sanitize_string('{"account_id": "123456789012"}') == '{"account_id": "[ACCOUNT_REDACTED]"}'
        assert _sanitize_string("SELECT * FROM accounts WHERE id = 123456789012") == "SELECT * FROM accounts WHERE id = [ACCOUNT_REDACTED]"

    def test_routing_numbers_whitespace_and_boundaries(self):
        """Test routing number variations."""
        assert _sanitize_string("021000021") == "[ROUTING_REDACTED]"
        assert _sanitize_string("  021000021  ") == "[ROUTING_REDACTED]"
        assert _sanitize_string("\t021000021\t") == "[ROUTING_REDACTED]"
        assert _sanitize_string("RT#021000021") == "RT#[ROUTING_REDACTED]"
        assert _sanitize_string("000000001") == "[ROUTING_REDACTED]"
        # Embedded in alphanumeric ID
        assert _sanitize_string("RT021000021") == "RT021000021"

    def test_nacha_94_records_with_punctuation(self):
        """Test NACHA 94-char records containing punctuation and special chars."""
        clean_nacha = "6" + "A" * 93
        punct_nacha = "620" + "12345678" + "9" + "123456789012" + "0000100000" + "ID-12345/INV*99" + "  " + "JOHN DOE" + "   " + "012345670000001"
        punct_nacha = punct_nacha.ljust(94, " ")[:94]

        # Clean alphanumeric 94-char is redacted
        assert _sanitize_string(clean_nacha) == "[NACHA_RECORD_REDACTED]"
        # Punctuation breaks NACHA_94_REGEX [0-9A-Za-z\s]{90,}
        sanitized_punct = _sanitize_string(punct_nacha)
        assert "[NACHA_RECORD_REDACTED]" not in sanitized_punct
        assert "JOHN DOE" in sanitized_punct

    def test_multiline_secrets_and_private_keys(self):
        """Test multi-line private keys and secrets."""
        pem_key = (
            "-----BEGIN RSA PRIVATE KEY-----\n"
            "MIIEowIBAAKCAQEA0Y+0rQ8V9k1p...\n"
            "wIDAQABAoIBAQCW8+2h...\n"
            "-----END RSA PRIVATE KEY-----"
        )
        s_pem = _sanitize_string(pem_key)
        # Header is replaced by [SECRET_REDACTED], but key body is preserved
        assert "[SECRET_REDACTED]" in s_pem
        assert "MIIEowIBAAKCAQEA0Y+0rQ8V9k1p" in s_pem

        # Bearer multiline
        assert _sanitize_string("Bearer \nabcdef12345678901234567890") == "[SECRET_REDACTED]"

        # Standard API keys
        assert _sanitize_string("sk_live_12345678901234567890") == "[SECRET_REDACTED]"
        assert _sanitize_string("SK_LIVE_12345678901234567890") == "[SECRET_REDACTED]"
        assert _sanitize_string("ghp_abcdefghijklmnopqrstuvwxyz123456") == "[SECRET_REDACTED]"

    def test_control_chars_and_unicode_spoofing(self):
        """Test control characters, unicode bi-directional overrides, and zero-width chars."""
        # Zero-width space inside an account number bypasses \d regex
        zw_account = "12345\u200b6789012"
        s_zw = _sanitize_string(zw_account)
        assert s_zw == "12345\u200b6789012"

        # Control characters are stripped
        control_string = "safe\x00data\x07with\x0ccontrols\x1b[31mcolor"
        s_ctrl = _sanitize_string(control_string)
        assert "\x00" not in s_ctrl and "\x07" not in s_ctrl and "\x0c" not in s_ctrl


# ==============================================================================
# Suite 3: Exception Recording PII Leak Analysis
# ==============================================================================
class TestExceptionPIISanitization:
    """Test whether record_exception sanitizes exception messages on SanitizedSpan vs MockSpan."""

    def test_sanitized_span_record_exception_message_leak(self, memory_tracer):
        """Verify: exception.message leaks raw PII when recorded via SanitizedSpan.record_exception."""
        tracer, exporter = memory_tracer

        sensitive_error = ValueError("Payment failed: routing 021000021, account 123456789012, secret sk_live_12345678901234567890")

        with tracer.start_as_current_span("exception_test") as span:
            span.record_exception(sensitive_error, attributes={"extra.info": "Routing 021000021"})

        spans = exporter.get_finished_spans()
        assert len(spans) == 1
        exported_span = spans[0]

        assert len(exported_span.events) == 1
        exc_event = exported_span.events[0]
        event_attrs = exc_event.attributes

        # The attributes kwarg is sanitized
        assert event_attrs["extra.info"] == "Routing [ROUTING_REDACTED]"

        # BUT the exception.message and exception.stacktrace created by OTel SDK contain raw PII
        assert "021000021" in event_attrs["exception.message"]
        assert "123456789012" in event_attrs["exception.message"]
        assert "sk_live_12345678901234567890" in event_attrs["exception.message"]

    def test_mock_span_record_exception_sanitization(self):
        """Check how MockSpan handles exception messages."""
        span = MockSpan("mock_exc")
        sensitive_error = ValueError("Payment failed: routing 021000021, account 123456789012")
        span.record_exception(sensitive_error)

        assert span.attributes["error.message"] == "Payment failed: routing [ROUTING_REDACTED], account [ACCOUNT_REDACTED]"


# ==============================================================================
# Suite 4: High Concurrency Stress Testing
# ==============================================================================
class TestConcurrencyStress:
    """Stress-test thread safety of span mutations."""

    def test_multithreaded_sanitized_span_attribute_mutations(self, memory_tracer):
        """Spawn 50 concurrent threads setting attributes and events on the same SanitizedSpan."""
        tracer, exporter = memory_tracer

        num_threads = 50
        mutations_per_thread = 20
        errors: List[Exception] = []

        with tracer.start_as_current_span("concurrent_span") as span:
            def worker(thread_idx: int):
                try:
                    for i in range(mutations_per_thread):
                        key = f"thread_{thread_idx}_key_{i}"
                        val = f"account 123456789012 routing 021000021 iter {i}"
                        span.set_attribute(key, val)
                        if i % 5 == 0:
                            span.set_attributes({
                                f"batch_{thread_idx}_{i}_a": "sk_live_12345678901234567890",
                                f"batch_{thread_idx}_{i}_b": 100 + i,
                            })
                            span.add_event(f"event_{thread_idx}_{i}", {"detail": "021000021"})
                except Exception as e:
                    errors.append(e)

            threads = [threading.Thread(target=worker, args=(t,)) for t in range(num_threads)]
            for t in threads:
                t.start()
            for t in threads:
                t.join()

        assert len(errors) == 0, f"Thread errors: {errors}"
        spans = exporter.get_finished_spans()
        assert len(spans) == 1

    def test_multithreaded_mock_span_mutations(self):
        """Test thread safety of MockSpan."""
        span = MockSpan("concurrent_mock")
        num_threads = 50
        mutations_per_thread = 20
        errors: List[Exception] = []

        def worker(thread_idx: int):
            try:
                for i in range(mutations_per_thread):
                    span.set_attribute(f"k_{thread_idx}_{i}", f"123456789012-{i}")
                    if i % 5 == 0:
                        span.add_event(f"ev_{thread_idx}_{i}", {"val": "021000021"})
            except Exception as e:
                errors.append(e)

        threads = [threading.Thread(target=worker, args=(t,)) for t in range(num_threads)]
        for t in threads:
            t.start()
        for t in threads:
            t.join()

        assert len(errors) == 0


# ==============================================================================
# Suite 5: MockSpan vs SanitizedSpan Interface Parity
# ==============================================================================
class TestInterfaceParity:
    """Verify exact API compatibility between MockSpan and SanitizedSpan."""

    def test_method_signatures_and_attributes(self, memory_tracer):
        """Compare all public methods on MockSpan and SanitizedSpan."""
        tracer, _ = memory_tracer
        mock_span = MockSpan("mock_test")

        with tracer.start_as_current_span("sanitized_test") as sanitized_span:
            core_methods = [
                "set_attribute",
                "set_attributes",
                "set_status",
                "record_exception",
                "add_event",
                "end",
                "is_recording",
                "get_span_context",
            ]
            for m in core_methods:
                assert hasattr(mock_span, m), f"MockSpan missing {m}"
                assert hasattr(sanitized_span, m), f"SanitizedSpan missing {m}"

    def test_none_and_empty_inputs(self, memory_tracer):
        """Test edge cases with None and empty inputs."""
        tracer, exporter = memory_tracer

        with tracer.start_as_current_span("edge_span") as span:
            span.set_attributes({})
            span.add_event("empty_event", None)
            span.add_event("empty_event_2", {})

        mock = MockSpan("mock_edge")
        mock.set_attributes({})
        mock.add_event("empty_event", None)
        mock.add_event("empty_event_2", {})

        assert len(mock.events) == 2
