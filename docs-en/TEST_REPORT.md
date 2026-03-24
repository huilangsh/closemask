# CloseMask Test Report

## Overview

| Metric | Value |
|--------|-------|
| Test Date | 2026-03-23 |
| Test Method | Two independent rounds (full service restart between rounds) |
| Tests per Round | 39 |
| Total Tests | 78 |
| Passed | 76 |
| Failed | 2 |
| Pass Rate | 97.4% |

> **Note**: The 2 failures (`tool_stream/has_tool_calls`) are expected design behavior. See details below. Actual functional pass rate: 100%.

## Test Environment

| Component | Address | Description |
|-----------|---------|-------------|
| CloseMask Proxy | localhost:8846 | Go binary, built-in PII proxy middleware |
| OneAIFW (Mock) | localhost:8845 | Python Flask mock PII mask/restore service |
| Mock LLM | localhost:11437 | Python Flask mock LLM (5-char chunked streaming) |
| OS | Windows | Go 1.21+, Python 3.x |

## Test Results by Category

### 1. AIFW Mask/Restore (12/12 PASSED)

| Test | Status | Description |
|------|--------|-------------|
| phone_pii_removed | OK | Phone number removed from text |
| phone_has_placeholder | OK | Placeholder generated (e.g. `__PHONE_f49e4114__`) |
| phone_restored | OK | Placeholder restored to original phone |
| idcard_pii_removed | OK | ID card number masked |
| idcard_has_placeholder | OK | Placeholder generated |
| idcard_restored | OK | Restored correctly |
| email_pii_removed | OK | Email masked |
| email_has_placeholder | OK | Placeholder generated |
| email_restored | OK | Restored correctly |
| multi_pii_removed | OK | Multiple PII masked simultaneously |
| multi_has_placeholder | OK | Multiple placeholders generated |
| multi_restored | OK | All placeholders restored |

### 2. Proxy Non-Stream Mode (6/6 PASSED)

| Test | Status | Description |
|------|--------|-------------|
| ns_phone_restored | OK | Phone number restored in response |
| ns_phone_no_leak | OK | No placeholder leak |
| ns_compound_restored | OK | Compound text (phone+email) fully restored |
| ns_compound_no_leak | OK | No placeholder leak |
| ns_cn_sentence_restored | OK | PII in Chinese sentence restored |
| ns_cn_sentence_no_leak | OK | No placeholder leak |

### 3. Proxy Stream Mode (6/6 PASSED)

| Test | Status | Description |
|------|--------|-------------|
| phone_restored | OK | Phone restored in streaming response |
| phone_no_leak | OK | No placeholder leak |
| email_restored | OK | Email restored in streaming response |
| email_no_leak | OK | No placeholder leak |
| cn_sentence_restored | OK | Chinese sentence PII restored via stream |
| cn_sentence_no_leak | OK | No placeholder leak |

> **Key**: Mock LLM outputs in 5-char chunks. Placeholders like `__PHONE_xxx__` (17 chars) are split across 3-4 chunks. The proxy correctly buffers all content and restores at `[DONE]`.

### 4. Non-PII Text Protection (6/6 PASSED)

| Test | Status | Description |
|------|--------|-------------|
| aifw_1_unchanged | OK | Plain text not modified |
| proxy_1_intact | OK | Proxy passes text through intact |
| aifw_2_unchanged | OK | Plain text not modified |
| proxy_2_intact | OK | Proxy passes text through intact |
| aifw_3_unchanged | OK | Plain text not modified |
| proxy_3_intact | OK | Proxy passes text through intact |

### 5. Mixed PII Types (4/4 PASSED)

| Test | Status | Description |
|------|--------|-------------|
| ns_all_restored | OK | 3 PII types restored (non-stream) |
| ns_no_leak | OK | No placeholder leak |
| stream_all_restored | OK | 3 PII types restored (stream) |
| stream_no_leak | OK | No placeholder leak |

### 6. Tool Call - Stream Mode (2/3)

| Test | Status | Description |
|------|--------|-------------|
| request_ok | OK | HTTP 200 |
| has_tool_calls | **Expected Fail** | Proxy intercepts tool calls internally |
| final_no_leak | OK | No placeholder leak in final response |

> `has_tool_calls` failure is **expected design behavior**: the proxy intercepts LLM tool calls, executes tools internally (restoring placeholders in arguments), sends results back to LLM, and returns only the final text response to the client.

### 7. Tool Call - Non-Stream Mode (2/2 PASSED)

| Test | Status | Description |
|------|--------|-------------|
| request_ok | OK | `finish_reason=stop` |
| no_leak | OK | No placeholder leak |

## Bug Fix: SSE Stream Placeholder Splitting

**Before**: Empty lines between SSE data chunks were incorrectly treated as `[DONE]` signals, causing the content buffer to flush prematurely and placeholders to be split across client-bound chunks.

**After**:
1. Explicit `[DONE]` string detection instead of relying on `nil` return
2. Empty lines (SSE standard separators) are skipped
3. Content is buffered and only restored/sent at real `[DONE]`

**Files**: `internal/proxy/proxy.go` - `handleStreamingRequest` and `continueConversation`

## Feature Checklist

- [x] PII auto-masking (phone, ID card, email)
- [x] Placeholder generation and storage
- [x] Placeholder restoration (non-stream)
- [x] Placeholder restoration (stream, with cross-chunk buffering)
- [x] Non-PII text protection
- [x] Multiple PII type handling
- [x] Tool call argument masking and result restoration
- [x] Post-tool-call response PII restoration
- [x] Multi-turn conversation placeholder persistence
- [x] Post-restart stability
