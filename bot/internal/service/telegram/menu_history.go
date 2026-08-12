// Package telegram also hosts the order history views (FR-14).
//
// @file      internal/service/telegram/menu_history.go
// @for       FR-14 Riwayat: paged order list, detail per order, pagination keyboard.
// @uses      fmt, strings, github.com/go-telegram/bot/models, internal/domain, internal/repository/postgres
// @reason    Parity with the reference history_handler: menu:history, page:n,
// detail:id with pagination 5/page and non-action page indicator (FR-02 AC).
// @author    Dodi Rusmana <rusmanadodi@kentangtechstore.com>
// @layer     service
// @stability stable
// @since     2026-08-11
package telegram

import (
	"fmt"
	"strings"

	"github.com/go-telegram/bot/models"
	"github.com/kentangtech/bot-order/internal/domain"
	"github.com/kentangtech/bot-order/internal/repository/postgres"
)

// History callback data contract (FR-14, parity history_handler reference).
const (
	PrefixHistory       = "history:"
	PrefixHistoryPage   = "history:page:"
	PrefixHistoryDetail = "history:detail:"
	CallbackHistoryNoop = "history:noop" // page indicator — answered, never edited
)

// HistoryEmptyText is shown when the user has no orders yet (FR-14 AC).
func HistoryEmptyText() string {
	return "Kamu belum punya transaksi.\n\nSilakan beli paket VPN atau top up saldo dulu ya."
}

// HistoryEmptyKeyboard offers buy/topup shortcuts when the list is empty.
func HistoryEmptyKeyboard() models.ReplyMarkup {
	return models.InlineKeyboardMarkup{InlineKeyboard: [][]models.InlineKeyboardButton{
		{
			{Text: "Beli VPN", CallbackData: CallbackBuy},
			{Text: "Top Up", CallbackData: CallbackTopup},
		},
		backRow(CallbackHome, "🏠 Menu Utama"),
	}}
}

// HistoryListText renders one page of the user's orders, newest first
// (FR-14 list, 5/page). Copy is emoji-free; icons live only on nav buttons.
func HistoryListText(orders []postgres.Order, page, totalPages int) string {
	var b strings.Builder
	b.WriteString(fmt.Sprintf("Riwayat Transaksi\n━━━━━━━━━━━━━━\nHalaman %d dari %d\n", page, totalPages))
	for i, o := range orders {
		b.WriteString(fmt.Sprintf("\n%d. %s\n", i+1, o.OrderID))
		b.WriteString(fmt.Sprintf("   %s • %s\n", orderTypeLabel(o.OrderType), orderStatusLabel(o.Status)))
		b.WriteString(fmt.Sprintf("   %s\n", historyAmountLabel(o)))
		b.WriteString(fmt.Sprintf("   %s\n", o.CreatedAt.Format("02 Jan 2006 15:04")))
	}
	b.WriteString("\n━━━━━━━━━━━━━━\nPilih transaksi untuk melihat detail.")
	return b.String()
}

// HistoryListKeyboard renders pagination (prev/next), a non-action page
// indicator and home. The indicator answers without editing (FR-02 AC).
func HistoryListKeyboard(page, totalPages int) models.ReplyMarkup {
	return models.InlineKeyboardMarkup{InlineKeyboard: [][]models.InlineKeyboardButton{
		pagerRow(PrefixHistoryPage, CallbackHistoryNoop, page, totalPages),
		backRow(CallbackHome, "🏠 Menu Utama"),
	}}
}

// pagerRow builds the shared pagination nav: prev + non-action page indicator
// + next. Used by the FR-08 account list and FR-14 history list (parity
// reference `pagination` helper — prev/next buttons carry the prefix).
func pagerRow(prefix, noopCb string, page, totalPages int) []models.InlineKeyboardButton {
	nav := make([]models.InlineKeyboardButton, 0, 3)
	if page > 1 {
		nav = append(nav, models.InlineKeyboardButton{
			Text: "⬅️ Sebelumnya", CallbackData: prefix + fmt.Sprintf("%d", page-1),
		})
	}
	nav = append(nav, models.InlineKeyboardButton{
		Text: fmt.Sprintf("%d/%d", page, totalPages), CallbackData: noopCb,
	})
	if page < totalPages {
		nav = append(nav, models.InlineKeyboardButton{
			Text: "Berikutnya ➡️", CallbackData: prefix + fmt.Sprintf("%d", page+1),
		})
	}
	return nav
}

