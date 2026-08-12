// Package trafficsvc test covers the on-demand per-client refresh (FR-08 AC-3).
//
// @file      internal/service/traffic/refresh_test.go
// @for       RefreshClient happy path, missing client, panel failure.
// @uses      testing, context, time, internal/repository/postgres, internal/repository/xui
// @reason    The manual-refresh path in the account traffic page must never
// fabricate numbers and must stay bounded (AGENTS.md §1.6/§1.7).
// @author    Dodi Rusmana <rusmanadodi@kentangtechstore.com>
// @layer     service
// @stability stable
// @since     2026-08-11
package trafficsvc

import (
	"context"
	"testing"
	"time"

	"github.com/kentangtech/bot-order/internal/repository/xui"
)

// refreshPanel is a minimal Panel for the per-client refresh path.
type refreshPanel struct {
	traffic xui.ClientTraffic
	err     error
}

func (p *refreshPanel) GetInbounds(context.Context) ([]xui.Inbound, error)         { return nil, nil }
func (p *refreshPanel) GetOnlineClients(context.Context) ([]xui.OnlineUser, error) { return nil, nil }
func (p *refreshPanel) GetClientTrafficByEmail(context.Context, string) (xui.ClientTraffic, error) {
	return p.traffic, p.err
}

var _ Panel = (*refreshPanel)(nil)

func TestRefreshClient_GivenTraffic_ThenSingleBatchWritten(t *testing.T) {
	store := &fakeStore{}
	svc := New(store, func(_ context.Context, _ int64) (Panel, error) {
		return &refreshPanel{traffic: xui.ClientTraffic{Email: "a@vpn.kt", Up: 123, Down: 456}}, nil
	}, 200, 30*time.Second, testLogger())

	if err := svc.RefreshClient(context.Background(), 7, 1, "a@vpn.kt"); err != nil {
		t.Fatalf("RefreshClient: %v", err)
	}
	if len(store.batches) != 1 || len(store.batches[0]) != 1 {
		t.Fatalf("batches = %+v, want one update", store.batches)
	}
	u := store.batches[0][0]
	if u.ClientID != 7 || u.Up != 123 || u.Down != 456 {
		t.Errorf("update = %+v, want client 7 up=123 down=456", u)
	}
	if u.LastOnline != nil {
		t.Errorf("manual refresh must not touch last_online: %v", u.LastOnline)
	}
}

func TestRefreshClient_GivenMissingClient_ThenErrorAndNoBatch(t *testing.T) {
	store := &fakeStore{}
	svc := New(store, func(_ context.Context, _ int64) (Panel, error) {
		return &refreshPanel{}, nil // zero-value traffic → Email == ""
	}, 200, 30*time.Second, testLogger())

	if err := svc.RefreshClient(context.Background(), 7, 1, "ghost@vpn.kt"); err == nil {
		t.Fatal("RefreshClient = nil, want not-found error")
	}
	if len(store.batches) != 0 {
		t.Fatalf("batches = %d, want 0", len(store.batches))
	}
}

func TestRefreshClient_GivenPanelFailure_ThenErrorAndNoBatch(t *testing.T) {
	store := &fakeStore{}
	svc := New(store, func(_ context.Context, _ int64) (Panel, error) {
		return &refreshPanel{err: errPanel}, nil
	}, 200, 30*time.Second, testLogger())

	if err := svc.RefreshClient(context.Background(), 7, 1, "a@vpn.kt"); err == nil {
		t.Fatal("RefreshClient = nil, want panel error")
	}
	if len(store.batches) != 0 {
		t.Fatalf("batches = %d, want 0", len(store.batches))
	}
}

// Compile-time check that the store seam stays sufficient for refresh.go.
var _ Store = (*fakeStore)(nil)
