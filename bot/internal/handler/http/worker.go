// Package httphandler serves the /api/v1 HTTP surface (REST convention §26).
//
// @file      internal/handler/http/worker.go
// @for       Bounded update worker pool with per-user serialization lock (PRD §14.2).
// @uses      context, log/slog, sync, time, github.com/go-telegram/bot/models, internal/repository/redis, internal/service/telegram
// @reason    Moves heavy update processing off the HTTP request path so Telegram gets a fast 200.
// @author    Dodi Rusmana <rusmanadodi@kentangtechstore.com>
// @layer     handler
// @stability stable
// @since     2026-08-11
package httphandler

import (
	"context"
	"log/slog"
	"runtime/debug"
	"sync"
	"time"

	"github.com/go-telegram/bot/models"
	telegramhandler "github.com/kentangtech/bot-order/internal/handler/telegram"
	"github.com/kentangtech/bot-order/internal/repository/redis"
	telegramservice "github.com/kentangtech/bot-order/internal/service/telegram"
)

// processTimeout bounds every update-processing run (AGENTS.md §1.6).
const processTimeout = 60 * time.Second

// UpdateProcessor consumes a Telegram update (the dispatcher).
type UpdateProcessor interface {
	Handle(ctx context.Context, upd *models.Update)
}

// LockStore is the per-user lock seam (repository/redis.AcquireLock/ReleaseLock).
type LockStore interface {
	AcquireLock(ctx context.Context, key string, ttl time.Duration) (bool, error)
	ReleaseLock(ctx context.Context, key string) error
}

// Worker processes updates from a bounded queue, serializing per user.
type Worker struct {
	ch     chan *models.Update
	wg     sync.WaitGroup
	proc   UpdateProcessor
	locks  LockStore
	logger *slog.Logger
}

// NewWorker starts size goroutines draining a queue of capacity buffer.
// A full queue drops the update — Telegram retries it automatically.
func NewWorker(size, buffer int, proc UpdateProcessor, locks LockStore, logger *slog.Logger) *Worker {
	w := &Worker{
		ch:     make(chan *models.Update, buffer),
		proc:   proc,
		locks:  locks,
		logger: logger,
	}
	for i := 0; i < size; i++ {
		w.wg.Add(1)
		go w.run()
	}
	return w
}

// Enqueue hands an update to the pool without blocking the HTTP handler.
// It returns false when the queue is full (drop; Telegram will retry).
func (w *Worker) Enqueue(_ context.Context, upd *models.Update) bool {
	select {
	case w.ch <- upd:
		return true
	default:
		w.logger.Warn("update queue full, dropping (Telegram will retry)",
			"update_id", upd.ID, "queue_cap", cap(w.ch))
		return false
	}
}

// Close stops the pool after in-flight updates finish (graceful drain).
func (w *Worker) Close() {
	close(w.ch)
	w.wg.Wait()
}

func (w *Worker) run() {
	defer w.wg.Done()
	for upd := range w.ch {
		ctx, cancel := context.WithTimeout(context.Background(), processTimeout)
		w.processSafely(ctx, upd)
		cancel()
	}
}

// processSafely runs one update and recovers from any panic: a single bad
// update must never kill the process (AGENTS.md §1.6 — non-negotiable).
func (w *Worker) processSafely(ctx context.Context, upd *models.Update) {
	defer func() {
		if r := recover(); r != nil {
			w.logger.Error("update panic recovered",
				"update_id", upd.ID, "panic", r, "stack", string(debug.Stack()))
		}
	}()
	w.process(ctx, upd)
}

// process serializes updates from the same user (PRD §14.2.4) then dispatches.
// The lock is released right after handling so a user's next tap is not dropped;
// the TTL (UserLockTTL) remains only as a crash guard.
func (w *Worker) process(ctx context.Context, upd *models.Update) {
	uid := telegramhandler.UserIDOf(upd)
	if uid == 0 {
		w.proc.Handle(ctx, upd)
		return
	}

	locked, err := w.locks.AcquireLock(ctx, redis.UserLockKey(uid), telegramservice.UserLockTTL)
	if err != nil {
		w.logger.Error("per-user lock failed, dropping update",
			"user_id", uid, "update_id", upd.ID, "error", err)
		return
	}
	if !locked {
		w.logger.Debug("per-user lock busy, skipping update",
			"user_id", uid, "update_id", upd.ID)
		return
	}

	w.proc.Handle(ctx, upd)

	if err := w.locks.ReleaseLock(ctx, redis.UserLockKey(uid)); err != nil {
		// TTL auto-expiry still protects us; a missed release is non-fatal.
		w.logger.Error("per-user unlock failed", "user_id", uid, "error", err)
	}
}
