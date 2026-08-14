#!/usr/bin/env bash
# ==============================================================================
# Sentinel Flow: Turnkey Unified Service Launcher (Bash / Linux / macOS)
# Launches: Go Gateway (:8080) + Python AI Tier (:8000) + React Operations UI (:3000)
# Supports: Native processes (default) or Rootless Podman (--podman)
# ==============================================================================

set -e

echo -e "\033[1;36m==========================================================\033[0m"
echo -e "\033[1;36m  SENTINEL FLOW: Financial File Reliability Gateway\033[0m"
echo -e "\033[1;36m  Starting Unified Distributed Services...\033[0m"
echo -e "\033[1;36m==========================================================\033[0m"

if [[ "$1" == "--podman" ]]; then
    echo -e "\n\033[1;33m[Podman Mode] Launching multi-container stack via podman-compose...\033[0m"
    podman-compose up -d --build
    echo -e "\033[1;32m  -> Containers running on ports 3000, 8080, 8000.\033[0m"
    exit 0
fi

cleanup() {
    echo -e "\n\033[1;33mShutting down Sentinel Flow background services...\033[0m"
    kill 0
}
trap cleanup SIGINT SIGTERM EXIT

# 1. Start Python AI Tier (:8000)
echo -e "\n\033[1;33m[1/3] Starting Python Astra 2.0 AI Tier on port 8000...\033[0m"
(cd ai-tier && uvicorn main:app --port 8000) &

# 2. Start Go Gateway Tier (:8080)
echo -e "\n\033[1;33m[2/3] Starting Go Gateway Tier on port 8080...\033[0m"
(cd gateway && go run main.go processor.go ledger.go watcher.go generator.go benchmark.go compliance.go iso20022.go bai2.go metrics.go security.go swift.go webhook.go anomaly.go connector.go agent_swarm.go healing.go drift.go stream.go vault.go instant_payment.go failover.go) &

# 3. Start React Operations UI (:3000)
echo -e "\n\033[1;33m[3/3] Starting React 18 Operations Cockpit on port 3000...\033[0m"
npm run dev
