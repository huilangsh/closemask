# CloseMask Code Review Report

> Review Date: 2026-03-24  
> Scope: All Go source files under `internal/` and `cmd/`, plus `release/github-release/` counterparts  
> Versions: Dev vs Release comparison

---

## Summary

| Severity | Count | Percentage |
|----------|-------|-----------|
| **CRITICAL** | 5 | 12.5% |
| **HIGH** | 10 | 25.0% |
| **MEDIUM** | 13 | 32.5% |
| **LOW** | 9 | 22.5% |
| **INFO** | 7 | 17.5% |
| **Total** | **44** | 100% |

---

## CRITICAL Issues

### C-01: Global Message Index Shared Across Sessions — PII Data Corruption

- **File**: `internal/proxy/proxy.go` line 44, 113-118
- **Description**: `messageIdx` is a global counter on the Proxy struct, shared across all sessions. Session A and Session B message indices interleave, causing `MaskMetaManager` to potentially map PII to wrong sessions.
- **Risk**: **PII restored to wrong session/user** — data breach.
- **Fix**: Change `messageIdx` to `map[string]int` (key=sessionID), per-session counter.

### C-02: PII Logged in Plaintext

- **File**: `internal/proxy/proxy.go` line 179, 236
- **Description**: `log.Printf("added placeholder: %s -> %s", pii.Placeholder, pii.Value)` writes raw PII (ID numbers, phone numbers, bank accounts) to logs accessible in monitoring systems.
- **Risk**: **PII leakage via logs**, violating GDPR and data protection laws.
- **Fix**: Remove or redact PII values from logs; only log placeholders.

### C-03: Streaming Tool Call Chain — Unbounded Recursion

- **File**: `internal/proxy/proxy.go` line 563-656 `continueConversation()`
- **Description**: The streaming path calls `continueConversation()` recursively without depth limit. Non-streaming path has `maxDepth=10`.
- **Risk**: **Service crash (DoS)**, stack overflow.
- **Fix**: Add `depth` parameter with `maxDepth` limit (consistent with non-streaming).

### C-04: Redis No Authentication — Hardcoded Empty Password

- **File**: `internal/storage/storage.go` line 87-89; `internal/session/redis_manager.go` line 22
- **Description**: Redis password hardcoded as empty string. Anyone with Redis port access can read/write all session data including PII mappings.
- **Risk**: **Data breach, data tampering**.
- **Fix**: Read Redis password from config/env, validate connection at startup.

### C-05: No Authentication or Authorization

- **File**: `internal/proxy/proxy.go` line 93, 96, 101
- **Description**: All endpoints (`/v1/chat/completions`, `/health`, `/tools`) have zero authentication.
- **Risk**: **Unauthorized access, information disclosure, attack surface exposure**.
- **Fix**: Add API Key / Bearer Token middleware.

---

## HIGH Issues

| ID | Issue | File | Risk |
|----|-------|------|------|
| H-01 | `reqBody` mutated in-place during tool call chain | `proxy.go:566` | Data pollution |
| H-02 | `bufio.Scanner` default 64KB buffer can't handle large lines; `scanner.Err()` unchecked | `proxy.go:273,587` | Silent data loss |
| H-03 | `io.ReadAll` without size limit | `proxy.go:389` | OOM, DoS |
| H-04 | HTTP request body without size limit | `proxy.go:144` | OOM, DoS |
| H-05 | Release version restores only `choices[0]` | `release/.../proxy.go:458` | Partial PII leak |
| H-06 | LLM failure in tool call chain leaves client hanging (no `[DONE]`) | `proxy.go:578` | Client hang |
| H-07 | Internal error details exposed to clients | `proxy.go:145,255,383` | Info disclosure |
| H-08 | `r.RemoteAddr` as default session ID — session hijackable | `proxy.go:130` | Session hijack |
| H-09 | DiskStorage concurrent write without file lock — TOCTOU | `storage/disk.go:70` | Data loss |
| H-10 | LayeredStorage async write errors silently discarded | `storage/layered.go:54,93,113` | Data loss |

---

## MEDIUM Issues

| ID | Issue | File |
|----|-------|------|
| M-01 | Goroutine leaks — no stop mechanism for cleanup tickers | `session/manager.go`, `storage/memory.go`, `storage/disk.go` |
| M-02 | `LayeredStorage.Close()` potential deadlock — `stopChan` closed after `Wait()` | `storage/layered.go:346` |
| M-03 | DEBUG logs expose full request/response bodies (PII) | `proxy.go:372,396` |
| M-04 | Redis connection failure only logged, error not returned | `storage/storage.go:95` |
| M-05 | No HTTP method validation (GET/DELETE treated as POST) | `proxy.go:121` |
| M-06 | `containsChinese` only covers CJK basic block | `proxy.go:659` |
| M-07 | Release `RestoreAll` offset logic differs from dev | `pii/restore.go:67/70` |
| M-08 | `RestoreArgs` tries any string as placeholder | `pii/restore.go:10` |
| M-09 | DiskStorage sessionID length unchecked — long filenames | `storage/disk.go:43` |
| M-10 | `AddChunk` return value ignored | `proxy.go:330`, `stream/handler.go:70` |
| M-11 | No config validation for `RedisAddr` | `cmd/server/main.go:44` |
| M-12 | Health endpoint doesn't check dependency status | `proxy.go:96` |
| M-13 | Release `SessionManager` exposes `RLock/RUnlock` | `session/manager.go` (release) |

---

## LOW Issues

| ID | Issue | File |
|----|-------|------|
| L-01 | Missing Content-Length header on responses | `proxy.go:104,487` |
| L-02 | Magic numbers scattered (timeouts, depths, intervals) | Multiple |
| L-03 | DiskStorage.Close() doesn't stop cleanup goroutine | `storage/disk.go:362` |
| L-04 | SessionManager missing Close() method | `session/manager.go` |
| L-05 | Redis connection never closed | `proxy.go:68` |
| L-06 | `%+v` logging of tool args may be verbose | `proxy.go:511` |
| L-07 | Unnecessary `argsBuilder` allocation | `proxy.go:423` |
| L-08 | JSON encode / HTTP write errors unchecked | `proxy.go:97,104,299,612` |
| L-09 | Config file path hardcoded | `cmd/server/main.go:12` |

---

## Priority Fix Roadmap

| Priority | ID | Action | Effort |
|----------|----|--------|--------|
| P0 | C-01 | Per-session message index | Small |
| P0 | C-02 | Remove PII from logs | Small |
| P0 | C-03 | Add recursion depth limit to streaming tool calls | Small |
| P0 | C-05 | Add API Key auth middleware | Medium |
| P1 | C-04 | Redis password from config | Small |
| P1 | H-02 | Increase Scanner buffer + check Err() | Small |
| P1 | H-03 | LimitReader for response body | Small |
| P1 | H-04 | MaxBytesReader for request body | Small |
| P1 | H-06 | Send [DONE] on tool call LLM failure | Small |
| P1 | H-07 | Generic error messages to clients | Small |
| P2 | H-05 | Sync release non-streaming restore logic | Small |
| P2 | H-08 | Random UUID for session ID | Medium |
| P2 | H-09 | File lock for DiskStorage | Medium |
| P2 | M-01 | Add Close() to stop background goroutines | Medium |
| P2 | I-01 | Sync dev and release versions | Medium |
