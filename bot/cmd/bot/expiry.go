// Package main also hosts the FR-09 expiry-notifier wiring.
//
// @file      cmd/bot/expiry.go
// @for       Wire the H-7/H-3/H-1 expiry reminder interval worker.
// @uses      context, log/slog, sync, time, internal/config, internal/job,
// internal/repository/postgres, internal/repository/telegram, internal/service/expiry
// @reason    Keeps main.go under the 250-line limit (§1.1); mirrors the other
// start* worker helpers split from the composition root (AGENTS.md §1.5).
// @author    Dodi Rusmana <rusmanadodi@kentangtechstore.com>
// @layer     service
// @stability stable
// @since     2026-08-17
package main

import (
	"context"
	"log/slog"
	"sync"
	"time"

	"github.com/kentangtech/bot-order/internal/config"
	"github.com/kentangtech/bot-order/internal/job"
	"github.com/kentangtech/bot-order/internal/repository/postgres"
	telegramrepo "github.com/kentangtech/bot-order/internal/repository/telegram"
	expirysvc "github.com/kentangtech/bot-order/internal/service/expiry"
)

// startExpiryNotify wires the FR-09 H-7/H-3/H-1 reminder worker and returns
// the WaitGroup the caller drains on shutdown. Dates in messages are rendered
// in TIME_LOCATION (FR-09 AC); the loop stops via ctx cancellation.
func startExpiryNotify(ctx context.Context, cfg *config.Config, db *postgres.Repository, tgClient *telegramrepo.Client, logger *slog.Logger) *sync.WaitGroup {
	clientRepo := postgres.NewClientRepo(db.DB())
	expirySvc := expirysvc.New(clientRepo, tgClient, cfg.ExpiryNotifyDays,
		cfg.ExpiryNotifyBatch, cfg.TimeLocation, logger)
	// Timeout per sweep 2 mnt (Telegram calls) — expirysvc menyelesaikan
	// semua ambang dalam satu sweep, bounded (AGENTS.md §1.6).
	notifier := job.NewIntervalWorker(cfg.ExpiryNotifyInterval, 2*time.Minute, expirySvc, logger)
	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		notifier.Run(ctx)
	}()
	logger.Info("expiry notifier started",
		"interval_minutes", cfg.ExpiryNotifyInterval.Minutes(),
		"days", cfg.ExpiryNotifyDays, "batch", cfg.ExpiryNotifyBatch)
	return &wg
}
