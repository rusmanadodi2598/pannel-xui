// Package telegramhandler_test covers the FR-08 AC-4 delete flow.
//
// @file      internal/handler/telegram/account_delete_test.go
// @for       Step 1 confirm page, step 2 panel-first then DB delete, failures.
// @uses      testing, context, strings, time, github.com/go-telegram/bot/models,
// internal/repository/postgres, internal/service/telegram
// @reason    Deletion is destructive — the test locks the 2-step contract,
// the panel-first ordering (DB row never deleted when the panel fails) and
// the ownership guard (FR-08 AC-4, AGENTS.md §2.1).
// @author    Dodi Rusmana <rusmanadodi@kentangtechstore.com>
// @layer     handler
// @stability stable
// @since     2026-08-11
package telegramhandler

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/kentangtech/bot-order/internal/repository/postgres"
	telegramservice "github.com/kentangtech/bot-order/internal/service/telegram"
)

// deleteClientFixture builds one owned client view for the delete tests.
func deleteClientFixture() postgres.ClientView {
	expiry := time.Now().Add(10 * 24 * time.Hour)
	return postgres.ClientView{
		VPNClient: postgres.VPNClient{
			ID: 3, ServerID: 1, InboundID: 5, Email: "del@vpn.kt",
			UUID: "uuid-3", Protocol: "vless", ExpiresAt: &expiry,
		},
		ServerName: "ID-01", CountryCode: "ID", FlagEmoji: "🇮🇩",
	}
}

func TestAccountDelete_GivenStepOne_ThenConfirmationPageShown(t *testing.T) {
	shop := newFakeShop()
	shop.users.user = &postgres.User{ID: 9, TelegramID: 7}
	shop.clients.byID = deleteClientFixture()

	api := &fakeAPI{}
	d := dispatcherWithShop(api, shop)
	d.Handle(context.Background(), cbUpdate(7, telegramservice.PrefixAccountDelete+"3"))

	if len(api.edited) != 1 {
		t.Fatalf("edited = %d, want 1", len(api.edited))
	}
	if !strings.Contains(api.edited[0].text, "tidak bisa dikembalikan") ||
		!strings.Contains(api.edited[0].text, "del@vpn.kt") {
		t.Errorf("confirm text = %q", api.edited[0].text)
	}
	assertButton(t, api.edited[0], telegramservice.PrefixAccountDeleteConfirm+"3")
	// Nothing deleted yet.
	if shop.deleter.called || shop.clients.lastDeleted != 0 {
		t.Errorf("delete must not run on step 1: panel=%v db=%d", shop.deleter.called, shop.clients.lastDeleted)
	}
}

func TestAccountDeleteConfirm_GivenOwnedClient_ThenPanelFirstThenDB(t *testing.T) {
	shop := newFakeShop()
	shop.users.user = &postgres.User{ID: 9, TelegramID: 7}
	shop.clients.byID = deleteClientFixture()

	api := &fakeAPI{}
	d := dispatcherWithShop(api, shop)
	d.Handle(context.Background(), cbUpdate(7, telegramservice.PrefixAccountDeleteConfirm+"3"))

	if !shop.deleter.called || shop.deleter.serverID != 1 || shop.deleter.inboundID != 5 || shop.deleter.clientID != "uuid-3" {
		t.Errorf("panel delete = %+v, want server 1 inbound 5 client uuid-3", shop.deleter)
	}
	if shop.clients.lastDeleted != 3 {
		t.Errorf("db delete = %d, want 3", shop.clients.lastDeleted)
	}
	// FR-08 AC-4: the deletion is recorded in the user's Riwayat.
	if shop.orders.deleted == nil || shop.orders.deleted.Email != "del@vpn.kt" ||
		shop.orders.deleted.ServerID != 1 || shop.orders.deleted.Protocol != "vless" {
		t.Errorf("deletion record = %+v, want del@vpn.kt server 1 vless", shop.orders.deleted)
	}
	if len(api.edited) != 1 || !strings.Contains(api.edited[0].text, "Akun dihapus") {
		t.Errorf("success edit = %+v", api.edited)
	}
}

