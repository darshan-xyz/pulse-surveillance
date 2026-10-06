<#
.SYNOPSIS
    Pulse 1-Click All-in-One Surveillance & AI Fire Detection System Launcher.
.DESCRIPTION
    Starts the Python YOLO11 AI inference service, Go Cloud Backend, Go Edge Daemon,
    Svelte Web Dashboard, and Cloudflare Public Ingress Tunnel in a single command.
#>

$ErrorActionPreference = "Stop"
$Host.UI.RawUI.WindowTitle = "Pulse Surveillance & AI Fire Detection System"

$ScriptDir = Split-Path -Parent $MyInvocation.MyCommand.Path
Set-Location $ScriptDir

Write-Host "================================================================" -ForegroundColor Cyan
Write-Host "   PULSE 1-CLICK ALL-IN-ONE SURVEILLANCE & FIRE DETECTION      " -ForegroundColor Cyan
Write-Host "================================================================" -ForegroundColor Cyan

# Check if node is installed
if (-not (Get-Command node -ErrorAction SilentlyContinue)) {
    Write-Host "`n[ERROR] Node.js is required to run the launcher. Please install Node.js." -ForegroundColor Red
    Read-Host "Press Enter to exit..."
    exit 1
}

# Run the 1-click orchestrator
node "hss-phase1a\scripts\start_all.cjs"
