// Package topupsvc quotes QRIS fees and delegates payment creation.
//
// @file      internal/service/topup/topup.go
// @for       FR-06 (M5 partial): fee math §15.7 + PaymentGateway seam.
// @uses      context, errors, fmt, math, time, internal/domain
// @reason    The KentangTech payment API is being rewritten to Go — the bot
// only depends on the PaymentGateway interface; the real client is
// swapped in without touching menus/flows (product decision).
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
)

// ErrPaymentAPIUnavailable is returned by the stub gateway until the rewritten
// KentangTech Go API ships (product decision — menus live, API deferred).
var ErrPaymentAPIUnavailable = errors.New("payment API belum tersedia")

// ErrInvalidNominal is returned when the net amount is out of the allowed range.
var ErrInvalidNominal = errors.New("nominal di luar batas")

// Quote is one fee calculation for a chosen NET amount (PRD §15.7).
type Quote struct {
	Net        domain.Money // saldo bersih yang dikredit ke user
	Gross      domain.Money // total yang dibayar user (sudah + fee)
	TotalFee   domain.Money // qris_fee + ppn_fee
	FeePercent float64      // effective rate, e.g. 0.02775
}

// CreatePaymentRequest is what the (future) API client needs to create a QRIS.
type CreatePaymentRequest struct {
	TelegramUserID int64
	FirstName      string
	Username       string
	NetAmount      domain.Money
	GrossAmount    domain.Money
}

// PaymentResult is the created QRIS payment (contract §15.7).
type PaymentResult struct {
	OrderID   string
	QRString  string
	Amount    domain.Money
	ExpiresAt time.Time
}

// PaymentGateway creates payments on the KentangTech API. Implemented by the
// real HTTP client once the rewritten Go API ships; StubGateway now.
type PaymentGateway interface {
	CreatePayment(ctx context.Context, req CreatePaymentRequest) (*PaymentResult, error)
}

// StubGateway answers ErrPaymentAPIUnavailable — the API rewrite is in flight
// (product decision: build menus now, wire the client later, no rewrite).
type StubGateway struct{}

// CreatePayment always reports the API is not yet available.
func (StubGateway) CreatePayment(context.Context, CreatePaymentRequest) (*PaymentResult, error) {
	return nil, ErrPaymentAPIUnavailable
}

// Service quotes fees and delegates payment creation.
type Service struct {
	gateway PaymentGateway
	feePct  float64
	ppnPct  float64
	minNet  domain.Money
	maxNet  domain.Money
}

// New builds the topup service. gateway may be StubGateway{} while the API is
// being rewritten.
func New(gateway PaymentGateway, feePct, ppnPct float64, minNet, maxNet domain.Money) *Service {
	return &Service{gateway: gateway, feePct: feePct, ppnPct: ppnPct, minNet: minNet, maxNet: maxNet}
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

// CreatePayment delegates to the gateway (stub for now).
func (s *Service) CreatePayment(ctx context.Context, req CreatePaymentRequest) (*PaymentResult, error) {
	return s.gateway.CreatePayment(ctx, req)
}

// MinNet returns the smallest accepted net nominal (custom prompt copy).
func (s *Service) MinNet() domain.Money { return s.minNet }

// MaxNet returns the largest accepted net nominal (custom prompt copy).
func (s *Service) MaxNet() domain.Money { return s.maxNet }
