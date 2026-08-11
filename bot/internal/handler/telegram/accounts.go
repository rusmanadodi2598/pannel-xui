// Package telegramhandler also hosts the account list view.
//
// @file      internal/handler/telegram/accounts.go
// @for       FR-08 subset (M4): list the user's VPN accounts with status.
// @uses      context, time, github.com/go-telegram/bot/models, internal/service/telegram
// @reason    Detail/config actions land in M6; M4 ships the read-only list.
// @author    Dodi Rusmana <rusmanadodi@kentangtechstore.com>
// @layer     handler
// @stability stable
// @since     2026-08-11
package telegramhandler

import (
	"context"
	"time"

	"github.com/go-telegram/bot/models"
	telegramservice "github.com/kentangtech/bot-order/internal/service/telegram"
)

// handleAccount renders the user's accounts (FR-08 list, no pagination yet).
func (d *Dispatcher) handleAccount(ctx context.Context, cb *models.CallbackQuery) {
	user, err := d.shop.Users.EnsureUser(ctx, cb.From.ID, cb.From.Username, cb.From.FirstName)
	if err != nil {
		d.logger.Error("ensuring user", "user_id", cb.From.ID, "error", err)
		d.answer(ctx, cb.ID, "Gagal memuat akun, coba lagi ya.")
		return
	}
	clients, err := d.shop.Clients.ListByUser(ctx, user.ID, 5)
	if err != nil {
		d.logger.Error("listing clients", "user_id", user.ID, "error", err)
		d.answer(ctx, cb.ID, "Gagal memuat daftar akun, coba lagi ya.")
		return
	}

	text := telegramservice.AccountsText(clients, time.Now())
	keyboard := models.InlineKeyboardMarkup{InlineKeyboard: [][]models.InlineKeyboardButton{
		{{Text: "🏠 Menu Utama", CallbackData: telegramservice.CallbackHome}},
	}}
	d.editCB(ctx, cb, text, keyboard)
}
