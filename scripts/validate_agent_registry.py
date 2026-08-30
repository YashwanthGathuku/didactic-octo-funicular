#!/usr/bin/env python3
"""Validate SentinelFlow Agent Registry against Python Manifests and Query Fleet Catalog.

Enforces least-privilege capability mapping, mandatory global denials, semver compliance,
valid department assignment, and provides cross-department discovery CLI for the 7 specialist agents.
"""

from __future__ import annotations

import argparse
import json
from pathlib import Path
import re
import sys
from typing import Any, Dict, List, Optional, Tuple

# Set up repository paths
REPO_ROOT = Path(__file__).resolve().parent.parent
AI_TIER_PATH = REPO_ROOT / "ai-tier"
if str(AI_TIER_PATH) not in sys.path:
    sys.path.insert(0, str(AI_TIER_PATH))

try:
    from contracts.manifests import FIXED_AGENT_ROSTER, AgentManifest
except ImportError as e:
    print(f"Error importing AI-tier contracts: {e}", file=sys.stderr)
    sys.exit(1)

DEFAULT_REGISTRY_PATH = REPO_ROOT / "docs" / "registry" / "agent_registry_v1.json"

SEMVER_REGEX = re.compile(
    r"^(0|[1-9]\d*)\.(0|[1-9]\d*)\.(0|[1-9]\d*)"
    r"(?:-((?:0|[1-9]\d*|\d*[a-zA-Z-][0-9a-zA-Z-]*)(?:\.(?:0|[1-9]\d*|\d*[a-zA-Z-][0-9a-zA-Z-]*))*))?"
    r"(?:\+([0-9a-zA-Z-]+(?:\.[0-9a-zA-Z-]+)*))?$"
)

ALLOWED_DEPARTMENTS = {
    "Treasury Operations",
    "Compliance & Risk",
    "Payments Engineering",
}

GLOBAL_MANDATORY_DENIALS = [
    "artifact.release",
    "incident.approve",
    "ledger.mutate",
    "database.raw_sql",
    "system.shell",
    "agent.create_dynamic",
    "artifact.write_direct",
]

AUTONOMY_LEVELS = {
    "A1": 1,
    "A2": 2,
    "A3": 3,
    "A4": 4,
    "A5": 5,
}

REQUIRED_AGENT_FIELDS = [
    "agentId",
    "displayName",
    "version",
    "framework",
    "modelFamily",
    "environment",
    "department",
    "autonomyCeiling",
    "dataClassification",
    "runtimeEndpoint",
    "registeredCapabilities",
    "deniedCapabilities",
    "nonAuthorityStatement",
]


def load_registry(registry_path: Path) -> Dict[str, Any]:
    """Loads and parses the agent registry JSON file."""
    if not registry_path.exists():
        raise FileNotFoundError(f"Registry file not found: {registry_path}")
    with open(registry_path, "r", encoding="utf-8") as f:
        return json.load(f)


