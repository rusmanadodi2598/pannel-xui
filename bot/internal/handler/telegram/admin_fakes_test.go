// Package telegramhandler test also hosts the admin fakes.
//
// @file      internal/handler/telegram/admin_fakes_test.go
// @for       In-memory AdminOps + AdminFSM fakes and the admin dispatcher helper.
// @uses      context, io, log/slog, internal/domain, internal/repository/postgres
// @reason    Keeps admin_test.go / admin_user_test.go under 250 lines (§1.1).
// @author    Dodi Rusmana <rusmanadodi@kentangtechstore.com>
// @layer     handler
// @stability experimental
// @since     2026-08-11
package telegramhandler

import (
	"context"
	"io"
	"log/slog"

	"github.com/kentangtech/bot-order/internal/domain"
	"github.com/kentangtech/bot-order/internal/repository/postgres"
)

// fakeAdminOps implements AdminOps with call recording.
type fakeAdminOps struct {
	plans      []domain.VpnPlan
	plan       *domain.VpnPlan
	err        error
	priceSet   *priceCall
	enabledSet *toggleCall
	banned     []int64
	unbanned   []int64
	user       *postgres.User
	broadcast  string
	bcastTotal int
	bcastErr   error
}

type priceCall struct {
	country string
	days    int
	price   domain.Money
}

type toggleCall struct {
	country string
	days    int
	enabled bool
}

func (f *fakeAdminOps) ListPlans(context.Context) ([]domain.VpnPlan, error) { return f.plans, f.err }
func (f *fakeAdminOps) GetPlan(context.Context, string, int) (*domain.VpnPlan, error) {
	return f.plan, f.err
}
func (f *fakeAdminOps) SetPrice(_ context.Context, country string, days int, price domain.Money) error {
	f.priceSet = &priceCall{country, days, price}
	return f.err
}
func (f *fakeAdminOps) SetEnabled(_ context.Context, country string, days int, enabled bool) error {
	f.enabledSet = &toggleCall{country, days, enabled}
	return f.err
}
func (f *fakeAdminOps) ReloadPricing(context.Context) error { return f.err }
func (f *fakeAdminOps) LookupUser(context.Context, int64) (*postgres.User, error) {
	return f.user, f.err
}
func (f *fakeAdminOps) BanUser(_ context.Context, tgID int64) error {
	f.banned = append(f.banned, tgID)
	return f.err
}
func (f *fakeAdminOps) UnbanUser(_ context.Context, tgID int64) error {
	f.unbanned = append(f.unbanned, tgID)
	return f.err
}
func (f *fakeAdminOps) Broadcast(_ context.Context, _ int64, text string) (int, error) {
	f.broadcast = text
	return f.bcastTotal, f.bcastErr
}

// fakeAdminFSM implements AdminFSM in memory.
type fakeAdminFSM struct {
	state string
	armed bool
	err   error
}

func (f *fakeAdminFSM) Set(_ context.Context, _ int64, state string) error {
	f.state, f.armed = state, true
	return f.err
}
func (f *fakeAdminFSM) Get(context.Context, int64) (string, bool, error) {
	if !f.armed {
		return "", false, f.err
	}
	return f.state, true, f.err
}
func (f *fakeAdminFSM) Clear(context.Context, int64) error {
	f.state, f.armed = "", false
	return f.err
}

type fakeAdminDeps struct {
	ops *fakeAdminOps
	fsm *fakeAdminFSM
}

func dispatcherWithAdmin(api API, f *fakeAdminDeps) *Dispatcher {
	// Default nil seams so every test gets non-nil fakes (a nil *fakeAdminFSM
	// stored in the AdminFSM interface is non-nil and would panic on use).
	if f.ops == nil {
		f.ops = &fakeAdminOps{}
	}
	if f.fsm == nil {
		f.fsm = &fakeAdminFSM{}
	}
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	admin := &Admin{Ops: f.ops, FSM: f.fsm}
	return NewDispatcher(api, &fakeGate{}, &fakeBan{}, &fakeLimiter{allow: true}, logger, groupLink, []int64{7}, nil, nil, admin)
}
