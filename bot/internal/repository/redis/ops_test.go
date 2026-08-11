// Package redis test covers the ops primitives on a real in-memory Redis.
//
// @file      internal/repository/redis/ops_test.go
// @for       miniredis: setnx dedup, string get/set, exists, sliding window, lock.
// @uses      testing, github.com/alicebob/miniredis/v2, context, time
// @reason    Guards the primitives that dedup, rate limiting and serialization depend on.
// @author    Dodi Rusmana <rusmanadodi@kentangtechstore.com>
// @layer     repository
// @stability stable
// @since     2026-08-11
package redis

import (
	"context"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
)

func newTestClient(t *testing.T) (*Client, *miniredis.Miniredis) {
	t.Helper()
	mr := miniredis.RunT(t)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	c, err := Open(ctx, "redis://"+mr.Addr(), PoolOptions{PoolSize: 5, DialTimeout: time.Second})
	if err != nil {
		t.Fatalf("open redis: %v", err)
	}
	t.Cleanup(func() { _ = c.Close() })
	return c, mr
}

func TestSetNX_GivenNewKey_ThenTrueAndStored(t *testing.T) {
	c, _ := newTestClient(t)
	key := UpdateDedupKey(1)
	ctx := context.Background()

	ok, err := c.SetNX(ctx, key, "1", time.Hour)
	if err != nil || !ok {
		t.Fatalf("first SetNX = %v, %v; want true, nil", ok, err)
	}
	ok, err = c.SetNX(ctx, key, "1", time.Hour)
	if err != nil || ok {
		t.Fatalf("second SetNX = %v, %v; want false, nil (dedup)", ok, err)
	}
}

func TestGetString_GivenMissingKey_ThenNotFound(t *testing.T) {
	c, _ := newTestClient(t)
	val, found, err := c.GetString(context.Background(), "nope")
	if err != nil || found || val != "" {
		t.Fatalf("GetString = %q, %v, %v; want \"\", false, nil", val, found, err)
	}
}

func TestSetGetString_GivenTTL_ThenRoundTrips(t *testing.T) {
	c, mr := newTestClient(t)
	key := GateCacheKey(42)
	ctx := context.Background()

	if err := c.SetString(ctx, key, "ok", 2*time.Hour); err != nil {
		t.Fatalf("SetString: %v", err)
	}
	val, found, err := c.GetString(ctx, key)
	if err != nil || !found || val != "ok" {
		t.Fatalf("GetString = %q, %v, %v; want \"ok\", true, nil", val, found, err)
	}
	mr.FastForward(3 * time.Hour)
	_, found, _ = c.GetString(ctx, key)
	if found {
		t.Fatal("key still present after TTL expiry")
	}
}

func TestExists_GivenSetKey_ThenTrue(t *testing.T) {
	c, _ := newTestClient(t)
	ctx := context.Background()
	if err := c.SetString(ctx, BanKey(7), "1", time.Hour); err != nil {
		t.Fatalf("SetString: %v", err)
	}
	ok, err := c.Exists(ctx, BanKey(7))
	if err != nil || !ok {
		t.Fatalf("Exists = %v, %v; want true, nil", ok, err)
	}
	ok, _ = c.Exists(ctx, BanKey(8))
	if ok {
		t.Fatal("Exists returned true for absent key")
	}
}

func TestSlidingWindow_GivenLimit2_ThenAllowsTwoDeniesThird(t *testing.T) {
	c, _ := newTestClient(t)
	ctx := context.Background()
	key := RateLimitKey(99)

	for i := 0; i < 2; i++ {
		ok, err := c.SlidingWindow(ctx, key, time.Minute, 2)
		if err != nil || !ok {
			t.Fatalf("call %d allowed = %v, %v; want true, nil", i+1, ok, err)
		}
	}
	ok, err := c.SlidingWindow(ctx, key, time.Minute, 2)
	if err != nil || ok {
		t.Fatalf("third call allowed = %v, %v; want false, nil", ok, err)
	}
}

func TestSlidingWindow_GivenWindowPassed_ThenAllowsAgain(t *testing.T) {
	c, mr := newTestClient(t)
	ctx := context.Background()
	key := RateLimitKey(100)

	for i := 0; i < 2; i++ {
		_, _ = c.SlidingWindow(ctx, key, time.Minute, 2)
	}
	ok, _ := c.SlidingWindow(ctx, key, time.Minute, 2)
	if ok {
		t.Fatal("third call allowed before window elapsed")
	}
	mr.FastForward(61 * time.Second)
	ok, err := c.SlidingWindow(ctx, key, time.Minute, 2)
	if err != nil || !ok {
		t.Fatalf("call after window = %v, %v; want true, nil", ok, err)
	}
}

func TestTopupFSM_GivenPendingFlow_ThenClearsOnDemand(t *testing.T) {
	c, mr := newTestClient(t)
	ctx := context.Background()
	fsm := NewTopupFSM(c, 10*time.Minute)

	pending, err := fsm.Pending(ctx, 7)
	if err != nil || pending {
		t.Fatalf("initial Pending = %v, %v; want false, nil", pending, err)
	}
	if err := fsm.SetPending(ctx, 7); err != nil {
		t.Fatalf("SetPending: %v", err)
	}
	pending, err = fsm.Pending(ctx, 7)
	if err != nil || !pending {
		t.Fatalf("Pending after set = %v, %v; want true, nil", pending, err)
	}
	if err := fsm.Clear(ctx, 7); err != nil {
		t.Fatalf("Clear: %v", err)
	}
	pending, _ = fsm.Pending(ctx, 7)
	if pending {
		t.Fatal("Pending after clear = true")
	}
	// TTL auto-expiry is the crash guard for abandoned input.
	if err := fsm.SetPending(ctx, 8); err != nil {
		t.Fatalf("SetPending(8): %v", err)
	}
	mr.FastForward(11 * time.Minute)
	pending, _ = fsm.Pending(ctx, 8)
	if pending {
		t.Fatal("Pending still true after TTL expiry")
	}
}

func TestAcquireLock_GivenBusyKey_ThenOnlyFirstWins(t *testing.T) {
	c, _ := newTestClient(t)
	ctx := context.Background()
	key := UserLockKey(5)

	first, err := c.AcquireLock(ctx, key, 30*time.Second)
	if err != nil || !first {
		t.Fatalf("first lock = %v, %v; want true, nil", first, err)
	}
	second, _ := c.AcquireLock(ctx, key, 30*time.Second)
	if second {
		t.Fatal("second lock won while key busy")
	}
}
