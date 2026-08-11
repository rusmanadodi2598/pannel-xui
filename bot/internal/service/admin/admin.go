// Package adminsvc implements the FR-11 admin operations (M6).
//
// @file      internal/service/admin/admin.go
// @for       FR-11: price management, ban/unban (marker + DB flag), broadcast.
// @uses      context, errors, fmt, log/slog, time, github.com/go-telegram/bot/models,
// internal/domain, internal/repository/postgres, internal/repository/redis
// @reason    Keeps the admin handler thin: every side effect (DB, Redis marker,
//
//	broadcast loop) lives here behind narrow seams (AGENTS.md §1.5).
//
// @author    Dodi Rusmana <rusmanadodi@kentangtechstore.com>
// @layer     service
// @stability experimental
// @since     2026-08-11
package adminsvc

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"time"

	"github.com/go-telegram/bot/models"
	"github.com/kentangtech/bot-order/internal/domain"
	"github.com/kentangtech/bot-order/internal/repository/postgres"
)

// ErrBroadcastRunning is returned when another broadcast is still in flight.
var ErrBroadcastRunning = errors.New("broadcast already running")

// PlanStore persists pricing rows (pricingsvc.Service implements it).
type PlanStore interface {
	ListAll(ctx context.Context) ([]domain.VpnPlan, error)
	Get(ctx context.Context, country string, days int) (*domain.VpnPlan, error)
	SetPrice(ctx context.Context, country string, days int, price domain.Money) error
	SetEnabled(ctx context.Context, country string, days int, enabled bool) error
	Reload(ctx context.Context) error
}

// UserStore persists user rows (usersvc.Service implements it).
type UserStore interface {
	SetBanned(ctx context.Context, tgID int64, banned bool) error
	Get(ctx context.Context, tgID int64) (*postgres.User, error)
	ListTelegramIDs(ctx context.Context, limit, offset int) ([]int64, error)
	CountUsers(ctx context.Context) (int64, error)
}

// BanMarker is the gate-level ban seam (telegramservice.BanService).
type BanMarker interface {
	Ban(ctx context.Context, userID int64) error
	Unban(ctx context.Context, userID int64) error
}

// Messenger sends Telegram messages (repository/telegram.Client).
type Messenger interface {
	SendMessage(ctx context.Context, chatID int64, text string, parseMode models.ParseMode, markup models.ReplyMarkup) error
}

// BroadcastLocker serializes broadcasts (repository/redis.Client).
type BroadcastLocker interface {
	AcquireLock(ctx context.Context, key string, ttl time.Duration) (bool, error)
	ReleaseLock(ctx context.Context, key string) error
} // Service orchestrates FR-11 admin operations.
type Service struct {
	plans   PlanStore
	users   UserStore
	banner  BanMarker
	send    Messenger
	lock    BroadcastLocker
	logger  *slog.Logger
	chunk   int             // broadcast batch (PRD: 100 msg/chunk)
	delay   time.Duration   // pause per chunk (PRD: 6 detik)
	baseCtx context.Context // parent for broadcast goroutines (default Background)
}

// New builds the admin service. chunk/delay follow PRD FR-11 (100 msg / 6 s)
// and are kept as fields so tests can shrink them.
func New(plans PlanStore, users UserStore, banner BanMarker, send Messenger, lock BroadcastLocker, logger *slog.Logger) *Service {
	return &Service{
		plans: plans, users: users, banner: banner, send: send, lock: lock,
		logger: logger, chunk: broadcastChunk, delay: broadcastDelay, baseCtx: context.Background(),
	}
}

// SetShutdownContext ties in-flight broadcasts to the app shutdown signal so a
// graceful stop cancels deliveries promptly instead of running to timeout
// (AGENTS.md §1.6 — goroutine lifecycle). Call once at boot, before any use.
func (s *Service) SetShutdownContext(ctx context.Context) {
	s.baseCtx = ctx
}

// --- pricing (FR-11) ---

func (s *Service) ListPlans(ctx context.Context) ([]domain.VpnPlan, error) {
	return s.plans.ListAll(ctx)
}

func (s *Service) GetPlan(ctx context.Context, country string, days int) (*domain.VpnPlan, error) {
	return s.plans.Get(ctx, country, days)
}

func (s *Service) SetPrice(ctx context.Context, country string, days int, price domain.Money) error {
	return s.plans.SetPrice(ctx, country, days, price)
}

func (s *Service) SetEnabled(ctx context.Context, country string, days int, enabled bool) error {
	return s.plans.SetEnabled(ctx, country, days, enabled)
}

func (s *Service) ReloadPricing(ctx context.Context) error {
	return s.plans.Reload(ctx)
}

// --- users (FR-11) ---

// LookupUser returns the user row for a confirm screen; gorm.ErrRecordNotFound
// means the id never registered (ban still applies to the gate marker).
func (s *Service) LookupUser(ctx context.Context, tgID int64) (*postgres.User, error) {
	return s.users.Get(ctx, tgID)
}

// BanUser bans at both layers: the Redis gate marker (immediate) and the
// persistent DB flag (survives Redis flush; blocks the debit guard too).
// On a DB failure the marker is rolled back so the two layers never diverge
// (a Redis-only ban would silently revert on the next flush — fix review v1.20).
func (s *Service) BanUser(ctx context.Context, tgID int64) error {
	if err := s.banner.Ban(ctx, tgID); err != nil {
		return fmt.Errorf("setting ban marker: %w", err)
	}
	if err := s.users.SetBanned(ctx, tgID, true); err != nil {
		_ = s.banner.Unban(ctx, tgID)
		return fmt.Errorf("setting ban flag: %w", err)
	}
	return nil
}

// UnbanUser lifts both layers.
func (s *Service) UnbanUser(ctx context.Context, tgID int64) error {
	if err := s.banner.Unban(ctx, tgID); err != nil {
		return fmt.Errorf("clearing ban marker: %w", err)
	}
	return s.users.SetBanned(ctx, tgID, false)
}
