@echo off
chcp 65001 >nul 2>&1
title CloseMask - 编译脚本

echo ========================================
echo   CloseMask - 编译脚本 v2.3
echo ========================================
echo.

:: 检查 Go 环境
where go >nul 2>&1
if errorlevel 1 (
    echo [错误] 未找到 Go 环境，请先安装 Go 1.21+
    echo [下载] https://go.dev/dl/
    pause
    exit /b 1
)

echo [信息] Go 版本:
go version
echo.

echo [信息] 正在编译 CloseMask...
go build -o "%~dp0closemask.exe" ./cmd/server

if errorlevel 1 (
    echo [错误] 编译失败！
    pause
    exit /b 1
)

echo [信息] 编译成功！
echo [信息] 运行 start.bat 启动服务，或直接运行 closemask.exe
echo.

:: 运行 vet 检查
echo [信息] 运行 go vet 检查...
go vet ./...
if errorlevel 1 (
    echo [警告] go vet 发现问题，请检查代码
) else (
    echo [信息] go vet 检查通过
)

:: 运行测试
echo.
echo [信息] 运行单元测试...
go test ./...
if errorlevel 1 (
    echo [警告] 部分测试未通过
) else (
    echo [信息] 所有测试通过
)

echo.
pause
