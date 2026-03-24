package storage

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"sync"
	"time"

	"github.com/redis/go-redis/v9"
)

// Message 消息
type Message struct {
	ID        int                    `json:"id"`
	Role      string                 `json:"role"`
	Content   string                 `json:"content,omitempty"`
	ToolCalls []ToolCall             `json:"tool_calls,omitempty"`
	ToolCallID string                 `json:"tool_call_id,omitempty"`
	Metadata  map[string]interface{} `json:"metadata,omitempty"`
}

// ToolCall 工具调用
type ToolCall struct {
	ID       int    `json:"id"`
	Index    int    `json:"index"`
	ToolCallID string `json:"tool_call_id"`
	Type     string `json:"type"`
	Function struct {
		Name      string `json:"name"`
		Arguments string `json:"arguments"`
	} `json:"function"`
}

// MaskMeta maskMeta
type MaskMeta struct {
	MessageID int    `json:"message_id"`
	Language  string `json:"language"`
	MaskMeta  string `json:"mask_meta"`
}

// Placeholder 占位符映射
type Placeholder struct {
	SessionID   string `json:"session_id"`
	Placeholder string `json:"placeholder"`
	Value       string `json:"value"`
	CreatedAt   time.Time `json:"created_at"`
}

// Storage 存储接口
type Storage interface {
	// 消息操作
	SaveMessage(ctx context.Context, sessionID string, msg *Message) error
	GetMessages(ctx context.Context, sessionID string) ([]*Message, error)
	DeleteMessages(ctx context.Context, sessionID string) error
	TruncateMessages(ctx context.Context, sessionID string, keepLastN int) error

	// maskMeta 操作
	SaveMaskMeta(ctx context.Context, sessionID string, meta *MaskMeta) error
	GetMaskMeta(ctx context.Context, sessionID string, messageID int) (*MaskMeta, error)
	GetAllMaskMeta(ctx context.Context, sessionID string) ([]*MaskMeta, error)
	DeleteMaskMeta(ctx context.Context, sessionID string) error

	// 占位符操作
	SavePlaceholder(ctx context.Context, sessionID string, placeholder, value string) error
	GetPlaceholder(ctx context.Context, sessionID string, placeholder string) (string, error)
	GetAllPlaceholders(ctx context.Context, sessionID string) (map[string]string, error)
	DeletePlaceholders(ctx context.Context, sessionID string) error

	// 会话操作
	TouchSession(ctx context.Context, sessionID string) error
	DeleteSession(ctx context.Context, sessionID string) error
}

// RedisStorage Redis 存储实现
type RedisStorage struct {
	client       *redis.Client
	messageTTL   time.Duration // 消息保留时长
	sessionTTL   time.Duration // 会话 TTL
	msgCounter   sync.Map       // 消息计数器（本地缓存）
}

// NewRedisStorage 创建 Redis 存储
func NewRedisStorage(addr, password string, messageTTL, sessionTTL time.Duration) (*RedisStorage, error) {
	rdb := redis.NewClient(&redis.Options{
		Addr:     addr,
		Password: password,
		DB:       0,
	})

	// 测试连接
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := rdb.Ping(ctx).Err(); err != nil {
		rdb.Close()
		return nil, fmt.Errorf("Redis connection failed: %w", err)
	}

	return &RedisStorage{
		client:     rdb,
		messageTTL: messageTTL,
		sessionTTL: sessionTTL,
	}, nil
}

// ============ 消息操作 ============

// SaveMessage 保存消息
func (r *RedisStorage) SaveMessage(ctx context.Context, sessionID string, msg *Message) error {
	key := fmt.Sprintf("session:%s:messages", sessionID)

	data, err := json.Marshal(msg)
	if err != nil {
		return fmt.Errorf("序列化消息失败: %w", err)
	}

	// 使用 Redis List 存储消息
	pipe := r.client.Pipeline()
	pipe.RPush(ctx, key, data)
	pipe.Expire(ctx, key, r.messageTTL)
	pipe.Expire(ctx, fmt.Sprintf("session:%s", sessionID), r.sessionTTL)

	_, err = pipe.Exec(ctx)
	return err
}

// GetMessages 获取消息
func (r *RedisStorage) GetMessages(ctx context.Context, sessionID string) ([]*Message, error) {
	key := fmt.Sprintf("session:%s:messages", sessionID)

	// 获取所有消息
	results, err := r.client.LRange(ctx, key, 0, -1).Result()
	if err != nil {
		return nil, err
	}

	messages := make([]*Message, 0, len(results))
	for _, result := range results {
		var msg Message
		if err := json.Unmarshal([]byte(result), &msg); err != nil {
			log.Printf("反序列化消息失败: %v", err)
			continue
		}
		messages = append(messages, &msg)
	}

	return messages, nil
}

// DeleteMessages 删除所有消息
func (r *RedisStorage) DeleteMessages(ctx context.Context, sessionID string) error {
	key := fmt.Sprintf("session:%s:messages", sessionID)
	return r.client.Del(ctx, key).Err()
}