// HistoryDetailText renders one order's full details (FR-14 detail AC).
func HistoryDetailText(o postgres.Order) string {
	var b strings.Builder
	b.WriteString("Detail Transaksi\n━━━━━━━━━━━━━━\n")
	b.WriteString(fmt.Sprintf("Order ID: %s\n", o.OrderID))
	b.WriteString(fmt.Sprintf("Tipe: %s\n", orderTypeLabel(o.OrderType)))
	b.WriteString(fmt.Sprintf("Status: %s\n", orderStatusLabel(o.Status)))
	b.WriteString(fmt.Sprintf("Tanggal: %s\n", o.CreatedAt.Format("02 Jan 2006 15:04")))
	b.WriteString(fmt.Sprintf("Nominal: %s\n", historyAmountLabel(o)))
	if strings.TrimSpace(o.AccountEmail) != "" {
		b.WriteString(fmt.Sprintf("Akun: %s\n", o.AccountEmail))
	}
	if strings.TrimSpace(o.Protocol) != "" {
		b.WriteString(fmt.Sprintf("Protocol: %s\n", strings.ToUpper(o.Protocol)))
	}
	if o.DurationDays > 0 {
		b.WriteString(fmt.Sprintf("Durasi: %d Hari\n", o.DurationDays))
	}
	if o.CompletedAt != nil {
		b.WriteString(fmt.Sprintf("Selesai: %s\n", o.CompletedAt.Format("02 Jan 2006 15:04")))
	}
	b.WriteString("━━━━━━━━━━━━━━")
	return b.String()
}

// HistoryDetailKeyboard goes back to the list and home.
func HistoryDetailKeyboard() models.ReplyMarkup {
	return models.InlineKeyboardMarkup{InlineKeyboard: packRows(
		backBtn(CallbackHistory, "⬅️ Kembali"),
		backBtn(CallbackHome, "🏠 Menu Utama"),
	)}
}

// orderTypeLabel maps the DB order type to the user-facing Indonesian label.
func orderTypeLabel(raw string) string {
	switch domain.OrderType(raw) {
	case domain.OrderTypePurchase:
		return "Beli VPN"
	case domain.OrderTypeRenewal:
		return "Perpanjang"
	case domain.OrderTypeTopup:
		return "Top Up"
	case domain.OrderTypeTrial:
		return "Trial"
	case domain.OrderTypeDeletion:
		return "Hapus Akun"
	default:
		return strings.ToUpper(raw)
	}
}

// orderStatusLabel maps the DB order status to the user-facing label
// (FR-14 AC: pending/processing/completed/failed/cancelled/refunded labeled).
func orderStatusLabel(raw string) string {
	switch domain.OrderStatus(raw) {
	case domain.OrderPending:
		return "Menunggu"
	case domain.OrderProcessing:
		return "Diproses"
	case domain.OrderCompleted:
		return "Selesai"
	case domain.OrderFailed:
		return "Gagal"
	case domain.OrderCancelled:
		return "Dibatalkan"
	case domain.OrderRefunded:
		return "Dikembalikan"
	default:
		return strings.ToUpper(raw)
	}
}

// historyAmountLabel hides the zero-amount display for free orders (trial)
// and shows an em-dash for account deletions, which carry no monetary value
// (FR-08 AC-4 record, FR-14 view).
func historyAmountLabel(o postgres.Order) string {
	if domain.OrderType(o.OrderType) == domain.OrderTypeDeletion {
		return "—"
	}
	if o.FinalAmount.IsZero() {
		return "Gratis"
	}
	return o.FinalAmount.FormatIDR()
}
