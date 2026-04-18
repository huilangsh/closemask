# CloseMask Design Document

## System Architecture

CloseMask adopts a dual-engine masking architecture, combining local regex-based credential masking with OneAIFW-powered PII detection to ensure comprehensive privacy protection.

### Overall Architecture

```
┌─────────────────────────────────────────────────────────┐
│                  Application Layer                     │
│  (Agent Applications, Client SDKs, Direct API Calls)   │
└────────────────────┬────────────────────────────────────┘
                     │ HTTP/SSE
                     ▼
┌─────────────────────────────────────────────────────────┐
│                 CloseMask Proxy Layer                   │
├─────────────────────────────────────────────────────────┤
│  ┌──────────────┐  ┌──────────────┐  ┌──────────────┐  │
│  │  HTTP Server │  │  Stream      │  │  Session     │  │
│  │              │  │  Processor   │  │  Manager     │  │
│  └──────┬───────┘  └──────┬───────┘  └──────┬───────┘  │
└─────────┼──────────────────┼──────────────────┼─────────┘
          │                  │                  │
          ▼                  ▼                  ▼
┌─────────────────────────────────────────────────────────┐
│                   Business Logic Layer                 │
├─────────────────────────────────────────────────────────┤
│  ┌──────────────┐  ┌──────────────┐  ┌──────────────┐  │
│  │ LocalMasker  │  │  Tool Call   │  │  PII         │  │
│  │ (Local Regex)│  │  Handler     │  │  Processor   │  │
│  └──────┬───────┘  └──────┬───────┘  └──────┬───────┘  │
└─────────┼──────────────────┼──────────────────┼─────────┘
          │                  │                  │
          │         ┌────────┴────────┐         │
          │         │  Placeholder    │         │
          │         │  Manager (FIFO) │         │
          │         └────────┬────────┘         │
          │                  │                  │
          └──────────────────┼──────────────────┘
                             │
                             ▼
                   ┌─────────────────┐
                   │   OneAIFW       │
                   │  (PII Engine)   │
                   └─────────────────┘
```

## Core Components

### 0. Three-Tier Detection Architecture

CloseMask provides three levels of PII detection capability:

| Tier | Engine | Detection Capability | Requires |
|------|--------|---------------------|----------|
| **Lite** | LocalMasker + BuiltInPII | API Key/JWT/Phone/ID Card/Email/Bank Card/IP | **Zero dependency** |
| **Full** | Above + OneAIFW | Name/Organization/Address + all above | Deploy OneAIFW |
| **Auto** (default) | Auto-select | Use OneAIFW if available, fallback to built-in | Auto-detect |

**Configuration**: `"pii_engine": "auto" | "builtin" | "oneaifw"`

**Auto-discovery**: CloseMask automatically detects and starts OneAIFW if `oneaifw.exe` or `oneaifw/aifw_service.py` is found in the same directory.

### 1. LocalMasker (Local Regex Masking)

**Responsibilities**:
- Detect and mask technical credentials using local regex patterns
- Priority key-name matching (environment variable names trigger masking)
- Value format matching (detect value characteristics like `sk-`, `AKIA`, `eyJ`)

**Key-Name Priority Matching**:
When a line contains `KEY_NAME=value` pattern and the key name matches known credential patterns (e.g., `OPENAI_API_KEY`, `DASHSCOPE_API_KEY`, `DATABASE_URL`), the value is masked regardless of its format.

**Three Masking Levels**:
| Level | Behavior |
|-------|----------|
| `off` | Disable local masking entirely |
| `strict` | Match key names + value formats (default) |
| `aggressive` | Broader matching, e.g., any `sk-` prefix |

**Detected Credential Types**:
- OpenAI API Keys (`sk-proj-...`, `sk-...`)
- Anthropic API Keys (`sk-ant-...`)
- DashScope API Keys (`sk-dashscope-...`)
- Zhipu API Keys
- DeepSeek API Keys
- Database URLs with passwords
- Bearer JWT tokens
- AWS Access Keys (`AKIA...`)
- Generic API keys and secrets

