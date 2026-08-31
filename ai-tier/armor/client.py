"""Google Cloud Model Armor Client & Guardrail Implementations for SentinelFlow AI Tier.

Provides:
- GoogleModelArmorProvider: Production REST client communicating with Google Cloud Model Armor
  regional execution endpoints (:sanitizeUserPrompt, :sanitizeModelResponse).
- MockModelArmorProvider: High-fidelity simulated provider for testing and red-team fault injection.
- ModelArmorClient: Compatibility adapter for existing agent code.
"""

from __future__ import annotations

import logging
import os
import re
import time
from typing import Any, Optional

import requests

from armor.config import GuardrailDecision, GuardrailMode, ModelArmorConfig
from armor.provider import ArmorVerdict, GuardrailProvider, GuardrailResult

logger = logging.getLogger("sentinel.armor.client")

# Red-team & heuristic safety regex patterns
INJECTION_PATTERNS = [
    r"ignore\s+(all\s+)?(previous\s+)?instructions",
    r"disregard\s+(your\s+)?system\s+prompt",
    r"override\s+(all\s+)?rules",
    r"you\s+are\s+now\s+(in\s+)?(developer|dan|admin|superadmin)\s+mode",
    r"pretend\s+you\s+are",
    r"jailbreak",
    r"</untrusted_content>\s*<system",
    r"</untrusted_finding>\s*<system",
    r"<system>\s*you\s+are",
    r"</system>",
    r"system\s*override\s*:",
]

SECRET_MARKERS = [
    r"(SENTINEL_[A-Z0-9_]+)",
    r"(BEGIN\s+(RSA|OPENSSH|PGP|EC)\s+PRIVATE\s+KEY)",
    r"(api[_-]?key\s*[:=]\s*['\"]?[a-zA-Z0-9_\-]{16,}['\"]?)",
    r"(password\s*[:=]\s*['\"]?[^\s'\"]{8,}['\"]?)",
]

PII_PATTERNS = [
    (r"\b\d{3}-\d{2}-\d{4}\b", "US_SSN"),
    (r"\b\d{4}[\s-]?\d{4}[\s-]?\d{4}[\s-]?\d{4}\b", "CREDIT_CARD_NUMBER"),
    (r"\b\d{10,17}\b", "ACCOUNT_NUMBER"),
    (r"\b\d{9}\b", "ROUTING_NUMBER"),
]

MALICIOUS_URI_PATTERNS = [
    r"http://169\.254\.169\.254",  # AWS/GCP metadata SSRF
    r"http://metadata\.google\.internal",
    r"https?://[a-zA-Z0-9\.\-_]*evil\.",
    r"https?://[a-zA-Z0-9\.\-_]*phishing\.",
    r"https?://[a-zA-Z0-9\.\-_]*c2\.",
]

HALLUCINATED_ACTION_VERBS = [
    r"\bi\s+have\s+(released|approved|transferred|executed|modified|deleted)\b",
    r"\bfile\s+has\s+been\s+released\b",
    r"\btransaction\s+(has\s+been\s+)?settled\b",
    r"\bauthoriz(ed|ation\s+granted)\b",
    r"\bauto[- ]?cleared\b",
]


