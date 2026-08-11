// Package expirysvc test covers the FR-09 sweep semantics.
//
// @file      internal/service/expiry/expiry_test.go
// @for       Given clients in windows, then each notified once; send failure never marks.
// @uses      testing, context, io, log/slog, time, github.com/go-telegram/bot/models,
// internal/repository/postgres
// @reason    Guards the anti-spam contract (FR-09 AC) without any network.
// @author    Dodi Rusmana <rusmanadodi@kentangtechstore.com>
// @layer     service
// @stability stable
// @since     2026-08-11
package expirysvc

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"testing"
	"time"

	"github.com/go-telegram/bot/models"
	"github.com/kentangtech/bot-order/internal/repository/postgres"
)

var errBoom = errors.New("boom")

func testLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(io.Discard, nil))
}

type fakeStore struct {
	candidates []postgres.ExpiryCandidate
	listErr    error
	markErr    error
	gotUpper   []int // call log: upper days per list
	gotLower   []int
	marked     map[int64]int // clientID → day
}

// ListExpiryCandidates mirrors the real repo's window filter (exclusive (lower, upper])
// so the service-under-test sees the same per-window subset as production.
func (f *fakeStore) ListExpiryCandidates(_ context.Context, upper, lower, _ int) ([]postgres.ExpiryCandidate, error) {
	f.gotUpper = append(f.gotUpper, upper)
	f.gotLower = append(f.gotLower, lower)
	if f.listErr != nil {
		return nil, f.listErr
	}
	now := time.Now()
	var out []postgres.ExpiryCandidate
	for _, c := range f.candidates {
		remDays := c.ExpiresAt.Sub(now).Hours() / 24
		if remDays > float64(lower) && remDays <= float64(upper) {
			out = append(out, c)
		}
	}
	return out, nil
}

func (f *fakeStore) MarkNotified(_ context.Context, clientID int64, day int) error {
	if f.marked == nil {
		f.marked = map[int64]int{}
	}
	if f.markErr == nil {
		f.marked[clientID] = day
	}
	return f.markErr
}

type fakeSender struct {
	sent []int64 // chat IDs
	err  error
}

func (f *fakeSender) SendMessage(_ context.Context, chatID int64, _ string, _ models.ParseMode, _ models.ReplyMarkup) error {
	f.sent = append(f.sent, chatID)
	return f.err
}

func cand(id, chatID int64, server, email string, exp time.Time) postgres.ExpiryCandidate {
	return postgres.ExpiryCandidate{ClientID: id, TelegramID: chatID, Email: email, ServerName: server, ExpiresAt: exp}
}

func TestRunOnce_GivenCandidatesInEveryWindow_ThenEachNotifiedOnce(t *testing.T) {
	store := &fakeStore{candidates: []postgres.ExpiryCandidate{
		cand(1, 1001, "ID-01", "a@vpn.kt", time.Now().Add(5*24*time.Hour)),
		cand(2, 1002, "SG-01", "b@vpn.kt", time.Now().Add(2*24*time.Hour)),
		cand(3, 1003, "JP-01", "c@vpn.kt", time.Now().Add(12*time.Hour)),
	}}
	sender := &fakeSender{}
	s := New(store, sender, []int{7, 3, 1}, 50, time.UTC, testLogger())

	if err := s.RunOnce(context.Background()); err != nil {
		t.Fatalf("RunOnce: %v", err)
	}

	// Windows must be swept descending: (7,3], (3,1], (1,0].
	wantU := []int{7, 3, 1}
	wantL := []int{3, 1, 0}
	for i := range wantU {
		if store.gotUpper[i] != wantU[i] || store.gotLower[i] != wantL[i] {
			t.Fatalf("window %d = (%d,%d), want (%d,%d)",
				i, store.gotUpper[i], store.gotLower[i], wantU[i], wantL[i])
		}
	}
	if len(sender.sent) != 3 {
		t.Fatalf("sent = %d, want 3", len(sender.sent))
	}
	if len(store.marked) != 3 {
		t.Fatalf("marked = %d, want 3", len(store.marked))
	}
	if store.marked[1] != 7 || store.marked[2] != 3 || store.marked[3] != 1 {
		t.Errorf("marked days = %v, want {1:7 2:3 3:1}", store.marked)
	}
}

func TestRunOnce_GivenSendError_ThenNotMarked(t *testing.T) {
	store := &fakeStore{candidates: []postgres.ExpiryCandidate{cand(1, 1001, "ID-01", "a@vpn.kt", time.Now().Add(5*24*time.Hour))}}
	sender := &fakeSender{err: errBoom}
	s := New(store, sender, []int{7}, 50, time.UTC, testLogger())

	if err := s.RunOnce(context.Background()); err != nil {
		t.Fatalf("RunOnce: %v", err)
	}
	if len(sender.sent) != 1 {
		t.Fatalf("send attempts = %d, want 1", len(sender.sent))
	}
	if _, ok := store.marked[1]; ok {
		t.Error("client must NOT be marked when the send failed (retry next sweep)")
	}
}

func TestRunOnce_GivenListError_ThenReturnsErrorAndContinues(t *testing.T) {
	store := &fakeStore{listErr: errBoom}
	sender := &fakeSender{}
	s := New(store, sender, []int{7, 3}, 50, time.UTC, testLogger())

	err := s.RunOnce(context.Background())
	if err == nil {
		t.Fatal("RunOnce must surface the window failure")
	}
	// Both windows must still be swept (the failure is per-window).
	if len(store.gotUpper) != 2 {
		t.Fatalf("windows swept = %d, want 2", len(store.gotUpper))
	}
}

func TestNew_GivenUnsortedDays_ThenSortedDescending(t *testing.T) {
	s := New(&fakeStore{}, &fakeSender{}, []int{1, 7, 3}, 50, time.UTC, testLogger())
	got := s.Days()
	if len(got) != 3 || got[0] != 7 || got[1] != 3 || got[2] != 1 {
		t.Fatalf("Days = %v, want [7 3 1]", got)
	}
}
