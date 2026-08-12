// Package main also hosts the FR-04 AC admin-group order notifier.
//
// @file      cmd/bot/notify.go
// @for       Adapter: ordersvc.OrderNotifier → SendMessage to NOTIFICATION_GROUP_ID.
// @uses      context, log/slog, github.com/go-telegram/bot/models,
// internal/repository/telegram, internal/service/order, internal/service/telegram
// @reason    Composition-root glue (AGENTS.md §1.5): the order service stays
// transport-free; this adapter renders the notice and delivers it, logging
// failures without affecting the completed order. Split from shop.go for §1.1.
// @author    Dodi Rusmana <rusmanadodi@kentangtechstore.com>
// @layer     service
// @stability experimental
// @since     2026-08-12
package main

import (
	"context"
	"log/slog"

	telegramrepo "github.com/kentangtech/bot-order/internal/repository/telegram"
	ordersvc "github.com/kentangtech/bot-order/internal/service/order"
	telegramservice "github.com/kentangtech/bot-order/internal/service/telegram"
)

// orderNotifier delivers the completed-order notice to the admin group
// (FR-04 AC, v1.41). Best-effort: a failed delivery is logged, never fatal.
type orderNotifier struct {
	tg     *telegramrepo.Client
	chatID int64
	logger *slog.Logger
}

// NotifyOrderCompleted renders and sends the notice to NOTIFICATION_GROUP_ID.
func (n *orderNotifier) NotifyOrderCompleted(ctx context.Context, notice ordersvc.OrderNotice) error {
	text := telegramservice.AdminOrderNoticeText(
		notice.OrderID, notice.OrderType, notice.UserLabel, notice.PlanLabel,
		notice.AccountEmail, notice.Amount, notice.BalanceAfter, notice.NewExpiry)
	if err := n.tg.SendMessage(ctx, n.chatID, text, "", nil); err != nil {
		n.logger.Error("admin order notice failed", "order_id", notice.OrderID, "error", err)
		return err
	}
	return nil
}
