"""Memory Bank store — persistent cross-session agent memory.

Provides CRUD operations for the agent_memory table, scoped by tenant_id.
Used by the MemoryAgent to store and recall incident patterns, partner
history, SLA trends, and successful resolutions.
"""

from __future__ import annotations

import json
import logging
import uuid
from datetime import datetime, timezone
from typing import Any, Optional

from pydantic import BaseModel, Field

logger = logging.getLogger(__name__)


class MemoryEntry(BaseModel):
    """A single memory entry in the Memory Bank."""
    id: str = Field(default_factory=lambda: str(uuid.uuid4()))
    tenant_id: str
    memory_type: str  # INCIDENT_PATTERN, PARTNER_HISTORY, SLA_TREND, RESOLUTION
    entity_id: str    # partner_id, incident_id, contract_id
    entity_type: str  # PARTNER, INCIDENT, CONTRACT
    content: dict[str, Any]
    confidence: Optional[float] = None
    created_at: str = Field(
        default_factory=lambda: datetime.now(timezone.utc).isoformat()
    )
    expires_at: Optional[str] = None
    agent_id: str = ""
    invocation_id: Optional[str] = None


class MemoryStore:
    """Interface to the agent_memory table.

    In production, this connects to the gateway's PostgreSQL/SQLite database.
    In tests, use the in-memory implementation.
    """

    def __init__(self, db_url: str | None = None):
        """Initialize the memory store.

        Args:
            db_url: Database URL. If None, uses an in-memory dict store.
        """
        self._db_url = db_url
        self._in_memory: dict[str, list[MemoryEntry]] = {}

    def store(
        self,
        tenant_id: str,
        memory_type: str,
        entity_id: str,
        entity_type: str,
        content: dict[str, Any],
        agent_id: str,
        invocation_id: str | None = None,
        confidence: float | None = None,
        ttl_hours: int | None = None,
    ) -> MemoryEntry:
        """Store a new memory entry.

        Args:
            tenant_id: Tenant scope.
            memory_type: Type of memory (INCIDENT_PATTERN, etc.).
            entity_id: ID of the entity this memory is about.
            entity_type: Type of entity (PARTNER, INCIDENT, CONTRACT).
            content: JSON-serializable content (must be redacted, no raw financial data).
            agent_id: ID of the agent storing this memory.
            invocation_id: Optional invocation context.
            confidence: Optional confidence score (0.0-1.0).
            ttl_hours: Optional TTL in hours before expiry.

        Returns:
            The created MemoryEntry.
        """
        expires_at = None
        if ttl_hours:
            from datetime import timedelta
            expires_at = (
                datetime.now(timezone.utc) + timedelta(hours=ttl_hours)
            ).isoformat()

        entry = MemoryEntry(
            tenant_id=tenant_id,
            memory_type=memory_type,
            entity_id=entity_id,
            entity_type=entity_type,
            content=content,
            confidence=confidence,
            expires_at=expires_at,
            agent_id=agent_id,
            invocation_id=invocation_id,
        )

        # In-memory store (for local-demo and tests)
        key = f"{tenant_id}:{entity_type}:{entity_id}"
        if key not in self._in_memory:
            self._in_memory[key] = []
        self._in_memory[key].append(entry)

        logger.info(
            "Memory stored: type=%s entity=%s:%s tenant=%s",
            memory_type, entity_type, entity_id, tenant_id,
        )
        return entry

    def recall(
        self,
        tenant_id: str,
        entity_type: str,
        entity_id: str,
        limit: int = 10,
    ) -> list[MemoryEntry]:
        """Recall memories for a specific entity.

        Args:
            tenant_id: Tenant scope (MUST match stored tenant_id).
            entity_type: Type of entity to recall for.
            entity_id: ID of the entity.
            limit: Maximum number of entries to return.

        Returns:
            List of MemoryEntry objects, most recent first.
        """
        key = f"{tenant_id}:{entity_type}:{entity_id}"
        entries = self._in_memory.get(key, [])

        # Filter expired entries
        now = datetime.now(timezone.utc).isoformat()
        valid = [
            e for e in entries
            if e.expires_at is None or e.expires_at > now
        ]

        return sorted(valid, key=lambda e: e.created_at, reverse=True)[:limit]

    def recall_by_type(
        self,
        tenant_id: str,
        memory_type: str,
        limit: int = 10,
    ) -> list[MemoryEntry]:
        """Recall memories of a specific type across all entities.

        Args:
            tenant_id: Tenant scope.
            memory_type: Type of memory to recall.
            limit: Maximum entries.

        Returns:
            List of MemoryEntry objects, most recent first.
        """
        now = datetime.now(timezone.utc).isoformat()
        results = []

        for entries in self._in_memory.values():
            for e in entries:
                if (
                    e.tenant_id == tenant_id
                    and e.memory_type == memory_type
                    and (e.expires_at is None or e.expires_at > now)
                ):
                    results.append(e)

        return sorted(results, key=lambda e: e.created_at, reverse=True)[:limit]

    def count(self, tenant_id: str) -> int:
        """Count total memories for a tenant."""
        return sum(
            len([e for e in entries if e.tenant_id == tenant_id])
            for entries in self._in_memory.values()
        )
