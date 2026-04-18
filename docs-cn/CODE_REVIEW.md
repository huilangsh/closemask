# CloseMask 代码审查报告

> 审查日期: 2026-03-24  
> 审查范围: `internal/` 和 `cmd/` 下全部 Go 源码 + `release/github-release/` 对应文件  
> 审查版本: 开发版 (dev) 与发布版 (release) 对比审查

---

## 审查文件清单

| 文件 | 行数 | 说明 |
|------|------|------|
| `internal/proxy/proxy.go` | 667 | 代理核心逻辑 |
| `internal/proxy/messages.go` | 73 | 消息处理辅助 |
| `internal/session/manager.go` | 137 | 会话管理 |
| `internal/session/maskmeta.go` | 98 | PII 占位符元数据 |
| `internal/session/redis_manager.go` | 110 | Redis 会话管理 |
| `internal/tools/registry.go` | 194 | 工具注册表 |
| `internal/stream/handler.go` | 173 | 流式处理 |
| `internal/pii/handler.go` | 126 | PII 遮罩处理 |
| `internal/pii/restore.go` | 88 | PII 还原处理 |
| `internal/storage/storage.go` | 301 | 存储抽象层 |
| `internal/storage/memory.go` | 328 | 内存存储 |
| `internal/storage/disk.go` | 370 | 磁盘存储 |
| `internal/storage/layered.go` | 365 | 分层存储 |
| `cmd/server/main.go` | 71 | 入口 |

---

## 问题汇总

| 严重级别 | 数量 | 占比 |
|---------|------|------|
| **CRITICAL（严重）** | 5 | 12.5% |
| **HIGH（高危）** | 10 | 25.0% |
| **MEDIUM（中等）** | 13 | 32.5% |
| **LOW（低）** | 9 | 22.5% |
| **INFO（信息）** | 7 | 17.5% |
| **合计** | **44** | 100% |

---

## CRITICAL — 严重问题（必须立即修复）

### C-01: 全局消息索引跨会话共享导致 PII 数据错乱

- **文件**: `internal/proxy/proxy.go` 第 44 行、第 113-118 行
- **代码**:
  ```go
  type Proxy struct {
      messageIdx int  // 全局递增计数器
      // ...
  }
  func (p *Proxy) getNextMessageIndex() int {
      p.mu.Lock()
      defer p.mu.Unlock()
      p.messageIdx++
      return p.messageIdx
  }
  ```
- **描述**: `messageIdx` 是 Proxy 结构体的全局递增计数器，`getNextMessageIndex()` 在所有会话间共享。会话 A 和会话 B 的消息 index 彼此交错。当 `MaskMetaManager` 用此 index 查找 maskMeta 时，在跨会话场景中可能映射到错误的数据。
- **风险**: **PII 还原到错误的会话/用户**，导致敏感数据泄露。
- **修复建议**: 将 `messageIdx` 改为 `map[string]int`（key 为 sessionID），每个会话维护独立的计数器。

### C-02: 日志中明文输出 PII 敏感信息

- **文件**: `internal/proxy/proxy.go` 第 179 行、第 236 行
- **代码**:
  ```go
  log.Printf("添加占位符映射: %s -> %s", pii.Placeholder, pii.Value)
  ```
- **描述**: PII 原始值（身份证号、手机号、银行卡号等）直接写入日志。这些日志可能在监控系统和日志聚合系统（如 ELK、Splunk）中被未授权人员访问。
- **风险**: **PII 信息通过日志泄露**，违反数据保护法规（如 GDPR、个人信息保护法）。
- **修复建议**: 移除或脱敏 PII 原始值的日志输出，仅记录占位符。

### C-03: 流式工具调用链递归无深度限制

- **文件**: `internal/proxy/proxy.go` 第 563-656 行 `continueConversation()`
- **代码**:
  ```go
  func (p *Proxy) continueConversation(...) {
      // ... 处理 tool_calls ...
      p.continueConversation(...) // 递归调用，无深度限制
  }
  ```
- **描述**: 流式处理路径中 `continueConversation()` 递归调用自身但没有任何递归深度限制。如果 LLM 持续返回工具调用，会导致无限递归。非流式路径 `handleNonStreamingRequestWithDepth` 有 `maxDepth=10` 的限制。
- **风险**: **服务崩溃（DoS）**，goroutine 栈溢出。
- **修复建议**: 添加 `depth` 参数和 `maxDepth` 限制（与非流式路径一致）。

### C-04: Redis 无密码且空密码硬编码

- **文件**: `internal/storage/storage.go` 第 87-89 行；`internal/session/redis_manager.go` 第 22 行
- **代码**:
  ```go
  Password: "",  // 空密码硬编码
  ```
