package proxy

import (
	"bufio"
	"bytes"
	"context"
	"crypto/rand"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"time"

	"agent-pii-proxy/internal/pii"
	"agent-pii-proxy/internal/session"
	"agent-pii-proxy/internal/storage"
	"agent-pii-proxy/internal/stream"
	"agent-pii-proxy/internal/tools"
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
	OneAIFWURL             string `json:"oneaifw_url"`
	LLMURL                 string `json:"llm_url"`
	Port                   int    `json:"port"`
	StorageType            string `json:"storage_type"`        // "memory", "redis", "layered", "disk"
	RedisAddr              string `json:"redis_addr"`
	RedisPassword          string `json:"redis_password"`      // Redis 密码
	DataDir                string `json:"data_dir"`            // 磁盘存储目录（layered/disk 模式）
	MessageTTL             string `json:"message_ttl"`         // 消息保留时长
	SessionTTL             string `json:"session_ttl"`         // 会话 TTL
	MaxMessagesPerSession  int    `json:"max_messages_per_session"` // 单会话最大消息数
	APIKey                 string `json:"api_key"`             // CloseMask 自身的访问认证密钥
	MaskFailStrategy       string `json:"mask_fail_strategy"`  // "block", "redact", "passthrough"
	MaxPlaceholdersPerSession int `json:"max_placeholders_per_session"` // 单会话最大占位符数
	LocalMaskLevel         string `json:"local_mask_level"`    // "strict", "aggressive", "off"
	PIIEngine              string `json:"pii_engine"`          // "auto", "builtin", "oneaifw"
	LogToFile              bool   `json:"log_to_file"`         // 是否将日志写入文件（默认只输出终端）
	PlaceholderHashLength  int    `json:"placeholder_hash_length"` // 占位符哈希长度（6或8，默认6）
	PlaceholderHMACKey     string `json:"placeholder_hmac_key"`    // HMAC密钥（空则用plain sha256）
	LogLevel               string `json:"log_level"`               // 日志级别: "quiet", "info", "debug"
}

// PII 引擎状态
type piiEngineStatus struct {
	name       string // 引擎名称
	available  bool   // 是否可用
	types      []string // 可检测的 PII 类型
	reason     string // 不可用原因
}

