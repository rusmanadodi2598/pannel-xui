// Package config also hosts the default constants (split for §1.1 line limit).
//
// @file      internal/config/defaults.go
// @for       Every default value of the typed configuration (PRD §19.2).
// @uses      time
// @reason    Keeps config.go under 250 lines while centralizing defaults so a
// bare .env (only required fields) boots with sane, documented values.
// @author    Dodi Rusmana <rusmanadodi@kentangtechstore.com>
// @layer     config
// @stability stable
// @since     2026-08-12
package config

import "time"

// Defaults (PRD §19.2).
const (
	DefaultWebhookPort           = 8443
	DefaultWebhookPath           = "/api/v1/webhooks/telegram" // REST API convention (PRD §26)
	DefaultWebhookMaxConnections = 40
	DefaultWebhookWorkers        = 8
	DefaultWebhookQueueBuffer    = 64
	DefaultRateLimitRequests     = 30
	DefaultXUIAPITimeoutSec      = 30
	DefaultMinTopupAmount        = 5000
	DefaultMaxTopupAmount        = 5000000
	DefaultQRISFeePercent        = 0.025
	DefaultQRISPPNPercent        = 0.11
	DefaultQRISExpiryMinutes     = 15
	// PG charge validity window (015 §7.3) — the QRIS the user must pay.
	DefaultKTSChargeTTL = 24 * time.Hour
	DefaultTimeLocation = "Asia/Jakarta"
	DefaultLogLevel     = "info"

	// Trial policy (FR-07): 2 akun/hari, durasi 1 jam, kuota 1 GB, 1 IP.
	DefaultTrialEnabled       = true
	DefaultTrialDailyLimit    = 2
	DefaultTrialDurationHours = 1
	DefaultTrialTrafficGB     = 1
	DefaultTrialIPLimit       = 1

	// Expiry reminders (FR-09): enabled, sweep interval, per-window batch.
	DefaultExpiryNotifyEnabled     = true
	DefaultExpiryNotifyIntervalMin = 360 // 6 jam: reminder lebih responsif daripada harian 09:00
	DefaultExpiryNotifyBatch       = 50

	// Connection pools (AGENTS.md §1.7: limits must be explicit).
	DefaultDBMaxOpenConns    = 25
	DefaultDBMaxIdleConns    = 10
	DefaultDBConnMaxLifetime = 30 * time.Minute
	DefaultRedisPoolSize     = 50
	DefaultRedisDialTimeout  = 5 * time.Second
)
