// Package telegram tests the FR-14 history views (pure presentation).
//
// @file      internal/service/telegram/menu_history_test.go
// @for       Unit tests: list text, detail text, pagination/empty keyboards.
// @uses      testing, time, github.com/kentangtech/bot-order/internal/repository/postgres
// @reason    Locks the FR-14 copy + keyboard layout (labels, 5/page navigation,
// noop indicator) without any network (AGENTS.md §2.1).
// @author    Dodi Rusmana <rusmanadodi@kentangtechstore.com>
// @layer     service
// @stability stable
// @since     2026-08-11
package telegram

import (
	"strings"
	"testing"
	"time"

	"github.com/go-telegram/bot/models"
	"github.com/kentangtech/bot-order/internal/domain"
	"github.com/kentangtech/bot-order/internal/repository/postgres"
)

func historyOrderFixture(orderID, orderType, status string, amount int64) postgres.Order {
	return postgres.Order{
		OrderID: orderID, OrderType: orderType, Status: status,
		FinalAmount: domain.Money(amount), AccountEmail: "a@vpn.kt", Protocol: "vless",
		DurationDays: 30, CreatedAt: time.Date(2026, 8, 5, 10, 0, 0, 0, time.UTC),
	}
}

func TestHistoryListText_GivenOrders_ThenPagedListRendered(t *testing.T) {
	orders := []postgres.Order{
		historyOrderFixture("KTS-AB01-VPN", "purchase", "completed", 7000),
		historyOrderFixture("KTS-AB02-VPN", "trial", "failed", 0),
	}
	text := HistoryListText(orders, 1, 2)

	for _, want := range []string{
		"Riwayat Transaksi", "Halaman 1 dari 2",
		"KTS-AB01-VPN", "Beli VPN", "Selesai", "Rp 7.000", "05 Aug 2026",
		"KTS-AB02-VPN", "Trial", "Gagal", "Gratis",
	} {
		if !strings.Contains(text, want) {
			t.Errorf("list text missing %q:\n%s", want, text)
		}
	}
}

func TestHistoryDetailText_GivenOrder_ThenFieldsRendered(t *testing.T) {
	order := historyOrderFixture("KTS-AB03-VPN", "renewal", "completed", 4000)
	order.Protocol = "trojan"
	text := HistoryDetailText(order)

	for _, want := range []string{
		"Detail Transaksi", "KTS-AB03-VPN", "Perpanjang", "Selesai",
		"Rp 4.000", "a@vpn.kt", "TROJAN", "30 Hari",
	} {
		if !strings.Contains(text, want) {
			t.Errorf("detail text missing %q:\n%s", want, text)
		}
	}
}

func TestHistoryListText_GivenDeletionOrder_ThenHapusAkunLabelAndDash(t *testing.T) {
	orders := []postgres.Order{historyOrderFixture("KTS-AB05-VPN", "deletion", "completed", 0)}
	text := HistoryListText(orders, 1, 1)
	for _, want := range []string{"Hapus Akun", "—"} {
		if !strings.Contains(text, want) {
			t.Errorf("deletion list missing %q in:\n%s", want, text)
		}
	}
	if strings.Contains(text, "Gratis") {
		t.Errorf("deletion amount must not render 'Gratis':\n%s", text)
	}
}

func TestHistoryDetailText_GivenDeletionOrder_ThenLabelAndDash(t *testing.T) {
	order := historyOrderFixture("KTS-AB06-VPN", "deletion", "completed", 0)
	text := HistoryDetailText(order)
	if !strings.Contains(text, "Hapus Akun") || !strings.Contains(text, "—") {
		t.Errorf("deletion detail missing label/dash in:\n%s", text)
	}
}

func TestHistoryDetailText_GivenRefunded_ThenLabelMapped(t *testing.T) {
	order := historyOrderFixture("KTS-AB04-VPN", "topup", "refunded", 25000)
	if text := HistoryDetailText(order); !strings.Contains(text, "Dikembalikan") {
		t.Errorf("refunded label missing:\n%s", text)
	}
}

func TestHistoryListKeyboard_GivenFirstPage_ThenNextAndNoopWithoutPrev(t *testing.T) {
	markup := HistoryListKeyboard(1, 3)
	kb := markup.(models.InlineKeyboardMarkup)
	data := keyboardCallbacks(kb)

	if !contains(data, PrefixHistoryPage+"2") || !contains(data, CallbackHistoryNoop) {
		t.Errorf("first page missing next/noop: %v", data)
	}
	if contains(data, PrefixHistoryPage+"0") {
		t.Errorf("first page must not render prev: %v", data)
	}
	if !contains(data, CallbackHome) {
		t.Errorf("first page missing home: %v", data)
	}
}

func TestHistoryListKeyboard_GivenLastPage_ThenPrevWithoutNext(t *testing.T) {
	markup := HistoryListKeyboard(3, 3)
	kb := markup.(models.InlineKeyboardMarkup)
	data := keyboardCallbacks(kb)

	if !contains(data, PrefixHistoryPage+"2") || !contains(data, CallbackHistoryNoop) {
		t.Errorf("last page missing prev/noop: %v", data)
	}
	if contains(data, PrefixHistoryPage+"4") {
		t.Errorf("last page must not render next: %v", data)
	}
}

func TestHistoryEmptyKeyboard_GivenNone_ThenBuyTopupAndHome(t *testing.T) {
	markup := HistoryEmptyKeyboard()
	kb := markup.(models.InlineKeyboardMarkup)
	data := keyboardCallbacks(kb)

	for _, want := range []string{CallbackBuy, CallbackTopup, CallbackHome} {
		if !contains(data, want) {
			t.Errorf("empty keyboard missing %q: %v", want, data)
		}
	}
}

func TestHistoryEmptyText_GivenNone_ThenBuyTopupHint(t *testing.T) {
	text := HistoryEmptyText()
	if !strings.Contains(text, "belum punya transaksi") {
		t.Errorf("empty text = %q", text)
	}
}

// keyboardCallbacks flattens an inline keyboard into its callback data list.
func keyboardCallbacks(kb models.InlineKeyboardMarkup) []string {
	var out []string
	for _, row := range kb.InlineKeyboard {
		for _, btn := range row {
			out = append(out, btn.CallbackData)
		}
	}
	return out
}

func contains(list []string, s string) bool {
	for _, v := range list {
		if v == s {
			return true
		}
	}
	return false
}
