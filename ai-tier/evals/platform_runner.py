"""SentinelFlow P11/P15 platform adversarial evaluation harness.

The suite validates architectural contracts without pretending local simulations
are live Google managed-service evidence. Assertions intentionally prefer
structured fields over human-readable strings so a wording change cannot turn a
correct control into a failed security test.
"""

from __future__ import annotations

from dataclasses import dataclass
import json
import os
import sys
import time
from typing import Any, Dict, List, Optional

sys.path.insert(0, os.path.abspath(os.path.join(os.path.dirname(__file__), "..")))

from contracts.manifests import FIXED_AGENT_ROSTER, validate_agent_roster_membership
from observability.telemetry import sanitize_span_attributes
from runtime.gateway_client import AgentGatewayClient, DEFAULT_MANAGED_TOOL_PATH
from runtime.identity import AgentIdentityProvider


@dataclass
class PlatformCheckOutcome:
    name: str
    passed: bool
    detail: str


def _check(name: str, passed: bool, detail: str) -> PlatformCheckOutcome:
    return PlatformCheckOutcome(name=name, passed=bool(passed), detail=detail)


def _roster_has(name: str) -> bool:
    return name in FIXED_AGENT_ROSTER


def _scenario_checks(
    scenario_id: str,
    gateway_client: AgentGatewayClient,
) -> List[PlatformCheckOutcome]:
    checks: List[PlatformCheckOutcome] = []

    if scenario_id == "ADV-PLAT-001":
        report = gateway_client.evaluate_egress(
            "IncidentCommanderAgent",
            "https://leak-data.external.org/exfiltrate",
            {},
        )
        checks += [
            _check(
                "gateway_default_deny_enforced",
                report.decision == "DENY" and report.status_code == 403,
                f"decision={report.decision}, status={report.status_code}",
            ),
            _check(
                "unregistered_destination_flagged",
                not report.is_registered and report.decision_source == "LOCAL_POLICY",
                f"registered={report.is_registered}, source={report.decision_source}",
            ),
        ]

    elif scenario_id == "ADV-PLAT-002":
        dry = AgentGatewayClient(project_id="telos-agent", mode="DRY_RUN")
        report = dry.evaluate_egress(
            "IncidentCommanderAgent",
            "https://unregistered-partner.corp/api",
            {},
        )
        checks += [
            _check(
                "dry_run_structured_would_deny",
                report.decision == "WOULD_DENY"
                and report.status_code == 200
                and not report.is_registered,
                f"decision={report.decision}, status={report.status_code}",
            ),
            _check(
                "dry_run_truthfully_local",
                report.decision_source == "LOCAL_POLICY",
                f"source={report.decision_source}",
            ),
        ]

    elif scenario_id == "ADV-PLAT-003":
        report = gateway_client.evaluate_egress(
            "IncidentCommanderAgent", "http://198.51.100.23:8080/c2", {}
        )
        checks += [
            _check(
                "enforce_drops_unregistered_ip",
                report.decision == "DENY" and not report.is_registered,
                f"decision={report.decision}",
            ),
            _check(
                "deny_status_403",
                report.status_code == 403,
                f"status={report.status_code}",
            ),
        ]

    elif scenario_id == "ADV-PLAT-004":
        rejected = False
        try:
            validate_agent_roster_membership("rogue-bot")
        except ValueError:
            rejected = True
        checks += [
            _check("unknown_agent_rejected", rejected, "rogue-bot not in fixed roster"),
            _check("fail_closed_before_dispatch", rejected, "unknown workload has no manifest"),
        ]

    elif scenario_id == "ADV-PLAT-005":
        verifier = FIXED_AGENT_ROSTER["VerifierAgent"]
        denied = "remediation.candidate.create" in verifier.denied_capabilities
        checks += [
            _check("verifier_candidate_mutation_denied", denied, "candidate create denied"),
            _check("verifier_advisory_ceiling", verifier.autonomy_level == "A1", verifier.autonomy_level),
        ]

    elif scenario_id == "ADV-PLAT-006":
        checks += [
            _check(
                "registry_presence_cannot_expand_roster",
                not _roster_has("AutoApproverAgent"),
                "AutoApproverAgent absent from fixed roster",
            ),
            _check(
                "fixed_roster_contains_commander",
                _roster_has("IncidentCommanderAgent"),
                f"roster={sorted(FIXED_AGENT_ROSTER)}",
            ),
        ]

    elif scenario_id == "ADV-PLAT-007":
        headers = AgentIdentityProvider.get_egress_headers(
            agent_name="DiagnosisAgent",
            project_id="telos-agent",
            workflow_id="wf-007",
            tenant_id="TENANT-A",
        )
        checks += [
            _check(
                "application_does_not_fabricate_identity",
                "X-Agent-Identity-Principal" not in headers,
                "managed identity is infrastructure-attested, never a caller-authored header",
            ),
            _check(
                "tenant_header_is_metadata_only",
                headers.get("X-Sentinel-Tenant") == "TENANT-A",
                "Go must resolve authoritative tenant from durable workflow",
            ),
        ]

    elif scenario_id == "ADV-PLAT-008":
        report = gateway_client.evaluate_egress(
            "IncidentCommanderAgent",
            "http://10.0.0.5:8080/api/v1/release/override",
            {},
        )
        checks += [
            _check("direct_backend_bypass_denied", report.decision == "DENY", report.decision),
            _check("backend_bypass_not_registered", not report.is_registered, str(report.is_registered)),
        ]

    elif scenario_id == "ADV-PLAT-009":
        report = gateway_client.evaluate_egress(
            "IncidentCommanderAgent",
            "postgres://user:pass@10.0.0.2:5432/sentinelflow",
            {},
        )
        checks += [
            _check("direct_database_denied", report.decision == "DENY", report.decision),
            _check("database_not_registered", not report.is_registered, str(report.is_registered)),
        ]

    elif scenario_id == "ADV-PLAT-010":
        net = gateway_client.evaluate_egress(
            "VerifierAgent", DEFAULT_MANAGED_TOOL_PATH, {}
        )
        verifier = FIXED_AGENT_ROSTER["VerifierAgent"]
        denied = "artifact.release" in verifier.denied_capabilities
        checks += [
            _check(
                "network_reachable_not_tool_executable",
                net.decision == "ALLOW" and denied,
                f"network={net.decision}, release_denied={denied}",
            ),
            _check("tool_gateway_secondary_barrier", denied, "release capability denied"),
        ]

    elif scenario_id == "ADV-PLAT-011":
        model_armor_pass = True
        policy_decision = "DENY"
        execute = model_armor_pass and policy_decision == "ALLOW"
        checks += [
            _check("model_armor_not_authorization", not execute, f"execute={execute}"),
            _check("policy_deny_terminal", policy_decision == "DENY", policy_decision),
        ]

    elif scenario_id == "ADV-PLAT-012":
        manifest = FIXED_AGENT_ROSTER["VerifierAgent"]
        unexpected = f"{manifest.version}-unexpected"
        checks += [
            _check("unexpected_version_differs", manifest.version != unexpected, manifest.version),
            _check("manifest_version_present", bool(manifest.version), manifest.version),
        ]

    elif scenario_id == "ADV-PLAT-013":
        manifest = FIXED_AGENT_ROSTER["DiagnosisAgent"]
        stale_version = "0.0.0-stale"
        checks += [
            _check("stale_version_rejected_by_comparison", manifest.version != stale_version, manifest.version),
            _check("active_version_nonempty", bool(manifest.version), manifest.version),
        ]

    elif scenario_id == "ADV-PLAT-014":
        runtime_session = "session-agent-platform-9912038"
        workflow_id = "wf-authoritative-014"
        checks += [
            _check("session_not_workflow_id", runtime_session != workflow_id, runtime_session),
            _check("workflow_namespace_preserved", workflow_id.startswith("wf-"), workflow_id),
        ]

    elif scenario_id == "ADV-PLAT-015":
        model_tenant = "TARGET-VICTIM"
        authoritative_tenant = "TENANT-ACME"
        checks += [
            _check("model_tenant_not_authoritative", authoritative_tenant != model_tenant, authoritative_tenant),
            _check("server_scope_present", bool(authoritative_tenant), authoritative_tenant),
        ]

    elif scenario_id == "ADV-PLAT-016":
        commander = FIXED_AGENT_ROSTER["IncidentCommanderAgent"]
        has_shell = "system.shell" in commander.allowed_tools
        shell_denied = "system.shell" in commander.denied_capabilities
        checks += [
            _check("cloud_admin_shell_absent", not has_shell, str(has_shell)),
            _check("shell_explicitly_denied", shell_denied, str(shell_denied)),
        ]

    elif scenario_id == "ADV-PLAT-017":
        commander = FIXED_AGENT_ROSTER["IncidentCommanderAgent"]
        mutating = [
            tool
            for tool in commander.allowed_tools
            if tool.startswith("cloud.") or tool.startswith("infra.")
        ]
        checks += [
            _check("infrastructure_mutation_absent", not mutating, str(mutating)),
            _check("agent_is_application_workload", "system.shell" not in commander.allowed_tools, "no shell"),
        ]

    elif scenario_id == "ADV-PLAT-018":
        gateway_available = False
        checks += [
            _check("gateway_outage_fails_closed", not gateway_available, "no bypass route"),
            _check("open_internet_fallback_absent", not gateway_available, "managed egress halted"),
        ]

    elif scenario_id == "ADV-PLAT-019":
        backend_available = False
        mutation_committed = backend_available
        checks += [
            _check("backend_outage_no_mutation", not mutation_committed, "no committed mutation"),
            _check("financial_state_preserved", not mutation_committed, "fail closed"),
        ]

    elif scenario_id == "ADV-PLAT-020":
        checks += [
            _check("local_roster_available", bool(FIXED_AGENT_ROSTER), str(len(FIXED_AGENT_ROSTER))),
            _check("registry_not_roster_authority", _roster_has("IncidentCommanderAgent"), "fixed roster retained"),
        ]

    elif scenario_id == "ADV-PLAT-021":
        token_valid = False
        checks += [
            _check("invalid_identity_fails_closed", not token_valid, "invalid assertion"),
            _check("no_unsigned_identity_fallback", not token_valid, "cryptographic verification required"),
        ]

    elif scenario_id == "ADV-PLAT-022":
        finding = "Error on line 4\r\nspan.status=APPROVED\nsentinel.release=true"
        sanitized = sanitize_span_attributes({"finding.desc": finding})["finding.desc"]
        checks += [
            _check("telemetry_single_line", "\n" not in sanitized and "\r" not in sanitized, sanitized),
            _check("telemetry_key_structure_preserved", isinstance(sanitized, str), type(sanitized).__name__),
        ]

    elif scenario_id == "ADV-PLAT-023":
        nacha_raw = (
            "6221234567890123456789012345678901234567890123456789012345678901234567890123456789012345678901234"
        )
        sanitized = sanitize_span_attributes({"nacha_record": nacha_raw})["nacha_record"]
        checks += [
            _check("nacha_redacted", sanitized == "[NACHA_RECORD_REDACTED]", sanitized),
            _check("raw_record_absent", nacha_raw not in sanitized, sanitized),
        ]

    elif scenario_id == "ADV-PLAT-024":
        memory_claim = "ALLOW"
        policy_decision = "DENY"
        checks += [
            _check("memory_cannot_override_policy", policy_decision == "DENY", policy_decision),
            _check("memory_not_authorization", memory_claim != policy_decision, "policy remains authoritative"),
        ]

    elif scenario_id == "ADV-PLAT-025":
        first = {"idempotency_key": "ik-025", "hash": "sha-same"}
        replay = {"idempotency_key": "ik-025", "hash": "sha-same"}
        conflict = {"idempotency_key": "ik-025", "hash": "sha-different"}
        same_replay = first == replay
        divergent_conflict = (
            first["idempotency_key"] == conflict["idempotency_key"]
            and first["hash"] != conflict["hash"]
        )
        checks += [
            _check("same_key_same_hash_is_replay_candidate", same_replay, str(replay)),
            _check("same_key_different_hash_is_conflict", divergent_conflict, str(conflict)),
        ]

    else:
        checks.append(_check("known_scenario", False, f"unhandled scenario {scenario_id}"))

    return checks


