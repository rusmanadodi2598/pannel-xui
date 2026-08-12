// Package telegram also hosts the admin broadcast + ban/unban views (FR-11, M6).
//
// @file      internal/service/telegram/menu_admin_ban.go
// @for       FR-11 broadcast + ban/unban confirm screens & keyboards.
// @uses      fmt, strings, github.com/go-telegram/bot/models, internal/repository/postgres
// @reason    Split from menu_admin.go for the §1.1 line limit; pure
// presentation so the admin handler stays network-free and testable.
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
	return models.InlineKeyboardMarkup{InlineKeyboard: packRows(
		models.InlineKeyboardButton{Text: "Kirim ke Semua", CallbackData: CallbackAdminBcastSend},
		backBtn(CallbackAdminCancel, "Batal ✕"),
	)}
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
	return models.InlineKeyboardMarkup{InlineKeyboard: packRows(
		models.InlineKeyboardButton{Text: "Konfirmasi Ban", CallbackData: PrefixAdminBanConfirm + fmt.Sprintf("%d", tgID)},
		backBtn(CallbackAdminCancel, "Batal ✕"),
	)}
}

// AdminUnbanConfirmKeyboard asks explicit confirmation with the target id.
func AdminUnbanConfirmKeyboard(tgID int64) models.ReplyMarkup {
	return models.InlineKeyboardMarkup{InlineKeyboard: packRows(
		models.InlineKeyboardButton{Text: "Konfirmasi Unban", CallbackData: PrefixAdminUnbanConfirm + fmt.Sprintf("%d", tgID)},
		backBtn(CallbackAdminCancel, "Batal ✕"),
	)}
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
