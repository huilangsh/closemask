package pii

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"sync"
	"time"
)

// maxErrorResponseSize 错误响应体最大读取大小
const maxErrorResponseSize = 1 << 20 // 1MB

// MaskRequest 遮罩请求
type MaskRequest struct {
	Text     string `json:"text"`
	Language string `json:"language"`
}

// MaskResponse 遮罩响应
type MaskResponse struct {
	Output struct {
		Text     string `json:"text"`
		MaskMeta string `json:"maskMeta"`
	} `json:"output"`
}

// RestoreRequest 还原请求
type RestoreRequest struct {
	Text     string `json:"text"`
	MaskMeta string `json:"maskMeta"`
}

// RestoreResponse 还原响应
type RestoreResponse struct {
	Output struct {
		Text string `json:"text"`
	} `json:"output"`
}

// PIIHandler PII 处理器
type PIIHandler struct {
	maskURL     string // 预计算的遮罩 URL
	restoreURL  string // 预计算的还原 URL
	client      *http.Client
	// 熔断器：连续失败后暂停尝试
	mu             sync.Mutex
	consecFails    int       // 连续失败次数
	lastFailTime   time.Time // 上次失败时间
	circuitOpen    bool      // 熔断器是否开启
}

const (
	circuitThreshold = 2               // 连续失败 2 次后熔断
	circuitCooldown  = 30 * time.Second // 熔断冷却期 30 秒
)

// NewPIIHandler 创建 PII 处理器
func NewPIIHandler(oneaifwURL string) *PIIHandler {
	return &PIIHandler{
		maskURL:    oneaifwURL + "/api/mask_text",
		restoreURL: oneaifwURL + "/api/restore_text",
		client: &http.Client{
			Timeout: 10 * time.Second,
		},
	}
}

// doPost 公共 POST 请求方法
func (h *PIIHandler) doPost(url string, reqBody interface{}, respBody interface{}) error {
	// 熔断检查
	h.mu.Lock()
	if h.circuitOpen {
		if time.Since(h.lastFailTime) > circuitCooldown {
			// 冷却期结束，尝试半开
			h.circuitOpen = false
			h.consecFails = 0
		} else {
			h.mu.Unlock()
			return fmt.Errorf("OneAIFW 熔断中（冷却至 %v），跳过请求", h.lastFailTime.Add(circuitCooldown).Format("15:04:05"))
		}
	}
	h.mu.Unlock()

	body, err := json.Marshal(reqBody)
	if err != nil {
		return fmt.Errorf("序列化请求失败: %w", err)
	}

	resp, err := h.client.Post(url, "application/json", bytes.NewReader(body))
	if err != nil {
		h.recordFailure()
		return fmt.Errorf("调用 OneAIFW 接口失败: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		respData, _ := io.ReadAll(io.LimitReader(resp.Body, maxErrorResponseSize))
		h.recordFailure()
		return fmt.Errorf("OneAIFW 返回错误: %s, body: %s", resp.Status, string(respData))
	}

	if err := json.NewDecoder(resp.Body).Decode(respBody); err != nil {
		h.recordFailure()
		return fmt.Errorf("解析响应失败: %w", err)
	}

	// 成功则重置
	h.recordSuccess()
	return nil
}

// recordFailure 记录一次失败
func (h *PIIHandler) recordFailure() {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.consecFails++
	h.lastFailTime = time.Now()
	if h.consecFails >= circuitThreshold {
		h.circuitOpen = true
	}
}

// recordSuccess 记录一次成功
func (h *PIIHandler) recordSuccess() {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.consecFails = 0
	h.circuitOpen = false
}

// IsCircuitOpen 返回熔断器是否开启
func (h *PIIHandler) IsCircuitOpen() bool {
	h.mu.Lock()
	defer h.mu.Unlock()
	return h.circuitOpen
}

// Mask 遮罩文本
// 返回：遮罩后的文本、maskMeta、错误
func (h *PIIHandler) Mask(text, language string) (string, string, error) {
	reqBody := MaskRequest{
		Text:     text,
		Language: language,
	}

	var result MaskResponse
	if err := h.doPost(h.maskURL, reqBody, &result); err != nil {
		return "", "", err
	}

	return result.Output.Text, result.Output.MaskMeta, nil
}

// Restore 还原文本
// 返回：还原后的文本、错误
func (h *PIIHandler) Restore(text, maskMeta string) (string, error) {
	reqBody := RestoreRequest{
		Text:     text,
		MaskMeta: maskMeta,
	}

	var result RestoreResponse
	if err := h.doPost(h.restoreURL, reqBody, &result); err != nil {
		return "", err
	}

	return result.Output.Text, nil
}
