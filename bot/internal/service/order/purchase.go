// Package ordersvc also hosts the purchase fulfillment (FR-03/FR-04, v1.47 rewrite).
//
// @file      internal/service/order/purchase.go
// @for       Purchase state machine with explicit server + inbound selection.
// @uses      context, errors, time, internal/domain, internal/repository/postgres
// @reason    The buy flow pins the exact panel inbound the user picked; money
// flow is debit-first (v1.47): prepare → insert row → debit → panel commit,
// with auto-refund + row delete on failure — no orphaned unpaid account.
// @author    Dodi Rusmana <rusmanadodi@kentangtechstore.com>
// @layer     service
// @stability stable
// @since     2026-08-11
package ordersvc

import (
	"context"
	"errors"
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
// auto-pick path (server by country, protocol "vless"). Money flow (v1.47):
//  1. live price + server pick + idempotence guard (FindInFlight)
//  2. order pending → processing
//  3. prepare the client (read-only: inbound + credentials + share link)
//  4. insert the client row (no panel mutation yet, no money moved)
//  5. atomic debit (balance >= price — never negative) BEFORE the panel call
//  6. panel commit (addClient) — the last fallible step
//
// A failure before step 6 leaves no panel account (clean failed order); a
// commit failure refunds the exact amount + deletes the row (auto-refund,
// parity renewal v1.37) — an unpaid active account is impossible.
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

	// Step 3: prepare WITHOUT mutating the panel (read-only GetInbounds +
	// credentials + share link). A failure here is a clean failed order — no
	// row, no money, no panel account.
	email := clientEmail(order.OrderID)
	prepared, err := s.panels.PrepareClient(ctx, int64(serverID), inboundID, email, order.Protocol, days, int64(order.TrafficGB), int64(order.IPLimit))
	if err != nil {
		_ = order.MarkFailed(err.Error())
		_ = s.orders.Save(ctx, toOrderRow(order))
		return &PurchaseResult{OrderID: order.OrderID, Status: order.Status, ErrorMessage: err.Error()}, ErrFulfillFailed
	}

	// Step 4: client row BEFORE the panel call (v1.47). The credentials are
	// already generated, so the row is complete. A failure here is still a
	// clean failed order (no money moved, no panel mutation).
	client, err := domain.NewVPNClient(user.ID, int64(serverID), prepared.Panel.InboundID, email, order.Protocol, prepared.Panel.UUID, prepared.Panel.Password, days, int64(order.TrafficGB), int64(order.IPLimit))
	if err != nil {
		_ = order.MarkFailed("gagal mencatat akun")
		_ = s.orders.Save(ctx, toOrderRow(order))
		return &PurchaseResult{OrderID: order.OrderID, Status: order.Status, ErrorMessage: err.Error()}, ErrFulfillFailed
	}
	client.ConfigLink = prepared.Panel.ConfigLink
	client.InboundNetwork = prepared.Panel.InboundNetwork
	client.InboundPath = prepared.Panel.InboundPath
	// FR-13: persist subId + subscription URLs (only the .txt export ships them).
	client.SubID = prepared.Panel.SubID
	client.SubscriptionURL = s.subLinks.URL(prepared.Panel.SubID)
	client.SubscriptionJSONURL = s.subLinks.JSONURL(prepared.Panel.SubID)
	row := toClientRow(client)
	if err := s.clients.Create(ctx, row); err != nil {
		_ = order.MarkFailed("gagal menyimpan akun")
		_ = s.orders.Save(ctx, toOrderRow(order))
		return &PurchaseResult{OrderID: order.OrderID, Status: order.Status, ErrorMessage: err.Error()}, ErrFulfillFailed
	}

	// Step 5: debit-first — money moves BEFORE the panel mutation. The row
	// already exists, so a debit failure only deletes the row: never an unpaid
	// panel account (the panel has not been touched yet).
	balanceAfter, err := s.users.Debit(ctx, user.ID, plan.Price, order.OrderID)
	if err != nil {
		_ = s.clients.DeleteOwned(ctx, row.ID, user.ID) // best-effort cleanup
		_ = order.MarkFailed("debit gagal, akun belum dibayar")
		_ = s.orders.Save(ctx, toOrderRow(order))
		result := &PurchaseResult{OrderID: order.OrderID, Status: order.Status, ErrorMessage: err.Error()}
		if errors.Is(err, postgres.ErrInsufficientBalance) {
			return result, ErrInsufficientBalance
		}
		return result, ErrFulfillFailed
	}

	// Step 6: panel commit — the last fallible step. A failure refunds the
	// exact amount (Credit + ledger) and deletes the row: no money lost, no
	// orphaned account (v1.37 debit-first + auto-refund parity).
	if err := s.panels.CommitClient(ctx, int64(serverID), prepared); err != nil {
		s.refund(ctx, user.ID, plan.Price, order.OrderID)
		_ = s.clients.DeleteOwned(ctx, row.ID, user.ID)
		_ = order.MarkFailed(err.Error())
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
