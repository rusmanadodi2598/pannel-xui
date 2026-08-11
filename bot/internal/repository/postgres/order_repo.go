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

	"gorm.io/gorm"
)

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

// ListByUser returns the user's recent orders, newest first (history view).
func (r *OrderRepo) ListByUser(ctx context.Context, userID int64, limit int) ([]Order, error) {
	if limit <= 0 || limit > 50 {
		limit = 5
	}
	var rows []Order
	err := r.db.WithContext(ctx).
		Where("user_id = ?", userID).
		Order("created_at DESC").
		Limit(limit).
		Find(&rows).Error
	if err != nil {
		return nil, fmt.Errorf("listing user orders: %w", err)
	}
	return rows, nil
}
