// Package adminsvc also hosts the chunked broadcast loop (FR-11).
//
// @file      internal/service/admin/broadcast.go
// @for       FR-11 broadcast: chunked delivery (100 msg / 6 s) + completion report.
// @uses      context, log/slog, runtime/debug, time, internal/repository/redis,
// internal/service/telegram (copy)
// @reason    Broadcast outlives the webhook request, so it runs in a bounded,
//
//	panic-recovered goroutine guarded by a Redis lock (AGENTS.md §1.6).
//
// @author    Dodi Rusmana <rusmanadodi@kentangtechstore.com>
// @layer     service
// @stability experimental
// @since     2026-08-11
package adminsvc

import (
	"context"
	"fmt"
	"runtime/debug"
	"time"

	"github.com/kentangtech/bot-order/internal/repository/redis"
	telegramservice "github.com/kentangtech/bot-order/internal/service/telegram"
)

const (
	broadcastChunk   = 100              // PRD FR-11: 100 pesan per chunk
	broadcastDelay   = 6 * time.Second  // jeda per chunk (rate limit Telegram)
	broadcastTimeout = 15 * time.Minute // batas total satu broadcast (crash guard)
)

// Broadcast starts a chunked broadcast to every registered user and returns
// the target count. The delivery runs in a background goroutine (the webhook
// request must return fast); a Redis lock guarantees only one broadcast at a
// time. The lock TTL equals the whole-broadcast timeout, so a crashed run can
// never wedge future broadcasts.
func (s *Service) Broadcast(ctx context.Context, adminChatID int64, text string) (int, error) {
	total, err := s.users.CountUsers(ctx)
	if err != nil {
		return 0, fmt.Errorf("counting broadcast targets: %w", err)
	}
	if total == 0 {
		return 0, nil
	}
	locked, err := s.lock.AcquireLock(ctx, redis.AdminBroadcastKey(), broadcastTimeout)
	if err != nil {
		return 0, fmt.Errorf("acquiring broadcast lock: %w", err)
	}
	if !locked {
		return 0, ErrBroadcastRunning
	}
	go s.runBroadcast(adminChatID, text, total)
	return int(total), nil
}

// runBroadcast delivers the message in chunks of s.chunk with a pause between
// chunks (Telegram rate limit), then reports the outcome to the admin.
func (s *Service) runBroadcast(adminChatID int64, text string, total int64) {
	defer func() {
		if r := recover(); r != nil {
			s.logger.Error("broadcast panic recovered", "panic", r, "stack", string(debug.Stack()))
		}
	}()
	defer func() {
		if err := s.lock.ReleaseLock(context.Background(), redis.AdminBroadcastKey()); err != nil {
			s.logger.Error("broadcast unlock failed", "error", err)
		}
	}()

	bctx, cancel := context.WithTimeout(s.baseCtx, broadcastTimeout)
	defer cancel()

	sent, failed := 0, 0
	for offset := 0; offset < int(total) && bctx.Err() == nil; offset += s.chunk {
		ids, err := s.users.ListTelegramIDs(bctx, s.chunk, offset)
		if err != nil {
			s.logger.Error("broadcast page failed", "offset", offset, "error", err)
			break // page count unknown — keep the report honest, do not guess
		}
		for _, id := range ids {
			if err := s.send.SendMessage(bctx, id, text, "", nil); err != nil {
				failed++
				s.logger.Debug("broadcast send failed", "user_id", id, "error", err)
			} else {
				sent++
			}
		}
		if len(ids) < s.chunk {
			break
		}
		select {
		case <-bctx.Done():
		case <-time.After(s.delay):
		}
	}

	// Completion report: a failed report is logged, never fatal.
	if err := s.send.SendMessage(context.Background(), adminChatID,
		telegramservice.BroadcastDoneText(sent, failed), "", nil); err != nil {
		s.logger.Error("broadcast completion report failed", "admin_chat", adminChatID, "error", err)
	}
}
