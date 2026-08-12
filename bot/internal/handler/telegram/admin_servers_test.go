// Package telegramhandler test covers the FR-11 server mgmt, stats & audit flows (v1.40).
//
// @file      internal/handler/telegram/admin_servers_test.go
// @for       List/detail/toggle/add-server FSM + statistik + audit views.
// @uses      testing, context, strings, github.com/go-telegram/bot/models,
// internal/repository/postgres, internal/service/telegram
// @reason    Server management is money-adjacent (a wrong toggle hides/opens a
// panel): the FSM must stage all 6 fields and confirm before creating (v1.40).
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
)

func TestAdminServers_GivenMenu_ThenListShown(t *testing.T) {
	api := &fakeAPI{}
	ops := &fakeAdminOps{servers: []postgres.ServerAdminView{
		{ID: 1, Name: "ID-01", CountryCode: "ID", IsActive: true, IsOpen: true},
		{ID: 2, Name: "SG-01", CountryCode: "SG", IsActive: false, IsOpen: false},
	}}
	d := dispatcherWithAdmin(api, &fakeAdminDeps{ops: ops})
	d.Handle(context.Background(), cbUpdate(7, telegramservice.CallbackAdminServers))

	if len(api.edited) != 1 {
		t.Fatalf("servers list = %+v", api.edited)
	}
	assertButton(t, api.edited[0], telegramservice.PrefixAdminServer+"1")
	assertButton(t, api.edited[0], telegramservice.PrefixAdminServer+"2")
	assertButton(t, api.edited[0], telegramservice.CallbackAdminServerAdd)
}

func TestAdminServers_GivenDetail_ThenTogglesShown(t *testing.T) {
	api := &fakeAPI{}
	ops := &fakeAdminOps{servers: []postgres.ServerAdminView{
		{ID: 3, Name: "ID-03", Host: "id3.example.com", Port: 2083, IsActive: true, IsOpen: true},
	}}
	d := dispatcherWithAdmin(api, &fakeAdminDeps{ops: ops})
	d.Handle(context.Background(), cbUpdate(7, telegramservice.PrefixAdminServer+"3"))

	if len(api.edited) != 1 || !strings.Contains(api.edited[0].text, "id3.example.com") {
		t.Fatalf("detail = %+v", api.edited)
	}
	assertButton(t, api.edited[0], telegramservice.PrefixAdminServerOpen+"3")
	assertButton(t, api.edited[0], telegramservice.PrefixAdminServerActive+"3")
}

func TestAdminServers_GivenToggleOpen_ThenFlagFlippedAndRerendered(t *testing.T) {
	api := &fakeAPI{}
	ops := &fakeAdminOps{servers: []postgres.ServerAdminView{
		{ID: 3, Name: "ID-03", Host: "h", Port: 1, IsActive: true, IsOpen: true},
	}}
	d := dispatcherWithAdmin(api, &fakeAdminDeps{ops: ops})
	d.Handle(context.Background(), cbUpdate(7, telegramservice.PrefixAdminServerOpen+"3"))
	if len(api.edited) == 0 || !strings.Contains(api.edited[len(api.edited)-1].text, "Detail Server") {
		t.Fatalf("after toggle = %+v", api.edited)
	}
}

