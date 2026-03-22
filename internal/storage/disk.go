package storage

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"
)

// DiskStorage 磁盘存储实现（长期持久化）
type DiskStorage struct {
	dataDir   string
	cache     map[string]*SessionData // 内存缓存（使用 memory.go 的 SessionData）
	cacheMu   sync.RWMutex
	cacheTTL  time.Duration
}

// NewDiskStorage 创建磁盘存储
func NewDiskStorage(dataDir string) (*DiskStorage, error) {
	// 确保目录存在
	if err := os.MkdirAll(dataDir, 0755); err != nil {
		return nil, fmt.Errorf("创建数据目录失败: %w", err)
	}

	ds := &DiskStorage{
		dataDir:  dataDir,
		cache:    make(map[string]*SessionData),
		cacheTTL: 5 * time.Minute, // 缓存5分钟
	}

	// 启动缓存清理
	go ds.cleanupCacheLoop()

	return ds, nil
}

// ============ 私有方法 ============

// getSessionFilepath 获取会话文件路径
func (ds *DiskStorage) getSessionFilepath(sessionID string) string {
	// 使用安全的文件名
	safeID := fmt.Sprintf("%x", sessionID)
	return filepath.Join(ds.dataDir, safeID+".json")
}

// loadSession 从磁盘加载会话
func (ds *DiskStorage) loadSession(sessionID string) (*SessionData, error) {
	filepath := ds.getSessionFilepath(sessionID)

	data, err := os.ReadFile(filepath)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, fmt.Errorf("会话不存在")
		}
		return nil, fmt.Errorf("读取会话文件失败: %w", err)
	}

	var session SessionData
	if err := json.Unmarshal(data, &session); err != nil {
		return nil, fmt.Errorf("反序列化会话失败: %w", err)
	}

	return &session, nil
}

// saveSession 保存会话到磁盘
func (ds *DiskStorage) saveSession(sessionID string, session *SessionData) error {
	filepath := ds.getSessionFilepath(sessionID)

	data, err := json.MarshalIndent(session, "", "  ")
	if err != nil {
		return fmt.Errorf("序列化会话失败: %w", err)
	}

	// 原子写入
	tmpPath := filepath + ".tmp"
	if err := os.WriteFile(tmpPath, data, 0644); err != nil {
		return fmt.Errorf("写入临时文件失败: %w", err)
	}

	if err := os.Rename(tmpPath, filepath); err != nil {
		return fmt.Errorf("重命名文件失败: %w", err)
	}

	return nil
}

// getOrLoadSession 获取或加载会话（带缓存）
func (ds *DiskStorage) getOrLoadSession(sessionID string) (*SessionData, error) {
	ds.cacheMu.RLock()
	cached, exists := ds.cache[sessionID]
	ds.cacheMu.RUnlock()

	if exists {
		// 检查缓存是否过期
		if time.Since(cached.LastAccess) < ds.cacheTTL {
			return cached, nil
		}
	}

	// 从磁盘加载
	session, err := ds.loadSession(sessionID)
	if err != nil {
		return nil, err
	}

	// 更新最后访问时间
	session.LastAccess = time.Now()

	// 更新缓存
	ds.cacheMu.Lock()
	ds.cache[sessionID] = session
	ds.cacheMu.Unlock()

	return session, nil
}

// updateSession 更新会话
func (ds *DiskStorage) updateSession(sessionID string, updateFn func(*SessionData)) error {
	// 获取会话
	session, err := ds.getOrLoadSession(sessionID)
	if err != nil {
		// 会话不存在，创建新的
		if err.Error() == "会话不存在" {
			// 注意：SessionData 定义在 memory.go 中
			// 这里创建一个基本的会话数据结构
			session = &SessionData{
				CreatedAt:    time.Now(),
				LastAccess:   time.Now(),
			}
			// 由于 SessionData 的字段在 memory.go 中定义，我们需要手动设置
			// 这里假设 SessionData 有 ID, Messages, MaskMetas, Placeholders 字段
		} else {
			return err
		}
	}

	// 更新会话
	updateFn(session)

	// 保存到磁盘
	return ds.saveSession(sessionID, session)
}

// cleanupCacheLoop 清理缓存
func (ds *DiskStorage) cleanupCacheLoop() {
	ticker := time.NewTicker(1 * time.Minute)
	defer ticker.Stop()

	for range ticker.C {
		ds.cacheMu.Lock()
		now := time.Now()
		for id, session := range ds.cache {
			if now.Sub(session.LastAccess) > ds.cacheTTL {
				delete(ds.cache, id)
			}
		}
		ds.cacheMu.Unlock()
	}
}

// ============ 消息操作 ============

// SaveMessage 保存消息
func (ds *DiskStorage) SaveMessage(ctx context.Context, sessionID string, msg *Message) error {
	return ds.updateSession(sessionID, func(session *SessionData) {
		session.Messages = append(session.Messages, msg)
		session.LastAccess = time.Now()
	})
}

// GetMessages 获取消息
func (ds *DiskStorage) GetMessages(ctx context.Context, sessionID string) ([]*Message, error) {
	session, err := ds.getOrLoadSession(sessionID)
	if err != nil {
		return []*Message{}, err
	}

	// 返回副本
	messages := make([]*Message, len(session.Messages))
	copy(messages, session.Messages)
	return messages, nil
}

// DeleteMessages 删除所有消息
func (ds *DiskStorage) DeleteMessages(ctx context.Context, sessionID string) error {
	return ds.updateSession(sessionID, func(session *SessionData) {
		session.Messages = session.Messages[:0]
		session.LastAccess = time.Now()
	})
}

