package pii

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"sync"
)

// ModelManager 模型管理器
type ModelManager struct {
	modelDir string
	proxy    string
	mu       sync.RWMutex
}

// ModelConfig 模型配置
type ModelConfig struct {
	ModelDir string `json:"model_dir"`
	Proxy    string `json:"proxy"` // 代理地址
}

// ModelInfo 模型信息
type ModelInfo struct {
	Name      string `json:"name"`
	Language  string `json:"language"`
	LocalPath string `json:"local_path"`
	Size      int64  `json:"size"`
	Checksum  string `json:"checksum"`
}

// NewModelManager 创建模型管理器
func NewModelManager(config ModelConfig) *ModelManager {
	if config.ModelDir == "" {
		config.ModelDir = "./data/models"
	}

	return &ModelManager{
		modelDir: config.ModelDir,
		proxy:    config.Proxy,
	}
}

// DownloadModel 下载模型
func (m *ModelManager) DownloadModel(modelName string, language string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	// 创建模型目录
	modelDir := filepath.Join(m.modelDir, strings.ReplaceAll(modelName, "/", "_"))
	if err := os.MkdirAll(modelDir, 0755); err != nil {
		return fmt.Errorf("创建模型目录失败: %w", err)
	}

	// HuggingFace 模型文件 URL
	baseURL := fmt.Sprintf("https://huggingface.co/%s/resolve/main", modelName)

	// 需要下载的文件
	files := []struct {
		name     string
		url      string
		destPath string
	}{
		{
			name:     "vocab.json",
			url:      fmt.Sprintf("%s/vocab.json", baseURL),
			destPath: filepath.Join(modelDir, "vocab.json"),
		},
		{
			name:     "labels.json",
			url:      fmt.Sprintf("%s/labels.json", baseURL),
			destPath: filepath.Join(modelDir, "labels.json"),
		},
		{
			name:     "config.json",
			url:      fmt.Sprintf("%s/config.json", baseURL),
			destPath: filepath.Join(modelDir, "config.json"),
		},
	}

	// 下载基础文件
	for _, file := range files {
		// 检查文件是否已存在
		if _, err := os.Stat(file.destPath); err == nil {
			continue
		}

		fmt.Printf("下载 %s...\n", file.name)
		if err := m.downloadFile(file.url, file.destPath); err != nil {
			return fmt.Errorf("下载 %s 失败: %w", file.name, err)
		}
	}

	// 下载 ONNX 模型（优先使用量化版本）
	onnxDir := filepath.Join(modelDir, "onnx")
	if err := os.MkdirAll(onnxDir, 0755); err != nil {
		return fmt.Errorf("创建 ONNX 目录失败: %w", err)
	}

	onnxFiles := []struct {
		name     string
		url      string
		destPath string
	}{
		{
			name:     "model_quantized.onnx",
			url:      fmt.Sprintf("%s/onnx/model_quantized.onnx", baseURL),
			destPath: filepath.Join(onnxDir, "model_quantized.onnx"),
		},
		{
			name:     "model.onnx",
			url:      fmt.Sprintf("%s/onnx/model.onnx", baseURL),
			destPath: filepath.Join(onnxDir, "model.onnx"),
		},
	}

	// 尝试下载 ONNX 模型
	downloaded := false
	for _, file := range onnxFiles {
		// 检查文件是否已存在
		if _, err := os.Stat(file.destPath); err == nil {
			downloaded = true
			continue
		}

		fmt.Printf("下载 %s...\n", file.name)
		if err := m.downloadFile(file.url, file.destPath); err != nil {
			fmt.Printf("下载 %s 失败（尝试下一个）: %v\n", file.name, err)
			continue
		}
		downloaded = true
		break
	}

	if !downloaded {
		return fmt.Errorf("下载 ONNX 模型失败，请手动下载模型并放到 %s 目录", onnxDir)
	}

	// 如果 labels.json 不存在，从 config.json 生成
	labelsPath := filepath.Join(modelDir, "labels.json")
	if _, err := os.Stat(labelsPath); os.IsNotExist(err) {
		configPath := filepath.Join(modelDir, "config.json")
		if err := m.generateLabelsFromConfig(configPath, labelsPath); err != nil {
			return fmt.Errorf("生成标签文件失败: %w", err)
		}
	}

	return nil
}

