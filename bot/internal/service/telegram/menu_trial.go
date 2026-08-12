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
	"strings"

	"github.com/go-telegram/bot/models"
	"github.com/kentangtech/bot-order/internal/repository/postgres"
)

// Trial callback data contract (FR-07).
const (
	CallbackTrialBack    = "trial:back"
	PrefixTrialServer    = "trial:server:"
	PrefixTrialInbound   = "trial:inbound:"
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

// TrialServersKeyboard lists buyable servers as trial targets (FR-07,
// 2-1-2-1 zigzag).
func TrialServersKeyboard(servers []postgres.ServerView) models.ReplyMarkup {
	buttons := make([]models.InlineKeyboardButton, 0, len(servers)+1)
	for _, s := range servers {
		buttons = append(buttons, models.InlineKeyboardButton{
			Text:         fmt.Sprintf("%s %s", flagOrGlobe(s.FlagEmoji), s.Name),
			CallbackData: PrefixTrialServer + fmt.Sprintf("%d", s.ID),
		})
	}
	buttons = append(buttons, backBtn(CallbackTrialBack, "🏠 Menu Utama"))
	return models.InlineKeyboardMarkup{InlineKeyboard: packRows(buttons...)}
}

// TrialInboundListText introduces the protocol picker (FR-07).
func TrialInboundListText(server postgres.ServerView) string {
	return fmt.Sprintf("Pilih protocol trial di %s %s:\n\n"+
		"Daftar ini diambil langsung dari panel — pilih yang kamu mau.",
		flagOrGlobe(server.FlagEmoji), server.Name)
}

// TrialConfirmText summarizes the trial before confirmation (branded banner
// v1.44).
func TrialConfirmText(server postgres.ServerView, hours, trafficGB, ipLimit int, protocol string) string {
	if protocol == "" {
		protocol = "VLESS"
	}
	return fmt.Sprintf(BrandHeader()+"\n\nKonfirmasi Trial\n━━━━━━━━━━━━━━\n"+
		"Server: %s %s\n"+
		"Protocol: %s\n"+
		"Durasi: %d jam\n"+
		"Kuota: %d GB / %d IP\n━━━━━━━━━━━━━━\n\n"+
		"Trial gratis, tidak memotong saldo. Tekan Konfirmasi untuk membuat akun trial.",
		flagOrGlobe(server.FlagEmoji), server.Name, strings.ToUpper(protocol), hours, trafficGB, ipLimit)
}

// TrialConfirmKeyboard asks explicit confirmation (FR-07), carrying the
// chosen server + inbound.
func TrialConfirmKeyboard(serverID, inboundID int64) models.ReplyMarkup {
	return models.InlineKeyboardMarkup{InlineKeyboard: packRows(
		models.InlineKeyboardButton{Text: "Konfirmasi Trial", CallbackData: PrefixTrialConfirm +
			fmt.Sprintf("%d:%d", serverID, inboundID)},
		backBtn(CallbackTrialBack, "⬅️ Kembali"),
	)}
}

// TrialSuccessText reports a created trial account + remaining quota today
// (branded banner v1.43). The import URL is intentionally NOT rendered here
// (v1.36) — it lives only in the .txt export, which the keyboard below
// offers right after.
func TrialSuccessText(orderID, email, serverName string, remaining int) string {
	return fmt.Sprintf(BrandHeader()+"\n\nTrial Berhasil\n━━━━━━━━━━━━━━\n"+
		"Order ID: %s\n"+
		"Server: %s\n"+
		"Email akun: %s\n"+
		"Masa aktif: 1 jam\n"+
		"Sisa trial hari ini: %d\n━━━━━━━━━━━━━━\n\n%s",
		orderID, serverName, email, remaining, exportHintText)
}

// TrialSuccessKeyboard offers the v2Ray config view, export + the premium
// purchase path (FR-07 AC-2, 2-1-2-1 zigzag). "Beli VPN Premium" is a
// forward navigation (Next) button — icon allowed.
func TrialSuccessKeyboard(clientID int64) models.ReplyMarkup {
	return models.InlineKeyboardMarkup{InlineKeyboard: packRows(
		models.InlineKeyboardButton{Text: "Config V2Ray", CallbackData: PrefixAccountConfig + fmt.Sprintf("%d", clientID)},
		models.InlineKeyboardButton{Text: "Ekspor .txt", CallbackData: PrefixAccountExport + fmt.Sprintf("%d", clientID)},
		models.InlineKeyboardButton{Text: "Beli VPN Premium ➡️", CallbackData: CallbackBuy},
		backBtn(CallbackHome, "🏠 Menu Utama"),
	)}
}

// TrialFailedText reports a failed trial creation (branded banner v1.44).
func TrialFailedText() string {
	return BrandHeader() + "\n\nGagal membuat akun trial.\n\nTerjadi kendala di server. Silakan coba lagi ya."
}
