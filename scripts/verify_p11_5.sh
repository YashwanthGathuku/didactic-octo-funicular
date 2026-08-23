#!/usr/bin/env bash
set -euo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"

echo "== SentinelFlow P11.5 local verification =="

echo "[1/5] Go managed-ingress auth tests"
(
  cd "$ROOT/gateway"
  go test -v ./internal/auth/...
)

echo "[2/5] Python Agent Platform runtime tests"
(
  cd "$ROOT"
  pytest ai-tier/tests/test_platform_runtime.py -v
)

echo "[3/5] Build fixed managed ADK topology (no cloud call)"
(
  cd "$ROOT/ai-tier"
  python - <<'PY'
from runtime.managed_adk import build_managed_fleet
fleet = build_managed_fleet()
assert fleet.root_agent.name == "IncidentCommanderAgent"
assert len(fleet.specialists) == 6
print("managed topology OK: 7 fixed ADK agents")
PY
)

echo "[4/5] Agent Runtime deployment dry-run (must create no resources)"
(
  cd "$ROOT/ai-tier"
  python -m runtime.deploy_agent_runtime --project "${GOOGLE_CLOUD_PROJECT:-telos-agent}" --location "${GOOGLE_CLOUD_LOCATION:-us-central1}"
)

echo "[5/5] Submission model-version guard"
if grep -RInE --exclude-dir=.git --exclude='*.md' --exclude='*.json' --exclude='*.yaml' --exclude='*.yml' \
  'gemini-(1\.5|2\.0|2\.5)' "$ROOT/ai-tier"; then
  echo "ERROR: legacy Gemini model found in executable AI-tier source" >&2
  exit 1
fi

echo "P11.5 LOCAL CODE GATE PASSED."
echo "NOTE: live Agent Runtime / Gateway / Registry / Identity remain NOT_RUN until real cloud proof is executed."
