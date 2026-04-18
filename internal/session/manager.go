package session

import (
	"strings"
	"sync"
	"time"
)

// Session 表示一个用户会话
type Session struct {
	ID               string
	MaskMap          map[string]string // 占位符 -> 原值
	placeholderOrder []string          // 占位符添加顺序（FIFO 淘汰用）
	maxPlaceholders  int               // 单会话最大占位符数（0 = 无限制）
	CreatedAt        time.Time
	LastAccess       time.Time
	MaskMetaMgr      *MaskMetaManager // maskMeta 管理器
	mu               sync.RWMutex
}

// SessionManager 管理所有会话
type SessionManager struct {
	sessions sync.Map // sessionID -> *Session
	ttl      time.Duration
	stopChan chan struct{}
}

// NewSessionManager 创建会话管理器
func NewSessionManager(ttl time.Duration) *SessionManager {
	sm := &SessionManager{
		ttl:      ttl,
		stopChan: make(chan struct{}),
	}
	go sm.cleanupExpiredSessions()
	return sm
}

// Close 关闭会话管理器，停止后台清理
func (sm *SessionManager) Close() {
	close(sm.stopChan)
}

// GetOrCreate 获取或创建会话
func (sm *SessionManager) GetOrCreate(sessionID string) *Session {
	s, _ := sm.sessions.LoadOrStore(sessionID, &Session{
		ID:          sessionID,
		MaskMap:     make(map[string]string),
		MaskMetaMgr: NewMaskMetaManager(),
		CreatedAt:   time.Now(),
		LastAccess:  time.Now(),
	})
	session := s.(*Session)

	session.mu.Lock()
	session.LastAccess = time.Now()
	session.mu.Unlock()

	return session
}

// Get 获取会话，不存在返回 nil
func (sm *SessionManager) Get(sessionID string) *Session {
	s, ok := sm.sessions.Load(sessionID)
	if !ok {
		return nil
	}
	session := s.(*Session)

	session.mu.RLock()
	expired := time.Since(session.LastAccess) > sm.ttl
	session.mu.RUnlock()

	if expired {
		sm.sessions.Delete(sessionID)
		return nil
	}

	return session
}

// Delete 删除会话
func (sm *SessionManager) Delete(sessionID string) {
	sm.sessions.Delete(sessionID)
}

// AddPlaceholder 添加占位符映射
func (s *Session) AddPlaceholder(placeholder, value string) {
	s.mu.Lock()
	defer s.mu.Unlock()

	// 如果已存在，更新值但不重复添加顺序
	if _, exists := s.MaskMap[placeholder]; !exists {
		s.placeholderOrder = append(s.placeholderOrder, placeholder)
	}

	s.MaskMap[placeholder] = value

	// FIFO 淘汰
	if s.maxPlaceholders > 0 && len(s.MaskMap) > s.maxPlaceholders {
		// 删除最早的占位符
		for len(s.MaskMap) > s.maxPlaceholders && len(s.placeholderOrder) > 0 {
			oldest := s.placeholderOrder[0]
			s.placeholderOrder = s.placeholderOrder[1:]
			delete(s.MaskMap, oldest)
		}
	}
}

// SetMaxPlaceholders 设置最大占位符数
func (s *Session) SetMaxPlaceholders(max int) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.maxPlaceholders = max
}

// PlaceholderCount 返回当前占位符数量
func (s *Session) PlaceholderCount() int {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return len(s.MaskMap)
}

// Restore 还原占位符
func (s *Session) Restore(placeholder string) (string, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	val, ok := s.MaskMap[placeholder]
	return val, ok
}

// RestoreAll 还原文本中的所有占位符
func (s *Session) RestoreAll(text string) string {
	s.mu.RLock()
	defer s.mu.RUnlock()
	result := text
	for placeholder, value := range s.MaskMap {
		result = strings.ReplaceAll(result, placeholder, value)
	}
	return result
}

// GetMaskMap 获取占位符映射的副本
func (s *Session) GetMaskMap() map[string]string {
	s.mu.RLock()
	defer s.mu.RUnlock()
	copy := make(map[string]string, len(s.MaskMap))
	for k, v := range s.MaskMap {
		copy[k] = v
	}
	return copy
}

// GetMaskMapKeys 获取占位符映射的所有键
func (s *Session) GetMaskMapKeys() []string {
	s.mu.RLock()
	defer s.mu.RUnlock()
	keys := make([]string, 0, len(s.MaskMap))
	for k := range s.MaskMap {
		keys = append(keys, k)
	}
	return keys
}

// GetMaskMetaManager 获取 maskMeta 管理器
func (s *Session) GetMaskMetaManager() *MaskMetaManager {
	return s.MaskMetaMgr
}

// cleanupExpiredSessions 定期清理过期会话
func (sm *SessionManager) cleanupExpiredSessions() {
	ticker := time.NewTicker(5 * time.Minute)
	defer ticker.Stop()

	for {
		select {
		case <-sm.stopChan:
			return
		case <-ticker.C:
			now := time.Now()
			sm.sessions.Range(func(key, value interface{}) bool {
				s := value.(*Session)
				s.mu.RLock()
				expired := now.Sub(s.LastAccess) > sm.ttl
				s.mu.RUnlock()

				if expired {
					sm.sessions.Delete(key)
				}
				return true
			})
		}
	}
}

// Range 遍历所有 session
func (sm *SessionManager) Range(fn func(id string, sess *Session)) {
	sm.sessions.Range(func(key, value interface{}) bool {
		fn(key.(string), value.(*Session))
		return true
	})
}
