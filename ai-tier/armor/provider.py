"""GuardrailProvider Interface & Result Specifications for SentinelFlow AI Tier.

Establishes the contract for prompt and response content screening,
sanitization, violation tracking, and tamper-evident audit logging.
"""

from __future__ import annotations

import abc
from enum import Enum
from typing import Any, Dict, List, Optional
from pydantic import BaseModel, Field

from armor.config import GuardrailDecision


class ArmorVerdict(str, Enum):
    """Result status of a Model Armor screening."""

    ALLOWED = "ALLOWED"
    BLOCKED = "BLOCKED"
    FLAGGED = "FLAGGED"
    ERROR = "ERROR"


class GuardrailResult(BaseModel):
    """Complete, structured screening verdict from a GuardrailProvider."""

    decision: GuardrailDecision = Field(
        ..., description="High-level guardrail action: ALLOW, BLOCK, SANITIZED, ERROR"
    )
    verdict: ArmorVerdict = Field(
        default=ArmorVerdict.ALLOWED, description="Fine-grained screening verdict"
    )
    transformed_text: Optional[str] = Field(
        default=None, description="Sanitized or de-identified text if applicable"
    )
    violations: List[str] = Field(
        default_factory=list, description="List of detected policy or filter violation codes"
    )
    reason: Optional[str] = Field(
        default=None, description="Human-readable explanation of the verdict"
    )
    pii_detected: bool = Field(
        default=False, description="Whether sensitive PII/SDP data was detected"
    )
    injection_detected: bool = Field(
        default=False, description="Whether prompt injection or jailbreak was detected"
    )
    confidence: float = Field(default=1.0, ge=0.0, le=1.0, description="Detection confidence score")
    latency_ms: float = Field(
        default=0.0, description="Screening roundtrip latency in milliseconds"
    )
    provider: str = Field(default="google_model_armor", description="Name of guardrail provider")
    template_ref: str = Field(default="", description="GCP template resource reference")
    raw_metadata: Optional[Dict[str, Any]] = Field(
        default=None, description="Raw response payload from provider"
    )

    @property
    def is_blocked(self) -> bool:
        """Returns True if the content must be blocked from downstream dispatch."""
        return self.decision in (GuardrailDecision.BLOCK, GuardrailDecision.ERROR)

    @property
    def is_allowed(self) -> bool:
        """Returns True if the content is safe to proceed without blocking."""
        return self.decision in (GuardrailDecision.ALLOW, GuardrailDecision.SANITIZED)


class GuardrailProvider(abc.ABC):
    """Abstract interface for input/output AI safety guardrails."""

    @abc.abstractmethod
    def screen_prompt(
        self,
        prompt: str,
        tenant_id: str,
        correlation_id: str = "",
    ) -> GuardrailResult:
        """Screens an input prompt before dispatch to the language model.

        Args:
            prompt: Sanitized candidate prompt string.
            tenant_id: Authenticated tenant identifier for audit isolation.
            correlation_id: Workflow/Incident correlation ID.

        Returns:
            GuardrailResult with authoritative decision.
        """
        raise NotImplementedError

    @abc.abstractmethod
    def screen_response(
        self,
        response: str,
        user_prompt: str,
        tenant_id: str,
        correlation_id: str = "",
    ) -> GuardrailResult:
        """Screens a model output before returning to downstream callers.

        Args:
            response: Raw model output string.
            user_prompt: Original user prompt that produced the output.
            tenant_id: Authenticated tenant identifier for audit isolation.
            correlation_id: Workflow/Incident correlation ID.

        Returns:
            GuardrailResult with authoritative decision and optional sanitized text.
        """
        raise NotImplementedError
