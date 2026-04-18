package main

import (
	"agent-pii-proxy/internal/proxy"
	"bytes"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"log"
	"os"
	"strconv"
)

func main() {
	configPath := flag.String("config", "config.json", "配置文件路径")
	flag.Parse()

	if envPath := os.Getenv("CLOSEMASK_CONFIG"); envPath != "" {
		configPath = &envPath
	}

	config, err := loadConfig(*configPath)
	if err != nil {
		fatalWait("加载配置失败: %v", err)
	}

	configJSON, _ := json.MarshalIndent(config, "", "  ")
	log.Printf("加载配置:\n%s", string(configJSON))

	// 日志文件配置（默认只输出终端）
	if config.LogToFile {
		if err := os.MkdirAll("./logs", 0755); err != nil {
			log.Printf("创建日志目录失败: %v，仅输出到终端", err)
		} else {
			logFile, err := os.OpenFile("./logs/closemask.log", os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0644)
			if err != nil {
				log.Printf("打开日志文件失败: %v，仅输出到终端", err)
			} else {
				multiWriter := io.MultiWriter(os.Stderr, logFile)
				log.SetOutput(multiWriter)
				log.Printf("日志已同时写入 ./logs/closemask.log")
			}
		}
	}

	p := proxy.NewProxy(config)

	p.PrintBanner()

	log.Printf("  /v1/chat/completions - 代理端点")
	log.Printf("  /health - 健康检查")
	log.Printf("  /tools - 工具列表")

	if err := p.Start(); err != nil {
		fatalWait("启动失败: %v", err)
	}
}

// fatalWait 输出错误信息并等待用户按键后退出（防止闪退看不到错误）
func fatalWait(format string, args ...interface{}) {
	msg := fmt.Sprintf(format, args...)
	fmt.Fprintf(os.Stderr, "\n❌ %s\n", msg)
	fmt.Fprintf(os.Stderr, "\n按回车键退出...")
	fmt.Scanln()
	os.Exit(1)
}

func loadConfig(path string) (*proxy.Config, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}

	// 跳过 UTF-8 BOM（Windows 记事本等编辑器会自动添加）
	data = bytes.TrimPrefix(data, []byte("\xef\xbb\xbf"))

	var config proxy.Config
	if err := json.Unmarshal(data, &config); err != nil {
		return nil, err
	}

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
		config.SessionTTL = "24h"
	}
	if config.StorageType == "" {
		config.StorageType = "layered"
	}
	if config.DataDir == "" {
		config.DataDir = "./data"
	}
	if config.MaskFailStrategy == "" {
		config.MaskFailStrategy = "pass"
	}
	if config.MaxPlaceholdersPerSession == 0 {
		config.MaxPlaceholdersPerSession = 500
	}
	if config.LocalMaskLevel == "" {
		config.LocalMaskLevel = "strict"
	}
	if config.PIIEngine == "" {
		config.PIIEngine = "auto"
	}
	if config.PlaceholderHashLength == 0 {
		config.PlaceholderHashLength = 6
	}
	if config.LogLevel == "" {
		config.LogLevel = "info"
	}

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
	if v := os.Getenv("CLOSEMASK_DATA_DIR"); v != "" {
		config.DataDir = v
	}
	if v := os.Getenv("CLOSEMASK_LOCAL_MASK_LEVEL"); v != "" {
		config.LocalMaskLevel = v
	}
	if v := os.Getenv("CLOSEMASK_MASK_FAIL_STRATEGY"); v != "" {
		config.MaskFailStrategy = v
	}
	if v := os.Getenv("CLOSEMASK_PII_ENGINE"); v != "" {
		config.PIIEngine = v
	}
	if v := os.Getenv("CLOSEMASK_PLACEHOLDER_HASH_LENGTH"); v != "" {
		if n, err := strconv.Atoi(v); err == nil {
			config.PlaceholderHashLength = n
		}
	}
	if v := os.Getenv("CLOSEMASK_PLACEHOLDER_HMAC_KEY"); v != "" {
		config.PlaceholderHMACKey = v
	}
	if v := os.Getenv("CLOSEMASK_LOG_LEVEL"); v != "" {
		config.LogLevel = v
	}

	return &config, nil
}
