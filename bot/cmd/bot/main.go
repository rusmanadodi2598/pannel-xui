// Package main is the bot-order composition root.
//
// @file      cmd/bot/main.go
// @for       Application bootstrap: config, logging, DB+Redis wiring, HTTP server lifecycle.
// @uses      internal/config, internal/repository/postgres, internal/repository/redis, internal/handler/http, net/http, os/signal
// @reason    Composition root that wires infrastructure and starts the versioned /api/v1 webhook server (AGENTS.md §1.5).
// @author    Dodi Rusmana <rusmanadodi@kentangtechstore.com>
// @layer     service
// @stability stable
// @since     2026-08-11
package main

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"sync"
	"syscall"
	"time"

	"github.com/kentangtech/bot-order/internal/config"
	"github.com/kentangtech/bot-order/internal/domain"
	"github.com/kentangtech/bot-order/internal/handler/http"
	telegramhandler "github.com/kentangtech/bot-order/internal/handler/telegram"
	"github.com/kentangtech/bot-order/internal/job"
	"github.com/kentangtech/bot-order/internal/repository/postgres"
	"github.com/kentangtech/bot-order/internal/repository/redis"
	telegramrepo "github.com/kentangtech/bot-order/internal/repository/telegram"
	expirysvc "github.com/kentangtech/bot-order/internal/service/expiry"
	telegramservice "github.com/kentangtech/bot-order/internal/service/telegram"
	topupsvc "github.com/kentangtech/bot-order/internal/service/topup"
)

// version is injected at build time via -ldflags "-X main.version=...".
var version = "dev"

// telegramAPITimeout bounds every outbound Telegram Bot API call (AGENTS.md §1.6).
const telegramAPITimeout = 30 * time.Second

// topupFSMTTL bounds how long a pending custom-nominal input stays valid.
const topupFSMTTL = 10 * time.Minute

func main() {
	if err := run(); err != nil {
		fmt.Fprintf(os.Stderr, "bot-order: %v\n", err)
		os.Exit(1)
	}
}

