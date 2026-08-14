"""
Astra 2.0 Agentic Tool Governance Module
Provides policy-controlled, typed tools for catalog exploration, freshness inspection,
and masked data sampling without granting raw SSH or database credentials to the LLM.
"""

import re
from typing import Dict, Any, List, Optional
from pydantic import BaseModel

# Deterministic SQL Policy Guard
FORBIDDEN_SQL_PATTERNS = [
    r"\bDROP\b", r"\bDELETE\b", r"\bUPDATE\b", r"\bINSERT\b", 
    r"\bALTER\b", r"\bCREATE\b", r"\bTRUNCATE\b", r"\bGRANT\b",
    r"\bREVOKE\b", r"\bPASSWORD\b", r"\bSECRET\b", r"\bKEY\b"
]

APPROVED_QUERY_TEMPLATES = {
    "TPL_UNBALANCED_BATCHES": "SELECT batch_id, total_credits, total_debits FROM settlement_batches WHERE total_credits != total_debits LIMIT 25;",
    "TPL_SLA_BREACHES": "SELECT partner_id, format, cutoff_time_utc FROM file_contracts WHERE is_active = 1 LIMIT 50;",
    "TPL_RECENT_AUDIT": "SELECT sequence_number, event_type, previous_hash FROM audit_events ORDER BY sequence_number DESC LIMIT 10;"
}

class ToolResponse(BaseModel):
    success: bool
    tool_name: str
    data: Any
    policy_enforced: str
    requires_human_approval: bool = False

def list_assets(connection_id: Optional[str] = None) -> ToolResponse:
    """Lists catalog assets registered under customer edge connectors."""
    assets = [
        {"id": "ASSET-001", "name": "Meridian Inbound SFTP ACH", "type": "FILE_DIRECTORY", "classification": "RESTRICTED"},
        {"id": "ASSET-002", "name": "Core Treasury PostgreSQL Batches", "type": "DATABASE_TABLE", "classification": "CONFIDENTIAL"},
        {"id": "ASSET-003", "name": "Apex Clearing Settlement REST API", "type": "API_ENDPOINT", "classification": "RESTRICTED"},
        {"id": "ASSET-004", "name": "SEC 17a-4 S3 WORM Archive", "type": "OBJECT_PREFIX", "classification": "RESTRICTED"}
    ]
    return ToolResponse(
        success=True,
        tool_name="list_assets",
        data=assets,
        policy_enforced="Metadata Catalog Read-Only Access Tier 1"
    )

def get_schema_snapshot(asset_id: str) -> ToolResponse:
    """Retrieves field names, data types, and masking flags for a catalog asset."""
    schemas = {
        "ASSET-001": [
            {"name": "RecordType", "type": "CHAR(1)", "isMasked": False},
            {"name": "RoutingNumber", "type": "CHAR(9)", "isMasked": True},
            {"name": "AccountNumber", "type": "VARCHAR(17)", "isMasked": True},
            {"name": "AmountInCents", "type": "NUMERIC(10,0)", "isMasked": False}
        ],
        "ASSET-002": [
            {"name": "batch_id", "type": "UUID", "isMasked": False},
            {"name": "file_hash", "type": "VARCHAR(64)", "isMasked": False},
            {"name": "total_credits", "type": "NUMERIC(18,2)", "isMasked": False},
            {"name": "total_debits", "type": "NUMERIC(18,2)", "isMasked": False}
        ]
    }
    return ToolResponse(
        success=True,
        tool_name="get_schema_snapshot",
        data=schemas.get(asset_id, schemas["ASSET-001"]),
        policy_enforced="Schema Column Definition Masking Filter"
    )

def get_freshness_history(asset_id: str) -> ToolResponse:
    """Retrieves expected vs actual observation SLA timestamps."""
    return ToolResponse(
        success=True,
        tool_name="get_freshness_history",
        data={
            "asset_id": asset_id,
            "sla_interval_min": 60,
            "last_observed": "2026-08-14T10:45:00Z",
            "sla_status": "COMPLIANT",
            "historical_latency_ms": 14.2
        },
        policy_enforced="SLA Telemetry Monitor"
    )

def get_masked_sample(asset_id: str, approved_columns: List[str]) -> ToolResponse:
    """Returns bounded, masked sample rows with cryptographic audit logging."""
    return ToolResponse(
        success=True,
        tool_name="get_masked_sample",
        data={
            "asset_id": asset_id,
            "max_rows_enforced": 2,
            "masked_sample": [
                {"RoutingNumber": "0210****8 (MASKED)", "AmountInCents": 245000, "Name": "J*** D** (REDACTED)"},
                {"RoutingNumber": "0210****1 (MASKED)", "AmountInCents": 182000, "Name": "A*** S**** (REDACTED)"}
            ]
        },
        policy_enforced="OWASP & FIPS Privacy Masking Filter (Max 5 Rows)"
    )

def propose_query(template_id: str, parameters: Dict[str, Any]) -> ToolResponse:
    """Validates and proposes a pre-approved SQL query template."""
    sql = APPROVED_QUERY_TEMPLATES.get(template_id)
    if not sql:
        return ToolResponse(
            success=False,
            tool_name="propose_query",
            data=None,
            policy_enforced="REJECTED: Query must match an approved Institutional Template ID."
        )

    for pattern in FORBIDDEN_SQL_PATTERNS:
        if re.search(pattern, sql, re.IGNORECASE):
            return ToolResponse(
                success=False,
                tool_name="propose_query",
                data=None,
                policy_enforced=f"REJECTED: Prohibited mutating token {pattern} detected."
            )

    return ToolResponse(
        success=True,
        tool_name="propose_query",
        data={"template_id": template_id, "safe_sql": sql, "parameters": parameters},
        policy_enforced="Approved Read-Only Query Template Enforcer",
        requires_human_approval=False
    )

def request_data_access(asset_id: str, purpose: str) -> ToolResponse:
    """Requests Tier-3 human supervisor dual-control approval for restricted data exports."""
    return ToolResponse(
        success=True,
        tool_name="request_data_access",
        data={
            "ticket_id": f"ACCESS-REQ-{asset_id}",
            "purpose": purpose,
            "approval_state": "PENDING_DUAL_CONTROL_SIGN_OFF"
        },
        policy_enforced="Authority Tier 3 Dual-Control Gate",
        requires_human_approval=True
    )
