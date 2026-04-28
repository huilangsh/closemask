@echo off
chcp 65001 >nul 2>&1
setlocal enabledelayedexpansion

:: 切换到 release 目录
cd /d "%~dp0.."

:: CloseMask - GitHub Release 打包脚本
:: dist 文件夹存放最终发布包

set VERSION=0.11.2
set DIST_DIR=dist

echo ========================================
echo   CloseMask Release 打包脚本 v%VERSION%
echo ========================================
echo.

:: 检查 Go 环境
where go >nul 2>&1
if errorlevel 1 (
    echo [错误] 未找到 Go 环境
    exit /b 1
)

:: 清理 dist 目录
if exist "%DIST_DIR%" rmdir /s /q "%DIST_DIR%"
mkdir "%DIST_DIR%"

echo [1/6] 编译 Windows amd64...
set GOOS=windows
set GOARCH=amd64
set CGO_ENABLED=0
go build -ldflags="-s -w" -o "%DIST_DIR%\closemask-windows-amd64.exe" ./cmd/server
echo       完成！

echo [2/6] 编译 Windows arm64...
set GOARCH=arm64
go build -ldflags="-s -w" -o "%DIST_DIR%\closemask-windows-arm64.exe" ./cmd/server
echo       完成！

echo [3/6] 编译 Linux amd64...
set GOOS=linux
set GOARCH=amd64
go build -ldflags="-s -w" -o "%DIST_DIR%\closemask-linux-amd64" ./cmd/server
echo       完成！

echo [4/6] 编译 Linux arm64...
set GOARCH=arm64
go build -ldflags="-s -w" -o "%DIST_DIR%\closemask-linux-arm64" ./cmd/server
echo       完成！

echo [5/6] 编译 Darwin amd64...
set GOOS=darwin
set GOARCH=amd64
go build -ldflags="-s -w" -o "%DIST_DIR%\closemask-darwin-amd64" ./cmd/server
echo       完成！

echo [6/6] 编译 Darwin arm64...
set GOARCH=arm64
go build -ldflags="-s -w" -o "%DIST_DIR%\closemask-darwin-arm64" ./cmd/server
echo       完成！

echo.
echo [信息] 创建发布包...

:: 创建 Windows amd64 发布包
mkdir tmp
copy "%DIST_DIR%\closemask-windows-amd64.exe" "tmp\closemask.exe" >nul
copy config.json.release "tmp\config.json" >nul
copy LICENSE "tmp\" >nul
:: NER 服务（排除 logs 和 __pycache__）
xcopy "ner_service" "tmp\ner_service\" /E /I /Q /EXCLUDE:scripts\ner_exclude.txt >nul
(
echo # CloseMask v%VERSION%
echo.
echo ## 快速开始 / Quick Start
echo.
echo 1. 编辑 config.json，设置 llm_url
echo    Edit config.json, set llm_url
echo.
echo 2. 启动服务 / Start service:
echo    closemask.exe -config config.json
echo.
echo ## NER 服务（可选）/ NER Service ^(optional^)
echo.
echo 启用语义 PII 检测 / Enable semantic PII detection:
echo.
echo 1. 修改 config.json: "ner_enabled": true
echo 2. 启动 NER 服务 / Start NER service:
echo    cd ner_service
echo    start_ner.bat
echo.
echo 首次运行会自动创建虚拟环境并安装依赖
echo Auto-creates .venv and installs dependencies on first run
echo.
echo ## 文档 / Docs
echo.
echo https://github.com/huilangsh/closemask
) > tmp\README.md
powershell Compress-Archive -Path "tmp\*" -DestinationPath "%DIST_DIR%\closemask-%VERSION%-windows-amd64.zip" -Force
rmdir /s /q tmp
echo       closemask-%VERSION%-windows-amd64.zip

