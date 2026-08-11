// Package telegramhandler test covers the FR-11 admin menu & price flows.
//
// @file      internal/handler/telegram/admin_test.go
// @for       Given admin commands/callbacks, then menu & price flows execute;
//
//	non-admins are denied on every surface.
//
// @uses      testing, context, strings, internal/domain, internal/service/telegram
// @reason    Guards the FR-11 conversation contract without DB/network.
// @author    Dodi Rusmana <rusmanadodi@kentangtechstore.com>
// @layer     handler
// @stability experimental
// @since     2026-08-11
package telegramhandler

import (
	"context"
	"strings"
	"testing"

	"github.com/kentangtech/bot-order/internal/domain"
	telegramservice "github.com/kentangtech/bot-order/internal/service/telegram"
)

func TestAdmin_GivenNonAdminCommand_ThenDenied(t *testing.T) {
	api := &fakeAPI{}
	d := dispatcherWithAdmin(api, &fakeAdminDeps{})
	d.Handle(context.Background(), msgUpdate(99, "/admin"))
	if len(api.sent) != 1 || api.sent[0].text != telegramservice.AdminDeniedText() {
		t.Fatalf("non-admin /admin = %+v", api.sent)
	}
}

func TestAdmin_GivenCommand_ThenMenu(t *testing.T) {
	api := &fakeAPI{}
	d := dispatcherWithAdmin(api, &fakeAdminDeps{})
	d.Handle(context.Background(), msgUpdate(7, "/admin"))
	if len(api.sent) != 1 || api.sent[0].text != telegramservice.AdminMenuText() {
		t.Fatalf("admin /admin = %+v", api.sent)
	}
	assertButton(t, editCall{markup: api.sent[0].markup}, telegramservice.CallbackAdminPrice)
}

func TestAdmin_GivenNonAdminCallback_ThenDeniedAnswer(t *testing.T) {
	api := &fakeAPI{}
	d := dispatcherWithAdmin(api, &fakeAdminDeps{})
	d.Handle(context.Background(), cbUpdate(99, telegramservice.CallbackAdminPrice))
	if len(api.answered) != 1 || !strings.Contains(api.answered[0], "Akses ditolak") {
		t.Fatalf("non-admin callback = %+v", api.answered)
	}
}

func TestAdmin_GivenPrice_ThenPlanListShown(t *testing.T) {
	api := &fakeAPI{}
	ops := &fakeAdminOps{plans: []domain.VpnPlan{
		{CountryCode: "ID", CountryName: "Indonesia", Days: 15, Price: 4000, Enabled: true},
		{CountryCode: "ID", CountryName: "Indonesia", Days: 30, Price: 7000, Enabled: false},
	}}
	d := dispatcherWithAdmin(api, &fakeAdminDeps{ops: ops})
	d.Handle(context.Background(), cbUpdate(7, telegramservice.CallbackAdminPrice))

	if len(api.edited) != 1 || api.edited[0].text != telegramservice.AdminPriceText() {
		t.Fatalf("price list = %+v", api.edited)
	}
	assertButton(t, api.edited[0], telegramservice.PrefixAdminPlan+"ID:15")
	assertButton(t, api.edited[0], telegramservice.PrefixAdminPlan+"ID:30")
}

func TestAdmin_GivenPlanDetail_ThenDetailShown(t *testing.T) {
	api := &fakeAPI{}
	ops := &fakeAdminOps{plan: &domain.VpnPlan{CountryCode: "ID", CountryName: "Indonesia", Days: 15, Price: 4000, Enabled: true}}
	d := dispatcherWithAdmin(api, &fakeAdminDeps{ops: ops})
	d.Handle(context.Background(), cbUpdate(7, telegramservice.PrefixAdminPlan+"ID:15"))

	if len(api.edited) != 1 || !strings.Contains(api.edited[0].text, "Detail Paket") {
		t.Fatalf("plan detail = %+v", api.edited)
	}
	assertButton(t, api.edited[0], telegramservice.PrefixAdminSetPrice+"ID:15")
	assertButton(t, api.edited[0], telegramservice.PrefixAdminToggle+"ID:15")
}

func TestAdmin_GivenSetPrice_ThenFSMArmedAndPrompt(t *testing.T) {
	api := &fakeAPI{}
	fsm := &fakeAdminFSM{}
	d := dispatcherWithAdmin(api, &fakeAdminDeps{fsm: fsm})
	d.Handle(context.Background(), cbUpdate(7, telegramservice.PrefixAdminSetPrice+"ID:15"))

	if !fsm.armed || fsm.state != "price:ID:15" {
		t.Fatalf("fsm = %q armed=%v, want price:ID:15", fsm.state, fsm.armed)
	}
	if len(api.edited) != 1 || api.edited[0].text != telegramservice.AdminSetPricePrompt("ID", 15) {
		t.Fatalf("prompt = %+v", api.edited)
	}
}

func TestAdmin_GivenPriceInput_ThenSetPriceCalled(t *testing.T) {
	api := &fakeAPI{}
	ops := &fakeAdminOps{}
	fsm := &fakeAdminFSM{state: "price:ID:15", armed: true}
	d := dispatcherWithAdmin(api, &fakeAdminDeps{ops: ops, fsm: fsm})
	d.Handle(context.Background(), msgUpdate(7, "7500"))

	if ops.priceSet == nil || ops.priceSet.country != "ID" || ops.priceSet.days != 15 || ops.priceSet.price != 7500 {
		t.Fatalf("SetPrice = %+v", ops.priceSet)
	}
	if fsm.armed {
		t.Error("fsm must be cleared after a successful price input")
	}
	if len(api.sent) != 1 || !strings.Contains(api.sent[0].text, "Rp 7.500") {
		t.Fatalf("confirmation = %+v", api.sent)
	}
}

func TestAdmin_GivenInvalidPriceInput_ThenReprompt(t *testing.T) {
	api := &fakeAPI{}
	ops := &fakeAdminOps{}
	fsm := &fakeAdminFSM{state: "price:ID:15", armed: true}
	d := dispatcherWithAdmin(api, &fakeAdminDeps{ops: ops, fsm: fsm})
	d.Handle(context.Background(), msgUpdate(7, "abc"))

	if ops.priceSet != nil {
		t.Error("SetPrice must not be called for invalid input")
	}
	if !fsm.armed {
		t.Error("fsm must stay armed for another attempt")
	}
}

func TestAdmin_GivenToggle_ThenSetEnabledCalledAndDetailRendered(t *testing.T) {
	api := &fakeAPI{}
	ops := &fakeAdminOps{plan: &domain.VpnPlan{CountryCode: "ID", Days: 15, Enabled: true}}
	d := dispatcherWithAdmin(api, &fakeAdminDeps{ops: ops})
	d.Handle(context.Background(), cbUpdate(7, telegramservice.PrefixAdminToggle+"ID:15"))

	if ops.enabledSet == nil || ops.enabledSet.enabled {
		t.Fatalf("SetEnabled = %+v, want enabled=false", ops.enabledSet)
	}
	// The toggle answers with the state text, then the detail re-render answers.
	if len(api.answered) < 1 || !strings.Contains(api.answered[0], "dinonaktifkan") {
		t.Fatalf("toggle answers = %+v", api.answered)
	}
}
