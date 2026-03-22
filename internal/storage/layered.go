package storage

import (
	"context"
	"fmt"
	"sync"
	"time"
)

// LayeredStorage 分层存储实现：内存（热数据）+ 磁盘（冷数据）
type LayeredStorage struct {
	hot      *MemoryStorage         // 热数据：内存
	cold     *DiskStorage            // 冷数据：磁盘
	asyncWg  sync.WaitGroup          // 异步写等待组
	stopChan chan struct{}           // 停止信号
}

// NewLayeredStorage 创建分层存储
func NewLayeredStorage(dataDir string, messageTTL, sessionTTL time.Duration) (*LayeredStorage, error) {
	// 创建冷存储（磁盘）
	cold, err := NewDiskStorage(dataDir)
	if err != nil {
		return nil, fmt.Errorf("创建磁盘存储失败: %w", err)
	}

	// 创建热存储（内存）
	hot := NewMemoryStorage(messageTTL, sessionTTL)

	ls := &LayeredStorage{
		hot:      hot,
		cold:     cold,
		stopChan: make(chan struct{}),
	}

	// 启动异步同步协程
	go ls.asyncSyncLoop()

	return ls, nil
}

// ============ 消息操作 ============

// SaveMessage 保存消息（写热数据，异步写冷数据）
func (ls *LayeredStorage) SaveMessage(ctx context.Context, sessionID string, msg *Message) error {
	// 写入热数据
	if err := ls.hot.SaveMessage(ctx, sessionID, msg); err != nil {
		return err
	}

	// 异步写入冷数据
	ls.asyncWg.Add(1)
	go func() {
		defer ls.asyncWg.Done()
		_ = ls.cold.SaveMessage(context.Background(), sessionID, msg)
	}()

	return nil
}

// GetMessages 获取消息（优先从热数据读取）
func (ls *LayeredStorage) GetMessages(ctx context.Context, sessionID string) ([]*Message, error) {
	// 先从热数据获取
	messages, err := ls.hot.GetMessages(ctx, sessionID)
	if err != nil || len(messages) > 0 {
		return messages, err
	}

	// 热数据没有，从冷数据加载并提升到热数据
	messages, err = ls.cold.GetMessages(ctx, sessionID)
	if err != nil {
		return nil, err
	}

	// 提升到热数据
	for _, msg := range messages {
		_ = ls.hot.SaveMessage(ctx, sessionID, msg)
	}

	return messages, nil
}

// DeleteMessages 删除消息
func (ls *LayeredStorage) DeleteMessages(ctx context.Context, sessionID string) error {
	var err1, err2 error

	// 同步删除热数据
	err1 = ls.hot.DeleteMessages(ctx, sessionID)

	// 异步删除冷数据
	ls.asyncWg.Add(1)
	go func() {
		defer ls.asyncWg.Done()
		err2 = ls.cold.DeleteMessages(context.Background(), sessionID)
	}()

	if err1 != nil {
		return err1
	}
	return err2
}

// TruncateMessages 截断消息
func (ls *LayeredStorage) TruncateMessages(ctx context.Context, sessionID string, keepLastN int) error {
	// 同步截断热数据
	if err := ls.hot.TruncateMessages(ctx, sessionID, keepLastN); err != nil {
		return err
	}

	// 异步截断冷数据
	ls.asyncWg.Add(1)
	go func() {
		defer ls.asyncWg.Done()
		_ = ls.cold.TruncateMessages(context.Background(), sessionID, keepLastN)
	}()

	return nil
}

// ============ maskMeta 操作 ============

// SaveMaskMeta 保存 maskMeta
func (ls *LayeredStorage) SaveMaskMeta(ctx context.Context, sessionID string, meta *MaskMeta) error {
	if err := ls.hot.SaveMaskMeta(ctx, sessionID, meta); err != nil {
		return err
	}

	ls.asyncWg.Add(1)
	go func() {
		defer ls.asyncWg.Done()
		_ = ls.cold.SaveMaskMeta(context.Background(), sessionID, meta)
	}()

	return nil
}

// GetMaskMeta 获取 maskMeta
func (ls *LayeredStorage) GetMaskMeta(ctx context.Context, sessionID string, messageID int) (*MaskMeta, error) {
	// 先从热数据获取
	meta, err := ls.hot.GetMaskMeta(ctx, sessionID, messageID)
	if err == nil {
		return meta, nil
	}

	// 热数据没有，从冷数据加载
	meta, err = ls.cold.GetMaskMeta(ctx, sessionID, messageID)
	if err != nil {
		return nil, err
	}

	// 提升到热数据
	_ = ls.hot.SaveMaskMeta(ctx, sessionID, meta)

	return meta, nil
}

// GetAllMaskMeta 获取所有 maskMeta
func (ls *LayeredStorage) GetAllMaskMeta(ctx context.Context, sessionID string) ([]*MaskMeta, error) {
	// 先从热数据获取
	metas, err := ls.hot.GetAllMaskMeta(ctx, sessionID)
	if err == nil && len(metas) > 0 {
		return metas, nil
	}

	// 热数据没有，从冷数据加载
	metas, err = ls.cold.GetAllMaskMeta(ctx, sessionID)
	if err != nil {
		return nil, err
	}

	// 提升到热数据
	for _, meta := range metas {
		_ = ls.hot.SaveMaskMeta(ctx, sessionID, meta)
	}

	return metas, nil
}

