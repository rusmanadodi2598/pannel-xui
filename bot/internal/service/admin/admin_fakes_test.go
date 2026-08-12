// Package adminsvc test also hosts the shared fakes.
//
// @file      internal/service/admin/admin_fakes_test.go
// @for       In-memory PlanStore/UserStore/BanMarker/Messenger/BroadcastLocker fakes.
// @uses      context, io, log/slog, sync, time, github.com/go-telegram/bot/models,
// internal/domain, internal/repository/postgres
// @reason    Keeps admin_test.go / broadcast_test.go under 250 lines (§1.1).
// @author    Dodi Rusmana <rusmanadodi@kentangtechstore.com>
// @layer     service
// @stability experimental
// @since     2026-08-11
package adminsvc

import (
	"context"
	"io"
	"log/slog"
	"sync"
	"time"

	"github.com/go-telegram/bot/models"
	"github.com/kentangtech/bot-order/internal/domain"
	"github.com/kentangtech/bot-order/internal/repository/postgres"
)

func testLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(io.Discard, nil))
}

type fakePlans struct {
	plans     []domain.VpnPlan
	plan      *domain.VpnPlan
	err       error
	setPrice  bool
	setEnable *bool
}

func (f *fakePlans) ListAll(context.Context) ([]domain.VpnPlan, error) { return f.plans, f.err }
func (f *fakePlans) Get(context.Context, string, int) (*domain.VpnPlan, error) {
	return f.plan, f.err
}
func (f *fakePlans) SetPrice(context.Context, string, int, domain.Money) error {
	f.setPrice = true
	return f.err
}
func (f *fakePlans) SetEnabled(context.Context, string, int, bool) error {
	f.setEnable = new(bool)
	*f.setEnable = true
	return f.err
}
func (f *fakePlans) Reload(context.Context) error { return f.err }

type fakeUsers struct {
	mu        sync.Mutex
	total     int64
	ids       []int64
	banned    []int64
	unbanned  []int64
	user      *postgres.User
	err       error
	credited  []adjustMove
	debited   []adjustMove
	creditErr error
	debitErr  error
}

type adjustMove struct {
	userID  int64
	amount  domain.Money
	orderID string
}

func (f *fakeUsers) SetBanned(_ context.Context, tgID int64, banned bool) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	if banned {
		f.banned = append(f.banned, tgID)
	} else {
		f.unbanned = append(f.unbanned, tgID)
	}
	return f.err
}
func (f *fakeUsers) Get(context.Context, int64) (*postgres.User, error) { return f.user, f.err }
func (f *fakeUsers) Credit(_ context.Context, userID int64, amount domain.Money, orderID string) (domain.Money, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.creditErr != nil {
		return 0, f.creditErr
	}
	f.credited = append(f.credited, adjustMove{userID, amount, orderID})
	return f.user.Balance + amount, nil
}
func (f *fakeUsers) Debit(_ context.Context, userID int64, amount domain.Money, orderID string) (domain.Money, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.debitErr != nil {
		return 0, f.debitErr
	}
	f.debited = append(f.debited, adjustMove{userID, amount, orderID})
	return f.user.Balance - amount, nil
}
func (f *fakeUsers) ListTelegramIDs(_ context.Context, limit, offset int) ([]int64, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if offset >= len(f.ids) {
		return nil, nil
	}
	end := offset + limit
	if end > len(f.ids) {
		end = len(f.ids)
	}
	return f.ids[offset:end], nil
}
func (f *fakeUsers) CountUsers(context.Context) (int64, error) { return f.total, f.err }

type fakeBanner struct {
	banned   []int64
	unbanned []int64
	err      error
}

func (f *fakeBanner) Ban(_ context.Context, id int64) error {
	f.banned = append(f.banned, id)
	return f.err
}
func (f *fakeBanner) Unban(_ context.Context, id int64) error {
	f.unbanned = append(f.unbanned, id)
	return f.err
}

type fakeSender struct {
	mu    sync.Mutex
	sent  []int64
	done  chan struct{} // closed when the admin completion report arrives
	admin int64
	err   error
}

func (f *fakeSender) SendMessage(_ context.Context, chatID int64, _ string, _ models.ParseMode, _ models.ReplyMarkup) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.sent = append(f.sent, chatID)
	if chatID == f.admin && f.done != nil {
		close(f.done)
		f.done = nil
	}
	return f.err
}

func (f *fakeSender) count() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return len(f.sent)
}

type fakeLocker struct {
	mu       sync.Mutex
	busy     bool
	acquired int
	released int
}

func (f *fakeLocker) AcquireLock(context.Context, string, time.Duration) (bool, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.busy {
		return false, nil
	}
	f.acquired++
	return true, nil
}
func (f *fakeLocker) ReleaseLock(context.Context, string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.released++
	return nil
}
func (f *fakeLocker) releaseCount() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.released
}

// newTestService builds a service with small chunk and no delay for tests.
func newTestService(users *fakeUsers, sender *fakeSender, locker *fakeLocker) *Service {
	s := New(&fakePlans{}, users, &fakeBanner{}, sender, locker, &fakeServerOps{}, &fakeStats{}, &fakeAudit{}, testLogger())
	s.chunk = 2
	s.delay = 0
	return s
}