func run() error {
	cfg, err := config.Load()
	if err != nil {
		return fmt.Errorf("loading config: %w", err)
	}

	logger := newLogger(cfg.LogLevel)
	slog.SetDefault(logger)
	logger.Info("bot-order starting",
		"version", version,
		"webhook", cfg.WebhookPath,
		"port", cfg.WebhookPort,
	)

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	// PostgreSQL — pool limits eksplisit (AGENTS.md §1.7).
	db, err := postgres.Open(ctx, cfg.DatabaseURL, postgres.PoolOptions{
		MaxOpenConns:    cfg.DBMaxOpenConns,
		MaxIdleConns:    cfg.DBMaxIdleConns,
		ConnMaxLifetime: cfg.DBConnMaxLifetime,
	})
	if err != nil {
		return fmt.Errorf("connecting postgres: %w", err)
	}
	defer func() {
		if err := db.Close(); err != nil {
			logger.Error("closing postgres", "error", err)
		}
	}()

	// Migrations embedded (golang-migrate) — gagal cepat bila skema tidak naik.
	if err := postgres.MigrateUp(cfg.DatabaseURL); err != nil {
		return fmt.Errorf("running migrations: %w", err)
	}
	logger.Info("database migrations applied")

	// Redis — pool settings eksplisit.
	rdb, err := redis.Open(ctx, cfg.RedisURL, redis.PoolOptions{
		PoolSize:    cfg.RedisPoolSize,
		DialTimeout: cfg.RedisDialTimeout,
	})
	if err != nil {
		return fmt.Errorf("connecting redis: %w", err)
	}
	defer func() {
		if err := rdb.Close(); err != nil {
			logger.Error("closing redis", "error", err)
		}
	}()
	logger.Info("redis connected")

	// Telegram client — validates the token via getMe, fails fast on bad secrets.
	tgClient, err := telegramrepo.NewClient(cfg.BotToken, telegramAPITimeout)
	if err != nil {
		return fmt.Errorf("initializing telegram client: %w", err)
	}

	// Webhook registration before serving any traffic (PRD §14.1).
	webhookSvc := telegramservice.NewWebhookService(
		tgClient, cfg.BotDomain, cfg.WebhookPath, cfg.WebhookSecret,
		cfg.WebhookMaxConnections, cfg.WebhookDropPending,
	)
	webhookInfo, err := webhookSvc.Register(ctx)
	if err != nil {
		return fmt.Errorf("registering telegram webhook: %w", err)
	}
	logger.Info("telegram webhook registered",
		"url", webhookInfo.URL,
		"pending_updates", webhookInfo.PendingUpdateCount,
	)

	// Middleware chain services (PRD §14.2.5).
	gate := telegramservice.NewGateService(tgClient, rdb, cfg.RequiredGroupID)
	banned := telegramservice.NewBanService(rdb)
	limiter := telegramservice.NewRateLimiter(rdb, cfg.RateLimitRequests, telegramservice.RateLimitWindow)

	// M4 auto-order + M6 admin services (FR-03/04/05/08, FR-07, FR-11).
	bundle, err := buildShop(ctx, cfg, db, rdb, banned, tgClient, logger)
	if err != nil {
		return err
	}
	shop := bundle.Shop

	// M6 (FR-09): notifikasi kadaluarsa H-7/H-3/H-1 — worker interval; tanggal
	// di pesan diformat sesuai TIME_LOCATION (FR-09 AC). Loop berhenti via ctx.
	if cfg.ExpiryNotifyEnabled {
		clientRepo := postgres.NewClientRepo(db.DB())
		expirySvc := expirysvc.New(clientRepo, tgClient, cfg.ExpiryNotifyDays,
			cfg.ExpiryNotifyBatch, cfg.TimeLocation, logger)
		// Timeout per sweep 2 mnt (Telegram calls) — expirysvc menyelesaikan
		// semua ambang dalam satu sweep, bounded (AGENTS.md §1.6).
		notifier := job.NewIntervalWorker(cfg.ExpiryNotifyInterval, 2*time.Minute, expirySvc, logger)
		var notifyWG sync.WaitGroup
		notifyWG.Add(1)
		go func() {
			defer notifyWG.Done()
			notifier.Run(ctx)
		}()
		// Drain sebelum run() kembali: batalkan ctx DULU (stop idempoten) — defer
		// LIFO berarti wait terdaftar setelah stop harus memanggil stop sendiri,
		// kalau tidak jalur error errCh menggantung di Wait() (fix review v1.19).
		defer func() {
			stop()
			notifyWG.Wait()
		}()
		logger.Info("expiry notifier started",
			"interval_minutes", cfg.ExpiryNotifyInterval.Minutes(),
			"days", cfg.ExpiryNotifyDays, "batch", cfg.ExpiryNotifyBatch)
	}

	// M6 (PRD §16.2): sinkron traffic XUI → vpn_clients — worker interval;
	// per-server timeout + satu panel gagal tidak menggagalkan sweep.
	if cfg.TrafficSyncEnabled {
		trafficWG := startTrafficSync(ctx, cfg, db, bundle, logger)
		defer func() {
			stop()
			trafficWG.Wait()
		}()
	}

	// M5: topup flow (FR-06) — menus live, payment API deferred behind a stub
	// gateway until the KentangTech Go rewrite ships (product decision).
	topups := topupsvc.New(
		topupsvc.StubGateway{},
		cfg.QRISFeePercent,
		cfg.QRISPPNPercent,
		domain.Money(cfg.MinTopupAmount),
		domain.Money(cfg.MaxTopupAmount),
	)
	topupFSM := redis.NewTopupFSM(rdb, topupFSMTTL)
	topup := &telegramhandler.Topup{Users: shop.Users, Topups: topups, FSM: topupFSM}

	// Dispatcher consumed by the bounded worker pool (per-user serialization).
	dispatcher := telegramhandler.NewDispatcher(tgClient, gate, banned, limiter, logger, cfg.RequiredGroupLink, cfg.AdminIDs, shop, topup, bundle.Admin)
	worker := httphandler.NewWorker(cfg.WebhookWorkers, cfg.WebhookQueueBuffer, dispatcher, rdb, logger)
	defer worker.Close()

	handler := httphandler.New(httphandler.Options{
		Logger:        logger,
		Version:       version,
		WebhookPath:   cfg.WebhookPath,
		WebhookSecret: cfg.WebhookSecret,
		DB:            db,
		Redis:         rdb,
		Worker:        worker,
		Dedup:         rdb,
	})

	server := &http.Server{
		Addr:              fmt.Sprintf(":%d", cfg.WebhookPort),
		Handler:           handler,
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       30 * time.Second,
		WriteTimeout:      30 * time.Second,
		IdleTimeout:       60 * time.Second,
	}

	errCh := make(chan error, 1)
	go func() {
		logger.Info("http server listening", "addr", server.Addr)
		if err := server.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			errCh <- err
		}
	}()

	select {
	case err := <-errCh:
		return fmt.Errorf("http server: %w", err)
	case <-ctx.Done():
		logger.Info("shutdown signal received, draining")
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		if err := server.Shutdown(shutdownCtx); err != nil {
			return fmt.Errorf("graceful shutdown: %w", err)
		}
		logger.Info("bot-order stopped cleanly")
		return nil
	}
}

// newLogger builds a JSON slog handler at the configured level.
func newLogger(level slog.Level) *slog.Logger {
	return slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: level}))
}
