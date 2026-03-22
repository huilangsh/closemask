package proxy

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"strings"
	"sync"
	"time"

	"agent-pii-proxy/internal/pii"
	"agent-pii-proxy/internal/session"
	"agent-pii-proxy/internal/storage"
	"agent-pii-proxy/internal/stream"
	"agent-pii-proxy/internal/tools"
)

// Config 代理配置
type Config struct {
	OneAIFWURL         string `json:"oneaifw_url"`
	LLMURL             string `json:"llm_url"`
	Port               int    `json:"port"`
	StorageType        string `json:"storage_type"`        // "memory", "redis", or "layered"
	RedisAddr          string `json:"redis_addr"`
	DataDir            string `json:"data_dir"`            // 磁盘存储目录（layered 模式）
	MessageTTL         string `json:"message_ttl"`         // 消息保留时长
	SessionTTL         string `json:"session_ttl"`         // 会话 TTL
	MaxMessagesPerSession int `json:"max_messages_per_session"` // 单会话最大消息数
}

// Proxy 代理中间件
type Proxy struct {
	config      *Config
	piHandler   *pii.PIIHandler
	sessMgr     *session.SessionManager
	storage     storage.Storage
	toolReg     *tools.ToolRegistry
	httpClient  *http.Client
	messageIdx  int // 消息索引计数器（会话级）
	msgIdxMutex sync.Mutex
}

// NewProxy 创建代理
func NewProxy(config *Config) *Proxy {
	piHandler := pii.NewPIIHandler(config.OneAIFWURL)

	// 解析 TTL
	sessionTTL, _ := time.ParseDuration(config.SessionTTL)
	if sessionTTL == 0 {
		sessionTTL = 2 * time.Hour
	}

	messageTTL, _ := time.ParseDuration(config.MessageTTL)
	if messageTTL == 0 {
		messageTTL = 24 * time.Hour
	}

	// 创建存储
	var stor storage.Storage
	switch config.StorageType {
	case "redis":
		log.Printf("使用 Redis 存储模式")
		stor = storage.NewRedisStorage(config.RedisAddr, messageTTL, sessionTTL)
	default:
		log.Printf("使用内存存储模式")
		stor = storage.NewMemoryStorage(messageTTL, sessionTTL)
	}

	p := &Proxy{
		config:    config,
		piHandler: piHandler,
		sessMgr:   session.NewSessionManager(sessionTTL),
		storage:   stor,
		toolReg:   tools.NewToolRegistry(),
		httpClient: &http.Client{
			Timeout: 60 * time.Second,
		},
	}

	return p
}

// Start 启动代理服务
func (p *Proxy) Start() error {
	mux := http.NewServeMux()

	// 主代理端点
	mux.HandleFunc("/v1/chat/completions", p.handleChatCompletions)

	// 健康检查
	mux.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte("OK"))
	})

	// 工具列表
	mux.HandleFunc("/tools", func(w http.ResponseWriter, r *http.Request) {
		tools := p.toolReg.List()
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(tools)
	})

	addr := fmt.Sprintf(":%d", p.config.Port)
	log.Printf("代理服务启动在 %s", addr)
	return http.ListenAndServe(addr, mux)
}

// getNextMessageIndex 获取下一个消息索引
func (p *Proxy) getNextMessageIndex() int {
	p.msgIdxMutex.Lock()
	defer p.msgIdxMutex.Unlock()
	p.messageIdx++
	return p.messageIdx
}

