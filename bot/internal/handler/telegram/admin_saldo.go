// Package telegramhandler also hosts the admin adjust-saldo flow (FR-11, v1.39).
//
// @file      internal/handler/telegram/admin_saldo.go
// @for       FR-11: admin credit/debit manual — FSM input (id, nominal), confirm, execute.
// @uses      context, errors, fmt, strings, github.com/go-telegram/bot/models,
// internal/repository/postgres, internal/service/telegram, gorm.io/gorm
// @reason    Manual balance corrections (compensation, manual topup/refund) must
// be atomic + ledgered — they reuse ordersvc's Credit/Debit via AdjustBalance
// (split from admin.go for §1.1).
// @author    Dodi Rusmana <rusmanadodi@kentangtechstore.com>
// @layer     handler
// @stability experimental
// @since     2026-08-12
package telegramhandler

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/go-telegram/bot/models"
	"github.com/kentangtech/bot-order/internal/repository/postgres"
	adminsvc "github.com/kentangtech/bot-order/internal/service/admin"
	telegramservice "github.com/kentangtech/bot-order/internal/service/telegram"
	"gorm.io/gorm"
)

// adminSaldoMenu shows the credit/debit picker (FR-11 v1.39).
func (d *Dispatcher) adminSaldoMenu(ctx context.Context, cb *models.CallbackQuery) {
	d.adminClearFSM(ctx, cb.From.ID)
	d.editCB(ctx, cb, telegramservice.AdminSaldoMenuText(), telegramservice.AdminSaldoMenuKeyboard())
}

// adminSaldoArm arms the FSM and asks for the target Telegram id.
func (d *Dispatcher) adminSaldoArm(ctx context.Context, cb *models.CallbackQuery, credit bool) {
	state := "saldo:debit"
	if credit {
		state = "saldo:kredit"
	}
	if err := d.admin.FSM.Set(ctx, cb.From.ID, state); err != nil {
		d.logger.Error("admin fsm set failed", "user_id", cb.From.ID, "error", err)
		d.answer(ctx, cb.ID, "Terjadi kendala, coba lagi ya.")
		return
	}
	d.editCB(ctx, cb, telegramservice.AdminSaldoIDPrompt(credit), nil)
}

// adminSaldoIDInput resolves the typed id, stages it and asks for the nominal.
func (d *Dispatcher) adminSaldoIDInput(ctx context.Context, msg *models.Message, credit bool) {
	tgID, err := parseID64(strings.TrimSpace(msg.Text))
	if err != nil || tgID <= 0 {
		d.send(ctx, msg.Chat.ID, telegramservice.AdminSaldoIDPrompt(credit), nil)
		return
	}
	u, lerr := d.admin.Ops.LookupUser(ctx, tgID)
	if errors.Is(lerr, gorm.ErrRecordNotFound) {
		_ = d.admin.FSM.Clear(ctx, msg.From.ID)
		d.send(ctx, msg.Chat.ID, telegramservice.AdminUserNotFoundText(tgID), nil)
		return
	}
	if lerr != nil {
		d.logger.Error("admin lookup user failed", "user_id", tgID, "error", lerr)
		_ = d.admin.FSM.Clear(ctx, msg.From.ID)
		d.send(ctx, msg.Chat.ID, "Terjadi kendala, coba lagi ya.", nil)
		return
	}
	// Stage the target id so the amount step knows who to adjust.
	state := fmt.Sprintf("saldo:kredit:%d", tgID)
	if !credit {
		state = fmt.Sprintf("saldo:debit:%d", tgID)
	}
	if err := d.admin.FSM.Set(ctx, msg.From.ID, state); err != nil {
		d.logger.Error("admin fsm set failed", "user_id", msg.From.ID, "error", err)
		_ = d.admin.FSM.Clear(ctx, msg.From.ID)
		d.send(ctx, msg.Chat.ID, "Terjadi kendala, coba lagi ya.", nil)
		return
	}
	d.send(ctx, msg.Chat.ID, telegramservice.AdminSaldoAmountPrompt(credit, u), nil)
}

