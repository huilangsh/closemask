package stream

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/google/uuid"
)

// Chunk SSE 数据块
type Chunk struct {
	ID      string  `json:"id"`
	Object  string  `json:"object"`
	Created int64   `json:"created"`
	Model   string  `json:"model"`
	Choices []Choice `json:"choices"`
}

// Choice 选择项
type Choice struct {
	Index        int    `json:"index"`
	Delta        *Delta `json:"delta"`
	FinishReason *string `json:"finish_reason"`
}

// Delta 增量数据
type Delta struct {
	Role      string     `json:"role,omitempty"`
	Content   string     `json:"content,omitempty"`
	ToolCalls []ToolCall `json:"tool_calls,omitempty"`
}

// ToolCall 工具调用
type ToolCall struct {
	Index    int    `json:"index"`
	ID       string `json:"id,omitempty"`
	Type     string `json:"type,omitempty"`
	Function Function `json:"function"`
}

// Function 函数
type Function struct {
	Name      string `json:"name,omitempty"`
	Arguments string `json:"arguments,omitempty"`
}

// BufferedToolCall 缓冲的工具调用
type BufferedToolCall struct {
	Name      string
	ID        string
	Type      string
	Arguments strings.Builder
	Complete  bool
}

// ToolCallBuffer 工具调用缓冲区
type ToolCallBuffer struct {
	Calls map[int]*BufferedToolCall
}

// NewToolCallBuffer 创建工具调用缓冲区
func NewToolCallBuffer() *ToolCallBuffer {
	return &ToolCallBuffer{
		Calls: make(map[int]*BufferedToolCall),
	}
}

// AddChunk 添加数据块到缓冲区
func (b *ToolCallBuffer) AddChunk(chunk *Chunk) error {
	if len(chunk.Choices) == 0 {
		return nil
	}

	for _, toolCall := range chunk.Choices[0].Delta.ToolCalls {
		index := toolCall.Index

		if b.Calls[index] == nil {
			b.Calls[index] = &BufferedToolCall{
				Type: toolCall.Type,
			}
		}

		buffered := b.Calls[index]

		// 更新字段
		if toolCall.ID != "" {
			buffered.ID = toolCall.ID
		}
		if toolCall.Function.Name != "" {
			buffered.Name = toolCall.Function.Name
		}
		if toolCall.Function.Arguments != "" {
			buffered.Arguments.WriteString(toolCall.Function.Arguments)
		}
	}

	return nil
}

// IsComplete 检查工具调用是否完整
func (b *ToolCallBuffer) IsComplete(finishReason *string) bool {
	if finishReason == nil {
		return false
	}
	return *finishReason == "tool_calls" && len(b.Calls) > 0
}

// GetToolCalls 获取完整的工具调用
func (b *ToolCallBuffer) GetToolCalls() map[string]interface{} {
	result := make(map[string]interface{})
	for index, buffered := range b.Calls {
		// 解析参数
		var args interface{}
		json.Unmarshal([]byte(buffered.Arguments.String()), &args)

		result[fmt.Sprintf("%d", index)] = map[string]interface{}{
			"id":   buffered.ID,
			"type": buffered.Type,
			"function": map[string]interface{}{
				"name":      buffered.Name,
				"arguments": buffered.Arguments.String(),
			},
		}
	}
	return result
}

// Clear 清空缓冲区
func (b *ToolCallBuffer) Clear() {
	b.Calls = make(map[int]*BufferedToolCall)
}

// ParseChunk 解析 SSE 数据行
func ParseChunk(line string) (*Chunk, error) {
	// 跳过空行
	if line == "" {
		return nil, nil
	}

	// 跳过 [DONE]
	if strings.Contains(line, "[DONE]") {
		return nil, nil
	}

	// 提取 data: 前缀
	data := strings.TrimPrefix(line, "data: ")
	if data == line {
		return nil, fmt.Errorf("invalid SSE line format: %s", line)
	}

	var chunk Chunk
	if err := json.Unmarshal([]byte(data), &chunk); err != nil {
		return nil, fmt.Errorf("failed to parse chunk: %w", err)
	}

	return &chunk, nil
}

// SerializeChunk 序列化数据块
func SerializeChunk(chunk *Chunk) (string, error) {
	data, err := json.Marshal(chunk)
	if err != nil {
		return "", err
	}
	return fmt.Sprintf("data: %s\n\n", string(data)), nil
}

// GenerateToolCallID 生成工具调用 ID
func GenerateToolCallID() string {
	return fmt.Sprintf("call_%s", uuid.New().String())
}
