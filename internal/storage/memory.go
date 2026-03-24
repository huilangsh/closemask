package storage

import (
	"context"
	"fmt"
	"sync"
	"time"
)

// MemoryStorage 内存存储实现（用于单机开发）
type MemoryStorage struct {
	sessions    map[string]*SessionData
	mu          sync.RWMutex
	messageTTL  time.Duration
	sessionTTL  time.Duration
	cleanupTick *time.Ticker
}

// SessionData 会话数据
type SessionData struct {
	ID           string
	Messages     []*Message
	MaskMetas    map[int]*MaskMeta
	Placeholders map[string]string
	CreatedAt    time.Time
	LastAccess   time.Time
	mu           sync.RWMutex
}

// NewMemoryStorage 创建内存存储
func NewMemoryStorage(messageTTL, sessionTTL time.Duration) *MemoryStorage {
	ms := &MemoryStorage{
		sessions:    make(map[string]*SessionData),
		messageTTL:  messageTTL,
		sessionTTL:  sessionTTL,
		cleanupTick: time.NewTicker(5 * time.Minute),
	}

	// 启动清理协程
	go ms.cleanupExpired()

	return ms
}

// getOrCreateSession 获取或创建会话
func (m *MemoryStorage) getOrCreateSession(sessionID string) *SessionData {
	m.mu.RLock()
	sess, exists := m.sessions[sessionID]
	m.mu.RUnlock()

	if exists {
		sess.mu.Lock()
		sess.LastAccess = time.Now()
		sess.mu.Unlock()
		return sess
	}

	m.mu.Lock()
	defer m.mu.Unlock()

	// 双重检查
	if sess, exists := m.sessions[sessionID]; exists {
		return sess
	}

	sess = &SessionData{
		ID:           sessionID,
		Messages:     make([]*Message, 0),
		MaskMetas:    make(map[int]*MaskMeta),
		Placeholders: make(map[string]string),
		CreatedAt:    time.Now(),
		LastAccess:   time.Now(),
	}

	m.sessions[sessionID] = sess
	return sess
}

// getSession 获取会话
func (m *MemoryStorage) getSession(sessionID string) *SessionData {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.sessions[sessionID]
}

// deleteSession 删除会话
func (m *MemoryStorage) deleteSession(sessionID string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	delete(m.sessions, sessionID)
}

// cleanupExpired 清理过期会话
func (m *MemoryStorage) cleanupExpired() {
	for range m.cleanupTick.C {
		now := time.Now()

		m.mu.Lock()
		for id, sess := range m.sessions {
			sess.mu.RLock()
			expired := now.Sub(sess.LastAccess) > m.sessionTTL
			sess.mu.RUnlock()

			if expired {
				delete(m.sessions, id)
			}
		}
		m.mu.Unlock()
	}
}

// ============ 消息操作 ============

// SaveMessage 保存消息
func (m *MemoryStorage) SaveMessage(ctx context.Context, sessionID string, msg *Message) error {
	sess := m.getOrCreateSession(sessionID)

	sess.mu.Lock()
	defer sess.mu.Unlock()

	sess.Messages = append(sess.Messages, msg)
	return nil
}

// GetMessages 获取消息
func (m *MemoryStorage) GetMessages(ctx context.Context, sessionID string) ([]*Message, error) {
	sess := m.getSession(sessionID)
	if sess == nil {
		return []*Message{}, nil
	}

	sess.mu.RLock()
	defer sess.mu.RUnlock()

	// 返回副本
	messages := make([]*Message, len(sess.Messages))
	copy(messages, sess.Messages)
	return messages, nil
}

// DeleteMessages 删除所有消息
func (m *MemoryStorage) DeleteMessages(ctx context.Context, sessionID string) error {
	sess := m.getSession(sessionID)
	if sess == nil {
		return nil
	}

	sess.mu.Lock()
	sess.Messages = sess.Messages[:0]
	sess.mu.Unlock()

	return nil
}

// TruncateMessages 截断消息，只保留最后 N 条
func (m *MemoryStorage) TruncateMessages(ctx context.Context, sessionID string, keepLastN int) error {
	if keepLastN <= 0 {
		return m.DeleteMessages(ctx, sessionID)
	}

	sess := m.getSession(sessionID)
	if sess == nil {
		return nil
	}

	sess.mu.Lock()
	defer sess.mu.Unlock()

	if len(sess.Messages) <= keepLastN {
		return nil
	}

	sess.Messages = sess.Messages[len(sess.Messages)-keepLastN:]
	return nil
}

