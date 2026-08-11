// Package job hosts background workers (FR-09 expiry reminders, traffic sync, M6).
//
// @file      internal/job/interval.go
// @for       Generic interval worker: immediate first sweep, then every interval until ctx done.
// @uses      context, log/slog, runtime/debug, time
// @reason    Periodic proactive work lives off the request path; every sweep is
// bounded and panic-recovered (AGENTS.md §1.6), and the loop terminates on
// context cancellation (no leaked goroutine). Shared by expiry & traffic sync.
//
// @author    Dodi Rusmana <rusmanadodi@kentangtechstore.com>
// @layer     job
// @stability stable
// @since     2026-08-11
package job

import (
	"context"
	"log/slog"
	"runtime/debug"
	"time"
)

// Sweeper performs one bounded sweep (expirysvc.Service / trafficsvc.Service).
type Sweeper interface {
	RunOnce(ctx context.Context) error
}

// IntervalWorker runs the sweep loop: one run immediately, then one per
// interval, until the context is cancelled.
type IntervalWorker struct {
	svc      Sweeper
	interval time.Duration
	timeout  time.Duration // per-sweep budget (bounded outbound calls)
	logger   *slog.Logger
}

// NewIntervalWorker builds the worker. The caller owns the goroutine: start it
// with Run in a `go` statement and cancel ctx on shutdown to drain. The sweep
// timeout is per worker: expiry reminders (Telegram calls) need ~2 min, while
// traffic sync must be sized to the panel fleet (panels × XUI timeout).
func NewIntervalWorker(interval, timeout time.Duration, svc Sweeper, logger *slog.Logger) *IntervalWorker {
	return &IntervalWorker{svc: svc, interval: interval, timeout: timeout, logger: logger}
}

// Run blocks until ctx is cancelled. The first sweep fires immediately so a
// freshly deployed bot does not wait a full interval before the first run.
func (j *IntervalWorker) Run(ctx context.Context) {
	j.sweep(ctx)
	ticker := time.NewTicker(j.interval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			j.sweep(ctx)
		}
	}
}

// sweep runs one bounded, panic-recovered sweep. A panic in the sweep must
// never kill the process (AGENTS.md §1.6 — non-negotiable).
func (j *IntervalWorker) sweep(ctx context.Context) {
	defer func() {
		if r := recover(); r != nil {
			j.logger.Error("sweep panic recovered",
				"panic", r, "stack", string(debug.Stack()))
		}
	}()
	sctx, cancel := context.WithTimeout(ctx, j.timeout)
	defer cancel()
	if err := j.svc.RunOnce(sctx); err != nil {
		j.logger.Error("sweep failed", "error", err)
	}
}

// Sweep runs one sweep immediately (test seam and manual trigger).
func (j *IntervalWorker) Sweep(ctx context.Context) error {
	return j.svc.RunOnce(ctx)
}
