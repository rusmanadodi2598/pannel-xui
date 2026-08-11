// Package redis implements the Redis repository layer (go-redis/v9).
//
// @file      internal/repository/redis/redis.go
// @for       Redis client with explicit pool settings and health ping.
// @uses      github.com/redis/go-redis/v9, context, time
// @reason    Centralizes Redis access for sessions, idempotency and rate limits (PRD §9.2).
// @author    Dodi Rusmana <rusmanadodi@kentangtechstore.com>
// @layer     repository
// @stability stable
// @since     2026-08-11
package redis

import (
	"context"
	"fmt"
	"time"

	goredis "github.com/redis/go-redis/v9"
)

// Client wraps the go-redis handle with explicit pool settings.
type Client struct {
	rdb *goredis.Client
}

// PoolOptions carries the Redis pool limits.
type PoolOptions struct {
	PoolSize    int
	DialTimeout time.Duration
}

// Open parses the redis:// URL, applies pool settings and pings once.
func Open(ctx context.Context, url string, pool PoolOptions) (*Client, error) {
	opts, err := goredis.ParseURL(url)
	if err != nil {
		return nil, fmt.Errorf("parsing redis url: %w", err)
	}
	opts.PoolSize = pool.PoolSize
	opts.DialTimeout = pool.DialTimeout
	opts.ReadTimeout = 5 * time.Second
	opts.WriteTimeout = 5 * time.Second

	rdb := goredis.NewClient(opts)
	pingCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	if err := rdb.Ping(pingCtx).Err(); err != nil {
		_ = rdb.Close()
		return nil, fmt.Errorf("pinging redis: %w", err)
	}
	return &Client{rdb: rdb}, nil
}

// Ping checks Redis connectivity for the health endpoint.
func (c *Client) Ping(ctx context.Context) error {
	return c.rdb.Ping(ctx).Err()
}

// Close releases the connection pool (called on graceful shutdown).
func (c *Client) Close() error {
	return c.rdb.Close()
}

// Raw exposes the go-redis client for future milestones (locks, rate limits).
func (c *Client) Raw() *goredis.Client {
	return c.rdb
}
