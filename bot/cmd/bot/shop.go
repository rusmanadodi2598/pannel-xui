// Package main also hosts the shop composition helpers.
//
// @file      cmd/bot/shop.go
// @for       Composition root: wire pricing, panels, orders, trial and admin services.
// @uses      context, log/slog, time, internal/config, internal/crypto, internal/handler/telegram,
// internal/repository/postgres, internal/repository/redis, internal/repository/telegram,
// internal/repository/xui, internal/service/admin, internal/service/order,
// internal/service/pricing, internal/service/server, internal/service/telegram,
// internal/service/trial, internal/service/user
// @reason    Keeps main.go under the 250-line limit (§1.1) while centralizing
// the M4/M6 service wiring (no business logic — AGENTS.md §1.5).
// @author    Dodi Rusmana <rusmanadodi@kentangtechstore.com>
// @layer     service
// @stability stable
// @since     2026-08-11
package main

import (
	"context"
	"log/slog"
	"sync"
	"time"

	"github.com/kentangtech/bot-order/internal/config"
	"github.com/kentangtech/bot-order/internal/crypto"
	"github.com/kentangtech/bot-order/internal/domain"
	telegramhandler "github.com/kentangtech/bot-order/internal/handler/telegram"
	"github.com/kentangtech/bot-order/internal/job"
	"github.com/kentangtech/bot-order/internal/repository/kts"
	"github.com/kentangtech/bot-order/internal/repository/postgres"
	"github.com/kentangtech/bot-order/internal/repository/redis"
	telegramrepo "github.com/kentangtech/bot-order/internal/repository/telegram"
	"github.com/kentangtech/bot-order/internal/repository/xui"
	adminsvc "github.com/kentangtech/bot-order/internal/service/admin"
	healthsvc "github.com/kentangtech/bot-order/internal/service/health"
	ordersvc "github.com/kentangtech/bot-order/internal/service/order"
	pricingsvc "github.com/kentangtech/bot-order/internal/service/pricing"
	serversvc "github.com/kentangtech/bot-order/internal/service/server"
	telegramservice "github.com/kentangtech/bot-order/internal/service/telegram"
	topupsvc "github.com/kentangtech/bot-order/internal/service/topup"
	trafficsvc "github.com/kentangtech/bot-order/internal/service/traffic"
	trialsvc "github.com/kentangtech/bot-order/internal/service/trial"
	trialcleanupsvc "github.com/kentangtech/bot-order/internal/service/trialcleanup"
	usersvc "github.com/kentangtech/bot-order/internal/service/user"
)

// adminFSMTTL bounds how long a pending admin free-text input stays valid.
const adminFSMTTL = 10 * time.Minute

// shopBundle carries the dispatcher feature seams built in one pass.
type shopBundle struct {
	Shop         *telegramhandler.Shop
	Admin        *telegramhandler.Admin
	Servers      *serversvc.Service       // traffic-sync panel factory (PRD §16.2)
	Traffic      *trafficsvc.Service      // shared by the sweep worker + manual refresh
	Health       *healthsvc.Service       // PRD §17: server mati tidak dijual
	TrialCleanup *trialcleanupsvc.Service // PRD worker: disable expired trials
	Topup        *topupsvc.Service        // Phase 4: PG charge topup + settlement
}

