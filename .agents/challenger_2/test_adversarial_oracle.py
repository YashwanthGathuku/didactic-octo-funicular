import json
import re
import sys
import yaml
import importlib.util

print("================================================================")
print(" SENTINELFLOW ADVERSARIAL ORACLE VERIFICATION SUITE")
print("================================================================")

# ----------------------------------------------------------------------
# Stage 1: CAPABILITY_MATRIX.yaml verification
# ----------------------------------------------------------------------
with open("docs/CAPABILITY_MATRIX.yaml", "r", encoding="utf-8") as f:
    cap = yaml.safe_load(f)

lens_cap = cap.get("capabilities", {}).get("sentinelflow_lens") or cap.get("sentinelflow_lens")
print("\n[CHECK 1] docs/CAPABILITY_MATRIX.yaml sentinelflow_lens:")
print(f"   Status:        {lens_cap.get('status')}")
print(f"   Evidence:      {lens_cap.get('evidence')}")
print(f"   Test Command:  {lens_cap.get('test_command')}")
assert lens_cap is not None, "sentinelflow_lens capability not found"
assert lens_cap.get("status") == "TESTED", f"Expected TESTED, got {lens_cap.get('status')}"
print("   --> PASS: sentinelflow_lens status is TESTED")

# ----------------------------------------------------------------------
# Stage 2: Raw-SQL Authority Guard Scan
# ----------------------------------------------------------------------
import os
raw_sql_patterns = [
    re.compile(r'json:"(sql|raw_sql)"', re.IGNORECASE),
    re.compile(r'SELECT\s+\*.*\+', re.IGNORECASE),
    re.compile(r'executes?ql', re.IGNORECASE),
]
found_violations = []
paths_to_check = ["gateway/lens.go"]
for root, dirs, files in os.walk("gateway/internal/lens"):
    for f in files:
        if not f.endswith("_test.go") and not f.endswith(".md"):
            paths_to_check.append(os.path.join(root, f))

for p in paths_to_check:
    with open(p, "r", encoding="utf-8", errors="ignore") as f:
        content = f.read()
        for idx, line in enumerate(content.splitlines(), 1):
            for pat in raw_sql_patterns:
                if pat.search(line):
                    found_violations.append((p, idx, line))

print(f"\n[CHECK 2] Raw SQL Authority Guard Scan across {len(paths_to_check)} files:")
for p in paths_to_check:
    print(f"   Scanned: {p}")
assert len(found_violations) == 0, f"Found raw SQL violations: {found_violations}"
print("   --> PASS: Zero raw-SQL authority patterns found")

# ----------------------------------------------------------------------
# Stage 3: GCP Registry Capability Verification
# ----------------------------------------------------------------------
with open("docs/registry/agent_registry_v1.json", "r", encoding="utf-8") as f:
    reg_data = json.load(f)

fleet = reg_data["agentRegistry"]["agents"][0]
registered_caps = fleet.get("registeredCapabilities", [])
print(f"\n[CHECK 3] docs/registry/agent_registry_v1.json registeredCapabilities:")
for c in registered_caps:
    print(f"   - {c}")

assert "lens.query" in registered_caps, "lens.query missing from registry registeredCapabilities"
assert "returnrisk.result.get" in registered_caps, "returnrisk.result.get missing from registry registeredCapabilities"
print("   --> PASS: lens.query and returnrisk.result.get are registered in registry JSON")

# ----------------------------------------------------------------------
# Stage 4: 7-Agent Fleet Manifest Synchronization (Go vs Python)
# ----------------------------------------------------------------------
spec = importlib.util.spec_from_file_location("manifests", "ai-tier/contracts/manifests.py")
manifests_mod = importlib.util.module_from_spec(spec)
spec.loader.exec_module(manifests_mod)
py_roster = manifests_mod.FIXED_AGENT_ROSTER

with open("gateway/internal/auth/agent_identity.go", "r", encoding="utf-8") as f:
    go_content = f.read()

# Parse rosterAgent block from Go
# Format:
# "AgentName": rosterAgent(
#     "AgentName", AutonomyA1,
#     []string{"tool1", "tool2", ...},
#     ...
# )
go_roster = {}
# Regex matching agent definition block
block_pattern = re.compile(
    r'"(?P<name>[A-Za-z0-9_]+)":\s*rosterAgent\(\s*"(?P<name2>[A-Za-z0-9_]+)",\s*[A-Za-z0-9_]+,\s*\[\]string\{(?P<tools>[^}]+)\}',
    re.DOTALL
)
for match in block_pattern.finditer(go_content):
    agent_name = match.group("name")
    tools_raw = match.group("tools")
    tools = [t.strip().strip('"') for t in tools_raw.split(',') if t.strip().strip('"')]
    go_roster[agent_name] = sorted(tools)

agents_7 = [
    "IncidentCommanderAgent",
    "DiagnosisAgent",
    "PolicySLAAgent",
    "RemediationAgent",
    "VerifierAgent",
    "MemoryAgent",
    "ReturnRiskAgent"
]

print(f"\n[CHECK 4] 7-Agent Fleet Manifest Synchronization Matrix:")
for agent in agents_7:
    assert agent in py_roster, f"{agent} missing from Python roster"
    assert agent in go_roster, f"{agent} missing from Go roster"
    
    py_tools = sorted(py_roster[agent].allowed_tools)
    go_tools = go_roster[agent]
    
    print(f"\n   Agent: {agent}")
    print(f"     Go tools:     {go_tools}")
    print(f"     Python tools: {py_tools}")
    assert py_tools == go_tools, f"Roster mismatch for {agent}: Go={go_tools} vs Py={py_tools}"

# Explicit verification of lens.query for IncidentCommander and ReturnRisk
assert "lens.query" in py_roster["IncidentCommanderAgent"].allowed_tools
assert "lens.query" in py_roster["ReturnRiskAgent"].allowed_tools
assert "lens.query" in go_roster["IncidentCommanderAgent"]
assert "lens.query" in go_roster["ReturnRiskAgent"]

print("\n   --> PASS: All 7 agents have perfectly matching allowed tools across Go and Python")
print("\n================================================================")
print(" ALL ADVERSARIAL ORACLE CHECKS PASSED EMPIRICALLY (0 FAILURES)")
print("================================================================")