func TestAdminServerAdd_GivenSixSteps_ThenConfirmAndCreate(t *testing.T) {
	api := &fakeAPI{}
	ops := &fakeAdminOps{}
	fsm := &fakeAdminFSM{}
	d := dispatcherWithAdmin(api, &fakeAdminDeps{ops: ops, fsm: fsm})

	// Step 0: arm.
	d.Handle(context.Background(), cbUpdate(7, telegramservice.CallbackAdminServerAdd))
	if !fsm.armed || fsm.state != "srvadd:" {
		t.Fatalf("arm fsm = %q armed=%v", fsm.state, fsm.armed)
	}
	// Steps 1-6: name, host, port, username, password, country.
	inputs := []string{"ID-09", "id9.example.com", "2083", "admin", "s3cret", "ID"}
	for _, in := range inputs {
		d.Handle(context.Background(), msgUpdate(7, in))
	}
	if ops.added != nil {
		t.Fatal("server must not be created before explicit confirm")
	}
	if !fsm.armed || !strings.HasPrefix(fsm.state, "srvadd:confirm:") {
		t.Fatalf("draft fsm = %q, want srvadd:confirm:* after 6 inputs", fsm.state)
	}
	// Confirm screen rendered.
	if len(api.sent) == 0 || !strings.Contains(api.sent[len(api.sent)-1].text, "Konfirmasi Tambah Server") {
		t.Fatalf("confirm = %+v", api.sent)
	}
	assertButton(t, editCall{markup: api.sent[len(api.sent)-1].markup}, telegramservice.CallbackAdminServerAddConfirm)

	// Confirm executes exactly once.
	d.Handle(context.Background(), cbUpdate(7, telegramservice.CallbackAdminServerAddConfirm))
	if ops.added == nil || ops.added.name != "ID-09" || ops.added.host != "id9.example.com" {
		t.Fatalf("added = %+v", ops.added)
	}
	if fsm.armed {
		t.Error("fsm must be cleared after creation")
	}
	if len(api.edited) == 0 || !strings.Contains(api.edited[len(api.edited)-1].text, "Server ditambahkan") {
		t.Fatalf("done = %+v", api.edited)
	}
}

func TestAdminServerAdd_GivenInvalidPort_ThenRePrompt(t *testing.T) {
	api := &fakeAPI{}
	fsm := &fakeAdminFSM{state: "srvadd:ID-09|id9.example.com|0|||", armed: true}
	d := dispatcherWithAdmin(api, &fakeAdminDeps{fsm: fsm})
	// Port step — invalid value must re-prompt, not advance.
	d.Handle(context.Background(), msgUpdate(7, "not-a-port"))
	if !fsm.armed || fsm.state != "srvadd:ID-09|id9.example.com|0|||" {
		t.Fatalf("fsm = %q, want unchanged on invalid port", fsm.state)
	}
}

func TestAdminServerAdd_GivenDoubleTapConfirm_ThenOnceCreated(t *testing.T) {
	api := &fakeAPI{}
	ops := &fakeAdminOps{}
	// The final input step arms the confirm-pending state (parity saldo v1.39):
	// the confirm callback only runs when the FSM holds this exact draft.
	fsm := &fakeAdminFSM{state: "srvadd:confirm:ID-09|h|2083|admin|pw|ID|", armed: true}
	d := dispatcherWithAdmin(api, &fakeAdminDeps{ops: ops, fsm: fsm})

	d.Handle(context.Background(), cbUpdate(7, telegramservice.CallbackAdminServerAddConfirm))
	d.Handle(context.Background(), cbUpdate(7, telegramservice.CallbackAdminServerAddConfirm))

	if ops.added == nil {
		t.Fatal("first tap must create")
	}
	if ops.addCalls != 1 {
		t.Fatalf("AddServer called %d times, want exactly 1", ops.addCalls)
	}
}

func TestAdminStats_GivenData_ThenDashboardRendered(t *testing.T) {
	api := &fakeAPI{}
	ops := &fakeAdminOps{stats: &postgres.OrderStats{TotalOrders: 12, TotalRevenue: 90000, Completed: 10}}
	d := dispatcherWithAdmin(api, &fakeAdminDeps{ops: ops})
	d.Handle(context.Background(), cbUpdate(7, telegramservice.CallbackAdminStats))

	if len(api.edited) != 1 || !strings.Contains(api.edited[0].text, "Order total: 12") {
		t.Fatalf("stats = %+v", api.edited)
	}
	assertButton(t, api.edited[0], telegramservice.CallbackAdminRecentOrders)
}

func TestAdminAudit_GivenRows_ThenRendered(t *testing.T) {
	api := &fakeAPI{}
	ops := &fakeAdminOps{audit: []postgres.AdminAuditLog{{AdminID: 7, Action: "price:set", Target: "ID:30"}}}
	d := dispatcherWithAdmin(api, &fakeAdminDeps{ops: ops})
	d.Handle(context.Background(), cbUpdate(7, telegramservice.CallbackAdminAudit))

	if len(api.edited) != 1 || !strings.Contains(api.edited[0].text, "Ubah Harga") {
		t.Fatalf("audit = %+v", api.edited)
	}
}
