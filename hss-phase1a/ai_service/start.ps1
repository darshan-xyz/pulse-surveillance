# PowerShell Startup Script for Pulse AI Inference Service
param (
    [string]$HostAddress = "127.0.0.1",
    [int]$Port = 8000,
    [string]$ModelPath = "",
    [switch]$Background
)

$scriptDir = Split-Path -Parent $MyInvocation.MyCommand.Path
Set-Location $scriptDir

$pythonExe = "python"
if (-not (Get-Command python -ErrorAction SilentlyContinue)) {
    if (Test-Path "C:\Users\KV\AppData\Local\Programs\Python\Python311\python.exe") {
        $pythonExe = "C:\Users\KV\AppData\Local\Programs\Python\Python311\python.exe"
    }
}

$argsList = @("main.py", "--host", $HostAddress, "--port", "$Port")
if ($ModelPath -ne "") {
    $argsList += @("--model-path", $ModelPath)
}

Write-Host "[Pulse AI] Launching YOLO11 Fire & Smoke Inference Service on http://${HostAddress}:${Port}" -ForegroundColor Green

if ($Background) {
    Start-Process -FilePath $pythonExe -ArgumentList $argsList -WindowStyle Hidden
    Write-Host "[Pulse AI] Service started in background." -ForegroundColor Cyan
} else {
    & $pythonExe @argsList
}
