// Package telegram also hosts the admin panel views (FR-11, M6).
//
// @file      internal/service/telegram/menu_admin.go
// @for       FR-11 admin menu: price management, broadcast, ban/unban keyboards & copy.
// @uses      fmt, strings, github.com/go-telegram/bot/models, internal/domain,
// internal/repository/postgres
// @reason    Pure presentation per UI copy policy (emoji-free body); the admin
//
//	handler stays network-free and testable.
//
// @author    Dodi Rusmana <rusmanadodi@kentangtechstore.com>
// @layer     service
// @stability experimental
// @since     2026-08-11
package telegram

import (
	"fmt"
	"strings"

	"github.com/go-telegram/bot/models"
	"github.com/kentangtech/bot-order/internal/domain"
	"github.com/kentangtech/bot-order/internal/repository/postgres"
)

// Admin callback data contract (FR-11, pola admin:*).
const (
	CallbackAdminMenu        = "admin:menu"
	CallbackAdminBack        = "admin:back"
	CallbackAdminPrice       = "admin:price"
	CallbackAdminReload      = "admin:reload"
	CallbackAdminBroadcast   = "admin:broadcast"
	CallbackAdminBcastSend   = "admin:bcast:send"
	CallbackAdminBcastCancel = "admin:bcast:cancel"
	CallbackAdminBan         = "admin:ban"
	CallbackAdminUnban       = "admin:unban"
	CallbackAdminCancel      = "admin:cancel"

	PrefixAdminPlan         = "admin:plan:"
	PrefixAdminSetPrice     = "admin:setprice:"
	PrefixAdminToggle       = "admin:toggle:"
	PrefixAdminBanConfirm   = "admin:ban:confirm:"
	PrefixAdminUnbanConfirm = "admin:unban:confirm:"
)

// AdminDeniedText answers non-admin taps on admin surfaces.
func AdminDeniedText() string {
	return "Akses ditolak. Panel ini khusus admin."
}

// AdminMenuText introduces the admin panel (FR-11).
func AdminMenuText() string {
	return "Panel Admin\n━━━━━━━━━━━━━━\n\n" +
		"Kelola harga paket, broadcast pengumuman, dan status user.\n\n" +
		"Menu ini hanya dapat diakses ADMIN_IDS."
}

// AdminMenuKeyboard renders the FR-11 admin actions.
func AdminMenuKeyboard() models.ReplyMarkup {
	return models.InlineKeyboardMarkup{InlineKeyboard: [][]models.InlineKeyboardButton{
		{{Text: "👛 Harga", CallbackData: CallbackAdminPrice}},
		{{Text: "📣 Broadcast", CallbackData: CallbackAdminBroadcast}},
		{{Text: "⛔ Ban User", CallbackData: CallbackAdminBan}},
		{{Text: "✅ Unban User", CallbackData: CallbackAdminUnban}},
		backRow(CallbackHome, "🏠 Menu Utama"),
	}}
}

// AdminPriceText introduces the plan list.
func AdminPriceText() string {
	return "Daftar Paket\n━━━━━━━━━━━━━━\n\nPilih paket untuk ubah harga atau toggle status."
}

// AdminPriceKeyboard lists every plan (enabled and disabled); disabled plans
// are prefixed with a marker so the admin sees the sellable state at a glance.
func AdminPriceKeyboard(plans []domain.VpnPlan) models.ReplyMarkup {
	rows := make([][]models.InlineKeyboardButton, 0, len(plans)+2)
	for _, p := range plans {
		label := fmt.Sprintf("%s %d Hari — %s", p.CountryCode, p.Days, p.Price.FormatIDR())
		if !p.Enabled {
			label = "🚫 " + label
		}
		rows = append(rows, []models.InlineKeyboardButton{{
			Text:         label,
			CallbackData: PrefixAdminPlan + p.CountryCode + ":" + fmt.Sprintf("%d", p.Days),
		}})
	}
	rows = append(rows, []models.InlineKeyboardButton{{Text: "🔄 Reload Seed", CallbackData: CallbackAdminReload}})
	rows = append(rows, backRow(CallbackAdminMenu, "⬅️ Kembali"))
	return models.InlineKeyboardMarkup{InlineKeyboard: rows}
}

