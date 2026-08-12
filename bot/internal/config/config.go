// Package config provides typed, validated environment configuration.
//
// @file      internal/config/config.go
// @for       Bot-order typed environment configuration with fail-fast validation.
// @uses      os, strconv, strings, time, log/slog (stdlib only)
// @reason    Centralizes every setting so business code never reads raw env vars (AGENTS.md §1.4).
// @author    Dodi Rusmana <rusmanadodi@kentangtechstore.com>
// @layer     config
// @stability stable
// @since     2026-08-11
package config

import (
	"log/slog"
	"time"
)

// Config holds every setting of the bot, parsed and validated at boot.
type Config struct {
	BotToken              string
	BotDomain             string
	WebhookPort           int
	WebhookPath           string
	WebhookSecret         string
	WebhookMaxConnections int
	WebhookDropPending    bool
	WebhookWorkers        int
	WebhookQueueBuffer    int
	DatabaseURL           string
	DBMaxOpenConns        int
	DBMaxIdleConns        int
	DBConnMaxLifetime     time.Duration
	RedisURL              string
	RedisPoolSize         int
	RedisDialTimeout      time.Duration
	EncryptionKey         []byte
	AdminIDs              []int64
	RequiredGroupID       int64
	RequiredGroupLink     string
	NotificationGroupID   int64
	ExpiryNotifyDays      []int
	RateLimitRequests     int
	TimeLocation          *time.Location
	XUIAPITimeout         time.Duration
	APIBaseURL            string
	TopupAPIKey           string
	TopupWebhookSecret    string
	MinTopupAmount        int
	MaxTopupAmount        int
	QRISFeePercent        float64
	QRISPPNPercent        float64
	QRISExpiryMinutes     int
	LogLevel              slog.Level
	Panels                []ServerSeed // multi X-UI instances (FR-10, M4)
	PricingSeedFile       string       // JSON file seeded into `pricing` at boot (PRD §13.7)
	TrialEnabled          bool         // FR-07: fitur trial aktif/nonaktif
	TrialDailyLimit       int          // FR-07 AC-1: maks akun trial per hari per user
	TrialDurationHours    int          // FR-07: durasi trial dalam jam
	TrialTrafficGB        int          // FR-07: kuota trial dalam GB
	TrialIPLimit          int          // FR-07: limit IP akun trial

	// Expiry reminders (FR-09).
	ExpiryNotifyEnabled  bool          // worker notifikasi kadaluarsa aktif
	ExpiryNotifyInterval time.Duration // jarak antar sweep (EXPIRY_NOTIFY_INTERVAL_MIN)
	ExpiryNotifyBatch    int           // maks akun diproses per ambang per sweep

	TrafficSyncEnabled  bool          // PRD §16.2
	TrafficSyncInterval time.Duration // jarak antar sweep (TRAFFIC_SYNC_INTERVAL_MIN)
	TrafficSyncBatch    int           // maks client diproses per sweep

	// Health check (PRD §17): ping tiap panel, server mati tidak dijual.
	HealthCheckEnabled  bool
	HealthCheckInterval time.Duration

	// Trial cleanup (PRD worker): nonaktifkan akun trial expired di panel.
	TrialCleanupEnabled  bool
	TrialCleanupInterval time.Duration
	TrialCleanupBatch    int

	// FR-13 subscription: panel sub server public URL + paths (Opsi 2: domain
	// sama dengan panel, port beda — default sub server 2096).
	SubEnabled     bool
	SubBaseURL     string // public base, e.g. https://id2.kentangtechstore.net:2096
	SubPath        string // panel subPath (default /sub)
	SubJSONEnabled bool
	SubJSONPath    string // panel subJsonPath (default /json)
}

