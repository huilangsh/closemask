#!/bin/bash
# CloseMask - 跨平台打包脚本
# 用于生成各平台的发布版本

set -e

VERSION="0.11.0"
DIST_DIR="dist"
RELEASE_DIR="release-packages"
SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
ROOT_DIR="$(dirname "$SCRIPT_DIR")"

cd "$ROOT_DIR"

echo "========================================"
echo "  CloseMask 跨平台打包脚本 v${VERSION}"
echo "========================================"
echo ""

# 检查 Go 环境
if ! command -v go &> /dev/null; then
    echo "[错误] 未找到 Go 环境"
    exit 1
fi

# 创建目录
mkdir -p "$DIST_DIR" "$RELEASE_DIR"

# 清理旧的发布包
rm -f "$RELEASE_DIR"/*.zip

echo "[1/5] 编译 Windows amd64..."
GOOS=windows GOARCH=amd64 CGO_ENABLED=0 go build -ldflags="-s -w" -o "$DIST_DIR/closemask-windows-amd64.exe" ./cmd/server
echo "      完成！"

echo "[2/5] 编译 Windows arm64..."
GOOS=windows GOARCH=arm64 CGO_ENABLED=0 go build -ldflags="-s -w" -o "$DIST_DIR/closemask-windows-arm64.exe" ./cmd/server || echo "      [警告] 跳过"
echo "      完成！"

echo "[3/5] 编译 Linux amd64..."
GOOS=linux GOARCH=amd64 CGO_ENABLED=0 go build -ldflags="-s -w" -o "$DIST_DIR/closemask-linux-amd64" ./cmd/server
echo "      完成！"

echo "[4/5] 编译 Linux arm64..."
GOOS=linux GOARCH=arm64 CGO_ENABLED=0 go build -ldflags="-s -w" -o "$DIST_DIR/closemask-linux-arm64" ./cmd/server || echo "      [警告] 跳过"
echo "      完成！"

echo "[5/5] 编译 Darwin amd64..."
GOOS=darwin GOARCH=amd64 CGO_ENABLED=0 go build -ldflags="-s -w" -o "$DIST_DIR/closemask-darwin-amd64" ./cmd/server || echo "      [警告] 跳过"
echo "      完成！"

echo "[6/6] 编译 Darwin arm64..."
GOOS=darwin GOARCH=arm64 CGO_ENABLED=0 go build -ldflags="-s -w" -o "$DIST_DIR/closemask-darwin-arm64" ./cmd/server || echo "      [警告] 跳过"
echo "      完成！"

echo ""
echo "[信息] 打包发布版本..."

# 创建临时目录用于打包
TMP_DIR="tmp_package"
rm -rf "$TMP_DIR"
mkdir -p "$TMP_DIR"

# 复制文档和配置文件
cp README.md LICENSE config.json start.bat start.sh start_ner.bat stop_ner.bat "$TMP_DIR/" 2>/dev/null || true

# 打包各平台版本
# Windows amd64
cp "$DIST_DIR/closemask-windows-amd64.exe" "$TMP_DIR/closemask.exe"
cd "$TMP_DIR" && zip -r "../$RELEASE_DIR/closemask-$VERSION-windows-amd64.zip" . && cd ..
echo "      closemask-$VERSION-windows-amd64.zip"

# Windows arm64
if [ -f "$DIST_DIR/closemask-windows-arm64.exe" ]; then
    cp "$DIST_DIR/closemask-windows-arm64.exe" "$TMP_DIR/closemask.exe"
    cd "$TMP_DIR" && zip -r "../$RELEASE_DIR/closemask-$VERSION-windows-arm64.zip" . && cd ..
    echo "      closemask-$VERSION-windows-arm64.zip"
fi

# Linux amd64
cp "$DIST_DIR/closemask-linux-amd64" "$TMP_DIR/closemask"
chmod +x "$TMP_DIR/closemask"
cd "$TMP_DIR" && zip -r "../$RELEASE_DIR/closemask-$VERSION-linux-amd64.zip" . && cd ..
echo "      closemask-$VERSION-linux-amd64.zip"

# Linux arm64
if [ -f "$DIST_DIR/closemask-linux-arm64" ]; then
    cp "$DIST_DIR/closemask-linux-arm64" "$TMP_DIR/closemask"
    chmod +x "$TMP_DIR/closemask"
    cd "$TMP_DIR" && zip -r "../$RELEASE_DIR/closemask-$VERSION-linux-arm64.zip" . && cd ..
    echo "      closemask-$VERSION-linux-arm64.zip"
fi

# Darwin amd64
if [ -f "$DIST_DIR/closemask-darwin-amd64" ]; then
    cp "$DIST_DIR/closemask-darwin-amd64" "$TMP_DIR/closemask"
    chmod +x "$TMP_DIR/closemask"
    cd "$TMP_DIR" && zip -r "../$RELEASE_DIR/closemask-$VERSION-darwin-amd64.zip" . && cd ..
    echo "      closemask-$VERSION-darwin-amd64.zip"
fi

# Darwin arm64
if [ -f "$DIST_DIR/closemask-darwin-arm64" ]; then
    cp "$DIST_DIR/closemask-darwin-arm64" "$TMP_DIR/closemask"
    chmod +x "$TMP_DIR/closemask"
    cd "$TMP_DIR" && zip -r "../$RELEASE_DIR/closemask-$VERSION-darwin-arm64.zip" . && cd ..
    echo "      closemask-$VERSION-darwin-arm64.zip"
fi

# 清理临时目录
rm -rf "$TMP_DIR"

echo ""
echo "========================================"
echo "  打包完成！"
echo "========================================"
echo ""
echo "发布包位于: $RELEASE_DIR/"
ls -la "$RELEASE_DIR"/*.zip
echo ""
