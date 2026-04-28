@echo off
chcp 65001 >nul
title CloseMask NER Service

echo ========================================
echo   CloseMask NER Service
echo   Port: 8847
echo ========================================
echo.

cd /d "%~dp0"

REM Check Python - use 'where' command for reliable detection
where python >nul 2>&1
if errorlevel 1 (
    REM Try 'py' launcher as fallback
    where py >nul 2>&1
    if errorlevel 1 (
        echo [ERROR] Python not found!
        echo Please install Python 3.8+ from https://www.python.org/downloads/
        pause
        exit /b 1
    )
    set PYTHON_CMD=py
) else (
    set PYTHON_CMD=python
)

REM Set venv path
set VENV_PATH=%~dp0.venv

REM Check and create venv
if not exist "%VENV_PATH%\Scripts\activate.bat" (
    echo [INFO] Creating venv...
    %PYTHON_CMD% -m venv "%VENV_PATH%"
)

call "%VENV_PATH%\Scripts\activate.bat"

echo [INFO] Checking dependencies...
pip show fastapi >nul 2>&1
if errorlevel 1 (
    echo [INFO] Installing dependencies...
    pip install -r requirements.txt
)

if not exist "logs" mkdir logs

echo [INFO] Starting NER Service on port 8847...
echo [INFO] Close this window to stop the service
echo.
python ner_service.py

echo [INFO] NER Service stopped
pause