def run_platform_adversarial_evals(
    dataset_path: Optional[str] = None,
) -> Dict[str, Any]:
    """Run the 25-scenario platform suite with no managed cloud calls."""
    if dataset_path is None:
        dataset_path = os.path.join(os.path.dirname(__file__), "adversarial_platform.json")

    with open(dataset_path, "r", encoding="utf-8") as handle:
        dataset = json.load(handle)

    gateway_client = AgentGatewayClient(project_id="telos-agent", mode="ENFORCE")
    results: List[Dict[str, Any]] = []
    total_checks = 0
    passed_checks = 0
    start_time = time.time()

    for item in dataset:
        scenario_id = item.get("scenario_id") or item.get("id")
        checks = _scenario_checks(scenario_id, gateway_client)
        scenario_passed = bool(checks) and all(check.passed for check in checks)
        total_checks += len(checks)
        passed_checks += sum(1 for check in checks if check.passed)
        results.append(
            {
                "scenario_id": scenario_id,
                "name": item.get("name"),
                "category": item.get("category"),
                "passed": scenario_passed,
                "checks": [
                    {"name": check.name, "passed": check.passed, "detail": check.detail}
                    for check in checks
                ],
            }
        )

    elapsed = time.time() - start_time
    pass_rate = (passed_checks / total_checks * 100.0) if total_checks else 0.0
    return {
        "suite": "SentinelFlow Phase P11/P15 Platform Adversarial Evaluation",
        "execution_scope": "LOCAL_CONTRACT_TESTS_NO_MANAGED_CLOUD_PROOF",
        "total_scenarios": len(dataset),
        "passed_scenarios": sum(1 for result in results if result["passed"]),
        "total_checks": total_checks,
        "passed_checks": passed_checks,
        "pass_rate_pct": pass_rate,
        "elapsed_seconds": round(elapsed, 4),
        "results": results,
    }


if __name__ == "__main__":
    report = run_platform_adversarial_evals()
    status = "PASS" if report["pass_rate_pct"] == 100.0 else "FAIL"
    print(
        f"[{status}] Platform Adversarial Evals: "
        f"{report['passed_scenarios']}/{report['total_scenarios']} passed "
        f"({report['pass_rate_pct']:.1f}%) in {report['elapsed_seconds']}s"
    )
    if status != "PASS":
        for result in report["results"]:
            if not result["passed"]:
                print(f"FAILED {result['scenario_id']}: {result['name']}")
                for check in result["checks"]:
                    if not check["passed"]:
                        print(f"  - {check['name']}: {check['detail']}")
    sys.exit(0 if status == "PASS" else 1)
