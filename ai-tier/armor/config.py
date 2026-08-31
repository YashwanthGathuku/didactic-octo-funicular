"""Configuration and Enums for SentinelFlow Model Armor AI Boundary Guardrails.

Governs input/output screening, regional execution endpoints, timeouts,
payload size limits, and fail-closed operational modes.
"""

from __future__ import annotations

import os
from enum import Enum
from typing import Optional
from pydantic import BaseModel, Field


class GuardrailMode(str, Enum):
    """Operational mode for Model Armor AI Boundary Guardrails."""

    REQUIRED = "required"  # Strict fail-closed: if guardrail unavailable, AI calls fail closed
    OBSERVE = "observe"  # Advisory/telemetry: logs violations without breaking fallback pipelines
    DISABLED = "disabled"  # Bypassed completely (local offline test harness only)


class GuardrailDecision(str, Enum):
    """Authoritative guardrail content-filtering decision."""

    ALLOW = "ALLOW"
    BLOCK = "BLOCK"
    SANITIZED = "SANITIZED"
    ERROR = "ERROR"


class ModelArmorConfig(BaseModel):
    """Configuration specification for Google Cloud Model Armor integration."""

    mode: GuardrailMode = Field(
        default_factory=lambda: GuardrailMode(
            os.getenv("SENTINEL_MODEL_ARMOR_MODE", os.getenv("MODEL_ARMOR_MODE", "observe")).lower()
        )
    )
    project_id: str = Field(
        default_factory=lambda: os.getenv(
            "SENTINEL_MODEL_ARMOR_PROJECT", os.getenv("GOOGLE_CLOUD_PROJECT", "project-3687901b-8355-4073-ac3")
        )
    )
    location: str = Field(
        default_factory=lambda: os.getenv("SENTINEL_MODEL_ARMOR_LOCATION", "us-central1")
    )
    template_id: str = Field(
        default_factory=lambda: os.getenv(
            "SENTINEL_MODEL_ARMOR_TEMPLATE", "sentinelflow-guardrail-template"
        )
    )
    timeout_seconds: float = Field(
        default_factory=lambda: float(os.getenv("SENTINEL_MODEL_ARMOR_TIMEOUT_SECONDS", "5.0"))
    )
    max_input_bytes: int = Field(
        default_factory=lambda: int(os.getenv("SENTINEL_MODEL_ARMOR_MAX_INPUT_BYTES", "65536"))
    )
    max_output_bytes: int = Field(
        default_factory=lambda: int(os.getenv("SENTINEL_MODEL_ARMOR_MAX_OUTPUT_BYTES", "65536"))
    )
    custom_endpoint: Optional[str] = Field(
        default_factory=lambda: os.getenv("MODEL_ARMOR_ENDPOINT", None)
    )

    @property
    def regional_endpoint(self) -> str:
        """Returns the mandatory regional execution endpoint (REP) for sanitization."""
        if self.custom_endpoint:
            return self.custom_endpoint
        return f"https://modelarmor.{self.location}.rep.googleapis.com/v1"

    @property
    def region(self) -> str:
        return self.location

    @property
    def template_path(self) -> str:
        return f"projects/{self.project_id}/locations/{self.location}/templates/{self.template_id}"

    @property
    def fail_closed_on_outage(self) -> bool:
        return self.mode == GuardrailMode.REQUIRED

    @property
    def user_prompt_template_name(self) -> str:
        return f"projects/{self.project_id}/locations/{self.location}/templates/{self.template_id}"

    @property
    def model_response_template_name(self) -> str:
        return f"projects/{self.project_id}/locations/{self.location}/templates/{self.template_id}"

    @property
    def template_resource_name(self) -> str:
        """Returns the fully-qualified GCP resource name for the template."""
        return f"projects/{self.project_id}/locations/{self.location}/templates/{self.template_id}"
