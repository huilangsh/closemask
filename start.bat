@echo off
chcp 65001 >nul 2>&1
title CloseMask - PII 保护代理

echo ========================================
echo   CloseMask - PII 保护代理
echo   一键启动脚本 v2.3
echo ========================================
echo.

:: 检查 closemask.exe 是否存在
if not exist "%~dp0closemask.exe" (
    echo [错误] 未找到 closemask.exe
    echo [提示] 请先运行 build.bat 编译，或从 Release 下载预编译版本
    echo.
    pause
    exit /b 1
)

:: 检查配置文件
if not exist "%~dp0config.json" (
    echo [警告] 未找到 config.json，将使用默认配置
    echo.
)

:: 创建数据目录
if not exist "%~dp0data" (
    mkdir "%~dp0data"
    echo [信息] 已创建数据目录: data\
)

echo [信息] 正在启动 CloseMask...
echo [信息] 默认端口: 8846
echo [信息] 按 Ctrl+C 停止服务
echo.

"%~dp0closemask.exe" -config "%~dp0config.json" %*

if errorlevel 1 (
    echo.
    echo [错误] CloseMask 异常退出，错误码: %errorlevel%
    pause
)