// DeleteMaskMeta 删除 maskMeta
func (ls *LayeredStorage) DeleteMaskMeta(ctx context.Context, sessionID string) error {
	var err1, err2 error

	err1 = ls.hot.DeleteMaskMeta(ctx, sessionID)

	ls.asyncWg.Add(1)
	go func() {
		defer ls.asyncWg.Done()
		err2 = ls.cold.DeleteMaskMeta(context.Background(), sessionID)
	}()

	if err1 != nil {
		return err1
	}
	return err2
}

// ============ 占位符操作 ============

// SavePlaceholder 保存占位符
func (ls *LayeredStorage) SavePlaceholder(ctx context.Context, sessionID string, placeholder, value string) error {
	if err := ls.hot.SavePlaceholder(ctx, sessionID, placeholder, value); err != nil {
		return err
	}

	ls.asyncWg.Add(1)
	go func() {
		defer ls.asyncWg.Done()
		_ = ls.cold.SavePlaceholder(context.Background(), sessionID, placeholder, value)
	}()

	return nil
}

// GetPlaceholder 获取占位符
func (ls *LayeredStorage) GetPlaceholder(ctx context.Context, sessionID string, placeholder string) (string, error) {
	// 先从热数据获取
	val, err := ls.hot.GetPlaceholder(ctx, sessionID, placeholder)
	if err == nil {
		return val, nil
	}

	// 热数据没有，从冷数据加载整个 placeholder map
	placeholders, err := ls.cold.GetAllPlaceholders(ctx, sessionID)
	if err != nil {
		return "", err
	}

	// 提升到热数据
	for k, v := range placeholders {
		_ = ls.hot.SavePlaceholder(ctx, sessionID, k, v)
	}

	val, exists := placeholders[placeholder]
	if !exists {
		return "", fmt.Errorf("占位符不存在")
	}

	return val, nil
}

// GetAllPlaceholders 获取所有占位符
func (ls *LayeredStorage) GetAllPlaceholders(ctx context.Context, sessionID string) (map[string]string, error) {
	// 先从热数据获取
	placeholders, err := ls.hot.GetAllPlaceholders(ctx, sessionID)
	if err == nil && len(placeholders) > 0 {
		return placeholders, nil
	}

	// 热数据没有，从冷数据加载
	placeholders, err = ls.cold.GetAllPlaceholders(ctx, sessionID)
	if err != nil {
		return nil, err
	}

	// 提升到热数据
	for k, v := range placeholders {
		_ = ls.hot.SavePlaceholder(ctx, sessionID, k, v)
	}

	return placeholders, nil
}

// DeletePlaceholders 删除占位符
func (ls *LayeredStorage) DeletePlaceholders(ctx context.Context, sessionID string) error {
	var err1, err2 error

	err1 = ls.hot.DeletePlaceholders(ctx, sessionID)

	ls.asyncWg.Add(1)
	go func() {
		defer ls.asyncWg.Done()
		err2 = ls.cold.DeletePlaceholders(context.Background(), sessionID)
	}()

	if err1 != nil {
		return err1
	}
	return err2
}

// ============ 会话操作 ============

// TouchSession 刷新会话 TTL
func (ls *LayeredStorage) TouchSession(ctx context.Context, sessionID string) error {
	// 只刷新热数据
	return ls.hot.TouchSession(ctx, sessionID)
}

// DeleteSession 删除会话
func (ls *LayeredStorage) DeleteSession(ctx context.Context, sessionID string) error {
	var err1, err2 error

	err1 = ls.hot.DeleteSession(ctx, sessionID)

	ls.asyncWg.Add(1)
	go func() {
		defer ls.asyncWg.Done()
		err2 = ls.cold.DeleteSession(context.Background(), sessionID)
	}()

	if err1 != nil {
		return err1
	}
	return err2
}

// ============ 额外功能 ============

// GetSession 获取完整会话数据（直接从磁盘读取）
func (ls *LayeredStorage) GetSession(ctx context.Context, sessionID string) (*SessionData, error) {
	return ls.cold.GetSession(ctx, sessionID)
}

// LoadSessionFromDisk 从磁盘加载会话到内存
func (ls *LayeredStorage) LoadSessionFromDisk(ctx context.Context, sessionID string) error {
	// 获取完整会话数据
	sessionData, err := ls.cold.GetSession(ctx, sessionID)
	if err != nil {
		return fmt.Errorf("从磁盘加载会话失败: %w", err)
	}

	// 加载消息
	for _, msg := range sessionData.Messages {
		_ = ls.hot.SaveMessage(ctx, sessionID, msg)
	}

	// 加载 maskMeta
	for _, meta := range sessionData.MaskMetas {
		_ = ls.hot.SaveMaskMeta(ctx, sessionID, meta)
	}

	// 加载 placeholders
	for k, v := range sessionData.Placeholders {
		_ = ls.hot.SavePlaceholder(ctx, sessionID, k, v)
	}

	return nil
}

// asyncSyncLoop 异步同步循环
func (ls *LayeredStorage) asyncSyncLoop() {
	// 这里可以添加批量同步逻辑
	<-ls.stopChan
}

// Close 关闭存储
func (ls *LayeredStorage) Close() error {
	// 等待所有异步写入完成
	ls.asyncWg.Wait()

	// 关闭热存储
	if err := ls.hot.Close(); err != nil {
		return err
	}

	// 关闭冷存储
	if err := ls.cold.Close(); err != nil {
		return err
	}

	// 停止异步循环
	close(ls.stopChan)

	return nil
}