// Proxy 代理中间件
type Proxy struct {
	config           *Config
	piHandler        *pii.PIIHandler
	localMasker      *pii.LocalMasker
	builtInPII       *pii.BuiltInPIIDetector
	sessMgr          *session.SessionManager
	storage          storage.Storage
	toolReg          *tools.ToolRegistry
	httpClient       *http.Client
	healthClient     *http.Client       // 健康检查专用客户端（短超时）
	messageIdxMap    map[string]int     // sessionID -> 消息索引（按会话隔离）
	msgIdxMutex      sync.Mutex
	apiKey           string             // CloseMask 自身的访问认证密钥
	piiEngines       []piiEngineStatus  // PII 引擎状态列表
	aifwAvailable    bool               // OneAIFW 是否可用
	activeEngine     string             // 当前活跃引擎名
	oneaifwProcess   *os.Process        // OneAIFW 子进程（如果由 CloseMask 启动）
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
			LogInfof("⚠️ Redis 存储模式需要配置 redis_addr，降级到内存存储")
			stor = storage.NewMemoryStorage(messageTTL, sessionTTL)
		} else {
			LogInfof("使用 Redis 存储模式")
			var storErr error
			stor, storErr = storage.NewRedisStorage(config.RedisAddr, config.RedisPassword, messageTTL, sessionTTL)
			if storErr != nil {
				LogInfof("⚠️ Redis 存储初始化失败: %v，降级到内存存储", storErr)
				stor = storage.NewMemoryStorage(messageTTL, sessionTTL)
			}
		}
	case "layered":
		dataDir := config.DataDir
		if dataDir == "" {
			dataDir = "./data"
		}
		LogInfof("使用分层存储模式 (数据目录: %s)", dataDir)
		var storErr error
		stor, storErr = storage.NewLayeredStorage(dataDir, messageTTL, sessionTTL)
		if storErr != nil {
			LogInfof("⚠️ 分层存储初始化失败: %v，降级到内存存储", storErr)
			stor = storage.NewMemoryStorage(messageTTL, sessionTTL)
		}
	case "disk":
		dataDir := config.DataDir
		if dataDir == "" {
			dataDir = "./data"
		}
		LogInfof("使用磁盘存储模式 (数据目录: %s)", dataDir)
		var storErr error
		stor, storErr = storage.NewDiskStorage(dataDir, sessionTTL)
		if storErr != nil {
			LogInfof("⚠️ 磁盘存储初始化失败: %v，降级到内存存储", storErr)
			stor = storage.NewMemoryStorage(messageTTL, sessionTTL)
		}
	default:
		LogInfof("使用内存存储模式")
		stor = storage.NewMemoryStorage(messageTTL, sessionTTL)
	}

	// 初始化本地预扫描器
	localMasker := pii.NewLocalMasker(config.LocalMaskLevel)

	// 初始化内置 PII 检测器（开箱即用，不依赖 OneAIFW）
	builtInPII := pii.NewBuiltInPIIDetector()

	// 初始化确定性占位符生成器
	hashLen := config.PlaceholderHashLength
	if hashLen == 0 {
		hashLen = 6
	}
	pii.InitPlaceholderGenerator(hashLen, config.PlaceholderHMACKey)

	// 初始化日志级别
	if config.LogLevel == "" {
		config.LogLevel = "info"
	}
	InitLogger(config.LogLevel)

	// 检测 PII 引擎状态
	var engines []piiEngineStatus
	aifwAvailable := false
	activeEngine := "builtin"

	// 引擎 1: 内置正则凭据检测
	credTypes := []string{"API_KEY", "JWT_TOKEN", "AWS_ACCESS_KEY", "DB_PASSWORD", "BEARER_TOKEN", "CREDENTIAL"}
	if localMasker != nil {
		engines = append(engines, piiEngineStatus{
			name:      "local_masker",
			available: true,
			types:     credTypes,
		})
	}

	// 引擎 2: 内置 PII 检测器
	builtinTypes := []string{"PHONE", "ID_CARD", "EMAIL", "BANK_CARD", "IP_ADDRESS"}
	engines = append(engines, piiEngineStatus{
		name:      "builtin_pii",
		available: true,
		types:     builtinTypes,
	})

	// 引擎 3: OneAIFW（可选增强）
	aifwEngine := piiEngineStatus{
		name:      "oneaifw",
		available: false,
		types:     []string{"ALL (21+ types including NAME, ADDRESS, ORG)"},
		reason:    "未配置或不可达",
	}

	// 根据 pii_engine 配置决定是否尝试 OneAIFW
	piiEngine := config.PIIEngine
	if piiEngine == "" {
		piiEngine = "auto"
	}

	if piiEngine == "auto" || piiEngine == "oneaifw" {
		if config.OneAIFWURL != "" {
			// 检查 OneAIFW 是否已在运行
			if checkOneAIFWRunning(config.OneAIFWURL) {
				aifwEngine.available = true
				aifwEngine.reason = ""
				aifwAvailable = true
				LogInfof("✅ OneAIFW 已在运行 (%s)", config.OneAIFWURL)
			} else if tryStartOneAIFW(config.OneAIFWURL) {
				// 尝试自动启动同目录的 OneAIFW
				aifwEngine.available = true
				aifwEngine.reason = ""
				aifwAvailable = true
				LogInfof("✅ OneAIFW 已自动启动 (%s)", config.OneAIFWURL)
			} else {
				aifwEngine.reason = "服务不可达且无法自动启动"
				LogInfof("⚠️ OneAIFW 不可用 (%s)，使用内置检测", aifwEngine.reason)
			}
		} else {
			aifwEngine.reason = "未配置 oneaifw_url"
			if piiEngine == "oneaifw" {
				LogInfof("⚠️ pii_engine=oneaifw 但未配置 oneaifw_url，降级到内置检测")
			}
		}
	}

	engines = append(engines, aifwEngine)

	// 决定活跃引擎
	if aifwAvailable {
		activeEngine = "oneaifw"
	} else {
		activeEngine = "builtin"
	}

	p := &Proxy{
		config:        config,
		piHandler:     piHandler,
		localMasker:   localMasker,
		builtInPII:    builtInPII,
		sessMgr:       session.NewSessionManager(sessionTTL),
		storage:       stor,
		toolReg:       tools.NewToolRegistry(),
		messageIdxMap: make(map[string]int),
		apiKey:        config.APIKey,
		piiEngines:    engines,
		aifwAvailable: aifwAvailable,
		activeEngine:  activeEngine,
		httpClient: &http.Client{
			Timeout: defaultHTTPTimeout,
		},
		healthClient: &http.Client{
			Timeout: 3 * time.Second,
		},
	}

	return p
}

// HandleChatCompletionsForTest 暴露 handleChatCompletions 供集成测试使用
func (p *Proxy) HandleChatCompletionsForTest(w http.ResponseWriter, r *http.Request) {
	p.handleChatCompletions(w, r)
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
	// 同时注册 /v1/chat/completions 和 /chat/completions，兼容不同 IDE 的路径拼接方式
	chatHandler := func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
			return
		}
		p.handleChatCompletions(w, r)
	}
	mux.HandleFunc("/v1/chat/completions", chatHandler)
	mux.HandleFunc("/chat/completions", chatHandler)

	// 健康检查（检查依赖服务状态）
	mux.HandleFunc("/health", p.handleHealth)

	// 工具列表
	mux.HandleFunc("/tools", p.handleTools)

	// 调试端点：查看所有 session 的占位符映射
	mux.HandleFunc("/debug/sessions", p.handleDebugSessions)

	addr := fmt.Sprintf(":%d", p.config.Port)
	LogInfof("代理服务启动在 %s", addr)

	server := &http.Server{
		Addr:    addr,
		Handler: handler,
	}

	return server.ListenAndServe()
}

// authMiddleware CloseMask 认证中间件
// 使用 X-CloseMask-Key 头进行认证，避免与 LLM 的 Authorization 头冲突
func (p *Proxy) authMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// /health 不需要认证
		if r.URL.Path == "/health" {
			next.ServeHTTP(w, r)
			return
		}

		apiKey := r.Header.Get("X-CloseMask-Key")
		if apiKey == "" {
			apiKey = r.URL.Query().Get("api_key")
		}

		if apiKey != p.apiKey {
			http.Error(w, "Unauthorized", http.StatusUnauthorized)
			return
		}

		next.ServeHTTP(w, r)
	})
}

