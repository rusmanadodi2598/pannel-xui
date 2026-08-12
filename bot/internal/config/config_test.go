// Package config_test covers typed environment configuration (AGENTS.md §2.1).
//
// @file      internal/config/config_test.go
// @for       Unit tests for env parsing, defaults and fail-fast validation.
// @uses      testing, os, encoding/base64
// @reason    Guards the config contract that every other package depends on.
// @author    Dodi Rusmana <rusmanadodi@kentangtechstore.com>
// @layer     config
// @stability stable
// @since     2026-08-11
package config

import (
	"encoding/base64"
	"os"
	"testing"
	"time"
)

// validKey returns a base64-encoded 32-byte AES-256 key for tests.
func validKey() string {
	return base64.StdEncoding.EncodeToString(make([]byte, 32))
}

func TestLoad_AllValid(t *testing.T) {
	applyEnv(t, validEnv())

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if cfg.BotToken != "123456:TEST" {
		t.Errorf("BotToken = %q, want %q", cfg.BotToken, "123456:TEST")
	}
	if cfg.WebhookPort != 8443 {
		t.Errorf("WebhookPort = %d, want 8443", cfg.WebhookPort)
	}
	if got := len(cfg.AdminIDs); got != 2 || cfg.AdminIDs[1] != 987654321 {
		t.Errorf("AdminIDs = %v, want [123456789 987654321]", cfg.AdminIDs)
	}
	if got := cfg.ExpiryNotifyDays; len(got) != 3 || got[2] != 1 {
		t.Errorf("ExpiryNotifyDays = %v, want [7 3 1]", got)
	}
	if got := cfg.TimeLocation.String(); got != "Asia/Jakarta" {
		t.Errorf("TimeLocation = %q, want Asia/Jakarta", got)
	}
	if cfg.XUIAPITimeout != 30*time.Second {
		t.Errorf("XUIAPITimeout = %v, want 30s", cfg.XUIAPITimeout)
	}
}

func TestLoad_RequiredMissing(t *testing.T) {
	missing := []string{"BOT_TOKEN", "BOT_DOMAIN", "WEBHOOK_SECRET", "DATABASE_URL", "REDIS_URL", "ENCRYPTION_KEY"}
	for _, key := range missing {
		t.Run(key, func(t *testing.T) {
			env := validEnv()
			env[key] = ""
			applyEnv(t, env)
			if _, err := Load(); err == nil {
				t.Fatalf("Load() with empty %s: expected error, got nil", key)
			}
		})
	}
}

