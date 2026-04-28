# 更新日志 (Changelog)

## [0.11.2] - 2026-04-27

### 修复
- 🐛 **FastAPI 弃用警告**：将 `@app.on_event("startup")` 改为 `lifespan` 事件处理器
- 🐛 **批处理文件编码**：修复 Windows 下中文乱码问题

### 文件变更
- 修改 `ner_service/ner_service.py`：使用 lifespan 事件处理器
- 修改 `ner_service/start_ner.bat`：确保 UTF-8 编码

## [0.11.1] - 2026-04-27

### 修复
- 🐛 **NER 类型映射修复**：修复中文 NER 模型类型不被识别的问题
  - NER 服务返回 `name`、`address` 等类型
  - Go 代码期望 `PER`、`LOC` 等标准类型
  - 添加映射：`name` → `USER_NAME`，`address` → `PHYSICAL_ADDRESS`
  - 修复后人名、地址可被正确遮罩

### 文件变更
- 修改 `internal/pii/ner_detector_nocgo.go`：扩展 `MapNERTypeToPIIType`
- 修改 `internal/pii/ner_detector_cgo.go`：扩展 `MapNERTypeToPIIType`

## [0.11.0] - 2026-04-27

### 优化
- ⚡ **占位符存储优化**：按类型+hash前缀分文件夹存储，大幅提升查询效率
  - 存储结构：`data/placeholders/{PII_TYPE}/{HASH_PREFIX}.json`
  - 查询占位符 `${CRED_33xxx}` 只需读取 `CRED/33.json` 一个文件
  - 支持百万级占位符的高效查询

### 代码清理
- 🗑️ 删除未使用的存储代码
  - 移除 `memory_cache.go`、`async_persister.go`（已被 `PlaceholderStorage` 替代）
  - 移除对应的测试文件

### 文件变更
- 修改 `internal/storage/layered.go`：集成 `PlaceholderStorage`
- 修改 `internal/storage/placeholder_storage.go`：新增 `ClearAll()` 方法
- 删除 `internal/storage/memory_cache.go`
- 删除 `internal/storage/memory_cache_test.go`
- 删除 `internal/storage/async_persister.go`
- 删除 `internal/storage/async_persister_test.go`

### 存储结构变化
```
修改前：                          修改后：
data/sessions/                    data/
├── {sessionID}.json              ├── sessions/
└── global_placeholders/          │   └── {sessionID}.json
    └── placeholders_0000.json    └── placeholders/
                                     ├── CRED/33.json
                                     ├── EMAIL/d7.json
                                     └── PHONE/38.json
```

### 新增功能
- 🚀 **独立 Python NER 服务**：跨平台开箱即用
  - FastAPI 服务，监听 8847 端口
  - 支持 `/detect`、`/health`、`/models` 接口
  - 首次启动自动下载模型
  - 支持中英文 NER 模型

- 🚀 **BAT 启动脚本**：简化服务管理
  - `start_ner.bat`：启动 NER 服务
  - `stop_ner.bat`：停止 NER 服务
  - 自动检查依赖、端口占用、服务状态

- 🚀 **远程 NER 调用**：Go 主服务支持 HTTP 调用 Python 服务
  - 配置 `ner.mode: remote` 启用
  - 5 秒超时 + 3 次重试
  - 熔断器：连续失败 2 次后暂停 30 秒
  - 自动降级：NER 不可用时回退到正则

- 🚀 **新增 11 种 PII 类型**：扩展检测范围
  - 国际格式（3 种）：国际手机号、美国 SSN、英国 NINO
  - 扩展类型（8 种）：信用卡、车牌、社会信用代码、护照、IPv6、JWT、AWS Key、敏感路径

### 配置变更
- 新增 `ner_mode`：`embedded`(CGO) 或 `remote`(Python)
- 新增 `ner_remote_endpoint`：远程 NER 服务地址
- 新增 `ner_remote_timeout`：远程调用超时
- 新增 `ner_remote_fallback`：是否启用降级

### 测试
- ✅ 所有存储测试通过
- ✅ 编译成功

## [0.10.1] - 2026-04-23

### 修复
- 🐛 **密码正则表达式扩展**：支持更多格式
  - 新增支持：`密码是`、`密码为`、`password is`、`pwd is` 等格式
  - 原有支持：`密码:`、`密码=`、`password:`、`pwd:` 等
  - 修复：`Wi-Fi密码是 Admin@123456` 现在可以正确遮罩

- 🐛 **验证码正则表达式扩展**：支持更多格式
  - 新增支持：`验证码是`、`验证码为`、`verification code is` 等格式
  - 原有支持：`验证码:`、`验证码=`、`verification code:` 等

- 🐛 **助记词正则表达式扩展**：支持更多格式
  - 新增支持：`助记词是`、`助记词为`、`seed is` 等格式
  - 原有支持：`助记词:`、`助记词=`、`seed:` 等