// handleChatCompletions 处理聊天补全请求
func (p *Proxy) handleChatCompletions(w http.ResponseWriter, r *http.Request) {
	ctx := context.Background()

	// 获取 session ID
	sessionID := r.Header.Get("X-Session-ID")
	if sessionID == "" {
		sessionID = r.URL.Query().Get("session_id")
	}
	if sessionID == "" {
		sessionID = r.RemoteAddr // 使用客户端地址作为默认
	}

	// 获取会话
	sess := p.sessMgr.GetOrCreate(sessionID)
	maskMetaMgr := sess.GetMaskMetaManager()

	// 刷新会话 TTL（存储层）
	if err := p.storage.TouchSession(ctx, sessionID); err != nil {
		log.Printf("刷新会话 TTL 失败: %v", err)
	}

	// 解析请求
	var reqBody map[string]interface{}
	if err := json.NewDecoder(r.Body).Decode(&reqBody); err != nil {
		http.Error(w, fmt.Sprintf("解析请求失败: %v", err), http.StatusBadRequest)
		return
	}

	// 检查是否是流式请求
	streamReq := false
	if sr, ok := reqBody["stream"].(bool); ok {
		streamReq = sr
	}

	// 遮罩请求消息中的 PII
	if messages, ok := reqBody["messages"].([]interface{}); ok {
		for i, msg := range messages {
			if msgMap, ok := msg.(map[string]interface{}); ok {
				if content, ok := msgMap["content"].(string); ok && content != "" {
					// 检测语言
					language := "en"
					if containsChinese(content) {
						language = "zh"
					}

					// 遮罩
					masked, maskMeta, err := p.piHandler.Mask(content, language)
					if err != nil {
						log.Printf("遮罩失败: %v", err)
						continue
					}

					// 更新消息内容
					messages[i].(map[string]interface{})["content"] = masked

					// 保存 maskMeta
					msgIdx := p.getNextMessageIndex()
					maskMetaMgr.Add(msgIdx, language, maskMeta)
					log.Printf("消息 %d 已遮罩, maskMeta: %s", msgIdx, maskMeta)

					// 保存到存储层
					if err := p.storage.SaveMaskMeta(ctx, sessionID, &storage.MaskMeta{
						MessageID: msgIdx,
						Language:  language,
						MaskMeta:  maskMeta,
					}); err != nil {
						log.Printf("保存 maskMeta 到存储失败: %v", err)
					}

					// 提取占位符并添加到会话
					placeholders := p.extractPlaceholders(content, masked, maskMeta, sess)

					// 保存占位符到存储
					for placeholder, value := range placeholders {
						if err := p.storage.SavePlaceholder(ctx, sessionID, placeholder, value); err != nil {
							log.Printf("保存占位符到存储失败: %v", err)
						}
					}
				}
			}
		}
	}

	// 转发请求到 LLM
	if streamReq {
		p.handleStreamingRequest(w, r, reqBody, sess)
	} else {
		p.handleNonStreamingRequest(w, r, reqBody, sess)
	}
}

// extractPlaceholders 从遮罩结果中提取占位符并保存映射
func (p *Proxy) extractPlaceholders(original, masked, maskMeta string, sess *session.Session) map[string]string {
	placeholders := make(map[string]string)

	// 简化实现：通过对比提取占位符
	// 实际应该解析 maskMeta JSON

	// 提取 __TYPE_INDEX__ 格式的占位符
	// 这里简化处理，直接从 maskMeta 解析
	var meta struct {
		PII []struct {
			Type       string `json:"type"`
			Value      string `json:"value"`
			Start      int    `json:"start"`
			End        int    `json:"end"`
			Placeholder string `json:"placeholder"`
		} `json:"pii"`
	}

	if err := json.Unmarshal([]byte(maskMeta), &meta); err == nil {
		for _, pii := range meta.PII {
			if pii.Placeholder != "" && pii.Value != "" {
				sess.AddPlaceholder(pii.Placeholder, pii.Value)
				placeholders[pii.Placeholder] = pii.Value
				log.Printf("添加占位符映射: %s -> %s", pii.Placeholder, pii.Value)
			}
		}
	}

	return placeholders
}

