// Package topupsvc quotes QRIS fees and orchestrates PG charge topups (FR-06).
//
// @file      internal/service/topup/topup.go
// @for       FR-06 (Phase 4): fee math §15.7 + PG Aggregate charge lifecycle.
// @uses      context, errors, fmt, math, time, internal/domain, internal/repository/kts,
// internal/repository/postgres, gorm.io/gorm
// @reason    Persist dulu → create charge → confirm, pola debit-first Phase 3:
// the payment row exists before the gateway is touched, so the webhook always
// has the local orderId → user + NET map to credit (net diterima user, gross
// di-handle gateway — 015 §4.4).
// @author    Dodi Rusmana <rusmanadodi@kentangtechstore.com>
// @layer     service
// @stability experimental
// @since     2026-08-11
package topupsvc

import (
	"context"
	"errors"
	"fmt"
	"math"
	"time"

	"github.com/kentangtech/bot-order/internal/domain"
	"github.com/kentangtech/bot-order/internal/repository/kts"
	"github.com/kentangtech/bot-order/internal/repository/postgres"
	"gorm.io/gorm"
)

// ErrInvalidNominal is returned when the net amount is out of the allowed range.
var ErrInvalidNominal = errors.New("nominal di luar batas")

// Quote is one fee calculation for a chosen NET amount (PRD §15.7).
type Quote struct {
	Net        domain.Money // saldo bersih yang dikredit ke user
	Gross      domain.Money // total yang dibayar user (sudah + fee)
	TotalFee   domain.Money // qris_fee + ppn_fee
	FeePercent float64      // effective rate, e.g. 0.02775
}

// CreatePaymentRequest is what the gateway flow needs to create a PG charge.
type CreatePaymentRequest struct {
	TelegramUserID int64
	FirstName      string
	Username       string
	NetAmount      domain.Money
	GrossAmount    domain.Money
}

// PaymentResult is the created QRIS payment (contract §15.7).
type PaymentResult struct {
	OrderID     string
	CheckoutURL string
	Amount      domain.Money // gross yang dibayar user (display)
	ExpiresAt   time.Time
}

// PaymentGateway creates and confirms PG charges (kts.Client implements it).
// Amount yang dikirim = NET; gross-up (2.5% MDR + 11% PPN) di-handle gateway.
type PaymentGateway interface {
	CreateCharge(ctx context.Context, req kts.CreateChargeRequest) (*kts.Charge, error)
	ConfirmCharge(ctx context.Context, orderID string) (*kts.Charge, error)
}

// PaymentStore persists topup payment rows (postgres.PaymentRepo implements it).
type PaymentStore interface {
	Create(ctx context.Context, p *postgres.Payment) error
	GetByOrderID(ctx context.Context, orderID string) (*postgres.Payment, error)
	SaveProviderRef(ctx context.Context, orderID, providerRef string) error
	MarkFailed(ctx context.Context, orderID, reason string) error
	MarkSettledTx(ctx context.Context, tx *gorm.DB, orderID, status string, paidAt *time.Time) (bool, error)
}

// UserLedger resolves users and credits balance atomically inside the
// settlement transaction (postgres.UserRepo implements it).
type UserLedger interface {
	FindOrCreate(ctx context.Context, tgID int64, username, firstName string) (*postgres.User, error)
	WithTx(ctx context.Context, fn func(tx *gorm.DB) error) error
	CreditTx(ctx context.Context, tx *gorm.DB, userID int64, amount domain.Money, orderID string) (domain.Money, error)
}

// Service quotes fees and orchestrates the PG charge lifecycle.
type Service struct {
	gateway  PaymentGateway
	payments PaymentStore
	users    UserLedger
	notify   TopupNotifier // best-effort settlement notice (nil = disabled)
	feePct   float64
	ppnPct   float64
	minNet   domain.Money
	maxNet   domain.Money
	now      func() time.Time // injectable for tests; default time.Now
}

