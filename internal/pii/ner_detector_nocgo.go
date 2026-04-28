//go:build !cgo

package pii

import (
	"context"
	"fmt"
	"sync"
	"time"
)

// NERMode NER 工作模式
type NERMode string

const (
	NERModeEmbedded NERMode = "embedded" // CGO 内嵌模式
	NERModeRemote   NERMode = "remote"   // 远程 Python 服务模式
)

// NERDetector NER 检测器（无 CGO 实现，支持远程模式）
type NERDetector struct {
	enabled  bool
	mode     NERMode
	modelDir string
	models   map[string]string
	timeout  time.Duration
	mu       sync.RWMutex
	loaded   bool

	// 远程模式
	remote *RemoteNERDetector
}

// NERConfig NER 配置
type NERConfig struct {
	Enabled  bool              `json:"enabled"`
	Mode     NERMode           `json:"mode"`     // embedded 或 remote
	ModelDir string            `json:"model_dir"`
	Models   map[string]string `json:"models"`
	Timeout  time.Duration     `json:"timeout"`

	// 远程模式配置
	RemoteEndpoint string        `json:"remote_endpoint"` // http://127.0.0.1:8847
	RemoteFallback bool          `json:"remote_fallback"` // NER 不可用时降级到正则
	RemoteMaxRetry int           `json:"remote_max_retry"`
	RemoteRetryWait time.Duration `json:"remote_retry_wait"`
}

// NEREntity NER 实体
type NEREntity struct {
	Type  string
	Value string
	Start int
	End   int
	Score float64
}

// NewNERDetector 创建 NER 检测器
func NewNERDetector(config NERConfig) *NERDetector {
	if config.Timeout <= 0 {
		config.Timeout = 5 * time.Second
	}
	if config.ModelDir == "" {
		config.ModelDir = "./data/models"
	}
	if config.Models == nil {
		config.Models = map[string]string{
			"zh": "ckiplab/bert-tiny-chinese-ner",
			"en": "dslim/distilbert-NER",
		}
	}
	if config.Mode == "" {
		config.Mode = NERModeRemote // 默认使用远程模式
	}

	detector := &NERDetector{
		enabled:  config.Enabled,
		mode:     config.Mode,
		modelDir: config.ModelDir,
		models:   config.Models,
		timeout:  config.Timeout,
	}

	// 如果是远程模式，初始化远程检测器
	if config.Mode == NERModeRemote && config.Enabled {
		detector.remote = NewRemoteNERDetector(RemoteNERConfig{
			Enabled:    config.Enabled,
			Endpoint:   config.RemoteEndpoint,
			Timeout:    config.Timeout,
			Fallback:   config.RemoteFallback,
			MaxRetry:   config.RemoteMaxRetry,
			RetryWait:  config.RemoteRetryWait,
		})
	}

	return detector
}

// Detect 检测文本中的 NER 实体
func (n *NERDetector) Detect(ctx context.Context, text string, language string) ([]NEREntity, error) {
	if !n.enabled {
		return nil, nil
	}

	// 远程模式
	if n.mode == NERModeRemote && n.remote != nil {
		return n.remote.Detect(ctx, text, language)
	}

	// 内嵌模式需要 CGO
	return nil, fmt.Errorf("NER 内嵌模式需要 CGO 支持，请使用远程模式或安装 GCC")
}

// IsEnabled 检查是否启用
func (n *NERDetector) IsEnabled() bool {
	return n.enabled
}

// IsHealthy 检查服务是否健康（远程模式）
func (n *NERDetector) IsHealthy() bool {
	if n.mode == NERModeRemote && n.remote != nil {
		return n.remote.IsHealthy()
	}
	return n.enabled
}

// IsFallbackEnabled 检查是否启用降级（远程模式）
func (n *NERDetector) IsFallbackEnabled() bool {
	if n.mode == NERModeRemote && n.remote != nil {
		return n.remote.IsFallbackEnabled()
	}
	return false
}

// Enable 启用 NER
func (n *NERDetector) Enable() {
	n.mu.Lock()
	defer n.mu.Unlock()
	n.enabled = true
}

// Disable 禁用 NER
func (n *NERDetector) Disable() {
	n.mu.Lock()
	defer n.mu.Unlock()
	n.enabled = false
}

// LoadModel 加载模型
func (n *NERDetector) LoadModel(language string) error {
	if n.mode == NERModeRemote {
		// 远程模式不需要预加载
		n.mu.Lock()
		n.loaded = true
		n.mu.Unlock()
		return nil
	}
	return fmt.Errorf("NER 内嵌模式需要 CGO 支持")
}

// UnloadModel 卸载模型
func (n *NERDetector) UnloadModel() {
	n.mu.Lock()
	defer n.mu.Unlock()
	n.loaded = false
}

// Close 关闭检测器
func (n *NERDetector) Close() error {
	if n.remote != nil {
		return n.remote.Close()
	}
	return nil
}

// GetStats 获取统计信息
func (n *NERDetector) GetStats() map[string]interface{} {
	n.mu.RLock()
	defer n.mu.RUnlock()

	stats := map[string]interface{}{
		"enabled":  n.enabled,
		"mode":     string(n.mode),
		"loaded":   n.loaded,
		"modelDir": n.modelDir,
		"models":   n.models,
		"cgo":      false,
	}

	if n.mode == NERModeRemote && n.remote != nil {
		stats["remote"] = n.remote.GetStats()
	}

	return stats
}

// MapNERTypeToPIIType 映射 NER 类型到 PII 类型
func MapNERTypeToPIIType(nerType string) string {
	switch nerType {
	// 标准 NER 标签
	case "PER", "B-PER", "I-PER", "PERSON":
		return "USER_NAME"
	case "ORG", "B-ORG", "I-ORG", "ORGANIZATION":
		return "ORGANIZATION"
	case "LOC", "B-LOC", "I-LOC", "LOCATION", "GPE":
		return "PHYSICAL_ADDRESS"
	case "DATE", "TIME":
		return "DATE_TIME"

	// 中文模型常见标签 (gyr66/bert-base-chinese-finetuned-ner)
	case "name", "NAME":
		return "USER_NAME"
	case "address", "ADDRESS":
		return "PHYSICAL_ADDRESS"
	case "company", "COMPANY", "organization":
		return "ORGANIZATION"

	default:
		return ""
	}
}

// DetectAndMaskWithNER 使用 NER 检测并遮罩
func (n *NERDetector) DetectAndMaskWithNER(text string, language string, addPlaceholder func(placeholder, value string)) (string, error) {
	if !n.enabled {
		return text, nil
	}

	ctx := context.Background()
	entities, err := n.Detect(ctx, text, language)
	if err != nil {
		// 远程模式降级处理
		if n.mode == NERModeRemote && n.IsFallbackEnabled() {
			return text, nil // 降级到正则
		}
		return text, err
	}

	// 从后往前替换，避免索引偏移
	result := text
	for i := len(entities) - 1; i >= 0; i-- {
		e := entities[i]
		piiType := MapNERTypeToPIIType(e.Type)
		if piiType == "" {
			continue
		}

		placeholder := GeneratePlaceholder(piiType, e.Value)
		if addPlaceholder != nil {
			addPlaceholder(placeholder, e.Value)
		}

		result = result[:e.Start] + placeholder + result[e.End:]
	}

	return result, nil
}

// InitializeONNX 初始化 ONNX Runtime 环境（存根实现）
func InitializeONNX(libPath string) error {
	return fmt.Errorf("NER 内嵌模式需要 CGO 支持")
}

// DestroyONNX 销毁 ONNX Runtime 环境（存根实现）
func DestroyONNX() {}
