"""GuardedModelBoundary — Unified Secure Model Boundary Wrapper for SentinelFlow.

Enforces:
1. Pre-invocation Data Minimization (NACHA 94-char records, Account & Routing number masking).
2. 4-Domain Trust Partitioning (SYSTEM_POLICY, TRUSTED_CONTEXT, UNTRUSTED_CONTENT, TOOL_OUTPUT).
3. Pre-invocation Model Armor prompt screening (PII, prompt injection, and jailbreak detection).
4. Governed Model Invocation with structured output schema enforcement and timeout budget.
5. Post-invocation Model Armor screening (PII leakage, credential exposure, and hallucinated actions).
6. Post-invocation Evidence Grounding Verification (ReturnedCitations ⊆ AuthorizedEvidenceSet).
7. Fail-closed deterministic fallback dispatch and tamper-evident audit logging.
"""

from __future__ import annotations

import hashlib
import json
import logging
import os
import re
import time
from dataclasses import dataclass
from typing import Any, Callable, Dict, Generic, List, Optional, Type, TypeVar

from pydantic import BaseModel

from armor.config import GuardrailDecision, ModelArmorConfig
from armor.provider import ArmorVerdict, GuardrailProvider
from armor.client import GoogleModelArmorProvider
from guardrails.evidence import (
    AuthorizedEvidenceSet,
    EvidenceGroundingVerifier,
    GroundingViolationError,
)
from guardrails.prompt import PromptTrustPartitioner
from models.envelope import AgentContextEnvelope
from observability.telemetry import get_tracer

logger = logging.getLogger("sentinel.guardrails.boundary")

T = TypeVar("T", bound=BaseModel)


class DataSovereigntyViolationError(Exception):
    """Raised when a model or memory invocation targets a region outside the tenant's allowed regions.

    This is a typed failure following the same pattern as GroundingViolationError.
    It must NEVER be silently relabelled as deterministic success.
    """

    def __init__(self, message: str, target_region: str, allowed_regions: List[str]):
        super().__init__(
            f"[DATA_SOVEREIGNTY_VIOLATION] {message} "
            f"(target={target_region}, allowed={allowed_regions})"
        )
        self.target_region = target_region
        self.allowed_regions = allowed_regions

# Strict Data Minimization Regex Patterns
ROUTING_NUMBER_REGEX = re.compile(r"\b\d{9}\b")
ACCOUNT_NUMBER_REGEX = re.compile(r"\b\d{10,17}\b")
NACHA_94_RECORD_REGEX = re.compile(r"\b[156789][0-9A-Za-z\s]{93}\b")


@dataclass
class BoundaryAuditRecord:
    """Tamper-evident audit trail for model boundary invocation."""

    model_name: str
    provider: str
    execution_source: str  # LIVE_GEMINI | DETERMINISTIC_FALLBACK | NOT_RUN
    latency_ms: float
    pre_guardrail_input_hash: str
    post_guardrail_input_hash: str
    model_output_hash: str
    post_guardrail_output_hash: str
    prompt_tokens: int = 0
    completion_tokens: int = 0
    total_tokens: int = 0
    estimated_cost_usd: float = 0.0
    guardrail_mode: str = "observe"
    guardrail_input_decision: str = "ALLOW"
    guardrail_output_decision: str = "ALLOW"
    grounding_verdict: str = "VERIFIED"
    error_detail: Optional[Dict[str, Any]] = None


@dataclass
class GuardedInvocationResult(Generic[T]):
    """Unified result container returned by GuardedModelBoundary."""

    success: bool
    output: Optional[T]
    audit: BoundaryAuditRecord
    error_code: Optional[str] = None
    error_message: Optional[str] = None


