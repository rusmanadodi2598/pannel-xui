// Package telegramhandler_test covers the M4 shop flows (buy/renew/accounts).
//
// @file      internal/handler/telegram/shop_test.go
// @for       Unit tests: buy menu→country→plan→confirm→execute, renew, accounts.
// @uses      testing, context, log/slog, io, strings, github.com/go-telegram/bot/models,
// internal/domain, internal/repository/postgres, internal/service/order
// @reason    Locks the FR-03/FR-05/FR-08 callback contract end-to-end (AGENTS.md §2.1).
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

	"github.com/kentangtech/bot-order/internal/domain"
	"github.com/kentangtech/bot-order/internal/repository/postgres"
	ordersvc "github.com/kentangtech/bot-order/internal/service/order"
	serversvc "github.com/kentangtech/bot-order/internal/service/server"
	telegramservice "github.com/kentangtech/bot-order/internal/service/telegram"
)

func TestBuyFlow_GivenCountryToConfirm_ThenOrderExecuted(t *testing.T) {
	shop := newFakeShop()
	shop.plans.list = []domain.VpnPlan{
		{CountryCode: "ID", CountryName: "Indonesia", Days: 15, Price: 4000},
		{CountryCode: "ID", CountryName: "Indonesia", Days: 30, Price: 7000},
	}
	shop.plans.get = &domain.VpnPlan{CountryCode: "ID", CountryName: "Indonesia", Days: 30, Price: 7000}
	shop.servers.list = []postgres.ServerView{{ID: 1, CountryCode: "ID", Name: "ID-01"}}
	shop.servers.inbounds = []serversvc.InboundOption{
		{ServerID: 1, ServerName: "ID-01", Country: "ID", InboundID: 4, Protocol: "vless", Remark: "reality"},
		{ServerID: 1, ServerName: "ID-01", Country: "ID", InboundID: 5, Protocol: "trojan", Remark: "ws"},
	}
	shop.orders.res = &ordersvc.PurchaseResult{
		OrderID: "KTS-TEST0001-VPN", Status: domain.OrderCompleted,
		AccountEmail: "ktsx@vpn.kt", BalanceAfter: 43000, Plan: shop.plans.get,
		ClientID: 3,
	}

	api := &fakeAPI{}
	d := dispatcherWithShop(api, shop)

	// Step 1: menu:buy → country picker.
	d.Handle(context.Background(), cbUpdate(7, telegramservice.CallbackBuy))
	if len(api.edited) != 1 || !strings.Contains(api.edited[0].text, "Pilih negara") {
		t.Fatalf("step1 edited = %+v", api.edited)
	}
	assertButton(t, api.edited[0], "buy:country:ID")

	// Step 2: pick country → inbound (server + protocol) picker from the panel.
	d.Handle(context.Background(), cbUpdate(7, telegramservice.PrefixBuyCountry+"ID"))
	if len(api.edited) != 2 || !strings.Contains(api.edited[1].text, "protocol") {
		t.Fatalf("step2 edited = %+v", api.edited)
	}
	assertButton(t, api.edited[1], "buy:inbound:1:4:ID")
	assertButton(t, api.edited[1], "buy:inbound:1:5:ID")

	// Step 3: pick inbound → plan list (server+inbound pinned in callbacks).
	d.Handle(context.Background(), cbUpdate(7, telegramservice.PrefixBuyInbound+"1:4:ID"))
	if len(api.edited) != 3 || !strings.Contains(api.edited[2].text, "Indonesia") {
		t.Fatalf("step3 edited = %+v", api.edited)
	}
	assertButton(t, api.edited[2], "buy:plan:1:4:ID:15")

	// Step 4: pick plan → confirm summary (live price + balance + protocol).
	d.Handle(context.Background(), cbUpdate(7, telegramservice.PrefixBuyPlan+"1:4:ID:30"))
	if len(api.edited) != 4 || !strings.Contains(api.edited[3].text, "Rp 7.000") ||
		!strings.Contains(api.edited[3].text, "VLESS") {
		t.Fatalf("step4 edited = %+v", api.edited)
	}
	assertButton(t, api.edited[3], "buy:confirm:1:4:ID:30")

	// Step 5: confirm → order executed with the pinned server+inbound+protocol.
	d.Handle(context.Background(), cbUpdate(7, telegramservice.PrefixBuyConfirm+"1:4:ID:30"))
	if len(api.sent) != 1 {
		t.Fatalf("step5 sent = %+v", api.sent)
	}
	if !strings.Contains(api.sent[0].text, "Order Berhasil") || !strings.Contains(api.sent[0].text, "KTS-TEST0001-VPN") {
		t.Errorf("success text = %q", api.sent[0].text)
	}
	// v1.36: the import URL no longer renders in the success message — it
	// lives only in the .txt export (export button still offered).
	if strings.Contains(api.sent[0].text, "vless://") {
		t.Errorf("success text must not render the import URL (v1.36): %q", api.sent[0].text)
	}
	if !strings.Contains(api.sent[0].text, "Ekspor .txt") {
		t.Errorf("success text missing export hint: %q", api.sent[0].text)
	}
	assertButtonInMarkup(t, api.sent[0].markup, telegramservice.PrefixAccountExport+"3")
	if shop.orders.purchased == nil || shop.orders.purchased.Country != "ID" ||
		shop.orders.purchased.Days != 30 || shop.orders.purchased.ServerID != 1 ||
		shop.orders.purchased.Inbound != 4 || shop.orders.purchased.Protocol != "vless" {
		t.Errorf("purchase call = %+v", shop.orders.purchased)
	}
}

