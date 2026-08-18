// Package telegramhandler also hosts the topup flow (FR-06, M5 partial).
//
// @file      internal/handler/telegram/topup.go
// @for       Topup menu → quick-pick/custom nominal → confirm → gateway seam.
// @uses      context, strings, github.com/go-telegram/bot/models, internal/domain,
// internal/service/telegram, internal/service/topup
// @reason    Menus & flow are product-final; the payment call goes through the
// PaymentGateway seam (Phase 4: kts PG charge), settlement via webhook.
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
	"github.com/kentangtech/bot-order/internal/domain"
	telegramservice "github.com/kentangtech/bot-order/internal/service/telegram"
	topupsvc "github.com/kentangtech/bot-order/internal/service/topup"
)

// Topup holds the topup service seams (implemented by topupsvc + redis.FSM).
type Topup struct {
	Users  UserReader
	Topups TopupRunner
	FSM    TopupFSM
}

// TopupRunner quotes fees and creates payments (topupsvc.Service).
type TopupRunner interface {
	Quote(net domain.Money) (topupsvc.Quote, error)
	CreatePayment(ctx context.Context, req topupsvc.CreatePaymentRequest) (*topupsvc.PaymentResult, error)
	MinNet() domain.Money
	MaxNet() domain.Money
}

// TopupFSM persists the custom-nominal input state (redis.TopupFSM).
type TopupFSM interface {
	SetPending(ctx context.Context, userID int64) error
	Pending(ctx context.Context, userID int64) (bool, error)
	Clear(ctx context.Context, userID int64) error
}

// routeTopup dispatches topup callbacks (FR-06).
func (d *Dispatcher) routeTopup(ctx context.Context, cb *models.CallbackQuery) {
	data := cb.Data
	switch {
	case data == telegramservice.CallbackTopup:
		d.topupMenu(ctx, cb)
	case strings.HasPrefix(data, telegramservice.PrefixTopupAmount):
		d.topupAmount(ctx, cb, strings.TrimPrefix(data, telegramservice.PrefixTopupAmount))
	case data == telegramservice.PrefixTopupCustom:
		d.topupCustom(ctx, cb)
	case strings.HasPrefix(data, telegramservice.PrefixTopupConfirm):
		d.topupConfirm(ctx, cb, strings.TrimPrefix(data, telegramservice.PrefixTopupConfirm))
	case data == telegramservice.CallbackTopupBack:
		d.topupCancel(ctx, cb)
	default:
		d.answer(ctx, cb.ID, telegramservice.UnavailableText())
	}
}

// topupMenu shows the quick-pick keyboard with the current balance.
// Any stale custom-input marker is dropped so the flow always restarts clean.
func (d *Dispatcher) topupMenu(ctx context.Context, cb *models.CallbackQuery) {
	d.topupClearFSM(ctx, cb.From.ID)
	balance := d.topupBalance(ctx, cb.From.ID)
	d.editCB(ctx, cb, telegramservice.TopupMenuText(balance), telegramservice.TopupKeyboard())
}

// topupBalance reads the user's balance; a missing user row reads as zero.
func (d *Dispatcher) topupBalance(ctx context.Context, tgID int64) domain.Money {
	balance, err := d.topup.Users.Balance(ctx, tgID)
	if err != nil {
		d.logger.Debug("topup balance unavailable", "user_id", tgID, "error", err)
		return 0
	}
	return balance
}

// topupAmount quotes the picked NET nominal and asks for confirmation.
func (d *Dispatcher) topupAmount(ctx context.Context, cb *models.CallbackQuery, raw string) {
	net, err := parseMoney(raw)
	if err != nil {
		d.answer(ctx, cb.ID, "Nominal tidak valid.")
		return
	}
	d.topupClearFSM(ctx, cb.From.ID)
	quote, err := d.topup.Topups.Quote(net)
	if err != nil {
		d.answer(ctx, cb.ID, "Nominal di luar batas. Pilih nominal lain ya.")
		return
	}
	d.editCB(ctx, cb, telegramservice.TopupSummaryText(quote, d.topupBalance(ctx, cb.From.ID)), telegramservice.TopupConfirmKeyboard(net))
}

