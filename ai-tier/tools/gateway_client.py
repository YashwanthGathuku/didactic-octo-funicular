"""Resilient HTTP Client for SentinelFlow Go Tool Gateway.

Connects the Python AI Tier (Google ADK / Gemini) with the Go Tool Gateway.
Enforces distributed tracing, server-side context propagation, idempotency,
and rigorous error classification.
"""

from __future__ import annotations

import json
import logging
import time
import uuid
from dataclasses import dataclass
from typing import Any, Dict, List, Optional, Union
import httpx
from pydantic import BaseModel, Field

logger = logging.getLogger("sentinel.tools.gateway_client")


class ToolGatewayError(Exception):
    """Base exception for all Tool Gateway errors."""
    def __init__(self, message: str, code: str = "TOOL_GATEWAY_ERROR", details: Optional[Dict[str, Any]] = None):
        super().__init__(f"[{code}] {message}")
        self.code = code
        self.message = message
        self.details = details or {}


class ToolPolicyDeniedError(ToolGatewayError):
    """Raised when deterministic policy denies tool execution."""


class ToolPreconditionFailedError(ToolGatewayError):
    """Raised on TOCTOU resource precondition violation."""


class ToolOutputValidationError(ToolGatewayError):
    """Raised when tool output fails schema/data classification/PII validation."""


class ToolTimeoutError(ToolGatewayError):
    """Raised when tool execution exceeds bounded timeout."""


class ToolIdempotencyConflictError(ToolGatewayError):
    """Raised when an idempotency key is reused with differing payload."""


class ToolExecutionRecord(BaseModel):
    """Authoritative tool execution response from Go Tool Gateway."""
    invocation_id: str
    tool_id: str
    tool_version: str = "1.0.0"
    status: str
    output: Optional[Union[Dict[str, Any], List[Any]]] = None
    output_bytes: int = 0
    duration_ms: float = 0.0
    policy_decision_hash: Optional[str] = None
    policy_bundle_hash: Optional[str] = None
    manifest_hash: Optional[str] = None
    output_hash: Optional[str] = None
    timestamp: str = ""
    error: Optional[Dict[str, Any]] = None


@dataclass
class ToolGatewayContext:
    """Security and distributed tracing context injected from authenticated agent envelope."""
    tenant_id: str
    correlation_id: str
    trace_id: Optional[str] = None
    workflow_id: Optional[str] = None
    caller_id: str = "DiagnosisAgent"
    caller_type: str = "AGENT"
    caller_autonomy_level: int = 1
    execution_mode: str = "LIVE"  # LIVE, ADVISORY, SHADOW
    artifact_sha256: Optional[str] = None
    resource_version: int = 1


