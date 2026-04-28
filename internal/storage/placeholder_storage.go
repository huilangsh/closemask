package storage

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"sync"
	"time"
)

// PlaceholderStorage 占位符存储（按类型+hash前缀分文件）
type PlaceholderStorage struct {
	dataDir string
	mu      sync.RWMutex
	// 文件写入锁（按类型+hash前缀）
	fileMu   map[string]*sync.Mutex
	fileMuMu sync.Mutex
}

// placeholderFile 占位符文件结构
type placeholderFile struct {
	Type        string            `json:"type"`
	HashPrefix  string            `json:"hash_prefix"`
	UpdatedAt   string            `json:"updated_at"`
	Placeholders map[string]string `json:"placeholders"`
}

// NewPlaceholderStorage 创建占位符存储
func NewPlaceholderStorage(dataDir string) (*PlaceholderStorage, error) {
	if dataDir == "" {
		dataDir = "./data/placeholders"
	}

	// 确保目录存在
	if err := os.MkdirAll(dataDir, 0755); err != nil {
		return nil, fmt.Errorf("创建占位符目录失败: %w", err)
	}

	return &PlaceholderStorage{
		dataDir: dataDir,
		fileMu:  make(map[string]*sync.Mutex),
	}, nil
}

// ============ 公共方法 ============

// SavePlaceholder 保存占位符映射（不依赖 sessionID）
func (ps *PlaceholderStorage) SavePlaceholder(ctx context.Context, placeholder, value string) error {
	// 解析占位符类型和 hash
	piiType, hash, err := parsePlaceholder(placeholder)
	if err != nil {
		return err
	}

	// 计算文件路径
	hashPrefix := hash[:2]
	filePath := ps.getFilePath(piiType, hashPrefix)

	// 获取文件锁
	mu := ps.getFileMu(piiType, hashPrefix)
	mu.Lock()
	defer mu.Unlock()

	// 加载现有数据
	data, err := ps.loadFile(filePath)
	if err != nil {
		return err
	}

	// 更新数据
	if data.Placeholders == nil {
		data.Placeholders = make(map[string]string)
	}
	data.Placeholders[placeholder] = value
	data.UpdatedAt = time.Now().Format(time.RFC3339)

	// 保存文件
	return ps.saveFile(filePath, data)
}

// GetPlaceholder 获取占位符映射（不依赖 sessionID）
func (ps *PlaceholderStorage) GetPlaceholder(ctx context.Context, placeholder string) (string, error) {
	// 解析占位符类型和 hash
	piiType, hash, err := parsePlaceholder(placeholder)
	if err != nil {
		return "", err
	}

	// 计算文件路径
	hashPrefix := hash[:2]
	filePath := ps.getFilePath(piiType, hashPrefix)

	// 加载数据
	data, err := ps.loadFile(filePath)
	if err != nil {
		return "", fmt.Errorf("占位符不存在: %w", err)
	}

	// 查找占位符
	value, exists := data.Placeholders[placeholder]
	if !exists {
		return "", fmt.Errorf("占位符不存在")
	}

	return value, nil
}

// Contains 检查占位符是否存在
func (ps *PlaceholderStorage) Contains(ctx context.Context, placeholder string) bool {
	_, err := ps.GetPlaceholder(ctx, placeholder)
	return err == nil
}

// GetAllPlaceholders 获取所有占位符（用于调试）
func (ps *PlaceholderStorage) GetAllPlaceholders(ctx context.Context) (map[string]string, error) {
	ps.mu.RLock()
	defer ps.mu.RUnlock()

	result := make(map[string]string)

	// 遍历所有类型目录
	types, err := os.ReadDir(ps.dataDir)
	if err != nil {
		return nil, fmt.Errorf("读取目录失败: %w", err)
	}

	for _, typeDir := range types {
		if !typeDir.IsDir() {
			continue
		}

		// 遍历所有 hash 前缀文件
		files, err := os.ReadDir(filepath.Join(ps.dataDir, typeDir.Name()))
		if err != nil {
			continue
		}

		for _, file := range files {
			if file.IsDir() || filepath.Ext(file.Name()) != ".json" {
				continue
			}

			filePath := filepath.Join(ps.dataDir, typeDir.Name(), file.Name())
			data, err := ps.loadFile(filePath)
			if err != nil {
				continue
			}

			for k, v := range data.Placeholders {
				result[k] = v
			}
		}
	}

	return result, nil
}

