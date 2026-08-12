// Package config also hosts the trial-cleanup settings (split for §1.1 line limit).
//
// @file      internal/config/trialcleanup.go
// @for       Trial-cleanup worker settings: enabled, sweep interval, batch size.
// @uses      fmt, time
// @reason    Keeps config.go under 250 lines while centralizing the trial
// cleanup tuning (FR-07: trial window is 1 hour, so a 15-minute sweep bounds
// how long an expired trial stays enabled on the panel).
// @author    Dodi Rusmana <rusmanadodi@kentangtechstore.com>
// @layer     config
// @stability stable
// @since     2026-08-12
package config

import (
	"fmt"
	"time"
)

// Trial-cleanup worker defaults (PRD worker list; trial window = 1 jam FR-07).
const (
	DefaultTrialCleanupEnabled     = true
	DefaultTrialCleanupIntervalMin = 15
	DefaultTrialCleanupBatch       = 50
)

// applyTrialCleanup parses TRIAL_CLEANUP_* env vars into the config.
func (c *Config) applyTrialCleanup() error {
	var err error
	if c.TrialCleanupEnabled, err = parseBoolEnv("TRIAL_CLEANUP_ENABLED", DefaultTrialCleanupEnabled); err != nil {
		return err
	}
	intervalMin, err := parseIntEnv("TRIAL_CLEANUP_INTERVAL_MIN", DefaultTrialCleanupIntervalMin)
	if err != nil {
		return err
	}
	c.TrialCleanupInterval = time.Duration(intervalMin) * time.Minute
	if c.TrialCleanupBatch, err = parseIntEnv("TRIAL_CLEANUP_BATCH", DefaultTrialCleanupBatch); err != nil {
		return err
	}
	return nil
}

// validateTrialCleanup enforces the trial-cleanup invariants.
func (c *Config) validateTrialCleanup() error {
	if c.TrialCleanupInterval <= 0 || c.TrialCleanupInterval > 24*time.Hour {
		return fmt.Errorf("TRIAL_CLEANUP_INTERVAL_MIN out of range 1-1440: %v", c.TrialCleanupInterval)
	}
	if c.TrialCleanupBatch <= 0 {
		return fmt.Errorf("TRIAL_CLEANUP_BATCH must be positive: %d", c.TrialCleanupBatch)
	}
	return nil
}
