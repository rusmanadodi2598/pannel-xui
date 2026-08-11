// Package telegramhandler test covers the middleware chain and gate bypass.
//
// @file      internal/handler/telegram/dispatcher_gate_test.go
// @for       Ban deny, gate deny/unknown, ADMIN_IDS bypass, rate limit, gate:check callback.
// @uses      testing, context, strings, github.com/go-telegram/bot/models, internal/service/telegram
// @reason    Locks the middleware semantics separately from routing (250-line limit §1.1).
// @author    Dodi Rusmana <rusmanadodi@kentangtechstore.com>
// @layer     handler
// @stability stable
// @since     2026-08-11
package telegramhandler

import (
	"context"
	"strings"
	"testing"

	"github.com/go-telegram/bot/models"
	telegramservice "github.com/kentangtech/bot-order/internal/service/telegram"
)

func TestHandle_GivenBannedUser_ThenDeniedWithoutRouting(t *testing.T) {
	api := &fakeAPI{}
	d := newDispatcher(api, &fakeGate{enabled: true, result: telegramservice.GateAllowed}, &fakeBan{banned: true}, &fakeLimiter{allow: true})

	d.Handle(context.Background(), msgUpdate(7, "/start"))
	if len(api.sent) != 1 || api.sent[0].text != telegramservice.BannedText() {
		t.Fatalf("sent = %+v", api.sent)
	}
}

func TestHandle_GivenBanCheckError_ThenFailClosed(t *testing.T) {
	api := &fakeAPI{}
	d := newDispatcher(api, &fakeGate{}, &fakeBan{err: errBoom}, &fakeLimiter{allow: true})
	d.Handle(context.Background(), msgUpdate(7, "/start"))
	if len(api.sent) != 1 || api.sent[0].text != telegramservice.BannedText() {
		t.Fatalf("sent = %+v", api.sent)
	}
}

func TestHandle_GivenNotMember_ThenJoinPrompt(t *testing.T) {
	api := &fakeAPI{}
	d := newDispatcher(api, &fakeGate{enabled: true, result: telegramservice.GateDenied}, &fakeBan{}, &fakeLimiter{allow: true})

	d.Handle(context.Background(), msgUpdate(7, "/start"))
	if len(api.sent) != 1 {
		t.Fatalf("sent = %+v", api.sent)
	}
	call := api.sent[0]
	if !strings.Contains(call.text, groupLink) {
		t.Errorf("join text missing link: %s", call.text)
	}
	kb, ok := call.markup.(models.InlineKeyboardMarkup)
	if !ok || kb.InlineKeyboard[1][0].CallbackData != telegramservice.CallbackGateCheck {
		t.Errorf("expected join keyboard with recheck button, got %+v", call.markup)
	}
}

func TestHandle_GivenGateUnknown_ThenFailClosed(t *testing.T) {
	api := &fakeAPI{}
	d := newDispatcher(api, &fakeGate{enabled: true, result: telegramservice.GateUnknown}, &fakeBan{}, &fakeLimiter{allow: true})
	d.Handle(context.Background(), msgUpdate(7, "/start"))
	if len(api.sent) != 1 || strings.Contains(api.sent[0].text, "KentangTech") {
		t.Fatalf("expected generic error text, got %+v", api.sent)
	}
}

func TestHandle_GivenAdminAndGateDenied_ThenBypassesGate(t *testing.T) {
	api := &fakeAPI{}
	d := newDispatcherWithAdmins(api, &fakeGate{enabled: true, result: telegramservice.GateDenied}, &fakeBan{}, &fakeLimiter{allow: true}, []int64{7})

	d.Handle(context.Background(), msgUpdate(7, "/start"))
	if len(api.sent) != 1 || api.sent[0].text != telegramservice.HomeText("Dodi") {
		t.Fatalf("admin must bypass gate and get home menu, got %+v", api.sent)
	}
}

func TestHandle_GivenAdminAndGateDenied_ThenBanStillApplies(t *testing.T) {
	api := &fakeAPI{}
	d := newDispatcherWithAdmins(api, &fakeGate{enabled: true, result: telegramservice.GateDenied}, &fakeBan{banned: true}, &fakeLimiter{allow: true}, []int64{7})

	d.Handle(context.Background(), msgUpdate(7, "/start"))
	if len(api.sent) != 1 || api.sent[0].text != telegramservice.BannedText() {
		t.Fatalf("banned admin must still be rejected, got %+v", api.sent)
	}
}

func TestHandle_GivenNonAdminAndGateDenied_ThenStillDenied(t *testing.T) {
	api := &fakeAPI{}
	d := newDispatcherWithAdmins(api, &fakeGate{enabled: true, result: telegramservice.GateDenied}, &fakeBan{}, &fakeLimiter{allow: true}, []int64{99})

	d.Handle(context.Background(), msgUpdate(7, "/start"))
	if len(api.sent) != 1 || !strings.Contains(api.sent[0].text, groupLink) {
		t.Fatalf("non-admin must get join prompt, got %+v", api.sent)
	}
}

func TestHandle_GivenRateLimited_ThenThrottled(t *testing.T) {
	api := &fakeAPI{}
	d := newDispatcher(api, &fakeGate{enabled: true, result: telegramservice.GateAllowed}, &fakeBan{}, &fakeLimiter{allow: false})
	d.Handle(context.Background(), msgUpdate(7, "/start"))
	if len(api.sent) != 1 || api.sent[0].text != telegramservice.RateLimitText() {
		t.Fatalf("sent = %+v", api.sent)
	}
}

func TestHandle_GivenRateLimitError_ThenFailOpen(t *testing.T) {
	api := &fakeAPI{}
	d := newDispatcher(api, &fakeGate{}, &fakeBan{}, &fakeLimiter{err: errBoom})
	d.Handle(context.Background(), msgUpdate(7, "/start"))
	if len(api.sent) != 1 || api.sent[0].text != telegramservice.HomeText("Dodi") {
		t.Fatalf("expected home menu despite limiter error, got %+v", api.sent)
	}
}

func TestHandle_GivenGateCheckCallbackWhenMember_ThenHome(t *testing.T) {
	api := &fakeAPI{}
	d := newDispatcher(api, &fakeGate{enabled: true, result: telegramservice.GateAllowed}, &fakeBan{}, &fakeLimiter{allow: true})
	d.Handle(context.Background(), cbUpdate(7, telegramservice.CallbackGateCheck))
	if len(api.edited) != 1 {
		t.Fatalf("edited = %+v", api.edited)
	}
}

func TestHandle_GivenGateCheckCallbackWhenNotMember_ThenAnswerDenied(t *testing.T) {
	api := &fakeAPI{}
	d := newDispatcher(api, &fakeGate{enabled: true, result: telegramservice.GateDenied}, &fakeBan{}, &fakeLimiter{allow: true})
	d.Handle(context.Background(), cbUpdate(7, telegramservice.CallbackGateCheck))
	if len(api.edited) != 0 || len(api.answered) != 1 {
		t.Fatalf("edited = %+v answered = %+v", api.edited, api.answered)
	}
}
