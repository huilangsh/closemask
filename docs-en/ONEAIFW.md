# OneAIFW Integration

CloseMask integrates with [OneAIFW](https://github.com/funstory-ai/aifw) as the underlying PII detection engine. This document explains the integration details, why we chose OneAIFW, and how it enhances CloseMask's capabilities.

## What is OneAIFW?

OneAIFW is a high-performance, open-source PII (Personally Identifiable Information) detection and masking engine. It provides accurate detection of sensitive data across multiple languages and formats.

### Key Characteristics

- **License**: MIT License - Commercial-friendly with no restrictions
- **Architecture**: Zig + Rust core with Python bindings
- **Performance**: Sub-millisecond detection for common patterns
- **Languages**: Native support for Chinese and English
- **PII Types**: 21+ types including ID cards, phone numbers, emails, API keys, tokens, and more

## Why OneAIFW?

### 1. MIT License

OneAIFW's MIT license means:
- ✅ Free commercial use without restrictions
- ✅ No copyleft requirements (unlike GPL)
- ✅ Can modify and distribute proprietary versions
- ✅ No patent or royalty requirements
- ✅ Ideal for enterprise deployment

### 2. High Performance

- Core engine built in Zig + Rust for maximum performance
- Native compilation to WASM for browser environments
- Efficient regex-based pattern matching
- Minimal overhead: typically 10-50ms per request

### 3. Multilingual Support

- Native Chinese language support (critical for Asian markets)
- Comprehensive English pattern library
- Extensible to additional languages via Presidio integration

### 4. Active Maintenance

- Regular updates and security patches
- Community-driven feature development
- Professional codebase with good documentation
- Responsive to issues and pull requests

## CloseMask's Value Add

OneAIFW provides excellent PII detection, but CloseMask extends its capabilities specifically for AI agent workflows:

### OneAIFW Limitations (We Address)

| Limitation | CloseMask Solution |
|------------|-------------------|
| No tool call support | ✅ Full tool call parameter masking and restoration |
| No SSE streaming proxy | ✅ Real-time streaming with PII protection |
| Basic HTTP API only | ✅ OpenAI-compatible API interface |
| No session management | ✅ Multi-turn conversation placeholder persistence |
| No LLM integration | ✅ Seamless LLM provider integration |

### Extended Functionality

**Tool Call Support**
```
User: "Search for user with ID 110101199003077777"
→ CloseMask masks: "Search for user with ID ${ID_CARD_a1b2c3}"
→ LLM generates: tool_call({function: "search", args: {id: "${ID_CARD_a1b2c3}"}})
→ CloseMask restores: tool_call({function: "search", args: {id: "110101199003077777"}})
```

**SSE Streaming**
```
User: "Tell me about account 6222000012345678"
→ CloseMask masks account number
→ LLM streams: "Here's info about ${BANK_CARD_xxx}..."
→ CloseMask restores: "Here's info about 6222000012345678..."
```

**Multi-Turn Persistence**
```
Turn 1: User says "My ID is 110101199003077777"
         → Masked to ${ID_CARD_a1b2c3}

Turn 2: User says "Update my ID to 110101199003077777"
         → Same ID → Same placeholder ${ID_CARD_a1b2c3}
         → Consistency maintained across conversation
```

## Architecture

```
┌─────────────────────────────────────────────────────────────┐
│                    CloseMask Layer                       │
│  - OpenAI-compatible API                               │
│  - Tool call processing                                 │
│  - SSE streaming                                       │
│  - Session management                                   │
│  - Placeholder persistence                              │
└────────────────────────┬────────────────────────────────┘
                       │
                       │ HTTP REST API
                       ▼
┌─────────────────────────────────────────────────────────────┐
│                   OneAIFW Engine                        │
│  - PII detection (Zig + Rust)                          │
│  - Pattern matching (regex + NER)                        │
│  - Masking/restoration                                   │
│  - Multilingual support                                   │
└─────────────────────────────────────────────────────────────┘
```

## API Integration

CloseMask communicates with OneAIFW via standard HTTP REST APIs:

### Mask Request

```http
POST /api/mask_text HTTP/1.1
Host: oneaifw-service:8844
Content-Type: application/json

{
  "text": "My ID is 110101199003077777",
  "language": "zh"
}
```

Response:
```json
{
  "output": {
    "text": "My ID is ${ID_CARD_a1b2c3}",
    "maskMeta": "base64_encoded_metadata"
  },
  "error": null
}
```

### Restore Request

```http
POST /api/restore_text HTTP/1.1
Host: oneaifw-service:8844
Content-Type: application/json

{
  "text": "My ID is ${ID_CARD_a1b2c3}",
  "maskMeta": "base64_encoded_metadata"
}
```

Response:
```json
{
  "output": {
    "text": "My ID is 110101199003077777"
  },
  "error": null
}
```

## Supported PII Types

OneAIFW detects the following PII categories:

### Personal Information
- ID cards (Chinese national ID, SSN, etc.)
- Phone numbers (mobile, landline, international)
- Email addresses
- Physical addresses
- Names

### Financial Data
- Bank card numbers (Credit/Debit cards)
- Payment information
- Transaction amounts
- Financial credentials

### Technical Credentials
- API keys (Access Keys, Secret Keys)
- Authentication tokens (Bearer tokens, JWT)
- Certificate private keys
- SSH private keys
- UUIDs

### Other Sensitive Data
- Verification codes
- Passwords
- Validation tokens

For a complete list, see [OneAIFW documentation](https://github.com/funstory-ai/aifw).

## Deployment

### Running OneAIFW

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

OneAIFW will start on `http://localhost:8844` by default.

### Configuration

Configure CloseMask to use OneAIFW:

```json
{
  "llm_url": "http://localhost:11434",
  "oneaifw_url": "http://localhost:8845",
  "pii_engine": "auto"
}
```

### Docker Deployment

```bash
# Run OneAIFW in Docker
docker run -d \
  -p 8844:8844 \
  -e AIFW_HTTP_API_KEY=your-key \
  -v ~/.aifw:/data/aifw \
  funstoryai/oneaifw:latest
```

## Performance

| Metric | Value |
|---------|-------|
| Mask operation | 10-30ms |
| Restore operation | 5-20ms |
| Throughput | 1000+ requests/second |
| Memory footprint | <50MB |
| Recognition accuracy | 99%+ |

## Security Considerations

### Network Security
- Use HTTPS in production (OneAIFW supports TLS)
- Implement mutual TLS between CloseMask and OneAIFW
- Add authentication via OneAIFW's HTTP API key

### Data Privacy
- OneAIFW processes PII but does not store it
- Mask metadata is temporary and in-memory only
- No data is logged or persisted

### Failure Handling
- CloseMask implements circuit breaker pattern
- Fallback strategies when OneAIFW is unavailable
- Graceful degradation when PII service fails

## Monitoring

CloseMask monitors OneAIFW health and performance:

### Health Checks
```
GET /api/health
```

Response:
```json
{
  "status": "ok"
}
```

### Metrics
- OneAIFW response time (p50, p95, p99)
- Mask/restore operation latency
- Error rate and retry count
- Connection pool status

## Troubleshooting

### Connection Errors
- Verify OneAIFW is running: `curl http://localhost:8844/api/health`
- Check firewall rules between services
- Verify URL configuration in CloseMask

### Performance Issues
- Increase OneAIFW worker threads
- Add caching for common PII patterns
- Consider scaling OneAIFW horizontally

### Accuracy Issues
- Update OneAIFW to latest version
- Configure custom regex rules via OneAIFW's config
- Report issues to OneAIFW project

## Future Enhancements

### Potential Upgrades
- Embed OneAIFW as native Go library (if feasible)
- Implement local PII detection for reduced latency
- Add custom PII pattern definitions
- Support for additional languages

### Contributing
- Submit improvements to OneAIFW project
- Share custom PII patterns
- Report bugs and issues
- Contribute to documentation

## License

OneAIFW is licensed under the MIT License.

**TL;DR**: You can use, modify, and distribute it freely, even in commercial products. Only requirement is to keep the copyright notice.

CloseMask also uses the MIT License, ensuring full compatibility.

## References

- [OneAIFW GitHub Repository](https://github.com/funstory-ai/aifw)
- [OneAIFW Documentation](https://github.com/funstory-ai/aifw/blob/main/README.md)
- [MIT License](https://choosealicense.com/licenses/mit/)