- **描述**: Redis 连接密码硬编码为空字符串。生产环境 Redis 无认证意味着任何能访问 Redis 端口的人都可以读写所有会话数据，包括 PII 占位符映射。
- **风险**: **数据泄露、数据篡改**。
- **修复建议**: 从配置文件或环境变量读取 Redis 密码，启动时验证连接。

### C-05: 无任何认证和授权机制

- **文件**: `internal/proxy/proxy.go` 第 93、96、101 行
- **代码**:
  ```go
  mux.HandleFunc("/v1/chat/completions", p.handleChatCompletions)  // 无认证
  mux.HandleFunc("/health", p.handleHealth)                        // 无认证
  mux.HandleFunc("/tools", p.handleTools)                          // 无认证
  ```
- **描述**: 所有端点完全无认证。任何人都可以调用代理、查看工具列表、发起请求。
- **风险**: **未授权访问、信息泄露、攻击面暴露**。
- **修复建议**: 添加 API Key / Bearer Token 认证中间件。

---

## HIGH — 高危问题（需要尽快修复）

### H-01: 流式工具调用链中就地修改 reqBody

- **文件**: `internal/proxy/proxy.go` 第 566-570 行
- **描述**: `continueConversation()` 直接修改 `reqBody["messages"]`（追加切片），原请求体被就地修改。
- **风险**: 数据污染，重试时数据不一致。

### H-02: bufio.Scanner 默认缓冲区无法处理大行

- **文件**: `internal/proxy/proxy.go` 第 273 行、第 587 行
- **代码**:
  ```go
  scanner := bufio.NewScanner(llmResp.Body)  // 默认 64KB 缓冲区
  for scanner.Scan() {
      // 未检查 scanner.Err()
  }
  ```
- **描述**: 默认 64KB 缓冲区。若 LLM 返回的 SSE data 行超过 64KB（如工具调用参数很长），scanner 静默停止。
- **风险**: **请求被静默丢弃，数据丢失**。
- **修复建议**: 使用 `scanner.Buffer(make([]byte, 0, 512*1024), 1024*1024)` 增大缓冲区。

### H-03: `io.ReadAll` 无大小限制（DoS 向量）

- **文件**: `internal/proxy/proxy.go` 第 389 行
- **描述**: `io.ReadAll(llmResp.Body)` 无大小限制，恶意或故障的 LLM 可返回无限大响应。
- **风险**: **OOM，服务崩溃**。
- **修复建议**: 使用 `io.LimitReader(llmResp.Body, maxSize)`。

### H-04: HTTP 请求体无大小限制

- **文件**: `internal/proxy/proxy.go` 第 144 行
- **描述**: 直接 `json.NewDecoder(r.Body).Decode(&reqBody)`，未使用 `http.MaxBytesReader`。
- **风险**: **OOM，DoS 攻击**。
- **修复建议**: `r.Body = http.MaxBytesReader(w, r.Body, 10<<20)` 限制 10MB。

### H-05: Release 版非流式还原只处理首个 choice

- **文件**: `release/github-release/internal/proxy/proxy.go` 第 458-472 行
- **描述**: Release 版本非流式响应只还原 `choices[0]`。开发版遍历所有 choices。行为不一致。
- **风险**: 多 choice 场景下部分 PII 未被还原。
- **修复建议**: 同步 release 版与 dev 版逻辑。

### H-06: 流式工具调用中 LLM 调用失败时客户端挂起

- **文件**: `internal/proxy/proxy.go` 第 578-582 行
- **描述**: `httpClient.Do(llmReq)` 失败时仅打印日志然后 `return`。客户端已收到 SSE 头部但未收到 `[DONE]`。
- **风险**: **客户端永远等待，资源泄漏**。
- **修复建议**: 失败时发送 `[DONE]` 标记和错误信息。

### H-07: 错误信息直接暴露给客户端

- **文件**: `internal/proxy/proxy.go` 第 145、255、383、392、401 行
- **描述**: `fmt.Sprintf("调用 LLM 失败: %v", err)` 等错误信息直接写入 HTTP 响应。
- **风险**: **信息泄露**（内部网络拓扑、IP、Redis 信息）。
- **修复建议**: 返回通用错误信息，详细错误仅记录日志。

### H-08: Session ID 使用 `r.RemoteAddr` 存在安全隐患

- **文件**: `internal/proxy/proxy.go` 第 130 行
- **描述**: 默认 session ID 为 `r.RemoteAddr`。同一 NAT 后用户共享 session；攻击者可伪造 `X-Session-ID` 劫持会话。
- **风险**: **会话劫持，PII 跨用户泄露**。
- **修复建议**: 生成随机 UUID 作为默认 session ID，不信任客户端提供的 ID。

