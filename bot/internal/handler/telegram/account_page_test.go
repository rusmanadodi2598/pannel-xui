// Package telegramhandler_test covers the FR-08 AC-1 account pagination.
//
// @file      internal/handler/telegram/account_page_test.go
// @for       Page navigation: account:menu page 1, account:page:N, clamp, noop.
// @uses      testing, context, strings, github.com/go-telegram/bot/models,
// internal/repository/postgres, internal/service/telegram
// @reason    Locks the 5/page FR-08 contract (parity reference account:page:{n})
// and the clamp + noop behaviour shared with FR-14 history (AGENTS.md §2.1).
// @author    Dodi Rusmana <rusmanadodi@kentangtechstore.com>
// @layer     handler
// @stability stable
// @since     2026-08-11
package telegramhandler

import (
	"context"
	"strings"
	"testing"

	"github.com/kentangtech/bot-order/internal/repository/postgres"
	telegramservice "github.com/kentangtech/bot-order/internal/service/telegram"
)

// clientViewFixture builds n client rows with distinct emails.
func clientViewFixture(n int) []postgres.ClientView {
	rows := make([]postgres.ClientView, 0, n)
	for i := 1; i <= n; i++ {
		rows = append(rows, postgres.ClientView{
			VPNClient:  postgres.VPNClient{ID: int64(i), Email: "u" + string(rune('0'+i)) + "@vpn.kt", Protocol: "vless"},
			ServerName: "ID-01", CountryCode: "ID", FlagEmoji: "🇮🇩",
		})
	}
	return rows
}

func TestAccountMenu_GivenSixClients_ThenPageOneRendered(t *testing.T) {
	shop := newFakeShop()
	shop.users.user = &postgres.User{ID: 9, TelegramID: 7}
	shop.clients.list = clientViewFixture(6)
	shop.clients.count = 6

	api := &fakeAPI{}
	d := dispatcherWithShop(api, shop)
	d.Handle(context.Background(), cbUpdate(7, telegramservice.CallbackAccount))

	if len(api.edited) != 1 {
		t.Fatalf("edited = %d, want 1", len(api.edited))
	}
	text := api.edited[0].text
	if !strings.Contains(text, "Halaman 1 dari 2") {
		t.Errorf("text missing page header: %q", text)
	}
	// Page 1 shows clients 1-5 (offset 0), not client 6.
	if !strings.Contains(text, "u1@vpn.kt") || !strings.Contains(text, "u5@vpn.kt") ||
		strings.Contains(text, "u6@vpn.kt") {
		t.Errorf("page 1 content wrong: %q", text)
	}
	// Pager: no prev, indicator 1/2, next to page 2.
	assertButton(t, api.edited[0], telegramservice.PrefixAccountPage+"2")
}

func TestAccountPage_GivenSecondPage_ThenOffsetFive(t *testing.T) {
	shop := newFakeShop()
	shop.users.user = &postgres.User{ID: 9, TelegramID: 7}
	shop.clients.list = clientViewFixture(6)
	shop.clients.count = 6

	api := &fakeAPI{}
	d := dispatcherWithShop(api, shop)
	d.Handle(context.Background(), cbUpdate(7, telegramservice.PrefixAccountPage+"2"))

	if len(api.edited) != 1 {
		t.Fatalf("edited = %d, want 1", len(api.edited))
	}
	if shop.clients.lastOffset != 5 {
		t.Errorf("offset = %d, want 5", shop.clients.lastOffset)
	}
	if !strings.Contains(api.edited[0].text, "u6@vpn.kt") ||
		strings.Contains(api.edited[0].text, "u1@vpn.kt") {
		t.Errorf("page 2 content wrong: %q", api.edited[0].text)
	}
}

func TestAccountPage_GivenOutOfRange_ThenClampedToLast(t *testing.T) {
	shop := newFakeShop()
	shop.users.user = &postgres.User{ID: 9, TelegramID: 7}
	shop.clients.list = clientViewFixture(6)
	shop.clients.count = 6

	api := &fakeAPI{}
	d := dispatcherWithShop(api, shop)
	d.Handle(context.Background(), cbUpdate(7, telegramservice.PrefixAccountPage+"99"))

	if len(api.edited) != 1 {
		t.Fatalf("edited = %d, want 1", len(api.edited))
	}
	if !strings.Contains(api.edited[0].text, "Halaman 2 dari 2") {
		t.Errorf("clamped page header missing: %q", api.edited[0].text)
	}
	if shop.clients.lastOffset != 5 {
		t.Errorf("clamped offset = %d, want 5", shop.clients.lastOffset)
	}
}

func TestAccountPage_GivenNoopIndicator_ThenEmptyAnswerNoEdit(t *testing.T) {
	api := &fakeAPI{}
	d := dispatcherWithShop(api, newFakeShop())
	d.Handle(context.Background(), cbUpdate(7, telegramservice.CallbackAccountNoop))

	if len(api.answered) != 1 {
		t.Fatalf("answered = %+v, want 1 (noop answered, never edited)", api.answered)
	}
	// FR-02 AC: the page indicator must answer EMPTY (no "under development"
	// toast) — the fake records "cbID:text", so an empty answer ends with ":".
	if api.answered[0] != "cb-1:" {
		t.Errorf("noop answer = %q, want empty (cb-1:)", api.answered[0])
	}
	if len(api.edited) != 0 {
		t.Fatalf("noop must not edit: %+v", api.edited)
	}
}

func TestAccountMenu_GivenNoClients_ThenEmptyPromptAndBuyShortcut(t *testing.T) {
	shop := newFakeShop()
	shop.users.user = &postgres.User{ID: 9, TelegramID: 7}
	shop.clients.count = 0

	api := &fakeAPI{}
	d := dispatcherWithShop(api, shop)
	d.Handle(context.Background(), cbUpdate(7, telegramservice.CallbackAccount))

	if len(api.edited) != 1 {
		t.Fatalf("edited = %d, want 1", len(api.edited))
	}
	if !strings.Contains(api.edited[0].text, "belum punya akun") {
		t.Errorf("empty text = %q", api.edited[0].text)
	}
	assertButton(t, api.edited[0], telegramservice.CallbackBuy)
}

func TestAccountPage_GivenBadPage_ThenUnavailable(t *testing.T) {
	api := &fakeAPI{}
	d := dispatcherWithShop(api, newFakeShop())
	d.Handle(context.Background(), cbUpdate(7, telegramservice.PrefixAccountPage+"abc"))

	if len(api.answered) != 1 || !strings.Contains(api.answered[0], telegramservice.UnavailableText()) {
		t.Fatalf("answered = %+v, want unavailable", api.answered)
	}
}
