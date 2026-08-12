// Package telegramhandler_test covers the account v2Ray config view (v1.26).
//
// @file      internal/handler/telegram/shop_config_test.go
// @for       account:config:{id} routing → dual TLS/NTLS config view + ownership.
// @uses      testing, context, strings, internal/repository/postgres,
// internal/service/telegram
// @reason    Locks the v1.26 config-view callback contract end-to-end
// (AGENTS.md §2.1) without growing shop_test.go past 250 lines (§1.1).
// @author    Dodi Rusmana <rusmanadodi@kentangtechstore.com>
// @layer     handler
// @stability stable
// @since     2026-08-11
package telegramhandler

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/kentangtech/bot-order/internal/repository/postgres"
	telegramservice "github.com/kentangtech/bot-order/internal/service/telegram"
)

var errNotFound = errors.New("client not found")

func TestAccountConfig_GivenOwnedWSClient_ThenParametersRenderedWithoutURLs(t *testing.T) {
	shop := newFakeShop()
	shop.users.user = &postgres.User{ID: 9, TelegramID: 7}
	shop.clients.byID = postgres.ClientView{
		VPNClient:  postgres.VPNClient{ID: 3, Email: "kts-abcd1234@vpn.kt", UUID: "uuid-1", Protocol: "vless"},
		ServerName: "ID-01", ServerHost: "id2.kentangtechstore.net", FlagEmoji: "🇮🇩",
	}

	api := &fakeAPI{}
	d := dispatcherWithShop(api, shop)
	d.Handle(context.Background(), cbUpdate(7, telegramservice.PrefixAccountConfig+"3"))

	if len(api.edited) != 1 {
		t.Fatalf("edited = %+v", api.edited)
	}
	text := api.edited[0].text
	for _, want := range []string{"Detail Konfigurasi VPN", "Domain   : id2.kentangtechstore.net", "Port TLS : 443", "Port Non-TLS: 80", "Ekspor .txt"} {
		if !strings.Contains(text, want) {
			t.Errorf("config view missing %q in:\n%s", want, text)
		}
	}
	// v1.36: view Config V2Ray TIDAK menampilkan URL — hanya di ekspor .txt.
	if strings.Contains(text, "URL Config") || strings.Contains(text, "vless://") {
		t.Errorf("config view must not render URLs (v1.36):\n%s", text)
	}
	// Keyboard: back to detail view.
	assertButton(t, api.edited[0], telegramservice.PrefixAccountView+"3")
}

func TestAccountConfig_GivenNonWSProtocol_ThenNoURLAndExportHint(t *testing.T) {
	shop := newFakeShop()
	shop.users.user = &postgres.User{ID: 9, TelegramID: 7}
	shop.clients.byID = postgres.ClientView{
		VPNClient:  postgres.VPNClient{ID: 4, Email: "kts-x@vpn.kt", Protocol: "hysteria", ConfigLink: "hysteria2://auth@h:20005"},
		ServerName: "ID-01", ServerHost: "id2.kentangtechstore.net",
	}

	api := &fakeAPI{}
	d := dispatcherWithShop(api, shop)
	d.Handle(context.Background(), cbUpdate(7, telegramservice.PrefixAccountConfig+"4"))

	if len(api.edited) != 1 {
		t.Fatalf("config view must render for non-ws protocol: %+v", api.edited)
	}
	// v1.36: native ConfigLink tidak dirender di view — URL hanya di ekspor .txt.
	if strings.Contains(api.edited[0].text, "hysteria2://") {
		t.Errorf("non-ws config view must not render native URL (v1.36): %+v", api.edited)
	}
	if !strings.Contains(api.edited[0].text, "Ekspor .txt") {
		t.Errorf("non-ws config view missing export hint: %+v", api.edited)
	}
}

func TestAccountConfig_GivenNotOwned_ThenNotFoundAnswer(t *testing.T) {
	shop := newFakeShop()
	shop.users.user = &postgres.User{ID: 9, TelegramID: 7}
	shop.clients.byErr = errNotFound

	api := &fakeAPI{}
	d := dispatcherWithShop(api, shop)
	d.Handle(context.Background(), cbUpdate(7, telegramservice.PrefixAccountConfig+"99"))

	if len(api.edited) != 0 {
		t.Fatalf("must not render for unowned client: %+v", api.edited)
	}
	if len(api.answered) != 1 {
		t.Fatalf("must answer the callback: %+v", api.answered)
	}
}

func TestAccountConfig_GivenMalformedID_ThenUnavailable(t *testing.T) {
	api := &fakeAPI{}
	d := dispatcherWithShop(api, newFakeShop())
	d.Handle(context.Background(), cbUpdate(7, telegramservice.PrefixAccountConfig+"abc"))

	if len(api.edited) != 0 || len(api.answered) != 1 {
		t.Fatalf("malformed id must only answer: edited=%+v answered=%+v", api.edited, api.answered)
	}
}