func TestLoad_InvalidValues(t *testing.T) {
	cases := []struct {
		name string
		mut  func(map[string]string)
	}{
		{"short webhook secret", func(e map[string]string) { e["WEBHOOK_SECRET"] = "short" }},
		{"invalid webhook port", func(e map[string]string) { e["WEBHOOK_PORT"] = "abc" }},
		{"invalid fee range", func(e map[string]string) { e["QRIS_FEE_PERCENT"] = "1.5" }},
		{"invalid fee syntax", func(e map[string]string) { e["QRIS_FEE_PERCENT"] = "abc" }},
		{"invalid admin id", func(e map[string]string) { e["ADMIN_IDS"] = "12x" }},
		{"invalid xui timeout", func(e map[string]string) { e["XUI_API_TIMEOUT"] = "abc" }},
		{"invalid notify days", func(e map[string]string) { e["EXPIRY_NOTIFY_DAYS"] = "7,x" }},
		{"invalid time location", func(e map[string]string) { e["TIME_LOCATION"] = "Mars/Phobos" }},
		{"invalid db open conns", func(e map[string]string) { e["DB_MAX_OPEN_CONNS"] = "0" }},
		{"invalid db idle > open", func(e map[string]string) { e["DB_MAX_OPEN_CONNS"] = "10"; e["DB_MAX_IDLE_CONNS"] = "20" }},
		{"invalid db lifetime", func(e map[string]string) { e["DB_CONN_MAX_LIFETIME_MIN"] = "0" }},
		{"invalid redis pool", func(e map[string]string) { e["REDIS_POOL_SIZE"] = "0" }},
		{"invalid redis dial timeout", func(e map[string]string) { e["REDIS_DIAL_TIMEOUT_SEC"] = "-1" }},
		{"invalid webhook max connections", func(e map[string]string) { e["WEBHOOK_MAX_CONNECTIONS"] = "101" }},
		{"invalid webhook workers", func(e map[string]string) { e["WEBHOOK_WORKERS"] = "0" }},
		{"invalid webhook queue buffer", func(e map[string]string) { e["WEBHOOK_QUEUE_BUFFER"] = "-1" }},
		{"invalid drop pending", func(e map[string]string) { e["WEBHOOK_DROP_PENDING"] = "abc" }},
		{"invalid trial enabled", func(e map[string]string) { e["TRIAL_ENABLED"] = "abc" }},
		{"invalid trial daily limit", func(e map[string]string) { e["TRIAL_DAILY_LIMIT"] = "0" }},
		{"invalid trial duration", func(e map[string]string) { e["TRIAL_DURATION_HOURS"] = "-1" }}, {"invalid trial traffic", func(e map[string]string) { e["TRIAL_TRAFFIC_GB"] = "0" }},
		{"invalid trial ip limit", func(e map[string]string) { e["TRIAL_IP_LIMIT"] = "0" }},
		{"invalid expiry enabled", func(e map[string]string) { e["EXPIRY_NOTIFY_ENABLED"] = "abc" }},
		{"invalid expiry interval", func(e map[string]string) { e["EXPIRY_NOTIFY_INTERVAL_MIN"] = "0" }},
		{"invalid expiry batch", func(e map[string]string) { e["EXPIRY_NOTIFY_BATCH"] = "0" }},
		{"invalid traffic sync enabled", func(e map[string]string) { e["TRAFFIC_SYNC_ENABLED"] = "abc" }},
		{"invalid traffic sync interval", func(e map[string]string) { e["TRAFFIC_SYNC_INTERVAL_MIN"] = "0" }},
		{"invalid traffic sync interval high", func(e map[string]string) { e["TRAFFIC_SYNC_INTERVAL_MIN"] = "61" }},
		{"invalid traffic sync batch", func(e map[string]string) { e["TRAFFIC_SYNC_BATCH"] = "0" }},
		{"invalid health check enabled", func(e map[string]string) { e["HEALTH_CHECK_ENABLED"] = "abc" }},
		{"invalid health check interval", func(e map[string]string) { e["HEALTH_CHECK_INTERVAL_SEC"] = "0" }},
		{"invalid health check interval high", func(e map[string]string) { e["HEALTH_CHECK_INTERVAL_SEC"] = "3601" }},
		{"invalid trial cleanup enabled", func(e map[string]string) { e["TRIAL_CLEANUP_ENABLED"] = "abc" }},
		{"invalid trial cleanup interval", func(e map[string]string) { e["TRIAL_CLEANUP_INTERVAL_MIN"] = "0" }},
		{"invalid trial cleanup batch", func(e map[string]string) { e["TRIAL_CLEANUP_BATCH"] = "0" }},
		{"invalid sub enabled", func(e map[string]string) { e["SUB_ENABLED"] = "abc" }},
		{"sub enabled without base url", func(e map[string]string) { e["SUB_ENABLED"] = "true"; e["SUB_BASE_URL"] = "" }},
		{"invalid sub base url", func(e map[string]string) { e["SUB_ENABLED"] = "true"; e["SUB_BASE_URL"] = "not-a-url" }},
		{"invalid sub path", func(e map[string]string) { e["SUB_PATH"] = "noslash" }},
		{"invalid sub json path", func(e map[string]string) { e["SUB_JSON_PATH"] = "nopath" }},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			env := validEnv()
			tc.mut(env)
			applyEnv(t, env)
			if _, err := Load(); err == nil {
				t.Fatalf("Load() with %s: expected error, got nil", tc.name)
			}
		})
	}
}

func TestLoad_WrongEncryptionKey(t *testing.T) {
	env := validEnv()
	env["ENCRYPTION_KEY"] = base64.StdEncoding.EncodeToString([]byte("too-short-key"))
	applyEnv(t, env)
	if _, err := Load(); err == nil {
		t.Fatal("Load() with 9-byte key: expected error, got nil")
	}
}

