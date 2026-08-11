// Package telegram hosts webhook registration, gate/ban/rate-limit policies and menu views.
//
// @file      internal/service/telegram/keys.go
// @for       TTL policies and key helpers shared by gate, ban, rate limit and webhook dedup.
// @uses      time, internal/repository/redis
// @reason    Keeps every TTL and key shape in one place (PRD §14.2/14.3, FR-01).
// @author    Dodi Rusmana <rusmanadodi@kentangtechstore.com>
// @layer     service
// @stability stable
// @since     2026-08-11
package telegram

import (
	"time"

	"github.com/kentangtech/bot-order/internal/repository/redis"
)

// Policy constants (PRD §14.1/14.2, FR-01).
const (
	// GateCacheTTL caches a successful group-membership check (FR-01: 6 jam).
	GateCacheTTL = 6 * time.Hour
	// UpdateDedupTTL keeps the update_id dedup marker (PRD §14.3: 24 jam).
	UpdateDedupTTL = 24 * time.Hour
	// RateLimitWindow is the sliding window for per-user throttling (30/menit).
	RateLimitWindow = time.Minute
	// UserLockTTL bounds the per-user serialization lock (PRD §14.2.4).
	UserLockTTL = 30 * time.Second
)

// Key helpers delegate to the repository-level builders (single source of truth).
func GateCacheKey(userID int64) string { return redis.GateCacheKey(userID) }
func BanKey(userID int64) string       { return redis.BanKey(userID) }
func RateLimitKey(userID int64) string { return redis.RateLimitKey(userID) }
