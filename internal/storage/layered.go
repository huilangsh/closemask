package storage

import (
	"context"
	"fmt"
	"log"
	"path/filepath"
	"sync"
	"time"
)

// LayeredStorage 分层存储实现：内存（热数据）+ 磁盘（冷数据）
type LayeredStorage struct {
	hot               *MemoryStorage
	cold              *DiskStorage
	placeholderStorage *PlaceholderStorage // 占位符存储（按类型+hash分文件夹）
	asyncWg           sync.WaitGroup
	stopChan          chan struct{}
}

// NewLayeredStorage 创建分层存储
func NewLayeredStorage(dataDir string, messageTTL, sessionTTL time.Duration) (*LayeredStorage, error) {
	cold, err := NewDiskStorage(dataDir, sessionTTL)
	if err != nil {
		return nil, fmt.Errorf("创建磁盘存储失败: %w", err)
	}
	hot := NewMemoryStorage(messageTTL, sessionTTL)

	// 创建占位符存储（按类型+hash分文件夹）
	placeholderDir := filepath.Join(dataDir, "placeholders")
	placeholderStorage, err := NewPlaceholderStorage(placeholderDir)
	if err != nil {
		return nil, fmt.Errorf("创建占位符存储失败: %w", err)
	}

	ls := &LayeredStorage{
		hot:               hot,
		cold:              cold,
		placeholderStorage: placeholderStorage,
		stopChan:          make(chan struct{}),
	}
	go ls.asyncSyncLoop()
	return ls, nil
}

// ============ 消息操作 ============

func (ls *LayeredStorage) SaveMessage(ctx context.Context, sessionID string, msg *Message) error {
	if err := ls.hot.SaveMessage(ctx, sessionID, msg); err != nil {
		return err
	}
	ls.asyncWg.Add(1)
	go func() {
		defer ls.asyncWg.Done()
		if err := ls.cold.SaveMessage(context.Background(), sessionID, msg); err != nil {
			log.Printf("[WARN] 异步写入冷存储(SaveMessage)失败: %v", err)
		}
	}()
	return nil
}

func (ls *LayeredStorage) GetMessages(ctx context.Context, sessionID string) ([]*Message, error) {
	messages, err := ls.hot.GetMessages(ctx, sessionID)
	if err != nil || len(messages) > 0 {
		return messages, err
	}
	messages, err = ls.cold.GetMessages(ctx, sessionID)
	if err != nil {
		return nil, err
	}
	for _, msg := range messages {
		_ = ls.hot.SaveMessage(ctx, sessionID, msg)
	}
	return messages, nil
}

func (ls *LayeredStorage) DeleteMessages(ctx context.Context, sessionID string) error {
	err1 := ls.hot.DeleteMessages(ctx, sessionID)
	ls.asyncWg.Add(1)
	go func() {
		defer ls.asyncWg.Done()
		if err := ls.cold.DeleteMessages(context.Background(), sessionID); err != nil {
			log.Printf("[WARN] 异步删除冷存储(DeleteMessages)失败: %v", err)
		}
	}()
	return err1
}

func (ls *LayeredStorage) TruncateMessages(ctx context.Context, sessionID string, keepLastN int) error {
	if err := ls.hot.TruncateMessages(ctx, sessionID, keepLastN); err != nil {
		return err
	}
	ls.asyncWg.Add(1)
	go func() {
		defer ls.asyncWg.Done()
		if err := ls.cold.TruncateMessages(context.Background(), sessionID, keepLastN); err != nil {
			log.Printf("[WARN] 异步截断冷存储(TruncateMessages)失败: %v", err)
		}
	}()
	return nil
}

// ============ maskMeta 操作 ============

func (ls *LayeredStorage) SaveMaskMeta(ctx context.Context, sessionID string, meta *MaskMeta) error {
	if err := ls.hot.SaveMaskMeta(ctx, sessionID, meta); err != nil {
		return err
	}
	ls.asyncWg.Add(1)
	go func() {
		defer ls.asyncWg.Done()
		if err := ls.cold.SaveMaskMeta(context.Background(), sessionID, meta); err != nil {
			log.Printf("[WARN] 异步写入冷存储(SaveMaskMeta)失败: %v", err)
		}
	}()
	return nil
}

func (ls *LayeredStorage) GetMaskMeta(ctx context.Context, sessionID string, messageID int) (*MaskMeta, error) {
	meta, err := ls.hot.GetMaskMeta(ctx, sessionID, messageID)
	if err == nil {
		return meta, nil
	}
	meta, err = ls.cold.GetMaskMeta(ctx, sessionID, messageID)
	if err != nil {
		return nil, err
	}
	_ = ls.hot.SaveMaskMeta(ctx, sessionID, meta)
	return meta, nil
}

func (ls *LayeredStorage) GetAllMaskMeta(ctx context.Context, sessionID string) ([]*MaskMeta, error) {
	metas, err := ls.hot.GetAllMaskMeta(ctx, sessionID)
	if err == nil && len(metas) > 0 {
		return metas, nil
	}
	metas, err = ls.cold.GetAllMaskMeta(ctx, sessionID)
	if err != nil {
		return nil, err
	}
	for _, meta := range metas {
		_ = ls.hot.SaveMaskMeta(ctx, sessionID, meta)
	}
	return metas, nil
}

