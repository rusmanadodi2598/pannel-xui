// Package postgres also hosts the pricing repository.
//
// @file      internal/repository/postgres/pricing_repo.go
// @for       Idempotent pricing upsert (seed) + live price reads (PRD §13.7, FR-03 AC).
// @uses      context, fmt, gorm.io/gorm, internal/domain
// @reason    Buy price must always come from DB (never hardcoded), seeded at boot (M4).
// @author    Dodi Rusmana <rusmanadodi@kentangtechstore.com>
// @layer     repository
// @stability stable
// @since     2026-08-11
package postgres

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/kentangtech/bot-order/internal/domain"
	"gorm.io/gorm"
)

// ErrPlanNotFound is returned when no enabled pricing row matches (country, days).
var ErrPlanNotFound = errors.New("plan not found")

// PricingRepo persists sellable plans.
type PricingRepo struct{ db *gorm.DB }

// NewPricingRepo builds the repository on the shared GORM handle.
func NewPricingRepo(db *gorm.DB) *PricingRepo { return &PricingRepo{db: db} }

// UpsertMany upserts seed rows on UNIQUE (country_code, plan_days) (idempotent).
// Per-row select is an N+1 but deliberately bounded: the seed file ships ≤ 12
// plans (AGENTS.md §1.7 small-N tradeoff documented); boot-time only.
func (r *PricingRepo) UpsertMany(ctx context.Context, rows []Pricing) error {
	if len(rows) == 0 {
		return nil
	}
	for _, row := range rows {
		var existing Pricing
		err := r.db.WithContext(ctx).
			Where("country_code = ? AND plan_days = ?", row.CountryCode, row.PlanDays).
			First(&existing).Error
		switch {
		case err == nil:
			if existing.Price != row.Price || existing.Enabled != row.Enabled {
				if uerr := r.db.WithContext(ctx).Model(&existing).
					Updates(map[string]any{"price": row.Price, "enabled": row.Enabled, "updated_at": time.Now()}).
					Error; uerr != nil {
					return fmt.Errorf("updating pricing %s%d: %w", row.CountryCode, row.PlanDays, uerr)
				}
			}
		case errors.Is(err, gorm.ErrRecordNotFound):
			row.ID = 0
			if cerr := r.db.WithContext(ctx).Create(&row).Error; cerr != nil {
				return fmt.Errorf("inserting pricing %s%d: %w", row.CountryCode, row.PlanDays, cerr)
			}
		default:
			return fmt.Errorf("reading pricing: %w", err)
		}
	}
	return nil
}

// ListEnabled returns all enabled plans (buy menu, FR-03).
func (r *PricingRepo) ListEnabled(ctx context.Context) ([]Pricing, error) {
	var rows []Pricing
	if err := r.db.WithContext(ctx).Where("enabled = true").Order("country_code, plan_days").Find(&rows).Error; err != nil {
		return nil, fmt.Errorf("listing pricing: %w", err)
	}
	return rows, nil
}

// GetPlan returns one enabled plan; ErrPlanNotFound when absent/disabled.
func (r *PricingRepo) GetPlan(ctx context.Context, country string, days int) (*Pricing, error) {
	var row Pricing
	err := r.db.WithContext(ctx).
		Where("country_code = ? AND plan_days = ? AND enabled = true", country, days).
		First(&row).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, ErrPlanNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("getting plan %s%d: %w", country, days, err)
	}
	return &row, nil
}

// ToPlan maps a pricing row to the domain VpnPlan value object.
func (p *Pricing) ToPlan() *domain.VpnPlan {
	return &domain.VpnPlan{
		CountryCode: p.CountryCode,
		CountryName: countryName(p.CountryCode),
		Days:        p.PlanDays,
		Price:       p.Price,
		Enabled:     p.Enabled,
	}
}

func countryName(code string) string {
	if n, ok := domain.CountryNames[code]; ok {
		return n
	}
	return code
}
