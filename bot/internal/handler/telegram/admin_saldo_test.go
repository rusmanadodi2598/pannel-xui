// Package telegramhandler test covers the admin adjust-saldo flow (FR-11, v1.39).
//
// @file      internal/handler/telegram/admin_saldo_test.go
// @for       FSM flow: menu → id input → nominal input → confirm → AdjustBalance.
// @uses      testing, context, strings, github.com/go-telegram/bot/models,
// internal/repository/postgres, internal/service/telegram, gorm.io/gorm
// @reason    Manual balance corrections are money-moving: the handler flow must
// stage the id + nominal, confirm explicitly and never call AdjustBalance early.
// @author    Dodi Rusmana <rusmanadodi@kentangtechstore.com>
// @layer     handler
// @stability experimental
// @since     2026-08-12
package telegramhandler

import (
	"context"
	"strings"
	"testing"

	"github.com/kentangtech/bot-order/internal/repository/postgres"
	telegramservice "github.com/kentangtech/bot-order/internal/service/telegram"
	"gorm.io/gorm"
)

func TestAdminSaldo_GivenMenu_ThenCreditAndDebitShown(t *testing.T) {
	api := &fakeAPI{}
	d := dispatcherWithAdmin(api, &fakeAdminDeps{})
	d.Handle(context.Background(), cbUpdate(7, telegramservice.CallbackAdminSaldo))

	if len(api.edited) != 1 || api.edited[0].text != telegramservice.AdminSaldoMenuText() {
		t.Fatalf("saldo menu = %+v", api.edited)
	}
	assertButton(t, api.edited[0], telegramservice.PrefixAdminSaldoKredit)
	assertButton(t, api.edited[0], telegramservice.PrefixAdminSaldoDebit)
}

func TestAdminSaldo_GivenKreditArmed_ThenFSMAndIDPrompt(t *testing.T) {
	api := &fakeAPI{}
	fsm := &fakeAdminFSM{}
	d := dispatcherWithAdmin(api, &fakeAdminDeps{fsm: fsm})
	d.Handle(context.Background(), cbUpdate(7, telegramservice.PrefixAdminSaldoKredit))

	if !fsm.armed || fsm.state != "saldo:kredit" {
		t.Fatalf("fsm = %q armed=%v, want saldo:kredit", fsm.state, fsm.armed)
	}
	if len(api.edited) != 1 || api.edited[0].text != telegramservice.AdminSaldoIDPrompt(true) {
		t.Fatalf("prompt = %+v", api.edited)
	}
}

func TestAdminSaldo_GivenIDInput_ThenStagedAndAmountPrompt(t *testing.T) {
	api := &fakeAPI{}
	ops := &fakeAdminOps{user: &postgres.User{ID: 9, TelegramID: 42, FirstName: "Budi"}}
	fsm := &fakeAdminFSM{state: "saldo:kredit", armed: true}
	d := dispatcherWithAdmin(api, &fakeAdminDeps{ops: ops, fsm: fsm})
	d.Handle(context.Background(), msgUpdate(7, "42"))

	if fsm.state != "saldo:kredit:42" {
		t.Fatalf("fsm = %q, want saldo:kredit:42", fsm.state)
	}
	if len(api.sent) != 1 || !strings.Contains(api.sent[0].text, "Budi") {
		t.Fatalf("amount prompt = %+v", api.sent)
	}
}

func TestAdminSaldo_GivenUnknownID_ThenClearedAndNotFound(t *testing.T) {
	api := &fakeAPI{}
	ops := &fakeAdminOps{lookupErr: gorm.ErrRecordNotFound}
	fsm := &fakeAdminFSM{state: "saldo:kredit", armed: true}
	d := dispatcherWithAdmin(api, &fakeAdminDeps{ops: ops, fsm: fsm})
	d.Handle(context.Background(), msgUpdate(7, "999"))

	if fsm.armed {
		t.Error("fsm must be cleared for an unknown user")
	}
	if len(api.sent) != 1 || !strings.Contains(api.sent[0].text, "belum terdaftar") {
		t.Fatalf("not-found = %+v", api.sent)
	}
}

func TestAdminSaldo_GivenAmountInput_ThenConfirmShownAndStaged(t *testing.T) {
	api := &fakeAPI{}
	ops := &fakeAdminOps{user: &postgres.User{ID: 9, TelegramID: 42, FirstName: "Budi"}}
	fsm := &fakeAdminFSM{state: "saldo:kredit:42", armed: true}
	d := dispatcherWithAdmin(api, &fakeAdminDeps{ops: ops, fsm: fsm})
	d.Handle(context.Background(), msgUpdate(7, "25000"))

	// The confirm-pending state stays armed so confirm verifies the staging
	// before executing (fix review v1.39: double-tap idempotence).
	if !fsm.armed || fsm.state != "saldo:confirm:kredit:42:25000" {
		t.Fatalf("fsm = %q armed=%v, want saldo:confirm:kredit:42:25000", fsm.state, fsm.armed)
	}
	if len(api.sent) != 1 || !strings.Contains(api.sent[0].text, "Kredit") {
		t.Fatalf("confirm = %+v", api.sent)
	}
	assertButton(t, editCall{markup: api.sent[0].markup}, telegramservice.PrefixAdminSaldoConfirm+"kredit:42:25000")
}

