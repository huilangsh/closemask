package proxy

import (
	"bufio"
	"bytes"
	"context"
	"crypto/rand"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"

	"closemask/internal/pii"
	"closemask/internal/session"
	"closemask/internal/storage"
	"closemask/internal/stream"
	"closemask/internal/tools"
)

// 常量定义
const (
	maxRequestSize        = 10 << 20 // 10MB 请求体大小限制
	maxResponseSize       = 10 << 20 // 10MB 响应体大小限制
	maxScannerBufferSize  = 1 << 20  // 1MB SSE 行缓冲区
	maxToolCallDepth      = 10       // 工具调用最大递归深度
	defaultHTTPTimeout    = 60 * time.Second
	toolExecTimeout       = 30 * time.Second
	sessionCleanupInterval = 5 * time.Minute
	defaultSessionTTL     = 2 * time.Hour
	defaultMessageTTL     = 24 * time.Hour
	piiRedactLen          = 4 // PII 日志脱敏保留的字符数
)

// Config 代理配置
type Config struct {
	OneAIFWURL         string `json:"oneaifw_url"`
	LLMURL             string `json:"llm_url"`
	Port               int    `json:"port"`
	StorageType        string `json:"storage_type"`        // "memory", "redis", or "layered"
	RedisAddr          string `json:"redis_addr"`
	RedisPassword      string `json:"redis_password"`      // Redis 密码
	DataDir            string `json:"data_dir"`            // 磁盘存储目录（layered 模式）
	MessageTTL         string `json:"message_ttl"`         // 消息保留时长
	SessionTTL         string `json:"session_ttl"`         // 会话 TTL
	MaxMessagesPerSession int `json:"max_messages_per_session"` // 单会话最大消息数
	APIKey             string `json:"api_key"`             // API 认证密钥
}

// Proxy 代理中间件
type Proxy struct {
	config        *Config
	piHandler     *pii.PIIHandler
	sessMgr       *session.SessionManager
	storage       storage.Storage
	toolReg       *tools.ToolRegistry
	httpClient    *http.Client
	messageIdxMap map[string]int // sessionID -> 消息索引（按会话隔离）
	msgIdxMutex   sync.Mutex
	apiKey        string // API 认证密钥
}

// redactPII 脱敏 PII 值，仅保留前后各 N 个字符
func redactPII(value string) string {
	if len(value) <= piiRedactLen*2 {
		return "****"
	}
	return value[:piiRedactLen] + "****" + value[len(value)-piiRedactLen:]
}

// generateSessionID 生成随机 session ID
func generateSessionID() string {
	b := make([]byte, 16)
	_, _ = rand.Read(b)
	return fmt.Sprintf("%x-%x-%x-%x-%x", b[0:4], b[4:6], b[6:8], b[8:10], b[10:])
}

// truncateLog 截断日志字符串，防止过长
func truncateLog(v interface{}, maxLen int) string {
	s := fmt.Sprintf("%+v", v)
	if len(s) > maxLen {
		s = s[:maxLen] + "...(truncated)"
	}
	return s
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
		if config.RedisAddr == "" {
			log.Fatalf("Redis 存储模式需要配置 redis_addr")
		}
		log.Printf("使用 Redis 存储模式")
		var storErr error
		stor, storErr = storage.NewRedisStorage(config.RedisAddr, config.RedisPassword, messageTTL, sessionTTL)
		if storErr != nil {
			log.Fatalf("Redis 存储初始化失败: %v", storErr)
		}
	default:
		log.Printf("使用内存存储模式")
		stor = storage.NewMemoryStorage(messageTTL, sessionTTL)
	}

	p := &Proxy{
		config:        config,
		piHandler:     piHandler,
		sessMgr:       session.NewSessionManager(sessionTTL),
		storage:       stor,
		toolReg:       tools.NewToolRegistry(),
		messageIdxMap: make(map[string]int),
		apiKey:        config.APIKey,
		httpClient: &http.Client{
			Timeout: defaultHTTPTimeout,
		},
	}

	return p
}

