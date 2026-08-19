// Package httphandler also hosts the /api/v1/orders admin read handlers (PRD §26.5).
//
// @file      internal/handler/http/orders_api.go
// @for       Admin orders list/stats + order detail by public orderId.
// @uses      errors, net/http, time, gorm.io/gorm, internal/repository/postgres
// @reason    Read-only order projections — no credentials, no unbounded fetch
// (AGENTS.md §1.7). Split from users_api.go for the §1.1 line limit.
// @author    Dodi Rusmana <rusmanadodi@kentangtechstore.com>
// @layer     handler
// @stability experimental
// @since     2026-08-18
package httphandler

import (
	"errors"
	"net/http"
	"time"

	"github.com/kentangtech/bot-order/internal/repository/postgres"
	"gorm.io/gorm"
)

// orderDTO is the order response shape (Money → int64 rupiah).
type orderDTO struct {
	ID            int64      `json:"id"`
	OrderID       string     `json:"orderId"`
	OrderType     string     `json:"orderType"`
	UserID        int64      `json:"userId"`
	ServerID      *int64     `json:"serverId,omitempty"`
	ClientID      *int64     `json:"clientId,omitempty"`
	Protocol      string     `json:"protocol,omitempty"`
	DurationDays  int        `json:"durationDays"`
	TrafficGB     int        `json:"trafficGb"`
	Amount        int64      `json:"amount"`
	FinalAmount   int64      `json:"finalAmount"`
	Currency      string     `json:"currency"`
	Status        string     `json:"status"`
	AccountEmail  string     `json:"accountEmail,omitempty"`
	AccountRemark string     `json:"accountRemark,omitempty"`
	ErrorMessage  string     `json:"errorMessage,omitempty"`
	BalanceBefore *int64     `json:"balanceBefore,omitempty"`
	BalanceAfter  *int64     `json:"balanceAfter,omitempty"`
	CompletedAt   *time.Time `json:"completedAt,omitempty"`
	CreatedAt     time.Time  `json:"createdAt"`
}

func toOrderDTO(o postgres.Order) orderDTO {
	d := orderDTO{
		ID:            o.ID,
		OrderID:       o.OrderID,
		OrderType:     o.OrderType,
		UserID:        o.UserID,
		ServerID:      o.ServerID,
		ClientID:      o.ClientID,
		Protocol:      o.Protocol,
		DurationDays:  o.DurationDays,
		TrafficGB:     o.TrafficGB,
		Amount:        o.Amount.Rupiah(),
		FinalAmount:   o.FinalAmount.Rupiah(),
		Currency:      o.Currency,
		Status:        o.Status,
		AccountEmail:  o.AccountEmail,
		AccountRemark: o.AccountRemark,
		ErrorMessage:  o.ErrorMessage,
		CompletedAt:   o.CompletedAt,
		CreatedAt:     o.CreatedAt,
	}
	if o.BalanceBefore != nil {
		v := o.BalanceBefore.Rupiah()
		d.BalanceBefore = &v
	}
	if o.BalanceAfter != nil {
		v := o.BalanceAfter.Rupiah()
		d.BalanceAfter = &v
	}
	return d
}

// statsDTO is the admin order/revenue dashboard (FR-11 stats, PRD §26.5).
type statsDTO struct {
	TotalOrders   int64 `json:"totalOrders"`
	TodayOrders   int64 `json:"todayOrders"`
	TotalRevenue  int64 `json:"totalRevenue"`
	TodayRevenue  int64 `json:"todayRevenue"`
	Completed     int64 `json:"completed"`
	Failed        int64 `json:"failed"`
	Pending       int64 `json:"pending"`
	Processing    int64 `json:"processing"`
	Cancelled     int64 `json:"cancelled"`
	Refunded      int64 `json:"refunded"`
	TotalUsers    int64 `json:"totalUsers"`
	ActiveClients int64 `json:"activeClients"`
}

func toStatsDTO(s postgres.OrderStats) statsDTO {
	return statsDTO{
		TotalOrders:   s.TotalOrders,
		TodayOrders:   s.TodayOrders,
		TotalRevenue:  s.TotalRevenue.Rupiah(),
		TodayRevenue:  s.TodayRevenue.Rupiah(),
		Completed:     s.Completed,
		Failed:        s.Failed,
		Pending:       s.Pending,
		Processing:    s.Processing,
		Cancelled:     s.Cancelled,
		Refunded:      s.Refunded,
		TotalUsers:    s.TotalUsers,
		ActiveClients: s.ActiveClients,
	}
}

// listOrders returns the admin dashboard: stats + the most recent orders
// (bounded) in one response — the §26.5 "List/statistik order" surface.
func (o Options) listOrders(w http.ResponseWriter, r *http.Request) {
	loc := o.Location
	if loc == nil {
		loc = time.Local
	}
	stats, err := o.Orders.Stats(r.Context(), loc)
	if err != nil {
		o.Logger.Error("api: order stats", "error", err)
		writeAPIError(w, http.StatusInternalServerError, "INTERNAL", "Failed to aggregate orders")
		return
	}
	recent, err := o.Orders.RecentOrders(r.Context(), 50)
	if err != nil {
		o.Logger.Error("api: recent orders", "error", err)
		writeAPIError(w, http.StatusInternalServerError, "INTERNAL", "Failed to list orders")
		return
	}
	orders := make([]orderDTO, 0, len(recent))
	for _, v := range recent {
		orders = append(orders, toOrderDTO(v))
	}
	writeData(w, http.StatusOK, map[string]any{
		"stats":  toStatsDTO(stats),
		"orders": orders,
	})
}

func (o Options) getOrder(w http.ResponseWriter, r *http.Request) {
	orderID := r.PathValue("orderId")
	ord, err := o.Orders.GetByOrderID(r.Context(), orderID)
	if errors.Is(err, gorm.ErrRecordNotFound) {
		writeAPIError(w, http.StatusNotFound, "ORDER_NOT_FOUND", "Order not found")
		return
	}
	if err != nil {
		o.Logger.Error("api: getting order", "orderId", orderID, "error", err)
		writeAPIError(w, http.StatusInternalServerError, "INTERNAL", "Failed to get order")
		return
	}
	writeData(w, http.StatusOK, toOrderDTO(*ord))
}
