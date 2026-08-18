// Package telegramhandler also hosts the trial execution steps (FR-07).
//
// @file      internal/handler/telegram/trial_execute.go
// @for       FR-07 trial: server/inbound pick, confirm, and daily-limit claim + execute.
// @uses      context, fmt, github.com/go-telegram/bot/models, internal/service/order,
// internal/service/server, internal/service/telegram, internal/service/trial
// @reason    Split from trial.go to respect the 250-line limit (AGENTS.md §1.1);
// the claim is re-checked at server pick, confirm AND executed atomically.
// @author    Dodi Rusmana <rusmanadodi@kentangtechstore.com>
// @layer     handler
// @stability experimental
// @since     2026-08-17
package telegramhandler

import (
	"context"
	"fmt"

	"github.com/go-telegram/bot/models"
	ordersvc "github.com/kentangtech/bot-order/internal/service/order"
	serversvc "github.com/kentangtech/bot-order/internal/service/server"
	telegramservice "github.com/kentangtech/bot-order/internal/service/telegram"
	trialsvc "github.com/kentangtech/bot-order/internal/service/trial"
)

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
