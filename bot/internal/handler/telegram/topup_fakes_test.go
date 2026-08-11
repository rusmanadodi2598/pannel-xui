// Package telegramhandler test also hosts the topup fakes.
//
// @file      internal/handler/telegram/topup_fakes_test.go
// @for       In-memory TopupRunner + TopupFSM fakes for the FR-06 flow tests.
// @uses      context, io, log/slog, internal/domain, internal/service/topup
// @reason    Keeps topup_test.go under 250 lines (AGENTS.md §1.1).
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
	topupsvc "github.com/kentangtech/bot-order/internal/service/topup"
)

type fakeTopups struct {
	quote      *topupsvc.Quote
	quoteErr   error
	result     *topupsvc.PaymentResult
	createErr  error
	createdReq *topupsvc.CreatePaymentRequest
	minNet     domain.Money
	maxNet     domain.Money
}

func (f *fakeTopups) Quote(net domain.Money) (topupsvc.Quote, error) {
	if f.quoteErr != nil {
		return topupsvc.Quote{}, f.quoteErr
	}
	if f.quote != nil {
		return *f.quote, nil
	}
	// Default: no fee (gross == net) so assertions on the summary stay simple.
	return topupsvc.Quote{Net: net, Gross: net, TotalFee: 0, FeePercent: 0}, nil
}

func (f *fakeTopups) CreatePayment(ctx context.Context, req topupsvc.CreatePaymentRequest) (*topupsvc.PaymentResult, error) {
	f.createdReq = &req
	return f.result, f.createErr
}

func (f *fakeTopups) MinNet() domain.Money { return f.minNet }
func (f *fakeTopups) MaxNet() domain.Money { return f.maxNet }

type fakeTopupFSM struct {
	pending    bool
	setCalls   int
	clearCalls int
	err        error
}

func (f *fakeTopupFSM) SetPending(context.Context, int64) error {
	f.setCalls++
	return f.err
}
func (f *fakeTopupFSM) Pending(context.Context, int64) (bool, error) { return f.pending, f.err }
func (f *fakeTopupFSM) Clear(context.Context, int64) error {
	f.clearCalls++
	return f.err
}

type fakeTopupDeps struct {
	topups *fakeTopups
	fsm    *fakeTopupFSM
	users  *fakeUsers
}

func newFakeTopup() *fakeTopupDeps {
	return &fakeTopupDeps{
		topups: &fakeTopups{minNet: 5000, maxNet: 5000000},
		fsm:    &fakeTopupFSM{},
		users:  &fakeUsers{balance: 50000},
	}
}

func dispatcherWithTopup(api API, f *fakeTopupDeps) *Dispatcher {
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	topup := &Topup{Users: f.users, Topups: f.topups, FSM: f.fsm}
	return NewDispatcher(api, &fakeGate{}, &fakeBan{}, &fakeLimiter{allow: true}, logger, groupLink, nil, nil, topup, nil)
}

var _ TopupRunner = (*fakeTopups)(nil)
var _ TopupFSM = (*fakeTopupFSM)(nil)
