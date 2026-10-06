# Pulse Surveillance Platform - Root E2E Runner
Write-Host "Executing Pulse YOLO11 Fire & Smoke Detection E2E Suite..." -ForegroundColor Cyan

$hssDir = Join-Path (Split-Path -Parent $MyInvocation.MyCommand.Path) "hss-phase1a"
Push-Location $hssDir

try {
    go run ./tests/e2e/runner/main.go
    exit $LASTEXITCODE
} finally {
    Pop-Location
}
