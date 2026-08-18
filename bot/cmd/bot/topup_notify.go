// Package main also hosts the topup settlement notifier adapter (Phase 4).
//
// @file      cmd/bot/topup_notify.go
// @for       Adapter: topupsvc.TopupNotifier → SendMessage to the user (+ admin
// group when NOTIFICATION_GROUP_ID is configured).
// @uses      context, log/slog, internal/repository/telegram, internal/service/telegram,
// internal/service/topup
// @reason    Composition-root glue (AGENTS.md §1.5): the topup service stays
// transport-free; this adapter renders the settlement notice and delivers it,
// logging failures without affecting the settled balance.
// @author    Dodi Rusmana <rusmanadodi@kentangtechstore.com>
// @layer     service
// @stability experimental
// @since     2026-08-18
package main

import (
	"context"
	"log/slog"

	telegramrepo "github.com/kentangtech/bot-order/internal/repository/telegram"
	telegramservice "github.com/kentangtech/bot-order/internal/service/telegram"
	topupsvc "github.com/kentangtech/bot-order/internal/service/topup"
)

// topupNotifier delivers the settlement notice to the user and, when an admin
// group is configured, a compact line to the group. Best-effort: a failed
// delivery is logged, never fatal to the settled balance.
type topupNotifier struct {
	tg     *telegramrepo.Client
	chatID int64 // admin group (0 = disabled)
	logger *slog.Logger
}

// NotifyTopupSettled sends the user-facing settlement message and the admin
// group line (FR-06, Phase 4).
func (n *topupNotifier) NotifyTopupSettled(ctx context.Context, notice topupsvc.TopupNotice) error {
	text := telegramservice.TopupSettledText(notice.Status, notice.Amount, notice.BalanceAfter)
	if err := n.tg.SendMessage(ctx, notice.TelegramID, text, "", nil); err != nil {
		n.logger.Error("topup notice to user failed", "order_id", notice.OrderID, "error", err)
		return err
	}
	if n.chatID != 0 {
		admin := telegramservice.AdminTopupNoticeText(notice.OrderID, notice.Status, notice.TelegramID, notice.Amount, notice.BalanceAfter)
		if err := n.tg.SendMessage(ctx, n.chatID, admin, "", nil); err != nil {
			n.logger.Error("topup notice to admin group failed", "order_id", notice.OrderID, "error", err)
		}
	}
	return nil
}