### 1b. BuiltInPIIDetector (Built-in PII Detection)

**Responsibilities**:
- Detect common PII types using local regex patterns
- Zero-dependency, works without OneAIFW
- Compatible maskMeta format for seamless integration

**Detected PII Types**:
- Chinese phone numbers (1[3-9]xxxxxxxxx)
- Chinese ID cards (18-digit)
- Email addresses
- Bank card numbers (16-19 digit)
- IPv4 addresses

### 2. HTTP Server

**Responsibilities**:
- Receive OpenAI-compatible HTTP requests
- Route requests to appropriate handlers
- Handle health check and metrics endpoints

**Key Features**:
- POST /v1/chat/completions (streaming and non-streaming)
- GET /health health check
- LimitReader for request size protection
- Multi-provider support via OpenAI-compatible protocol

**LLM Provider Compatibility**:

CloseMask uses the OpenAI-compatible protocol (`/v1/chat/completions`) to communicate with upstream LLM providers. This design choice enables broad compatibility:

| Provider Type | Connection Method | Example `llm_url` |
|---------------|-------------------|-------------------|
| OpenAI | Direct | `https://api.openai.com/v1` |
| Azure OpenAI | Direct | `https://{resource}.openai.azure.com/...` |
| Ollama | Direct | `http://localhost:11434` |
| Groq | Direct | `https://api.groq.com/openai/v1` |
| DeepSeek | Direct | `https://api.deepseek.com/v1` |
| Anthropic Claude | Via proxy | `http://localhost:3000/v1` (one-api) |
| Other compatible | Direct | Corresponding base URL |

