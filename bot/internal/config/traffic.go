// Package config also hosts the traffic-sync settings (split for §1.1 line limit).
//
// @file      internal/config/traffic.go
// @for       Traffic-sync worker settings: enabled, sweep interval, batch size.
// @uses      fmt, time
// @reason    Keeps config.go under 250 lines while centralizing PRD §16.2 tuning.
// @author    Dodi Rusmana <rusmanadodi@kentangtechstore.com>
// @layer     config
// @stability stable
// @since     2026-08-11
package config

import (
	"fmt"
	"time"
)

// Traffic-sync worker defaults (PRD §16.2: tiap 5–10 mnt).
const (
	DefaultTrafficSyncEnabled     = true
	DefaultTrafficSyncIntervalMin = 5
	DefaultTrafficSyncBatch       = 200
)

// applyTrafficSync parses TRAFFIC_SYNC_* env vars into the config.
func (c *Config) applyTrafficSync() error {
	var err error
	if c.TrafficSyncEnabled, err = parseBoolEnv("TRAFFIC_SYNC_ENABLED", DefaultTrafficSyncEnabled); err != nil {
		return err
	}
	intervalMin, err := parseIntEnv("TRAFFIC_SYNC_INTERVAL_MIN", DefaultTrafficSyncIntervalMin)
	if err != nil {
		return err
	}
	c.TrafficSyncInterval = time.Duration(intervalMin) * time.Minute
	if c.TrafficSyncBatch, err = parseIntEnv("TRAFFIC_SYNC_BATCH", DefaultTrafficSyncBatch); err != nil {
		return err
	}
	return nil
}

// validateTrafficSync enforces the traffic-sync invariants.
func (c *Config) validateTrafficSync() error {
	if c.TrafficSyncInterval <= 0 || c.TrafficSyncInterval > 60*time.Minute {
		return fmt.Errorf("TRAFFIC_SYNC_INTERVAL_MIN out of range 1-60: %v", c.TrafficSyncInterval)
	}
	if c.TrafficSyncBatch <= 0 {
		return fmt.Errorf("TRAFFIC_SYNC_BATCH must be positive: %d", c.TrafficSyncBatch)
	}
	return nil
}