// checkOneAIFWRunning 检查 OneAIFW 服务是否在运行
func checkOneAIFWRunning(url string) bool {
	client := &http.Client{Timeout: 2 * time.Second}
	resp, err := client.Get(url + "/health")
	if err != nil {
		return false
	}
	defer resp.Body.Close()
	return resp.StatusCode == http.StatusOK
}

// tryStartOneAIFW 尝试自动启动同目录的 OneAIFW
func tryStartOneAIFW(url string) bool {
	// 查找可执行文件
	exePath, err := os.Executable()
	if err != nil {
		return false
	}
	dir := filepath.Dir(exePath)

	// 查找 oneaifw.exe (Windows) 或 oneaifw (Linux/Mac)
	var oneaifwExe string
	if runtime.GOOS == "windows" {
		oneaifwExe = filepath.Join(dir, "oneaifw.exe")
	} else {
		oneaifwExe = filepath.Join(dir, "oneaifw")
	}

	if _, err := os.Stat(oneaifwExe); os.IsNotExist(err) {
		// 尝试 Python 脚本方式
		pyScript := filepath.Join(dir, "oneaifw", "aifw_service.py")
		if _, err := os.Stat(pyScript); os.IsNotExist(err) {
			return false
		}
		// 用 Python 启动
		cmd := exec.Command("python", pyScript, "--port", "8845")
		cmd.Dir = dir
		if err := cmd.Start(); err != nil {
			LogErrorf("自动启动 OneAIFW Python 服务失败: %v", err)
			return false
		}
		LogInfof("🔄 已启动 OneAIFW Python 服务 (PID: %d)", cmd.Process.Pid)
	} else {
		// 用 exe 启动
		cmd := exec.Command(oneaifwExe, "--port", "8845")
		cmd.Dir = dir
		if err := cmd.Start(); err != nil {
			LogErrorf("自动启动 OneAIFW 可执行文件失败: %v", err)
			return false
		}
		LogInfof("🔄 已启动 OneAIFW (PID: %d)", cmd.Process.Pid)
	}

	// 等待就绪（最多 15 秒）
	for i := 0; i < 15; i++ {
		time.Sleep(1 * time.Second)
		if checkOneAIFWRunning(url) {
			return true
		}
	}
	return false
}

// PrintBanner 打印启动横幅
func (p *Proxy) PrintBanner() {
	fmt.Println()
	fmt.Println("╔══════════════════════════════════════════════════════╗")
	fmt.Println("║  CloseMask - AI Agent PII Middleware                 ║")
	fmt.Println("╠══════════════════════════════════════════════════════╣")
	fmt.Println("║                                                      ║")

	// PII 引擎状态
	fmt.Println("║  PII 检测引擎:                                       ║")
	for _, e := range p.piiEngines {
		status := "❌"
		detail := ""
		if e.available {
			status = "✅"
		} else if e.reason != "" {
			detail = " ← " + e.reason
		}
		line := fmt.Sprintf("║  %s %-14s%s", status, e.name, detail)
		// 填充到固定宽度
		for len(line) < 55 {
			line += " "
		}
		line += "║"
		fmt.Println(line)
	}

	// 能力评分
	capability := 0
	allTypes := make(map[string]bool)
	for _, e := range p.piiEngines {
		if e.available {
			capability += 33
			for _, t := range e.types {
				allTypes[t] = true
			}
		}
	}
	if p.aifwAvailable {
		capability = 100
	} else if capability > 80 {
		capability = 80
	}

	barLen := 10
	filled := capability * barLen / 100
	bar := strings.Repeat("█", filled) + strings.Repeat("░", barLen-filled)

	fmt.Println("║                                                      ║")
	capLine := fmt.Sprintf("║  当前检测能力: %s %d%%", bar, capability)
	for len(capLine) < 55 {
		capLine += " "
	}
	capLine += "║"
	fmt.Println(capLine)

	// 升级建议
	if !p.aifwAvailable {
		hint := "║  💡 配置 OneAIFW 可获得人名/组织/地址检测能力         ║"
		fmt.Println(hint)
	}

	fmt.Println("║                                                      ║")
	fmt.Println("║  代理服务: http://localhost:" + fmt.Sprintf("%d", p.config.Port) + "                      ║")
	fmt.Println("║  健康检查: http://localhost:" + fmt.Sprintf("%d", p.config.Port) + "/health             ║")
	fmt.Println("╚══════════════════════════════════════════════════════╝")
	fmt.Println()
}

