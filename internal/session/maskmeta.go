package session

import (
	"encoding/json"
	"sync"
)

// MaskMetaEntry maskMeta 条目
type MaskMetaEntry struct {
	Index    int
	Language string
	MaskMeta string
}

// MaskMetaManager maskMeta 管理器
type MaskMetaManager struct {
	entries map[int]*MaskMetaEntry // 消息索引 -> maskMeta
	mu      sync.RWMutex
}

// NewMaskMetaManager 创建 maskMeta 管理器
func NewMaskMetaManager() *MaskMetaManager {
	return &MaskMetaManager{
		entries: make(map[int]*MaskMetaEntry),
	}
}

// Add 添加 maskMeta
func (m *MaskMetaManager) Add(index int, language, maskMeta string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.entries[index] = &MaskMetaEntry{
		Index:    index,
		Language: language,
		MaskMeta: maskMeta,
	}
}

// Get 获取 maskMeta
func (m *MaskMetaManager) Get(index int) (language, maskMeta string, ok bool) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	entry, exists := m.entries[index]
	if !exists {
		return "", "", false
	}
	return entry.Language, entry.MaskMeta, true
}

// Remove 删除 maskMeta
func (m *MaskMetaManager) Remove(index int) {
	m.mu.Lock()
	defer m.mu.Unlock()
	delete(m.entries, index)
}

// Clear 清空所有
func (m *MaskMetaManager) Clear() {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.entries = make(map[int]*MaskMetaEntry)
}

// GetAll 获取所有 maskMeta
func (m *MaskMetaManager) GetAll() map[int]*MaskMetaEntry {
	m.mu.RLock()
	defer m.mu.RUnlock()

	copy := make(map[int]*MaskMetaEntry, len(m.entries))
	for k, v := range m.entries {
		copy[k] = v
	}
	return copy
}

// ToJSON 序列化为 JSON
func (m *MaskMetaManager) ToJSON() (string, error) {
	data := m.GetAll()
	jsonData, err := json.Marshal(data)
	if err != nil {
		return "", err
	}
	return string(jsonData), nil
}

// FromJSON 从 JSON 反序列化
func (m *MaskMetaManager) FromJSON(jsonStr string) error {
	var data map[int]*MaskMetaEntry
	if err := json.Unmarshal([]byte(jsonStr), &data); err != nil {
		return err
	}

	m.mu.Lock()
	defer m.mu.Unlock()
	m.entries = data
	return nil
}