class GuardedModelBoundary:
    """Centralized, hardened model boundary wrapper for all SentinelFlow agents."""

    def __init__(
        self,
        guardrail_provider: Optional[GuardrailProvider] = None,
        config: Optional[ModelArmorConfig] = None,
        default_model: str = "gemini-3.5-flash",
        provider: Optional[GuardrailProvider] = None,
        location: str = "us-central1",
    ):
        self.config = config or ModelArmorConfig()
        effective_provider = guardrail_provider or provider
        self.guardrail = effective_provider or GoogleModelArmorProvider(self.config)
        self.default_model = os.getenv("SENTINEL_GEMINI_MODEL", default_model)
        self.location = location

    @classmethod
    def sanitize_financial_content(cls, text: str) -> str:
        """Applies strict data minimization to redact sensitive financial identifiers."""
        if not text:
            return ""
        # 1. Mask 94-char NACHA raw records
        sanitized = NACHA_94_RECORD_REGEX.sub("[NACHA_RECORD_REDACTED]", text)
        # 2. Mask account numbers (10-17 digits)
        sanitized = ACCOUNT_NUMBER_REGEX.sub("[ACCOUNT_REDACTED]", sanitized)
        # 3. Mask 9-digit ABA routing transit numbers
        sanitized = ROUTING_NUMBER_REGEX.sub("[ROUTING_REDACTED]", sanitized)
        return sanitized

    @classmethod
    def minimize_envelope(cls, envelope: AgentContextEnvelope) -> AgentContextEnvelope:
        """Produces a sanitized copy of the AgentContextEnvelope with minimized finding data."""
        sanitized_findings = []
        for f in envelope.findings:
            f_copy = f.model_copy(
                update={
                    "description": cls.sanitize_financial_content(f.description),
                    "expected_value": cls.sanitize_financial_content(f.expected_value or "")
                    if f.expected_value
                    else None,
                    "actual_value": cls.sanitize_financial_content(f.actual_value or "")
                    if f.actual_value
                    else None,
                    "evidence_redacted": cls.sanitize_financial_content(f.evidence_redacted or "")
                    if f.evidence_redacted
                    else None,
                }
            )
            sanitized_findings.append(f_copy)

        return envelope.model_copy(
            update={
                "filename": cls.sanitize_financial_content(envelope.filename),
                "findings": sanitized_findings,
            }
        )

    def invoke(
        self,
        envelope: AgentContextEnvelope,
        response_schema: Type[T],
        role_system_prompt: Optional[str] = None,
        tool_outputs: Optional[List[Dict[str, Any]]] = None,
        evidence_set: Optional[AuthorizedEvidenceSet] = None,
        fallback_fn: Optional[Callable[[AgentContextEnvelope, AuthorizedEvidenceSet], T]] = None,
        strict_grounding: bool = True,
        max_tokens: int = 2048,
        temperature: float = 0.1,
        allowed_regions: Optional[List[str]] = None,
    ) -> GuardedInvocationResult[T]:
        """Executes a fully screened, evidence-grounded model call with fail-closed guarantees."""
        start_time = time.time()
        tenant_id = envelope.tenant_id
        correlation_id = (
            envelope.correlation_id or envelope.workflow_id or str(envelope.incident_id)
        )

        # Raw input hash before minimization
        raw_dump = envelope.model_dump_json()
        pre_guardrail_input_hash = hashlib.sha256(raw_dump.encode("utf-8")).hexdigest()

        # 0. Step 0: Pre-Invocation Data Sovereignty Check
        # If allowed_regions is specified and our location is not in it, fail closed.
        # This is a typed failure — NOT a silent fallback to deterministic mode.
        if allowed_regions is not None and self.location not in allowed_regions:
            latency_ms = (time.time() - start_time) * 1000.0
            logger.error(
                "Data sovereignty violation: model boundary location %s not in tenant allowed regions %s for tenant %s",
                self.location,
                allowed_regions,
                tenant_id,
            )
            return GuardedInvocationResult(
                success=False,
                output=None,
                audit=BoundaryAuditRecord(
                    model_name=self.default_model,
                    provider="google",
                    execution_source="NOT_RUN",
                    latency_ms=latency_ms,
                    pre_guardrail_input_hash=pre_guardrail_input_hash,
                    post_guardrail_input_hash="",
                    model_output_hash="",
                    post_guardrail_output_hash="",
                    guardrail_mode=self.config.mode.value,
                    guardrail_input_decision="NOT_SCREENED",
                    guardrail_output_decision="NOT_SCREENED",
                    grounding_verdict="NOT_EVALUATED",
                    error_detail={
                        "code": "DATA_SOVEREIGNTY_VIOLATION",
                        "target_region": self.location,
                        "allowed_regions": allowed_regions,
                        "reason": f"Model endpoint region '{self.location}' is outside tenant's permitted regions {allowed_regions}",
                    },
                ),
                error_code="DATA_SOVEREIGNTY_VIOLATION",
                error_message=f"Model endpoint region '{self.location}' is outside tenant's permitted regions {allowed_regions}",
            )

        # 1. Step 1: Pre-Invocation Data Minimization
        minimized_env = self.minimize_envelope(envelope)

        # 2. Step 2: Initialize Authorized Evidence Set
        active_evidence_set = evidence_set or AuthorizedEvidenceSet.from_envelope(
            minimized_env.model_dump()
        )

        # 3. Step 3: 4-Domain Trust Partitioning
        partitioned = PromptTrustPartitioner.compile(minimized_env, tool_outputs=tool_outputs)
        system_instruction = (
            f"{partitioned.system_instruction}\n\n{role_system_prompt}"
            if role_system_prompt
            else partitioned.system_instruction
        )
        user_prompt = partitioned.user_prompt
        post_guardrail_input_hash = hashlib.sha256(user_prompt.encode("utf-8")).hexdigest()

        tracer = get_tracer("sentinelflow.boundary")

        # 4. Step 4: Pre-Invocation Model Armor Screening
        with tracer.start_as_current_span(
            "sentinelflow.boundary.screen_input",
            attributes={
                "tenant.id": tenant_id,
                "correlation.id": correlation_id,
                "guardrail.mode": self.config.mode.value,
                "pre_guardrail_input_hash": pre_guardrail_input_hash,
                "post_guardrail_input_hash": post_guardrail_input_hash,
            },
        ) as screen_in_span:
            prompt_screening = self.guardrail.screen_prompt(
                user_prompt, tenant_id=tenant_id, correlation_id=correlation_id
            )
            screen_in_span.set_attribute("guardrail.input_decision", prompt_screening.decision.value)
            if prompt_screening.is_blocked:
                screen_in_span.set_attribute("guardrail.verdict", "BLOCKED")

        if prompt_screening.is_blocked:
            latency_ms = (time.time() - start_time) * 1000.0
            error_code = (
                "GUARDRAIL_UNAVAILABLE"
                if (
                    prompt_screening.decision == GuardrailDecision.ERROR
                    or prompt_screening.verdict == ArmorVerdict.ERROR
                    or "GUARDRAIL_UNAVAILABLE" in (prompt_screening.reason or "")
                )
                else "PROMPT_SECURITY_BLOCKED"
            )
            logger.warning(
                "Model Armor screening blocked prompt input for tenant %s (code=%s): %s",
                tenant_id,
                error_code,
                prompt_screening.reason,
            )
            return GuardedInvocationResult(
                success=False,
                output=None,
                audit=BoundaryAuditRecord(
                    model_name=self.default_model,
                    provider="google_model_armor",
                    execution_source="LIVE_GEMINI",
                    latency_ms=latency_ms,
                    pre_guardrail_input_hash=pre_guardrail_input_hash,
                    post_guardrail_input_hash=post_guardrail_input_hash,
                    model_output_hash="",
                    post_guardrail_output_hash="",
                    guardrail_mode=self.config.mode.value,
                    guardrail_input_decision=prompt_screening.decision.value,
                    guardrail_output_decision="NOT_SCREENED",
                    grounding_verdict="UNGROUNDED_REJECTED",
                    error_detail={
                        "code": error_code,
                        "reason": prompt_screening.reason,
                        "violations": prompt_screening.violations,
                    },
                ),
                error_code=error_code,
                error_message=prompt_screening.reason,
            )

        # Check execution mode and API key credentials
        ai_mode = os.getenv("SENTINEL_AI_MODE", "auto").lower()
        google_key = os.getenv("GOOGLE_API_KEY") or os.getenv("GEMINI_API_KEY")

        # 5. Step 5: Governed Live Model Invocation Path
        if google_key and ai_mode in ("live", "auto"):
            try:
                from google import genai
                from google.genai import types as genai_types

                client = genai.Client(api_key=google_key)
                config = genai_types.GenerateContentConfig(
                    system_instruction=system_instruction,
                    temperature=temperature,
                    max_output_tokens=max_tokens,
                    response_mime_type="application/json",
                    response_schema=response_schema,
                )

                with tracer.start_as_current_span(
                    "sentinelflow.boundary.model_call",
                    attributes={
                        "gen_ai.system": "google",
                        "gen_ai.request.model": self.default_model,
                        "model.name": self.default_model,
                        "provider": "google",
                        "execution_source": "LIVE_GEMINI",
                    },
                ) as model_span:
                    response = client.models.generate_content(
                        model=self.default_model,
                        contents=user_prompt,
                        config=config,
                    )

                latency_ms = (time.time() - start_time) * 1000.0
                raw_text = response.text or "{}"
                model_output_hash = hashlib.sha256(raw_text.encode("utf-8")).hexdigest()

                # 6. Step 6: Post-Invocation Model Armor Screening
                with tracer.start_as_current_span(
                    "sentinelflow.boundary.screen_output",
                    attributes={
                        "tenant.id": tenant_id,
                        "correlation.id": correlation_id,
                        "model_output_hash": model_output_hash,
                    },
                ) as screen_out_span:
                    response_screening = self.guardrail.screen_response(
                        raw_text,
                        user_prompt=user_prompt,
                        tenant_id=tenant_id,
                        correlation_id=correlation_id,
                    )
                    screen_out_span.set_attribute("guardrail.output_decision", response_screening.decision.value)

                if response_screening.is_blocked:
                    logger.error(
                        "Model Armor BLOCKED response output for tenant %s: %s",
                        tenant_id,
                        response_screening.reason,
                    )
                    return GuardedInvocationResult(
                        success=False,
                        output=None,
                        audit=BoundaryAuditRecord(
                            model_name=self.default_model,
                            provider="google",
                            execution_source="LIVE_GEMINI",
                            latency_ms=latency_ms,
                            pre_guardrail_input_hash=pre_guardrail_input_hash,
                            post_guardrail_input_hash=post_guardrail_input_hash,
                            model_output_hash=model_output_hash,
                            post_guardrail_output_hash="",
                            guardrail_mode=self.config.mode.value,
                            guardrail_input_decision=prompt_screening.decision.value,
                            guardrail_output_decision=response_screening.decision.value,
                            grounding_verdict="UNGROUNDED_REJECTED",
                            error_detail={
                                "code": "MODEL_RESPONSE_BLOCKED",
                                "reason": response_screening.reason,
                                "violations": response_screening.violations,
                            },
                        ),
                        error_code="MODEL_RESPONSE_BLOCKED",
                        error_message=response_screening.reason,
                    )

                final_text = response_screening.transformed_text or raw_text
                post_guardrail_output_hash = hashlib.sha256(final_text.encode("utf-8")).hexdigest()

                # Validate Structured Schema
                try:
                    parsed_json = json.loads(final_text)
                    validated_output = response_schema.model_validate(parsed_json)
                except Exception as schema_err:
                    logger.error(
                        "Structured schema validation failed on model output: %s", schema_err
                    )
                    return GuardedInvocationResult(
                        success=False,
                        output=None,
                        audit=BoundaryAuditRecord(
                            model_name=self.default_model,
                            provider="google",
                            execution_source="LIVE_GEMINI",
                            latency_ms=latency_ms,
                            pre_guardrail_input_hash=pre_guardrail_input_hash,
                            post_guardrail_input_hash=post_guardrail_input_hash,
                            model_output_hash=model_output_hash,
                            post_guardrail_output_hash=post_guardrail_output_hash,
                            guardrail_mode=self.config.mode.value,
                            guardrail_input_decision=prompt_screening.decision.value,
                            guardrail_output_decision=response_screening.decision.value,
                            grounding_verdict="INVALID_SCHEMA",
                            error_detail={"code": "MODEL_OUTPUT_INVALID", "error": str(schema_err)},
                        ),
                        error_code="MODEL_OUTPUT_INVALID",
                        error_message=f"Model output violates {response_schema.__name__} schema: {schema_err}",
                    )

                # 7. Step 7: Authoritative Evidence Grounding Verification
                if hasattr(validated_output, "evidence_refs"):
                    claimed_refs = getattr(validated_output, "evidence_refs", [])
                    grounding_res = EvidenceGroundingVerifier.verify_references(
                        claimed_refs, active_evidence_set, strict=strict_grounding
                    )
                    if not grounding_res.is_valid:
                        raise GroundingViolationError(
                            grounding_res.error_message or "Evidence grounding failed",
                            list(grounding_res.unauthorized_citations),
                        )
                    grounding_verdict_str = grounding_res.verdict.value
                else:
                    grounding_verdict_str = "VERIFIED"

                # Extract token usage
                prompt_tokens = 0
                candidates_tokens = 0
                if hasattr(response, "usage_metadata") and response.usage_metadata:
                    prompt_tokens = getattr(response.usage_metadata, "prompt_token_count", 0) or 0
                    candidates_tokens = (
                        getattr(response.usage_metadata, "candidates_token_count", 0) or 0
                    )
                total_tokens = prompt_tokens + candidates_tokens

                audit = BoundaryAuditRecord(
                    model_name=self.default_model,
                    provider="google",
                    execution_source="LIVE_GEMINI",
                    latency_ms=latency_ms,
                    pre_guardrail_input_hash=pre_guardrail_input_hash,
                    post_guardrail_input_hash=post_guardrail_input_hash,
                    model_output_hash=model_output_hash,
                    post_guardrail_output_hash=post_guardrail_output_hash,
                    prompt_tokens=prompt_tokens,
                    completion_tokens=candidates_tokens,
                    total_tokens=total_tokens,
                    estimated_cost_usd=(prompt_tokens * 0.000000075)
                    + (candidates_tokens * 0.0000003),
                    guardrail_mode=self.config.mode.value,
                    guardrail_input_decision=prompt_screening.decision.value,
                    guardrail_output_decision=response_screening.decision.value,
                    grounding_verdict=grounding_verdict_str,
                )

                return GuardedInvocationResult(
                    success=True,
                    output=validated_output,
                    audit=audit,
                )

            except GroundingViolationError as e:
                latency_ms = (time.time() - start_time) * 1000.0
                logger.error("Grounding violation in GuardedModelBoundary: %s", e)
                return GuardedInvocationResult(
                    success=False,
                    output=None,
                    audit=BoundaryAuditRecord(
                        model_name=self.default_model,
                        provider="google",
                        execution_source="LIVE_GEMINI",
                        latency_ms=latency_ms,
                        pre_guardrail_input_hash=pre_guardrail_input_hash,
                        post_guardrail_input_hash=post_guardrail_input_hash,
                        model_output_hash="",
                        post_guardrail_output_hash="",
                        guardrail_mode=self.config.mode.value,
                        guardrail_input_decision=prompt_screening.decision.value,
                        guardrail_output_decision="ERROR",
                        grounding_verdict="UNGROUNDED_REJECTED",
                        error_detail={
                            "code": "GROUNDING_VIOLATION",
                            "unauthorized": e.unauthorized_citations,
                        },
                    ),
                    error_code="GROUNDING_VIOLATION",
                    error_message=str(e),
                )
            except Exception as e:
                logger.warning("Live model execution encountered: %s", e)
                if ai_mode == "live":
                    # Fail closed in strict live mode
                    latency_ms = (time.time() - start_time) * 1000.0
                    return GuardedInvocationResult(
                        success=False,
                        output=None,
                        audit=BoundaryAuditRecord(
                            model_name=self.default_model,
                            provider="google",
                            execution_source="LIVE_GEMINI",
                            latency_ms=latency_ms,
                            pre_guardrail_input_hash=pre_guardrail_input_hash,
                            post_guardrail_input_hash=post_guardrail_input_hash,
                            model_output_hash="",
                            post_guardrail_output_hash="",
                            guardrail_mode=self.config.mode.value,
                            guardrail_input_decision=prompt_screening.decision.value,
                            guardrail_output_decision="ERROR",
                            error_detail={"code": "LIVE_EXECUTION_FAILED", "message": str(e)},
                        ),
                        error_code="LIVE_EXECUTION_FAILED",
                        error_message=str(e),
                    )

        # 8. Step 8: Deterministic Rule-Grounded Fallback Path
        if fallback_fn:
            latency_ms = (time.time() - start_time) * 1000.0
            with tracer.start_as_current_span(
                "sentinelflow.boundary.model_call",
                attributes={
                    "gen_ai.system": "sentinelflow.deterministic_fallback",
                    "gen_ai.request.model": "deterministic-rule-engine",
                    "model.name": "deterministic-baseline",
                    "provider": "deterministic",
                    "execution_source": "DETERMINISTIC_FALLBACK",
                },
            ):
                fallback_output = fallback_fn(minimized_env, active_evidence_set)

            with tracer.start_as_current_span(
                "sentinelflow.boundary.screen_output",
                attributes={
                    "tenant.id": tenant_id,
                    "correlation.id": correlation_id,
                    "guardrail.output_decision": "ALLOW",
                },
            ) as fallback_screen_out:
                if strict_grounding and active_evidence_set is not None:
                    grounding_res = EvidenceGroundingVerifier.verify(
                        fallback_output, active_evidence_set, strict=True
                    )
                    if not grounding_res.is_valid:
                        fallback_screen_out.set_attribute("grounding.verdict", "UNGROUNDED_REJECTED")
                        return GuardedInvocationResult(
                            success=False,
                            output=None,
                            audit=BoundaryAuditRecord(
                                model_name="deterministic-baseline",
                                provider="deterministic",
                                execution_source="DETERMINISTIC_FALLBACK",
                                latency_ms=latency_ms,
                                pre_guardrail_input_hash=pre_guardrail_input_hash,
                                post_guardrail_input_hash=post_guardrail_input_hash,
                                model_output_hash="",
                                post_guardrail_output_hash="",
                                guardrail_mode=self.config.mode.value,
                                guardrail_input_decision=prompt_screening.decision.value,
                                guardrail_output_decision="ERROR",
                                grounding_verdict="UNGROUNDED_REJECTED",
                                error_detail={
                                    "code": "GROUNDING_VIOLATION",
                                    "unauthorized": list(grounding_res.unauthorized_citations),
                                },
                            ),
                            error_code="GROUNDING_VIOLATION",
                            error_message=grounding_res.error_message or "Fallback grounding failed",
                        )
                    fallback_screen_out.set_attribute("grounding.verdict", grounding_res.verdict.value)
                else:
                    fallback_screen_out.set_attribute("grounding.verdict", "VERIFIED")

                output_json = fallback_output.model_dump_json()
                fallback_hash = hashlib.sha256(output_json.encode("utf-8")).hexdigest()
                fallback_screen_out.set_attribute("model_output_hash", fallback_hash)

            audit = BoundaryAuditRecord(
                model_name="deterministic-baseline",
                provider="deterministic",
                execution_source="DETERMINISTIC_FALLBACK",
                latency_ms=latency_ms,
                pre_guardrail_input_hash=pre_guardrail_input_hash,
                post_guardrail_input_hash=post_guardrail_input_hash,
                model_output_hash=fallback_hash,
                post_guardrail_output_hash=fallback_hash,
                guardrail_mode=self.config.mode.value,
                guardrail_input_decision=prompt_screening.decision.value,
                guardrail_output_decision="ALLOW",
                grounding_verdict="VERIFIED",
            )
            return GuardedInvocationResult(
                success=True,
                output=fallback_output,
                audit=audit,
            )

        # Fail closed if no fallback provided
        latency_ms = (time.time() - start_time) * 1000.0
        return GuardedInvocationResult(
            success=False,
            output=None,
            audit=BoundaryAuditRecord(
                model_name=self.default_model,
                provider="google",
                execution_source="NOT_RUN",
                latency_ms=latency_ms,
                pre_guardrail_input_hash=pre_guardrail_input_hash,
                post_guardrail_input_hash=post_guardrail_input_hash,
                model_output_hash="",
                post_guardrail_output_hash="",
                guardrail_mode=self.config.mode.value,
                guardrail_input_decision=prompt_screening.decision.value,
                guardrail_output_decision="ERROR",
                error_detail={
                    "code": "PROVIDER_UNAVAILABLE",
                    "message": "No live provider or fallback available",
                },
            ),
            error_code="PROVIDER_UNAVAILABLE",
            error_message="Live Gemini model unavailable and no fallback handler provided",
        )
