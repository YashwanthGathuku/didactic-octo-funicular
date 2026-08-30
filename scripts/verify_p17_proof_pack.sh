#!/usr/bin/env bash
set -euo pipefail
ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"

printf '== SentinelFlow P17 proof-pack LOCAL gate ==\n'

echo '[1/6] Python proof scripts compile'
python -m py_compile \
  "$ROOT/ai-tier/runtime/live_gemini_proof.py" \
  "$ROOT/ai-tier/runtime/live_model_armor_proof.py" \
  "$ROOT/ai-tier/runtime/live_memory_bank_proof.py" \
  "$ROOT/scripts/prepare_ibm_aml_demo.py" \
  "$ROOT/scripts/verify_public_demo_data.py" \
  "$ROOT/scripts/prepare_moov_ach_demo.py" \
  "$ROOT/scripts/validate_agent_registry.py"

echo '[2/6] Cloud proof scripts default to dry-run'
PYTHONPATH="$ROOT/ai-tier" python "$ROOT/ai-tier/runtime/live_gemini_proof.py" | grep -q 'NOT_RUN'
PYTHONPATH="$ROOT/ai-tier" python "$ROOT/ai-tier/runtime/live_model_armor_proof.py" | grep -q 'NOT_RUN'
PYTHONPATH="$ROOT/ai-tier" python "$ROOT/ai-tier/runtime/live_memory_bank_proof.py" | grep -q 'NOT_RUN'

echo '[3/6] Model Armor setup defaults to dry-run'
bash "$ROOT/deployment/gcp/setup_model_armor_demo.sh" | grep -q 'DRY_RUN'

echo '[4/6] IBM ground-truth separation smoke test'
TMP="$(mktemp -d)"
trap 'rm -rf "$TMP"' EXIT
cat > "$TMP/ibm.csv" <<'CSV'
Timestamp,From Bank,Account,To Bank,Account.1,Amount Received,Receiving Currency,Amount Paid,Payment Currency,Payment Format,Is Laundering
2026/08/20 09:00,001,A100,002,B200,100.00,US Dollar,100.00,US Dollar,ACH,0
2026/08/20 09:01,001,A101,003,B201,8500.00,US Dollar,8500.00,US Dollar,ACH,1
2026/08/20 09:02,004,A102,005,B202,20.00,US Dollar,20.00,US Dollar,Cheque,0
CSV
python "$ROOT/scripts/prepare_ibm_aml_demo.py" "$TMP/ibm.csv" --output-dir "$TMP/out" --max-rows 10 >/dev/null
python "$ROOT/scripts/verify_public_demo_data.py" "$TMP/out" | grep -q 'PASS'
! head -n1 "$TMP/out/ibm_aml_ach_lens.csv" | grep -qi 'laundering'

echo '[5/6] Moov/NACHA 94-char fixture validation smoke test'
# Generate a deterministic 94-character valid-shape record.
python - "$TMP/sample.ach" <<'PY'
from pathlib import Path
import sys
p=Path(sys.argv[1])
p.write_text('1' + (' ' * 93) + '\n', encoding='ascii')
PY
python "$ROOT/scripts/prepare_moov_ach_demo.py" "$TMP/sample.ach" --output-dir "$TMP/moov" >/dev/null

echo '[6/6] Agent registry validation & least-privilege manifest conformance'
python "$ROOT/scripts/validate_agent_registry.py" --check

echo 'P17 PROOF PACK LOCAL PASS: no cloud mutation performed.'