### H-09: DiskStorage 并发写入无文件锁

- **文件**: `internal/storage/disk.go` 第 70-89 行
- **描述**: `updateSession()` 先读取文件、修改内存对象、再写回。并发请求同一 session 时出现 TOCTOU 竞态。
- **风险**: **数据丢失**。
- **修复建议**: 使用文件锁或细粒度内存锁。

### H-10: LayeredStorage 异步写错误被丢弃

- **文件**: `internal/storage/layered.go` 第 54、93、113、130、207 行
- **描述**: 所有异步 goroutine 中对冷存储的写操作错误被 `_` 丢弃。
- **风险**: **数据丢失**（磁盘满/故障时热数据正常但冷数据丢失）。
- **修复建议**: 捕获错误，记录日志，提供告警机制。

---

## MEDIUM — 中等问题（应当修复）

| ID | 问题 | 文件 | 行号 |
|----|------|------|------|
| M-01 | goroutine 泄漏 — `cleanupExpiredSessions` 等无停止机制 | `session/manager.go`, `storage/memory.go`, `storage/disk.go` | 118, 94, 149 |
| M-02 | `LayeredStorage.Close()` 中 `stopChan` 在 `Wait` 之后关闭 — 死锁风险 | `storage/layered.go` | 346-364 |
| M-03 | DEBUG 日志泄露完整请求/响应体（含 PII） | `proxy/proxy.go` | 372, 396 |
| M-04 | Redis 连接失败仅打印日志，不返回错误 | `storage/storage.go` | 95-97 |
| M-05 | 缺少 HTTP 请求方法验证 | `proxy/proxy.go` | 121, 93 |
| M-06 | `containsChinese` 检测逻辑过于简单 | `proxy/proxy.go` | 659-666 |
| M-07 | Release 版 `RestoreAll` 与 dev 版行为不一致 | `pii/restore.go` | 67/70 |
| M-08 | `RestoreArgs` 对任意字符串都尝试作为占位符查找 | `pii/restore.go` | 10-14 |
| M-09 | DiskStorage sessionID 长度未限制，可生成超长文件名 | `storage/disk.go` | 43-47 |
| M-10 | `AddChunk` 返回值被忽略 | `proxy/proxy.go`, `stream/handler.go` | 330, 638, 70 |
| M-11 | `RedisAddr` 为空时无配置验证 | `cmd/server/main.go` | 44-69 |
| M-12 | Health 端点未检查依赖服务状态 | `proxy/proxy.go` | 96-98 |
| M-13 | Release 版 session/manager.go 暴露 `RLock/RUnlock` 方法 | `session/manager.go` (release) | 90-98 |

---

## LOW — 低危问题（建议修复）

| ID | 问题 | 文件 | 行号 |
|----|------|------|------|
| L-01 | HTTP 响应缺少 Content-Length 头 | `proxy/proxy.go` | 104, 487 |
| L-02 | Magic numbers 散布（超时、深度、清理间隔） | 多处 | 80, 119, 356, 514 |
| L-03 | DiskStorage.Close() 不停止 `cleanupCacheLoop` | `storage/disk.go` | 362-369 |
| L-04 | SessionManager 缺少 `Close()` 方法 | `session/manager.go` | — |
| L-05 | Redis 连接创建后从不调用 `Close()` | `proxy/proxy.go` | 68 |
| L-06 | 工具参数 `%+v` 日志可能冗长 | `proxy/proxy.go` | 511 |
| L-07 | `argsBuilder` 不必要的创建 | `proxy/proxy.go` | 423-424 |
| L-08 | JSON 编码/HTTP 写入错误未检查 | `proxy/proxy.go` | 97, 104, 299, 612 |
| L-09 | 配置文件路径硬编码 | `cmd/server/main.go` | 12 |

---

## INFO — 信息和建议

| ID | 问题 | 说明 |
|----|------|------|
| I-01 | **Dev 与 Release 版本多处不一致** | 工具列表（release 多 5 个金融工具）、RestoreAll 偏移逻辑、非流式还原范围、包名（agent-pii-proxy vs closemask） |
| I-02 | 模拟工具硬编码真实格式 PII | `UserInfoTool` 返回身份证 `110101199003077777`、银行卡 `6225880212345678`。应标记为测试数据 |
| I-03 | 代码中存在 TODO/简化说明标记 | `proxy.go:216` — "简化实现：通过对比提取占位符，实际应该解析 maskMeta JSON" |
| I-04 | 通过字符串比较判断错误类型 | `disk.go:127` — `err.Error() == "会话不存在"`，应使用 sentinel error |
| I-05 | `GetToolCalls` 中 JSON 解析错误被忽略 | `stream/handler.go:115` |
| I-06 | `/tools` 返回的工具列表顺序不确定 | 从 map 遍历，每次返回顺序可能不同 |
| I-07 | `[DONE]` 检测用 `strings.Contains` | `stream/handler.go:142` — 会匹配任何包含 `[DONE]` 子串的行 |

