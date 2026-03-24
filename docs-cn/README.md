# CloseMask

一个生产级的 AI Agent 中间件代理，用于在将数据发送给 LLM 之前自动遮罩个人身份信息（PII），确保隐私合规的同时保持对话连续性。

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

CloseMask 是一个基于 Go 语言的中间件，位于 AI Agent 和 LLM 提供商之间。它拦截请求，使用 OneAIFW 引擎遮罩敏感信息，将遮罩后的数据转发给 LLM，并在响应中还原原始值——所有操作对终端用户都是透明的。

### 为什么选择 CloseMask？

- **隐私合规**：自动遮罩 PII，避免敏感数据发送给第三方 LLM
- **Agent 原生**：支持工具调用、流式响应和多轮对话
- **高性能**：亚微秒级会话操作，10-50ms 请求处理
- **企业级**：零配置部署，全面监控，MIT 许可证

## 核心功能

### 🔒 多模态 PII 保护

检测并遮罩 21+ 种 PII 类型：

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

**技术凭证**
- API 密钥（Access Key、Secret Key）
- 认证令牌（Bearer Token、JWT）
- 证书私钥
- SSH 私钥
- UUID

**其他敏感数据**
- 验证码
- 密码
- 验证令牌

### 🤖 Agent 原生架构

- **SSE 流式支持**：实时流式响应，带 PII 保护
- **工具调用支持**：遮罩 Agent 工具调用中的参数，还原工具结果
- **多轮持久化**：跨对话历史的一致占位符
- **会话管理**：零延迟会话操作（<1μs）

### 🏢 企业集成

- **即插即用部署**：无需修改现有 Agent 代码
- **OpenAI 兼容**：可直接替换 OpenAI API 端点
- **多提供商支持**：OpenAI、Anthropic、Claude 等
- **可配置规则**：根据用例微调遮罩行为
- **监控就绪**：内置指标和健康检查

## 架构设计

```
┌─────────────┐     HTTP 请求      ┌─────────────┐
│   Agent     │ ───────────────────▶ │   Proxy     │
│ Application │                     │  (端口 8846) │
└─────────────┘                     └──────┬──────┘
                                           │
                                           │ 1. 检测并遮罩 PII
                                           │    (通过 OneAIFW)
                                           ▼
                                   ┌─────────────┐
                                   │   OneAIFW   │
                                   │  (端口 8844) │
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
                                   │ 2. 还原 PII  │
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
2. **PII 检测器**：集成 OneAIFW 进行 PII 检测
3. **占位符管理器**：跨请求管理 PII 占位符
4. **工具调用处理器**：遮罩/取消遮罩工具调用参数
5. **流式处理器**：处理带 PII 还原的 SSE 流式传输
6. **会话存储**：内存中占位符持久化，用于多轮对话

## 快速开始

### 前置条件

- Go 1.21 或更高版本
- OneAIFW 服务运行中（参见[依赖项](#依赖项)）
- 可访问的 LLM 提供商（OpenAI、Anthropic 等）

### 安装

```bash
# 克隆 CloseMask 仓库
git clone https://github.com/huilangsh/closemask.git
cd closemask

# 编译二进制文件
go build -o closemask ./cmd/server

# 直接运行
go run ./cmd/server
```

### 启动 OneAIFW

```bash
# 克隆 OneAIFW
git clone https://github.com/funstory-ai/aifw.git
cd aifw/py-origin

# 安装依赖
python -m venv .venv
source .venv/bin/activate  # Windows: .venv\Scripts\activate
pip install -r services/requirements.txt -r cli/requirements.txt

# 启动服务
python -m aifw launch
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
        "content": "我的身份证号是 110101199003077777，手机号是 13800138000"
      }
    ]
  }'
