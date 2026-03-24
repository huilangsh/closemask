# 辅助脚本

本目录包含用于本地开发和测试的辅助脚本。

## 脚本说明

### `mock_llm.py`

模拟 LLM 服务，用于本地测试 CloseMask 代理，无需连接真实 LLM。

- **端口**：`11437`
- **功能**：
  - 非流式和流式（SSE）响应
  - 工具调用模拟
  - 简单的对话能力

```bash
pip install flask requests
python scripts/mock_llm.py
```

### `aifw_mock.py`

OneAIFW 的本地模拟版，用于在没有完整 OneAIFW 环境时进行测试。

- **端口**：`8845`
- **API 端点**：
  - `POST /api/mask` - 遮罩 PII
  - `POST /api/restore` - 还原 PII
  - `POST /api/call` - 遮罩调用
  - `GET /api/config` - 获取配置
  - `GET /api/health` - 健康检查

```bash
pip install flask requests
python scripts/aifw_mock.py
```

## 使用场景

这些脚本用于**本地开发和测试**。生产环境请使用：

- **OneAIFW**：https://github.com/funstory-ai/aifw
- **真实 LLM**：OpenAI、Anthropic 等提供商