// TruncateMessages 截断消息
func (ds *DiskStorage) TruncateMessages(ctx context.Context, sessionID string, keepLastN int) error {
	if keepLastN <= 0 {
		return ds.DeleteMessages(ctx, sessionID)
	}

	return ds.updateSession(sessionID, func(session *SessionData) {
		if len(session.Messages) > keepLastN {
			session.Messages = session.Messages[len(session.Messages)-keepLastN:]
		}
		session.LastAccess = time.Now()
	})
}

// ============ maskMeta 操作 ============

// SaveMaskMeta 保存 maskMeta
func (ds *DiskStorage) SaveMaskMeta(ctx context.Context, sessionID string, meta *MaskMeta) error {
	return ds.updateSession(sessionID, func(session *SessionData) {
		session.MaskMetas[meta.MessageID] = meta
		session.LastAccess = time.Now()
	})
}

// GetMaskMeta 获取 maskMeta
func (ds *DiskStorage) GetMaskMeta(ctx context.Context, sessionID string, messageID int) (*MaskMeta, error) {
	session, err := ds.getOrLoadSession(sessionID)
	if err != nil {
		return nil, err
	}

	meta, exists := session.MaskMetas[messageID]
	if !exists {
		return nil, fmt.Errorf("maskMeta 不存在")
	}

	// 返回副本
	copy := *meta
	return &copy, nil
}

// GetAllMaskMeta 获取所有 maskMeta
func (ds *DiskStorage) GetAllMaskMeta(ctx context.Context, sessionID string) ([]*MaskMeta, error) {
	session, err := ds.getOrLoadSession(sessionID)
	if err != nil {
		return []*MaskMeta{}, err
	}

	metas := make([]*MaskMeta, 0, len(session.MaskMetas))
	for _, meta := range session.MaskMetas {
		copy := *meta
		metas = append(metas, &copy)
	}

	return metas, nil
}

// DeleteMaskMeta 删除所有 maskMeta
func (ds *DiskStorage) DeleteMaskMeta(ctx context.Context, sessionID string) error {
	return ds.updateSession(sessionID, func(session *SessionData) {
		session.MaskMetas = make(map[int]*MaskMeta)
		session.LastAccess = time.Now()
	})
}

// ============ 占位符操作 ============

// SavePlaceholder 保存占位符
func (ds *DiskStorage) SavePlaceholder(ctx context.Context, sessionID string, placeholder, value string) error {
	return ds.updateSession(sessionID, func(session *SessionData) {
		session.Placeholders[placeholder] = value
		session.LastAccess = time.Now()
	})
}

// GetPlaceholder 获取占位符
func (ds *DiskStorage) GetPlaceholder(ctx context.Context, sessionID string, placeholder string) (string, error) {
	session, err := ds.getOrLoadSession(sessionID)
	if err != nil {
		return "", err
	}

	val, exists := session.Placeholders[placeholder]
	if !exists {
		return "", fmt.Errorf("占位符不存在")
	}

	return val, nil
}

// GetAllPlaceholders 获取所有占位符
func (ds *DiskStorage) GetAllPlaceholders(ctx context.Context, sessionID string) (map[string]string, error) {
	session, err := ds.getOrLoadSession(sessionID)
	if err != nil {
		return make(map[string]string), err
	}

	copy := make(map[string]string, len(session.Placeholders))
	for k, v := range session.Placeholders {
		copy[k] = v
	}

	return copy, nil
}

// DeletePlaceholders 删除所有占位符
func (ds *DiskStorage) DeletePlaceholders(ctx context.Context, sessionID string) error {
	return ds.updateSession(sessionID, func(session *SessionData) {
		session.Placeholders = make(map[string]string)
		session.LastAccess = time.Now()
	})
}

// ============ 会话操作 ============

// TouchSession 刷新会话 TTL（磁盘存储不需要 TTL）
func (ds *DiskStorage) TouchSession(ctx context.Context, sessionID string) error {
	return ds.updateSession(sessionID, func(session *SessionData) {
		session.LastAccess = time.Now()
	})
}

// DeleteSession 删除会话
func (ds *DiskStorage) DeleteSession(ctx context.Context, sessionID string) error {
	filepath := ds.getSessionFilepath(sessionID)

	// 删除文件
	if err := os.Remove(filepath); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("删除会话文件失败: %w", err)
	}

	// 清除缓存
	ds.cacheMu.Lock()
	delete(ds.cache, sessionID)
	ds.cacheMu.Unlock()

	return nil
}

// ============ 额外功能 ============

// GetSession 获取完整会话数据
func (ds *DiskStorage) GetSession(ctx context.Context, sessionID string) (*SessionData, error) {
	return ds.getOrLoadSession(sessionID)
}

// ListSessions 列出所有会话
func (ds *DiskStorage) ListSessions(ctx context.Context) ([]string, error) {
	entries, err := os.ReadDir(ds.dataDir)
	if err != nil {
		return nil, fmt.Errorf("读取目录失败: %w", err)
	}

	sessionIDs := make([]string, 0)
	for _, entry := range entries {
		if !entry.IsDir() && filepath.Ext(entry.Name()) == ".json" {
			// 从文件名提取 sessionID（去除 .json 后缀）
			sessionID := entry.Name()[:len(entry.Name())-5]
			sessionIDs = append(sessionIDs, sessionID)
		}
	}

	return sessionIDs, nil
}

// Close 关闭存储
func (ds *DiskStorage) Close() error {
	// 清空缓存
	ds.cacheMu.Lock()
	ds.cache = make(map[string]*SessionData)
	ds.cacheMu.Unlock()

	return nil
}