For Anthropic Claude, an OpenAI-compatible proxy layer (such as [one-api](https://github.com/songquanpeng/one-api), [LiteLLM](https://github.com/BerriAI/litellm), or [OpenRouter](https://openrouter.ai/)) is required since Anthropic's native API uses a different protocol format (`/v1/messages`). The proxy layer handles protocol conversion while CloseMask focuses on PII protection.

### 3. Stream Processor

**Responsibilities**:
- Handle SSE (Server-Sent Events) streaming responses
- Restore PII placeholders in each chunk
- Buffer content for cross-chunk placeholder reassembly

**Challenges and Solutions**:
- **Challenge**: PII placeholders may span multiple chunks
- **Solution**: Buffer all content until `[DONE]`, then restore all placeholders at once

### 4. Session Manager

**Responsibilities**:
- Manage session state for multi-turn conversations
- Store PII placeholder mappings with FIFO eviction
- Provide session isolation and expiration mechanisms

**Data Structure**:
```go
type Session struct {
    ID               string
    MaskMap          map[string]string  // placeholder -> original value
    placeholderOrder []string           // FIFO order for eviction
    maxPlaceholders  int                // max placeholders per session
    CreatedAt        time.Time
    LastAccess       time.Time
    MaskMetaMgr      *MaskMetaManager
    mu               sync.RWMutex
}
```

**FIFO Eviction**: When placeholder count exceeds `max_placeholders_per_session`, the oldest placeholders are evicted first.

### 5. Tool Call Handler

**Responsibilities**:
- Detect and parse tool call requests
- Mask PII in tool call parameters
- Restore PII in tool results (including substring restoration via `RestoreArgs`)

**Workflow**:
```
1. Detect tool_calls in LLM response
2. Parse tool parameters
3. Call RestoreArgs to restore placeholders in parameters
4. Execute tool with original parameters
5. Mask PII in tool results
6. Send masked tool results back to LLM
7. Continue conversation until LLM returns final text response
8. Restore PII in final response
```

### 6. Placeholder Manager

**Responsibilities**:
- Generate deterministic placeholders in `${TYPE_hash}` format based on PII value hash
- Maintain placeholder-to-original-value mappings
- Ensure placeholder consistency across requests (same value = same placeholder)
- FIFO eviction when exceeding limits
- Backward compatibility with legacy `${CRED_N}` format

**Placeholder Format** (V0.9.1+):
```
${TYPE_hash}
```
Where:
- `TYPE` is the PII type name (CRED, PHONE, ID_CARD, EMAIL, BANK_CARD, IP_ADDRESS)
- `hash` is the first 6-8 hex characters of sha256/hmac-sha256 of the original value
- Example: `${CRED_a1b2c3}`, `${PHONE_d4e5f6}`, `${EMAIL_f7e8d9}`

**Deterministic Generation**:
- Same PII value always generates the same placeholder, regardless of session or request order
- Configurable hash length: `placeholder_hash_length` (6 or 8, default 6)
- Optional HMAC key: `placeholder_hmac_key` (default: plain sha256)

**IsPlaceholder Detection**:
- Matches `${TYPE_hash}` format (TYPE: uppercase letters + underscore, hash: 6-8 hex chars)
- Backward compatible with legacy `${CRED_N}` format

### 7. PII Processor

**Responsibilities**:
- Call OneAIFW API to detect PII
- Generate placeholders for detected PII
- Restore placeholders to original values with degradation

**Mask Fail Strategy** (when OneAIFW is unavailable):
| Strategy | Behavior |
|----------|----------|
| `block` | Return 503 only if ALL engines are unavailable (built-in detection auto-fallback) |
| `passthrough` | Forward original content to LLM |
| `redact` | Replace detected credentials with `[REDACTED]` |

> **V2.2**: With BuiltInPIIDetector always active, `block` strategy rarely triggers because built-in detection covers core PII types (phone, ID card, email, bank card, API keys).

**Restore Degradation**:
- If placeholder mapping found → restore original value
- If placeholder mapping NOT found → replace with `[PII-UNRECOVERABLE]`
- Never leave raw `__CRED_N__` placeholders in output

### 8. Storage System

**Storage Types**:

| Type | Description | Use Case |
|------|-------------|----------|
| `memory` | In-memory only | Development, low concurrency |
| `disk` | Disk persistence with expiry cleanup | Single-instance, needs persistence |
| `layered` | Memory hot + Disk cold (recommended) | Production, performance + durability |
| `redis` | Redis distributed storage | High concurrency, multi-instance |

**Layered Storage**:
- Read: Memory first, fallback to disk with backfill
- Write: Synchronous to memory + asynchronous to disk
- Cleanup: Background goroutine periodically removes expired disk files
- Recovery: On startup, loads persistent data from disk

**Disk Storage**:
- Files stored under `{data_dir}/{session_id}/` directory
- Background cleanup removes expired session directories
- Atomic file writes via temp file + rename

## Data Flow

### Non-Streaming Request Flow

```
┌─────────┐
│ Client  │
└────┬────┘
     │ 1. HTTP Request (with PII and credentials)
     ▼
┌──────────────┐
│ HTTP Server  │
└────┬─────────┘
     │ 2. Extract message content
     ▼
┌──────────────┐
│ LocalMasker  │
└────┬─────────┘
     │ 3. Mask credentials (API Keys, JWTs, etc.)
     ▼
┌──────────────┐
│ PII Detection│
└────┬─────────┘
     │ 4. Call OneAIFW for PII masking
     ▼
┌──────────────┐
│ OneAIFW      │
└────┬─────────┘
     │ 5. Return masked text
     ▼
┌──────────────┐
│ Placeholder  │
│ Manager      │
└────┬─────────┘
     │ 6. Store mappings
     ▼
┌──────────────┐
│ Send to LLM  │
└────┬─────────┘
     │ 7. Masked request
     ▼
┌──────────────┐
│   LLM        │
└────┬─────────┘
     │ 8. Response (with placeholders)
     ▼
┌──────────────┐
│ PII Restore  │
└────┬─────────┘
     │ 9. Lookup mappings + degradation
     ▼
┌──────────────┐
│ Return to    │
│ Client       │
└──────────────┘
```

### Tool Call Flow

```
┌─────────┐
│ Client  │
└────┬────┘
     │ 1. Chat request with tools
     ▼
┌──────────────┐
│ Proxy        │  ── Mask request ──▶ LLM
└────┬─────────┘
     │ 2. LLM returns tool_calls
     ▼
┌──────────────┐
│ Tool Handler │  ── RestoreArgs ──▶ Execute tool
└────┬─────────┘
     │ 3. Tool result
     ▼
┌──────────────┐
│ Mask result  │  ── Send back ──▶ LLM
└────┬─────────┘
     │ 4. LLM continues
     ▼
┌──────────────┐
│ Restore PII  │  ── Final response ──▶ Client
└──────────────┘
```

## Security Design

### PII Protection Mechanisms

1. **Local-First Masking**: Credentials are masked locally before any external service call
2. **End-to-End Encryption**: HTTPS for all communication between proxy and LLM
3. **Memory Safety**: PII data only exists in memory, disk storage is optional
4. **Session Isolation**: Complete isolation of PII mappings per session
5. **FIFO Eviction**: Automatic cleanup of old placeholders to limit memory exposure
6. **Restore Degradation**: `[PII-UNRECOVERABLE]` instead of raw placeholder leakage

### Defensive Measures

1. **Input Validation**: Strict validation of all incoming requests
2. **Request Size Limit**: LimitReader prevents oversized request bodies
3. **Audit Logging**: Record all PII operations (without actual PII data)
4. **Principle of Least Privilege**: Only use necessary API permissions

## Performance Optimization

### V2.1 Optimizations

1. **strings.Builder** in RestoreAll instead of string concatenation
2. **strconv.Itoa** instead of fmt.Sprintf for placeholder generation
3. **io.WriteString** instead of fmt.Fprintf for HTTP responses
4. **Pre-computed URLs** in handler to avoid repeated string operations
5. **Health client reuse** in proxy for connection pooling
6. **Extracted doPost** method to reduce code duplication

### Caching Strategies

1. **Session Caching**: Session data stored in memory (hot layer)
2. **Storage Backfill**: Miss in memory triggers disk read + memory backfill
3. **Placeholder Mapping Caching**: Active session mappings kept in memory

### Concurrency

1. **Request Concurrency**: Using goroutines for concurrent request processing
2. **Async Disk Writes**: Layered storage writes to disk asynchronously
3. **Stream Processing**: SSE streaming reduces latency

### Resource Management

1. **Connection Pooling**: Reuse HTTP connections (healthClient)
2. **Memory Limits**: FIFO eviction limits placeholder count per session
3. **Disk Cleanup**: Background goroutine removes expired disk files
4. **Graceful Shutdown**: Complete existing requests before shutting down

## Error Handling

### Error Categories

1. **OneAIFW Errors**: PII detection failures → mask_fail_strategy
2. **LLM Errors**: LLM service unavailable or response errors
3. **Session Errors**: Session not found or expired
4. **Network Errors**: Network connection issues

### Error Recovery

1. **Mask Fail Strategy**: Configurable (block/passthrough/redact)
2. **Restore Degradation**: `[PII-UNRECOVERABLE]` for missing mappings
3. **Timeout Control**: All external calls have timeout limits

## Extensibility Design

### Plugin Architecture

Support extending functionality through plugins:
- Custom PII detectors
- Custom placeholder formats
- Custom storage backends

### Multi-Tenant Support

Design supports multi-tenant architecture:
- Tenant isolation
- Independent configuration
- Independent monitoring

## Monitoring and Observability

### Metrics

1. **Request Metrics**: Total requests, success rate, latency
2. **PII Metrics**: Detection count, masking count, restoration count
3. **Session Metrics**: Active sessions, session expiration count, FIFO evictions
4. **Error Metrics**: Error count, error type distribution

### Logging

1. **Structured Logging**: JSON format for easy parsing
2. **Log Levels**: DEBUG, INFO, WARN, ERROR
3. **Sensitive Data Redaction**: No actual PII in logs

## Future Improvements

1. **Custom PII Rules**: Support user-defined PII detection rules
2. **Federated Learning**: Privacy-preserving PII model training
3. **Edge Deployment**: Support running on edge devices
4. **More Storage Backends**: Support PostgreSQL, MongoDB, etc.
5. **High Availability**: Support cluster deployment and failover
6. **Config Hot Reload**: Reload config without restart
