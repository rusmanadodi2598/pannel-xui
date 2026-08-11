// Package telegram hosts webhook registration, gate/ban/rate-limit policies and menu views.
//
// @file      internal/service/telegram/menu.go
// @for       FR-02 main menu keyboard + join/deny/rate-limit copy (pure presentation).
// @uses      github.com/go-telegram/bot/models, fmt
// @reason    Keeps Telegram copy and keyboard layout testable without any network.
// @author    Dodi Rusmana <rusmanadodi@kentangtechstore.com>
// @layer     service
// @stability stable
// @since     2026-08-11
package telegram

import (
	"fmt"
	"strings"

	"github.com/go-telegram/bot/models"
)

// Callback data contract (PRD §7, pola menu:<x>).
const (
	CallbackHome      = "menu:home"
	CallbackBuy       = "buy:menu"
	CallbackRenew     = "renew:menu"
	CallbackAccount   = "account:menu"
	CallbackTopup     = "topup:menu"
	CallbackTrial     = "trial:menu"
	CallbackHistory   = "history:menu"
	CallbackHelp      = "help:menu"
	CallbackGateCheck = "gate:check"
)

// HomeKeyboard renders the FR-02 main menu (2-column inline keyboard).
func HomeKeyboard() models.ReplyMarkup {
	return models.InlineKeyboardMarkup{
		InlineKeyboard: [][]models.InlineKeyboardButton{
			{
				{Text: "🛒 Beli VPN", CallbackData: CallbackBuy},
				{Text: "🔄 Perpanjang", CallbackData: CallbackRenew},
			},
			{
				{Text: "👤 Akun Saya", CallbackData: CallbackAccount},
				{Text: "💳 Top Up", CallbackData: CallbackTopup},
			},
			{
				{Text: "🎁 Trial", CallbackData: CallbackTrial},
				{Text: "📜 Riwayat", CallbackData: CallbackHistory},
			},
			{
				{Text: "ℹ️ Bantuan", CallbackData: CallbackHelp},
			},
		},
	}
}

// HomeText greets the user and lists the menu (FR-02).
// Copy is intentionally emoji-free (icons live only on buttons — product rule).
func HomeText(firstName string) string {
	return fmt.Sprintf(
		"Halo, %s!\n\n"+
			"Selamat datang di KentangTech VPN Bot.\n"+
			"Kelola akun VPN kamu langsung dari sini:\n\n"+
			"• Beli akun VPN baru\n"+
			"• Perpanjang masa aktif\n"+
			"• Cek status akun kamu\n"+
			"• Top up saldo\n"+
			"• Klaim trial gratis\n"+
			"• Riwayat transaksi\n"+
			"• Bantuan\n\n"+
			"Pilih menu di bawah.",
		firstName,
	)
}

// JoinKeyboard offers the group link (when configured) and a re-check button (FR-01).
// The URL button is omitted when the link is empty — a URL-less inline button
// is rejected by the Bot API ("Text buttons are unallowed in the inline keyboard").
func JoinKeyboard(groupLink string) models.ReplyMarkup {
	rows := make([][]models.InlineKeyboardButton, 0, 2)
	if strings.TrimSpace(groupLink) != "" {
		rows = append(rows, []models.InlineKeyboardButton{{Text: "🔗 Join Grup", URL: groupLink}})
	}
	rows = append(rows, []models.InlineKeyboardButton{{Text: "✅ Sudah Join", CallbackData: CallbackGateCheck}})
	return models.InlineKeyboardMarkup{InlineKeyboard: rows}
}

// JoinText explains the mandatory group requirement (FR-01).
func JoinText(groupLink string) string {
	return fmt.Sprintf(
		"Sebelum menggunakan bot ini, kamu wajib join grup kami dulu:\n\n"+
			"%s\n\n"+
			"Setelah join, tekan tombol \"Sudah Join\" di bawah ya.",
		groupLink,
	)
}

// BannedText is shown to banned users (FR-01 AC).
func BannedText() string {
	return "Akses ditolak.\n\nAkun kamu telah diblokir. Hubungi admin jika ini sebuah kesalahan."
}

// RateLimitText is shown when a user exceeds the per-minute limit.
func RateLimitText() string {
	return "Terlalu banyak permintaan. Coba lagi beberapa saat."
}

// UnavailableText answers menu buttons whose feature lands in a later milestone.
func UnavailableText() string {
	return "Fitur ini sedang dalam pengembangan dan akan segera hadir."
}

// HelpHintText is the default reply for unrecognized text.
func HelpHintText() string {
	return "Gunakan /start untuk membuka menu utama."
}