// New builds the topup service. notify is optional (variadic, same pattern as
// ordersvc.New): pass a TopupNotifier to enable settlement notices.
func New(gateway PaymentGateway, payments PaymentStore, users UserLedger, feePct, ppnPct float64, minNet, maxNet domain.Money, notify ...TopupNotifier) *Service {
	svc := &Service{gateway: gateway, payments: payments, users: users, feePct: feePct, ppnPct: ppnPct, minNet: minNet, maxNet: maxNet, now: time.Now}
	if len(notify) > 0 && notify[0] != nil {
		svc.notify = notify[0]
	}
	return svc
}

// Quote computes gross/fee from the user-chosen NET amount (PRD §15.7):
//
//	effective_rate = fee * (1 + ppn)
//	gross          = ceil(net / (1 - effective_rate)) → rounded up to ×100
//	total_fee      = gross - net
func (s *Service) Quote(net domain.Money) (Quote, error) {
	if net < s.minNet || net > s.maxNet {
		return Quote{}, fmt.Errorf("%w: min %d, maks %d", ErrInvalidNominal, s.minNet, s.maxNet)
	}
	effective := s.feePct * (1 + s.ppnPct)
	grossFloat := math.Ceil(float64(net) / (1 - effective))
	gross := domain.Money(math.Ceil(grossFloat/100) * 100)
	return Quote{
		Net:        net,
		Gross:      gross,
		TotalFee:   gross - net,
		FeePercent: effective,
	}, nil
}

// CreatePayment runs the FR-06 charge flow: persist the payment row BEFORE the
// gateway is touched (pola Phase 3), create the charge, then confirm it to get
// the checkout URL. A gateway failure marks the row failed (no zombie pending)
// and returns the error — the menu shows the unavailable text.
func (s *Service) CreatePayment(ctx context.Context, req CreatePaymentRequest) (*PaymentResult, error) {
	u, err := s.users.FindOrCreate(ctx, req.TelegramUserID, req.Username, req.FirstName)
	if err != nil {
		return nil, fmt.Errorf("resolving user: %w", err)
	}
	orderID := newTopupOrderID()
	row := &postgres.Payment{
		OrderID:     orderID,
		UserID:      u.ID,
		TelegramID:  u.TelegramID,
		AmountNet:   req.NetAmount,
		AmountGross: req.GrossAmount,
		FeeAmount:   req.GrossAmount - req.NetAmount,
		FeePct:      s.feePct * (1 + s.ppnPct),
		Status:      postgres.PaymentStatusPending,
	}
	if err := s.payments.Create(ctx, row); err != nil {
		return nil, fmt.Errorf("persisting topup payment: %w", err)
	}

	if _, err := s.gateway.CreateCharge(ctx, kts.CreateChargeRequest{
		OrderID:     orderID,
		Amount:      kts.Money{Amount: req.NetAmount.Rupiah(), Currency: "IDR"},
		Description: "Top up saldo bot — " + req.FirstName,
	}); err != nil {
		_ = s.payments.MarkFailed(ctx, orderID, err.Error())
		return nil, fmt.Errorf("creating pg charge: %w", err)
	}
	confirm, err := s.gateway.ConfirmCharge(ctx, orderID)
	if err != nil {
		_ = s.payments.MarkFailed(ctx, orderID, err.Error())
		return nil, fmt.Errorf("confirming pg charge: %w", err)
	}
	_ = s.payments.SaveProviderRef(ctx, orderID, confirm.ProviderReference)

	return &PaymentResult{
		OrderID:     orderID,
		CheckoutURL: confirm.CheckoutURL,
		Amount:      req.GrossAmount,
		ExpiresAt:   confirm.ExpiresAt,
	}, nil
}

// MinNet returns the smallest accepted net nominal (custom prompt copy).
func (s *Service) MinNet() domain.Money { return s.minNet }

// MaxNet returns the largest accepted net nominal (custom prompt copy).
func (s *Service) MaxNet() domain.Money { return s.maxNet }

// newTopupOrderID builds the single E2E orderId for the gateway: tp_<hex> —
// charset [A-Za-z0-9._-], 4..50, no ':' (015 §2.1, travels in X-Webhook-Id).
func newTopupOrderID() string {
	return "tp_" + domain.NewSecret(10)
}
