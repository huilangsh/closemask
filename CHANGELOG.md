# 更新日志 (Changelog)

## [1.0.0] - 2026-03-22

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

### 文档
- README.md - 项目介绍
- DEPLOYMENT_GUIDE.md - 部署指南
- COMPLETE_FINAL_REPORT.md - 完整测试报告
- agent_pii_proxy_design.md - 架构设计

### 架构
- Go 1.21+ 代理中间件
- OneAIFW PII 遮罩服务集成
- OpenAI 兼容 API 设计
