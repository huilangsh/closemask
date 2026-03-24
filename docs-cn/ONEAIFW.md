# OneAIFW 集成说明

CloseMask 集成了 [OneAIFW](https://github.com/funstory-ai/aifw) 作为底层 PII 检测引擎。本文档详细说明了集成细节、选择 OneAIFW 的原因，以及它如何增强 CloseMask 的能力。

## OneAIFW 是什么？

OneAIFW 是一个高性能、开源的 PII（个人身份信息）检测和遮罩引擎。它在多种语言和格式中提供准确的敏感数据检测。

### 主要特性

- **许可证**: MIT 许可证 - 商业友好，无限制
- **架构**: Zig + Rust 核心，Python 绑定
- **性能**: 常见模式的亚毫秒级检测
- **语言**: 原生支持中文和英文
- **PII 类型**: 21+ 种类型，包括身份证、手机号、邮箱、API 密钥、令牌等

## 为什么选择 OneAIFW？

### 1. MIT 许可证

OneAIFW 的 MIT 许可证意味着：
- ✅ 免费商业使用，无限制
- ✅ 无 copyleft 要求（不像 GPL）
- ✅ 可以修改和分发专有版本
- ✅ 无专利或版税要求
- ✅ 非常适合企业部署

### 2. 高性能

- 基于 Zig + Rust 的核心引擎，实现最大性能
- 原生编译为 WASM，支持浏览器环境
- 高效的基于正则表达式的模式匹配
- 最小开销：通常每次请求 10-50ms

### 3. 多语言支持

- 原生中文语言支持（对亚洲市场至关重要）
- 全面的英文模式库
- 通过 Presidio 集成可扩展到其他语言

### 4. 活跃维护

- 定期更新和安全补丁
- 社区驱动的功能开发
- 专业的代码库和良好的文档
- 对问题和拉取请求响应迅速

## CloseMask 的增值功能

OneAIFW 提供了优秀的 PII 检测，但 CloseMask 专门针对 AI Agent 工作流扩展了其能力：

### OneAIFW 的限制（我们解决的问题）

| 限制 | CloseMask 解决方案 |
|------|------------------|
| 不支持工具调用 | ✅ 完整的工具调用参数遮罩和还原 |
| 不支持 SSE 流式代理 | ✅ 带有 PII 保护的实时流式传输 |
| 只有基本 HTTP API | ✅ OpenAI 兼容的 API 接口 |
| 没有会话管理 | ✅ 多轮对话占位符持久化 |
| 没有 LLM 集成 | ✅ 无缝 LLM 提供商集成 |

### 扩展功能

**工具调用支持**
```
用户: "搜索身份证号为 110101199003077777 的用户"
→ CloseMask 遮罩: "搜索身份证号为 [ID_CARD_1] 的用户"
→ LLM 生成: tool_call({function: "search", args: {id: "[ID_CARD_1]"}})
→ CloseMask 还原: tool_call({function: "search", args: {id: "110101199003077777"}})
```

**SSE 流式传输**
```
用户: "告诉我关于账户 6222000012345678 的信息"
→ CloseMask 遮罩账户号码
→ LLM 流式返回: "关于 [BANK_CARD_1] 的信息如下..."
→ CloseMask 还原: "关于 6222000012345678 的信息如下..."
```

**多轮对话持久化**
```
第1轮: 用户说 "我的身份证是 110101199003077777"
         → 遮罩为 [ID_CARD_1]

第2轮: 用户说 "把我的身份证更新为 110101199003077777"
         → 同一个身份证 → 同一个占位符 [ID_CARD_1]
         → 在整个对话中保持一致性
```

## 架构

```
┌─────────────────────────────────────────────────────────────┐
│                   CloseMask 层                        │
│  - OpenAI 兼容 API                                  │
│  - 工具调用处理                                       │
│  - SSE 流式传输                                       │
│  - 会话管理                                           │
│  - 占位符持久化                                       │
└────────────────────────┬────────────────────────────────┘
                       │
                       │ HTTP REST API
                       ▼
┌─────────────────────────────────────────────────────────────┐
│                    OneAIFW 引擎                      │
│  - PII 检测 (Zig + Rust)                          │
│  - 模式匹配 (regex + NER)                           │
│  - 遮罩/还原                                         │
│  - 多语言支持                                         │
└─────────────────────────────────────────────────────────────┘
```

## API 集成

CloseMask 通过标准 HTTP REST API 与 OneAIFW 通信：

### 遮罩请求

```http
POST /api/mask_text HTTP/1.1
Host: oneaifw-service:8844
Content-Type: application/json

{
  "text": "我的身份证是 110101199003077777",
  "language": "zh"
}
```

响应：
```json
{
  "output": {
    "text": "我的身份证是 __PII_ID_CARD_00000001__",
    "maskMeta": "base64_encoded_metadata"
  },
  "error": null
}
```

### 还原请求

```http
POST /api/restore_text HTTP/1.1
Host: oneaifw-service:8844
Content-Type: application/json

{
  "text": "我的身份证是 __PII_ID_CARD_00000001__",
  "maskMeta": "base64_encoded_metadata"
}
```

响应：
```json
{
  "output": {
    "text": "我的身份证是 110101199003077777"
  },
  "error": null
}
```

## 支持的 PII 类型

OneAIFW 检测以下 PII 类别：

### 个人信息
- 身份证（中国身份证、SSN 等）
- 手机号码（手机、座机、国际号码）
- 电子邮件地址
- 物理地址
- 姓名

### 金融数据
- 银行卡号（信用卡/借记卡）
- 支付信息
- 交易金额
- 金融凭证

### 技术凭证
- API 密钥（Access Key、Secret Key）
- 认证令牌（Bearer Token、JWT）
- 证书私钥
- SSH 私钥
- UUID

### 其他敏感数据
- 验证码
- 密码
- 验证令牌

完整列表请参阅 [OneAIFW 文档](https://github.com/funstory-ai/aifw)。

## 部署

### 运行 OneAIFW

```bash
# 克隆 OneAIFW
git clone https://github.com/funstory-ai/aifw.git
cd aifw/py-origin

# 安装依赖
python -m venv .venv
source .venv/bin/activate  # Windows: .venv\Scripts\activate
pip install -r services/requirements.txt -r cli/requirements.txt

# 启动服务
python -m aifw launch
```

OneAIFW 将在默认的 `http://localhost:8844` 上启动。

### 配置

配置 CloseMask 使用 OneAIFW：

```json
{
  "oneaifw": {
    "url": "http://localhost:8844",
    "timeout": "10s",
    "apiKey": "your-http-api-key"
  }
}
```

### Docker 部署

```bash
# 在 Docker 中运行 OneAIFW
docker run -d \
  -p 8844:8844 \
  -e AIFW_HTTP_API_KEY=your-key \
  -v ~/.aifw:/data/aifw \
  funstoryai/oneaifw:latest
```

## 性能

| 指标 | 数值 |
|--------|------|
| 遮罩操作 | 10-30ms |
| 还原操作 | 5-20ms |
| 吞吐量 | 1000+ 请求/秒 |
| 内存占用 | <50MB |
| 识别准确率 | 99%+ |

## 安全考虑

### 网络安全
- 生产环境使用 HTTPS（OneAIFW 支持 TLS）
- 在 CloseMask 和 OneAIFW 之间实现双向 TLS
- 通过 OneAIFW 的 HTTP API 密钥添加认证

### 数据隐私
- OneAIFW 处理 PII 但不存储它
- 遮罩元数据是临时的，仅在内存中
- 不记录或持久化任何数据

### 故障处理
- CloseMask 实现了熔断器模式
- OneAIFW 不可用时的降级策略
- PII 服务故障时的优雅降级

## 监控

CloseMask 监控 OneAIFW 的健康状态和性能：

### 健康检查
```
GET /api/health
```

响应：
```json
{
  "status": "ok"
}
```

### 指标
- OneAIFW 响应时间（p50、p95、p99）
- 遮罩/还原操作延迟
- 错误率和重试次数
- 连接池状态

## 故障排除

### 连接错误
- 验证 OneAIFW 正在运行：`curl http://localhost:8844/api/health`
- 检查服务之间的防火墙规则
- 验证 CloseMask 中的 URL 配置

### 性能问题
- 增加 OneAIFW 工作线程
- 为常见 PII 模式添加缓存
- 考虑水平扩展 OneAIFW

### 准确性问题
- 将 OneAIFW 更新到最新版本
- 通过 OneAIFW 的配置自定义正则规则
- 向 OneAIFW 项目报告问题

## 未来增强

### 潜在升级
- 将 OneAIFW 嵌入为原生 Go 库（如果可行）
- 实现本地 PII 检测以减少延迟
- 添加自定义 PII 模式定义
- 支持更多语言

### 贡献
- 向 OneAIFW 项目提交改进
- 分享自定义 PII 模式
- 报告错误和问题
- 贡献文档

## 许可证

OneAIFW 采用 MIT 许可证。

**简而言之**：您可以自由使用、修改和分发它，即使在商业产品中也是如此。唯一的要求是保留版权声明。

CloseMask 也使用 MIT 许可证，确保完全兼容。

## 参考资料

- [OneAIFW GitHub 仓库](https://github.com/funstory-ai/aifw)
- [OneAIFW 文档](https://github.com/funstory-ai/aifw/blob/main/README.md)
- [MIT 许可证](https://choosealicense.com/licenses/mit/)
