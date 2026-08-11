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
	"syscall"
	"time"

	"github.com/kentangtech/bot-order/internal/config"
	"github.com/kentangtech/bot-order/internal/crypto"
	"github.com/kentangtech/bot-order/internal/domain"
	"github.com/kentangtech/bot-order/internal/handler/http"
	telegramhandler "github.com/kentangtech/bot-order/internal/handler/telegram"
	"github.com/kentangtech/bot-order/internal/repository/postgres"
	"github.com/kentangtech/bot-order/internal/repository/redis"
	telegramrepo "github.com/kentangtech/bot-order/internal/repository/telegram"
	"github.com/kentangtech/bot-order/internal/repository/xui"
	ordersvc "github.com/kentangtech/bot-order/internal/service/order"
	pricingsvc "github.com/kentangtech/bot-order/internal/service/pricing"
	serversvc "github.com/kentangtech/bot-order/internal/service/server"
	telegramservice "github.com/kentangtech/bot-order/internal/service/telegram"
	topupsvc "github.com/kentangtech/bot-order/internal/service/topup"
	usersvc "github.com/kentangtech/bot-order/internal/service/user"
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

	// M4: auto-order services (FR-03/FR-04/FR-05/FR-08).
	shop, err := buildShop(ctx, cfg, db, rdb, logger)
	if err != nil {
		return err
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
	dispatcher := telegramhandler.NewDispatcher(tgClient, gate, banned, limiter, logger, cfg.RequiredGroupLink, cfg.AdminIDs, shop, topup)
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

// buildShop seeds pricing & panels and wires the M4 shop services.
func buildShop(ctx context.Context, cfg *config.Config, db *postgres.Repository, rdb *redis.Client, logger *slog.Logger) (*telegramhandler.Shop, error) {
	box, err := crypto.NewSecretBox(cfg.EncryptionKey)
	if err != nil {
		return nil, fmt.Errorf("building secret box: %w", err)
	}

	gormDB := db.DB()
	userRepo := postgres.NewUserRepo(gormDB)
	pricingRepo := postgres.NewPricingRepo(gormDB)
	serverRepo := postgres.NewServerRepo(gormDB)
	clientRepo := postgres.NewClientRepo(gormDB)
	orderRepo := postgres.NewOrderRepo(gormDB)

	pricing := pricingsvc.New(pricingRepo, pricingsvc.FileSeeder{Path: cfg.PricingSeedFile})
	if err := pricing.EnsureSeeded(ctx); err != nil {
		return nil, err
	}
	logger.Info("pricing seeded", "file", cfg.PricingSeedFile)

	sessionCache := xui.NewRedisSessionCache(rdb.Raw())
	servers := serversvc.New(serverRepo, box, sessionCache)
	if err := servers.EnsureSeeded(ctx, cfg.Panels); err != nil {
		return nil, err
	}
	logger.Info("panels seeded", "count", len(cfg.Panels))

	users := usersvc.New(userRepo)
	orders := ordersvc.New(orderRepo, clientRepo, userRepo, pricing, servers, servers)

	return &telegramhandler.Shop{
		Plans:   pricing,
		Servers: servers,
		Users:   users,
		Orders:  orders,
		Clients: clientRepo,
	}, nil
}

// newLogger builds a JSON slog handler at the configured level.
func newLogger(level slog.Level) *slog.Logger {
	return slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: level}))
}