func TestAdminSaldo_GivenInvalidAmount_ThenRePromptWithUserLabel(t *testing.T) {
	api := &fakeAPI{}
	ops := &fakeAdminOps{user: &postgres.User{ID: 9, TelegramID: 42, FirstName: "Budi"}}
	fsm := &fakeAdminFSM{state: "saldo:kredit:42", armed: true}
	d := dispatcherWithAdmin(api, &fakeAdminDeps{ops: ops, fsm: fsm})
	d.Handle(context.Background(), msgUpdate(7, "abc"))

	// The id+stage must survive a bad nominal so the user can retype.
	if !fsm.armed || fsm.state != "saldo:kredit:42" {
		t.Fatalf("fsm = %q armed=%v, want staged id kept", fsm.state, fsm.armed)
	}
	if len(api.sent) != 1 || !strings.Contains(api.sent[0].text, "Budi") {
		t.Fatalf("re-prompt = %+v (must show real user label)", api.sent)
	}
}

func TestAdminSaldo_GivenConfirm_ThenAdjustBalanceExecuted(t *testing.T) {
	api := &fakeAPI{}
	ops := &fakeAdminOps{user: &postgres.User{ID: 9, TelegramID: 42, FirstName: "Budi"}}
	fsm := &fakeAdminFSM{state: "saldo:confirm:kredit:42:25000", armed: true}
	d := dispatcherWithAdmin(api, &fakeAdminDeps{ops: ops, fsm: fsm})
	d.Handle(context.Background(), cbUpdate(7, telegramservice.PrefixAdminSaldoConfirm+"kredit:42:25000"))

	if ops.adjust == nil || !ops.adjust.credit || ops.adjust.tgID != 42 || ops.adjust.amount != 25000 {
		t.Fatalf("adjust = %+v, want credit 25000 for 42", ops.adjust)
	}
	if fsm.armed {
		t.Error("fsm must be cleared after execution")
	}
	if len(api.edited) != 1 || !strings.Contains(api.edited[0].text, "Rp 123.000") {
		t.Fatalf("done = %+v", api.edited)
	}
}

func TestAdminSaldo_GivenDoubleTapConfirm_ThenOnlyOnceExecuted(t *testing.T) {
	api := &fakeAPI{}
	ops := &fakeAdminOps{user: &postgres.User{ID: 9, TelegramID: 42, FirstName: "Budi"}}
	fsm := &fakeAdminFSM{state: "saldo:confirm:kredit:42:25000", armed: true}
	d := dispatcherWithAdmin(api, &fakeAdminDeps{ops: ops, fsm: fsm})
	// First tap executes and clears the FSM.
	d.Handle(context.Background(), cbUpdate(7, telegramservice.PrefixAdminSaldoConfirm+"kredit:42:25000"))
	// Second tap arrives with the same payload but no staged state → must NOT run.
	d.Handle(context.Background(), cbUpdate(7, telegramservice.PrefixAdminSaldoConfirm+"kredit:42:25000"))

	if ops.adjust == nil {
		t.Fatal("first tap must execute")
	}
	if ops.adjustCalls != 1 {
		t.Fatalf("AdjustBalance called %d times, want exactly 1", ops.adjustCalls)
	}
	if len(api.answered) < 2 || !strings.Contains(api.answered[1], "kedaluwarsa") {
		t.Fatalf("second tap answers = %+v, want expired notice", api.answered)
	}
}

func TestAdminSaldo_GivenDebitConfirm_ThenDebitExecuted(t *testing.T) {
	api := &fakeAPI{}
	ops := &fakeAdminOps{user: &postgres.User{ID: 9, TelegramID: 42, FirstName: "Budi"}}
	fsm := &fakeAdminFSM{state: "saldo:confirm:debit:42:5000", armed: true}
	d := dispatcherWithAdmin(api, &fakeAdminDeps{ops: ops, fsm: fsm})
	d.Handle(context.Background(), cbUpdate(7, telegramservice.PrefixAdminSaldoConfirm+"debit:42:5000"))

	if ops.adjust == nil || ops.adjust.credit || ops.adjust.amount != 5000 {
		t.Fatalf("adjust = %+v, want debit 5000", ops.adjust)
	}
}

func TestAdminSaldo_GivenInsufficientDebit_ThenFriendlyAnswer(t *testing.T) {
	api := &fakeAPI{}
	ops := &fakeAdminOps{adjustErr: postgres.ErrInsufficientBalance}
	fsm := &fakeAdminFSM{state: "saldo:confirm:debit:42:999999", armed: true}
	d := dispatcherWithAdmin(api, &fakeAdminDeps{ops: ops, fsm: fsm})
	d.Handle(context.Background(), cbUpdate(7, telegramservice.PrefixAdminSaldoConfirm+"debit:42:999999"))

	if len(api.answered) != 1 || !strings.Contains(api.answered[0], "tidak mencukupi") {
		t.Fatalf("answers = %+v", api.answered)
	}
}

func TestAdminSaldo_GivenNonAdmin_ThenDenied(t *testing.T) {
	api := &fakeAPI{}
	d := dispatcherWithAdmin(api, &fakeAdminDeps{})
	d.Handle(context.Background(), cbUpdate(99, telegramservice.CallbackAdminSaldo))

	if len(api.answered) != 1 || !strings.Contains(api.answered[0], "Akses ditolak") {
		t.Fatalf("non-admin = %+v", api.answered)
	}
}