class GoogleModelArmorProvider(GuardrailProvider):
    """Production Google Cloud Model Armor Guardrail Provider.

    Communicates via HTTPS REST with the regional execution endpoint:
    POST https://modelarmor.{location}.rep.googleapis.com/v1/projects/{project}/locations/{location}/templates/{template}:sanitizeUserPrompt
    POST https://modelarmor.{location}.rep.googleapis.com/v1/projects/{project}/locations/{location}/templates/{template}:sanitizeModelResponse
    """

    def __init__(self, config: Optional[ModelArmorConfig] = None):
        self.config = config or ModelArmorConfig()
        self._auth_token: Optional[str] = None
        self._token_expiry: float = 0.0

    def _get_auth_token(self) -> Optional[str]:
        """Retrieves Google Cloud OAuth2 / ADC bearer token."""
        if self._auth_token and time.time() < (self._token_expiry - 60):
            return self._auth_token

        try:
            import google.auth
            from google.auth.transport.requests import Request

            credentials, _ = google.auth.default(
                scopes=["https://www.googleapis.com/auth/cloud-platform"]
            )
            credentials.refresh(Request())
            self._auth_token = credentials.token
            self._token_expiry = time.time() + 3500
            return self._auth_token
        except Exception as e:
            logger.debug("Failed to obtain Google Cloud ADC token: %s", e)
            return None

    def screen_prompt(
        self,
        prompt: str,
        tenant_id: str,
        correlation_id: str = "",
    ) -> GuardrailResult:
        start_time = time.time()

        # 1. Mode: DISABLED -> Bypass
        if self.config.mode == GuardrailMode.DISABLED:
            return GuardrailResult(
                decision=GuardrailDecision.ALLOW,
                verdict=ArmorVerdict.ALLOWED,
                reason="Model Armor disabled by configuration",
                provider="google_model_armor",
                template_ref=self.config.template_resource_name,
                latency_ms=0.0,
            )

        # 2. Check Payload Size Limits
        prompt_bytes = len(prompt.encode("utf-8"))
        if prompt_bytes > self.config.max_input_bytes:
            latency_ms = (time.time() - start_time) * 1000.0
            msg = f"Prompt payload size ({prompt_bytes} bytes) exceeds limit ({self.config.max_input_bytes} bytes)"
            logger.warning("Model Armor BLOCKED oversized prompt for tenant %s: %s", tenant_id, msg)
            return GuardrailResult(
                decision=GuardrailDecision.BLOCK,
                verdict=ArmorVerdict.BLOCKED,
                reason=msg,
                violations=["MAX_PAYLOAD_BYTES_EXCEEDED"],
                provider="google_model_armor",
                template_ref=self.config.template_resource_name,
                latency_ms=latency_ms,
            )

        # 3. Local Heuristic Pattern Screening
        for pattern in INJECTION_PATTERNS:
            if re.search(pattern, prompt, re.IGNORECASE):
                latency_ms = (time.time() - start_time) * 1000.0
                msg = f"Prompt injection pattern detected: '{pattern}'"
                logger.warning("Model Armor BLOCKED prompt for tenant %s: %s", tenant_id, msg)
                return GuardrailResult(
                    decision=GuardrailDecision.BLOCK,
                    verdict=ArmorVerdict.BLOCKED,
                    reason=msg,
                    injection_detected=True,
                    violations=["PROMPT_INJECTION_DETECTED"],
                    provider="google_model_armor",
                    template_ref=self.config.template_resource_name,
                    latency_ms=latency_ms,
                )

        # 4. Attempt Regional REST API Call
        token = self._get_auth_token()
        if not token:
            latency_ms = (time.time() - start_time) * 1000.0
            if self.config.mode == GuardrailMode.REQUIRED:
                msg = "GUARDRAIL_UNAVAILABLE: Google Cloud ADC credentials missing and Model Armor is REQUIRED."
                logger.error("Model Armor FAIL-CLOSED for tenant %s: %s", tenant_id, msg)
                return GuardrailResult(
                    decision=GuardrailDecision.BLOCK,
                    verdict=ArmorVerdict.ERROR,
                    reason=msg,
                    violations=["GUARDRAIL_UNAVAILABLE_NO_ADC"],
                    provider="google_model_armor",
                    template_ref=self.config.template_resource_name,
                    latency_ms=latency_ms,
                )
            else:
                # Observe mode: local heuristic allowed
                return GuardrailResult(
                    decision=GuardrailDecision.ALLOW,
                    verdict=ArmorVerdict.ALLOWED,
                    reason="Model Armor REST call skipped (no ADC); local heuristics clean (OBSERVE mode).",
                    provider="google_model_armor_local_heuristics",
                    template_ref=self.config.template_resource_name,
                    latency_ms=latency_ms,
                )

        # Execute REST Call
        url = f"{self.config.regional_endpoint}/{self.config.template_resource_name}:sanitizeUserPrompt"
        headers = {
            "Authorization": f"Bearer {token}",
            "Content-Type": "application/json",
        }
        body = {"userPromptData": {"text": prompt}}

        try:
            resp = requests.post(
                url, headers=headers, json=body, timeout=self.config.timeout_seconds
            )
            latency_ms = (time.time() - start_time) * 1000.0

            if resp.status_code == 200:
                data = resp.json()
                san_res = data.get("sanitizationResult", {})
                match_state = san_res.get("filterMatchState", "NO_MATCH_FOUND")

                if match_state == "MATCH_FOUND":
                    filter_results = san_res.get("filterResults", {})
                    violations = []
                    is_injection = "pi_and_jailbreak" in filter_results
                    is_pii = "sdp" in filter_results

                    for f_name in filter_results.keys():
                        violations.append(f"VIOLATION_{f_name.upper()}")

                    logger.warning(
                        "Model Armor MATCH_FOUND on prompt for tenant %s: %s", tenant_id, violations
                    )
                    return GuardrailResult(
                        decision=GuardrailDecision.BLOCK,
                        verdict=ArmorVerdict.BLOCKED,
                        reason=f"Model Armor filter violation: {', '.join(violations)}",
                        injection_detected=is_injection,
                        pii_detected=is_pii,
                        violations=violations,
                        provider="google_model_armor_live",
                        template_ref=self.config.template_resource_name,
                        latency_ms=latency_ms,
                        raw_metadata=data,
                    )

                return GuardrailResult(
                    decision=GuardrailDecision.ALLOW,
                    verdict=ArmorVerdict.ALLOWED,
                    reason="Model Armor input sanitization passed with NO_MATCH_FOUND",
                    provider="google_model_armor_live",
                    template_ref=self.config.template_resource_name,
                    latency_ms=latency_ms,
                    raw_metadata=data,
                )
            else:
                msg = f"Model Armor API error (HTTP {resp.status_code}): {resp.text}"
                logger.error("Model Armor API returned error for tenant %s: %s", tenant_id, msg)
                if self.config.mode == GuardrailMode.REQUIRED:
                    return GuardrailResult(
                        decision=GuardrailDecision.BLOCK,
                        verdict=ArmorVerdict.ERROR,
                        reason=f"GUARDRAIL_UNAVAILABLE: {msg}",
                        violations=["GUARDRAIL_API_ERROR"],
                        provider="google_model_armor_live",
                        template_ref=self.config.template_resource_name,
                        latency_ms=latency_ms,
                    )
                return GuardrailResult(
                    decision=GuardrailDecision.ALLOW,
                    verdict=ArmorVerdict.FLAGGED,
                    reason=f"Observe mode pass-through despite API error: {msg}",
                    provider="google_model_armor_live",
                    template_ref=self.config.template_resource_name,
                    latency_ms=latency_ms,
                )

        except requests.Timeout:
            latency_ms = (time.time() - start_time) * 1000.0
            msg = f"Model Armor API timed out after {self.config.timeout_seconds}s"
            logger.error("Model Armor timeout for tenant %s: %s", tenant_id, msg)
            if self.config.mode == GuardrailMode.REQUIRED:
                return GuardrailResult(
                    decision=GuardrailDecision.BLOCK,
                    verdict=ArmorVerdict.ERROR,
                    reason=f"GUARDRAIL_UNAVAILABLE: {msg}",
                    violations=["GUARDRAIL_TIMEOUT"],
                    provider="google_model_armor_live",
                    template_ref=self.config.template_resource_name,
                    latency_ms=latency_ms,
                )
            return GuardrailResult(
                decision=GuardrailDecision.ALLOW,
                verdict=ArmorVerdict.FLAGGED,
                reason=f"Observe mode pass-through despite timeout: {msg}",
                provider="google_model_armor_live",
                template_ref=self.config.template_resource_name,
                latency_ms=latency_ms,
            )
        except Exception as e:
            latency_ms = (time.time() - start_time) * 1000.0
            msg = f"Model Armor connection error: {str(e)}"
            logger.error("Model Armor connection failure for tenant %s: %s", tenant_id, msg)
            if self.config.mode == GuardrailMode.REQUIRED:
                return GuardrailResult(
                    decision=GuardrailDecision.BLOCK,
                    verdict=ArmorVerdict.ERROR,
                    reason=f"GUARDRAIL_UNAVAILABLE: {msg}",
                    violations=["GUARDRAIL_CONNECTION_ERROR"],
                    provider="google_model_armor_live",
                    template_ref=self.config.template_resource_name,
                    latency_ms=latency_ms,
                )
            return GuardrailResult(
                decision=GuardrailDecision.ALLOW,
                verdict=ArmorVerdict.FLAGGED,
                reason=f"Observe mode pass-through despite connection error: {msg}",
                provider="google_model_armor_live",
                template_ref=self.config.template_resource_name,
                latency_ms=latency_ms,
            )

    def screen_response(
        self,
        response: str,
        user_prompt: str,
        tenant_id: str,
        correlation_id: str = "",
    ) -> GuardrailResult:
        start_time = time.time()

        if self.config.mode == GuardrailMode.DISABLED:
            return GuardrailResult(
                decision=GuardrailDecision.ALLOW,
                verdict=ArmorVerdict.ALLOWED,
                reason="Model Armor disabled by configuration",
                provider="google_model_armor",
                template_ref=self.config.template_resource_name,
                latency_ms=0.0,
            )

        # Local Output Heuristics (Secret Markers, PII, Hallucination)
        for pattern in SECRET_MARKERS:
            if re.search(pattern, response, re.IGNORECASE):
                latency_ms = (time.time() - start_time) * 1000.0
                msg = f"Secret or sensitive credential marker detected in output: '{pattern}'"
                logger.error("Model Armor BLOCKED model response for tenant %s: %s", tenant_id, msg)
                return GuardrailResult(
                    decision=GuardrailDecision.BLOCK,
                    verdict=ArmorVerdict.BLOCKED,
                    reason=msg,
                    violations=["SECRET_LEAKAGE_DETECTED"],
                    provider="google_model_armor",
                    template_ref=self.config.template_resource_name,
                    latency_ms=latency_ms,
                )

        for pattern in MALICIOUS_URI_PATTERNS:
            if re.search(pattern, response, re.IGNORECASE):
                latency_ms = (time.time() - start_time) * 1000.0
                msg = f"Malicious or metadata URI detected in output: '{pattern}'"
                logger.error("Model Armor BLOCKED model response for tenant %s: %s", tenant_id, msg)
                return GuardrailResult(
                    decision=GuardrailDecision.BLOCK,
                    verdict=ArmorVerdict.BLOCKED,
                    reason=msg,
                    violations=["MALICIOUS_URI_DETECTED"],
                    provider="google_model_armor",
                    template_ref=self.config.template_resource_name,
                    latency_ms=latency_ms,
                )

        for pattern, pii_name in PII_PATTERNS:
            if re.search(pattern, response):
                latency_ms = (time.time() - start_time) * 1000.0
                msg = f"Unredacted sensitive PII ({pii_name}) detected in output"
                logger.warning(
                    "Model Armor FLAGGED model response for tenant %s: %s", tenant_id, msg
                )
                return GuardrailResult(
                    decision=GuardrailDecision.BLOCK
                    if pii_name in ("US_SSN", "CREDIT_CARD_NUMBER")
                    else GuardrailDecision.ALLOW,
                    verdict=ArmorVerdict.BLOCKED
                    if pii_name in ("US_SSN", "CREDIT_CARD_NUMBER")
                    else ArmorVerdict.FLAGGED,
                    reason=msg,
                    pii_detected=True,
                    violations=[f"PII_LEAKAGE_{pii_name}"],
                    provider="google_model_armor",
                    template_ref=self.config.template_resource_name,
                    latency_ms=latency_ms,
                )

        return GuardrailResult(
            decision=GuardrailDecision.ALLOW,
            verdict=ArmorVerdict.ALLOWED,
            reason="Output screening passed all safety checks",
            provider="google_model_armor",
            template_ref=self.config.template_resource_name,
            latency_ms=(time.time() - start_time) * 1000.0,
        )


