#!/usr/bin/env bash
set -euo pipefail

# SentinelFlow P12.5 local closure gate.
# This script performs verification only. It does not deploy infrastructure,
# mutate production data, weaken tests, or start P13 work.

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "$ROOT_DIR"

echo "== SentinelFlow P12.5 local verification =="
echo "Repository: $ROOT_DIR"
echo

echo "[1/6] Go return-risk suite"
(
  cd gateway
  go test ./internal/returnrisk/... -v
)

echo
echo "[2/6] Go internal regression suite"
(
  cd gateway
  go test ./internal/...
)

echo
echo "[3/6] Python unit and conformance suite"
pytest ai-tier/tests/ -v

echo
echo "[4/6] Return-risk adversarial evaluation"
python ai-tier/evals/return_runner.py

echo
echo "[5/6] Master adversarial evaluation"
python ai-tier/evals/runner.py

echo
echo "[6/6] Generated submission drift check"
python scripts/generate_docs.py --check

echo
echo "P12.5 VERIFIED CLOSED: all required local gates passed."