### 完善
- ✨ **NER 检测器完整实现**：集成 ONNX Runtime 推理
  - 实现 `Detect()` 方法，支持 ONNX 模型推理
  - 实现 BIO 标签解析，支持 PER/ORG/LOC 实体识别
  - 条件编译：支持 CGO 和非 CGO 环境
  - 非 CGO 环境降级处理，返回友好错误提示

- ✨ **模型管理器完整实现**：从 HuggingFace 下载模型
  - 实现 `DownloadModel()` 方法
  - 支持下载 vocab.json、labels.json、config.json
  - 支持下载 ONNX 模型（优先量化版本）
  - 自动从 config.json 生成 labels.json

### 测试
- ✅ 新增密码、验证码、助记词正则表达式单元测试
- ✅ 新增审计对话集成测试（测试完整的 PII 遮罩流程）
- ✅ 新增 NER 检测器单元测试

## [0.10.1] - 2026-04-22

### 架构重构
- 🔄 **存储架构重构**：移除全局存储，改为按类型+hash前缀分文件存储
  - 新文件组织：`data/placeholders/{type}/{hash[0:2]}.json`
  - 优势：手工索引、查找快速、分布均匀

- 🔄 **内存管理重构**：内存优先 + 异步持久化
  - 内存 TTL：1 小时
  - 内存阈值：256 MB
  - 渐进淘汰：≥50% 开始淘汰已持久化映射
  - 拒绝请求：≥80% 返回 503

### 新增功能
- ✨ **PII 类型扩展**：从 5 种扩展到 10 种
  - 新增：URL_ADDRESS、PRIVATE_KEY、VERIFICATION_CODE、PASSWORD、RANDOM_SEED
  - 参照 OneAIFW 正则模式优化

- ✨ **三级置信度策略**：strict/balanced/aggressive
  - strict(≥0.8)：仅高置信度，几乎不误报
  - balanced(≥0.6)：平衡模式，推荐默认
  - aggressive(≥0.4)：激进模式，可能误报

- ✨ **NER 集成**：Go + ONNX Runtime（框架）
  - 支持中文模型：ckiplab/bert-tiny-chinese-ner
  - 支持英文模型：dslim/distilbert-NER
  - 默认关闭，用户可选开启
  - 模型自动下载、代理支持

- ✨ **模型管理**：下载、配置、本地存储
  - 启动时检查模型文件
  - 缺失时自动下载
  - 支持离线部署

### 优化
- ⚡ 流式响应不存内存，处理完即丢弃，仅保留映射信息
- ⚡ 异步持久化：批量写入（100次/5秒）+ 淘汰强制写 + 关闭强制写
- ⚡ 退出机制：信号捕获 → 等待流式完成 → 刷盘 → 关闭

### 移除
- ❌ 移除全局存储（global.go、global_persistent.go）
- ❌ 移除 session 文件持久化

### 配置变更
- 新增 `memory.max_memory_mb`：内存阈值（默认 256）
- 新增 `memory.ttl_minutes`：TTL 分钟数（默认 60）
- 新增 `memory.evict_threshold`：淘汰阈值（默认 0.5）
- 新增 `memory.reject_threshold`：拒绝阈值（默认 0.8）
- 新增 `pii.level`：置信度级别（默认 balanced）
- 新增 `pii.ner_enabled`：是否启用 NER（默认 false）
- 新增 `pii.ner_models`：NER 模型配置
- 新增 `pii.ner_model_dir`：模型存储目录
- 新增 `pii.ner_download_proxy`：模型下载代理

## [0.9.4] - 2026-04-21

### 修复
- 🐛 **OneAIFW 健康检查路径 BUG**：修正健康检查端点路径
  - 旧逻辑：请求 `/health`
  - 新逻辑：请求 `/api/health`（与 OneAIFW 实际端点匹配）
  - 影响：修复后 CloseMask 能正确检测到 OneAIFW 服务状态

## [0.9.3] - 2026-04-21

### 修复
- 🐛 **llm_url 路径拼接 BUG**：修正为统一追加 `/chat/completions`
  - 旧逻辑：自动追加 `/v1/chat/completions`，导致百度千帆 `/v2/coding` 被错误拼接为 `/v2/coding/v1/chat/completions`
  - 新逻辑：统一追加 `/chat/completions`，支持任意 base URL 格式
- 🐛 **占位符还原 BUG**：修复 `NormalizePlaceholders` 函数错误修改标准格式占位符的问题
  - 问题：`fuzzyLegacyPlaceholderRe` 正则错误匹配新格式占位符的一部分（如 `${CRED_5c2a9c}` 被拆分为 `${CRED_5}` 和 `c2a9c}`）
  - 修复：在旧格式 fuzzy 匹配中检查后面是否紧跟 hex 字符，如果是则跳过

### 变更
- `llm_url` 配置项填写 base URL，CloseMask 自动追加 `/chat/completions`
- 示例：`https://qianfan.baidubce.com/v2/coding` → `https://qianfan.baidubce.com/v2/coding/chat/completions`
