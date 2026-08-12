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
	"fmt"
	"strings"

	"github.com/go-telegram/bot/models"
	ordersvc "github.com/kentangtech/bot-order/internal/service/order"
	serversvc "github.com/kentangtech/bot-order/internal/service/server"
	telegramservice "github.com/kentangtech/bot-order/internal/service/telegram"
	trialsvc "github.com/kentangtech/bot-order/internal/service/trial"
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

// trialServer re-checks the limit (AC-1) and shows the panel's inbound picker
// for that server (FR-07: pick protocol before confirm).
func (d *Dispatcher) trialServer(ctx context.Context, cb *models.CallbackQuery, serverID int64) {
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
	server, ok := d.buyableServer(ctx, serverID)
	if !ok {
		d.answer(ctx, cb.ID, "Server tidak tersedia.")
		return
	}
	opts, err := d.shop.Servers.ListInbounds(ctx, serverID)
	if err != nil {
		d.logger.Error("trial inbounds failed", "user_id", cb.From.ID, "server_id", serverID, "error", err)
		d.answer(ctx, cb.ID, "Gagal memuat protocol server, coba lagi ya.")
		return
	}
	if len(opts) == 0 {
		d.editCB(ctx, cb, "Belum ada protocol tersedia di server ini. Coba lagi nanti ya.", nil)
		return
	}
	d.editCB(ctx, cb, telegramservice.TrialInboundListText(server),
		telegramservice.InboundsKeyboard(opts, func(o serversvc.InboundOption) string {
			return telegramservice.PrefixTrialInbound + fmt.Sprintf("%d:%d", o.ServerID, o.InboundID)
		}, telegramservice.CallbackTrialBack))
}

// trialConfirm resolves the protocol live and shows the confirmation.
func (d *Dispatcher) trialConfirm(ctx context.Context, cb *models.CallbackQuery, serverID, inboundID int64) {
	protocol, ok := d.inboundProtocol(ctx, int(serverID), int(inboundID))
	if !ok {
		d.answer(ctx, cb.ID, "Protocol tidak tersedia lagi. Silakan pilih ulang.")
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
	server, ok := d.buyableServer(ctx, serverID)
	if !ok {
		d.answer(ctx, cb.ID, "Server tidak tersedia.")
		return
	}
	d.editCB(ctx, cb,
		telegramservice.TrialConfirmText(server, d.shop.TrialLm.Hours(), d.shop.TrialLm.TrafficGB(), d.shop.TrialLm.IPLimit(), protocol),
		telegramservice.TrialConfirmKeyboard(serverID, inboundID))
}

// trialExecute validates the user/server first, then claims the daily slot
// (anti-race) and provisions the account on the picked inbound. The claim is
// the last gate before CreateTrial so a failed pre-check never burns a slot.
func (d *Dispatcher) trialExecute(ctx context.Context, cb *models.CallbackQuery, serverID, inboundID int64) {
	protocol, ok := d.inboundProtocol(ctx, int(serverID), int(inboundID))
	if !ok {
		d.answer(ctx, cb.ID, "Protocol sudah tidak tersedia. Silakan ulangi dari awal ya.")
		return
	}
	user, err := d.shop.Users.EnsureUser(ctx, cb.From.ID, cb.From.Username, cb.From.FirstName)
	if err != nil {
		d.logger.Error("trial ensure user failed", "user_id", cb.From.ID, "error", err)
		d.answer(ctx, cb.ID, "Gagal memuat akun, coba lagi ya.")
		return
	}
	server, ok := d.buyableServer(ctx, serverID)
	if !ok {
		d.answer(ctx, cb.ID, "Server tidak tersedia.")
		return
	}

	// AC-1 anti-race gate: the atomic claim decides — a double-tap in the same
	// second cannot both pass the daily limit. Claimed only after the user and
	// server exist so failed pre-checks never consume quota.
	claimed, err := d.shop.TrialLm.Claim(ctx, cb.From.ID)
	if err == trialsvc.ErrDailyLimitReached {
		d.editCB(ctx, cb, telegramservice.TrialLimitText(), nil)
		return
	}
	if err != nil {
		d.logger.Error("trial claim failed", "user_id", cb.From.ID, "error", err)
		d.answer(ctx, cb.ID, "Terjadi kendala, coba lagi ya.")
		return
	}

	if !d.editCB(ctx, cb, "Membuat akun trial...", nil) {
		return
	}

	res, err := d.shop.Trials.CreateTrial(ctx, user, serverID, ordersvc.TrialSpec{
		Hours:     d.shop.TrialLm.Hours(),
		TrafficGB: d.shop.TrialLm.TrafficGB(),
		IPLimit:   d.shop.TrialLm.IPLimit(),
		InboundID: int(inboundID),
		Protocol:  protocol,
	})
	if err != nil || res == nil {
		if err != nil {
			d.logger.Error("trial create failed", "user_id", cb.From.ID, "server_id", serverID, "error", err)
		} else {
			d.logger.Error("trial create returned nil result", "user_id", cb.From.ID, "server_id", serverID)
		}
		d.send(ctx, cb.Message.Message.Chat.ID, telegramservice.TrialFailedText(), nil)
		return
	}
	remaining := d.shop.TrialLm.Limit() - claimed
	if remaining < 0 {
		remaining = 0
	}
	d.send(ctx, cb.Message.Message.Chat.ID,
		telegramservice.TrialSuccessText(res.OrderID, res.AccountEmail, server.Name, remaining),
		telegramservice.TrialSuccessKeyboard(res.ClientID))
}
