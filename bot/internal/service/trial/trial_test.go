// Package trialsvc_test covers the FR-07 daily limit policy.
//
// @file      internal/service/trial/trial_test.go
// @for       Unit tests: Enabled, Remaining, Claim limit + rollback, midnight TTL.
// @uses      testing, context, errors, time
// @reason    The trial limit is a financial-adjacent policy — every edge is
// locked before the handler flow is wired (AGENTS.md §2.1).
// @author    Dodi Rusmana <rusmanadodi@kentangtechstore.com>
// @layer     service
// @stability experimental
// @since     2026-08-11
package trialsvc

import (
	"context"
	"errors"
	"testing"
	"time"
)

type fakeCounter struct {
	count   int64
	claimed []int64
}

func (f *fakeCounter) Count(context.Context, int64) (int64, error) { return f.count, nil }
func (f *fakeCounter) Claim(_ context.Context, _ int64, _ time.Duration) (int64, error) {
	f.count++
	f.claimed = append(f.claimed, f.count)
	return f.count, nil
}
func (f *fakeCounter) Rollback(_ context.Context, _ int64) error {
	f.count--
	return nil
}

func newSvc(counter Counter) *Service {
	loc, _ := time.LoadLocation("Asia/Jakarta")
	return New(counter, true, 2, 1, 1, 1, loc)
}

func TestEnabled_GivenOn_ThenTrue(t *testing.T) {
	if !newSvc(&fakeCounter{}).Enabled() {
		t.Fatal("Enabled = false, want true")
	}
	off := New(&fakeCounter{}, false, 2, 1, 1, 1, time.UTC)
	if off.Enabled() {
		t.Fatal("Enabled = true with enabled=false, want false")
	}
}

func TestRemaining_GivenSomeClaims_ThenLimitMinusCount(t *testing.T) {
	svc := newSvc(&fakeCounter{count: 1})
	rem, err := svc.Remaining(context.Background(), 7)
	if err != nil || rem != 1 {
		t.Fatalf("Remaining = %d, %v; want 1, nil", rem, err)
	}
}

func TestRemaining_GivenOverLimit_ThenClampedToZero(t *testing.T) {
	svc := newSvc(&fakeCounter{count: 5})
	rem, _ := svc.Remaining(context.Background(), 7)
	if rem != 0 {
		t.Fatalf("Remaining = %d, want 0 (clamped)", rem)
	}
}

func TestClaim_GivenWithinLimit_ThenAllowed(t *testing.T) {
	svc := newSvc(&fakeCounter{})
	n, err := svc.Claim(context.Background(), 7)
	if err != nil || n != 1 {
		t.Fatalf("Claim = %d, %v; want 1, nil", n, err)
	}
}

func TestClaim_GivenLimitReached_ThenErrAndRollback(t *testing.T) {
	f := &fakeCounter{count: 2}
	svc := newSvc(f)
	_, err := svc.Claim(context.Background(), 7)
	if !errors.Is(err, ErrDailyLimitReached) {
		t.Fatalf("Claim err = %v, want ErrDailyLimitReached", err)
	}
	if f.count != 2 {
		t.Fatalf("counter after rollback = %d, want 2 (denied claim consumes nothing)", f.count)
	}
}

func TestTTLUntilMidnight_GivenJustBeforeMidnight_ThenShort(t *testing.T) {
	loc, _ := time.LoadLocation("Asia/Jakarta")
	svc := &Service{loc: loc}
	now := time.Date(2026, 8, 11, 23, 59, 30, 0, loc)
	ttl := svc.ttlUntilMidnight(now)
	if ttl <= 0 || ttl > 31*time.Second {
		t.Fatalf("ttl = %v, want ~30s", ttl)
	}
}

func TestTTLUntilMidnight_GivenMidday_ThenManyHours(t *testing.T) {
	loc, _ := time.LoadLocation("Asia/Jakarta")
	svc := &Service{loc: loc}
	now := time.Date(2026, 8, 11, 12, 0, 0, 0, loc)
	ttl := svc.ttlUntilMidnight(now)
	if ttl < 11*time.Hour || ttl > 13*time.Hour {
		t.Fatalf("ttl = %v, want ~12h", ttl)
	}
}

var _ Counter = (*fakeCounter)(nil)
