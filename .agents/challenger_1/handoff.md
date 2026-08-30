# Adversarial Challenge Report: OpenTelemetry Telemetry & PII Sanitization

**Agent:** `challenger_1` (EMPIRICAL CHALLENGER: critic, specialist)  
**Working Directory:** `C:\Users\Gathu\Projects\fintech\.agents\challenger_1`  
**Target File:** `ai-tier/observability/telemetry.py`  
**Test Suite:** `ai-tier/tests/test_observability_adversarial.py`  
**Date:** 2026-08-28T05:15:00Z  
**Verdict:** `REQUEST_CHANGES` (2 Critical, 1 High, 2 Medium vulnerabilities confirmed empirically)

---

## 1. Observation

Direct empirical observations from executing the stress harness in `ai-tier/tests/test_observability_adversarial.py`:

### Observation 1: PII Bypass in Sequence/List Attributes (`Sequence[str]`)
- **Code Reference**: `ai-tier/observability/telemetry.py:47-55`
  ```python
  def sanitize_span_attributes(attributes: Dict[str, Any]) -> Dict[str, Any]:
      sanitized: Dict[str, Any] = {}
      for key, value in attributes.items():
          if isinstance(value, str):
              sanitized[key] = _sanitize_string(value)
          else:
              sanitized[key] = value
      return sanitized
  ```
- **Test Result**: When setting span attributes with list or tuple of strings:
  `span.set_attribute("accounts", ["123456789012", "987654321098"])`
  `span.set_attribute("routings", ["021000021", "123456789"])`
  `span.set_attribute("tokens", ["sk_live_12345678901234567890"])`
- **Exported Span Attributes in Cloud Trace Exporter**:
  ```python
  {
      'accounts': ('123456789012', '987654321098'),
      'routings': ('021000021', '123456789'),
      'tokens': ('sk_live_12345678901234567890',)
  }
  ```
  OpenTelemetry Python SDK natively supports `Sequence[str]`. Because `isinstance(value, str)` is `False`, all string elements in lists/tuples bypass `_sanitize_string()` completely and export raw unredacted PII to Google Cloud Trace.

### Observation 2: Raw PII & Secret Leak via `SanitizedSpan.record_exception()`
- **Code Reference**: `ai-tier/observability/telemetry.py:173-186`
  ```python
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
  ```
- **Test Result**: When recording an exception containing sensitive data:
  `span.record_exception(ValueError("Payment failed: routing 021000021, account 123456789012, secret sk_live_12345678901234567890"))`
- **Exported Event Attributes on Span**:
  ```python
  {
      'exception.type': 'ValueError',
      'exception.message': 'Payment failed: routing 021000021, account 123456789012, secret sk_live_12345678901234567890',
      'exception.stacktrace': 'ValueError: Payment failed: routing 021000021, account 123456789012, secret sk_live_12345678901234567890\n',
      'exception.escaped': 'False'
  }
  ```
  The underlying OpenTelemetry SDK `record_exception` creates an event named `"exception"` with `exception.message = str(exception)` and `exception.stacktrace = traceback.format_exc()`. `SanitizedSpan` passes the raw exception object to the SDK without sanitizing its string representation or stacktrace, leaking plaintext routing numbers, account numbers, and API tokens in trace events.

### Observation 3: Multi-Line PEM Cryptographic Key Leak
- **Code Reference**: `ai-tier/observability/telemetry.py:25-45`
  `SECRET_REGEX` regex matches `BEGIN RSA PRIVATE KEY` but does not match the multi-line base64 key body. `LINE_BREAK_REGEX` then replaces newlines with spaces.
- **Test Result**:
  `"-----BEGIN RSA PRIVATE KEY-----\nMIIEowIBAAKCAQEA...\n-----END RSA PRIVATE KEY-----"`
  Transforms to:
  `"-----[SECRET_REDACTED]----- MIIEowIBAAKCAQEA... -----END RSA PRIVATE KEY-----"`
  The entire private key payload remains in the exported attribute.

### Observation 4: Delimited SSNs (`123-45-6789`) Bypass Redaction
- **Test Result**:
  - `SSN: 123-45-6789` -> `SSN: 123-45-6789` (Unredacted)
  - `SSN: 123 45 6789` -> `SSN: 123 45 6789` (Unredacted)
  - `SSN: 123.45.6789` -> `SSN: 123.45.6789` (Unredacted)
  `ROUTING_REGEX` (`\b\d{9}\b`) only matches contiguous 9 digits. Formatted SSNs pass through unredacted.

### Observation 5: PII in Span Attribute Keys
- **Test Result**: `span.set_attribute("sk_live_12345678901234567890", "val")` results in exported key `"sk_live_12345678901234567890"`. Keys are never passed to `_sanitize_string()`.

### Observation 6: Concurrency & Interface Parity
- **Concurrency**: 50 concurrent threads executing 1,000 mutations across `SanitizedSpan` and `MockSpan` completed without exceptions, deadlocks, or state corruption (`PASSED`).
- **Interface Parity**: All 8 core methods (`set_attribute`, `set_attributes`, `set_status`, `record_exception`, `add_event`, `end`, `is_recording`, `get_span_context`) are implemented on both `MockSpan` and `SanitizedSpan` (`PASSED`).

