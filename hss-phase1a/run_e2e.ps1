# Pulse Surveillance Platform - YOLO11 Fire & Smoke Detection E2E Runner
Write-Host "================================================================================" -ForegroundColor Cyan
Write-Host "   PULSE SURVEILLANCE PLATFORM — YOLO11 FIRE & SMOKE E2E TEST RUNNER" -ForegroundColor Cyan
Write-Host "================================================================================" -ForegroundColor Cyan

$scriptDir = Split-Path -Parent $MyInvocation.MyCommand.Path
Push-Location $scriptDir

try {
    go run ./tests/e2e/runner/main.go
    $exitCode = $LASTEXITCODE
    if ($exitCode -ne 0) {
        Write-Host "E2E Tests Failed with exit code $exitCode" -ForegroundColor Red
        exit $exitCode
    }
} finally {
    Pop-Location
}
