# CloseMask

CloseMask is a production-ready middleware proxy for AI agents that automatically masks Personally Identifiable Information (PII) before sending data to LLMs, ensuring privacy compliance while maintaining conversational continuity.

## Problem Statement

AI agents often process sensitive user data—ID numbers, phone numbers, bank accounts, API keys, and other PII. Sending this data directly to third-party LLMs creates privacy risks and compliance violations.

**The challenge**: How can you protect user privacy while maintaining quality of AI interactions?

## Solution

CloseMask sits between your AI agents and LLM providers. It automatically detects and masks sensitive information before data leaves your infrastructure, then restores original values in LLM responses—all transparently to end users.

## Key Features

**Intelligent PII Detection**
- Detects 21+ PII types: ID cards, phone numbers, emails, bank cards, API keys, tokens, passwords, and more
- Supports Chinese and English text with 99%+ accuracy
- Handles complex patterns: credit cards, verification codes, technical credentials, enterprise data

**Agent-Native Architecture**
- SSE streaming support for real-time AI conversations
- Tool call parameter masking and restoration
- Placeholder persistence across multi-turn conversations
- Zero-latency session management (<1μs)

**Enterprise-Ready**
- Drop-in deployment—no code changes required
- OpenAI-compatible API endpoints
- Multi-provider support: OpenAI, Anthropic, Claude, Azure OpenAI
- Configurable masking rules per use case
- Built-in monitoring and health checks

## How It Works

```
User: "My ID is 110101199003077777"
    ↓
CloseMask detects ID → masks to [ID_CARD_1]
    ↓
Sends to LLM: "My ID is [ID_CARD_1]"
    ↓
LLM responds: "I can help verify your identity..."
    ↓
CloseMask restores: "I can help verify your identity..."
    ↓
User receives: "I can help verify your identity..."
```

**PII never reaches the LLM—privacy protected.**

## Architecture

CloseMask is built on top of the OneAIFW PII detection engine, providing an agent-ready layer with critical workflow features:

**PII Detection Layer (OneAIFW)**
- High-performance detection engine (Zig + Rust core)
- MIT-licensed, commercial-friendly
- 21+ PII types, Chinese/English support
- Core masking and restoration logic

**Agent Layer (CloseMask)**
- OpenAI-compatible API interface
- SSE streaming support for real-time responses
- Tool call parameter masking and restoration
- Multi-turn conversation placeholder persistence
- Session management and state handling

## Performance

- Processing time: 10-50ms per request
- Throughput: 1000+ requests/second
- Memory footprint: <50MB
- Recognition accuracy: 99%+ on common patterns

## Use Cases

**Customer Service Agents**
- Handle user authentication securely
- Process account verification without exposing PII

**Financial Assistants**
- Process payment information safely
- Manage bank account queries securely

**Healthcare Applications**
- Protect patient data during AI interactions
- Maintain HIPAA/GDPR compliance

**Enterprise Systems**
- Secure business intelligence processing
- Protect sensitive corporate data

## Tech Stack

- **Core**: Go 1.21+
- **PII Engine**: OneAIFW (MIT-licensed, Zig+Rust core)
- **Protocol**: HTTP/HTTPS with SSE streaming
- **Deployment**: Docker, Kubernetes, binary distribution

## Quick Start

```bash
# Start CloseMask
./closemask

# Send request
curl -X POST http://localhost:8846/v1/chat/completions \
  -H "Content-Type: application/json" \
  -d '{
    "model": "gpt-3.5-turbo",
    "messages": [{"role": "user", "content": "My ID is 110101199003077777"}]
  }'
```

Response (PII masked transparently, then restored):
```json
{
  "choices": [{
    "message": {
      "content": "I can help you with that. What would you like me to do?"
    }
  }]
}
```

## License

MIT License—free for commercial use, modification, and distribution.

## Documentation

- [Full README](README.md) - Complete documentation
- [Design Overview](DESIGN.md) - Architecture and technical details
- [OneAIFW Integration](ONEAIFW.md) - PII engine integration guide
- [Deployment Guide](DEPLOYMENT.md) - Production deployment instructions
- [Test Report](test-reports/TEST_REPORT.md) - Comprehensive testing results
- [Dependencies](DEPENDENCY_NOTES.md) - System requirements and monitoring

---

**CloseMask**: Privacy protection that doesn't compromise AI quality.