def validate_registry(
    registry_data: Dict[str, Any],
    manifest_roster: Dict[str, AgentManifest] = FIXED_AGENT_ROSTER,
) -> Tuple[bool, List[str]]:
    """Validates the agent registry against Python manifests, least-privilege, and security invariants."""
    errors: List[str] = []

    if "agentRegistry" not in registry_data:
        return False, ["Missing top-level 'agentRegistry' object"]

    reg = registry_data["agentRegistry"]
    agents = reg.get("agents", [])
    if not isinstance(agents, list):
        return False, ["'agentRegistry.agents' must be a list"]

    if len(agents) == 0:
        return False, ["'agentRegistry.agents' is empty"]

    seen_agent_ids = set()

    for idx, agent in enumerate(agents):
        agent_id = agent.get("agentId")
        if not agent_id:
            errors.append(f"Agent #{idx + 1} missing required 'agentId'")
            continue

        # 1. Duplicate check
        if agent_id in seen_agent_ids:
            errors.append(f"Duplicate agentId '{agent_id}' detected at index #{idx + 1}")
        seen_agent_ids.add(agent_id)

        # 2. Required fields check
        for field in REQUIRED_AGENT_FIELDS:
            if field not in agent:
                errors.append(f"Agent '{agent_id}' missing required field: '{field}'")

        # 3. SemVer compliance check
        version = str(agent.get("version", ""))
        if not SEMVER_REGEX.match(version):
            errors.append(
                f"Agent '{agent_id}' has invalid semantic version string: '{version}'"
            )

        # 4. Department assignment check
        department = agent.get("department", "")
        if department not in ALLOWED_DEPARTMENTS:
            errors.append(
                f"Agent '{agent_id}' has invalid department '{department}'. Must be one of: {sorted(ALLOWED_DEPARTMENTS)}"
            )

        # 5. Autonomy ceiling check
        autonomy = agent.get("autonomyCeiling", "")
        if autonomy not in AUTONOMY_LEVELS:
            errors.append(
                f"Agent '{agent_id}' has invalid autonomyCeiling '{autonomy}'. Must be one of: {list(AUTONOMY_LEVELS.keys())}"
            )

        # 6. Manifest agreement & least privilege
        if agent_id not in manifest_roster:
            errors.append(
                f"Agent '{agent_id}' is not present in Python fixed manifest roster ({list(manifest_roster.keys())})"
            )
            continue

        manifest = manifest_roster[agent_id]
        manifest_allowed = set(manifest.allowed_tools)
        registered_caps = set(agent.get("registeredCapabilities", []))

        # Check for registry over-grant (capability in registry but not in manifest)
        over_granted = registered_caps - manifest_allowed
        if over_granted:
            errors.append(
                f"REGISTRY OVER-GRANT: Agent '{agent_id}' registered capabilities {sorted(over_granted)} "
                f"which are NOT permitted by Python manifest allowed_tools: {sorted(manifest_allowed)}"
            )

        # Check for missing capabilities (manifest allowed but registry omitted)
        under_granted = manifest_allowed - registered_caps
        if under_granted:
            errors.append(
                f"REGISTRY UNDER-GRANT: Agent '{agent_id}' missing manifest allowed_tools: {sorted(under_granted)}"
            )

        # 7. Mandatory global denials check
        denied_caps = set(agent.get("deniedCapabilities", []))
        for global_denial in GLOBAL_MANDATORY_DENIALS:
            if global_denial not in denied_caps:
                errors.append(
                    f"MISSING GLOBAL DENIAL: Agent '{agent_id}' missing mandatory global denial '{global_denial}'"
                )

        # Check any agent-specific denials from Python manifest
        manifest_denials = set(manifest.denied_capabilities)
        missing_manifest_denials = manifest_denials - denied_caps
        if missing_manifest_denials:
            errors.append(
                f"MISSING MANIFEST DENIAL: Agent '{agent_id}' missing manifest denial: {sorted(missing_manifest_denials)}"
            )

    # Check for missing roster agents in registry
    roster_agent_ids = set(manifest_roster.keys())
    missing_roster_agents = roster_agent_ids - seen_agent_ids
    if missing_roster_agents:
        errors.append(
            f"Missing required roster agents in registry: {sorted(missing_roster_agents)}"
        )

    is_valid = len(errors) == 0
    return is_valid, errors


def format_table(headers: List[str], rows: List[List[str]]) -> str:
    """Formats rows into an ASCII table."""
    widths = [len(h) for h in headers]
    for row in rows:
        for i, cell in enumerate(row):
            widths[i] = max(widths[i], len(str(cell)))

    col_fmt = " | ".join(f"{{:<{w}}}" for w in widths)
    sep_line = "-+-".join("-" * w for w in widths)

    lines = []
    lines.append(col_fmt.format(*headers))
    lines.append(sep_line)
    for row in rows:
        lines.append(col_fmt.format(*[str(c) for c in row]))
    return "\n".join(lines)


def query_registry(
    registry_data: Dict[str, Any],
    department: Optional[str] = None,
    max_autonomy: Optional[str] = None,
    denied: Optional[str] = None,
    capability: Optional[str] = None,
) -> List[Dict[str, Any]]:
    """Filters agents based on discovery criteria."""
    agents = registry_data.get("agentRegistry", {}).get("agents", [])
    results = []

    for agent in agents:
        # Department filter
        if department and agent.get("department", "").lower() != department.lower():
            continue

        # Max autonomy ceiling filter
        if max_autonomy:
            agent_ceiling = agent.get("autonomyCeiling", "A1")
            agent_level = AUTONOMY_LEVELS.get(agent_ceiling, 99)
            max_level = AUTONOMY_LEVELS.get(max_autonomy.upper(), -1)
            if agent_level > max_level:
                continue

        # Denied capability filter
        if denied:
            agent_denied = agent.get("deniedCapabilities", [])
            if denied not in agent_denied:
                continue

        # Registered capability filter
        if capability:
            agent_caps = agent.get("registeredCapabilities", [])
            if capability not in agent_caps:
                continue

        results.append(agent)

    return results


