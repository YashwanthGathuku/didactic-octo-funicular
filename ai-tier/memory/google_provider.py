"""Google Cloud Agent Platform Memory Bank Provider for SentinelFlow P10.

Communicates via HTTPS REST with Google Agent Platform / Vertex AI Memory Bank API
using Google Application Default Credentials (ADC) bearer tokens.
"""

from __future__ import annotations

import logging
import os
import random
import time
from typing import Any, Dict, List, Optional
import requests

from contracts.memory import (
    MemoryEventEnvelope,
    MemoryHit,
    MemoryQuery,
    PartnerOperationalProfile,
)
from memory.provider import IngestionResult, ManagedMemoryProvider, MemoryProviderHealth
from memory.ranking import DeterministicMemoryRanker

logger = logging.getLogger("sentinel.memory.google")


class GoogleMemoryBankProvider(ManagedMemoryProvider):
    """Production provider integrating with Google Agent Platform Memory Bank."""

    def __init__(
        self,
        project_id: Optional[str] = None,
        location: str = "us-central1",
        agent_id: str = "sentinelflow-agent",
        timeout_seconds: float = 5.0,
        max_retries: int = 3,
    ):
        self.project_id = project_id or os.getenv("GOOGLE_CLOUD_PROJECT", "telos-agent")
        self.location = location
        self.agent_id = agent_id
        self.timeout_seconds = timeout_seconds
        self.max_retries = max_retries
        self.base_url = (
            f"https://agentplatform.{self.location}.rep.googleapis.com/v1/projects/"
            f"{self.project_id}/locations/{self.location}/agents/{self.agent_id}/memoryBank"
        )
        self._auth_token: Optional[str] = None
        self._token_expiry: float = 0.0

    def _get_adc_token(self) -> str:
        """Retrieves or refreshes Google Cloud OAuth2 ADC token."""
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
            logger.error("Failed to acquire Google Cloud ADC token for Memory Bank: %s", e)
            raise RuntimeError(f"Google ADC token acquisition failed: {e}") from e

    def _execute_with_retry(
        self,
        method: str,
        endpoint: str,
        headers: Dict[str, str],
        payload: Optional[Dict[str, Any]] = None,
    ) -> requests.Response:
        """Executes HTTP request with exponential backoff and jitter."""
        url = f"{self.base_url}/{endpoint.lstrip('/')}"
        token = self._get_adc_token()
        headers["Authorization"] = f"Bearer {token}"
        headers["Content-Type"] = "application/json"

        last_err: Optional[Exception] = None
        for attempt in range(1, self.max_retries + 1):
            try:
                resp = requests.request(
                    method=method,
                    url=url,
                    headers=headers,
                    json=payload,
                    timeout=self.timeout_seconds,
                )
                if resp.status_code < 500:
                    return resp
                logger.warning(
                    "Memory Bank 5xx error (status=%d, attempt=%d/%d): %s",
                    resp.status_code,
                    attempt,
                    self.max_retries,
                    resp.text,
                )
            except (requests.Timeout, requests.ConnectionError) as exc:
                last_err = exc
                logger.warning(
                    "Memory Bank network error (attempt=%d/%d): %s",
                    attempt,
                    self.max_retries,
                    exc,
                )

            # Exponential backoff with jitter
            backoff = (0.5 * (2 ** (attempt - 1))) + random.uniform(0.0, 0.2)
            time.sleep(backoff)

        if last_err:
            raise last_err
        raise RuntimeError(f"Memory Bank request failed after {self.max_retries} attempts")

    def ingest_event(self, event: MemoryEventEnvelope) -> IngestionResult:
        """Ingests event into Google Agent Platform Memory Bank."""
        headers = {
            "X-Sentinel-Tenant-Scope": event.tenant_scope_token,
            "X-Sentinel-Event-Hash": event.event_hash,
        }
        payload = {
            "eventId": event.event_id,
            "tenantScope": event.tenant_scope_token,
            "topic": event.memory_topic,
            "subject": event.subject_ref,
            "fact": event.sanitized_fact,
            "sourceReferences": event.source_refs,
            "occurredAt": event.occurred_at,
            "metadata": event.metadata,
            "eventHash": event.event_hash,
        }
        try:
            resp = self._execute_with_retry("POST", "events:ingest", headers, payload)
            if resp.status_code in (200, 201):
                return IngestionResult(
                    success=True,
                    event_id=event.event_id,
                    event_hash=event.event_hash,
                    tenant_scope_token=event.tenant_scope_token,
                )
            return IngestionResult(
                success=False,
                event_id=event.event_id,
                event_hash=event.event_hash,
                tenant_scope_token=event.tenant_scope_token,
                error_code=f"HTTP_{resp.status_code}",
                error_message=resp.text,
            )
        except Exception as e:
            logger.error("Ingestion failed for event %s: %s", event.event_id, e)
            return IngestionResult(
                success=False,
                event_id=event.event_id,
                event_hash=event.event_hash,
                tenant_scope_token=event.tenant_scope_token,
                error_code="INGESTION_ERROR",
                error_message=str(e),
            )

    def retrieve_memories(self, query: MemoryQuery) -> List[MemoryHit]:
        """Retrieves and ranks memories from Google Memory Bank."""
        headers = {
            "X-Sentinel-Tenant-Scope": query.tenant_scope_token,
            "X-Sentinel-Correlation-ID": query.correlation_id,
        }
        payload = {
            "tenantScope": query.tenant_scope_token,
            "topic": query.memory_topic,
            "subject": query.subject_ref,
            "queryText": query.query_text,
            "maxResults": query.limit,
            "lookbackDays": query.lookback_days,
        }
        try:
            resp = self._execute_with_retry("POST", "memories:search", headers, payload)
            if resp.status_code != 200:
                logger.error("Memory search returned status %d: %s", resp.status_code, resp.text)
                return []

            data = resp.json()
            raw_memories = data.get("memories", [])
            candidate_events = []
            for item in raw_memories:
                evt = MemoryEventEnvelope(
                    event_id=item.get("eventId", ""),
                    tenant_scope_token=query.tenant_scope_token,
                    memory_topic=item.get("topic", "INCIDENT_PATTERN"),
                    subject_ref=item.get("subject", ""),
                    sanitized_fact=item.get("fact", ""),
                    source_refs=item.get("sourceReferences", []),
                    occurred_at=item.get("occurredAt", ""),
                    metadata=item.get("metadata", {}),
                    event_hash=item.get("eventHash", ""),
                )
                candidate_events.append(evt)

            return DeterministicMemoryRanker.rank_events(candidate_events, query)
        except Exception as e:
            logger.error("Memory retrieval error for tenant %s: %s", query.tenant_scope_token, e)
            return []

    def get_profile(
        self,
        partner_ref: str,
        tenant_scope_token: str,
    ) -> Optional[PartnerOperationalProfile]:
        """Fetches partner operational profile from Memory Bank."""
        headers = {
            "X-Sentinel-Tenant-Scope": tenant_scope_token,
        }
        try:
            resp = self._execute_with_retry("GET", f"profiles/{partner_ref}", headers)
            if resp.status_code == 200:
                data = resp.json()
                return PartnerOperationalProfile.model_validate(data)
            return None
        except Exception as e:
            logger.warning("Partner profile lookup failed for %s: %s", partner_ref, e)
            return None

    def health_check(self) -> MemoryProviderHealth:
        """Checks API reachability."""
        start = time.time()
        try:
            resp = self._execute_with_retry("GET", "healthz", {})
            latency = (time.time() - start) * 1000.0
            status = "HEALTHY" if resp.status_code == 200 else "DEGRADED"
            return MemoryProviderHealth(
                status=status,
                provider_name="google_agent_platform_memory_bank",
                latency_ms=latency,
                tenant_isolated=True,
            )
        except Exception as e:
            latency = (time.time() - start) * 1000.0
            return MemoryProviderHealth(
                status="UNHEALTHY",
                provider_name="google_agent_platform_memory_bank",
                latency_ms=latency,
                tenant_isolated=True,
                details={"error": str(e)},
            )