// generateLabelsFromConfig 从 config.json 生成 labels.json
func (m *ModelManager) generateLabelsFromConfig(configPath, labelsPath string) error {
	data, err := os.ReadFile(configPath)
	if err != nil {
		return err
	}

	var config struct {
		ID2Label map[string]string `json:"id2label"`
	}

	if err := json.Unmarshal(data, &config); err != nil {
		return err
	}

	if len(config.ID2Label) == 0 {
		// 使用默认 NER 标签
		defaultLabels := []string{"O", "B-PER", "I-PER", "B-ORG", "I-ORG", "B-LOC", "I-LOC"}
		labelsData, _ := json.Marshal(defaultLabels)
		return os.WriteFile(labelsPath, labelsData, 0644)
	}

	// 将 map 转换为有序数组
	maxID := 0
	for id := range config.ID2Label {
		var idInt int
		fmt.Sscanf(id, "%d", &idInt)
		if idInt > maxID {
			maxID = idInt
		}
	}

	labels := make([]string, maxID+1)
	for id, label := range config.ID2Label {
		var idInt int
		fmt.Sscanf(id, "%d", &idInt)
		labels[idInt] = label
	}

	labelsData, _ := json.Marshal(labels)
	return os.WriteFile(labelsPath, labelsData, 0644)
}

// IsModelDownloaded 检查模型是否已下载
func (m *ModelManager) IsModelDownloaded(modelName string) bool {
	m.mu.RLock()
	defer m.mu.RUnlock()

	modelDir := filepath.Join(m.modelDir, strings.ReplaceAll(modelName, "/", "_"))
	onnxPath := filepath.Join(modelDir, "onnx", "model_quantized.onnx")

	_, err := os.Stat(onnxPath)
	return err == nil
}

// GetModelPath 获取模型路径
func (m *ModelManager) GetModelPath(modelName string) string {
	return filepath.Join(m.modelDir, strings.ReplaceAll(modelName, "/", "_"))
}

// ListModels 列出已下载的模型
func (m *ModelManager) ListModels() ([]ModelInfo, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	entries, err := os.ReadDir(m.modelDir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("读取模型目录失败: %w", err)
	}

	var models []ModelInfo
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}

		modelDir := filepath.Join(m.modelDir, entry.Name())
		onnxPath := filepath.Join(modelDir, "onnx", "model_quantized.onnx")

		info, err := os.Stat(onnxPath)
		if err != nil {
			continue
		}

		models = append(models, ModelInfo{
			Name:      entry.Name(),
			LocalPath: modelDir,
			Size:      info.Size(),
		})
	}

	return models, nil
}

// DeleteModel 删除模型
func (m *ModelManager) DeleteModel(modelName string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	modelDir := filepath.Join(m.modelDir, strings.ReplaceAll(modelName, "/", "_"))
	return os.RemoveAll(modelDir)
}

// ============ 辅助函数 ============

// downloadFile 下载文件
func (m *ModelManager) downloadFile(urlStr string, destPath string) error {
	// 创建 HTTP 客户端
	client := &http.Client{}
	if m.proxy != "" {
		proxyURL, err := url.Parse(m.proxy)
		if err != nil {
			return fmt.Errorf("解析代理地址失败: %w", err)
		}
		client.Transport = &http.Transport{
			Proxy: http.ProxyURL(proxyURL),
		}
	}

	// 发送请求
	resp, err := client.Get(urlStr)
	if err != nil {
		return fmt.Errorf("下载失败: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("下载失败: HTTP %d", resp.StatusCode)
	}

	// 创建目标文件
	if err := os.MkdirAll(filepath.Dir(destPath), 0755); err != nil {
		return fmt.Errorf("创建目录失败: %w", err)
	}

	file, err := os.Create(destPath)
	if err != nil {
		return fmt.Errorf("创建文件失败: %w", err)
	}
	defer file.Close()

	// 写入文件
	_, err = io.Copy(file, resp.Body)
	if err != nil {
		return fmt.Errorf("写入文件失败: %w", err)
	}

	return nil
}

// computeChecksum 计算文件校验和
func computeChecksum(filePath string) (string, error) {
	file, err := os.Open(filePath)
	if err != nil {
		return "", err
	}
	defer file.Close()

	hash := sha256.New()
	if _, err := io.Copy(hash, file); err != nil {
		return "", err
	}

	return hex.EncodeToString(hash.Sum(nil)), nil
}
