// Package ordersvc also hosts the purchase fulfillment (FR-03/FR-04, M7 fix).
//
// @file      internal/service/order/purchase.go
// @for       Purchase state machine with explicit server + inbound selection.
// @uses      context, time, internal/domain, internal/repository/postgres
// @reason    The buy flow pins the exact panel inbound the user picked; the
// legacy auto-pick path stays for backward compatibility (M7 gap fix).
//
// @author    Dodi Rusmana <rusmanadodi@kentangtechstore.com>
// @layer     service
// @stability stable
// @since     2026-08-11
package ordersvc

import (
	"context"
	"time"

	"github.com/kentangtech/bot-order/internal/domain"
	"github.com/kentangtech/bot-order/internal/repository/postgres"
)

// PurchaseResult summarizes a finished order for the user message.
// NewExpiry is the actual computed expiry (renewal extends from remaining time).
type PurchaseResult struct {
	OrderID      string
	Status       domain.OrderStatus
	AccountEmail string
	BalanceAfter domain.Money
	Plan         *domain.VpnPlan
	ServerID     int64
	ClientID     int64
	NewExpiry    time.Time
	ErrorMessage string
}

// Purchase buys a new VPN account (FR-03/FR-04). The caller picks the exact
// panel (serverID) and inbound (inboundID) in the buy flow; protocol is the
// picked inbound's protocol. Passing 0 for serverID+inboundID keeps the legacy
// auto-pick path (server by country, protocol "vless"). Steps:
// 1. live price + server pick   2. create pending order
// 3. panel addClient (outside DB tx)   4. atomic debit + ledger
// 5. client row + order completed — all only after panel success.
func (s *Service) Purchase(ctx context.Context, user *postgres.User, country string, days, serverID, inboundID int, protocol string) (*PurchaseResult, error) {
	plan, err := s.plans.GetPlan(ctx, country, days)
	if err != nil {
		return nil, ErrPlanNotFound
	}
	if protocol == "" {
		protocol = "vless" // legacy default (server not picked explicitly)
	}
	if serverID <= 0 {
		sid, err := s.servers.PickForCountry(ctx, country)
		if err != nil {
			return nil, ErrNoServer
		}
		serverID = int(sid)
	}
	// Idempotence guard (v1.37): a duplicate execution (Telegram retry / double
	// tap) while a purchase is still in flight must not create a second order
	// or move money twice. Checked before the balance read so a duplicate
	// always surfaces as ErrOrderInFlight, never a misleading balance error.
	existing, err := s.orders.FindInFlight(ctx, user.ID, string(domain.OrderTypePurchase))
	if err != nil {
		return nil, err
	}
	if existing != nil {
		return nil, ErrOrderInFlight
	}
	if user.Balance < plan.Price {
		return nil, ErrInsufficientBalance
	}

	order := domain.NewOrder(s.newID(), domain.OrderTypePurchase, user.ID, int64(serverID), protocol, days, plan.Price)
	order.TrafficGB, order.IPLimit = trafficGB(), ipLimit()
	dbOrder := toOrderRow(order)
	if err := s.orders.Create(ctx, dbOrder); err != nil {
		return nil, err
	}
	order.ID = dbOrder.ID

	// pending → processing before any panel I/O (FR-04 state machine).
	if err := order.Transition(domain.OrderProcessing); err != nil {
		return nil, err
	}
	if err := s.orders.Save(ctx, toOrderRow(order)); err != nil {
		return nil, err
	}

	email := clientEmail(order.OrderID)
	pc, err := s.panels.CreateClient(ctx, int64(serverID), inboundID, email, order.Protocol, days, int64(order.TrafficGB), int64(order.IPLimit))
	if err != nil {
		_ = order.MarkFailed(err.Error())
		_ = s.orders.Save(ctx, toOrderRow(order))
		return &PurchaseResult{OrderID: order.OrderID, Status: order.Status, ErrorMessage: err.Error()}, ErrFulfillFailed
	}

	client, err := domain.NewVPNClient(user.ID, int64(serverID), pc.InboundID, email, order.Protocol, pc.UUID, pc.Password, days, int64(order.TrafficGB), int64(order.IPLimit))
	if err != nil {
		_ = order.MarkFailed("gagal mencatat akun")
		_ = s.orders.Save(ctx, toOrderRow(order))
		return &PurchaseResult{OrderID: order.OrderID, Status: order.Status, ErrorMessage: err.Error()}, ErrFulfillFailed
	}
	client.ConfigLink = pc.ConfigLink
	client.InboundNetwork = pc.InboundNetwork
	client.InboundPath = pc.InboundPath
	row := toClientRow(client)
	if err := s.clients.Create(ctx, row); err != nil {
		// Panel provisioned but the DB record failed — no money taken yet; the
		// orphan panel client is cleaned up by M6 reconciliation.
		_ = order.MarkFailed("gagal menyimpan akun, panel client perlu dirollback")
		_ = s.orders.Save(ctx, toOrderRow(order))
		return &PurchaseResult{OrderID: order.OrderID, Status: order.Status, ErrorMessage: err.Error()}, ErrFulfillFailed
	}

	// Client row exists BEFORE the debit: a debit failure leaves an unpaid,
	// recoverable record instead of charging the user without an account.
	balanceAfter, err := s.users.Debit(ctx, user.ID, plan.Price, order.OrderID)
	if err != nil {
		_ = order.MarkFailed("debit gagal, akun belum dibayar")
		_ = s.orders.Save(ctx, toOrderRow(order))
		return &PurchaseResult{OrderID: order.OrderID, Status: order.Status, ErrorMessage: err.Error()}, ErrFulfillFailed
	}

	if err := order.Complete(balanceAfter); err != nil {
		return nil, err
	}
	order.AccountEmail = email
	order.ClientID = row.ID
	if err := s.orders.Save(ctx, toOrderRow(order)); err != nil {
		return nil, err
	}

	// FR-04 AC (v1.41): completed purchase → notice to the admin group
	// (NOTIFICATION_GROUP_ID); best-effort, never fails the order.
	s.notifyCompleted(ctx, OrderNotice{
		OrderID:      order.OrderID,
		OrderType:    domain.OrderTypePurchase,
		UserLabel:    userLabel(user),
		PlanLabel:    planLabel(plan.CountryCode, days),
		Amount:       plan.Price,
		AccountEmail: email,
		BalanceAfter: balanceAfter,
		NewExpiry:    *client.ExpiresAt,
	})

	return &PurchaseResult{
		OrderID: order.OrderID, Status: order.Status, AccountEmail: email,
		BalanceAfter: balanceAfter, Plan: plan, ServerID: int64(serverID), ClientID: row.ID,
		NewExpiry: *client.ExpiresAt,
	}, nil
}
