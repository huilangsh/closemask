#!/bin/bash
# CloseMask - 编译脚本 v0.9.3

set -e

SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
cd "$SCRIPT_DIR"

echo "========================================"
echo "  CloseMask - 编译脚本 v0.9.3"
echo "========================================"
echo ""

# 检查 Go 环境
if ! command -v go &> /dev/null; then
    echo "[错误] 未找到 Go 环境，请先安装 Go 1.21+"
    echo "[下载] https://go.dev/dl/"
    exit 1
fi

echo "[信息] Go 版本:"
go version
echo ""

echo "[信息] 正在编译 CloseMask..."
go build -o ./closemask ./cmd/server

echo "[信息] 编译成功！"
echo "[信息] 运行 ./start.sh 启动服务，或直接运行 ./closemask"
echo ""

# 运行 vet 检查
echo "[信息] 运行 go vet 检查..."
go vet ./... && echo "[信息] go vet 检查通过" || echo "[警告] go vet 发现问题"

# 运行测试
echo ""
echo "[信息] 运行单元测试..."
go test ./... && echo "[信息] 所有测试通过" || echo "[警告] 部分测试未通过"
