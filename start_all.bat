@echo off
chcp 65001 >nul 2>&1
title CloseMask - 完整环境启动

echo ========================================
echo   CloseMask 完整环境一键启动
echo   包含: AIFW Mock + Mock LLM + CloseMask
echo ========================================
echo.

set "SCRIPT_DIR=%~dp0"

:: ============ 1. 检查 Python ============
where python >nul 2>&1
if errorlevel 1 (
    echo [错误] 未找到 Python，请先安装 Python 3.x
    echo [下载] https://www.python.org/downloads/
    pause
    exit /b 1
)
echo [OK] Python 已安装

:: ============ 2. 安装 Flask ============
pip show flask >nul 2>&1
if errorlevel 1 (
    echo [信息] 安装 Flask...
    pip install flask requests
)

:: ============ 3. 杀掉残留进程 ============
echo [信息] 清理残留进程...
for /f "tokens=5" %%a in ('netstat -ano ^| findstr ":8845 " ^| findstr "LISTENING"') do (
    taskkill /F /PID %%a >nul 2>&1
)
for /f "tokens=5" %%a in ('netstat -ano ^| findstr ":11437 " ^| findstr "LISTENING"') do (
    taskkill /F /PID %%a >nul 2>&1
)
for /f "tokens=5" %%a in ('netstat -ano ^| findstr ":8846 " ^| findstr "LISTENING"') do (
    taskkill /F /PID %%a >nul 2>&1
)

:: ============ 4. 启动 AIFW Mock (端口 8845) ============
echo [信息] 启动 OneAIFW Mock 服务 (端口 8845)...
start "OneAIFW Mock" /MIN python "%SCRIPT_DIR%scripts\aifw_mock.py"

:: 等待端口就绪
echo [信息] 等待 AIFW 服务就绪...
set AIFW_READY=0
for /L %%i in (1,1,15) do (
    if !AIFW_READY!==0 (
        timeout /t 1 /nobreak >nul
        powershell -Command "try { $r = Invoke-WebRequest -Uri http://localhost:8845/api/health -TimeoutSec 2 -UseBasicParsing; if($r.StatusCode -eq 200){exit 0} } catch { exit 1 }" >nul 2>&1
        if not errorlevel 1 (
            set AIFW_READY=1
            echo [OK] OneAIFW Mock 已就绪
        )
    )
)
if %AIFW_READY%==0 (
    echo [警告] OneAIFW Mock 未能在 15 秒内就绪，继续启动...
)

:: ============ 5. 启动 Mock LLM (端口 11437) ============
echo [信息] 启动 Mock LLM 服务 (端口 11437)...
start "Mock LLM" /MIN python "%SCRIPT_DIR%scripts\mock_llm.py"

:: 等待端口就绪
echo [信息] 等待 Mock LLM 服务就绪...
set LLM_READY=0
for /L %%i in (1,1,15) do (
    if !LLM_READY!==0 (
        timeout /t 1 /nobreak >nul
        powershell -Command "try { $r = Invoke-WebRequest -Uri http://localhost:11437/health -TimeoutSec 2 -UseBasicParsing; if($r.StatusCode -eq 200){exit 0} } catch { exit 1 }" >nul 2>&1
        if not errorlevel 1 (
            set LLM_READY=1
            echo [OK] Mock LLM 已就绪
        )
    )
)
if %LLM_READY%==0 (
    echo [警告] Mock LLM 未能在 15 秒内就绪，继续启动...
)

:: ============ 6. 创建数据目录 ============
if not exist "%SCRIPT_DIR%data" (
    mkdir "%SCRIPT_DIR%data"
)

:: ============ 7. 启动 CloseMask ============
echo.
echo ========================================
echo   所有服务已启动！
echo ========================================
echo.
echo   OneAIFW Mock:  http://localhost:8845
echo   Mock LLM:      http://localhost:11437
echo   CloseMask:     http://localhost:8846
echo.
echo   测试命令:
echo   curl -X POST http://localhost:8846/v1/chat/completions -H "Content-Type: application/json" -d "{\"model\":\"test\",\"messages\":[{\"role\":\"user\",\"content\":\"My OPENAI_API_KEY=sk-proj-abc123\"}]}"
echo.
echo   按 Ctrl+C 停止 CloseMask
echo ========================================
echo.

"%SCRIPT_DIR%closemask.exe" -config "%SCRIPT_DIR%config.json"

:: ============ 8. 清理 ============
echo [信息] 正在停止辅助服务...
for /f "tokens=5" %%a in ('netstat -ano ^| findstr ":8845 " ^| findstr "LISTENING"') do (
    taskkill /F /PID %%a >nul 2>&1
)
for /f "tokens=5" %%a in ('netstat -ano ^| findstr ":11437 " ^| findstr "LISTENING"') do (
    taskkill /F /PID %%a >nul 2>&1
)
echo [信息] 所有服务已停止
pause
