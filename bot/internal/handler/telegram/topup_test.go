// Package telegramhandler test covers the FR-06 topup flow end to end.
//
// @file      internal/handler/telegram/topup_test.go
// @for       Menu → quick-pick → custom FSM → confirm (stub gateway) routing.
// @uses      testing, context, strings, github.com/go-telegram/bot/models,
// internal/service/telegram, internal/service/topup
// @reason    Menus/flows are product-final; tests pin the routing & FSM contract.
// @author    Dodi Rusmana <rusmanadodi@kentangtechstore.com>
// @layer     handler
// @stability experimental
// @since     2026-08-11
package telegramhandler

import (
	"context"
	"errors"
	"strings"
	"testing"

	telegramservice "github.com/kentangtech/bot-order/internal/service/telegram"
	topupsvc "github.com/kentangtech/bot-order/internal/service/topup"
)

func TestTopup_GivenMenuCallback_ThenAmountKeyboardWithBalance(t *testing.T) {
	api := &fakeAPI{}
	d := dispatcherWithTopup(api, newFakeTopup())
	d.Handle(context.Background(), cbUpdate(7, telegramservice.CallbackTopup))

	if len(api.edited) != 1 {
		t.Fatalf("edited = %+v", api.edited)
	}
	if !strings.Contains(api.edited[0].text, "Saldo saat ini: Rp 50.000") {
		t.Errorf("menu text missing balance: %q", api.edited[0].text)
	}
	assertButton(t, api.edited[0], telegramservice.PrefixTopupAmount+"10000")
	assertButton(t, api.edited[0], telegramservice.CallbackTopupBack)
}

func TestTopup_GivenAmountPick_ThenConfirmSummary(t *testing.T) {
	f := newFakeTopup()
	f.topups.quote = &topupsvc.Quote{Net: 10000, Gross: 10300, TotalFee: 300, FeePercent: 0.02775}
	api := &fakeAPI{}
	d := dispatcherWithTopup(api, f)
	d.Handle(context.Background(), cbUpdate(7, telegramservice.PrefixTopupAmount+"10000"))

	if len(api.edited) != 1 {
		t.Fatalf("edited = %+v", api.edited)
	}
	e := api.edited[0]
	if !strings.Contains(e.text, "Saldo diterima: Rp 10.000") || !strings.Contains(e.text, "Total bayar: Rp 10.300") {
		t.Errorf("summary text mismatch: %q", e.text)
	}
	assertButton(t, e, telegramservice.PrefixTopupConfirm+"10000")
	assertButton(t, e, telegramservice.CallbackTopup) // back re-renders amount picker
	if f.fsm.clearCalls == 0 {
		t.Error("amount pick must clear any stale FSM")
	}
}

func TestTopup_GivenAmountOutOfRange_ThenAnswered(t *testing.T) {
	f := newFakeTopup()
	f.topups.quoteErr = topupsvc.ErrInvalidNominal
	api := &fakeAPI{}
	d := dispatcherWithTopup(api, f)
	d.Handle(context.Background(), cbUpdate(7, telegramservice.PrefixTopupAmount+"100"))

	if len(api.answered) != 1 || len(api.edited) != 0 {
		t.Fatalf("answered = %+v, edited = %+v", api.answered, api.edited)
	}
}

func TestTopup_GivenConfirmWithStubGateway_ThenUnavailableTextNoPanic(t *testing.T) {
	f := newFakeTopup()
	f.topups.quote = &topupsvc.Quote{Net: 10000, Gross: 10300, TotalFee: 300, FeePercent: 0.02775}
	f.topups.createErr = errors.New("gateway down")
	api := &fakeAPI{}
	d := dispatcherWithTopup(api, f)
	d.Handle(context.Background(), cbUpdate(7, telegramservice.PrefixTopupConfirm+"10000"))

	if len(api.edited) != 1 {
		t.Fatalf("edited = %+v", api.edited)
	}
	if api.edited[0].text != telegramservice.TopupAPIUnavailableText() {
		t.Errorf("expected unavailable text, got %q", api.edited[0].text)
	}
	if f.fsm.clearCalls == 0 {
		t.Error("confirm must clear the custom FSM")
	}
}

