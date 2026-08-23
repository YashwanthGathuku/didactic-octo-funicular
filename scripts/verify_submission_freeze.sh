#!/usr/bin/env bash
set -euo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"

echo "============================================================"
echo " SentinelFlow submission hardening freeze — LOCAL ONLY"
echo "============================================================"
echo "No cloud deployment or production mutation is performed."

if ! command -v go >/dev/null 2>&1; then
  echo "ERROR: go is required" >&2
  exit 1
fi
if ! command -v python >/dev/null 2>&1; then
  echo "ERROR: python is required" >&2
  exit 1
fi
if ! command -v npm >/dev/null 2>&1; then
  echo "ERROR: npm is required" >&2
  exit 1
fi

# ---------------------------------------------------------------------------
# Submission eligibility / P11 identity truth checks
# ---------------------------------------------------------------------------
echo "[1/12] Executable model-version guard"
if grep -RInE --exclude-dir=.git --exclude='*.md' --exclude='*.json' --exclude='*.yaml' --exclude='*.yml' \
  'gemini-(1\.5|2\.0|2\.5)' "$ROOT/ai-tier"; then
  echo "ERROR: legacy Gemini model found in executable AI-tier source" >&2
  exit 1
fi

if grep -RIn --exclude='test_*' --exclude='*_test.py' 'X-Agent-Identity-Principal' "$ROOT/ai-tier/runtime"; then
  echo "ERROR: runtime code still manufactures the legacy identity-principal header" >&2
  exit 1
fi

if grep -RInE --exclude='test_*' --exclude='*_test.py' 'status["'"']?\s*[:=]\s*["'"']COMPLETED["'"']' "$ROOT/ai-tier/runtime"; then
  echo "ERROR: runtime adapter appears to claim COMPLETED without managed proof" >&2
  exit 1
fi

echo "[2/12] Go P11.5 managed-ingress authentication"
(
  cd "$ROOT/gateway"
  go test -race ./internal/auth/...
)

echo "[3/12] Python P11.5 managed runtime packaging"
(
  cd "$ROOT"
  pytest ai-tier/tests/test_platform_runtime.py -v
)

echo "[4/12] Managed Agent Runtime deployment dry-run"
(
  cd "$ROOT/ai-tier"
  python -m runtime.deploy_agent_runtime \
    --project "${GOOGLE_CLOUD_PROJECT:-telos-agent}" \
    --location "${GOOGLE_CLOUD_LOCATION:-us-central1}"
)

echo "[5/12] P12.5 deterministic return-risk gate"
(
  cd "$ROOT/gateway"
  go test -race ./internal/returnrisk/...
)
(
  cd "$ROOT"
  pytest ai-tier/tests/test_return_risk_agent.py -v
  python ai-tier/evals/return_runner.py
)

echo "[6/12] P13-P15 execution-control + Tool Gateway race gate"
(
  cd "$ROOT/gateway"
  go test -race ./internal/executioncontrol/... ./internal/toolgateway/...
)

echo "[7/12] Full Go internal regression"
(
  cd "$ROOT/gateway"
  go test -race ./internal/...
)

echo "[8/12] Full Python AI-tier regression"
(
  cd "$ROOT"
  pytest ai-tier/tests/ -v
)

echo "[9/12] Master adversarial evaluation"
(
  cd "$ROOT"
  python ai-tier/evals/runner.py
)

echo "[10/12] Frontend unit tests"
(
  cd "$ROOT"
  npm test -- --run
)

echo "[11/12] Frontend production build"
(
  cd "$ROOT"
  npm run build
)

echo "[12/12] Generated documentation synchronization"
(
  cd "$ROOT"
  python scripts/generate_docs.py
  python scripts/generate_docs.py --check
)

echo "============================================================"
echo " SUBMISSION HARDENING FREEZE LOCAL GATE PASSED"
echo "============================================================"
echo "Live Agent Runtime / Agent Identity / Agent Gateway / Registry /"
echo "Memory Bank / Model Armor service / Cloud Trace remain separate"
echo "PASS_LIVE gates until actual Google Cloud evidence is captured."
