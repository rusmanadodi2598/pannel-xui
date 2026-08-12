// Package config also hosts cross-field validation (split for §1.1 line limit).
//
// @file      internal/config/validate.go
// @for       validate(): enforces every cross-field invariant of the configuration.
// @uses      fmt, strings
// @reason    Keeps config.go under 250 lines while centralizing fail-fast checks.
// @author    Dodi Rusmana <rusmanadodi@kentangtechstore.com>
// @layer     config
// @stability stable
// @since     2026-08-11
package config

import (
	"fmt"
	"strings"
	"time"
)

// validate enforces the cross-field invariants of the configuration.
func (c *Config) validate() error {
	if c.BotToken == "" {
		return fmt.Errorf("BOT_TOKEN is required")
	}
	if len(c.WebhookSecret) < 32 {
		return fmt.Errorf("WEBHOOK_SECRET must be at least 32 characters (PRD §14.1)")
	}
	if c.BotDomain == "" {
		return fmt.Errorf("BOT_DOMAIN is required for the Telegram webhook (HTTPS)")
	}
	if c.DatabaseURL == "" {
		return fmt.Errorf("DATABASE_URL is required")
	}
	if c.RedisURL == "" {
		return fmt.Errorf("REDIS_URL is required")
	}
	if c.WebhookPort <= 0 || c.WebhookPort > 65535 {
		return fmt.Errorf("WEBHOOK_PORT out of range: %d", c.WebhookPort)
	}
	if !strings.HasPrefix(c.WebhookPath, "/") {
		return fmt.Errorf("WEBHOOK_PATH must start with '/': %q", c.WebhookPath)
	}
	if c.WebhookMaxConnections < 1 || c.WebhookMaxConnections > 100 {
		return fmt.Errorf("WEBHOOK_MAX_CONNECTIONS out of range 1-100: %d", c.WebhookMaxConnections)
	}
	if c.WebhookWorkers <= 0 || c.WebhookWorkers > 256 {
		return fmt.Errorf("WEBHOOK_WORKERS out of range 1-256: %d", c.WebhookWorkers)
	}
	if c.WebhookQueueBuffer <= 0 {
		return fmt.Errorf("WEBHOOK_QUEUE_BUFFER must be positive: %d", c.WebhookQueueBuffer)
	}
	if c.RateLimitRequests <= 0 {
		return fmt.Errorf("RATE_LIMIT_REQUESTS must be positive: %d", c.RateLimitRequests)
	}
	if c.MinTopupAmount < 0 || c.MaxTopupAmount < c.MinTopupAmount {
		return fmt.Errorf("invalid topup amount range: min=%d max=%d", c.MinTopupAmount, c.MaxTopupAmount)
	}
	if c.QRISFeePercent <= 0 || c.QRISFeePercent >= 1 || c.QRISPPNPercent < 0 {
		return fmt.Errorf("invalid QRIS fee config: fee=%v ppn=%v", c.QRISFeePercent, c.QRISPPNPercent)
	}
	if c.QRISExpiryMinutes <= 0 {
		return fmt.Errorf("QRIS_EXPIRY_MINUTES must be positive: %d", c.QRISExpiryMinutes)
	}
	if c.XUIAPITimeout <= 0 {
		return fmt.Errorf("XUI_API_TIMEOUT must be positive: %v", c.XUIAPITimeout)
	}
	if c.DBMaxOpenConns <= 0 || c.DBMaxIdleConns <= 0 {
		return fmt.Errorf("DB pool must be positive: open=%d idle=%d", c.DBMaxOpenConns, c.DBMaxIdleConns)
	}
	if c.DBMaxIdleConns > c.DBMaxOpenConns {
		return fmt.Errorf("DB_MAX_IDLE_CONNS (%d) cannot exceed DB_MAX_OPEN_CONNS (%d)", c.DBMaxIdleConns, c.DBMaxOpenConns)
	}
	if c.DBConnMaxLifetime <= 0 {
		return fmt.Errorf("DB_CONN_MAX_LIFETIME_MIN must be positive: %v", c.DBConnMaxLifetime)
	}
	if c.RedisPoolSize <= 0 {
		return fmt.Errorf("REDIS_POOL_SIZE must be positive: %d", c.RedisPoolSize)
	}
	if c.RedisDialTimeout <= 0 {
		return fmt.Errorf("REDIS_DIAL_TIMEOUT_SEC must be positive: %v", c.RedisDialTimeout)
	}
	if len(c.ExpiryNotifyDays) == 0 {
		return fmt.Errorf("EXPIRY_NOTIFY_DAYS must contain at least one day")
	}
	for _, d := range c.ExpiryNotifyDays {
		if d <= 0 {
			return fmt.Errorf("EXPIRY_NOTIFY_DAYS contains non-positive day: %d", d)
		}
	}
	if c.TrialDailyLimit <= 0 {
		return fmt.Errorf("TRIAL_DAILY_LIMIT must be positive: %d", c.TrialDailyLimit)
	}
	if c.TrialDurationHours <= 0 {
		return fmt.Errorf("TRIAL_DURATION_HOURS must be positive: %d", c.TrialDurationHours)
	}
	if c.TrialTrafficGB <= 0 {
		return fmt.Errorf("TRIAL_TRAFFIC_GB must be positive: %d", c.TrialTrafficGB)
	}
	if c.TrialIPLimit <= 0 {
		return fmt.Errorf("TRIAL_IP_LIMIT must be positive: %d", c.TrialIPLimit)
	}
	if c.ExpiryNotifyInterval <= 0 || c.ExpiryNotifyInterval > 24*time.Hour {
		return fmt.Errorf("EXPIRY_NOTIFY_INTERVAL_MIN out of range 1-1440: %v", c.ExpiryNotifyInterval)
	}
	if c.ExpiryNotifyBatch <= 0 {
		return fmt.Errorf("EXPIRY_NOTIFY_BATCH must be positive: %d", c.ExpiryNotifyBatch)
	}
	if err := c.validateTrafficSync(); err != nil {
		return err
	}
	if err := c.validateHealthCheck(); err != nil {
		return err
	}
	if err := c.validateTrialCleanup(); err != nil {
		return err
	}
	if err := c.validateSubscription(); err != nil {
		return err
	}
	return nil
}