func TestTopup_GivenConfirmWithResult_ThenPaymentText(t *testing.T) {
	f := newFakeTopup()
	f.topups.quote = &topupsvc.Quote{Net: 10000, Gross: 10300, TotalFee: 300, FeePercent: 0.02775}
	f.topups.result = &topupsvc.PaymentResult{OrderID: "TP-1", CheckoutURL: "https://api.midtrans.com/v2/qris/abc", Amount: 10300}
	api := &fakeAPI{}
	d := dispatcherWithTopup(api, f)
	d.Handle(context.Background(), cbUpdate(7, telegramservice.PrefixTopupConfirm+"10000"))

	if len(api.edited) != 1 {
		t.Fatalf("edited = %+v", api.edited)
	}
	if !strings.Contains(api.edited[0].text, "Pembayaran QRIS dibuat") {
		t.Errorf("payment text mismatch: %q", api.edited[0].text)
	}
	if f.topups.createdReq == nil || f.topups.createdReq.TelegramUserID != 7 || f.topups.createdReq.NetAmount != 10000 {
		t.Errorf("created request = %+v", f.topups.createdReq)
	}
}

func TestTopup_GivenCustomTap_ThenFSMArmedAndPrompt(t *testing.T) {
	f := newFakeTopup()
	api := &fakeAPI{}
	d := dispatcherWithTopup(api, f)
	d.Handle(context.Background(), cbUpdate(7, telegramservice.PrefixTopupCustom))

	if f.fsm.setCalls != 1 {
		t.Errorf("FSM set calls = %d, want 1", f.fsm.setCalls)
	}
	if len(api.edited) != 1 || !strings.Contains(api.edited[0].text, "Masukkan nominal saldo") {
		t.Fatalf("edited = %+v", api.edited)
	}
	assertButton(t, api.edited[0], telegramservice.CallbackTopupBack)
}

func TestTopup_GivenFSMText_ThenSummaryAndFSMCleared(t *testing.T) {
	f := newFakeTopup()
	f.fsm.pending = true
	api := &fakeAPI{}
	d := dispatcherWithTopup(api, f)
	d.Handle(context.Background(), msgUpdate(7, "25000"))

	if len(api.sent) != 1 {
		t.Fatalf("sent = %+v", api.sent)
	}
	if !strings.Contains(api.sent[0].text, "Ringkasan Top Up") {
		t.Errorf("summary mismatch: %q", api.sent[0].text)
	}
	assertButtonInMarkup(t, api.sent[0].markup, telegramservice.PrefixTopupConfirm+"25000")
	if f.fsm.clearCalls == 0 {
		t.Error("valid custom nominal must clear the FSM")
	}
}

func TestTopup_GivenFSMTextInvalid_ThenPromptAgain(t *testing.T) {
	f := newFakeTopup()
	f.fsm.pending = true
	api := &fakeAPI{}
	d := dispatcherWithTopup(api, f)
	d.Handle(context.Background(), msgUpdate(7, "abc"))

	if len(api.sent) != 1 || !strings.Contains(api.sent[0].text, "Masukkan nominal saldo") {
		t.Fatalf("sent = %+v", api.sent)
	}
	if f.fsm.clearCalls != 0 {
		t.Error("invalid input must NOT clear the FSM (user can retry)")
	}
}

func TestTopup_GivenFSMTextOutOfRange_ThenPromptAgain(t *testing.T) {
	f := newFakeTopup()
	f.fsm.pending = true
	f.topups.quoteErr = topupsvc.ErrInvalidNominal
	api := &fakeAPI{}
	d := dispatcherWithTopup(api, f)
	d.Handle(context.Background(), msgUpdate(7, "100"))

	if len(api.sent) != 1 || !strings.Contains(api.sent[0].text, "Masukkan nominal saldo") {
		t.Fatalf("sent = %+v", api.sent)
	}
	if f.fsm.clearCalls != 0 {
		t.Error("out-of-range input must NOT clear the FSM")
	}
}

func TestTopup_GivenCancelCommand_ThenFSMClearedAndHome(t *testing.T) {
	f := newFakeTopup()
	f.fsm.pending = true
	api := &fakeAPI{}
	d := dispatcherWithTopup(api, f)
	d.Handle(context.Background(), msgUpdate(7, "/cancel"))

	if f.fsm.clearCalls != 1 {
		t.Errorf("FSM clear calls = %d, want 1", f.fsm.clearCalls)
	}
	if len(api.sent) != 1 || api.sent[0].text != telegramservice.TopupCancelledText() {
		t.Fatalf("sent = %+v", api.sent)
	}
	assertButtonInMarkup(t, api.sent[0].markup, telegramservice.CallbackBuy)
}

func TestTopup_GivenBackCallback_ThenFSMClearedAndHomeEdited(t *testing.T) {
	f := newFakeTopup()
	f.fsm.pending = true
	api := &fakeAPI{}
	d := dispatcherWithTopup(api, f)
	d.Handle(context.Background(), cbUpdate(7, telegramservice.CallbackTopupBack))

	if f.fsm.clearCalls != 1 {
		t.Errorf("FSM clear calls = %d, want 1", f.fsm.clearCalls)
	}
	if len(api.edited) != 1 {
		t.Fatalf("edited = %+v", api.edited)
	}
}