def main() -> None:
    parser = argparse.ArgumentParser(
        description="SentinelFlow Agent Registry Validator and Cross-Department Discovery Tool"
    )
    parser.add_argument(
        "--registry-file",
        type=Path,
        default=DEFAULT_REGISTRY_PATH,
        help="Path to agent registry JSON file",
    )
    parser.add_argument(
        "--check",
        action="store_true",
        help="Run strict least-privilege and manifest agreement validation",
    )
    parser.add_argument(
        "--department",
        type=str,
        help="Filter agents by department ('Treasury Operations', 'Compliance & Risk', 'Payments Engineering')",
    )
    parser.add_argument(
        "--max-autonomy",
        type=str,
        choices=["A1", "A2", "A3", "A4", "A5"],
        help="Filter agents with autonomy level up to specified ceiling",
    )
    parser.add_argument(
        "--denied",
        type=str,
        help="Filter agents that explicitly enforce a specific denied capability (e.g. 'ledger.mutate')",
    )
    parser.add_argument(
        "--capability",
        type=str,
        help="Filter agents that possess a specific registered capability (e.g. 'lens.query')",
    )
    parser.add_argument(
        "--json",
        action="store_true",
        help="Output query results in JSON format instead of table",
    )

    args = parser.parse_args()

    try:
        registry_data = load_registry(args.registry_file)
    except Exception as e:
        print(f"ERROR: Failed to load registry from {args.registry_file}: {e}", file=sys.stderr)
        sys.exit(1)

    is_query = any([args.department, args.max_autonomy, args.denied, args.capability])

    if args.check or not is_query:
        is_valid, errors = validate_registry(registry_data, FIXED_AGENT_ROSTER)
        if not is_valid:
            print("============================================================", file=sys.stderr)
            print("  AGENT REGISTRY VALIDATION FAILED", file=sys.stderr)
            print("============================================================", file=sys.stderr)
            for err in errors:
                print(f"  [ERROR] {err}", file=sys.stderr)
            sys.exit(1)

        if args.check and not is_query:
            print("============================================================")
            print("  SENTINELFLOW AGENT REGISTRY VALIDATION PASSED")
            print("============================================================")
            print(f"Registry File: {args.registry_file}")
            print(f"Verified Agents: {len(registry_data['agentRegistry']['agents'])}")
            print("Least-Privilege Manifest Alignment: 100% CONFIRMED")
            print(f"Mandatory Global Denials: {len(GLOBAL_MANDATORY_DENIALS)}/7 Enforced on All Agents")
            print("\nCatalog Summary:")
            
            headers = ["Agent ID", "Version", "Department", "Autonomy", "Capabilities", "Denials"]
            rows = []
            for agent in registry_data["agentRegistry"]["agents"]:
                rows.append([
                    agent["agentId"],
                    agent["version"],
                    agent["department"],
                    agent["autonomyCeiling"],
                    len(agent.get("registeredCapabilities", [])),
                    len(agent.get("deniedCapabilities", [])),
                ])
            print(format_table(headers, rows))
            return

    # Discovery / Query Mode
    matching_agents = query_registry(
        registry_data,
        department=args.department,
        max_autonomy=args.max_autonomy,
        denied=args.denied,
        capability=args.capability,
    )

    if args.json:
        print(json.dumps(matching_agents, indent=2))
        return

    print("==========================================================================================")
    print("  SENTINELFLOW CROSS-DEPARTMENT AGENT DISCOVERY CATALOG")
    print("==========================================================================================")
    filters_applied = []
    if args.department:
        filters_applied.append(f"Department='{args.department}'")
    if args.max_autonomy:
        filters_applied.append(f"MaxAutonomy='{args.max_autonomy}'")
    if args.denied:
        filters_applied.append(f"EnforcesDenial='{args.denied}'")
    if args.capability:
        filters_applied.append(f"HasCapability='{args.capability}'")
    
    if filters_applied:
        print("Filters: " + " | ".join(filters_applied))
    else:
        print("Filter: ALL REGISTERED AGENTS")
    print(f"Matches Found: {len(matching_agents)}\n")

    if matching_agents:
        headers = ["Agent ID", "Version", "Department", "Autonomy", "Registered Capabilities", "Key Denials"]
        rows = []
        for a in matching_agents:
            caps_str = ", ".join(a.get("registeredCapabilities", []))
            if len(caps_str) > 38:
                caps_str = caps_str[:35] + "..."
            denied_str = ", ".join(a.get("deniedCapabilities", [])[:3]) + f" (+{len(a.get('deniedCapabilities', []))-3} more)"
            rows.append([
                a["agentId"],
                a["version"],
                a["department"],
                a["autonomyCeiling"],
                caps_str,
                denied_str,
            ])
        print(format_table(headers, rows))
    else:
        print("No agents matched the specified discovery criteria.")


if __name__ == "__main__":
    main()
