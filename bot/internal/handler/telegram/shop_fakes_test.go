// Package telegramhandler_test also hosts the shop fakes.
//
// @file      internal/handler/telegram/shop_fakes_test.go
// @for       In-memory fakes for PlanReader/ServerReader/UserReader/OrderRunner/ClientReader.
// @uses      context, log/slog, io, testing, github.com/go-telegram/bot/models,
// internal/domain, internal/repository/postgres, internal/service/order
// @reason    Keeps shop_test.go under 250 lines (AGENTS.md §1.1).
// @author    Dodi Rusmana <rusmanadodi@kentangtechstore.com>
// @layer     handler
// @stability stable
// @since     2026-08-11
package telegramhandler

import (
	"context"
	"io"
	"log/slog"
	"testing"

	"github.com/go-telegram/bot/models"
	"github.com/kentangtech/bot-order/internal/domain"
	"github.com/kentangtech/bot-order/internal/repository/postgres"
	ordersvc "github.com/kentangtech/bot-order/internal/service/order"
	serversvc "github.com/kentangtech/bot-order/internal/service/server"
	trialsvc "github.com/kentangtech/bot-order/internal/service/trial"
)

type fakeShopDeps struct {
	plans   *fakePlans
	servers *fakeServers
	users   *fakeUsers
	orders  *fakeOrders
	clients *fakeClients
	trials  *fakeTrialRunner
	tlim    *fakeTrialLimiter
	history *fakeHistory
	deleter *fakeDeleter
	traffic *fakeTraffic
}

func newFakeShop() *fakeShopDeps {
	return &fakeShopDeps{
		plans:   &fakePlans{},
		servers: &fakeServers{},
		users:   &fakeUsers{user: &postgres.User{ID: 9, TelegramID: 7, Balance: 50000}},
		orders:  &fakeOrders{},
		clients: &fakeClients{},
		trials:  &fakeTrialRunner{},
		tlim:    &fakeTrialLimiter{enabled: true, remaining: 2, limit: 2},
		history: &fakeHistory{},
		deleter: &fakeDeleter{},
		traffic: &fakeTraffic{},
	}
}

func dispatcherWithShop(api API, f *fakeShopDeps) *Dispatcher {
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	shop := &Shop{
		Plans: f.plans, Servers: f.servers, Users: f.users, Orders: f.orders,
		Clients: f.clients, Trials: f.trials, TrialLm: f.tlim, History: f.history,
		Deleter: f.deleter, Traffic: f.traffic,
	}
	return NewDispatcher(api, &fakeGate{}, &fakeBan{}, &fakeLimiter{allow: true}, logger, groupLink, nil, shop, nil, nil)
}

type fakeTrialRunner struct {
	called       *int64
	lastInbound  int
	lastProtocol string
	result       *ordersvc.PurchaseResult
	err          error
}

func (f *fakeTrialRunner) CreateTrial(_ context.Context, _ *postgres.User, serverID int64, spec ordersvc.TrialSpec) (*ordersvc.PurchaseResult, error) {
	if f.called != nil {
		*f.called = serverID
	}
	f.lastInbound = spec.InboundID
	f.lastProtocol = spec.Protocol
	return f.result, f.err
}

type fakeTrialLimiter struct {
	enabled   bool
	remaining int
	limit     int
	claimed   int
	err       error
}

func (f *fakeTrialLimiter) Enabled() bool  { return f.enabled }
func (f *fakeTrialLimiter) Limit() int     { return f.limit }
func (f *fakeTrialLimiter) Hours() int     { return 1 }
func (f *fakeTrialLimiter) TrafficGB() int { return 1 }
func (f *fakeTrialLimiter) IPLimit() int   { return 1 }
func (f *fakeTrialLimiter) Remaining(context.Context, int64) (int, error) {
	return f.remaining, f.err
}
func (f *fakeTrialLimiter) Claim(context.Context, int64) (int, error) {
	if f.err != nil {
		return 0, f.err
	}
	if f.remaining <= 0 {
		return 0, trialsvc.ErrDailyLimitReached
	}
	f.remaining--
	f.claimed++
	return f.claimed, nil
}

