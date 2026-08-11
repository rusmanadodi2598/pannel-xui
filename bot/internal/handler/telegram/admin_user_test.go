// Package telegramhandler test covers the FR-11 broadcast & user flows.
//
// @file      internal/handler/telegram/admin_user_test.go
// @for       Given admin text/callbacks, then broadcast and ban/unban execute.
// @uses      testing, context, strings, internal/repository/postgres,
// internal/service/telegram
// @reason    Split from admin_test.go to respect the 250-line limit (§1.1).
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
	telegramservice "github.com/kentangtech/bot-order/internal/service/telegram"
)

func TestAdmin_GivenBroadcastFlow_ThenPromptInputPreviewAndSend(t *testing.T) {
	api := &fakeAPI{}
	ops := &fakeAdminOps{bcastTotal: 5}
	fsm := &fakeAdminFSM{}
	d := dispatcherWithAdmin(api, &fakeAdminDeps{ops: ops, fsm: fsm})

	// Step 1: arm the FSM.
	d.Handle(context.Background(), cbUpdate(7, telegramservice.CallbackAdminBroadcast))
	if !fsm.armed || fsm.state != "broadcast" {
		t.Fatalf("fsm = %q, want broadcast", fsm.state)
	}
	// Step 2: typed message → preview + confirm.
	d.Handle(context.Background(), msgUpdate(7, "Promo minggu ini!"))
	if fsm.state != "broadcast:Promo minggu ini!" {
		t.Fatalf("fsm = %q, want staged broadcast text", fsm.state)
	}
	// Step 3: confirm → Broadcast called.
	d.Handle(context.Background(), cbUpdate(7, telegramservice.CallbackAdminBcastSend))
	if ops.broadcast != "Promo minggu ini!" {
		t.Fatalf("broadcast text = %q", ops.broadcast)
	}
	if len(api.edited) != 2 || !strings.Contains(api.edited[1].text, "5 user") {
		t.Fatalf("start text = %+v", api.edited)
	}
}

func TestAdmin_GivenBanFlow_ThenConfirmAndBanExecuted(t *testing.T) {
	api := &fakeAPI{}
	ops := &fakeAdminOps{user: &postgres.User{TelegramID: 123, FirstName: "Budi"}}
	fsm := &fakeAdminFSM{}
	d := dispatcherWithAdmin(api, &fakeAdminDeps{ops: ops, fsm: fsm})

	d.Handle(context.Background(), cbUpdate(7, telegramservice.CallbackAdminBan))
	if fsm.state != "ban" {
		t.Fatalf("fsm = %q, want ban", fsm.state)
	}
	d.Handle(context.Background(), msgUpdate(7, "123"))
	if len(api.sent) != 1 || !strings.Contains(api.sent[0].text, "Konfirmasi Ban") {
		t.Fatalf("ban confirm screen = %+v", api.sent)
	}
	assertButton(t, editCall{markup: api.sent[0].markup}, telegramservice.PrefixAdminBanConfirm+"123")

	d.Handle(context.Background(), cbUpdate(7, telegramservice.PrefixAdminBanConfirm+"123"))
	if len(ops.banned) != 1 || ops.banned[0] != 123 {
		t.Fatalf("banned = %v, want [123]", ops.banned)
	}
	if fsm.armed {
		t.Error("fsm must be cleared after ban")
	}
}

func TestAdmin_GivenUnbanFlow_ThenConfirmAndUnbanExecuted(t *testing.T) {
	api := &fakeAPI{}
	ops := &fakeAdminOps{}
	fsm := &fakeAdminFSM{}
	d := dispatcherWithAdmin(api, &fakeAdminDeps{ops: ops, fsm: fsm})

	d.Handle(context.Background(), cbUpdate(7, telegramservice.CallbackAdminUnban))
	d.Handle(context.Background(), msgUpdate(7, "456"))
	d.Handle(context.Background(), cbUpdate(7, telegramservice.PrefixAdminUnbanConfirm+"456"))
	if len(ops.unbanned) != 1 || ops.unbanned[0] != 456 {
		t.Fatalf("unbanned = %v, want [456]", ops.unbanned)
	}
}

func TestAdmin_GivenCancel_ThenFSMClearedAndMenu(t *testing.T) {
	api := &fakeAPI{}
	fsm := &fakeAdminFSM{state: "ban", armed: true}
	d := dispatcherWithAdmin(api, &fakeAdminDeps{fsm: fsm})
	d.Handle(context.Background(), cbUpdate(7, telegramservice.CallbackAdminCancel))

	if fsm.armed {
		t.Error("fsm must be cleared on cancel")
	}
	if len(api.edited) != 1 || api.edited[0].text != telegramservice.AdminMenuText() {
		t.Fatalf("cancel result = %+v", api.edited)
	}
}
