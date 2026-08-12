// Package telegramhandler_test also hosts the client fakes.
//
// @file      internal/handler/telegram/client_fakes_test.go
// @for       In-memory fake for ClientReader (list paged + owned views + delete).
// @uses      context, github.com/kentangtech/bot-order/internal/repository/postgres
// @reason    Split from shop_fakes_test.go to respect the 250-line limit
// (AGENTS.md §1.1) after FR-08 AC-4 added DeleteOwned.
// @author    Dodi Rusmana <rusmanadodi@kentangtechstore.com>
// @layer     handler
// @stability stable
// @since     2026-08-11
package telegramhandler

import (
	"context"

	"github.com/kentangtech/bot-order/internal/repository/postgres"
)

// fakeClients serves the FR-08 ClientReader seam: paged list + owned views +
// ownership-guarded delete.
type fakeClients struct {
	list        []postgres.ClientView
	byID        postgres.ClientView
	byErr       error
	err         error
	count       int64
	lastOffset  int
	lastDeleted int64
	delErr      error
}

func (f *fakeClients) ListByUser(context.Context, int64, int) ([]postgres.ClientView, error) {
	return f.list, f.err
}
func (f *fakeClients) CountByUser(context.Context, int64) (int64, error) {
	return f.count, f.err
}
func (f *fakeClients) ListByUserPage(_ context.Context, _ int64, limit, offset int) ([]postgres.ClientView, error) {
	if f.err != nil {
		return nil, f.err
	}
	f.lastOffset = offset
	if offset >= len(f.list) {
		return nil, nil
	}
	end := offset + limit
	if end > len(f.list) {
		end = len(f.list)
	}
	return f.list[offset:end], nil
}
func (f *fakeClients) GetViewOwned(_ context.Context, _, _ int64) (postgres.ClientView, error) {
	return f.byID, f.byErr
}
func (f *fakeClients) DeleteOwned(_ context.Context, clientID, userID int64) error {
	if f.byErr != nil {
		return f.byErr
	}
	f.lastDeleted = clientID
	return f.delErr
}

// Compile-time interface check.
var _ ClientReader = (*fakeClients)(nil)
