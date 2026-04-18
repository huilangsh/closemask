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
	dataDir    string
	cache      map[string]*SessionData // 内存缓存
	cacheMu    sync.RWMutex
	cacheTTL   time.Duration
	sessionTTL time.Duration // 会话文件过期时间
	stopChan   chan struct{} // 停止信号
	fileMu     map[string]*sync.Mutex // 按 sessionID 的文件写入锁
	fileMuMu   sync.Mutex             // 保护 fileMu 本身
}

// NewDiskStorage 创建磁盘存储
func NewDiskStorage(dataDir string, sessionTTL time.Duration) (*DiskStorage, error) {
	if err := os.MkdirAll(dataDir, 0755); err != nil {
		return nil, fmt.Errorf("创建数据目录失败: %w", err)
	}

	ds := &DiskStorage{
		dataDir:    dataDir,
		cache:      make(map[string]*SessionData),
		cacheTTL:   5 * time.Minute,
		sessionTTL: sessionTTL,
		stopChan:   make(chan struct{}),
		fileMu:     make(map[string]*sync.Mutex),
	}

	// 启动时清理过期文件
	ds.cleanupExpiredFiles()

	// 启动缓存清理
	go ds.cleanupCacheLoop()

	return ds, nil
}

// ============ 私有方法 ============

func (ds *DiskStorage) getSessionFilepath(sessionID string) string {
	safeID := sessionID
	if len(safeID) > 64 {
		safeID = safeID[:64]
	}
	hexID := fmt.Sprintf("%x", safeID)
	return filepath.Join(ds.dataDir, hexID+".json")
}

var errSessionNotFound = fmt.Errorf("session not found")

// getSessionFileMu 获取指定 session 的文件写入互斥锁
func (ds *DiskStorage) getSessionFileMu(sessionID string) *sync.Mutex {
	ds.fileMuMu.Lock()
	defer ds.fileMuMu.Unlock()
	mu, exists := ds.fileMu[sessionID]
	if !exists {
		mu = &sync.Mutex{}
		ds.fileMu[sessionID] = mu
	}
	return mu
}

func (ds *DiskStorage) loadSession(sessionID string) (*SessionData, error) {
	fp := ds.getSessionFilepath(sessionID)
	data, err := os.ReadFile(fp)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, errSessionNotFound
		}
		return nil, fmt.Errorf("读取会话文件失败: %w", err)
	}

	var session SessionData
	if err := json.Unmarshal(data, &session); err != nil {
		return nil, fmt.Errorf("反序列化会话失败: %w", err)
	}
	return &session, nil
}

func (ds *DiskStorage) saveSession(sessionID string, session *SessionData) error {
	fp := ds.getSessionFilepath(sessionID)
	data, err := json.MarshalIndent(session, "", "  ")
	if err != nil {
		return fmt.Errorf("序列化会话失败: %w", err)
	}
	tmpPath := fp + ".tmp"
	if err := os.WriteFile(tmpPath, data, 0644); err != nil {
		return fmt.Errorf("写入临时文件失败: %w", err)
	}
	// Windows 下 rename 可能因文件被占用失败，重试几次
	var renameErr error
	for i := 0; i < 3; i++ {
		renameErr = os.Rename(tmpPath, fp)
		if renameErr == nil {
			return nil
		}
		time.Sleep(10 * time.Millisecond)
	}
	return fmt.Errorf("重命名文件失败: %w", renameErr)
}

func (ds *DiskStorage) getOrLoadSession(sessionID string) (*SessionData, error) {
	ds.cacheMu.RLock()
	cached, exists := ds.cache[sessionID]
	ds.cacheMu.RUnlock()

	if exists && time.Since(cached.LastAccess) < ds.cacheTTL {
		return cached, nil
	}

	session, err := ds.loadSession(sessionID)
	if err != nil {
		return nil, err
	}
	session.LastAccess = time.Now()

	ds.cacheMu.Lock()
	ds.cache[sessionID] = session
	ds.cacheMu.Unlock()
	return session, nil
}

func (ds *DiskStorage) updateSession(sessionID string, updateFn func(*SessionData)) error {
	// 按 session 加锁，防止 Windows 下并发 rename 时文件被占用
	mu := ds.getSessionFileMu(sessionID)
	mu.Lock()
	defer mu.Unlock()

	session, err := ds.getOrLoadSession(sessionID)
	if err != nil {
		if err == errSessionNotFound {
			session = &SessionData{
				Messages:     make([]*Message, 0),
				MaskMetas:    make(map[int]*MaskMeta),
				Placeholders: make(map[string]string),
				CreatedAt:    time.Now(),
				LastAccess:   time.Now(),
			}
		} else {
			return err
		}
	}
	updateFn(session)
	return ds.saveSession(sessionID, session)
}

func (ds *DiskStorage) cleanupCacheLoop() {
	ticker := time.NewTicker(1 * time.Minute)
	defer ticker.Stop()

	cleanupTicker := time.NewTicker(30 * time.Minute)
	defer cleanupTicker.Stop()

	for {
		select {
		case <-ds.stopChan:
			return
		case <-ticker.C:
			ds.cacheMu.Lock()
			now := time.Now()
			for id, session := range ds.cache {
				if now.Sub(session.LastAccess) > ds.cacheTTL {
					delete(ds.cache, id)
				}
			}
			ds.cacheMu.Unlock()
		case <-cleanupTicker.C:
			ds.cleanupExpiredFiles()
		}
	}
}

