// Package telegramhandler_test also hosts the traffic refresh fake.
//
// @file      internal/handler/telegram/traffic_fakes_test.go
// @for       In-memory fake for the TrafficRefresher seam (FR-08 AC-3).
// @uses      context
// @reason    Keeps shop_fakes_test.go under 250 lines (AGENTS.md §1.1).
// @author    Dodi Rusmana <rusmanadodi@kentangtechstore.com>
// @layer     handler
// @stability stable
// @since     2026-08-11
package telegramhandler

import "context"

// fakeTraffic records refresh calls; onRefresh lets a test mutate the fake
// client store so the handler's post-sync re-read is observable.
type fakeTraffic struct {
	clientID  int64
	serverID  int64
	email     string
	calls     int
	err       error
	onRefresh func()
}

func (f *fakeTraffic) RefreshClient(_ context.Context, clientID, serverID int64, email string) error {
	f.calls++
	f.clientID = clientID
	f.serverID = serverID
	f.email = email
	if f.onRefresh != nil {
		f.onRefresh()
	}
	return f.err
}

// Compile-time interface check.
var _ TrafficRefresher = (*fakeTraffic)(nil)