// TruncateMessages 截断消息，只保留最后 N 条
func (r *RedisStorage) TruncateMessages(ctx context.Context, sessionID string, keepLastN int) error {
	if keepLastN <= 0 {
		return r.DeleteMessages(ctx, sessionID)
	}

	key := fmt.Sprintf("session:%s:messages", sessionID)

	// 获取消息数量
	count, err := r.client.LLen(ctx, key).Result()
	if err != nil {
		return err
	}

	if count <= int64(keepLastN) {
		return nil // 无需截断
	}

	// 删除前面的消息
	removeCount := count - int64(keepLastN)
	return r.client.LTrim(ctx, key, removeCount, -1).Err()
}

// ============ maskMeta 操作 ============

// SaveMaskMeta 保存 maskMeta
func (r *RedisStorage) SaveMaskMeta(ctx context.Context, sessionID string, meta *MaskMeta) error {
	key := fmt.Sprintf("session:%s:maskmeta", sessionID)
	field := fmt.Sprintf("%d", meta.MessageID)

	data, err := json.Marshal(meta)
	if err != nil {
		return fmt.Errorf("序列化 maskMeta 失败: %w", err)
	}

	pipe := r.client.Pipeline()
	pipe.HSet(ctx, key, field, data)
	pipe.Expire(ctx, key, r.messageTTL)

	_, err = pipe.Exec(ctx)
	return err
}

// GetMaskMeta 获取 maskMeta
func (r *RedisStorage) GetMaskMeta(ctx context.Context, sessionID string, messageID int) (*MaskMeta, error) {
	key := fmt.Sprintf("session:%s:maskmeta", sessionID)
	field := fmt.Sprintf("%d", messageID)

	data, err := r.client.HGet(ctx, key, field).Result()
	if err != nil {
		return nil, err
	}

	var meta MaskMeta
	if err := json.Unmarshal([]byte(data), &meta); err != nil {
		return nil, err
	}

	return &meta, nil
}

// GetAllMaskMeta 获取所有 maskMeta
func (r *RedisStorage) GetAllMaskMeta(ctx context.Context, sessionID string) ([]*MaskMeta, error) {
	key := fmt.Sprintf("session:%s:maskmeta", sessionID)

	results, err := r.client.HGetAll(ctx, key).Result()
	if err != nil {
		return nil, err
	}

	metas := make([]*MaskMeta, 0, len(results))
	for _, data := range results {
		var meta MaskMeta
		if err := json.Unmarshal([]byte(data), &meta); err != nil {
			log.Printf("反序列化 maskMeta 失败: %v", err)
			continue
		}
		metas = append(metas, &meta)
	}

	return metas, nil
}

// DeleteMaskMeta 删除所有 maskMeta
func (r *RedisStorage) DeleteMaskMeta(ctx context.Context, sessionID string) error {
	key := fmt.Sprintf("session:%s:maskmeta", sessionID)
	return r.client.Del(ctx, key).Err()
}

// ============ 占位符操作 ============

// SavePlaceholder 保存占位符
func (r *RedisStorage) SavePlaceholder(ctx context.Context, sessionID string, placeholder, value string) error {
	key := fmt.Sprintf("session:%s:placeholders", sessionID)

	pipe := r.client.Pipeline()
	pipe.HSet(ctx, key, placeholder, value)
	pipe.Expire(ctx, key, r.messageTTL)

	_, err := pipe.Exec(ctx)
	return err
}

// GetPlaceholder 获取占位符
func (r *RedisStorage) GetPlaceholder(ctx context.Context, sessionID string, placeholder string) (string, error) {
	key := fmt.Sprintf("session:%s:placeholders", sessionID)
	return r.client.HGet(ctx, key, placeholder).Result()
}

// GetAllPlaceholders 获取所有占位符
func (r *RedisStorage) GetAllPlaceholders(ctx context.Context, sessionID string) (map[string]string, error) {
	key := fmt.Sprintf("session:%s:placeholders", sessionID)
	return r.client.HGetAll(ctx, key).Result()
}

// DeletePlaceholders 删除所有占位符
func (r *RedisStorage) DeletePlaceholders(ctx context.Context, sessionID string) error {
	key := fmt.Sprintf("session:%s:placeholders", sessionID)
	return r.client.Del(ctx, key).Err()
}

// ============ 会话操作 ============

// TouchSession 刷新会话 TTL
func (r *RedisStorage) TouchSession(ctx context.Context, sessionID string) error {
	key := fmt.Sprintf("session:%s", sessionID)
	return r.client.Expire(ctx, key, r.sessionTTL).Err()
}

// DeleteSession 删除会话（包括所有数据）
func (r *RedisStorage) DeleteSession(ctx context.Context, sessionID string) error {
	pipe := r.client.Pipeline()
	pipe.Del(ctx, fmt.Sprintf("session:%s", sessionID))
	pipe.Del(ctx, fmt.Sprintf("session:%s:messages", sessionID))
	pipe.Del(ctx, fmt.Sprintf("session:%s:maskmeta", sessionID))
	pipe.Del(ctx, fmt.Sprintf("session:%s:placeholders", sessionID))

	_, err := pipe.Exec(ctx)
	return err
}

// Close 关闭连接
func (r *RedisStorage) Close() error {
	return r.client.Close()
}
