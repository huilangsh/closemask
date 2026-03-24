# CloseMask

[English](#english) | [中文](#chinese)

---

## English

CloseMask is a production-ready middleware proxy for AI agents that automatically masks Personally Identifiable Information (PII) before sending data to LLMs, ensuring privacy compliance while maintaining conversational continuity.

**📖 [Full Documentation (English)](./docs-en/README.md)**

## 中文

CloseMask 是一个生产级的 AI Agent 中间件代理，用于在将数据发送给 LLM 之前自动遮罩个人身份信息（PII），确保隐私合规的同时保持对话连续性。

**📖 [完整文档（中文）](./docs-cn/README.md)**

---

## 快速开始

```bash
# 克隆仓库
git clone https://github.com/huilangsh/closemask.git
cd closemask

# 编译
go build -o closemask ./cmd/server

# 运行
./closemask
```

## 许可证

MIT License - 免费商业使用

详见 [LICENSE](./LICENSE)。
