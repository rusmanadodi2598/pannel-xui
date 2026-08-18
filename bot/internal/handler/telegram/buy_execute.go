// Package telegramhandler also hosts the buy execution step.
//
// @file      internal/handler/telegram/buy_execute.go
// @for       FR-03/FR-04 buy: order execution + insufficient-balance copy helpers.
// @uses      context, github.com/go-telegram/bot/models, internal/service/order, internal/service/telegram
// @reason    Split from buy.go to respect the 250-line limit (AGENTS.md §1.1);
// keeps the order-execution branch and its shared copy helpers together.
// @author    Dodi Rusmana <rusmanadodi@kentangtechstore.com>
// @layer     handler
// @stability stable
// @since     2026-08-17
package telegramhandler

import (
	"context"

	"github.com/go-telegram/bot/models"
	ordersvc "github.com/kentangtech/bot-order/internal/service/order"
	telegramservice "github.com/kentangtech/bot-order/internal/service/telegram"
)

// buyExecute runs the order (FR-04 state machine) and reports the outcome.
// The protocol is re-resolved from the live panel so the order record always
// matches the pinned inbound — a stale/inaccessible inbound aborts the order
// instead of silently recording a wrong protocol.
func (d *Dispatcher) buyExecute(ctx context.Context, cb *models.CallbackQuery, country string, days, serverID, inboundID int, protocol string) {
	if protocol == "" {
		var ok bool
		protocol, ok = d.inboundProtocol(ctx, serverID, inboundID)
		if !ok {
			d.answer(ctx, cb.ID, "Protocol sudah tidak tersedia. Silakan ulangi dari awal ya.")
			return
		}
	}
	user, err := d.shop.Users.EnsureUser(ctx, cb.From.ID, cb.From.Username, cb.From.FirstName)
	if err != nil {
		d.logger.Error("ensuring user", "user_id", cb.From.ID, "error", err)
		d.answer(ctx, cb.ID, "Gagal memuat akun, coba lagi ya.")
		return
	}

	if !d.editCB(ctx, cb, "⏳ Memproses order...", nil) {
		return
	}

	res, err := d.shop.Orders.Purchase(ctx, user, country, days, serverID, inboundID, protocol)
	switch {
	case err == nil:
		d.send(ctx, cb.Message.Message.Chat.ID,
			telegramservice.BuySuccessText(res.OrderID, res.AccountEmail, days, res.BalanceAfter, res.Plan.CountryName),
			telegramservice.BuySuccessKeyboard(res.ClientID))
	case err == ordersvc.ErrInsufficientBalance:
		d.send(ctx, cb.Message.Message.Chat.ID, insufficientText(), topupHintKeyboard())
	case err == ordersvc.ErrNoServer:
		d.send(ctx, cb.Message.Message.Chat.ID, "Belum ada server tersedia untuk negara ini. Coba lagi nanti ya.", nil)
	case err == ordersvc.ErrPlanNotFound:
		d.send(ctx, cb.Message.Message.Chat.ID, "Paket sudah tidak tersedia.", nil)
	case err == ordersvc.ErrOrderInFlight:
		d.send(ctx, cb.Message.Message.Chat.ID, "Order sebelumnya masih diproses. Tunggu sebentar ya.", nil)
	default:
		d.logger.Error("purchase failed", "user_id", cb.From.ID, "country", country, "days", days, "error", err)
		// res may be nil for pre-order failures (DB errors) — never dereference.
		orderID := ""
		if res != nil {
			orderID = res.OrderID
		}
		d.send(ctx, cb.Message.Message.Chat.ID,
			telegramservice.BuyFailedText(orderID, "Terjadi kendala saat memproses order di server."), nil)
	}
}

// insufficientText is the shared "top up first" copy (buy + renew flows).
func insufficientText() string {
	return "Saldo kamu tidak cukup untuk paket ini.\n\nSilakan top up saldo dulu ya."
}

// topupHintKeyboard offers the top-up shortcut after an insufficient balance
// (buy + renew flows).
func topupHintKeyboard() models.ReplyMarkup {
	return models.InlineKeyboardMarkup{InlineKeyboard: [][]models.InlineKeyboardButton{
		{{Text: "💳 Top Up", CallbackData: telegramservice.CallbackTopup}},
		{{Text: "🏠 Menu Utama", CallbackData: telegramservice.CallbackHome}},
	}}
}
