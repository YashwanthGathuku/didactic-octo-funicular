#!/usr/bin/env bash
set -euo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"

echo "== SentinelFlow P13-P15 hardening verification =="
echo "Repository: $ROOT"

echo
echo "[1/8] Execution-control kill switches and budgets"
(
  cd "$ROOT/gateway"
  go test -race ./internal/executioncontrol/... -count=1
)

echo
echo "[2/8] Governed Tool Gateway"
(
  cd "$ROOT/gateway"
  go test -race ./internal/toolgateway/... -count=1
)

echo
echo "[3/8] Deterministic Policy Engine"
(
  cd "$ROOT/gateway"
  go test -race ./internal/policy/... -count=1
)

echo
echo "[4/8] Independent verification + candidate safety"
(
  cd "$ROOT/gateway"
  go test -race ./internal/verification/... ./internal/candidate/... -count=1
)

echo
echo "[5/8] AI guardrail/provider regression"
(
  cd "$ROOT"
  python -m pytest \
    ai-tier/tests/test_model_armor.py \
    ai-tier/tests/test_guarded_model_boundary.py \
    ai-tier/tests/test_platform_runtime.py \
    -q
)

echo
echo "[6/8] Platform adversarial controls"
(
  cd "$ROOT"
  python ai-tier/evals/platform_runner.py
)

echo
echo "[7/8] Full adversarial fleet suite"
(
  cd "$ROOT"
  python ai-tier/evals/runner.py
)

echo
echo "[8/8] Secret hygiene and docs truth"
(
  cd "$ROOT/gateway"
  go test -run TestNoLiteralSecretsInSourceOrConfig . -count=1
)
(
  cd "$ROOT"
  python scripts/generate_docs.py --check
)

echo
echo "P13-P15 VERIFIED CLOSED: hardening, failure controls, and red-team gates passed locally."
