package pii

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"
)

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
	oneaifwURL string
	client     *http.Client
}

// NewPIIHandler 创建 PII 处理器
func NewPIIHandler(oneaifwURL string) *PIIHandler {
	return &PIIHandler{
		oneaifwURL: oneaifwURL,
		client: &http.Client{
			Timeout: 10 * time.Second,
		},
	}
}

// Mask 遮罩文本
// 返回：遮罩后的文本、maskMeta、错误
func (h *PIIHandler) Mask(text, language string) (string, string, error) {
	reqBody := MaskRequest{
		Text:     text,
		Language: language,
	}

	body, err := json.Marshal(reqBody)
	if err != nil {
		return "", "", fmt.Errorf("序列化请求失败: %w", err)
	}

	resp, err := h.client.Post(
		h.oneaifwURL+"/api/mask_text",
		"application/json",
		bytes.NewReader(body),
	)
	if err != nil {
		return "", "", fmt.Errorf("调用 OneAIFW 遮罩接口失败: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		respBody, _ := io.ReadAll(resp.Body)
		return "", "", fmt.Errorf("OneAIFW 返回错误: %s, body: %s", resp.Status, string(respBody))
	}

	var result MaskResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return "", "", fmt.Errorf("解析响应失败: %w", err)
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

	body, err := json.Marshal(reqBody)
	if err != nil {
		return "", fmt.Errorf("序列化请求失败: %w", err)
	}

	resp, err := h.client.Post(
		h.oneaifwURL+"/api/restore_text",
		"application/json",
		bytes.NewReader(body),
	)
	if err != nil {
		return "", fmt.Errorf("调用 OneAIFW 还原接口失败: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		respBody, _ := io.ReadAll(resp.Body)
		return "", fmt.Errorf("OneAIFW 返回错误: %s, body: %s", resp.Status, string(respBody))
	}

	var result RestoreResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return "", fmt.Errorf("解析响应失败: %w", err)
	}

	return result.Output.Text, nil
}