// ============ maskMeta 操作 ============

// SaveMaskMeta 保存 maskMeta
func (m *MemoryStorage) SaveMaskMeta(ctx context.Context, sessionID string, meta *MaskMeta) error {
	sess := m.getOrCreateSession(sessionID)

	sess.mu.Lock()
	defer sess.mu.Unlock()

	sess.MaskMetas[meta.MessageID] = meta
	return nil
}

// GetMaskMeta 获取 maskMeta
func (m *MemoryStorage) GetMaskMeta(ctx context.Context, sessionID string, messageID int) (*MaskMeta, error) {
	sess := m.getSession(sessionID)
	if sess == nil {
		return nil, fmt.Errorf("会话不存在")
	}

	sess.mu.RLock()
	defer sess.mu.RUnlock()

	meta, exists := sess.MaskMetas[messageID]
	if !exists {
		return nil, fmt.Errorf("maskMeta 不存在")
	}

	// 返回副本
	copy := *meta
	return &copy, nil
}

// GetAllMaskMeta 获取所有 maskMeta
func (m *MemoryStorage) GetAllMaskMeta(ctx context.Context, sessionID string) ([]*MaskMeta, error) {
	sess := m.getSession(sessionID)
	if sess == nil {
		return []*MaskMeta{}, nil
	}

	sess.mu.RLock()
	defer sess.mu.RUnlock()

	metas := make([]*MaskMeta, 0, len(sess.MaskMetas))
	for _, meta := range sess.MaskMetas {
		copy := *meta
		metas = append(metas, &copy)
	}

	return metas, nil
}

// DeleteMaskMeta 删除所有 maskMeta
func (m *MemoryStorage) DeleteMaskMeta(ctx context.Context, sessionID string) error {
	sess := m.getSession(sessionID)
	if sess == nil {
		return nil
	}

	sess.mu.Lock()
	sess.MaskMetas = make(map[int]*MaskMeta)
	sess.mu.Unlock()

	return nil
}

// ============ 占位符操作 ============

// SavePlaceholder 保存占位符
func (m *MemoryStorage) SavePlaceholder(ctx context.Context, sessionID string, placeholder, value string) error {
	sess := m.getOrCreateSession(sessionID)

	sess.mu.Lock()
	defer sess.mu.Unlock()

	sess.Placeholders[placeholder] = value
	return nil
}

// GetPlaceholder 获取占位符
func (m *MemoryStorage) GetPlaceholder(ctx context.Context, sessionID string, placeholder string) (string, error) {
	sess := m.getSession(sessionID)
	if sess == nil {
		return "", fmt.Errorf("会话不存在")
	}

	sess.mu.RLock()
	defer sess.mu.RUnlock()

	val, exists := sess.Placeholders[placeholder]
	if !exists {
		return "", fmt.Errorf("占位符不存在")
	}

	return val, nil
}

// GetAllPlaceholders 获取所有占位符
func (m *MemoryStorage) GetAllPlaceholders(ctx context.Context, sessionID string) (map[string]string, error) {
	sess := m.getSession(sessionID)
	if sess == nil {
		return make(map[string]string), nil
	}

	sess.mu.RLock()
	defer sess.mu.RUnlock()

	copy := make(map[string]string, len(sess.Placeholders))
	for k, v := range sess.Placeholders {
		copy[k] = v
	}

	return copy, nil
}

// DeletePlaceholders 删除所有占位符
func (m *MemoryStorage) DeletePlaceholders(ctx context.Context, sessionID string) error {
	sess := m.getSession(sessionID)
	if sess == nil {
		return nil
	}

	sess.mu.Lock()
	sess.Placeholders = make(map[string]string)
	sess.mu.Unlock()

	return nil
}

// ============ 会话操作 ============

// TouchSession 刷新会话 TTL
func (m *MemoryStorage) TouchSession(ctx context.Context, sessionID string) error {
	sess := m.getOrCreateSession(sessionID)
	sess.mu.Lock()
	sess.LastAccess = time.Now()
	sess.mu.Unlock()
	return nil
}

// DeleteSession 删除会话
func (m *MemoryStorage) DeleteSession(ctx context.Context, sessionID string) error {
	m.deleteSession(sessionID)
	return nil
}

// Close 关闭存储
func (m *MemoryStorage) Close() error {
	m.cleanupTick.Stop()
	return nil
}
