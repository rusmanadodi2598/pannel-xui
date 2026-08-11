// Package telegram test covers the group gate and rate limiter policies.
//
// @file      internal/service/telegram/gate_test.go
// @for       GateService (cache hit, fresh member/denied/unknown, disabled gate) + RateLimiter.
// @uses      testing, context, time, errors, github.com/go-telegram/bot/models
// @reason    Locks FR-01 gate semantics before any dispatcher depends on them.
// @author    Dodi Rusmana <rusmanadodi@kentangtechstore.com>
// @layer     service
// @stability stable
// @since     2026-08-11
package telegram

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/go-telegram/bot/models"
)

var errBoom = errors.New("boom")

type fakeMembership struct {
	mtype models.ChatMemberType
	err   error
	calls int
}

func (f *fakeMembership) GetChatMember(_ context.Context, _, _ int64) (models.ChatMemberType, error) {
	f.calls++
	return f.mtype, f.err
}

type fakeKV struct {
	vals map[string]string
}

func newFakeKV() *fakeKV { return &fakeKV{vals: map[string]string{}} }

func (f *fakeKV) GetString(_ context.Context, key string) (string, bool, error) {
	v, ok := f.vals[key]
	return v, ok, nil
}

func (f *fakeKV) SetString(_ context.Context, key, value string, ttl time.Duration) error {
	f.vals[key] = value
	return nil
}

func TestGate_GivenDisabled_ThenAlwaysAllowed(t *testing.T) {
	g := NewGateService(&fakeMembership{}, newFakeKV(), 0)
	if g.Check(context.Background(), 1) != GateAllowed {
		t.Fatal("disabled gate must allow everyone")
	}
	if g.CheckFresh(context.Background(), 1) != GateAllowed {
		t.Fatal("disabled gate must allow fresh checks too")
	}
}

func TestGate_GivenCachedMember_ThenSkipsAPI(t *testing.T) {
	api := &fakeMembership{mtype: models.ChatMemberTypeMember}
	store := newFakeKV()
	_ = store.SetString(context.Background(), GateCacheKey(7), "ok", GateCacheTTL)
	g := NewGateService(api, store, -100123)

	if res := g.Check(context.Background(), 7); res != GateAllowed {
		t.Fatalf("result = %v, want allowed", res)
	}
	if api.calls != 0 {
		t.Fatalf("API called %d times, want 0 (cache hit)", api.calls)
	}
}

func TestGate_GivenMember_ThenAllowedAndCached(t *testing.T) {
	api := &fakeMembership{mtype: models.ChatMemberTypeMember}
	store := newFakeKV()
	g := NewGateService(api, store, -100123)

	if res := g.Check(context.Background(), 7); res != GateAllowed {
		t.Fatalf("result = %v, want allowed", res)
	}
	if _, ok := store.vals[GateCacheKey(7)]; !ok {
		t.Fatal("member result must be cached")
	}
	// Second check must not hit the API again.
	if res := g.Check(context.Background(), 7); res != GateAllowed {
		t.Fatalf("cached result = %v", res)
	}
	if api.calls != 1 {
		t.Fatalf("API calls = %d, want 1", api.calls)
	}
}

func TestGate_GivenAdministrator_ThenAllowed(t *testing.T) {
	g := NewGateService(&fakeMembership{mtype: models.ChatMemberTypeAdministrator}, newFakeKV(), -100123)
	if res := g.Check(context.Background(), 1); res != GateAllowed {
		t.Fatalf("result = %v, want allowed", res)
	}
}

func TestGate_GivenLeftMember_ThenDenied(t *testing.T) {
	g := NewGateService(&fakeMembership{mtype: models.ChatMemberTypeLeft}, newFakeKV(), -100123)
	if res := g.Check(context.Background(), 1); res != GateDenied {
		t.Fatalf("result = %v, want denied", res)
	}
}

func TestGate_GivenAPIError_ThenUnknown(t *testing.T) {
	g := NewGateService(&fakeMembership{err: errBoom}, newFakeKV(), -100123)
	if res := g.Check(context.Background(), 1); res != GateUnknown {
		t.Fatalf("result = %v, want unknown (fail closed)", res)
	}
}

func TestGateCheckFresh_GivenLeftThenMember_ThenRechecksWithoutCache(t *testing.T) {
	api := &fakeMembership{mtype: models.ChatMemberTypeLeft}
	store := newFakeKV()
	g := NewGateService(api, store, -100123)

	if res := g.CheckFresh(context.Background(), 1); res != GateDenied {
		t.Fatalf("first fresh = %v, want denied", res)
	}
	api.mtype = models.ChatMemberTypeMember
	if res := g.CheckFresh(context.Background(), 1); res != GateAllowed {
		t.Fatalf("second fresh = %v, want allowed", res)
	}
}

type fakeRateStore struct {
	allowed bool
	err     error
	key     string
	window  time.Duration
	limit   int
}

func (f *fakeRateStore) SlidingWindow(_ context.Context, key string, window time.Duration, limit int) (bool, error) {
	f.key, f.window, f.limit = key, window, limit
	return f.allowed, f.err
}

func TestRateLimiter_GivenBudget_ThenAllows(t *testing.T) {
	store := &fakeRateStore{allowed: true}
	l := NewRateLimiter(store, 30, RateLimitWindow)
	ok, err := l.Allow(context.Background(), 9)
	if err != nil || !ok {
		t.Fatalf("Allow = %v, %v; want true, nil", ok, err)
	}
	if store.key != RateLimitKey(9) || store.limit != 30 || store.window != RateLimitWindow {
		t.Errorf("store got key=%q limit=%d window=%v", store.key, store.limit, store.window)
	}
}

func TestRateLimiter_GivenExhausted_ThenDenies(t *testing.T) {
	l := NewRateLimiter(&fakeRateStore{allowed: false}, 30, RateLimitWindow)
	ok, _ := l.Allow(context.Background(), 9)
	if ok {
		t.Fatal("Allow must deny when budget exhausted")
	}
}