func TestLoad_Defaults(t *testing.T) {
	env := validEnv()
	// Keep only required fields; everything else must fall back to defaults.
	for k := range env {
		switch k {
		case "BOT_TOKEN", "BOT_DOMAIN", "WEBHOOK_SECRET", "DATABASE_URL", "REDIS_URL", "ENCRYPTION_KEY":
		default:
			delete(env, k)
		}
	}
	applyEnv(t, env)

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if cfg.WebhookPort != DefaultWebhookPort {
		t.Errorf("WebhookPort = %d, want %d", cfg.WebhookPort, DefaultWebhookPort)
	}
	if cfg.WebhookPath != "/api/v1/webhooks/telegram" {
		t.Errorf("WebhookPath = %q, want /api/v1/webhooks/telegram", cfg.WebhookPath)
	}
	if cfg.RateLimitRequests != 30 {
		t.Errorf("RateLimitRequests = %d, want 30", cfg.RateLimitRequests)
	}
	if cfg.MinTopupAmount != 5000 || cfg.MaxTopupAmount != 5000000 {
		t.Errorf("topup range = %d..%d, want 5000..5000000", cfg.MinTopupAmount, cfg.MaxTopupAmount)
	}
	if cfg.QRISFeePercent != 0.025 || cfg.QRISPPNPercent != 0.11 {
		t.Errorf("QRIS fees = %v/%v, want 0.025/0.11", cfg.QRISFeePercent, cfg.QRISPPNPercent)
	}
	if len(cfg.ExpiryNotifyDays) != 3 {
		t.Errorf("ExpiryNotifyDays = %v, want [7 3 1]", cfg.ExpiryNotifyDays)
	}
	if cfg.DBMaxOpenConns != 25 || cfg.DBMaxIdleConns != 10 || cfg.DBConnMaxLifetime != 30*time.Minute {
		t.Errorf("DB pool = %d/%d/%v, want 25/10/30m", cfg.DBMaxOpenConns, cfg.DBMaxIdleConns, cfg.DBConnMaxLifetime)
	}
	if cfg.RedisPoolSize != 50 || cfg.RedisDialTimeout != 5*time.Second {
		t.Errorf("Redis pool = %d/%v, want 50/5s", cfg.RedisPoolSize, cfg.RedisDialTimeout)
	}
	if cfg.WebhookMaxConnections != 40 || cfg.WebhookDropPending != true ||
		cfg.WebhookWorkers != 8 || cfg.WebhookQueueBuffer != 64 {
		t.Errorf("webhook tuning = %d/%v/%d/%d, want 40/true/8/64",
			cfg.WebhookMaxConnections, cfg.WebhookDropPending, cfg.WebhookWorkers, cfg.WebhookQueueBuffer)
	}
	if !cfg.TrialEnabled || cfg.TrialDailyLimit != 2 || cfg.TrialDurationHours != 1 ||
		cfg.TrialTrafficGB != 1 || cfg.TrialIPLimit != 1 {
		t.Errorf("trial policy = %v/%d/%d/%d/%d, want true/2/1/1/1",
			cfg.TrialEnabled, cfg.TrialDailyLimit, cfg.TrialDurationHours, cfg.TrialTrafficGB, cfg.TrialIPLimit)
	}
	if !cfg.ExpiryNotifyEnabled || cfg.ExpiryNotifyInterval != 6*time.Hour || cfg.ExpiryNotifyBatch != 50 {
		t.Errorf("expiry notify = %v/%v/%d, want true/6h/50",
			cfg.ExpiryNotifyEnabled, cfg.ExpiryNotifyInterval, cfg.ExpiryNotifyBatch)
	}
	if !cfg.TrafficSyncEnabled || cfg.TrafficSyncInterval != 5*time.Minute || cfg.TrafficSyncBatch != 200 {
		t.Errorf("traffic sync = %v/%v/%d, want true/5m/200",
			cfg.TrafficSyncEnabled, cfg.TrafficSyncInterval, cfg.TrafficSyncBatch)
	}
	if !cfg.HealthCheckEnabled || cfg.HealthCheckInterval != 60*time.Second {
		t.Errorf("health check = %v/%v, want true/60s",
			cfg.HealthCheckEnabled, cfg.HealthCheckInterval)
	}
	if !cfg.TrialCleanupEnabled || cfg.TrialCleanupInterval != 15*time.Minute || cfg.TrialCleanupBatch != 50 {
		t.Errorf("trial cleanup = %v/%v/%d, want true/15m/50",
			cfg.TrialCleanupEnabled, cfg.TrialCleanupInterval, cfg.TrialCleanupBatch)
	}
	if cfg.SubEnabled || cfg.SubBaseURL != "" || cfg.SubPath != "/sub" || cfg.SubJSONPath != "/json" {
		t.Errorf("subscription defaults = %v/%q/%q/%q, want false//'/sub'/'/json'",
			cfg.SubEnabled, cfg.SubBaseURL, cfg.SubPath, cfg.SubJSONPath)
	}
}