// handleStreamingRequest 处理流式请求
func (p *Proxy) handleStreamingRequest(w http.ResponseWriter, r *http.Request, reqBody map[string]interface{}, sess *session.Session) {
	// 准备转发请求
	body, _ := json.Marshal(reqBody)
	llmReq, _ := http.NewRequest("POST", p.config.LLMURL+"/v1/chat/completions", bytes.NewReader(body))
	llmReq.Header = r.Header.Clone()
	llmReq.Header.Set("Content-Type", "application/json")

	// 调用 LLM
	llmResp, err := p.httpClient.Do(llmReq)
	if err != nil {
		http.Error(w, fmt.Sprintf("调用 LLM 失败: %v", err), http.StatusBadGateway)
		return
	}
	defer llmResp.Body.Close()

	// 设置 SSE 响应头
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")

	flusher, ok := w.(http.Flusher)
	if !ok {
		http.Error(w, "Streaming not supported", http.StatusInternalServerError)
		return
	}

	// 处理流式响应
	buffer := stream.NewToolCallBuffer()
	scanner := bufio.NewScanner(llmResp.Body)

	for scanner.Scan() {
		line := scanner.Text()

		// 解析 chunk
		chunk, err := stream.ParseChunk(line)
		if err != nil {
			log.Printf("解析 chunk 失败: %v", err)
			continue
		}
		if chunk == nil {
			// 转发 [DONE] 等特殊行
			fmt.Fprintf(w, "%s\n\n", line)
			flusher.Flush()
			continue
		}

		// 检查是否有工具调用
		if len(chunk.Choices) > 0 && len(chunk.Choices[0].Delta.ToolCalls) > 0 {
			// 添加到缓冲区
			buffer.AddChunk(chunk)

			// 检查是否完整
			if buffer.IsComplete(chunk.Choices[0].FinishReason) {
				// 执行工具调用
				toolMessages := p.executeToolCalls(buffer, sess)
				buffer.Clear()

				// 将工具结果发送给 LLM 继续生成
				if len(toolMessages) > 0 {
					p.continueConversation(w, r, reqBody, toolMessages, sess, flusher)
				}
			}
		} else {
			// 非工具调用，直接转发
			serialized, _ := stream.SerializeChunk(chunk)
			fmt.Fprint(w, serialized)
			flusher.Flush()
		}
	}
}

// handleNonStreamingRequest 处理非流式请求
// handleNonStreamingRequest 处理非流式请求，支持工具调用
// maxIterations: 最大迭代次数，防止无限循环
func (p *Proxy) handleNonStreamingRequest(w http.ResponseWriter, r *http.Request, reqBody map[string]interface{}, sess *session.Session) {
	p.handleNonStreamingRequestWithDepth(w, r, reqBody, sess, 0, 10)
}

