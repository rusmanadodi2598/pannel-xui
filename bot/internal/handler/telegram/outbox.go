// Package telegramhandler also hosts the messaging outbox helpers.
//
// @file      internal/handler/telegram/outbox.go
// @for       send/edit/answer logging wrappers + user extraction + join prompt.
// @uses      context, strings, log/slog, github.com/go-telegram/bot/models, internal/service/telegram
// @reason    Keeps dispatcher.go under 250 lines (AGENTS.md §1.1); a failed
// Telegram call is logged, never fatal (webhooks are idempotent).
// @author    Dodi Rusmana <rusmanadodi@kentangtechstore.com>
// @layer     handler
// @stability stable
// @since     2026-08-11
package telegramhandler

import (
	"context"
	"strings"

	"github.com/go-telegram/bot/models"
	telegramservice "github.com/kentangtech/bot-order/internal/service/telegram"
)

// editCB edits the callback's message in place when one exists (defensive:
// Telegram may send a callback whose message is inaccessible → Message is nil).
// It answers the callback either way so the client never hangs.
func (d *Dispatcher) editCB(ctx context.Context, cb *models.CallbackQuery, text string, markup models.ReplyMarkup) bool {
	msg := cb.Message.Message
	if msg == nil {
		d.answer(ctx, cb.ID, telegramservice.UnavailableText())
		return false
	}
	d.edit(ctx, msg.Chat.ID, msg.ID, text, markup)
	d.answer(ctx, cb.ID, "")
	return true
}

// sendJoinPrompt shows the mandatory-group prompt (FR-01).
func (d *Dispatcher) sendJoinPrompt(ctx context.Context, upd *models.Update) {
	if upd.Message != nil {
		d.send(ctx, upd.Message.Chat.ID, telegramservice.JoinText(d.groupLink), telegramservice.JoinKeyboard(d.groupLink))
		return
	}
	if upd.CallbackQuery != nil {
		d.answer(ctx, upd.CallbackQuery.ID, "Join grup dulu yuk.")
	}
}

// reject answers a denied update without revealing internal state.
func (d *Dispatcher) reject(ctx context.Context, upd *models.Update, text string) {
	if upd.Message != nil {
		d.send(ctx, upd.Message.Chat.ID, text, nil)
		return
	}
	if upd.CallbackQuery != nil {
		d.answer(ctx, upd.CallbackQuery.ID, text)
	}
}

// send/edit/answer are logging wrappers around the API seam.
func (d *Dispatcher) send(ctx context.Context, chatID int64, text string, markup models.ReplyMarkup) {
	if err := d.api.SendMessage(ctx, chatID, text, "", markup); err != nil {
		d.logger.Error("telegram send failed", "chat_id", chatID, "error", err)
	}
}

func (d *Dispatcher) edit(ctx context.Context, chatID int64, messageID int, text string, markup models.ReplyMarkup) {
	if err := d.api.EditMessageText(ctx, chatID, messageID, text, "", markup); err != nil {
		d.logger.Error("telegram edit failed", "chat_id", chatID, "message_id", messageID, "error", err)
	}
}

func (d *Dispatcher) answer(ctx context.Context, callbackID, text string) {
	if err := d.api.AnswerCallbackQuery(ctx, callbackID, text); err != nil {
		d.logger.Error("telegram answer failed", "callback_id", callbackID, "error", err)
	}
}

// isAdmin reports whether the user is in ADMIN_IDS (gate bypass, PRD FR-01 deviasi).
func (d *Dispatcher) isAdmin(uid int64) bool {
	_, ok := d.adminIDs[uid]
	return ok
}

// UserIDOf extracts the acting user from a message or callback update.
// It is shared with the HTTP worker for the per-user serialization lock.
func UserIDOf(upd *models.Update) int64 {
	switch {
	case upd.Message != nil && upd.Message.From != nil:
		return upd.Message.From.ID
	case upd.CallbackQuery != nil:
		return upd.CallbackQuery.From.ID
	}
	return 0
}

// firstName returns the user's first name or a generic salutation.
func firstName(u *models.User) string {
	if u != nil && strings.TrimSpace(u.FirstName) != "" {
		return u.FirstName
	}
	return "Sobat"
}
