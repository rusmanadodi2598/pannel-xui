// Package telegramhandler_test covers the FR-07 trial flow.
//
// @file      internal/handler/telegram/trial_test.go
// @for       Unit tests: trial menu → server → confirm (claim) → success; limits.
// @uses      testing, context, strings, github.com/go-telegram/bot/models,
// internal/repository/postgres, internal/service/order
// @reason    Locks the FR-07 callback contract + anti-race claim at confirm.
// @author    Dodi Rusmana <rusmanadodi@kentangtechstore.com>
// @layer     handler
// @stability experimental
// @since     2026-08-11
package telegramhandler

import (
	"context"
	"strings"
	"testing"

	"github.com/kentangtech/bot-order/internal/repository/postgres"
	ordersvc "github.com/kentangtech/bot-order/internal/service/order"
	telegramservice "github.com/kentangtech/bot-order/internal/service/telegram"
)

func TestTrialFlow_GivenRemainingQuota_ThenMenuServerConfirmExecuted(t *testing.T) {
	shop := newFakeShop()
	shop.servers.list = []postgres.ServerView{{ID: 1, Name: "ID-01", CountryCode: "ID", FlagEmoji: "🇮🇩"}}
	var calledServer int64
	shop.trials.called = &calledServer
	shop.trials.result = &ordersvc.PurchaseResult{
		OrderID: "KTS-TRIAL0001-VPN", Status: "completed", AccountEmail: "trial@vpn.kt",
	}

	api := &fakeAPI{}
	d := dispatcherWithShop(api, shop)

	// Step 1: trial:menu → remaining + server picker.
	d.Handle(context.Background(), cbUpdate(7, telegramservice.CallbackTrial))
	if len(api.edited) != 1 || !strings.Contains(api.edited[0].text, "Sisa kesempatan trial") {
		t.Fatalf("step1 edited = %+v", api.edited)
	}
	assertButton(t, api.edited[0], telegramservice.PrefixTrialServer+"1")

	// Step 2: trial:server:1 → confirmation summary.
	d.Handle(context.Background(), cbUpdate(7, telegramservice.PrefixTrialServer+"1"))
	if len(api.edited) != 2 || !strings.Contains(api.edited[1].text, "ID-01") {
		t.Fatalf("step2 edited = %+v", api.edited)
	}
	assertButton(t, api.edited[1], telegramservice.PrefixTrialConfirm+"1")

	// Step 3: trial:confirm:1 → claim + create → success message.
	d.Handle(context.Background(), cbUpdate(7, telegramservice.PrefixTrialConfirm+"1"))
	if len(api.sent) != 1 || !strings.Contains(api.sent[0].text, "Trial Berhasil") {
		t.Fatalf("step3 sent = %+v", api.sent)
	}
	if calledServer != 1 {
		t.Errorf("CreateTrial server = %d, want 1", calledServer)
	}
	if shop.tlim.claimed != 1 {
		t.Errorf("claimed = %d, want 1", shop.tlim.claimed)
	}
}

func TestTrialFlow_GivenLimitReachedAtMenu_ThenLimitText(t *testing.T) {
	shop := newFakeShop()
	shop.tlim.remaining = 0

	api := &fakeAPI{}
	d := dispatcherWithShop(api, shop)
	d.Handle(context.Background(), cbUpdate(7, telegramservice.CallbackTrial))

	if len(api.edited) != 1 || !strings.Contains(api.edited[0].text, "sudah habis") {
		t.Fatalf("edited = %+v", api.edited)
	}
}

func TestTrialFlow_GivenLimitReachedAtConfirm_ThenNoAccountCreated(t *testing.T) {
	shop := newFakeShop()
	shop.servers.list = []postgres.ServerView{{ID: 1, Name: "ID-01"}}
	shop.tlim.remaining = 0 // menu passes, but confirm claims → over limit

	api := &fakeAPI{}
	d := dispatcherWithShop(api, shop)
	d.Handle(context.Background(), cbUpdate(7, telegramservice.PrefixTrialConfirm+"1"))

	if len(api.edited) != 1 || !strings.Contains(api.edited[0].text, "sudah habis") {
		t.Fatalf("edited = %+v", api.edited)
	}
	if shop.trials.called != nil {
		t.Error("CreateTrial must not run when the claim fails")
	}
}

func TestTrialFlow_GivenFeatureDisabled_ThenDisabledText(t *testing.T) {
	shop := newFakeShop()
	shop.tlim.enabled = false

	api := &fakeAPI{}
	d := dispatcherWithShop(api, shop)
	d.Handle(context.Background(), cbUpdate(7, telegramservice.CallbackTrial))

	if len(api.edited) != 1 || !strings.Contains(api.edited[0].text, "tidak tersedia") {
		t.Fatalf("edited = %+v", api.edited)
	}
}

func TestTrialFlow_GivenNoServers_ThenUnavailableMessage(t *testing.T) {
	shop := newFakeShop()
	shop.servers.list = nil

	api := &fakeAPI{}
	d := dispatcherWithShop(api, shop)
	d.Handle(context.Background(), cbUpdate(7, telegramservice.CallbackTrial))

	if len(api.edited) != 1 || !strings.Contains(api.edited[0].text, "Belum ada server") {
		t.Fatalf("edited = %+v", api.edited)
	}
}

func TestTrialFlow_GivenUnknownServer_ThenAnswered(t *testing.T) {
	shop := newFakeShop()
	shop.servers.list = []postgres.ServerView{{ID: 1, Name: "ID-01"}}

	api := &fakeAPI{}
	d := dispatcherWithShop(api, shop)
	d.Handle(context.Background(), cbUpdate(7, telegramservice.PrefixTrialServer+"99"))

	if len(api.edited) != 0 || len(api.answered) != 1 {
		t.Fatalf("edited = %+v answered = %+v", api.edited, api.answered)
	}
}

func TestTrialFlow_GivenInaccessibleMessage_ThenNoPanic(t *testing.T) {
	shop := newFakeShop()
	shop.servers.list = []postgres.ServerView{{ID: 1, Name: "ID-01"}}

	api := &fakeAPI{}
	d := dispatcherWithShop(api, shop)
	upd := cbUpdate(7, telegramservice.CallbackTrial)
	upd.CallbackQuery.Message.Message = nil
	d.Handle(context.Background(), upd)

	if len(api.edited) != 0 || len(api.answered) != 1 {
		t.Fatalf("edited = %+v answered = %+v", api.edited, api.answered)
	}
}

func TestTrialCommand_GivenTextTrial_ThenMenuSent(t *testing.T) {
	shop := newFakeShop()
	shop.servers.list = []postgres.ServerView{{ID: 1, Name: "ID-01"}}

	api := &fakeAPI{}
	d := dispatcherWithShop(api, shop)
	d.Handle(context.Background(), msgUpdate(7, "/trial"))

	if len(api.sent) != 1 || !strings.Contains(api.sent[0].text, "Coba VPN Gratis") {
		t.Fatalf("sent = %+v", api.sent)
	}
}
