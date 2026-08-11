// Package redis also hosts the daily trial counter.
//
// @file      internal/repository/redis/trial_counter.go
// @for       FR-07 AC-1: per-user daily trial claim counter (INCR + TTL rollover).
// @uses      context, strconv, time, internal/repository/redis (ops.go primitives)
// @reason    Trial limit (maks 2x/hari) must survive restarts and roll over at
// midnight; a Redis counter with end-of-day TTL does both (PRD §16, M6).
// @author    Dodi Rusmana <rusmanadodi@kentangtechstore.com>
// @layer     repository
// @stability experimental
// @since     2026-08-11
package redis

import (
	"context"
	"fmt"
	"strconv"
	"time"

	goredis "github.com/redis/go-redis/v9"
)

// TrialCounter tracks how many trial accounts a user claimed today.
// The counter rolls over when the key expires (TTL until end of day).
type TrialCounter struct {
	client *Client
}

// NewTrialCounter builds the counter on the shared Redis client.
func NewTrialCounter(client *Client) *TrialCounter {
	return &TrialCounter{client: client}
}

// Count returns the number of trial claims today (0 when none yet).
func (c *TrialCounter) Count(ctx context.Context, userID int64) (int64, error) {
	raw, found, err := c.client.GetString(ctx, TrialCounterKey(userID))
	if err != nil {
		return 0, err
	}
	if !found {
		return 0, nil
	}
	n, err := strconv.ParseInt(raw, 10, 64)
	if err != nil {
		return 0, err
	}
	return n, nil
}

// claimScript INCRs and, when the counter is new (value == 1), sets the TTL in
// the same atomic script — a crash between INCR and EXPIRE can never leave a
// TTL-less key that permanently locks the user out of trials.
var claimScript = goredis.NewScript(`
local n = redis.call('INCR', KEYS[1])
if n == 1 then
  redis.call('EXPIRE', KEYS[1], ARGV[1])
end
return n
`)

// Claim increments the counter and reports the new total. The caller checks
// the limit AFTER the increment and rolls back via Rollback when exceeded
// (the increment itself must be atomic — PRD §FR-07 AC-1 anti-race).
func (c *TrialCounter) Claim(ctx context.Context, userID int64, ttl time.Duration) (int64, error) {
	n, err := claimScript.Run(ctx, c.client.rdb, []string{TrialCounterKey(userID)}, int64(ttl.Seconds())).Int64()
	if err != nil {
		return 0, fmt.Errorf("redis trial claim %d: %w", userID, err)
	}
	return n, nil
}

// Rollback decrements an over-limit claim so the denied attempt consumes no quota.
func (c *TrialCounter) Rollback(ctx context.Context, userID int64) error {
	_, err := c.client.Decr(ctx, TrialCounterKey(userID))
	return err
}