// buildShop seeds pricing & panels and wires the M4 shop, M6 trial and M6
// admin services (composition root — no business logic, AGENTS.md §1.5).
func buildShop(ctx context.Context, cfg *config.Config, db *postgres.Repository, rdb *redis.Client, banned *telegramservice.BanService, tgClient *telegramrepo.Client, logger *slog.Logger) (*shopBundle, error) {
	box, err := crypto.NewSecretBox(cfg.EncryptionKey)
	if err != nil {
		return nil, err
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

	// Phase 4 (FR-06): PG Aggregate topup — the bot is a merchant on the
	// gateway. KTSSecret is the single secretKey: outbound S2S signing AND
	// inbound X-Webhook-Signature verification (013 §2.2). The notifier is
	// wired with the optional admin group (NOTIFICATION_GROUP_ID).
	ktsClient, err := kts.New(cfg.KTSBaseURL, cfg.KTSAPIKey, cfg.KTSSecret, cfg.XUIAPITimeout)
	if err != nil {
		return nil, err
	}
	paymentRepo := postgres.NewPaymentRepo(gormDB)
	topupNotif := &topupNotifier{tg: tgClient, chatID: cfg.NotificationGroupID, logger: logger}
	topups := topupsvc.New(ktsClient, paymentRepo, userRepo,
		cfg.QRISFeePercent, cfg.QRISPPNPercent,
		domain.Money(cfg.MinTopupAmount), domain.Money(cfg.MaxTopupAmount), topupNotif)

	// FR-04 AC (v1.41): completed paid orders notify the admin group. The
	// adapter is only wired when NOTIFICATION_GROUP_ID is configured. Telegram
	// group chat IDs are NEGATIVE (supergroups -100...), so the gate is != 0,
	// not > 0 (fix review v1.41).
	var orderNotify ordersvc.OrderNotifier
	if cfg.NotificationGroupID != 0 {
		orderNotify = &orderNotifier{tg: tgClient, chatID: cfg.NotificationGroupID, logger: logger}
	}
	orders := ordersvc.New(orderRepo, clientRepo, userRepo, pricing, servers, servers, orderNotify)
	// FR-13 (v1.46): sub server URL prefix + paths (Opsi 2 — domain sama dengan
	// panel, port beda; default panel subPort 2096). The order flow persists
	// these URLs; only the .txt export ships them to the user. JSON sub hanya
	// diaktifkan bila SUB_JSON_ENABLED (kalau tidak, path dikosongkan).
	subJSONPath := ""
	if cfg.SubJSONEnabled {
		subJSONPath = cfg.SubJSONPath
	}
	orders.SetSubLinks(ordersvc.SubLinks{
		BaseURL:  cfg.SubBaseURL,
		LinkPath: cfg.SubPath,
		JSONPath: subJSONPath,
	})

	// M6 (FR-07): trial daily limit via Redis counter with end-of-day TTL;
	// the account defaults (hours/GB/IP) come from config, not hardcode.
	trialCounter := redis.NewTrialCounter(rdb)
	trialLimit := trialsvc.New(trialCounter, cfg.TrialEnabled, cfg.TrialDailyLimit,
		cfg.TrialDurationHours, cfg.TrialTrafficGB, cfg.TrialIPLimit, cfg.TimeLocation)

	// M6 (PRD §16.2 / FR-08 AC-3): ONE traffic-sync instance is shared by the
	// background sweep (RunOnce) and the on-demand per-account refresh
	// (RefreshClient) — no duplicated state or panel factory.
	trafficSvc := trafficsvc.New(clientRepo,
		func(ctx context.Context, serverID int64) (trafficsvc.Panel, error) {
			return servers.PanelClient(ctx, serverID)
		},
		cfg.TrafficSyncBatch, cfg.XUIAPITimeout, logger)

	// M7 (PRD §17): health check per active panel — the sweep writes
	// health_status and the buy menu stops selling "down" panels.
	healthSvc := healthsvc.New(serverRepo,
		func(ctx context.Context, serverID int64) (healthsvc.Panel, error) {
			return servers.PanelClient(ctx, serverID)
		},
		cfg.XUIAPITimeout, logger)

	// M7 (PRD worker): trial cleanup — expired 1-hour trial accounts are
	// disabled on the panel (serversvc.DisableClients, one GetInbounds per
	// server) and only then marked expired in the DB.
	cleanupSvc := trialcleanupsvc.New(clientRepo, servers,
		cfg.TrialCleanupBatch, cfg.XUIAPITimeout, logger)

	shop := &telegramhandler.Shop{
		Plans:   pricing,
		Servers: servers,
		Users:   users,
		Orders:  orders,
		Clients: clientRepo,
		Trials:  orders,
		TrialLm: trialLimit,
		// M7 (FR-14): riwayat order user — repo reads paged + ownership-guarded.
		History: orderRepo,
		// M7 (FR-08 AC-4): hapus akun — panel delClient (serversvc) lalu DB row.
		Deleter: servers,
		// M7 (FR-08 AC-3): refresh traffic manual per akun dari panel.
		Traffic: trafficSvc,
	}

	// M6 (FR-11): panel admin — price, broadcast, ban/unban, adjust saldo +
	// server management (serversvc), statistik (orderRepo) & audit trail (auditRepo).
	auditRepo := postgres.NewAuditRepo(gormDB)
	adminSvc := adminsvc.New(pricing, users, banned, tgClient, rdb, servers, orderRepo, auditRepo, logger)
	// Broadcast goroutines inherit the signal ctx: graceful shutdown cancels
	// in-flight deliveries promptly (AGENTS.md §1.6).
	adminSvc.SetShutdownContext(ctx)
	admin := &telegramhandler.Admin{Ops: adminSvc, FSM: redis.NewAdminFSM(rdb, adminFSMTTL)}
	return &shopBundle{
		Shop: shop, Admin: admin, Servers: servers, Traffic: trafficSvc,
		Health: healthSvc, TrialCleanup: cleanupSvc, Topup: topups,
	}, nil
}

// startTrafficSync wires the PRD §16.2 traffic-sync worker around the shared
// trafficsvc instance from buildShop (kept out of main.go for the §1.1 line
// limit) and returns the WaitGroup the caller drains.
func startTrafficSync(ctx context.Context, cfg *config.Config, trafficSvc *trafficsvc.Service, logger *slog.Logger) *sync.WaitGroup {
	// Sweep timeout di-sizing untuk fleet: setiap panel ber-budget
	// XUIAPITimeout, plus 2 mnt buffer — satu sweep tidak terpotong di
	// tengah fleet (fix review v1.21).
	sweepTimeout := time.Duration(len(cfg.Panels))*cfg.XUIAPITimeout + 2*time.Minute
	trafficWorker := job.NewIntervalWorker(cfg.TrafficSyncInterval, sweepTimeout, trafficSvc, logger)
	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		trafficWorker.Run(ctx)
	}()
	logger.Info("traffic sync started",
		"interval_minutes", cfg.TrafficSyncInterval.Minutes(),
		"batch", cfg.TrafficSyncBatch)
	return &wg
}