// handleHealth 处理健康检查
func (p *Proxy) handleHealth(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	// 构建 JSON 响应
	type engineInfo struct {
		Available bool     `json:"available"`
		Types     []string `json:"types,omitempty"`
		Reason    string   `json:"reason,omitempty"`
	}

	type healthResponse struct {
		Status       string                `json:"status"`
		ActiveEngine string                `json:"active_engine"`
		Engines      map[string]engineInfo `json:"pii_engines"`
		UpgradeHints []string              `json:"upgrade_hints,omitempty"`
	}

	resp := healthResponse{
		Status:       "OK",
		ActiveEngine: p.activeEngine,
		Engines:      make(map[string]engineInfo),
	}

	for _, e := range p.piiEngines {
		resp.Engines[e.name] = engineInfo{
			Available: e.available,
			Types:     e.types,
			Reason:    e.reason,
		}
	}

	// 检查 LLM
	llmCheck := p.checkLLMHealth()
	if !llmCheck {
		resp.Status = "DEGRADED"
		resp.UpgradeHints = append(resp.UpgradeHints, "LLM 服务不可达")
	}

	// 升级提示
	if !p.aifwAvailable {
		resp.UpgradeHints = append(resp.UpgradeHints,
			"配置 OneAIFW 可检测人名/组织名/地址: https://github.com/funstory-ai/aifw")
	}

	// 状态码
	code := http.StatusOK
	if resp.Status != "OK" {
		code = http.StatusServiceUnavailable
	}

	w.Header().Set("Content-Type", "application/json")
	data, _ := json.MarshalIndent(resp, "", "  ")
	w.WriteHeader(code)
	w.Write(data)
}

// checkAIFWHealth 检查 AIFW 服务健康
func (p *Proxy) checkAIFWHealth() bool {
	resp, err := p.healthClient.Get(p.config.OneAIFWURL + "/health")
	if err != nil {
		return false
	}
	defer resp.Body.Close()
	return resp.StatusCode == http.StatusOK
}

// buildLLMURL 返回 LLM 请求 URL
// llm_url 必须填完整端点地址（如 https://api.openai.com/v1/chat/completions）
func (p *Proxy) buildLLMURL() string {
	return strings.TrimRight(p.config.LLMURL, "/")
}

