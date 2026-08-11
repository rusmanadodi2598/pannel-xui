// Package pricingsvc also hosts the seed-file loader.
//
// @file      internal/service/pricing/seed.go
// @for       Parse PRICING_SEED_FILE JSON (categories by country) into pricing rows.
// @uses      os, encoding/json, fmt, internal/repository/postgres
// @reason    The seed JSON shape mirrors the reference (price/validity/code) — parsed once.
// @author    Dodi Rusmana <rusmanadodi@kentangtechstore.com>
// @layer     service
// @stability stable
// @since     2026-08-11
package pricingsvc

import (
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/kentangtech/bot-order/internal/domain"
	"github.com/kentangtech/bot-order/internal/repository/postgres"
)

// Seeder loads pricing rows from the JSON seed file (injectable for tests).
type Seeder interface {
	Load() ([]postgres.Pricing, error)
}

// FileSeeder reads the seed JSON from disk.
type FileSeeder struct{ Path string }

// Load parses seed/pricing.json into Upsert-ready rows.
func (f FileSeeder) Load() ([]postgres.Pricing, error) {
	raw, err := os.ReadFile(f.Path)
	if err != nil {
		return nil, fmt.Errorf("reading seed file %s: %w", f.Path, err)
	}
	var doc seedDocument
	if err := json.Unmarshal(raw, &doc); err != nil {
		return nil, fmt.Errorf("parsing seed file %s: %w", f.Path, err)
	}
	var rows []postgres.Pricing
	for country, items := range doc.Categories {
		for _, it := range items {
			if !it.IsActive {
				continue
			}
			rows = append(rows, postgres.Pricing{
				CountryCode: strings.ToUpper(country),
				PlanDays:    it.Days(),
				Price:       domain.Money(it.Price),
				Enabled:     true,
				UpdatedAt:   time.Now(),
			})
		}
	}
	if len(rows) == 0 {
		return nil, fmt.Errorf("seed file %s contains no active plans", f.Path)
	}
	return rows, nil
}

// seedDocument mirrors the user-provided pricing JSON schema.
type seedDocument struct {
	Currency   string                `json:"currency"`
	Categories map[string][]seedPlan `json:"categories"`
}

// seedPlan is one sellable item inside a country category.
type seedPlan struct {
	ID       int64  `json:"id"`
	Name     string `json:"name"`
	Operator string `json:"operator"`
	Code     string `json:"code"`
	Price    int64  `json:"price"`
	Fee      int64  `json:"fee"`
	Validity string `json:"validity"`
	IsActive bool   `json:"is_active"`
}

// Days extracts the duration from Validity ("15 Hari") or Code ("ID30").
func (p seedPlan) Days() int {
	n := parseLeadingInt(p.Validity)
	if n > 0 {
		return n
	}
	return parseLeadingInt(p.Code)
}

func parseLeadingInt(s string) int {
	var n int
	started := false
	for _, r := range s {
		if r >= '0' && r <= '9' {
			n = n*10 + int(r-'0')
			started = true
			continue
		}
		if started {
			break
		}
	}
	return n
}