// Load reads the environment, applies defaults and validates every field.
// It fails fast with a clean error on missing or malformed configuration
// (AGENTS.md §1.4) — user-supplied env values never panic.
func Load() (*Config, error) {
	timeLoc, err := loadLocation(getEnv("TIME_LOCATION", DefaultTimeLocation))
	if err != nil {
		return nil, err
	}
	timeoutSec, err := parseInt("XUI_API_TIMEOUT", getEnv("XUI_API_TIMEOUT", "30"))
	if err != nil {
		return nil, err
	}
	qrisFee, err := parseFloat("QRIS_FEE_PERCENT", getEnv("QRIS_FEE_PERCENT", "0.025"))
	if err != nil {
		return nil, err
	}
	qrisPPN, err := parseFloat("QRIS_PPN_PERCENT", getEnv("QRIS_PPN_PERCENT", "0.11"))
	if err != nil {
		return nil, err
	}
	notifyDays, err := parseDayList("EXPIRY_NOTIFY_DAYS", getEnv("EXPIRY_NOTIFY_DAYS", "7,3,1"))
	if err != nil {
		return nil, err
	}

	cfg := &Config{
		BotToken:           getEnv("BOT_TOKEN", ""),
		BotDomain:          getEnv("BOT_DOMAIN", ""),
		WebhookSecret:      getEnv("WEBHOOK_SECRET", ""),
		DatabaseURL:        getEnv("DATABASE_URL", ""),
		RedisURL:           getEnv("REDIS_URL", ""),
		RequiredGroupLink:  getEnv("REQUIRED_GROUP_LINK", ""),
		APIBaseURL:         getEnv("API_BASE_URL", ""),
		TopupAPIKey:        getEnv("TOPUP_API_KEY", ""),
		TopupWebhookSecret: getEnv("TOPUP_WEBHOOK_SECRET", ""),
		ExpiryNotifyDays:   notifyDays,
		TimeLocation:       timeLoc,
		XUIAPITimeout:      time.Duration(timeoutSec) * time.Second,
		WebhookPath:        getEnv("WEBHOOK_PATH", DefaultWebhookPath),
		QRISFeePercent:     qrisFee,
		QRISPPNPercent:     qrisPPN,
		LogLevel:           parseLogLevel(getEnv("LOG_LEVEL", DefaultLogLevel)),
	}

	if cfg.WebhookPort, err = parseIntEnv("WEBHOOK_PORT", DefaultWebhookPort); err != nil {
		return nil, err
	}
	if cfg.WebhookMaxConnections, err = parseIntEnv("WEBHOOK_MAX_CONNECTIONS", DefaultWebhookMaxConnections); err != nil {
		return nil, err
	}
	if cfg.WebhookWorkers, err = parseIntEnv("WEBHOOK_WORKERS", DefaultWebhookWorkers); err != nil {
		return nil, err
	}
	if cfg.WebhookQueueBuffer, err = parseIntEnv("WEBHOOK_QUEUE_BUFFER", DefaultWebhookQueueBuffer); err != nil {
		return nil, err
	}
	if cfg.WebhookDropPending, err = parseBoolEnv("WEBHOOK_DROP_PENDING", true); err != nil {
		return nil, err
	}
	if cfg.RateLimitRequests, err = parseIntEnv("RATE_LIMIT_REQUESTS", DefaultRateLimitRequests); err != nil {
		return nil, err
	}
	if cfg.MinTopupAmount, err = parseIntEnv("MIN_TOPUP_AMOUNT", DefaultMinTopupAmount); err != nil {
		return nil, err
	}
	if cfg.MaxTopupAmount, err = parseIntEnv("MAX_TOPUP_AMOUNT", DefaultMaxTopupAmount); err != nil {
		return nil, err
	}
	if cfg.QRISExpiryMinutes, err = parseIntEnv("QRIS_EXPIRY_MINUTES", DefaultQRISExpiryMinutes); err != nil {
		return nil, err
	}
	if cfg.DBMaxOpenConns, err = parseIntEnv("DB_MAX_OPEN_CONNS", DefaultDBMaxOpenConns); err != nil {
		return nil, err
	}
	if cfg.DBMaxIdleConns, err = parseIntEnv("DB_MAX_IDLE_CONNS", DefaultDBMaxIdleConns); err != nil {
		return nil, err
	}
	lifetimeMin, err := parseIntEnv("DB_CONN_MAX_LIFETIME_MIN", int(DefaultDBConnMaxLifetime.Minutes()))
	if err != nil {
		return nil, err
	}
	cfg.DBConnMaxLifetime = time.Duration(lifetimeMin) * time.Minute
	if cfg.RedisPoolSize, err = parseIntEnv("REDIS_POOL_SIZE", DefaultRedisPoolSize); err != nil {
		return nil, err
	}
	dialTimeoutSec, err := parseIntEnv("REDIS_DIAL_TIMEOUT_SEC", int(DefaultRedisDialTimeout.Seconds()))
	if err != nil {
		return nil, err
	}
	cfg.RedisDialTimeout = time.Duration(dialTimeoutSec) * time.Second
	if cfg.RequiredGroupID, err = parseID("REQUIRED_GROUP_ID", getEnv("REQUIRED_GROUP_ID", "0")); err != nil {
		return nil, err
	}
	if cfg.NotificationGroupID, err = parseID("NOTIFICATION_GROUP_ID", getEnv("NOTIFICATION_GROUP_ID", "0")); err != nil {
		return nil, err
	}
	if cfg.AdminIDs, err = parseIDList("ADMIN_IDS", getEnv("ADMIN_IDS", "")); err != nil {
		return nil, err
	}
	if cfg.EncryptionKey, err = decodeEncryptionKey(getEnv("ENCRYPTION_KEY", "")); err != nil {
		return nil, err
	}
	cfg.PricingSeedFile = getEnv("PRICING_SEED_FILE", DefaultPricingSeedFile)
	if cfg.Panels, err = ParseServerSeeds(); err != nil {
		return nil, err
	}
	if cfg.TrialEnabled, err = parseBoolEnv("TRIAL_ENABLED", DefaultTrialEnabled); err != nil {
		return nil, err
	}
	if cfg.TrialDailyLimit, err = parseIntEnv("TRIAL_DAILY_LIMIT", DefaultTrialDailyLimit); err != nil {
		return nil, err
	}
	if cfg.TrialDurationHours, err = parseIntEnv("TRIAL_DURATION_HOURS", DefaultTrialDurationHours); err != nil {
		return nil, err
	}
	if cfg.TrialTrafficGB, err = parseIntEnv("TRIAL_TRAFFIC_GB", DefaultTrialTrafficGB); err != nil {
		return nil, err
	}
	if cfg.TrialIPLimit, err = parseIntEnv("TRIAL_IP_LIMIT", DefaultTrialIPLimit); err != nil {
		return nil, err
	}
	if cfg.ExpiryNotifyEnabled, err = parseBoolEnv("EXPIRY_NOTIFY_ENABLED", DefaultExpiryNotifyEnabled); err != nil {
		return nil, err
	}
	notifyIntervalMin, err := parseIntEnv("EXPIRY_NOTIFY_INTERVAL_MIN", DefaultExpiryNotifyIntervalMin)
	if err != nil {
		return nil, err
	}
	cfg.ExpiryNotifyInterval = time.Duration(notifyIntervalMin) * time.Minute
	if cfg.ExpiryNotifyBatch, err = parseIntEnv("EXPIRY_NOTIFY_BATCH", DefaultExpiryNotifyBatch); err != nil {
		return nil, err
	}
	if err := cfg.applyTrafficSync(); err != nil {
		return nil, err
	}
	if err := cfg.applyHealthCheck(); err != nil {
		return nil, err
	}
	if err := cfg.applyTrialCleanup(); err != nil {
		return nil, err
	}
	if err := cfg.applySubscription(); err != nil {
		return nil, err
	}

	if err := cfg.validate(); err != nil {
		return nil, err
	}
	return cfg, nil
}