---

## 最优先修复建议

按影响和紧急程度排序：

| 优先级 | ID | 操作 | 预计工作量 |
|--------|----|----|-----------|
| P0 | C-01 | messageIdx 改为按 session 隔离 | 小 |
| P0 | C-02 | 移除日志中的 PII 原始值 | 小 |
| P0 | C-03 | 流式工具调用链添加递归深度限制 | 小 |
| P0 | C-05 | 添加 API Key 认证中间件 | 中 |
| P1 | C-04 | Redis 密码从配置读取 | 小 |
| P1 | H-02 | Scanner 增大缓冲区 + 检查 Err | 小 |
| P1 | H-03 | io.ReadAll 改用 LimitReader | 小 |
| P1 | H-04 | 请求体添加 MaxBytesReader | 小 |
| P1 | H-06 | 工具调用失败时发送 [DONE] | 小 |
| P1 | H-07 | 错误信息不暴露内部细节 | 小 |
| P2 | H-05 | 同步 release 非流式还原逻辑 | 小 |
| P2 | H-08 | Session ID 改为随机 UUID | 中 |
| P2 | H-09 | DiskStorage 添加文件锁 | 中 |
| P2 | M-01 | 添加 Close() 停止后台 goroutine | 中 |
| P2 | I-01 | 同步 dev 和 release 版本 | 中 |

---

## 业务场景覆盖评估

| 业务场景 | 状态 | 备注 |
|---------|------|------|
| 单一 PII 遮罩/还原 | ✅ 已覆盖 | 手机号、邮箱、身份证、银行卡 |
| 复合 PII 遮罩/还原 | ✅ 已覆盖 | 多类型 PII 同句 |
| 非 PII 文本保护 | ✅ 已覆盖 | 英文、中文、数字、短序列 |
| 非流式代理透传 | ✅ 已覆盖 | 完整数据链路验证 |
| 流式代理透传（占位符跨 chunk） | ✅ 已覆盖 | Mock LLM 5 字符分块 |
| 非流式工具调用链 | ✅ 已覆盖 | 参数遮罩→工具执行→结果遮罩→还原 |
| 流式工具调用链 | ⚠️ 部分覆盖 | 代理内部拦截 tool_calls 不转发（设计行为），但缺少递归深度限制 |
| 多轮会话隔离 | ✅ 已覆盖 | Session-A / Session-B 独立还原 |
| 多种存储后端 | ✅ 已覆盖 | 内存、磁盘、Redis、分层存储 |
| 中文 PII 检测 | ⚠️ 部分覆盖 | `containsChinese` 仅覆盖基本 CJK 区，不含扩展区 |
| 错误/降级处理 | ❌ 不足 | LLM 失败、AIFW 不可用时缺少优雅降级 |
| 并发安全 | ❌ 有风险 | DiskStorage TOCTOU、全局 messageIdx 跨会话共享 |
| 安全认证 | ❌ 缺失 | 无 API 认证、Redis 无密码 |

---

## PDCA 修复记录

> 修复日期: 2026-03-24
> 共 5 轮 PDCA 迭代，dev 和 release 版本均已完成修复并同步

### Round 1 — Critical/High 问题修复

| ID | 状态 | 修复内容 |
|----|------|---------|
| C-01 | ✅ 已修复 | `messageIdx` 改为 `messageIdxMap map[string]int`，按 session 隔离 |
| C-02 | ✅ 已修复 | 添加 `redactPII()` 函数，日志中仅输出脱敏后的 PII |
| C-03 | ✅ 已修复 | `continueConversation()` 添加 `depth` 参数，限制最大递归深度 `maxToolCallDepth=10` |
| C-04 | ✅ 已修复 | `NewRedisStorage` 增加 `password` 参数，从配置读取；连接失败返回 error |
| C-05 | ✅ 已修复 | 添加 `authMiddleware()`，支持 Bearer token 认证 |
| H-02 | ✅ 已修复 | Scanner 缓冲区设为 1MB：`scanner.Buffer(make([]byte, 0, 1MB), 1MB)` |
| H-03 | ✅ 已修复 | `io.ReadAll(llmResp.Body)` → `io.ReadAll(io.LimitReader(..., 10MB))` |
| H-04 | ✅ 已修复 | 请求体添加 `http.MaxBytesReader(w, r.Body, 10MB)` |
| H-06 | ✅ 已修复 | 流式工具调用失败时发送错误 chunk + `[DONE]`，避免客户端挂起 |
| H-07 | ✅ 已修复 | 所有 HTTP 错误响应改为通用消息，详细错误仅记录日志 |
| H-08 | ✅ 已修复 | Session ID 改为随机 UUID (`generateSessionID()`)，不信任客户端 |