// AdminPlanDetailText shows one plan with its sellable state.
func AdminPlanDetailText(p domain.VpnPlan) string {
	status := "Aktif"
	if !p.Enabled {
		status = "Nonaktif"
	}
	return fmt.Sprintf("Detail Paket\n━━━━━━━━━━━━━━\n"+
		"Negara: %s (%s)\n"+
		"Durasi: %d Hari\n"+
		"Harga: %s\n"+
		"Status: %s\n━━━━━━━━━━━━━━",
		p.CountryName, p.CountryCode, p.Days, p.Price.FormatIDR(), status)
}

// AdminPlanDetailKeyboard offers set-price and toggle actions.
func AdminPlanDetailKeyboard(country string, days int) models.ReplyMarkup {
	return models.InlineKeyboardMarkup{InlineKeyboard: [][]models.InlineKeyboardButton{
		{{Text: "✏️ Ubah Harga", CallbackData: PrefixAdminSetPrice + country + ":" + fmt.Sprintf("%d", days)}},
		{{Text: "🔁 Toggle Status", CallbackData: PrefixAdminToggle + country + ":" + fmt.Sprintf("%d", days)}},
		backRow(CallbackAdminPrice, "⬅️ Daftar Paket"),
	}}
}

// AdminSetPricePrompt asks for the new price (FSM input, FR-11).
func AdminSetPricePrompt(country string, days int) string {
	return fmt.Sprintf("Ketik harga baru untuk paket %s %d Hari (angka rupiah, contoh: 7500).\n\n"+
		"Ketik /cancel untuk membatalkan.", country, days)
}

// AdminPriceSavedText confirms a price update.
func AdminPriceSavedText(country string, days int, price domain.Money) string {
	return fmt.Sprintf("Harga paket %s %d Hari diubah menjadi %s.",
		country, days, price.FormatIDR())
}

// AdminPlanToggledText confirms an enabled/disabled toggle.
func AdminPlanToggledText(country string, days int, enabled bool) string {
	state := "diaktifkan"
	if !enabled {
		state = "dinonaktifkan"
	}
	return fmt.Sprintf("Paket %s %d Hari %s. Harga live langsung terupdate.", country, days, state)
}

// AdminReloadText confirms a pricing reseed.
func AdminReloadText() string {
	return "Pricing di-reload dari file seed. Daftar harga live sudah diperbarui."
}

// AdminBroadcastPromptText asks for the announcement text (FSM input, FR-11).
func AdminBroadcastPromptText() string {
	return "Ketik pesan pengumuman yang akan dikirim ke semua user.\n\n" +
		"Ketik /cancel untuk membatalkan."
}

// AdminBroadcastPreviewText quotes the message before the final confirm.
func AdminBroadcastPreviewText(text string) string {
	return fmt.Sprintf("Pratinjau Broadcast\n━━━━━━━━━━━━━━\n\n%s\n\n━━━━━━━━━━━━━━\n\nKirim pesan ini ke semua user?", text)
}

// AdminBroadcastConfirmKeyboard asks explicit confirmation.
func AdminBroadcastConfirmKeyboard() models.ReplyMarkup {
	return models.InlineKeyboardMarkup{InlineKeyboard: [][]models.InlineKeyboardButton{
		{{Text: "📨 Kirim ke Semua", CallbackData: CallbackAdminBcastSend}},
		backRow(CallbackAdminCancel, "❌ Batalkan"),
	}}
}

// AdminBroadcastStartText confirms the broadcast started (chunked delivery).
func AdminBroadcastStartText(total int) string {
	return fmt.Sprintf("Broadcast sedang dikirim ke %d user (100 pesan per 6 detik).\n"+
		"Hasil akhir akan dilaporkan di sini.", total)
}

