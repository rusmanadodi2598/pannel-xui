// Package telegramhandler_test also hosts the FR-08 AC-4 delete fakes.
//
// @file      internal/handler/telegram/delete_fakes_test.go
// @for       In-memory fake for ClientDeleter (panel delClient seam).
// @uses      context, internal/repository/postgres
// @reason    Keeps shop_fakes_test.go under 250 lines (AGENTS.md §1.1) while
// giving the delete-flow test a deterministic panel-delete seam.
// @author    Dodi Rusmana <rusmanadodi@kentangtechstore.com>
// @layer     handler
// @stability stable
// @since     2026-08-11
package telegramhandler

import (
	"context"
)

// fakeDeleter records the panel delClient call (FR-08 AC-4 seam).
type fakeDeleter struct {
	called    bool
	serverID  int64
	inboundID int
	clientID  string
	err       error
}

func (f *fakeDeleter) DeleteClient(_ context.Context, serverID int64, inboundID int, clientID string) error {
	f.called = true
	f.serverID = serverID
	f.inboundID = inboundID
	f.clientID = clientID
	return f.err
}

// Compile-time interface check.
var _ ClientDeleter = (*fakeDeleter)(nil)
