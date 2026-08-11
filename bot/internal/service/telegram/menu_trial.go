// Package telegram also hosts the trial views (FR-07).
//
// @file      internal/service/telegram/menu_trial.go
// @for       FR-07 keyboards + copy: trial menu, server pick, confirm, success.
// @uses      fmt, github.com/go-telegram/bot/models, internal/repository/postgres
// @reason    Pure presentation — copy is emoji-free (UI copy policy), icons on
// buttons only; keeps the handler layer network-free and testable.
// @author    Dodi Rusmana <rusmanadodi@kentangtechstore.com>
// @layer     service
// @stability experimental
// @since     2026-08-11
package telegram

import (
	"fmt"

	"github.com/go-telegram/bot/models"
	"github.com/kentangtech/bot-order/internal/repository/postgres"
)

// Trial callback data contract (FR-07).
const (
	CallbackTrialBack    = "trial:back"
	PrefixTrialServer    = "trial:server:"
	PrefixTrialConfirm   = "trial:confirm:"
	CallbackTrialPremium = "trial:buy"
)

// TrialMenuText introduces the trial feature with the remaining quota today.
func TrialMenuText(remaining, hours, trafficGB int) string {
	return fmt.Sprintf("Coba VPN Gratis\n━━━━━━━━━━━━━━\n\n"+
		"Kamu bisa mencoba VPN secara gratis selama %d jam (kuota %d GB).\n\n"+
		"Sisa kesempatan trial hari ini: %d\n\n"+
		"Pilih server di bawah untuk mulai trial.", hours, trafficGB, remaining)
}

// TrialLimitText reports the daily quota exhausted (FR-07 AC-1).
func TrialLimitText() string {
	return "Kesempatan trial hari ini sudah habis.\n\nCoba lagi besok, atau beli paket VPN untuk akses penuh."
}

// TrialDisabledText reports the feature is off (config TRIAL_ENABLED=false).
func TrialDisabledText() string {
	return "Fitur trial sedang tidak tersedia."
}

// TrialServersKeyboard lists buyable servers as trial targets (FR-07).
func TrialServersKeyboard(servers []postgres.ServerView) models.ReplyMarkup {
	rows := make([][]models.InlineKeyboardButton, 0, len(servers)+1)
	for _, s := range servers {
		rows = append(rows, []models.InlineKeyboardButton{{
			Text:         fmt.Sprintf("%s %s", flagOrGlobe(s.FlagEmoji), s.Name),
			CallbackData: PrefixTrialServer + fmt.Sprintf("%d", s.ID),
		}})
	}
	rows = append(rows, backRow(CallbackTrialBack, "🏠 Menu Utama"))
	return models.InlineKeyboardMarkup{InlineKeyboard: rows}
}

// TrialConfirmText summarizes the trial before confirmation.
func TrialConfirmText(server postgres.ServerView, hours, trafficGB, ipLimit int) string {
	return fmt.Sprintf("Konfirmasi Trial\n━━━━━━━━━━━━━━\n"+
		"Server: %s %s\n"+
		"Durasi: %d jam\n"+
		"Kuota: %d GB / %d IP\n━━━━━━━━━━━━━━\n\n"+
		"Trial gratis, tidak memotong saldo. Tekan Konfirmasi untuk membuat akun trial.",
		flagOrGlobe(server.FlagEmoji), server.Name, hours, trafficGB, ipLimit)
}

// TrialConfirmKeyboard asks explicit confirmation (FR-07).
func TrialConfirmKeyboard(serverID int64) models.ReplyMarkup {
	return models.InlineKeyboardMarkup{InlineKeyboard: [][]models.InlineKeyboardButton{
		{{Text: "✅ Konfirmasi Trial", CallbackData: PrefixTrialConfirm + fmt.Sprintf("%d", serverID)}},
		backRow(CallbackTrialBack, "⬅️ Kembali"),
	}}
}

// TrialSuccessText reports a created trial account + remaining quota today.
func TrialSuccessText(orderID, email, serverName string, remaining int) string {
	return fmt.Sprintf("Trial Berhasil\n━━━━━━━━━━━━━━\n"+
		"Order ID: %s\n"+
		"Server: %s\n"+
		"Email akun: %s\n"+
		"Masa aktif: 1 jam\n"+
		"Sisa trial hari ini: %d\n━━━━━━━━━━━━━━\n\n"+
		"Detail koneksi tersedia di menu Akun Saya.", orderID, serverName, email, remaining)
}

// TrialSuccessKeyboard offers the premium purchase path (FR-07 AC-2).
func TrialSuccessKeyboard() models.ReplyMarkup {
	return models.InlineKeyboardMarkup{InlineKeyboard: [][]models.InlineKeyboardButton{
		{{Text: "🛒 Beli VPN Premium", CallbackData: CallbackBuy}},
		backRow(CallbackHome, "🏠 Menu Utama"),
	}}
}

// TrialFailedText reports a failed trial creation.
func TrialFailedText() string {
	return "Gagal membuat akun trial.\n\nTerjadi kendala di server. Silakan coba lagi ya."
}
