#!/bin/bash
# CloseMask 完整环境一键启动
# 包含: AIFW Mock + Mock LLM + CloseMask

set -e

SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
cd "$SCRIPT_DIR"

echo "========================================"
echo "  CloseMask 完整环境一键启动"
echo "  包含: AIFW Mock + Mock LLM + CloseMask"
echo "========================================"
echo ""

# ============ 1. 检查 Python ============
if ! command -v python3 &> /dev/null; then
    if ! command -v python &> /dev/null; then
        echo "[错误] 未找到 Python，请先安装 Python 3.x"
        exit 1
    fi
    PYTHON=python
else
    PYTHON=python3
fi
echo "[OK] Python 已安装: $PYTHON"

# ============ 2. 安装 Flask ============
$PYTHON -c "import flask" 2>/dev/null || {
    echo "[信息] 安装 Flask..."
    $PYTHON -m pip install flask requests
}

# ============ 3. 杀掉残留进程 ============
echo "[信息] 清理残留进程..."
pkill -f "aifw_mock.py" 2>/dev/null || true
pkill -f "mock_llm.py" 2>/dev/null || true
lsof -ti:8845 | xargs kill 2>/dev/null || true
lsof -ti:11437 | xargs kill 2>/dev/null || true
lsof -ti:8846 | xargs kill 2>/dev/null || true

# ============ 4. 启动 AIFW Mock ============
echo "[信息] 启动 OneAIFW Mock 服务 (端口 8845)..."
$PYTHON ./scripts/aifw_mock.py &
AIFW_PID=$!

# 等待就绪
echo "[信息] 等待 AIFW 服务就绪..."
for i in $(seq 1 15); do
    if curl -s http://localhost:8845/api/health > /dev/null 2>&1; then
        echo "[OK] OneAIFW Mock 已就绪"
        break
    fi
    sleep 1
done

# ============ 5. 启动 Mock LLM ============
echo "[信息] 启动 Mock LLM 服务 (端口 11437)..."
$PYTHON ./scripts/mock_llm.py &
LLM_PID=$!

# 等待就绪
echo "[信息] 等待 Mock LLM 服务就绪..."
for i in $(seq 1 15); do
    if curl -s http://localhost:11437/health > /dev/null 2>&1; then
        echo "[OK] Mock LLM 已就绪"
        break
    fi
    sleep 1
done

# ============ 6. 创建数据目录 ============
mkdir -p ./data

# ============ 7. 启动 CloseMask ============
echo ""
echo "========================================"
echo "  所有服务已启动！"
echo "========================================"
echo ""
echo "  OneAIFW Mock:  http://localhost:8845"
echo "  Mock LLM:      http://localhost:11437"
echo "  CloseMask:     http://localhost:8846"
echo ""
echo "  按 Ctrl+C 停止所有服务"
echo "========================================"
echo ""

# 退出时清理
cleanup() {
    echo ""
    echo "[信息] 正在停止所有服务..."
    kill $AIFW_PID 2>/dev/null || true
    kill $LLM_PID 2>/dev/null || true
    echo "[信息] 所有服务已停止"
}
trap cleanup EXIT INT TERM

./closemask -config ./config.json
