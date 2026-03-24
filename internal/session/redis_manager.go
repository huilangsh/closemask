package session

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/redis/go-redis/v9"
)

// RedisSessionManager 基于 Redis 的会话管理器（用于分布式部署）
type RedisSessionManager struct {
	client *redis.Client
	ttl    time.Duration
}

// NewRedisSessionManager 创建 Redis 会话管理器
func NewRedisSessionManager(addr string, ttl time.Duration) *RedisSessionManager {
	rdb := redis.NewClient(&redis.Options{
		Addr:     addr,
		Password: "", // 无密码
		DB:       0,  // 默认数据库
	})

	return &RedisSessionManager{
		client: rdb,
		ttl:    ttl,
	}
}

// GetOrCreate 获取或创建会话
func (r *RedisSessionManager) GetOrCreate(ctx context.Context, sessionID string) *RedisSession {
	// 刷新 TTL
	key := fmt.Sprintf("session:%s", sessionID)
	r.client.Expire(ctx, key, r.ttl)

	return &RedisSession{
		ID:       sessionID,
		manager:  r,
		redisKey: key,
	}
}

// AddPlaceholder 添加占位符映射
func (r *RedisSessionManager) AddPlaceholder(ctx context.Context, sessionID, placeholder, value string) error {
	key := fmt.Sprintf("session:%s:mask_map", sessionID)
	err := r.client.HSet(ctx, key, placeholder, value).Err()
	if err != nil {
		return err
	}
	return r.client.Expire(ctx, key, r.ttl).Err()
}

// Restore 还原占位符
func (r *RedisSessionManager) Restore(ctx context.Context, sessionID, placeholder string) (string, bool) {
	key := fmt.Sprintf("session:%s:mask_map", sessionID)
	val, err := r.client.HGet(ctx, key, placeholder).Result()
	if err != nil {
		return "", false
	}
	return val, true
}

// RestoreAll 还原文本中的所有占位符
func (r *RedisSessionManager) RestoreAll(ctx context.Context, sessionID, text string) (string, error) {
	key := fmt.Sprintf("session:%s:mask_map", sessionID)
	maskMap, err := r.client.HGetAll(ctx, key).Result()
	if err != nil {
		return "", err
	}

	result := text
	for placeholder, value := range maskMap {
		result = strings.ReplaceAll(result, placeholder, value)
	}

	return result, nil
}

// Delete 删除会话
func (r *RedisSessionManager) Delete(ctx context.Context, sessionID string) error {
	key := fmt.Sprintf("session:%s", sessionID)
	maskKey := fmt.Sprintf("session:%s:mask_map", sessionID)

	return r.client.Del(ctx, key, maskKey).Err()
}

// RedisSession Redis 会话
type RedisSession struct {
	ID       string
	manager  *RedisSessionManager
	redisKey string
}

// AddPlaceholder 添加占位符映射
func (s *RedisSession) AddPlaceholder(ctx context.Context, placeholder, value string) error {
	return s.manager.AddPlaceholder(ctx, s.ID, placeholder, value)
}

// Restore 还原占位符
func (s *RedisSession) Restore(ctx context.Context, placeholder string) (string, bool) {
	return s.manager.Restore(ctx, s.ID, placeholder)
}

// RestoreAll 还原文本中的所有占位符
func (s *RedisSession) RestoreAll(ctx context.Context, text string) (string, error) {
	return s.manager.RestoreAll(ctx, s.ID, text)
}
