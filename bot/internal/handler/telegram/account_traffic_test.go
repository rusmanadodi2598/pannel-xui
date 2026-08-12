// Package telegramhandler_test covers the FR-08 AC-3 traffic page.
//
// @file      internal/handler/telegram/account_traffic_test.go
// @for       Traffic page: sync-on-view, post-sync re-read, stale fallback.
// @uses      testing, context, errors, strings, github.com/go-telegram/bot/models,
// internal/repository/postgres, internal/service/telegram
// @reason    Locks the manual-refresh contract: ownership guard, best-effort
// sync, and fresh re-read after sync (AGENTS.md §2.1).
// @author    Dodi Rusmana <rusmanadodi@kentangtechstore.com>
// @layer     handler
// @stability stable
// @since     2026-08-11
package telegramhandler

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"strings"
	"testing"

	"github.com/kentangtech/bot-order/internal/repository/postgres"
	telegramservice "github.com/kentangtech/bot-order/internal/service/telegram"
)

func trafficView(email string, used, limit int64) postgres.ClientView {
	return postgres.ClientView{
		VPNClient: postgres.VPNClient{
			ID: 1, Email: email, ServerID: 2, InboundID: 9, Protocol: "vless",
			TrafficUsed: used, TrafficLimit: limit,
			TrafficUp: used / 2, TrafficDown: used - used/2,
		},
		ServerName: "ID-01",
	}
}

func TestAccountTraffic_GivenOwnedClient_ThenSyncedAndRendered(t *testing.T) {
	shop := newFakeShop()
	shop.users.user = &postgres.User{ID: 9, TelegramID: 7}
	shop.clients.byID = trafficView("a@vpn.kt", 95, 100)

	api := &fakeAPI{}
	d := dispatcherWithShop(api, shop)
	d.Handle(context.Background(), cbUpdate(7, telegramservice.PrefixAccountTraffic+"1"))

	// The manual refresh hit the seam with the client's panel coordinates.
	if shop.traffic.calls != 1 || shop.traffic.clientID != 1 ||
		shop.traffic.serverID != 2 || shop.traffic.email != "a@vpn.kt" {
		t.Fatalf("refresh call = client:%d server:%d email:%s calls:%d, want client 1 server 2 a@vpn.kt",
			shop.traffic.clientID, shop.traffic.serverID, shop.traffic.email, shop.traffic.calls)
	}
	if len(api.edited) != 1 {
		t.Fatalf("edited = %d, want 1", len(api.edited))
	}
	text := api.edited[0].text
	if !strings.Contains(text, "Detail Traffic") || !strings.Contains(text, "a@vpn.kt") {
		t.Errorf("traffic text missing header/email: %q", text)
	}
	if !strings.Contains(text, "🔴") || !strings.Contains(text, "Hampir Habis") {
		t.Errorf("95/100 usage must render red status: %q", text)
	}
	// Keyboard: refresh re-triggers the same callback + back to detail.
	assertButton(t, api.edited[0], telegramservice.PrefixAccountTraffic+"1")
	assertButton(t, api.edited[0], telegramservice.PrefixAccountView+"1")
}

func TestAccountTraffic_GivenSyncSucceeds_ThenPageShowsFreshNumbers(t *testing.T) {
	shop := newFakeShop()
	shop.users.user = &postgres.User{ID: 9, TelegramID: 7}
	shop.clients.byID = trafficView("a@vpn.kt", 10, 100) // stale: green
	shop.traffic.onRefresh = func() {
		shop.clients.byID = trafficView("a@vpn.kt", 95, 100) // fresh: red
	}

	api := &fakeAPI{}
	d := dispatcherWithShop(api, shop)
	d.Handle(context.Background(), cbUpdate(7, telegramservice.PrefixAccountTraffic+"1"))

	// The handler re-reads the row AFTER the sync, so the page must show the
	// fresh numbers (red), not the pre-refresh ones.
	if !strings.Contains(api.edited[0].text, "🔴") {
		t.Errorf("page must re-read after sync, got: %q", api.edited[0].text)
	}
}

