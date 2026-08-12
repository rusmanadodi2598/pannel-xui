// Package telegramhandler_test covers the v1.37 renewal rules (AGENTS.md §2.1).
//
// @file      internal/handler/telegram/renew_test.go
// @for       Paid-only renew picker + friendly messages for trial/in-flight rejection.
// @uses      testing, context, strings, time, github.com/go-telegram/bot/models,
// internal/repository/postgres, internal/service/order
// @reason    Trial accounts must never appear in the renew menu or accept a
// renew callback, and idempotence rejections must read friendly (v1.37).
// @author    Dodi Rusmana <rusmanadodi@kentangtechstore.com>
// @layer     handler
// @stability stable
// @since     2026-08-12
package telegramhandler

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/go-telegram/bot/models"
	"github.com/kentangtech/bot-order/internal/repository/postgres"
	ordersvc "github.com/kentangtech/bot-order/internal/service/order"
	telegramservice "github.com/kentangtech/bot-order/internal/service/telegram"
)

func renewViews() (trial, paid postgres.ClientView) {
	expiry := time.Now().Add(30 * 24 * time.Hour)
	trial = postgres.ClientView{
		VPNClient:  postgres.VPNClient{ID: 3, Email: "trial@vpn.kt", Protocol: "vless", IsTrial: true, ExpiresAt: &expiry},
		ServerName: "ID-01", CountryCode: "ID", FlagEmoji: "🇮🇩",
	}
	paid = postgres.ClientView{
		VPNClient:  postgres.VPNClient{ID: 4, Email: "paid@vpn.kt", Protocol: "vless", ExpiresAt: &expiry},
		ServerName: "ID-01", CountryCode: "ID", FlagEmoji: "🇮🇩",
	}
	return trial, paid
}

func TestRenewMenu_GivenTrialAndPaid_ThenOnlyPaidListed(t *testing.T) {
	trial, paid := renewViews()
	shop := newFakeShop()
	shop.clients.list = []postgres.ClientView{trial, paid}

	api := &fakeAPI{}
	d := dispatcherWithShop(api, shop)
	d.Handle(context.Background(), cbUpdate(7, telegramservice.CallbackRenew))

	if len(api.edited) != 1 || !strings.Contains(api.edited[0].text, "Pilih akun") {
		t.Fatalf("renew menu edited = %+v", api.edited)
	}
	// The picker shows the paid account as a button and must NOT offer the
	// trial account at all (v1.37 paid-only).
	assertButton(t, api.edited[0], telegramservice.PrefixRenewClient+"4")
	assertNoButton(t, api.edited[0], telegramservice.PrefixRenewClient+"3")
}

func TestRenewPlans_GivenTrialClient_ThenNotFound(t *testing.T) {
	trial, _ := renewViews()
	shop := newFakeShop()
	shop.clients.list = []postgres.ClientView{trial}

	api := &fakeAPI{}
	d := dispatcherWithShop(api, shop)
	d.Handle(context.Background(), cbUpdate(7, telegramservice.PrefixRenewClient+"3"))

	if len(api.answered) != 1 || !strings.Contains(api.answered[0], "Akun tidak ditemukan.") {
		t.Fatalf("answered = %+v, want 'Akun tidak ditemukan.'", api.answered)
	}
	if len(api.edited) != 0 {
		t.Errorf("trial renew must not render a plan picker: %+v", api.edited)
	}
}

func TestRenewExecute_GivenTrialClient_ThenTrialMessage(t *testing.T) {
	shop := newFakeShop()
	shop.orders.err = ordersvc.ErrTrialNotRenewable

	api := &fakeAPI{}
	d := dispatcherWithShop(api, shop)
	d.Handle(context.Background(), cbUpdate(7, telegramservice.PrefixRenewConfirm+"3:ID:30"))

	if len(api.sent) != 1 || !strings.Contains(api.sent[0].text, "trial tidak bisa diperpanjang") {
		t.Fatalf("sent = %+v, want trial-not-renewable message", api.sent)
	}
}

func TestRenewExecute_GivenInFlightOrder_ThenInFlightMessage(t *testing.T) {
	shop := newFakeShop()
	shop.orders.err = ordersvc.ErrOrderInFlight

	api := &fakeAPI{}
	d := dispatcherWithShop(api, shop)
	d.Handle(context.Background(), cbUpdate(7, telegramservice.PrefixRenewConfirm+"3:ID:30"))

	if len(api.sent) != 1 || !strings.Contains(api.sent[0].text, "masih diproses") {
		t.Fatalf("sent = %+v, want in-flight message", api.sent)
	}
}

// assertNoButton asserts a callback button is NOT present in the rendered markup.
func assertNoButton(t *testing.T, e editCall, wantData string) {
	t.Helper()
	kb, ok := e.markup.(models.InlineKeyboardMarkup)
	if !ok {
		t.Fatalf("no inline keyboard in %+v", e.markup)
	}
	for _, row := range kb.InlineKeyboard {
		for _, btn := range row {
			if btn.CallbackData == wantData {
				t.Fatalf("button %q must NOT be present", wantData)
			}
		}
	}
}