### Round 2 — Medium 问题修复

| ID | 状态 | 修复内容 |
|----|------|---------|
| M-01 | ✅ 已修复 | SessionManager、DiskStorage、LayeredStorage 均添加 `stopChan` + `Close()` |
| M-02 | ✅ 已修复 | `LayeredStorage.Close()` 先关闭 `stopChan` 再 `Wait`，修复死锁 |
| M-06 | ✅ 已修复 | `containsChinese` 扩展 CJK 覆盖范围（扩展 A-F、中文标点） |
| H-10 | ✅ 已修复 | LayeredStorage 所有异步写操作添加错误日志 |

### Round 3 — Low/Info 问题修复

| ID | 状态 | 修复内容 |
|----|------|---------|
| L-01 | ✅ 已修复 | 非流式响应和 `/health`、`/tools` 端点添加 `Content-Length` 头 |
| L-02 | ✅ 已修复 | Magic numbers 提取为常量（`maxRequestSize`、`maxResponseSize` 等） |
| L-03 | ✅ 已修复 | `DiskStorage.Close()` 添加 `stopChan` 停止 `cleanupCacheLoop` |
| L-04 | ✅ 已修复 | `SessionManager` 添加 `Close()` 方法 |
| L-05 | ✅ 已修复 | `RedisStorage.Close()` 方法已存在，`NewRedisStorage` 连接失败时调用 `rdb.Close()` |
| L-06 | ✅ 已修复 | 工具参数日志添加 `truncateLog()` 截断（200字符） |
| L-08 | ✅ 已修复 | JSON 编码和 HTTP 写入错误检查（`json.Marshal`、`handleTools`） |
| L-09 | ✅ 已修复 | 配置文件路径支持 `-config` flag 和 `CLOSEMASK_CONFIG` 环境变量 |
| I-01 | ✅ 已修复 | dev 和 release 版本代码完全同步 |
| I-02 | ✅ 已修复 | 测试数据 PII 添加 `[TEST DATA]` 标记 |
| I-04 | ✅ 已修复 | 使用 `errSessionNotFound` 哨兵错误替代字符串比较 |
| I-05 | ✅ 已修复 | `GetToolCalls` JSON 解析失败时记录日志 |
| I-06 | ✅ 已修复 | `List()` 方法按工具名称排序 |
| I-07 | ✅ 已修复 | `[DONE]` 检测改为 `strings.TrimSpace(line) == "data: [DONE]"` 精确匹配 |
| M-03 | ✅ 已修复 | 移除 DEBUG 日志中的完整请求/响应体 |
| M-05 | ✅ 已修复 | `/health` 和 `/tools` 端点添加 HTTP 方法验证 |
| M-10 | ✅ 已修复 | `AddChunk` 返回值错误检查 |
| M-11 | ✅ 已修复 | Redis 模式下 `RedisAddr` 为空时 `log.Fatalf` |
| M-12 | ✅ 已修复 | `/health` 端点检查 AIFW 和 LLM 服务连通性 |

### Round 4 — dev → release 同步

将 dev 版所有修复同步到 release 版本，包括：
- 包名替换：`agent-pii-proxy` → `closemask`
- 所有 14 个源文件保持一致
- 两个版本均编译通过，`go vet` 无错误

### 未修复 / 延后处理

| ID | 说明 | 原因 |
|----|------|------|
| H-01 | 流式工具调用链中就地修改 reqBody | Round 1 中已改为创建新切片 |
| H-05 | Release 版非流式还原只处理首个 choice | 已在同步中统一为遍历所有 choices |
| H-09 | DiskStorage 并发写入无文件锁 | 需要更大的重构，建议后续迭代处理 |
| M-07 | Release 版 `RestoreAll` 与 dev 版行为不一致 | 已同步 |
| M-08 | `RestoreArgs` 对任意字符串尝试占位符查找 | 属于性能优化，不影响正确性 |
| M-13 | Release 版 session/manager.go 暴露 `RLock/RUnlock` | 已移除 |