// cleanupExpiredFiles 清理过期的会话文件
func (ds *DiskStorage) cleanupExpiredFiles() {
	if ds.sessionTTL <= 0 {
		return
	}
	entries, err := os.ReadDir(ds.dataDir)
	if err != nil {
		return
	}
	now := time.Now()
	for _, entry := range entries {
		if entry.IsDir() || filepath.Ext(entry.Name()) != ".json" {
			continue
		}
		filePath := filepath.Join(ds.dataDir, entry.Name())
		info, err := entry.Info()
		if err != nil {
			continue
		}
		if now.Sub(info.ModTime()) > ds.sessionTTL {
			os.Remove(filePath)
		}
	}
}

// ============ 消息操作 ============

func (ds *DiskStorage) SaveMessage(ctx context.Context, sessionID string, msg *Message) error {
	return ds.updateSession(sessionID, func(session *SessionData) {
		session.Messages = append(session.Messages, msg)
		session.LastAccess = time.Now()
	})
}

func (ds *DiskStorage) GetMessages(ctx context.Context, sessionID string) ([]*Message, error) {
	session, err := ds.getOrLoadSession(sessionID)
	if err != nil {
		return []*Message{}, err
	}
	messages := make([]*Message, len(session.Messages))
	copy(messages, session.Messages)
	return messages, nil
}

func (ds *DiskStorage) DeleteMessages(ctx context.Context, sessionID string) error {
	return ds.updateSession(sessionID, func(session *SessionData) {
		session.Messages = session.Messages[:0]
		session.LastAccess = time.Now()
	})
}

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

func (ds *DiskStorage) SaveMaskMeta(ctx context.Context, sessionID string, meta *MaskMeta) error {
	return ds.updateSession(sessionID, func(session *SessionData) {
		session.MaskMetas[meta.MessageID] = meta
		session.LastAccess = time.Now()
	})
}

func (ds *DiskStorage) GetMaskMeta(ctx context.Context, sessionID string, messageID int) (*MaskMeta, error) {
	session, err := ds.getOrLoadSession(sessionID)
	if err != nil {
		return nil, err
	}
	meta, exists := session.MaskMetas[messageID]
	if !exists {
		return nil, fmt.Errorf("maskMeta 不存在")
	}
	copy := *meta
	return &copy, nil
}

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

func (ds *DiskStorage) DeleteMaskMeta(ctx context.Context, sessionID string) error {
	return ds.updateSession(sessionID, func(session *SessionData) {
		session.MaskMetas = make(map[int]*MaskMeta)
		session.LastAccess = time.Now()
	})
}

// ============ 占位符操作 ============

func (ds *DiskStorage) SavePlaceholder(ctx context.Context, sessionID string, placeholder, value string) error {
	return ds.updateSession(sessionID, func(session *SessionData) {
		session.Placeholders[placeholder] = value
		session.LastAccess = time.Now()
	})
}

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

func (ds *DiskStorage) DeletePlaceholders(ctx context.Context, sessionID string) error {
	return ds.updateSession(sessionID, func(session *SessionData) {
		session.Placeholders = make(map[string]string)
		session.LastAccess = time.Now()
	})
}

// ============ 会话操作 ============

func (ds *DiskStorage) TouchSession(ctx context.Context, sessionID string) error {
	return ds.updateSession(sessionID, func(session *SessionData) {
		session.LastAccess = time.Now()
	})
}

func (ds *DiskStorage) DeleteSession(ctx context.Context, sessionID string) error {
	fp := ds.getSessionFilepath(sessionID)
	if err := os.Remove(fp); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("删除会话文件失败: %w", err)
	}
	ds.cacheMu.Lock()
	delete(ds.cache, sessionID)
	ds.cacheMu.Unlock()
	return nil
}

// ============ 额外功能 ============

func (ds *DiskStorage) GetSession(ctx context.Context, sessionID string) (*SessionData, error) {
	return ds.getOrLoadSession(sessionID)
}

func (ds *DiskStorage) ListSessions(ctx context.Context) ([]string, error) {
	entries, err := os.ReadDir(ds.dataDir)
	if err != nil {
		return nil, fmt.Errorf("读取目录失败: %w", err)
	}
	sessionIDs := make([]string, 0)
	for _, entry := range entries {
		if !entry.IsDir() && filepath.Ext(entry.Name()) == ".json" {
			sessionID := entry.Name()[:len(entry.Name())-5]
			sessionIDs = append(sessionIDs, sessionID)
		}
	}
	return sessionIDs, nil
}

func (ds *DiskStorage) Close() error {
	select {
	case <-ds.stopChan:
	default:
		close(ds.stopChan)
	}
	ds.cacheMu.Lock()
	ds.cache = make(map[string]*SessionData)
	ds.cacheMu.Unlock()
	return nil
}