class ToolGatewayClient:
    """Resilient HTTP client communicating with SentinelFlow Go Tool Gateway."""

    def __init__(
        self,
        base_url: str = "http://127.0.0.1:8080",
        timeout_seconds: float = 5.0,
        max_response_bytes: int = 1024 * 1024,  # 1 MiB limit
        max_retries: int = 2,
    ):
        self.base_url = base_url.rstrip("/")
        self.timeout_seconds = timeout_seconds
        self.max_response_bytes = max_response_bytes
        self.max_retries = max_retries
        self._client = httpx.Client(
            base_url=self.base_url,
            timeout=httpx.Timeout(timeout_seconds, connect=2.0),
            limits=httpx.Limits(max_keepalive_connections=20, max_connections=50),
        )

    def close(self):
        self._client.close()

    def __enter__(self):
        return self

    def __exit__(self, exc_type, exc_val, exc_tb):
        self.close()

    def execute_tool(
        self,
        tool_id: str,
        business_args: Dict[str, Any],
        context: ToolGatewayContext,
        tool_version: str = "1.0.0",
        idempotency_key: Optional[str] = None,
        expected_artifact_sha256: Optional[str] = None,
        expected_row_version: Optional[int] = None,
    ) -> ToolExecutionRecord:
        """Executes a tool on Go Tool Gateway with strict tracing & tenant isolation."""
        if not context.tenant_id:
            raise ToolGatewayError("tenant_id must be provided by server execution context", "MISSING_TENANT_ID")

        req_id = f"tool-req-{uuid.uuid4().hex[:12]}"
        idem_key = idempotency_key or f"ik-{tool_id}-{context.correlation_id}-{uuid.uuid4().hex[:8]}"

        headers = {
            "Content-Type": "application/json",
            "X-Sentinel-Tenant": context.tenant_id,
            "X-Correlation-ID": context.correlation_id,
            "X-Request-ID": req_id,
            "X-Idempotency-Key": idem_key,
        }
        if context.trace_id:
            headers["X-Trace-ID"] = context.trace_id

        # Server-enforced payload structure: business args are cleanly separated from context
        payload: Dict[str, Any] = {
            "tool_id": tool_id,
            "tool_version": tool_version,
            "args": business_args,
            "idempotency_key": idem_key,
        }

        # Optional TOCTOU preconditions
        preconditions: Dict[str, Any] = {}
        if expected_artifact_sha256:
            preconditions["expected_artifact_sha256"] = expected_artifact_sha256
        if expected_row_version is not None:
            preconditions["expected_row_version"] = expected_row_version
        if preconditions:
            payload["resource_preconditions"] = preconditions

        url = f"{self.base_url}/api/v1/tools/execute"
        raw_body = json.dumps(payload).encode("utf-8")

        last_err: Optional[Exception] = None
        for attempt in range(self.max_retries + 1):
            if attempt > 0:
                time.sleep(0.1 * (2 ** (attempt - 1)))  # Exponential backoff

            try:
                resp = self._client.post(url, content=raw_body, headers=headers)

                # Check response size limit
                if len(resp.content) > self.max_response_bytes:
                    raise ToolOutputValidationError(
                        f"Response size {len(resp.content)} exceeded ceiling {self.max_response_bytes} bytes",
                        "RESPONSE_TOO_LARGE",
                    )

                if resp.status_code == 200:
                    data = resp.json()
                    return ToolExecutionRecord(**data)

                # Map HTTP error codes to structured domain exceptions
                if resp.status_code == 403:
                    err_data = (
                        resp.json()
                        if resp.headers.get("content-type", "").startswith("application/json")
                        else {}
                    )
                    code = err_data.get("code", "POLICY_DENIED")
                    msg = err_data.get("message", resp.text)
                    raise ToolPolicyDeniedError(msg, code, err_data)

                if resp.status_code == 409:
                    raise ToolIdempotencyConflictError(
                        f"Idempotency conflict: {resp.text}", "IDEMPOTENCY_CONFLICT"
                    )

                if resp.status_code == 412:
                    raise ToolPreconditionFailedError(
                        f"Precondition failed: {resp.text}", "PRECONDITION_FAILED"
                    )

                if resp.status_code == 422:
                    raise ToolOutputValidationError(
                        f"Validation failed: {resp.text}", "VALIDATION_FAILED"
                    )

                if resp.status_code == 504:
                    raise ToolTimeoutError(
                        f"Tool execution timed out: {resp.text}", "TIMEOUT"
                    )

                # Transient 5xx errors retryable
                if resp.status_code >= 500:
                    last_err = ToolGatewayError(
                        f"Gateway error HTTP {resp.status_code}: {resp.text}", "SERVER_ERROR"
                    )
                    continue

                raise ToolGatewayError(
                    f"Unexpected status HTTP {resp.status_code}: {resp.text}",
                    "UNEXPECTED_HTTP_STATUS",
                )

            except (httpx.TimeoutException, httpx.NetworkError) as e:
                last_err = ToolTimeoutError(
                    f"Network / timeout error communicating with Tool Gateway: {str(e)}",
                    "NETWORK_ERROR",
                )
                continue

        raise last_err or ToolGatewayError("Max retries exhausted", "RETRIES_EXHAUSTED")
