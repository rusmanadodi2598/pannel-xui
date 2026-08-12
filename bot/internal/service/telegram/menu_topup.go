// Package telegram also hosts the topup views (FR-06, M5 partial).
//
// @file      internal/service/telegram/menu_topup.go
// @for       Topup quick-pick/custom keyboards + clean copy (no emoji in text).
// @uses      fmt, github.com/go-telegram/bot/models, internal/domain, internal/service/topup
// @reason    Menus are product-final; only the payment API call is deferred (§15.7).
// @author    Dodi Rusmana <rusmanadodi@kentangtechstore.com>
// @layer     service
// @stability experimental
// @since     2026-08-11
package telegram

import (
	"fmt"

	"github.com/go-telegram/bot/models"
	"github.com/kentangtech/bot-order/internal/domain"
	topupsvc "github.com/kentangtech/bot-order/internal/service/topup"
)

// Topup callback contract (FR-06).
const (
	CallbackTopupBack  = "topup:back"
	PrefixTopupAmount  = "topup:amount:"
	PrefixTopupCustom  = "topup:custom"
	PrefixTopupConfirm = "topup:confirm:"
)

// TopupQuickPickValues are the quick-pick NET nominals (FR-06).
var TopupQuickPickValues = []domain.Money{10000, 25000, 50000, 100000, 200000, 500000}

// TopupKeyboard renders quick-pick + custom input (FR-06).
func TopupKeyboard() models.ReplyMarkup {
	rows := make([][]models.InlineKeyboardButton, 0, len(TopupQuickPickValues)/2+3)
	for i := 0; i < len(TopupQuickPickValues); i += 2 {
		row := []models.InlineKeyboardButton{
			{Text: TopupQuickPickValues[i].FormatIDR(), CallbackData: PrefixTopupAmount + fmt.Sprintf("%d", TopupQuickPickValues[i].Rupiah())},
		}
		if i+1 < len(TopupQuickPickValues) {
			row = append(row, models.InlineKeyboardButton{
				Text: TopupQuickPickValues[i+1].FormatIDR(), CallbackData: PrefixTopupAmount + fmt.Sprintf("%d", TopupQuickPickValues[i+1].Rupiah()),
			})
		}
		rows = append(rows, row)
	}
	rows = append(rows, []models.InlineKeyboardButton{{Text: "Nominal Lain", CallbackData: PrefixTopupCustom}})
	rows = append(rows, backRow(CallbackTopupBack, "🏠 Menu Utama"))
	return models.InlineKeyboardMarkup{InlineKeyboard: rows}
}

// TopupConfirmKeyboard asks explicit confirmation with the chosen net amount.
// Back re-renders the amount picker (topup:menu), not the home menu.
func TopupConfirmKeyboard(net domain.Money) models.ReplyMarkup {
	return models.InlineKeyboardMarkup{InlineKeyboard: packRows(
		models.InlineKeyboardButton{Text: "Konfirmasi Top Up", CallbackData: PrefixTopupConfirm + fmt.Sprintf("%d", net.Rupiah())},
		backBtn(CallbackTopup, "⬅️ Kembali"),
	)}
}

// TopupCustomKeyboard is shown while waiting for a typed nominal; the cancel
// button is a shortcut besides the /cancel command (Cancel → icon allowed).
func TopupCustomKeyboard() models.ReplyMarkup {
	return models.InlineKeyboardMarkup{InlineKeyboard: [][]models.InlineKeyboardButton{
		backRow(CallbackTopupBack, "Batalkan ✕"),
	}}
}

// TopupMenuText introduces the nominal step (FR-06).
func TopupMenuText(balance domain.Money) string {
	return fmt.Sprintf("Pilih nominal saldo yang ingin kamu isi:\n\n"+
		"Saldo saat ini: %s\n\n"+
		"Nominal yang kamu pilih adalah saldo BERSIH yang diterima (fee sudah dihitung otomatis).",
		balance.FormatIDR())
}

// TopupCustomPrompt asks for a custom amount (FR-06 AC: min/max + /cancel).
func TopupCustomPrompt(min, max domain.Money) string {
	return fmt.Sprintf("Masukkan nominal saldo yang diinginkan (angka saja):\n\n"+
		"Minimal %s, maksimal %s.\n\nKetik /cancel untuk membatalkan.",
		min.FormatIDR(), max.FormatIDR())
}

// TopupSummaryText renders the confirmation summary (FR-06, branded v1.43).
func TopupSummaryText(q topupsvc.Quote, balance domain.Money) string {
	return fmt.Sprintf(BrandHeader()+"\n\nRingkasan Top Up\n━━━━━━━━━━━━━━\n"+
		"Saldo diterima: %s\n"+
		"Total bayar: %s\n"+
		"Biaya layanan: %s (efektif %.3f%%)\n"+
		"Saldo saat ini: %s\n━━━━━━━━━━━━━━\n\n"+
		"Tekan Konfirmasi Top Up untuk membuat QR pembayaran.",
		q.Net.FormatIDR(), q.Gross.FormatIDR(), q.TotalFee.FormatIDR(), q.FeePercent*100, balance.FormatIDR())
}

// TopupAPIUnavailableText explains the deferred API clearly (product decision).
func TopupAPIUnavailableText() string {
	return "Fitur top up sedang dalam persiapan.\n\n" +
		"Menu pembayaran sudah aktif, namun kanal pembayaran sedang di-upgrade. Coba lagi dalam beberapa saat ya."
}

// TopupPaymentText renders a created QRIS payment (branded v1.43; unreachable
// until the rewritten API ships — kept so the success path is product-final).
func TopupPaymentText(p *topupsvc.PaymentResult) string {
	return fmt.Sprintf(BrandHeader()+"\n\nPembayaran QRIS dibuat\n━━━━━━━━━━━━━━\n"+
		"Total bayar: %s\n"+
		"Berlaku sampai: %s\n\n"+
		"Scan QR di bawah untuk menyelesaikan pembayaran.",
		p.Amount.FormatIDR(), p.ExpiresAt.Format("02 Jan 2006 15:04"))
}

// TopupCancelledText confirms a cancelled custom input (/cancel or back).
func TopupCancelledText() string {
	return "Input dibatalkan. Kembali ke menu utama."
}
