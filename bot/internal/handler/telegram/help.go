// Package telegramhandler also hosts the static help flow (FR-15).
//
// @file      internal/handler/telegram/help.go
// @for       FR-15 Bantuan: route help:menu/order/topup/disclaimer/tos/info.
// @uses      context, github.com/go-telegram/bot/models, internal/service/telegram
// @reason    Parity with the reference help_handler: pure static id-ID pages,
// no service seam — edit-in-place navigation only (FR-15 AC).
// @author    Dodi Rusmana <rusmanadodi@kentangtechstore.com>
// @layer     handler
// @stability stable
// @since     2026-08-11
package telegramhandler

import (
	"context"

	"github.com/go-telegram/bot/models"
	telegramservice "github.com/kentangtech/bot-order/internal/service/telegram"
)

// handleHelp routes the FR-15 static pages: hub → category → sub-page.
func (d *Dispatcher) handleHelp(ctx context.Context, cb *models.CallbackQuery, data string) {
	switch {
	case data == telegramservice.CallbackHelp:
		d.editCB(ctx, cb, telegramservice.HelpMenuText(), telegramservice.HelpMenuKeyboard())
	case data == telegramservice.CallbackHelpOrder:
		d.editCB(ctx, cb, telegramservice.HelpOrderText(), telegramservice.HelpOrderKeyboard())
	case data == telegramservice.CallbackHelpTopup:
		d.editCB(ctx, cb, telegramservice.HelpTopupText(), telegramservice.HelpTopupKeyboard())
	case data == telegramservice.CallbackHelpDisclaimer:
		d.editCB(ctx, cb, telegramservice.HelpDisclaimerText(), telegramservice.HelpDisclaimerKeyboard())
	case data == telegramservice.CallbackHelpTosAccount:
		d.editCB(ctx, cb, telegramservice.HelpTosAccountText(), telegramservice.HelpTosAccountKeyboard())
	case data == telegramservice.CallbackHelpTosPayment:
		d.editCB(ctx, cb, telegramservice.HelpTosPaymentText(), telegramservice.HelpTosPaymentKeyboard())
	case data == telegramservice.CallbackHelpInfo:
		d.editCB(ctx, cb, telegramservice.HelpInfoText(), telegramservice.HelpInfoKeyboard())
	default:
		d.answer(ctx, cb.ID, telegramservice.UnavailableText())
	}
}