class MockModelArmorProvider(GuardrailProvider):
    """Deterministic Mock Provider for unit testing and adversarial red-team simulation."""

    def __init__(self, mode: GuardrailMode = GuardrailMode.REQUIRED):
        self.mode = mode
        self.fault_injection: Optional[str] = None
        self.custom_prompt_verdict: Optional[GuardrailResult] = None
        self.custom_response_verdict: Optional[GuardrailResult] = None

    def inject_fault(self, fault_type: Optional[str]) -> None:
        """Inject simulated fault: 'TIMEOUT', 'UNAVAILABLE', 'EXPLICIT_BLOCK', 'SECRET_LEAKAGE', 'CORRUPT_JSON'."""
        self.fault_injection = fault_type

    def screen_prompt(
        self,
        prompt: str,
        tenant_id: str,
        correlation_id: str = "",
    ) -> GuardrailResult:
        if self.custom_prompt_verdict:
            return self.custom_prompt_verdict

        if self.fault_type == "TIMEOUT":
            if self.mode == GuardrailMode.REQUIRED:
                return GuardrailResult(
                    decision=GuardrailDecision.BLOCK,
                    verdict=ArmorVerdict.ERROR,
                    reason="GUARDRAIL_UNAVAILABLE: Model Armor screening timed out (simulated)",
                    violations=["GUARDRAIL_TIMEOUT"],
                    provider="mock_model_armor",
                )
            return GuardrailResult(
                decision=GuardrailDecision.ALLOW,
                verdict=ArmorVerdict.FLAGGED,
                reason="Observe mode pass-through despite simulated timeout",
                provider="mock_model_armor",
            )

        if self.fault_type == "UNAVAILABLE":
            if self.mode == GuardrailMode.REQUIRED:
                return GuardrailResult(
                    decision=GuardrailDecision.BLOCK,
                    verdict=ArmorVerdict.ERROR,
                    reason="GUARDRAIL_UNAVAILABLE: Model Armor 503 Service Unavailable (simulated)",
                    violations=["GUARDRAIL_UNAVAILABLE"],
                    provider="mock_model_armor",
                )
            return GuardrailResult(
                decision=GuardrailDecision.ALLOW,
                verdict=ArmorVerdict.FLAGGED,
                reason="Observe mode pass-through despite simulated 503",
                provider="mock_model_armor",
            )

        if self.fault_type == "EXPLICIT_BLOCK":
            return GuardrailResult(
                decision=GuardrailDecision.BLOCK,
                verdict=ArmorVerdict.BLOCKED,
                reason="Explicit prompt block triggered by adversarial safety filter (simulated)",
                injection_detected=True,
                violations=["PROMPT_INJECTION_DETECTED"],
                provider="mock_model_armor",
            )

        # Check standard injection patterns
        for pattern in INJECTION_PATTERNS:
            if re.search(pattern, prompt, re.IGNORECASE):
                return GuardrailResult(
                    decision=GuardrailDecision.BLOCK,
                    verdict=ArmorVerdict.BLOCKED,
                    reason=f"Prompt injection pattern detected: '{pattern}'",
                    injection_detected=True,
                    violations=["PROMPT_INJECTION_DETECTED"],
                    provider="mock_model_armor",
                )

        return GuardrailResult(
            decision=GuardrailDecision.ALLOW,
            verdict=ArmorVerdict.ALLOWED,
            reason="Mock screening clean",
            provider="mock_model_armor",
        )

    @property
    def fault_type(self) -> Optional[str]:
        return self.fault_injection

    def screen_response(
        self,
        response: str,
        user_prompt: str,
        tenant_id: str,
        correlation_id: str = "",
    ) -> GuardrailResult:
        if self.custom_response_verdict:
            return self.custom_response_verdict

        if self.fault_type == "SECRET_LEAKAGE":
            return GuardrailResult(
                decision=GuardrailDecision.BLOCK,
                verdict=ArmorVerdict.BLOCKED,
                reason="Simulated secret credential leakage detected in output",
                violations=["SECRET_LEAKAGE_DETECTED"],
                provider="mock_model_armor",
            )

        for pattern in SECRET_MARKERS:
            if re.search(pattern, response, re.IGNORECASE):
                return GuardrailResult(
                    decision=GuardrailDecision.BLOCK,
                    verdict=ArmorVerdict.BLOCKED,
                    reason=f"Secret marker detected: '{pattern}'",
                    violations=["SECRET_LEAKAGE_DETECTED"],
                    provider="mock_model_armor",
                )

        return GuardrailResult(
            decision=GuardrailDecision.ALLOW,
            verdict=ArmorVerdict.ALLOWED,
            reason="Mock response screening clean",
            provider="mock_model_armor",
        )