// Start 启动代理服务
func (p *Proxy) Start() error {
	mux := http.NewServeMux()

	// 认证中间件（如果配置了 API Key）
	var handler http.Handler = mux
	if p.apiKey != "" {
		handler = p.authMiddleware(mux)
	}

	// 主代理端点（仅 POST）
	mux.HandleFunc("/v1/chat/completions", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
			return
		}
		p.handleChatCompletions(w, r)
	})

	// 健康检查（检查依赖服务状态）
	mux.HandleFunc("/health", p.handleHealth)

	// 工具列表
	mux.HandleFunc("/tools", p.handleTools)

	addr := fmt.Sprintf(":%d", p.config.Port)
	log.Printf("代理服务启动在 %s", addr)

	server := &http.Server{
		Addr:    addr,
		Handler: handler,
	}

	// 优雅关闭：捕获中断信号
	go func() {
		// 这里可以监听 SIGINT/SIGTERM 进行优雅关闭
		// 暂时简化处理
	}()

	return server.ListenAndServe()
}

// authMiddleware API Key 认证中间件
func (p *Proxy) authMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// /health 不需要认证
		if r.URL.Path == "/health" {
			next.ServeHTTP(w, r)
			return
		}

		apiKey := r.Header.Get("Authorization")
		if apiKey == "" {
			apiKey = r.URL.Query().Get("api_key")
		}

		// 支持 Bearer token 格式
		if strings.HasPrefix(apiKey, "Bearer ") {
			apiKey = strings.TrimPrefix(apiKey, "Bearer ")
		}

		if apiKey != p.apiKey {
			http.Error(w, "Unauthorized", http.StatusUnauthorized)
			return
		}

		next.ServeHTTP(w, r)
	})
}

// handleHealth 处理健康检查
func (p *Proxy) handleHealth(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	// 检查依赖服务可用性
	status := "OK"
	code := http.StatusOK

	// 检查 AIFW 连通性
	aifwCheck := p.checkAIFWHealth()
	if !aifwCheck {
		status = "DEGRADED: AIFW unavailable"
		code = http.StatusServiceUnavailable
	}

	// 检查 LLM 连通性
	llmCheck := p.checkLLMHealth()
	if !llmCheck {
		if code == http.StatusOK {
			status = "DEGRADED: LLM unavailable"
			code = http.StatusServiceUnavailable
		} else {
			status += "; LLM unavailable"
		}
	}

	body := []byte(status)
	w.Header().Set("Content-Length", strconv.Itoa(len(body)))
	w.WriteHeader(code)
	w.Write(body)
}

// checkAIFWHealth 检查 AIFW 服务健康
func (p *Proxy) checkAIFWHealth() bool {
	client := &http.Client{Timeout: 3 * time.Second}
	resp, err := client.Get(p.config.OneAIFWURL + "/health")
	if err != nil {
		return false
	}
	defer resp.Body.Close()
	return resp.StatusCode == http.StatusOK
}

// checkLLMHealth 检查 LLM 服务健康
func (p *Proxy) checkLLMHealth() bool {
	client := &http.Client{Timeout: 3 * time.Second}
	resp, err := client.Get(p.config.LLMURL + "/health")
	if err != nil {
		return false
	}
	defer resp.Body.Close()
	return resp.StatusCode == http.StatusOK
}