func TestAccountTraffic_GivenSyncFailure_ThenStalePageStillRendered(t *testing.T) {
	shop := newFakeShop()
	shop.users.user = &postgres.User{ID: 9, TelegramID: 7}
	shop.clients.byID = trafficView("a@vpn.kt", 10, 100)
	shop.traffic.err = errors.New("panel down")

	api := &fakeAPI{}
	d := dispatcherWithShop(api, shop)
	d.Handle(context.Background(), cbUpdate(7, telegramservice.PrefixAccountTraffic+"1"))

	if len(api.edited) != 1 {
		t.Fatalf("edited = %d, want 1 (best effort, stale data)", len(api.edited))
	}
	if !strings.Contains(api.edited[0].text, "🟢") {
		t.Errorf("failed sync must still render last known values: %q", api.edited[0].text)
	}
	// The user must not be left wondering why the numbers did not move.
	// editCB answers empty afterwards, so the toast is the FIRST answer.
	if len(api.answered) < 1 || !strings.Contains(api.answered[0], "Gagal sync, menampilkan data terakhir.") {
		t.Errorf("answered = %+v, want failure toast first", api.answered)
	}
}

func TestAccountTraffic_GivenUnwiredSeam_ThenPageRenderedWithoutRefresh(t *testing.T) {
	f := newFakeShop()
	f.users.user = &postgres.User{ID: 9, TelegramID: 7}
	f.clients.byID = trafficView("a@vpn.kt", 10, 100)

	// Build the dispatcher with a Shop literal that omits Traffic — the seam
	// is a true nil interface (not a typed-nil), the defensive handler path.
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	unwired := &Shop{
		Plans: f.plans, Servers: f.servers, Users: f.users, Orders: f.orders,
		Clients: f.clients, Trials: f.trials, TrialLm: f.tlim, History: f.history,
		Deleter: f.deleter,
	}
	api := &fakeAPI{}
	d := NewDispatcher(api, &fakeGate{}, &fakeBan{}, &fakeLimiter{allow: true}, logger, groupLink, nil, unwired, nil, nil)
	d.Handle(context.Background(), cbUpdate(7, telegramservice.PrefixAccountTraffic+"1"))

	if len(api.edited) != 1 {
		t.Fatalf("edited = %d, want 1 (page renders from the DB row)", len(api.edited))
	}
	if !strings.Contains(api.edited[0].text, "🟢") || !strings.Contains(api.edited[0].text, "a@vpn.kt") {
		t.Errorf("unwired seam must still render the page: %q", api.edited[0].text)
	}
}

func TestAccountTraffic_GivenForeignClient_ThenNotFoundAndNoRefresh(t *testing.T) {
	shop := newFakeShop()
	shop.users.user = &postgres.User{ID: 9, TelegramID: 7}
	shop.clients.byErr = postgres.ErrClientNotFound

	api := &fakeAPI{}
	d := dispatcherWithShop(api, shop)
	d.Handle(context.Background(), cbUpdate(7, telegramservice.PrefixAccountTraffic+"99"))

	if len(api.answered) != 1 || !strings.Contains(api.answered[0], "Akun tidak ditemukan") {
		t.Fatalf("answered = %+v, want not-found toast", api.answered)
	}
	if shop.traffic.calls != 0 {
		t.Errorf("refresh called %d times for foreign client, want 0", shop.traffic.calls)
	}
}

func TestAccountTraffic_GivenBadID_ThenUnavailable(t *testing.T) {
	api := &fakeAPI{}
	d := dispatcherWithShop(api, newFakeShop())
	d.Handle(context.Background(), cbUpdate(7, telegramservice.PrefixAccountTraffic+"abc"))

	if len(api.answered) != 1 || !strings.Contains(api.answered[0], telegramservice.UnavailableText()) {
		t.Fatalf("answered = %+v, want unavailable", api.answered)
	}
}