// checkLLMHealth 检查 LLM 服务健康
func (p *Proxy) checkLLMHealth() bool {
	resp, err := p.healthClient.Get(p.config.LLMURL + "/health")
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
		LogErrorf("序列化工具列表失败: %v", err)
		http.Error(w, "Internal server error", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Content-Length", strconv.Itoa(len(data)))
	w.Write(data)
}

// handleDebugSessions 调试端点：查看所有 session 的占位符映射
func (p *Proxy) handleDebugSessions(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	type sessionInfo struct {
		SessionID      string            `json:"session_id"`
		PlaceholderCount int             `json:"placeholder_count"`
		MaskMap        map[string]string `json:"mask_map"`
		CreatedAt      string            `json:"created_at"`
		LastAccess     string            `json:"last_access"`
	}

	var sessions []sessionInfo
	p.sessMgr.Range(func(id string, sess *session.Session) {
		sessions = append(sessions, sessionInfo{
			SessionID:       id,
			PlaceholderCount: sess.PlaceholderCount(),
			MaskMap:         sess.GetMaskMap(),
			CreatedAt:       sess.CreatedAt.Format(time.RFC3339),
			LastAccess:      sess.LastAccess.Format(time.RFC3339),
		})
	})

	type debugResponse struct {
		TotalSessions int            `json:"total_sessions"`
		Sessions      []sessionInfo  `json:"sessions"`
	}

	resp := debugResponse{
		TotalSessions: len(sessions),
		Sessions:      sessions,
	}

	w.Header().Set("Content-Type", "application/json")
	data, _ := json.MarshalIndent(resp, "", "  ")
	w.Write(data)
}

// getNextMessageIndex 获取下一个消息索引（按会话隔离）
func (p *Proxy) getNextMessageIndex(sessionID string) int {
	p.msgIdxMutex.Lock()
	defer p.msgIdxMutex.Unlock()
	p.messageIdxMap[sessionID]++
	return p.messageIdxMap[sessionID]
}

// makeRestoreFunc 创建带 storage 回填的还原函数
func (p *Proxy) makeRestoreFunc(sess *session.Session, sessionID string) func(string) (string, bool) {
	return func(placeholder string) (string, bool) {
		if val, ok := sess.Restore(placeholder); ok {
			return val, true
		}
		ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
		defer cancel()
		val, err := p.storage.GetPlaceholder(ctx, sessionID, placeholder)
		if err == nil && val != "" {
			sess.AddPlaceholder(placeholder, val)
			return val, true
		}
		return "", false
	}
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

	// 设置单会话最大占位符数
	if p.config.MaxPlaceholdersPerSession > 0 {
		sess.SetMaxPlaceholders(p.config.MaxPlaceholdersPerSession)
	}

	// 刷新会话 TTL（存储层）
	if err := p.storage.TouchSession(ctx, sessionID); err != nil {
		LogErrorf("刷新会话 TTL 失败: %v", err)
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
	maskLogSID := sessionID[:min(8, len(sessionID))]
	LogInfof("[REQ] session=%s stream=%v PlaceholderCount=%d", maskLogSID, streamReq, sess.PlaceholderCount())
	if messages, ok := reqBody["messages"].([]interface{}); ok {
		for i, msg := range messages {
			if msgMap, ok := msg.(map[string]interface{}); ok {
				if content, ok := msgMap["content"].(string); ok && content != "" {
					original := content
					LogDebugf("≍ msg[%d] role=%s original=%q", i, msgMap["role"], original)

					// 第一步：本地凭据预扫描（API Key/JWT/AWS Key 等）
					localMasked := p.localMasker.Mask(content, func(placeholder, value string) {
						sess.AddPlaceholder(placeholder, value)
						if err := p.storage.SavePlaceholder(ctx, sessionID, placeholder, value); err != nil {
							LogErrorf("保存本地遮罩占位符到存储失败: %v", err)
						}
						LogInfof("≍ %s -> %s (本地凭据, session=%s)", placeholder, redactPII(value), maskLogSID)
					})

					// 第二步：内置 PII 检测（手机号/身份证/邮箱/银行卡等）
					builtInMasked, builtInMeta := p.builtInPII.DetectAndMask(localMasked, func(placeholder, value string) {
						sess.AddPlaceholder(placeholder, value)
						if err := p.storage.SavePlaceholder(ctx, sessionID, placeholder, value); err != nil {
							LogErrorf("保存内置PII占位符到存储失败: %v", err)
						}
						LogInfof("≍ %s -> %s (内置PII, session=%s)", placeholder, redactPII(value), maskLogSID)
					})

					// 处理内置 PII 检测结果
					if builtInMasked != localMasked {
						messages[i].(map[string]interface{})["content"] = builtInMasked

						msgIdx := p.getNextMessageIndex(sessionID)
						language := "en"
						if containsChinese(builtInMasked) {
							language = "zh"
						}
						maskMetaMgr.Add(msgIdx, language, builtInMeta)

						if err := p.storage.SaveMaskMeta(ctx, sessionID, &storage.MaskMeta{
							MessageID: msgIdx,
							Language:  language,
							MaskMeta:  builtInMeta,
						}); err != nil {
							LogErrorf("保存 maskMeta 到存储失败: %v", err)
						}

						_ = p.extractPlaceholders(localMasked, builtInMasked, builtInMeta, sess)
					}

					// 第三步：OneAIFW 遮罩（可选增强）
					masked, maskMeta, err := p.piHandler.Mask(builtInMasked, "auto")
					if err != nil {
						if !p.piHandler.IsCircuitOpen() {
							LogInfof("OneAIFW 遮罩失败（已由内置检测兜底）: %v", err)
						}
						if p.config.OneAIFWURL != "" && p.config.MaskFailStrategy == "block" {
							LogInfof("[INFO] 内置PII检测已兜底，请求继续处理")
						}
					} else if masked != builtInMasked {
						// OneAIFW 遮罩成功，需要重映射占位符
						remasked := p.remapOneAIFWPlaceholders(masked, maskMeta)
						messages[i].(map[string]interface{})["content"] = remasked

						msgIdx := p.getNextMessageIndex(sessionID)
						aifwLanguage := "en"
						if containsChinese(builtInMasked) {
							aifwLanguage = "zh"
						}
						maskMetaMgr.Add(msgIdx, aifwLanguage, maskMeta)

						if err := p.storage.SaveMaskMeta(ctx, sessionID, &storage.MaskMeta{
							MessageID: msgIdx,
							Language:  aifwLanguage,
							MaskMeta:  maskMeta,
						}); err != nil {
							LogErrorf("保存 maskMeta 到存储失败: %v", err)
						}

						placeholders := p.extractPlaceholders(builtInMasked, masked, maskMeta, sess)
						for placeholder, value := range placeholders {
							if err := p.storage.SavePlaceholder(ctx, sessionID, placeholder, value); err != nil {
								LogErrorf("保存占位符到存储失败: %v", err)
							}
							LogInfof("≍ %s -> %s (OneAIFW, session=%s)", placeholder, redactPII(value), maskLogSID)
						}
					}

					// 汇总日志
					finalContent, _ := msgMap["content"].(string)
					if finalContent != original {
						LogInfof("≍ msg[%d] 遮罩完成: %d字 -> %d字 (session=%s)", i, len(original), len(finalContent), maskLogSID)
					}
				}
			}
		}
	}

	// 注入系统提示：告知 LLM 遇到占位符正常处理
	// 只要 session 中存在占位符就注入（不管本轮是否有新遮罩，因为历史消息中可能有占位符）
	if sess.PlaceholderCount() > 0 {
		p.injectSystemPrompt(reqBody, sess)
	}

	// 转发请求到 LLM
	LogInfof("[DISPATCH] session=%s stream=%v -> %s", maskLogSID, streamReq, map[bool]string{true: "streaming", false: "non-streaming"}[streamReq])
	if streamReq {
		p.handleStreamingRequest(w, r, reqBody, sess)
	} else {
		p.handleNonStreamingRequest(w, r, reqBody, sess)
	}
}

// injectSystemPrompt 注入系统提示，告知 LLM 遇到占位符时正常处理
func (p *Proxy) injectSystemPrompt(reqBody map[string]interface{}, sess *session.Session) {
	// 策略：用 ${} 模板变量格式（LLM 不会当作 HTML 标签忽略），不解释占位符含义
	// 关键：强调 LLM 必须在输出中包含这些变量，把它们当作真实值来使用
	const systemHint = "IMPORTANT: The conversation contains template variables in the format ${TYPE_hash} (e.g., ${PHONE_a1b2c3}, ${CRED_f4e5d6}). Each variable represents a real value that you MUST include in your response whenever you would naturally use that value. Rules:\n1. When you need to output, repeat, or reference a value represented by a variable, write the variable EXACTLY as-is (e.g., ${PHONE_a1b2c3}). Do NOT omit it, replace it with a description, or leave it blank.\n2. Do not modify the variable format — keep the ${}, the underscore, and the hash characters.\n3. Treat the variable as if it were the actual value. For example, if ${PHONE_a1b2c3} represents a phone number and the user asks 'what is my phone?', your response should be 'Your phone is ${PHONE_a1b2c3}.'\n4. Never mention the existence of these variables or explain their purpose to the user."

	messages, ok := reqBody["messages"].([]interface{})
	if !ok {
		return
	}

	// 检查是否已有 system 消息
	hasSystem := false
	for _, msg := range messages {
		if msgMap, ok := msg.(map[string]interface{}); ok {
			if role, ok := msgMap["role"].(string); ok && role == "system" {
				// 追加到现有 system 消息
				if content, ok := msgMap["content"].(string); ok {
					msgMap["content"] = content + "\n\n" + systemHint
					hasSystem = true
					break
				}
			}
		}
	}

	// 没有现有 system 消息，在开头插入
	if !hasSystem {
		systemMsg := map[string]interface{}{
			"role":    "system",
			"content": systemHint,
		}
		reqBody["messages"] = append([]interface{}{systemMsg}, messages...)
	}
}

// extractPlaceholders 从遮罩结果中提取占位符并保存映射
// OneAIFW 返回的占位符格式 ${PHONE_0} 需要重映射为确定性占位符 ${PHONE_hash6}
func (p *Proxy) extractPlaceholders(original, masked, maskMeta string, sess *session.Session) map[string]string {
	placeholders := make(map[string]string)

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
		for _, piiItem := range meta.PII {
			if piiItem.Placeholder != "" && piiItem.Value != "" {
				// 生成确定性占位符
				newPlaceholder := pii.GeneratePlaceholder(piiItem.Type, piiItem.Value)

				// 如果 OneAIFW 的占位符与确定性占位符不同，需要在 masked 文本中替换
				if piiItem.Placeholder != newPlaceholder {
					// 替换在 session 中的映射
					// (extractPlaceholders 的调用方会在 masked 文本中做替换)
					LogDebugf("OneAIFW重映射: %s -> %s (value=%s)", piiItem.Placeholder, newPlaceholder, redactPII(piiItem.Value))
				}

				sess.AddPlaceholder(newPlaceholder, piiItem.Value)
				placeholders[newPlaceholder] = piiItem.Value
				LogDebugf("添加占位符映射: %s -> %s", newPlaceholder, redactPII(piiItem.Value))
			}
		}
	}

	return placeholders
}

// remapOneAIFWPlaceholders 在 masked 文本中将 OneAIFW 占位符替换为确定性占位符
func (p *Proxy) remapOneAIFWPlaceholders(masked, maskMeta string) string {
	var meta struct {
		PII []struct {
			Type       string `json:"type"`
			Value      string `json:"value"`
			Placeholder string `json:"placeholder"`
		} `json:"pii"`
	}

	if err := json.Unmarshal([]byte(maskMeta), &meta); err != nil {
		return masked
	}

	result := masked
	for _, piiItem := range meta.PII {
		if piiItem.Placeholder != "" && piiItem.Value != "" {
			newPlaceholder := pii.GeneratePlaceholder(piiItem.Type, piiItem.Value)
			if piiItem.Placeholder != newPlaceholder {
				result = strings.ReplaceAll(result, piiItem.Placeholder, newPlaceholder)
			}
		}
	}
	return result
}

// handleStreamingRequest 处理流式请求
func (p *Proxy) handleStreamingRequest(w http.ResponseWriter, r *http.Request, reqBody map[string]interface{}, sess *session.Session) {
	maskLogSID := sess.ID[:min(8, len(sess.ID))]

	// 准备转发请求
	body, _ := json.Marshal(reqBody)
	llmReq, _ := http.NewRequest("POST", p.buildLLMURL(), bytes.NewReader(body))
	llmReq.Header = r.Header.Clone()
	llmReq.Header.Set("Content-Type", "application/json")

	// 调用 LLM
	llmResp, err := p.httpClient.Do(llmReq)
	if err != nil {
		LogInfof("[ERROR] 调用 LLM 失败: %v", err)
		http.Error(w, "LLM service unavailable", http.StatusBadGateway)
		return
	}
	defer llmResp.Body.Close()
	LogDebugf("[STREAM-LLM-RESP] status=%d session=%s", llmResp.StatusCode, maskLogSID)

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
	scanner := bufio.NewScanner(llmResp.Body)
	scanner.Buffer(make([]byte, 0, maxScannerBufferSize), maxScannerBufferSize)

	// 跨 chunk 缓冲：处理占位符被 SSE 拆散的情况
	// 当 content 末尾包含不完整的占位符前缀（如 "${CRED" 或 "${CRED_0"）时，
	// 先不发该部分，等下一个 chunk 拼接后再还原
	var pendingSuffix string
	var fullResponse strings.Builder // 收集完整 LLM 响应用于调试

	chunkCount := 0
	for scanner.Scan() {
		line := scanner.Text()
		chunkCount++

		// [DONE] 直接透传（先flush残余缓冲）
		if strings.TrimSpace(line) == "data: [DONE]" {
			if pendingSuffix != "" {
				LogDebugf("[STREAM-DONE-FLUSH] 处理残余缓冲: %q (session=%s, placeholders=%d)", pendingSuffix, maskLogSID, sess.PlaceholderCount())
				restoreFunc := p.makeRestoreFunc(sess, sess.ID)
				restored := pii.RestoreAll(pendingSuffix, restoreFunc)
				if restored != pendingSuffix {
					LogDebugf("[STREAM-DONE-RESTORE-OK] 残余还原成功: %q -> %q (session=%s)", pendingSuffix, restored, maskLogSID)
				} else {
					LogDebugf("[STREAM-DONE-RESTORE-FAIL] 残余还原失败: %q (session=%s, mapKeys=%v)", pendingSuffix, maskLogSID, sess.GetMaskMapKeys())
				}
				// 发一个内容 chunk 把残余发出去
				flushChunk := &stream.Chunk{
					ID:      fmt.Sprintf("chatcmpl-flush-%d", time.Now().UnixNano()),
					Object:  "chat.completion.chunk",
					Created: time.Now().Unix(),
					Model:   "closemask-proxy",
					Choices: []stream.Choice{{
						Index: 0,
						Delta: &stream.Delta{Content: restored},
					}},
				}
				serialized, _ := stream.SerializeChunk(flushChunk)
				fmt.Fprint(w, serialized)
				flusher.Flush()
				pendingSuffix = ""
			}
			fmt.Fprintf(w, "data: [DONE]\n\n")
			flusher.Flush()
			LogDebugf("[STREAM-END] session=%s totalChunks=%d pendingSuffix=%q", maskLogSID, chunkCount, pendingSuffix)
			LogDebugf("[STREAM-FULL-RESP] session=%s len=%d content=%q", maskLogSID, fullResponse.Len(), fullResponse.String())
			continue
		}

		// 跳过空行（SSE 分隔符）
		if line == "" {
			continue
		}

		// 解析 chunk
		chunk, err := stream.ParseChunk(line)
		if err != nil {
			LogErrorf("解析 chunk 失败: %v", err)
			continue
		}
		if chunk == nil {
			continue
		}

		// 还原占位符后透传所有 chunk
		if len(chunk.Choices) > 0 {
			// 还原 content 中的占位符（带跨 chunk 缓冲）
			if chunk.Choices[0].Delta != nil && chunk.Choices[0].Delta.Content != "" {
				combined := pendingSuffix + chunk.Choices[0].Delta.Content
				pendingSuffix = ""

				// 收集原始 LLM 响应（还原前）
				fullResponse.WriteString(chunk.Choices[0].Delta.Content)

				// 调试：记录每个 chunk 的原始内容（前80字符）
				preview := combined
				if len(preview) > 80 {
					preview = preview[:80] + "..."
				}
				LogDebugf("[STREAM-CHUNK] content=%q session=%s", preview, maskLogSID)

				// 检查末尾是否有不完整的占位符前缀
				// LLM 可能在任意位置拆分占位符，如 "${CR" + "ED_" + "0}"
				// 需要检测从 "$" 开始的所有可能不完整前缀
				cutIdx := len(combined)
				pendingStart := -1
				// 从短到长搜索所有可能的不完整前缀起始位置
				// 优先找到离末尾最近的 "$" 符号
				for _, prefix := range []string{"$"} {
					idx := strings.LastIndex(combined, prefix)
					if idx != -1 && idx > pendingStart {
						tail := combined[idx:]
						if isPartialPlaceholder(tail) {
							pendingStart = idx
						}
					}
				}
				if pendingStart != -1 {
					cutIdx = pendingStart
					pendingSuffix = combined[pendingStart:]
					LogDebugf("[STREAM-BUFFER] 缓冲不完整占位符: %q (session=%s)", pendingSuffix, maskLogSID)
				}

				toRestore := combined[:cutIdx]

				// 无条件执行还原——不再依赖 CRED 检测条件
				// 因为 LLM 可能以各种方式输出占位符文本
				restoreFunc := p.makeRestoreFunc(sess, sess.ID)
				before := toRestore
				toRestore = pii.RestoreAll(before, restoreFunc)
				if toRestore != before {
					LogInfof("[STREAM-RESTORE-OK] 还原成功: %q -> %q (session=%s)", before, toRestore, maskLogSID)
				}
				chunk.Choices[0].Delta.Content = toRestore
			}
			// 还原 tool_calls 参数中的占位符
			if len(chunk.Choices[0].Delta.ToolCalls) > 0 {
				LogDebugf("[STREAM] Tool call detected, forwarding to client")
				restoreFunc := p.makeRestoreFunc(sess, sess.ID)
				for i, tc := range chunk.Choices[0].Delta.ToolCalls {
					if tc.Function.Arguments != "" {
						before := tc.Function.Arguments
						chunk.Choices[0].Delta.ToolCalls[i].Function.Arguments = pii.RestoreAll(before, restoreFunc)
						if chunk.Choices[0].Delta.ToolCalls[i].Function.Arguments != before {
							LogDebugf("[RESTORE] 还原tool_call[%d]参数中的占位符 (session=%s)", tc.Index, maskLogSID)
						}
					}
				}
			}
		}

		// 序列化并透传
		serialized, _ := stream.SerializeChunk(chunk)
		fmt.Fprint(w, serialized)
		flusher.Flush()
	}
}

// handleNonStreamingRequest 处理非流式请求
// handleNonStreamingRequest 处理非流式请求，支持工具调用
// handleNonStreamingRequest 处理非流式请求
func (p *Proxy) handleNonStreamingRequest(w http.ResponseWriter, r *http.Request, reqBody map[string]interface{}, sess *session.Session) {
	maskLogSID := sess.ID[:min(8, len(sess.ID))]

	// 准备转发请求
	body, _ := json.Marshal(reqBody)
	
	llmReq, _ := http.NewRequest("POST", p.buildLLMURL(), bytes.NewReader(body))
	llmReq.Header = r.Header.Clone()
	llmReq.Header.Set("Content-Type", "application/json")

	// 调用 LLM
	llmResp, err := p.httpClient.Do(llmReq)
	if err != nil {
		LogInfof("[ERROR] 调用 LLM 失败: %v", err)
		http.Error(w, "LLM service unavailable", http.StatusBadGateway)
		return
	}
	defer llmResp.Body.Close()

	// 读取并解析响应（限制大小）
	respBody, err := io.ReadAll(io.LimitReader(llmResp.Body, maxResponseSize))
	if err != nil {
		LogInfof("[ERROR] 读取 LLM 响应失败: %v", err)
		http.Error(w, "Failed to read LLM response", http.StatusBadGateway)
		return
	}

	var llmResult map[string]interface{}
	if err := json.Unmarshal(respBody, &llmResult); err != nil {
		LogInfof("[ERROR] 解析 LLM 响应失败: %v", err)
		http.Error(w, "Invalid LLM response", http.StatusBadGateway)
		return
	}

	// 还原响应中的占位符后直接返回给客户端
	// IDE 自己管理工具调用循环，CloseMask 不代替执行工具
	if choices, ok := llmResult["choices"].([]interface{}); ok {
		for _, c := range choices {
			if choice, ok := c.(map[string]interface{}); ok {
				if message, ok := choice["message"].(map[string]interface{}); ok {
					// 还原 content 中的占位符
					if content, ok := message["content"].(string); ok && content != "" {
						LogDebugf("[RESTORE-RAW] LLM raw content: %q (session=%s, placeholders=%d)", content, maskLogSID, sess.PlaceholderCount())
						LogDebugf("[RESTORE-MAP] Session MaskMap keys: %v", sess.GetMaskMapKeys())
						restoreFunc := p.makeRestoreFunc(sess, sess.ID)
						restored := pii.RestoreAll(content, restoreFunc)
						if restored != content {
							LogInfof("[RESTORE-OK] restored: %d -> %d chars, result: %q", len(content), len(restored), restored)
						} else {
							LogInfof("[RESTORE-SKIP] no placeholder found in LLM response")
						}
						message["content"] = restored
					}
					// 还原 tool_calls 参数中的占位符
					if toolCalls, ok := message["tool_calls"].([]interface{}); ok {
						for _, tc := range toolCalls {
							if tcMap, ok := tc.(map[string]interface{}); ok {
								if fn, ok := tcMap["function"].(map[string]interface{}); ok {
									if args, ok := fn["arguments"].(string); ok && args != "" {
										restoreFunc := p.makeRestoreFunc(sess, sess.ID)
										restored := pii.RestoreAll(args, restoreFunc)
										fn["arguments"] = restored
									}
								}
							}
						}
						LogDebugf("[DEBUG] 非流式响应包含 %d 个 tool_calls，已还原参数并透传", len(toolCalls))
					}
				}
			}
		}
	}

	respData, err := json.Marshal(llmResult)
	if err != nil {
		LogErrorf("[ERROR] 序列化响应失败: %v", err)
		http.Error(w, "Internal server error", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Content-Length", strconv.Itoa(len(respData)))
	w.Write(respData)
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

// isPartialPlaceholder 检查文本末尾是否包含可能不完整的占位符前缀
// 占位符格式为 ${TYPE_hash}，LLM 可能在任意位置拆分
// 通用检测：只要末尾有未闭合的 ${ 结构，就认为需要缓冲
func isPartialPlaceholder(s string) bool {
	if len(s) == 0 {
		return false
	}
	// 查找最后一个 ${
	lastDollar := strings.LastIndex(s, "${")
	if lastDollar == -1 {
		return false
	}
	tail := s[lastDollar:]
	// 如果从 ${ 开始到末尾没有闭合 }，说明占位符不完整
	if !strings.Contains(tail, "}") {
		return true
	}
	// 如果有 }，检查是否是完整有效占位符
	closeIdx := strings.Index(tail, "}")
	candidate := tail[:closeIdx+1]
	if pii.IsPlaceholderToken(candidate) {
		// 完整占位符，检查后面是否还有未闭合的 ${
		after := tail[closeIdx+1:]
		if after != "" {
			return isPartialPlaceholder(after)
		}
		return false
	}
	// 不是有效占位符，可能是哈希被截断
	return true
}
