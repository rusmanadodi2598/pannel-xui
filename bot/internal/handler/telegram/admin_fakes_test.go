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
	"time"

	"github.com/kentangtech/bot-order/internal/domain"
	"github.com/kentangtech/bot-order/internal/repository/postgres"
	"github.com/kentangtech/bot-order/internal/service/server"
) // fakeAdminOps implements AdminOps with call recording.
type fakeAdminOps struct {
	plans       []domain.VpnPlan
	plan        *domain.VpnPlan
	err         error
	priceSet    *priceCall
	enabledSet  *toggleCall
	banned      []int64
	unbanned    []int64
	user        *postgres.User
	broadcast   string
	bcastTotal  int
	bcastErr    error
	adjust      *adjustCall
	adjustCalls int
	adjustErr   error
	lookupErr   error
	servers     []postgres.ServerAdminView
	serverErr   error
	added       *serverAddCall
	addCalls    int
	stats       *postgres.OrderStats
	recent      []postgres.Order
	audit       []postgres.AdminAuditLog
}

type serverAddCall struct {
	adminID int64
	name    string
	host    string
}

type adjustCall struct {
	tgID   int64
	amount domain.Money
	credit bool
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
func (f *fakeAdminOps) SetPrice(_ context.Context, _ int64, country string, days int, price domain.Money) error {
	f.priceSet = &priceCall{country, days, price}
	return f.err
}
func (f *fakeAdminOps) SetEnabled(_ context.Context, _ int64, country string, days int, enabled bool) error {
	f.enabledSet = &toggleCall{country, days, enabled}
	return f.err
}
func (f *fakeAdminOps) ReloadPricing(context.Context, int64) error { return f.err }
func (f *fakeAdminOps) LookupUser(context.Context, int64) (*postgres.User, error) {
	if f.lookupErr != nil {
		return nil, f.lookupErr
	}
	return f.user, f.err
}
func (f *fakeAdminOps) BanUser(_ context.Context, _ int64, tgID int64) error {
	f.banned = append(f.banned, tgID)
	return f.err
}
func (f *fakeAdminOps) UnbanUser(_ context.Context, _ int64, tgID int64) error {
	f.unbanned = append(f.unbanned, tgID)
	return f.err
}
func (f *fakeAdminOps) Broadcast(_ context.Context, _ int64, text string) (int, error) {
	f.broadcast = text
	return f.bcastTotal, f.bcastErr
}
func (f *fakeAdminOps) AdjustBalance(_ context.Context, _ int64, tgID int64, amount domain.Money, credit bool) (domain.Money, error) {
	f.adjust = &adjustCall{tgID, amount, credit}
	f.adjustCalls++
	if f.adjustErr != nil {
		return 0, f.adjustErr
	}
	return 123000, nil
}
func (f *fakeAdminOps) ListServers(context.Context) ([]postgres.ServerAdminView, error) {
	return f.servers, f.serverErr
}
func (f *fakeAdminOps) ToggleServerOpen(_ context.Context, _ int64, _ int64, _ bool) error {
	return f.serverErr
}
func (f *fakeAdminOps) ToggleServerActive(_ context.Context, _ int64, _ int64, _ bool) error {
	return f.serverErr
}
func (f *fakeAdminOps) AddServer(_ context.Context, adminID int64, in serversvc.NewServerInput) (int64, error) {
	f.added = &serverAddCall{adminID: adminID, name: in.Name, host: in.Host}
	f.addCalls++
	return 77, f.serverErr
}
func (f *fakeAdminOps) Stats(context.Context, *time.Location) (postgres.OrderStats, error) {
	if f.stats != nil {
		return *f.stats, f.serverErr
	}
	return postgres.OrderStats{}, f.serverErr
}
func (f *fakeAdminOps) RecentOrders(context.Context, int) ([]postgres.Order, error) {
	return f.recent, f.serverErr
}
func (f *fakeAdminOps) AuditLog(context.Context, int) ([]postgres.AdminAuditLog, error) {
	return f.audit, f.serverErr
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