:: 创建 Windows arm64 发布包
mkdir tmp
copy "%DIST_DIR%\closemask-windows-arm64.exe" "tmp\closemask.exe" >nul
copy config.json.release "tmp\config.json" >nul
copy LICENSE "tmp\" >nul
xcopy "ner_service" "tmp\ner_service\" /E /I /Q /EXCLUDE:scripts\ner_exclude.txt >nul
(
echo # CloseMask v%VERSION%
echo.
echo ## 快速开始 / Quick Start
echo.
echo 1. 编辑 config.json，设置 llm_url
echo    Edit config.json, set llm_url
echo.
echo 2. 启动服务 / Start service:
echo    closemask.exe -config config.json
echo.
echo ## NER 服务（可选）/ NER Service ^(optional^)
echo.
echo 启用语义 PII 检测 / Enable semantic PII detection:
echo.
echo 1. 修改 config.json: "ner_enabled": true
echo 2. 启动 NER 服务 / Start NER service:
echo    cd ner_service
echo    start_ner.bat
echo.
echo 首次运行会自动创建虚拟环境并安装依赖
echo Auto-creates .venv and installs dependencies on first run
echo.
echo ## 文档 / Docs
echo.
echo https://github.com/huilangsh/closemask
) > tmp\README.md
powershell Compress-Archive -Path "tmp\*" -DestinationPath "%DIST_DIR%\closemask-%VERSION%-windows-arm64.zip" -Force
rmdir /s /q tmp
echo       closemask-%VERSION%-windows-arm64.zip

:: 创建 Linux amd64 发布包
mkdir tmp
copy "%DIST_DIR%\closemask-linux-amd64" "tmp\closemask" >nul
copy config.json.release "tmp\config.json" >nul
copy LICENSE "tmp\" >nul
xcopy "ner_service" "tmp\ner_service\" /E /I /Q /EXCLUDE:scripts\ner_exclude.txt >nul
(
echo # CloseMask v%VERSION%
echo.
echo ## 快速开始 / Quick Start
echo.
echo 1. 编辑 config.json，设置 llm_url
echo    Edit config.json, set llm_url
echo.
echo 2. 启动服务 / Start service:
echo    chmod +x closemask ^&^& ./closemask -config config.json
echo.
echo ## NER 服务（可选）/ NER Service ^(optional^)
echo.
echo 启用语义 PII 检测 / Enable semantic PII detection:
echo.
echo 1. 修改 config.json: "ner_enabled": true
echo 2. 启动 NER 服务 / Start NER service:
echo    cd ner_service
echo    start_ner.bat
echo.
echo 首次运行会自动创建虚拟环境并安装依赖
echo Auto-creates .venv and installs dependencies on first run
echo.
echo ## 文档 / Docs
echo.
echo https://github.com/huilangsh/closemask
) > tmp\README.md
powershell Compress-Archive -Path "tmp\*" -DestinationPath "%DIST_DIR%\closemask-%VERSION%-linux-amd64.zip" -Force
rmdir /s /q tmp
echo       closemask-%VERSION%-linux-amd64.zip

:: 创建 Linux arm64 发布包
mkdir tmp
copy "%DIST_DIR%\closemask-linux-arm64" "tmp\closemask" >nul
copy config.json.release "tmp\config.json" >nul
copy LICENSE "tmp\" >nul
xcopy "ner_service" "tmp\ner_service\" /E /I /Q /EXCLUDE:scripts\ner_exclude.txt >nul
(
echo # CloseMask v%VERSION%
echo.
echo ## 快速开始 / Quick Start
echo.
echo 1. 编辑 config.json，设置 llm_url
echo    Edit config.json, set llm_url
echo.
echo 2. 启动服务 / Start service:
echo    chmod +x closemask ^&^& ./closemask -config config.json
echo.
echo ## NER 服务（可选）/ NER Service ^(optional^)
echo.
echo 启用语义 PII 检测 / Enable semantic PII detection:
echo.
echo 1. 修改 config.json: "ner_enabled": true
echo 2. 启动 NER 服务 / Start NER service:
echo    cd ner_service
echo    start_ner.bat
echo.
echo 首次运行会自动创建虚拟环境并安装依赖
echo Auto-creates .venv and installs dependencies on first run
echo.
echo ## 文档 / Docs
echo.
echo https://github.com/huilangsh/closemask
) > tmp\README.md
powershell Compress-Archive -Path "tmp\*" -DestinationPath "%DIST_DIR%\closemask-%VERSION%-linux-arm64.zip" -Force
rmdir /s /q tmp
echo       closemask-%VERSION%-linux-arm64.zip

