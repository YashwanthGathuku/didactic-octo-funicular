"""Abstract Base Class for SentinelFlow P10 Managed Memory Providers."""

from __future__ import annotations

import abc
from dataclasses import dataclass
from typing import Any, Dict, List, Optional
from contracts.memory import (
    MemoryEventEnvelope,
    MemoryHit,
    MemoryQuery,
    PartnerOperationalProfile,
)


@dataclass
class IngestionResult:
    """Result returned upon memory event ingestion."""
    success: bool
    event_id: str
    event_hash: str
    tenant_scope_token: str
    error_code: Optional[str] = None
    error_message: Optional[str] = None


@dataclass
class MemoryProviderHealth:
    """Health check status for the memory provider."""
    status: str  # HEALTHY | DEGRADED | UNHEALTHY
    provider_name: str
    latency_ms: float
    tenant_isolated: bool = True
    details: Optional[Dict[str, Any]] = None


class ManagedMemoryProvider(abc.ABC):
    """Abstract interface for SentinelFlow Managed Memory Bank providers."""

    @abc.abstractmethod
    def ingest_event(self, event: MemoryEventEnvelope) -> IngestionResult:
        """Ingests a data-minimized, canonical memory event.

        Args:
            event: Validated MemoryEventEnvelope with cryptographic event_hash.

        Returns:
            IngestionResult with success status and stored hash.
        """
        raise NotImplementedError

    @abc.abstractmethod
    def retrieve_memories(self, query: MemoryQuery) -> List[MemoryHit]:
        """Retrieves and ranks memories matching a bounded query.

        Args:
            query: MemoryQuery with tenant_scope_token and limit <= 5.

        Returns:
            Sorted list of MemoryHit objects adhering to bounded limits.
        """
        raise NotImplementedError

    @abc.abstractmethod
    def get_profile(
        self,
        partner_ref: str,
        tenant_scope_token: str,
    ) -> Optional[PartnerOperationalProfile]:
        """Retrieves the aggregated operational profile for a partner.

        Args:
            partner_ref: Target counterparty identifier.
            tenant_scope_token: Authenticated tenant scope token.

        Returns:
            PartnerOperationalProfile if available, else None.
        """
        raise NotImplementedError

    @abc.abstractmethod
    def health_check(self) -> MemoryProviderHealth:
        """Performs an active liveness and connectivity health probe."""
        raise NotImplementedError
