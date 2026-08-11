// Package trafficsvc test covers the sweep orchestration.
//
// @file      internal/service/traffic/traffic_test.go
// @for       Grouped-per-server fetch, batch write, online flag, missing/skipped clients, server failure isolation.
// @uses      testing, context, errors, time, internal/repository/postgres, internal/repository/xui
// @reason    Guards the PRD §16.2 contract: one panel down never fails the whole sweep.
// @author    Dodi Rusmana <rusmanadodi@kentangtechstore.com>
// @layer     service
// @stability stable
// @since     2026-08-11
package trafficsvc

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"testing"
	"time"

	"github.com/kentangtech/bot-order/internal/repository/postgres"
	"github.com/kentangtech/bot-order/internal/repository/xui"
)

var errPanel = errors.New("panel down")

type fakeStore struct {
	cands   []postgres.TrafficCandidate
	batches [][]postgres.TrafficUpdate
}

func (f *fakeStore) ListTrafficCandidates(context.Context, int) ([]postgres.TrafficCandidate, error) {
	return f.cands, nil
}

func (f *fakeStore) SyncTrafficBatch(_ context.Context, _ time.Time, ups []postgres.TrafficUpdate) error {
	f.batches = append(f.batches, ups)
	return nil
}

type fakePanel struct {
	inbounds []xui.Inbound
	online   []xui.OnlineUser
	err      error
}

func (f *fakePanel) GetInbounds(context.Context) ([]xui.Inbound, error) {
	if f.err != nil {
		return nil, f.err
	}
	return f.inbounds, nil
}

func (f *fakePanel) GetOnlineClients(context.Context) ([]xui.OnlineUser, error) {
	if f.err != nil {
		return nil, f.err
	}
	return f.online, nil
}

func testLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(io.Discard, nil))
}

func cands(startID int64, serverID int64, emails ...string) []postgres.TrafficCandidate {
	out := make([]postgres.TrafficCandidate, 0, len(emails))
	for i, e := range emails {
		out = append(out, postgres.TrafficCandidate{ClientID: startID + int64(i), ServerID: serverID, Email: e})
	}
	return out
}

func inboundStats(stats ...xui.ClientTraffic) []xui.Inbound {
	return []xui.Inbound{{
		ID: 1, Enable: true, Port: 443, Protocol: "vless",
		ClientStats: stats,
	}}
}

func TestRunOnce_GivenCandidatesOnTwoServers_ThenOneBatchPerServer(t *testing.T) {
	store := &fakeStore{cands: append(cands(1, 1, "a@vpn.kt", "b@vpn.kt"), cands(3, 2, "c@vpn.kt")...)}
	svc := New(store, func(_ context.Context, serverID int64) (Panel, error) {
		if serverID == 1 {
			return &fakePanel{inbounds: inboundStats(
				xui.ClientTraffic{Email: "a@vpn.kt", Up: 100, Down: 200},
				xui.ClientTraffic{Email: "b@vpn.kt", Up: 50, Down: 25},
			)}, nil
		}
		return &fakePanel{inbounds: inboundStats(
			xui.ClientTraffic{Email: "c@vpn.kt", Up: 5, Down: 6},
		)}, nil
	}, 200, 30*time.Second, testLogger())

	if err := svc.RunOnce(context.Background()); err != nil {
		t.Fatalf("RunOnce: %v", err)
	}
	if len(store.batches) != 2 {
		t.Fatalf("batches = %d, want 1 per server (2)", len(store.batches))
	}
	got := map[int64]postgres.TrafficUpdate{}
	for _, u := range store.batches[0] {
		got[u.ClientID] = u
	}
	if u := got[1]; u.Up != 100 || u.Down != 200 {
		t.Errorf("client 1 traffic = %+v, want up=100 down=200", u)
	}
	if u := got[2]; u.Up != 50 || u.Down != 25 {
		t.Errorf("client 2 traffic = %+v, want up=50 down=25", u)
	}
	// Server 2 → client 3.
	last := store.batches[1]
	if len(last) != 1 || last[0].ClientID != 3 || last[0].Up != 5 || last[0].Down != 6 {
		t.Errorf("server 2 batch = %+v, want single client 3 up=5 down=6", last)
	}
}