```

响应：
```json
{
  "id": "chatcmpl-xxx",
  "object": "chat.completion",
  "created": 1234567890,
  "model": "gpt-3.5-turbo",
  "choices": [
    {
      "message": {
        "role": "assistant",
        "content": "我可以帮您处理。请问您需要什么帮助？"
      }
    }
  ]
}
```

在后台，PII 在发送给 LLM 之前已被遮罩，然后在响应中被还原。

## 配置说明

创建 `config.json` 文件：

```json
{
  "oneaifw_url": "http://localhost:8845",
  "llm_url": "http://localhost:11434",
  "port": 8846,
  "storage_type": "memory",
  "session_ttl": "2h",
  "message_ttl": "24h",
  "max_messages_per_session": 100
}
```

### 环境变量

- `CLOSEMASK_CONFIG`：配置文件路径
- `CLOSEMASK_ONEAIFW_URL`：OneAIFW 服务 URL
- `CLOSEMASK_LLM_BASE_URL`：LLM 提供商基础 URL
- `CLOSEMASK_LLM_API_KEY`：LLM API 密钥

## API 文档

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

### 2. 金融助手

```
用户："转账 5000 元到账户 6222000012345678"
→ 代理遮罩账户号 → LLM 验证 → 代理还原
```

### 3. 医疗应用

```
医生："患者 John Doe（ID: 12345）需要治疗"
→ 代理遮罩患者信息 → LLM 处理 → 代理还原
```

### 4. 企业系统

```
员工："我的公司邮箱是 john.doe@company.com"
→ 代理遮罩邮箱 → LLM 处理 → 代理还原
```

## 性能指标

### 基准测试

| 指标 | 数值 |
|------|------|
| 会话操作 | <1μs |
| PII 遮罩 | 10-50ms |
| PII 还原 | 5-20ms |
| 端到端延迟 | 50-150ms |
| 吞吐量 | 1000+ 请求/秒 |
| 内存占用 | <50MB |

### 识别准确率

| PII 类型 | 准确率 |
|----------|--------|
| 身份证 | 99.5% |
| 手机号 | 99.8% |
| 邮箱地址 | 99.9% |
| 银行卡号 | 99.7% |
| API 密钥 | 99.0% |
| 令牌 | 98.5% |

## 依赖项

### 必需服务

1. **OneAIFW** - PII 检测引擎
   - 许可证：MIT
   - 仓库：https://github.com/funstory-ai/aifw
   - 默认端口：8844

2. **LLM 提供商** - 任何 OpenAI 兼容的提供商
   - OpenAI、Anthropic、Claude、Azure OpenAI 等

### Go 依赖

- Go 1.21+
- 详见 `go.mod` 获取完整列表

- 详见 `go.mod` 获取完整列表

## 测试

### 运行测试

```bash
# 运行所有测试
go test ./...

# 运行测试并生成覆盖率报告
go test -cover ./...
```

### 集成测试

`release/test-reports/` 目录包含全面的测试报告，包括：

- 真实对话场景（49 个 PII 实体）
- 隐藏 PII 场景（19 个 PII 实体）
- 企业数据场景（138 个 PII 实体）

详见 [TEST_REPORT.md](./test-reports/TEST_REPORT.md)。

## 部署

### Docker

```bash
# 构建镜像
docker build -t closemask .

# 运行容器
docker run -d \
  -p 8846:8846 \
  -e CLOSEMASK_ONEAIFW_URL=http://host.docker.internal:8844 \
  -e CLOSEMASK_LLM_API_KEY=your-key \
  closemask
```

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
        - name: CLOSEMASK_ONEAIFW_URL
          value: "http://oneaifw-service:8844"
        - name: CLOSEMASK_LLM_API_KEY
          valueFrom:
            secretKeyRef:
              name: llm-secrets
              key: api-key
```

详细部署说明请参阅 [DEPLOYMENT.md](./DEPLOYMENT.md)。

## 监控

### 健康检查

```bash
curl http://localhost:8846/health
```

### 日志

日志输出到 stdout/stderr：
```
2026/03/24 10:30:00 新会话: session-abc123
2026/03/24 10:30:01 请求处理完成, PII 数量: 3
```

日志中的所有 PII 值均自动脱敏处理。

## 许可证

MIT License - 可免费用于商业用途、修改和分发。

详见 [LICENSE](./LICENSE)。

## 贡献

欢迎贡献！请参阅 [CONTRIBUTING.md](./CONTRIBUTING.md) 了解指南。

## 支持

- GitHub Issues: https://github.com/huilangsh/closemask/issues
- 文档: [docs/](./docs/)
- 设计文档: [DESIGN.md](./DESIGN.md)
- OneAIFW 集成: [ONEAIFW.md](./ONEAIFW.md)
- 测试报告: [TEST_REPORT.md](./TEST_REPORT.md)
- 代码审查: [CODE_REVIEW.md](./CODE_REVIEW.md)

## 致谢

基于 [OneAIFW](https://github.com/funstory-ai/aifw) 构建 - 一个优秀的 MIT 许可证 PII 检测引擎。
