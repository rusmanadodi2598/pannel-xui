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
	telegramhandler "github.com/kentangtech/bot-order/internal/handler/telegram"
	"github.com/kentangtech/bot-order/internal/job"
	"github.com/kentangtech/bot-order/internal/repository/postgres"
	"github.com/kentangtech/bot-order/internal/repository/redis"
	telegramrepo "github.com/kentangtech/bot-order/internal/repository/telegram"
	"github.com/kentangtech/bot-order/internal/repository/xui"
	adminsvc "github.com/kentangtech/bot-order/internal/service/admin"
	ordersvc "github.com/kentangtech/bot-order/internal/service/order"
	pricingsvc "github.com/kentangtech/bot-order/internal/service/pricing"
	serversvc "github.com/kentangtech/bot-order/internal/service/server"
	telegramservice "github.com/kentangtech/bot-order/internal/service/telegram"
	trafficsvc "github.com/kentangtech/bot-order/internal/service/traffic"
	trialsvc "github.com/kentangtech/bot-order/internal/service/trial"
	usersvc "github.com/kentangtech/bot-order/internal/service/user"
)

// adminFSMTTL bounds how long a pending admin free-text input stays valid.
const adminFSMTTL = 10 * time.Minute

// shopBundle carries the dispatcher feature seams built in one pass.
type shopBundle struct {
	Shop    *telegramhandler.Shop
	Admin   *telegramhandler.Admin
	Servers *serversvc.Service // traffic-sync panel factory (PRD §16.2)
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
	orders := ordersvc.New(orderRepo, clientRepo, userRepo, pricing, servers, servers)

	// M6 (FR-07): trial daily limit via Redis counter with end-of-day TTL;
	// the account defaults (hours/GB/IP) come from config, not hardcode.
	trialCounter := redis.NewTrialCounter(rdb)
	trialLimit := trialsvc.New(trialCounter, cfg.TrialEnabled, cfg.TrialDailyLimit,
		cfg.TrialDurationHours, cfg.TrialTrafficGB, cfg.TrialIPLimit, cfg.TimeLocation)

	shop := &telegramhandler.Shop{
		Plans:   pricing,
		Servers: servers,
		Users:   users,
		Orders:  orders,
		Clients: clientRepo,
		Trials:  orders,
		TrialLm: trialLimit,
	}

	// M6 (FR-11): panel admin — price management, broadcast, ban/unban.
	adminSvc := adminsvc.New(pricing, users, banned, tgClient, rdb, logger)
	// Broadcast goroutines inherit the signal ctx: graceful shutdown cancels
	// in-flight deliveries promptly (AGENTS.md §1.6).
	adminSvc.SetShutdownContext(ctx)
	admin := &telegramhandler.Admin{Ops: adminSvc, FSM: redis.NewAdminFSM(rdb, adminFSMTTL)}
	return &shopBundle{Shop: shop, Admin: admin, Servers: servers}, nil
}

// startTrafficSync wires the PRD §16.2 traffic-sync worker (kept out of main.go
// for the §1.1 line limit) and returns the WaitGroup the caller drains.
func startTrafficSync(ctx context.Context, cfg *config.Config, db *postgres.Repository, bundle *shopBundle, logger *slog.Logger) *sync.WaitGroup {
	clientRepo := postgres.NewClientRepo(db.DB())
	trafficSvc := trafficsvc.New(clientRepo,
		func(ctx context.Context, serverID int64) (trafficsvc.Panel, error) {
			return bundle.Servers.PanelClient(ctx, serverID)
		},
		cfg.TrafficSyncBatch, cfg.XUIAPITimeout, logger) // Sweep timeout di-sizing untuk fleet: setiap panel ber-budget
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
