// Package telegramhandler test also covers the topup edge cases.
//
// @file      internal/handler/telegram/topup_edge_test.go
// @for       Help-hint privacy, /start FSM abort, nil-message callback safety.
// @uses      testing, context, github.com/go-telegram/bot/models, internal/service/telegram
// @reason    Split from topup_test.go to respect the 250-line limit (AGENTS.md §1.1).
// @author    Dodi Rusmana <rusmanadodi@kentangtechstore.com>
// @layer     handler
// @stability experimental
// @since     2026-08-11
package telegramhandler

import (
	"context"
	"testing"

	telegramservice "github.com/kentangtech/bot-order/internal/service/telegram"
)

func TestTopup_GivenTextWithoutFSM_ThenHelpHint(t *testing.T) {
	api := &fakeAPI{}
	d := dispatcherWithTopup(api, newFakeTopup())
	d.Handle(context.Background(), msgUpdate(7, "halo dunia"))

	if len(api.sent) != 1 || api.sent[0].text != telegramservice.HelpHintText() {
		t.Fatalf("sent = %+v", api.sent)
	}
}

func TestTopup_GivenGroupTextWithoutFSM_ThenNoReply(t *testing.T) {
	api := &fakeAPI{}
	d := dispatcherWithTopup(api, newFakeTopup())
	upd := msgUpdate(7, "halo dunia")
	upd.Message.Chat.Type = "group" // bot must stay silent in shared chats
	d.Handle(context.Background(), upd)

	if len(api.sent) != 0 {
		t.Fatalf("group text must not be answered: %+v", api.sent)
	}
}

func TestTopup_GivenStartWhileFSMPending_ThenFSMClearedAndHome(t *testing.T) {
	f := newFakeTopup()
	f.fsm.pending = true
	api := &fakeAPI{}
	d := dispatcherWithTopup(api, f)
	d.Handle(context.Background(), msgUpdate(7, "/start"))

	if f.fsm.clearCalls != 1 {
		t.Errorf("FSM clear calls = %d, want 1 (start aborts pending input)", f.fsm.clearCalls)
	}
	if len(api.sent) != 1 || api.sent[0].text != telegramservice.HomeText("Dodi") {
		t.Fatalf("sent = %+v", api.sent)
	}
}

func TestTopup_GivenCallbackWithoutMessage_ThenNoPanic(t *testing.T) {
	f := newFakeTopup()
	api := &fakeAPI{}
	d := dispatcherWithTopup(api, f)
	upd := cbUpdate(7, telegramservice.CallbackTopup)
	upd.CallbackQuery.Message.Message = nil
	d.Handle(context.Background(), upd)

	if len(api.answered) != 1 {
		t.Fatalf("answered = %+v", api.answered)
	}
}