// Close 关闭存储
func (ps *PlaceholderStorage) Close() error {
	return nil
}

// ClearAll 清空所有占位符
func (ps *PlaceholderStorage) ClearAll() error {
	ps.mu.Lock()
	defer ps.mu.Unlock()

	// 删除整个数据目录
	entries, err := os.ReadDir(ps.dataDir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return fmt.Errorf("读取目录失败: %w", err)
	}

	for _, entry := range entries {
		if entry.IsDir() {
			// 删除类型目录
			typeDir := filepath.Join(ps.dataDir, entry.Name())
			if err := os.RemoveAll(typeDir); err != nil {
				log.Printf("[WARN] 删除目录 %s 失败: %v", typeDir, err)
			}
		}
	}

	return nil
}

// ============ 私有方法 ============

// getFilePath 获取文件路径
func (ps *PlaceholderStorage) getFilePath(piiType, hashPrefix string) string {
	return filepath.Join(ps.dataDir, piiType, hashPrefix+".json")
}

// getFileMu 获取文件写入锁
func (ps *PlaceholderStorage) getFileMu(piiType, hashPrefix string) *sync.Mutex {
	key := piiType + "/" + hashPrefix
	ps.fileMuMu.Lock()
	defer ps.fileMuMu.Unlock()
	mu, exists := ps.fileMu[key]
	if !exists {
		mu = &sync.Mutex{}
		ps.fileMu[key] = mu
	}
	return mu
}

// loadFile 加载文件
func (ps *PlaceholderStorage) loadFile(filePath string) (*placeholderFile, error) {
	data, err := os.ReadFile(filePath)
	if err != nil {
		if os.IsNotExist(err) {
			// 文件不存在，返回空数据
			return &placeholderFile{
				Placeholders: make(map[string]string),
			}, nil
		}
		return nil, fmt.Errorf("读取文件失败: %w", err)
	}

	var file placeholderFile
	if err := json.Unmarshal(data, &file); err != nil {
		return nil, fmt.Errorf("解析文件失败: %w", err)
	}

	return &file, nil
}

// saveFile 保存文件
func (ps *PlaceholderStorage) saveFile(filePath string, data *placeholderFile) error {
	// 确保目录存在
	dir := filepath.Dir(filePath)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return fmt.Errorf("创建目录失败: %w", err)
	}

	// 序列化数据
	jsonData, err := json.MarshalIndent(data, "", "  ")
	if err != nil {
		return fmt.Errorf("序列化失败: %w", err)
	}

	// 写入临时文件
	tmpPath := filePath + ".tmp"
	if err := os.WriteFile(tmpPath, jsonData, 0644); err != nil {
		return fmt.Errorf("写入临时文件失败: %w", err)
	}

	// 重命名（原子操作）
	// Windows 下 rename 可能因文件被占用失败，重试几次
	var renameErr error
	for i := 0; i < 3; i++ {
		renameErr = os.Rename(tmpPath, filePath)
		if renameErr == nil {
			return nil
		}
		time.Sleep(10 * time.Millisecond)
	}
	return fmt.Errorf("重命名文件失败: %w", renameErr)
}

// ============ 辅助函数 ============

// parsePlaceholder 解析占位符，提取类型和 hash
// 格式：${TYPE_hash}
func parsePlaceholder(placeholder string) (piiType, hash string, err error) {
	// 检查格式
	if len(placeholder) < 4 || placeholder[:2] != "${" || placeholder[len(placeholder)-1] != '}' {
		return "", "", fmt.Errorf("无效的占位符格式: %s", placeholder)
	}

	// 提取内容
	content := placeholder[2 : len(placeholder)-1]

	// 查找下划线分隔符
	underscoreIdx := -1
	for i, c := range content {
		if c == '_' {
			underscoreIdx = i
			break
		}
	}

	if underscoreIdx == -1 {
		return "", "", fmt.Errorf("无效的占位符格式: %s", placeholder)
	}

	piiType = content[:underscoreIdx]
	hash = content[underscoreIdx+1:]

	if piiType == "" || hash == "" {
		return "", "", fmt.Errorf("无效的占位符格式: %s", placeholder)
	}

	return piiType, hash, nil
}

// ComputeHash 计算 PII 值的 hash
func ComputeHash(value string) string {
	h := sha256.Sum256([]byte(value))
	return hex.EncodeToString(h[:])[:16] // 取前 16 个字符
}
