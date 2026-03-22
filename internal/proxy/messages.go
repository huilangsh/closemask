package proxy

import (
	"context"
	"log"

	"agent-pii-proxy/internal/storage"
)

// saveMessage 保存消息到存储
func (p *Proxy) saveMessage(ctx context.Context, sessionID string, role, content string) error {
	msg := &storage.Message{
		ID:        p.getNextMessageIndex(),
		Role:      role,
		Content:   content,
	}

	if err := p.storage.SaveMessage(ctx, sessionID, msg); err != nil {
		log.Printf("保存消息失败: %v", err)
		return err
	}

	// 检查是否需要截断消息
	if p.config.MaxMessagesPerSession > 0 {
		messages, err := p.storage.GetMessages(ctx, sessionID)
		if err != nil {
			log.Printf("获取消息失败: %v", err)
			return err
		}

		if len(messages) > p.config.MaxMessagesPerSession {
			log.Printf("消息数量 %d 超过限制 %d，开始截断", len(messages), p.config.MaxMessagesPerSession)
			// 保留最后的 max_messages 条消息
			keepCount := p.config.MaxMessagesPerSession
			if err := p.storage.TruncateMessages(ctx, sessionID, keepCount); err != nil {
				log.Printf("截断消息失败: %v", err)
			} else {
				log.Printf("截断完成，保留 %d 条消息", keepCount)
			}
		}
	}

	return nil
}

// loadMessages 从存储加载消息历史
func (p *Proxy) loadMessages(ctx context.Context, sessionID string) ([]map[string]interface{}, error) {
	messages, err := p.storage.GetMessages(ctx, sessionID)
	if err != nil {
		return nil, err
	}

	// 转换为 LLM API 格式
	result := make([]map[string]interface{}, 0, len(messages))
	for _, msg := range messages {
		llmMsg := map[string]interface{}{
			"role": msg.Role,
		}
		if msg.Content != "" {
			llmMsg["content"] = msg.Content
		}
		if len(msg.ToolCalls) > 0 {
			llmMsg["tool_calls"] = msg.ToolCalls
		}
		if msg.ToolCallID != "" {
			llmMsg["tool_call_id"] = msg.ToolCallID
		}
		result = append(result, llmMsg)
	}

	return result, nil
}
