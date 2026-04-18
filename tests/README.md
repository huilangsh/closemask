# CloseMask 测试套件

本目录包含 CloseMask 的全量模拟测试脚本和测试数据。

## 目录结构

```
tests/
├── test_comprehensive_scenario.py    # 综合测试场景（多轮对话、工具调用、流式响应）
├── test_enterprise_scenarios.py      # 企业敏感数据测试
├── test_real_conversation.py         # 真实对话场景测试
├── test_all_scenarios.py             # 全量场景自动化测试
└── scenarios/                        # 测试数据
    ├── test_enterprise_data_scenarios.txt  # 企业数据场景（138个 PII 实体）
    ├── test_hidden_pii_scenarios.txt        # 隐藏 PII 场景（19个 PII 实体）
    └── test_real_conversation.txt           # 真实对话数据（49个 PII 实体）
```

## 前置条件

1. **CloseMask 代理** 运行在 `localhost:8846`
2. **OneAIFW 服务** 运行在 `localhost:8845`
3. **Mock LLM** 运行在 `localhost:11437`（可选）

### 快速启动测试环境

```bash
# 1. 启动 Mock LLM（如果不用真实 LLM）
pip install flask requests
python scripts/mock_llm.py

# 2. 启动 OneAIFW 本地模拟版
python scripts/aifw_mock.py

# 3. 启动 CloseMask
go run ./cmd/server/main.go

# 4. 运行测试
cd tests
python test_all_scenarios.py
```

## 测试说明

### 综合测试 (`test_comprehensive_scenario.py`)

模拟真实多轮对话场景：
- 长输入接近上下文窗口限制
- 多轮对话历史和占位符持久化
- 工具调用（含 accessToken 等敏感信息）
- 多种 PII 类型遮罩和还原
- 流式和非流式混合请求

### 企业数据测试 (`test_enterprise_scenarios.py`)

读取场景文件逐条测试：
- 企业敏感数据遮罩（API 密钥、数据库连接串等）
- 隐藏 PII 场景（编码后、URL 内嵌的 PII）

### 真实对话测试 (`test_real_conversation.py`)

基于真实客户服务对话数据：
- 模拟客户咨询、密码重置、订单查询等场景
- 验证遮罩和还原的准确性

### 全量测试 (`test_all_scenarios.py`)

自动化运行所有场景测试，汇总结果。

## 测试结果

详见 [docs-en/TEST_REPORT.md](../docs-en/TEST_REPORT.md) 或 [docs-cn/TEST_REPORT.md](../docs-cn/TEST_REPORT.md)。

### 自动化测试通过率（2026-03-23）

| 测试类别 | 每轮用例数 | 状态 |
|----------|-----------|------|
| AIFW 遮罩/还原 | 12 | 全部通过 |
| 代理非流式 PII 处理 | 6 | 全部通过 |
| 代理流式 PII 处理 | 6 | 全部通过 |
| 非 PII 文本保护 | 6 | 全部通过 |
| 混合 PII 类型 | 4 | 全部通过 |
| 工具调用（流式） | 3 | 2 通过，1 预期失败 |
| 工具调用（非流式） | 2 | 全部通过 |
| **合计** | **39/轮** | **38 通过，1 预期失败** |

> 两轮测试（服务重启后）结果完全一致。唯一"失败"项 `has_tool_calls` 是设计预期行为。
