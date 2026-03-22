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

CloseMask is a Go-based middleware that sits between AI agents and LLM providers. It intercepts requests, masks sensitive information using the OneAIFW engine, forwards masked data to LLMs, and restores original values in responses - all transparently to the end user.

### Why CloseMask?

- **Privacy Compliance**: Automatically masks PII before sending to third-party LLMs
- **Agent-Native**: Supports tool calls, streaming, and multi-turn conversations
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
- **Multi-Provider Support**: OpenAI, Anthropic, Claude, and more
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
git clone https://github.com/yourusername/closemask.git
cd closemask

# Build the binary
go build -o closemask ./cmd/proxy

# Run directly
go run ./cmd/proxy
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

# Start the service
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

Behind the scenes, the PII was masked before sending to LLM, then restored in the response.

## Configuration

Create a `config.json` file:

```json
{
  "server": {
    "host": "0.0.0.0",
    "port": 8846
  },
  "oneaifw": {
    "url": "http://localhost:8844",
    "timeout": "10s"
  },
  "llm": {
    "baseUrl": "https://api.openai.com/v1",
    "apiKey": "your-api-key-here",
    "defaultModel": "gpt-3.5-turbo"
  },
  "session": {
    "ttl": "1h",
    "maxSize": 10000
  }
}
```

### Environment Variables

- `PII_PROXY_CONFIG`: Path to config file
- `PII_PROXY_ONEAIFW_URL`: OneAIFW service URL
- `PII_PROXY_LLM_BASE_URL`: LLM provider base URL
- `PII_PROXY_LLM_API_KEY`: LLM API key

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

Response:
```json
{
  "status": "healthy",
  "oneaifw": "connected",
  "llm": "connected"
}
```

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
   - OpenAI, Anthropic, Claude, Azure OpenAI, etc.

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

The `release/test-reports/` directory contains comprehensive test reports including:

- Real conversation scenarios (49 PII entities)
- Hidden PII scenarios (19 PII entities)
- Enterprise data scenarios (138 PII entities)

See [TEST_REPORT.md](./test-reports/TEST_REPORT.md) for details.

## Deployment

### Docker

```bash
# Build image
docker build -t closemask .

# Run container
docker run -d \
  -p 8846:8846 \
  -e PII_PROXY_ONEAIFW_URL=http://host.docker.internal:8844 \
  -e PII_PROXY_LLM_API_KEY=your-key \
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
        - name: PII_PROXY_ONEAIFW_URL
          value: "http://oneaifw-service:8844"
        - name: PII_PROXY_LLM_API_KEY
          valueFrom:
            secretKeyRef:
              name: llm-secrets
              key: api-key
```

For detailed deployment instructions, see [DEPLOYMENT.md](./DEPLOYMENT.md).

## Monitoring

### Metrics

The proxy exposes metrics at `/metrics` (Prometheus format):

- `pii_mask_requests_total` - Total mask requests
- `pii_mask_duration_seconds` - Mask operation duration
- `pii_restore_requests_total` - Total restore requests
- `pii_restore_duration_seconds` - Restore operation duration
- `proxy_requests_total` - Total proxy requests
- `proxy_errors_total` - Total errors

### Logging

Logs are structured JSON format:
```json
{
  "timestamp": "2025-03-22T10:30:00Z",
  "level": "info",
  "message": "Request processed",
  "request_id": "req_123",
  "pii_count": 3,
  "duration_ms": 45
}
```

## License

MIT License - Free for commercial use, modification, and distribution.

See [LICENSE](./LICENSE) for details.

## Contributing

Contributions are welcome! Please see [CONTRIBUTING.md](./CONTRIBUTING.md) for guidelines.

## Support

- GitHub Issues: https://github.com/yourusername/closemask/issues
- Documentation: [docs/](./docs/)
- Design: [DESIGN.md](./DESIGN.md)
- OneAIFW Integration: [ONEAIFW.md](./ONEAIFW.md)

## Acknowledgments

Built on top of [OneAIFW](https://github.com/funstory-ai/aifw) - an excellent PII detection engine under MIT license.