---

## 2. Logic Chain

1. *From Requirement R2*: "In production, span attributes leave the process boundary to Google Cloud Trace; unsanitized attributes constitute a critical PII egress violation."
2. *From Observation 1*: OpenTelemetry SDK supports sequence attributes (`Sequence[str]`). `sanitize_span_attributes` only checks `isinstance(value, str)`, so any sequence attribute is passed without sanitization. An agent logging a list of account numbers or tokens causes immediate PII egress to Google Cloud Trace.
3. *From Observation 2*: When exceptions occur, errors containing account numbers, routing numbers, or auth headers are passed to `span.record_exception(e)`. The SDK creates `"exception"` events containing `str(e)` and the stacktrace. Because `SanitizedSpan` does not sanitize the exception message or stacktrace, full PII egress occurs through trace event payloads.
4. *From Observation 3 & 4*: The regexes do not account for multi-line private key bodies or formatted SSN delimiters, allowing cryptographic keys and formatted SSNs to egress to Cloud Trace.
5. *From Observations 1-5*: The PII sanitization boundary has critical leak vectors that must be remediated before production enablement.

---

## 3. Caveats

- `MockSpan` already sanitizes `error.message` because it routes exception recording through its internal `set_attribute()` method. The vulnerability is specifically present in `SanitizedSpan` when live tracing is enabled (`SENTINEL_OTEL_ENABLED="true"`).
- OTel SDK automatically ignores non-primitive values (such as arbitrary dictionaries and custom objects) in span attributes with an SDK warning, so nested dicts do not leak directly as attribute values, but lists/tuples of strings DO export.
- Concurrency testing was executed under CPython 3.13 standard thread model.

---

## 4. Conclusion

**Verdict: `REQUEST_CHANGES`**

The OpenTelemetry integration has solid architectural foundations (W3C propagation, environment gating, canonical span catalog, concurrency safety), but contains **two critical PII egress vulnerabilities** in `ai-tier/observability/telemetry.py`:

### Required Changes:

1. **Fix Sequence Attribute Sanitization (Critical)**:
   In `sanitize_span_attributes`:
   ```python
   def sanitize_span_attributes(attributes: Dict[str, Any]) -> Dict[str, Any]:
       sanitized: Dict[str, Any] = {}
       for key, value in attributes.items():
           sanitized_key = _sanitize_string(key) if isinstance(key, str) else key
           if isinstance(value, str):
               sanitized[sanitized_key] = _sanitize_string(value)
           elif isinstance(value, (list, tuple, set)):
               sanitized[sanitized_key] = type(value)(
                   _sanitize_string(v) if isinstance(v, str) else v for v in value
               )
           else:
               sanitized[sanitized_key] = value
       return sanitized
   ```

2. **Fix Exception Recording Sanitization (Critical)**:
   In `SanitizedSpan.record_exception`:
   Sanitize the exception message and pass sanitized exception attributes to avoid leaking raw exception text and stacktraces into Cloud Trace events. For example, wrap or record a sanitized exception:
   ```python
   def record_exception(
       self,
       exception: Exception,
       attributes: Optional[Dict[str, Any]] = None,
       timestamp: Optional[int] = None,
       escaped: bool = False,
   ) -> None:
       sanitized_msg = _sanitize_string(str(exception))
       sanitized_attrs = sanitize_span_attributes(attributes or {}) if attributes else {}
       # Create a safe exception instance with sanitized message
       safe_exception = type(exception)(sanitized_msg)
       self._span.record_exception(
           safe_exception,
           attributes=sanitized_attrs or None,
           timestamp=timestamp,
           escaped=escaped,
       )
   ```

3. **Multi-line PEM Key Redaction (High)**:
   Enhance `SECRET_REGEX` or add a block replacer in `_sanitize_string` to redact entire PEM blocks:
   ```python
   PEM_BLOCK_REGEX = re.compile(r"-----BEGIN [A-Z ]+ PRIVATE KEY-----[\s\S]*?-----END [A-Z ]+ PRIVATE KEY-----")
   ```

4. **SSN Formatted Delimiters Redaction (Medium)**:
   Add SSN delimited regex:
   ```python
   SSN_REGEX = re.compile(r"\b\d{3}[-\s._]\d{2}[-\s._]\d{4}\b")
   ```

---

## 5. Verification Method

To reproduce and verify these findings empirically:

1. **Run the Adversarial Test Suite**:
   ```powershell
   pytest ai-tier/tests/test_observability_adversarial.py -v
   ```
   *Expected Output*: 16 passed, confirming empirical reproduction of all 6 observations.

2. **Inspect Specific Failure Scenarios**:
   - Sequence PII bypass: `TestComplexTypesSanitization.test_list_of_strings_containing_pii_bypass`
   - Exception leak: `TestExceptionPIISanitization.test_sanitized_span_record_exception_message_leak`
   - PEM key leak: `TestPIIEdgeCases.test_multiline_secrets_and_private_keys`
   - SSN bypass: `TestPIIEdgeCases.test_ssn_variations`
