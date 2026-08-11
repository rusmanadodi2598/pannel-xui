// Package telegram hosts webhook registration, gate/ban/rate-limit policies and menu views.
//
// @file      internal/service/telegram/ratelimit.go
// @for       Per-user sliding-window rate limit (PRD §14.2.5, 30/menit).
// @uses      context, time
// @reason    Keeps throttling policy behind a tiny seam for unit testing.
// @author    Dodi Rusmana <rusmanadodi@kentangtechstore.com>
// @layer     service
// @stability stable
// @since     2026-08-11
package telegram

import (
	"context"
	"time"
)

// RateStore is the sliding-window seam (implemented by repository/redis).
type RateStore interface {
	SlidingWindow(ctx context.Context, key string, window time.Duration, limit int) (bool, error)
}

// RateLimiter throttles a user to limit requests per window (default 30/min).
type RateLimiter struct {
	store  RateStore
	limit  int
	window time.Duration
}

// NewRateLimiter wires the limiter with its policy.
func NewRateLimiter(store RateStore, limit int, window time.Duration) *RateLimiter {
	return &RateLimiter{store: store, limit: limit, window: window}
}

// Allow reports whether the user may proceed. Errors are surfaced to the
// caller so the dispatcher can choose fail-open for throttling (avoids a
// Redis blip killing the whole bot).
func (l *RateLimiter) Allow(ctx context.Context, userID int64) (bool, error) {
	return l.store.SlidingWindow(ctx, RateLimitKey(userID), l.window, l.limit)
}
