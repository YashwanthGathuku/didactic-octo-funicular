"""Tool definitions with least-privilege scopes for the agent fleet.

Each tool explicitly declares:
- scope: READ or WRITE
- resources: Which data resources it accesses
- description: What the tool does

Agents may only use tools within their declared scope.
"""

from __future__ import annotations
from typing import Any


# --- Read-Only Tools (available to all agents) ---


def lookup_finding(finding_id: str) -> dict[str, Any]:
    """Look up a specific validation finding by ID.

    Scope: READ
    Resources: validation_findings
    """
    # Implementation will be connected to gateway API
    return {"finding_id": finding_id, "status": "stub"}


def lookup_nacha_rule(rule_code: str) -> dict[str, Any]:
    """Look up a NACHA rule by its code.

    Scope: READ
    Resources: nacha_rules (in-memory reference data)
    """
    return {"rule_code": rule_code, "status": "stub"}


def check_sla_status(contract_id: str) -> dict[str, Any]:
    """Check the current SLA status for a contract.

    Scope: READ
    Resources: expectations, file_contracts
    """
    return {"contract_id": contract_id, "status": "stub"}


def recall_partner_history(partner_name: str, tenant_id: str) -> dict[str, Any]:
    """Recall historical incident and delivery data for a partner.

    Scope: READ
    Resources: agent_memory, incidents, expectations
    """
    return {"partner_name": partner_name, "tenant_id": tenant_id, "status": "stub"}


def recall_similar_incidents(finding_codes: list[str], tenant_id: str) -> dict[str, Any]:
    """Find past incidents with similar finding codes.

    Scope: READ
    Resources: agent_memory, incidents, validation_findings
    """
    return {"finding_codes": finding_codes, "tenant_id": tenant_id, "status": "stub"}


# --- Write Tools (restricted to specific agents) ---


def store_memory(tenant_id: str, memory_type: str, entity_id: str, content: str) -> dict[str, Any]:
    """Store a new memory entry in the Memory Bank.

    Scope: WRITE
    Resources: agent_memory
    Restricted to: MemoryAgent only
    """
    return {"tenant_id": tenant_id, "memory_type": memory_type, "status": "stub"}


def propose_derived_artifact(
    original_id: str, correction_spec: dict, reason: str
) -> dict[str, Any]:
    """Propose a derived artifact from a quarantined original.

    Scope: WRITE
    Resources: file_instances (derived_from linkage only)
    Restricted to: RemediationAgent only
    """
    return {"original_id": original_id, "reason": reason, "status": "stub"}


# --- Tool Scope Registry ---
# Maps agent names to their permitted tools for enforcement.

AGENT_TOOL_SCOPES: dict[str, list[str]] = {
    "TriageAgent": ["lookup_finding", "check_sla_status"],
    "ComplianceAgent": ["lookup_finding", "lookup_nacha_rule"],
    "RemediationAgent": ["lookup_finding", "lookup_nacha_rule", "propose_derived_artifact"],
    "VerifierAgent": ["lookup_finding"],
    "MemoryAgent": ["recall_partner_history", "recall_similar_incidents", "store_memory"],
    "EscalationAgent": ["check_sla_status", "recall_partner_history"],
}
