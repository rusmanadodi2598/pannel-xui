// Package domain holds entities and value objects (DDD, AGENTS.md §2.2).
//
// @file      internal/domain/plan.go
// @for       VpnPlan value object (country, duration days, price) + display helpers.
// @uses      fmt, strings
// @reason    Keeps plan naming/formatting consistent across buy & renew views.
// @author    Dodi Rusmana <rusmanadodi@kentangtechstore.com>
// @layer     domain
// @stability stable
// @since     2026-08-11
package domain

import (
	"fmt"
	"strings"
)

// CountryNames maps ISO country codes to the Indonesian display name.
var CountryNames = map[string]string{
	"ID": "Indonesia",
	"SG": "Singapore",
	"JP": "Japan",
	"CN": "China",
	"US": "United States",
	"DE": "Germany",
	"NL": "Netherlands",
}

// VpnPlan is an immutable sellable plan (one pricing row).
type VpnPlan struct {
	CountryCode string
	CountryName string
	Days        int
	Price       Money
	Enabled     bool
}

// NewVpnPlan validates and builds a plan.
func NewVpnPlan(countryCode string, days int, price Money, enabled bool) (*VpnPlan, error) {
	if strings.TrimSpace(countryCode) == "" {
		return nil, fmt.Errorf("plan country code is required")
	}
	if days <= 0 {
		return nil, fmt.Errorf("plan days must be positive: %d", days)
	}
	name, ok := CountryNames[strings.ToUpper(countryCode)]
	if !ok {
		name = countryCode
	}
	return &VpnPlan{CountryCode: strings.ToUpper(countryCode), CountryName: name, Days: days, Price: price, Enabled: enabled}, nil
}

// Name renders "VPN Indonesia 30 Hari" (parity with the pricing seed JSON).
func (p *VpnPlan) Name() string {
	return fmt.Sprintf("VPN %s %d Hari", p.CountryName, p.Days)
}

// Code renders "ID30" (parity with the pricing seed JSON).
func (p *VpnPlan) Code() string {
	return fmt.Sprintf("%s%d", p.CountryCode, p.Days)
}

// ValidityLabel renders "30 Hari".
func (p *VpnPlan) ValidityLabel() string {
	return fmt.Sprintf("%d Hari", p.Days)
}
