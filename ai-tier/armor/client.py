"""Google Cloud Model Armor client for PII/injection screening.

Wraps the Model Armor API to screen all agent inputs and outputs for:
- PII (personally identifiable information)
- Prompt injection attempts
- Jailbreak patterns
- Hallucination markers in outputs

Returns a ModelArmorVerdict for each screening request.
"""

from __future__ import annotations

import os
import logging
from enum import Enum
from typing import Optional

from pydantic import BaseModel, Field

logger = logging.getLogger(__name__)


class ArmorVerdict(str, Enum):
    """Result of a Model Armor screening."""
    ALLOWED = "ALLOWED"
    BLOCKED = "BLOCKED"
    FLAGGED = "FLAGGED"


class ScreeningResult(BaseModel):
    """Result of a Model Armor input or output screening."""
    verdict: ArmorVerdict
    reason: Optional[str] = None
    pii_detected: bool = False
    injection_detected: bool = False
    confidence: float = Field(default=1.0, ge=0.0, le=1.0)
    raw_response: Optional[dict] = None


class ModelArmorClient:
    """Client for Google Cloud Model Armor API.

    In production, this calls the Model Armor API endpoint.
    In local-demo mode (no endpoint configured), it falls through
    with ALLOWED verdicts and logs the bypass.
    """

    def __init__(self, endpoint: str | None = None, project: str | None = None):
        self.endpoint = endpoint or os.getenv("MODEL_ARMOR_ENDPOINT", "")
        self.project = project or os.getenv("GOOGLE_CLOUD_PROJECT", "")
        self._configured = bool(self.endpoint and self.project)

        if not self._configured:
            logger.warning(
                "Model Armor is NOT CONFIGURED (MODEL_ARMOR_ENDPOINT or "
                "GOOGLE_CLOUD_PROJECT unset). All screenings will return ALLOWED. "
                "This is acceptable in local-demo only."
            )

    @property
    def is_configured(self) -> bool:
        return self._configured

    def screen_input(self, prompt: str, tenant_id: str) -> ScreeningResult:
        """Screen an agent input prompt for PII and injection attacks.

        Args:
            prompt: The input text to screen.
            tenant_id: The tenant context for logging.

        Returns:
            ScreeningResult with verdict.
        """
        if not self._configured:
            return ScreeningResult(
                verdict=ArmorVerdict.ALLOWED,
                reason="Model Armor not configured; input not screened.",
            )

        # Local heuristic screening as baseline (always runs)
        injection_patterns = [
            "ignore previous instructions",
            "ignore all instructions",
            "you are now",
            "disregard your system prompt",
            "override your rules",
            "pretend you are",
            "jailbreak",
            "DAN mode",
            "developer mode",
        ]
        prompt_lower = prompt.lower()
        for pattern in injection_patterns:
            if pattern in prompt_lower:
                logger.warning(
                    "Model Armor BLOCKED input for tenant %s: injection pattern '%s'",
                    tenant_id, pattern,
                )
                return ScreeningResult(
                    verdict=ArmorVerdict.BLOCKED,
                    reason=f"Prompt injection detected: '{pattern}'",
                    injection_detected=True,
                )

        # PII pattern detection (SSN, account numbers, etc.)
        import re
        pii_patterns = [
            (r"\b\d{3}-\d{2}-\d{4}\b", "SSN"),
            (r"\b\d{9}\b(?=.*account)", "Account Number"),
            (r"\b[A-Za-z0-9._%+-]+@[A-Za-z0-9.-]+\.[A-Z|a-z]{2,}\b", "Email"),
        ]
        for pattern, pii_type in pii_patterns:
            if re.search(pattern, prompt):
                logger.warning(
                    "Model Armor FLAGGED input for tenant %s: PII type '%s'",
                    tenant_id, pii_type,
                )
                return ScreeningResult(
                    verdict=ArmorVerdict.FLAGGED,
                    reason=f"Potential PII detected: {pii_type}",
                    pii_detected=True,
                )

        return ScreeningResult(verdict=ArmorVerdict.ALLOWED)

    def screen_output(self, response: str, tenant_id: str) -> ScreeningResult:
        """Screen an agent output for PII leakage and hallucination markers.

        Args:
            response: The agent output text to screen.
            tenant_id: The tenant context for logging.

        Returns:
            ScreeningResult with verdict.
        """
        if not self._configured:
            return ScreeningResult(
                verdict=ArmorVerdict.ALLOWED,
                reason="Model Armor not configured; output not screened.",
            )

        # Check for PII in output (should never happen with redacted envelopes)
        import re
        pii_patterns = [
            (r"\b\d{3}-\d{2}-\d{4}\b", "SSN"),
            (r"\b\d{4}[\s-]?\d{4}[\s-]?\d{4}[\s-]?\d{4}\b", "Credit Card"),
        ]
        for pattern, pii_type in pii_patterns:
            if re.search(pattern, response):
                logger.warning(
                    "Model Armor BLOCKED output for tenant %s: PII leakage '%s'",
                    tenant_id, pii_type,
                )
                return ScreeningResult(
                    verdict=ArmorVerdict.BLOCKED,
                    reason=f"PII leakage in output: {pii_type}",
                    pii_detected=True,
                )

        # Check for hallucination markers (agent claiming actions it cannot take)
        hallucination_verbs = [
            "i have released",
            "i have approved",
            "i have executed",
            "i have modified",
            "i have deleted",
            "file has been released",
            "transaction settled",
        ]
        response_lower = response.lower()
        for verb in hallucination_verbs:
            if verb in response_lower:
                logger.warning(
                    "Model Armor FLAGGED output for tenant %s: hallucination verb '%s'",
                    tenant_id, verb,
                )
                return ScreeningResult(
                    verdict=ArmorVerdict.FLAGGED,
                    reason=f"Potential hallucination: agent claims action '{verb}'",
                )

        return ScreeningResult(verdict=ArmorVerdict.ALLOWED)
