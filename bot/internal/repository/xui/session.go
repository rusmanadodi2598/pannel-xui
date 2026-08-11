// Package xui also hosts the session cookie cache.
//
// @file      internal/repository/xui/session.go
// @for       SessionCache interface + Redis implementation for panel cookies.
// @uses      context, time, github.com/redis/go-redis/v9
// @reason    Caches the x-ui session cookie to minimize panel logins (PRD §15.1).
// @author    Dodi Rusmana <rusmanadodi@kentangtechstore.com>
// @layer     repository
// @stability stable
// @since     2026-08-11
package xui

import (
	"context"
	"fmt"
	"time"

	goredis "github.com/redis/go-redis/v9"
)

// sessionKey is the Redis key prefix for cached panel sessions.
const sessionKey = "xui:session:%d"

// SessionCache stores/loads a panel session cookie per server.
type SessionCache interface {
	Get(ctx context.Context, serverID int64) (string, error)
	Set(ctx context.Context, serverID int64, cookie string, ttl time.Duration) error
	Del(ctx context.Context, serverID int64) error
}

// RedisSessionCache implements SessionCache over go-redis.
type RedisSessionCache struct {
	rdb goredis.Cmdable
}

// NewRedisSessionCache wraps a go-redis client (or miniredis in tests).
func NewRedisSessionCache(rdb goredis.Cmdable) *RedisSessionCache {
	return &RedisSessionCache{rdb: rdb}
}

func (c *RedisSessionCache) Get(ctx context.Context, serverID int64) (string, error) {
	v, err := c.rdb.Get(ctx, fmt.Sprintf(sessionKey, serverID)).Result()
	if err == goredis.Nil {
		return "", nil
	}
	if err != nil {
		return "", fmt.Errorf("xui session get: %w", err)
	}
	return v, nil
}

func (c *RedisSessionCache) Set(ctx context.Context, serverID int64, cookie string, ttl time.Duration) error {
	if err := c.rdb.Set(ctx, fmt.Sprintf(sessionKey, serverID), cookie, ttl).Err(); err != nil {
		return fmt.Errorf("xui session set: %w", err)
	}
	return nil
}

func (c *RedisSessionCache) Del(ctx context.Context, serverID int64) error {
	if err := c.rdb.Del(ctx, fmt.Sprintf(sessionKey, serverID)).Err(); err != nil {
		return fmt.Errorf("xui session del: %w", err)
	}
	return nil
}