:: 创建 Darwin amd64 发布包
mkdir tmp
copy "%DIST_DIR%\closemask-darwin-amd64" "tmp\closemask" >nul
copy config.json.release "tmp\config.json" >nul
copy LICENSE "tmp\" >nul
xcopy "ner_service" "tmp\ner_service\" /E /I /Q /EXCLUDE:scripts\ner_exclude.txt >nul
(
echo # CloseMask v%VERSION%
echo.
echo ## 快速开始 / Quick Start
echo.
echo 1. 编辑 config.json，设置 llm_url
echo    Edit config.json, set llm_url
echo.
echo 2. 启动服务 / Start service:
echo    chmod +x closemask ^&^& ./closemask -config config.json
echo.
echo ## NER 服务（可选）/ NER Service ^(optional^)
echo.
echo 启用语义 PII 检测 / Enable semantic PII detection:
echo.
echo 1. 修改 config.json: "ner_enabled": true
echo 2. 启动 NER 服务 / Start NER service:
echo    cd ner_service
echo    start_ner.bat
echo.
echo 首次运行会自动创建虚拟环境并安装依赖
echo Auto-creates .venv and installs dependencies on first run
echo.
echo ## 文档 / Docs
echo.
echo https://github.com/huilangsh/closemask
) > tmp\README.md
powershell Compress-Archive -Path "tmp\*" -DestinationPath "%DIST_DIR%\closemask-%VERSION%-darwin-amd64.zip" -Force
rmdir /s /q tmp
echo       closemask-%VERSION%-darwin-amd64.zip

:: 创建 Darwin arm64 发布包
mkdir tmp
copy "%DIST_DIR%\closemask-darwin-arm64" "tmp\closemask" >nul
copy config.json.release "tmp\config.json" >nul
copy LICENSE "tmp\" >nul
xcopy "ner_service" "tmp\ner_service\" /E /I /Q /EXCLUDE:scripts\ner_exclude.txt >nul
(
echo # CloseMask v%VERSION%
echo.
echo ## 快速开始 / Quick Start
echo.
echo 1. 编辑 config.json，设置 llm_url
echo    Edit config.json, set llm_url
echo.
echo 2. 启动服务 / Start service:
echo    chmod +x closemask ^&^& ./closemask -config config.json
echo.
echo ## NER 服务（可选）/ NER Service ^(optional^)
echo.
echo 启用语义 PII 检测 / Enable semantic PII detection:
echo.
echo 1. 修改 config.json: "ner_enabled": true
echo 2. 启动 NER 服务 / Start NER service:
echo    cd ner_service
echo    start_ner.bat
echo.
echo 首次运行会自动创建虚拟环境并安装依赖
echo Auto-creates .venv and installs dependencies on first run
echo.
echo ## 文档 / Docs
echo.
echo https://github.com/huilangsh/closemask
) > tmp\README.md
powershell Compress-Archive -Path "tmp\*" -DestinationPath "%DIST_DIR%\closemask-%VERSION%-darwin-arm64.zip" -Force
rmdir /s /q tmp
echo       closemask-%VERSION%-darwin-arm64.zip

:: 删除编译产物，只保留 zip
del "%DIST_DIR%\closemask-windows-*.exe" 2>nul
del "%DIST_DIR%\closemask-linux-*" 2>nul
del "%DIST_DIR%\closemask-darwin-*" 2>nul

echo.
echo ========================================
echo   打包完成！
echo ========================================
echo.
echo 发布包:
dir /b "%DIST_DIR%\*.zip"
echo.
