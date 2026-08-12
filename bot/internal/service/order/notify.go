// Package ordersvc also hosts the completed-order admin notice (FR-04 AC, v1.41).
//
// @file      internal/service/order/notify.go
// @for       OrderNotice payload + OrderNotifier seam fired after a paid order
// completes (purchase + renewal) — closes the FR-04 AC "notifikasi ke grup
// admin" gap.
// @uses      context, fmt, time, internal/domain, internal/repository/postgres
// @reason    The order service stays transport-free: the composition root wires
// the actual Telegram sender. Best-effort by design — a failed admin notice
// must never fail the order. Split from order.go for §1.1.
// @author    Dodi Rusmana <rusmanadodi@kentangtechstore.com>
// @layer     service
// @stability experimental
// @since     2026-08-12
package ordersvc

import (
	"context"
	"fmt"
	"time"

	"github.com/kentangtech/bot-order/internal/domain"
	"github.com/kentangtech/bot-order/internal/repository/postgres"
)

// OrderNotice is what the admin group sees after a paid order completes
// (FR-04 AC). UserLabel/PlanLabel are pre-formatted by the order flows.
type OrderNotice struct {
	OrderID      string
	OrderType    domain.OrderType
	UserLabel    string
	PlanLabel    string // e.g. "ID 30 Hari"
	Amount       domain.Money
	AccountEmail string
	BalanceAfter domain.Money
	NewExpiry    time.Time
}

// OrderNotifier reports a completed paid order (repository/telegram.Client
// backed by a small adapter in cmd/bot implements it; nil = disabled).
type OrderNotifier interface {
	NotifyOrderCompleted(ctx context.Context, n OrderNotice) error
}

// notifyCompleted fires the admin-group notice after a paid order completes.
// Best-effort: the notice is operational, never order-critical — the
// composition-root adapter (which owns a logger) records failures, and the
// order stays completed regardless.
func (s *Service) notifyCompleted(ctx context.Context, n OrderNotice) {
	if s.notify == nil {
		return
	}
	_ = s.notify.NotifyOrderCompleted(ctx, n)
}

// userLabel renders a compact human label for the notice.
func userLabel(u *postgres.User) string {
	if u == nil {
		return "?"
	}
	if u.FirstName != "" {
		return u.FirstName
	}
	if u.Username != "" {
		return "@" + u.Username
	}
	return fmt.Sprintf("ID %d", u.TelegramID)
}

// planLabel renders the compact plan label shared by admin notices (FR-04 AC).
func planLabel(country string, days int) string {
	return fmt.Sprintf("%s %d Hari", country, days)
}
