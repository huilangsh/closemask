package main

import (
	"agent-pii-proxy/internal/proxy"
	"encoding/json"
	"log"
	"os"
)

func main() {
	// 加载配置
	config, err := loadConfig("config.json")
	if err != nil {
		log.Fatalf("加载配置失败: %v", err)
	}

	// 打印配置信息
	configJSON, _ := json.MarshalIndent(config, "", "  ")
	log.Printf("加载配置:\n%s", string(configJSON))

	// 创建代理
	p := proxy.NewProxy(config)

	// 启动服务
	log.Println("========================================")
	log.Println("  Agent PII 代理中间件")
	log.Println("========================================")
	log.Printf("  OneAIFW: %s", config.OneAIFWURL)
	log.Printf("  LLM API: %s", config.LLMURL)
	log.Printf("  端口: %d", config.Port)
	log.Printf("  会话 TTL: %s", config.SessionTTL)
	log.Println("========================================")
	log.Println("  /v1/chat/completions - 代理端点")
	log.Println("  /health - 健康检查")
	log.Println("  /tools - 工具列表")
	log.Println("========================================")

	if err := p.Start(); err != nil {
		log.Fatalf("启动失败: %v", err)
	}
}

// loadConfig 加载配置文件
func loadConfig(path string) (*proxy.Config, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}

	var config proxy.Config
	if err := json.Unmarshal(data, &config); err != nil {
		return nil, err
	}

	// 设置默认值
	if config.Port == 0 {
		config.Port = 8846
	}
	if config.OneAIFWURL == "" {
		config.OneAIFWURL = "http://localhost:8845"
	}
	if config.LLMURL == "" {
		config.LLMURL = "http://localhost:11434"
	}
	if config.SessionTTL == "" {
		config.SessionTTL = "2h"
	}

	return &config, nil
}
