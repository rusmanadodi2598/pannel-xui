// Package topupsvc tests also host the gateway/store/ledger fakes (AGENTS.md §2.1).
//
// @file      internal/service/topup/fakes_test.go
// @for       In-memory PaymentGateway + PaymentStore + UserLedger fakes.
// @uses      context, time, internal/domain, internal/repository/kts,
// internal/repository/postgres, gorm.io/gorm
// @reason    Keeps the topup tests hermetic (no live gateway/DB) while covering
// the charge lifecycle and idempotent settlement (AGENTS.md §2.1).
// @author    Dodi Rusmana <rusmanadodi@kentangtechstore.com>
// @layer     service
// @stability experimental
// @since     2026-08-18
package topupsvc

import (
	"context"
	"time"

	"github.com/kentangtech/bot-order/internal/domain"
	"github.com/kentangtech/bot-order/internal/repository/kts"
	"github.com/kentangtech/bot-order/internal/repository/postgres"
	"gorm.io/gorm"
)

type fakeGateway struct {
	createResp  *kts.Charge
	createErr   error
	confirmResp *kts.Charge
	confirmErr  error
	createdReq  *kts.CreateChargeRequest
	confirmedID string
}

func (f *fakeGateway) CreateCharge(ctx context.Context, req kts.CreateChargeRequest) (*kts.Charge, error) {
	f.createdReq = &req
	return f.createResp, f.createErr
}

func (f *fakeGateway) ConfirmCharge(ctx context.Context, orderID string) (*kts.Charge, error) {
	f.confirmedID = orderID
	return f.confirmResp, f.confirmErr
}

type fakePaymentStore struct {
	rows       map[string]*postgres.Payment
	createErr  error
	getErr     error
	markErr    error
	settledIDs []string
}

func newFakePaymentStore() *fakePaymentStore {
	return &fakePaymentStore{rows: map[string]*postgres.Payment{}}
}

func (f *fakePaymentStore) Create(ctx context.Context, p *postgres.Payment) error {
	if f.createErr != nil {
		return f.createErr
	}
	f.rows[p.OrderID] = p
	return nil
}

func (f *fakePaymentStore) GetByOrderID(ctx context.Context, orderID string) (*postgres.Payment, error) {
	if f.getErr != nil {
		return nil, f.getErr
	}
	p, ok := f.rows[orderID]
	if !ok {
		return nil, postgres.ErrPaymentNotFound
	}
	return p, nil
}

func (f *fakePaymentStore) SaveProviderRef(ctx context.Context, orderID, providerRef string) error {
	if p, ok := f.rows[orderID]; ok {
		p.ProviderRef = providerRef
	}
	return nil
}

func (f *fakePaymentStore) MarkFailed(ctx context.Context, orderID, reason string) error {
	if p, ok := f.rows[orderID]; ok {
		p.Status = postgres.PaymentStatusFailed
		p.ProviderStatus = reason
	}
	return nil
}

func (f *fakePaymentStore) MarkSettledTx(ctx context.Context, tx *gorm.DB, orderID, status string, paidAt *time.Time) (bool, error) {
	if f.markErr != nil {
		return false, f.markErr
	}
	p, ok := f.rows[orderID]
	if !ok || p.Status != postgres.PaymentStatusPending {
		return false, nil
	}
	p.Status = status
	p.PaidAt = paidAt
	f.settledIDs = append(f.settledIDs, orderID)
	return true, nil
}

type fakeUserLedger struct {
	user      *postgres.User
	balance   domain.Money
	creditErr error
	credits   []string
}

func (f *fakeUserLedger) FindOrCreate(ctx context.Context, tgID int64, username, firstName string) (*postgres.User, error) {
	return f.user, nil
}

func (f *fakeUserLedger) WithTx(ctx context.Context, fn func(tx *gorm.DB) error) error {
	return fn(nil)
}

func (f *fakeUserLedger) CreditTx(ctx context.Context, tx *gorm.DB, userID int64, amount domain.Money, orderID string) (domain.Money, error) {
	if f.creditErr != nil {
		return 0, f.creditErr
	}
	f.balance += amount
	f.credits = append(f.credits, orderID)
	return f.balance, nil
}

// testDeps bundles the fakes a test wires into the service.
type testDeps struct {
	gw      *fakeGateway
	store   *fakePaymentStore
	ledger  *fakeUserLedger
	notices []TopupNotice
}

func newTestDeps() *testDeps {
	d := &testDeps{
		gw:     &fakeGateway{},
		store:  newFakePaymentStore(),
		ledger: &fakeUserLedger{user: &postgres.User{ID: 1, TelegramID: 7}},
	}
	return d
}

// build returns a Service wired to the fakes (production fee defaults).
func (d *testDeps) build() *Service {
	return New(d.gw, d.store, d.ledger, 0.025, 0.11, 5000, 5000000, d)
}

func (d *testDeps) NotifyTopupSettled(ctx context.Context, n TopupNotice) error {
	d.notices = append(d.notices, n)
	return nil
}
