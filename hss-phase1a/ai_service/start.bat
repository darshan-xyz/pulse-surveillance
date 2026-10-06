@echo off
REM Pulse AI Inference Service Starter
SET CURRENT_DIR=%~dp0
cd /d "%CURRENT_DIR%"

SET PYTHON_CMD=python
where python >nul 2>&1
if %ERRORLEVEL% NEQ 0 (
    if exist "C:\Users\KV\AppData\Local\Programs\Python\Python311\python.exe" (
        SET PYTHON_CMD="C:\Users\KV\AppData\Local\Programs\Python\Python311\python.exe"
    )
)

echo [Pulse AI] Starting YOLO11 Fire and Smoke AI Inference Service...
%PYTHON_CMD% main.py --host 127.0.0.1 --port 8000
