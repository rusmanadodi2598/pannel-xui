// Package ordersvc implements the auto-order fulfillment (FR-03/FR-04/FR-05, M4).
//
// @file      internal/service/order/order.go
// @for       Purchase & renewal: state machine, panel provisioning, atomic debit.
// @uses      context, errors, fmt, strings, time, internal/domain, internal/repository/postgres
// @reason    Order state machine (pending→processing→completed|failed) with the
// atomicity rule: debit happens ONLY after panel success (FR-04 AC-1).
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
)

// OrderStore persists orders (postgres.OrderRepo implements it).
type OrderStore interface {
	Create(ctx context.Context, o *postgres.Order) error
	Save(ctx context.Context, o *postgres.Order) error
}

// ClientStore persists VPN clients (postgres.ClientRepo implements it).
type ClientStore interface {
	Create(ctx context.Context, c *postgres.VPNClient) error
	GetOwned(ctx context.Context, clientID, userID int64) (*postgres.VPNClient, error)
	UpdateExpiry(ctx context.Context, clientID int64, expiresAt time.Time, trafficLimit *int64) error
}

// UserStore debits balance atomically (postgres.UserRepo implements it).
type UserStore interface {
	Debit(ctx context.Context, userID int64, amount domain.Money, orderID string) (domain.Money, error)
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
type PanelGateway interface {
	CreateClient(ctx context.Context, serverID int64, email, protocol string, days int, trafficGB, ipLimit int64) (domain.PanelClient, error)
	CreateTrialClient(ctx context.Context, serverID int64, email, protocol string, hours int, trafficGB, ipLimit int64) (domain.PanelClient, error)
	RenewClient(ctx context.Context, serverID int64, clientID, email, protocol string, newExpiry time.Time) error
}

// Service orchestrates order creation, fulfillment and completion.
type Service struct {
	orders  OrderStore
	clients ClientStore
	users   UserStore
	plans   PlanReader
	servers ServerPicker
	panels  PanelGateway
	newID   func() string // injectable for tests; default domain.NewOrderID
}

// New builds the order service.
func New(o OrderStore, c ClientStore, u UserStore, p PlanReader, s ServerPicker, g PanelGateway) *Service {
	return &Service{orders: o, clients: c, users: u, plans: p, servers: s, panels: g, newID: domain.NewOrderID}
} // PurchaseResult summarizes a finished order for the user message.
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

// Purchase buys a new VPN account (FR-03/FR-04). Steps:
// 1. live price + server pick   2. create pending order
// 3. panel addClient (outside DB tx)   4. atomic debit + ledger
// 5. client row + order completed — all only after panel success.
func (s *Service) Purchase(ctx context.Context, user *postgres.User, country string, days int) (*PurchaseResult, error) {
	plan, err := s.plans.GetPlan(ctx, country, days)
	if err != nil {
		return nil, ErrPlanNotFound
	}
	serverID, err := s.servers.PickForCountry(ctx, country)
	if err != nil {
		return nil, ErrNoServer
	}
	if user.Balance < plan.Price {
		return nil, ErrInsufficientBalance
	}

	order := domain.NewOrder(s.newID(), domain.OrderTypePurchase, user.ID, serverID, "vless", days, plan.Price)
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
	pc, err := s.panels.CreateClient(ctx, serverID, email, order.Protocol, days, int64(order.TrafficGB), int64(order.IPLimit))
	if err != nil {
		_ = order.MarkFailed(err.Error())
		_ = s.orders.Save(ctx, toOrderRow(order))
		return &PurchaseResult{OrderID: order.OrderID, Status: order.Status, ErrorMessage: err.Error()}, ErrFulfillFailed
	}

	client, err := domain.NewVPNClient(user.ID, serverID, pc.InboundID, email, order.Protocol, pc.UUID, pc.Password, days, int64(order.TrafficGB), int64(order.IPLimit))
	if err != nil {
		_ = order.MarkFailed("gagal mencatat akun")
		_ = s.orders.Save(ctx, toOrderRow(order))
		return &PurchaseResult{OrderID: order.OrderID, Status: order.Status, ErrorMessage: err.Error()}, ErrFulfillFailed
	}
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

	return &PurchaseResult{
		OrderID: order.OrderID, Status: order.Status, AccountEmail: email,
		BalanceAfter: balanceAfter, Plan: plan, ServerID: serverID, ClientID: row.ID,
		NewExpiry: *client.ExpiresAt,
	}, nil
}

// Renew extends an existing account (FR-05). The expiry is computed from the
// remaining time (never double-counted) — base = max(now, current expiry).
func (s *Service) Renew(ctx context.Context, user *postgres.User, clientID int64, country string, days int) (*PurchaseResult, error) {
	client, err := s.clients.GetOwned(ctx, clientID, user.ID)
	if err != nil {
		return nil, ErrClientNotFound
	}
	plan, err := s.plans.GetPlan(ctx, country, days)
	if err != nil {
		return nil, ErrPlanNotFound
	}
	if user.Balance < plan.Price {
		return nil, ErrInsufficientBalance
	}

	base := time.Now()
	if client.ExpiresAt != nil && client.ExpiresAt.After(base) {
		base = *client.ExpiresAt
	}
	newExpiry := base.AddDate(0, 0, days)

	order := domain.NewOrder(s.newID(), domain.OrderTypeRenewal, user.ID, client.ServerID, client.Protocol, days, plan.Price)
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

	credential := client.UUID
	if credential == "" {
		credential = client.Password
	}
	if err := s.panels.RenewClient(ctx, client.ServerID, credential, client.Email, client.Protocol, newExpiry); err != nil {
		_ = order.MarkFailed(err.Error())
		_ = s.orders.Save(ctx, toOrderRow(order))
		return &PurchaseResult{OrderID: order.OrderID, Status: order.Status, ErrorMessage: err.Error()}, ErrFulfillFailed
	}

	balanceAfter, err := s.users.Debit(ctx, user.ID, plan.Price, order.OrderID)
	if err != nil {
		_ = order.MarkFailed("debit gagal, renewal panel perlu dirollback")
		_ = s.orders.Save(ctx, toOrderRow(order))
		return &PurchaseResult{OrderID: order.OrderID, Status: order.Status, ErrorMessage: err.Error()}, ErrFulfillFailed
	}
	if err := s.clients.UpdateExpiry(ctx, client.ID, newExpiry, nil); err != nil {
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

	return &PurchaseResult{
		OrderID: order.OrderID, Status: order.Status, AccountEmail: client.Email,
		BalanceAfter: balanceAfter, Plan: plan, ServerID: client.ServerID, ClientID: client.ID,
		NewExpiry: newExpiry,
	}, nil
}
