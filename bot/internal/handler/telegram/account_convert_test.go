// Package telegramhandler_test covers the FR-08 AC-2 YAML convert view.
//
// @file      internal/handler/telegram/account_convert_test.go
// @for       account:convert:{id} routing, render, ownership guard.
// @uses      testing, context, strings, github.com/go-telegram/bot/models,
// internal/repository/postgres, internal/service/telegram
// @reason    Locks the AC-2 contract: ownership-guarded like view/config,
// no new service seam (reuses ClientReader — AGENTS.md §2.1).
// @author    Dodi Rusmana <rusmanadodi@kentangtechstore.com>
// @layer     handler
// @stability stable
// @since     2026-08-11
package telegramhandler

import (
	"context"
	"strings"
	"testing"

	"github.com/kentangtech/bot-order/internal/repository/postgres"
	telegramservice "github.com/kentangtech/bot-order/internal/service/telegram"
)

func convertViewClient() postgres.ClientView {
	return postgres.ClientView{
		VPNClient: postgres.VPNClient{
			ID: 1, Email: "kts-abcd1234", ServerID: 2, Protocol: "vless",
			UUID: "uuid-1", InboundNetwork: "ws", InboundPath: "/vlessws",
		},
		ServerName: "ID-01", ServerHost: "id2.kentangtechstore.net",
	}
}

func TestAccountConvert_GivenOwnedClient_ThenYAMLRendered(t *testing.T) {
	shop := newFakeShop()
	shop.users.user = &postgres.User{ID: 9, TelegramID: 7}
	shop.clients.byID = convertViewClient()

	api := &fakeAPI{}
	d := dispatcherWithShop(api, shop)
	d.Handle(context.Background(), cbUpdate(7, telegramservice.PrefixAccountConvert+"1"))

	if len(api.edited) != 1 {
		t.Fatalf("edited = %d, want 1", len(api.edited))
	}
	text := api.edited[0].text
	if !strings.Contains(text, "Convert Config YAML") || !strings.Contains(text, "type: vless") {
		t.Errorf("yaml view missing header/block: %q", text)
	}
	assertButton(t, api.edited[0], telegramservice.PrefixAccountConfig+"1")
	assertButton(t, api.edited[0], telegramservice.PrefixAccountView+"1")
}

func TestAccountConvert_GivenForeignClient_ThenNotFound(t *testing.T) {
	shop := newFakeShop()
	shop.users.user = &postgres.User{ID: 9, TelegramID: 7}
	shop.clients.byErr = postgres.ErrClientNotFound

	api := &fakeAPI{}
	d := dispatcherWithShop(api, shop)
	d.Handle(context.Background(), cbUpdate(7, telegramservice.PrefixAccountConvert+"99"))

	if len(api.answered) != 1 || !strings.Contains(api.answered[0], "Akun tidak ditemukan") {
		t.Fatalf("answered = %+v, want not-found toast", api.answered)
	}
	if len(api.edited) != 0 {
		t.Fatalf("foreign client must not render: %+v", api.edited)
	}
}

func TestAccountConvert_GivenBadID_ThenUnavailable(t *testing.T) {
	api := &fakeAPI{}
	d := dispatcherWithShop(api, newFakeShop())
	d.Handle(context.Background(), cbUpdate(7, telegramservice.PrefixAccountConvert+"abc"))

	if len(api.answered) != 1 || !strings.Contains(api.answered[0], telegramservice.UnavailableText()) {
		t.Fatalf("answered = %+v, want unavailable", api.answered)
	}
}
