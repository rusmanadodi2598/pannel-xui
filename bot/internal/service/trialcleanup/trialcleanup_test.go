// Package trialcleanupsvc test covers the cleanup sweep (AGENTS.md §2.1).
//
// @file      internal/service/trialcleanup/trialcleanup_test.go
// @for       Unit tests: disable+mark flow, panel-failure isolation, no-candidate short-circuit.
// @uses      testing, context, errors, io, log/slog, time, internal/repository/postgres
// @reason    Guards the cleanup contract: a trial row is marked expired ONLY
// after its panel disable succeeded — a panel error must never mark rows.
// @author    Dodi Rusmana <rusmanadodi@kentangtechstore.com>
// @layer     service
// @stability stable
// @since     2026-08-12
package trialcleanupsvc

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"sync"
	"testing"
	"time"

	"github.com/kentangtech/bot-order/internal/repository/postgres"
)

var errPanelFail = errors.New("panel failure")

type fakeCleanupStore struct {
	cands  []postgres.ExpiredTrialCandidate
	marked []int64
}

func (f *fakeCleanupStore) ListExpiredTrialCandidates(context.Context, int) ([]postgres.ExpiredTrialCandidate, error) {
	return f.cands, nil
}

func (f *fakeCleanupStore) MarkTrialExpired(_ context.Context, clientID int64) error {
	f.marked = append(f.marked, clientID)
	return nil
}

type fakeDisabler struct {
	disabled map[int64][]string // serverID → emails sent to the panel
	failOn   map[int64][]string // serverID → emails that fail
}

func (f *fakeDisabler) DisableClients(_ context.Context, serverID int64, emails []string) ([]string, error) {
	f.disabled[serverID] = append(f.disabled[serverID], emails...)
	var failed []string
	for _, e := range emails {
		for _, fe := range f.failOn[serverID] {
			if fe == e {
				failed = append(failed, e)
			}
		}
	}
	if len(failed) > 0 {
		return failed, errPanelFail
	}
	return nil, nil
}

// partialTimeoutDisabler confirms the first email, then blocks until the
// per-server budget expires — the panel did disable one client before dying.
type partialTimeoutDisabler struct {
	mu       sync.Mutex
	disabled []string
}

func (f *partialTimeoutDisabler) DisableClients(ctx context.Context, _ int64, emails []string) ([]string, error) {
	f.mu.Lock()
	f.disabled = append(f.disabled, emails[:1]...)
	f.mu.Unlock()
	<-ctx.Done()
	return emails[1:], ctx.Err() // only the first email was confirmed disabled
}

func testCleanupLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(io.Discard, nil))
}

func cand(clientID, serverID int64, email, protocol string) postgres.ExpiredTrialCandidate {
	return postgres.ExpiredTrialCandidate{ClientID: clientID, ServerID: serverID, Email: email, Protocol: protocol}
}

func TestRunOnce_GivenExpiredTrials_ThenDisabledAndMarked(t *testing.T) {
	store := &fakeCleanupStore{cands: []postgres.ExpiredTrialCandidate{
		cand(1, 1, "t1@vpn.kt", "vless"),
		cand(2, 1, "t2@vpn.kt", "trojan"),
		cand(3, 2, "t3@vpn.kt", "vless"),
	}}
	disabler := &fakeDisabler{disabled: map[int64][]string{}}
	svc := New(store, disabler, 50, 30*time.Second, testCleanupLogger())

	if err := svc.RunOnce(context.Background()); err != nil {
		t.Fatalf("RunOnce: %v", err)
	}
	if got := len(disabler.disabled[1]); got != 2 {
		t.Errorf("server 1 disabled = %d emails, want 2", got)
	}
	if got := len(disabler.disabled[2]); got != 1 {
		t.Errorf("server 2 disabled = %d emails, want 1", got)
	}
	if len(store.marked) != 3 {
		t.Fatalf("marked = %d, want 3 (all successfully disabled)", len(store.marked))
	}
}

func TestRunOnce_GivenPanelFailure_ThenAffectedRowsNotMarked(t *testing.T) {
	store := &fakeCleanupStore{cands: []postgres.ExpiredTrialCandidate{
		cand(1, 1, "t1@vpn.kt", "vless"),
		cand(2, 1, "t2@vpn.kt", "trojan"),
	}}
	disabler := &fakeDisabler{
		disabled: map[int64][]string{},
		failOn:   map[int64][]string{1: {"t2@vpn.kt"}},
	}
	svc := New(store, disabler, 50, 30*time.Second, testCleanupLogger())

	err := svc.RunOnce(context.Background())
	if err == nil {
		t.Fatal("RunOnce = nil, want error reporting the failed disable")
	}
	if len(store.marked) != 1 || store.marked[0] != 1 {
		t.Fatalf("marked = %v, want only client 1 (client 2's panel disable failed)", store.marked)
	}
}

// TestRunOnce_GivenPartialDisableThenTimeout_ThenOnlyConfirmedMarked locks the
// v1.45 fix: the mark for the confirmed client must succeed even after the
// per-server budget is exhausted (DB write uses the parent context), and
// nothing else gets marked.
func TestRunOnce_GivenPartialDisableThenTimeout_ThenOnlyConfirmedMarked(t *testing.T) {
	store := &fakeCleanupStore{cands: []postgres.ExpiredTrialCandidate{
		cand(1, 1, "t1@vpn.kt", "vless"),
		cand(2, 1, "t2@vpn.kt", "trojan"),
	}}
	disabler := &partialTimeoutDisabler{}
	svc := New(store, disabler, 50, 50*time.Millisecond, testCleanupLogger())

	if err := svc.RunOnce(context.Background()); err == nil {
		t.Fatal("RunOnce = nil, want error reporting the timed-out server")
	}
	if len(store.marked) != 1 || store.marked[0] != 1 {
		t.Fatalf("marked = %v, want only client 1 (confirmed disabled before timeout)", store.marked)
	}
}

func TestRunOnce_GivenNoCandidates_ThenNoCalls(t *testing.T) {
	store := &fakeCleanupStore{}
	disabler := &fakeDisabler{disabled: map[int64][]string{}}
	svc := New(store, disabler, 50, 30*time.Second, testCleanupLogger())

	if err := svc.RunOnce(context.Background()); err != nil {
		t.Fatalf("RunOnce: %v", err)
	}
	if len(disabler.disabled) != 0 || len(store.marked) != 0 {
		t.Fatalf("panel calls = %v, marks = %v; want none", disabler.disabled, store.marked)
	}
}

func TestRunOnce_GivenOneServerFails_ThenOtherStillCleaned(t *testing.T) {
	store := &fakeCleanupStore{cands: []postgres.ExpiredTrialCandidate{
		cand(1, 1, "t1@vpn.kt", "vless"),
		cand(2, 2, "t2@vpn.kt", "vless"),
	}}
	disabler := &fakeDisabler{
		disabled: map[int64][]string{},
		failOn:   map[int64][]string{1: {"t1@vpn.kt"}},
	}
	svc := New(store, disabler, 50, 30*time.Second, testCleanupLogger())

	err := svc.RunOnce(context.Background())
	if err == nil {
		t.Fatal("RunOnce = nil, want error reporting server 1")
	}
	if len(store.marked) != 1 || store.marked[0] != 2 {
		t.Fatalf("marked = %v, want only client 2 (server 2 still cleaned)", store.marked)
	}
}
