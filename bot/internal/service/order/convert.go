// Package ordersvc also hosts the domain ↔ DB row converters.
//
// @file      internal/service/order/convert.go
// @for       toOrderRow / toClientRow + pointer helpers (M4 persistence mapping).
// @uses      time, internal/domain, internal/repository/postgres
// @reason    Keeps order.go under 250 lines (AGENTS.md §1.1) and the mapping
// in one place so domain changes stay localised.
// @author    Dodi Rusmana <rusmanadodi@kentangtechstore.com>
// @layer     service
// @stability stable
// @since     2026-08-11
package ordersvc

import (
	"strings"
	"time"

	"github.com/kentangtech/bot-order/internal/domain"
	"github.com/kentangtech/bot-order/internal/repository/postgres"
)

// toOrderRow converts the domain order to the postgres row.
func toOrderRow(o *domain.Order) *postgres.Order {
	return &postgres.Order{
		ID: o.ID, OrderID: o.OrderID, OrderType: string(o.Type), UserID: o.UserID,
		ServerID: ptr64(o.ServerID), ClientID: ptr64(o.ClientID), Protocol: o.Protocol,
		DurationDays: o.DurationDays, TrafficGB: o.TrafficGB, IPLimit: o.IPLimit,
		Amount: o.Amount, Discount: o.Discount, FinalAmount: o.FinalAmount,
		Currency: o.Currency, Status: string(o.Status), ErrorMessage: o.ErrorMessage,
		AccountEmail: o.AccountEmail, BalanceBefore: ptrMoney(o.BalanceBefore),
		BalanceAfter: ptrMoney(o.BalanceAfter), CompletedAt: o.CompletedAt,
		CreatedAt: o.CreatedAt, UpdatedAt: time.Now(),
	}
}

// toClientRow converts the domain client to the postgres row.
// ConfigLink is mapped so the share URI survives restarts (M7 detail/export).
func toClientRow(c *domain.VPNClient) *postgres.VPNClient {
	return &postgres.VPNClient{
		UserID: c.UserID, ServerID: c.ServerID, InboundID: c.InboundID,
		Email: c.Email, UUID: c.UUID, Password: c.Password, Protocol: c.Protocol,
		TrafficLimit: c.TrafficLimit, IPLimit: c.IPLimit, IsActive: true,
		IsTrial: c.IsTrial, ExpiresAt: c.ExpiresAt, ConfigLink: c.ConfigLink,
		InboundNetwork: c.InboundNetwork, InboundPath: c.InboundPath,
		SubID: c.SubID, SubscriptionURL: c.SubscriptionURL,
		SubscriptionJSONURL: c.SubscriptionJSONURL,
	}
}

// ptr64 maps a zero value to NULL (client_id/server_id are set only after the
// related row exists — e.g. ClientID 0 before the client record is created).
func ptr64(v int64) *int64 {
	if v == 0 {
		return nil
	}
	return &v
}
func ptrMoney(v domain.Money) *domain.Money { return &v }

// trafficGB returns the default traffic quota in GB (FR-03; configurable later).
func trafficGB() int { return 100 }

// ipLimit returns the default per-client IP limit.
func ipLimit() int { return 1 }

// clientEmail builds the X-UI client email from the order id, lowercased.
func clientEmail(orderID string) string {
	slug := strings.ToLower(strings.ReplaceAll(orderID, "-", ""))
	return slug + "@vpn.kt"
}
