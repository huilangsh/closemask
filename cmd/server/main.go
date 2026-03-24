package main

import (
	"closemask/internal/proxy"
	"encoding/json"
	"flag"
	"log"
	"os"
	"strconv"
)

func main() {
	// 解析命令行参数
	configPath := flag.String("config", "config.json", "配置文件路径")
	flag.Parse()

	// 支持环境变量覆盖配置文件路径
	if envPath := os.Getenv("CLOSEMASK_CONFIG"); envPath != "" {
		configPath = &envPath
	}

	// 加载配置
	config, err := loadConfig(*configPath)
	if err != nil {
		log.Fatalf("加载配置失败: %v", err)
	}

	// 打印配置信息（不包含敏感字段）
	configJSON, _ := json.MarshalIndent(config, "", "  ")
	log.Printf("加载配置:\n%s", string(configJSON))

	// 创建代理
	p := proxy.NewProxy(config)

	// 启动服务
	log.Println("========================================")
	log.Println("  CloseMask - AI Agent PII Middleware")
	log.Println("========================================")
	log.Printf("  OneAIFW: %s", config.OneAIFWURL)
	log.Printf("  LLM API: %s", config.LLMURL)
	log.Printf("  端口: %d", config.Port)
	log.Printf("  存储类型: %s", config.StorageType)
	log.Printf("  会话 TTL: %s", config.SessionTTL)
	if config.APIKey != "" {
		log.Printf("  API 认证: 已启用")
	} else {
		log.Printf("  API 认证: 未启用（仅建议开发环境使用）")
	}
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

	// 环境变量覆盖
	if v := os.Getenv("CLOSEMASK_ONEAIFW_URL"); v != "" {
		config.OneAIFWURL = v
	}
	if v := os.Getenv("CLOSEMASK_LLM_URL"); v != "" {
		config.LLMURL = v
	}
	if v := os.Getenv("CLOSEMASK_PORT"); v != "" {
		if port, err := strconv.Atoi(v); err == nil {
			config.Port = port
		}
	}
	if v := os.Getenv("CLOSEMASK_API_KEY"); v != "" {
		config.APIKey = v
	}
	if v := os.Getenv("CLOSEMASK_REDIS_ADDR"); v != "" {
		config.RedisAddr = v
	}
	if v := os.Getenv("CLOSEMASK_REDIS_PASSWORD"); v != "" {
		config.RedisPassword = v
	}

	return &config, nil
}
