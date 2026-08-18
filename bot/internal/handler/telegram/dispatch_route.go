// Package telegramhandler also hosts the update routing.
//
// @file      internal/handler/telegram/dispatch_route.go
// @for       Update routing: /start, /cancel, FSM-aware text, callback dispatch.
// @uses      context, strings, github.com/go-telegram/bot/models, internal/service/telegram
// @reason    Split from dispatcher.go to respect the 250-line limit (AGENTS.md
// §1.1); the middleware chain stays in dispatcher.go while routing lives here.
// @author    Dodi Rusmana <rusmanadodi@kentangtechstore.com>
// @layer     handler
// @stability stable
// @since     2026-08-17
package telegramhandler

import (
	"context"
	"strings"

	"github.com/go-telegram/bot/models"
	telegramservice "github.com/kentangtech/bot-order/internal/service/telegram"
)

// route dispatches by update kind: /start, /cancel, FSM-aware text or callback.
func (d *Dispatcher) route(ctx context.Context, upd *models.Update) {
	switch {
	case upd.Message != nil && upd.Message.Text == "/start":
		d.handleStart(ctx, upd.Message)
	case upd.Message != nil && upd.Message.Text == "/cancel":
		d.handleCancel(ctx, upd.Message)
	case upd.Message != nil && upd.Message.Text == "/trial":
		// FR-07: /trial opens the trial menu directly.
		if d.shop != nil && d.shop.Trials != nil && d.shop.TrialLm != nil && d.shop.TrialLm.Enabled() {
			d.trialMenuSend(ctx, upd.Message)
			return
		}
		d.send(ctx, upd.Message.Chat.ID, telegramservice.TrialDisabledText(), nil)
	case upd.Message != nil && upd.Message.Text == "/admin":
		// FR-11: /admin opens the admin panel (ADMIN_IDS only).
		d.handleAdmin(ctx, upd.Message)
	case upd.Message != nil && upd.Message.Text != "":
		// FSM-aware: pending topup custom-input or admin input consumes the text.
		if d.topupHandleText(ctx, upd.Message) {
			return
		}
		if d.adminHandleText(ctx, upd.Message) {
			return
		}
		// Unrecognized text only answers in private chats — never in a group,
		// where the bot would pollute shared conversation with hints.
		if upd.Message.Chat.Type == "private" {
			d.send(ctx, upd.Message.Chat.ID, telegramservice.HelpHintText(), nil)
		}
	case upd.CallbackQuery != nil:
		d.handleCallback(ctx, upd.CallbackQuery)
	default:
		d.logger.Debug("unhandled update", "update_id", upd.ID)
	}
}

// handleCancel aborts any pending flow (FR-06 custom / FR-11 admin input) and shows home.
func (d *Dispatcher) handleCancel(ctx context.Context, msg *models.Message) {
	d.topupClearFSM(ctx, msg.From.ID)
	d.adminClearFSM(ctx, msg.From.ID)
	d.send(ctx, msg.Chat.ID, telegramservice.TopupCancelledText(), telegramservice.HomeKeyboard())
}

// handleStart answers FR-01 onboarding with the FR-02 main menu.
// Plain text, no parse mode: usernames may contain markdown-special characters.
// Any pending flow (topup custom input) is aborted — /start always restarts clean.
func (d *Dispatcher) handleStart(ctx context.Context, msg *models.Message) {
	d.topupClearFSM(ctx, msg.From.ID)
	d.adminClearFSM(ctx, msg.From.ID)
	d.send(ctx, msg.Chat.ID, telegramservice.HomeText(firstName(msg.From)), telegramservice.HomeKeyboard())
}

// handleCallback routes inline button taps.
func (d *Dispatcher) handleCallback(ctx context.Context, cb *models.CallbackQuery) {
	switch {
	case cb.Data == telegramservice.CallbackGateCheck:
		d.handleGateCheck(ctx, cb)
	case cb.Data == telegramservice.CallbackHome:
		// Main menu re-renders in place (FR-02 AC).
		d.editHome(ctx, cb)
	case strings.HasPrefix(cb.Data, "topup:"):
		if d.topup != nil {
			d.routeTopup(ctx, cb)
			return
		}
		d.answer(ctx, cb.ID, telegramservice.UnavailableText())
	case strings.HasPrefix(cb.Data, "buy:") || strings.HasPrefix(cb.Data, "renew:") ||
		cb.Data == telegramservice.CallbackAccount || strings.HasPrefix(cb.Data, "account:") ||
		strings.HasPrefix(cb.Data, "history:"):
		if d.shop != nil {
			d.routeShop(ctx, cb)
			return
		}
		d.answer(ctx, cb.ID, telegramservice.UnavailableText())
	case strings.HasPrefix(cb.Data, "trial:"):
		// FR-07: trial flow (nil-safe — lands when the trial service is wired).
		if d.shop != nil && d.shop.Trials != nil && d.shop.TrialLm != nil {
			d.routeTrial(ctx, cb)
			return
		}
		d.answer(ctx, cb.ID, telegramservice.UnavailableText())
	case strings.HasPrefix(cb.Data, telegramservice.PrefixHelp):
		// FR-15: static help/ToS content — no service seam (edit-in-place).
		d.handleHelp(ctx, cb, cb.Data)
	case strings.HasPrefix(cb.Data, "admin:"):
		// FR-11: admin panel (nil-safe; non-admins are denied inside routeAdmin).
		if d.admin != nil {
			d.routeAdmin(ctx, cb)
			return
		}
		d.answer(ctx, cb.ID, telegramservice.UnavailableText())
	default:
		// Known menu buttons whose feature lands in later milestones answer noop.
		d.answer(ctx, cb.ID, telegramservice.UnavailableText())
	}
}

// handleGateCheck re-verifies group membership without the cache (FR-01).
func (d *Dispatcher) handleGateCheck(ctx context.Context, cb *models.CallbackQuery) {
	if d.gate.Enabled() {
		switch d.gate.CheckFresh(ctx, cb.From.ID) {
		case telegramservice.GateAllowed:
			d.editHome(ctx, cb)
			return
		case telegramservice.GateDenied:
			d.answer(ctx, cb.ID, "Kamu belum join grup.")
			return
		default:
			d.answer(ctx, cb.ID, "Gagal verifikasi, coba lagi ya.")
			return
		}
	}
	d.editHome(ctx, cb)
}

// editHome re-renders the main menu in place (FR-02 AC: edit, not resend).
func (d *Dispatcher) editHome(ctx context.Context, cb *models.CallbackQuery) {
	msg := cb.Message.Message
	if msg == nil {
		d.answer(ctx, cb.ID, telegramservice.UnavailableText())
		return
	}
	d.edit(ctx, msg.Chat.ID, msg.ID, telegramservice.HomeText(firstName(&cb.From)), telegramservice.HomeKeyboard())
	d.answer(ctx, cb.ID, "")
}
