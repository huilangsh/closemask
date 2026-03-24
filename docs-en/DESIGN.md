# CloseMask Design Document

## System Architecture

CloseMask adopts a layered architecture design, separating PII detection from proxy functionality to ensure maintainability and extensibility.

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
│  │  Tool Call   │  │  Placeholder │  │  PII         │  │
│  │  Handler     │  │  Manager     │  │  Processor   │  │
│  └──────┬───────┘  └──────┬───────┘  └──────┬───────┘  │
└─────────┼──────────────────┼──────────────────┼─────────┘
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

### 1. HTTP Server

**Responsibilities**:
- Receive OpenAI-compatible HTTP requests
- Route requests to appropriate handlers
- Handle health check and metrics endpoints

**Key Features**:
- Supports POST /v1/chat/completions (streaming and non-streaming)
- GET /health health check
- GET /metrics Prometheus metrics

### 2. Stream Processor

**Responsibilities**:
- Handle SSE (Server-Sent Events) streaming responses
- Restore PII placeholders in each chunk
- Maintain streaming continuity

**Challenges and Solutions**:
- **Challenge**: PII placeholders may span multiple chunks
- **Solution**: Use stream buffering and intelligent reassembly

### 3. Session Manager

**Responsibilities**:
- Manage session state for multi-turn conversations
- Store PII placeholder mappings
- Provide session isolation and expiration mechanisms

**Data Structure**:
```go
type Session struct {
    ID        string
    CreatedAt time.Time
    ExpiresAt time.Time
    Placeholders map[string]string  // placeholder -> original value
}

type SessionStore interface {
    Create(sessionID string) (*Session, error)
    Get(sessionID string) (*Session, error)
    Update(sessionID string, placeholders map[string]string) error
    Delete(sessionID string) error
}
```

**Storage Strategies**:
- **In-Memory Storage**: High performance, suitable for low concurrency
- **Redis Storage**: Distributed, suitable for high concurrency
- **Layered Storage**: Memory + Redis persistence

### 4. Tool Call Handler

**Responsibilities**:
- Detect and parse tool call requests
- Mask PII in tool call parameters
- Restore PII in tool results

**Workflow**:
```
1. Detect tool_calls
2. Parse tool parameters
3. Call PII detector to mask parameters
4. Update session placeholder mappings
5. Send masked request to LLM
6. Receive tool results
7. Restore PII in tool results
8. Return restored results
```

### 5. Placeholder Manager

**Responsibilities**:
- Generate unique placeholders
- Maintain placeholder to original value mappings
- Ensure placeholder consistency across requests

**Placeholder Format**:
```
__{TYPE}_{UUID}__
```

**Examples**:
- `__ID_CARD_550e8400-e29b-41d4-a716-446655440000__`
- `__PHONE_550e8400-e29b-41d4-a716-446655440001__`

**Generation Strategy**:
```go
func GeneratePlaceholder(piiType string) string {
    uuid := uuid.New().String()
    return fmt.Sprintf("__%s_%s__", strings.ToUpper(piiType), uuid)
}
```

### 6. PII Processor

**Responsibilities**:
- Call OneAIFW API to detect PII
- Generate placeholders
- Restore placeholders to original values

**API Integration**:
```
POST /api/mask
Request: {"text": "My ID is 110101199003077777"}
Response: {"masked_text": "My ID is __ID_CARD_xxx__", "placeholders": [...]}

POST /api/restore
Request: {"text": "User __ID_CARD_xxx__ exists", "placeholders": [...]}
Response: {"restored_text": "User 110101199003077777 exists"}
```

## Data Flow

### Non-Streaming Request Flow