func TestLoad_SubscriptionEnabled(t *testing.T) {
	env := validEnv()
	env["SUB_ENABLED"] = "true"
	env["SUB_BASE_URL"] = "https://id2.kentangtechstore.net:2096"
	env["SUB_PATH"] = "/sub/"
	env["SUB_JSON_ENABLED"] = "true"
	env["SUB_JSON_PATH"] = "/json"
	applyEnv(t, env)

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if !cfg.SubEnabled || cfg.SubBaseURL != "https://id2.kentangtechstore.net:2096" {
		t.Errorf("sub config = %v/%q, want true/base URL", cfg.SubEnabled, cfg.SubBaseURL)
	}
	// normalizeSubPath trims the trailing slash (join in SubLinks is canonical).
	if cfg.SubPath != "/sub" {
		t.Errorf("SubPath = %q, want /sub (trailing slash trimmed)", cfg.SubPath)
	}
	if !cfg.SubJSONEnabled || cfg.SubJSONPath != "/json" {
		t.Errorf("json sub = %v/%q, want true//json", cfg.SubJSONEnabled, cfg.SubJSONPath)
	}
}

// validEnv returns a complete, valid environment for tests.
func validEnv() map[string]string {
	return map[string]string{
		"BOT_TOKEN":                  "123456:TEST",
		"BOT_DOMAIN":                 "bot.example.com",
		"WEBHOOK_PORT":               "8443",
		"WEBHOOK_PATH":               "/api/v1/webhooks/telegram",
		"WEBHOOK_SECRET":             "0123456789abcdef0123456789abcdef",
		"DATABASE_URL":               "postgres://bot:bot@localhost:5432/bot",
		"REDIS_URL":                  "redis://localhost:6379/3",
		"ENCRYPTION_KEY":             validKey(),
		"ADMIN_IDS":                  "123456789,987654321",
		"REQUIRED_GROUP_ID":          "-100123456789",
		"REQUIRED_GROUP_LINK":        "https://t.me/kentangtech",
		"NOTIFICATION_GROUP_ID":      "-100987654321",
		"EXPIRY_NOTIFY_DAYS":         "7,3,1",
		"RATE_LIMIT_REQUESTS":        "30",
		"TIME_LOCATION":              "Asia/Jakarta",
		"XUI_API_TIMEOUT":            "30",
		"API_BASE_URL":               "https://hostinger.kentangtechstore.com",
		"TOPUP_API_KEY":              "secret-api-key",
		"TOPUP_WEBHOOK_SECRET":       "secret-webhook-key",
		"MIN_TOPUP_AMOUNT":           "5000",
		"MAX_TOPUP_AMOUNT":           "5000000",
		"QRIS_FEE_PERCENT":           "0.025",
		"QRIS_PPN_PERCENT":           "0.11",
		"QRIS_EXPIRY_MINUTES":        "15",
		"LOG_LEVEL":                  "info",
		"TRIAL_ENABLED":              "true",
		"TRIAL_DAILY_LIMIT":          "2",
		"TRIAL_DURATION_HOURS":       "1",
		"TRIAL_TRAFFIC_GB":           "1",
		"TRIAL_IP_LIMIT":             "1",
		"EXPIRY_NOTIFY_ENABLED":      "true",
		"EXPIRY_NOTIFY_INTERVAL_MIN": "360",
		"EXPIRY_NOTIFY_BATCH":        "50",
		"HEALTH_CHECK_ENABLED":       "true",
		"HEALTH_CHECK_INTERVAL_SEC":  "60",
		"TRIAL_CLEANUP_ENABLED":      "true",
		"TRIAL_CLEANUP_INTERVAL_MIN": "15",
		"TRIAL_CLEANUP_BATCH":        "50",
	}
}

// applyEnv sets the given env for the test and clears the rest of the keys.
func applyEnv(t *testing.T, env map[string]string) {
	t.Helper()
	for k := range validEnv() {
		if _, ok := env[k]; !ok {
			t.Setenv(k, "")
		}
	}
	for k, v := range env {
		if v == "" {
			os.Unsetenv(k)
			continue
		}
		t.Setenv(k, v)
	}
}
