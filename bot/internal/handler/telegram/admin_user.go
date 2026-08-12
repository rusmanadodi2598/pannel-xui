// Package telegramhandler also hosts the admin broadcast & user flows (FR-11).
//
// @file      internal/handler/telegram/admin_user.go
// @for       FR-11 broadcast + ban/unban: FSM free-text inputs and confirms.
// @uses      context, errors, strings, github.com/go-telegram/bot/models,
// internal/repository/postgres, internal/service/telegram, internal/service/admin
// @reason    Split from admin.go to respect the 250-line limit (§1.1); the
//
//	admin free-text FSM mirrors the topup custom-input pattern (M5).
//
// @author    Dodi Rusmana <rusmanadodi@kentangtechstore.com>
// @layer     handler
// @stability experimental
// @since     2026-08-11
package telegramhandler

import (
	"context"
	"errors"
	"strings"

	"github.com/go-telegram/bot/models"
	adminsvc "github.com/kentangtech/bot-order/internal/service/admin"
	telegramservice "github.com/kentangtech/bot-order/internal/service/telegram"
	"gorm.io/gorm"
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

// adminBanPrompt arms the ban FSM and asks for the target id.
func (d *Dispatcher) adminBanPrompt(ctx context.Context, cb *models.CallbackQuery) {
	if err := d.admin.FSM.Set(ctx, cb.From.ID, "ban"); err != nil {
		d.logger.Error("admin fsm set failed", "user_id", cb.From.ID, "error", err)
		d.answer(ctx, cb.ID, "Terjadi kendala, coba lagi ya.")
		return
	}
	d.editCB(ctx, cb, telegramservice.AdminBanPromptText(), nil)
}

// adminUnbanPrompt arms the unban FSM and asks for the target id.
func (d *Dispatcher) adminUnbanPrompt(ctx context.Context, cb *models.CallbackQuery) {
	if err := d.admin.FSM.Set(ctx, cb.From.ID, "unban"); err != nil {
		d.logger.Error("admin fsm set failed", "user_id", cb.From.ID, "error", err)
		d.answer(ctx, cb.ID, "Terjadi kendala, coba lagi ya.")
		return
	}
	d.editCB(ctx, cb, telegramservice.AdminUnbanPromptText(), nil)
}

// adminBanConfirm executes the ban and confirms.
func (d *Dispatcher) adminBanConfirm(ctx context.Context, cb *models.CallbackQuery, raw string) {
	tgID, err := parseID64(strings.TrimSpace(raw))
	if err != nil || tgID <= 0 {
		d.answer(ctx, cb.ID, "ID user tidak valid.")
		return
	}
	d.adminClearFSM(ctx, cb.From.ID)
	if err := d.admin.Ops.BanUser(ctx, cb.From.ID, tgID); err != nil {
		d.logger.Error("admin ban failed", "user_id", tgID, "error", err)
		d.editCB(ctx, cb, "Gagal memproses ban. Coba lagi ya.", nil)
		return
	}
	d.editCB(ctx, cb, telegramservice.AdminBanDoneText(tgID), telegramservice.AdminMenuKeyboard())
}

// adminUnbanConfirm executes the unban and confirms.
func (d *Dispatcher) adminUnbanConfirm(ctx context.Context, cb *models.CallbackQuery, raw string) {
	tgID, err := parseID64(strings.TrimSpace(raw))
	if err != nil || tgID <= 0 {
		d.answer(ctx, cb.ID, "ID user tidak valid.")
		return
	}
	d.adminClearFSM(ctx, cb.From.ID)
	if err := d.admin.Ops.UnbanUser(ctx, cb.From.ID, tgID); err != nil {
		d.logger.Error("admin unban failed", "user_id", tgID, "error", err)
		d.editCB(ctx, cb, "Gagal memproses unban. Coba lagi ya.", nil)
		return
	}
	d.editCB(ctx, cb, telegramservice.AdminUnbanDoneText(tgID), telegramservice.AdminMenuKeyboard())
}

// adminHandleText consumes free text while an admin FSM is armed. It reports
// whether the message was consumed (FSM pending). /cancel is routed separately.
func (d *Dispatcher) adminHandleText(ctx context.Context, msg *models.Message) bool {
	if d.admin == nil || d.admin.Ops == nil || d.admin.FSM == nil {
		return false
	}
	state, ok, err := d.admin.FSM.Get(ctx, msg.From.ID)
	if err != nil {
		d.logger.Error("admin fsm read failed", "user_id", msg.From.ID, "error", err)
		return false
	}
	if !ok {
		return false
	}
	// Only an admin can be mid-admin-input; a stale marker is cleared.
	if !d.isAdmin(msg.From.ID) {
		_ = d.admin.FSM.Clear(ctx, msg.From.ID)
		return true
	}

	switch {
	case strings.HasPrefix(state, "price:"):
		d.adminPriceInput(ctx, msg, strings.TrimPrefix(state, "price:"))
	case state == "broadcast":
		d.adminBroadcastInput(ctx, msg)
	case state == "ban":
		d.adminBanInput(ctx, msg, true)
	case state == "unban":
		d.adminBanInput(ctx, msg, false)
	case state == "saldo:kredit":
		d.adminSaldoIDInput(ctx, msg, true)
	case state == "saldo:debit":
		d.adminSaldoIDInput(ctx, msg, false)
	case strings.HasPrefix(state, "saldo:kredit:") || strings.HasPrefix(state, "saldo:debit:"):
		d.adminSaldoAmountInput(ctx, msg, state)
	case strings.HasPrefix(state, "srvadd:"):
		d.adminServerAddInput(ctx, msg, state)
	default:
		_ = d.admin.FSM.Clear(ctx, msg.From.ID)
	}
	return true
}

// adminPriceInput parses the typed price, updates the plan and confirms.
func (d *Dispatcher) adminPriceInput(ctx context.Context, msg *models.Message, raw string) {
	country, days, ok := parsePlanData(raw)
	if !ok {
		_ = d.admin.FSM.Clear(ctx, msg.From.ID)
		d.send(ctx, msg.Chat.ID, telegramservice.AdminMenuText(), telegramservice.AdminMenuKeyboard())
		return
	}
	price, err := parseMoney(msg.Text)
	if err != nil {
		d.send(ctx, msg.Chat.ID, telegramservice.AdminSetPricePrompt(country, days), nil)
		return
	}
	_ = d.admin.FSM.Clear(ctx, msg.From.ID)
	if err := d.admin.Ops.SetPrice(ctx, msg.From.ID, country, days, price); err != nil {
		d.logger.Error("admin set price failed", "country", country, "days", days, "error", err)
		d.send(ctx, msg.Chat.ID, "Gagal mengubah harga. Coba lagi ya.", telegramservice.AdminMenuKeyboard())
		return
	}
	d.send(ctx, msg.Chat.ID, telegramservice.AdminPriceSavedText(country, days, price),
		telegramservice.AdminPlanDetailKeyboard(country, days))
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

// adminBanInput resolves the typed id and shows the confirm screen.
func (d *Dispatcher) adminBanInput(ctx context.Context, msg *models.Message, ban bool) {
	tgID, err := parseID64(strings.TrimSpace(msg.Text))
	if err != nil || tgID <= 0 {
		prompt := telegramservice.AdminBanPromptText()
		if !ban {
			prompt = telegramservice.AdminUnbanPromptText()
		}
		d.send(ctx, msg.Chat.ID, prompt, nil)
		return
	}
	_ = d.admin.FSM.Clear(ctx, msg.From.ID)

	u, lerr := d.admin.Ops.LookupUser(ctx, tgID)
	if lerr != nil && !errors.Is(lerr, gorm.ErrRecordNotFound) {
		d.logger.Error("admin lookup user failed", "user_id", tgID, "error", lerr)
	}
	if errors.Is(lerr, gorm.ErrRecordNotFound) {
		d.send(ctx, msg.Chat.ID, telegramservice.AdminUserNotFoundText(tgID), nil)
		u = nil
	}
	if ban {
		d.send(ctx, msg.Chat.ID, telegramservice.AdminBanConfirmText(u, tgID),
			telegramservice.AdminBanConfirmKeyboard(tgID))
		return
	}
	d.send(ctx, msg.Chat.ID, telegramservice.AdminUnbanConfirmText(u, tgID),
		telegramservice.AdminUnbanConfirmKeyboard(tgID))
}

// adminClearToMenu cancels any admin input and returns to the panel menu.
func (d *Dispatcher) adminClearToMenu(ctx context.Context, cb *models.CallbackQuery) {
	d.adminClearFSM(ctx, cb.From.ID)
	d.editCB(ctx, cb, telegramservice.AdminMenuText(), telegramservice.AdminMenuKeyboard())
}

// adminClearFSM best-effort clears the admin input marker.
func (d *Dispatcher) adminClearFSM(ctx context.Context, userID int64) {
	if d.admin == nil || d.admin.FSM == nil {
		return
	}
	if err := d.admin.FSM.Clear(ctx, userID); err != nil {
		d.logger.Error("admin fsm clear failed", "user_id", userID, "error", err)
	}
}
