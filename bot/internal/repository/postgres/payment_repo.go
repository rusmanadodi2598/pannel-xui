// Package postgres also hosts the topup payment repository (FR-06, Phase 4).
//
// @file      internal/repository/postgres/payment_repo.go
// @for       Persist topup payment rows + atomic pending→terminal transition.
// @uses      context, errors, fmt, time, gorm.io/gorm
// @reason    The pg.charge webhook needs the local orderId → user + NET map to
// credit the right amount; the conditional transition makes poll + webhook
// double-delivery unable to double-credit (AGENTS.md §1.6 idempotency).
// @author    Dodi Rusmana <rusmanadodi@kentangtechstore.com>
// @layer     repository
// @stability experimental
// @since     2026-08-18
package postgres

import (
	"context"
	"errors"
	"fmt"
	"time"

	"gorm.io/gorm"
)

// Payment terminal statuses mirroring the `payments` table contract (PRD §13.6).
const (
	PaymentStatusPending = "pending"
	PaymentStatusSuccess = "success"
	PaymentStatusFailed  = "failed"
	PaymentStatusExpired = "expired"
)

// ErrPaymentNotFound is returned when a topup payment row is missing.
var ErrPaymentNotFound = errors.New("payment not found")

// PaymentRepo persists topup payment rows.
type PaymentRepo struct{ db *gorm.DB }

// NewPaymentRepo builds the repository on the shared GORM handle.
func NewPaymentRepo(db *gorm.DB) *PaymentRepo { return &PaymentRepo{db: db} }

// Create inserts one pending topup payment row (orderId is UNIQUE).
func (r *PaymentRepo) Create(ctx context.Context, p *Payment) error {
	if err := r.db.WithContext(ctx).Create(p).Error; err != nil {
		return fmt.Errorf("creating payment %s: %w", p.OrderID, err)
	}
	return nil
}

// GetByOrderID loads a payment by its gateway orderId.
func (r *PaymentRepo) GetByOrderID(ctx context.Context, orderID string) (*Payment, error) {
	var p Payment
	err := r.db.WithContext(ctx).Where("order_id = ?", orderID).First(&p).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, ErrPaymentNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("getting payment %s: %w", orderID, err)
	}
	return &p, nil
}

// SaveProviderRef stores the provider (Midtrans) reference after confirm so the
// row is traceable end-to-end (015 §2.1 single id discipline).
func (r *PaymentRepo) SaveProviderRef(ctx context.Context, orderID, providerRef string) error {
	res := r.db.WithContext(ctx).Model(&Payment{}).
		Where("order_id = ?", orderID).
		Update("provider_ref", providerRef)
	if res.Error != nil {
		return fmt.Errorf("saving payment %s provider ref: %w", orderID, res.Error)
	}
	return nil
}

// MarkFailed marks a pending payment failed without a credit (charge creation
// aborted client-side) — a failed row is never a zombie pending.
func (r *PaymentRepo) MarkFailed(ctx context.Context, orderID, reason string) error {
	res := r.db.WithContext(ctx).Model(&Payment{}).
		Where("order_id = ? AND status = ?", orderID, PaymentStatusPending).
		Updates(map[string]any{"status": PaymentStatusFailed, "provider_status": reason, "updated_at": time.Now()})
	if res.Error != nil {
		return fmt.Errorf("marking payment %s failed: %w", orderID, res.Error)
	}
	return nil
}

// MarkSettledTx atomically flips pending → terminal inside tx. It returns true
// only for the winning writer: a poll + webhook race can never double-credit
// because the credit happens in the same transaction as this transition.
func (r *PaymentRepo) MarkSettledTx(ctx context.Context, tx *gorm.DB, orderID, status string, paidAt *time.Time) (bool, error) {
	res := tx.WithContext(ctx).Model(&Payment{}).
		Where("order_id = ? AND status = ?", orderID, PaymentStatusPending).
		Updates(map[string]any{"status": status, "paid_at": paidAt, "updated_at": time.Now()})
	if res.Error != nil {
		return false, fmt.Errorf("marking payment %s settled: %w", orderID, res.Error)
	}
	return res.RowsAffected == 1, nil
}