// handleTools 处理工具列表
func (p *Proxy) handleTools(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}
	tools := p.toolReg.List()
	data, err := json.Marshal(tools)
	if err != nil {
		log.Printf("序列化工具列表失败: %v", err)
		http.Error(w, "Internal server error", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Content-Length", strconv.Itoa(len(data)))
	w.Write(data)
}

// getNextMessageIndex 获取下一个消息索引（按会话隔离）
func (p *Proxy) getNextMessageIndex(sessionID string) int {
	p.msgIdxMutex.Lock()
	defer p.msgIdxMutex.Unlock()
	p.messageIdxMap[sessionID]++
	return p.messageIdxMap[sessionID]
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
		sessionID = generateSessionID() // 生成随机 UUID
	}

	// 获取会话
	sess := p.sessMgr.GetOrCreate(sessionID)
	maskMetaMgr := sess.GetMaskMetaManager()

	// 刷新会话 TTL（存储层）
	if err := p.storage.TouchSession(ctx, sessionID); err != nil {
		log.Printf("刷新会话 TTL 失败: %v", err)
	}

	// 解析请求（限制请求体大小）
	r.Body = http.MaxBytesReader(w, r.Body, maxRequestSize)
	var reqBody map[string]interface{}
	if err := json.NewDecoder(r.Body).Decode(&reqBody); err != nil {
		http.Error(w, "Invalid request body", http.StatusBadRequest)
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
					msgIdx := p.getNextMessageIndex(sessionID)
					maskMetaMgr.Add(msgIdx, language, maskMeta)
					log.Printf("消息 %d 已遮罩 (session=%s)", msgIdx, sessionID[:min(8, len(sessionID))])

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
				log.Printf("添加占位符映射: %s -> %s", pii.Placeholder, redactPII(pii.Value))
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
		log.Printf("[ERROR] 调用 LLM 失败: %v", err)
		http.Error(w, "LLM service unavailable", http.StatusBadGateway)
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
	scanner.Buffer(make([]byte, 0, maxScannerBufferSize), maxScannerBufferSize)

	// 内容缓冲区：累积内容，在 [DONE] 时一次性还原占位符
	var contentBuffer strings.Builder

	for scanner.Scan() {
		line := scanner.Text()

		// 检查是否是真正的 [DONE] — 精确匹配
		if strings.TrimSpace(line) == "data: [DONE]" {
			// [DONE] - 发送累积的内容（还原后），然后发送 [DONE]
			if contentBuffer.Len() > 0 {
				restored := pii.RestoreAll(contentBuffer.String(), func(placeholder string) (string, bool) {
					return sess.Restore(placeholder)
				})
				finalChunk := &stream.Chunk{
					ID:      fmt.Sprintf("chatcmpl-restored-%d", time.Now().UnixNano()),
					Object:  "chat.completion.chunk",
					Created: time.Now().Unix(),
					Model:   "closemask-proxy",
					Choices: []stream.Choice{{
						Index:        0,
						Delta:        &stream.Delta{Content: restored},
						FinishReason: &[]string{"stop"}[0],
					}},
				}
				serialized, _ := stream.SerializeChunk(finalChunk)
				fmt.Fprint(w, serialized)
				flusher.Flush()
				contentBuffer.Reset()
			}
			// 发送 [DONE]
			fmt.Fprintf(w, "data: [DONE]\n\n")
			flusher.Flush()
			continue
		}

		// 跳过空行（SSE 分隔符）
		if line == "" {
			continue
		}

		// 解析 chunk
		chunk, err := stream.ParseChunk(line)
		if err != nil {
			log.Printf("解析 chunk 失败: %v", err)
			continue
		}
		if chunk == nil {
			// 非数据行，跳过
			continue
		}

		// 检查是否有工具调用
		if len(chunk.Choices) > 0 && len(chunk.Choices[0].Delta.ToolCalls) > 0 {
			log.Printf("[STREAM] Tool call detected")
			// 添加到缓冲区
			if err := buffer.AddChunk(chunk); err != nil {
				log.Printf("添加工具调用 chunk 失败: %v", err)
				continue
			}

			// 检查是否完整
			if buffer.IsComplete(chunk.Choices[0].FinishReason) {
				// 执行工具调用
				toolMessages := p.executeToolCalls(buffer, sess)
				buffer.Clear()

				// 将工具结果发送给 LLM 继续生成
				if len(toolMessages) > 0 {
					p.continueConversation(w, r, reqBody, toolMessages, sess, flusher, 0)
				}
			}
		} else {
			// 非工具调用：累积内容到缓冲区，在 [DONE] 时统一发送
			if len(chunk.Choices) > 0 && chunk.Choices[0].Delta != nil && chunk.Choices[0].Delta.Content != "" {
				contentBuffer.WriteString(chunk.Choices[0].Delta.Content)
			}
		}
	}
}

// handleNonStreamingRequest 处理非流式请求
// handleNonStreamingRequest 处理非流式请求，支持工具调用
// maxIterations: 最大迭代次数，防止无限循环
func (p *Proxy) handleNonStreamingRequest(w http.ResponseWriter, r *http.Request, reqBody map[string]interface{}, sess *session.Session) {
	p.handleNonStreamingRequestWithDepth(w, r, reqBody, sess, 0, maxToolCallDepth)
}

// handleNonStreamingRequestWithDepth 处理非流式请求，带递归深度限制
func (p *Proxy) handleNonStreamingRequestWithDepth(w http.ResponseWriter, r *http.Request, reqBody map[string]interface{}, sess *session.Session, depth int, maxDepth int) {
	// 检查递归深度
	if depth >= maxDepth {
		log.Printf("[ERROR] 达到最大递归深度 %d，停止处理", maxDepth)
		http.Error(w, "Max tool call depth exceeded", http.StatusInternalServerError)
		return
	}
	
	// 准备转发请求
	body, _ := json.Marshal(reqBody)
	
	llmReq, _ := http.NewRequest("POST", p.config.LLMURL+"/v1/chat/completions", bytes.NewReader(body))
	llmReq.Header = r.Header.Clone()
	llmReq.Header.Set("Content-Type", "application/json")

	// 调用 LLM
	llmResp, err := p.httpClient.Do(llmReq)
	if err != nil {
		log.Printf("[ERROR] 调用 LLM 失败: %v", err)
		http.Error(w, "LLM service unavailable", http.StatusBadGateway)
		return
	}
	defer llmResp.Body.Close()

	// 读取并解析响应（限制大小）
	respBody, err := io.ReadAll(io.LimitReader(llmResp.Body, maxResponseSize))
	if err != nil {
		log.Printf("[ERROR] 读取 LLM 响应失败: %v", err)
		http.Error(w, "Failed to read LLM response", http.StatusBadGateway)
		return
	}

	var llmResult map[string]interface{}
	if err := json.Unmarshal(respBody, &llmResult); err != nil {
		log.Printf("[ERROR] 解析 LLM 响应失败: %v", err)
		http.Error(w, "Invalid LLM response", http.StatusBadGateway)
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

					var argsBuf strings.Builder
					argsBuf.WriteString(args)
					buffer.Calls[index] = &stream.BufferedToolCall{
						Name:      name,
						ID:        tcMap["id"].(string),
						Type:      "function",
						Arguments: argsBuf,
						Complete:  true,
					}
					argsPreview := truncateLog(args, 200)
					log.Printf("[DEBUG] 工具调用: %s, 参数: %s", name, argsPreview)
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

	// 还原响应中的占位符
	if choices, ok := llmResult["choices"].([]interface{}); ok {
		for _, c := range choices {
			if choice, ok := c.(map[string]interface{}); ok {
				if message, ok := choice["message"].(map[string]interface{}); ok {
					if content, ok := message["content"].(string); ok && content != "" {
						restored := pii.RestoreAll(content, func(placeholder string) (string, bool) {
							return sess.Restore(placeholder)
						})
						if restored != content {
							log.Printf("[DEBUG] 还原响应占位符: %d -> %d 字符", len(content), len(restored))
						}
						message["content"] = restored
					}
				}
			}
		}
	}

	respData, err := json.Marshal(llmResult)
	if err != nil {
		log.Printf("[ERROR] 序列化响应失败: %v", err)
		http.Error(w, "Internal server error", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Content-Length", strconv.Itoa(len(respData)))
	w.Write(respData)
}

// executeToolCalls 执行工具调用
func (p *Proxy) executeToolCalls(buffer *stream.ToolCallBuffer, sess *session.Session) []map[string]interface{} {
	log.Printf("执行工具调用，共 %d 个工具", len(buffer.Calls))

	toolMessages := make([]map[string]interface{}, 0, len(buffer.Calls))

	for index, buffered := range buffer.Calls {
		log.Printf("工具 %d: %s, 参数: %s", index, buffered.Name, buffered.Arguments.String())

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

		log.Printf("还原后的参数: %s", truncateLog(restoredArgs, 200))

		// 执行工具
		ctx, cancel := context.WithTimeout(context.Background(), toolExecTimeout)
		result, err := p.toolReg.Execute(ctx, buffered.Name, restoredArgs.(map[string]interface{}))
		cancel()

		if err != nil {
			log.Printf("工具执行失败: %v", err)
			result = map[string]interface{}{
				"error": err.Error(),
			}
		}

		log.Printf("工具结果: %s", truncateLog(result, 200))

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

// continueConversation 继续对话（流式），带递归深度限制
func (p *Proxy) continueConversation(w http.ResponseWriter, r *http.Request, reqBody map[string]interface{}, toolMessages []map[string]interface{}, sess *session.Session, flusher http.Flusher, depth int) {
	// 递归深度检查
	if depth >= maxToolCallDepth {
		log.Printf("[ERROR] 流式工具调用达到最大深度 %d", maxToolCallDepth)
		// 发送错误消息然后 [DONE]
		errChunk := &stream.Chunk{
			ID:      fmt.Sprintf("chatcmpl-error-%d", time.Now().UnixNano()),
			Object:  "chat.completion.chunk",
			Created: time.Now().Unix(),
			Model:   "closemask-proxy",
			Choices: []stream.Choice{{
				Index: 0,
				Delta: &stream.Delta{Content: "[CloseMask] Maximum tool call depth exceeded"},
			}},
		}
		serialized, _ := stream.SerializeChunk(errChunk)
		fmt.Fprint(w, serialized)
		flusher.Flush()
		fmt.Fprintf(w, "data: [DONE]\n\n")
		flusher.Flush()
		return
	}

	log.Printf("[STREAM] continueConversation called with %d tool messages, depth=%d", len(toolMessages), depth)
	// 添加工具消息到历史（创建新切片避免修改原始请求体）
	newMessages := make([]interface{}, len(reqBody["messages"].([]interface{})))
	copy(newMessages, reqBody["messages"].([]interface{}))
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
		log.Printf("[ERROR] 继续对话失败: %v", err)
		// 发送错误消息和 [DONE]，避免客户端挂起
		errChunk := &stream.Chunk{
			ID:      fmt.Sprintf("chatcmpl-error-%d", time.Now().UnixNano()),
			Object:  "chat.completion.chunk",
			Created: time.Now().Unix(),
			Model:   "closemask-proxy",
			Choices: []stream.Choice{{
				Index: 0,
				Delta: &stream.Delta{Content: "[CloseMask] LLM service unavailable"},
			}},
		}
		serialized, _ := stream.SerializeChunk(errChunk)
		fmt.Fprint(w, serialized)
		flusher.Flush()
		fmt.Fprintf(w, "data: [DONE]\n\n")
		flusher.Flush()
		return
	}
	defer llmResp.Body.Close()

	// 流式转发响应
	buffer := stream.NewToolCallBuffer()
	scanner := bufio.NewScanner(llmResp.Body)
	scanner.Buffer(make([]byte, 0, maxScannerBufferSize), maxScannerBufferSize)

	// 内容缓冲区
	var contentBuffer strings.Builder

	for scanner.Scan() {
		line := scanner.Text()

		// 检查是否是真正的 [DONE] — 精确匹配
		if strings.TrimSpace(line) == "data: [DONE]" {
			// [DONE] - 发送累积的内容
			if contentBuffer.Len() > 0 {
				restored := pii.RestoreAll(contentBuffer.String(), func(placeholder string) (string, bool) {
					return sess.Restore(placeholder)
				})
				finalChunk := &stream.Chunk{
					ID:      fmt.Sprintf("chatcmpl-restored-%d", time.Now().UnixNano()),
					Object:  "chat.completion.chunk",
					Created: time.Now().Unix(),
					Model:   "closemask-proxy",
					Choices: []stream.Choice{{
						Index: 0,
						Delta: &stream.Delta{Content: restored},
					}},
				}
				serialized, _ := stream.SerializeChunk(finalChunk)
				fmt.Fprint(w, serialized)
				flusher.Flush()
			}
			fmt.Fprintf(w, "data: [DONE]\n\n")
			flusher.Flush()
			continue
		}

		// 跳过空行
		if line == "" {
			continue
		}

		// 解析 chunk
		chunk, err := stream.ParseChunk(line)
		if err != nil {
			log.Printf("解析 chunk 失败: %v", err)
			continue
		}
		if chunk == nil {
			continue
		}

		// 递归处理工具调用
		if len(chunk.Choices) > 0 && len(chunk.Choices[0].Delta.ToolCalls) > 0 {
			if err := buffer.AddChunk(chunk); err != nil {
				log.Printf("添加工具调用 chunk 失败: %v", err)
				continue
			}

			if buffer.IsComplete(chunk.Choices[0].FinishReason) {
				nestedToolMessages := p.executeToolCalls(buffer, sess)
				buffer.Clear()

				if len(nestedToolMessages) > 0 {
					p.continueConversation(w, r, reqBody, nestedToolMessages, sess, flusher, depth+1)
					return
				}
			}
		} else {
			// 累积内容，在 [DONE] 时统一发送
			if len(chunk.Choices) > 0 && chunk.Choices[0].Delta != nil && chunk.Choices[0].Delta.Content != "" {
				contentBuffer.WriteString(chunk.Choices[0].Delta.Content)
			}
		}
	}
}

// containsChinese 检查是否包含中文字符（覆盖 CJK 基本区 + 扩展区 + 常用标点）
func containsChinese(s string) bool {
	for _, r := range s {
		// CJK Unified Ideographs (基本区 + 扩展 A-F)
		if (r >= 0x4E00 && r <= 0x9FFF) || (r >= 0x3400 && r <= 0x4DBF) ||
			(r >= 0x20000 && r <= 0x2A6DF) || (r >= 0x2A700 && r <= 0x2B73F) ||
			(r >= 0x2B740 && r <= 0x2B81F) || (r >= 0x2B820 && r <= 0x2CEAF) ||
			(r >= 0x2CEB0 && r <= 0x2EBEF) {
			return true
		}
		// 中文标点符号
		if (r >= 0x3000 && r <= 0x303F) || (r >= 0xFF00 && r <= 0xFFEF) {
			return true
		}
	}
	return false
}