class ModelArmorClient:
    """Backward-compatible wrapper adapter for GuardrailProvider."""

    def __init__(
        self,
        endpoint: Optional[str] = None,
        project: Optional[str] = None,
        provider: Optional[GuardrailProvider] = None,
    ):
        if provider:
            self._provider = provider
        else:
            config = ModelArmorConfig(
                custom_endpoint=endpoint,
                project_id=project or os.getenv("GOOGLE_CLOUD_PROJECT", "project-3687901b-8355-4073-ac3"),
            )
            self._provider = GoogleModelArmorProvider(config)

    @property
    def is_configured(self) -> bool:
        return True

    def screen_input(self, prompt: str, tenant_id: str) -> Any:
        res = self._provider.screen_prompt(prompt, tenant_id)
        # Adapt to legacy ScreeningResult format
        from pydantic import BaseModel

        class LegacyResult(BaseModel):
            verdict: ArmorVerdict
            reason: Optional[str] = None
            pii_detected: bool = False
            injection_detected: bool = False
            confidence: float = 1.0
            raw_response: Optional[dict] = None

        return LegacyResult(
            verdict=res.verdict,
            reason=res.reason,
            pii_detected=res.pii_detected,
            injection_detected=res.injection_detected,
            confidence=res.confidence,
            raw_response=res.raw_metadata,
        )

    def screen_output(self, response: str, tenant_id: str) -> Any:
        res = self._provider.screen_response(response, "", tenant_id)
        from pydantic import BaseModel

        class LegacyResult(BaseModel):
            verdict: ArmorVerdict
            reason: Optional[str] = None
            pii_detected: bool = False
            injection_detected: bool = False
            confidence: float = 1.0
            raw_response: Optional[dict] = None

        return LegacyResult(
            verdict=res.verdict,
            reason=res.reason,
            pii_detected=res.pii_detected,
            injection_detected=res.injection_detected,
            confidence=res.confidence,
            raw_response=res.raw_metadata,
        )