// compile-time interface checks
var (
	_ TrialRunner  = (*fakeTrialRunner)(nil)
	_ TrialLimiter = (*fakeTrialLimiter)(nil)
)

type fakePlans struct {
	list []domain.VpnPlan
	get  *domain.VpnPlan
	err  error
}

func (f *fakePlans) ListEnabled(context.Context) ([]domain.VpnPlan, error) { return f.list, f.err }
func (f *fakePlans) GetPlan(_ context.Context, _ string, _ int) (*domain.VpnPlan, error) {
	return f.get, f.err
}

type fakeServers struct {
	list      []postgres.ServerView
	inbounds  []serversvc.InboundOption
	inboundsE error
	err       error
}

func (f *fakeServers) ListBuyable(context.Context) ([]postgres.ServerView, error) {
	return f.list, f.err
}
func (f *fakeServers) ListInbounds(context.Context, int64) ([]serversvc.InboundOption, error) {
	return f.inbounds, f.inboundsE
}

type fakeUsers struct {
	user    *postgres.User
	balance domain.Money
	err     error
}

func (f *fakeUsers) EnsureUser(_ context.Context, _ int64, _, _ string) (*postgres.User, error) {
	return f.user, f.err
}
func (f *fakeUsers) Balance(context.Context, int64) (domain.Money, error) { return f.balance, f.err }

type purchaseCall struct {
	Country  string
	Days     int
	ServerID int
	Inbound  int
	Protocol string
}
type renewCall struct {
	ClientID int64
	Country  string
	Days     int
}
type deletionCall struct {
	UserID   int64
	ServerID int64
	Protocol string
	Email    string
}

type fakeOrders struct {
	purchased *purchaseCall
	renewed   *renewCall
	deleted   *deletionCall
	res       *ordersvc.PurchaseResult
	err       error
}

func (f *fakeOrders) Purchase(_ context.Context, _ *postgres.User, country string, days, serverID, inboundID int, protocol string) (*ordersvc.PurchaseResult, error) {
	f.purchased = &purchaseCall{Country: country, Days: days, ServerID: serverID, Inbound: inboundID, Protocol: protocol}
	return f.res, f.err
}
func (f *fakeOrders) Renew(_ context.Context, _ *postgres.User, clientID int64, country string, days int) (*ordersvc.PurchaseResult, error) {
	f.renewed = &renewCall{ClientID: clientID, Country: country, Days: days}
	return f.res, f.err
}
func (f *fakeOrders) RecordDeletion(_ context.Context, userID, serverID int64, protocol, email string) error {
	f.deleted = &deletionCall{UserID: userID, ServerID: serverID, Protocol: protocol, Email: email}
	return f.err
}

// assertButton checks that a rendered edit contains a button with the callback.
func assertButton(t *testing.T, e editCall, wantData string) {
	t.Helper()
	assertButtonInMarkup(t, e.markup, wantData)
}

// assertButtonInMarkup checks any reply markup for a button with the callback.
func assertButtonInMarkup(t *testing.T, markup models.ReplyMarkup, wantData string) {
	t.Helper()
	kb, ok := markup.(models.InlineKeyboardMarkup)
	if !ok {
		t.Fatalf("no inline keyboard in %+v", markup)
	}
	for _, row := range kb.InlineKeyboard {
		for _, btn := range row {
			if btn.CallbackData == wantData {
				return
			}
		}
	}
	t.Fatalf("button %q not found in keyboard", wantData)
}

// Compile-time interface checks.
var (
	_ PlanReader   = (*fakePlans)(nil)
	_ ServerReader = (*fakeServers)(nil)
	_ UserReader   = (*fakeUsers)(nil)
	_ OrderRunner  = (*fakeOrders)(nil)
)
