"""Non-Authoritative Ephemeral ADK Session & Test Store for SentinelFlow (SGACA Phase P06.6).

CRITICAL ARCHITECTURAL INVARIANT:
This Python session store is NON-AUTHORITATIVE and strictly for ephemeral ADK session
caching and local unit test simulation.
Authoritative workflow ownership, trigger idempotency, row_version optimistic locking,
authoritative event journals, TOCTOU decisions, and state transitions reside SOLELY in the Go Control Plane
(gateway/migrations/016_agent_workflow_state.sql, gateway/migrations/019_agent_workflow_trigger_idempotency.sql,
gateway/agent_workflow_service.go, gateway/agent_orchestrator.go).

Deleting or losing this entire Python store has ZERO effect on the consistency or integrity
of SentinelFlow's persisted financial, audit, or workflow records.
"""

from __future__ import annotations

import json
import logging
import os
import sqlite3
import time
from typing import Any, Dict, List, Optional, Tuple

logger = logging.getLogger("sentinel.ai.non_authoritative_session")

DEFAULT_DB_PATH = os.getenv("SENTINEL_SESSION_DB_PATH", os.path.join(os.path.dirname(__file__), "sessions.db"))


class NonAuthoritativeSessionStore:
    """Non-authoritative ephemeral session and result cache for ADK testing."""

    def __init__(self, db_path: Optional[str] = None):
        self.db_path = db_path or DEFAULT_DB_PATH
        os.makedirs(os.path.dirname(os.path.abspath(self.db_path)), exist_ok=True)
        self._init_db()

    def _get_connection(self) -> sqlite3.Connection:
        conn = sqlite3.connect(self.db_path, timeout=30.0)
        conn.row_factory = sqlite3.Row
        conn.execute("PRAGMA journal_mode=WAL;")
        conn.execute("PRAGMA foreign_keys=ON;")
        return conn

    def _init_db(self) -> None:
        with self._get_connection() as conn:
            conn.executescript("""
            CREATE TABLE IF NOT EXISTS non_authoritative_sessions (
                id              TEXT PRIMARY KEY,
                tenant_id       TEXT NOT NULL,
                incident_id     INTEGER NOT NULL,
                artifact_id     INTEGER NOT NULL,
                artifact_sha256 TEXT NOT NULL,
                state           TEXT NOT NULL,
                agent_name      TEXT NOT NULL,
                agent_version   TEXT NOT NULL,
                workflow_type   TEXT NOT NULL DEFAULT 'TRIAGE_AND_REMEDIATION',
                correlation_id  TEXT NOT NULL,
                trace_id        TEXT,
                row_version     INTEGER NOT NULL DEFAULT 1,
                error_detail    TEXT,
                policy_bundle_hash TEXT,
                plan_json       TEXT,
                synthesis_json  TEXT,
                created_at      TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
                updated_at      TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
                started_at      TIMESTAMP,
                completed_at    TIMESTAMP
            );

            CREATE TABLE IF NOT EXISTS non_authoritative_events (
                id              TEXT PRIMARY KEY,
                workflow_id     TEXT NOT NULL,
                tenant_id       TEXT NOT NULL,
                idempotency_key TEXT NOT NULL,
                event_type      TEXT NOT NULL,
                state_from      TEXT NOT NULL,
                state_to        TEXT NOT NULL,
                row_version     INTEGER NOT NULL,
                payload         TEXT NOT NULL,
                created_at      TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP
            );

            CREATE TABLE IF NOT EXISTS non_authoritative_specialist_results (
                id                          TEXT PRIMARY KEY,
                workflow_id                 TEXT NOT NULL,
                tenant_id                   TEXT NOT NULL,
                agent_name                  TEXT NOT NULL,
                agent_version               TEXT NOT NULL,
                manifest_hash               TEXT NOT NULL,
                input_context_hash          TEXT NOT NULL,
                artifact_sha256             TEXT NOT NULL,
                policy_bundle_hash          TEXT NOT NULL,
                authorized_evidence_set_hash TEXT NOT NULL,
                tool_manifest_hash          TEXT NOT NULL,
                status                      TEXT NOT NULL,
                output_json                 TEXT NOT NULL,
                evidence_refs_json          TEXT NOT NULL,
                latency_ms                  REAL NOT NULL DEFAULT 0.0,
                created_at                  TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP
            );
            """)

    def get_or_create_workflow(
        self,
        tenant_id: str,
        incident_id: int,
        artifact_id: int,
        artifact_sha256: str,
        correlation_id: str,
        workflow_type: str = "ARTIFACT_QUARANTINED",
        policy_bundle_hash: Optional[str] = None,
        trace_id: Optional[str] = None,
    ) -> Tuple[Dict[str, Any], bool]:
        """Local non-authoritative session lookup or creation."""
        with self._get_connection() as conn:
            cur = conn.cursor()
            cur.execute(
                """
                SELECT * FROM non_authoritative_sessions
                WHERE tenant_id = ? AND incident_id = ? AND workflow_type = ?
                """,
                (tenant_id, incident_id, workflow_type),
            )
            row = cur.fetchone()
            if row:
                return dict(row), False

            wf_id = f"session-{tenant_id}-{incident_id}-{workflow_type.lower()}"
            cur.execute(
                """
                INSERT INTO non_authoritative_sessions (
                    id, tenant_id, incident_id, artifact_id, artifact_sha256,
                    state, agent_name, agent_version, workflow_type,
                    correlation_id, trace_id, row_version, policy_bundle_hash
                ) VALUES (?, ?, ?, ?, ?, 'PENDING', 'IncidentCommanderAgent', '1.0.0', ?, ?, ?, 1, ?)
                """,
                (
                    wf_id,
                    tenant_id,
                    incident_id,
                    artifact_id,
                    artifact_sha256,
                    workflow_type,
                    correlation_id,
                    trace_id,
                    policy_bundle_hash,
                ),
            )
            conn.commit()

            cur.execute("SELECT * FROM non_authoritative_sessions WHERE id = ?", (wf_id,))
            created_row = cur.fetchone()
            return dict(created_row), True

    def transition_state(
        self,
        workflow_id: str,
        tenant_id: str,
        new_state: str,
        error_detail: Optional[str] = None,
        plan_json: Optional[str] = None,
        synthesis_json: Optional[str] = None,
    ) -> bool:
        """Local non-authoritative session transition."""
        with self._get_connection() as conn:
            cur = conn.cursor()
            cur.execute(
                """
                UPDATE non_authoritative_sessions
                SET state = ?,
                    row_version = row_version + 1,
                    error_detail = COALESCE(?, error_detail),
                    plan_json = COALESCE(?, plan_json),
                    synthesis_json = COALESCE(?, synthesis_json),
                    updated_at = CURRENT_TIMESTAMP
                WHERE id = ? AND tenant_id = ?
                """,
                (new_state, error_detail, plan_json, synthesis_json, workflow_id, tenant_id),
            )
            conn.commit()
            return cur.rowcount > 0

    def record_event(
        self,
        workflow_id: str,
        tenant_id: str,
        event_type: str,
        state_from: str,
        state_to: str,
        idempotency_key: str,
        payload: Dict[str, Any],
        row_version: int = 1,
    ) -> bool:
        """Local non-authoritative session telemetry event."""
        event_id = f"sev-{workflow_id}-{idempotency_key}"
        with self._get_connection() as conn:
            cur = conn.cursor()
            try:
                cur.execute(
                    """
                    INSERT INTO non_authoritative_events (
                        id, workflow_id, tenant_id, idempotency_key, event_type,
                        state_from, state_to, row_version, payload
                    ) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)
                    """,
                    (
                        event_id,
                        workflow_id,
                        tenant_id,
                        idempotency_key,
                        event_type,
                        state_from,
                        state_to,
                        row_version,
                        json.dumps(payload),
                    ),
                )
                conn.commit()
                return True
            except sqlite3.IntegrityError:
                return False

    def save_specialist_result(
        self,
        workflow_id: str,
        tenant_id: str,
        agent_name: str,
        agent_version: str,
        manifest_hash: str,
        input_context_hash: str,
        artifact_sha256: str,
        policy_bundle_hash: str,
        authorized_evidence_set_hash: str,
        tool_manifest_hash: str,
        status: str,
        output_json: str,
        evidence_refs: List[str],
        latency_ms: float = 0.0,
    ) -> str:
        """Local non-authoritative specialist cache."""
        result_id = f"sres-{workflow_id}-{agent_name}-{input_context_hash[:8]}"
        with self._get_connection() as conn:
            cur = conn.cursor()
            cur.execute(
                """
                INSERT OR REPLACE INTO non_authoritative_specialist_results (
                    id, workflow_id, tenant_id, agent_name, agent_version,
                    manifest_hash, input_context_hash, artifact_sha256,
                    policy_bundle_hash, authorized_evidence_set_hash, tool_manifest_hash,
                    status, output_json, evidence_refs_json, latency_ms
                ) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
                """,
                (
                    result_id,
                    workflow_id,
                    tenant_id,
                    agent_name,
                    agent_version,
                    manifest_hash,
                    input_context_hash,
                    artifact_sha256,
                    policy_bundle_hash,
                    authorized_evidence_set_hash,
                    tool_manifest_hash,
                    status,
                    output_json,
                    json.dumps(evidence_refs),
                    latency_ms,
                ),
            )
            conn.commit()
            return result_id

    def get_specialist_result_if_valid(
        self,
        workflow_id: str,
        agent_name: str,
        manifest_hash: str,
        input_context_hash: str,
        artifact_sha256: str,
        policy_bundle_hash: str,
        authorized_evidence_set_hash: str,
        tool_manifest_hash: str,
    ) -> Optional[Dict[str, Any]]:
        """Local non-authoritative specialist cache lookup."""
        with self._get_connection() as conn:
            cur = conn.cursor()
            cur.execute(
                """
                SELECT * FROM non_authoritative_specialist_results
                WHERE workflow_id = ?
                  AND agent_name = ?
                  AND manifest_hash = ?
                  AND input_context_hash = ?
                  AND artifact_sha256 = ?
                  AND policy_bundle_hash = ?
                  AND authorized_evidence_set_hash = ?
                  AND tool_manifest_hash = ?
                  AND status = 'SUCCESS'
                ORDER BY created_at DESC
                LIMIT 1
                """,
                (
                    workflow_id,
                    agent_name,
                    manifest_hash,
                    input_context_hash,
                    artifact_sha256,
                    policy_bundle_hash,
                    authorized_evidence_set_hash,
                    tool_manifest_hash,
                ),
            )
            row = cur.fetchone()
            if row:
                return dict(row)
            return None


# Backward-compatible alias for existing unit tests
DurableWorkflowStore = NonAuthoritativeSessionStore
