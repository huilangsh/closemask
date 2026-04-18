# 更新日志 (Changelog)

## [0.9.2] - 2026-04-18

### 新增
- ✅ Anthropic Claude 使用指南：文档说明通过 OpenAI 兼容代理层（one-api/LiteLLM/OpenRouter）对接 Anthropic API（当前不支持原生 `/v1/messages` 协议）
- ✅ LLM 提供商兼容性文档：新增 OpenAI、Azure、Ollama、Groq、DeepSeek、Anthropic 等多家提供商的配置说明
- ✅ 多提供商架构说明：CloseMask 采用 OpenAI 兼容协议，支持所有 OpenAI 兼容端点
- ✅ ANTHROPIC_API_KEY 键名精确匹配：LocalMasker 增加 Anthropic API Key 的键名优先检测
- ✅ 项目定位描述：明确 CloseMask 是第三方中转 API 的 PII 遮罩代理，保护敏感数据不离开基础设施

### 文档
- 📝 README.md 增加 LLM 提供商兼容性表格和 Anthropic 配置指南
- 📝 README.md 中英文增加"为什么需要 CloseMask"项目定位说明
- 📝 docs-cn/README.md 增加详细的 LLM 兼容性和 Anthropic 配置文档
- 📝 docs-en/README.md 增加 LLM compatibility and Anthropic configuration guide
- 📝 修正所有文档中的占位符格式（`__TYPE_xxx__` → `${TYPE_hash}`）
- 📝 修正 INTRODUCTION.md 配置示例为当前扁平格式
- 📝 修正架构图（移除多余的 OneAIFW 实例）
- 📝 修正遮罩失败策略默认值为 `pass`

## [0.9.1] - 2026-04-18

### 新增
- ✅ 确定性占位符：基于 PII 值哈希生成占位符（`${TYPE_hash}`，如 `${PHONE_a1b2c3}`），同一值永远生成同一占位符，不依赖 session 和跨轮映射
- ✅ 占位符哈希长度可配置：`placeholder_hash_length`（6或8，默认6），支持 `CLOSEMASK_PLACEHOLDER_HASH_LENGTH` 环境变量
- ✅ HMAC 密钥可配置：`placeholder_hmac_key`（默认空，用 plain sha256），支持 `CLOSEMASK_PLACEHOLDER_HMAC_KEY` 环境变量
- ✅ 日志级别控制：`log_level` 配置项（`quiet`/`info`/`debug`，默认 `info`），支持 `CLOSEMASK_LOG_LEVEL` 环境变量
- ✅ OneAIFW 占位符重映射：自动将 OneAIFW 返回的 `${PHONE_0}` 格式转换为确定性 `${PHONE_hash}` 格式
- ✅ 结构化日志：所有日志按级别分类（Info/Debug/Error），`quiet` 模式仅输出错误

### 变更
- 占位符格式从 `${CRED_N}`（递增数字）改为 `${TYPE_hash}`（类型+哈希），TYPE 由检测器决定（CRED/PHONE/EMAIL/ID_CARD/BANK_CARD/IP_ADDRESS）
- 还原逻辑适配新占位符格式，同时保留 `${CRED_N}` 旧格式向后兼容
- `NormalizePlaceholders` 模糊匹配重写，支持 `${TYPE_hash}` 格式变体和哈希截断检测
- `isPartialPlaceholder` 重写为通用 `${` 前缀检测，支持任意 TYPE 的占位符跨 chunk 缓冲
- 所有 `log.Printf` 替换为 `LogInfof`/`LogDebugf`/`LogErrorf` 分级日志

### 修复
- 🐛 流式跨 chunk 占位符缓冲仅检测 `${CRED_` 前缀导致非 CRED 类型占位符缓冲不触发

## [0.3.0] - 2026-04-12

### 变更
- 🔄 **删除 `llm_api_key` 配置项**：CloseMask 作为反向代理，自动透传客户端请求中的 `Authorization` 头给 LLM，无需单独配置
- 🔄 **CloseMask 认证改用 `X-CloseMask-Key` 头**：避免与 LLM 的 `Authorization` 头冲突
- 🔄 请求中的 `Authorization` 头直接透传给上游 LLM，不再被 CloseMask 认证中间件消费

### 修复
- 🐛 修复客户端通过 `Authorization` 头传递 LLM API Key 时被 CloseMask 认证拦截导致 401 的问题
- 🐛 修复 `continueConversation` 中 `llmAPIKey` 覆盖原始 `Authorization` 头的问题

## [0.2.0] - 2026-04-12

### 新增
- ✅ 三档检测架构：内置正则 → 内置 PII → OneAIFW，按需选择
- ✅ `pii_engine` 配置项：`auto`（默认）/ `builtin` / `oneaifw`
- ✅ 启动横幅：自动显示 PII 引擎状态和检测能力评分
- ✅ `/health` JSON 增强响应：返回各引擎状态、活跃引擎、升级建议
- ✅ OneAIFW 自动发现：同目录 `oneaifw.exe` 或 `oneaifw/aifw_service.py` 自动启动
- ✅ 开箱即用：无需 OneAIFW 即可检测手机号/身份证/邮箱/银行卡/IP/API Key

### 变更
- 🔄 `mask_fail_strategy=block`：内置 PII 检测兜底后，OneAIFW 不可用不再返回 503
- 🔄 `oneaifw_url` 变为可选：不配置则使用内置检测
- 🔄 `/health` 从纯文本改为 JSON 格式

## [0.1.0] - 2026-03-22

### 新增
- ✅ PII 自动遮罩功能 (手机号、身份证、邮箱)
- ✅ PII 自动还原功能
- ✅ Agent 工具调用支持
- ✅ SSE 流式响应支持
- ✅ 多轮对话占位符持久化
- ✅ 会话管理和自动过期
- ✅ 分层存储系统 (Memory + File + Redis)
- ✅ 4 个内置工具 (search、get_weather、calculate、get_user_info)

### 性能
- 会话操作 < 1μs
- PII 遮罩 10-50ms
- 代理简单对话 0.02-0.03s
- 代理工具调用 0.03-0.04s

### 测试
- ✅ 100% 测试通过率 (8/8)
- ✅ 所有核心功能验证完成
