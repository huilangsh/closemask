# CloseMask

一个生产级的 AI Agent 中间件代理，用于在将数据发送给 LLM 之前自动遮罩个人身份信息（PII）和技术凭证，确保隐私合规的同时保持对话连续性。

## 📋 目录

- [概述](#概述)
- [核心功能](#核心功能)
- [架构设计](#架构设计)
- [快速开始](#快速开始)
- [配置说明](#配置说明)
- [API 文档](#api-文档)
- [使用场景](#使用场景)
- [性能指标](#性能指标)
- [依赖项](#依赖项)
- [测试](#测试)
- [部署](#部署)
- [许可证](#许可证)

## 概述

CloseMask 是一个基于 Go 语言的轻量级中间件，部署在 AI Agent 和第三方 LLM API 服务之间。它拦截请求，使用三档检测引擎遮罩敏感信息，将遮罩后的数据转发给 LLM，并在响应中还原原始值——所有操作对终端用户都是透明的。

### 为什么需要 CloseMask？

当你使用第三方 LLM API（OpenAI、Anthropic、Azure 等）时，每一条请求都可能将敏感数据暴露给外部服务器：

- **API Key 泄露**：开发者在调试时不小心把 `sk-proj-...` 发给 LLM
- **用户隐私暴露**：客服对话中的手机号、身份证号被发送到第三方
- **企业数据外泄**：员工向 LLM 粘贴了包含数据库密码的配置信息
- **合规风险**：PII 数据跨境传输违反 GDPR、PIPL 等法规

CloseMask 就是为此而生——在你和第三方 API 之间建立一道防护屏障。它自动检测并遮罩所有敏感数据，用占位符替代后转发给 LLM，LLM 的响应中包含占位符时自动还原为原始值。你的 Agent 和用户完全感知不到这个过程，但你的数据永远不会以明文形式离开你的基础设施。

### 为什么选择 CloseMask？

- **开箱即用**：单个二进制文件即可运行，内置凭据遮罩 + PII 检测，无需外部依赖
- **隐私合规**：自动遮罩 PII 和技术凭证，避免敏感数据发送给第三方 LLM
- **三档架构**：内置检测 → OneAIFW 增强，按需选择检测能力
- **Agent 原生**：支持工具调用、流式响应和多轮对话
- **多提供商**：支持 OpenAI、Anthropic（通过兼容代理）、Azure、Ollama、DeepSeek 等
- **高性能**：亚微秒级会话操作，10-50ms 请求处理
- **企业级**：零配置部署，全面监控，MIT 许可证

## 核心功能

### 🔒 三档检测架构

CloseMask 提供三级 PII 检测能力，用户按需选择：

| 档位 | 引擎 | 检测能力 | 需要什么 |
|------|------|----------|----------|
| **Lite** | 内置正则 + 内置 PII | API Key/JWT/手机号/身份证/邮箱/银行卡/IP | **零依赖，开箱即用** |
| **Full** | 上面 + OneAIFW | 人名/组织名/地址 + 上述全部 | 部署 OneAIFW |
| **Auto**（默认） | 自动选择 | 有 OneAIFW 用 OneAIFW，没有用内置 | 自动检测 |

**配置方式**：
```json
{
  "pii_engine": "auto"    // "auto" | "builtin" | "oneaifw"
}
```

#### 第一档：内置检测（零依赖，开箱即用）

**LocalMasker（凭据遮罩）**：

键名优先匹配（环境变量名触发遮罩）：
- `OPENAI_API_KEY=sk-proj-...` → `OPENAI_API_KEY=${CRED_a1b2c3}`
- `DASHSCOPE_API_KEY=sk-dashscope-...` → `DASHSCOPE_API_KEY=${CRED_d4e5f6}`
- `DATABASE_URL=postgres://admin:secret@db` → `DATABASE_URL=postgres://admin:${CRED_f7e8d9}@db`
- `ZHIPU_API_KEY=...`、`DEEPSEEK_API_KEY=...` 等

值格式匹配（检测值本身的特征）：
- Bearer JWT：`Bearer eyJhbG...`
- AWS Access Key：`AKIA...`

**BuiltInPIIDetector（PII 检测）**：
- 手机号（中国 1[3-9]开头的 11 位号码）
- 身份证号（18 位中国居民身份证）
- 邮箱地址
- 银行卡号（16-19 位）
- IPv4 地址

#### 第二档：OneAIFW 增强（21+ PII 类型）

检测并遮罩个人身份信息：

**个人信息**
- 身份证（中国居民身份证、SSN 等）
- 手机号（移动、固话、国际号码）
- 邮箱地址
- 物理地址
- 姓名

**金融数据**
- 银行卡号（信用卡/借记卡）
- 支付信息
- 交易金额
- 金融凭证

**其他敏感数据**
- 验证码
- 密码
- 验证令牌

### 🛡️ 遮罩失败策略（mask_fail_strategy）

当 OneAIFW 不可用时，内置 PII 检测自动兜底。可配置处理策略：

| 策略 | 行为 | 适用场景 |
|------|------|----------|
| `pass` | 原始内容透传给 LLM | 可用性优先（默认），允许短暂风险 |
| `block` | 仅在所有引擎都不可用时拒绝请求 | 安全优先，内置检测兜底后基本不会触发 |
| `redact` | 将检测到的凭证替换为 `[REDACTED]` | 平衡策略 |

> **注意**：V2.2 起内置 PII 检测器始终生效，OneAIFW 不可用时自动降级到内置检测。`block` 策略仅在所有检测引擎均不可用时才拒绝请求。

### 🤖 Agent 原生架构

- **SSE 流式支持**：实时流式响应，带 PII 保护
- **工具调用支持**：遮罩 Agent 工具调用中的参数，还原工具结果
- **多轮持久化**：跨对话历史的一致占位符
- **会话管理**：零延迟会话操作（<1μs）
- **FIFO 淘汰**：占位符超过限制时自动淘汰最早的映射

### 💾 分层存储系统

| 存储类型 | 说明 | 适用场景 |
|----------|------|----------|
| `memory` | 纯内存存储 | 开发测试，低并发 |
| `disk` | 纯磁盘持久化 | 单机部署，需要持久化 |
| `layered` | 内存热数据 + 磁盘冷数据 | **生产推荐**，兼顾性能与持久化 |
| `redis` | Redis 分布式存储 | 高并发，多实例部署 |

**Layered 存储特性**：
- 读操作优先从内存读取（热数据），miss 时回填
- 写操作同步写内存 + 异步写磁盘
- 后台定期清理过期磁盘文件
- 重启后自动从磁盘恢复数据

### 🔄 还原机制

- **确定性占位符**：基于 PII 值哈希生成占位符（`${TYPE_hash}` 格式），同一值永远生成同一占位符
- **精确还原**：通过占位符映射表 `${TYPE_hash}` → 原值，精确替换
- **子串还原**：支持 URL 等嵌入占位符场景（`RestoreArgs`）
- **降级处理**：未找到映射时返回 `[PII-UNRECOVERABLE]`，不泄漏占位符
- **向后兼容**：支持旧格式 `${CRED_N}` 的还原和 fuzzy 匹配

## 架构设计

```
┌─────────────┐     HTTP 请求      ┌─────────────┐
│   Agent     │ ───────────────────▶ │   Proxy     │
│ Application │                     │  (端口 8846) │
└─────────────┘                     └──────┬──────┘
                                           │
                                           │ 1. LocalMasker 本地遮罩
                                           │    (API Key, JWT, AWS Key)
                                           │
                                           │ 2. BuiltInPII 内置检测
                                           │    (手机号, 身份证, 邮箱, 银行卡)
                                           │
                                           │ 3. OneAIFW 增强检测（可选）
                                           │    (人名, 组织, 地址, 21+ PII)
                                           ▼
                                   ┌─────────────┐
                                   │   OneAIFW   │
                                   │  (端口 8845) │
                                   └──────┬──────┘
                                          已遮罩
                                          数据
                                           │
                                           ▼
                                   ┌─────────────┐
                                   │     LLM     │
                                   │  提供商     │
                                   └──────┬──────┘
                                           │
                                           │ 响应
                                           │
                                           ▼
                                   ┌─────────────┐
                                   │   Proxy     │
                                   │             │
                                   │ 4. 还原 PII  │
                                   └──────┬──────┘
                                           │
                                           │ 已还原
                                           │ 响应
                                           ▼
                                   ┌─────────────┐
                                   │   Agent     │
                                   │ Application │
                                   └─────────────┘
```

### 关键组件

1. **HTTP 服务器**：OpenAI 兼容的 API 端点
2. **LocalMasker**：本地正则遮罩，键名优先匹配 + 值格式匹配
3. **PII 检测器**：集成 OneAIFW 进行 PII 检测
4. **占位符管理器**：跨请求管理 PII 占位符（FIFO 淘汰）
5. **工具调用处理器**：遮罩/还原工具调用参数
6. **流式处理器**：处理带 PII 还原的 SSE 流式传输
7. **会话存储**：支持 Memory / Disk / Layered / Redis

### 占位符格式

V0.9.1 起使用确定性哈希格式：

```
${TYPE_hash}
```

- `TYPE` 为 PII 类型（CRED/PHONE/ID_CARD/EMAIL/BANK_CARD/IP_ADDRESS）
- `hash` 为 sha256/hmac-sha256 的前 6-8 位 hex
- 示例：`${CRED_a1b2c3}`、`${PHONE_d4e5f6}`、`${EMAIL_f7e8d9}`
- **同一值永远生成同一占位符**，不依赖 session 和跨轮映射

> V2 使用 `${CRED_N}` 格式（递增数字），V0.9.1 起改为确定性哈希格式，同时保留旧格式向后兼容。

## 快速开始

### 最简方式（零依赖，开箱即用）

```bash
# 1. 下载 closemask.exe
# 2. 创建 config.json（只需填 llm_url）
# 3. 运行

# Windows
closemask.exe -config config.json

# Linux/Mac
./closemask -config config.json
```

最小配置 `config.json`：
```json
{
  "llm_url": "http://localhost:11434"
}
```

启动后 CloseMask 自动显示检测能力：
```
╔══════════════════════════════════════════════════════╗
║  CloseMask - AI Agent PII Middleware                 ║
╠══════════════════════════════════════════════════════╣
║  PII 检测引擎:                                       ║
║  ✅ local_masker                                     ║
║  ✅ builtin_pii                                      ║
║  ❌ oneaifw       ← 服务不可达且无法自动启动          ║
║                                                      ║
║  当前检测能力: ██████░░░░ 66%                        ║
║  💡 配置 OneAIFW 可获得人名/组织/地址检测能力         ║
╚══════════════════════════════════════════════════════╝
```

### 增强方式（+ OneAIFW）

部署 OneAIFW 后，CloseMask 自动检测并启用完整检测能力：

```bash
# 克隆 OneAIFW
git clone https://github.com/funstory-ai/aifw.git
cd aifw/py-origin

# 安装依赖
pip install -r services/requirements.txt -r cli/requirements.txt

# 启动服务
python -m aifw launch
```

然后在 `config.json` 中配置：
```json
{
  "llm_url": "http://localhost:11434",
  "oneaifw_url": "http://localhost:8845",
  "pii_engine": "auto"
}
```

### 一键启动（预编译版本，含 Mock 服务）

```bash
# Windows — 包含 AIFW Mock + Mock LLM + CloseMask
start_all.bat

# Linux/Mac
chmod +x start_all.sh && ./start_all.sh
```

### 从源码编译

```bash
git clone https://github.com/huilangsh/closemask.git
cd closemask
go build -o closemask ./cmd/server
```

### 启动 CloseMask

```bash
# 使用默认配置运行
./closemask

# 或指定自定义配置
./closemask -config config.json
```

### 发起第一个请求

```bash
curl -X POST http://localhost:8846/v1/chat/completions \
  -H "Content-Type: application/json" \
  -d '{
    "model": "gpt-3.5-turbo",
    "messages": [
      {
        "role": "user",
        "content": "我的 OPENAI_API_KEY=sk-proj-abc123def4567890"
      }
    ]
  }'
```

在后台，API Key 在发送给 LLM 之前已被本地遮罩为 `${CRED_a1b2c3}`，然后在响应中被还原。

## 配置说明

创建 `config.json` 文件：

```json
{
  "oneaifw_url": "http://localhost:8845",
  "llm_url": "http://localhost:11434",
  "port": 8846,
  "storage_type": "layered",
  "session_ttl": "24h",
  "message_ttl": "24h",
  "max_messages_per_session": 100,
  "data_dir": "./data",
  "mask_fail_strategy": "block",
  "max_placeholders_per_session": 500,
  "local_mask_level": "strict",
  "pii_engine": "auto",
  "log_to_file": false
}
```

### 配置项说明

| 配置项 | 默认值 | 说明 |
|--------|--------|------|
| `llm_url` | `http://localhost:11434` | LLM 提供商地址（**必填**） |
| `port` | `8846` | 代理监听端口 |
| `api_key` | 空 | CloseMask 自身认证密钥（用 `X-CloseMask-Key` 头传递）。**LLM 的 API Key 不用填这里**，直接在请求中用 `Authorization` 头传递，CloseMask 会自动透传给 LLM |
| `storage_type` | `layered` | 存储类型：memory/disk/layered/redis |
| `session_ttl` | `24h` | 会话过期时间 |
| `message_ttl` | `24h` | 消息过期时间 |
| `max_messages_per_session` | `100` | 单会话最大消息数 |
| `data_dir` | `./data` | 磁盘存储数据目录 |
| `mask_fail_strategy` | `pass` | 遮罩失败策略：pass（放行）/block（拒绝）/redact（替换） |
| `max_placeholders_per_session` | `500` | 单会话最大占位符数（FIFO 淘汰） |
| `local_mask_level` | `aggressive` | 本地遮罩级别：off/strict/aggressive |
| `pii_engine` | `auto` | PII 引擎选择：auto/builtin/oneaifw |
| `log_to_file` | `false` | 是否将日志写入文件 `./logs/closemask.log`（默认仅输出到终端） |
| `placeholder_hash_length` | `6` | 占位符哈希长度（6 或 8） |
| `placeholder_hmac_key` | 空 | HMAC 密钥（空则用 plain sha256） |
| `log_level` | `info` | 日志级别：quiet（仅错误）/ info（默认）/ debug（全部） |

### 环境变量

- `CLOSEMASK_CONFIG`：配置文件路径
- `CLOSEMASK_LLM_URL`：LLM 提供商基础 URL
- `CLOSEMASK_PORT`：代理端口
- `CLOSEMASK_API_KEY`：CloseMask 认证密钥（用 `X-CloseMask-Key` 头传递）
- `CLOSEMASK_DATA_DIR`：数据存储目录
- `CLOSEMASK_LOCAL_MASK_LEVEL`：本地遮罩级别
- `CLOSEMASK_MASK_FAIL_STRATEGY`：遮罩失败策略
- `CLOSEMASK_PII_ENGINE`：PII 引擎选择
- `CLOSEMASK_PLACEHOLDER_HASH_LENGTH`：占位符哈希长度（6或8）
- `CLOSEMASK_PLACEHOLDER_HMAC_KEY`：HMAC 密钥
- `CLOSEMASK_LOG_LEVEL`：日志级别（quiet/info/debug）

## API 文档

### LLM 提供商兼容性

CloseMask 采用 **OpenAI 兼容协议**，支持所有 OpenAI 兼容的 LLM 提供商。**`llm_url` 必须填完整端点地址**：

| 提供商 | `llm_url` 配置 |
|--------|---------------|
| **OpenAI** | `https://api.openai.com/v1/chat/completions` |
| **Ollama** | `http://localhost:11434/v1/chat/completions` |
| **Groq** | `https://api.groq.com/openai/v1/chat/completions` |
| **DeepSeek** | `https://api.deepseek.com/chat/completions` |
| **通义千问** | `https://dashscope.aliyuncs.com/compatible-mode/v1/chat/completions` |
| **Moonshot Kimi** | `https://api.moonshot.cn/v1/chat/completions` |
| **智谱 GLM** | `https://open.bigmodel.cn/api/coding/paas/v4/chat/completions` |
| **Azure OpenAI** | 见下方说明 |
| **Anthropic Claude** | 通过 OpenAI 兼容代理（见下方说明） |
| **其他** | 查阅官方文档，填完整端点地址 |

#### 使用 Azure OpenAI

Azure OpenAI 的 URL 格式特殊，需要填完整路径：

```
https://{your-resource}.openai.azure.com/openai/deployments/{deployment-id}/chat/completions?api-version=2024-10-21
```

配置示例：

```json
{
  "llm_url": "https://my-resource.openai.azure.com/openai/deployments/gpt-4/chat/completions?api-version=2024-10-21"
}
```

> ⚠️ Azure 使用 `api-key` 请求头而非 `Authorization: Bearer`，需确保你的客户端正确传递。

#### 使用 Anthropic Claude

> ⚠️ **注意**：CloseMask 当前**不支持** Anthropic 原生 API（`/v1/messages`），仅支持 OpenAI 兼容协议（`/v1/chat/completions`）。使用 Anthropic 需要通过以下代理层进行协议转换。

Anthropic 原生 API（`/v1/messages`）与 OpenAI 格式不同，CloseMask 需要通过 **OpenAI 兼容代理层**来对接。有以下几种方案：

**方案 1：使用 one-api / new-api（推荐）**

[one-api](https://github.com/songquanpeng/one-api) 是一个 OpenAI API 管理和分发系统，支持 Anthropic 等多种提供商：

```bash
# 部署 one-api
docker run -d --name one-api -p 3000:3000 justsong/one-api

# 在 one-api 管理界面添加 Anthropic 渠道和 API Key
# 然后在 CloseMask 中配置：
```

```json
{
  "llm_url": "http://localhost:3000/v1"
}
```

**方案 2：使用 LiteLLM**

[LiteLLM](https://github.com/BerriAI/litellm) 将各种 LLM API 统一为 OpenAI 格式：

```bash
# 安装 LiteLLM
pip install litellm[proxy]

# 启动代理（以 Claude 为例）
litellm --model claude-3-5-sonnet-20241022 --port 4000

# 配置 CloseMask
```

```json
{
  "llm_url": "http://localhost:4000/v1"
}
```

**方案 3：使用 OpenRouter**

[OpenRouter](https://openrouter.ai/) 提供统一的 OpenAI 兼容 API，支持 Anthropic 等多家提供商：

```json
{
  "llm_url": "https://openrouter.ai/api/v1"
}
```

请求时通过 `Authorization` 头传递对应平台的 API Key，CloseMask 会自动透传给上游 LLM。

> **注意**：无论使用哪种代理方案，CloseMask 对 Anthropic 的 PII 遮罩和还原功能完全一致，代理层仅负责协议转换。

### OpenAI 兼容端点

#### 聊天完成（非流式）

```
POST /v1/chat/completions
```

请求体遵循 OpenAI 格式：
```json
{
  "model": "gpt-3.5-turbo",
  "messages": [
    {"role": "system", "content": "你是一个有用的助手。"},
    {"role": "user", "content": "包含 PII 的用户消息"}
  ],
  "temperature": 0.7,
  "max_tokens": 1000
}
```

#### 聊天完成（流式）

```
POST /v1/chat/completions
```

在请求体中添加 `"stream": true` 以启用 SSE 流式传输。

响应格式遵循 OpenAI SSE 协议，并在每个数据块中还原 PII。

#### 工具调用

完全支持。工具调用参数在发送给 LLM 之前被遮罩，工具结果自动取消遮罩。

```json
{
  "model": "gpt-3.5-turbo",
  "messages": [
    {"role": "user", "content": "搜索 John Doe"}
  ],
  "tools": [
    {
      "type": "function",
      "function": {
        "name": "search",
        "description": "搜索用户",
        "parameters": {
          "type": "object",
          "properties": {
            "query": {"type": "string"}
          }
        }
      }
    }
  ]
}
```

### 健康检查

```
GET /health
```

响应（纯文本）：
```
OK
```

如果 AIFW 或 LLM 不可达，返回 `503 Service Unavailable` 及状态信息。

## 使用场景

### 1. 客户服务 Agent

```
客户："我的身份证号是 110101199003077777，帮我重置密码"
→ 代理遮罩身份证 → LLM 处理 → 代理还原响应
```

### 2. 开发助手

```
开发者："我的 OPENAI_API_KEY=sk-proj-abc123"
→ LocalMasker 遮罩 → LLM 处理 → 代理还原响应
```

### 3. 金融助手

```
用户："转账 5000 元到账户 6222000012345678"
→ 代理遮罩账户号 → LLM 验证 → 代理还原
```

### 4. 企业系统

```
员工："我的 DATABASE_URL=postgres://admin:secret@db:5432/mydb"
→ LocalMasker 遮罩密码 → LLM 处理 → 代理还原
```

## 性能指标

### 基准测试

| 指标 | 数值 |
|------|------|
| 会话操作 | <1μs |
| 本地遮罩 | <1ms |
| OneAIFW 遮罩 | 10-50ms |
| PII 还原 | 5-20ms |
| 端到端延迟 | 50-150ms |
| 吞吐量 | 1000+ 请求/秒 |
| 内存占用 | <50MB |

## 依赖项

### 核心依赖（零外部依赖即可运行）

- Go 1.21+
- CloseMask 内置 LocalMasker + BuiltInPIIDetector，**无需任何外部服务即可工作**

### 可选服务

1. **OneAIFW** - PII 检测增强引擎
   - 许可证：MIT
   - 仓库：https://github.com/funstory-ai/aifw
   - 默认端口：8845
   - 部署方式：Python 服务 / PyInstaller 打包的 exe
   - 自动发现：同目录下的 `oneaifw.exe` 或 `oneaifw/aifw_service.py` 会自动启动

2. **LLM 提供商** - 任何 OpenAI 兼容的提供商
   - OpenAI、Azure OpenAI、Ollama、Groq、DeepSeek 等
   - Anthropic Claude 通过 OpenAI 兼容代理（one-api/LiteLLM/OpenRouter）支持

> **开箱即用**：CloseMask 无需任何外部依赖即可遮罩 API Key、手机号、身份证、邮箱、银行卡等核心 PII。

### Go 依赖

- Go 1.21+
- 详见 `go.mod` 获取完整列表

## 测试

### 运行测试

```bash
# 运行所有测试
go test ./...

# 运行测试并生成覆盖率报告
go test -cover ./...

# 运行集成测试（含 Mock LLM/AIFW）
go test -v ./internal/integration/
```

### 集成测试覆盖

- 本地凭据遮罩（6 种凭证类型）
- RestoreAll 降级机制
- RestoreArgs 子串还原
- MaskMap FIFO 淘汰
- 存储层（Memory / Layered / Disk）
- 遮罩失败策略（block / passthrough / redact）
- 流式响应凭据保护
- 工具调用凭据保护
- 会话隔离
- 大请求体处理
- 无效请求处理

详见 [TEST_REPORT.md](./TEST_REPORT.md)。

## 部署

### Docker

```bash
# 构建镜像
docker build -t closemask .

# 运行容器
docker run -d \
  -p 8846:8846 \
  closemask
```

> **注意**：LLM 的 API Key 通过请求中的 `Authorization` 头传递，CloseMask 自动透传给 LLM，无需配置到 CloseMask 中。

### Kubernetes

```yaml
apiVersion: apps/v1
kind: Deployment
metadata:
  name: closemask
spec:
  replicas: 3
  selector:
    matchLabels:
      app: closemask
  template:
    metadata:
      labels:
        app: closemask
    spec:
      containers:
      - name: proxy
        image: closemask:latest
        ports:
        - containerPort: 8846
        env:
        - name: CLOSEMASK_LLM_URL
          value: "https://api.openai.com/v1"
```

## 监控

### 健康检查

```bash
curl http://localhost:8846/health
```

### 日志

默认日志仅输出到终端（stderr），实时显示遮罩和还原操作：

```
2026/04/12 10:30:00 ≍ ${CRED_a1b2c3} -> sk-pr****7890 (本地凭据, session=a1b2c3d4)
2026/04/12 10:30:00 ≍ ${PHONE_d4e5f6} -> 138****5678 (内置PII, session=a1b2c3d4)
2026/04/12 10:30:00 ≍ msg[0] 遮罩完成: 45字 -> 38字 (session=a1b2c3d4)
2026/04/12 10:30:02 [RESTORE-OK] restored: 38 -> 45 chars (session=a1b2c3d4)
```

如需持久化日志，在 `config.json` 中设置 `"log_to_file": true`，日志将同时写入 `./logs/closemask.log`。

日志中的所有 PII 值均自动脱敏处理（仅保留前后各 4 个字符）。

## 许可证

MIT License - 可免费用于商业用途、修改和分发。

详见 [LICENSE](./LICENSE)。

## 贡献

欢迎贡献！请参阅 [CONTRIBUTING.md](./CONTRIBUTING.md) 了解指南。

## 支持

- GitHub Issues: https://github.com/huilangsh/closemask/issues
- 设计文档: [DESIGN.md](../docs-en/DESIGN.md)
- 测试报告: [TEST_REPORT.md](./TEST_REPORT.md)

## 致谢

基于 [OneAIFW](https://github.com/funstory-ai/aifw) 构建 - 一个优秀的 MIT 许可证 PII 检测引擎。
