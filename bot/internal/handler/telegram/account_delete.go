// Package telegramhandler also hosts the account-delete flow (FR-08 AC-4).
//
// @file      internal/handler/telegram/account_delete.go
// @for       FR-08 AC-4: 2-step delete — confirm page then panel+DB removal.
// @uses      context, time, github.com/go-telegram/bot/models, internal/service/telegram
// @reason    Deletion is destructive and ordered: the panel delClient runs
// BEFORE the DB row is removed (the row is a mirror, not the source of truth);
// ownership is re-checked at both steps (parity reference delete flow).
// @author    Dodi Rusmana <rusmanadodi@kentangtechstore.com>
// @layer     handler
// @stability stable
// @since     2026-08-11
package telegramhandler

import (
	"context"
	"fmt"
	"time"

	"github.com/go-telegram/bot/models"
	"github.com/kentangtech/bot-order/internal/domain"
	telegramservice "github.com/kentangtech/bot-order/internal/service/telegram"
)

// accountDelete shows the confirmation page for one owned account (FR-08
// AC-4 step 1). The account is never deleted from this callback.
func (d *Dispatcher) accountDelete(ctx context.Context, cb *models.CallbackQuery, clientID int64) {
	user, err := d.shop.Users.EnsureUser(ctx, cb.From.ID, cb.From.Username, cb.From.FirstName)
	if err != nil {
		d.logger.Error("ensuring user", "user_id", cb.From.ID, "error", err)
		d.answer(ctx, cb.ID, "Gagal memuat akun, coba lagi ya.")
		return
	}
	client, err := d.shop.Clients.GetViewOwned(ctx, clientID, user.ID)
	if err != nil {
		d.logger.Error("getting client for delete", "user_id", user.ID, "client_id", clientID, "error", err)
		d.answer(ctx, cb.ID, "Akun tidak ditemukan.")
		return
	}
	d.editCB(ctx, cb, telegramservice.AccountDeleteText(client, time.Now()),
		telegramservice.AccountDeleteKeyboard(clientID))
}

// accountDeleteConfirm executes the deletion (FR-08 AC-4 step 2): panel
// delClient first, DB row after. A panel failure aborts everything — the
// local row must not vanish while the panel client still exists.
func (d *Dispatcher) accountDeleteConfirm(ctx context.Context, cb *models.CallbackQuery, clientID int64) {
	user, err := d.shop.Users.EnsureUser(ctx, cb.From.ID, cb.From.Username, cb.From.FirstName)
	if err != nil {
		d.logger.Error("ensuring user", "user_id", cb.From.ID, "error", err)
		d.answer(ctx, cb.ID, "Gagal memuat akun, coba lagi ya.")
		return
	}
	client, err := d.shop.Clients.GetViewOwned(ctx, clientID, user.ID)
	if err != nil {
		d.logger.Error("getting client for delete confirm", "user_id", user.ID, "client_id", clientID, "error", err)
		d.answer(ctx, cb.ID, "Akun tidak ditemukan.")
		return
	}
	// The panel client id is the credential the panel tracks per protocol
	// (domain.PanelClientKey, v1.38): UUID for vless/vmess, password for
	// trojan/hysteria, EMAIL for shadowsocks — x-ui keys ss clients by email.
	panelClientID := domain.PanelClientKey(client.Protocol, client.UUID, client.Password, client.Email)
	if panelClientID == "" {
		d.logger.Error("client has no panel credential", "user_id", user.ID, "client_id", clientID)
		d.answer(ctx, cb.ID, "Akun tidak bisa dihapus, hubungi admin ya.")
		return
	}

	// Step A: panel first — a failure aborts before touching the DB row.
	if err := d.shop.Deleter.DeleteClient(ctx, client.ServerID, client.InboundID, panelClientID); err != nil {
		d.logger.Error("panel delete failed", "user_id", user.ID, "client_id", clientID, "error", err)
		d.editCB(ctx, cb, "Gagal menghapus akun di server. Coba lagi nanti ya.", deleteBackKeyboard(clientID))
		return
	}
	// Step B: DB mirror after the panel is done.
	if err := d.shop.Clients.DeleteOwned(ctx, clientID, user.ID); err != nil {
		d.logger.Error("db delete failed after panel delete", "user_id", user.ID, "client_id", clientID, "error", err)
		d.editCB(ctx, cb, "Akun sudah dihapus di server, tapi gagal dicatat lokal. Hubungi admin ya.", backHomeKeyboard())
		return
	}
	// Step C (FR-08 AC-4): record the action in the user's Riwayat (FR-14).
	// Best-effort — the deletion already happened on panel + DB; a failed
	// record is logged, never blocks the success message.
	if err := d.shop.Orders.RecordDeletion(ctx, user.ID, client.ServerID, client.Protocol, client.Email); err != nil {
		d.logger.Error("recording account deletion", "user_id", user.ID, "client_id", clientID, "error", err)
	}
	d.editCB(ctx, cb, telegramservice.AccountDeletedText(client), nil)
}

// deleteBackKeyboard keeps navigation alive on a failed deletion.
func deleteBackKeyboard(clientID int64) models.ReplyMarkup {
	return models.InlineKeyboardMarkup{InlineKeyboard: [][]models.InlineKeyboardButton{
		{{Text: "Coba Lagi", CallbackData: telegramservice.PrefixAccountDelete + fmt.Sprintf("%d", clientID)}},
		{{Text: "⬅️ Akun Saya", CallbackData: telegramservice.CallbackAccount}},
	}}
}
