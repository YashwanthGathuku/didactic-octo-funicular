# ==============================================================================
# Sentinel Flow: Turnkey Unified Service Launcher (PowerShell / Windows)
# Launches: Go Gateway (:8080) + Python AI Tier (:8000) + React Operations UI (:3000)
# Supports: Native processes (default) or Rootless Podman (-Podman)
# ==============================================================================
param (
    [switch]$Podman
)

Write-Host "==========================================================" -ForegroundColor Cyan
Write-Host "  SENTINEL FLOW: Financial File Reliability Gateway" -ForegroundColor Cyan
Write-Host "  Starting Unified Distributed Services..." -ForegroundColor Cyan
Write-Host "==========================================================" -ForegroundColor Cyan

$WorkspaceRoot = Get-Location

if ($Podman) {
    Write-Host "`n[Podman Mode] Launching multi-container stack via podman-compose..." -ForegroundColor Yellow
    podman-compose up -d --build
    Write-Host "  -> Podman containers running on ports 3000, 8080, 8000." -ForegroundColor Green
    Start-Process "http://localhost:3000"
    exit 0
}

# 1. Start Python AI Tier (:8000)
Write-Host "`n[1/3] Starting Python Astra 2.0 AI Tier on port 8000..." -ForegroundColor Yellow
$PythonProcess = Start-Process -FilePath "powershell.exe" -ArgumentList "-NoExit", "-Command", "cd `"$WorkspaceRoot\ai-tier`"; uvicorn main:app --port 8000" -PassThru
Write-Host "  -> Python AI Tier started (PID: $($PythonProcess.Id))" -ForegroundColor Green

# 2. Start Go Gateway Tier (:8080)
Write-Host "`n[2/3] Starting Go Gateway Tier on port 8080..." -ForegroundColor Yellow
$GoProcess = Start-Process -FilePath "powershell.exe" -ArgumentList "-NoExit", "-Command", "cd `"$WorkspaceRoot\gateway`"; go run main.go processor.go ledger.go watcher.go generator.go benchmark.go compliance.go iso20022.go bai2.go metrics.go security.go swift.go webhook.go anomaly.go connector.go agent_swarm.go healing.go drift.go stream.go vault.go instant_payment.go failover.go" -PassThru
Write-Host "  -> Go Gateway started (PID: $($GoProcess.Id))" -ForegroundColor Green

# 3. Start React Operations PWA (:3000)
Write-Host "`n[3/3] Starting React 18 Operations Cockpit on port 3000..." -ForegroundColor Yellow
Start-Process "http://localhost:3000"
npm run dev