func (ls *LayeredStorage) DeleteMaskMeta(ctx context.Context, sessionID string) error {
	err1 := ls.hot.DeleteMaskMeta(ctx, sessionID)
	ls.asyncWg.Add(1)
	go func() {
		defer ls.asyncWg.Done()
		if err := ls.cold.DeleteMaskMeta(context.Background(), sessionID); err != nil {
			log.Printf("[WARN] 异步删除冷存储(DeleteMaskMeta)失败: %v", err)
		}
	}()
	if err1 != nil {
		return err1
	}
	return nil
}

// ============ 占位符操作 ============
// 注意：占位符使用全局存储（按类型+hash分文件夹），不区分 sessionID

func (ls *LayeredStorage) SavePlaceholder(ctx context.Context, sessionID string, placeholder, value string) error {
	// 1. 写入内存缓存
	if err := ls.hot.SavePlaceholder(ctx, sessionID, placeholder, value); err != nil {
		return err
	}
	// 2. 异步持久化到磁盘（按类型+hash分文件夹）
	ls.asyncWg.Add(1)
	go func() {
		defer ls.asyncWg.Done()
		if err := ls.placeholderStorage.SavePlaceholder(context.Background(), placeholder, value); err != nil {
			log.Printf("[WARN] 异步持久化占位符失败: %v", err)
		}
	}()
	return nil
}

func (ls *LayeredStorage) GetPlaceholder(ctx context.Context, sessionID string, placeholder string) (string, error) {
	// 1. 先从内存缓存查找
	val, err := ls.hot.GetPlaceholder(ctx, sessionID, placeholder)
	if err == nil {
		return val, nil
	}
	// 2. 从磁盘存储查找（按类型+hash分文件夹）
	val, err = ls.placeholderStorage.GetPlaceholder(ctx, placeholder)
	if err != nil {
		return "", err
	}
	// 3. 回填到内存缓存
	_ = ls.hot.SavePlaceholder(ctx, sessionID, placeholder, val)
	return val, nil
}

func (ls *LayeredStorage) GetAllPlaceholders(ctx context.Context, sessionID string) (map[string]string, error) {
	// 1. 先从内存缓存查找
	placeholders, err := ls.hot.GetAllPlaceholders(ctx, sessionID)
	if err == nil && len(placeholders) > 0 {
		return placeholders, nil
	}
	// 2. 从磁盘存储查找（全局占位符）
	placeholders, err = ls.placeholderStorage.GetAllPlaceholders(ctx)
	if err != nil {
		return nil, err
	}
	// 3. 回填到内存缓存
	for k, v := range placeholders {
		_ = ls.hot.SavePlaceholder(ctx, sessionID, k, v)
	}
	return placeholders, nil
}

func (ls *LayeredStorage) DeletePlaceholders(ctx context.Context, sessionID string) error {
	// 清空内存缓存
	_ = ls.hot.DeletePlaceholders(ctx, sessionID)
	// 注意：全局占位符存储不按 sessionID 删除，保留持久化数据
	// 如需清空所有占位符，请使用 DeleteAllPlaceholders
	return nil
}

// DeleteAllPlaceholders 清空所有占位符（全局）
func (ls *LayeredStorage) DeleteAllPlaceholders(ctx context.Context) error {
	// 清空内存缓存
	ls.hot.mu.Lock()
	for sid := range ls.hot.sessions {
		delete(ls.hot.sessions, sid)
	}
	ls.hot.mu.Unlock()
	// 清空磁盘存储
	if ls.placeholderStorage != nil {
		return ls.placeholderStorage.ClearAll()
	}
	return nil
}

// ============ 会话操作 ============

func (ls *LayeredStorage) TouchSession(ctx context.Context, sessionID string) error {
	return ls.hot.TouchSession(ctx, sessionID)
}

func (ls *LayeredStorage) DeleteSession(ctx context.Context, sessionID string) error {
	err1 := ls.hot.DeleteSession(ctx, sessionID)
	ls.asyncWg.Add(1)
	go func() {
		defer ls.asyncWg.Done()
		if err := ls.cold.DeleteSession(context.Background(), sessionID); err != nil {
			log.Printf("[WARN] 异步删除冷存储(DeleteSession)失败: %v", err)
		}
	}()
	if err1 != nil {
		return err1
	}
	return nil
}

// ============ 额外功能 ============

func (ls *LayeredStorage) GetSession(ctx context.Context, sessionID string) (*SessionData, error) {
	return ls.cold.GetSession(ctx, sessionID)
}

// LoadSessionFromDisk 从磁盘加载会话到内存
func (ls *LayeredStorage) LoadSessionFromDisk(ctx context.Context, sessionID string) error {
	sessionData, err := ls.cold.GetSession(ctx, sessionID)
	if err != nil {
		return fmt.Errorf("从磁盘加载会话失败: %w", err)
	}
	for _, msg := range sessionData.Messages {
		_ = ls.hot.SaveMessage(ctx, sessionID, msg)
	}
	for _, meta := range sessionData.MaskMetas {
		_ = ls.hot.SaveMaskMeta(ctx, sessionID, meta)
	}
	for k, v := range sessionData.Placeholders {
		_ = ls.hot.SavePlaceholder(ctx, sessionID, k, v)
	}
	return nil
}

func (ls *LayeredStorage) asyncSyncLoop() {
	<-ls.stopChan
}

func (ls *LayeredStorage) Close() error {
	select {
	case <-ls.stopChan:
	default:
		close(ls.stopChan)
	}
	ls.asyncWg.Wait()
	if err := ls.hot.Close(); err != nil {
		return err
	}
	if err := ls.cold.Close(); err != nil {
		return err
	}
	if ls.placeholderStorage != nil {
		_ = ls.placeholderStorage.Close()
	}
	return nil
}
