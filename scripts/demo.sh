#!/usr/bin/env bash
set -euo pipefail

echo "============================================================"
echo " SentinelFlow End-to-End Demonstration Script"
echo "============================================================"

# 1. Ensure local .env exists with dynamically generated development credentials
if [ ! -f .env ]; then
    echo "[+] Creating local .env configuration..."
    RAND_SECRET=$(head -c 32 /dev/urandom 2>/dev/null | od -An -tx1 | tr -d ' \n' || date +%s%N)
    echo "POSTGRES_PASSWORD=${RAND_SECRET}" > .env
fi

# 2. Generate synthetic NACHA test file
DEMO_FILE="demo_payroll_batch.ach"
echo "[+] Generating synthetic NACHA PPD payment batch..."
python3 scripts/generate_nacha.py --entries 25 --amount-cents 4500000 --output "${DEMO_FILE}"

# 3. Check if Gateway is running locally
GATEWAY_URL="http://127.0.0.1:8080"
echo "[+] Probing SentinelFlow Gateway at ${GATEWAY_URL}..."

if ! curl -s "${GATEWAY_URL}/api/v1/ready" > /dev/null 2>&1; then
    echo "[*] Gateway not running locally. Starting Docker Compose stack..."
    docker compose up -d --build
    echo "[*] Waiting for Gateway readiness..."
    until curl -s "${GATEWAY_URL}/api/v1/ready" | grep -q "READY" 2>/dev/null; do
        sleep 2
    done
fi

echo "[OK] Gateway is READY."

# 4. Upload synthetic batch file
echo "[+] Ingesting synthetic batch file to Gateway API..."
UPLOAD_RESP=$(curl -s -X POST \
  -H "X-Tenant-ID: tenant-treasury-01" \
  -H "Content-Type: multipart/form-data" \
  -F "file=@${DEMO_FILE}" \
  "${GATEWAY_URL}/api/v1/files/upload" || true)

echo "    Upload Response: ${UPLOAD_RESP}"

# 5. Clean up temporary test file
rm -f "${DEMO_FILE}"

echo "============================================================"
echo " SentinelFlow Demo Completed Successfully!"
echo " Web UI Available at:   http://localhost:3000"
echo " Gateway API at:        http://localhost:8080"
echo "============================================================"