```
┌─────────┐
│ Client  │
└────┬────┘
     │ 1. HTTP Request (with PII)
     ▼
┌──────────────┐
│ HTTP Server  │
└────┬─────────┘
     │ 2. Extract message content
     ▼
┌──────────────┐
│ PII Detection│
└────┬─────────┘
     │ 3. Call OneAIFW
     ▼
┌──────────────┐
│ OneAIFW      │
└────┬─────────┘
     │ 4. Return masked text
     ▼
┌──────────────┐
│ Placeholder  │
│ Manager      │
└────┬─────────┘
     │ 5. Store mappings
     ▼
┌──────────────┐
│ Send to LLM  │
└────┬─────────┘
     │ 6. Masked request
     ▼
┌──────────────┐
│   LLM        │
└────┬─────────┘
     │ 7. Response (with placeholders)
     ▼
┌──────────────┐
│ PII Restore  │
└────┬─────────┘
     │ 8. Lookup mappings
     ▼
┌──────────────┐
│ Placeholder  │
│ Manager      │
└────┬─────────┘
     │ 9. Return original values
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
     │ 1. Tool call request (with PII)
     ▼
┌──────────────┐
│ Tool Handler │
└────┬─────────┘
     │ 2. Parse tool parameters
     ▼
┌──────────────┐
│ PII Detection│
└────┬─────────┘
     │ 3. Mask parameters
     ▼
┌──────────────┐
│ Placeholder  │
│ Manager      │
└────┬─────────┘
     │ 4. Store mappings
     ▼
┌──────────────┐
│ Send to LLM  │
└────┬─────────┘
     │ 5. Masked tool call
     ▼
┌──────────────┐
│   LLM        │
└────┬─────────┘
     │ 6. Tool execution result
     ▼
┌──────────────┐
│ Tool Handler │
└────┬─────────┘
     │ 7. Restore tool results
     ▼
┌──────────────┐
│ Placeholder  │
│ Manager      │
└────┬─────────┘
     │ 8. Lookup mappings
     ▼
┌──────────────┐
│ Return to    │
│ Client       │
└──────────────┘
```

## Security Design

### PII Protection Mechanisms

1. **End-to-End Encryption**: HTTPS for all communication between proxy and LLM
2. **Memory Safety**: PII data only exists in memory, never written to disk
3. **Session Isolation**: Complete isolation of PII mappings per session
4. **Automatic Expiration**: Sessions and mappings expire automatically to prevent leaks

### Defensive Measures

1. **Input Validation**: Strict validation of all incoming requests
2. **Rate Limiting**: Protection against abusive requests
3. **Audit Logging**: Record all PII operations (without actual PII data)
4. **Principle of Least Privilege**: Only use necessary API permissions

## Performance Optimization

### Caching Strategies

1. **Session Caching**: Session data stored in memory
2. **PII Detection Caching**: Detection results for identical text can be cached
3. **Placeholder Mapping Caching**: Active session mappings kept in memory

### Concurrency

1. **Request Concurrency**: Using goroutines for concurrent request processing
2. **PII Detection Concurrency**: Batch detection for efficiency
3. **Stream Processing**: SSE streaming reduces latency

### Resource Management

1. **Connection Pooling**: Reuse HTTP connections
2. **Memory Limits**: Limit session count and size
3. **Graceful Shutdown**: Complete existing requests before shutting down

## Error Handling

### Error Categories

1. **OneAIFW Errors**: PII detection failures
2. **LLM Errors**: LLM service unavailable or response errors
3. **Session Errors**: Session not found or expired
4. **Network Errors**: Network connection issues

### Error Recovery

1. **Retry Mechanism**: Configurable retry strategies
2. **Degradation**: Reject requests when OneAIFW is unavailable
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
3. **Session Metrics**: Active sessions, session expiration count
4. **Error Metrics**: Error count, error type distribution

### Logging

1. **Structured Logging**: JSON format for easy parsing
2. **Log Levels**: DEBUG, INFO, WARN, ERROR
3. **Sensitive Data Redaction**: No actual PII in logs

### Tracing

1. **Request Tracing**: Unique ID for each request
2. **Distributed Tracing**: Support for OpenTelemetry
3. **Performance Analysis**: Record critical operation durations

## Future Improvements

1. **Custom PII Rules**: Support user-defined PII detection rules
2. **Federated Learning**: Privacy-preserving PII model training
3. **Edge Deployment**: Support running on edge devices
4. **More Storage Backends**: Support PostgreSQL, MongoDB, etc.
5. **High Availability**: Support cluster deployment and failover
