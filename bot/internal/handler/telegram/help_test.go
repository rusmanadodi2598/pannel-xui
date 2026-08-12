// Package telegramhandler test covers the FR-15 static help routing.
//
// @file      internal/handler/telegram/help_test.go
// @for       Route each help:* callback to the right static page (edit, not send).
// @uses      testing, context, strings, internal/service/telegram
// @reason    Help is pure static content (no service seam); the test locks the
// full FR-15 callback contract and the edit-in-place navigation behaviour.
// @author    Dodi Rusmana <rusmanadodi@kentangtechstore.com>
// @layer     handler
// @stability stable
// @since     2026-08-11
package telegramhandler

import (
	"context"
	"strings"
	"testing"

	telegramservice "github.com/kentangtech/bot-order/internal/service/telegram"
)

func TestHelp_GivenMenuCallback_ThenEditsHelpHub(t *testing.T) {
	api := &fakeAPI{}
	d := newDispatcher(api, &fakeGate{}, &fakeBan{}, &fakeLimiter{allow: true})
	d.Handle(context.Background(), cbUpdate(7, telegramservice.CallbackHelp))

	if len(api.edited) != 1 {
		t.Fatalf("edited = %d, want 1", len(api.edited))
	}
	if api.edited[0].text != telegramservice.HelpMenuText() {
		t.Errorf("text mismatch: %q", api.edited[0].text)
	}
	if len(api.sent) != 0 {
		t.Fatalf("help must edit, not send: %+v", api.sent)
	}
	if len(api.answered) != 1 {
		t.Fatalf("expected callback answered, got %+v", api.answered)
	}
}

func TestHelp_GivenEveryCallback_ThenEditsItsPage(t *testing.T) {
	pages := map[string]string{
		telegramservice.CallbackHelpOrder:      telegramservice.HelpOrderText(),
		telegramservice.CallbackHelpTopup:      telegramservice.HelpTopupText(),
		telegramservice.CallbackHelpDisclaimer: telegramservice.HelpDisclaimerText(),
		telegramservice.CallbackHelpTosAccount: telegramservice.HelpTosAccountText(),
		telegramservice.CallbackHelpTosPayment: telegramservice.HelpTosPaymentText(),
		telegramservice.CallbackHelpInfo:       telegramservice.HelpInfoText(),
	}
	for cb, want := range pages {
		api := &fakeAPI{}
		d := newDispatcher(api, &fakeGate{}, &fakeBan{}, &fakeLimiter{allow: true})
		d.Handle(context.Background(), cbUpdate(7, cb))

		if len(api.edited) != 1 {
			t.Errorf("%s: edited = %d, want 1", cb, len(api.edited))
			continue
		}
		if api.edited[0].text != want {
			t.Errorf("%s: text mismatch\ngot:  %q\nwant: %q", cb, api.edited[0].text, want)
		}
		if len(api.sent) != 0 {
			t.Errorf("%s: must edit, not send", cb)
		}
	}
}

func TestHelp_GivenUnknownHelpCallback_ThenNoopAnswer(t *testing.T) {
	api := &fakeAPI{}
	d := newDispatcher(api, &fakeGate{}, &fakeBan{}, &fakeLimiter{allow: true})
	d.Handle(context.Background(), cbUpdate(7, "help:bogus"))

	if len(api.answered) != 1 {
		t.Fatalf("answered = %+v, want 1", api.answered)
	}
	if !strings.Contains(api.answered[0], telegramservice.UnavailableText()) {
		t.Errorf("answer = %q, want unavailable text", api.answered[0])
	}
	if len(api.edited) != 0 {
		t.Fatalf("unknown help callback must not edit: %+v", api.edited)
	}
}
