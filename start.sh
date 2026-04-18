#!/bin/bash
# CloseMask - PII 保护代理一键启动脚本 v2.3

set -e

SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
cd "$SCRIPT_DIR"

echo "========================================"
echo "  CloseMask - PII 保护代理"
echo "  一键启动脚本 v2.3"
echo "========================================"
echo ""

# 检查 closemask 可执行文件
if [ ! -f "./closemask" ]; then
    echo "[错误] 未找到 closemask 可执行文件"
    echo "[提示] 请先运行 ./build.sh 编译"
    exit 1
fi

# 检查配置文件
if [ ! -f "./config.json" ]; then
    echo "[警告] 未找到 config.json，将使用默认配置"
fi

# 创建数据目录
mkdir -p ./data

echo "[信息] 正在启动 CloseMask..."
echo "[信息] 默认端口: 8846"
echo "[信息] 按 Ctrl+C 停止服务"
echo ""

./closemask -config ./config.json "$@"