// handleNonStreamingRequestWithDepth 处理非流式请求，带递归深度限制
func (p *Proxy) handleNonStreamingRequestWithDepth(w http.ResponseWriter, r *http.Request, reqBody map[string]interface{}, sess *session.Session, depth int, maxDepth int) {
	log.Printf("[DEBUG] 开始处理非流式请求，深度: %d/%d", depth, maxDepth)
	
	// 检查递归深度
	if depth >= maxDepth {
		log.Printf("[ERROR] 达到最大递归深度 %d，停止处理", maxDepth)
		http.Error(w, fmt.Sprintf("达到最大工具调用深度 %d", maxDepth), http.StatusInternalServerError)
		return
	}
	
	// 准备转发请求
	body, _ := json.Marshal(reqBody)
	log.Printf("[DEBUG] 转发请求体: %s", string(body))
	
	llmReq, _ := http.NewRequest("POST", p.config.LLMURL+"/v1/chat/completions", bytes.NewReader(body))
	llmReq.Header = r.Header.Clone()
	llmReq.Header.Set("Content-Type", "application/json")

	// 调用 LLM
	log.Printf("[DEBUG] 发送请求到 LLM: %s", p.config.LLMURL+"/v1/chat/completions")
	llmResp, err := p.httpClient.Do(llmReq)
	if err != nil {
		log.Printf("[ERROR] 调用 LLM 失败: %v", err)
		http.Error(w, fmt.Sprintf("调用 LLM 失败: %v", err), http.StatusBadGateway)
		return
	}
	defer llmResp.Body.Close()

	// 读取并解析响应
	respBody, err := io.ReadAll(llmResp.Body)
	if err != nil {
		log.Printf("[ERROR] 读取 LLM 响应失败: %v", err)
		http.Error(w, fmt.Sprintf("读取 LLM 响应失败: %v", err), http.StatusBadGateway)
		return
	}
	
	log.Printf("[DEBUG] LLM 响应状态: %d, 响应体: %s", llmResp.StatusCode, string(respBody))

	var llmResult map[string]interface{}
	if err := json.Unmarshal(respBody, &llmResult); err != nil {
		log.Printf("[ERROR] 解析 LLM 响应失败: %v, 响应体: %s", err, string(respBody))
		http.Error(w, fmt.Sprintf("解析 LLM 响应失败: %v", err), http.StatusBadGateway)
		return
	}

	// 检查是否有工具调用
	if choices, ok := llmResult["choices"].([]interface{}); ok && len(choices) > 0 {
		if choice, ok := choices[0].(map[string]interface{}); ok {
			if finishReason, ok := choice["finish_reason"].(string); ok && finishReason == "tool_calls" {
				log.Printf("[DEBUG] 检测到工具调用")
				// 有工具调用
				message := choice["message"].(map[string]interface{})
				toolCalls := message["tool_calls"].([]interface{})

				// 构造工具调用缓冲区
				buffer := stream.NewToolCallBuffer()
				for _, tc := range toolCalls {
					tcMap := tc.(map[string]interface{})
					index := int(tcMap["index"].(float64))
					function := tcMap["function"].(map[string]interface{})
					name := function["name"].(string)
					args := function["arguments"].(string)

					var argsBuilder strings.Builder
					argsBuilder.WriteString(args)
					buffer.Calls[index] = &stream.BufferedToolCall{
						Name:      name,
						ID:        tcMap["id"].(string),
						Type:      "function",
						Arguments: argsBuilder,
						Complete:  true,
					}
					log.Printf("[DEBUG] 工具调用: %s, 参数: %s", name, args)
				}

				// 执行工具调用
				toolMessages := p.executeToolCalls(buffer, sess)

				// 继续对话
				if len(toolMessages) > 0 {
					log.Printf("[DEBUG] 工具执行完成，继续对话，深度: %d", depth+1)
					newMessages := reqBody["messages"].([]interface{})
					
					// 添加 assistant 消息（包含 tool_calls）
					assistantMessage := map[string]interface{}{
						"role": "assistant",
						"tool_calls": toolCalls,
					}
					newMessages = append(newMessages, assistantMessage)
					
					// 添加工具响应消息
					for _, msg := range toolMessages {
						newMessages = append(newMessages, msg)
					}
					reqBody["messages"] = newMessages

					// 递归调用，增加深度限制
					p.handleNonStreamingRequestWithDepth(w, r, reqBody, sess, depth+1, maxDepth)
					return
				}
			}
		}
	}

	// 没有工具调用或达到最大深度，直接返回
	log.Printf("[DEBUG] 无工具调用或处理完成，返回响应")
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(llmResult)
}

