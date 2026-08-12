// Package telegramhandler_test also hosts the FR-14 history fakes.
//
// @file      internal/handler/telegram/history_fakes_test.go
// @for       In-memory fake for HistoryReader (paged rows + owned detail).
// @uses      context, github.com/kentangtech/bot-order/internal/repository/postgres
// @reason    Keeps shop_fakes_test.go under 250 lines (AGENTS.md §1.1) while
// giving history_test.go a deterministic read seam.
// @author    Dodi Rusmana <rusmanadodi@kentangtechstore.com>
// @layer     handler
// @stability stable
// @since     2026-08-11
package telegramhandler

import (
	"context"

	"github.com/kentangtech/bot-order/internal/repository/postgres"
)

// fakeHistory serves the FR-14 history seam: paged rows + owned detail.
type fakeHistory struct {
	count      int64
	orders     []postgres.Order
	byID       *postgres.Order
	lastLimit  int
	lastOffset int
	err        error
}

func (f *fakeHistory) CountByUser(context.Context, int64) (int64, error) {
	return f.count, f.err
}
func (f *fakeHistory) ListByUserPage(_ context.Context, _ int64, limit, offset int) ([]postgres.Order, error) {
	if f.err != nil {
		return nil, f.err
	}
	f.lastLimit, f.lastOffset = limit, offset
	if offset >= len(f.orders) {
		return nil, nil
	}
	end := offset + limit
	if end > len(f.orders) {
		end = len(f.orders)
	}
	return f.orders[offset:end], nil
}
func (f *fakeHistory) GetOwned(_ context.Context, orderID, _ int64) (*postgres.Order, error) {
	if f.err != nil {
		return nil, f.err
	}
	if f.byID != nil {
		return f.byID, nil
	}
	for i := range f.orders {
		if f.orders[i].ID == orderID {
			return &f.orders[i], nil
		}
	}
	return nil, postgres.ErrOrderNotFound
}

// Compile-time interface check.
var _ HistoryReader = (*fakeHistory)(nil)