func TestRunOnce_GivenOnlineClient_ThenLastOnlineSetOnlyForIt(t *testing.T) {
	store := &fakeStore{cands: cands(1, 1, "a@vpn.kt", "b@vpn.kt")}
	svc := New(store, func(_ context.Context, _ int64) (Panel, error) {
		return &fakePanel{
			inbounds: inboundStats(
				xui.ClientTraffic{Email: "a@vpn.kt", Up: 10, Down: 10},
				xui.ClientTraffic{Email: "b@vpn.kt", Up: 20, Down: 20},
			),
			online: []xui.OnlineUser{{Email: "a@vpn.kt"}},
		}, nil
	}, 200, 30*time.Second, testLogger())

	if err := svc.RunOnce(context.Background()); err != nil {
		t.Fatalf("RunOnce: %v", err)
	}
	var a, b *time.Time
	for _, u := range store.batches[0] {
		switch u.ClientID {
		case 1:
			a = u.LastOnline
		case 2:
			b = u.LastOnline
		}
	}
	if a == nil {
		t.Error("client 1 (online) LastOnline = nil, want set")
	}
	if b != nil {
		t.Error("client 2 (offline) LastOnline = non-nil, want nil (keep previous)")
	}
}

func TestRunOnce_GivenClientMissingOnPanel_ThenSkipped(t *testing.T) {
	store := &fakeStore{cands: cands(1, 1, "present@vpn.kt", "ghost@vpn.kt")}
	svc := New(store, func(_ context.Context, _ int64) (Panel, error) {
		return &fakePanel{inbounds: inboundStats(
			xui.ClientTraffic{Email: "present@vpn.kt", Up: 1, Down: 2},
		)}, nil
	}, 200, 30*time.Second, testLogger())

	if err := svc.RunOnce(context.Background()); err != nil {
		t.Fatalf("RunOnce: %v", err)
	}
	if len(store.batches) != 1 || len(store.batches[0]) != 1 || store.batches[0][0].ClientID != 1 {
		t.Fatalf("batch = %+v, want only the present client", store.batches)
	}
}

func TestRunOnce_GivenNoCandidates_ThenNoCalls(t *testing.T) {
	store := &fakeStore{}
	svc := New(store, func(_ context.Context, _ int64) (Panel, error) {
		t.Fatal("panel must not be built with no candidates")
		return nil, nil
	}, 200, 30*time.Second, testLogger())

	if err := svc.RunOnce(context.Background()); err != nil {
		t.Fatalf("RunOnce: %v", err)
	}
	if len(store.batches) != 0 {
		t.Fatalf("batches = %d, want 0", len(store.batches))
	}
}

func TestRunOnce_GivenOneServerFails_ThenOtherStillSyncsAndErrorReturned(t *testing.T) {
	store := &fakeStore{cands: append(cands(1, 1, "a@vpn.kt"), cands(2, 2, "b@vpn.kt")...)}
	svc := New(store, func(_ context.Context, serverID int64) (Panel, error) {
		if serverID == 1 {
			return &fakePanel{err: errPanel}, nil
		}
		return &fakePanel{inbounds: inboundStats(xui.ClientTraffic{Email: "b@vpn.kt", Up: 7, Down: 8})}, nil
	}, 200, 30*time.Second, testLogger())

	err := svc.RunOnce(context.Background())
	if err == nil {
		t.Fatal("RunOnce = nil, want error reporting the failed panel")
	}
	if len(store.batches) != 1 || store.batches[0][0].ClientID != 2 {
		t.Fatalf("batches = %+v, want only server 2 synced", store.batches)
	}
}
