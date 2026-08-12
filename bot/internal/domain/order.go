// Package domain holds entities and value objects (DDD, AGENTS.md §2.2).
//
// @file      internal/domain/order.go
// @for       Order entity + OrderStatus state machine (PRD FR-04).
// @uses      fmt, strings
// @reason    Encapsulates order transitions so services cannot reach invalid states.
// @author    Dodi Rusmana <rusmanadodi@kentangtechstore.com>
// @layer     domain
// @stability stable
// @since     2026-08-11
package domain

import (
	"fmt"
	"strings"
	"time"
)

// OrderStatus is the order lifecycle state (PRD §13.4 / FR-04).
type OrderStatus string

const (
	OrderPending    OrderStatus = "pending"    // dibuat, menunggu fulfillment
	OrderProcessing OrderStatus = "processing" // XUI operation in flight
	OrderCompleted  OrderStatus = "completed"  // akun dibuat, saldo didebit
	OrderFailed     OrderStatus = "failed"     // error panel, saldo TIDAK didebit
	OrderCancelled  OrderStatus = "cancelled"  // dibatalkan user (input FSM)
	OrderRefunded   OrderStatus = "refunded"   // dikembalikan (PRD §13.4; label defensif — tidak diproduksi saat ini)
)

// OrderType discriminates the order kind (§13.4).
type OrderType string

const (
	OrderTypePurchase OrderType = "purchase"
	OrderTypeRenewal  OrderType = "renewal"
	OrderTypeTopup    OrderType = "topup"
	OrderTypeTrial    OrderType = "trial"
	// OrderTypeDeletion records an account deletion (FR-08 AC-4): a
	// zero-amount, completed order row so the action shows in Riwayat (FR-14).
	OrderTypeDeletion OrderType = "deletion"
)

// TransitionResult describes a valid state move.
type TransitionResult struct {
	From OrderStatus
	To   OrderStatus
}

// Order aggregates the order state machine (rich domain model, AGENTS.md §2.2).
type Order struct {
	ID            int64
	OrderID       string // KTS-XXXXXXXX-VPN
	Type          OrderType
	UserID        int64
	ServerID      int64
	ClientID      int64
	Protocol      string
	DurationDays  int
	TrafficGB     int
	IPLimit       int
	Amount        Money
	Discount      Money
	FinalAmount   Money
	Currency      string
	Status        OrderStatus
	Notes         string
	ErrorMessage  string
	AccountEmail  string
	BalanceBefore Money
	BalanceAfter  Money
	CompletedAt   *time.Time
	CreatedAt     time.Time
}

// NewOrder builds a pending order with a generated public order ID.
func NewOrder(orderID string, typ OrderType, userID, serverID int64, protocol string, days int, amount Money) *Order {
	return &Order{
		OrderID:      orderID,
		Type:         typ,
		UserID:       userID,
		ServerID:     serverID,
		Protocol:     protocol,
		DurationDays: days,
		Amount:       amount,
		Discount:     0,
		FinalAmount:  amount,
		Currency:     "IDR",
		Status:       OrderPending,
		CreatedAt:    time.Now(),
	}
}

// NewDeletionRecord builds the account-deletion history row (FR-08 AC-4).
// Deletions have no fulfillment lifecycle and no balance movement — the
// factory creates the record already completed (initial state, not a
// transition) so it lands in the user's Riwayat without touching the FSM.
func NewDeletionRecord(orderID string, userID, serverID int64, protocol, email string) *Order {
	now := time.Now()
	return &Order{
		OrderID:      orderID,
		Type:         OrderTypeDeletion,
		UserID:       userID,
		ServerID:     serverID,
		Protocol:     protocol,
		AccountEmail: email,
		Currency:     "IDR",
		Status:       OrderCompleted,
		CompletedAt:  &now,
		CreatedAt:    now,
	}
}

// Complete marks the order completed and records the resulting balance.
func (o *Order) Complete(balanceAfter Money) error {
	if err := o.Transition(OrderCompleted); err != nil {
		return err
	}
	now := time.Now()
	o.BalanceAfter = balanceAfter
	o.CompletedAt = &now
	return nil
}

// Transition moves the order to a new state, rejecting invalid moves.
// Valid: pending→processing|failed|cancelled, processing→completed|failed.
func (o *Order) Transition(to OrderStatus) error {
	if !validTransition(o.Status, to) {
		return fmt.Errorf("invalid order transition %s → %s", o.Status, to)
	}
	o.Status = to
	return nil
}

// MarkFailed records the error and moves to failed (saldo tidak didebit).
func (o *Order) MarkFailed(errMsg string) error {
	if err := o.Transition(OrderFailed); err != nil {
		return err
	}
	o.ErrorMessage = errMsg
	return nil
}

// IsTerminal reports whether the order reached a final state.
func (o *Order) IsTerminal() bool {
	return o.Status == OrderCompleted || o.Status == OrderFailed || o.Status == OrderCancelled
}

func validTransition(from, to OrderStatus) bool {
	switch from {
	case OrderPending:
		return to == OrderProcessing || to == OrderFailed || to == OrderCancelled
	case OrderProcessing:
		return to == OrderCompleted || to == OrderFailed
	default:
		return false
	}
}

// ParseOrderType maps a raw string to OrderType (invalid → error).
func ParseOrderType(raw string) (OrderType, error) {
	switch OrderType(strings.ToLower(raw)) {
	case OrderTypePurchase:
		return OrderTypePurchase, nil
	case OrderTypeRenewal:
		return OrderTypeRenewal, nil
	case OrderTypeTopup:
		return OrderTypeTopup, nil
	case OrderTypeTrial:
		return OrderTypeTrial, nil
	case OrderTypeDeletion:
		return OrderTypeDeletion, nil
	default:
		return "", fmt.Errorf("unknown order type %q", raw)
	}
}
