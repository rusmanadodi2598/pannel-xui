// Package telegram also hosts the account-delete confirmation views (FR-08 AC-4).
//
// @file      internal/service/telegram/menu_account_delete.go
// @for       FR-08 AC-4: 2-step delete — confirmation text + Ya/Batal keyboard.
// @uses      fmt, strings, github.com/go-telegram/bot/models, internal/repository/postgres
// @reason    Delete is destructive: the confirm page must show the account and
// an explicit irreversible warning before the account:delete_confirm:{id} tap
// (parity reference account_handler delete flow).
// @author    Dodi Rusmana <rusmanadodi@kentangtechstore.com>
// @layer     service
// @stability stable
// @since     2026-08-11
package telegram

import (
	"fmt"
	"strings"
	"time"

	"github.com/go-telegram/bot/models"
	"github.com/kentangtech/bot-order/internal/repository/postgres"
)

// AccountDeleteText is the FR-08 AC-4 confirmation page: which account is
// being deleted + an explicit irreversible warning.
func AccountDeleteText(c postgres.ClientView, now time.Time) string {
	status := "Aktif"
	if c.IsExpired || (c.ExpiresAt != nil && !c.ExpiresAt.After(now)) {
		status = "Expired"
	}
	var b strings.Builder
	b.WriteString("Konfirmasi Hapus Akun\n━━━━━━━━━━━━━━\n")
	b.WriteString(fmt.Sprintf("Server: %s %s\n", flagOrGlobe(c.FlagEmoji), c.ServerName))
	b.WriteString(fmt.Sprintf("Protocol: %s\n", strings.ToUpper(c.Protocol)))
	b.WriteString(fmt.Sprintf("Email: %s\n", c.Email))
	if c.ExpiresAt != nil {
		b.WriteString(fmt.Sprintf("Masa aktif: %s (%s)\n", c.ExpiresAt.Format("02 Jan 2006"), status))
	}
	b.WriteString("━━━━━━━━━━━━━━\n\nPERINGATAN: Akun yang dihapus tidak bisa dikembalikan!\n\n" +
		"Config link, traffic, dan riwayat akun ini akan hilang.")
	return b.String()
}

// AccountDeleteKeyboard asks explicit confirmation: "Ya, Hapus" executes the
// deletion, "Batal" returns to the account detail (FR-08 AC-4 2-step).
func AccountDeleteKeyboard(clientID int64) models.ReplyMarkup {
	return models.InlineKeyboardMarkup{InlineKeyboard: packRows(
		models.InlineKeyboardButton{Text: "Ya, Hapus", CallbackData: PrefixAccountDeleteConfirm + fmt.Sprintf("%d", clientID)},
		backBtn(PrefixAccountView+fmt.Sprintf("%d", clientID), "Batal"),
	)}
}

// AccountDeletedText is shown after a successful deletion (FR-08 AC-4).
func AccountDeletedText(c postgres.ClientView) string {
	return fmt.Sprintf("Akun dihapus\n━━━━━━━━━━━━━━\nEmail: %s\nServer: %s\n\n"+
		"Akun sudah dihapus dari server dan tidak bisa dipulihkan.",
		c.Email, c.ServerName)
}
