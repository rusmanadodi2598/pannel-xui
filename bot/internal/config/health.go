// Package config also hosts the health-check settings (split for §1.1 line limit).
//
// @file      internal/config/health.go
// @for       Health-check worker settings: enabled, ping interval.
// @uses      fmt, time
// @reason    Keeps config.go under 250 lines while centralizing PRD §17 tuning
// (server mati tidak dijual — dead panels must be marked down promptly).
// @author    Dodi Rusmana <rusmanadodi@kentangtechstore.com>
// @layer     config
// @stability stable
// @since     2026-08-12
package config

import (
	"fmt"
	"time"
)

// Health-check worker defaults (PRD §17: tiap 60 detik).
const (
	DefaultHealthCheckEnabled     = true
	DefaultHealthCheckIntervalSec = 60
)

// applyHealthCheck parses HEALTH_CHECK_* env vars into the config.
func (c *Config) applyHealthCheck() error {
	var err error
	if c.HealthCheckEnabled, err = parseBoolEnv("HEALTH_CHECK_ENABLED", DefaultHealthCheckEnabled); err != nil {
		return err
	}
	intervalSec, err := parseIntEnv("HEALTH_CHECK_INTERVAL_SEC", DefaultHealthCheckIntervalSec)
	if err != nil {
		return err
	}
	c.HealthCheckInterval = time.Duration(intervalSec) * time.Second
	return nil
}

// validateHealthCheck enforces the health-check invariants.
func (c *Config) validateHealthCheck() error {
	if c.HealthCheckInterval <= 0 || c.HealthCheckInterval > 1*time.Hour {
		return fmt.Errorf("HEALTH_CHECK_INTERVAL_SEC out of range 1-3600: %v", c.HealthCheckInterval)
	}
	return nil
}
