package pii

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"sync"
	"sync/atomic"
	"time"
)

// RemoteNERConfig 远程 NER 服务配置
type RemoteNERConfig struct {
	Enabled   bool          `json:"enabled"`
	Endpoint  string        `json:"endpoint"`   // http://127.0.0.1:8847
	Timeout   time.Duration `json:"timeout"`    // 5s
	Fallback  bool          `json:"fallback"`   // NER 不可用时降级到正则
	MaxRetry  int           `json:"max_retry"`  // 最大重试次数
	RetryWait time.Duration `json:"retry_wait"` // 重试间隔
}

// RemoteNERDetector 远程 NER 检测器
type RemoteNERDetector struct {
	config    RemoteNERConfig
	client    *http.Client
	healthy   atomic.Bool
	lastCheck time.Time
	mu        sync.RWMutex

	// 熔断器
	failCount    int
	lastFailTime time.Time
	circuitOpen  bool
}

// NewRemoteNERDetector 创建远程 NER 检测器
func NewRemoteNERDetector(config RemoteNERConfig) *RemoteNERDetector {
	if config.Timeout <= 0 {
		config.Timeout = 5 * time.Second
	}
	if config.Endpoint == "" {
		config.Endpoint = "http://127.0.0.1:8847"
	}
	if config.MaxRetry <= 0 {
		config.MaxRetry = 3
	}
	if config.RetryWait <= 0 {
		config.RetryWait = 100 * time.Millisecond
	}

	d := &RemoteNERDetector{
		config: config,
		client: &http.Client{
			Timeout: config.Timeout,
		},
	}
	d.healthy.Store(false)

	// 启动健康检查
	go d.healthCheckLoop()

	return d
}

// Detect 调用远程 NER 服务检测实体
func (d *RemoteNERDetector) Detect(ctx context.Context, text string, language string) ([]NEREntity, error) {
	if !d.config.Enabled {
		return nil, nil
	}

	// 检查熔断器
	if d.isCircuitOpen() {
		return nil, fmt.Errorf("NER service circuit breaker open")
	}

	// 构建请求
	reqBody := map[string]string{
		"text":     text,
		"language": language,
	}
	body, err := json.Marshal(reqBody)
	if err != nil {
		return nil, fmt.Errorf("marshal request: %w", err)
	}

	// 发送请求（带重试）
	var lastErr error
	for i := 0; i < d.config.MaxRetry; i++ {
		if i > 0 {
			time.Sleep(d.config.RetryWait)
		}

		req, err := http.NewRequestWithContext(ctx, "POST", d.config.Endpoint+"/detect", bytes.NewReader(body))
		if err != nil {
			lastErr = fmt.Errorf("create request: %w", err)
			continue
		}
		req.Header.Set("Content-Type", "application/json")

		resp, err := d.client.Do(req)
		if err != nil {
			lastErr = fmt.Errorf("send request: %w", err)
			d.recordFailure()
			continue
		}

		respBody, err := io.ReadAll(resp.Body)
		resp.Body.Close()
		if err != nil {
			lastErr = fmt.Errorf("read response: %w", err)
			d.recordFailure()
			continue
		}

		if resp.StatusCode != http.StatusOK {
			lastErr = fmt.Errorf("NER service error: %d - %s", resp.StatusCode, string(respBody))
			d.recordFailure()
			continue
		}

		// 解析响应
		var result struct {
			Entities []struct {
				Type  string  `json:"type"`
				Value string  `json:"value"`
				Start int     `json:"start"`
				End   int     `json:"end"`
				Score float64 `json:"score"`
			} `json:"entities"`
		}
		if err := json.Unmarshal(respBody, &result); err != nil {
			lastErr = fmt.Errorf("unmarshal response: %w", err)
			d.recordFailure()
			continue
		}

		// 成功
		d.recordSuccess()
		entities := make([]NEREntity, len(result.Entities))
		for i, e := range result.Entities {
			entities[i] = NEREntity{
				Type:  e.Type,
				Value: e.Value,
				Start: e.Start,
				End:   e.End,
				Score: e.Score,
			}
		}
		return entities, nil
	}

	return nil, lastErr
}

// HealthCheck 检查 NER 服务健康状态
func (d *RemoteNERDetector) HealthCheck(ctx context.Context) bool {
	if !d.config.Enabled {
		return false
	}

	req, err := http.NewRequestWithContext(ctx, "GET", d.config.Endpoint+"/health", nil)
	if err != nil {
		return false
	}

	resp, err := d.client.Do(req)
	if err != nil {
		return false
	}
	defer resp.Body.Close()

	return resp.StatusCode == http.StatusOK
}

// IsHealthy 返回服务是否健康
func (d *RemoteNERDetector) IsHealthy() bool {
	return d.healthy.Load()
}

// IsFallbackEnabled 返回是否启用降级
func (d *RemoteNERDetector) IsFallbackEnabled() bool {
	return d.config.Fallback
}

// healthCheckLoop 定期健康检查
func (d *RemoteNERDetector) healthCheckLoop() {
	ticker := time.NewTicker(10 * time.Second)
	defer ticker.Stop()

	// 首次检查
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	d.healthy.Store(d.HealthCheck(ctx))
	cancel()

	for range ticker.C {
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		d.healthy.Store(d.HealthCheck(ctx))
		cancel()

		d.mu.Lock()
		d.lastCheck = time.Now()
		d.mu.Unlock()
	}
}

// isCircuitOpen 检查熔断器是否打开
func (d *RemoteNERDetector) isCircuitOpen() bool {
	d.mu.RLock()
	defer d.mu.RUnlock()

	if !d.circuitOpen {
		return false
	}

	// 熔断器打开 30 秒后尝试半开
	if time.Since(d.lastFailTime) > 30*time.Second {
		d.mu.RUnlock()
		d.mu.Lock()
		d.circuitOpen = false
		d.mu.Unlock()
		d.mu.RLock()
		return false
	}

	return true
}

// recordFailure 记录失败
func (d *RemoteNERDetector) recordFailure() {
	d.mu.Lock()
	defer d.mu.Unlock()

	d.failCount++
	d.lastFailTime = time.Now()

	// 连续失败 2 次打开熔断器
	if d.failCount >= 2 {
		d.circuitOpen = true
	}
}

// recordSuccess 记录成功
func (d *RemoteNERDetector) recordSuccess() {
	d.mu.Lock()
	defer d.mu.Unlock()
	d.failCount = 0
	d.circuitOpen = false
}

// Close 关闭检测器
func (d *RemoteNERDetector) Close() error {
	if d.client != nil {
		d.client.CloseIdleConnections()
	}
	return nil
}

// GetStats 获取统计信息
func (d *RemoteNERDetector) GetStats() map[string]interface{} {
	d.mu.RLock()
	defer d.mu.RUnlock()

	return map[string]interface{}{
		"enabled":      d.config.Enabled,
		"endpoint":     d.config.Endpoint,
		"healthy":      d.healthy.Load(),
		"fallback":     d.config.Fallback,
		"circuit_open": d.circuitOpen,
		"fail_count":   d.failCount,
		"last_check":   d.lastCheck,
	}
}
