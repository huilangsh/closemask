# CloseMask

[English](#english) | [中文](#chinese)

---

## English

CloseMask is a lightweight middleware proxy that sits between your AI agents and third-party LLM API services, automatically masking Personally Identifiable Information (PII) and technical credentials before data leaves your infrastructure — ensuring privacy compliance while maintaining conversational continuity.

**Why CloseMask?** With the proliferation of token-based API proxies and relay services — many of which operate in regulatory gray areas — your sensitive data is at risk every time you call an LLM API. CloseMask acts as a privacy shield: it intercepts outgoing requests, masks all PII and credentials with deterministic placeholders, forwards the sanitized data to the LLM, then transparently restores original values in responses. Your users and agents never notice the difference, but your sensitive data never reaches third-party services.

**Key Features:**
- **Zero-dependency**: Built-in credential masking + PII detection, works out of the box
- **3-tier detection**: Built-in regex → Built-in PII → OneAIFW (21+ PII types)
- **Auto-discovery**: OneAIFW detected and auto-started if present in same directory
- **Multi-provider**: OpenAI, Anthropic (via OpenAI-compatible proxy), Azure, Ollama, etc.
- SSE streaming & tool call support
- Layered storage (Memory + Disk + Redis)

**📖 [Full Documentation (English)](./docs-en/DESIGN.md)**

## 中文

CloseMask 是一个轻量级中间件代理，部署在你的 AI Agent 和第三方 LLM API 服务之间，在数据离开你的基础设施之前自动遮罩个人身份信息（PII）和技术凭证——确保隐私合规的同时保持对话连续性。

**为什么需要 CloseMask？** 随着 Token 搬运代理服务的泛滥，许多中转 API 存在合规风险，每次调用 LLM API 都可能泄露你的敏感数据。CloseMask 就像一面隐私防护盾：拦截发出的请求，用确定性占位符替换所有 PII 和凭据，将脱敏后的数据转发给 LLM，然后在响应中透明地还原原始值。你的用户和 Agent 完全感知不到差异，但敏感数据永远不会泄露给第三方服务。
【其实就是最近看中转站的太多了，中转确实可以截获数据】
**核心特性：**
- **开箱即用**：内置凭据遮罩 + PII 检测，无需外部依赖
- **三档检测**：内置正则 → 内置 PII → OneAIFW（21+ PII 类型）
- **自动发现**：同目录下的 OneAIFW 自动检测和启动
- **多提供商**：OpenAI、Anthropic（通过 OpenAI 兼容代理）、Azure、Ollama 等
- SSE 流式响应 & Agent 工具调用支持
- 分层存储（内存 + 磁盘 + Redis）

**📖 [完整文档（中文）](./docs-cn/README.md)**

---

## 快速开始

### 最简方式（零依赖）

```bash
# 1. 创建 config.json（只需填 llm_url）
# 2. 运行

# Windows
closemask.exe -config config.json

# Linux/Mac
./closemask -config config.json
```

最小配置：
```json
{
  "llm_url": "http://localhost:11434/v1/chat/completions"
}
```

## 工作原理示例

### 示例 1：用户对话中的 PII 遮罩

**用户输入：**
```
我的身份证号是 110101199003077777，手机号 13812345678，帮我查一下订单
```

**CloseMask 遮罩后发送给 LLM：**
```
我的身份证号是 ${ID_CARD_a1b2c3}，手机号 ${PHONE_d4e5f6}，帮我查一下订单
```

**LLM 响应：**
```
好的，已为您查询到 ${ID_CARD_a1b2c3} 的订单信息...
```

**CloseMask 还原后返回给用户：**
```
好的，已为您查询到 110101199003077777 的订单信息...
```

### 示例 2：开发者调试时的凭据遮罩

**开发者输入：**
```
帮我看看这段代码有什么问题：
OPENAI_API_KEY=sk-proj-abcdefghijklmnopqrstuvwx123456
DATABASE_URL=postgres://admin:secret123@db.example.com:5432/mydb
```

**CloseMask 遮罩后：**
```
帮我看看这段代码有什么问题：
OPENAI_API_KEY=${CRED_x7y8z9}
DATABASE_URL=postgres://admin:${CRED_m1n2o3}@db.example.com:5432/mydb
```

**LLM 只看到占位符，真实凭据不会泄露。**

### 示例 3：Agent 工具调用

**用户：** 搜索身份证号 110101199003077777 的用户

**CloseMask 遮罩后 LLM 生成工具调用：**
```json
{
  "function": "search_user",
  "arguments": {"id_card": "${ID_CARD_a1b2c3}"}
}
```

**CloseMask 还原后实际执行：**
```json
{
  "function": "search_user",
  "arguments": {"id_card": "110101199003077777"}
}
```

工具正常工作，数据库查询成功，全程 LLM 未接触真实 PII。

---

### 完整环境（含 Mock 服务测试）

```bash
# Windows
start_all.bat

# Linux/Mac
chmod +x start_all.sh && ./start_all.sh
```

### 从源码编译

```bash
git clone https://github.com/huilangsh/closemask.git
cd closemask

# Windows
build.bat

# Linux/Mac
chmod +x build.sh && ./build.sh
```

## 配置

编辑 `config.json`：

```json
{
  "llm_url": "http://localhost:11434",
  "oneaifw_url": "http://localhost:8845",
  "port": 8846,
  "storage_type": "layered",
  "local_mask_level": "strict",
  "mask_fail_strategy": "block",
  "max_placeholders_per_session": 500,
  "pii_engine": "auto",
  "log_to_file": false
}
```

### LLM 提供商兼容性

CloseMask 采用 OpenAI 兼容协议，支持所有 OpenAI 兼容的 LLM 提供商。**`llm_url` 必须填完整端点地址**：

| 提供商 | `llm_url` 配置 |
|--------|---------------|
| **OpenAI** | `https://api.openai.com/v1/chat/completions` |
| **Ollama** | `http://localhost:11434/v1/chat/completions` |
| **Groq** | `https://api.groq.com/openai/v1/chat/completions` |
| **DeepSeek** | `https://api.deepseek.com/chat/completions` |
| **通义千问** | `https://dashscope.aliyuncs.com/compatible-mode/v1/chat/completions` |
| **Moonshot Kimi** | `https://api.moonshot.cn/v1/chat/completions` |
| **智谱 GLM** | `https://open.bigmodel.cn/api/coding/paas/v4/chat/completions` |
| **Azure OpenAI** | `https://{resource}.openai.azure.com/openai/deployments/{deployment}/chat/completions?api-version=2024-10-21` |
| **Anthropic Claude** | 通过 OpenAI 兼容代理（见下方说明） |
| **其他** | 查阅官方文档，填完整端点地址 |

#### 使用 Anthropic Claude

> ⚠️ **注意**：CloseMask 当前**不支持** Anthropic 原生 API（`/v1/messages`），仅支持 OpenAI 兼容协议（`/v1/chat/completions`）。使用 Anthropic 需要通过以下代理层进行协议转换。

Anthropic 原生 API（`/v1/messages`）与 OpenAI 格式不同，CloseMask 需要通过 OpenAI 兼容代理层来对接：

**方案 1：使用 one-api / new-api（推荐）**

```bash
# 部署 one-api
docker run -d --name one-api -p 3000:3000 justsong/one-api

# 在 one-api 中添加 Anthropic 渠道，然后在 CloseMask 中配置：
```

```json
{
  "llm_url": "http://localhost:3000/v1"
}
```

**方案 2：使用 LiteLLM**

```bash
# 安装 LiteLLM
pip install litellm[proxy]

# 启动代理
litellm --model claude-3-5-sonnet-20241022 --port 4000
```

```json
{
  "llm_url": "http://localhost:4000/v1"
}
```

**方案 3：使用 OpenRouter**

```json
{
  "llm_url": "https://openrouter.ai/api/v1"
}
```

请求时通过 `Authorization` 头传递对应平台的 API Key，CloseMask 会自动透传给上游。

| 配置项 | 默认值 | 说明 |
|--------|--------|------|
| `pii_engine` | `auto` | PII 引擎：auto（自动选择）/ builtin（仅内置）/ oneaifw（仅 OneAIFW） |
| `oneaifw_url` | `http://localhost:8845` | OneAIFW 地址（可选，不配置则使用内置检测） |
| `placeholder_hash_length` | `6` | 占位符哈希长度（6 或 8） |
| `placeholder_hmac_key` | 空 | HMAC 密钥（空则用 plain sha256） |
| `log_level` | `info` | 日志级别：quiet / info / debug |
| `log_to_file` | `false` | 是否将日志写入文件（默认仅输出终端） |

## 许可证

MIT License - 免费商业使用

详见 [LICENSE](./LICENSE)。
