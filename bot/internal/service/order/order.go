// Package ordersvc implements the auto-order fulfillment (FR-03/FR-04/FR-05, M4).
//
// @file      internal/service/order/order.go
// @for       Purchase & renewal: state machine, panel provisioning, atomic debit.
// @uses      context, errors, time, internal/domain, internal/repository/postgres
// @reason    Order state machine (pending→processing→completed|failed) with
// per-flow money invariants: purchase and renewal are both debit-first
// (prepare → row → debit → panel commit) and refund on failure (v1.47).
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

// Typed errors the telegram handler maps to friendly messages.
var (
	ErrInsufficientBalance = errors.New("saldo tidak cukup")
	ErrPlanNotFound        = errors.New("paket tidak ditemukan")
	ErrNoServer            = errors.New("tidak ada server untuk negara ini")
	ErrClientNotFound      = errors.New("akun tidak ditemukan")
	ErrFulfillFailed       = errors.New("gagal memproses di panel")
	// ErrTrialNotRenewable guards FR-05 (v1.37): renewal is paid-only.
	ErrTrialNotRenewable = errors.New("akun trial tidak dapat diperpanjang")
	// ErrOrderInFlight is the idempotence guard (v1.37): a duplicate execution
	// while an order of the same type is still pending/processing.
	ErrOrderInFlight = errors.New("order sebelumnya masih diproses")
)

// OrderStore persists orders (postgres.OrderRepo implements it).
type OrderStore interface {
	Create(ctx context.Context, o *postgres.Order) error
	Save(ctx context.Context, o *postgres.Order) error
	// FindInFlight returns the newest order of the given type for the user that
	// is still pending/processing, or nil when none is in flight (v1.37
	// idempotence guard against duplicate executions).
	FindInFlight(ctx context.Context, userID int64, orderType string) (*postgres.Order, error)
}

// ClientStore persists VPN clients (postgres.ClientRepo implements it).
// DeleteOwned is the failure cleanup of the debit-first purchase flow: a row
// inserted before the panel commit is deleted when the debit or the panel call
// fails (no orphaned row or account).
type ClientStore interface {
	Create(ctx context.Context, c *postgres.VPNClient) error
	GetOwned(ctx context.Context, clientID, userID int64) (*postgres.VPNClient, error)
	UpdateExpiry(ctx context.Context, clientID int64, expiresAt time.Time, trafficLimit *int64) error
	DeleteOwned(ctx context.Context, clientID, userID int64) error
}

// UserStore debits balance atomically (postgres.UserRepo implements it).
type UserStore interface {
	Debit(ctx context.Context, userID int64, amount domain.Money, orderID string) (domain.Money, error)
	// Credit restores balance atomically — the refund compensation of the
	// debit-first renewal flow (v1.37).
	Credit(ctx context.Context, userID int64, amount domain.Money, orderID string) (domain.Money, error)
}

// PlanReader reads live enabled plans (pricingsvc.Service implements it).
type PlanReader interface {
	GetPlan(ctx context.Context, country string, days int) (*domain.VpnPlan, error)
}

// ServerPicker selects the panel for a country (serversvc.Service implements it).
type ServerPicker interface {
	PickForCountry(ctx context.Context, country string) (int64, error)
}

// PanelGateway provisions clients on X-UI panels (serversvc.Service implements it).
// inboundID pins the exact panel inbound the user chose (0 = match by protocol).
// PrepareClient builds the record WITHOUT mutating the panel; the purchase flow
// persists the row and debits, then CommitClient runs addClient — a panel
// failure then only refunds + deletes the row (debit-first, no orphaned active
// account, parity renewal v1.37).
type PanelGateway interface {
	PrepareClient(ctx context.Context, serverID int64, inboundID int, email, protocol string, days int, trafficGB, ipLimit int64) (domain.PreparedClient, error)
	CommitClient(ctx context.Context, serverID int64, p domain.PreparedClient) error
	CreateTrialClient(ctx context.Context, serverID int64, inboundID int, email, protocol string, hours int, trafficGB, ipLimit int64) (domain.PanelClient, error)
	RenewClient(ctx context.Context, serverID int64, clientID, email, protocol string, newExpiry time.Time) error
}

// Service orchestrates order creation, fulfillment and completion.
type Service struct {
	orders   OrderStore
	clients  ClientStore
	users    UserStore
	plans    PlanReader
	servers  ServerPicker
	panels   PanelGateway
	newID    func() string // injectable for tests; default domain.NewOrderID
	notify   OrderNotifier // FR-04 AC admin-group notice (nil = disabled)
	subLinks SubLinks      // FR-13: panel sub server URLs (SetSubLinks at wiring)
}

// New builds the order service. The trailing notifier is optional (variadic
// so existing call sites and tests keep compiling): pass an OrderNotifier to
// enable the FR-04 AC admin-group notice, or nothing to disable it.
func New(o OrderStore, c ClientStore, u UserStore, p PlanReader, s ServerPicker, g PanelGateway, notify ...OrderNotifier) *Service {
	svc := &Service{orders: o, clients: c, users: u, plans: p, servers: s, panels: g, newID: domain.NewOrderID}
	if len(notify) > 0 && notify[0] != nil {
		svc.notify = notify[0]
	}
	return svc
}

// Renew lives in renew.go (v1.37 rewrite: paid-only, debit-first, auto-refund).