// adminSaldoAmountInput parses the typed nominal and shows the confirmation.
func (d *Dispatcher) adminSaldoAmountInput(ctx context.Context, msg *models.Message, state string) {
	credit := strings.HasPrefix(state, "saldo:kredit:")
	tgID := parseID(strings.TrimPrefix(state, "saldo:kredit:"))
	if !credit {
		tgID = parseID(strings.TrimPrefix(state, "saldo:debit:"))
	}
	if tgID <= 0 {
		_ = d.admin.FSM.Clear(ctx, msg.From.ID)
		d.send(ctx, msg.Chat.ID, "Input saldo tidak valid. Coba lagi ya.", nil)
		return
	}
	amount, err := parseMoney(msg.Text)
	if err != nil {
		// Re-prompt with the real user label (fix review v1.39: nil user showed
		// a generic label on re-ask).
		u, _ := d.admin.Ops.LookupUser(ctx, tgID) // best-effort display label
		d.send(ctx, msg.Chat.ID, telegramservice.AdminSaldoAmountPrompt(credit, u), nil)
		return
	}
	// Arm a confirm-pending state so the confirm callback can verify this exact
	// adjustment was staged before executing (fix review v1.39: idempotence — a
	// double tap / Telegram retry on confirm must never run AdjustBalance twice).
	kind := "debit"
	if credit {
		kind = "kredit"
	}
	pending := fmt.Sprintf("saldo:confirm:%s:%d:%d", kind, tgID, amount.Rupiah())
	if err := d.admin.FSM.Set(ctx, msg.From.ID, pending); err != nil {
		d.logger.Error("admin fsm set failed", "user_id", msg.From.ID, "error", err)
		_ = d.admin.FSM.Clear(ctx, msg.From.ID)
		d.send(ctx, msg.Chat.ID, "Terjadi kendala, coba lagi ya.", nil)
		return
	}
	u, _ := d.admin.Ops.LookupUser(ctx, tgID) // best-effort display label
	d.send(ctx, msg.Chat.ID, telegramservice.AdminSaldoConfirmText(credit, u, tgID, amount),
		telegramservice.AdminSaldoConfirmKeyboard(credit, tgID, amount))
}

// adminSaldoConfirm executes the staged adjustment (FR-11 v1.39).
func (d *Dispatcher) adminSaldoConfirm(ctx context.Context, cb *models.CallbackQuery, credit bool, raw string) {
	parts := strings.Split(raw, ":")
	if len(parts) != 2 {
		d.answer(ctx, cb.ID, telegramservice.UnavailableText())
		return
	}
	tgID, err := parseID64(parts[0])
	if err != nil || tgID <= 0 {
		d.answer(ctx, cb.ID, telegramservice.UnavailableText())
		return
	}
	amount, err := parseMoney(parts[1])
	if err != nil {
		d.answer(ctx, cb.ID, telegramservice.UnavailableText())
		return
	}
	// Idempotence guard: only execute when this exact adjustment was staged by
	// the amount-input step. A stale/double-tapped confirm (FSM already cleared
	// or mismatched payload) is answered without touching the balance.
	kind := "debit"
	if credit {
		kind = "kredit"
	}
	want := fmt.Sprintf("saldo:confirm:%s:%d:%d", kind, tgID, amount.Rupiah())
	state, ok, ferr := d.admin.FSM.Get(ctx, cb.From.ID)
	if ferr != nil {
		d.logger.Error("admin fsm read failed", "user_id", cb.From.ID, "error", ferr)
		d.answer(ctx, cb.ID, "Terjadi kendala, coba lagi ya.")
		return
	}
	if !ok || state != want {
		d.answer(ctx, cb.ID, "Konfirmasi sudah kedaluwarsa. Ulangi dari awal ya.")
		return
	}
	d.adminClearFSM(ctx, cb.From.ID)

	newBalance, err := d.admin.Ops.AdjustBalance(ctx, cb.From.ID, tgID, amount, credit)
	if errors.Is(err, adminsvc.ErrUserNotFound) {
		d.answer(ctx, cb.ID, telegramservice.AdminUserNotFoundText(tgID))
		return
	}
	if errors.Is(err, postgres.ErrInsufficientBalance) {
		d.answer(ctx, cb.ID, "Saldo user tidak mencukupi untuk debit ini.")
		return
	}
	if err != nil {
		d.logger.Error("admin adjust balance failed", "user_id", tgID, "credit", credit, "error", err)
		d.answer(ctx, cb.ID, "Gagal mengubah saldo. Coba lagi ya.")
		return
	}
	d.editCB(ctx, cb, telegramservice.AdminSaldoDoneText(credit, tgID, amount, newBalance),
		telegramservice.AdminMenuKeyboard())
}
