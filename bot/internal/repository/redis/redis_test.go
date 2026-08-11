// Package redis_test covers the Redis repository against a real server.
//
// @file      internal/repository/redis/redis_test.go
// @for       Integration tests: open/ping, set/get roundtrip, malformed URL and unreachable errors.
// @uses      testing, context, time, os
// @reason    Verifies pool settings and client contract against the staging Redis (AGENTS.md §2.1).
// @author    Dodi Rusmana <rusmanadodi@kentangtechstore.com>
// @layer     repository
// @stability stable
// @since     2026-08-11
package redis_test

import (
	"context"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/kentangtech/bot-order/internal/repository/redis"
)

// testURL returns the Redis URL for tests (staging host, dedicated DB 15).
func testURL() string {
	if url := os.Getenv("TEST_REDIS_URL"); url != "" {
		return url
	}
	return "redis://127.0.0.1:6379/15"
}

func testPool() redis.PoolOptions {
	return redis.PoolOptions{PoolSize: 10, DialTimeout: 5 * time.Second}
}

func openClient(t *testing.T) *redis.Client {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	c, err := redis.Open(ctx, testURL(), testPool())
	if err != nil {
		t.Fatalf("redis.Open: %v", err)
	}
	t.Cleanup(func() { _ = c.Close() })
	return c
}

func TestOpen_GivenReachableServer_ThenPingOK(t *testing.T) {
	c := openClient(t)
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	if err := c.Ping(ctx); err != nil {
		t.Fatalf("Ping: %v", err)
	}
}

func TestOpen_GivenMalformedURL_ThenError(t *testing.T) {
	ctx := context.Background()
	_, err := redis.Open(ctx, "not-a-url", testPool())
	if err == nil {
		t.Fatal("expected error for malformed URL, got nil")
	}
	if !strings.Contains(err.Error(), "parsing redis url") {
		t.Errorf("error should mention parsing redis url, got: %v", err)
	}
}

func TestOpen_GivenUnreachableServer_ThenError(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 8*time.Second)
	defer cancel()
	_, err := redis.Open(ctx, "redis://127.0.0.1:59999/15", testPool())
	if err == nil {
		t.Fatal("expected error for unreachable Redis, got nil")
	}
}

func TestClient_SetGetRoundtrip(t *testing.T) {
	c := openClient(t)
	rdb := c.Raw()
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	key := "bot:test:roundtrip"
	want := "ok-value"
	if err := rdb.Set(ctx, key, want, 60*time.Second).Err(); err != nil {
		t.Fatalf("Set: %v", err)
	}
	got, err := rdb.Get(ctx, key).Result()
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if got != want {
		t.Errorf("roundtrip = %q, want %q", got, want)
	}
}

func TestClient_SetNXIdempotencyKey(t *testing.T) {
	c := openClient(t)
	rdb := c.Raw()
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	key := "bot:test:setnx"
	_ = rdb.Del(ctx, key).Err() // clean slate

	first, err := rdb.SetNX(ctx, key, "1", 60*time.Second).Result()
	if err != nil {
		t.Fatalf("first SetNX: %v", err)
	}
	if !first {
		t.Fatal("first SetNX should acquire the key")
	}

	second, err := rdb.SetNX(ctx, key, "1", 60*time.Second).Result()
	if err != nil {
		t.Fatalf("second SetNX: %v", err)
	}
	if second {
		t.Fatal("second SetNX must not re-acquire — idempotency contract (FR-06)")
	}
}
