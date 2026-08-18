// Package topupsvc also hosts the pg.charge settlement (Phase 4, FR-06).
//
// @file      internal/service/topup/settle.go
// @for       ApplySettlement: atomic credit + ledger when a pg.charge webhook
// reports a terminal state (succeeded|failed|expired).
// @uses      context, fmt, time, internal/domain, internal/repository/postgres,
// gorm.io/gorm
// @reason    Credit happens inside the same transaction as the pending→terminal
// transition, so poll + webhook double-delivery can never double-credit
// (AGENTS.md §1.6 idempotency); the notification is best-effort like ordersvc.
// @author    Dodi Rusmana <rusmanadodi@kentangtechstore.com>
// @layer     service
// @stability experimental
// @since     2026-08-18
package topupsvc

import (
	"context"
	"fmt"
	"time"

	"github.com/kentangtech/bot-order/internal/domain"
	"github.com/kentangtech/bot-order/internal/repository/postgres"
	"gorm.io/gorm"
)

// SettlementResult summarizes an applied webhook/poll settlement.
type SettlementResult struct {
	OrderID         string
	Status          string // success|failed|expired (local terminal state)
	BalanceAfter    domain.Money
	AlreadyTerminal bool // webhook delivered after another writer settled it
}

// TopupNotice is what the user (and admin group) see after a settlement.
type TopupNotice struct {
	OrderID      string
	TelegramID   int64
	Amount       domain.Money // net yang dikredit
	BalanceAfter domain.Money
	Status       string // success|failed|expired
}

// TopupNotifier reports a terminal topup settlement (adapter in cmd/bot
// implements it; nil = disabled, best-effort like ordersvc.OrderNotifier).
type TopupNotifier interface {
	NotifyTopupSettled(ctx context.Context, n TopupNotice) error
}

// ApplySettlement credits the user's balance when a pg.charge webhook reports
// succeeded; failed/expired only mark the payment terminal (no credit). The
// pending→terminal transition is conditional, so the poll fallback and the
// webhook can race safely: only one writer flips the row, and the credit runs
// in that same transaction. Unknown statuses are rejected (malformed webhook).
func (s *Service) ApplySettlement(ctx context.Context, orderID, status string) (*SettlementResult, error) {
	p, err := s.payments.GetByOrderID(ctx, orderID)
	if err != nil {
		return nil, err
	}
	if p.Status != postgres.PaymentStatusPending {
		return &SettlementResult{OrderID: orderID, Status: p.Status, AlreadyTerminal: true}, nil
	}
	terminal, credit := postgres.PaymentStatusFailed, false
	switch status {
	case "succeeded":
		terminal, credit = postgres.PaymentStatusSuccess, true
	case "failed":
		terminal = postgres.PaymentStatusFailed
	case "expired":
		terminal = postgres.PaymentStatusExpired
	default:
		return nil, fmt.Errorf("unknown pg.charge status %q", status)
	}
	now := s.now()
	var paidAt *time.Time
	if credit {
		paidAt = &now
	}

	var applied bool
	var balance domain.Money
	err = s.users.WithTx(ctx, func(tx *gorm.DB) error {
		var err error
		applied, err = s.payments.MarkSettledTx(ctx, tx, orderID, terminal, paidAt)
		if err != nil || !applied {
			return err
		}
		if credit {
			balance, err = s.users.CreditTx(ctx, tx, p.UserID, p.AmountNet, orderID)
			if err != nil {
				return fmt.Errorf("crediting topup %s: %w", orderID, err)
			}
		}
		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("settling topup %s: %w", orderID, err)
	}
	if !applied {
		return &SettlementResult{OrderID: orderID, AlreadyTerminal: true}, nil
	}

	res := &SettlementResult{OrderID: orderID, Status: terminal, BalanceAfter: balance}
	if s.notify != nil {
		_ = s.notify.NotifyTopupSettled(ctx, TopupNotice{
			OrderID:      orderID,
			TelegramID:   p.TelegramID,
			Amount:       p.AmountNet,
			BalanceAfter: balance,
			Status:       terminal,
		})
	}
	return res, nil
}
