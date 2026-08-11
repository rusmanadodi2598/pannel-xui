// Package trialsvc enforces the daily trial limit and holds trial policy (FR-07).
//
// @file      internal/service/trial/trial.go
// @for       FR-07: enabled flag, daily limit (maks 2x/hari), claim anti-race.
// @uses      context, errors, time, internal/repository/redis
// @reason    Trial abuse is a product risk — the limit must be checked at menu,
// server pick AND confirm (PRD FR-07 AC-1); the service owns that policy.
// @author    Dodi Rusmana <rusmanadodi@kentangtechstore.com>
// @layer     service
// @stability experimental
// @since     2026-08-11
package trialsvc

import (
	"context"
	"errors"
	"time"
)

// ErrDailyLimitReached is returned when the user used all trial slots today.
var ErrDailyLimitReached = errors.New("trial harian sudah habis")

// Counter is the daily claim store (redis.TrialCounter implements it).
type Counter interface {
	Count(ctx context.Context, userID int64) (int64, error)
	Claim(ctx context.Context, userID int64, ttl time.Duration) (int64, error)
	Rollback(ctx context.Context, userID int64) error
}

// Service enforces the FR-07 trial policy and carries the provisioning defaults.
type Service struct {
	counter Counter
	enabled bool
	limit   int
	hours   int
	traffic int
	ipLimit int
	loc     *time.Location
}

// New builds the trial service. enabled=false disables the feature entirely;
// hours/traffic/ipLimit are the account defaults (TRIAL_DURATION_HOURS/GB/IP).
func New(counter Counter, enabled bool, limit, hours, traffic, ipLimit int, loc *time.Location) *Service {
	return &Service{counter: counter, enabled: enabled, limit: limit, hours: hours, traffic: traffic, ipLimit: ipLimit, loc: loc}
}

// Enabled reports whether the trial feature is on (FR-07).
func (s *Service) Enabled() bool { return s.enabled }

// Limit returns the maximum trial claims per day (config TRIAL_DAILY_LIMIT).
func (s *Service) Limit() int { return s.limit }

// Hours returns the trial account duration (config TRIAL_DURATION_HOURS).
func (s *Service) Hours() int { return s.hours }

// TrafficGB returns the trial quota in GB (config TRIAL_TRAFFIC_GB).
func (s *Service) TrafficGB() int { return s.traffic }

// IPLimit returns the trial per-client IP limit (config TRIAL_IP_LIMIT).
func (s *Service) IPLimit() int { return s.ipLimit }

// Remaining returns how many trial slots are left today (menu/server pick check).
func (s *Service) Remaining(ctx context.Context, userID int64) (int, error) {
	n, err := s.counter.Count(ctx, userID)
	if err != nil {
		return 0, err
	}
	rem := s.limit - int(n)
	if rem < 0 {
		return 0, nil
	}
	return rem, nil
}

// Claim atomically increments the counter; when the new total exceeds the
// daily limit the claim is rolled back and ErrDailyLimitReached returned
// (the confirm step is the anti-race gate — FR-07 AC-1).
func (s *Service) Claim(ctx context.Context, userID int64) (int, error) {
	n, err := s.counter.Claim(ctx, userID, s.ttlUntilMidnight(time.Now()))
	if err != nil {
		return 0, err
	}
	if n > int64(s.limit) {
		_ = s.counter.Rollback(ctx, userID)
		return 0, ErrDailyLimitReached
	}
	return int(n), nil
}

// ttlUntilMidnight returns the time until the next local midnight in the
// configured location — the counter expires exactly when a new day starts.
func (s *Service) ttlUntilMidnight(now time.Time) time.Duration {
	next := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, s.loc).AddDate(0, 0, 1)
	return next.Sub(now)
}
