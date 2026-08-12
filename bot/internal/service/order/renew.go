// Package ordersvc also hosts the renewal fulfillment (FR-05, v1.37 rewrite).
//
// @file      internal/service/order/renew.go
// @for       Renewal: paid-only guard, in-flight dedup, debit-first + auto-refund.
// @uses      context, errors, time, internal/domain, internal/repository/postgres
// @reason    Renewal modifies an existing account, so money moves BEFORE the
// panel call (guarded, never negative) and any later failure triggers an
// atomic refund — the balance is never inconsistent with the account state
// (user decision v1.37; purchase keeps the FR-04 AC-1 panel-first rule).
// @author    Dodi Rusmana <rusmanadodi@kentangtechstore.com>
// @layer     service
// @stability stable
// @since     2026-08-12
package ordersvc

import (
	"context"
	"errors"
	"time"

	"github.com/kentangtech/bot-order/internal/domain"
	"github.com/kentangtech/bot-order/internal/repository/postgres"
)

// Renew extends an existing paid account (FR-05, paid-only since v1.37).
// The expiry is computed from the remaining time (never double-counted) —
// base = max(now, current expiry). Money flow (v1.37):
//  1. guards: ownership, paid-only (trial rejected), live plan, in-flight dedup
//  2. order pending → processing (references the client from creation)
//  3. atomic debit (balance >= price — never negative) BEFORE any panel I/O
//  4. panel extends the client expiry
//  5. DB expiry updated, order completed
//
// Any failure after step 3 refunds the exact amount (Credit + ledger row) so
// the balance is never lost and the user never pays for a failed renewal.
func (s *Service) Renew(ctx context.Context, user *postgres.User, clientID int64, country string, days int) (*PurchaseResult, error) {
	client, err := s.clients.GetOwned(ctx, clientID, user.ID)
	if err != nil {
		return nil, ErrClientNotFound
	}
	// FR-05 (v1.37): renewal is for paid accounts only — trial accounts are
	// created once with hour-based expiry and are never renewable.
	if client.IsTrial {
		return nil, ErrTrialNotRenewable
	}
	// Idempotence guard (v1.37): a duplicate execution (Telegram retry / double
	// tap) while a renewal is still in flight must not create a second order or
	// move money twice. Checked before the plan/balance reads so a duplicate
	// always surfaces as ErrOrderInFlight, never a misleading balance error.
	existing, err := s.orders.FindInFlight(ctx, user.ID, string(domain.OrderTypeRenewal))
	if err != nil {
		return nil, err
	}
	if existing != nil {
		return nil, ErrOrderInFlight
	}
	plan, err := s.plans.GetPlan(ctx, country, days)
	if err != nil {
		return nil, ErrPlanNotFound
	}
	if user.Balance < plan.Price {
		return nil, ErrInsufficientBalance
	}
	// Panel client key per protocol (v1.38): vless/vmess→UUID, trojan/hysteria→
	// password (auth), shadowsocks→email — x-ui keys ss clients by email, not
	// password (web/service/inbound.go UpdateInboundClient).
	credential := domain.PanelClientKey(client.Protocol, client.UUID, client.Password, client.Email)
	// A row without any panel credential (legacy/corrupt) can never be renewed
	// on the panel — fail BEFORE any order/debit/panel side effect (same guard
	// pattern as the delete flow, v1.38).
	if credential == "" {
		return nil, ErrClientNotFound
	}

	base := time.Now()
	if client.ExpiresAt != nil && client.ExpiresAt.After(base) {
		base = *client.ExpiresAt
	}
	newExpiry := base.AddDate(0, 0, days)

	order := domain.NewOrder(s.newID(), domain.OrderTypeRenewal, user.ID, client.ServerID, client.Protocol, days, plan.Price)
	order.ClientID = client.ID // known up front — the row references the account from creation
	dbOrder := toOrderRow(order)
	if err := s.orders.Create(ctx, dbOrder); err != nil {
		return nil, err
	}
	order.ID = dbOrder.ID

	// pending → processing before any money/panel I/O (FR-04 state machine).
	if err := order.Transition(domain.OrderProcessing); err != nil {
		return nil, err
	}
	if err := s.orders.Save(ctx, toOrderRow(order)); err != nil {
		return nil, err
	}

	// Atomic debit FIRST (v1.37): the SQL guard (balance >= amount AND NOT
	// banned) makes a negative balance impossible, and the panel is never
	// extended without payment. An insufficient race surfaces here even though
	// the soft pre-check above already filtered the obvious case.
	balanceAfter, err := s.users.Debit(ctx, user.ID, plan.Price, order.OrderID)
	if err != nil {
		_ = order.MarkFailed("debit gagal, renewal tidak diproses")
		_ = s.orders.Save(ctx, toOrderRow(order))
		result := &PurchaseResult{OrderID: order.OrderID, Status: order.Status, ErrorMessage: err.Error()}
		if errors.Is(err, postgres.ErrInsufficientBalance) {
			return result, ErrInsufficientBalance
		}
		return result, ErrFulfillFailed
	}

	if err := s.panels.RenewClient(ctx, client.ServerID, credential, client.Email, client.Protocol, newExpiry); err != nil {
		s.refund(ctx, user.ID, plan.Price, order.OrderID)
		_ = order.MarkFailed(err.Error())
		_ = s.orders.Save(ctx, toOrderRow(order))
		return &PurchaseResult{OrderID: order.OrderID, Status: order.Status, ErrorMessage: err.Error()}, ErrFulfillFailed
	}
	if err := s.clients.UpdateExpiry(ctx, client.ID, newExpiry, nil); err != nil {
		s.refund(ctx, user.ID, plan.Price, order.OrderID)
		_ = order.MarkFailed("gagal memperbarui masa aktif")
		_ = s.orders.Save(ctx, toOrderRow(order))
		return &PurchaseResult{OrderID: order.OrderID, Status: order.Status, ErrorMessage: err.Error()}, ErrFulfillFailed
	}

	if err := order.Complete(balanceAfter); err != nil {
		return nil, err
	}
	order.AccountEmail = client.Email
	order.ClientID = client.ID
	if err := s.orders.Save(ctx, toOrderRow(order)); err != nil {
		return nil, err
	}

	// FR-04 AC (v1.41): completed renewal → notice to the admin group
	// (NOTIFICATION_GROUP_ID); best-effort, never fails the order.
	s.notifyCompleted(ctx, OrderNotice{
		OrderID:      order.OrderID,
		OrderType:    domain.OrderTypeRenewal,
		UserLabel:    userLabel(user),
		PlanLabel:    planLabel(plan.CountryCode, days),
		Amount:       plan.Price,
		AccountEmail: client.Email,
		BalanceAfter: balanceAfter,
		NewExpiry:    newExpiry,
	})

	return &PurchaseResult{
		OrderID: order.OrderID, Status: order.Status, AccountEmail: client.Email,
		BalanceAfter: balanceAfter, Plan: plan, ServerID: client.ServerID, ClientID: client.ID,
		NewExpiry: newExpiry,
	}, nil
}

// refund restores the exact debited amount after a failed renewal (v1.37).
// Best-effort: the failure already marked the order failed, and the ledger
// keeps the debit row. If the refund itself fails, the reconciliation trail is
// the order's ErrorMessage + the unmatched debit ledger row (no credit).
func (s *Service) refund(ctx context.Context, userID int64, amount domain.Money, orderID string) {
	_, _ = s.users.Credit(ctx, userID, amount, orderID)
}