// BroadcastDoneText reports the final broadcast outcome.
func BroadcastDoneText(sent, failed int) string {
	return fmt.Sprintf("Broadcast selesai\n━━━━━━━━━━━━━━\nTerkirim: %d\nGagal: %d", sent, failed)
}

// AdminBanPromptText asks for the target Telegram id (FSM input, FR-11).
func AdminBanPromptText() string {
	return "Ketik Telegram ID user yang ingin di-ban (angka).\n\nKetik /cancel untuk membatalkan."
}

// AdminUnbanPromptText asks for the target Telegram id (FSM input, FR-11).
func AdminUnbanPromptText() string {
	return "Ketik Telegram ID user yang ingin di-unban (angka).\n\nKetik /cancel untuk membatalkan."
}

// AdminUserNotFoundText reports an unregistered target (ban still applies).
func AdminUserNotFoundText(tgID int64) string {
	return fmt.Sprintf("User %d belum terdaftar di bot. Aksi tetap bisa diproses.", tgID)
}

// AdminBanConfirmText summarizes the ban before confirmation.
func AdminBanConfirmText(u *postgres.User, tgID int64) string {
	who := adminUserLabel(u)
	return fmt.Sprintf("Konfirmasi Ban\n━━━━━━━━━━━━━━\n%s (ID %d)\n\n"+
		"User ini tidak bisa lagi memakai bot. Lanjutkan?", who, tgID)
}

// AdminUnbanConfirmText summarizes the unban before confirmation.
func AdminUnbanConfirmText(u *postgres.User, tgID int64) string {
	who := adminUserLabel(u)
	return fmt.Sprintf("Konfirmasi Unban\n━━━━━━━━━━━━━━\n%s (ID %d)\n\n"+
		"User ini akan dapat memakai bot kembali. Lanjutkan?", who, tgID)
}

// AdminBanConfirmKeyboard asks explicit confirmation with the target id.
func AdminBanConfirmKeyboard(tgID int64) models.ReplyMarkup {
	return models.InlineKeyboardMarkup{InlineKeyboard: [][]models.InlineKeyboardButton{
		{{Text: "⛔ Konfirmasi Ban", CallbackData: PrefixAdminBanConfirm + fmt.Sprintf("%d", tgID)}},
		backRow(CallbackAdminCancel, "❌ Batal"),
	}}
}

// AdminUnbanConfirmKeyboard asks explicit confirmation with the target id.
func AdminUnbanConfirmKeyboard(tgID int64) models.ReplyMarkup {
	return models.InlineKeyboardMarkup{InlineKeyboard: [][]models.InlineKeyboardButton{
		{{Text: "✅ Konfirmasi Unban", CallbackData: PrefixAdminUnbanConfirm + fmt.Sprintf("%d", tgID)}},
		backRow(CallbackAdminCancel, "❌ Batal"),
	}}
}

// AdminBanDoneText confirms a completed ban.
func AdminBanDoneText(tgID int64) string {
	return fmt.Sprintf("User %d sudah di-ban. Marker gate + flag DB diperbarui.", tgID)
}

// AdminUnbanDoneText confirms a completed unban.
func AdminUnbanDoneText(tgID int64) string {
	return fmt.Sprintf("User %d sudah di-unban. Akses dipulihkan.", tgID)
}

// AdminInputCancelledText confirms an aborted admin input flow.
func AdminInputCancelledText() string {
	return "Input dibatalkan. Kembali ke panel admin."
}

// adminUserLabel renders a human label for the confirm screens.
func adminUserLabel(u *postgres.User) string {
	if u == nil {
		return "User tidak terdaftar"
	}
	if strings.TrimSpace(u.FirstName) != "" {
		return strings.TrimSpace(u.FirstName)
	}
	if u.Username != "" {
		return "@" + u.Username
	}
	return "User " + fmt.Sprintf("%d", u.TelegramID)
}
