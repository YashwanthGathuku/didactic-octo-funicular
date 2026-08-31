#!/usr/bin/env bash
set -euo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
PROJECT="${GOOGLE_CLOUD_PROJECT:-project-3687901b-8355-4073-ac3}"
REGION="${GOOGLE_CLOUD_LOCATION:-us-central1}"

printf '== SentinelFlow P17 local deployment-readiness gate ==\n'
printf 'project=%s region=%s\n' "$PROJECT" "$REGION"

echo
echo "[1/7] Managed ADK topology"
(
  cd "$ROOT"
  python - <<'PY'
import sys
sys.path.insert(0, "ai-tier")
from runtime.managed_adk import MANAGED_MODEL, build_managed_fleet
fleet = build_managed_fleet()
assert MANAGED_MODEL == "gemini-3.5-flash"
assert fleet.root_agent.name == "IncidentCommanderAgent"
assert len(fleet.specialists) == 6
print("PASS: real ADK fixed seven-agent topology")
PY
)

echo
echo "[2/7] Agent Runtime deployment dry-run"
(
  cd "$ROOT"
  PYTHONPATH=ai-tier python ai-tier/runtime/deploy_agent_runtime.py \
    --project "$PROJECT" \
    --location "$REGION" \
    --display-name sentinelflow-p17-dev
)

echo
echo "[3/7] Agent Runtime smoke dry-run"
(
  cd "$ROOT"
  PYTHONPATH=ai-tier python ai-tier/runtime/smoke_agent_runtime.py \
    --project "$PROJECT" \
    --location "$REGION" \
    --engine-id DRY-RUN-ENGINE
)

echo
echo "[4/7] Agent Gateway/Registry/IAP plan"
(
  cd "$ROOT"
  GOOGLE_CLOUD_PROJECT="$PROJECT" GOOGLE_CLOUD_LOCATION="$REGION" \
    bash deployment/gcp/setup_agent_gateway.sh
)

echo
echo "[5/7] Runtime-to-Gateway binding plan"
(
  cd "$ROOT"
  GOOGLE_CLOUD_PROJECT="$PROJECT" GOOGLE_CLOUD_LOCATION="$REGION" \
    bash deployment/gcp/bind_runtime_gateway.sh
)

echo
echo "[6/7] Managed ingress security tests"
(
  cd "$ROOT/gateway"
  go test ./internal/auth/... -count=1
  go test ./internal/toolgateway/... -count=1
  go test . -run 'Managed|AgentIdentity|IAP' -count=1
)

echo
echo "[7/7] UI proof defaults and production build"
(
  cd "$ROOT"
  grep -q "Defaults to NOT_RUN" src/components/ops/SubmissionProofScreen.tsx
  npm run build
)

echo
echo "P17 LOCAL READY: deployment tooling is dry-run clean. No live Google resource was created or proven by this gate."
