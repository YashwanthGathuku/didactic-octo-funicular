#!/usr/bin/env bash
set -euo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"

echo "============================================================"
echo " SentinelFlow Lens Lite verification — LOCAL ONLY"
echo "============================================================"
echo "No cloud deployment or production mutation is performed."

for cmd in go python npm; do
  if ! command -v "$cmd" >/dev/null 2>&1; then
    echo "ERROR: $cmd is required" >&2
    exit 1
  fi
done

required=(
  "$ROOT/gateway/internal/lens/service.go"
  "$ROOT/gateway/migrations/023_lens_lite.sql"
  "$ROOT/gateway/lens.go"
  "$ROOT/src/components/ops/LensWorkspace.tsx"
  "$ROOT/demo/lens_return_events.csv"
)
for path in "${required[@]}"; do
  if [[ ! -f "$path" ]]; then
    echo "ERROR: Lens Lite implementation file missing: $path" >&2
    exit 1
  fi
done

if grep -nA3 '^  sentinelflow_lens:' "$ROOT/docs/CAPABILITY_MATRIX.yaml" | grep -q 'status: PLANNED'; then
  echo "ERROR: capability matrix still marks Lens as PLANNED" >&2
  exit 1
fi

echo "[1/7] Synthetic demo provenance and determinism"
(
  cd "$ROOT"
  python -m unittest tests.test_lens_demo_data -v
  python scripts/generate_lens_demo_data.py --output demo/lens_return_events.csv
)

echo "[2/7] Go Lens semantic compiler + tenant/provenance tests"
(
  cd "$ROOT/gateway"
  go test -race ./internal/lens/...
)

echo "[3/7] Go Lens HTTP/migration compile gate"
(
  cd "$ROOT/gateway"
  go test ./... -run 'TestLens|TestNonExistentSentinelLensCompileGate' -count=1
)

echo "[4/7] Raw-SQL authority guard"
if grep -RInE --exclude='*_test.go' --exclude='*.md' 'json:"(sql|raw_sql)"|SELECT[[:space:]]+\*.*\+|executes?ql' "$ROOT/gateway/internal/lens" "$ROOT/gateway/lens.go"; then
  echo "ERROR: Lens executable surface appears to expose raw SQL authority" >&2
  exit 1
fi

echo "[5/7] Frontend Lens tests + production build"
(
  cd "$ROOT"
  npm test
  npm run build
)

echo "[6/7] Documentation/capability synchronization"
(
  cd "$ROOT"
  python scripts/generate_docs.py
  python scripts/generate_docs.py --check
)

echo "[7/7] Original 12-stage submission freeze regression"
(
  cd "$ROOT"
  bash scripts/verify_submission_freeze.sh
)

echo "============================================================"
echo " SENTINELFLOW LENS LITE LOCAL GATE PASSED"
echo "============================================================"
