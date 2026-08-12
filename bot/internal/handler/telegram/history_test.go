// Package telegramhandler_test covers the FR-14 history flow.
//
// @file      internal/handler/telegram/history_test.go
// @for       Unit tests: paged list, page navigation, owned detail, noop indicator.
// @uses      testing, context, strings, time, github.com/kentangtech/bot-order/internal/repository/postgres
// @reason    Locks the FR-14 callback contract (list/page/detail/noop) and the
// ownership guard (AGENTS.md §2.1 Given-When-Then naming).
// @author    Dodi Rusmana <rusmanadodi@kentangtechstore.com>
// @layer     handler
// @stability stable
// @since     2026-08-11
package telegramhandler

import (
	"context"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/kentangtech/bot-order/internal/repository/postgres"
	telegramservice "github.com/kentangtech/bot-order/internal/service/telegram"
)

func orderFixture(id int64, orderID string) postgres.Order {
	return postgres.Order{
		ID: id, OrderID: orderID, OrderType: "purchase", UserID: 9,
		Status: "completed", FinalAmount: 7000, AccountEmail: "a@vpn.kt",
		Protocol: "vless", DurationDays: 30, CreatedAt: time.Date(2026, 8, 5, 10, 0, 0, 0, time.UTC),
	}
}

// ordersFixture builds n orders (IDs 1..n) so paginated fakes return rows.
func ordersFixture(n int) []postgres.Order {
	out := make([]postgres.Order, 0, n)
	for i := n; i >= 1; i-- {
		out = append(out, orderFixture(int64(i), fmt.Sprintf("KTS-FIX%02d-VPN", i)))
	}
	return out
}

func TestHistoryFlow_GivenOrders_ThenPagedListRendered(t *testing.T) {
	shop := newFakeShop()
	shop.users.user = &postgres.User{ID: 9, TelegramID: 7}
	shop.history.count = 6
	shop.history.orders = []postgres.Order{
		orderFixture(2, "KTS-AB02-VPN"), orderFixture(1, "KTS-AB01-VPN"),
	}

	api := &fakeAPI{}
	d := dispatcherWithShop(api, shop)
	d.Handle(context.Background(), cbUpdate(7, telegramservice.CallbackHistory))

	if len(api.edited) != 1 {
		t.Fatalf("edited = %+v", api.edited)
	}
	text := api.edited[0].text
	if !strings.Contains(text, "Riwayat Transaksi") || !strings.Contains(text, "Halaman 1 dari 2") ||
		!strings.Contains(text, "KTS-AB02-VPN") || !strings.Contains(text, "Selesai") {
		t.Errorf("list text = %q", text)
	}
	if shop.history.lastLimit != 5 || shop.history.lastOffset != 0 {
		t.Errorf("page 1 query = limit %d offset %d", shop.history.lastLimit, shop.history.lastOffset)
	}
	assertButton(t, api.edited[0], telegramservice.PrefixHistoryPage+"2")
	assertButton(t, api.edited[0], telegramservice.CallbackHistoryNoop)
}

func TestHistoryFlow_GivenEmptyHistory_ThenEmptyTextAndBuyTopup(t *testing.T) {
	shop := newFakeShop()
	shop.users.user = &postgres.User{ID: 9, TelegramID: 7}
	shop.history.count = 0

	api := &fakeAPI{}
	d := dispatcherWithShop(api, shop)
	d.Handle(context.Background(), cbUpdate(7, telegramservice.CallbackHistory))

	if len(api.edited) != 1 || !strings.Contains(api.edited[0].text, "belum punya transaksi") {
		t.Fatalf("edited = %+v", api.edited)
	}
	assertButton(t, api.edited[0], telegramservice.CallbackBuy)
	assertButton(t, api.edited[0], telegramservice.CallbackTopup)
}

func TestHistoryFlow_GivenPageTap_ThenPageQueriedWithOffset(t *testing.T) {
	shop := newFakeShop()
	shop.users.user = &postgres.User{ID: 9, TelegramID: 7}
	shop.history.count = 6
	shop.history.orders = ordersFixture(6)

	api := &fakeAPI{}
	d := dispatcherWithShop(api, shop)
	d.Handle(context.Background(), cbUpdate(7, telegramservice.PrefixHistoryPage+"2"))

	if len(api.edited) != 1 || !strings.Contains(api.edited[0].text, "Halaman 2 dari 2") {
		t.Fatalf("edited = %+v", api.edited)
	}
	if shop.history.lastLimit != 5 || shop.history.lastOffset != 5 {
		t.Errorf("page 2 query = limit %d offset %d", shop.history.lastLimit, shop.history.lastOffset)
	}
	// Last page: no next button, prev + noop present.
	assertButton(t, api.edited[0], telegramservice.PrefixHistoryPage+"1")
	assertButton(t, api.edited[0], telegramservice.CallbackHistoryNoop)
}

