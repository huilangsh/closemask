# CloseMask

A production-ready middleware proxy for AI agents that automatically masks Personally Identifiable Information (PII) before sending data to LLMs, ensuring privacy compliance while maintaining conversational continuity.

## 📋 Table of Contents

- [Overview](#overview)
- [Features](#features)
- [Architecture](#architecture)
- [Quick Start](#quick-start)
- [Configuration](#configuration)
- [API Documentation](#api-documentation)
- [Use Cases](#use-cases)
- [Performance](#performance)
- [Dependencies](#dependencies)
- [Testing](#testing)
- [Deployment](#deployment)
- [License](#license)

## Overview

CloseMask is a Go-based lightweight middleware that sits between your AI agents and third-party LLM API services. It intercepts requests, masks sensitive information using a three-tier detection engine (local regex → built-in PII → OneAIFW), forwards sanitized data to LLMs, and restores original values in responses - all transparently to the end user.

### Why CloseMask?

When you use third-party LLM APIs (OpenAI, Anthropic, Azure, etc.), every request you send potentially exposes sensitive data to external servers:

- **API Key Leaks**: Developers accidentally paste `sk-proj-...` into LLM prompts during debugging
- **User Privacy Exposure**: Customer service conversations contain phone numbers and ID cards sent to third parties
- **Corporate Data Breach**: Employees share database passwords with LLMs in configuration questions
- **Compliance Risk**: Cross-border PII transmission violates GDPR, PIPL, and other regulations

CloseMask exists to solve this — it builds a protective barrier between you and third-party APIs. It automatically detects and masks all sensitive data, replaces them with placeholders before forwarding to the LLM, and transparently restores original values when placeholders appear in LLM responses. Your agents and users never notice the difference, but your data never leaves your infrastructure in plaintext.

### Why CloseMask?

- **Privacy Compliance**: Automatically masks PII before sending to third-party LLMs
- **Agent-Native**: Supports tool calls, streaming, and multi-turn conversations
- **Multi-Provider**: OpenAI, Anthropic (via OpenAI-compatible proxy), Azure, Ollama, DeepSeek, etc.
- **High Performance**: Sub-millisecond session operations, 10-50ms request processing
- **Enterprise Ready**: Zero-config deployment, comprehensive monitoring, MIT license

## Features

### 🔒 Multi-Modal PII Protection

Detects and masks 21+ PII types:

**Personal Information**
- ID cards (Chinese national ID, SSN, etc.)
- Phone numbers (mobile, landline, international)
- Email addresses
- Physical addresses
- Names

**Financial Data**
- Bank card numbers (Credit/Debit cards)
- Payment information
- Transaction amounts
- Financial credentials

**Technical Credentials**
- API keys (Access Keys, Secret Keys)
- Authentication tokens (Bearer tokens, JWT)
- Certificate private keys
- SSH private keys
- UUIDs

**Other Sensitive Data**
- Verification codes
- Passwords
- Validation tokens

### 🤖 Agent-Native Architecture

- **SSE Streaming Support**: Real-time streaming responses with PII protection
- **Tool Call Support**: Masks parameters in agent tool calls and restores results
- **Multi-Turn Persistence**: Consistent placeholders across conversation history
- **Session Management**: Zero-latency session operations (<1μs)

### 🏢 Enterprise Integration

- **Drop-in Deployment**: No code changes required for existing agents
- **OpenAI Compatible**: Drop-in replacement for OpenAI API endpoints
- **Multi-Provider Support**: OpenAI, Anthropic (via compatible proxy), Azure, Ollama, DeepSeek, and more
- **Configurable Rules**: Fine-tune masking behavior per use case
- **Monitoring Ready**: Built-in metrics and health checks

## Architecture

```
┌─────────────┐     HTTP Request     ┌─────────────┐
│   Agent     │ ───────────────────▶ │   Proxy     │
│ Application │                      │  (Port 8846)│
└─────────────┘                      └──────┬──────┘
                                            │
                                            │ 1. Detect & Mask PII
                                            │    (via OneAIFW)
                                            ▼
                                    ┌─────────────┐
                                    │   OneAIFW   │
                                    │  (Port 8844)│
                                    └──────┬──────┘
                                           Masked
                                           Data
                                            │
                                            ▼
                                    ┌─────────────┐
                                    │     LLM     │
                                    │  Provider   │
                                    └──────┬──────┘
                                           │
                                           │ Response
                                           │
                                            ▼
                                    ┌─────────────┐
                                    │   Proxy     │
                                    │             │
                                    │ 2. Restore  │
                                    │    PII      │
                                    └──────┬──────┘
                                           │
                                           │ Restored
                                           │ Response
                                           ▼
                                    ┌─────────────┐
                                    │   Agent     │
                                    │ Application │
                                    └─────────────┘
```

### Key Components

1. **HTTP Server**: OpenAI-compatible API endpoint
2. **PII Detector**: Integration with OneAIFW for PII detection
3. **Placeholder Manager**: Manages PII placeholders across requests
4. **Tool Call Processor**: Masks/unmasks tool call parameters
5. **Stream Processor**: Handles SSE streaming with PII restoration
6. **Session Store**: In-memory placeholder persistence for multi-turn conversations

## Quick Start

### Prerequisites

- Go 1.21 or higher
- OneAIFW service running (see [Dependencies](#dependencies))
- Access to an LLM provider (OpenAI, Anthropic, etc.)

### Installation

```bash
# Clone CloseMask repository
git clone https://github.com/huilangsh/closemask.git
cd closemask

# Build binary
go build -o closemask ./cmd/server

# Run directly
go run ./cmd/server
```

### Start OneAIFW

```bash
# Clone OneAIFW
git clone https://github.com/funstory-ai/aifw.git
cd aifw/py-origin

# Install dependencies
python -m venv .venv
source .venv/bin/activate  # Windows: .venv\Scripts\activate
pip install -r services/requirements.txt -r cli/requirements.txt

# Start service
python -m aifw launch
```

### Start CloseMask

```bash
# Run with default configuration
./closemask

# Or specify custom configuration
./closemask -config config.json
```

### Make Your First Request

```bash
curl -X POST http://localhost:8846/v1/chat/completions \
  -H "Content-Type: application/json" \
  -d '{
    "model": "gpt-3.5-turbo",
    "messages": [
      {
        "role": "user",
        "content": "My ID is 110101199003077777 and phone is 13800138000"
      }
    ]
  }'
```

Response:
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
        "content": "I can help you with that. What would you like me to do?"
      }
    }
  ]
}
```

Behind the scenes, PII was masked before sending to LLM, then restored in response.

## Configuration

Create a `config.json` file:

```json
{
  "oneaifw_url": "http://localhost:8845",
  "llm_url": "http://localhost:11434",
  "port": 8846,
  "storage_type": "memory",
  "session_ttl": "2h",
  "message_ttl": "24h",
  "max_messages_per_session": 100,
  "log_to_file": false
}
```

### Environment Variables

- `CLOSEMASK_CONFIG`: Path to config file
- `CLOSEMASK_LLM_BASE_URL`: LLM provider base URL
- `CLOSEMASK_API_KEY`: CloseMask authentication key (passed via `X-CloseMask-Key` header). **LLM API keys are NOT configured here** - they are passed via the `Authorization` header in requests and automatically forwarded to the LLM

## LLM Provider Compatibility

CloseMask uses the **OpenAI-compatible protocol** and supports all OpenAI-compatible LLM providers. **`llm_url` must be the complete endpoint URL**:

| Provider | `llm_url` Configuration |
|----------|------------------------|
| **OpenAI** | `https://api.openai.com/v1/chat/completions` |
| **Ollama** | `http://localhost:11434/v1/chat/completions` |
| **Groq** | `https://api.groq.com/openai/v1/chat/completions` |
| **DeepSeek** | `https://api.deepseek.com/chat/completions` |
| **Qwen (Alibaba Cloud)** | `https://dashscope.aliyuncs.com/compatible-mode/v1/chat/completions` |
| **Moonshot Kimi** | `https://api.moonshot.cn/v1/chat/completions` |
| **Zhipu GLM** | `https://open.bigmodel.cn/api/coding/paas/v4/chat/completions` |
| **Azure OpenAI** | See below |
| **Anthropic Claude** | Via OpenAI-compatible proxy (see below) |
| **Other** | Check official docs, use complete endpoint URL |

### Using Anthropic Claude

> ⚠️ **Note**: CloseMask does **not** currently support the Anthropic native API (`/v1/messages`). It only supports the OpenAI-compatible protocol (`/v1/chat/completions`). Using Anthropic requires one of the following proxy layers for protocol conversion.

Anthropic's native API (`/v1/messages`) differs from OpenAI's format. CloseMask requires an **OpenAI-compatible proxy layer** to work with Anthropic. Several options are available:

**Option 1: one-api / new-api (Recommended)**

[one-api](https://github.com/songquanpeng/one-api) is an OpenAI API management and distribution system that supports Anthropic and other providers:

```bash
# Deploy one-api
docker run -d --name one-api -p 3000:3000 justsong/one-api

# Add Anthropic channel and API key in one-api admin panel
# Then configure CloseMask:
```

```json
{
  "llm_url": "http://localhost:3000/v1"
}
```

**Option 2: LiteLLM**

[LiteLLM](https://github.com/BerriAI/litellm) unifies various LLM APIs into OpenAI format:

```bash
# Install LiteLLM
pip install litellm[proxy]

# Start proxy (e.g., with Claude)
litellm --model claude-3-5-sonnet-20241022 --port 4000
```

```json
{
  "llm_url": "http://localhost:4000/v1"
}
```

**Option 3: OpenRouter**

[OpenRouter](https://openrouter.ai/) provides a unified OpenAI-compatible API supporting Anthropic and other providers:

```json
{
  "llm_url": "https://openrouter.ai/api/v1"
}
```

API keys are passed via the `Authorization` header in requests and automatically forwarded to the upstream LLM by CloseMask.

> **Note**: Regardless of which proxy solution you use, CloseMask's PII masking and restoration functionality works identically with Anthropic. The proxy layer only handles protocol conversion.

## API Documentation

### OpenAI-Compatible Endpoints

#### Chat Completions (Non-Streaming)

```
POST /v1/chat/completions
```

Request body follows OpenAI format:
```json
{
  "model": "gpt-3.5-turbo",
  "messages": [
    {"role": "system", "content": "You are a helpful assistant."},
    {"role": "user", "content": "User message with PII"}
  ],
  "temperature": 0.7,
  "max_tokens": 1000
}
```

#### Chat Completions (Streaming)

```
POST /v1/chat/completions
```

Add `"stream": true` to request body for SSE streaming.

Response format follows OpenAI SSE protocol with PII restored in each chunk.

#### Tool Calls

Fully supported. Tool call parameters are masked before sending to LLM, and tool results are automatically unmasked.

```json
{
  "model": "gpt-3.5-turbo",
  "messages": [
    {"role": "user", "content": "Search for John Doe"}
  ],
  "tools": [
    {
      "type": "function",
      "function": {
        "name": "search",
        "description": "Search for user",
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

### Health Check

```
GET /health
```

Response (plain text):
```
OK
```

Returns `503 Service Unavailable` with status message if AIFW or LLM is unreachable.

## Use Cases

### 1. Customer Service Agents

```
Customer: "My ID is 110101199003077777, help me reset password"
→ Proxy masks ID → LLM processes → Proxy restores response
```

### 2. Financial Assistants

```
User: "Transfer $5000 to account 6222000012345678"
→ Proxy masks account number → LLM validates → Proxy restores
```

### 3. Healthcare Applications

```
Doctor: "Patient John Doe (ID: 12345) needs treatment"
→ Proxy masks patient info → LLM processes → Proxy restores
```

### 4. Enterprise Systems

```
Employee: "My company email is john.doe@company.com"
→ Proxy masks email → LLM processes → Proxy restores
```

## Performance

### Benchmarks

| Metric | Value |
|--------|-------|
| Session Operations | <1μs |
| PII Masking | 10-50ms |
| PII Restoration | 5-20ms |
| End-to-End Latency | 50-150ms |
| Throughput | 1000+ req/s |
| Memory Footprint | <50MB |

### Recognition Accuracy

| PII Type | Accuracy |
|----------|----------|
| ID Cards | 99.5% |
| Phone Numbers | 99.8% |
| Email Addresses | 99.9% |
| Bank Cards | 99.7% |
| API Keys | 99.0% |
| Tokens | 98.5% |

## Dependencies

### Required Services

1. **OneAIFW** - PII detection engine
   - License: MIT
   - Repository: https://github.com/funstory-ai/aifw
   - Default Port: 8844

2. **LLM Provider** - Any OpenAI-compatible provider
   - OpenAI, Azure OpenAI, Ollama, Groq, DeepSeek, etc.
   - Anthropic Claude supported via OpenAI-compatible proxy (one-api/LiteLLM/OpenRouter)

### Go Dependencies

- Go 1.21+
- See `go.mod` for complete list

For detailed dependency information, see [DEPENDENCY_NOTES.md](./DEPENDENCY_NOTES.md).

## Testing

### Run Tests

```bash
# Run all tests
go test ./...

# Run with coverage
go test -cover ./...
```

### Integration Tests

The `tests/` directory contains integration test scripts:

- `run_test.py` - Main test runner
- Test scenarios covering real conversations, hidden PII, and enterprise data

See [docs-en/TEST_REPORT.md](./docs-en/TEST_REPORT.md) for details.

## Deployment

### Docker

```bash
# Build image
docker build -t closemask .

# Run container
docker run -d \
  -p 8846:8846 \
  closemask
```

> **Note**: LLM API keys are passed via the `Authorization` header in requests and automatically forwarded to the LLM by CloseMask. No need to configure them in CloseMask.

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

For detailed deployment instructions, see [DEPLOYMENT.md](./DEPLOYMENT.md).

## Monitoring

### Health Check

```bash
curl http://localhost:8846/health
```

### Logging

By default, logs are output to the terminal (stderr) only, showing mask and restore operations in real-time:

```
2026/04/12 10:30:00 ╔═ __CRED_0__ -> sk-pr****7890 (local_masker, session=a1b2c3d4)
2026/04/12 10:30:00 ╔═ __CRED_1__ -> 138****5678 (builtin_pii, session=a1b2c3d4)
2026/04/12 10:30:00 ╔═ msg[0] 遮罩完成: 45字 -> 38字 (session=a1b2c3d4)
2026/04/12 10:30:02 [RESTORE] 还原响应内容中的占位符 (session=a1b2c3d4)
```

To persist logs to a file, set `"log_to_file": true` in `config.json`. Logs will also be written to `./logs/closemask.log`.

All PII values in logs are automatically redacted (only first and last 4 characters are kept).

## License

MIT License - Free for commercial use, modification, and distribution.

See [LICENSE](./LICENSE) for details.

## Contributing

Contributions are welcome! Please see [CONTRIBUTING.md](./CONTRIBUTING.md) for guidelines.

## Support

- GitHub Issues: https://github.com/huilangsh/closemask/issues
- Documentation: [docs/](./docs/)
- Design: [DESIGN.md](./DESIGN.md)
- OneAIFW Integration: [ONEAIFW.md](./ONEAIFW.md)
- Test Report: [TEST_REPORT.md](./TEST_REPORT.md)
- Code Review: [CODE_REVIEW.md](./CODE_REVIEW.md)

## Acknowledgments

Built on top of [OneAIFW](https://github.com/funstory-ai/aifw) - an excellent PII detection engine under MIT license.
