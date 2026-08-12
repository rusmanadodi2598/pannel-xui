// Package postgres also hosts the admin order & revenue stats (FR-11, v1.40).
//
// @file      internal/repository/postgres/order_stats.go
// @for       Admin stats: order counts + revenue today/total, status breakdown.
// @uses      context, fmt, time, internal/domain
// @reason    FR-11 statistik — aggregation lives in SQL (one round-trip, no
// N+1), the admin service just formats it. Split for the §1.1 line limit.
// @author    Dodi Rusmana <rusmanadodi@kentangtechstore.com>
// @layer     repository
// @stability stable
// @since     2026-08-12
package postgres

import (
	"context"
	"fmt"
	"time"

	"github.com/kentangtech/bot-order/internal/domain"
	"gorm.io/gorm"
)

// StatusCount is one order-status bucket (FR-11 breakdown).
type StatusCount struct {
	Status string
	Count  int64
}

// OrderStats is the FR-11 dashboard read model.
type OrderStats struct {
	TotalOrders   int64
	TodayOrders   int64
	TotalRevenue  domain.Money // sum of final_amount on completed orders
	TodayRevenue  domain.Money
	Completed     int64
	Failed        int64
	Pending       int64
	Processing    int64
	Cancelled     int64
	Refunded      int64
	TotalUsers    int64
	ActiveClients int64
} // Stats aggregates order counts + revenue for the admin dashboard (FR-11).
// Revenue counts only completed orders (money actually collected); the status
// buckets use the order status column directly. Status values are hardcoded
// literals (not bind params) so the FILTER clause stays unambiguous.
func (r *OrderRepo) Stats(ctx context.Context, loc *time.Location) (OrderStats, error) {
	today := time.Now().In(loc)
	startOfDay := time.Date(today.Year(), today.Month(), today.Day(), 0, 0, 0, 0, loc)
	completed := string(domain.OrderCompleted)

	var out OrderStats
	err := r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := tx.Table("orders").
			Select("COUNT(*) AS total_orders, "+
				"COUNT(*) FILTER (WHERE created_at >= ?) AS today_orders, "+
				"COALESCE(SUM(final_amount) FILTER (WHERE status = '"+completed+"'), 0) AS total_revenue, "+
				"COALESCE(SUM(final_amount) FILTER (WHERE status = '"+completed+"' AND created_at >= ?), 0) AS today_revenue, "+
				"COUNT(*) FILTER (WHERE status = '"+completed+"') AS completed, "+
				"COUNT(*) FILTER (WHERE status = 'failed') AS failed, "+
				"COUNT(*) FILTER (WHERE status = 'pending') AS pending, "+
				"COUNT(*) FILTER (WHERE status = 'processing') AS processing, "+
				"COUNT(*) FILTER (WHERE status = 'cancelled') AS cancelled, "+
				"COUNT(*) FILTER (WHERE status = 'refunded') AS refunded",
				startOfDay, startOfDay).
			Scan(&out).Error; err != nil {
			return fmt.Errorf("aggregating order stats: %w", err)
		}
		if err := tx.Model(&User{}).Count(&out.TotalUsers).Error; err != nil {
			return fmt.Errorf("counting users: %w", err)
		}
		if err := tx.Model(&VPNClient{}).
			Where("is_active = true AND (is_expired = false OR expires_at > now())").
			Count(&out.ActiveClients).Error; err != nil {
			return fmt.Errorf("counting active clients: %w", err)
		}
		return nil
	})
	if err != nil {
		return OrderStats{}, err
	}
	return out, nil
}

// RecentOrders returns the newest orders for the admin dashboard (FR-11),
// joined with the owner's name for display, bounded to limit.
func (r *OrderRepo) RecentOrders(ctx context.Context, limit int) ([]Order, error) {
	if limit <= 0 || limit > 50 {
		limit = 10
	}
	var rows []Order
	err := r.db.WithContext(ctx).
		Order("created_at DESC, id DESC").
		Limit(limit).
		Find(&rows).Error
	if err != nil {
		return nil, fmt.Errorf("listing recent orders: %w", err)
	}
	return rows, nil
}
