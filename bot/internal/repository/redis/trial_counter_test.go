// Package redis test covers the FR-07 daily trial counter.
//
// @file      internal/repository/redis/trial_counter_test.go
// @for       miniredis: Count empty, Claim increments + sets TTL, Rollback decrements.
// @uses      testing, context, time, github.com/alicebob/miniredis/v2
// @reason    The trial counter backs a daily quota — TTL rollover and rollback
// semantics must hold against a real Redis (AGENTS.md §2.1).
// @author    Dodi Rusmana <rusmanadodi@kentangtechstore.com>
// @layer     repository
// @stability experimental
// @since     2026-08-11
package redis

import (
	"context"
	"testing"
	"time"
)

func TestTrialCounter_GivenNoClaims_ThenCountZero(t *testing.T) {
	c, _ := newTestClient(t)
	counter := NewTrialCounter(c)
	n, err := counter.Count(context.Background(), 7)
	if err != nil || n != 0 {
		t.Fatalf("Count = %d, %v; want 0, nil", n, err)
	}
}

func TestTrialCounter_GivenClaims_ThenIncrementsAndSetsTTL(t *testing.T) {
	c, mr := newTestClient(t)
	counter := NewTrialCounter(c)
	ctx := context.Background()

	n, err := counter.Claim(ctx, 7, 2*time.Hour)
	if err != nil || n != 1 {
		t.Fatalf("first Claim = %d, %v; want 1, nil", n, err)
	}
	n, err = counter.Claim(ctx, 7, 2*time.Hour)
	if err != nil || n != 2 {
		t.Fatalf("second Claim = %d, %v; want 2, nil", n, err)
	}
	got, _ := counter.Count(ctx, 7)
	if got != 2 {
		t.Fatalf("Count = %d, want 2", got)
	}
	if ttl := mr.TTL(TrialCounterKey(7)); ttl != 2*time.Hour {
		t.Fatalf("TTL = %v, want 2h (set only on first claim)", ttl)
	}
}

func TestTrialCounter_GivenTTLExpiry_ThenRollsOver(t *testing.T) {
	c, mr := newTestClient(t)
	counter := NewTrialCounter(c)
	ctx := context.Background()

	_, _ = counter.Claim(ctx, 7, 2*time.Hour)
	mr.FastForward(3 * time.Hour)
	n, err := counter.Count(ctx, 7)
	if err != nil || n != 0 {
		t.Fatalf("Count after expiry = %d, %v; want 0 (new day)", n, err)
	}
	n, err = counter.Claim(ctx, 7, 2*time.Hour)
	if err != nil || n != 1 {
		t.Fatalf("Claim after expiry = %d, %v; want 1 (fresh quota)", n, err)
	}
}

func TestTrialCounter_GivenRollback_ThenDecrements(t *testing.T) {
	c, _ := newTestClient(t)
	counter := NewTrialCounter(c)
	ctx := context.Background()

	_, _ = counter.Claim(ctx, 7, 2*time.Hour)
	_, _ = counter.Claim(ctx, 7, 2*time.Hour)
	if err := counter.Rollback(ctx, 7); err != nil {
		t.Fatalf("Rollback: %v", err)
	}
	n, _ := counter.Count(ctx, 7)
	if n != 1 {
		t.Fatalf("Count after rollback = %d, want 1", n)
	}
}
