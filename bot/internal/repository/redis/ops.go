// Package redis hosts the key-value operations shared by bot services.
//
// @file      internal/repository/redis/ops.go
// @for       Redis primitives: dedup, gate cache, sliding-window rate limit, per-user lock, ban marker.
// @uses      github.com/redis/go-redis/v9, context, strconv, time, fmt
// @reason    Centralizes every Redis key shape and pipeline so services never touch raw commands (PRD §9.2).
// @author    Dodi Rusmana <rusmanadodi@kentangtechstore.com>
// @layer     repository
// @stability stable
// @since     2026-08-11
package redis

import (
	"context"
	"fmt"
	"strconv"
	"time"

	goredis "github.com/redis/go-redis/v9"
)

// Redis key builders — single source of truth for every key the bot writes.
func UpdateDedupKey(updateID int64) string { return fmt.Sprintf("bot:update:%d", updateID) }
func GateCacheKey(userID int64) string     { return fmt.Sprintf("bot:gate:%d", userID) }
func RateLimitKey(userID int64) string     { return fmt.Sprintf("bot:rl:%d", userID) }
func BanKey(userID int64) string           { return fmt.Sprintf("bot:ban:%d", userID) }
func UserLockKey(userID int64) string      { return fmt.Sprintf("bot:lock:user:%d", userID) }
func TopupFSMKey(userID int64) string      { return fmt.Sprintf("bot:fsm:topup:%d", userID) }

// SetNX stores value only when the key is absent (idempotency, dedup).
// It returns true when the key was newly created.
func (c *Client) SetNX(ctx context.Context, key, value string, ttl time.Duration) (bool, error) {
	ok, err := c.rdb.SetNX(ctx, key, value, ttl).Result()
	if err != nil {
		return false, fmt.Errorf("redis setnx %s: %w", key, err)
	}
	return ok, nil
}

// GetString reads a string value; found is false when the key is missing.
func (c *Client) GetString(ctx context.Context, key string) (string, bool, error) {
	val, err := c.rdb.Get(ctx, key).Result()
	if err == goredis.Nil {
		return "", false, nil
	}
	if err != nil {
		return "", false, fmt.Errorf("redis get %s: %w", key, err)
	}
	return val, true, nil
}

// SetString writes a value with a TTL.
func (c *Client) SetString(ctx context.Context, key, value string, ttl time.Duration) error {
	if err := c.rdb.Set(ctx, key, value, ttl).Err(); err != nil {
		return fmt.Errorf("redis set %s: %w", key, err)
	}
	return nil
}

// Delete removes a key (idempotent — missing keys are not an error).
func (c *Client) Delete(ctx context.Context, key string) error {
	if err := c.rdb.Del(ctx, key).Err(); err != nil {
		return fmt.Errorf("redis del %s: %w", key, err)
	}
	return nil
}

// Exists reports whether a key is present.
func (c *Client) Exists(ctx context.Context, key string) (bool, error) {
	n, err := c.rdb.Exists(ctx, key).Result()
	if err != nil {
		return false, fmt.Errorf("redis exists %s: %w", key, err)
	}
	return n > 0, nil
}

// SlidingWindow implements a per-key sliding-window rate limit: every call
// trims entries older than the window, records the current tick, and reports
// whether the count is still within the limit. The TTL refresh keeps the key
// alive only while the user is active.
func (c *Client) SlidingWindow(ctx context.Context, key string, window time.Duration, limit int) (bool, error) {
	now := time.Now().UnixNano()
	trimBefore := now - window.Nanoseconds()

	pipe := c.rdb.TxPipeline()
	pipe.ZRemRangeByScore(ctx, key, "0", strconv.FormatInt(trimBefore, 10))
	pipe.ZAdd(ctx, key, goredis.Z{Score: float64(now), Member: strconv.FormatInt(now, 10)})
	countCmd := pipe.ZCard(ctx, key)
	pipe.Expire(ctx, key, window)

	if _, err := pipe.Exec(ctx); err != nil {
		return false, fmt.Errorf("redis sliding window %s: %w", key, err)
	}
	return countCmd.Val() <= int64(limit), nil
}

// AcquireLock takes a non-blocking lock with TTL auto-release. It returns
// true only when this caller won the lock (per-user serialization, PRD §14.2).
func (c *Client) AcquireLock(ctx context.Context, key string, ttl time.Duration) (bool, error) {
	ok, err := c.rdb.SetNX(ctx, key, "1", ttl).Result()
	if err != nil {
		return false, fmt.Errorf("redis lock %s: %w", key, err)
	}
	return ok, nil
}

// ReleaseLock deletes a lock explicitly so the next update from the same user
// is not dropped. The TTL remains the crash guard when release is missed.
func (c *Client) ReleaseLock(ctx context.Context, key string) error {
	if err := c.rdb.Del(ctx, key).Err(); err != nil {
		return fmt.Errorf("redis unlock %s: %w", key, err)
	}
	return nil
}
