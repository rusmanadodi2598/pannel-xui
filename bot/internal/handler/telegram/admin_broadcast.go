// Package telegramhandler also hosts the admin broadcast flow (FR-11).
//
// @file      internal/handler/telegram/admin_broadcast.go
// @for       FR-11 broadcast: FSM prompt → staged preview → confirm + start.
// @uses      context, errors, strings, github.com/go-telegram/bot/models,
// internal/service/admin, internal/service/telegram
// @reason    Split from admin_user.go to respect the 250-line limit (AGENTS.md
// §1.1); the broadcast free-text FSM mirrors the topup custom-input pattern.
// @author    Dodi Rusmana <rusmanadodi@kentangtechstore.com>
// @layer     handler
// @stability experimental
// @since     2026-08-17
package telegramhandler

import (
	"context"
	"errors"
	"strings"

	"github.com/go-telegram/bot/models"
	adminsvc "github.com/kentangtech/bot-order/internal/service/admin"
	telegramservice "github.com/kentangtech/bot-order/internal/service/telegram"
)

// adminBroadcastPrompt arms the broadcast FSM and asks for the message.
func (d *Dispatcher) adminBroadcastPrompt(ctx context.Context, cb *models.CallbackQuery) {
	if err := d.admin.FSM.Set(ctx, cb.From.ID, "broadcast"); err != nil {
		d.logger.Error("admin fsm set failed", "user_id", cb.From.ID, "error", err)
		d.answer(ctx, cb.ID, "Terjadi kendala, coba lagi ya.")
		return
	}
	d.editCB(ctx, cb, telegramservice.AdminBroadcastPromptText(), nil)
}

// adminBcastSend reads the staged broadcast text from the FSM and starts it.
func (d *Dispatcher) adminBcastSend(ctx context.Context, cb *models.CallbackQuery) {
	state, ok, err := d.admin.FSM.Get(ctx, cb.From.ID)
	if err != nil || !ok || !strings.HasPrefix(state, "broadcast:") {
		d.answer(ctx, cb.ID, "Tidak ada pesan broadcast yang disiapkan. Coba lagi ya.")
		return
	}
	d.adminClearFSM(ctx, cb.From.ID)

	text := strings.TrimPrefix(state, "broadcast:")
	total, err := d.admin.Ops.Broadcast(ctx, cb.From.ID, text)
	if errors.Is(err, adminsvc.ErrBroadcastRunning) {
		d.editCB(ctx, cb, "Broadcast lain sedang berjalan. Tunggu selesai dulu ya.", nil)
		return
	}
	if err != nil {
		d.logger.Error("admin broadcast failed", "error", err)
		d.editCB(ctx, cb, "Gagal memulai broadcast. Coba lagi ya.", nil)
		return
	}
	if total == 0 {
		d.editCB(ctx, cb, "Belum ada user terdaftar. Broadcast dibatalkan.", nil)
		return
	}
	d.editCB(ctx, cb, telegramservice.AdminBroadcastStartText(total), nil)
}

// adminBroadcastInput stages the message and shows the preview + confirm.
func (d *Dispatcher) adminBroadcastInput(ctx context.Context, msg *models.Message) {
	text := strings.TrimSpace(msg.Text)
	if text == "" {
		d.send(ctx, msg.Chat.ID, telegramservice.AdminBroadcastPromptText(), nil)
		return
	}
	if err := d.admin.FSM.Set(ctx, msg.From.ID, "broadcast:"+text); err != nil {
		d.logger.Error("admin fsm set failed", "user_id", msg.From.ID, "error", err)
		d.send(ctx, msg.Chat.ID, "Terjadi kendala, coba lagi ya.", nil)
		return
	}
	d.send(ctx, msg.Chat.ID, telegramservice.AdminBroadcastPreviewText(text),
		telegramservice.AdminBroadcastConfirmKeyboard())
}