// startHealthCheck wires the PRD §17 health-check worker. Sweep timeout is
// sized for the fleet (panels × XUI timeout + 1 min buffer) so one sweep is
// not cut off mid-fleet.
func startHealthCheck(ctx context.Context, cfg *config.Config, healthSvc *healthsvc.Service, logger *slog.Logger) *sync.WaitGroup {
	sweepTimeout := time.Duration(len(cfg.Panels))*cfg.XUIAPITimeout + time.Minute
	healthWorker := job.NewIntervalWorker(cfg.HealthCheckInterval, sweepTimeout, healthSvc, logger)
	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		healthWorker.Run(ctx)
	}()
	logger.Info("health check started",
		"interval_seconds", cfg.HealthCheckInterval.Seconds())
	return &wg
}

// startTrialCleanup wires the trial-cleanup worker (FR-07: expired 1-hour
// trial accounts get disabled on the panel). Sweep timeout sized like the
// other fleet workers.
func startTrialCleanup(ctx context.Context, cfg *config.Config, cleanupSvc *trialcleanupsvc.Service, logger *slog.Logger) *sync.WaitGroup {
	sweepTimeout := time.Duration(len(cfg.Panels))*cfg.XUIAPITimeout + time.Minute
	cleanupWorker := job.NewIntervalWorker(cfg.TrialCleanupInterval, sweepTimeout, cleanupSvc, logger)
	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		cleanupWorker.Run(ctx)
	}()
	logger.Info("trial cleanup started",
		"interval_minutes", cfg.TrialCleanupInterval.Minutes(),
		"batch", cfg.TrialCleanupBatch)
	return &wg
}