func TestBuyFlow_GivenInsufficientBalance_ThenTopupHint(t *testing.T) {
	shop := newFakeShop()
	shop.plans.get = &domain.VpnPlan{CountryCode: "ID", CountryName: "Indonesia", Days: 30, Price: 7000}
	shop.servers.list = []postgres.ServerView{{ID: 1, CountryCode: "ID"}}
	shop.servers.inbounds = []serversvc.InboundOption{
		{ServerID: 1, ServerName: "ID-01", Country: "ID", InboundID: 4, Protocol: "vless"},
	}
	shop.orders.err = ordersvc.ErrInsufficientBalance

	api := &fakeAPI{}
	d := dispatcherWithShop(api, shop)
	d.Handle(context.Background(), cbUpdate(7, telegramservice.PrefixBuyConfirm+"1:4:ID:30"))

	if len(api.sent) != 1 || !strings.Contains(api.sent[0].text, "tidak cukup") {
		t.Fatalf("sent = %+v", api.sent)
	}
	assertButtonInMarkup(t, api.sent[0].markup, telegramservice.CallbackTopup)
}

func TestRenewFlow_GivenClientToConfirm_ThenRenewalExecuted(t *testing.T) {
	expiry := time.Now().Add(10 * 24 * time.Hour)
	shop := newFakeShop()
	shop.users.user = &postgres.User{ID: 9, TelegramID: 7, Balance: 50000}
	shop.clients.list = []postgres.ClientView{{
		VPNClient:  postgres.VPNClient{ID: 3, Email: "a@vpn.kt", Protocol: "vless", ExpiresAt: &expiry},
		ServerName: "ID-01", CountryCode: "ID", FlagEmoji: "🇮🇩",
	}}
	shop.plans.list = []domain.VpnPlan{{CountryCode: "ID", CountryName: "Indonesia", Days: 30, Price: 7000}}
	shop.plans.get = &domain.VpnPlan{CountryCode: "ID", CountryName: "Indonesia", Days: 30, Price: 7000}
	shop.orders.res = &ordersvc.PurchaseResult{
		OrderID: "KTS-TEST0002-VPN", Status: domain.OrderCompleted,
		AccountEmail: "a@vpn.kt", BalanceAfter: 43000, Plan: shop.plans.get,
	}

	api := &fakeAPI{}
	d := dispatcherWithShop(api, shop)

	d.Handle(context.Background(), cbUpdate(7, telegramservice.CallbackRenew))
	if len(api.edited) != 1 || !strings.Contains(api.edited[0].text, "Pilih akun") {
		t.Fatalf("renew step1 = %+v", api.edited)
	}
	assertButton(t, api.edited[0], "renew:client:3")

	d.Handle(context.Background(), cbUpdate(7, telegramservice.PrefixRenewPlan+"3:ID:30"))
	if len(api.edited) != 2 || !strings.Contains(api.edited[1].text, "a@vpn.kt") {
		t.Fatalf("renew step2 = %+v", api.edited)
	}
	assertButton(t, api.edited[1], "renew:confirm:3:ID:30")

	d.Handle(context.Background(), cbUpdate(7, telegramservice.PrefixRenewConfirm+"3:ID:30"))
	if len(api.sent) != 1 || !strings.Contains(api.sent[0].text, "Perpanjangan Berhasil") {
		t.Fatalf("renew sent = %+v", api.sent)
	}
	if shop.orders.renewed == nil || shop.orders.renewed.ClientID != 3 || shop.orders.renewed.Days != 30 {
		t.Errorf("renew call = %+v", shop.orders.renewed)
	}
}

func TestAccountMenu_GivenClients_ThenListRendered(t *testing.T) {
	shop := newFakeShop()
	shop.users.user = &postgres.User{ID: 9, TelegramID: 7}
	shop.clients.list = []postgres.ClientView{{
		VPNClient:  postgres.VPNClient{ID: 3, Email: "a@vpn.kt", Protocol: "vless"},
		ServerName: "ID-01", CountryCode: "ID", FlagEmoji: "🇮🇩",
	}}
	shop.clients.count = 1

	api := &fakeAPI{}
	d := dispatcherWithShop(api, shop)
	d.Handle(context.Background(), cbUpdate(7, telegramservice.CallbackAccount))

	if len(api.edited) != 1 || !strings.Contains(api.edited[0].text, "a@vpn.kt") {
		t.Fatalf("account edited = %+v", api.edited)
	}
}

func TestBuyFlow_GivenInaccessibleMessage_ThenNoPanicAndAnswer(t *testing.T) {
	// Telegram can send a callback whose message is inaccessible (deleted);
	// handlers must not dereference a nil message (regression of staging crash).
	shop := newFakeShop()
	shop.plans.list = []domain.VpnPlan{{CountryCode: "ID", CountryName: "Indonesia", Days: 15, Price: 4000}}
	shop.servers.list = []postgres.ServerView{{ID: 1, CountryCode: "ID"}}

	api := &fakeAPI{}
	d := dispatcherWithShop(api, shop)
	upd := cbUpdate(7, telegramservice.CallbackBuy)
	upd.CallbackQuery.Message.Message = nil // simulate inaccessible message
	d.Handle(context.Background(), upd)

	if len(api.edited) != 0 {
		t.Fatalf("must not edit a nil message: %+v", api.edited)
	}
	if len(api.answered) != 1 {
		t.Fatalf("callback must still be answered: %+v", api.answered)
	}
}

func TestBuyFlow_GivenBuyBack_ThenHomeMenu(t *testing.T) {
	api := &fakeAPI{}
	d := dispatcherWithShop(api, newFakeShop())
	d.Handle(context.Background(), cbUpdate(7, telegramservice.CallbackBuyBack))
	if len(api.edited) != 1 || !strings.Contains(api.edited[0].text, "Selamat datang") {
		t.Fatalf("back = %+v", api.edited)
	}
}