func TestAccountDeleteConfirm_GivenPanelFailure_ThenDBRowKept(t *testing.T) {
	shop := newFakeShop()
	shop.users.user = &postgres.User{ID: 9, TelegramID: 7}
	shop.clients.byID = deleteClientFixture()
	shop.deleter.err = errBoom

	api := &fakeAPI{}
	d := dispatcherWithShop(api, shop)
	d.Handle(context.Background(), cbUpdate(7, telegramservice.PrefixAccountDeleteConfirm+"3"))

	if !shop.deleter.called {
		t.Fatal("panel delete must be attempted")
	}
	if shop.clients.lastDeleted != 0 {
		t.Errorf("db delete = %d, want 0 (row must stay when the panel fails)", shop.clients.lastDeleted)
	}
	if shop.orders.deleted != nil {
		t.Errorf("deletion must not be recorded when the panel fails: %+v", shop.orders.deleted)
	}
	if len(api.edited) != 1 || !strings.Contains(api.edited[0].text, "Gagal menghapus") {
		t.Errorf("failure edit = %+v", api.edited)
	}
}

func TestAccountDeleteConfirm_GivenDBFailureAfterPanel_ThenWarnsAdmin(t *testing.T) {
	shop := newFakeShop()
	shop.users.user = &postgres.User{ID: 9, TelegramID: 7}
	shop.clients.byID = deleteClientFixture()
	shop.clients.delErr = errBoom // panel OK, DB row delete fails

	api := &fakeAPI{}
	d := dispatcherWithShop(api, shop)
	d.Handle(context.Background(), cbUpdate(7, telegramservice.PrefixAccountDeleteConfirm+"3"))

	if !shop.deleter.called {
		t.Fatal("panel delete must run first")
	}
	if len(api.edited) != 1 || !strings.Contains(api.edited[0].text, "Hubungi admin") {
		t.Errorf("db-failure edit = %+v, want warn-admin message", api.edited)
	}
	// The failure page keeps navigation alive (not a dead end).
	assertButtonInMarkup(t, api.edited[0].markup, telegramservice.CallbackHome)
	// The history record is written only after the DB delete succeeds.
	if shop.orders.deleted != nil {
		t.Errorf("deletion must not be recorded when the DB delete fails: %+v", shop.orders.deleted)
	}
}

func TestAccountDeleteConfirm_GivenForeignClient_ThenDenied(t *testing.T) {
	shop := newFakeShop()
	shop.users.user = &postgres.User{ID: 9, TelegramID: 7}
	shop.clients.byErr = postgres.ErrClientNotFound

	api := &fakeAPI{}
	d := dispatcherWithShop(api, shop)
	d.Handle(context.Background(), cbUpdate(7, telegramservice.PrefixAccountDeleteConfirm+"3"))

	if shop.deleter.called || shop.clients.lastDeleted != 0 {
		t.Errorf("delete must be denied for foreign client: panel=%v db=%d", shop.deleter.called, shop.clients.lastDeleted)
	}
	if len(api.answered) != 1 || !strings.Contains(api.answered[0], "tidak ditemukan") {
		t.Errorf("answer = %+v, want not-found", api.answered)
	}
}

func TestAccountDelete_GivenBadID_ThenUnavailable(t *testing.T) {
	api := &fakeAPI{}
	d := dispatcherWithShop(api, newFakeShop())
	d.Handle(context.Background(), cbUpdate(7, telegramservice.PrefixAccountDelete+"abc"))

	if len(api.answered) != 1 || !strings.Contains(api.answered[0], telegramservice.UnavailableText()) {
		t.Fatalf("answered = %+v, want unavailable", api.answered)
	}
}