func TestHistoryFlow_GivenOutOfRangePage_ThenClampedToLast(t *testing.T) {
	shop := newFakeShop()
	shop.users.user = &postgres.User{ID: 9, TelegramID: 7}
	shop.history.count = 10
	shop.history.orders = ordersFixture(10)

	api := &fakeAPI{}
	d := dispatcherWithShop(api, shop)
	d.Handle(context.Background(), cbUpdate(7, telegramservice.PrefixHistoryPage+"99"))

	if len(api.edited) != 1 || !strings.Contains(api.edited[0].text, "Halaman 2 dari 2") {
		t.Fatalf("clamped page = %+v", api.edited)
	}
	if shop.history.lastOffset != 5 {
		t.Errorf("clamped offset = %d, want 5", shop.history.lastOffset)
	}
}

func TestHistoryFlow_GivenDetailTap_ThenOwnedDetailRendered(t *testing.T) {
	shop := newFakeShop()
	shop.users.user = &postgres.User{ID: 9, TelegramID: 7}
	shop.history.byID = &postgres.Order{
		ID: 3, OrderID: "KTS-AB03-VPN", OrderType: "renewal", UserID: 9,
		Status: "completed", FinalAmount: 4000, AccountEmail: "a@vpn.kt",
		Protocol: "trojan", DurationDays: 15, CreatedAt: time.Now(),
	}

	api := &fakeAPI{}
	d := dispatcherWithShop(api, shop)
	d.Handle(context.Background(), cbUpdate(7, telegramservice.PrefixHistoryDetail+"3"))

	if len(api.edited) != 1 {
		t.Fatalf("edited = %+v", api.edited)
	}
	text := api.edited[0].text
	if !strings.Contains(text, "Detail Transaksi") || !strings.Contains(text, "KTS-AB03-VPN") ||
		!strings.Contains(text, "Perpanjang") || !strings.Contains(text, "TROJAN") ||
		!strings.Contains(text, "15 Hari") {
		t.Errorf("detail text = %q", text)
	}
	assertButton(t, api.edited[0], telegramservice.CallbackHistory)
}

func TestHistoryFlow_GivenForeignOrMissingDetail_ThenNotFound(t *testing.T) {
	shop := newFakeShop()
	shop.users.user = &postgres.User{ID: 9, TelegramID: 7}
	shop.history.err = postgres.ErrOrderNotFound

	api := &fakeAPI{}
	d := dispatcherWithShop(api, shop)
	d.Handle(context.Background(), cbUpdate(7, telegramservice.PrefixHistoryDetail+"99"))

	if len(api.edited) != 0 {
		t.Fatalf("must not edit for foreign order: %+v", api.edited)
	}
	if len(api.answered) != 1 || !strings.Contains(api.answered[0], "Transaksi tidak ditemukan") {
		t.Fatalf("answered = %+v", api.answered)
	}
}

func TestHistoryFlow_GivenRepoError_ThenFriendlyAnswer(t *testing.T) {
	shop := newFakeShop()
	shop.users.user = &postgres.User{ID: 9, TelegramID: 7}
	shop.history.err = errBoom

	api := &fakeAPI{}
	d := dispatcherWithShop(api, shop)
	d.Handle(context.Background(), cbUpdate(7, telegramservice.CallbackHistory))

	if len(api.edited) != 0 {
		t.Fatalf("must not edit on repo error: %+v", api.edited)
	}
	if len(api.answered) != 1 || !strings.Contains(api.answered[0], "Gagal memuat riwayat") {
		t.Fatalf("answered = %+v", api.answered)
	}
}

func TestHistoryFlow_GivenNoopTap_ThenAnsweredWithoutEdit(t *testing.T) {
	api := &fakeAPI{}
	d := dispatcherWithShop(api, newFakeShop())
	d.Handle(context.Background(), cbUpdate(7, telegramservice.CallbackHistoryNoop))

	if len(api.edited) != 0 {
		t.Fatalf("noop must not edit: %+v", api.edited)
	}
	if len(api.answered) != 1 {
		t.Fatalf("noop must answer: %+v", api.answered)
	}
}

func TestHistoryFlow_GivenInvalidPayloads_ThenUnavailable(t *testing.T) {
	for _, data := range []string{
		telegramservice.PrefixHistoryPage + "abc",
		telegramservice.PrefixHistoryPage + "0",
		telegramservice.PrefixHistoryDetail + "xyz",
		telegramservice.PrefixHistoryDetail + "-1",
	} {
		api := &fakeAPI{}
		d := dispatcherWithShop(api, newFakeShop())
		d.Handle(context.Background(), cbUpdate(7, data))
		if len(api.edited) != 0 || len(api.answered) != 1 {
			t.Errorf("%s: edited=%d answered=%d, want 0 edits + 1 answer", data, len(api.edited), len(api.answered))
		}
	}
}