// topupCustom arms the FSM and prompts for a typed nominal (FR-06 AC).
func (d *Dispatcher) topupCustom(ctx context.Context, cb *models.CallbackQuery) {
	if err := d.topup.FSM.SetPending(ctx, cb.From.ID); err != nil {
		d.logger.Error("topup fsm set failed", "user_id", cb.From.ID, "error", err)
		d.answer(ctx, cb.ID, "Terjadi kendala, coba lagi ya.")
		return
	}
	text := telegramservice.TopupCustomPrompt(d.topup.Topups.MinNet(), d.topup.Topups.MaxNet())
	d.editCB(ctx, cb, text, telegramservice.TopupCustomKeyboard())
}

// topupHandleText consumes free text while the custom FSM is armed; it reports
// whether the message was consumed (FSM pending). /cancel is routed separately.
func (d *Dispatcher) topupHandleText(ctx context.Context, msg *models.Message) bool {
	if d.topup == nil {
		return false
	}
	pending, err := d.topup.FSM.Pending(ctx, msg.From.ID)
	if err != nil {
		d.logger.Error("topup fsm read failed", "user_id", msg.From.ID, "error", err)
		return false
	}
	if !pending {
		return false
	}

	net, err := parseMoney(msg.Text)
	if err != nil {
		d.send(ctx, msg.Chat.ID, telegramservice.TopupCustomPrompt(d.topup.Topups.MinNet(), d.topup.Topups.MaxNet()), telegramservice.TopupCustomKeyboard())
		return true
	}
	quote, err := d.topup.Topups.Quote(net)
	if err != nil {
		d.send(ctx, msg.Chat.ID, telegramservice.TopupCustomPrompt(d.topup.Topups.MinNet(), d.topup.Topups.MaxNet()), telegramservice.TopupCustomKeyboard())
		return true
	}
	if err := d.topup.FSM.Clear(ctx, msg.From.ID); err != nil {
		d.logger.Error("topup fsm clear failed", "user_id", msg.From.ID, "error", err)
	}
	d.send(ctx, msg.Chat.ID, telegramservice.TopupSummaryText(quote, d.topupBalance(ctx, msg.From.ID)), telegramservice.TopupConfirmKeyboard(net))
	return true
}

// topupConfirm creates the QRIS payment via the gateway (stub until the API
// rewrite ships — the flow stays intact and only the client is swapped in).
func (d *Dispatcher) topupConfirm(ctx context.Context, cb *models.CallbackQuery, raw string) {
	net, err := parseMoney(raw)
	if err != nil {
		d.answer(ctx, cb.ID, "Nominal tidak valid.")
		return
	}
	quote, err := d.topup.Topups.Quote(net)
	if err != nil {
		d.answer(ctx, cb.ID, "Nominal di luar batas.")
		return
	}
	d.topupClearFSM(ctx, cb.From.ID)

	result, err := d.topup.Topups.CreatePayment(ctx, topupsvc.CreatePaymentRequest{
		TelegramUserID: cb.From.ID,
		FirstName:      cb.From.FirstName,
		Username:       cb.From.Username,
		NetAmount:      quote.Net,
		GrossAmount:    quote.Gross,
	})
	if err != nil {
		d.logger.Warn("topup payment gateway unavailable", "user_id", cb.From.ID, "error", err)
		d.editCB(ctx, cb, telegramservice.TopupAPIUnavailableText(), nil)
		return
	}
	d.editCB(ctx, cb, telegramservice.TopupPaymentText(result), nil)
}

// topupCancel clears any pending input and returns to the home menu.
func (d *Dispatcher) topupCancel(ctx context.Context, cb *models.CallbackQuery) {
	d.topupClearFSM(ctx, cb.From.ID)
	d.editHome(ctx, cb)
}

// topupClearFSM best-effort clears the custom-input marker (stale state must
// never wedge the next topup attempt).
func (d *Dispatcher) topupClearFSM(ctx context.Context, userID int64) {
	if d.topup == nil || d.topup.FSM == nil {
		return
	}
	if err := d.topup.FSM.Clear(ctx, userID); err != nil {
		d.logger.Error("topup fsm clear failed", "user_id", userID, "error", err)
	}
}

// parseMoney reads a positive whole-rupiah amount from callback/text payloads.
func parseMoney(raw string) (domain.Money, error) {
	v, err := parseID64(strings.TrimSpace(raw))
	if err != nil || v <= 0 {
		return 0, fmt.Errorf("invalid nominal %q", raw)
	}
	return domain.NewMoney(v)
}
