@echo off
TITLE Pulse Surveillance & AI Fire Detection System
SETLOCAL EnableDelayedExpansion

REM Change to project root
cd /d "%~dp0"

echo ================================================================
echo    PULSE 1-CLICK ALL-IN-ONE SURVEILLANCE & FIRE DETECTION
echo ================================================================
echo.
echo Launching:
echo   [1] YOLO11 Fire & Smoke AI Inference Service (:8000)
echo   [2] Pulse Cloud Signaling & Auth Backend (:9091)
echo   [3] Home Surveillance Edge Daemon (:8080)
echo   [4] Web Dashboard UI (:5173)
echo   [5] Instant Cloudflare Public Tunnel + QR Link
echo.

node "hss-phase1a\scripts\start_all.cjs"

pause
