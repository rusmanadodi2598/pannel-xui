// Package pricingsvc keeps sellable plans in sync between seed file and DB.
//
// @file      internal/service/pricing/pricing.go
// @for       Seed pricing at boot + serve live enabled plans (PRD §13.7, FR-03).
// @uses      context, internal/domain, internal/repository/postgres
// @reason    Buy prices always come from the DB (seeded from JSON), never hardcoded (M4).
// @author    Dodi Rusmana <rusmanadodi@kentangtechstore.com>
// @layer     service
// @stability stable
// @since     2026-08-11
package pricingsvc

import (
	"context"
	"fmt"

	"github.com/kentangtech/bot-order/internal/domain"
	"github.com/kentangtech/bot-order/internal/repository/postgres"
)

// Store is the pricing persistence seam (postgres.PricingRepo implements it).
type Store interface {
	UpsertMany(ctx context.Context, rows []postgres.Pricing) error
	ListEnabled(ctx context.Context) ([]postgres.Pricing, error)
	GetPlan(ctx context.Context, country string, days int) (*postgres.Pricing, error)
}

// Service orchestrates seeding and live price reads.
type Service struct {
	store  Store
	seeder Seeder
}

// New builds the pricing service. seeder may be nil in tests.
func New(store Store, seeder Seeder) *Service { return &Service{store: store, seeder: seeder} }

// EnsureSeeded applies the seed file idempotently at boot (PRD §13.7).
func (s *Service) EnsureSeeded(ctx context.Context) error {
	if s.seeder == nil {
		return nil
	}
	rows, err := s.seeder.Load()
	if err != nil {
		return fmt.Errorf("pricing seed: %w", err)
	}
	if err := s.store.UpsertMany(ctx, rows); err != nil {
		return fmt.Errorf("pricing seed: %w", err)
	}
	return nil
}

// ListEnabled returns all enabled plans grouped for the buy menu (FR-03).
func (s *Service) ListEnabled(ctx context.Context) ([]domain.VpnPlan, error) {
	rows, err := s.store.ListEnabled(ctx)
	if err != nil {
		return nil, err
	}
	plans := make([]domain.VpnPlan, 0, len(rows))
	for i := range rows {
		plans = append(plans, *rows[i].ToPlan())
	}
	return plans, nil
}

// GetPlan returns one live enabled plan; the price is authoritative for orders.
func (s *Service) GetPlan(ctx context.Context, country string, days int) (*domain.VpnPlan, error) {
	row, err := s.store.GetPlan(ctx, country, days)
	if err != nil {
		return nil, err
	}
	return row.ToPlan(), nil
}
