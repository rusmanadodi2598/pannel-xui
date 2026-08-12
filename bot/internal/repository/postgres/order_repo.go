// Package postgres also hosts the orders repository.
//
// @file      internal/repository/postgres/order_repo.go
// @for       Create/lookup/update orders + recent history (PRD §13.4, FR-04).
// @uses      context, errors, fmt, gorm.io/gorm
// @reason    Order state machine persists through OrderRepo; unique order_id
// prevents duplicates at the DB level (FR-04 AC-1).
// @author    Dodi Rusmana <rusmanadodi@kentangtechstore.com>
// @layer     repository
// @stability stable
// @since     2026-08-11
package postgres

import (
	"context"
	"errors"
	"fmt"

	"github.com/kentangtech/bot-order/internal/domain"
	"gorm.io/gorm"
)

// ErrOrderNotFound is returned for missing or foreign-owned orders (FR-14).
var ErrOrderNotFound = errors.New("order not found")

// OrderRepo persists orders.
type OrderRepo struct{ db *gorm.DB }

// NewOrderRepo builds the repository on the shared GORM handle.
func NewOrderRepo(db *gorm.DB) *OrderRepo { return &OrderRepo{db: db} }

// Create inserts one order (pending) and returns it with its DB id.
func (r *OrderRepo) Create(ctx context.Context, o *Order) error {
	if err := r.db.WithContext(ctx).Create(o).Error; err != nil {
		return fmt.Errorf("creating order %s: %w", o.OrderID, err)
	}
	return nil
}

// GetByOrderID loads an order by its public KTS-XXXXXXXX-VPN id.
func (r *OrderRepo) GetByOrderID(ctx context.Context, orderID string) (*Order, error) {
	var o Order
	err := r.db.WithContext(ctx).Where("order_id = ?", orderID).First(&o).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, gorm.ErrRecordNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("getting order %s: %w", orderID, err)
	}
	return &o, nil
}

// Save persists any order state change (status transitions, FR-04).
func (r *OrderRepo) Save(ctx context.Context, o *Order) error {
	if err := r.db.WithContext(ctx).Save(o).Error; err != nil {
		return fmt.Errorf("updating order %s: %w", o.OrderID, err)
	}
	return nil
}

// UpdateTx persists the order inside the fulfillment transaction.
func (r *OrderRepo) UpdateTx(ctx context.Context, tx *gorm.DB, o *Order) error {
	if err := tx.WithContext(ctx).Save(o).Error; err != nil {
		return fmt.Errorf("updating order %s: %w", o.OrderID, err)
	}
	return nil
}

// FindInFlight returns the newest order of the given type for the user that is
// still pending or processing, or nil when none is in flight (v1.37 idempotence
// guard — a duplicate execution must never create a second order or a second
// debit).
func (r *OrderRepo) FindInFlight(ctx context.Context, userID int64, orderType string) (*Order, error) {
	var o Order
	err := r.db.WithContext(ctx).
		Where("user_id = ? AND order_type = ? AND status IN ?",
			userID, orderType, []string{string(domain.OrderPending), string(domain.OrderProcessing)}).
		Order("id DESC").
		First(&o).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("finding in-flight order: %w", err)
	}
	return &o, nil
}

// ListByUser returns the user's recent orders, newest first (history view),
// bounded to limit (no pagination — FR-14 uses ListByUserPage).
func (r *OrderRepo) ListByUser(ctx context.Context, userID int64, limit int) ([]Order, error) {
	return r.ListByUserPage(ctx, userID, limit, 0)
}

// CountByUser counts the user's orders (FR-14 pagination sizing).
func (r *OrderRepo) CountByUser(ctx context.Context, userID int64) (int64, error) {
	var n int64
	err := r.db.WithContext(ctx).Model(&Order{}).Where("user_id = ?", userID).Count(&n).Error
	if err != nil {
		return 0, fmt.Errorf("counting user orders: %w", err)
	}
	return n, nil
}

// ListByUserPage returns one page of the user's orders, newest first
// (FR-14 list, 5/page). Limit is bounded and offset explicit — no unbounded
// fetch in a request-serving path (AGENTS.md §1.7).
func (r *OrderRepo) ListByUserPage(ctx context.Context, userID int64, limit, offset int) ([]Order, error) {
	if limit <= 0 || limit > 50 {
		limit = 5
	}
	if offset < 0 {
		offset = 0
	}
	var rows []Order
	err := r.db.WithContext(ctx).
		Where("user_id = ?", userID).
		Order("created_at DESC").
		Limit(limit).
		Offset(offset).
		Find(&rows).Error
	if err != nil {
		return nil, fmt.Errorf("listing user orders: %w", err)
	}
	return rows, nil
}

// GetOwned returns one order only when it belongs to the user — the FR-14
// detail guard (foreign or missing orders are indistinguishable).
func (r *OrderRepo) GetOwned(ctx context.Context, orderID, userID int64) (*Order, error) {
	var o Order
	err := r.db.WithContext(ctx).Where("id = ? AND user_id = ?", orderID, userID).First(&o).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, ErrOrderNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("getting order %d: %w", orderID, err)
	}
	return &o, nil
}
