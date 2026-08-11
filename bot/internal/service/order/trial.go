// Package ordersvc also hosts free trial order fulfillment (FR-07).
//
// @file      internal/service/order/trial.go
// @for       FR-07 AC-2: create trial order (no debit) + trial client row.
// @uses      context, time, internal/domain, internal/repository/postgres
// @reason    Trial is a free order: same state machine as Purchase but never
// debits balance — the daily limit is enforced by the caller (trialsvc).
// @author    Dodi Rusmana <rusmanadodi@kentangtechstore.com>
// @layer     service
// @stability experimental
// @since     2026-08-11
package ordersvc

import (
	"context"
	"fmt"

	"github.com/kentangtech/bot-order/internal/domain"
	"github.com/kentangtech/bot-order/internal/repository/postgres"
)

// TrialSpec carries the FR-07 provisioning limits (hours, quota, IP).
type TrialSpec struct {
	Hours     int
	TrafficGB int
	IPLimit   int
}

// CreateTrial provisions a free trial account (FR-07 AC-2):
// order type = trial, panel addClient with hour-based expiry, client row with
// is_trial=true — NO debit happens (trial is free).
func (s *Service) CreateTrial(ctx context.Context, user *postgres.User, serverID int64, spec TrialSpec) (*PurchaseResult, error) {
	// duration_days stays 0 for trials (the row stores hours in Notes so order
	// history/analytics never misread a 1-hour trial as "1 day").
	order := domain.NewOrder(s.newID(), domain.OrderTypeTrial, user.ID, serverID, "vless", 0, 0)
	order.TrafficGB, order.IPLimit = spec.TrafficGB, spec.IPLimit
	order.Notes = fmt.Sprintf("trial %d jam", spec.Hours)
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
	pc, err := s.panels.CreateTrialClient(ctx, serverID, email, order.Protocol, spec.Hours, int64(order.TrafficGB), int64(order.IPLimit))
	if err != nil {
		_ = order.MarkFailed(err.Error())
		_ = s.orders.Save(ctx, toOrderRow(order))
		return &PurchaseResult{OrderID: order.OrderID, Status: order.Status, ErrorMessage: err.Error()}, ErrFulfillFailed
	}

	client, err := domain.NewTrialClient(user.ID, serverID, pc.InboundID, email, order.Protocol, pc.UUID, pc.Password, spec.Hours, int64(order.TrafficGB), int64(order.IPLimit))
	if err != nil {
		_ = order.MarkFailed("gagal mencatat akun trial")
		_ = s.orders.Save(ctx, toOrderRow(order))
		return &PurchaseResult{OrderID: order.OrderID, Status: order.Status, ErrorMessage: err.Error()}, ErrFulfillFailed
	}
	row := toClientRow(client)
	if err := s.clients.Create(ctx, row); err != nil {
		_ = order.MarkFailed("gagal menyimpan akun trial")
		_ = s.orders.Save(ctx, toOrderRow(order))
		return &PurchaseResult{OrderID: order.OrderID, Status: order.Status, ErrorMessage: err.Error()}, ErrFulfillFailed
	}

	// Trial is free — no debit; balance stays unchanged (AC-2: hanya 1 jam).
	balanceAfter := user.Balance
	if err := order.Complete(balanceAfter); err != nil {
		return nil, err
	}
	order.AccountEmail = email
	order.ClientID = row.ID
	if err := s.orders.Save(ctx, toOrderRow(order)); err != nil {
		return nil, err
	}

	return &PurchaseResult{
		OrderID: order.OrderID, Status: order.Status, AccountEmail: email,
		BalanceAfter: balanceAfter, ServerID: serverID, ClientID: row.ID,
		NewExpiry: *client.ExpiresAt,
	}, nil
}
