// Package telegramhandler also hosts the trial flow (FR-07, M6).
//
// @file      internal/handler/telegram/trial.go
// @for       FR-07: trial menu → server pick → confirm (daily limit claim) → execute.
// @uses      context, strings, github.com/go-telegram/bot/models, internal/repository/postgres,
// internal/service/order, internal/service/trial
// @reason    Trial abuse is the top FR-07 risk — the limit is re-checked at the
// menu, at server pick AND claimed atomically at confirm (PRD FR-07 AC-1).
// @author    Dodi Rusmana <rusmanadodi@kentangtechstore.com>
// @layer     handler
// @stability experimental
// @since     2026-08-11
package telegramhandler

import (
	"context"
	"strings"

	"github.com/go-telegram/bot/models"
	telegramservice "github.com/kentangtech/bot-order/internal/service/telegram"
)

// routeTrial dispatches trial callbacks (FR-07): menu → server → inbound →
// confirm → execute (protocol picked from the panel, same as FR-03).
func (d *Dispatcher) routeTrial(ctx context.Context, cb *models.CallbackQuery) {
	data := cb.Data
	switch {
	case data == telegramservice.CallbackTrial:
		d.trialMenu(ctx, cb)
	case strings.HasPrefix(data, telegramservice.PrefixTrialServer):
		d.trialServer(ctx, cb, parseID(strings.TrimPrefix(data, telegramservice.PrefixTrialServer)))
	case strings.HasPrefix(data, telegramservice.PrefixTrialInbound):
		serverID, inboundID, ok := parseTrialInbound(strings.TrimPrefix(data, telegramservice.PrefixTrialInbound))
		if !ok {
			d.answer(ctx, cb.ID, telegramservice.UnavailableText())
			return
		}
		d.trialConfirm(ctx, cb, serverID, inboundID)
	case strings.HasPrefix(data, telegramservice.PrefixTrialConfirm):
		serverID, inboundID, ok := parseTrialInbound(strings.TrimPrefix(data, telegramservice.PrefixTrialConfirm))
		if !ok {
			d.answer(ctx, cb.ID, telegramservice.UnavailableText())
			return
		}
		d.trialExecute(ctx, cb, serverID, inboundID)
	case data == telegramservice.CallbackTrialPremium:
		d.handleBuy(ctx, cb, telegramservice.CallbackBuy)
	case data == telegramservice.CallbackTrialBack:
		d.editHome(ctx, cb)
	default:
		d.answer(ctx, cb.ID, telegramservice.UnavailableText())
	}
}

// trialMenu shows remaining quota + the server picker (feature off → disabled).
func (d *Dispatcher) trialMenu(ctx context.Context, cb *models.CallbackQuery) {
	if d.shop == nil || d.shop.Trials == nil || d.shop.TrialLm == nil {
		d.answer(ctx, cb.ID, telegramservice.UnavailableText())
		return
	}
	if !d.shop.TrialLm.Enabled() {
		d.editCB(ctx, cb, telegramservice.TrialDisabledText(), nil)
		return
	}
	remaining, err := d.shop.TrialLm.Remaining(ctx, cb.From.ID)
	if err != nil {
		d.logger.Error("trial remaining failed", "user_id", cb.From.ID, "error", err)
		d.answer(ctx, cb.ID, "Gagal memuat kuota trial, coba lagi ya.")
		return
	}
	if remaining <= 0 {
		d.editCB(ctx, cb, telegramservice.TrialLimitText(), nil)
		return
	}
	servers, err := d.shop.Servers.ListBuyable(ctx)
	if err != nil {
		d.logger.Error("trial servers failed", "user_id", cb.From.ID, "error", err)
		d.answer(ctx, cb.ID, "Gagal memuat server, coba lagi ya.")
		return
	}
	if len(servers) == 0 {
		d.editCB(ctx, cb, "Belum ada server tersedia. Coba lagi nanti ya.", nil)
		return
	}
	d.editCB(ctx, cb, telegramservice.TrialMenuText(remaining, d.shop.TrialLm.Hours(), d.shop.TrialLm.TrafficGB()),
		telegramservice.TrialServersKeyboard(servers))
}

// trialMenuSend answers /trial with a fresh menu message (not an edit).
// The dispatcher routes /trial here only when the feature is enabled.
func (d *Dispatcher) trialMenuSend(ctx context.Context, msg *models.Message) {
	remaining, err := d.shop.TrialLm.Remaining(ctx, msg.From.ID)
	if err != nil {
		d.logger.Error("trial remaining failed", "user_id", msg.From.ID, "error", err)
		d.send(ctx, msg.Chat.ID, "Gagal memuat kuota trial, coba lagi ya.", nil)
		return
	}
	if remaining <= 0 {
		d.send(ctx, msg.Chat.ID, telegramservice.TrialLimitText(), nil)
		return
	}
	servers, err := d.shop.Servers.ListBuyable(ctx)
	if err != nil {
		d.logger.Error("trial servers failed", "user_id", msg.From.ID, "error", err)
		d.send(ctx, msg.Chat.ID, "Gagal memuat server, coba lagi ya.", nil)
		return
	}
	if len(servers) == 0 {
		d.send(ctx, msg.Chat.ID, "Belum ada server tersedia. Coba lagi nanti ya.", nil)
		return
	}
	d.send(ctx, msg.Chat.ID,
		telegramservice.TrialMenuText(remaining, d.shop.TrialLm.Hours(), d.shop.TrialLm.TrafficGB()),
		telegramservice.TrialServersKeyboard(servers))
}
