// Package ordersvc_test also hosts the shared fakes.
//
// @file      internal/service/order/fakes_test.go
// @for       In-memory fakes for OrderStore/ClientStore/UserStore/PlanReader/ServerPicker/PanelGateway.
// @uses      context, time, internal/domain, internal/repository/postgres
// @reason    Keeps order_test.go under 250 lines (AGENTS.md §1.1).
// @author    Dodi Rusmana <rusmanadodi@kentangtechstore.com>
// @layer     service
// @stability stable
// @since     2026-08-11
package ordersvc

import (
	"context"
	"time"

	"github.com/kentangtech/bot-order/internal/domain"
	"github.com/kentangtech/bot-order/internal/repository/postgres"
)

type fakeStores struct {
	orders           *fakeOrderStore
	clients          *fakeClientStore
	users            *fakeUserStore
	plans            *fakePlanReader
	servers          *fakeServerPicker
	panels           *fakePanelGateway
	debited          int
	debitAfterPanel  bool // purchase: debit runs after the panel call (FR-04 AC-1)
	debitBeforePanel bool // renewal v1.37: debit runs before the panel call
	saved            []*postgres.Order
}

func newFakeStores() *fakeStores {
	f := &fakeStores{}
	f.orders = &fakeOrderStore{}
	f.clients = &fakeClientStore{}
	f.users = &fakeUserStore{balanceAfter: 43000}
	f.plans = &fakePlanReader{}
	f.servers = &fakeServerPicker{}
	f.panels = &fakePanelGateway{}
	// Record debit ordering per flow: purchase debits AFTER the panel call
	// (FR-04 AC-1); renewal debits BEFORE it (v1.37 debit-first).
	f.users.onDebit = func() {
		f.debited++
		f.debitAfterPanel = f.panels.called
		f.debitBeforePanel = !f.panels.called
	}
	f.orders.onSave = func(o *postgres.Order) { f.saved = append(f.saved, o) }
	return f
}

type fakeOrderStore struct {
	created  []*postgres.Order
	onSave   func(o *postgres.Order)
	inFlight *postgres.Order // returned by FindInFlight (nil = none in flight)
}

func (f *fakeOrderStore) Create(_ context.Context, o *postgres.Order) error {
	o.ID = int64(len(f.created) + 1)
	f.created = append(f.created, o)
	return nil
}
func (f *fakeOrderStore) Save(_ context.Context, o *postgres.Order) error {
	if f.onSave != nil {
		f.onSave(o)
	}
	return nil
}
func (f *fakeOrderStore) FindInFlight(_ context.Context, _ int64, _ string) (*postgres.Order, error) {
	return f.inFlight, nil
}

type fakeClientStore struct {
	owned         *postgres.VPNClient
	ownedErr      error
	created       []*postgres.VPNClient
	expiryUpdated *time.Time
	expiryErr     error
}

func (f *fakeClientStore) Create(_ context.Context, c *postgres.VPNClient) error {
	c.ID = 7
	f.created = append(f.created, c)
	return nil
}
func (f *fakeClientStore) GetOwned(_ context.Context, _, _ int64) (*postgres.VPNClient, error) {
	if f.ownedErr != nil {
		return nil, f.ownedErr
	}
	return f.owned, nil
}
func (f *fakeClientStore) UpdateExpiry(_ context.Context, _ int64, e time.Time, _ *int64) error {
	if f.expiryErr != nil {
		return f.expiryErr
	}
	f.expiryUpdated = &e
	return nil
}

type fakeUserStore struct {
	balanceAfter domain.Money
	debitErr     error
	creditErr    error
	onDebit      func() // called when Debit runs (ordering assertion)
	credited     []string
}

func (f *fakeUserStore) Debit(_ context.Context, _ int64, _ domain.Money, _ string) (domain.Money, error) {
	if f.debitErr != nil {
		return 0, f.debitErr
	}
	// onDebit fires only on success: it counts money actually moved.
	if f.onDebit != nil {
		f.onDebit()
	}
	return f.balanceAfter, nil
}
func (f *fakeUserStore) Credit(_ context.Context, _ int64, _ domain.Money, orderID string) (domain.Money, error) {
	if f.creditErr != nil {
		return 0, f.creditErr
	}
	f.credited = append(f.credited, orderID)
	return f.balanceAfter, nil
}

type fakePlanReader struct {
	plan *domain.VpnPlan
	err  error
}

func (f *fakePlanReader) GetPlan(_ context.Context, _ string, _ int) (*domain.VpnPlan, error) {
	if f.err != nil {
		return nil, f.err
	}
	return f.plan, nil
}

type fakeServerPicker struct {
	serverID int64
	err      error
}

func (f *fakeServerPicker) PickForCountry(_ context.Context, _ string) (int64, error) {
	if f.err != nil {
		return 0, f.err
	}
	return f.serverID, nil
}

type fakePanelGateway struct {
	called         bool
	trialCalled    bool
	trialInboundID int
	created        domain.PanelClient
	createErr      error
	renewCalled    bool
	renewErr       error
	renewClientID  string // panel client key passed to RenewClient (v1.38)
}

func (f *fakePanelGateway) CreateClient(_ context.Context, _ int64, _ int, _ string, _ string, _ int, _ int64, _ int64) (domain.PanelClient, error) {
	f.called = true
	if f.createErr != nil {
		return domain.PanelClient{}, f.createErr
	}
	return f.created, nil
}
func (f *fakePanelGateway) CreateTrialClient(_ context.Context, _ int64, inboundID int, _ string, _ string, _ int, _ int64, _ int64) (domain.PanelClient, error) {
	f.trialCalled = true
	f.trialInboundID = inboundID
	if f.createErr != nil {
		return domain.PanelClient{}, f.createErr
	}
	return f.created, nil
}
func (f *fakePanelGateway) RenewClient(_ context.Context, _ int64, clientID, _, _ string, _ time.Time) error {
	f.renewCalled = true
	f.renewClientID = clientID
	if f.renewErr != nil {
		return f.renewErr
	}
	return nil
}

// Compile-time check: fakes satisfy the service interfaces.
var (
	_ OrderStore   = (*fakeOrderStore)(nil)
	_ ClientStore  = (*fakeClientStore)(nil)
	_ UserStore    = (*fakeUserStore)(nil)
	_ PlanReader   = (*fakePlanReader)(nil)
	_ ServerPicker = (*fakeServerPicker)(nil)
	_ PanelGateway = (*fakePanelGateway)(nil)
)