// executeToolCalls 执行工具调用
func (p *Proxy) executeToolCalls(buffer *stream.ToolCallBuffer, sess *session.Session) []map[string]interface{} {
	log.Printf("执行工具调用，共 %d 个工具", len(buffer.Calls))

	toolMessages := make([]map[string]interface{}, 0, len(buffer.Calls))

	for index, buffered := range buffer.Calls {
		log.Printf("工具 %d: %s, 参数: %s", index, buffered.Name, buffered.Arguments)

		// 解析参数
		var args map[string]interface{}
		if err := json.Unmarshal([]byte(buffered.Arguments.String()), &args); err != nil {
			log.Printf("解析参数失败: %v", err)
			continue
		}

		// 还原占位符
		restoredArgs := p.piHandler.RestoreArgs(args, func(placeholder string) (string, bool) {
			return sess.Restore(placeholder)
		})

		log.Printf("还原后的参数: %+v", restoredArgs)

		// 执行工具
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		result, err := p.toolReg.Execute(ctx, buffered.Name, restoredArgs.(map[string]interface{}))
		cancel()

		if err != nil {
			log.Printf("工具执行失败: %v", err)
			result = map[string]interface{}{
				"error": err.Error(),
			}
		}

		log.Printf("工具结果: %+v", result)

		// 序列化结果
		resultJSON, _ := json.Marshal(result)
		resultStr := string(resultJSON)

		// 检测语言
		language := "zh"
		if !containsChinese(resultStr) {
			language = "en"
		}

		// 遮罩结果
		maskedResult, maskMeta, err := p.piHandler.Mask(resultStr, language)
		if err != nil {
			log.Printf("遮罩结果失败: %v", err)
			maskedResult = resultStr
		} else {
			// 提取新占位符
			p.extractPlaceholders(resultStr, maskedResult, maskMeta, sess)
		}

		log.Printf("遮罩后的结果: %s", maskedResult)

		// 构造工具响应消息
		toolMessage := map[string]interface{}{
			"role":        "tool",
			"tool_call_id": buffered.ID,
			"content":     maskedResult,
		}

		toolMessages = append(toolMessages, toolMessage)
	}

	return toolMessages
}

// continueConversation 继续对话（流式）
func (p *Proxy) continueConversation(w http.ResponseWriter, r *http.Request, reqBody map[string]interface{}, toolMessages []map[string]interface{}, sess *session.Session, flusher http.Flusher) {
	// 添加工具消息到历史
	newMessages := reqBody["messages"].([]interface{})
	for _, msg := range toolMessages {
		newMessages = append(newMessages, msg)
	}
	reqBody["messages"] = newMessages

	// 调用 LLM 继续生成
	body, _ := json.Marshal(reqBody)
	llmReq, _ := http.NewRequest("POST", p.config.LLMURL+"/v1/chat/completions", bytes.NewReader(body))
	llmReq.Header = r.Header.Clone()
	llmReq.Header.Set("Content-Type", "application/json")

	llmResp, err := p.httpClient.Do(llmReq)
	if err != nil {
		log.Printf("继续对话失败: %v", err)
		return
	}
	defer llmResp.Body.Close()

	// 流式转发响应
	buffer := stream.NewToolCallBuffer()
	scanner := bufio.NewScanner(llmResp.Body)

	for scanner.Scan() {
		line := scanner.Text()

		// 解析 chunk
		chunk, err := stream.ParseChunk(line)
		if err != nil {
			log.Printf("解析 chunk 失败: %v", err)
			continue
		}
		if chunk == nil {
			fmt.Fprintf(w, "%s\n\n", line)
			flusher.Flush()
			continue
		}

		// 递归处理工具调用
		if len(chunk.Choices) > 0 && len(chunk.Choices[0].Delta.ToolCalls) > 0 {
			buffer.AddChunk(chunk)

			if buffer.IsComplete(chunk.Choices[0].FinishReason) {
				nestedToolMessages := p.executeToolCalls(buffer, sess)
				buffer.Clear()

				if len(nestedToolMessages) > 0 {
					p.continueConversation(w, r, reqBody, nestedToolMessages, sess, flusher)
					return
				}
			}
		} else {
			serialized, _ := stream.SerializeChunk(chunk)
			fmt.Fprint(w, serialized)
			flusher.Flush()
		}
	}
}

// containsChinese 检查是否包含中文
func containsChinese(s string) bool {
	for _, r := range s {
		if r >= 0x4e00 && r <= 0x9fff {
			return true
		}
	}
	return false
}
